// Tests of the database schema: the tables and indexes, default values and constraints.

package store_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/store"
)

// newTestStore opens a *store.Store against a fresh file in a per-test temp
// directory and registers a cleanup to close it.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open(%q) returned error: %v, want nil", dbPath, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// tableNames returns the names of all tables (type='table') present in the
// sqlite_master of the given *sql.DB.
func tableNames(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		t.Fatalf("querying sqlite_master for tables: %v", err)
	}
	defer func() { _ = rows.Close() }()

	names := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning sqlite_master row: %v", err)
		}
		names[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating sqlite_master rows: %v", err)
	}
	return names
}

// indexNames returns the names of all indexes (type='index') present in the
// sqlite_master of the given *sql.DB.
func indexNames(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='index'`)
	if err != nil {
		t.Fatalf("querying sqlite_master for indexes: %v", err)
	}
	defer func() { _ = rows.Close() }()

	names := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning sqlite_master row: %v", err)
		}
		names[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating sqlite_master rows: %v", err)
	}
	return names
}

// insertMinimalGame inserts a games row using only the NOT NULL columns that
// lack a DEFAULT (room_code, game_def_id, board_state) and returns its id.
func insertMinimalGame(t *testing.T, db *sql.DB, roomCode string) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO games (room_code, game_def_id, board_state) VALUES (?, ?, ?)`,
		roomCode, "demo-freeplay", `{"positions":[]}`,
	)
	if err != nil {
		t.Fatalf("inserting minimal games row (room_code=%q): %v, want nil", roomCode, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("res.LastInsertId() after games insert: %v, want nil", err)
	}
	return id
}

// -----------------------------------------------------------------------
// 1 & 2: Open creates the three tables and the moves index.
// -----------------------------------------------------------------------

func TestOpen_CreatesGamesPlayersAndMovesTables(t *testing.T) {
	s := newTestStore(t)

	got := tableNames(t, s.DB())
	for _, want := range []string{"games", "players", "moves"} {
		if !got[want] {
			t.Errorf("sqlite_master tables = %v, missing expected table %q", got, want)
		}
	}
}

func TestOpen_CreatesMovesGameSeqIndex(t *testing.T) {
	s := newTestStore(t)

	got := indexNames(t, s.DB())
	if !got["idx_moves_game_seq"] {
		t.Errorf("sqlite_master indexes = %v, missing expected index %q", got, "idx_moves_game_seq")
	}
}

// -----------------------------------------------------------------------
// 3: minimal games insert + defaults.
// -----------------------------------------------------------------------

func TestGamesInsert_MinimalRowGetsDefaultStatusAndMoveSeq(t *testing.T) {
	s := newTestStore(t)
	db := s.DB()

	id := insertMinimalGame(t, db, "ROOM01")

	var status string
	var moveSeq int64
	err := db.QueryRow(`SELECT status, move_seq FROM games WHERE id = ?`, id).Scan(&status, &moveSeq)
	if err != nil {
		t.Fatalf("reading back inserted games row (id=%d): %v, want nil", id, err)
	}

	if status != "waiting" {
		t.Errorf("games.status = %q, want %q", status, "waiting")
	}
	if moveSeq != 0 {
		t.Errorf("games.move_seq = %d, want 0", moveSeq)
	}
}

// -----------------------------------------------------------------------
// 4: players UNIQUE(game_id, seat).
// -----------------------------------------------------------------------

func TestPlayersInsert_DuplicateSeatForSameGameFails(t *testing.T) {
	s := newTestStore(t)
	db := s.DB()

	gameID := insertMinimalGame(t, db, "ROOM02")

	if _, err := db.Exec(
		`INSERT INTO players (game_id, seat, name, color) VALUES (?, ?, ?, ?)`,
		gameID, 1, "Alice", "black",
	); err != nil {
		t.Fatalf("inserting first player (seat=1): %v, want nil", err)
	}

	_, err := db.Exec(
		`INSERT INTO players (game_id, seat, name, color) VALUES (?, ?, ?, ?)`,
		gameID, 1, "Bob", "white",
	)
	if err == nil {
		t.Fatal("inserting second player with same (game_id, seat) succeeded, want a UNIQUE constraint error")
	}
}

// -----------------------------------------------------------------------
// 5: players UNIQUE(game_id, name COLLATE NOCASE).
// -----------------------------------------------------------------------

func TestPlayersInsert_DuplicateNameDifferingOnlyByCaseFails(t *testing.T) {
	s := newTestStore(t)
	db := s.DB()

	gameID := insertMinimalGame(t, db, "ROOM03")

	if _, err := db.Exec(
		`INSERT INTO players (game_id, seat, name, color) VALUES (?, ?, ?, ?)`,
		gameID, 1, "Alice", "black",
	); err != nil {
		t.Fatalf("inserting first player %q: %v, want nil", "Alice", err)
	}

	_, err := db.Exec(
		`INSERT INTO players (game_id, seat, name, color) VALUES (?, ?, ?, ?)`,
		gameID, 2, "alice", "white",
	)
	if err == nil {
		t.Fatal(`inserting second player named "alice" (game_id, seat differ but name matches "Alice" case-insensitively) succeeded, want a UNIQUE constraint error`)
	}
}

// -----------------------------------------------------------------------
// 6: moves UNIQUE(game_id, seq).
// -----------------------------------------------------------------------

func TestMovesInsert_DuplicateSeqForSameGameFails(t *testing.T) {
	s := newTestStore(t)
	db := s.DB()

	gameID := insertMinimalGame(t, db, "ROOM04")

	insertMove := func(seq int) error {
		_, err := db.Exec(
			`INSERT INTO moves (game_id, seq, seat, color, from_position, to_position, resulting_state)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			gameID, seq, 1, "black", "0,0", "0,1", `{"positions":[]}`,
		)
		return err
	}

	if err := insertMove(1); err != nil {
		t.Fatalf("inserting first move (seq=1): %v, want nil", err)
	}

	if err := insertMove(1); err == nil {
		t.Fatal("inserting second move with same (game_id, seq)=1 succeeded, want a UNIQUE constraint error")
	}
}

// -----------------------------------------------------------------------
// 7: foreign key enforcement is active (PRAGMA foreign_keys = ON).
// -----------------------------------------------------------------------

func TestForeignKeyEnforcement_PlayerReferencingUnknownGameFails(t *testing.T) {
	s := newTestStore(t)
	db := s.DB()

	const nonExistentGameID = int64(999999)

	_, err := db.Exec(
		`INSERT INTO players (game_id, seat, name, color) VALUES (?, ?, ?, ?)`,
		nonExistentGameID, 1, "Alice", "black",
	)
	if err == nil {
		t.Fatal("inserting a players row referencing a non-existent game_id succeeded, want a foreign key constraint error (PRAGMA foreign_keys = ON not active?)")
	}
}

func TestForeignKeyEnforcement_MoveReferencingUnknownGameFails(t *testing.T) {
	s := newTestStore(t)
	db := s.DB()

	const nonExistentGameID = int64(999999)

	_, err := db.Exec(
		`INSERT INTO moves (game_id, seq, seat, color, from_position, to_position, resulting_state)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		nonExistentGameID, 1, 1, "black", "0,0", "0,1", `{"positions":[]}`,
	)
	if err == nil {
		t.Fatal("inserting a moves row referencing a non-existent game_id succeeded, want a foreign key constraint error (PRAGMA foreign_keys = ON not active?)")
	}
}

// -----------------------------------------------------------------------
// 8: ON DELETE CASCADE.
// -----------------------------------------------------------------------

func TestOnDeleteCascade_DeletingGameRemovesItsPlayersAndMoves(t *testing.T) {
	s := newTestStore(t)
	db := s.DB()

	gameID := insertMinimalGame(t, db, "ROOM05")

	if _, err := db.Exec(
		`INSERT INTO players (game_id, seat, name, color) VALUES (?, ?, ?, ?)`,
		gameID, 1, "Alice", "black",
	); err != nil {
		t.Fatalf("inserting player for cascade test: %v, want nil", err)
	}

	if _, err := db.Exec(
		`INSERT INTO moves (game_id, seq, seat, color, from_position, to_position, resulting_state)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		gameID, 1, 1, "black", "0,0", "0,1", `{"positions":[]}`,
	); err != nil {
		t.Fatalf("inserting move for cascade test: %v, want nil", err)
	}

	if _, err := db.Exec(`DELETE FROM games WHERE id = ?`, gameID); err != nil {
		t.Fatalf("deleting games row (id=%d): %v, want nil", gameID, err)
	}

	var playerCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM players WHERE game_id = ?`, gameID).Scan(&playerCount); err != nil {
		t.Fatalf("counting players after cascade delete: %v, want nil", err)
	}
	if playerCount != 0 {
		t.Errorf("players remaining for deleted game_id=%d: got %d, want 0 (ON DELETE CASCADE not applied — is PRAGMA foreign_keys = ON active?)", gameID, playerCount)
	}

	var moveCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM moves WHERE game_id = ?`, gameID).Scan(&moveCount); err != nil {
		t.Fatalf("counting moves after cascade delete: %v, want nil", err)
	}
	if moveCount != 0 {
		t.Errorf("moves remaining for deleted game_id=%d: got %d, want 0 (ON DELETE CASCADE not applied — is PRAGMA foreign_keys = ON active?)", gameID, moveCount)
	}
}

// -----------------------------------------------------------------------
// 9 & 10: the AC itself — calling store.Open twice in a row against the
// same fresh temp file does not error, and does not destroy prior data.
// -----------------------------------------------------------------------

func TestOpen_ReopenIsIdempotentAndPreservesData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reopen.db")

	first, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("first store.Open(%q) returned error: %v, want nil", dbPath, err)
	}

	gameID := insertMinimalGame(t, first.DB(), "ROOM06")

	if _, err := first.DB().Exec(
		`INSERT INTO players (game_id, seat, name, color) VALUES (?, ?, ?, ?)`,
		gameID, 1, "Alice", "black",
	); err != nil {
		t.Fatalf("inserting player before reopen: %v, want nil", err)
	}

	if _, err := first.DB().Exec(
		`INSERT INTO moves (game_id, seq, seat, color, from_position, to_position, resulting_state)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		gameID, 1, 1, "black", "0,0", "0,1", `{"positions":[]}`,
	); err != nil {
		t.Fatalf("inserting move before reopen: %v, want nil", err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("first.Close() returned error: %v, want nil", err)
	}

	// The AC: opening the SAME file path a second time must not error —
	// i.e. the CREATE TABLE IF NOT EXISTS statements must be idempotent
	// against an already-migrated file.
	second, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("second store.Open(%q) (same path as first) returned error: %v, want nil (CREATE TABLE IF NOT EXISTS must be idempotent)", dbPath, err)
	}
	if second == nil {
		t.Fatal("second store.Open(dbPath) returned nil *Store with nil error")
	}

	// Tables must still be present after the second Open.
	gotTables := tableNames(t, second.DB())
	for _, want := range []string{"games", "players", "moves"} {
		if !gotTables[want] {
			t.Errorf("after reopen, sqlite_master tables = %v, missing expected table %q", gotTables, want)
		}
	}

	// Data inserted before Close must still be present — the second Open
	// must not have dropped/recreated the tables destructively.
	var roomCode, status string
	err = second.DB().QueryRow(`SELECT room_code, status FROM games WHERE id = ?`, gameID).Scan(&roomCode, &status)
	if err != nil {
		t.Fatalf("reading back games row (id=%d) after reopen: %v, want nil (row should have survived reopen)", gameID, err)
	}
	if roomCode != "ROOM06" {
		t.Errorf("after reopen, games.room_code = %q, want %q", roomCode, "ROOM06")
	}

	var playerCount int
	if err := second.DB().QueryRow(`SELECT COUNT(*) FROM players WHERE game_id = ?`, gameID).Scan(&playerCount); err != nil {
		t.Fatalf("counting players after reopen: %v, want nil", err)
	}
	if playerCount != 1 {
		t.Errorf("after reopen, players count for game_id=%d = %d, want 1 (data should survive reopen)", gameID, playerCount)
	}

	var moveCount int
	if err := second.DB().QueryRow(`SELECT COUNT(*) FROM moves WHERE game_id = ?`, gameID).Scan(&moveCount); err != nil {
		t.Fatalf("counting moves after reopen: %v, want nil", err)
	}
	if moveCount != 1 {
		t.Errorf("after reopen, moves count for game_id=%d = %d, want 1 (data should survive reopen)", gameID, moveCount)
	}

	// 10: Close on the reopened Store also returns no error.
	if err := second.Close(); err != nil {
		t.Errorf("second.Close() returned error: %v, want nil", err)
	}
}

// TestOpen_ReopenThreeTimesStillNoError is a light extra edge case beyond
// the AC's literal "twice in a row": Open/Close cycling more than twice
// against the same file must also stay error-free, guarding against an
// implementation that special-cases exactly two opens.
func TestOpen_ReopenThreeTimesStillNoError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reopen3.db")

	for i := 0; i < 3; i++ {
		s, err := store.Open(dbPath)
		if err != nil {
			t.Fatalf("store.Open(%q) attempt #%d returned error: %v, want nil", dbPath, i+1, err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("s.Close() attempt #%d returned error: %v, want nil", i+1, err)
		}
	}
}
