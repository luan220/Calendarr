// setup.go — best-effort auto-configuration of the *arr stack.
//
// On startup, if Sonarr/Radarr have no download client at all and qBittorrent
// is detected, Calendarr wires qBittorrent in automatically as a download
// client. This repairs the common "fresh install with no download client, so
// nothing ever downloads" situation without the user touching Sonarr/Radarr.
//
// The guard is strict: we only add a client when the service has *zero*
// download clients. An instance the user has already configured (with qBit or
// anything else) is never modified.
//
// Steps that need a filesystem decision (root folder, qBittorrent download
// paths) are NOT auto-applied here — they are surfaced via /api/setup/status
// for a future guided prompt.
package main

import (
	"log"
	"net/http"
)

// autoSetup wires qBittorrent into Sonarr and Radarr when they have no download
// client configured. Best-effort: every failure is logged and recorded in
// setupState, never fatal. Safe to call on every startup and again after the
// qBittorrent password is entered (idempotent thanks to the zero-client guard
// and AddDownloadClient's by-name check).
func (s *server) autoSetup() {
	if !s.qbitDet.Installed {
		return
	}
	s.ensureDownloadClient("sonarr")
	s.ensureDownloadClient("radarr")
}

// ensureDownloadClient adds qBittorrent as a download client to one service
// (sonarr|radarr) when that service currently has none.
func (s *server) ensureDownloadClient(service string) {
	// The qBittorrent WebUI password is read fresh from config each time, so a
	// retry triggered after the user enters it (handleQbitConnect) picks it up.
	pass := loadConfig(s.cfgPath).QbitPass

	var (
		count    func() (int, error)
		add      func() (bool, error)
		category string
	)
	switch service {
	case "sonarr":
		if s.sc == nil {
			return
		}
		category = "tv-sonarr"
		count = s.sc.DownloadClientCount
		add = func() (bool, error) {
			return s.sc.AddDownloadClient("qBittorrent", "localhost", s.qbitDet.Port, s.qbitDet.Username, pass, category)
		}
	case "radarr":
		if s.rd == nil {
			return
		}
		category = "radarr"
		count = s.rd.DownloadClientCount
		add = func() (bool, error) {
			return s.rd.AddDownloadClient("qBittorrent", "localhost", s.qbitDet.Port, s.qbitDet.Username, pass, category)
		}
	default:
		return
	}

	n, err := count()
	if err != nil {
		log.Printf("auto-setup: %s download-client check failed: %v", service, err)
		return
	}
	if n > 0 {
		// Already configured — never touch the user's setup.
		s.clearSetupState(service + ".downloadClient")
		return
	}

	added, err := add()
	if err != nil {
		// The add POSTs without forceSave, so the service tested the connection
		// and rejected it — almost always because qBittorrent requires a
		// password we don't have yet. Record it so the UI can prompt for one.
		log.Printf("auto-setup: adding qBittorrent to %s failed: %v", service, err)
		s.setSetupState(service+".downloadClient", "auth-failed")
		return
	}
	if added {
		log.Printf("auto-setup: qBittorrent configured as a download client in %s", service)
	}
	s.clearSetupState(service + ".downloadClient")
}

func (s *server) setSetupState(key, val string) {
	s.setupMu.Lock()
	s.setupState[key] = val
	s.setupMu.Unlock()
}

func (s *server) clearSetupState(key string) {
	s.setupMu.Lock()
	delete(s.setupState, key)
	s.setupMu.Unlock()
}

// handleSetupStatus reports what auto-setup could and could not finish, so the
// UI can show a banner (e.g. "qBittorrent needs a password" linking to the
// existing Torrents-page prompt).
func (s *server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	s.setupMu.Lock()
	state := make(map[string]string, len(s.setupState))
	for k, v := range s.setupState {
		state[k] = v
	}
	s.setupMu.Unlock()

	writeJSON(w, map[string]any{
		"sonarr": map[string]any{
			"configured":          s.sc != nil,
			"downloadClientError": state["sonarr.downloadClient"],
		},
		"radarr": map[string]any{
			"configured":          s.rd != nil,
			"downloadClientError": state["radarr.downloadClient"],
		},
		"qbit": map[string]any{
			"installed": s.qbitDet.Installed,
		},
	})
}
