// Command app is the Standblatt Print Agent desktop app: it pairs the computer with ranges in Standblatt and prints
// their jobs on the local printers.
package main

import (
	"embed"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

var version = "dev"

// main sets up the log and the service and runs the app until it quits.
func main() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Fatal(err)
	}
	cacheDir = filepath.Join(cacheDir, "Standblatt")
	logFile, err := openLog(filepath.Join(cacheDir, "print-agent.log"))
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	logger := slog.New(slog.NewTextHandler(logFile, nil))
	log.SetOutput(logFile)
	hostname, _ := os.Hostname()
	logger.Info("print agent started", "version", version)

	svc := &Service{
		ConfigPath: filepath.Join(configDir, "Standblatt", "print-agent.json"),
		Version:    version,
		Hostname:   hostname,
		Backend:    newBackend(filepath.Join(cacheDir, "pdfium"), logger),
		Client:     &http.Client{Timeout: 60 * time.Second},
		Log:        logger,
	}
	app := application.New(application.Options{
		Name:     "Standblatt Print Agent",
		Services: []application.Service{application.NewService(svc)},
		Assets:   application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		// Wails logs every asset request at info.
		Logger: slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelWarn})),
	})
	svc.Autostart = app.Autostart
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Standblatt Print Agent",
		Width:  720,
		Height: 640,
		URL:    "/",
	})
	if err := app.Run(); err != nil {
		logger.Error("app", "err", err)
		os.Exit(1)
	}
}
