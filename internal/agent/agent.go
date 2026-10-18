// Package agent is the protocol client of the print agent: it reports the local printers to a Standblatt server,
// waits for jobs and hands them to a Backend. It only makes outgoing requests.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// Printer is a local printer with its options, as sent to the server.
type Printer struct {
	Name    string   `json:"name"`
	Options []Option `json:"options"`
}

// Option is a printer option: name, label, possible values and the default.
type Option struct {
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	Values  []string `json:"values"`
	Default string   `json:"default"`
}

// Job is a print job handed out by the server.
type Job struct {
	ID      int64             `json:"id"`
	Title   string            `json:"title"`
	Printer string            `json:"printer"`
	Options map[string]string `json:"options"`
	PDF     []byte            `json:"pdf"`
}

// Backend lists the local printers and prints jobs on them.
type Backend interface {
	Printers(ctx context.Context) ([]Printer, error)
	Print(ctx context.Context, j *Job) error
}

// Info is what the server answers a hello: where the agent prints and which routed printers it lacks.
type Info struct {
	Club            string   `json:"club"`
	Range           string   `json:"range"`
	MissingPrinters []string `json:"missingPrinters"`
}

// Agent talks to one Standblatt server with the token of one range.
type Agent struct {
	URL     string
	Token   string
	Version string
	Backend Backend
	Client  *http.Client
	Log     *slog.Logger
}

// request sends an authenticated request with an optional JSON body; status codes of 300 and above become errors.
func (a *Agent) request(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(a.URL, "/")+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("%s %s: %s %s", method, path, resp.Status, strings.TrimSpace(string(b)))
	}
	return resp, nil
}

// Hello reports the printers, the agent's version and the operating system to the server and returns its answer.
func (a *Agent) Hello(ctx context.Context) (Info, error) {
	var info Info
	printers, err := a.Backend.Printers(ctx)
	if err != nil {
		return info, err
	}
	resp, err := a.request(ctx, http.MethodPost, "/api/agent/hello",
		map[string]any{"printers": printers, "version": a.Version, "os": runtime.GOOS})
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return info, fmt.Errorf("hello answer: %w", err)
	}
	a.Log.Info("printers reported", "count", len(printers), "club", info.Club, "range", info.Range)
	if len(info.MissingPrinters) > 0 {
		a.Log.Warn("printers of the printing setup not found", "printers", info.MissingPrinters)
	}
	return info, nil
}

// Next waits for the next job; nil means none arrived within the server's wait time.
func (a *Agent) Next(ctx context.Context) (*Job, error) {
	resp, err := a.request(ctx, http.MethodGet, "/api/agent/jobs/next", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	var j Job
	return &j, json.NewDecoder(resp.Body).Decode(&j)
}

// Finish reports the outcome of a job.
func (a *Agent) Finish(ctx context.Context, id int64, printErr error) error {
	body := map[string]any{"ok": printErr == nil}
	if printErr != nil {
		body["error"] = printErr.Error()
	}
	resp, err := a.request(ctx, http.MethodPost, fmt.Sprintf("/api/agent/jobs/%d", id), body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Run reports the printers every five minutes and prints jobs until ctx ends.
func (a *Agent) Run(ctx context.Context) {
	lastHello := time.Time{}
	backoff := time.Second
	for ctx.Err() == nil {
		if time.Since(lastHello) > 5*time.Minute {
			if _, err := a.Hello(ctx); err != nil {
				a.Log.Warn("report printers", "err", err)
			} else {
				lastHello = time.Now()
			}
		}
		j, err := a.Next(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.Log.Warn("waiting for jobs", "err", err, "retry", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, time.Minute)
			continue
		}
		backoff = time.Second
		if j == nil {
			continue
		}
		printErr := a.Backend.Print(ctx, j)
		if printErr != nil {
			a.Log.Warn("print failed", "job", j.ID, "printer", j.Printer, "err", printErr)
		} else {
			a.Log.Info("printed", "job", j.ID, "printer", j.Printer, "title", j.Title)
		}
		if err := a.Finish(ctx, j.ID, printErr); err != nil {
			a.Log.Warn("report job", "job", j.ID, "err", err)
		}
	}
}
