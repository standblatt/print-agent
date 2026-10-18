package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/standblatt/print-agent/internal/agent"
)

// fakeBackend has one printer and records the jobs it prints.
type fakeBackend struct {
	mu   sync.Mutex
	jobs []agent.Job
}

func (f *fakeBackend) Printers(context.Context) ([]agent.Printer, error) {
	return []agent.Printer{{Name: "Office", Options: []agent.Option{}}}, nil
}

func (f *fakeBackend) Print(_ context.Context, j *agent.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobs = append(f.jobs, *j)
	return nil
}

func (f *fakeBackend) printed() []agent.Job {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]agent.Job{}, f.jobs...)
}

// fakeServer is a Standblatt server with one pairing and one job for the paired agent.
type fakeServer struct {
	*httptest.Server
	confirmed, expired, jobSent atomic.Bool
	finished                    atomic.Value
}

func newFakeServer(t *testing.T) *fakeServer {
	f := &fakeServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		authorized := r.Header.Get("Authorization") == "Bearer tok"
		switch {
		case r.URL.Path == "/api/agent/pair":
			if body["name"] != "RANGE-PC" || body["version"] != "v9" {
				t.Errorf("pair body %v", body)
			}
			io.WriteString(w, `{"code":"K7QD-4MZP","secret":"s3cret","expiresAt":"2030-01-01T00:00:00Z"}`)
		case r.URL.Path == "/api/agent/pair/status" && f.expired.Load():
			http.Error(w, "The code expired.", http.StatusGone)
		case r.URL.Path == "/api/agent/pair/status" && f.confirmed.Load():
			io.WriteString(w, `{"status":"connected","token":"tok","club":"SG Testdorf","range":"25 m"}`)
		case r.URL.Path == "/api/agent/pair/status":
			io.WriteString(w, `{"status":"pending"}`)
		case !authorized:
			http.Error(w, "Unknown or revoked agent token", http.StatusUnauthorized)
		case r.URL.Path == "/api/agent/hello":
			io.WriteString(w, `{"club":"SG Testdorf","range":"25 m","missingPrinters":["Label"]}`)
		case r.URL.Path == "/api/agent/jobs/next" && !f.jobSent.Swap(true):
			io.WriteString(w, `{"id":5,"title":"Lane sheet","printer":"Office","options":{},"pdf":"JVBERg=="}`)
		case r.URL.Path == "/api/agent/jobs/next":
			time.Sleep(20 * time.Millisecond)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/agent/jobs/5":
			f.finished.Store(body)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// newService returns a started service with its config in a temporary directory.
func newService(t *testing.T, dir string, b agent.Backend) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &Service{ConfigPath: filepath.Join(dir, "print-agent.json"), Version: "v9", Hostname: "RANGE-PC", Backend: b,
		Client: http.DefaultClient, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := s.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	return s
}

// eventually fails the test unless ok becomes true within two seconds.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); !ok(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for " + what)
		}
	}
}

func TestServicePairingAndPrinting(t *testing.T) {
	pairPoll = time.Millisecond
	srv := newFakeServer(t)
	dir := t.TempDir()
	b := &fakeBackend{}
	s := newService(t, dir, b)
	if st, _ := s.Settings(); st.Server != defaultServer {
		t.Fatalf("default server %q", st.Server)
	}
	if err := s.SetServer(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	p, err := s.StartPairing()
	if err != nil || p.Code != "K7QD-4MZP" || p.URL != srv.URL+"/connect?code=K7QD-4MZP" {
		t.Fatalf("pairing %+v %v", p, err)
	}
	if st := s.State(); st.Pairing == nil || st.Pairing.Code != p.Code || len(st.Ranges) != 0 {
		t.Fatalf("pending: %+v", st)
	}
	srv.confirmed.Store(true)
	eventually(t, "the range", func() bool { st := s.State(); return st.Pairing == nil && len(st.Ranges) == 1 })
	eventually(t, "the job", func() bool { return srv.finished.Load() != nil })
	if f := srv.finished.Load().(map[string]any); f["ok"] != true || len(b.printed()) != 1 || b.printed()[0].Title != "Lane sheet" {
		t.Fatalf("finished %v, printed %v", f, b.printed())
	}
	eventually(t, "the status", func() bool { return len(s.State().Ranges[0].Status.Jobs) == 1 })
	r := s.State().Ranges[0]
	if r.Club != "SG Testdorf" || r.Range != "25 m" || r.Server != srv.URL || r.Status.Info.MissingPrinters[0] != "Label" || r.Status.Err != "" {
		t.Fatalf("range %+v", r)
	}

	// A restart prints for the saved range again; the token is in the file but never in the state.
	again := newService(t, dir, &fakeBackend{})
	if st := again.State(); len(st.Ranges) != 1 || st.Ranges[0].ID != r.ID {
		t.Fatalf("after restart: %+v", st)
	}
	if b, _ := json.Marshal(s.State()); bytes.Contains(b, []byte("tok")) {
		t.Fatalf("the state shows the token: %s", b)
	}
	if err := again.Remove(r.ID); err != nil {
		t.Fatal(err)
	}
	if st := again.State(); len(st.Ranges) != 0 {
		t.Fatalf("after remove: %+v", st)
	}
	if c, _, _ := loadConfig(again.ConfigPath); len(c.Ranges) != 0 || c.Server != srv.URL {
		t.Fatalf("config after remove: %+v", c)
	}
}

func TestServicePairingEnds(t *testing.T) {
	pairPoll = time.Millisecond
	srv := newFakeServer(t)
	s := newService(t, t.TempDir(), &fakeBackend{})
	s.SetServer(srv.URL)

	if _, err := s.StartPairing(); err != nil {
		t.Fatal(err)
	}
	s.CancelPairing()
	if st := s.State(); st.Pairing != nil {
		t.Fatalf("cancelled: %+v", st.Pairing)
	}

	if _, err := s.StartPairing(); err != nil {
		t.Fatal(err)
	}
	srv.expired.Store(true)
	eventually(t, "the expiry", func() bool { p := s.State().Pairing; return p != nil && p.Expired })
	srv.expired.Store(false)
	srv.confirmed.Store(true)
	time.Sleep(20 * time.Millisecond)
	if st := s.State(); len(st.Ranges) != 0 {
		t.Fatalf("an expired pairing must not connect: %+v", st)
	}

	srv.Close()
	if _, err := s.StartPairing(); err == nil {
		t.Fatal("pairing without a server must fail")
	}
}

func TestServiceSettings(t *testing.T) {
	s := newService(t, t.TempDir(), &fakeBackend{})
	for _, bad := range []string{"", "app.standblatt.ch", "ftp://x", "https://"} {
		if err := s.SetServer(bad); err == nil {
			t.Errorf("server %q accepted", bad)
		}
	}
	if err := s.SetServer(" https://staging.standblatt.ch/ "); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := loadConfig(s.ConfigPath); c.Server != "https://staging.standblatt.ch" {
		t.Fatalf("saved server %q", c.Server)
	}
}

func TestServiceTestPage(t *testing.T) {
	b := &fakeBackend{}
	s := newService(t, t.TempDir(), b)
	if err := s.TestPage("Office"); err != nil {
		t.Fatal(err)
	}
	j := b.printed()
	if len(j) != 1 || j[0].Printer != "Office" || !bytes.HasPrefix(j[0].PDF, []byte("%PDF")) || !bytes.Contains(j[0].PDF, []byte("(RANGE-PC, ")) {
		t.Fatalf("printed %+v", j)
	}
}

func TestServiceBrokenConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "print-agent.json"), []byte("{"), 0o600)
	s := &Service{ConfigPath: filepath.Join(dir, "print-agent.json"), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err == nil || !strings.Contains(err.Error(), "print-agent.json") {
		t.Fatalf("got %v", err)
	}
}
