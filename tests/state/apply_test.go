// Tests of applying a move to a position: legal moves and captures, turn order, and the rejection of illegal moves.

package state_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/callback"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// --- fixture construction ------------------------------------------------------

// fixtureDefJSON builds a small 3x3 orthogonal4 grid, two-seat (1=black,
// 2=white), one-piece ("stone", step primitive, capture-or-empty occupancy)
// game definition, with turn_order.type and win_condition.type parameterized
// so individual tests can exercise ApplyMove's step-10/step-11 error paths
// without disturbing the otherwise-identical baseline shape.
func fixtureDefJSON(id, turnOrderType, winConditionType string) string {
	return fmt.Sprintf(`{
  "id": %q,
  "name": "Apply Move Fixture",
  "board": {
    "generator": { "type": "grid", "width": 3, "height": 3, "adjacency": "orthogonal4" }
  },
  "seats": [
    { "seat": 1, "color": "black" },
    { "seat": 2, "color": "white" }
  ],
  "pieces": {
    "stone": { "movement": { "primitive": "step", "occupancy": "capture-or-empty" } }
  },
  "turn_order": { "type": %q, "seats": [1, 2] },
  "win_condition": { "type": %q }
}`, id, turnOrderType, winConditionType)
}

// buildGame registers fixtureDefJSON(id, turnOrderType, winConditionType)
// under a fresh *catalog.Catalog (so each test gets an isolated
// *catalog.CompiledGame, distinguishable by id) with the given callbacks (nil
// is valid — catalog substitutes NoopCallbacks), and returns the compiled
// game.
func buildGame(t *testing.T, id, turnOrderType, winConditionType string, cb callback.GameCallbacks) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.Register(id, []byte(fixtureDefJSON(id, turnOrderType, winConditionType)), cb); err != nil {
		t.Fatalf("fixture setup: Register(%q) error = %v, want nil", id, err)
	}
	game, ok := cat.Get(id)
	if !ok || game == nil {
		t.Fatalf("fixture setup: Get(%q) = (%v, %v), want (non-nil, true)", id, game, ok)
	}
	return game
}

// baselineGame builds the standard "round-robin" / "none" fixture with no
// callbacks (NoopCallbacks), the shape almost every ApplyMove test needs.
func baselineGame(t *testing.T, id string) *catalog.CompiledGame {
	t.Helper()
	return buildGame(t, id, "round-robin", "none", nil)
}

// --- fake callbacks for the callback-gate tests --------------------------------

var errValidateSentinel = errors.New("fakeCallbacks: sentinel ValidateMove rejection")
var errOnAppliedSentinel = errors.New("fakeCallbacks: sentinel OnMoveApplied rejection")

// fakeCallbacks is a configurable callback.GameCallbacks test double: each
// hook returns/does whatever the test wired up, defaulting to NoopCallbacks-
// equivalent behavior (nil error, no winner) for any hook left unset.
type fakeCallbacks struct {
	validateErr  error
	onAppliedErr error
	checkWinner  func(st callback.BoardView) (*int, bool)
}

func (f fakeCallbacks) ValidateMove(ctx callback.MoveContext) error {
	return f.validateErr
}

func (f fakeCallbacks) OnMoveApplied(ctx callback.MoveContext) error {
	return f.onAppliedErr
}

func (f fakeCallbacks) CheckWinner(st callback.BoardView) (*int, bool) {
	if f.checkWinner != nil {
		return f.checkWinner(st)
	}
	return nil, false
}

var _ callback.GameCallbacks = fakeCallbacks{}

// --- shared assertion helper ----------------------------------------------------

// expectRejectedUnchanged asserts the backlog item 08 AC's illegal-move half:
// a non-nil error, a nil *state.MoveResult, and st left byte-for-byte
// unchanged (compared via MarshalJSON taken before the ApplyMove call, which
// the caller must have captured beforehand).
func expectRejectedUnchanged(t *testing.T, st *state.BoardState, before []byte, result *state.MoveResult, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("ApplyMove() error = nil, want non-nil")
	}
	if result != nil {
		t.Errorf("ApplyMove() result = %+v, want nil", result)
	}
	after, marshalErr := st.MarshalJSON()
	if marshalErr != nil {
		t.Fatalf("st.MarshalJSON() after ApplyMove error = %v, want nil", marshalErr)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("original state changed after rejected ApplyMove:\n before = %s\n after  = %s", before, after)
	}
}

// --- 1. Legal move, empty destination -------------------------------------------

func TestApplyMove_LegalMove_EmptyDestination(t *testing.T) {
	game := baselineGame(t, "am-legal-empty")

	st := state.NewBoardState()
	st.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	result, err := state.ApplyMove(game, st, 1, "0,0", "1,0")
	if err != nil {
		t.Fatalf("ApplyMove() error = %v, want nil", err)
	}
	if result == nil {
		t.Fatalf("ApplyMove() result = nil, want non-nil")
	}

	if result.NewState == st {
		t.Errorf("result.NewState is the same object as the input st, want a distinct clone")
	}

	if _, ok := result.NewState.Get("0,0"); ok {
		t.Errorf("NewState.Get(0,0) ok = true, want false (piece moved away)")
	}
	got, ok := result.NewState.Get("1,0")
	if !ok {
		t.Fatalf("NewState.Get(1,0) ok = false, want true")
	}
	want := state.Occupant{Piece: "stone", Color: "black"}
	if got != want {
		t.Errorf("NewState.Get(1,0) = %+v, want %+v", got, want)
	}

	if result.Captured != nil {
		t.Errorf("result.Captured = %v, want nil", *result.Captured)
	}
	if result.NextTurnSeat != 2 {
		t.Errorf("result.NextTurnSeat = %d, want 2", result.NextTurnSeat)
	}
	if result.GameEnded {
		t.Errorf("result.GameEnded = true, want false")
	}
	if result.WinnerSeat != nil {
		t.Errorf("result.WinnerSeat = %v, want nil", *result.WinnerSeat)
	}

	// Original st must be untouched.
	orig, ok := st.Get("0,0")
	if !ok || orig != want {
		t.Errorf("original st.Get(0,0) = (%+v, %v), want (%+v, true) — original must be unmutated", orig, ok, want)
	}
	if _, ok := st.Get("1,0"); ok {
		t.Errorf("original st.Get(1,0) ok = true, want false — original must be unmutated")
	}
}

// --- 2. Legal move, capture ------------------------------------------------------

func TestApplyMove_LegalMove_Capture(t *testing.T) {
	game := baselineGame(t, "am-legal-capture")

	st := state.NewBoardState()
	st.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})
	st.Set("1,0", state.Occupant{Piece: "stone", Color: "white"})

	result, err := state.ApplyMove(game, st, 1, "0,0", "1,0")
	if err != nil {
		t.Fatalf("ApplyMove() error = %v, want nil", err)
	}
	if result == nil {
		t.Fatalf("ApplyMove() result = nil, want non-nil")
	}

	if _, ok := result.NewState.Get("0,0"); ok {
		t.Errorf("NewState.Get(0,0) ok = true, want false")
	}
	got, ok := result.NewState.Get("1,0")
	if !ok {
		t.Fatalf("NewState.Get(1,0) ok = false, want true")
	}
	want := state.Occupant{Piece: "stone", Color: "black"}
	if got != want {
		t.Errorf("NewState.Get(1,0) = %+v, want %+v (white captured, black present)", got, want)
	}

	if result.Captured == nil {
		t.Fatalf("result.Captured = nil, want non-nil (pointing at 1,0)")
	}
	if *result.Captured != board.PositionID("1,0") {
		t.Errorf("*result.Captured = %q, want %q", *result.Captured, "1,0")
	}
	if result.NextTurnSeat != 2 {
		t.Errorf("result.NextTurnSeat = %d, want 2", result.NextTurnSeat)
	}
}

// --- 3. Turn order wrap-around ----------------------------------------------------

func TestApplyMove_TurnOrder_WrapsAround(t *testing.T) {
	game := baselineGame(t, "am-turn-wrap")

	st := state.NewBoardState()
	st.Set("1,0", state.Occupant{Piece: "stone", Color: "white"})

	result, err := state.ApplyMove(game, st, 2, "1,0", "2,0")
	if err != nil {
		t.Fatalf("ApplyMove() error = %v, want nil", err)
	}
	if result == nil {
		t.Fatalf("ApplyMove() result = nil, want non-nil")
	}
	if result.NextTurnSeat != 1 {
		t.Errorf("result.NextTurnSeat = %d, want 1 (wrap-around from seat 2 back to seat 1)", result.NextTurnSeat)
	}
}

// --- 4. Illegal — wrong seat's piece -----------------------------------------------

func TestApplyMove_Illegal_WrongSeatsPiece(t *testing.T) {
	game := baselineGame(t, "am-illegal-wrong-seat")

	st := state.NewBoardState()
	st.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	before, err := st.MarshalJSON()
	if err != nil {
		t.Fatalf("st.MarshalJSON() error = %v, want nil", err)
	}

	result, applyErr := state.ApplyMove(game, st, 2, "0,0", "1,0")
	expectRejectedUnchanged(t, st, before, result, applyErr)
}

// --- 5. Illegal — no piece at from -------------------------------------------------

func TestApplyMove_Illegal_NoPieceAtFrom(t *testing.T) {
	game := baselineGame(t, "am-illegal-no-piece")

	st := state.NewBoardState() // empty board

	before, err := st.MarshalJSON()
	if err != nil {
		t.Fatalf("st.MarshalJSON() error = %v, want nil", err)
	}

	result, applyErr := state.ApplyMove(game, st, 1, "0,0", "1,0")
	expectRejectedUnchanged(t, st, before, result, applyErr)
}

// --- 6. Illegal — same-color destination -------------------------------------------

func TestApplyMove_Illegal_SameColorDestination(t *testing.T) {
	game := baselineGame(t, "am-illegal-same-color")

	st := state.NewBoardState()
	st.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})
	st.Set("1,0", state.Occupant{Piece: "stone", Color: "black"})

	before, err := st.MarshalJSON()
	if err != nil {
		t.Fatalf("st.MarshalJSON() error = %v, want nil", err)
	}

	result, applyErr := state.ApplyMove(game, st, 1, "0,0", "1,0")
	expectRejectedUnchanged(t, st, before, result, applyErr)
}

// --- 7. Illegal — destination not adjacent -----------------------------------------

func TestApplyMove_Illegal_DestinationNotAdjacent(t *testing.T) {
	game := baselineGame(t, "am-illegal-not-adjacent")

	st := state.NewBoardState()
	st.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	before, err := st.MarshalJSON()
	if err != nil {
		t.Fatalf("st.MarshalJSON() error = %v, want nil", err)
	}

	result, applyErr := state.ApplyMove(game, st, 1, "0,0", "2,2")
	expectRejectedUnchanged(t, st, before, result, applyErr)
}

// --- 8. Illegal — `to` not a real board position at all ---------------------------

func TestApplyMove_Illegal_DestinationNotOnBoard(t *testing.T) {
	game := baselineGame(t, "am-illegal-off-board")

	st := state.NewBoardState()
	st.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	before, err := st.MarshalJSON()
	if err != nil {
		t.Fatalf("st.MarshalJSON() error = %v, want nil", err)
	}

	result, applyErr := state.ApplyMove(game, st, 1, "0,0", "99,99")
	expectRejectedUnchanged(t, st, before, result, applyErr)
}

// --- 9. Illegal — unknown seat --------------------------------------------------------

func TestApplyMove_Illegal_UnknownSeat(t *testing.T) {
	game := baselineGame(t, "am-illegal-unknown-seat")

	st := state.NewBoardState()
	st.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	before, err := st.MarshalJSON()
	if err != nil {
		t.Fatalf("st.MarshalJSON() error = %v, want nil", err)
	}

	result, applyErr := state.ApplyMove(game, st, 3, "0,0", "1,0")
	expectRejectedUnchanged(t, st, before, result, applyErr)
}

// --- 10. GameCallbacks.ValidateMove rejects -------------------------------------------

func TestApplyMove_CallbackValidateMoveRejects(t *testing.T) {
	cb := fakeCallbacks{validateErr: errValidateSentinel}
	game := buildGame(t, "am-callback-validate-rejects", "round-robin", "none", cb)

	st := state.NewBoardState()
	st.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	before, err := st.MarshalJSON()
	if err != nil {
		t.Fatalf("st.MarshalJSON() error = %v, want nil", err)
	}

	// Otherwise fully legal: adjacent, empty destination.
	result, applyErr := state.ApplyMove(game, st, 1, "0,0", "1,0")

	if !errors.Is(applyErr, errValidateSentinel) {
		t.Errorf("ApplyMove() error = %v, want errors.Is(..., errValidateSentinel)", applyErr)
	}
	expectRejectedUnchanged(t, st, before, result, applyErr)
}

// --- 11. GameCallbacks.OnMoveApplied errors -------------------------------------------

func TestApplyMove_CallbackOnMoveAppliedErrors(t *testing.T) {
	cb := fakeCallbacks{onAppliedErr: errOnAppliedSentinel}
	game := buildGame(t, "am-callback-onapplied-errors", "round-robin", "none", cb)

	st := state.NewBoardState()
	st.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	before, err := st.MarshalJSON()
	if err != nil {
		t.Fatalf("st.MarshalJSON() error = %v, want nil", err)
	}

	// Otherwise fully legal: primitive allows it, ValidateMove passes (nil),
	// the move is mechanically applied to an internal newState — but
	// OnMoveApplied must still cause the whole thing to be discarded.
	result, applyErr := state.ApplyMove(game, st, 1, "0,0", "1,0")

	if !errors.Is(applyErr, errOnAppliedSentinel) {
		t.Errorf("ApplyMove() error = %v, want errors.Is(..., errOnAppliedSentinel)", applyErr)
	}
	if result != nil {
		t.Errorf("ApplyMove() result = %+v, want nil (newState must be discarded entirely)", result)
	}
	expectRejectedUnchanged(t, st, before, result, applyErr)
}

// --- 12. win_condition: "callback" ----------------------------------------------------

func TestApplyMove_WinConditionCallback_ReportsWinner(t *testing.T) {
	seatOne := 1
	cb := fakeCallbacks{
		checkWinner: func(st callback.BoardView) (*int, bool) {
			return &seatOne, true
		},
	}
	game := buildGame(t, "am-win-callback", "round-robin", "callback", cb)

	st := state.NewBoardState()
	st.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	result, err := state.ApplyMove(game, st, 1, "0,0", "1,0")
	if err != nil {
		t.Fatalf("ApplyMove() error = %v, want nil", err)
	}
	if result == nil {
		t.Fatalf("ApplyMove() result = nil, want non-nil")
	}
	if !result.GameEnded {
		t.Errorf("result.GameEnded = false, want true")
	}
	if result.WinnerSeat == nil {
		t.Fatalf("result.WinnerSeat = nil, want non-nil (pointing at seat 1)")
	}
	if *result.WinnerSeat != 1 {
		t.Errorf("*result.WinnerSeat = %d, want 1", *result.WinnerSeat)
	}
}

// --- 13. Unsupported turn_order.type ---------------------------------------------------

func TestApplyMove_UnsupportedTurnOrderType_Errors(t *testing.T) {
	game := buildGame(t, "am-unsupported-turn-order", "simultaneous", "none", nil)

	st := state.NewBoardState()
	st.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	before, err := st.MarshalJSON()
	if err != nil {
		t.Fatalf("st.MarshalJSON() error = %v, want nil", err)
	}

	// Otherwise fully legal: passes primitive and ValidateMove; only the
	// turn-order computation step should reject it.
	result, applyErr := state.ApplyMove(game, st, 1, "0,0", "1,0")
	expectRejectedUnchanged(t, st, before, result, applyErr)
}

// --- 14. Unsupported win_condition.type -------------------------------------------------

func TestApplyMove_UnsupportedWinConditionType_Errors(t *testing.T) {
	game := buildGame(t, "am-unsupported-win-condition", "round-robin", "score-based", nil)

	st := state.NewBoardState()
	st.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	before, err := st.MarshalJSON()
	if err != nil {
		t.Fatalf("st.MarshalJSON() error = %v, want nil", err)
	}

	// Otherwise fully legal: passes primitive, ValidateMove, and turn order;
	// only the win-condition computation step should reject it.
	result, applyErr := state.ApplyMove(game, st, 1, "0,0", "1,0")
	expectRejectedUnchanged(t, st, before, result, applyErr)
}
