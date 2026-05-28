// server: all-in-one LAN server. Reads Sonarr live, serves the calendar UI
// plus a JSON API and a live WebSocket, and persists the "watched" state in
// SQLite. No VPS, no heavy framework, a single binary.
package main

//go:generate goversioninfo -64 -o rsrc.syso versioninfo.json

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "time/tzdata" // embeds the timezone database (Windows does not always ship one)

	"fyne.io/systray"
	"github.com/gorilla/websocket"

	"calendarr-local/internal/bazarr"
	"calendarr-local/internal/desktop"
	"calendarr-local/internal/discovery"
	"calendarr-local/internal/prowlarr"
	"calendarr-local/internal/qbit"
	"calendarr-local/internal/radarr"
	"calendarr-local/internal/sonarr"
	"calendarr-local/internal/store"
	"calendarr-local/web"
)

//go:embed icon.ico
var iconBytes []byte

// config holds optional settings read from config.json (next to the exe). Allows
// auto-start (which launches the exe WITHOUT arguments) to know the qBittorrent
// credentials, etc. Precedence: command-line argument > config.json > default.
type config struct {
	// Mode picks between the full server (Sonarr-connected, calendar UI on
	// :8787) and the lightweight client (playback helper only). Persisted
	// across restarts and toggleable from the tray menu. Default: "server".
	Mode string `json:"mode"`

	SonarrURL   string `json:"sonarrUrl"`
	SonarrKey   string `json:"sonarrKey"`
	QbitURL     string `json:"qbitUrl"`
	QbitUser    string `json:"qbitUser"`
	QbitPass    string `json:"qbitPass"`
	ProwlarrURL string `json:"prowlarrUrl"`
	ProwlarrKey string `json:"prowlarrKey"`
	RadarrURL   string `json:"radarrUrl"`
	RadarrKey   string `json:"radarrKey"`
	BazarrURL   string `json:"bazarrUrl"`
	BazarrKey   string `json:"bazarrKey"`
}

// Mode values for config.Mode. Kept short so they read naturally inside the
// JSON file the user can edit by hand if they want.
const (
	modeServer = "server"
	modeClient = "client"
)

// autostartTaskName is the Windows Task Scheduler entry name used to launch
// Calendarr at user logon. Single name (regardless of mode) because the same
// binary serves both modes — mode is read from config.json at startup.
const autostartTaskName = "Calendarr"

// loadConfig reads config.json. If it does not exist, writes an empty template
// to fill in (without ever touching an existing file).
func loadConfig(path string) config {
	var cfg config
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			tpl := config{QbitURL: "http://localhost:9191", QbitUser: "admin"}
			if data, e := json.MarshalIndent(tpl, "", "  "); e == nil {
				_ = os.WriteFile(path, data, 0o644)
			}
		}
		return cfg
	}
	_ = json.Unmarshal(b, &cfg)
	return cfg
}

const tz = "Europe/Paris"

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

type queueProg struct {
	Status   string
	Percent  int
	TimeLeft string
}

type server struct {
	sc  *sonarr.Client
	st  *store.Store
	loc *time.Location
	hub *hub
	qb  *qbit.Client
	pr  *prowlarr.Client
	rd  *radarr.Client
	bz  *bazarr.Client

	shareURL  string // LAN access address to display for sharing (http://PC-NAME:port)
	sonarrWeb string // Sonarr URL reachable from the LAN (for series links)
	radarrWeb string // Radarr URL reachable from the LAN (for movie links)

	qbitDet qbit.Detection // what we detected about the local qBittorrent install
	qbitURL string         // qBittorrent URL actually in use
	cfgPath string         // path to config.json (for storing the qBit password)

	mu    sync.Mutex
	queue map[int]queueProg // episodeId -> progress of the active download

	setupMu    sync.Mutex
	setupState map[string]string // auto-setup outcomes the UI may act on, e.g. "sonarr.downloadClient" -> "auth-failed"
}

func main() {
	mode := flag.String("mode", "", "server | client (overrides config.json; default: server)")
	addr := flag.String("addr", ":8787", "HTTP listen address (server mode)")
	helperAddr := flag.String("client-addr", "127.0.0.1:8788", "playback helper address (loopback only)")
	mpc := flag.String("mpc", "", "MPC-BE path (empty = auto-detect)")
	sonarrURL := flag.String("sonarr-url", "", "Sonarr URL (empty = auto-detect via config.xml)")
	sonarrKey := flag.String("sonarr-key", "", "Sonarr API key (empty = auto-detect)")
	dbPath := flag.String("db", "", "path to the SQLite file (default: next to the exe)")
	qbitURL := flag.String("qbit-url", "", "qBittorrent WebUI URL (empty = auto-detect via qBittorrent.ini)")
	qbitUser := flag.String("qbit-user", "", "qBittorrent username (empty = auto-detect)")
	qbitPass := flag.String("qbit-pass", "", "qBittorrent password")
	prowlarrURL := flag.String("prowlarr-url", "", "Prowlarr URL (empty = auto-detect via config.xml)")
	prowlarrKey := flag.String("prowlarr-key", "", "Prowlarr API key (empty = auto-detect)")
	radarrURL := flag.String("radarr-url", "", "Radarr URL (empty = auto-detect via config.xml)")
	radarrKey := flag.String("radarr-key", "", "Radarr API key (empty = auto-detect)")
	bazarrURL := flag.String("bazarr-url", "", "Bazarr URL (empty = auto-detect via config.ini)")
	bazarrKey := flag.String("bazarr-key", "", "Bazarr API key (empty = auto-detect)")
	notray := flag.Bool("notray", false, "do not create a notification-area icon (dev/preview)")
	dev := flag.Bool("dev", false, "serve web/ from disk (hot-reload the design without rebuilding)")
	open := flag.Bool("open", false, "open the interface in the browser (used by the desktop shortcut)")
	flag.Parse()

	// No console (build -H=windowsgui), so we log to a file next to the exe,
	// viewable via "Open terminal" in the tray menu.
	exePath, _ := os.Executable()
	logPath := filepath.Join(filepath.Dir(exePath), "calendarr.log")
	if lf, e := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); e == nil {
		log.SetOutput(lf)
	}

	cfgPath := filepath.Join(filepath.Dir(exePath), "config.json")
	cfg := loadConfig(cfgPath)

	// Mode dispatch: -mode flag wins, otherwise read from config, otherwise
	// default to server. The client mode is a different runtime path — it
	// returns from main() entirely, none of the server setup below runs.
	// The user can flip mode any time from the tray menu (Mode: Server /
	// Mode: Client) — a single click restarts the app in the chosen mode.
	effectiveMode := modeServer
	if cfg.Mode == modeClient {
		effectiveMode = modeClient
	}
	if *mode == modeServer || *mode == modeClient {
		effectiveMode = *mode
	}
	if effectiveMode == modeClient {
		runClientMode(*helperAddr, *mpc, logPath, cfgPath, cfg, *notray)
		return
	}
	flagSet := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { flagSet[f.Name] = true })
	// pick: explicit argument > value from config.json > flag default.
	pick := func(name, flagVal, cfgVal string) string {
		if flagSet[name] {
			return flagVal
		}
		if cfgVal != "" {
			return cfgVal
		}
		return flagVal
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}

	sc, err := sonarr.New(pick("sonarr-url", *sonarrURL, cfg.SonarrURL), pick("sonarr-key", *sonarrKey, cfg.SonarrKey))
	if err != nil {
		log.Printf("Sonarr not detected (Calendar page unavailable): %v", err)
		sc = nil
	} else {
		log.Printf("Sonarr detected: %s", sc.BaseURL)
	}

	dbFile := *dbPath
	if dbFile == "" {
		dbFile = filepath.Join(filepath.Dir(exePath), "calendarr.db")
	}
	// We NEVER overwrite an existing database: store.Open opens it as-is
	// (CREATE TABLE IF NOT EXISTS), the "watched" data is preserved.
	if _, e := os.Stat(dbFile); e == nil {
		log.Printf("Existing database reused (not replaced): %s", dbFile)
	} else {
		log.Printf("New database created: %s", dbFile)
	}
	st, err := store.Open(dbFile)
	if err != nil {
		fatal("SQLite database: " + err.Error())
	}
	defer st.Close()

	qbitDet := qbit.Detect()
	qbitURLv := pick("qbit-url", *qbitURL, cfg.QbitURL)
	if qbitURLv == "" {
		qbitURLv = qbitDet.URL // WebUI port read from qBittorrent.ini (8080 by default)
	}
	qbitUserv := pick("qbit-user", *qbitUser, cfg.QbitUser)
	if qbitUserv == "" {
		qbitUserv = qbitDet.Username
	}
	qb := qbit.New(qbitURLv, qbitUserv, pick("qbit-pass", *qbitPass, cfg.QbitPass))
	log.Printf("qBittorrent: %s (installed=%v, WebUI=%v)", qbitURLv, qbitDet.Installed, qbitDet.WebUIEnabled)
	go func() { _ = qb.Authenticate() }() // establish the session once, without blocking startup

	pr, err := prowlarr.New(pick("prowlarr-url", *prowlarrURL, cfg.ProwlarrURL), pick("prowlarr-key", *prowlarrKey, cfg.ProwlarrKey))
	if err != nil {
		log.Printf("Prowlarr not detected (Prowlarr page unavailable): %v", err)
		pr = nil
	} else {
		log.Printf("Prowlarr: %s", pr.BaseURL)
	}

	rd, err := radarr.New(pick("radarr-url", *radarrURL, cfg.RadarrURL), pick("radarr-key", *radarrKey, cfg.RadarrKey))
	if err != nil {
		log.Printf("Radarr not detected (Movies page unavailable): %v", err)
		rd = nil
	} else {
		log.Printf("Radarr: %s", rd.BaseURL)
	}

	bz, err := bazarr.New(pick("bazarr-url", *bazarrURL, cfg.BazarrURL), pick("bazarr-key", *bazarrKey, cfg.BazarrKey))
	if err != nil {
		log.Printf("Bazarr not detected (Subtitles page unavailable): %v", err)
		bz = nil
	} else {
		log.Printf("Bazarr: %s", bz.BaseURL)
	}

	// Share address: the Windows PC name (already assigned by the OS) plus the
	// port. On a LAN, other machines resolve this name on their own
	// (NetBIOS/mDNS), so the host has nothing to configure, just to share this line.
	hostName, _ := os.Hostname()
	_, port, _ := net.SplitHostPort(*addr)
	if port == "" {
		port = "8787"
	}
	shareURL := ""
	if hostName != "" {
		shareURL = "http://" + hostName + ":" + port
	}

	// Sonarr URL reachable from other devices: if Sonarr was auto-detected it
	// points to localhost; we replace it with the PC name (Sonarr runs on the
	// same machine as this server).
	sonarrWeb := ""
	if sc != nil {
		sonarrWeb = sc.BaseURL
		if hostName != "" {
			sonarrWeb = strings.Replace(sonarrWeb, "localhost", hostName, 1)
			sonarrWeb = strings.Replace(sonarrWeb, "127.0.0.1", hostName, 1)
		}
	}
	radarrWeb := ""
	if rd != nil {
		radarrWeb = rd.BaseURL
		if hostName != "" {
			radarrWeb = strings.Replace(radarrWeb, "localhost", hostName, 1)
			radarrWeb = strings.Replace(radarrWeb, "127.0.0.1", hostName, 1)
		}
	}

	srv := &server{sc: sc, st: st, loc: loc, hub: newHub(), qb: qb, pr: pr, rd: rd, bz: bz, queue: map[int]queueProg{}, shareURL: shareURL, sonarrWeb: sonarrWeb, radarrWeb: radarrWeb, qbitDet: qbitDet, qbitURL: qbitURLv, cfgPath: cfgPath, setupState: map[string]string{}}

	if *dev {
		// Design mode: serve the web/ folder as-is from disk.
		// Editing a file then refreshing the browser is enough — no rebuild,
		// no need to redeploy the exe to the server. Path resolved next to the exe
		// (independent of the launch folder). noCache prevents the browser from
		// keeping a stale CSS/JS version between refreshes.
		webDir := filepath.Join(filepath.Dir(exePath), "web")
		log.Printf("DEV MODE: web/ served from disk (%s)", webDir)
		http.Handle("/", noCache(http.FileServer(http.Dir(webDir))))
	} else {
		// no-cache also in prod: tiny LAN app, no perf concern, and it spares
		// the user a hard-refresh after every server.exe redeploy.
		http.Handle("/", noCache(http.FileServer(http.FS(web.FS))))
	}
	http.HandleFunc("/api/status", srv.handleStatus)
	http.HandleFunc("/api/setup/status", srv.handleSetupStatus)
	// Sonarr routes: guarded by needSonarr, returning a clean 503 if Sonarr is
	// absent (instead of crashing). The UI relies on /api/status to display a
	// "Sonarr not installed" message rather than calling these routes.
	http.HandleFunc("/api/calendar", srv.needSonarr(srv.handleCalendar))
	http.HandleFunc("/api/diskspace", srv.needSonarr(srv.handleDiskSpace))
	http.HandleFunc("/api/watched", srv.handleWatched)
	http.HandleFunc("/api/search", srv.needSonarr(srv.handleSearch))
	http.HandleFunc("/api/grab", srv.needSonarr(srv.handleGrab))
	http.HandleFunc("/api/torrents", srv.handleTorrents)
	http.HandleFunc("/api/qbit/status", srv.handleQbitStatus)
	http.HandleFunc("/api/qbit/connect", srv.handleQbitConnect)
	http.HandleFunc("/api/torrents/action", srv.handleTorrentAction)
	http.HandleFunc("/api/series/options", srv.needSonarr(srv.handleSeriesOptions))
	http.HandleFunc("/api/series/lookup", srv.needSonarr(srv.handleSeriesLookup))
	http.HandleFunc("/api/series/tag", srv.needSonarr(srv.handleCreateTag))
	http.HandleFunc("/api/series/add", srv.needSonarr(srv.handleAddSeries))
	http.HandleFunc("/api/prowlarr/indexers", srv.handleProwlarrIndexers)
	http.HandleFunc("/api/prowlarr/toggle", srv.handleProwlarrToggle)
	http.HandleFunc("/api/prowlarr/connect", srv.handleProwlarrConnect)
	http.HandleFunc("/api/prowlarr/sync", srv.handleProwlarrSync)
	http.HandleFunc("/api/prowlarr/schema", srv.handleProwlarrSchema)
	http.HandleFunc("/api/prowlarr/add", srv.handleProwlarrAdd)
	http.HandleFunc("/api/bazarr/overview", srv.handleBazarrOverview)
	http.HandleFunc("/api/bazarr/episode/subs", srv.handleBazarrEpisodeSubs)
	http.HandleFunc("/api/bazarr/episode/search", srv.handleBazarrEpisodeSearch)
	http.HandleFunc("/api/bazarr/episode/download", srv.handleBazarrEpisodeDownload)
	http.HandleFunc("/api/films", srv.handleFilms)
	http.HandleFunc("/api/films/search", srv.handleFilmsSearch)
	http.HandleFunc("/api/films/grab", srv.handleFilmsGrab)
	http.HandleFunc("/api/movies/options", srv.handleMovieOptions)
	http.HandleFunc("/api/movies/add", srv.handleAddMovie)
	http.HandleFunc("/api/search/add", srv.handleSearchAdd)
	http.HandleFunc("GET /play/movie/{movieId}/{name}", srv.handleMoviePlay)
	http.HandleFunc("/ws", srv.handleWS)
	http.HandleFunc("/sonarr/webhook", srv.handleWebhook)
	http.HandleFunc("/play/file", srv.needSonarr(srv.handlePlayFile))
	http.HandleFunc("GET /play/{episodeId}/{name}", srv.needSonarr(srv.handlePlay))

	// Background tasks tied to Sonarr: useless (and would crash) without it.
	if sc != nil {
		go srv.pollQueue()
		go srv.registerWebhook(*addr)
	}
	// Best-effort: wire qBittorrent into Sonarr/Radarr if they have no download
	// client yet (checks service nils + qBit detection internally).
	go srv.autoSetup()
	go func() {
		// Continuous LAN beacon: lets client.exe (on another PC) find this
		// server on its own and open the calendar.
		if err := discovery.Broadcast(port, hostName); err != nil {
			log.Printf("LAN discovery disabled: %v", err)
		}
	}()

	// We reserve the port ourselves to detect an already-running instance. If
	// the port is taken (server already running via auto-start), we don't show
	// an error: we open the UI if requested (desktop shortcut) then exit — no
	// second process, no crash.
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		if *open {
			desktop.OpenBrowser("http://localhost:" + port)
		}
		log.Printf("already running on %s — nothing to do", *addr)
		return
	}
	go func() {
		if err := http.Serve(ln, nil); err != nil {
			fatal("HTTP server: " + err.Error())
		}
	}()

	// Playback helper on loopback: lets the browser running on THIS machine
	// click ▶ and have MPC-BE open the stream. In client mode this is the
	// only listener; in server mode it sits alongside the main UI on :8787.
	if _, err := startPlayHelper(*helperAddr, *mpc); err != nil {
		log.Printf("playback helper not started (%s already in use): %v", *helperAddr, err)
	}

	log.Printf("Calendarr server ready: http://localhost%s", *addr)
	if shareURL != "" {
		log.Printf("LAN share (give this to others): %s", shareURL)
	}
	if *open {
		desktop.OpenBrowser("http://localhost:" + port)
	}

	if *notray {
		select {} // dev/preview mode: no icon, block here
	}
	runTray(logPath, port, cfgPath, cfg)
}

// noCache disables browser caching (-dev mode only) so every refresh reflects
// the latest design version on disk.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

// fatal logs the error, shows an error dialog (no console), and exits.
func fatal(msg string) {
	log.Print(msg)
	desktop.MessageBox("Calendarr — error", msg)
	os.Exit(1)
}

// runTray installs the system-tray icon for server mode. Left-click opens
// the calendar in the browser; right-click shows the menu (mode toggle,
// auto-start, terminal, close).
func runTray(logPath, port, cfgPath string, cfg config) {
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
		mTerm := systray.AddMenuItem("Open terminal", "Show the live server log")
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
				case <-mTerm.ClickedCh:
					desktop.OpenTerminal(logPath)
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
	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008} // DETACHED_PROCESS
	if err := cmd.Start(); err != nil {
		desktop.MessageBox("Calendarr", "Mode switch failed: "+err.Error())
		return
	}
	log.Printf("switching to %s mode — restarting", newMode)
	systray.Quit()
	os.Exit(0)
}

// needSonarr wraps a handler that depends on Sonarr: if Sonarr was not
// detected, it responds with a clean 503 instead of dereferencing a nil client.
func (s *server) needSonarr(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.sc == nil {
			http.Error(w, "Sonarr not configured", http.StatusServiceUnavailable)
			return
		}
		h(w, r)
	}
}

// handleStatus reports which services are available so the UI can display an
// "X not installed" message on the affected page instead of calling an API that
// would fail. (qBittorrent is handled directly by the Torrents page via
// /api/torrents, which already distinguishes "unreachable" from "no torrents".)
func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]bool{
		"sonarr":   s.sc != nil,
		"radarr":   s.rd != nil,
		"prowlarr": s.pr != nil,
		"bazarr":   s.bz != nil,
	})
}

func (s *server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	now := time.Now().In(s.loc)
	year := atoiDefault(r.URL.Query().Get("year"), now.Year())
	month := atoiDefault(r.URL.Query().Get("month"), int(now.Month()))

	cursor := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, s.loc)
	endOfMonth := cursor.AddDate(0, 1, 0).Add(-time.Second)
	from := cursor.AddDate(0, 0, -2).UTC()
	to := endOfMonth.AddDate(0, 0, 2).UTC()

	eps, err := s.sc.Calendar(from, to)
	if err != nil {
		http.Error(w, "Sonarr: "+err.Error(), http.StatusBadGateway)
		return
	}

	watched, err := s.st.WatchedSet()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	q := s.queue
	s.mu.Unlock()

	days := make(map[string][]map[string]any)
	downloaded := 0
	watchedList := []int{}

	for _, e := range eps {
		t, ok := parseAir(e.AirDateUtc)
		if !ok {
			continue
		}
		local := t.In(s.loc)
		key := local.Format("2006-01-02")
		if e.HasFile {
			downloaded++
		}
		if watched[e.ID] {
			watchedList = append(watchedList, e.ID)
		}
		overview := e.Overview
		if overview == "" {
			overview = e.Series.Overview
		}
		runtime := e.Runtime
		if runtime == 0 {
			runtime = e.Series.Runtime
		}
		ep := map[string]any{
			"id":           e.ID,
			"episodeId":    e.ID,
			"seriesId":     e.Series.ID,
			"series":       e.Series.Title,
			"seriesSlug":   e.Series.TitleSlug,
			"season":       e.SeasonNumber,
			"episode":      e.EpisodeNumber,
			"finaleType":   e.FinaleType,
			"episodeTitle": e.Title,
			"time":         local.Format("15:04"),
			"hasFile":      e.HasFile,
			"monitored":    e.Monitored,
			"poster":       e.Poster(),
			"banner":       e.Banner(),
			"overview":      overview,
			"runtime":       runtime,
			"network":       e.Series.Network,
			"year":          e.Series.Year,
			"genres":        e.Series.Genres,
			"certification": e.Series.Certification,
			"rating":        e.Series.Ratings.Value,
			"fileName":     baseName(e.EpisodeFile.Path),
		}
		if p, ok := q[e.ID]; ok {
			ep["downloadStatus"] = p.Status
			ep["downloadPercent"] = p.Percent
			ep["downloadTimeleft"] = p.TimeLeft
		}
		days[key] = append(days[key], ep)
	}

	prev := cursor.AddDate(0, -1, 0)
	next := cursor.AddDate(0, 1, 0)

	writeJSON(w, map[string]any{
		"year":       year,
		"month":      month,
		"monthLabel": frenchMonth(cursor),
		"today":      now.Format("2006-01-02"),
		"prev":       map[string]int{"year": prev.Year(), "month": int(prev.Month())},
		"next":       map[string]int{"year": next.Year(), "month": int(next.Month())},
		"days":       days,
		"watched":    watchedList,
		"stats":      map[string]int{"episodes": len(eps), "downloaded": downloaded, "watched": len(watchedList)},
		"share":      s.shareURL,
		"sonarrUrl":  s.sonarrWeb,
	})
}

func (s *server) handleWatched(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		EpisodeID int  `json:"episode_id"`
		Watched   bool `json:"watched"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := s.st.SetWatched(body.EpisodeID, body.Watched); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleDiskSpace returns the free/total space of the disk containing the
// root folder configured in Sonarr (the destination of imported downloads).
func (s *server) handleDiskSpace(w http.ResponseWriter, r *http.Request) {
	disks, err := s.sc.DiskSpace()
	if err != nil {
		http.Error(w, "Sonarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	rootPath := ""
	if roots, e := s.sc.RootFolderPaths(); e == nil && len(roots) > 0 {
		rootPath = roots[0].Path
	}
	var best sonarr.Disk
	for _, d := range disks {
		// Keep the longest mount point that prefixes the root folder.
		if rootPath != "" && d.Path != "" &&
			strings.HasPrefix(strings.ToLower(rootPath), strings.ToLower(d.Path)) &&
			len(d.Path) >= len(best.Path) {
			best = d
		}
	}
	if best.TotalSpace == 0 && len(disks) > 0 {
		best = disks[0]
	}
	writeJSON(w, map[string]any{"path": rootPath, "free": best.FreeSpace, "total": best.TotalSpace})
}

// handleSearch runs an interactive torrent search for an episode (via the
// indexers configured in Sonarr) and returns the list, sorted by descending
// seeder count.
func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	epID := atoiDefault(r.URL.Query().Get("episodeId"), 0)
	if epID == 0 {
		http.Error(w, "episodeId required", http.StatusBadRequest)
		return
	}
	rels, err := s.sc.SearchReleases(epID)
	if err != nil {
		http.Error(w, "Sonarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	out := make([]map[string]any, 0, len(rels))
	for _, rel := range rels {
		seeders := 0
		if rel.Seeders != nil {
			seeders = *rel.Seeders
		}
		out = append(out, map[string]any{
			"guid":       rel.GUID,
			"title":      rel.Title,
			"indexer":    rel.Indexer,
			"indexerId":  rel.IndexerID,
			"protocol":   rel.Protocol,
			"size":       rel.Size,
			"seeders":    seeders,
			"age":        rel.Age,
			"quality":    rel.Quality.Quality.Name,
			"infoUrl":    rel.InfoURL,
			"rejected":   rel.Rejected,
			"rejections": rel.Rejections,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ai, aj := out[i]["age"].(int), out[j]["age"].(int)
		if ai != aj {
			return ai < aj // newest first, oldest last
		}
		return out[i]["seeders"].(int) > out[j]["seeders"].(int)
	})
	writeJSON(w, map[string]any{"releases": out})
}

// handleGrab sends a chosen release to the download client via Sonarr.
func (s *server) handleGrab(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		GUID      string `json:"guid"`
		IndexerID int    `json:"indexerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.GUID == "" {
		http.Error(w, "guid required", http.StatusBadRequest)
		return
	}
	if err := s.sc.GrabRelease(body.GUID, body.IndexerID); err != nil {
		http.Error(w, "Sonarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// --- Torrents (qBittorrent) ---

func (s *server) handleTorrents(w http.ResponseWriter, r *http.Request) {
	ts, err := s.qb.Torrents()
	if err != nil {
		http.Error(w, "qBittorrent: "+err.Error(), http.StatusBadGateway)
		return
	}
	sort.SliceStable(ts, func(i, j int) bool { return ts[i].AddedOn > ts[j].AddedOn }) // newest on top
	writeJSON(w, map[string]any{"torrents": ts})
}

// handleQbitStatus tells the Torrents page about qBittorrent's state:
// installed? WebUI reachable? connected (auth OK)? — so it can show either the
// table, a password field, "enable the WebUI", or "not installed".
func (s *server) handleQbitStatus(w http.ResponseWriter, r *http.Request) {
	connected := s.qb.Connected()
	reachable := connected
	if !reachable {
		hc := &http.Client{Timeout: 3 * time.Second}
		if resp, err := hc.Get(s.qbitURL); err == nil {
			resp.Body.Close()
			reachable = true
		}
	}
	writeJSON(w, map[string]any{
		"installed": s.qbitDet.Installed || reachable,
		"reachable": reachable,
		"connected": connected,
		"url":       s.qbitURL,
		"username":  s.qbitDet.Username,
	})
}

// handleQbitConnect receives the password entered on the page, tests the
// connection, and stores it in config.json if it works.
func (s *server) handleQbitConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	user := strings.TrimSpace(body.Username)
	if user == "" {
		user = s.qbitDet.Username
	}
	s.qb.SetCreds(user, body.Password)
	if err := s.qb.Authenticate(); err != nil {
		writeJSON(w, map[string]any{"connected": false, "banned": errors.Is(err, qbit.ErrBanned)})
		return
	}
	s.saveQbitConfig(s.qbitURL, user, body.Password)
	// Password now known: retry wiring qBittorrent into Sonarr/Radarr in case
	// the startup attempt failed on authentication.
	go s.autoSetup()
	writeJSON(w, map[string]any{"connected": true})
}

// saveQbitConfig stores the qBittorrent credentials in config.json (while
// preserving the other fields) so they don't need to be re-entered next time.
func (s *server) saveQbitConfig(url, user, pass string) {
	if s.cfgPath == "" {
		return
	}
	c := loadConfig(s.cfgPath)
	c.QbitURL = url
	c.QbitUser = user
	c.QbitPass = pass
	if data, err := json.MarshalIndent(c, "", "  "); err == nil {
		_ = os.WriteFile(s.cfgPath, data, 0o644)
	}
}

func (s *server) handleTorrentAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Action      string `json:"action"`
		Hash        string `json:"hash"`
		DeleteFiles bool   `json:"deleteFiles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Hash == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	var err error
	switch body.Action {
	case "pause":
		err = s.qb.Pause(body.Hash)
	case "resume":
		err = s.qb.Resume(body.Hash)
	case "delete":
		err = s.qb.Delete(body.Hash, body.DeleteFiles)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "qBittorrent: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// --- Add series (Sonarr) ---

func pickFields(raw []byte, keys ...string) []map[string]any {
	var arr []map[string]any
	_ = json.Unmarshal(raw, &arr)
	out := make([]map[string]any, 0, len(arr))
	for _, m := range arr {
		o := map[string]any{}
		for _, k := range keys {
			o[k] = m[k]
		}
		out = append(out, o)
	}
	return out
}

func lookupPoster(m map[string]any) string {
	imgs, _ := m["images"].([]any)
	for _, it := range imgs {
		im, _ := it.(map[string]any)
		if im["coverType"] == "poster" {
			if u, ok := im["remoteUrl"].(string); ok {
				return u
			}
		}
	}
	return ""
}

func (s *server) handleSeriesOptions(w http.ResponseWriter, r *http.Request) {
	prof, err := s.sc.QualityProfiles()
	if err != nil {
		http.Error(w, "Sonarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	tags, err := s.sc.Tags()
	if err != nil {
		http.Error(w, "Sonarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	roots, err := s.sc.RootFolders()
	if err != nil {
		http.Error(w, "Sonarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{
		"qualityProfiles": pickFields(prof, "id", "name"),
		"tags":            pickFields(tags, "id", "label"),
		"rootFolders":     pickFields(roots, "id", "path"),
		"seriesTypes":     []string{"standard", "anime", "daily"},
		"monitorOptions":  []string{"all", "future", "missing", "existing", "firstSeason", "latestSeason", "pilot", "none"},
	})
}

func (s *server) handleSeriesLookup(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("term"))
	if term == "" {
		writeJSON(w, map[string]any{"results": []any{}})
		return
	}
	b, err := s.sc.LookupSeries(term)
	if err != nil {
		http.Error(w, "Sonarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	var arr []map[string]any
	_ = json.Unmarshal(b, &arr)
	out := make([]map[string]any, 0, len(arr))
	for _, m := range arr {
		out = append(out, map[string]any{
			"title":     m["title"],
			"year":      m["year"],
			"tvdbId":    m["tvdbId"],
			"titleSlug": m["titleSlug"],
			"status":    m["status"],
			"network":   m["network"],
			"overview":  m["overview"],
			"poster":    lookupPoster(m),
		})
	}
	writeJSON(w, map[string]any{"results": out})
}

func (s *server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Label) == "" {
		http.Error(w, "label required", http.StatusBadRequest)
		return
	}
	b, err := s.sc.CreateTag(strings.TrimSpace(body.Label))
	if err != nil {
		http.Error(w, "Sonarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	var tag map[string]any
	_ = json.Unmarshal(b, &tag)
	writeJSON(w, map[string]any{"id": tag["id"], "label": tag["label"]})
}

func (s *server) handleAddSeries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		TvdbID           int    `json:"tvdbId"`
		QualityProfileID int    `json:"qualityProfileId"`
		RootFolderPath   string `json:"rootFolderPath"`
		Monitored        bool   `json:"monitored"`
		SeriesType       string `json:"seriesType"`
		Monitor          string `json:"monitor"`
		SearchNow        bool   `json:"searchNow"`
		Tags             []int  `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TvdbID == 0 {
		http.Error(w, "tvdbId required", http.StatusBadRequest)
		return
	}
	if body.RootFolderPath == "" { // default: Sonarr's first root folder
		if roots, err := s.sc.RootFolders(); err == nil {
			var ra []map[string]any
			_ = json.Unmarshal(roots, &ra)
			if len(ra) > 0 {
				if p, ok := ra[0]["path"].(string); ok {
					body.RootFolderPath = p
				}
			}
		}
	}
	if body.Monitor == "" {
		body.Monitor = "all"
	}
	if _, err := s.sc.AddSeries(body.TvdbID, sonarr.AddOptions{
		QualityProfileID: body.QualityProfileID,
		RootFolderPath:   body.RootFolderPath,
		Monitored:        body.Monitored,
		SeriesType:       body.SeriesType,
		Monitor:          body.Monitor,
		SearchNow:        body.SearchNow,
		Tags:             body.Tags,
	}); err != nil {
		http.Error(w, "Sonarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// --- Prowlarr ---

func (s *server) handleProwlarrIndexers(w http.ResponseWriter, r *http.Request) {
	if s.pr == nil {
		http.Error(w, "Prowlarr not configured", http.StatusServiceUnavailable)
		return
	}
	ix, err := s.pr.Indexers()
	if err != nil {
		http.Error(w, "Prowlarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	apps, _ := s.pr.Applications() // best-effort
	// Indexers already present in Sonarr/Radarr (pushed by Prowlarr): for each
	// indexer, report which ones have accepted the sync.
	var sonarrNames []string
	if s.sc != nil {
		sonarrNames, _ = s.sc.IndexerNames()
	}
	var radarrNames []string
	if s.rd != nil {
		radarrNames, _ = s.rd.IndexerNames()
	}
	has := func(list []string, name string) bool {
		n := strings.ToLower(name)
		for _, x := range list {
			if strings.Contains(strings.ToLower(x), n) {
				return true
			}
		}
		return false
	}
	out := make([]map[string]any, 0, len(ix))
	for _, i := range ix {
		out = append(out, map[string]any{
			"id": i.ID, "name": i.Name, "enable": i.Enable, "protocol": i.Protocol,
			"privacy": i.Privacy, "priority": i.Priority,
			"inSonarr": has(sonarrNames, i.Name),
			"inRadarr": s.rd != nil && has(radarrNames, i.Name),
		})
	}
	writeJSON(w, map[string]any{"indexers": out, "apps": apps, "radarrConfigured": s.rd != nil})
}

// handleProwlarrConnect declares Sonarr or Radarr as an "Application" in
// Prowlarr: once connected, Prowlarr automatically syncs its indexers to that
// app (replaces Jackett). Idempotent on the Prowlarr side.
func (s *server) handleProwlarrConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.pr == nil {
		http.Error(w, "Prowlarr not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		App string `json:"app"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	var name, impl, appURL, appKey string
	switch body.App {
	case "sonarr":
		if s.sc == nil {
			http.Error(w, "Sonarr not configured", http.StatusServiceUnavailable)
			return
		}
		name, impl, appURL, appKey = "Sonarr", "Sonarr", s.sc.BaseURL, s.sc.APIKey
	case "radarr":
		if s.rd == nil {
			http.Error(w, "Radarr not configured", http.StatusServiceUnavailable)
			return
		}
		name, impl, appURL, appKey = "Radarr", "Radarr", s.rd.BaseURL, s.rd.APIKey
	default:
		http.Error(w, "unknown app", http.StatusBadRequest)
		return
	}
	created, err := s.pr.AddApplication(name, impl, s.pr.BaseURL, appURL, appKey)
	if err != nil {
		http.Error(w, "Prowlarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	_ = s.pr.SyncApps() // push indexers to the app just connected
	writeJSON(w, map[string]any{"ok": true, "created": created})
}

func (s *server) handleProwlarrSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.pr == nil {
		http.Error(w, "Prowlarr not configured", http.StatusServiceUnavailable)
		return
	}
	if err := s.pr.SyncApps(); err != nil {
		http.Error(w, "Prowlarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleProwlarrSchema returns the catalog of available indexers (slim).
func (s *server) handleProwlarrSchema(w http.ResponseWriter, r *http.Request) {
	if s.pr == nil {
		http.Error(w, "Prowlarr not configured", http.StatusServiceUnavailable)
		return
	}
	b, err := s.pr.IndexerSchema()
	if err != nil {
		http.Error(w, "Prowlarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	var arr []map[string]any
	_ = json.Unmarshal(b, &arr)
	out := make([]map[string]any, 0, len(arr))
	for _, m := range arr {
		out = append(out, map[string]any{
			"name": m["name"], "protocol": m["protocol"], "privacy": m["privacy"], "language": m["language"],
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ni, _ := out[i]["name"].(string)
		nj, _ := out[j]["name"].(string)
		return strings.ToLower(ni) < strings.ToLower(nj)
	})
	writeJSON(w, map[string]any{"indexers": out})
}

// handleProwlarrAdd adds an indexer from the catalog by its name.
func (s *server) handleProwlarrAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.pr == nil {
		http.Error(w, "Prowlarr not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	if err := s.pr.AddIndexer(body.Name); err != nil {
		http.Error(w, "Prowlarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *server) handleProwlarrToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.pr == nil {
		http.Error(w, "Prowlarr not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		ID     int  `json:"id"`
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == 0 {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	if err := s.pr.SetEnabled(body.ID, body.Enable); err != nil {
		http.Error(w, "Prowlarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// --- Bazarr ---

// handleBazarrOverview aggregates everything the Subtitles page needs in one
// shot: missing subs (episodes + movies), recent history, providers status and
// configured languages. Each sub-call is best-effort: a failure on one section
// does not blank the rest.
func (s *server) handleBazarrOverview(w http.ResponseWriter, r *http.Request) {
	if s.bz == nil {
		http.Error(w, "Bazarr not configured", http.StatusServiceUnavailable)
		return
	}
	out := map[string]any{}
	if eps, total, err := s.bz.WantedEpisodes(100); err == nil {
		out["wantedEpisodes"] = eps
		out["wantedEpisodesTotal"] = total
	} else {
		out["wantedEpisodesError"] = err.Error()
	}
	if mvs, total, err := s.bz.WantedMovies(100); err == nil {
		out["wantedMovies"] = mvs
		out["wantedMoviesTotal"] = total
	} else {
		out["wantedMoviesError"] = err.Error()
	}
	if h, err := s.bz.HistoryEpisodes(25); err == nil {
		out["historyEpisodes"] = h
	}
	if h, err := s.bz.HistoryMovies(25); err == nil {
		out["historyMovies"] = h
	}
	if p, err := s.bz.Providers(); err == nil {
		out["providers"] = p
	}
	if l, err := s.bz.Languages(true); err == nil {
		out["languages"] = l
	}
	writeJSON(w, out)
}

// handleBazarrEpisodeSubs lists subtitle tracks already present on disk for
// one episode. Fast (single Bazarr call). Used by the modal to show what the
// user already has before they decide to search for more.
func (s *server) handleBazarrEpisodeSubs(w http.ResponseWriter, r *http.Request) {
	if s.bz == nil {
		http.Error(w, "Bazarr not configured", http.StatusServiceUnavailable)
		return
	}
	seriesID := atoiDefault(r.URL.Query().Get("seriesId"), 0)
	episodeID := atoiDefault(r.URL.Query().Get("episodeId"), 0)
	if seriesID == 0 || episodeID == 0 {
		http.Error(w, "seriesId and episodeId required", http.StatusBadRequest)
		return
	}
	subs, err := s.bz.EpisodeSubtitles(seriesID, episodeID)
	if err != nil {
		http.Error(w, "Bazarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"subtitles": subs})
}

// handleBazarrEpisodeSearch triggers Bazarr's manual subtitle search for one
// episode. Slow (10-30s, sometimes more): we keep the response open while
// Bazarr queries every configured provider in parallel.
func (s *server) handleBazarrEpisodeSearch(w http.ResponseWriter, r *http.Request) {
	if s.bz == nil {
		http.Error(w, "Bazarr not configured", http.StatusServiceUnavailable)
		return
	}
	episodeID := atoiDefault(r.URL.Query().Get("episodeId"), 0)
	if episodeID == 0 {
		http.Error(w, "episodeId required", http.StatusBadRequest)
		return
	}
	results, err := s.bz.SearchEpisodeSubtitles(episodeID)
	if err != nil {
		http.Error(w, "Bazarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	// Echo each result's raw payload back so the client can return it
	// verbatim in the download call. Bazarr validates against original fields.
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		raw := r.Raw
		if raw == nil {
			raw = map[string]any{
				"language":         r.Language,
				"provider":         r.Provider,
				"subtitle":         r.Subtitle,
				"score":            r.Score,
				"hearing_impaired": r.HearingImpaired,
				"forced":           r.Forced,
				"url":              r.URL,
			}
		}
		out = append(out, raw)
	}
	writeJSON(w, map[string]any{"results": out})
}

// handleBazarrEpisodeDownload triggers Bazarr to actually fetch one subtitle
// and place the file next to the video. Body: {seriesId, episodeId, sub: {...}}
// where sub is the raw payload from the search response.
func (s *server) handleBazarrEpisodeDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.bz == nil {
		http.Error(w, "Bazarr not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		SeriesID  int            `json:"seriesId"`
		EpisodeID int            `json:"episodeId"`
		Sub       map[string]any `json:"sub"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.SeriesID == 0 || body.EpisodeID == 0 || body.Sub == nil {
		http.Error(w, "seriesId, episodeId and sub required", http.StatusBadRequest)
		return
	}
	if err := s.bz.DownloadEpisodeSubtitle(body.SeriesID, body.EpisodeID, body.Sub); err != nil {
		http.Error(w, "Bazarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// --- Movies (Radarr) ---

func (s *server) handleFilms(w http.ResponseWriter, r *http.Request) {
	if s.rd == nil {
		http.Error(w, "Radarr not configured", http.StatusServiceUnavailable)
		return
	}
	movies, err := s.rd.Library()
	if err != nil {
		http.Error(w, "Radarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	out := make([]map[string]any, 0, len(movies))
	available := 0
	for _, m := range movies {
		status := "unmonitored"
		if m.HasFile {
			status = "available"
			available++
		} else if m.Monitored {
			status = "missing"
		}
		out = append(out, map[string]any{
			"id": m.ID, "title": m.Title, "year": m.Year, "hasFile": m.HasFile,
			"monitored": m.Monitored, "status": status, "overview": m.Overview,
			"poster": m.Poster(), "banner": m.Banner(),
			"runtime": m.Runtime, "sizeOnDisk": m.SizeOnDisk, "slug": m.TitleSlug,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ti, _ := out[i]["title"].(string)
		tj, _ := out[j]["title"].(string)
		return strings.ToLower(ti) < strings.ToLower(tj)
	})
	writeJSON(w, map[string]any{
		"movies":    out,
		"radarrUrl": s.radarrWeb,
		"stats":     map[string]int{"total": len(out), "available": available},
	})
}

func (s *server) handleMoviePlay(w http.ResponseWriter, r *http.Request) {
	if s.rd == nil {
		http.Error(w, "Radarr not configured", http.StatusServiceUnavailable)
		return
	}
	id := atoiDefault(r.PathValue("movieId"), 0)
	if id == 0 {
		http.Error(w, "invalid movieId", http.StatusBadRequest)
		return
	}
	path, err := s.rd.MovieFilePath(id)
	if err != nil {
		http.Error(w, "file not found: "+err.Error(), http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, path)
}

func (s *server) handleFilmsSearch(w http.ResponseWriter, r *http.Request) {
	if s.rd == nil {
		http.Error(w, "Radarr not configured", http.StatusServiceUnavailable)
		return
	}
	id := atoiDefault(r.URL.Query().Get("movieId"), 0)
	if id == 0 {
		http.Error(w, "movieId required", http.StatusBadRequest)
		return
	}
	rels, err := s.rd.SearchReleases(id)
	if err != nil {
		http.Error(w, "Radarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	out := make([]map[string]any, 0, len(rels))
	for _, rel := range rels {
		seeders := 0
		if rel.Seeders != nil {
			seeders = *rel.Seeders
		}
		out = append(out, map[string]any{
			"guid": rel.GUID, "title": rel.Title, "indexer": rel.Indexer, "indexerId": rel.IndexerID,
			"protocol": rel.Protocol, "size": rel.Size, "seeders": seeders,
			"age": rel.Age, "quality": rel.Quality.Quality.Name,
			"rejected": rel.Rejected, "rejections": rel.Rejections,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ai, aj := out[i]["age"].(int), out[j]["age"].(int)
		if ai != aj {
			return ai < aj
		}
		return out[i]["seeders"].(int) > out[j]["seeders"].(int)
	})
	writeJSON(w, map[string]any{"releases": out})
}

func (s *server) handleFilmsGrab(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.rd == nil {
		http.Error(w, "Radarr not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		GUID      string `json:"guid"`
		IndexerID int    `json:"indexerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GUID == "" {
		http.Error(w, "guid required", http.StatusBadRequest)
		return
	}
	if err := s.rd.GrabRelease(body.GUID, body.IndexerID); err != nil {
		http.Error(w, "Radarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *server) handleMovieOptions(w http.ResponseWriter, r *http.Request) {
	if s.rd == nil {
		http.Error(w, "Radarr not configured", http.StatusServiceUnavailable)
		return
	}
	prof, err := s.rd.QualityProfiles()
	if err != nil {
		http.Error(w, "Radarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	tags, err := s.rd.Tags()
	if err != nil {
		http.Error(w, "Radarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	roots, err := s.rd.RootFolders()
	if err != nil {
		http.Error(w, "Radarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{
		"qualityProfiles":       pickFields(prof, "id", "name"),
		"tags":                  pickFields(tags, "id", "label"),
		"rootFolders":           pickFields(roots, "id", "path"),
		"availabilityOptions":   []string{"announced", "inCinemas", "released"},
	})
}

func (s *server) handleAddMovie(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if s.rd == nil {
		http.Error(w, "Radarr not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		TmdbID              int    `json:"tmdbId"`
		QualityProfileID    int    `json:"qualityProfileId"`
		RootFolderPath      string `json:"rootFolderPath"`
		Monitored           bool   `json:"monitored"`
		MinimumAvailability string `json:"minimumAvailability"`
		SearchNow           bool   `json:"searchNow"`
		Tags                []int  `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TmdbID == 0 {
		http.Error(w, "tmdbId required", http.StatusBadRequest)
		return
	}
	if body.RootFolderPath == "" {
		if roots, err := s.rd.RootFolders(); err == nil {
			var ra []map[string]any
			_ = json.Unmarshal(roots, &ra)
			if len(ra) > 0 {
				if p, ok := ra[0]["path"].(string); ok {
					body.RootFolderPath = p
				}
			}
		}
	}
	if body.MinimumAvailability == "" {
		body.MinimumAvailability = "released"
	}
	if _, err := s.rd.AddMovie(body.TmdbID, radarr.AddOptions{
		QualityProfileID:    body.QualityProfileID,
		RootFolderPath:      body.RootFolderPath,
		Monitored:           body.Monitored,
		MinimumAvailability: body.MinimumAvailability,
		SearchNow:           body.SearchNow,
		Tags:                body.Tags,
	}); err != nil {
		http.Error(w, "Radarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleSearchAdd performs a unified search across series (Sonarr) and movies
// (Radarr) for adding. Each result is tagged with its "type".
func (s *server) handleSearchAdd(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("term"))
	if term == "" {
		writeJSON(w, map[string]any{"results": []any{}})
		return
	}
	out := []map[string]any{}
	if s.sc != nil {
		if b, err := s.sc.LookupSeries(term); err == nil {
			var arr []map[string]any
			_ = json.Unmarshal(b, &arr)
			for i, m := range arr {
				if i >= 8 {
					break
				}
				out = append(out, map[string]any{
					"type": "series", "title": m["title"], "year": m["year"],
					"tvdbId": m["tvdbId"], "titleSlug": m["titleSlug"],
					"poster": lookupPoster(m), "sub": m["network"],
				})
			}
		}
	}
	if s.rd != nil {
		if b, err := s.rd.LookupMovies(term); err == nil {
			var arr []map[string]any
			_ = json.Unmarshal(b, &arr)
			for i, m := range arr {
				if i >= 8 {
					break
				}
				out = append(out, map[string]any{
					"type": "movie", "title": m["title"], "year": m["year"],
					"tmdbId": m["tmdbId"], "titleSlug": m["titleSlug"],
					"poster": lookupPoster(m), "sub": m["studio"],
				})
			}
		}
	}
	writeJSON(w, map[string]any{"results": out})
}

func (s *server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.hub.add(c)
	defer s.hub.remove(c)
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			break
		}
	}
}

// handlePlayFile serves an episode's video file over HTTP (with range support),
// so MPC-BE (launched by client.exe on the viewing machine) can play it from
// the LAN. The file path comes from Sonarr.
func (s *server) handlePlayFile(w http.ResponseWriter, r *http.Request) {
	epID := atoiDefault(r.URL.Query().Get("episodeId"), 0)
	if epID == 0 {
		http.Error(w, "episodeId required", http.StatusBadRequest)
		return
	}
	path, err := s.sc.EpisodeFilePath(epID)
	if err != nil {
		http.Error(w, "file not found: "+err.Error(), http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, path)
}

// handlePlay serves the file via a URL that ends with the file name
// (e.g. /play/16354/Episode.mkv), which is important so MPC-BE recognizes the media.
func (s *server) handlePlay(w http.ResponseWriter, r *http.Request) {
	epID := atoiDefault(r.PathValue("episodeId"), 0)
	if epID == 0 {
		http.Error(w, "invalid episodeId", http.StatusBadRequest)
		return
	}
	path, err := s.sc.EpisodeFilePath(epID)
	if err != nil {
		http.Error(w, "file not found: "+err.Error(), http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, path)
}

func baseName(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Base(p)
}

func (s *server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
	msg, _ := json.Marshal(map[string]any{"type": "calendar"})
	s.hub.broadcast(msg)
}

// pollQueue polls the Sonarr queue every 5s and broadcasts progress.
func (s *server) pollQueue() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	wasActive := false
	prev := map[int]bool{}
	prevActive := false
	var lastRefresh time.Time
	for range t.C {
		// As long as a download is active (in progress / importing), force Sonarr
		// to resync with qBittorrent (at most every 5s). Otherwise its queue stays
		// stale for ~1 min and a finished download keeps showing "in progress" for a while.
		if prevActive && time.Since(lastRefresh) >= 5*time.Second {
			_ = s.sc.RefreshDownloads()
			lastRefresh = time.Now()
		}
		items, err := s.sc.Queue()
		if err != nil {
			continue
		}
		m := make(map[int]queueProg, len(items))
		cur := make(map[int]bool, len(items))
		active := false
		for _, it := range items {
			m[it.EpisodeID] = queueProg{Status: it.Status, Percent: it.Percent, TimeLeft: it.TimeLeft}
			cur[it.EpisodeID] = true
			if it.Status == "downloading" || it.Status == "importing" {
				active = true
			}
		}
		s.mu.Lock()
		s.queue = m
		s.mu.Unlock()
		if len(m) > 0 || wasActive {
			s.broadcastProgress(m)
			wasActive = len(m) > 0
		}
		// An episode has left the queue (download/import finished): ask the
		// calendar to refresh to reflect the new file (works without a webhook).
		for id := range prev {
			if !cur[id] {
				msg, _ := json.Marshal(map[string]any{"type": "calendar"})
				s.hub.broadcast(msg)
				break
			}
		}
		prev = cur
		prevActive = active
	}
}

func (s *server) broadcastProgress(m map[int]queueProg) {
	items := make([]map[string]any, 0, len(m))
	for id, p := range m {
		items = append(items, map[string]any{"episodeId": id, "status": p.Status, "percent": p.Percent, "timeleft": p.TimeLeft})
	}
	msg, _ := json.Marshal(map[string]any{"type": "progress", "items": items})
	s.hub.broadcast(msg)
}

// registerWebhook registers (once) a Sonarr webhook pointing at this server,
// to refresh the calendar instantly on grab/download/import.
func (s *server) registerWebhook(addr string) {
	time.Sleep(1500 * time.Millisecond)
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = strings.TrimPrefix(addr, ":")
	}
	callback := "http://localhost:" + port + "/sonarr/webhook"
	if created, err := s.sc.EnsureWebhook("calendarr-local", callback); err != nil {
		log.Printf("webhook: auto-registration failed (the calendar will update on the next refetch): %v", err)
	} else if created {
		log.Printf("webhook: registered in Sonarr -> %s", callback)
	} else {
		log.Printf("webhook: already present in Sonarr")
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return def
}

func parseAir(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

var frMonths = []string{"", "Janvier", "Février", "Mars", "Avril", "Mai", "Juin", "Juillet", "Août", "Septembre", "Octobre", "Novembre", "Décembre"}

func frenchMonth(t time.Time) string {
	return frMonths[int(t.Month())] + " " + strconv.Itoa(t.Year())
}
