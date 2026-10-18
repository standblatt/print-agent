package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
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
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	if err := newAgent(srv.URL, &fakeBackend{}).Hello(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got["version"] != "v1.2.3" || got["os"] != runtime.GOOS {
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
	err := newAgent(srv.URL, &fakeBackend{fail: errors.New("no cups")}).Hello(context.Background())
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
	err := newAgent(srv.URL, &fakeBackend{}).Hello(context.Background())
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
			w.WriteHeader(http.StatusNoContent)
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
	newAgent(srv.URL, b).Run(ctx)
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("run did not finish both jobs in time")
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
	go func() {
		newAgent(srv.URL, &fakeBackend{}).Run(ctx)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run did not stop during backoff")
	}
}
