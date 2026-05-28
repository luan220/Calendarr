// Package radarr talks to the local Radarr API (movies). Same approach as the
// sonarr package: the API key is read from the local config.xml (zero-config
// install when running on the Radarr machine). Radarr often listens only on localhost.
package radarr

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
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

type xmlConfig struct {
	APIKey  string `xml:"ApiKey"`
	Port    int    `xml:"Port"`
	URLBase string `xml:"UrlBase"`
}

func configPaths() []string {
	var p []string
	if pd := os.Getenv("ProgramData"); pd != "" {
		p = append(p, filepath.Join(pd, "Radarr", "config.xml"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		p = append(p,
			filepath.Join(home, "AppData", "Roaming", "Radarr", "config.xml"),
			filepath.Join(home, ".config", "Radarr", "config.xml"),
		)
	}
	return p
}

func readLocalConfig() (string, string, error) {
	var lastErr error = fmt.Errorf("Radarr config.xml not found")
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
			port = 7878
		}
		base := fmt.Sprintf("http://localhost:%d", port)
		if ub := strings.Trim(c.URLBase, "/"); ub != "" {
			base += "/" + ub
		}
		return base, c.APIKey, nil
	}
	return "", "", lastErr
}

// Movie is the subset of a Radarr movie that we care about.
type Movie struct {
	ID         int    `json:"id"`
	Title      string `json:"title"`
	Year       int    `json:"year"`
	Overview   string `json:"overview"`
	HasFile    bool   `json:"hasFile"`
	Monitored  bool   `json:"monitored"`
	Status     string `json:"status"`
	TitleSlug  string `json:"titleSlug"`
	TmdbID     int    `json:"tmdbId"`
	Runtime    int    `json:"runtime"`
	SizeOnDisk int64  `json:"sizeOnDisk"`
	Images     []struct {
		CoverType string `json:"coverType"`
		RemoteURL string `json:"remoteUrl"`
	} `json:"images"`
	MovieFile struct {
		Path string `json:"path"`
	} `json:"movieFile"`
}

func (m Movie) image(t string) string {
	for _, img := range m.Images {
		if img.CoverType == t {
			return img.RemoteURL
		}
	}
	return ""
}

func (m Movie) Poster() string { return m.image("poster") }

// Banner returns a wide image for cards (movies mainly have fanart).
func (m Movie) Banner() string {
	if u := m.image("fanart"); u != "" {
		return u
	}
	return m.image("poster")
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

// Library returns the entire movie library.
func (c *Client) Library() ([]Movie, error) {
	b, err := c.apiGet("/api/v3/movie")
	if err != nil {
		return nil, err
	}
	var ms []Movie
	if err := json.Unmarshal(b, &ms); err != nil {
		return nil, err
	}
	return ms, nil
}

// MovieFilePath returns the file path of a movie (for playback).
func (c *Client) MovieFilePath(id int) (string, error) {
	b, err := c.apiGet(fmt.Sprintf("/api/v3/movie/%d", id))
	if err != nil {
		return "", err
	}
	var m Movie
	if err := json.Unmarshal(b, &m); err != nil {
		return "", err
	}
	if m.MovieFile.Path == "" {
		return "", fmt.Errorf("no file for movie %d", id)
	}
	return m.MovieFile.Path, nil
}

func (c *Client) LookupMovies(term string) ([]byte, error) {
	return c.apiGet("/api/v3/movie/lookup?term=" + url.QueryEscape(term))
}
func (c *Client) QualityProfiles() ([]byte, error) { return c.apiGet("/api/v3/qualityprofile") }
func (c *Client) Tags() ([]byte, error)            { return c.apiGet("/api/v3/tag") }
func (c *Client) RootFolders() ([]byte, error)     { return c.apiGet("/api/v3/rootfolder") }

// RootFolder is a Radarr root folder (where movies are imported).
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
// Radarr refuses to add a movie). Returns true if a folder was created.
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

// IndexerNames returns the names of the indexers configured in Radarr (those
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

// AddOptions holds the user's choices when adding a movie.
type AddOptions struct {
	QualityProfileID    int
	RootFolderPath      string
	Monitored           bool
	MinimumAvailability string // announced, inCinemas, released
	SearchNow           bool
	Tags                []int
}

func (c *Client) AddMovie(tmdbID int, o AddOptions) ([]byte, error) {
	b, err := c.apiGet(fmt.Sprintf("/api/v3/movie/lookup?term=tmdb:%d", tmdbID))
	if err != nil {
		return nil, err
	}
	var arr []map[string]any
	if err := json.Unmarshal(b, &arr); err != nil {
		return nil, err
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("movie not found (tmdb %d)", tmdbID)
	}
	m := arr[0]
	m["qualityProfileId"] = o.QualityProfileID
	m["rootFolderPath"] = o.RootFolderPath
	m["monitored"] = o.Monitored
	if o.MinimumAvailability != "" {
		m["minimumAvailability"] = o.MinimumAvailability
	}
	if o.Tags != nil {
		m["tags"] = o.Tags
	} else {
		m["tags"] = []int{}
	}
	m["addOptions"] = map[string]any{"searchForMovie": o.SearchNow}
	return c.apiPost("/api/v3/movie", m)
}

// Release is a release from the interactive search (same fields as Sonarr).
type Release struct {
	GUID      string `json:"guid"`
	Title     string `json:"title"`
	Indexer   string `json:"indexer"`
	IndexerID int    `json:"indexerId"`
	Protocol  string `json:"protocol"`
	Size      int64  `json:"size"`
	Seeders   *int   `json:"seeders"`
	Age       int    `json:"age"`
	Quality   struct {
		Quality struct {
			Name string `json:"name"`
		} `json:"quality"`
	} `json:"quality"`
	Rejected   bool     `json:"rejected"`
	Rejections []string `json:"rejections"`
}

func (c *Client) SearchReleases(movieID int) ([]Release, error) {
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v3/release?movieId=%d", c.BaseURL, movieID), nil)
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
		return nil, err
	}
	return rels, nil
}

func (c *Client) GrabRelease(guid string, indexerID int) error {
	_, err := c.apiPost("/api/v3/release", map[string]any{"guid": guid, "indexerId": indexerID})
	return err
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

// AddDownloadClient declares qBittorrent as a download client in Radarr
// (POST /api/v3/downloadclient). Idempotent by name.
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
		return false, fmt.Errorf("QBittorrent schema not found in Radarr")
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
