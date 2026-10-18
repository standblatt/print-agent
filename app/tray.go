package main

import (
	_ "embed"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	//go:embed build/tray-ok.png
	trayOK []byte
	//go:embed build/tray-error.png
	trayError []byte
)

// allOK tells whether every range prints, which the tray shows green; without a range nothing prints.
func allOK(st State) bool {
	for _, r := range st.Ranges {
		if r.Health != "ok" {
			return false
		}
	}
	return len(st.Ranges) > 0
}

// show brings the window to the front.
func show(w *application.WebviewWindow) {
	w.Show()
	w.Restore()
	w.Focus()
}

// newTray puts the app into the system tray: green when every range prints, red otherwise; a click or Open shows
// the window, Quit ends the app. The frontend sets the labels in its language through the service.
func newTray(app *application.App, w *application.WebviewWindow, svc *Service) {
	tray := app.SystemTray.New()
	menu := app.NewMenu()
	open := menu.Add("Open").OnClick(func(*application.Context) { show(w) })
	quit := menu.Add("Quit").OnClick(func(*application.Context) { app.Quit() })
	tray.SetIcon(trayError).SetMenu(menu).OnClick(func() { show(w) })
	tray.SetTooltip("Standblatt Print Agent")
	svc.OnMenuLabels = func(o, q string) {
		application.InvokeSync(func() {
			open.SetLabel(o)
			quit.SetLabel(q)
			menu.Update()
		})
	}
	go func() {
		ok := false
		for range time.Tick(2 * time.Second) {
			if now := allOK(svc.State()); now != ok {
				ok = now
				tray.SetIcon(map[bool][]byte{true: trayOK, false: trayError}[ok])
			}
		}
	}()
}
