package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPair(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path != "/api/agent/pair" || r.Header.Get("Authorization") != "" ||
			body["name"] != "DESKTOP-RANGE" || body["os"] != runtime.GOOS || body["version"] != "v1.2.3" {
			t.Errorf("unexpected request %s %q %v", r.URL.Path, r.Header.Get("Authorization"), body)
		}
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"code":"K7QD-4MZP","secret":"s3cret","expiresAt":"2026-10-18T22:00:00Z"}`)
	}))
	defer srv.Close()
	a := newAgent(srv.URL, &fakeBackend{})
	a.Token = ""
	p, err := a.Pair(context.Background(), "DESKTOP-RANGE")
	if err != nil || p.Code != "K7QD-4MZP" || p.Secret != "s3cret" || p.ExpiresAt.IsZero() {
		t.Fatalf("pairing %+v %v", p, err)
	}
}

// statusServer answers the polls with the given status codes and bodies in turn, the last one for ever.
func statusServer(t *testing.T, answers ...string) (*httptest.Server, *atomic.Int32) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path != "/api/agent/pair/status" || body["secret"] != "s3cret" {
			t.Errorf("unexpected poll %s %v", r.URL.Path, body)
		}
		n := int(calls.Add(1)) - 1
		code, answer, _ := strings.Cut(answers[min(n, len(answers)-1)], " ")
		switch code {
		case "410":
			http.Error(w, answer, http.StatusGone)
		case "404":
			http.Error(w, answer, http.StatusNotFound)
		case "502":
			http.Error(w, answer, http.StatusBadGateway)
		default:
			io.WriteString(w, answer)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestWaitPaired(t *testing.T) {
	ctx := context.Background()
	srv, calls := statusServer(t, `200 {"status":"pending"}`, `502 down`, `200 {"status":"connected","token":"tok","club":"SG Testdorf","range":"25 m"}`)
	p, err := newAgent(srv.URL, &fakeBackend{}).WaitPaired(ctx, "s3cret", time.Millisecond)
	if err != nil || p != (Paired{Token: "tok", Club: "SG Testdorf", Range: "25 m"}) || calls.Load() != 3 {
		t.Fatalf("paired %+v %v after %d polls", p, err, calls.Load())
	}

	srv, _ = statusServer(t, `200 {"status":"pending"}`, `410 The code expired.`)
	if _, err := newAgent(srv.URL, &fakeBackend{}).WaitPaired(ctx, "s3cret", time.Millisecond); !errors.Is(err, ErrPairingExpired) {
		t.Fatalf("expired: %v", err)
	}

	srv, _ = statusServer(t, `404 Unknown pairing`)
	if _, err := newAgent(srv.URL, &fakeBackend{}).WaitPaired(ctx, "s3cret", time.Millisecond); err == nil || !strings.Contains(err.Error(), "Unknown pairing") {
		t.Fatalf("unknown: %v", err)
	}

	srv, _ = statusServer(t, `200 {"status":"pending"}`)
	cctx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := newAgent(srv.URL, &fakeBackend{}).WaitPaired(cctx, "s3cret", time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled: %v", err)
	}
}
