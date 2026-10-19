package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/standblatt/print-agent/internal/agent"
)

// pairPoll is how often a started pairing asks the server whether it was confirmed.
var pairPoll = 2 * time.Second

// Service is bound to the frontend: the ranges this computer prints for, pairing, printers and settings.
type Service struct {
	ConfigPath string
	Version    string
	Hostname   string // proposed to the server as the agent's name
	Backend    agent.Backend
	Client     *http.Client
	Log        *slog.Logger
	Autostart  *application.AutostartManager // nil in tests
	Updates    updates                       // nil without in-app updates

	OnMenuLabels func(open, quit, update string) // set by the tray
	mu           sync.Mutex
	ctx          context.Context
	cfg          Config
	agents       map[string]running
	pairing      *pairing
	update       *UpdateState
}

// running is the agent of a range and the function that stops it.
type running struct {
	*agent.Agent
	stop context.CancelFunc
}

// pairing is the pairing in progress.
type pairing struct {
	PairingState
	cancel context.CancelFunc
}

// State is everything the window shows; the frontend polls it.
type State struct {
	Ranges  []RangeState  `json:"ranges"`
	Pairing *PairingState `json:"pairing"`
	Update  *UpdateState  `json:"update"`
}

// RangeState is a range this computer prints for and what its agent last saw.
type RangeState struct {
	ID     string       `json:"id"`
	Club   string       `json:"club"`
	Range  string       `json:"range"`
	Server string       `json:"server"`
	Health string       `json:"health"` // see health
	Status agent.Status `json:"status"`
}

// offlineAfter is how long without an answer of the server counts as offline; the server answers a wait for jobs
// within 25 s.
const offlineAfter = 90 * time.Second

// health sums a status up for the window: removed (the server refuses the token), offline (an error, or no answer
// for offlineAfter), starting (no answer yet), missing (printers of the printing setup not found) or ok.
func health(s agent.Status, now time.Time) string {
	switch {
	case s.HTTPStatus == http.StatusUnauthorized:
		return "removed"
	case s.Err != "" || !s.Contact.IsZero() && now.Sub(s.Contact) > offlineAfter:
		return "offline"
	case s.Contact.IsZero():
		return "starting"
	case len(s.Info.MissingPrinters) > 0:
		return "missing"
	}
	return "ok"
}

// PairingState is a pairing in progress: the code to confirm on the server, or why it ended without a range.
type PairingState struct {
	Code    string `json:"code"`
	URL     string `json:"url"` // the server's /connect page with the code
	Expired bool   `json:"expired"`
	Err     string `json:"err"`
}

// Settings are the settings the user can change, and the app's version.
type Settings struct {
	Server    string `json:"server"`
	Autostart bool   `json:"autostart"`
	Version   string `json:"version"`
}

// ServiceStartup loads the config and starts an agent per range; on the first start of a release it turns autostart
// on.
func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	cfg, created, err := loadConfig(s.ConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", s.ConfigPath, err)
	}
	if created {
		if err := saveConfig(s.ConfigPath, cfg); err != nil {
			return err
		}
		// A dev build (go run) must not register its temporary binary.
		if s.Autostart != nil && s.Version != "dev" {
			if err := s.enableAutostart(); err != nil {
				s.Log.Warn("turn autostart on", "err", err)
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx, s.cfg, s.agents = ctx, cfg, map[string]running{}
	for _, r := range cfg.Ranges {
		s.start(r)
	}
	if s.Updates != nil {
		go s.watchUpdates(ctx)
	}
	return nil
}

// newAgent returns an agent for server with token; without token it can only pair.
func (s *Service) newAgent(server, token string, log *slog.Logger) *agent.Agent {
	return &agent.Agent{URL: server, Token: token, Version: s.Version, Backend: s.Backend, Client: s.Client, Log: log}
}

// start runs the agent of r until the app ends or the range is removed; s.mu must be held.
func (s *Service) start(r Range) {
	ctx, stop := context.WithCancel(s.ctx)
	a := s.newAgent(r.Server, r.Token, s.Log.With("club", r.Club, "range", r.Range))
	s.agents[r.ID] = running{a, stop}
	go a.Run(ctx)
}

// State returns the ranges with what their agents last saw, the pairing in progress and a newer release.
func (s *Service) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := State{Ranges: []RangeState{}}
	for _, r := range s.cfg.Ranges {
		as := s.agents[r.ID].Status()
		st.Ranges = append(st.Ranges, RangeState{ID: r.ID, Club: r.Club, Range: r.Range, Server: r.Server, Health: health(as, time.Now()), Status: as})
	}
	if s.pairing != nil {
		p := s.pairing.PairingState
		st.Pairing = &p
	}
	if s.update != nil {
		u := *s.update
		switch us := s.Updates.State(); us {
		case updater.StateDownloading, updater.StateVerifying, updater.StateInstalling, updater.StateReady:
			u.State = string(us)
		}
		st.Update = &u
	}
	return st
}

// StartPairing asks the server for a code and waits in the background until a member confirms it on the server's
// /connect page; then the range is saved and its agent starts. A pairing in progress is cancelled.
func (s *Service) StartPairing() (PairingState, error) {
	s.CancelPairing()
	s.mu.Lock()
	server, ctx := s.cfg.Server, s.ctx
	s.mu.Unlock()
	a := s.newAgent(server, "", s.Log)
	start, cancelStart := context.WithTimeout(ctx, 30*time.Second)
	defer cancelStart()
	p, err := a.Pair(start, s.Hostname)
	if err != nil {
		return PairingState{}, err
	}
	pctx, cancel := context.WithCancel(ctx)
	state := PairingState{Code: p.Code, URL: strings.TrimSuffix(server, "/") + "/connect?code=" + url.QueryEscape(p.Code)}
	s.mu.Lock()
	s.pairing = &pairing{state, cancel}
	current := s.pairing
	s.mu.Unlock()
	go func() {
		paired, err := a.WaitPaired(pctx, p.Secret, pairPoll)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.pairing != current {
			return
		}
		if err != nil {
			s.pairing.Expired = errors.Is(err, agent.ErrPairingExpired)
			s.pairing.Err = err.Error()
			return
		}
		r := Range{ID: rand.Text()[:10], Server: server, Club: paired.Club, Range: paired.Range, Token: paired.Token}
		cfg := s.cfg
		cfg.Ranges = append(slices.Clone(cfg.Ranges), r)
		if err := saveConfig(s.ConfigPath, cfg); err != nil {
			s.pairing.Err = err.Error()
			return
		}
		s.cfg, s.pairing = cfg, nil
		s.start(r)
		s.Log.Info("range connected", "club", r.Club, "range", r.Range)
	}()
	return state, nil
}

// CancelPairing stops the pairing in progress, if any.
func (s *Service) CancelPairing() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pairing != nil {
		s.pairing.cancel()
		s.pairing = nil
	}
}

// Remove stops printing for a range and forgets its token; the agent stays listed on the server until a member
// deletes it there.
func (s *Service) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.cfg
	cfg.Ranges = slices.DeleteFunc(slices.Clone(cfg.Ranges), func(r Range) bool { return r.ID == id })
	if err := saveConfig(s.ConfigPath, cfg); err != nil {
		return err
	}
	s.cfg = cfg
	if a, ok := s.agents[id]; ok {
		a.stop()
		delete(s.agents, id)
	}
	return nil
}

// Printers lists the printers of this computer with their options.
func (s *Service) Printers() ([]agent.Printer, error) {
	return s.Backend.Printers(s.ctx)
}

// TestPage prints a label-sized test page on printer with its default options, without the server.
func (s *Service) TestPage(printer string) error {
	pdf := testPage("Standblatt test page", printer, s.Hostname+", "+time.Now().Format("2006-01-02 15:04"))
	return s.Backend.Print(s.ctx, &agent.Job{Title: "Standblatt test page", Printer: printer, Options: map[string]string{}, PDF: pdf})
}

// Settings returns the server for new pairings, whether the app starts with the computer and the app's version.
func (s *Service) Settings() Settings {
	s.mu.Lock()
	st := Settings{Server: s.cfg.Server, Version: s.Version}
	s.mu.Unlock()
	if s.Autostart != nil {
		on, err := s.Autostart.IsEnabled()
		if err != nil {
			s.Log.Warn("autostart status", "err", err)
		}
		st.Autostart = on
	}
	return st
}

// SetServer sets the server new ranges are paired with; connected ranges keep theirs.
func (s *Service) SetServer(server string) error {
	server = strings.TrimSuffix(strings.TrimSpace(server), "/")
	if u, err := url.Parse(server); err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" {
		return fmt.Errorf("%q is not a web address", server)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.cfg
	cfg.Server = server
	if err := saveConfig(s.ConfigPath, cfg); err != nil {
		return err
	}
	s.cfg = cfg
	return nil
}

// hiddenArg starts the app in the tray, as autostart does.
const hiddenArg = "-hidden"

// enableAutostart starts the app with the computer, in the tray.
func (s *Service) enableAutostart() error {
	return s.Autostart.EnableWithOptions(application.AutostartOptions{Arguments: []string{hiddenArg}})
}

// SetAutostart turns starting the app with the computer on or off.
func (s *Service) SetAutostart(on bool) error {
	if on {
		return s.enableAutostart()
	}
	return s.Autostart.Disable()
}

// SetMenuLabels sets the labels of the tray menu in the language of the window; update contains {version}.
func (s *Service) SetMenuLabels(open, quit, update string) {
	s.OnMenuLabels(open, quit, update)
}
