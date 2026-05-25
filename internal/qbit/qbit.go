// Package qbit parle à l'API WebUI de qBittorrent (v2). Gère le cookie de
// session et reconnecte si elle expire. Compatible qBittorrent 4.x et 5.x
// (les actions pause/resume ont été renommées stop/start en 5.x).
package qbit

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	base string
	user string
	pass string
	http *http.Client

	mu     sync.Mutex
	authed bool
}

func New(base, user, pass string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		base: strings.TrimRight(base, "/"),
		user: user,
		pass: pass,
		http: &http.Client{Timeout: 15 * time.Second, Jar: jar},
	}
}

// Connected teste s'il existe une session valide SANS tenter de login (pour ne pas
// accumuler des échecs qui feraient bannir l'IP par qBittorrent). 200 = connecté
// (cookie valide, ou authentification localhost contournée).
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	req, _ := http.NewRequest(http.MethodGet, c.base+"/api/v2/app/version", nil)
	req.Header.Set("Referer", c.base)
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// Authenticate tente un login. Renvoie ErrBanned si l'IP est bannie, une autre
// erreur si les identifiants sont refusés, nil si OK.
func (c *Client) Authenticate() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.login()
}

// SetCreds change l'utilisateur/mot de passe et force une nouvelle session.
func (c *Client) SetCreds(user, pass string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.user = user
	c.pass = pass
	c.authed = false
	if jar, err := cookiejar.New(nil); err == nil {
		c.http.Jar = jar
	}
}

// Detection = ce qu'on a pu déduire de l'install qBittorrent locale.
type Detection struct {
	Installed    bool
	WebUIEnabled bool
	Port         int
	Username     string
	URL          string
}

// Detect lit le qBittorrent.ini local pour en déduire la config WebUI (port,
// utilisateur, activée ?) et si qBittorrent est installé. Le mot de passe WebUI
// y est haché : il n'est PAS récupérable (l'utilisateur doit le saisir).
func Detect() Detection {
	d := Detection{Username: "admin"}
	if prefs := readQbitPrefs(); prefs != nil {
		d.Installed = true
		d.WebUIEnabled = strings.EqualFold(strings.TrimSpace(prefs["WebUI\\Enabled"]), "true")
		if p, err := strconv.Atoi(strings.TrimSpace(prefs["WebUI\\Port"])); err == nil && p > 0 {
			d.Port = p
		}
		if u := strings.TrimSpace(prefs["WebUI\\Username"]); u != "" {
			d.Username = u
		}
	}
	if !d.Installed && qbitExeExists() {
		d.Installed = true
	}
	if d.Port == 0 {
		d.Port = 8080 // port WebUI par défaut de qBittorrent
	}
	d.URL = fmt.Sprintf("http://localhost:%d", d.Port)
	return d
}

// readQbitPrefs renvoie les clés de la section [Preferences] du qBittorrent.ini.
func readQbitPrefs() map[string]string {
	for _, p := range qbitConfigPaths() {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		prefs := map[string]string{}
		section := ""
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, ";") {
				continue
			}
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				section = strings.ToLower(line[1 : len(line)-1])
				continue
			}
			if section != "preferences" {
				continue
			}
			if i := strings.Index(line, "="); i > 0 {
				prefs[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
			}
		}
		f.Close()
		return prefs
	}
	return nil
}

func qbitConfigPaths() []string {
	var paths []string
	if cfg, err := os.UserConfigDir(); err == nil {
		paths = append(paths,
			filepath.Join(cfg, "qBittorrent", "qBittorrent.ini"),  // Windows: %AppData%\Roaming
			filepath.Join(cfg, "qBittorrent", "qBittorrent.conf"), // Linux: ~/.config
		)
	}
	return paths
}

func qbitExeExists() bool {
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if pf := os.Getenv(env); pf != "" {
			if _, err := os.Stat(filepath.Join(pf, "qBittorrent", "qbittorrent.exe")); err == nil {
				return true
			}
		}
	}
	return false
}

// ErrBanned : qBittorrent a temporairement banni l'IP après trop d'échecs de login.
var ErrBanned = errors.New("qbit: adresse temporairement bannie par qBittorrent")

func (c *Client) login() error {
	form := url.Values{"username": {c.user}, "password": {c.pass}}
	req, _ := http.NewRequest(http.MethodPost, c.base+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", c.base) // qBittorrent exige un Referer = hôte
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("connexion qBittorrent: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusForbidden {
		return ErrBanned // qBit renvoie 403 sur /auth/login quand l'IP est bannie
	}
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(b)) != "Ok." {
		return fmt.Errorf("identifiants qBittorrent refusés (%s)", resp.Status)
	}
	c.authed = true
	return nil
}

// do exécute une requête en (re)connectant la session si besoin.
func (c *Client) do(method, path string, form url.Values) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.authed {
		// Login best-effort : si qBittorrent contourne l'authentification pour
		// localhost / le réseau privé (notre config WebUI), l'appel passe même
		// sans login valide. On ne bloque donc pas sur un échec de login ici — un
		// vrai refus se manifestera par un 403 ci-dessous (qui retente un login).
		_ = c.login()
	}
	call := func() (*http.Response, error) {
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req, _ := http.NewRequest(method, c.base+path, body)
		req.Header.Set("Referer", c.base)
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		return c.http.Do(req)
	}
	resp, err := call()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusForbidden { // session expirée
		resp.Body.Close()
		c.authed = false
		if err := c.login(); err != nil {
			return nil, err
		}
		if resp, err = call(); err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s -> %s", path, resp.Status)
	}
	c.authed = true // l'appel a réussi (login OU contournement) → on évite de relogin à chaque fois
	return b, nil
}

// Torrent = sous-ensemble des champs qBittorrent qui nous intéressent.
type Torrent struct {
	Hash      string  `json:"hash"`
	Name      string  `json:"name"`
	State     string  `json:"state"`
	Progress  float64 `json:"progress"`
	Size      int64   `json:"size"`
	DlSpeed   int64   `json:"dlspeed"`
	UpSpeed   int64   `json:"upspeed"`
	Ratio     float64 `json:"ratio"`
	NumSeeds  int     `json:"num_seeds"`
	NumLeechs int     `json:"num_leechs"`
	Eta       int64   `json:"eta"`
	AddedOn   int64   `json:"added_on"`
}

func (c *Client) Torrents() ([]Torrent, error) {
	b, err := c.do(http.MethodGet, "/api/v2/torrents/info", nil)
	if err != nil {
		return nil, err
	}
	var ts []Torrent
	if err := json.Unmarshal(b, &ts); err != nil {
		return nil, err
	}
	return ts, nil
}

// setState tente l'endpoint moderne (qBit 5.x) puis l'ancien (4.x).
func (c *Client) setState(hashes, modern, legacy string) error {
	form := url.Values{"hashes": {hashes}}
	if _, err := c.do(http.MethodPost, "/api/v2/torrents/"+modern, form); err != nil {
		if _, err2 := c.do(http.MethodPost, "/api/v2/torrents/"+legacy, form); err2 != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Pause(hashes string) error  { return c.setState(hashes, "stop", "pause") }
func (c *Client) Resume(hashes string) error { return c.setState(hashes, "start", "resume") }

func (c *Client) Delete(hashes string, deleteFiles bool) error {
	form := url.Values{"hashes": {hashes}, "deleteFiles": {fmt.Sprintf("%t", deleteFiles)}}
	_, err := c.do(http.MethodPost, "/api/v2/torrents/delete", form)
	return err
}

// SetDownloadPaths fixe le dossier de téléchargement final et, si fourni, le
// dossier temporaire pour les téléchargements en cours (via l'API officielle
// setPreferences, stable d'une version à l'autre contrairement au .ini).
func (c *Client) SetDownloadPaths(savePath, tempPath string) error {
	prefs := map[string]any{"save_path": savePath}
	if tempPath != "" {
		prefs["temp_path_enabled"] = true
		prefs["temp_path"] = tempPath
	} else {
		prefs["temp_path_enabled"] = false
	}
	j, _ := json.Marshal(prefs)
	_, err := c.do(http.MethodPost, "/api/v2/app/setPreferences", url.Values{"json": {string(j)}})
	return err
}
