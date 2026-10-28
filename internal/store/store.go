// Package store provides bingle's persistence layer over SQLite.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"
)

// schemaSQL holds the CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT
// EXISTS statements that define bingle's SQLite schema (see docs/history/plan.md
// section 6.6). It is executed against every freshly opened database so
// that Open is idempotent against an already-migrated file.
//
//go:embed schema.sql
var schemaSQL string

// Store wraps the SQLite connection used by the application.
type Store struct {
	db *sql.DB
}

// Open opens (creating if necessary) the SQLite database at dbPath and
// migrates it to the current schema (see schema.sql). Calling Open more
// than once against the same file is safe and non-destructive: every
// statement in schema.sql is a CREATE ... IF NOT EXISTS.
func Open(dbPath string) (*Store, error) {
	// Connection settings are applied via DSN-level parameters rather than
	// one-time db.Exec calls, because database/sql pools multiple physical
	// connections and later queries may land on a connection that never saw
	// a plain "PRAGMA ..." exec. modernc.org/sqlite applies _pragma DSN
	// parameters to every new physical connection it opens.
	//
	//   - foreign_keys(1): enforce FKs (and thus ON DELETE CASCADE).
	//   - journal_mode(WAL): readers never block the writer and vice versa,
	//     which matters when several browser tabs refresh while a move is
	//     being recorded.
	//   - busy_timeout(10000): a connection that does hit a lock waits for it
	//     instead of failing the statement with SQLITE_BUSY.
	//   - _txlock=immediate: write transactions take the write lock when they
	//     begin, so two concurrent read-then-write transactions queue up
	//     (and honour the busy timeout) instead of deadlocking on a lock
	//     upgrade.
	const params = "_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_txlock=immediate"
	dsn := dbPath
	if strings.Contains(dsn, "?") {
		dsn += "&" + params
	} else {
		dsn += "?" + params
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite db: %w", err)
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite schema: %w", err)
	}

	if err := addMissingColumns(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite schema: %w", err)
	}

	return &Store{db: db}, nil
}

// columnMigrations lists columns added after the original schema. Each is
// applied only when absent, so Open stays idempotent and upgrades databases
// created by earlier versions in place. Every column is NOT NULL with a
// default, so rows written by older code remain valid.
var columnMigrations = []struct{ table, column, ddl string }{
	{"games", "end_reason", "TEXT NOT NULL DEFAULT ''"},
	{"games", "move_time_limit", "INTEGER NOT NULL DEFAULT 0"},
	{"games", "draw_offer_seat", "INTEGER"},
	{"games", "end_detail", "TEXT NOT NULL DEFAULT ''"},
	{"games", "initial_state", "TEXT NOT NULL DEFAULT ''"},
	{"games", "initial_turn_seat", "INTEGER NOT NULL DEFAULT 0"},
	{"games", "takeback_seat", "INTEGER"},
	{"games", "takeback_seq", "INTEGER NOT NULL DEFAULT 0"},
	{"moves", "notation", "TEXT NOT NULL DEFAULT ''"},
	{"moves", "move_json", "TEXT NOT NULL DEFAULT ''"},
	{"players", "bot", "TEXT NOT NULL DEFAULT ''"},
	// lobby: the game waits for its host to start it, even with every seat
	// filled. auto: the move was played by the server on someone's behalf (a
	// reaction they chose to pass automatically).
	{"games", "lobby", "INTEGER NOT NULL DEFAULT 0"},
	{"moves", "auto", "INTEGER NOT NULL DEFAULT 0"},
	// locked: the move rolled dice (or drew a hidden token): it cannot be
	// taken back, and nothing before it can either.
	{"moves", "locked", "INTEGER NOT NULL DEFAULT 0"},
	// autoplay: a tactical bot played the move for a person whose Auto-Play
	// Timeout ran out.
	{"moves", "autoplay", "INTEGER NOT NULL DEFAULT 0"},
	// clock: the game clock of a game played with one (JSON; see
	// session.GameClock), empty for a game played without.
	{"games", "clock", "TEXT NOT NULL DEFAULT ''"},
}

func addMissingColumns(db *sql.DB) error {
	for _, m := range columnMigrations {
		has, err := hasColumn(db, m.table, m.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", m.table, m.column, m.ddl)
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}

func hasColumn(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			cid        int
			name, typ  string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Ping verifies the database is reachable and responsive.
func (s *Store) Ping(ctx context.Context) error {
	var result int
	if err := s.db.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
		return fmt.Errorf("select 1: %w", err)
	}
	return nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB exposes the underlying *sql.DB for test introspection. Production code uses
// the Store's methods; this is for tests that need raw SQL.
func (s *Store) DB() *sql.DB {
	return s.db
}
