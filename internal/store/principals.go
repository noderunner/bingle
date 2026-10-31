package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Principal kinds (ADR 0067).
const (
	KindGuest   = "guest"
	KindAccount = "account"
)

// CreateGuest records a new guest principal and its first key, by hash.
func (s *Store) CreateGuest(ctx context.Context, id, keyHash string) error {
	tx, err := s.begin(ctx, "CreateGuest")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.exec(ctx, "CreateGuest.principal", `INSERT INTO principals (id, kind) VALUES (?, ?)`, id, KindGuest); err != nil {
		return err
	}
	if _, err := tx.exec(ctx, "CreateGuest.key", `INSERT INTO guest_keys (principal_id, key_hash) VALUES (?, ?)`, id, keyHash); err != nil {
		return err
	}
	return tx.Commit()
}

// GuestByKey returns the principal whose live key hashes to keyHash: not
// revoked, and used since idleSince. ErrNotFound when there is none. A key
// in use has its last use, and its principal's last sighting, brought up to
// date at most once every touchEvery, so a busy guest is not a write per
// request.
func (s *Store) GuestByKey(ctx context.Context, keyHash string, idleSince time.Time, touchEvery time.Duration, now time.Time) (string, error) {
	var id string
	var lastUsed string
	err := s.queryRow(ctx, "GuestByKey",
		`SELECT k.principal_id, k.last_used_at FROM guest_keys k JOIN principals p ON p.id = k.principal_id
		 WHERE k.key_hash = ? AND k.revoked_at IS NULL AND k.last_used_at >= ? AND p.merged_into = ''`,
		keyHash, stamp(idleSince)).Scan(&id, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if lastUsed < stamp(now.Add(-touchEvery)) {
		if _, err := s.exec(ctx, "GuestByKey.touchKey", `UPDATE guest_keys SET last_used_at = ? WHERE key_hash = ?`, stamp(now), keyHash); err != nil {
			return "", err
		}
		if _, err := s.exec(ctx, "GuestByKey.touchPrincipal", `UPDATE principals SET last_seen_at = ? WHERE id = ?`, stamp(now), id); err != nil {
			return "", err
		}
	}
	return id, nil
}

// BindPlayer makes the seat row id the principal's, unless it already
// belongs to somebody. It reports whether the seat is now principalID's.
func (s *Store) BindPlayer(ctx context.Context, id int64, principalID string) (bool, error) {
	if _, err := s.exec(ctx, "BindPlayer", `UPDATE players SET principal_id = ? WHERE id = ? AND principal_id = ''`, principalID, id); err != nil {
		return false, err
	}
	var owner string
	if err := s.queryRow(ctx, "BindPlayer.read", `SELECT principal_id FROM players WHERE id = ?`, id).Scan(&owner); err != nil {
		return false, err
	}
	return owner == principalID, nil
}

// SetGameOwner records the principal that created a game.
func (s *Store) SetGameOwner(ctx context.Context, gameID int64, principalID string) error {
	_, err := s.exec(ctx, "SetGameOwner", `UPDATE games SET owner_principal_id = ? WHERE id = ? AND owner_principal_id = ''`, principalID, gameID)
	return err
}
