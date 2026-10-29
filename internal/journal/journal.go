// Package journal keeps the slow-request journal, and owns the database it is
// kept in.
//
// A request whose spans are worth keeping — it was slow, or the server failed
// it — leaves one row saying where its time went, so "the API is slow
// sometimes" becomes "POST /api/games/{code}/moves spent 410 ms of 430 ms
// waiting for the room lock".
//
// The rows live in **a database of their own**, beside the game database:
// measuring the thing being measured would mean write amplification on game
// data and schema churn in the game schema, and this file can be deleted or
// truncated without touching a single game (see
// docs/history/observability-plan.md section 4). From Stage 5 the metric
// history lands in the same file, which is why this package owns it rather
// than only the journal's table.
package journal

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the same pure-Go driver internal/store uses
)

//go:embed schema.sql
var schemaSQL string

// DB is the metrics database.
type DB struct {
	db   *sql.DB
	path string
}

// Open opens (creating if necessary) the metrics database at path and brings
// it to the current schema. The settings are internal/store's, for the same
// reasons: readers never block the writer, and a statement that meets a lock
// waits rather than failing.
func Open(path string) (*DB, error) {
	const params = "_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_txlock=immediate"
	dsn := path
	if strings.Contains(dsn, "?") {
		dsn += "&" + params
	} else {
		dsn += "?" + params
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("journal: open %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("journal: ping %s: %w", path, err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("journal: migrate %s: %w", path, err)
	}
	return &DB{db: db, path: path}, nil
}

// Close releases the database.
func (d *DB) Close() error {
	if d == nil {
		return nil
	}
	return d.db.Close()
}

// Path is the file the journal is kept in.
func (d *DB) Path() string {
	if d == nil {
		return ""
	}
	return d.path
}

// SlowRequest is one row of the journal: a request that was worth writing
// down, and where its time went.
type SlowRequest struct {
	ID        int64
	At        time.Time
	RequestID string
	// Route is chi's pattern, never the path: a path carries the room code
	// (ADR 0063). The journal is read like a log and the same rules apply.
	Route     string
	Method    string
	Status    int
	Duration  time.Duration
	Spans     string // the phase breakdown, as JSON
	Err       string
	SpanCount int
	SpansLost int
}

// Write records one request. It is called from the request's own goroutine
// after the response has gone out, so a slow write here delays nothing a
// player is waiting for — but it is still one small insert.
func (d *DB) Write(ctx context.Context, r SlowRequest) error {
	if d == nil {
		return nil
	}
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO slow_requests (at, request_id, route, method, status, duration_ms, spans, err, span_count, spans_lost)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.At.UTC().Unix(), r.RequestID, r.Route, r.Method, r.Status,
		float64(r.Duration)/float64(time.Millisecond), r.Spans, r.Err, r.SpanCount, r.SpansLost,
	)
	if err != nil {
		return fmt.Errorf("journal: writing a slow request: %w", err)
	}
	return nil
}

// Recent returns the newest rows, for a test and for the Health page Stage 5
// builds.
func (d *DB) Recent(ctx context.Context, limit int) ([]SlowRequest, error) {
	if d == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, at, request_id, route, method, status, duration_ms, spans, err, span_count, spans_lost
		 FROM slow_requests ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("journal: reading slow requests: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SlowRequest
	for rows.Next() {
		var r SlowRequest
		var at int64
		var ms float64
		if err := rows.Scan(&r.ID, &at, &r.RequestID, &r.Route, &r.Method, &r.Status, &ms,
			&r.Spans, &r.Err, &r.SpanCount, &r.SpansLost); err != nil {
			return nil, fmt.Errorf("journal: scanning a slow request: %w", err)
		}
		r.At = time.Unix(at, 0).UTC()
		r.Duration = time.Duration(ms * float64(time.Millisecond))
		out = append(out, r)
	}
	return out, rows.Err()
}

// Count is how many rows the journal holds.
func (d *DB) Count(ctx context.Context) (int64, error) {
	if d == nil {
		return 0, nil
	}
	var n int64
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM slow_requests`).Scan(&n); err != nil {
		return 0, fmt.Errorf("journal: counting slow requests: %w", err)
	}
	return n, nil
}
