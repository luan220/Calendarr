// Package store = persistance locale (SQLite pur Go, sans CGO). Pour l'instant :
// l'état "vu" des épisodes. Plus tard : réglages, cache.
package store

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS watched (
		episode_id INTEGER PRIMARY KEY,
		created_at TEXT DEFAULT (datetime('now'))
	)`); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// WatchedSet renvoie l'ensemble des episode_id marqués vus.
func (s *Store) WatchedSet() (map[int]bool, error) {
	rows, err := s.db.Query(`SELECT episode_id FROM watched`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[int]bool)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		m[id] = true
	}
	return m, rows.Err()
}

func (s *Store) SetWatched(episodeID int, watched bool) error {
	if watched {
		_, err := s.db.Exec(`INSERT OR IGNORE INTO watched(episode_id) VALUES(?)`, episodeID)
		return err
	}
	_, err := s.db.Exec(`DELETE FROM watched WHERE episode_id = ?`, episodeID)
	return err
}
