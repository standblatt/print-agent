// Command app is the Standblatt Print Agent desktop app: it pairs the computer with ranges in Standblatt and prints
// their jobs on the local printers.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

// main opens the window and runs the app until it quits.
func main() {
	app := application.New(application.Options{
		Name:   "Standblatt Print Agent",
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Standblatt Print Agent",
		Width:  720,
		Height: 640,
		URL:    "/",
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
