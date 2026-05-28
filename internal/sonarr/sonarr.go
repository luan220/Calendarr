// Package sonarr talks to the local Sonarr API. On the machine that hosts
// Sonarr, we read the API key directly from its config.xml: the user has
// nothing to configure (the "zero-config install" goal).
package sonarr

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Client queries a Sonarr instance.
type Client struct {
	BaseURL string
	APIKey  string
	http    *http.Client
}

// New builds a client. If baseURL or apiKey are empty, we try to deduce them
// from the local Sonarr config.xml (normal case: the agent runs on the Sonarr
// machine). In dev, explicit values are passed (dev workstation -> server).
func New(baseURL, apiKey string) (*Client, error) {
	if baseURL == "" || apiKey == "" {
		localURL, localKey, err := readLocalConfig()
		if err != nil {
			return nil, fmt.Errorf("Sonarr not detected (set sonarr_url + sonarr_api_key): %w", err)
		}
		if baseURL == "" {
			baseURL = localURL
		}
		if apiKey == "" {
			apiKey = localKey
		}
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		http:    &http.Client{Timeout: 20 * time.Second},
	}, nil
}

type xmlConfig struct {
	APIKey  string `xml:"ApiKey"`
	Port    int    `xml:"Port"`
	URLBase string `xml:"UrlBase"`
}

// configPaths lists the usual locations of the Sonarr config.xml.
func configPaths() []string {
	var paths []string
	if pd := os.Getenv("ProgramData"); pd != "" {
		paths = append(paths, filepath.Join(pd, "Sonarr", "config.xml"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, "AppData", "Roaming", "Sonarr", "config.xml"),
			filepath.Join(home, ".config", "Sonarr", "config.xml"),
		)
	}
	return paths
}

func readLocalConfig() (baseURL, apiKey string, err error) {
	var lastErr error = fmt.Errorf("config.xml not found")
	for _, p := range configPaths() {
		data, e := os.ReadFile(p)
		if e != nil {
			lastErr = e
			continue
		}
		var c xmlConfig
		if e := xml.Unmarshal(data, &c); e != nil {
			lastErr = e
			continue
		}
		if c.APIKey == "" {
			lastErr = fmt.Errorf("ApiKey empty in %s", p)
			continue
		}
		port := c.Port
		if port == 0 {
			port = 8989
		}
		base := fmt.Sprintf("http://localhost:%d", port)
		if ub := strings.Trim(c.URLBase, "/"); ub != "" {
			base += "/" + ub
		}
		return base, c.APIKey, nil
	}
	return "", "", lastErr
}

// Episode is the subset of a Sonarr calendar item that we care about.
type Episode struct {
	ID            int    `json:"id"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	FinaleType    string `json:"finaleType"` // "season" or "series" if it is a finale, otherwise empty
	Title         string `json:"title"`
	Overview      string `json:"overview"`
	Runtime       int    `json:"runtime"`
	AirDateUtc    string `json:"airDateUtc"`
	HasFile       bool   `json:"hasFile"`
	Monitored     bool   `json:"monitored"`
	Series        struct {
		ID            int      `json:"id"`
		Title         string   `json:"title"`
		TitleSlug     string   `json:"titleSlug"`
		TvdbID        int      `json:"tvdbId"`
		Network       string   `json:"network"`
		Year          int      `json:"year"`
		Overview      string   `json:"overview"`
		Runtime       int      `json:"runtime"`
		Certification string   `json:"certification"`
		Genres        []string `json:"genres"`
		Ratings       struct {
			Value float64 `json:"value"`
		} `json:"ratings"`
		Images []struct {
			CoverType string `json:"coverType"`
			RemoteURL string `json:"remoteUrl"`
		} `json:"images"`
	} `json:"series"`
	EpisodeFile struct {
		Path string `json:"path"`
	} `json:"episodeFile"`
}

// Poster returns the most relevant image URL (poster if available, otherwise the first).
func (e Episode) Poster() string {
	for _, img := range e.Series.Images {
		if img.CoverType == "poster" {
			return img.RemoteURL
		}
	}
	if len(e.Series.Images) > 0 {
		return e.Series.Images[0].RemoteURL
	}
	return ""
}

// Banner returns a wide image for calendar cards (the Sonarr banner is
// ~758x140, an ideal ratio). Fallback: fanart, then poster.
func (e Episode) Banner() string {
	var fanart, poster string
	for _, img := range e.Series.Images {
		switch img.CoverType {
		case "banner":
			return img.RemoteURL
		case "fanart":
			fanart = img.RemoteURL
		case "poster":
			poster = img.RemoteURL
		}
	}
	if fanart != "" {
		return fanart
	}
	return poster
}

// Calendar retrieves the episodes airing between start and end.
func (c *Client) Calendar(start, end time.Time) ([]Episode, error) {
	u, err := url.Parse(c.BaseURL + "/api/v3/calendar")
	if err != nil {
		return nil, fmt.Errorf("invalid Sonarr URL: %w", err)
	}
	q := u.Query()
	q.Set("start", start.UTC().Format("2006-01-02"))
	q.Set("end", end.UTC().Format("2006-01-02"))
	q.Set("includeSeries", "true")
	q.Set("includeEpisodeFile", "true")
	u.RawQuery = q.Encode()

	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	req.Header.Set("X-Api-Key", c.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Sonarr calendar call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Sonarr responded %s", resp.Status)
	}

	var eps []Episode
	if err := json.NewDecoder(resp.Body).Decode(&eps); err != nil {
		return nil, fmt.Errorf("calendar decoding: %w", err)
	}
	return eps, nil
}

func (c *Client) apiGet(path string) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodGet, c.BaseURL+path, nil)
	req.Header.Set("X-Api-Key", c.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s -> %s", path, resp.Status)
	}
	return b, nil
}

func (c *Client) apiPost(path string, body any) ([]byte, error) {
	j, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, c.BaseURL+path, bytes.NewReader(j))
	req.Header.Set("X-Api-Key", c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s -> %s: %s", path, resp.Status, strings.TrimSpace(string(b)))
	}
	return b, nil
}

// EnsureWebhook creates (if missing) a Webhook notification in Sonarr pointing
// at the agent. Idempotent by name. Returns true if a notification was created.
func (c *Client) EnsureWebhook(name, callbackURL string) (bool, error) {
	b, err := c.apiGet("/api/v3/notification")
	if err != nil {
		return false, err
	}
	var existing []map[string]any
	_ = json.Unmarshal(b, &existing)
	for _, n := range existing {
		if s, _ := n["name"].(string); s == name {
			return false, nil
		}
	}

	b, err = c.apiGet("/api/v3/notification/schema")
	if err != nil {
		return false, err
	}
	var schema []map[string]any
	_ = json.Unmarshal(b, &schema)

	var tpl map[string]any
	for _, s := range schema {
		if impl, _ := s["implementation"].(string); impl == "Webhook" {
			tpl = s
			break
		}
	}
	if tpl == nil {
		return false, fmt.Errorf("Webhook schema not found in Sonarr")
	}

	tpl["name"] = name
	// Enable a trigger only if it is supported by this version of Sonarr.
	setTrigger := func(trig, supports string) {
		if v, _ := tpl[supports].(bool); v {
			tpl[trig] = true
		}
	}
	setTrigger("onGrab", "supportsOnGrab")
	setTrigger("onDownload", "supportsOnDownload")
	setTrigger("onUpgrade", "supportsOnUpgrade")
	setTrigger("onImportComplete", "supportsOnImportComplete")
	setTrigger("onSeriesAdd", "supportsOnSeriesAdd")
	setTrigger("onEpisodeFileDelete", "supportsOnEpisodeFileDelete")

	if fields, ok := tpl["fields"].([]any); ok {
		for _, f := range fields {
			fm, _ := f.(map[string]any)
			switch fm["name"] {
			case "url":
				fm["value"] = callbackURL
			case "method":
				fm["value"] = 1 // POST
			}
		}
	}

	if _, err := c.apiPost("/api/v3/notification", tpl); err != nil {
		return false, err
	}
	return true, nil
}

// DownloadClientCount returns how many download clients are configured, so a
// caller can avoid touching an instance the user has already set up.
func (c *Client) DownloadClientCount() (int, error) {
	b, err := c.apiGet("/api/v3/downloadclient")
	if err != nil {
		return 0, err
	}
	var arr []map[string]any
	if err := json.Unmarshal(b, &arr); err != nil {
		return 0, err
	}
	return len(arr), nil
}

// AddDownloadClient declares qBittorrent as a download client in Sonarr
// (POST /api/v3/downloadclient). Idempotent by name. We start from the schema
// provided by Sonarr and only fill in the connection plus the category.
func (c *Client) AddDownloadClient(name, host string, port int, username, password, category string) (bool, error) {
	b, err := c.apiGet("/api/v3/downloadclient")
	if err != nil {
		return false, err
	}
	var existing []map[string]any
	_ = json.Unmarshal(b, &existing)
	for _, d := range existing {
		if s, _ := d["name"].(string); s == name {
			return false, nil
		}
	}

	b, err = c.apiGet("/api/v3/downloadclient/schema")
	if err != nil {
		return false, err
	}
	var schema []map[string]any
	_ = json.Unmarshal(b, &schema)
	var tpl map[string]any
	for _, s := range schema {
		if impl, _ := s["implementation"].(string); impl == "QBittorrent" {
			tpl = s
			break
		}
	}
	if tpl == nil {
		return false, fmt.Errorf("QBittorrent schema not found in Sonarr")
	}

	tpl["name"] = name
	tpl["enable"] = true
	if fields, ok := tpl["fields"].([]any); ok {
		for _, f := range fields {
			fm, _ := f.(map[string]any)
			switch fm["name"] {
			case "host":
				fm["value"] = host
			case "port":
				fm["value"] = port
			case "username":
				fm["value"] = username
			case "password":
				fm["value"] = password
			case "category", "tvCategory", "movieCategory":
				fm["value"] = category
			}
		}
	}

	if _, err := c.apiPost("/api/v3/downloadclient", tpl); err != nil {
		return false, err
	}
	return true, nil
}

// Release is a release returned by Sonarr's interactive search
// (GET /api/v3/release): queries all indexers configured in Sonarr.
type Release struct {
	GUID      string `json:"guid"`
	Title     string `json:"title"`
	Indexer   string `json:"indexer"`
	IndexerID int    `json:"indexerId"`
	Protocol  string `json:"protocol"`
	Size      int64  `json:"size"`
	Seeders   *int   `json:"seeders"`
	Leechers  *int   `json:"leechers"`
	Age       int    `json:"age"`
	InfoURL   string `json:"infoUrl"`
	Quality   struct {
		Quality struct {
			Name string `json:"name"`
		} `json:"quality"`
	} `json:"quality"`
	Rejected   bool     `json:"rejected"`
	Rejections []string `json:"rejections"`
}

// SearchReleases runs an interactive search for an episode (may take several
// seconds: Sonarr queries the indexers live).
func (c *Client) SearchReleases(episodeID int) ([]Release, error) {
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v3/release?episodeId=%d", c.BaseURL, episodeID), nil)
	req.Header.Set("X-Api-Key", c.APIKey)
	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("release search: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("search -> %s", resp.Status)
	}
	var rels []Release
	if err := json.Unmarshal(b, &rels); err != nil {
		return nil, fmt.Errorf("releases decoding: %w", err)
	}
	return rels, nil
}

// GrabRelease sends a release to the download client via Sonarr
// (POST /api/v3/release). Sonarr handles the download and then the import into
// the library; progress is then reported via the queue.
func (c *Client) GrabRelease(guid string, indexerID int) error {
	_, err := c.apiPost("/api/v3/release", map[string]any{"guid": guid, "indexerId": indexerID})
	return err
}

// RefreshDownloads forces Sonarr to resync its queue with the download client
// (qBittorrent) immediately, instead of waiting for its internal interval
// (~60s). Without this, a download completed on the qBit side keeps showing
// "in progress" in the Sonarr queue for nearly a minute.
func (c *Client) RefreshDownloads() error {
	_, err := c.apiPost("/api/v3/command", map[string]any{"name": "RefreshMonitoredDownloads"})
	return err
}

// --- Add series ---

func (c *Client) LookupSeries(term string) ([]byte, error) {
	return c.apiGet("/api/v3/series/lookup?term=" + url.QueryEscape(term))
}
func (c *Client) QualityProfiles() ([]byte, error) { return c.apiGet("/api/v3/qualityprofile") }

// Disk is the space of a disk as reported by Sonarr (/api/v3/diskspace).
type Disk struct {
	Path       string `json:"path"`
	Label      string `json:"label"`
	FreeSpace  int64  `json:"freeSpace"`
	TotalSpace int64  `json:"totalSpace"`
}

func (c *Client) DiskSpace() ([]Disk, error) {
	b, err := c.apiGet("/api/v3/diskspace")
	if err != nil {
		return nil, err
	}
	var d []Disk
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("diskspace decoding: %w", err)
	}
	return d, nil
}

// RootFolder is a Sonarr root folder (where media is imported).
type RootFolder struct {
	Path string `json:"path"`
}

func (c *Client) RootFolderPaths() ([]RootFolder, error) {
	b, err := c.apiGet("/api/v3/rootfolder")
	if err != nil {
		return nil, err
	}
	var r []RootFolder
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("rootfolder decoding: %w", err)
	}
	return r, nil
}

// EnsureRootFolder adds a root folder if it isn't already declared (otherwise
// Sonarr refuses to add a series: "Root Folder Path must not be empty").
// Returns true if a folder was created. Comparison is case-insensitive and
// ignores trailing slashes (Windows).
func (c *Client) EnsureRootFolder(path string) (bool, error) {
	norm := func(p string) string { return strings.ToLower(strings.TrimRight(p, `\/`)) }
	if existing, err := c.RootFolderPaths(); err == nil {
		for _, rf := range existing {
			if norm(rf.Path) == norm(path) {
				return false, nil
			}
		}
	}
	if _, err := c.apiPost("/api/v3/rootfolder", map[string]string{"path": path}); err != nil {
		return false, err
	}
	return true, nil
}
func (c *Client) Tags() ([]byte, error)            { return c.apiGet("/api/v3/tag") }
func (c *Client) RootFolders() ([]byte, error)     { return c.apiGet("/api/v3/rootfolder") }

// IndexerNames returns the names of the indexers configured in Sonarr (those
// pushed by Prowlarr are named "<name> (Prowlarr)").
func (c *Client) IndexerNames() ([]string, error) {
	b, err := c.apiGet("/api/v3/indexer")
	if err != nil {
		return nil, err
	}
	var arr []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &arr); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(arr))
	for _, a := range arr {
		names = append(names, a.Name)
	}
	return names, nil
}
func (c *Client) CreateTag(label string) ([]byte, error) {
	return c.apiPost("/api/v3/tag", map[string]any{"label": label})
}

// AddOptions holds the user's choices when adding a series.
type AddOptions struct {
	QualityProfileID int
	RootFolderPath   string
	Monitored        bool
	SeriesType       string // standard, daily, anime
	Monitor          string // all, future, missing, existing, firstSeason, latestSeason, pilot, none
	SearchNow        bool
	Tags             []int
}

// AddSeries adds a series. We re-fetch the full object via lookup (tvdb:ID)
// to gather what Sonarr expects (images, seasons...), then complete it with
// the user's choices before the POST.
func (c *Client) AddSeries(tvdbID int, o AddOptions) ([]byte, error) {
	b, err := c.apiGet(fmt.Sprintf("/api/v3/series/lookup?term=tvdb:%d", tvdbID))
	if err != nil {
		return nil, err
	}
	var arr []map[string]any
	if err := json.Unmarshal(b, &arr); err != nil {
		return nil, err
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("series not found (tvdb %d)", tvdbID)
	}
	s := arr[0]
	s["qualityProfileId"] = o.QualityProfileID
	s["rootFolderPath"] = o.RootFolderPath
	s["monitored"] = o.Monitored
	s["seasonFolder"] = true
	if o.SeriesType != "" {
		s["seriesType"] = o.SeriesType
	}
	if o.Tags != nil {
		s["tags"] = o.Tags
	} else {
		s["tags"] = []int{}
	}
	s["addOptions"] = map[string]any{
		"monitor":                      o.Monitor,
		"searchForMissingEpisodes":     o.SearchNow,
		"searchForCutoffUnmetEpisodes": false,
	}
	return c.apiPost("/api/v3/series", s)
}

// QueueItem is an in-progress download, normalized for pushing.
type QueueItem struct {
	EpisodeID int
	Status    string
	Percent   int
	TimeLeft  string
}

type queueRecord struct {
	EpisodeID    int     `json:"episodeId"`
	Size         float64 `json:"size"`
	SizeLeft     float64 `json:"sizeleft"`
	TimeLeft     string  `json:"timeleft"`
	Status       string  `json:"status"`
	TrackedState string  `json:"trackedDownloadState"` // downloading, importPending, importing, imported...
}

type queueResponse struct {
	Records []queueRecord `json:"records"`
}

// Queue retrieves Sonarr's download queue (which reflects qBittorrent).
func (c *Client) Queue() ([]QueueItem, error) {
	b, err := c.apiGet("/api/v3/queue?pageSize=200")
	if err != nil {
		return nil, err
	}
	var qr queueResponse
	if err := json.Unmarshal(b, &qr); err != nil {
		return nil, err
	}

	// An episode can have multiple entries (e.g. 4K in progress + 720p done).
	// We keep the most "active" one and ignore the "completed" entries (which
	// will become "available" via the import webhook).
	// norm normalizes an entry into (status, rank). We keep the most advanced
	// entry per episode. "importing" = Sonarr is moving the file into the
	// library (after 100%); we expose it so it can be shown on the calendar.
	norm := func(r queueRecord) (string, int) {
		switch strings.ToLower(r.TrackedState) {
		case "importing":
			return "importing", 4 // Sonarr is actively moving the file into the library
		case "importpending", "importblocked":
			return "pending", 4 // downloaded, awaiting import (sometimes blocked by Sonarr)
		}
		switch strings.ToLower(r.Status) {
		case "downloading":
			return "downloading", 3
		case "queued":
			return "queued", 2
		case "paused":
			return "paused", 1
		default: // completed (already imported), warning, delay, failed...
			return "", 0
		}
	}

	best := map[int]QueueItem{}
	bestRank := map[int]int{}
	for _, r := range qr.Records {
		st, rk := norm(r)
		if r.EpisodeID <= 0 || rk == 0 {
			continue
		}
		pct := 0
		if r.Size > 0 {
			pct = int(((r.Size - r.SizeLeft) / r.Size) * 100)
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
		}
		if st == "importing" {
			pct = 100
		}
		if cur, ok := bestRank[r.EpisodeID]; !ok || cur < rk {
			best[r.EpisodeID] = QueueItem{EpisodeID: r.EpisodeID, Status: st, Percent: pct, TimeLeft: r.TimeLeft}
			bestRank[r.EpisodeID] = rk
		}
	}

	items := make([]QueueItem, 0, len(best))
	for _, it := range best {
		items = append(items, it)
	}
	return items, nil
}
