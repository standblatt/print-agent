package main

import (
	_ "embed"
	"strings"
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
// the window, Quit ends the app, and while a newer release is available an item installs it. The frontend sets the
// labels in its language through the service. The menu and its labels are only touched on the main thread.
func newTray(app *application.App, w *application.WebviewWindow, svc *Service) {
	tray := app.SystemTray.New()
	menu := app.NewMenu()
	update := menu.Add("").OnClick(func(*application.Context) { go svc.InstallUpdate() }).SetHidden(true)
	open := menu.Add("Open").OnClick(func(*application.Context) { show(w) })
	quit := menu.Add("Quit").OnClick(func(*application.Context) { app.Quit() })
	tray.SetIcon(trayError).SetMenu(menu).OnClick(func() { show(w) })
	tray.SetTooltip("Standblatt Print Agent")
	updateLabel, shown := "Update to {version}", ""
	svc.OnMenuLabels = func(o, q, u string) {
		application.InvokeSync(func() {
			updateLabel = u
			open.SetLabel(o)
			quit.SetLabel(q)
			update.SetLabel(strings.ReplaceAll(updateLabel, "{version}", shown))
			menu.Update()
		})
	}
	go func() {
		ok := false
		for range time.Tick(2 * time.Second) {
			st := svc.State()
			if now := allOK(st); now != ok {
				ok = now
				tray.SetIcon(map[bool][]byte{true: trayOK, false: trayError}[ok])
			}
			v := ""
			if st.Update != nil {
				v = st.Update.Version
			}
			application.InvokeSync(func() {
				if v != shown {
					shown = v
					update.SetLabel(strings.ReplaceAll(updateLabel, "{version}", v)).SetHidden(v == "")
					menu.Update()
				}
			})
		}
	}()
}
