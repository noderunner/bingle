// Tests of creating and reading games in the database.

package store_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/store"
)

// newCreateGameTestStore opens a *store.Store against a fresh file in a
// per-test temp directory and registers a cleanup to close it. Mirrors
// newTestStore in store_test.go (kept separate per instructions not to edit
// that file).
func newCreateGameTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open(%q) returned error: %v, want nil", dbPath, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// countRows returns the number of rows in table matching the given
// WHERE-clause predicate (no "WHERE" keyword, no args) -- used for counting
// rows unconditionally across a whole table.
func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("counting rows with query %q: %v, want nil", query, err)
	}
	return n
}

// -----------------------------------------------------------------------
// 1 & 2: CreateGame succeeds, inserts exactly one games row and one
// players row, and the returned Game matches what was passed in / the
// documented defaults.
// -----------------------------------------------------------------------

func TestCreateGame_InsertsExactlyOneGameRowAndOnePlayerRow(t *testing.T) {
	s := newCreateGameTestStore(t)
	ctx := t.Context()

	game, player, err := s.CreateGame(ctx, "ROOM01", "test-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}

	gameCount := countRows(t, s.DB(), `SELECT COUNT(*) FROM games`)
	if gameCount != 1 {
		t.Errorf("games row count after CreateGame = %d, want exactly 1", gameCount)
	}

	playerCount := countRows(t, s.DB(), `SELECT COUNT(*) FROM players`)
	if playerCount != 1 {
		t.Errorf("players row count after CreateGame = %d, want exactly 1", playerCount)
	}

	if game.RoomCode != "ROOM01" {
		t.Errorf("game.RoomCode = %q, want %q", game.RoomCode, "ROOM01")
	}
	if game.GameDefID != "test-freeplay" {
		t.Errorf("game.GameDefID = %q, want %q", game.GameDefID, "test-freeplay")
	}
	if game.BoardState != `{"positions":[]}` {
		t.Errorf("game.BoardState = %q, want %q", game.BoardState, `{"positions":[]}`)
	}
	if game.Status != "waiting" {
		t.Errorf("game.Status = %q, want %q", game.Status, "waiting")
	}
	if game.MoveSeq != 0 {
		t.Errorf("game.MoveSeq = %d, want 0", game.MoveSeq)
	}
	if game.TurnSeat != nil {
		t.Errorf("game.TurnSeat = %v, want nil", game.TurnSeat)
	}
	if game.WinnerSeat != nil {
		t.Errorf("game.WinnerSeat = %v, want nil", game.WinnerSeat)
	}
	if game.CreatedAt == "" {
		t.Error("game.CreatedAt is empty, want non-empty")
	}
	if game.UpdatedAt == "" {
		t.Error("game.UpdatedAt is empty, want non-empty")
	}
	if game.ID == 0 {
		t.Error("game.ID is zero, want a non-zero assigned id")
	}

	// -----------------------------------------------------------------------
	// 3: the returned Player.
	// -----------------------------------------------------------------------
	if player.Seat != 1 {
		t.Errorf("player.Seat = %d, want 1", player.Seat)
	}
	if player.Name != "Alice" {
		t.Errorf("player.Name = %q, want %q", player.Name, "Alice")
	}
	if player.Color != "black" {
		t.Errorf("player.Color = %q, want %q", player.Color, "black")
	}
	if player.GameID != game.ID {
		t.Errorf("player.GameID = %d, want %d (game.ID)", player.GameID, game.ID)
	}
	if player.JoinedAt == "" {
		t.Error("player.JoinedAt is empty, want non-empty")
	}
	if player.LastSeenAt == "" {
		t.Error("player.LastSeenAt is empty, want non-empty")
	}
}

// -----------------------------------------------------------------------
// 4: GetGame returns a Game equal in every field to what CreateGame
// returned.
// -----------------------------------------------------------------------

func TestGetGame_ReturnsSameFieldsAsCreateGame(t *testing.T) {
	s := newCreateGameTestStore(t)
	ctx := t.Context()

	created, _, err := s.CreateGame(ctx, "ROOM02", "test-freeplay", `{"positions":[1,2,3]}`, "Bob", "white")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}

	got, err := s.GetGame(ctx, "ROOM02")
	if err != nil {
		t.Fatalf("GetGame(%q) returned error: %v, want nil", "ROOM02", err)
	}

	if got != created {
		t.Errorf("GetGame(%q) = %+v, want equal to CreateGame's returned Game %+v", "ROOM02", got, created)
	}
}

// -----------------------------------------------------------------------
// 5: GetGame for a never-created room code returns ErrNotFound.
// -----------------------------------------------------------------------

func TestGetGame_UnknownRoomCodeReturnsErrNotFound(t *testing.T) {
	s := newCreateGameTestStore(t)
	ctx := t.Context()

	_, err := s.GetGame(ctx, "NOSUCHROOM")
	if err == nil {
		t.Fatal("GetGame for unknown room code returned nil error, want ErrNotFound")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetGame for unknown room code returned error %v, want errors.Is(err, store.ErrNotFound) to be true", err)
	}
}

// -----------------------------------------------------------------------
// 6: two games with different room codes -- GetGame returns the correct,
// distinct one for each, no crosstalk.
// -----------------------------------------------------------------------

func TestGetGame_TwoGamesNoCrosstalk(t *testing.T) {
	s := newCreateGameTestStore(t)
	ctx := t.Context()

	game1, _, err := s.CreateGame(ctx, "ROOMA", "test-freeplay", `{"board":"A"}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame(ROOMA) returned error: %v, want nil", err)
	}
	game2, _, err := s.CreateGame(ctx, "ROOMB", "othergame", `{"board":"B"}`, "Carol", "white")
	if err != nil {
		t.Fatalf("CreateGame(ROOMB) returned error: %v, want nil", err)
	}

	gotA, err := s.GetGame(ctx, "ROOMA")
	if err != nil {
		t.Fatalf("GetGame(ROOMA) returned error: %v, want nil", err)
	}
	gotB, err := s.GetGame(ctx, "ROOMB")
	if err != nil {
		t.Fatalf("GetGame(ROOMB) returned error: %v, want nil", err)
	}

	if gotA.RoomCode != "ROOMA" || gotA.GameDefID != "test-freeplay" || gotA.BoardState != `{"board":"A"}` {
		t.Errorf("GetGame(ROOMA) = %+v, want RoomCode=ROOMA GameDefID=test-freeplay BoardState={\"board\":\"A\"}", gotA)
	}
	if gotB.RoomCode != "ROOMB" || gotB.GameDefID != "othergame" || gotB.BoardState != `{"board":"B"}` {
		t.Errorf("GetGame(ROOMB) = %+v, want RoomCode=ROOMB GameDefID=othergame BoardState={\"board\":\"B\"}", gotB)
	}

	if gotA.ID == gotB.ID {
		t.Errorf("GetGame(ROOMA).ID == GetGame(ROOMB).ID == %d, want distinct ids", gotA.ID)
	}
	if gotA.ID != game1.ID {
		t.Errorf("GetGame(ROOMA).ID = %d, want %d (matches CreateGame's returned id)", gotA.ID, game1.ID)
	}
	if gotB.ID != game2.ID {
		t.Errorf("GetGame(ROOMB).ID = %d, want %d (matches CreateGame's returned id)", gotB.ID, game2.ID)
	}
}

// -----------------------------------------------------------------------
// 7: atomicity on duplicate room code -- the AC's "both or neither"
// guarantee.
// -----------------------------------------------------------------------

func TestCreateGame_DuplicateRoomCodeFailsAtomicallyLeavingNoPartialRows(t *testing.T) {
	s := newCreateGameTestStore(t)
	ctx := t.Context()

	if _, _, err := s.CreateGame(ctx, "DUP01", "test-freeplay", `{"positions":[]}`, "Alice", "black"); err != nil {
		t.Fatalf("first CreateGame(DUP01) returned error: %v, want nil", err)
	}

	_, _, err := s.CreateGame(ctx, "DUP01", "test-freeplay", `{"positions":[]}`, "Zed", "red")
	if err == nil {
		t.Fatal("second CreateGame with duplicate room code DUP01 returned nil error, want non-nil (UNIQUE constraint violation)")
	}

	gameCount := countRows(t, s.DB(), `SELECT COUNT(*) FROM games WHERE room_code = ?`, "DUP01")
	if gameCount != 1 {
		t.Errorf("games row count for room_code=DUP01 after failed duplicate CreateGame = %d, want exactly 1 (no duplicate game row left behind)", gameCount)
	}

	// There must be exactly one players row for that game (the first
	// creator's seat) -- the failed second attempt must not have left a
	// dangling player row (e.g. from a non-atomic insert-player-then-fail
	// implementation), nor must the successful first game have somehow
	// gained a second player.
	var gameID int64
	if err := s.DB().QueryRow(`SELECT id FROM games WHERE room_code = ?`, "DUP01").Scan(&gameID); err != nil {
		t.Fatalf("looking up games.id for room_code=DUP01: %v, want nil", err)
	}
	playerCount := countRows(t, s.DB(), `SELECT COUNT(*) FROM players WHERE game_id = ?`, gameID)
	if playerCount != 1 {
		t.Errorf("players row count for DUP01's game_id=%d after failed duplicate CreateGame = %d, want exactly 1", gameID, playerCount)
	}

	// The surviving row must still belong to the FIRST creator, "Alice",
	// not "Zed" -- proving the second (failed) attempt didn't clobber
	// anything.
	var name string
	if err := s.DB().QueryRow(`SELECT name FROM players WHERE game_id = ?`, gameID).Scan(&name); err != nil {
		t.Fatalf("reading back surviving player's name: %v, want nil", err)
	}
	if name != "Alice" {
		t.Errorf("surviving player name after failed duplicate CreateGame = %q, want %q", name, "Alice")
	}

	// GetGame must still return the original creator's game untouched.
	got, err := s.GetGame(ctx, "DUP01")
	if err != nil {
		t.Fatalf("GetGame(DUP01) after failed duplicate CreateGame returned error: %v, want nil", err)
	}
	if got.ID != gameID {
		t.Errorf("GetGame(DUP01).ID = %d, want %d", got.ID, gameID)
	}
}

// -----------------------------------------------------------------------
// 8: seat numbering is per-game, not global -- two different successful
// CreateGame calls each put their own creator at seat 1 in their own game.
// -----------------------------------------------------------------------

func TestCreateGame_SeatNumberingIsPerGameNotGlobal(t *testing.T) {
	s := newCreateGameTestStore(t)
	ctx := t.Context()

	game1, player1, err := s.CreateGame(ctx, "SEATA", "test-freeplay", `{"positions":[]}`, "Dave", "black")
	if err != nil {
		t.Fatalf("CreateGame(SEATA) returned error: %v, want nil", err)
	}
	game2, player2, err := s.CreateGame(ctx, "SEATB", "test-freeplay", `{"positions":[]}`, "Erin", "white")
	if err != nil {
		t.Fatalf("CreateGame(SEATB) returned error: %v, want nil", err)
	}

	if player1.Seat != 1 {
		t.Errorf("player1 (SEATA creator).Seat = %d, want 1", player1.Seat)
	}
	if player2.Seat != 1 {
		t.Errorf("player2 (SEATB creator).Seat = %d, want 1", player2.Seat)
	}
	if player1.GameID != game1.ID {
		t.Errorf("player1.GameID = %d, want %d (game1.ID)", player1.GameID, game1.ID)
	}
	if player2.GameID != game2.ID {
		t.Errorf("player2.GameID = %d, want %d (game2.ID)", player2.GameID, game2.ID)
	}
	if player1.GameID == player2.GameID {
		t.Errorf("player1.GameID == player2.GameID == %d, want distinct game ids for distinct games", player1.GameID)
	}
	if player1.Name != "Dave" {
		t.Errorf("player1.Name = %q, want %q", player1.Name, "Dave")
	}
	if player2.Name != "Erin" {
		t.Errorf("player2.Name = %q, want %q", player2.Name, "Erin")
	}
}
