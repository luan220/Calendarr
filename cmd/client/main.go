// client: small helper to run on the viewing machine. It lives in the system
// tray: left-click opens the calendar, right-click shows the menu (auto-start,
// close). In the background it listens locally and, when the user clicks "play"
// in the calendar (browser), launches MPC-BE on the file URL served by the
// server — the browser cannot launch an application directly.
package main

import (
	"embed"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"fyne.io/systray"

	"calendarr-local/internal/desktop"
	"calendarr-local/internal/player"
)

//go:embed icon.ico
var iconFS embed.FS

func main() {
	addr := flag.String("addr", "127.0.0.1:8788", "local helper address")
	mpc := flag.String("mpc", "", "MPC-BE path (empty = auto-detect)")
	flag.Parse()

	exePath, _ := os.Executable()
	logPath := filepath.Join(filepath.Dir(exePath), "client.log")
	if lf, e := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); e == nil {
		log.SetOutput(lf)
	}

	if p := player.FindMPCBE(*mpc); p != "" {
		log.Printf("MPC-BE detected: %s", p)
	} else {
		log.Printf("MPC-BE not found — specify -mpc <path> if needed (the helper still runs)")
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		// Port already in use = another instance of the helper is already running.
		desktop.MessageBox("Calendarr", "client.exe is already running on this machine.")
		return
	}

	http.HandleFunc("/play", func(w http.ResponseWriter, r *http.Request) {
		// The page is served from the LAN (server:8787) and calls this helper
		// on loopback: Chrome sends a Private Network Access preflight that
		// must be explicitly allowed, otherwise the fetch is blocked.
		if cors(w, r) {
			return
		}
		url := r.URL.Query().Get("url")
		if url == "" {
			http.Error(w, "url required", http.StatusBadRequest)
			return
		}
		p := player.FindMPCBE(*mpc)
		if p == "" {
			http.Error(w, "MPC-BE not found", http.StatusServiceUnavailable)
			return
		}
		if err := player.Play(p, url); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("playing: %s", url)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		if cors(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	go func() {
		log.Printf("client ready: http://%s (MPC-BE playback helper)", *addr)
		if err := http.Serve(ln, nil); err != nil {
			log.Printf("helper stopped: %v", err)
		}
	}()

	runTray()
}

// runTray installs the system-tray icon. Left-click opens the calendar;
// right-click shows the menu (auto-start, close).
func runTray() {
	const appName = "CalendarrClient"
	iconBytes, _ := iconFS.ReadFile("icon.ico")
	onReady := func() {
		systray.SetIcon(iconBytes)
		systray.SetTooltip("Calendarr — click to open the calendar")
		systray.SetOnTapped(func() { go openCalendar() }) // left-click

		mAuto := systray.AddMenuItemCheckbox("Launch automatically when Windows starts", "Launch automatically when Windows starts", desktop.AutoStartEnabled(appName))
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Quit Calendarr")

		go func() {
			for {
				select {
				case <-mAuto.ClickedCh:
					enable := !mAuto.Checked()
					if err := desktop.SetAutoStart(appName, enable); err != nil {
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

// cors sets the CORS + Private Network Access headers. Returns true if the
// request was an OPTIONS preflight (already answered, the caller must stop).
func cors(w http.ResponseWriter, r *http.Request) bool {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "*")
	h.Set("Access-Control-Allow-Private-Network", "true")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}
