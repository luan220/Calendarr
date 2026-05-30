//go:build windows

// Windows system-tray integration (server + client modes). Kept in a
// Windows-only file so the rest of the program builds headless on Linux/macOS,
// where fyne/systray needs native GUI libraries that can't be cross-compiled.
package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"os"
	"os/exec"

	"fyne.io/systray"

	"calendarr-local/internal/desktop"
)

//go:embed icon.ico
var iconBytes []byte

// runTray installs the system-tray icon for server mode. Left-click opens
// the calendar in the browser; right-click shows the menu (mode toggle,
// auto-start, close).
func runTray(port, cfgPath string, cfg config) {
	_ = desktop.RefreshAutoStart(autostartTaskName)
	onReady := func() {
		systray.SetIcon(iconBytes)
		systray.SetTooltip("Calendarr — server (click to open the calendar)")
		// Left-click opens the local UI. We do not set SetOnSecondaryTapped,
		// so right-click keeps the default menu.
		systray.SetOnTapped(func() { desktop.OpenBrowser("http://localhost:" + port) })

		mModeServer := systray.AddMenuItemCheckbox("Mode: Server", "Full Sonarr-connected calendar (current mode)", true)
		mModeClient := systray.AddMenuItemCheckbox("Mode: Client", "Switch to client mode (playback helper only)", false)
		systray.AddSeparator()
		mAuto := systray.AddMenuItemCheckbox("Launch automatically when Windows starts", "Launch automatically when Windows starts", desktop.AutoStartEnabled(autostartTaskName))
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Stop the Calendarr server")

		go func() {
			for {
				select {
				case <-mModeServer.ClickedCh:
					// already in server mode — no-op (keep checkbox checked)
					mModeServer.Check()
				case <-mModeClient.ClickedCh:
					switchModeAndRestart(cfgPath, cfg, modeClient)
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

// switchModeAndRestart writes the new mode to config.json, spawns a fresh
// instance of the binary, and exits the current process. The child process
// re-reads config.json at startup and boots in the chosen mode. The brief
// gap during the swap is handled by bindWithRetry on the child side.
func switchModeAndRestart(cfgPath string, cfg config, newMode string) {
	cfg.Mode = newMode
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		desktop.MessageBox("Calendarr", "Mode switch failed: "+err.Error())
		return
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		desktop.MessageBox("Calendarr", "Mode switch failed: "+err.Error())
		return
	}
	exe, err := os.Executable()
	if err != nil {
		desktop.MessageBox("Calendarr", "Mode switch failed: "+err.Error())
		return
	}
	// Plain spawn: under -H=windowsgui there is no console to hide or detach
	// from, and the child outlives this process on its own. Avoiding
	// HideWindow / DETACHED_PROCESS keeps the relaunch from resembling a
	// dropper to antivirus heuristics.
	cmd := exec.Command(exe)
	if err := cmd.Start(); err != nil {
		desktop.MessageBox("Calendarr", "Mode switch failed: "+err.Error())
		return
	}
	log.Printf("switching to %s mode — restarting", newMode)
	systray.Quit()
	os.Exit(0)
}
