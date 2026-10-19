package main

import (
	"context"
	"regexp"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// updateEvery is how often the app looks for a new release.
const updateEvery = 6 * time.Hour

// releaseTag matches the version of a release build.
var releaseTag = regexp.MustCompile(`^v(\d+\.\d+\.\d+)$`)

// releaseVersion returns the version of a release build without the v ("0.2.0" for "v0.2.0"); false for dev and
// commit builds, which never update.
func releaseVersion(version string) (string, bool) {
	m := releaseTag.FindStringSubmatch(version)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// assetName is the release asset the updater swaps in on goos/goarch: the bare exe on Windows, the zipped bundle on
// macOS; empty on Linux, whose packages the package manager updates.
func assetName(goos, goarch string) string {
	switch goos {
	case "windows":
		return "standblatt-print-agent-app-windows-" + goarch + ".exe"
	case "darwin":
		return "standblatt-print-agent-app-darwin-" + goarch + ".app.zip"
	}
	return ""
}

// assetMatcher picks the asset called name; Wails' default would take the first asset naming the system, such as
// the DMG or the headless Linux agent.
func assetMatcher(name string) github.AssetMatcher {
	return func(_ updater.CheckRequest, assets []github.ReleaseAsset) int {
		for i, a := range assets {
			if a.Name == name {
				return i
			}
		}
		return -1
	}
}

// setupUpdates turns on in-app updates from the GitHub releases for release builds on Windows and macOS; the
// download is checked against the release's SHA256SUMS.
func setupUpdates(app *application.App, svc *Service) error {
	v, ok := releaseVersion(version)
	name := assetName(runtime.GOOS, runtime.GOARCH)
	if !ok || name == "" {
		return nil
	}
	gh, err := github.New(github.Config{Repository: "standblatt/print-agent", ChecksumAsset: "SHA256SUMS", AssetMatcher: assetMatcher(name)})
	if err != nil {
		return err
	}
	err = app.Updater.Init(updater.Config{CurrentVersion: v, Providers: []updater.Provider{gh}, Window: updater.WindowNone})
	if err != nil {
		return err
	}
	svc.Updates = app.Updater
	return nil
}

// updates is what the service needs of Wails' updater.
type updates interface {
	Check(ctx context.Context) (*updater.Release, error)
	DownloadAndInstall(ctx context.Context) error
	Restart(ctx context.Context) error
	State() updater.State
}

// UpdateState is a newer release the user can install, and how installing it goes.
type UpdateState struct {
	Version string `json:"version"`
	State   string `json:"state"` // the updater's state while installing: downloading, verifying, installing, ready
	Err     string `json:"err"`
}

// watchUpdates looks for a newer release now and every updateEvery until ctx ends; it never installs by itself.
func (s *Service) watchUpdates(ctx context.Context) {
	for {
		rel, err := s.Updates.Check(ctx)
		if err != nil {
			s.Log.Warn("look for updates", "err", err)
		} else if rel != nil {
			s.mu.Lock()
			if s.update == nil {
				s.Log.Info("update available", "version", rel.Version)
				s.update = &UpdateState{Version: rel.Version}
			}
			s.mu.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(updateEvery):
		}
	}
}

// InstallUpdate downloads the newer release, checks it and restarts the app with it.
func (s *Service) InstallUpdate() error {
	s.mu.Lock()
	if s.update == nil {
		s.mu.Unlock()
		return nil
	}
	s.update.Err = ""
	s.mu.Unlock()
	err := s.Updates.DownloadAndInstall(s.ctx)
	if err == nil {
		err = s.Updates.Restart(s.ctx)
	}
	if err != nil {
		s.Log.Error("install update", "err", err)
		s.mu.Lock()
		s.update.Err = err.Error()
		s.mu.Unlock()
	}
	return err
}
