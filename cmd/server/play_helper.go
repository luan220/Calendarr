// Playback helper: a tiny loopback-only HTTP listener whose only job is to
// accept "play this URL in MPC-BE" calls from the browser running on the same
// machine. The browser cannot launch a local application directly, so it
// fetches GET /play?url=<stream-url> on this loopback endpoint instead.
//
// This helper runs in BOTH modes:
//   - server mode: the binary listens on :8787 (UI/API) and ALSO on
//     127.0.0.1:8788 (this helper), so playing from the server-box works
//     without a second binary.
//   - client mode: the binary only listens on 127.0.0.1:8788 (this helper)
//     and the calendar UI is fetched from a server elsewhere on the LAN.
package main

import (
	"log"
	"net"
	"net/http"
	"time"

	"calendarr-local/internal/player"
)

// startPlayHelper installs the /play and /ping handlers on a fresh mux and
// serves them on addr (loopback). Returns the listener so the caller can
// close it, or an error on bind failure.
func startPlayHelper(addr, mpcPath string) (net.Listener, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/play", playHandler(mpcPath))
	mux.HandleFunc("/ping", pingHandler)

	ln, err := bindWithRetry(addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	go func() {
		log.Printf("playback helper ready: http://%s", addr)
		if err := http.Serve(ln, mux); err != nil {
			log.Printf("playback helper stopped: %v", err)
		}
	}()
	return ln, nil
}

// bindWithRetry handles the small race where, just after a mode-switch
// restart, the previous process may not have released the socket yet.
func bindWithRetry(addr string, max time.Duration) (net.Listener, error) {
	deadline := time.Now().Add(max)
	backoff := 100 * time.Millisecond
	for {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(backoff)
	}
}

func playHandler(mpcPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if helperCORS(w, r) {
			return
		}
		url := r.URL.Query().Get("url")
		if url == "" {
			http.Error(w, "url required", http.StatusBadRequest)
			return
		}
		p := player.FindMPCBE(mpcPath)
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
	}
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	if helperCORS(w, r) {
		return
	}
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// helperCORS sets the CORS + Private Network Access headers. Returns true if
// the request was an OPTIONS preflight (already answered, the caller must stop).
// The calendar page is served from :8787 (or from a LAN server in client mode),
// the helper is at 127.0.0.1:8788 — cross-origin, so the browser sends a
// preflight that we must accept.
func helperCORS(w http.ResponseWriter, r *http.Request) bool {
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
