package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Identity is who a principal is, as a seat or a chat line shows it: an
// account's handle, or a guest.
type Identity struct {
	Handle string // "" for a guest
	Guest  bool
}

// Identities says who each of the principals is. Unknown IDs are left out.
func (s *Store) Identities(ctx context.Context, ids []string) (map[string]Identity, error) {
	out := map[string]Identity{}
	var want []any
	seen := map[string]bool{}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			want = append(want, id)
		}
	}
	if len(want) == 0 {
		return out, nil
	}
	rows, err := s.query(ctx, "Identities", `SELECT p.id, p.kind, COALESCE(a.handle, '') FROM principals p
		LEFT JOIN accounts a ON a.id = p.account_id AND a.status = '`+AccountActive+`' AND a.deleted_at IS NULL
		WHERE p.id IN (?`+strings.Repeat(", ?", len(want)-1)+`)`, want...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, kind, handle string
		if err := rows.Scan(&id, &kind, &handle); err != nil {
			return nil, err
		}
		if kind == KindAccount && handle != "" {
			out[id] = Identity{Handle: handle}
		} else {
			out[id] = Identity{Guest: true}
		}
	}
	return out, rows.Err()
}

// OwnGame is a game a principal has a seat in.
type OwnGame struct {
	RoomCode, GameDefID, Status, UpdatedAt string
	// Names are the principal's seats' names, in seat order.
	Names []string
}

// OwnGames lists the games a principal has a seat in, the latest first.
func (s *Store) OwnGames(ctx context.Context, principalID string, limit int) ([]OwnGame, error) {
	rows, err := s.query(ctx, "OwnGames", `SELECT g.room_code, g.game_def_id, g.status, g.updated_at, p.name FROM players p JOIN games g ON g.id = p.game_id
		WHERE p.principal_id = ? ORDER BY g.updated_at DESC, g.id DESC, p.seat`, principalID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []OwnGame
	index := map[string]int{}
	for rows.Next() {
		var g OwnGame
		var name string
		if err := rows.Scan(&g.RoomCode, &g.GameDefID, &g.Status, &g.UpdatedAt, &name); err != nil {
			return nil, err
		}
		if i, ok := index[g.RoomCode]; ok {
			out[i].Names = append(out[i].Names, name)
			continue
		}
		if len(out) >= limit {
			continue
		}
		g.Names = []string{name}
		index[g.RoomCode] = len(out)
		out = append(out, g)
	}
	return out, rows.Err()
}

// ---- handles ----------------------------------------------------------------------

// HandleHeldBy is the account a handle is held for (0 for none): its owner,
// or the owner who renamed away from it in the last 30 days.
func (s *Store) HandleHeldBy(ctx context.Context, handle string, now time.Time) (int64, error) {
	h := strings.ToLower(handle)
	var id int64
	err := s.queryRow(ctx, "HandleHeldBy", `SELECT id FROM accounts WHERE handle_lower = ? AND deleted_at IS NULL
		UNION ALL SELECT account_id FROM handle_holds WHERE handle_lower = ? AND until > ? LIMIT 1`, h, h, stamp(now)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// RenameAccount gives an account a new handle and holds the old one for it
// until hold.
func (s *Store) RenameAccount(ctx context.Context, accountID int64, handle string, hold time.Time) error {
	tx, err := s.begin(ctx, "RenameAccount")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var old string
	if err := tx.queryRow(ctx, "RenameAccount", `SELECT handle_lower FROM accounts WHERE id = ?`, accountID).Scan(&old); err != nil {
		return err
	}
	// Taking one's own held handle back ends its hold.
	if _, err := tx.exec(ctx, "RenameAccount", `DELETE FROM handle_holds WHERE handle_lower = ? AND account_id = ?`, strings.ToLower(handle), accountID); err != nil {
		return err
	}
	if _, err := tx.exec(ctx, "RenameAccount", `UPDATE accounts SET handle = ?, handle_lower = ?, display_name = ? WHERE id = ?`, handle, strings.ToLower(handle), handle, accountID); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrTaken
		}
		return err
	}
	if _, err := tx.exec(ctx, "RenameAccount", `INSERT INTO handle_holds (handle_lower, account_id, until) VALUES (?, ?, ?)
		ON CONFLICT (handle_lower) DO UPDATE SET account_id = excluded.account_id, until = excluded.until`, old, accountID, stamp(hold)); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- sessions ------------------------------------------------------------------------

// SessionRow is one of an account's sessions as its account page lists it.
type SessionRow struct {
	IDHash, CreatedAt, LastSeenAt, Agent string
}

// Sessions lists an account's live sessions, the latest first.
func (s *Store) Sessions(ctx context.Context, accountID int64, now time.Time) ([]SessionRow, error) {
	rows, err := s.query(ctx, "Sessions", `SELECT id_hash, created_at, last_seen_at, agent FROM sessions WHERE account_id = ? AND expires_at > ? ORDER BY last_seen_at DESC`, accountID, stamp(now))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []SessionRow
	for rows.Next() {
		var r SessionRow
		if err := rows.Scan(&r.IDHash, &r.CreatedAt, &r.LastSeenAt, &r.Agent); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- guest keys -----------------------------------------------------------------------

// ReplaceGuestKey gives a guest a new key and retires every other: a key
// copied where it should not be stops working, and the guest stays.
func (s *Store) ReplaceGuestKey(ctx context.Context, principalID, keyHash string) error {
	tx, err := s.begin(ctx, "ReplaceGuestKey")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.exec(ctx, "ReplaceGuestKey", `UPDATE guest_keys SET revoked_at = CURRENT_TIMESTAMP WHERE principal_id = ? AND revoked_at IS NULL`, principalID); err != nil {
		return err
	}
	if _, err := tx.exec(ctx, "ReplaceGuestKey", `INSERT INTO guest_keys (principal_id, key_hash) VALUES (?, ?)`, principalID, keyHash); err != nil {
		return err
	}
	return tx.Commit()
}
