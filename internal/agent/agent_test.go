package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeBackend struct {
	mu      sync.Mutex
	printed []int64
	fail    error
}

func (f *fakeBackend) Printers(context.Context) ([]Printer, error) {
	if f.fail != nil {
		return nil, f.fail
	}
	return []Printer{{Name: "Office", Options: []Option{{Name: "InputSlot", Label: "Source", Values: []string{"Auto", "Tray2"}, Default: "Auto"}}}}, nil
}

func (f *fakeBackend) Print(_ context.Context, j *Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.printed = append(f.printed, j.ID)
	if j.Title == "broken" {
		return errors.New("paper jam")
	}
	return nil
}

func newAgent(url string, b Backend) *Agent {
	return &Agent{URL: url + "/", Token: "secret", Version: "v1.2.3", Backend: b, Client: http.DefaultClient,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestHello(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/agent/hello" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("unexpected request %s %s %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		json.NewDecoder(r.Body).Decode(&got)
		io.WriteString(w, `{"club":"SG Testdorf","range":"25 m","missingPrinters":["Brother_QL"]}`)
	}))
	defer srv.Close()
	info, err := newAgent(srv.URL, &fakeBackend{}).Hello(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := (Info{Club: "SG Testdorf", Range: "25 m", MissingPrinters: []string{"Brother_QL"}}); !reflect.DeepEqual(info, want) {
		t.Fatalf("info %+v", info)
	}
	if len(got) != 3 || got["version"] != "v1.2.3" || got["os"] != runtime.GOOS {
		t.Fatalf("hello body: %v", got)
	}
	printers := got["printers"].([]any)
	if len(printers) != 1 || printers[0].(map[string]any)["name"] != "Office" {
		t.Fatalf("printers: %v", printers)
	}
}

func TestHelloBackendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request expected when the printers cannot be listed")
	}))
	defer srv.Close()
	_, err := newAgent(srv.URL, &fakeBackend{fail: errors.New("no cups")}).Hello(context.Background())
	if err == nil || err.Error() != "no cups" {
		t.Fatalf("got %v", err)
	}
}

func TestNext(t *testing.T) {
	status := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/agent/jobs/next" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			io.WriteString(w, `{"id":7,"title":"Lane sheet","printer":"Office","options":{"InputSlot":"Tray2"},"pdf":"JVBERg=="}`)
		}
	}))
	defer srv.Close()
	a := newAgent(srv.URL, &fakeBackend{})
	if j, err := a.Next(context.Background()); err != nil || j != nil {
		t.Fatalf("204: %+v %v", j, err)
	}
	status = http.StatusOK
	j, err := a.Next(context.Background())
	if err != nil || j.ID != 7 || j.Printer != "Office" || j.Options["InputSlot"] != "Tray2" || string(j.PDF) != "%PDF" {
		t.Fatalf("200: %+v %v", j, err)
	}
}

func TestUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unknown or revoked agent token", http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := newAgent(srv.URL, &fakeBackend{}).Hello(context.Background())
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("got %v", err)
	}
}

func TestRun(t *testing.T) {
	var mu sync.Mutex
	jobs := []string{`{"id":1,"title":"ok","printer":"Office"}`, `{"id":2,"title":"broken","printer":"Office"}`}
	finished := map[string]map[string]any{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/api/agent/hello":
			io.WriteString(w, `{"club":"C","range":"R","missingPrinters":[]}`)
		case r.URL.Path == "/api/agent/jobs/next" && len(jobs) > 0:
			io.WriteString(w, jobs[0])
			jobs = jobs[1:]
		case r.URL.Path == "/api/agent/jobs/next":
			time.Sleep(10 * time.Millisecond)
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(r.URL.Path, "/api/agent/jobs/"):
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			finished[r.URL.Path] = body
			if len(finished) == 2 {
				cancel()
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	b := &fakeBackend{}
	a := newAgent(srv.URL, b)
	a.Run(ctx)
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("run did not finish both jobs in time")
	}
	st := a.Status()
	if st.Err != "" || time.Since(st.Contact) > time.Second || st.Info.Club != "C" || st.Info.Range != "R" || len(st.Jobs) != 2 ||
		st.Jobs[0].ID != 2 || st.Jobs[0].Err != "paper jam" || st.Jobs[1].ID != 1 || st.Jobs[1].Err != "" || st.Jobs[1].Title != "ok" {
		t.Fatalf("status %+v", st)
	}
	if len(b.printed) != 2 {
		t.Fatalf("printed: %v", b.printed)
	}
	if ok := finished["/api/agent/jobs/1"]; ok["ok"] != true || ok["error"] != nil {
		t.Fatalf("job 1: %v", ok)
	}
	if bad := finished["/api/agent/jobs/2"]; bad["ok"] != false || bad["error"] != "paper jam" {
		t.Fatalf("job 2: %v", bad)
	}
}

func TestRunStopsWhileBackingOff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	a := newAgent(srv.URL, &fakeBackend{})
	go func() {
		a.Run(ctx)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run did not stop during backoff")
	}
	if st := a.Status(); !strings.Contains(st.Err, "down") || st.HTTPStatus != http.StatusBadGateway || st.Contact.IsZero() {
		t.Fatalf("status %+v", st)
	}
}

func TestStatusKeepsLastJobs(t *testing.T) {
	var next atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/agent/hello":
			io.WriteString(w, `{"club":"C","range":"R","missingPrinters":[]}`)
		case "/api/agent/jobs/next":
			if id := next.Add(1); id <= keepJobs+5 {
				fmt.Fprintf(w, `{"id":%d,"title":"t","printer":"Office"}`, id)
				return
			}
			cancel()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	a := newAgent(srv.URL, &fakeBackend{})
	a.Run(ctx)
	jobs := a.Status().Jobs
	if len(jobs) != keepJobs || jobs[0].ID != keepJobs+5 || jobs[keepJobs-1].ID != 6 {
		t.Fatalf("%d jobs, first %d, last %d", len(jobs), jobs[0].ID, jobs[len(jobs)-1].ID)
	}
}

func TestStatusHelloError(t *testing.T) {
	var polls atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/agent/hello" {
			http.Error(w, "version is required", http.StatusUnprocessableEntity)
			return
		}
		if polls.Add(1) == 2 {
			cancel()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	a := newAgent(srv.URL, &fakeBackend{})
	a.Run(ctx)
	if st := a.Status(); !strings.Contains(st.Err, "version is required") || st.HTTPStatus != http.StatusUnprocessableEntity || st.Contact.IsZero() {
		t.Fatalf("a failing hello must stay visible while jobs work: %+v", st)
	}
}

func TestStatusRightAfterHello(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/agent/hello" {
			io.WriteString(w, `{"club":"C","range":"R","missingPrinters":[]}`)
			return
		}
		<-r.Context().Done() // the long poll for jobs
	}))
	defer srv.Close()
	a := newAgent(srv.URL, &fakeBackend{})
	done := make(chan struct{})
	go func() {
		a.Run(ctx)
		close(done)
	}()
	defer func() { cancel(); <-done }()
	for deadline := time.Now().Add(time.Second); ; time.Sleep(5 * time.Millisecond) {
		if st := a.Status(); !st.Contact.IsZero() && st.Info.Club == "C" && st.Err == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no status while the first wait for jobs runs: %+v", a.Status())
		}
	}
}
