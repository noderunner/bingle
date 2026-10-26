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
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO game_versions (family, version) VALUES (?, ?)`, family, version); err != nil {
		return fmt.Errorf("store: record game version: %w", err)
	}
	return nil
}

// GameVersions lists every version recorded, oldest first within a family.
func (s *Store) GameVersions(ctx context.Context) ([]GameVersion, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT family, version, first_seen FROM game_versions ORDER BY family, first_seen, version`)
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
