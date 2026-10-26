package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const sqlTime = "2006-01-02 15:04:05"

func stamp(t time.Time) string { return t.UTC().Format(sqlTime) }

// Contact is an address a browser asked to be notified at.
type Contact struct {
	ID             int64
	Address        string
	AddrHash       string
	SecretHash     string
	StopToken      string
	ConfirmHash    string
	ConfirmExpires string
	VerifiedAt     string // "" until confirmed
	CreatedAt      string
}

// Verified reports whether the address was confirmed.
func (c Contact) Verified() bool { return c.VerifiedAt != "" }

const contactCols = `id, address, addr_hash, secret_hash, stop_token, confirm_hash, COALESCE(confirm_expires, ''), COALESCE(verified_at, ''), created_at`

func scanContact(row interface{ Scan(...any) error }) (Contact, error) {
	var c Contact
	err := row.Scan(&c.ID, &c.Address, &c.AddrHash, &c.SecretHash, &c.StopToken, &c.ConfirmHash, &c.ConfirmExpires, &c.VerifiedAt, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Contact{}, ErrNotFound
	}
	return c, err
}

// CreateContact stores a new, unverified contact and returns its id.
func (s *Store) CreateContact(ctx context.Context, c Contact, expires time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO notify_contacts (address, addr_hash, secret_hash, stop_token, confirm_hash, confirm_expires) VALUES (?, ?, ?, ?, ?, ?)`,
		c.Address, c.AddrHash, c.SecretHash, c.StopToken, c.ConfirmHash, stamp(expires))
	if err != nil {
		return 0, fmt.Errorf("storing contact: %w", err)
	}
	return res.LastInsertId()
}

// GetContact returns the contact with the id.
func (s *Store) GetContact(ctx context.Context, id int64) (Contact, error) {
	return scanContact(s.db.QueryRowContext(ctx, `SELECT `+contactCols+` FROM notify_contacts WHERE id = ?`, id))
}

// ResetContact gives a contact a new address: unverified again, with a new
// confirmation token.
func (s *Store) ResetContact(ctx context.Context, id int64, address, addrHash, confirmHash string, expires time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE notify_contacts SET address = ?, addr_hash = ?, confirm_hash = ?, confirm_expires = ?, verified_at = NULL WHERE id = ?`,
		address, addrHash, confirmHash, stamp(expires), id)
	if err != nil {
		return fmt.Errorf("changing contact: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetConfirmToken stores a new confirmation token for an unverified contact.
func (s *Store) SetConfirmToken(ctx context.Context, id int64, confirmHash string, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notify_contacts SET confirm_hash = ?, confirm_expires = ? WHERE id = ?`, confirmHash, stamp(expires), id)
	return err
}

// ConfirmContact verifies the contact whose confirmation token has this hash,
// if the token has not expired. The token works once.
func (s *Store) ConfirmContact(ctx context.Context, confirmHash string, now time.Time) (Contact, error) {
	if confirmHash == "" {
		return Contact{}, ErrNotFound
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Contact{}, err
	}
	defer func() { _ = tx.Rollback() }()
	c, err := scanContact(tx.QueryRowContext(ctx, `SELECT `+contactCols+` FROM notify_contacts WHERE confirm_hash = ? AND confirm_expires > ?`, confirmHash, stamp(now)))
	if err != nil {
		return Contact{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE notify_contacts SET verified_at = ?, confirm_hash = '' WHERE id = ?`, stamp(now), c.ID); err != nil {
		return Contact{}, err
	}
	if err := tx.Commit(); err != nil {
		return Contact{}, err
	}
	c.VerifiedAt, c.ConfirmHash = stamp(now), ""
	return c, nil
}

// ContactByStopToken finds the contact an unsubscribe link belongs to.
func (s *Store) ContactByStopToken(ctx context.Context, token string) (Contact, error) {
	if token == "" {
		return Contact{}, ErrNotFound
	}
	return scanContact(s.db.QueryRowContext(ctx, `SELECT `+contactCols+` FROM notify_contacts WHERE stop_token = ?`, token))
}

// DeleteContact removes a contact and, with it, every seat's setting that used it.
func (s *Store) DeleteContact(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM notify_contacts WHERE id = ?`, id)
	return err
}

// PurgeContacts deletes contacts never verified that are older than before,
// and the sent-mail records older than before.
func (s *Store) PurgeContacts(ctx context.Context, before time.Time) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM notify_contacts WHERE verified_at IS NULL AND created_at < ?`, stamp(before)); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM notify_sent WHERE at < ?`, stamp(before))
	return err
}

// Suppress records that an address (by hash) is never to be mailed again.
func (s *Store) Suppress(ctx context.Context, addrHash string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO notify_suppress (addr_hash) VALUES (?)`, addrHash)
	return err
}

// IsSuppressed reports whether an address (by hash) asked not to be mailed.
func (s *Store) IsSuppressed(ctx context.Context, addrHash string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notify_suppress WHERE addr_hash = ?`, addrHash).Scan(&n)
	return n > 0, err
}

// IssueSeatToken stores the hash of a seat's secret unless the seat already
// has one. It reports whether it was stored.
func (s *Store) IssueSeatToken(ctx context.Context, gameID int64, nameKey, hash string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO seat_tokens (game_id, name_key, token_hash) VALUES (?, ?, ?)`, gameID, nameKey, hash)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SeatTokenHash returns the hash of a seat's secret ("" with ErrNotFound when none).
func (s *Store) SeatTokenHash(ctx context.Context, gameID int64, nameKey string) (string, error) {
	var h string
	err := s.db.QueryRowContext(ctx, `SELECT token_hash FROM seat_tokens WHERE game_id = ? AND name_key = ?`, gameID, nameKey).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return h, err
}

// NotifyPref is a seat's notification setting. ContactID is 0 when no
// address has been chosen for the seat.
type NotifyPref struct {
	GameID      int64
	NameKey     string
	ContactID   int64
	Enabled     bool
	AwayMinutes int
}

// SetNotifyPref stores (or replaces) a seat's setting.
func (s *Store) SetNotifyPref(ctx context.Context, p NotifyPref) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notify_prefs (game_id, name_key, contact_id, enabled, away_minutes) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(game_id, name_key) DO UPDATE SET contact_id = excluded.contact_id, enabled = excluded.enabled, away_minutes = excluded.away_minutes`,
		p.GameID, p.NameKey, nullID(p.ContactID), boolToInt(p.Enabled), p.AwayMinutes)
	if err != nil {
		return fmt.Errorf("storing notification setting: %w", err)
	}
	return nil
}

// GetNotifyPref returns a seat's setting, or ErrNotFound.
func (s *Store) GetNotifyPref(ctx context.Context, gameID int64, nameKey string) (NotifyPref, error) {
	var p NotifyPref
	var en int
	var cid sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT game_id, name_key, contact_id, enabled, away_minutes FROM notify_prefs WHERE game_id = ? AND name_key = ?`, gameID, nameKey).
		Scan(&p.GameID, &p.NameKey, &cid, &en, &p.AwayMinutes)
	if errors.Is(err, sql.ErrNoRows) {
		return NotifyPref{}, ErrNotFound
	}
	p.Enabled, p.ContactID = en != 0, cid.Int64
	return p, err
}

// SentFilter selects records of mail sent.
type SentFilter struct {
	Kinds    []string
	AddrHash string
	IP       string
	GameID   int64
	Since    time.Time
}

// RecordSent notes that a mail of the kind was sent (or attempted).
func (s *Store) RecordSent(ctx context.Context, kind, addrHash string, gameID int64, ip string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO notify_sent (at, kind, addr_hash, game_id, ip) VALUES (?, ?, ?, ?, ?)`, stamp(at), kind, addrHash, gameID, ip)
	return err
}

// CountSent counts the records that match the filter.
func (s *Store) CountSent(ctx context.Context, f SentFilter) (int, error) {
	q := `SELECT COUNT(*) FROM notify_sent WHERE at >= ?`
	args := []any{stamp(f.Since)}
	if len(f.Kinds) > 0 {
		q += ` AND kind IN (` + strings.TrimSuffix(strings.Repeat("?,", len(f.Kinds)), ",") + `)`
		for _, k := range f.Kinds {
			args = append(args, k)
		}
	}
	if f.AddrHash != "" {
		q += ` AND addr_hash = ?`
		args = append(args, f.AddrHash)
	}
	if f.IP != "" {
		q += ` AND ip = ?`
		args = append(args, f.IP)
	}
	if f.GameID != 0 {
		q += ` AND game_id = ?`
		args = append(args, f.GameID)
	}
	var n int
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

// nullID stores 0 as NULL.
func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
