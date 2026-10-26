// Package callback defines the Go-callback escape hatch described in
// docs/history/plan.md section 6.3: a small GameCallbacks interface a game module
// may implement to hook move validation, post-move effects, and win
// detection beyond what the DSL's declarative rules express, plus
// NoopCallbacks, the default implementation the demo game uses.
//
// callback depends only on board.PositionID and the small BoardView
// interface defined here — never on internal/engine/state or
// internal/engine/catalog directly — to avoid an import cycle (state depends
// on catalog, catalog depends on callback to hold a compiled game's
// GameCallbacks; see docs/history/plan.md section 6.3's correction).
package callback

import "github.com/noderunner/bingle/internal/engine/board"

// BoardView is the read/write board access a callback needs — decoupled
// from state.BoardState so this package has no dependency on state or
// catalog (avoiding an import cycle: callback -> state -> catalog ->
// callback). internal/engine/state.BoardState implements this.
type BoardView interface {
	Occupant(id board.PositionID) (color string, occupied bool)
	SetOccupant(id board.PositionID, color string)
	RemoveOccupant(id board.PositionID)
}

// MoveContext carries the information a GameCallbacks method needs about a
// single move: whose move it is, on which board, and its endpoints.
//
// For richer move kinds (placement, stack moves) the extra fields describe
// the move in full; From/To always hold the move's origin ("" for a
// placement) and final destination.
type MoveContext struct {
	GameID string
	Board  BoardView
	Seat   int
	Color  string
	From   board.PositionID
	To     board.PositionID

	// Action is the move's action ID ("step", "place", "move", ...).
	Action string
	// Piece is the piece type being placed (placement moves).
	Piece string
	// Count, Path and Drops describe a stack move: how many pieces were
	// picked up, which squares they were dropped on, and how many on each.
	Count int
	Path  []board.PositionID
	Drops []int
}

// GameCallbacks is the Go-callback escape hatch a game module may implement
// to extend the DSL's declarative rules.
type GameCallbacks interface {
	// ValidateMove returns a non-nil error to reject a move the DSL rules
	// would otherwise allow.
	ValidateMove(ctx MoveContext) error
	// OnMoveApplied is called after a move is applied to the in-memory
	// state, before persistence; it may mutate ctx.Board further (e.g.
	// multi-capture chains a future game needs).
	OnMoveApplied(ctx MoveContext) error
	// CheckWinner is only consulted when the DSL's win_condition.type ==
	// "callback".
	CheckWinner(st BoardView) (winnerSeat *int, ended bool)
}

// NoopCallbacks is the default GameCallbacks implementation: every hook is a
// no-op, and CheckWinner never declares a winner. The demo game (which has
// win_condition.type == "none") uses this.
type NoopCallbacks struct{}

// ValidateMove never rejects a move.
func (NoopCallbacks) ValidateMove(ctx MoveContext) error {
	return nil
}

// OnMoveApplied has no post-move effect.
func (NoopCallbacks) OnMoveApplied(ctx MoveContext) error {
	return nil
}

// CheckWinner never declares a winner.
func (NoopCallbacks) CheckWinner(st BoardView) (winnerSeat *int, ended bool) {
	return nil, false
}
