// Tests of reading the position after a given move.

package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/store"
)

// newGetStateAtSeqTestStore opens a *store.Store against a fresh file in a
// per-test temp directory and registers a cleanup to close it. Mirrors
// newRecordMoveTestStore/newCreateGameTestStore (kept separate per
// instructions not to edit those files).
func newGetStateAtSeqTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open(%q) returned error: %v, want nil", dbPath, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// -----------------------------------------------------------------------
// 1: the AC itself -- two recorded moves, GetStateAtSeq(1) and
// GetStateAtSeq(2) return exactly the resulting_state stored for each.
// -----------------------------------------------------------------------

func TestGetStateAtSeq_ReturnsResultingStateForEachRecordedSeq(t *testing.T) {
	s := newGetStateAtSeqTestStore(t)
	ctx := t.Context()

	game, _, err := s.CreateGame(ctx, "GSA01", "test-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}

	if _, err := s.RecordMove(ctx, game.ID, store.MoveInput{
		Seat: 1, Color: "black", From: "0,0", To: "0,1",
		ResultingBoardState: "board-state-1", NextTurnSeat: 2,
	}); err != nil {
		t.Fatalf("first RecordMove returned error: %v, want nil", err)
	}
	if _, err := s.RecordMove(ctx, game.ID, store.MoveInput{
		Seat: 2, Color: "white", From: "7,7", To: "7,6",
		ResultingBoardState: "board-state-2", NextTurnSeat: 1,
	}); err != nil {
		t.Fatalf("second RecordMove returned error: %v, want nil", err)
	}

	got1, err := s.GetStateAtSeq(ctx, game.ID, 1)
	if err != nil {
		t.Fatalf("GetStateAtSeq(gameID, 1) returned error: %v, want nil", err)
	}
	if got1 != "board-state-1" {
		t.Errorf("GetStateAtSeq(gameID, 1) = %q, want %q", got1, "board-state-1")
	}

	got2, err := s.GetStateAtSeq(ctx, game.ID, 2)
	if err != nil {
		t.Fatalf("GetStateAtSeq(gameID, 2) returned error: %v, want nil", err)
	}
	if got2 != "board-state-2" {
		t.Errorf("GetStateAtSeq(gameID, 2) = %q, want %q", got2, "board-state-2")
	}
}

// -----------------------------------------------------------------------
// 2: a seq higher than any recorded move returns store.ErrNotFound and an
// empty string.
// -----------------------------------------------------------------------

func TestGetStateAtSeq_SeqNeverRecordedReturnsErrNotFoundAndEmptyString(t *testing.T) {
	s := newGetStateAtSeqTestStore(t)
	ctx := t.Context()

	game, _, err := s.CreateGame(ctx, "GSA02", "test-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}
	if _, err := s.RecordMove(ctx, game.ID, store.MoveInput{
		Seat: 1, Color: "black", From: "0,0", To: "0,1",
		ResultingBoardState: "board-state-1", NextTurnSeat: 2,
	}); err != nil {
		t.Fatalf("RecordMove returned error: %v, want nil", err)
	}

	got, err := s.GetStateAtSeq(ctx, game.ID, 99)
	if err == nil {
		t.Fatal("GetStateAtSeq(gameID, 99) returned nil error, want store.ErrNotFound")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetStateAtSeq(gameID, 99) returned error %v, want errors.Is(err, store.ErrNotFound) to be true", err)
	}
	if got != "" {
		t.Errorf("GetStateAtSeq(gameID, 99) = %q, want empty string on error", got)
	}
}

// -----------------------------------------------------------------------
// 3: seq=0 is never a real moves row, even right after CreateGame with zero
// moves recorded -- must return store.ErrNotFound, not the game's initial
// board_state.
// -----------------------------------------------------------------------

func TestGetStateAtSeq_SeqZeroReturnsErrNotFoundEvenWithNoMovesRecorded(t *testing.T) {
	s := newGetStateAtSeqTestStore(t)
	ctx := t.Context()

	game, _, err := s.CreateGame(ctx, "GSA03", "test-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}
	if game.MoveSeq != 0 {
		t.Fatalf("precondition: game.MoveSeq = %d, want 0", game.MoveSeq)
	}

	got, err := s.GetStateAtSeq(ctx, game.ID, 0)
	if err == nil {
		t.Fatal("GetStateAtSeq(gameID, 0) returned nil error, want store.ErrNotFound")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetStateAtSeq(gameID, 0) returned error %v, want errors.Is(err, store.ErrNotFound) to be true", err)
	}
	if got != "" {
		t.Errorf("GetStateAtSeq(gameID, 0) = %q, want empty string on error", got)
	}
}

// Also verify seq=0 returns ErrNotFound even after moves exist for the game
// (seq=0 is still never a real moves row, regardless of how many real moves
// there are).
func TestGetStateAtSeq_SeqZeroReturnsErrNotFoundEvenAfterMovesRecorded(t *testing.T) {
	s := newGetStateAtSeqTestStore(t)
	ctx := t.Context()

	game, _, err := s.CreateGame(ctx, "GSA03B", "test-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}
	if _, err := s.RecordMove(ctx, game.ID, store.MoveInput{
		Seat: 1, Color: "black", From: "0,0", To: "0,1",
		ResultingBoardState: "board-state-1", NextTurnSeat: 2,
	}); err != nil {
		t.Fatalf("RecordMove returned error: %v, want nil", err)
	}

	got, err := s.GetStateAtSeq(ctx, game.ID, 0)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetStateAtSeq(gameID, 0) returned error %v, want errors.Is(err, store.ErrNotFound) to be true", err)
	}
	if got != "" {
		t.Errorf("GetStateAtSeq(gameID, 0) = %q, want empty string on error", got)
	}
}

// -----------------------------------------------------------------------
// 4: a gameID that was never created at all returns store.ErrNotFound.
// -----------------------------------------------------------------------

func TestGetStateAtSeq_UnknownGameIDReturnsErrNotFound(t *testing.T) {
	s := newGetStateAtSeqTestStore(t)
	ctx := t.Context()

	// Seed one legitimate game/move so we can confirm the lookup against a
	// different, non-existent id isn't accidentally satisfied by unrelated
	// rows.
	game, _, err := s.CreateGame(ctx, "GSA04", "test-freeplay", `{"positions":[]}`, "Alice", "black")
	if err != nil {
		t.Fatalf("CreateGame returned error: %v, want nil", err)
	}
	if _, err := s.RecordMove(ctx, game.ID, store.MoveInput{
		Seat: 1, Color: "black", From: "0,0", To: "0,1",
		ResultingBoardState: "board-state-1", NextTurnSeat: 2,
	}); err != nil {
		t.Fatalf("RecordMove returned error: %v, want nil", err)
	}

	const nonExistentGameID = int64(999999999)
	got, err := s.GetStateAtSeq(ctx, nonExistentGameID, 1)
	if err == nil {
		t.Fatal("GetStateAtSeq(nonExistentGameID, 1) returned nil error, want store.ErrNotFound")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetStateAtSeq(nonExistentGameID, 1) returned error %v, want errors.Is(err, store.ErrNotFound) to be true", err)
	}
	if got != "" {
		t.Errorf("GetStateAtSeq(nonExistentGameID, 1) = %q, want empty string on error", got)
	}
}

// -----------------------------------------------------------------------
// 5: cross-game isolation -- two games both have a seq=1 move with distinct
// resulting states; GetStateAtSeq for each game returns only its own state.
// -----------------------------------------------------------------------

func TestGetStateAtSeq_CrossGameIsolationForSameSeqNumber(t *testing.T) {
	s := newGetStateAtSeqTestStore(t)
	ctx := t.Context()

	gameA, _, err := s.CreateGame(ctx, "GSA05A", "test-freeplay", `{"positions":[]}`, "Dave", "black")
	if err != nil {
		t.Fatalf("CreateGame(gameA) returned error: %v, want nil", err)
	}
	gameB, _, err := s.CreateGame(ctx, "GSA05B", "test-freeplay", `{"positions":[]}`, "Erin", "white")
	if err != nil {
		t.Fatalf("CreateGame(gameB) returned error: %v, want nil", err)
	}

	if _, err := s.RecordMove(ctx, gameA.ID, store.MoveInput{
		Seat: 1, Color: "black", From: "0,0", To: "0,1",
		ResultingBoardState: "game-a-state", NextTurnSeat: 2,
	}); err != nil {
		t.Fatalf("RecordMove(gameA) returned error: %v, want nil", err)
	}
	if _, err := s.RecordMove(ctx, gameB.ID, store.MoveInput{
		Seat: 1, Color: "black", From: "7,7", To: "7,6",
		ResultingBoardState: "game-b-state", NextTurnSeat: 2,
	}); err != nil {
		t.Fatalf("RecordMove(gameB) returned error: %v, want nil", err)
	}

	gotA, err := s.GetStateAtSeq(ctx, gameA.ID, 1)
	if err != nil {
		t.Fatalf("GetStateAtSeq(gameA.ID, 1) returned error: %v, want nil", err)
	}
	if gotA != "game-a-state" {
		t.Errorf("GetStateAtSeq(gameA.ID, 1) = %q, want %q (must not leak gameB's state)", gotA, "game-a-state")
	}

	gotB, err := s.GetStateAtSeq(ctx, gameB.ID, 1)
	if err != nil {
		t.Fatalf("GetStateAtSeq(gameB.ID, 1) returned error: %v, want nil", err)
	}
	if gotB != "game-b-state" {
		t.Errorf("GetStateAtSeq(gameB.ID, 1) = %q, want %q (must not leak gameA's state)", gotB, "game-b-state")
	}
}
