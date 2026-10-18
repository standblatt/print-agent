// Command app is the Standblatt Print Agent desktop app: it pairs the computer with ranges in Standblatt and prints
// their jobs on the local printers.
package main

import (
	"embed"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

var version = "dev"

// main sets up the log, the service, the window and the tray and runs the app until Quit in the tray; closing the
// window only hides it. A second start shows the window of the first; -hidden (from autostart) starts in the tray.
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
	out := io.Writer(logFile)
	if version == "dev" {
		out = io.MultiWriter(logFile, os.Stderr)
	}
	logger := slog.New(slog.NewTextHandler(out, nil))
	log.SetOutput(out)
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
	var window *application.WebviewWindow
	app := application.New(application.Options{
		Name:     "Standblatt Print Agent",
		Services: []application.Service{application.NewService(svc)},
		Assets:   application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Logger:   logger,
		LogLevel: slog.LevelWarn, // Wails logs every asset request at info
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               "ch.standblatt.printagent",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { show(window) },
		},
	})
	svc.Autostart = app.Autostart
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Standblatt Print Agent",
		Width:  720,
		Height: 640,
		URL:    "/",
		Hidden: slices.Contains(os.Args[1:], hiddenArg),
	})
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})
	newTray(app, window, svc)
	if err := app.Run(); err != nil {
		logger.Error("app", "err", err)
		os.Exit(1)
	}
}
