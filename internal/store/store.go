// Package store provides bingle's persistence layer over SQLite.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"

	"github.com/noderunner/bingle/internal/metrics"
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
	// path is the database file, kept so the metrics can report how big it
	// and its write-ahead log have become (see stats.go).
	path string
	// mx is the registry statements are recorded into; nil records nothing
	// (see measure.go).
	mx *metrics.Metrics
	// stats caches what Snapshot reports.
	stats snapshotCache
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

	if err := renameColumns(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite schema: %w", err)
	}

	if err := addMissingColumns(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite schema: %w", err)
	}
	// Indexes on columns that addMissingColumns may just have added.
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_games_rating_pending ON games(rating_status) WHERE rating_status = 'pending'`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite schema: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_games_real_time ON games(status) WHERE real_time != ''`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite schema: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_players_principal ON players(principal_id) WHERE principal_id != ''`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite schema: %w", err)
	}

	if err := pinPrototypes(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite schema: %w", err)
	}

	return &Store{db: db, path: dbPath}, nil
}

// renamedColumns are the columns an older database has under another name: the
// seat to act is the impulse seat (ADR 0099).
var renamedColumns = []struct{ table, from, to string }{
	{"games", "turn_seat", "impulse_seat"},
	{"games", "initial_turn_seat", "initial_impulse_seat"},
}

// renameColumns brings the names of an older database up to date. It runs before
// addMissingColumns, which would otherwise add a new column beside the old one.
func renameColumns(db *sql.DB) error {
	for _, c := range renamedColumns {
		var old int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, c.table, c.from).Scan(&old); err != nil {
			return err
		}
		if old == 0 {
			continue
		}
		if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", c.table, c.from, c.to)); err != nil {
			return err
		}
	}
	return nil
}

// pinPrototypes moves what was stored under the bare id of a game that became a
// prototype to the version it was played under, since the bare id now means the
// newest version (ADR 0098): the games, those derived from it (id~suffix), its
// tournaments and its tutorial rooms ("id/tutorial"). It finds nothing to do once
// it has run.
func pinPrototypes(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range []string{"courtfall", "mycelium"} {
		pinned := id + "-1.0.0"
		for _, stmt := range []string{
			`UPDATE games SET game_def_id = ? || substr(game_def_id, ?) WHERE game_def_id = ? OR game_def_id GLOB ?`,
			`UPDATE tournaments SET game_def_id = ? || substr(game_def_id, ?) WHERE game_def_id = ? OR game_def_id GLOB ?`,
		} {
			if _, err := tx.Exec(stmt, pinned, len(id)+1, id, id+"~*"); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE games SET tutorial = ? || substr(tutorial, ?) WHERE tutorial GLOB ?`, pinned, len(id)+1, id+"/*"); err != nil {
			return err
		}
	}
	return tx.Commit()
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
	{"games", "initial_impulse_seat", "INTEGER NOT NULL DEFAULT 0"},
	// takeback_seat, takeback_seq: unused since take-backs went (ADR 0083). They
	// stay so that an older database opens as it did.
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
	// principal_id: who owns the seat, the game or the line (ADR 0067); empty
	// for a row written by an anonymous client or before principals existed.
	{"players", "principal_id", "TEXT NOT NULL DEFAULT ''"},
	{"games", "owner_principal_id", "TEXT NOT NULL DEFAULT ''"},
	{"chat_messages", "principal_id", "TEXT NOT NULL DEFAULT ''"},
	{"global_chat", "principal_id", "TEXT NOT NULL DEFAULT ''"},
	// tutorial: "family/id" of the tutorial a room plays (ADR 0077), empty
	// for an ordinary game. Tutorial rooms stay out of the lists of games.
	{"games", "tutorial", "TEXT NOT NULL DEFAULT ''"},
	// rated: the game moves its players' ratings when it ends. rating_status:
	// what became of that (see RatingPending); the game is marked pending in
	// the transaction that ends it.
	{"games", "rated", "INTEGER NOT NULL DEFAULT 0"},
	{"games", "rating_status", "TEXT NOT NULL DEFAULT ''"},
	// tournament_id: the tournament the game is part of (0 for none).
	{"games", "tournament_id", "INTEGER NOT NULL DEFAULT 0"},
	// tournament_id: the tournament a rated result came from (0 for none).
	{"rating_history", "tournament_id", "INTEGER NOT NULL DEFAULT 0"},
	// account_id: the contact is an account's own, for its tournament games.
	{"notify_contacts", "account_id", "INTEGER NOT NULL DEFAULT 0"},
	// card_set: the themed set of cards the table plays with (ADR 0085), empty
	// for the game's own cards.
	{"games", "card_set", "TEXT NOT NULL DEFAULT ''"},
	// real_time: the settings of a game played in real time (ADR 0089), empty
	// for any other.
	{"games", "real_time", "TEXT NOT NULL DEFAULT ''"},
	// A real-time tournament (ADR 0090), and who has checked in to it.
	{"tournaments", "real_time", "INTEGER NOT NULL DEFAULT 0"},
	{"tournaments", "starts_at", "TEXT NOT NULL DEFAULT ''"},
	{"tournaments", "check_in_seconds", "INTEGER NOT NULL DEFAULT 0"},
	{"tournaments", "round_break_seconds", "INTEGER NOT NULL DEFAULT 0"},
	{"tournaments", "first_move_seconds", "INTEGER NOT NULL DEFAULT 0"},
	{"tournaments", "next_round_at", "TEXT NOT NULL DEFAULT ''"},
	{"tournament_entries", "checked_in", "INTEGER NOT NULL DEFAULT 0"},
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
	if err := s.queryRow(ctx, "Ping", "SELECT 1").Scan(&result); err != nil {
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
