package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrPackInstalled is returned by SavePack for a version of a pack that is
// already installed: a version is a game of its own (ADR 0098), so its rules
// never change under the games played with it.
var ErrPackInstalled = errors.New("store: this version of the pack is already installed")

// StoredPack is one installed version of a data pack (ADR 0128).
type StoredPack struct {
	Name, Version string
	Title         string
	// Games are the ids this version registers.
	Games  []string
	SHA256 string
	Zip    []byte
	// InstalledBy is the admin's account, 0 when none is known.
	InstalledBy int64
	InstalledAt string
}

// SavePack stores p. A version already installed is ErrPackInstalled.
func (s *Store) SavePack(ctx context.Context, p StoredPack) error {
	games, err := json.Marshal(p.Games)
	if err != nil {
		return fmt.Errorf("store: save pack: %w", err)
	}
	var by any
	if p.InstalledBy != 0 {
		by = p.InstalledBy
	}
	res, err := s.exec(ctx, "SavePack", `
		INSERT INTO packs (name, version, title, games, sha256, zip, installed_by) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (name, version) DO NOTHING`,
		p.Name, p.Version, p.Title, string(games), p.SHA256, p.Zip, by)
	if err != nil {
		return fmt.Errorf("store: save pack: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: %s %s", ErrPackInstalled, p.Name, p.Version)
	}
	return nil
}

// Packs lists every installed version, each with its zip, by name and then in
// the order they were installed.
func (s *Store) Packs(ctx context.Context) ([]StoredPack, error) {
	rows, err := s.query(ctx, "Packs", `
		SELECT name, version, title, games, sha256, zip, COALESCE(installed_by, 0), installed_at
		FROM packs ORDER BY name, installed_at, version`)
	if err != nil {
		return nil, fmt.Errorf("store: list packs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StoredPack
	for rows.Next() {
		var p StoredPack
		var games string
		if err := rows.Scan(&p.Name, &p.Version, &p.Title, &games, &p.SHA256, &p.Zip, &p.InstalledBy, &p.InstalledAt); err != nil {
			return nil, fmt.Errorf("store: list packs: %w", err)
		}
		if err := json.Unmarshal([]byte(games), &p.Games); err != nil {
			return nil, fmt.Errorf("store: pack %s %s lists its games as %q: %w", p.Name, p.Version, games, err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePack removes one installed version; ErrNotFound when there is none.
// The games played with it are the caller's to end first.
func (s *Store) DeletePack(ctx context.Context, name, version string) error {
	res, err := s.exec(ctx, "DeletePack", `DELETE FROM packs WHERE name = ? AND version = ?`, name, version)
	if err != nil {
		return fmt.Errorf("store: delete pack: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
