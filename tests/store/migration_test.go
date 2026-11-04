package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/noderunner/bingle/internal/store"
)

// v1Schema is the schema bingle shipped before notation, bots and end
// reasons were added. Opening a database created with it must upgrade it in
// place without losing rows.
const v1Schema = `
CREATE TABLE games (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    room_code     TEXT NOT NULL UNIQUE,
    game_def_id   TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'waiting',
    board_state   TEXT NOT NULL,
    turn_seat     INTEGER,
    move_seq      INTEGER NOT NULL DEFAULT 0,
    winner_seat   INTEGER,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE players (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id       INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    seat          INTEGER NOT NULL,
    name          TEXT NOT NULL,
    color         TEXT NOT NULL,
    joined_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(game_id, seat),
    UNIQUE(game_id, name COLLATE NOCASE)
);
CREATE TABLE moves (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id              INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    seq                  INTEGER NOT NULL,
    seat                 INTEGER NOT NULL,
    color                TEXT NOT NULL,
    from_position        TEXT NOT NULL,
    to_position          TEXT NOT NULL,
    captured_position    TEXT,
    resulting_state      TEXT NOT NULL,
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(game_id, seq)
);
CREATE INDEX idx_moves_game_seq ON moves(game_id, seq);
`

func TestOpen_UpgradesAV1DatabaseInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		v1Schema,
		`INSERT INTO games (room_code, game_def_id, status, board_state, turn_seat, move_seq) VALUES ('OLDGAM', 'test-freeplay', 'active', '{"0,0":{"Piece":"stone","Color":"black"}}', 2, 1)`,
		`INSERT INTO players (game_id, seat, name, color) VALUES (1, 1, 'Alice', 'black'), (1, 2, 'Bob', 'white')`,
		`INSERT INTO moves (game_id, seq, seat, color, from_position, to_position, resulting_state) VALUES (1, 1, 1, 'black', '0,0', '0,1', '{"0,1":{"Piece":"stone","Color":"black"}}')`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("seeding v1 database: %v\n%s", err, stmt)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open on a v1 database: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()

	// Old rows read fine, with new columns defaulted.
	g, err := st.GetGame(ctx, "OLDGAM")
	if err != nil {
		t.Fatalf("GetGame: %v", err)
	}
	if g.Status != "active" || g.MoveSeq != 1 || g.EndReason != "" {
		t.Errorf("game = %+v", g)
	}
	players, err := st.ListPlayers(ctx, g.ID)
	if err != nil || len(players) != 2 || players[0].Bot != "" {
		t.Errorf("players = %+v err=%v", players, err)
	}
	moves, err := st.ListMoves(ctx, g.ID, 0)
	if err != nil || len(moves) != 1 || moves[0].Notation != "" || moves[0].MoveJSON != "" {
		t.Errorf("moves = %+v err=%v", moves, err)
	}

	// New writes use the new columns.
	_, err = st.RecordMove(ctx, g.ID, store.MoveInput{
		Seat: 2, Color: "white", From: "0,8", To: "0,7", ResultingBoardState: "{}",
		NextTurnSeat: 1, Notation: "0,8-0,7", MoveJSON: `{"from":"0,8"}`,
	})
	if err != nil {
		t.Fatalf("RecordMove after upgrade: %v", err)
	}

	// Opening again is a no-op.
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st2, err := store.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	_ = st2.Close()
}

// What was stored under the bare id of a game that became a prototype moves to
// the version it was played under, and nothing else moves. Opening again is
// harmless.
func TestOpen_PinsTheGamesOfPrototypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	games := map[string]struct{ def, tutorial, wantDef, wantTutorial string }{
		"AAAAAA": {"courtfall", "", "courtfall-1.0.0", ""},
		"BBBBBB": {"courtfall~arena", "", "courtfall-1.0.0~arena", ""},
		"CCCCCC": {"mycelium", "mycelium/mycelium-basics", "mycelium-1.0.0", "mycelium-1.0.0/mycelium-basics"},
		"DDDDDD": {"mycelium-1.0.0", "", "mycelium-1.0.0", ""},
		"EEEEEE": {"courtfall-2.0.0~arena", "", "courtfall-2.0.0~arena", ""},
		"FFFFFF": {"courtfallish", "courtfallish/x", "courtfallish", "courtfallish/x"},
		"GGGGGG": {"tak-5x5", "tak-5x5/basics", "tak-5x5", "tak-5x5/basics"},
	}
	for room, g := range games {
		if _, err := raw.Exec(`INSERT INTO games (room_code, game_def_id, board_state, tutorial) VALUES (?, ?, '{}', ?)`, room, g.def, g.tutorial); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		st, err := store.Open(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		for room, g := range games {
			got, err := st.GetGame(context.Background(), room)
			if err != nil {
				t.Fatal(err)
			}
			if got.GameDefID != g.wantDef || got.Tutorial != g.wantTutorial {
				t.Errorf("open %d: %s is %q %q, want %q %q", i, room, got.GameDefID, got.Tutorial, g.wantDef, g.wantTutorial)
			}
		}
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
