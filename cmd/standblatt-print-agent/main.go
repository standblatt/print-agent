// Command standblatt-print-agent prints Standblatt documents on the local printers of a range.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/standblatt/print-agent/internal/agent"
	"github.com/standblatt/print-agent/internal/cups"
)

var version = "dev"

// main configures the agent from the environment and runs it until SIGINT or SIGTERM, one loop per token
// (STANDBLATT_AGENT_TOKEN may list several, comma separated, to serve several ranges from one computer).
func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	url := os.Getenv("STANDBLATT_URL")
	var tokens []string
	for t := range strings.SplitSeq(os.Getenv("STANDBLATT_AGENT_TOKEN"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			tokens = append(tokens, t)
		}
	}
	if url == "" || len(tokens) == 0 {
		log.Error("STANDBLATT_URL and STANDBLATT_AGENT_TOKEN are required")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Info("print agent started", "version", version, "server", url, "ranges", len(tokens))
	var wg sync.WaitGroup
	for i, token := range tokens {
		l := log.With("token", i+1)
		a := &agent.Agent{
			URL:     url,
			Token:   token,
			Version: version,
			Backend: cups.CUPS{Log: l},
			Client:  &http.Client{Timeout: 60 * time.Second},
			Log:     l,
		}
		wg.Go(func() { a.Run(ctx) })
	}
	wg.Wait()
}
