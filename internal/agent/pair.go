package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"time"
)

// Pairing is a started pairing: the code the user confirms on /connect and the secret the agent polls with.
type Pairing struct {
	Code      string    `json:"code"`
	Secret    string    `json:"secret"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Paired is a confirmed pairing: the agent token and where the agent prints.
type Paired struct {
	Token string `json:"token"`
	Club  string `json:"club"`
	Range string `json:"range"`
}

// ErrPairingExpired means nobody confirmed the code in time.
var ErrPairingExpired = errors.New("the pairing code expired")

// post sends a JSON body to path and decodes the answer into out.
func (a *Agent) post(ctx context.Context, path string, body, out any) error {
	resp, err := a.request(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

// Pair starts pairing this computer, named name, with the server; it needs no token.
func (a *Agent) Pair(ctx context.Context, name string) (Pairing, error) {
	var p Pairing
	err := a.post(ctx, "/api/agent/pair", map[string]string{"name": name, "os": runtime.GOOS, "version": a.Version}, &p)
	return p, err
}

// WaitPaired polls the pairing every interval until a member confirms it, it expires or ctx ends; network errors
// and server errors are retried, since the user may still be confirming.
func (a *Agent) WaitPaired(ctx context.Context, secret string, interval time.Duration) (Paired, error) {
	for {
		var s struct {
			Status string `json:"status"`
			Paired
		}
		err := a.post(ctx, "/api/agent/pair/status", map[string]string{"secret": secret}, &s)
		var he *HTTPError
		switch {
		case err == nil && s.Status == "connected":
			return s.Paired, nil
		case errors.As(err, &he) && he.Status == http.StatusGone:
			return Paired{}, ErrPairingExpired
		case errors.As(err, &he) && he.Status < 500:
			return Paired{}, err
		case err != nil:
			a.Log.Warn("waiting for the pairing", "err", err)
		}
		select {
		case <-ctx.Done():
			return Paired{}, ctx.Err()
		case <-time.After(interval):
		}
	}
}
