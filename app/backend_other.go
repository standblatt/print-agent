//go:build !windows

package main

import (
	"log/slog"

	"github.com/standblatt/print-agent/internal/agent"
	"github.com/standblatt/print-agent/internal/cups"
)

// newBackend returns the CUPS backend, for running the app on macOS (and for development).
func newBackend(_ string, log *slog.Logger) agent.Backend {
	return cups.CUPS{Log: log}
}
