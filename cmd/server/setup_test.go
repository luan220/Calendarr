package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"calendarr-local/internal/qbit"
	"calendarr-local/internal/sonarr"
)

// newTestServer builds a minimal *server wired to a mock Sonarr at baseURL,
// with qBittorrent "detected" and a throwaway config path.
func newTestServer(t *testing.T, baseURL string) *server {
	t.Helper()
	sc, err := sonarr.New(baseURL, "test-key")
	if err != nil {
		t.Fatalf("sonarr.New: %v", err)
	}
	return &server{
		sc:         sc,
		qbitDet:    qbit.Detection{Installed: true, Port: 8080, Username: "admin"},
		cfgPath:    filepath.Join(t.TempDir(), "config.json"),
		setupState: map[string]string{},
	}
}

// Guard: a service that already has a download client must be left untouched.
func TestEnsureDownloadClient_SkipsWhenClientExists(t *testing.T) {
	var posts int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			atomic.AddInt32(&posts, 1)
		}
		if r.URL.Path == "/api/v3/downloadclient" && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[{"name":"Transmission"}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer mock.Close()

	newTestServer(t, mock.URL).ensureDownloadClient("sonarr")

	if posts != 0 {
		t.Fatalf("expected no POST when a download client already exists, got %d", posts)
	}
}

// Zero clients + qBittorrent detected -> exactly one add.
func TestEnsureDownloadClient_AddsWhenEmpty(t *testing.T) {
	var posts int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v3/downloadclient" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/api/v3/downloadclient/schema":
			_, _ = w.Write([]byte(`[{"implementation":"QBittorrent","fields":[{"name":"host"},{"name":"port"},{"name":"username"},{"name":"password"},{"name":"category"}]}]`))
		case r.URL.Path == "/api/v3/downloadclient" && r.Method == http.MethodPost:
			atomic.AddInt32(&posts, 1)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer mock.Close()

	newTestServer(t, mock.URL).ensureDownloadClient("sonarr")

	if posts != 1 {
		t.Fatalf("expected exactly one POST to add qBittorrent, got %d", posts)
	}
}
