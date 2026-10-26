// Tests of the reasons a move is refused: the game is not active, it is not the player's turn, or the move is illegal.

package session_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/callback"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// --- fixture: general rejection-path game (win_condition: none) -------------
//
// A 3x3 orthogonal4 grid, 2 seats (1=black, 2=white), one "stone" piece via
// "step"/"capture-or-empty", round-robin turn order [1, 2] — same style as
// make_move_test.go's fixture, deliberately re-declared here per this
// package's established self-containment convention.
//
// Layout (chosen so every rejection branch below has a hand-verified legal
// or illegal move to exercise):
//
//		         y=0    y=1    y=2
//		x=0:   black-A white-A  .
//		x=1:   black-B   .      .
//		x=2:     .       .      .
//
//	  - "0,0": black stone (seat 1) — the mover for the "your turn" tests.
//	  - "1,0": black stone (seat 1) — orthogonally adjacent to "0,0", so
//	    "0,0" -> "1,0" is a legal *shape* (adjacent) but illegal because both
//	    ends are black (destination occupied by the mover's own color).
//	  - "0,1": white stone (seat 2) — orthogonally adjacent to "0,0", giving
//	    seat 2 a piece with its own legal move ("0,1" -> "0,2") to attempt on
//	    the wrong turn.
//	  - "2,2": empty and NOT adjacent to "0,0" (illegal destination shape),
//	    also used as an empty "from" position (no piece at from).
//	  - "1,2": empty and adjacent to "2,2" (legal shape as a destination for
//	    the "no piece at from" test).
const rejectionFixtureGameDefID = "fixture-3x3-makemove-rejection"

const (
	rejFixtureBlackA       = "0,0" // seat 1 (black)'s stone whose turn it is
	rejFixtureBlackB       = "1,0" // seat 1 (black)'s second stone, adjacent to A
	rejFixtureWhiteA       = "0,1" // seat 2 (white)'s stone, adjacent to A
	rejFixtureWhiteDest    = "0,2" // empty, adjacent to WhiteA -- white's own legal move shape
	rejFixtureFarEmpty     = "2,2" // empty, NOT adjacent to A -- illegal destination / empty "from"
	rejFixtureFarEmptyDest = "1,2" // empty, adjacent to FarEmpty
)

const rejectionFixtureDefJSON = `{
  "id": "fixture-3x3-makemove-rejection",
  "name": "Fixture 3x3 (make-move rejection tests)",
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
  "setup": [
    { "position": "0,0", "piece": "stone", "color": "black" },
    { "position": "1,0", "piece": "stone", "color": "black" },
    { "position": "0,1", "piece": "stone", "color": "white" }
  ],
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "win_condition": { "type": "none" }
}`

// --- fixture: a game that ends on its very first move (win_condition:
// callback, backed by a fake callback.GameCallbacks whose CheckWinner always
// reports a winner) -- needed for the "ErrGameNotActive, status finished"
// case. Built the same way tests/state/apply_test.go builds its
// win-condition fixture: register via catalog.Register with a non-nil cb. --

const winFixtureGameDefID = "fixture-3x3-makemove-win"

const winFixtureDefJSON = `{
  "id": "fixture-3x3-makemove-win",
  "name": "Fixture 3x3 (make-move finished-status test)",
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
  "setup": [
    { "position": "0,0", "piece": "stone", "color": "black" },
    { "position": "1,0", "piece": "stone", "color": "white" }
  ],
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "win_condition": { "type": "callback" }
}`

// alwaysWinCallbacks is a callback.GameCallbacks test double whose
// CheckWinner always reports seat 1 as the winner and the game ended,
// regardless of board state -- enough to drive a game to "finished" on its
// first move, which is all this file needs from it. ValidateMove and
// OnMoveApplied are no-ops (never reject, never mutate further).
type alwaysWinCallbacks struct{}

func (alwaysWinCallbacks) ValidateMove(ctx callback.MoveContext) error  { return nil }
func (alwaysWinCallbacks) OnMoveApplied(ctx callback.MoveContext) error { return nil }
func (alwaysWinCallbacks) CheckWinner(st callback.BoardView) (*int, bool) {
	winner := 1
	return &winner, true
}

var _ callback.GameCallbacks = alwaysWinCallbacks{}

// newRejectionTestManager builds a fresh *catalog.Catalog with gameDefID
// registered against defJSON and cb, a fresh *store.Store against a
// temp-dir SQLite file, and returns a session.NewManager(st, cat) wired to
// both -- same pattern as make_move_test.go's newMakeMoveTestManager,
// reproduced locally per this package's self-containment convention, but
// parameterized so this one file can build both fixtures above.
func newRejectionTestManager(t *testing.T, gameDefID, defJSON string, cb callback.GameCallbacks) *session.Manager {
	t.Helper()

	cat := catalog.NewCatalog()
	if err := cat.Register(gameDefID, []byte(defJSON), cb); err != nil {
		t.Fatalf("fixture setup: catalog.Register(%q) error = %v, want nil", gameDefID, err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("fixture setup: store.Open(%q) error = %v, want nil", dbPath, err)
	}
	t.Cleanup(func() { _ = st.Close() })

	mgr := session.NewManager(st, cat)
	if mgr == nil {
		t.Fatalf("session.NewManager(...) = nil, want non-nil *Manager")
	}

	return mgr
}

// expectStateUnchanged asserts that after's MoveSeq and BoardState (the
// game's persisted state) are byte-for-byte/value-for-value identical to
// before's -- the "nothing was persisted" half of every rejection test in
// this file. Comparing GetState snapshots taken immediately before and
// after the rejected MakeMove call (rather than re-deriving expected
// values) matches tests/state/apply_test.go's expectRejectedUnchanged
// pattern, adapted to the session layer.
func expectStateUnchanged(t *testing.T, before, after session.StateSnapshot) {
	t.Helper()
	if after.MoveSeq != before.MoveSeq {
		t.Errorf("GetState() MoveSeq after rejected MakeMove = %d, want unchanged %d", after.MoveSeq, before.MoveSeq)
	}
	if after.BoardState != before.BoardState {
		t.Errorf("GetState() BoardState after rejected MakeMove changed:\nbefore = %s\nafter  = %s", before.BoardState, after.BoardState)
	}
	switch {
	case before.TurnSeat == nil && after.TurnSeat != nil:
		t.Errorf("GetState() TurnSeat after rejected MakeMove = %v, want nil (unchanged)", *after.TurnSeat)
	case before.TurnSeat != nil && after.TurnSeat == nil:
		t.Errorf("GetState() TurnSeat after rejected MakeMove = nil, want unchanged %d", *before.TurnSeat)
	case before.TurnSeat != nil && after.TurnSeat != nil && *before.TurnSeat != *after.TurnSeat:
		t.Errorf("GetState() TurnSeat after rejected MakeMove = %d, want unchanged %d", *after.TurnSeat, *before.TurnSeat)
	}
}

// --- 1. ErrGameNotActive, status "waiting" ----------------------------------
//
// CreateGame only (no second player has joined), so the game's status is
// still "waiting". MakeMove on this game (by the creator, any positions)
// must return an error satisfying errors.Is(err, session.ErrGameNotActive),
// and must leave persisted state (MoveSeq, BoardState) untouched.

func TestMakeMove_RejectsWhenGameStatusWaiting(t *testing.T) {
	mgr := newRejectionTestManager(t, rejectionFixtureGameDefID, rejectionFixtureDefJSON, nil)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, rejectionFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	before, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}
	if before.Status != "waiting" {
		t.Fatalf("GetState() Status = %q, want %q (precondition for this test)", before.Status, "waiting")
	}
	if before.MoveSeq != 0 {
		t.Fatalf("GetState() MoveSeq = %d, want 0 (precondition for this test)", before.MoveSeq)
	}

	_, err = mgr.MakeMove(ctx, created.RoomCode, "Alice",
		board.PositionID(rejFixtureBlackA), board.PositionID(rejFixtureBlackB))
	if err == nil {
		t.Fatalf("MakeMove() on a %q game error = nil, want non-nil", "waiting")
	}
	if !errors.Is(err, session.ErrGameNotActive) {
		t.Errorf("MakeMove() on a %q game error = %v, want errors.Is(err, session.ErrGameNotActive)", "waiting", err)
	}

	after, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}
	expectStateUnchanged(t, before, after)
}

// --- 2. ErrGameNotActive, status "finished" ---------------------------------
//
// A game whose win_condition is "callback" and whose GameCallbacks always
// reports a winner ends on its very first move (GameEnded: true, status
// becomes "finished"). A SUBSEQUENT MakeMove attempt on the same room must
// return errors.Is(err, session.ErrGameNotActive) and leave state unchanged.

func TestMakeMove_RejectsWhenGameStatusFinished(t *testing.T) {
	mgr := newRejectionTestManager(t, winFixtureGameDefID, winFixtureDefJSON, alwaysWinCallbacks{})
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, winFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}
	joined, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(%q, RolePlayer) error = %v, want nil", "Bob", err)
	}
	if joined.Status != "active" {
		t.Fatalf("JoinGame() Status = %q, want %q (precondition for this test)", joined.Status, "active")
	}

	// This move is legal (black "0,0" -> white "1,0", a capture) and, thanks
	// to alwaysWinCallbacks, ends the game.
	outcome, err := mgr.MakeMove(ctx, created.RoomCode, "Alice",
		board.PositionID("0,0"), board.PositionID("1,0"))
	if err != nil {
		t.Fatalf("MakeMove() [game-ending move] error = %v, want nil", err)
	}
	if !outcome.GameEnded {
		t.Fatalf("MakeMove() [game-ending move] GameEnded = false, want true (precondition for this test)")
	}
	if outcome.Status != "finished" {
		t.Fatalf("MakeMove() [game-ending move] Status = %q, want %q (precondition for this test)", outcome.Status, "finished")
	}

	before, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}
	if before.Status != "finished" {
		t.Fatalf("GetState() Status = %q, want %q (precondition for this test)", before.Status, "finished")
	}
	if before.MoveSeq != 1 {
		t.Fatalf("GetState() MoveSeq = %d, want 1 (precondition for this test)", before.MoveSeq)
	}

	// A subsequent move attempt on the now-finished game, by either seat.
	_, err = mgr.MakeMove(ctx, created.RoomCode, "Bob",
		board.PositionID("1,0"), board.PositionID("0,0"))
	if err == nil {
		t.Fatalf("MakeMove() on a %q game error = nil, want non-nil", "finished")
	}
	if !errors.Is(err, session.ErrGameNotActive) {
		t.Errorf("MakeMove() on a %q game error = %v, want errors.Is(err, session.ErrGameNotActive)", "finished", err)
	}

	after, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}
	expectStateUnchanged(t, before, after)
}

// --- 3. ErrNotYourTurn, wrong seat's turn -----------------------------------
//
// An active game where it's seat 1's turn (the default immediately after
// activation, per section 6.4c). MakeMove called with seat 2's player name,
// attempting an otherwise-legal move shape, must return
// errors.Is(err, session.ErrNotYourTurn) and leave MoveSeq/TurnSeat/
// BoardState unchanged.

func TestMakeMove_RejectsWrongSeatsTurn(t *testing.T) {
	mgr := newRejectionTestManager(t, rejectionFixtureGameDefID, rejectionFixtureDefJSON, nil)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, rejectionFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}
	joined, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(%q, RolePlayer) error = %v, want nil", "Bob", err)
	}
	if joined.Status != "active" {
		t.Fatalf("JoinGame() Status = %q, want %q (precondition for this test)", joined.Status, "active")
	}

	before, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}
	if before.TurnSeat == nil || *before.TurnSeat != 1 {
		t.Fatalf("GetState() TurnSeat = %v, want pointer to 1 (precondition for this test)", before.TurnSeat)
	}

	// Bob (seat 2) attempts his own, otherwise-legal move while it's still
	// seat 1's turn.
	_, err = mgr.MakeMove(ctx, created.RoomCode, "Bob",
		board.PositionID(rejFixtureWhiteA), board.PositionID(rejFixtureWhiteDest))
	if err == nil {
		t.Fatalf("MakeMove() by the wrong seat's name error = nil, want non-nil")
	}
	if !errors.Is(err, session.ErrNotYourTurn) {
		t.Errorf("MakeMove() by the wrong seat's name error = %v, want errors.Is(err, session.ErrNotYourTurn)", err)
	}

	after, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}
	expectStateUnchanged(t, before, after)
	if after.TurnSeat == nil || *after.TurnSeat != 1 {
		t.Errorf("GetState() TurnSeat after rejected MakeMove = %v, want still pointer to 1", after.TurnSeat)
	}
}

// --- 4. Illegal move: non-adjacent destination ------------------------------
//
// An active game, correct seat's turn, but `to` is not graph-adjacent to
// `from`. MakeMove must return a non-nil error (state.ApplyMove's own
// rejection, propagated as-is -- no specific sentinel asserted), and leave
// MoveSeq/BoardState unchanged.

func TestMakeMove_RejectsNonAdjacentDestination(t *testing.T) {
	mgr := newRejectionTestManager(t, rejectionFixtureGameDefID, rejectionFixtureDefJSON, nil)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, rejectionFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}
	if _, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatalf("JoinGame(%q, RolePlayer) error = %v, want nil", "Bob", err)
	}

	before, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}

	// "0,0" -> "2,2": correct seat (1, black), but not graph-adjacent.
	_, err = mgr.MakeMove(ctx, created.RoomCode, "Alice",
		board.PositionID(rejFixtureBlackA), board.PositionID(rejFixtureFarEmpty))
	if err == nil {
		t.Fatalf("MakeMove() to a non-adjacent destination error = nil, want non-nil")
	}

	after, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}
	expectStateUnchanged(t, before, after)
}

// --- 5. Illegal move: destination occupied by own color ---------------------
//
// An active game, correct seat's turn, `to` adjacent but occupied by the
// SAME color as the mover. MakeMove must return a non-nil error and leave
// state unchanged.

func TestMakeMove_RejectsDestinationOccupiedBySameColor(t *testing.T) {
	mgr := newRejectionTestManager(t, rejectionFixtureGameDefID, rejectionFixtureDefJSON, nil)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, rejectionFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}
	if _, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatalf("JoinGame(%q, RolePlayer) error = %v, want nil", "Bob", err)
	}

	before, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}

	// "0,0" -> "1,0": correct seat (1, black), adjacent, but "1,0" is also
	// black (the mover's own color).
	_, err = mgr.MakeMove(ctx, created.RoomCode, "Alice",
		board.PositionID(rejFixtureBlackA), board.PositionID(rejFixtureBlackB))
	if err == nil {
		t.Fatalf("MakeMove() onto the mover's own color error = nil, want non-nil")
	}

	after, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}
	expectStateUnchanged(t, before, after)
}

// --- 6. Illegal move: no piece at `from` ------------------------------------
//
// An active game, correct seat's turn, but `from` is an empty position.
// MakeMove must return a non-nil error and leave state unchanged.

func TestMakeMove_RejectsNoPieceAtFrom(t *testing.T) {
	mgr := newRejectionTestManager(t, rejectionFixtureGameDefID, rejectionFixtureDefJSON, nil)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, rejectionFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}
	if _, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatalf("JoinGame(%q, RolePlayer) error = %v, want nil", "Bob", err)
	}

	before, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}

	// "2,2" is empty; "1,2" (its neighbor) being empty too doesn't matter --
	// the rejection must happen because there's no piece at `from` at all.
	_, err = mgr.MakeMove(ctx, created.RoomCode, "Alice",
		board.PositionID(rejFixtureFarEmpty), board.PositionID(rejFixtureFarEmptyDest))
	if err == nil {
		t.Fatalf("MakeMove() from an empty position error = nil, want non-nil")
	}

	after, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}
	expectStateUnchanged(t, before, after)
}
