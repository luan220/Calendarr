//go:build !windows

// Non-Windows tray stubs. Linux/macOS builds run headless (no system tray —
// fyne/systray needs native GUI libs); these keep the process alive so the
// HTTP server (server mode) or the playback helper (client mode) stays up.
package main

import "log"

func runTray(port, cfgPath string, cfg config) {
	log.Printf("headless: no system tray on this platform — server running on :%s (open it in a browser; use -notray to silence this).", port)
	select {}
}

func runClientTray(helperAddr, cfgPath string, cfg config) {
	log.Printf("headless: no system tray on this platform — playback helper running on %s.", helperAddr)
	select {}
}
