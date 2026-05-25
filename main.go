// server : serveur tout-en-un (LAN). Lit Sonarr en direct, sert l'UI
// calendrier + une API JSON + un WebSocket live, et persiste l'état "vu" en
// SQLite. Pas de VPS, pas de framework lourd, un seul binaire.
package main

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "time/tzdata" // embarque la base des fuseaux (Windows n'en a pas toujours)

	"fyne.io/systray"
	"github.com/gorilla/websocket"

	"calendarr-local/internal/desktop"
	"calendarr-local/internal/discovery"
	"calendarr-local/internal/prowlarr"
	"calendarr-local/internal/qbit"
	"calendarr-local/internal/radarr"
	"calendarr-local/internal/sonarr"
	"calendarr-local/internal/store"
)

//go:embed web
var webFS embed.FS

//go:embed icon.ico
var iconBytes []byte

// config = réglages optionnels lus dans config.json (à côté de l'exe). Permet au
// démarrage auto (qui lance l'exe SANS arguments) de connaître les identifiants
// qBittorrent, etc. Précédence : argument en ligne de commande > config.json > défaut.
type config struct {
	SonarrURL   string `json:"sonarrUrl"`
	SonarrKey   string `json:"sonarrKey"`
	QbitURL     string `json:"qbitUrl"`
	QbitUser    string `json:"qbitUser"`
	QbitPass    string `json:"qbitPass"`
	ProwlarrURL string `json:"prowlarrUrl"`
	ProwlarrKey string `json:"prowlarrKey"`
	RadarrURL   string `json:"radarrUrl"`
	RadarrKey   string `json:"radarrKey"`
}

// loadConfig lit config.json. S'il n'existe pas, écrit un modèle vide à remplir
// (sans jamais toucher à un fichier existant).
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

	shareURL  string // adresse d'accès LAN à montrer pour partager (http://NOM-PC:port)
	sonarrWeb string // URL Sonarr joignable depuis le LAN (pour les liens vers la série)
	radarrWeb string // URL Radarr joignable depuis le LAN (pour les liens vers le film)

	qbitDet qbit.Detection // ce qu'on a détecté de l'install qBittorrent locale
	qbitURL string         // URL qBittorrent effectivement utilisée
	cfgPath string         // chemin de config.json (pour mémoriser le mot de passe qBit)

	mu    sync.Mutex
	queue map[int]queueProg // episodeId -> progression du DL en cours
}

func main() {
	addr := flag.String("addr", ":8787", "adresse d'écoute HTTP")
	sonarrURL := flag.String("sonarr-url", "", "URL Sonarr (vide = auto-détection via config.xml)")
	sonarrKey := flag.String("sonarr-key", "", "clé API Sonarr (vide = auto-détection)")
	dbPath := flag.String("db", "", "chemin du fichier SQLite (défaut: à côté de l'exe)")
	qbitURL := flag.String("qbit-url", "", "URL du WebUI qBittorrent (vide = auto-détection via qBittorrent.ini)")
	qbitUser := flag.String("qbit-user", "", "utilisateur qBittorrent (vide = auto-détection)")
	qbitPass := flag.String("qbit-pass", "", "mot de passe qBittorrent")
	prowlarrURL := flag.String("prowlarr-url", "", "URL Prowlarr (vide = auto-détection via config.xml)")
	prowlarrKey := flag.String("prowlarr-key", "", "clé API Prowlarr (vide = auto-détection)")
	radarrURL := flag.String("radarr-url", "", "URL Radarr (vide = auto-détection via config.xml)")
	radarrKey := flag.String("radarr-key", "", "clé API Radarr (vide = auto-détection)")
	notray := flag.Bool("notray", false, "ne pas créer d'icône dans la zone de notification (dev/preview)")
	dev := flag.Bool("dev", false, "servir web/ depuis le disque (rechargement à chaud du design, sans rebuild)")
	open := flag.Bool("open", false, "ouvrir l'interface dans le navigateur (utilisé par le raccourci bureau)")
	flag.Parse()

	// Pas de console (build -H=windowsgui) → on journalise dans un fichier à côté
	// de l'exe, consultable via « Ouvrir le terminal » dans le menu du tray.
	exePath, _ := os.Executable()
	logPath := filepath.Join(filepath.Dir(exePath), "server.log")
	if lf, e := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); e == nil {
		log.SetOutput(lf)
	}

	cfgPath := filepath.Join(filepath.Dir(exePath), "config.json")
	cfg := loadConfig(cfgPath)
	flagSet := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { flagSet[f.Name] = true })
	// pick : argument explicite > valeur de config.json > défaut du flag.
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
		log.Printf("Sonarr non détecté (page Calendrier indisponible): %v", err)
		sc = nil
	} else {
		log.Printf("Sonarr détecté: %s", sc.BaseURL)
	}

	dbFile := *dbPath
	if dbFile == "" {
		dbFile = filepath.Join(filepath.Dir(exePath), "calendarr.db")
	}
	// On n'écrase JAMAIS une base existante : store.Open l'ouvre telle quelle
	// (CREATE TABLE IF NOT EXISTS), les données "vu" sont préservées.
	if _, e := os.Stat(dbFile); e == nil {
		log.Printf("Base existante réutilisée (non remplacée): %s", dbFile)
	} else {
		log.Printf("Nouvelle base créée: %s", dbFile)
	}
	st, err := store.Open(dbFile)
	if err != nil {
		fatal("Base SQLite : " + err.Error())
	}
	defer st.Close()

	qbitDet := qbit.Detect()
	qbitURLv := pick("qbit-url", *qbitURL, cfg.QbitURL)
	if qbitURLv == "" {
		qbitURLv = qbitDet.URL // port WebUI lu dans qBittorrent.ini (8080 par défaut)
	}
	qbitUserv := pick("qbit-user", *qbitUser, cfg.QbitUser)
	if qbitUserv == "" {
		qbitUserv = qbitDet.Username
	}
	qb := qbit.New(qbitURLv, qbitUserv, pick("qbit-pass", *qbitPass, cfg.QbitPass))
	log.Printf("qBittorrent: %s (installé=%v, WebUI=%v)", qbitURLv, qbitDet.Installed, qbitDet.WebUIEnabled)
	go func() { _ = qb.Authenticate() }() // établit la session une fois, sans bloquer le démarrage

	pr, err := prowlarr.New(pick("prowlarr-url", *prowlarrURL, cfg.ProwlarrURL), pick("prowlarr-key", *prowlarrKey, cfg.ProwlarrKey))
	if err != nil {
		log.Printf("Prowlarr non détecté (page Prowlarr indisponible): %v", err)
		pr = nil
	} else {
		log.Printf("Prowlarr: %s", pr.BaseURL)
	}

	rd, err := radarr.New(pick("radarr-url", *radarrURL, cfg.RadarrURL), pick("radarr-key", *radarrKey, cfg.RadarrKey))
	if err != nil {
		log.Printf("Radarr non détecté (page Films indisponible): %v", err)
		rd = nil
	} else {
		log.Printf("Radarr: %s", rd.BaseURL)
	}

	// Adresse de partage : le nom Windows du PC (déjà attribué par l'OS) + le
	// port. Sur un LAN, les autres machines résolvent ce nom tout seules
	// (NetBIOS/mDNS) → l'hôte n'a rien à configurer, juste à donner cette ligne.
	hostName, _ := os.Hostname()
	_, port, _ := net.SplitHostPort(*addr)
	if port == "" {
		port = "8787"
	}
	shareURL := ""
	if hostName != "" {
		shareURL = "http://" + hostName + ":" + port
	}

	// URL Sonarr joignable depuis les autres appareils : si Sonarr est auto-
	// détecté il pointe sur localhost ; on remplace par le nom du PC (Sonarr
	// tourne sur la même machine que ce serveur).
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

	srv := &server{sc: sc, st: st, loc: loc, hub: newHub(), qb: qb, pr: pr, rd: rd, queue: map[int]queueProg{}, shareURL: shareURL, sonarrWeb: sonarrWeb, radarrWeb: radarrWeb, qbitDet: qbitDet, qbitURL: qbitURLv, cfgPath: cfgPath}

	if *dev {
		// Mode design : on sert le dossier web/ tel quel depuis le disque.
		// Éditer un fichier puis rafraîchir le navigateur suffit — aucun rebuild,
		// aucun renvoi de l'exe sur le serveur. Chemin résolu à côté de l'exe
		// (indépendant du dossier de lancement). noCache évite que le navigateur
		// garde une vieille version du CSS/JS entre deux rafraîchissements.
		webDir := filepath.Join(filepath.Dir(exePath), "web")
		log.Printf("MODE DEV : web/ servi depuis le disque (%s)", webDir)
		http.Handle("/", noCache(http.FileServer(http.Dir(webDir))))
	} else {
		sub, _ := fs.Sub(webFS, "web")
		http.Handle("/", http.FileServer(http.FS(sub)))
	}
	http.HandleFunc("/api/status", srv.handleStatus)
	// Routes Sonarr : protégées par needSonarr → 503 propre si Sonarr absent
	// (au lieu d'un crash). L'UI s'appuie sur /api/status pour afficher un
	// message « Sonarr non installé » plutôt que d'appeler ces routes.
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

	// Tâches de fond liées à Sonarr : inutiles (et qui crasheraient) sans lui.
	if sc != nil {
		go srv.pollQueue()
		go srv.registerWebhook(*addr)
	}
	go func() {
		// Phare LAN en continu : permet à client.exe (autre PC) de
		// trouver ce serveur tout seul et d'ouvrir le calendrier.
		if err := discovery.Broadcast(port, hostName); err != nil {
			log.Printf("découverte LAN désactivée: %v", err)
		}
	}()

	// On réserve le port nous-mêmes pour détecter une instance déjà lancée. Si le
	// port est pris (serveur déjà en service via le démarrage auto), on n'affiche
	// pas d'erreur : on ouvre l'UI si demandé (raccourci bureau) puis on sort —
	// pas de second processus, pas de plantage.
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		if *open {
			desktop.OpenBrowser("http://localhost:" + port)
		}
		log.Printf("déjà en service sur %s — rien à faire", *addr)
		return
	}
	go func() {
		if err := http.Serve(ln, nil); err != nil {
			fatal("Serveur HTTP : " + err.Error())
		}
	}()

	log.Printf("server prêt : http://localhost%s", *addr)
	if shareURL != "" {
		log.Printf("Partage LAN (à donner aux autres) : %s", shareURL)
	}
	if *open {
		desktop.OpenBrowser("http://localhost:" + port)
	}

	if *notray {
		select {} // mode dev/preview : pas d'icône, on bloque ici
	}
	runTray(logPath, port)
}

// noCache désactive le cache navigateur (mode -dev uniquement) pour que chaque
// rafraîchissement reflète la dernière version du design sur le disque.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

// fatal journalise, montre une boîte d'erreur (pas de console) et quitte.
func fatal(msg string) {
	log.Print(msg)
	desktop.MessageBox("Calendarr — erreur", msg)
	os.Exit(1)
}

// runTray installe l'icône dans la zone de notification. Clic gauche = ouvrir le
// calendrier dans le navigateur ; clic droit = menu (démarrage auto, terminal, fermer).
func runTray(logPath, port string) {
	const appName = "CalendarrServer"
	onReady := func() {
		systray.SetIcon(iconBytes)
		systray.SetTooltip("Calendarr — serveur (clic pour ouvrir le calendrier)")
		// Clic gauche = ouvrir l'UI locale (comme client.exe). On ne définit pas
		// SetOnSecondaryTapped → le clic droit garde le menu par défaut.
		systray.SetOnTapped(func() { desktop.OpenBrowser("http://localhost:" + port) })

		mAuto := systray.AddMenuItemCheckbox("Démarrer avec Windows", "Lancer automatiquement à l'ouverture de Windows", desktop.AutoStartEnabled(appName))
		mTerm := systray.AddMenuItem("Ouvrir le terminal", "Afficher le journal du serveur en direct")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Fermer", "Arrêter le serveur Calendarr")

		go func() {
			for {
				select {
				case <-mAuto.ClickedCh:
					enable := !mAuto.Checked()
					if err := desktop.SetAutoStart(appName, enable); err != nil {
						desktop.MessageBox("Calendarr", "Démarrage auto : "+err.Error())
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

// needSonarr enveloppe un handler qui dépend de Sonarr : si Sonarr n'a pas été
// détecté, il répond 503 proprement au lieu de déréférencer un client nil.
func (s *server) needSonarr(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.sc == nil {
			http.Error(w, "Sonarr non configuré", http.StatusServiceUnavailable)
			return
		}
		h(w, r)
	}
}

// handleStatus indique quels services sont disponibles, pour que l'UI affiche un
// message « X non installé » sur la page concernée au lieu d'appeler une API qui
// échouerait. (qBittorrent est géré directement par la page Torrents via
// /api/torrents, qui distingue déjà « injoignable » de « aucun torrent ».)
func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]bool{
		"sonarr":   s.sc != nil,
		"radarr":   s.rd != nil,
		"prowlarr": s.pr != nil,
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
		http.Error(w, "POST requis", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		EpisodeID int  `json:"episode_id"`
		Watched   bool `json:"watched"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "JSON invalide", http.StatusBadRequest)
		return
	}
	if err := s.st.SetWatched(body.EpisodeID, body.Watched); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleDiskSpace renvoie l'espace libre/total du disque contenant le dossier
// racine configuré dans Sonarr (la destination des téléchargements importés).
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
		// On garde le point de montage le plus long qui préfixe le dossier racine.
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

// handleSearch lance une recherche interactive de torrents pour un épisode
// (via les indexeurs configurés dans Sonarr) et renvoie la liste, triée par
// nombre de seeders décroissant.
func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	epID := atoiDefault(r.URL.Query().Get("episodeId"), 0)
	if epID == 0 {
		http.Error(w, "episodeId requis", http.StatusBadRequest)
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
			return ai < aj // plus récents d'abord, plus vieux à la fin
		}
		return out[i]["seeders"].(int) > out[j]["seeders"].(int)
	})
	writeJSON(w, map[string]any{"releases": out})
}

// handleGrab envoie une release choisie au client de téléchargement via Sonarr.
func (s *server) handleGrab(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST requis", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		GUID      string `json:"guid"`
		IndexerID int    `json:"indexerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "JSON invalide", http.StatusBadRequest)
		return
	}
	if body.GUID == "" {
		http.Error(w, "guid requis", http.StatusBadRequest)
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
	sort.SliceStable(ts, func(i, j int) bool { return ts[i].AddedOn > ts[j].AddedOn }) // plus récents en haut
	writeJSON(w, map[string]any{"torrents": ts})
}

// handleQbitStatus dit à la page Torrents dans quel état est qBittorrent :
// installé ? WebUI joignable ? connecté (auth OK) ? — pour afficher soit la table,
// soit un champ mot de passe, soit « active la WebUI », soit « non installé ».
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

// handleQbitConnect reçoit le mot de passe saisi dans la page, teste la connexion,
// et le mémorise dans config.json si ça marche.
func (s *server) handleQbitConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST requis", http.StatusMethodNotAllowed)
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
	writeJSON(w, map[string]any{"connected": true})
}

// saveQbitConfig mémorise les identifiants qBittorrent dans config.json (en
// préservant les autres champs) pour ne pas les ressaisir au prochain lancement.
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
		http.Error(w, "POST requis", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Action      string `json:"action"`
		Hash        string `json:"hash"`
		DeleteFiles bool   `json:"deleteFiles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Hash == "" {
		http.Error(w, "requête invalide", http.StatusBadRequest)
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
		http.Error(w, "action inconnue", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "qBittorrent: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// --- Ajout de série (Sonarr) ---

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
		http.Error(w, "POST requis", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Label) == "" {
		http.Error(w, "label requis", http.StatusBadRequest)
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
		http.Error(w, "POST requis", http.StatusMethodNotAllowed)
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
		http.Error(w, "tvdbId requis", http.StatusBadRequest)
		return
	}
	if body.RootFolderPath == "" { // défaut: 1er dossier racine de Sonarr
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
		http.Error(w, "Prowlarr non configuré", http.StatusServiceUnavailable)
		return
	}
	ix, err := s.pr.Indexers()
	if err != nil {
		http.Error(w, "Prowlarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	apps, _ := s.pr.Applications() // best-effort
	// indexeurs déjà présents dans Sonarr/Radarr (poussés par Prowlarr) → on
	// indique, par indexeur, lesquels ont bien accepté la synchro.
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

// handleProwlarrConnect déclare Sonarr ou Radarr comme "Application" dans
// Prowlarr : une fois connecté, Prowlarr synchronise ses indexeurs vers cette
// app automatiquement (remplace Jackett). Idempotent côté Prowlarr.
func (s *server) handleProwlarrConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST requis", http.StatusMethodNotAllowed)
		return
	}
	if s.pr == nil {
		http.Error(w, "Prowlarr non configuré", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		App string `json:"app"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "JSON invalide", http.StatusBadRequest)
		return
	}
	var name, impl, appURL, appKey string
	switch body.App {
	case "sonarr":
		if s.sc == nil {
			http.Error(w, "Sonarr non configuré", http.StatusServiceUnavailable)
			return
		}
		name, impl, appURL, appKey = "Sonarr", "Sonarr", s.sc.BaseURL, s.sc.APIKey
	case "radarr":
		if s.rd == nil {
			http.Error(w, "Radarr non configuré", http.StatusServiceUnavailable)
			return
		}
		name, impl, appURL, appKey = "Radarr", "Radarr", s.rd.BaseURL, s.rd.APIKey
	default:
		http.Error(w, "app inconnue", http.StatusBadRequest)
		return
	}
	created, err := s.pr.AddApplication(name, impl, s.pr.BaseURL, appURL, appKey)
	if err != nil {
		http.Error(w, "Prowlarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	_ = s.pr.SyncApps() // pousse les indexeurs vers l'app juste connectée
	writeJSON(w, map[string]any{"ok": true, "created": created})
}

func (s *server) handleProwlarrSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST requis", http.StatusMethodNotAllowed)
		return
	}
	if s.pr == nil {
		http.Error(w, "Prowlarr non configuré", http.StatusServiceUnavailable)
		return
	}
	if err := s.pr.SyncApps(); err != nil {
		http.Error(w, "Prowlarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleProwlarrSchema renvoie le catalogue d'indexeurs disponibles (slim).
func (s *server) handleProwlarrSchema(w http.ResponseWriter, r *http.Request) {
	if s.pr == nil {
		http.Error(w, "Prowlarr non configuré", http.StatusServiceUnavailable)
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

// handleProwlarrAdd ajoute un indexeur du catalogue par son nom.
func (s *server) handleProwlarrAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST requis", http.StatusMethodNotAllowed)
		return
	}
	if s.pr == nil {
		http.Error(w, "Prowlarr non configuré", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, "name requis", http.StatusBadRequest)
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
		http.Error(w, "POST requis", http.StatusMethodNotAllowed)
		return
	}
	if s.pr == nil {
		http.Error(w, "Prowlarr non configuré", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		ID     int  `json:"id"`
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == 0 {
		http.Error(w, "id requis", http.StatusBadRequest)
		return
	}
	if err := s.pr.SetEnabled(body.ID, body.Enable); err != nil {
		http.Error(w, "Prowlarr: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// --- Films (Radarr) ---

func (s *server) handleFilms(w http.ResponseWriter, r *http.Request) {
	if s.rd == nil {
		http.Error(w, "Radarr non configuré", http.StatusServiceUnavailable)
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
		http.Error(w, "Radarr non configuré", http.StatusServiceUnavailable)
		return
	}
	id := atoiDefault(r.PathValue("movieId"), 0)
	if id == 0 {
		http.Error(w, "movieId invalide", http.StatusBadRequest)
		return
	}
	path, err := s.rd.MovieFilePath(id)
	if err != nil {
		http.Error(w, "fichier introuvable: "+err.Error(), http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, path)
}

func (s *server) handleFilmsSearch(w http.ResponseWriter, r *http.Request) {
	if s.rd == nil {
		http.Error(w, "Radarr non configuré", http.StatusServiceUnavailable)
		return
	}
	id := atoiDefault(r.URL.Query().Get("movieId"), 0)
	if id == 0 {
		http.Error(w, "movieId requis", http.StatusBadRequest)
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
		http.Error(w, "POST requis", http.StatusMethodNotAllowed)
		return
	}
	if s.rd == nil {
		http.Error(w, "Radarr non configuré", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		GUID      string `json:"guid"`
		IndexerID int    `json:"indexerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GUID == "" {
		http.Error(w, "guid requis", http.StatusBadRequest)
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
		http.Error(w, "Radarr non configuré", http.StatusServiceUnavailable)
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
		http.Error(w, "POST requis", http.StatusMethodNotAllowed)
		return
	}
	if s.rd == nil {
		http.Error(w, "Radarr non configuré", http.StatusServiceUnavailable)
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
		http.Error(w, "tmdbId requis", http.StatusBadRequest)
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

// handleSearchAdd : recherche unifiée séries (Sonarr) + films (Radarr) pour
// l'ajout. Chaque résultat est étiqueté par "type".
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

// handlePlayFile sert le fichier vidéo d'un épisode en HTTP (avec range), pour
// que MPC-BE (lancé par Client.exe sur la machine de visionnage) le lise depuis
// le LAN. Le chemin du fichier vient de Sonarr.
func (s *server) handlePlayFile(w http.ResponseWriter, r *http.Request) {
	epID := atoiDefault(r.URL.Query().Get("episodeId"), 0)
	if epID == 0 {
		http.Error(w, "episodeId requis", http.StatusBadRequest)
		return
	}
	path, err := s.sc.EpisodeFilePath(epID)
	if err != nil {
		http.Error(w, "fichier introuvable: "+err.Error(), http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, path)
}

// handlePlay sert le fichier via une URL qui se termine par le nom du fichier
// (ex: /play/16354/Episode.mkv) — important pour que MPC-BE reconnaisse le média.
func (s *server) handlePlay(w http.ResponseWriter, r *http.Request) {
	epID := atoiDefault(r.PathValue("episodeId"), 0)
	if epID == 0 {
		http.Error(w, "episodeId invalide", http.StatusBadRequest)
		return
	}
	path, err := s.sc.EpisodeFilePath(epID)
	if err != nil {
		http.Error(w, "fichier introuvable: "+err.Error(), http.StatusNotFound)
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

// pollQueue interroge la file Sonarr toutes les 5s et diffuse la progression.
func (s *server) pollQueue() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	wasActive := false
	prev := map[int]bool{}
	prevActive := false
	var lastRefresh time.Time
	for range t.C {
		// Tant qu'un téléchargement est actif (en cours / import), on force Sonarr à
		// se resynchroniser avec qBittorrent (au plus toutes les 5s). Sinon sa file
		// reste périmée ~1 min et un DL terminé reste affiché « en cours » longtemps.
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
		// Un épisode a quitté la file (téléchargement/import terminé) → demande au
		// calendrier de se rafraîchir pour refléter le fichier (marche sans webhook).
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

// registerWebhook enregistre (une fois) un webhook Sonarr pointant sur ce serveur,
// pour rafraîchir le calendrier instantanément sur grab/download/import.
func (s *server) registerWebhook(addr string) {
	time.Sleep(1500 * time.Millisecond)
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = strings.TrimPrefix(addr, ":")
	}
	callback := "http://localhost:" + port + "/sonarr/webhook"
	if created, err := s.sc.EnsureWebhook("calendarr-local", callback); err != nil {
		log.Printf("webhook: auto-enregistrement échoué (le calendrier se met à jour au prochain refetch): %v", err)
	} else if created {
		log.Printf("webhook: enregistré dans Sonarr -> %s", callback)
	} else {
		log.Printf("webhook: déjà présent dans Sonarr")
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

var frMonths = []string{"", "janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}

func frenchMonth(t time.Time) string {
	return frMonths[int(t.Month())] + " " + strconv.Itoa(t.Year())
}
