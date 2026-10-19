package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

func TestReleaseVersion(t *testing.T) {
	for in, want := range map[string]string{"v0.2.0": "0.2.0", "v1.10.3": "1.10.3", "dev": "", "3f4b400": "", "v0.2.0-3-gabc1234": "", "v0.2.0-dirty": "", "0.2.0": ""} {
		if got, ok := releaseVersion(in); got != want || ok != (want != "") {
			t.Errorf("%q: %q %v, want %q", in, got, ok, want)
		}
	}
}

func TestAssetMatcher(t *testing.T) {
	var assets []github.ReleaseAsset
	for _, n := range []string{
		"SHA256SUMS", "Standblatt-Print-Agent-Setup.exe",
		"standblatt-print-agent-app-0.2.0-1.aarch64.rpm", "standblatt-print-agent-app-0.2.0-1.x86_64.rpm",
		"standblatt-print-agent-app-darwin-amd64.dmg", "standblatt-print-agent-app-darwin-arm64.dmg",
		"standblatt-print-agent-app-darwin-amd64.app.zip", "standblatt-print-agent-app-darwin-arm64.app.zip",
		"standblatt-print-agent-app-windows-amd64.exe", "standblatt-print-agent-app_0.2.0_amd64.deb",
		"standblatt-print-agent-app_0.2.0_arm64.deb", "standblatt-print-agent-linux-amd64", "standblatt-print-agent-linux-arm64",
	} {
		assets = append(assets, github.ReleaseAsset{Name: n})
	}
	for _, c := range []struct{ goos, goarch, want string }{
		{"windows", "amd64", "standblatt-print-agent-app-windows-amd64.exe"},
		{"darwin", "arm64", "standblatt-print-agent-app-darwin-arm64.app.zip"},
		{"darwin", "amd64", "standblatt-print-agent-app-darwin-amd64.app.zip"},
		{"linux", "amd64", ""},
		{"windows", "arm64", ""},
	} {
		name := assetName(c.goos, c.goarch)
		i := -1
		if name != "" {
			i = assetMatcher(name)(updater.CheckRequest{}, assets)
		}
		got := ""
		if i >= 0 {
			got = assets[i].Name
		}
		if got != c.want {
			t.Errorf("%s/%s: %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

// fakeUpdates offers one release and records what the service asks of it.
type fakeUpdates struct {
	mu                sync.Mutex
	checks, downloads int
	restarted         bool
	downloadErr       error
	state             updater.State
}

func (f *fakeUpdates) Check(context.Context) (*updater.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks++
	return &updater.Release{Version: "0.2.1"}, nil
}

func (f *fakeUpdates) DownloadAndInstall(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downloads++
	f.state = updater.StateReady
	return f.downloadErr
}

func (f *fakeUpdates) Restart(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restarted = true
	return nil
}

func (f *fakeUpdates) State() updater.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

// startWithUpdates starts a service with u as its updater (nil: no in-app updates).
func startWithUpdates(t *testing.T, u *fakeUpdates) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &Service{ConfigPath: filepath.Join(t.TempDir(), "print-agent.json"), Backend: &fakeBackend{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if u != nil {
		s.Updates = u
	}
	if err := s.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestServiceUpdate(t *testing.T) {
	u := &fakeUpdates{downloadErr: errors.New("checksum mismatch")}
	s := startWithUpdates(t, u)
	eventually(t, "the update", func() bool { return s.State().Update != nil })
	if up := s.State().Update; up.Version != "0.2.1" || up.State != "" || up.Err != "" {
		t.Fatalf("available: %+v", up)
	}

	if err := s.InstallUpdate(); err == nil || u.restarted {
		t.Fatalf("a failed download must not restart: %v", err)
	}
	if up := s.State().Update; up.Err != "checksum mismatch" || up.State != "ready" {
		t.Fatalf("after the failure: %+v", up)
	}

	u.downloadErr = nil
	if err := s.InstallUpdate(); err != nil || !u.restarted || u.downloads != 2 {
		t.Fatalf("install: %v, restarted %v, downloads %d", err, u.restarted, u.downloads)
	}
	if up := s.State().Update; up.Err != "" {
		t.Fatalf("a retry clears the error: %+v", up)
	}
}

func TestServiceWithoutUpdates(t *testing.T) {
	s := startWithUpdates(t, nil)
	if err := s.InstallUpdate(); err != nil || s.State().Update != nil {
		t.Fatalf("without updates: %v %+v", err, s.State().Update)
	}
}
