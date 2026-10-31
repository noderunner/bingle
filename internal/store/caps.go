package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// CapRow is one class's permission for one capability.
type CapRow struct {
	Class, Capability string
	Allowed           bool
}

// LimitRow is one class's rate limit of one kind, per minute.
type LimitRow struct {
	Class, Kind string
	PerMinute   int
}

// CapsSeeded reports whether the capabilities have been seeded (they are,
// from the first start on, and the file is not read again).
func (s *Store) CapsSeeded(ctx context.Context) (bool, error) {
	var n int
	err := s.queryRow(ctx, "CapsSeeded", `SELECT COUNT(*) FROM capabilities`).Scan(&n)
	return n > 0, err
}

// ReplaceCaps sets every capability and limit at once: the seed of a fresh
// database, and `bingle caps reset`.
func (s *Store) ReplaceCaps(ctx context.Context, caps []CapRow, limits []LimitRow) error {
	tx, err := s.begin(ctx, "ReplaceCaps")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.exec(ctx, "ReplaceCaps", `DELETE FROM capabilities`); err != nil {
		return err
	}
	if _, err := tx.exec(ctx, "ReplaceCaps", `DELETE FROM cap_limits`); err != nil {
		return err
	}
	for _, c := range caps {
		if _, err := tx.exec(ctx, "ReplaceCaps", `INSERT INTO capabilities (class, capability, allowed) VALUES (?, ?, ?)`, c.Class, c.Capability, c.Allowed); err != nil {
			return err
		}
	}
	for _, l := range limits {
		if _, err := tx.exec(ctx, "ReplaceCaps", `INSERT INTO cap_limits (class, kind, per_minute) VALUES (?, ?, ?)`, l.Class, l.Kind, l.PerMinute); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Caps reads every capability and limit.
func (s *Store) Caps(ctx context.Context) ([]CapRow, []LimitRow, error) {
	rows, err := s.query(ctx, "Caps", `SELECT class, capability, allowed FROM capabilities ORDER BY class, capability`)
	if err != nil {
		return nil, nil, err
	}
	var caps []CapRow
	for rows.Next() {
		var c CapRow
		if err := rows.Scan(&c.Class, &c.Capability, &c.Allowed); err != nil {
			_ = rows.Close()
			return nil, nil, err
		}
		caps = append(caps, c)
	}
	_ = rows.Close()
	rows, err = s.query(ctx, "Caps", `SELECT class, kind, per_minute FROM cap_limits ORDER BY class, kind`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	var limits []LimitRow
	for rows.Next() {
		var l LimitRow
		if err := rows.Scan(&l.Class, &l.Kind, &l.PerMinute); err != nil {
			return nil, nil, err
		}
		limits = append(limits, l)
	}
	return caps, limits, rows.Err()
}

// SetCap changes one class's permission for one capability.
func (s *Store) SetCap(ctx context.Context, class, capability string, allowed bool) error {
	_, err := s.exec(ctx, "SetCap", `INSERT INTO capabilities (class, capability, allowed) VALUES (?, ?, ?)
		ON CONFLICT (class, capability) DO UPDATE SET allowed = excluded.allowed`, class, capability, allowed)
	return err
}

// ---- API tokens --------------------------------------------------------------

// APIToken is an api_tokens row (never its secret).
type APIToken struct {
	ID         int64
	Name       string
	Scopes     []string
	CreatedAt  string
	LastUsedAt string
}

// CreateAPIToken records a token by hash.
func (s *Store) CreateAPIToken(ctx context.Context, accountID int64, name, hash string, scopes []string) (int64, error) {
	res, err := s.exec(ctx, "CreateAPIToken", `INSERT INTO api_tokens (account_id, name, hash, scopes) VALUES (?, ?, ?, ?)`, accountID, name, hash, strings.Join(scopes, ","))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// APITokenAccount is the active account a live token belongs to, and the
// token's scopes. Its last use is written at most every touchEvery.
func (s *Store) APITokenAccount(ctx context.Context, hash string, now time.Time, touchEvery time.Duration) (Account, []string, error) {
	var scopes, last string
	var a Account
	err := s.queryRow(ctx, "APITokenAccount",
		`SELECT t.scopes, COALESCE(t.last_used_at, ''), `+accountCols("a")+` FROM api_tokens t JOIN accounts a ON a.id = t.account_id
		 WHERE t.hash = ? AND t.revoked_at IS NULL AND a.status = ? AND a.deleted_at IS NULL`, hash, AccountActive).
		Scan(append([]any{&scopes, &last}, a.fields()...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, nil, ErrNotFound
	}
	if err != nil {
		return Account{}, nil, err
	}
	if last < stamp(now.Add(-touchEvery)) {
		if _, err := s.exec(ctx, "APITokenAccount.touch", `UPDATE api_tokens SET last_used_at = ? WHERE hash = ?`, stamp(now), hash); err != nil {
			return Account{}, nil, err
		}
	}
	return a, strings.Split(scopes, ","), nil
}

// APITokens lists an account's live tokens.
func (s *Store) APITokens(ctx context.Context, accountID int64) ([]APIToken, error) {
	rows, err := s.query(ctx, "APITokens", `SELECT id, name, scopes, created_at, COALESCE(last_used_at, '') FROM api_tokens WHERE account_id = ? AND revoked_at IS NULL ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []APIToken
	for rows.Next() {
		var t APIToken
		var scopes string
		if err := rows.Scan(&t.ID, &t.Name, &scopes, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		t.Scopes = strings.Split(scopes, ",")
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeAPIToken ends one of an account's tokens.
func (s *Store) RevokeAPIToken(ctx context.Context, accountID, id int64) error {
	res, err := s.exec(ctx, "RevokeAPIToken", `UPDATE api_tokens SET revoked_at = CURRENT_TIMESTAMP WHERE id = ? AND account_id = ? AND revoked_at IS NULL`, id, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// APITokenExists reports whether a token (by hash) exists, revoked or not.
func (s *Store) APITokenExists(ctx context.Context, hash string) (bool, error) {
	var n int
	err := s.queryRow(ctx, "APITokenExists", `SELECT COUNT(*) FROM api_tokens WHERE hash = ?`, hash).Scan(&n)
	return n > 0, err
}

// ---- invitations ----------------------------------------------------------------

// CreateInvite records an invitation by the hash of its code.
func (s *Store) CreateInvite(ctx context.Context, hash string, by int64, expires time.Time) error {
	_, err := s.exec(ctx, "CreateInvite", `INSERT INTO invites (code_hash, created_by, expires_at) VALUES (?, ?, ?)`, hash, by, stamp(expires))
	return err
}

// InviteLive reports whether an invitation can still be used.
func (s *Store) InviteLive(ctx context.Context, hash string, now time.Time) (bool, error) {
	var n int
	err := s.queryRow(ctx, "InviteLive", `SELECT COUNT(*) FROM invites WHERE code_hash = ? AND used_at IS NULL AND expires_at > ?`, hash, stamp(now)).Scan(&n)
	return n > 0, err
}

// UseInvite spends an invitation on an account; false when it was spent or
// out of date already.
func (s *Store) UseInvite(ctx context.Context, hash string, accountID int64, now time.Time) (bool, error) {
	res, err := s.exec(ctx, "UseInvite", `UPDATE invites SET used_by = ?, used_at = ? WHERE code_hash = ? AND used_at IS NULL AND expires_at > ?`, accountID, stamp(now), hash, stamp(now))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
