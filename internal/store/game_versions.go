package store

import (
	"context"
	"fmt"
)

// GameVersion is one version a game implementation has had.
type GameVersion struct {
	Family    string
	Version   string
	FirstSeen string
}

// RecordGameVersion notes that the server runs version of the game family. A
// version already noted keeps the time it was first seen.
func (s *Store) RecordGameVersion(ctx context.Context, family, version string) error {
	if _, err := s.exec(ctx, "RecordGameVersion", `INSERT OR IGNORE INTO game_versions (family, version) VALUES (?, ?)`, family, version); err != nil {
		return fmt.Errorf("store: record game version: %w", err)
	}
	return nil
}

// GameVersions lists every version recorded, oldest first within a family.
func (s *Store) GameVersions(ctx context.Context) ([]GameVersion, error) {
	rows, err := s.query(ctx, "GameVersions", `SELECT family, version, first_seen FROM game_versions ORDER BY family, first_seen, version`)
	if err != nil {
		return nil, fmt.Errorf("store: list game versions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []GameVersion
	for rows.Next() {
		var v GameVersion
		if err := rows.Scan(&v.Family, &v.Version, &v.FirstSeen); err != nil {
			return nil, fmt.Errorf("store: list game versions: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// RecordPackGame notes the name of a game an install pack brought (ADR 0147),
// so that its games stay readable by it once the pack is gone.
func (s *Store) RecordPackGame(ctx context.Context, id, name string) error {
	if _, err := s.exec(ctx, "RecordPackGame", `INSERT INTO pack_games (id, name) VALUES (?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name`, id, name); err != nil {
		return fmt.Errorf("store: record pack game: %w", err)
	}
	return nil
}

// PackGames lists every game install packs have brought, by id, with its name.
func (s *Store) PackGames(ctx context.Context) (map[string]string, error) {
	rows, err := s.query(ctx, "PackGames", `SELECT id, name FROM pack_games`)
	if err != nil {
		return nil, fmt.Errorf("store: list pack games: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("store: list pack games: %w", err)
		}
		out[id] = name
	}
	return out, rows.Err()
}
