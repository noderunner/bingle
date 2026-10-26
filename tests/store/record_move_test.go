// Tests of recording moves: sequence numbers, the game row, and captured squares.

package store_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/store"
)

// newRecordMoveTestStore opens a *store.Store against a fresh file in a
// per-test temp directory and registers a cleanup to close it. Mirrors
// newTestStore/newCreateGameTestStore (kept separate per instructions not to
// edit those files).
func newRecordMoveTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open(%q) returned error: %v, want nil", dbPath, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// rmCountRows returns the number of rows matching the given query/args.
// Named distinctly from create_game_test.go's countRows to avoid a
// same-package redeclaration, even though the bodies are identical.
func rmCountRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("counting rows with query %q: %v, want nil", query, err)
	}
	return n
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

// -----------------------------------------------------------------------
// 1: the AC itself -- RecordMove on a freshly created game (move_seq=0)
// produces seq=1, a Move whose fields match the input, and a GetGame that
// reflects the new board_state/move_seq/turn_seat.
// -----------------------------------------------------------------------

func TestRecordMove_FirstMoveGetsSeqOneAndUpdatesGame(t *testing.T) {
	s := newRecordMoveTestStore(t)
	ctx := t.Context()

	game, _, err := s.CreateGame(ctx, "ROOM01", "demo-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}
	if game.MoveSeq != 0 {
		t.Fatalf("precondition: game.MoveSeq = %d, want 0", game.MoveSeq)
	}

	in := store.MoveInput{
		Seat:                1,
		Color:               "black",
		From:                "0,0",
		To:                  "0,1",
		Captured:            nil,
		ResultingBoardState: `{"positions":["0,1"]}`,
		NextTurnSeat:        2,
		GameEnded:           false,
		WinnerSeat:          nil,
	}

	move, err := s.RecordMove(ctx, game.ID, in)
	if err != nil {
		t.Fatalf("RecordMove returned error: %v, want nil", err)
	}

	if move.Seq != 1 {
		t.Errorf("move.Seq = %d, want 1", move.Seq)
	}
	if move.GameID != game.ID {
		t.Errorf("move.GameID = %d, want %d (created game's id)", move.GameID, game.ID)
	}
	if move.Seat != in.Seat {
		t.Errorf("move.Seat = %d, want %d", move.Seat, in.Seat)
	}
	if move.Color != in.Color {
		t.Errorf("move.Color = %q, want %q", move.Color, in.Color)
	}
	if move.From != in.From {
		t.Errorf("move.From = %q, want %q", move.From, in.From)
	}
	if move.To != in.To {
		t.Errorf("move.To = %q, want %q", move.To, in.To)
	}
	if move.Captured != nil {
		t.Errorf("move.Captured = %v, want nil", move.Captured)
	}
	if move.ResultingState != in.ResultingBoardState {
		t.Errorf("move.ResultingState = %q, want %q", move.ResultingState, in.ResultingBoardState)
	}
	if move.CreatedAt == "" {
		t.Error("move.CreatedAt is empty, want non-empty")
	}
	if move.ID == 0 {
		t.Error("move.ID is zero, want a non-zero assigned id")
	}

	got, err := s.GetGame(ctx, "ROOM01")
	if err != nil {
		t.Fatalf("GetGame(ROOM01) returned error: %v, want nil", err)
	}
	if got.BoardState != in.ResultingBoardState {
		t.Errorf("after RecordMove, GetGame.BoardState = %q, want %q", got.BoardState, in.ResultingBoardState)
	}
	if got.MoveSeq != 1 {
		t.Errorf("after RecordMove, GetGame.MoveSeq = %d, want 1", got.MoveSeq)
	}
	if got.TurnSeat == nil || *got.TurnSeat != in.NextTurnSeat {
		t.Errorf("after RecordMove, GetGame.TurnSeat = %v, want pointer to %d", got.TurnSeat, in.NextTurnSeat)
	}
}

// -----------------------------------------------------------------------
// 2: a SECOND RecordMove call continues numbering from the game's current
// move_seq (not reset to 1 again), and GetGame reflects the newest values.
// -----------------------------------------------------------------------

func TestRecordMove_SecondMoveGetsSeqTwoAndUpdatesGameAgain(t *testing.T) {
	s := newRecordMoveTestStore(t)
	ctx := t.Context()

	game, _, err := s.CreateGame(ctx, "ROOM02", "demo-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}

	first := store.MoveInput{
		Seat:                1,
		Color:               "black",
		From:                "0,0",
		To:                  "0,1",
		ResultingBoardState: `{"positions":["0,1"]}`,
		NextTurnSeat:        2,
	}
	if _, err := s.RecordMove(ctx, game.ID, first); err != nil {
		t.Fatalf("first RecordMove returned error: %v, want nil", err)
	}

	second := store.MoveInput{
		Seat:                2,
		Color:               "white",
		From:                "7,7",
		To:                  "7,6",
		ResultingBoardState: `{"positions":["0,1","7,6"]}`,
		NextTurnSeat:        1,
	}
	move2, err := s.RecordMove(ctx, game.ID, second)
	if err != nil {
		t.Fatalf("second RecordMove returned error: %v, want nil", err)
	}

	if move2.Seq != 2 {
		t.Errorf("second move.Seq = %d, want 2 (continuing from the game's current move_seq, not reset)", move2.Seq)
	}

	got, err := s.GetGame(ctx, "ROOM02")
	if err != nil {
		t.Fatalf("GetGame(ROOM02) returned error: %v, want nil", err)
	}
	if got.MoveSeq != 2 {
		t.Errorf("after second RecordMove, GetGame.MoveSeq = %d, want 2", got.MoveSeq)
	}
	if got.BoardState != second.ResultingBoardState {
		t.Errorf("after second RecordMove, GetGame.BoardState = %q, want %q (the newest state)", got.BoardState, second.ResultingBoardState)
	}
	if got.TurnSeat == nil || *got.TurnSeat != second.NextTurnSeat {
		t.Errorf("after second RecordMove, GetGame.TurnSeat = %v, want pointer to %d", got.TurnSeat, second.NextTurnSeat)
	}
}

// -----------------------------------------------------------------------
// 3: querying s.DB() directly shows exactly 2 rows in moves for that
// game_id, with seq values 1 and 2 in that order.
// -----------------------------------------------------------------------

func TestRecordMove_TwoCallsInsertExactlyTwoMovesRowsInOrder(t *testing.T) {
	s := newRecordMoveTestStore(t)
	ctx := t.Context()

	game, _, err := s.CreateGame(ctx, "ROOM03", "demo-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}

	if _, err := s.RecordMove(ctx, game.ID, store.MoveInput{
		Seat: 1, Color: "black", From: "0,0", To: "0,1",
		ResultingBoardState: `{"positions":["0,1"]}`, NextTurnSeat: 2,
	}); err != nil {
		t.Fatalf("first RecordMove returned error: %v, want nil", err)
	}
	if _, err := s.RecordMove(ctx, game.ID, store.MoveInput{
		Seat: 2, Color: "white", From: "7,7", To: "7,6",
		ResultingBoardState: `{"positions":["0,1","7,6"]}`, NextTurnSeat: 1,
	}); err != nil {
		t.Fatalf("second RecordMove returned error: %v, want nil", err)
	}

	count := rmCountRows(t, s.DB(), `SELECT COUNT(*) FROM moves WHERE game_id = ?`, game.ID)
	if count != 2 {
		t.Fatalf("moves row count for game_id=%d = %d, want exactly 2", game.ID, count)
	}

	rows, err := s.DB().Query(`SELECT seq FROM moves WHERE game_id = ? ORDER BY seq ASC`, game.ID)
	if err != nil {
		t.Fatalf("querying moves.seq: %v, want nil", err)
	}
	defer func() { _ = rows.Close() }()

	var seqs []int
	for rows.Next() {
		var seq int
		if err := rows.Scan(&seq); err != nil {
			t.Fatalf("scanning moves.seq: %v, want nil", err)
		}
		seqs = append(seqs, seq)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating moves rows: %v, want nil", err)
	}

	if len(seqs) != 2 || seqs[0] != 1 || seqs[1] != 2 {
		t.Errorf("moves.seq values for game_id=%d = %v, want [1 2]", game.ID, seqs)
	}
}

// -----------------------------------------------------------------------
// 4: captures -- a non-nil Captured is stored and returned exactly, a nil
// Captured leaves captured_position NULL and Move.Captured nil.
// -----------------------------------------------------------------------

func TestRecordMove_CapturedPositionStoredWhenNonNil(t *testing.T) {
	s := newRecordMoveTestStore(t)
	ctx := t.Context()

	game, _, err := s.CreateGame(ctx, "ROOM04", "demo-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}

	in := store.MoveInput{
		Seat:                1,
		Color:               "black",
		From:                "0,0",
		To:                  "0,2",
		Captured:            strPtr("0,1"),
		ResultingBoardState: `{"positions":["0,2"]}`,
		NextTurnSeat:        2,
	}

	move, err := s.RecordMove(ctx, game.ID, in)
	if err != nil {
		t.Fatalf("RecordMove returned error: %v, want nil", err)
	}

	if move.Captured == nil || *move.Captured != "0,1" {
		t.Errorf("move.Captured = %v, want pointer to %q", move.Captured, "0,1")
	}

	var captured sql.NullString
	if err := s.DB().QueryRow(`SELECT captured_position FROM moves WHERE id = ?`, move.ID).Scan(&captured); err != nil {
		t.Fatalf("reading back captured_position: %v, want nil", err)
	}
	if !captured.Valid || captured.String != "0,1" {
		t.Errorf("moves.captured_position = %+v, want valid %q", captured, "0,1")
	}
}

func TestRecordMove_CapturedPositionLeftNullWhenNil(t *testing.T) {
	s := newRecordMoveTestStore(t)
	ctx := t.Context()

	game, _, err := s.CreateGame(ctx, "ROOM05", "demo-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}

	in := store.MoveInput{
		Seat:                1,
		Color:               "black",
		From:                "0,0",
		To:                  "0,1",
		Captured:            nil,
		ResultingBoardState: `{"positions":["0,1"]}`,
		NextTurnSeat:        2,
	}

	move, err := s.RecordMove(ctx, game.ID, in)
	if err != nil {
		t.Fatalf("RecordMove returned error: %v, want nil", err)
	}

	if move.Captured != nil {
		t.Errorf("move.Captured = %v, want nil", move.Captured)
	}

	var captured sql.NullString
	if err := s.DB().QueryRow(`SELECT captured_position FROM moves WHERE id = ?`, move.ID).Scan(&captured); err != nil {
		t.Fatalf("reading back captured_position: %v, want nil", err)
	}
	if captured.Valid {
		t.Errorf("moves.captured_position = %+v, want NULL (Valid=false)", captured)
	}
}

// -----------------------------------------------------------------------
// 5: game-ended path -- GameEnded:true sets turn_seat NULL, status
// "finished", and winner_seat to WinnerSeat.
// -----------------------------------------------------------------------

func TestRecordMove_GameEndedSetsTurnSeatNilStatusFinishedAndWinner(t *testing.T) {
	s := newRecordMoveTestStore(t)
	ctx := t.Context()

	game, _, err := s.CreateGame(ctx, "ROOM06", "demo-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}

	in := store.MoveInput{
		Seat:                1,
		Color:               "black",
		From:                "0,0",
		To:                  "0,1",
		ResultingBoardState: `{"positions":["0,1"],"finished":true}`,
		NextTurnSeat:        2, // must be ignored since GameEnded is true
		GameEnded:           true,
		WinnerSeat:          intPtr(1),
	}

	if _, err := s.RecordMove(ctx, game.ID, in); err != nil {
		t.Fatalf("RecordMove returned error: %v, want nil", err)
	}

	got, err := s.GetGame(ctx, "ROOM06")
	if err != nil {
		t.Fatalf("GetGame(ROOM06) returned error: %v, want nil", err)
	}
	if got.TurnSeat != nil {
		t.Errorf("after game-ending RecordMove, GetGame.TurnSeat = %v, want nil", got.TurnSeat)
	}
	if got.Status != "finished" {
		t.Errorf("after game-ending RecordMove, GetGame.Status = %q, want %q", got.Status, "finished")
	}
	if got.WinnerSeat == nil || *got.WinnerSeat != 1 {
		t.Errorf("after game-ending RecordMove, GetGame.WinnerSeat = %v, want pointer to 1", got.WinnerSeat)
	}
	if got.MoveSeq != 1 {
		t.Errorf("after game-ending RecordMove, GetGame.MoveSeq = %d, want 1", got.MoveSeq)
	}
	if got.BoardState != in.ResultingBoardState {
		t.Errorf("after game-ending RecordMove, GetGame.BoardState = %q, want %q", got.BoardState, in.ResultingBoardState)
	}
}

// -----------------------------------------------------------------------
// 6: unknown game -- RecordMove on a gameID that was never created returns
// an error satisfying errors.Is(err, store.ErrNotFound), and inserts
// nothing into moves.
// -----------------------------------------------------------------------

func TestRecordMove_UnknownGameIDReturnsErrNotFoundAndInsertsNothing(t *testing.T) {
	s := newRecordMoveTestStore(t)
	ctx := t.Context()

	// Seed one legitimate game/move so we can confirm the failed attempt
	// against a different, non-existent id doesn't disturb the overall
	// moves table, nor create any row for the bogus id.
	game, _, err := s.CreateGame(ctx, "ROOM07", "demo-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}
	if _, err := s.RecordMove(ctx, game.ID, store.MoveInput{
		Seat: 1, Color: "black", From: "0,0", To: "0,1",
		ResultingBoardState: `{"positions":["0,1"]}`, NextTurnSeat: 2,
	}); err != nil {
		t.Fatalf("seeding RecordMove returned error: %v, want nil", err)
	}

	totalBefore := rmCountRows(t, s.DB(), `SELECT COUNT(*) FROM moves`)

	const nonExistentGameID = int64(999999999)
	_, err = s.RecordMove(ctx, nonExistentGameID, store.MoveInput{
		Seat: 1, Color: "black", From: "0,0", To: "0,1",
		ResultingBoardState: `{"positions":["0,1"]}`, NextTurnSeat: 2,
	})
	if err == nil {
		t.Fatal("RecordMove against a non-existent game_id returned nil error, want ErrNotFound")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("RecordMove against a non-existent game_id returned error %v, want errors.Is(err, store.ErrNotFound) to be true", err)
	}

	bogusCount := rmCountRows(t, s.DB(), `SELECT COUNT(*) FROM moves WHERE game_id = ?`, nonExistentGameID)
	if bogusCount != 0 {
		t.Errorf("moves row count for the non-existent game_id=%d = %d, want 0", nonExistentGameID, bogusCount)
	}

	totalAfter := rmCountRows(t, s.DB(), `SELECT COUNT(*) FROM moves`)
	if totalAfter != totalBefore {
		t.Errorf("total moves row count changed from %d to %d after a failed RecordMove, want unchanged", totalBefore, totalAfter)
	}
}

// -----------------------------------------------------------------------
// 7: independent seq numbering per game -- two separate games each get
// their own RecordMove call, both showing Move.Seq == 1 in their own game.
// -----------------------------------------------------------------------

func TestRecordMove_SeqNumberingIsPerGameNotGlobal(t *testing.T) {
	s := newRecordMoveTestStore(t)
	ctx := t.Context()

	gameA, _, err := s.CreateGame(ctx, "SEQA", "demo-freeplay", `{"positions":[]}`, "Dave", "black")
	if err != nil {
		t.Fatalf("CreateGame(SEQA) returned error: %v, want nil", err)
	}
	gameB, _, err := s.CreateGame(ctx, "SEQB", "demo-freeplay", `{"positions":[]}`, "Erin", "white")
	if err != nil {
		t.Fatalf("CreateGame(SEQB) returned error: %v, want nil", err)
	}

	// Give gameA a first move so its move_seq is already at 1 before gameB
	// ever gets one -- if seq numbering were (incorrectly) global/shared,
	// gameB's first move would come back as seq 2, not 1.
	if _, err := s.RecordMove(ctx, gameA.ID, store.MoveInput{
		Seat: 1, Color: "black", From: "0,0", To: "0,1",
		ResultingBoardState: `{"positions":["0,1"]}`, NextTurnSeat: 2,
	}); err != nil {
		t.Fatalf("RecordMove(gameA) returned error: %v, want nil", err)
	}

	moveB, err := s.RecordMove(ctx, gameB.ID, store.MoveInput{
		Seat: 1, Color: "black", From: "7,7", To: "7,6",
		ResultingBoardState: `{"positions":["7,6"]}`, NextTurnSeat: 2,
	})
	if err != nil {
		t.Fatalf("RecordMove(gameB) returned error: %v, want nil", err)
	}

	if moveB.Seq != 1 {
		t.Errorf("gameB's first move.Seq = %d, want 1 (seq numbering must be per-game, not a shared global counter)", moveB.Seq)
	}
	if moveB.GameID != gameB.ID {
		t.Errorf("moveB.GameID = %d, want %d (gameB.ID)", moveB.GameID, gameB.ID)
	}
}
