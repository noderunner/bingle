package journal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// What went wrong in a browser.
//
// The UI is where most of bingle's recent work went and where there was no
// visibility at all: a script that throws on one person's browser left no
// trace anywhere. This is that trace.
//
// It is deduplicated by hash(message + stack + version), so a bad release is
// one row with a count rather than a row per reader. Keeping the version in
// the key limits the damage done by the other half of that trade: two
// unrelated faults with the same message and stack do merge, and a minified
// or inlined frame can make a stack unhelpful.

// ClientError is one distinct fault, with how often it has been seen.
type ClientError struct {
	Hash      string    `json:"hash"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Count     int64     `json:"count"`
	Version   string    `json:"version"`
	// Page is a route pattern ("/game/{code}"), never a URL: a URL carries
	// the room code (ADR 0063).
	Page    string `json:"page"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
	Browser string `json:"browser"`
}

// ClientErrorHash is the key a report is deduplicated by. It is exported
// because the caller computes it before deciding whether to store, and a test
// checks two reports of the same fault agree on it.
func ClientErrorHash(message, stack, version string) string {
	sum := sha256.Sum256([]byte(message + "\x00" + stack + "\x00" + version))
	return hex.EncodeToString(sum[:])[:32]
}

// RecordClientError stores one report, or counts it against the row that is
// already there. It returns the new count, so the caller can log the first one
// loudly and the four hundredth not at all.
func (d *DB) RecordClientError(ctx context.Context, e ClientError) (int64, error) {
	if d == nil {
		return 0, nil
	}
	at := e.LastSeen
	if at.IsZero() {
		at = time.Now()
	}
	secs := at.UTC().Unix()

	// The message, stack, page and browser of the row that is already there
	// are left alone: the first report of a fault is the one that was not
	// truncated by a later reader's different browser.
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO client_errors (hash, first_seen, last_seen, count, version, page, message, stack, browser)
		 VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?)
		 ON CONFLICT (hash) DO UPDATE SET
		   last_seen = excluded.last_seen,
		   count     = count + 1`,
		e.Hash, secs, secs, e.Version, e.Page, e.Message, e.Stack, e.Browser)
	if err != nil {
		return 0, fmt.Errorf("journal: recording a client error: %w", err)
	}

	var count int64
	if err := d.db.QueryRowContext(ctx,
		`SELECT count FROM client_errors WHERE hash = ?`, e.Hash).Scan(&count); err != nil {
		return 0, fmt.Errorf("journal: reading a client error's count: %w", err)
	}
	return count, nil
}

// ClientErrors returns the faults, newest first. The page shows them newest
// and noisiest first; "newest" is the order that matters after a release.
func (d *DB) ClientErrors(ctx context.Context, limit int) ([]ClientError, error) {
	if d == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT hash, first_seen, last_seen, count, version, page, message, stack, browser
		 FROM client_errors ORDER BY last_seen DESC, count DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("journal: reading client errors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ClientError
	for rows.Next() {
		var e ClientError
		var first, last int64
		if err := rows.Scan(&e.Hash, &first, &last, &e.Count, &e.Version, &e.Page,
			&e.Message, &e.Stack, &e.Browser); err != nil {
			return nil, fmt.Errorf("journal: scanning a client error: %w", err)
		}
		e.FirstSeen = time.Unix(first, 0).UTC()
		e.LastSeen = time.Unix(last, 0).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// PurgeClientErrors drops faults nothing has seen since before. A fault that
// has not happened for a fortnight has been fixed or has stopped mattering.
func (d *DB) PurgeClientErrors(ctx context.Context, before time.Time) (int64, error) {
	if d == nil {
		return 0, nil
	}
	res, err := d.db.ExecContext(ctx,
		`DELETE FROM client_errors WHERE last_seen < ?`, before.UTC().Unix())
	if err != nil {
		return 0, fmt.Errorf("journal: purging client errors: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
