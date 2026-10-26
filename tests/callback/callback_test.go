// Tests of the callback package: NoopCallbacks accepts every move, declares no winner, and satisfies the whole GameCallbacks interface.

package callback_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/callback"
)

// fakeBoard is a minimal test-only implementation of callback.BoardView,
// defined here (rather than reusing internal/engine/state, which depends on
// this package). It is a trivial map-backed board:
// good enough to construct varied callback.MoveContext.Board values and
// varied callback.BoardView states for CheckWinner, without asserting
// anything about the fake's own behavior.
type fakeBoard map[board.PositionID]string

func (b fakeBoard) Occupant(id board.PositionID) (string, bool) {
	color, ok := b[id]
	return color, ok
}

func (b fakeBoard) SetOccupant(id board.PositionID, color string) {
	b[id] = color
}

func (b fakeBoard) RemoveOccupant(id board.PositionID) {
	delete(b, id)
}

// --- compile-time interface satisfaction -----------------------------------

// var _ callback.GameCallbacks = callback.NoopCallbacks{} is itself a
// meaningful assertion: it fails to compile if NoopCallbacks is missing any
// GameCallbacks method or has a mismatched signature for one.
var _ callback.GameCallbacks = callback.NoopCallbacks{}

// var _ callback.BoardView = fakeBoard{} pins down that our test fake stays
// a valid stand-in for BoardView as the interface evolves.
var _ callback.BoardView = fakeBoard{}

// --- NoopCallbacks.ValidateMove ---------------------------------------------

func TestNoopCallbacks_ValidateMove_ReturnsNil_EmptyContext(t *testing.T) {
	var noop callback.NoopCallbacks

	err := noop.ValidateMove(callback.MoveContext{})
	if err != nil {
		t.Errorf("ValidateMove(zero-value MoveContext) = %v, want nil", err)
	}
}

func TestNoopCallbacks_ValidateMove_ReturnsNil_VariedContexts(t *testing.T) {
	var noop callback.NoopCallbacks

	cases := []struct {
		name string
		ctx  callback.MoveContext
	}{
		{
			name: "seat 1, black, adjacent grid move, empty board",
			ctx: callback.MoveContext{
				GameID: "game-1",
				Board:  fakeBoard{},
				Seat:   1,
				Color:  "black",
				From:   board.PositionID("0,0"),
				To:     board.PositionID("0,1"),
			},
		},
		{
			name: "seat 2, white, board with occupants, capture-like move",
			ctx: callback.MoveContext{
				GameID: "game-2",
				Board: fakeBoard{
					board.PositionID("3,3"): "white",
					board.PositionID("3,4"): "black",
				},
				Seat:  2,
				Color: "white",
				From:  board.PositionID("3,3"),
				To:    board.PositionID("3,4"),
			},
		},
		{
			name: "unusual seat number and non-grid-shaped position IDs",
			ctx: callback.MoveContext{
				GameID: "",
				Board:  fakeBoard{},
				Seat:   -7,
				Color:  "",
				From:   board.PositionID("hex-a1"),
				To:     board.PositionID("hex-a1"), // From == To, still just a value NoopCallbacks must not reject
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := noop.ValidateMove(tc.ctx)
			if err != nil {
				t.Errorf("ValidateMove(%+v) = %v, want nil", tc.ctx, err)
			}
		})
	}
}

// --- NoopCallbacks.OnMoveApplied --------------------------------------------

func TestNoopCallbacks_OnMoveApplied_ReturnsNil_EmptyContext(t *testing.T) {
	var noop callback.NoopCallbacks

	err := noop.OnMoveApplied(callback.MoveContext{})
	if err != nil {
		t.Errorf("OnMoveApplied(zero-value MoveContext) = %v, want nil", err)
	}
}

func TestNoopCallbacks_OnMoveApplied_ReturnsNil_VariedContexts(t *testing.T) {
	var noop callback.NoopCallbacks

	cases := []struct {
		name string
		ctx  callback.MoveContext
	}{
		{
			name: "seat 1, black, adjacent grid move, empty board",
			ctx: callback.MoveContext{
				GameID: "game-1",
				Board:  fakeBoard{},
				Seat:   1,
				Color:  "black",
				From:   board.PositionID("0,0"),
				To:     board.PositionID("0,1"),
			},
		},
		{
			name: "seat 2, white, board with occupants after a capture",
			ctx: callback.MoveContext{
				GameID: "game-2",
				Board: fakeBoard{
					board.PositionID("3,4"): "white", // mover's piece now occupies the captured square
				},
				Seat:  2,
				Color: "white",
				From:  board.PositionID("3,3"),
				To:    board.PositionID("3,4"),
			},
		},
		{
			name: "nil-ish zero seat and empty color still accepted",
			ctx: callback.MoveContext{
				GameID: "game-3",
				Board:  fakeBoard{},
				Seat:   0,
				Color:  "",
				From:   board.PositionID(""),
				To:     board.PositionID(""),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := noop.OnMoveApplied(tc.ctx)
			if err != nil {
				t.Errorf("OnMoveApplied(%+v) = %v, want nil", tc.ctx, err)
			}
		})
	}
}

// --- NoopCallbacks.CheckWinner -----------------------------------------------

func TestNoopCallbacks_CheckWinner_EmptyBoard_ReturnsNilFalse(t *testing.T) {
	var noop callback.NoopCallbacks

	winnerSeat, ended := noop.CheckWinner(fakeBoard{})
	if winnerSeat != nil {
		t.Errorf("CheckWinner(empty board) winnerSeat = %v, want nil", *winnerSeat)
	}
	if ended {
		t.Errorf("CheckWinner(empty board) ended = true, want false")
	}
}

func TestNoopCallbacks_CheckWinner_OccupiedBoard_ReturnsNilFalse(t *testing.T) {
	var noop callback.NoopCallbacks

	st := fakeBoard{
		board.PositionID("0,0"): "black",
		board.PositionID("8,8"): "white",
		board.PositionID("4,4"): "black",
	}

	winnerSeat, ended := noop.CheckWinner(st)
	if winnerSeat != nil {
		t.Errorf("CheckWinner(occupied board) winnerSeat = %v, want nil", *winnerSeat)
	}
	if ended {
		t.Errorf("CheckWinner(occupied board) ended = true, want false")
	}
}

func TestNoopCallbacks_CheckWinner_MultipleCallsAreConsistent(t *testing.T) {
	// A no-op default must be stateless: calling it repeatedly with different
	// board shapes should never start returning ended=true or a non-nil seat.
	var noop callback.NoopCallbacks

	boards := []callback.BoardView{
		fakeBoard{},
		fakeBoard{board.PositionID("1,1"): "black"},
		fakeBoard{
			board.PositionID("1,1"): "black",
			board.PositionID("2,2"): "white",
			board.PositionID("3,3"): "white",
		},
	}

	for i, st := range boards {
		winnerSeat, ended := noop.CheckWinner(st)
		if winnerSeat != nil || ended {
			t.Errorf("call %d: CheckWinner(%+v) = (%v, %v), want (nil, false)", i, st, winnerSeat, ended)
		}
	}
}
