// Package prowlarr parle à l'API de Prowlarr (v1). Comme Sonarr, la clé API se
// lit dans le config.xml local (install indolore quand on tourne sur la machine
// Prowlarr). Prowlarr n'écoute souvent que sur localhost.
package prowlarr

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
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
			Timeout:   12 * time.Second,
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
		p = append(p, filepath.Join(pd, "Prowlarr", "config.xml"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		p = append(p,
			filepath.Join(home, "AppData", "Roaming", "Prowlarr", "config.xml"),
			filepath.Join(home, ".config", "Prowlarr", "config.xml"),
		)
	}
	return p
}

func readLocalConfig() (string, string, error) {
	var lastErr error = fmt.Errorf("config.xml Prowlarr introuvable")
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
			lastErr = fmt.Errorf("ApiKey vide dans %s", p)
			continue
		}
		port := c.Port
		if port == 0 {
			port = 9696
		}
		base := fmt.Sprintf("http://localhost:%d", port)
		if ub := strings.Trim(c.URLBase, "/"); ub != "" {
			base += "/" + ub
		}
		return base, c.APIKey, nil
	}
	return "", "", lastErr
}

func (c *Client) req(method, path string, body []byte) ([]byte, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, c.BaseURL+path, r)
	req.Header.Set("X-Api-Key", c.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
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

// Indexer = vue simplifiée d'un indexeur Prowlarr pour l'affichage.
type Indexer struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Enable   bool   `json:"enable"`
	Protocol string `json:"protocol"`
	Privacy  string `json:"privacy"`
	Priority int    `json:"priority"`
}

func (c *Client) Indexers() ([]Indexer, error) {
	b, err := c.req(http.MethodGet, "/api/v1/indexer", nil)
	if err != nil {
		return nil, err
	}
	var ix []Indexer
	if err := json.Unmarshal(b, &ix); err != nil {
		return nil, err
	}
	return ix, nil
}

// SetEnabled active/désactive un indexeur. Prowlarr veut l'objet complet en PUT,
// donc on le relit, on bascule `enable`, et on le renvoie tel quel.
func (c *Client) SetEnabled(id int, enabled bool) error {
	b, err := c.req(http.MethodGet, fmt.Sprintf("/api/v1/indexer/%d", id), nil)
	if err != nil {
		return err
	}
	var full map[string]any
	if err := json.Unmarshal(b, &full); err != nil {
		return err
	}
	full["enable"] = enabled
	body, _ := json.Marshal(full)
	_, err = c.req(http.MethodPut, fmt.Sprintf("/api/v1/indexer/%d", id), body)
	return err
}

// App = application déclarée dans Prowlarr (Sonarr/Radarr).
type App struct {
	Name           string `json:"name"`
	Implementation string `json:"implementation"`
}

// Applications liste les apps déjà connectées à Prowlarr.
func (c *Client) Applications() ([]App, error) {
	b, err := c.req(http.MethodGet, "/api/v1/applications", nil)
	if err != nil {
		return nil, err
	}
	var apps []App
	if err := json.Unmarshal(b, &apps); err != nil {
		return nil, err
	}
	return apps, nil
}

// AddApplication déclare une app (Sonarr ou Radarr) dans Prowlarr → Settings →
// Apps. Une fois déclarée, Prowlarr synchronise automatiquement tous ses
// indexeurs vers cette app (c'est le mécanisme qui remplace Jackett). Idempotent
// par nom : ne recrée pas si déjà présent. Renvoie true si une app a été créée.
//
// On part du schéma fourni par Prowlarr (il contient implementation,
// configContract et les bonnes catégories de sync par défaut), on n'y remplace
// que le nom, le niveau de sync et les 3 champs de connexion.
func (c *Client) AddApplication(name, implementation, prowlarrURL, appBaseURL, appAPIKey string) (bool, error) {
	b, err := c.req(http.MethodGet, "/api/v1/applications", nil)
	if err != nil {
		return false, err
	}
	var existing []map[string]any
	_ = json.Unmarshal(b, &existing)
	for _, a := range existing {
		if s, _ := a["name"].(string); s == name {
			return false, nil
		}
	}

	b, err = c.req(http.MethodGet, "/api/v1/applications/schema", nil)
	if err != nil {
		return false, err
	}
	var schema []map[string]any
	_ = json.Unmarshal(b, &schema)
	var tpl map[string]any
	for _, s := range schema {
		if impl, _ := s["implementation"].(string); impl == implementation {
			tpl = s
			break
		}
	}
	if tpl == nil {
		return false, fmt.Errorf("schéma application %q introuvable dans Prowlarr", implementation)
	}

	tpl["name"] = name
	tpl["syncLevel"] = "fullSync"
	if fields, ok := tpl["fields"].([]any); ok {
		for _, f := range fields {
			fm, _ := f.(map[string]any)
			switch fm["name"] {
			case "prowlarrUrl":
				fm["value"] = prowlarrURL
			case "baseUrl":
				fm["value"] = appBaseURL
			case "apiKey":
				fm["value"] = appAPIKey
			}
		}
	}

	body, _ := json.Marshal(tpl)
	// PAS de forceSave ici : on veut que Prowlarr teste vraiment la connexion à
	// l'app. Sinon une app en erreur serait enregistrée mais ne se synchroniserait
	// jamais. En cas d'échec, l'erreur remonte à l'utilisateur.
	if _, err := c.req(http.MethodPost, "/api/v1/applications", body); err != nil {
		return false, err
	}
	return true, nil
}

// SyncApps force Prowlarr à pousser ses indexeurs vers les apps connectées
// (Sonarr/Radarr). Sans ça, la synchro n'a lieu qu'à intervalle régulier ou
// quand un indexeur change.
func (c *Client) SyncApps() error {
	_, err := c.req(http.MethodPost, "/api/v1/command", []byte(`{"name":"ApplicationIndexerSync"}`))
	return err
}

// IndexerSchema renvoie le catalogue d'indexeurs disponibles (brut).
func (c *Client) IndexerSchema() ([]byte, error) {
	return c.req(http.MethodGet, "/api/v1/indexer/schema", nil)
}

// AddIndexer ajoute un indexeur du catalogue (par son nom), activé. On repart de
// l'entrée de schéma (elle contient implementation/configContract/champs par
// défaut) — fonctionne tel quel pour les indexeurs publics ; les indexeurs
// privés qui exigent des identifiants renverront une erreur de validation.
func (c *Client) AddIndexer(name string) error {
	b, err := c.req(http.MethodGet, "/api/v1/indexer/schema", nil)
	if err != nil {
		return err
	}
	var schema []map[string]any
	if err := json.Unmarshal(b, &schema); err != nil {
		return err
	}
	var tpl map[string]any
	for _, s := range schema {
		if n, _ := s["name"].(string); n == name {
			tpl = s
			break
		}
	}
	if tpl == nil {
		return fmt.Errorf("indexeur %q introuvable dans le catalogue", name)
	}
	tpl["enable"] = true
	tpl["appProfileId"] = c.defaultAppProfileID() // requis par Prowlarr
	body, _ := json.Marshal(tpl)
	// forceSave=true : on enregistre sans exiger que le test de connexion passe
	// (utile pour les indexeurs publics parfois bloqués) ; configurable ensuite.
	_, err = c.req(http.MethodPost, "/api/v1/indexer?forceSave=true", body)
	return err
}

// defaultAppProfileID renvoie l'id du 1er « App Profile » Prowlarr (toujours
// présent ; 1 par défaut). Prowlarr l'exige pour créer un indexeur.
func (c *Client) defaultAppProfileID() int {
	b, err := c.req(http.MethodGet, "/api/v1/appprofile", nil)
	if err == nil {
		var profs []struct {
			ID int `json:"id"`
		}
		if json.Unmarshal(b, &profs) == nil && len(profs) > 0 {
			return profs[0].ID
		}
	}
	return 1
}
