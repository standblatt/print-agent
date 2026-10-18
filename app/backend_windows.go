package main

import (
	"log/slog"

	"github.com/standblatt/print-agent/internal/agent"
	"github.com/standblatt/print-agent/internal/winspool"
)

// newBackend returns the Windows spooler backend and starts PDFium in the background, so the first job does not
// wait for its compilation; the compiled code is kept in cacheDir.
func newBackend(cacheDir string, log *slog.Logger) agent.Backend {
	r := &winspool.Renderer{CacheDir: cacheDir}
	go func() {
		if err := r.Start(); err != nil {
			log.Error("start pdfium", "err", err)
		}
	}()
	return &winspool.Winspool{Renderer: r}
}
