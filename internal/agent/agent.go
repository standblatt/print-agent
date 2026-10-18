// Package agent is the protocol client of the print agent: it reports the local printers to a Standblatt server,
// waits for jobs and hands them to a Backend. It only makes outgoing requests.
package agent

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"sync"
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

	mu     sync.Mutex
	status Status
}

// Status is what Run last saw, for the desktop app to show.
type Status struct {
	Contact    time.Time   `json:"contact"`    // last answer of the server
	Err        string      `json:"err"`        // last error of hello or waiting for jobs, empty when both work
	HTTPStatus int         `json:"httpStatus"` // status code of Err, 0 for none or a network error
	Info       Info        `json:"info"`       // last answer to hello
	Jobs       []JobResult `json:"jobs"`       // the last jobs, newest first
}

// JobResult is a job Run printed or failed to print.
type JobResult struct {
	ID      int64     `json:"id"`
	Title   string    `json:"title"`
	Printer string    `json:"printer"`
	At      time.Time `json:"at"`
	Err     string    `json:"err"`
}

// keepJobs is how many jobs Status keeps.
const keepJobs = 20

// record notes the outcome of a request in the status: an answer of the server, even an error status, is contact.
func (a *Agent) record(err error) {
	a.update(func(s *Status) {
		var he *HTTPError
		if err == nil || errors.As(err, &he) {
			s.Contact = time.Now()
		}
		s.Err, s.HTTPStatus = "", 0
		if err != nil {
			s.Err = err.Error()
		}
		if he != nil {
			s.HTTPStatus = he.Status
		}
	})
}

// Status returns a copy of what Run last saw.
func (a *Agent) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.status
	s.Jobs = append([]JobResult{}, s.Jobs...)
	return s
}

// update changes the status under the lock.
func (a *Agent) update(f func(s *Status)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f(&a.status)
}

// HTTPError is an answer of the server with a status code of 300 or above.
type HTTPError struct {
	Method, Path string
	Status       int
	StatusText   string
	Body         string
}

// Error tells the request, the status and the server's message.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: %s %s", e.Method, e.Path, e.StatusText, e.Body)
}

// request sends a request with an optional JSON body, with the agent token when there is one; status codes of 300
// and above become an *HTTPError.
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
	if a.Token != "" {
		req.Header.Set("Authorization", "Bearer "+a.Token)
	}
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
		return nil, &HTTPError{Method: method, Path: path, Status: resp.StatusCode, StatusText: resp.Status, Body: strings.TrimSpace(string(b))}
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
	var helloErr error
	backoff := time.Second
	for ctx.Err() == nil {
		if time.Since(lastHello) > 5*time.Minute {
			info, err := a.Hello(ctx)
			if helloErr = err; err != nil {
				a.Log.Warn("report printers", "err", err)
			} else {
				lastHello = time.Now()
				a.update(func(s *Status) { s.Info = info })
			}
			a.record(err)
		}
		j, err := a.Next(ctx)
		if errors.Is(err, context.Canceled) {
			return
		}
		// A failing hello stays visible while waiting for jobs works.
		a.record(cmp.Or(err, helloErr))
		if err != nil {
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
		r := JobResult{ID: j.ID, Title: j.Title, Printer: j.Printer, At: time.Now()}
		if printErr != nil {
			r.Err = printErr.Error()
		}
		a.update(func(s *Status) { s.Jobs = append([]JobResult{r}, s.Jobs[:min(len(s.Jobs), keepJobs-1)]...) })
		if err := a.Finish(ctx, j.ID, printErr); err != nil {
			a.Log.Warn("report job", "job", j.ID, "err", err)
		}
	}
}
