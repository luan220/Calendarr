// Client mode: lightweight viewing-PC mode. Only runs the playback helper
// (loopback on :8788) and a tray icon whose left-click opens the calendar of
// a server reachable on the LAN. Use this on machines that do NOT host Sonarr
// — typically a TV-room PC or a laptop on the home network. The Sonarr-box
// runs the same binary in "server" mode.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"fyne.io/systray"

	"calendarr-local/internal/desktop"
	"calendarr-local/internal/discovery"
)

func runClientMode(addr, mpc, cfgPath string, cfg config, notray bool) {
	if _, err := startPlayHelper(addr, mpc); err != nil {
		desktop.MessageBox("Calendarr", "Calendarr is already running on this machine.")
		return
	}

	if notray {
		select {} // dev/preview: block here, no tray
	}
	runClientTray(addr, cfgPath, cfg)
}

// runClientTray installs the system-tray icon for client mode. Left-click
// opens the calendar (auto-discovers a server on the LAN); right-click shows
// the menu (mode toggle, auto-start, quit).
func runClientTray(helperAddr, cfgPath string, cfg config) {
	onReady := func() {
		systray.SetIcon(iconBytes)
		systray.SetTooltip("Calendarr (client) — click to open the calendar")
		systray.SetOnTapped(func() { go openCalendar() })

		mModeServer := systray.AddMenuItemCheckbox("Mode: Server", "Switch to server mode (full Sonarr-connected calendar)", false)
		mModeClient := systray.AddMenuItemCheckbox("Mode: Client", "Playback helper only (current mode)", true)
		systray.AddSeparator()
		mAuto := systray.AddMenuItemCheckbox("Launch automatically when Windows starts", "Launch automatically when Windows starts", desktop.AutoStartEnabled(autostartTaskName))
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Quit Calendarr")

		_ = desktop.RefreshAutoStart(autostartTaskName)

		go func() {
			for {
				select {
				case <-mModeServer.ClickedCh:
					switchModeAndRestart(cfgPath, cfg, modeServer)
				case <-mModeClient.ClickedCh:
					// already in client mode — no-op (keep checkbox checked)
					mModeClient.Check()
				case <-mAuto.ClickedCh:
					enable := !mAuto.Checked()
					if err := desktop.SetAutoStart(autostartTaskName, enable); err != nil {
						desktop.MessageBox("Calendarr", "Auto-start: "+err.Error())
						continue
					}
					if enable {
						mAuto.Check()
					} else {
						mAuto.Uncheck()
					}
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}
	systray.Run(onReady, func() { os.Exit(0) })
}

// openCalendar finds a server and opens the calendar in the browser.
// Best-effort, intended for a left-click on the tray icon.
//  1. server on THIS machine (single-PC install) -> localhost
//  2. otherwise, listen for the LAN beacon of a server elsewhere
func openCalendar() {
	if pingLocalServer("http://127.0.0.1:8787/api/status") {
		desktop.OpenBrowser("http://localhost:8787")
		return
	}
	if url, ok := discovery.Listen(25 * time.Second); ok {
		desktop.OpenBrowser(url)
		return
	}
	log.Printf("no server detected on the network (run Calendarr in server mode on the PC with Sonarr)")
}

func pingLocalServer(url string) bool {
	c := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := c.Get(url)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
