// Package bazarr talks to a local Bazarr instance (subtitles companion for
// Sonarr/Radarr). API key is read from Bazarr's config.ini (zero-config when
// running on the same machine). Bazarr listens on 6767 by default, often only
// on localhost.
package bazarr

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	APIKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) (*Client, error) {
	if baseURL == "" || apiKey == "" {
		u, k, err := readLocalConfig()
		if err != nil {
			return nil, err
		}
		if baseURL == "" {
			baseURL = u
		}
		if apiKey == "" {
			apiKey = k
		}
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		http: &http.Client{
			Timeout:   15 * time.Second,
			Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext},
		},
	}, nil
}

func configPaths() []string {
	var p []string
	// ProgramData / AppData (managed installer)
	if pd := os.Getenv("ProgramData"); pd != "" {
		p = append(p,
			filepath.Join(pd, "Bazarr", "data", "config", "config.ini"),
			filepath.Join(pd, "Bazarr", "config", "config.ini"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil {
		p = append(p,
			filepath.Join(home, "AppData", "Roaming", "Bazarr", "data", "config", "config.ini"),
			filepath.Join(home, "AppData", "Roaming", "Bazarr", "config", "config.ini"),
			filepath.Join(home, ".config", "bazarr", "config", "config.ini"),
		)
	}
	// Portable installs: Bazarr's Windows zip is often unpacked to a top-level
	// folder (C:\Bazarr is the most common), with `data\config\config.ini`
	// created next to bazarr.exe on first run. Cover the usual drive letters
	// and Program Files locations.
	roots := []string{`C:\Bazarr`, `D:\Bazarr`, `E:\Bazarr`}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if pf := os.Getenv(env); pf != "" {
			roots = append(roots, filepath.Join(pf, "Bazarr"))
		}
	}
	for _, root := range roots {
		p = append(p,
			filepath.Join(root, "data", "config", "config.ini"),
			filepath.Join(root, "config", "config.ini"),
		)
	}
	return p
}

// parseINI extracts apikey, port and base_url from the [general] section of a
// Bazarr config.ini. Minimal parser, just the few keys we need — no dependency.
func parseINI(r io.Reader) (apiKey string, port int, baseURL string) {
	sc := bufio.NewScanner(r)
	section := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		if section != "general" {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, `"'`)
		switch key {
		case "apikey":
			apiKey = val
		case "port":
			fmt.Sscanf(val, "%d", &port)
		case "base_url":
			baseURL = val
		}
	}
	return
}

func readLocalConfig() (string, string, error) {
	var lastErr error = fmt.Errorf("Bazarr config.ini not found")
	for _, p := range configPaths() {
		f, e := os.Open(p)
		if e != nil {
			lastErr = e
			continue
		}
		apiKey, port, ub := parseINI(f)
		f.Close()
		if apiKey == "" {
			lastErr = fmt.Errorf("apikey empty in %s", p)
			continue
		}
		if port == 0 {
			port = 6767
		}
		base := fmt.Sprintf("http://localhost:%d", port)
		if ub = strings.Trim(ub, "/"); ub != "" {
			base += "/" + ub
		}
		return base, apiKey, nil
	}
	return "", "", lastErr
}

func (c *Client) req(method, path string, body []byte) ([]byte, error) {
	return c.reqCT(method, path, body, "application/json")
}

func (c *Client) reqCT(method, path string, body []byte, contentType string) ([]byte, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, c.BaseURL+path, r)
	req.Header.Set("X-API-KEY", c.APIKey) // Bazarr is case-sensitive (uppercase)
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
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

// Status is what Bazarr exposes about itself; used as a health check.
type Status struct {
	BazarrVersion  string `json:"bazarr_version"`
	OperatingSystem string `json:"operating_system"`
	PythonVersion  string `json:"python_version"`
	StartTime      int64  `json:"start_time"`
}

func (c *Client) Status() (*Status, error) {
	b, err := c.req(http.MethodGet, "/api/system/status", nil)
	if err != nil {
		return nil, err
	}
	// Bazarr wraps it in {"data": {...}}.
	var wrap struct {
		Data Status `json:"data"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return nil, err
	}
	return &wrap.Data, nil
}

// Language is a configured language in Bazarr.
type Language struct {
	Code2   string `json:"code2"`
	Code3   string `json:"code3"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// Languages lists all languages known to Bazarr (set enabled=true to filter to
// the ones the user actually wants).
func (c *Client) Languages(enabledOnly bool) ([]Language, error) {
	q := ""
	if enabledOnly {
		q = "?enabled=true"
	}
	b, err := c.req(http.MethodGet, "/api/system/languages"+q, nil)
	if err != nil {
		return nil, err
	}
	var langs []Language
	if err := json.Unmarshal(b, &langs); err != nil {
		return nil, err
	}
	return langs, nil
}

// WantedEpisode is an episode missing one or more subtitle languages.
type WantedEpisode struct {
	SeriesID         int      `json:"sonarrSeriesId"`
	EpisodeID        int      `json:"sonarrEpisodeId"`
	SeriesTitle      string   `json:"seriesTitle"`
	EpisodeTitle     string   `json:"episodeTitle"`
	EpisodeNumber    string   `json:"episode_number"` // "S03E07"
	MissingLanguages []string `json:"missing_subtitles_languages,omitempty"`
}

// WantedMovie is a movie missing one or more subtitle languages.
type WantedMovie struct {
	MovieID          int      `json:"radarrId"`
	Title            string   `json:"title"`
	Year             string   `json:"year,omitempty"`
	MissingLanguages []string `json:"missing_subtitles_languages,omitempty"`
}

// WantedEpisodes returns episodes whose subtitles Bazarr has not been able to
// fetch yet. Paginated server-side; we ask for a reasonable first page.
func (c *Client) WantedEpisodes(length int) ([]WantedEpisode, int, error) {
	if length <= 0 {
		length = 100
	}
	b, err := c.req(http.MethodGet, fmt.Sprintf("/api/episodes/wanted?start=0&length=%d", length), nil)
	if err != nil {
		return nil, 0, err
	}
	var wrap struct {
		Data  []WantedEpisode `json:"data"`
		Total int             `json:"total"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		// Some Bazarr versions return the missing-languages field as objects
		// rather than plain strings; in that case we re-parse loosely so we
		// at least get the rest of the payload.
		var loose struct {
			Data  []map[string]any `json:"data"`
			Total int              `json:"total"`
		}
		if err2 := json.Unmarshal(b, &loose); err2 != nil {
			return nil, 0, err
		}
		out := make([]WantedEpisode, 0, len(loose.Data))
		for _, m := range loose.Data {
			out = append(out, WantedEpisode{
				SeriesID:         intOf(m["sonarrSeriesId"]),
				EpisodeID:        intOf(m["sonarrEpisodeId"]),
				SeriesTitle:      strOf(m["seriesTitle"]),
				EpisodeTitle:     strOf(m["episodeTitle"]),
				EpisodeNumber:    strOf(m["episode_number"]),
				MissingLanguages: langsOf(m["missing_subtitles_languages"]),
			})
		}
		return out, loose.Total, nil
	}
	return wrap.Data, wrap.Total, nil
}

// WantedMovies returns movies whose subtitles Bazarr has not been able to fetch yet.
func (c *Client) WantedMovies(length int) ([]WantedMovie, int, error) {
	if length <= 0 {
		length = 100
	}
	b, err := c.req(http.MethodGet, fmt.Sprintf("/api/movies/wanted?start=0&length=%d", length), nil)
	if err != nil {
		return nil, 0, err
	}
	var wrap struct {
		Data  []WantedMovie `json:"data"`
		Total int           `json:"total"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		var loose struct {
			Data  []map[string]any `json:"data"`
			Total int              `json:"total"`
		}
		if err2 := json.Unmarshal(b, &loose); err2 != nil {
			return nil, 0, err
		}
		out := make([]WantedMovie, 0, len(loose.Data))
		for _, m := range loose.Data {
			out = append(out, WantedMovie{
				MovieID:          intOf(m["radarrId"]),
				Title:            strOf(m["title"]),
				Year:             strOf(m["year"]),
				MissingLanguages: langsOf(m["missing_subtitles_languages"]),
			})
		}
		return out, loose.Total, nil
	}
	return wrap.Data, wrap.Total, nil
}

// HistoryItem is one entry in Bazarr's subtitle download history.
type HistoryItem struct {
	Action       int    `json:"action"`
	Language     string `json:"language"`
	Provider     string `json:"provider"`
	Score        any    `json:"score"` // sometimes string ("95%"), sometimes int
	Timestamp    string `json:"timestamp"`
	SeriesTitle  string `json:"seriesTitle,omitempty"`
	EpisodeTitle string `json:"episodeTitle,omitempty"`
	MovieTitle   string `json:"title,omitempty"`
	Description  string `json:"description,omitempty"`
}

// HistoryEpisodes returns recent subtitle downloads for series.
func (c *Client) HistoryEpisodes(length int) ([]HistoryItem, error) {
	if length <= 0 {
		length = 50
	}
	b, err := c.req(http.MethodGet, fmt.Sprintf("/api/episodes/history?start=0&length=%d", length), nil)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Data []HistoryItem `json:"data"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return nil, err
	}
	return wrap.Data, nil
}

// HistoryMovies returns recent subtitle downloads for movies.
func (c *Client) HistoryMovies(length int) ([]HistoryItem, error) {
	if length <= 0 {
		length = 50
	}
	b, err := c.req(http.MethodGet, fmt.Sprintf("/api/movies/history?start=0&length=%d", length), nil)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Data []HistoryItem `json:"data"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return nil, err
	}
	return wrap.Data, nil
}

// Provider describes one subtitle provider Bazarr knows about (configured or not).
type Provider struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "good" / error message
	Retry   string `json:"retry"`  // "never" or a duration
}

// CurrentSubtitle represents a subtitle track already present on disk for a
// given episode (or movie). Bazarr returns these inline with the episode/movie
// payload as a list of [language_name, file_path] pairs OR objects, depending
// on Bazarr version. We accept any reasonable shape via loose parsing.
type CurrentSubtitle struct {
	Name string `json:"name"` // e.g. "French" or "English (forced)"
	Path string `json:"path,omitempty"`
	Code string `json:"code,omitempty"`
}

// EpisodeSubtitles returns the subtitle tracks Bazarr knows about for one
// episode (i.e. files already on disk next to the video). Fast call. Returns
// an empty slice when Bazarr has no record of the episode (rather than an
// error) so the UI can render a clean "no subs yet" state.
func (c *Client) EpisodeSubtitles(seriesID, episodeID int) ([]CurrentSubtitle, error) {
	if seriesID <= 0 || episodeID <= 0 {
		return nil, fmt.Errorf("seriesID and episodeID required")
	}
	b, err := c.req(http.MethodGet, fmt.Sprintf("/api/episodes?seriesid[]=%d&episodeid[]=%d", seriesID, episodeID), nil)
	if err != nil {
		// Some Bazarr versions reject the bracketed form; retry with the
		// single-key variant.
		b, err = c.req(http.MethodGet, fmt.Sprintf("/api/episodes?seriesid=%d&episodeid=%d", seriesID, episodeID), nil)
		if err != nil {
			return nil, err
		}
	}
	var wrap struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return nil, err
	}
	if len(wrap.Data) == 0 {
		return nil, nil
	}
	ep := wrap.Data[0]
	return parseSubsList(ep["subtitles"]), nil
}

// SearchResult is one subtitle candidate returned by Bazarr's manual search.
// We keep the raw payload around (Raw) so the download call can echo it back
// verbatim — the Bazarr download endpoint expects the exact provider/hash/url
// fields it produced, and these vary across providers.
type SearchResult struct {
	Language         string `json:"language"`
	HearingImpaired  any    `json:"hearing_impaired"` // bool or string
	Forced           any    `json:"forced"`           // bool or string
	Provider         string `json:"provider"`
	Subtitle         string `json:"subtitle"` // hash/identifier for the download call
	URL              string `json:"url,omitempty"`
	Score            int    `json:"score"`
	OriginalFormat   bool   `json:"original_format,omitempty"`
	Uploader         string `json:"uploader,omitempty"`
	ReleaseInfo      []string `json:"release_info,omitempty"`
	Matches          []string `json:"matches,omitempty"`
	DontMatches      []string `json:"dont_matches,omitempty"`
	Raw              map[string]any `json:"-"` // full payload (for the download call)
}

// SearchEpisodeSubtitles runs Bazarr's manual search against all configured
// providers for one episode. Slow (10-30s). Bazarr aggregates results across
// providers and returns them sorted by score.
func (c *Client) SearchEpisodeSubtitles(episodeID int) ([]SearchResult, error) {
	if episodeID <= 0 {
		return nil, fmt.Errorf("episodeID required")
	}
	// Manual searches can take a while: bump the per-call timeout above the
	// client default (15s) so we don't drop legit slow searches.
	client := &http.Client{Timeout: 90 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/providers/episodes?episodeid=%d", c.BaseURL, episodeID), nil)
	req.Header.Set("X-API-KEY", c.APIKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("subtitle search: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("subtitle search -> %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	// Bazarr usually returns a top-level array, but a few endpoints wrap in
	// {"data": [...]}. Try both.
	var raw []map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		var wrap struct {
			Data []map[string]any `json:"data"`
		}
		if err2 := json.Unmarshal(b, &wrap); err2 != nil {
			return nil, err
		}
		raw = wrap.Data
	}
	out := make([]SearchResult, 0, len(raw))
	for _, m := range raw {
		sr := SearchResult{
			Language:        strOf(m["language"]),
			HearingImpaired: m["hearing_impaired"],
			Forced:          m["forced"],
			Provider:        strOf(m["provider"]),
			Subtitle:        strOf(m["subtitle"]),
			URL:             strOf(m["url"]),
			Score:           intOf(m["score"]),
			Uploader:        strOf(m["uploader"]),
			Raw:             m,
		}
		if v, ok := m["original_format"].(bool); ok {
			sr.OriginalFormat = v
		}
		if arr, ok := m["release_info"].([]any); ok {
			for _, x := range arr {
				if s, ok := x.(string); ok {
					sr.ReleaseInfo = append(sr.ReleaseInfo, s)
				}
			}
		}
		out = append(out, sr)
	}
	return out, nil
}

// DownloadEpisodeSubtitle asks Bazarr to fetch one specific subtitle for an
// episode. The `raw` payload should be the full SearchResult.Raw — Bazarr
// validates against the original provider fields.
func (c *Client) DownloadEpisodeSubtitle(seriesID, episodeID int, raw map[string]any) error {
	if seriesID <= 0 || episodeID <= 0 {
		return fmt.Errorf("seriesID and episodeID required")
	}
	// Bazarr's POST /api/providers/episodes expects form-encoded fields
	// matching what the GET returned, plus seriesid + episodeid.
	form := url.Values{}
	form.Set("seriesid", fmt.Sprintf("%d", seriesID))
	form.Set("episodeid", fmt.Sprintf("%d", episodeID))
	for k, v := range raw {
		switch x := v.(type) {
		case string:
			form.Set(k, x)
		case float64:
			form.Set(k, fmt.Sprintf("%g", x))
		case bool:
			if x {
				form.Set(k, "True")
			} else {
				form.Set(k, "False")
			}
		}
	}
	_, err := c.reqCT(http.MethodPost, "/api/providers/episodes", []byte(form.Encode()), "application/x-www-form-urlencoded")
	return err
}

// parseSubsList accepts the various shapes Bazarr uses for the "subtitles"
// field of an episode/movie and normalizes to a flat list.
func parseSubsList(v any) []CurrentSubtitle {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]CurrentSubtitle, 0, len(arr))
	for _, it := range arr {
		switch x := it.(type) {
		case []any:
			// Legacy [name, path] tuple.
			if len(x) >= 1 {
				cs := CurrentSubtitle{Name: strOf(x[0])}
				if len(x) >= 2 {
					cs.Path = strOf(x[1])
				}
				out = append(out, cs)
			}
		case map[string]any:
			cs := CurrentSubtitle{
				Name: strOf(x["name"]),
				Path: strOf(x["path"]),
				Code: strOf(x["code2"]),
			}
			if cs.Name == "" {
				cs.Name = strOf(x["language"])
			}
			out = append(out, cs)
		case string:
			out = append(out, CurrentSubtitle{Name: x})
		}
	}
	return out
}

// Providers returns the list of subtitle providers and their current status.
func (c *Client) Providers() ([]Provider, error) {
	b, err := c.req(http.MethodGet, "/api/providers", nil)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Data []Provider `json:"data"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return nil, err
	}
	return wrap.Data, nil
}

// --- helpers used by the loose-parse fallbacks ---

func intOf(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func strOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// langsOf extracts a list of language names whether Bazarr serves them as
// strings ["en", "fr"] or objects [{"name": "English", ...}, ...].
func langsOf(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		switch x := it.(type) {
		case string:
			out = append(out, x)
		case map[string]any:
			if s, ok := x["name"].(string); ok && s != "" {
				out = append(out, s)
				continue
			}
			if s, ok := x["code2"].(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

