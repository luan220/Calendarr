package sonarr

import (
	"encoding/json"
	"fmt"
)

// EpisodeFilePath renvoie le chemin disque du fichier d'un épisode (pour la lecture).
func (c *Client) EpisodeFilePath(episodeID int) (string, error) {
	b, err := c.apiGet(fmt.Sprintf("/api/v3/episode/%d", episodeID))
	if err != nil {
		return "", err
	}
	var ep struct {
		EpisodeFile struct {
			Path string `json:"path"`
		} `json:"episodeFile"`
	}
	if err := json.Unmarshal(b, &ep); err != nil {
		return "", err
	}
	if ep.EpisodeFile.Path == "" {
		return "", fmt.Errorf("aucun fichier pour l'épisode %d", episodeID)
	}
	return ep.EpisodeFile.Path, nil
}
