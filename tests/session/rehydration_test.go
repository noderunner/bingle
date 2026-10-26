// Tests that a new manager on the same database picks games up where the old one left them.

package session_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// --- fixture game definition -------------------------------------------------
//
// Deliberately re-declared locally (distinct identifier names from
// make_move_test.go's and this package's other fixtures) per this package's
// established self-containment convention: a self-contained 3x3 orthogonal4
// grid, 2 seats (1=black, 2=white), one "stone" piece via
// "step"/"capture-or-empty", round-robin turn order [1, 2], win_condition
// "none".
//
//	         y=0    y=1    y=2
//	x=0:   black-A   .      .
//	x=1:   white-B white-C  .
//	x=2:     .       .      .
//
//   - "0,0": black stone (seat 1)
//   - "1,0": white stone (seat 2) -- orthogonally adjacent to "0,0", so
//     black's first move "0,0" -> "1,0" is legal and captures this stone.
//   - "1,1": white stone (seat 2) -- survives the capture, giving seat 2 a
//     piece to move legally afterward (e.g. to the empty "1,2").

const rehydrationFixtureGameDefID = "fixture-3x3-rehydration"

const (
	rehydrationFixtureBlackPos        = "0,0" // seat 1 (black)'s only stone
	rehydrationFixtureWhiteCapturePos = "1,0" // seat 2 (white)'s stone black captures on move 1
	rehydrationFixtureWhiteFreePos    = "1,1" // seat 2 (white)'s surviving stone
	rehydrationFixtureWhiteFreeDest   = "1,2" // empty, adjacent to "1,1" -- white's non-capture destination
)

const rehydrationFixtureDefJSON = `{
  "id": "fixture-3x3-rehydration",
  "name": "Fixture 3x3 (rehydration tests)",
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
    { "position": "1,0", "piece": "stone", "color": "white" },
    { "position": "1,1", "piece": "stone", "color": "white" }
  ],
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "win_condition": { "type": "none" }
}`

// newRehydrationManagerAt builds a fresh *catalog.Catalog with the
// rehydration-tests fixture registered, opens a *store.Store against dbPath
// (an existing DB file is opened as-is; a nonexistent one is created), and
// returns a session.NewManager(st, cat) wired to both, along with the raw
// *store.Store handle so the caller can Close() it explicitly to simulate a
// process shutdown. Unlike this package's other fixtures' helpers, this one
// does NOT register a t.Cleanup for the store's Close -- callers that intend
// to simulate a restart need to control exactly when the underlying store
// closes, and register their own cleanup for whichever *store.Store ends up
// being the "last" one in a given test.
func newRehydrationManagerAt(t *testing.T, dbPath string) (*session.Manager, *store.Store) {
	t.Helper()

	cat := catalog.NewCatalog()
	if err := cat.Register(rehydrationFixtureGameDefID, []byte(rehydrationFixtureDefJSON), nil); err != nil {
		t.Fatalf("fixture setup: catalog.Register(%q) error = %v, want nil", rehydrationFixtureGameDefID, err)
	}

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("fixture setup: store.Open(%q) error = %v, want nil", dbPath, err)
	}

	mgr := session.NewManager(st, cat)
	if mgr == nil {
		t.Fatalf("session.NewManager(...) = nil, want non-nil *Manager")
	}

	return mgr, st
}

// createActivateAndMove runs CreateGame(Alice) + JoinGame(Bob, RolePlayer)
// (which activates the game and sets turn_seat to seat 1) + one legal
// capturing move by Alice (seat 1), leaving real, non-trivial state: a
// specific board_state and turn_seat flipped to seat 2. Returns the room
// code.
func createActivateAndMove(t *testing.T, mgr *session.Manager, ctx context.Context) string {
	t.Helper()

	created, err := mgr.CreateGame(ctx, rehydrationFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	joined, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(%q, RolePlayer) error = %v, want nil", "Bob", err)
	}
	if joined.Status != "active" {
		t.Fatalf("JoinGame(%q, RolePlayer) Status = %q, want %q (precondition for this test)", "Bob", joined.Status, "active")
	}

	if _, err := mgr.MakeMove(ctx, created.RoomCode, "Alice",
		board.PositionID(rehydrationFixtureBlackPos), board.PositionID(rehydrationFixtureWhiteCapturePos)); err != nil {
		t.Fatalf("MakeMove(%q, %q->%q) error = %v, want nil",
			"Alice", rehydrationFixtureBlackPos, rehydrationFixtureWhiteCapturePos, err)
	}

	return created.RoomCode
}

// --- 1. The AC itself: a fresh Manager against the same DB file returns the
//        exact same board_state/turn_seat (plus move_seq/status) GetState
//        reported before the "restart" ---------------------------------------

func TestManagerRehydration_GetState_MatchesPreRestartState(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	mgr1, st1 := newRehydrationManagerAt(t, dbPath)
	roomCode := createActivateAndMove(t, mgr1, ctx)

	preRestart, err := mgr1.GetState(ctx, roomCode, nil)
	if err != nil {
		t.Fatalf("Manager #1 GetState(%q) error = %v, want nil", roomCode, err)
	}
	// Sanity: confirm this snapshot really is the non-trivial post-move state
	// we think it is, so a passing comparison below can't be vacuous.
	if preRestart.MoveSeq != 1 {
		t.Fatalf("fixture setup: Manager #1 GetState() MoveSeq = %d, want 1 (precondition for this test)", preRestart.MoveSeq)
	}
	if preRestart.TurnSeat == nil || *preRestart.TurnSeat != 2 {
		t.Fatalf("fixture setup: Manager #1 GetState() TurnSeat = %v, want pointer to 2 (precondition for this test)", preRestart.TurnSeat)
	}

	// Simulate the process actually restarting: close the first store, then
	// open a brand-new store handle against the SAME file path, and a
	// brand-new catalog (re-registering the same fixture, since catalogs
	// aren't persisted).
	if err := st1.Close(); err != nil {
		t.Fatalf("Manager #1 store Close() error = %v, want nil", err)
	}

	mgr2, st2 := newRehydrationManagerAt(t, dbPath)
	t.Cleanup(func() { _ = st2.Close() })

	postRestart, err := mgr2.GetState(ctx, roomCode, nil)
	if err != nil {
		t.Fatalf("Manager #2 (post-restart) GetState(%q) error = %v, want nil", roomCode, err)
	}

	if postRestart.BoardState != preRestart.BoardState {
		t.Errorf("Manager #2 GetState() BoardState = %s, want unchanged from pre-restart %s",
			postRestart.BoardState, preRestart.BoardState)
	}
	if (postRestart.TurnSeat == nil) != (preRestart.TurnSeat == nil) {
		t.Fatalf("Manager #2 GetState() TurnSeat = %v, want same nilness as pre-restart %v",
			postRestart.TurnSeat, preRestart.TurnSeat)
	}
	if postRestart.TurnSeat != nil && *postRestart.TurnSeat != *preRestart.TurnSeat {
		t.Errorf("Manager #2 GetState() TurnSeat = %d, want %d (unchanged from pre-restart)",
			*postRestart.TurnSeat, *preRestart.TurnSeat)
	}
	if postRestart.MoveSeq != preRestart.MoveSeq {
		t.Errorf("Manager #2 GetState() MoveSeq = %d, want %d (unchanged from pre-restart)",
			postRestart.MoveSeq, preRestart.MoveSeq)
	}
	if postRestart.Status != preRestart.Status {
		t.Errorf("Manager #2 GetState() Status = %q, want %q (unchanged from pre-restart)",
			postRestart.Status, preRestart.Status)
	}
	if postRestart.RoomCode != preRestart.RoomCode {
		t.Errorf("Manager #2 GetState() RoomCode = %q, want %q", postRestart.RoomCode, preRestart.RoomCode)
	}
	if postRestart.GameDefID != preRestart.GameDefID {
		t.Errorf("Manager #2 GetState() GameDefID = %q, want %q", postRestart.GameDefID, preRestart.GameDefID)
	}
}

// --- 2. The rehydrated Manager isn't merely read-only: it can drive the
//        NEXT legal move through the full MakeMove pipeline, including a
//        per-room mutex it never had before (none existed on Manager #2
//        until this call lazily creates one) ---------------------------------

func TestManagerRehydration_SecondManagerCanMakeNextMove(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	mgr1, st1 := newRehydrationManagerAt(t, dbPath)
	roomCode := createActivateAndMove(t, mgr1, ctx)

	if err := st1.Close(); err != nil {
		t.Fatalf("Manager #1 store Close() error = %v, want nil", err)
	}

	mgr2, st2 := newRehydrationManagerAt(t, dbPath)
	t.Cleanup(func() { _ = st2.Close() })

	// It's seat 2 (Bob/white)'s turn after createActivateAndMove's single
	// move. Bob's surviving stone at rehydrationFixtureWhiteFreePos can move
	// legally (non-capturing) to rehydrationFixtureWhiteFreeDest.
	outcome, err := mgr2.MakeMove(ctx, roomCode, "Bob",
		board.PositionID(rehydrationFixtureWhiteFreePos), board.PositionID(rehydrationFixtureWhiteFreeDest))
	if err != nil {
		t.Fatalf("Manager #2 (post-restart) MakeMove(%q, %q->%q) error = %v, want nil",
			"Bob", rehydrationFixtureWhiteFreePos, rehydrationFixtureWhiteFreeDest, err)
	}

	if outcome.Seq != 2 {
		t.Errorf("Manager #2 MakeMove() Seq = %d, want 2 (second move overall, persisted across restart)", outcome.Seq)
	}
	if outcome.Seat != 2 {
		t.Errorf("Manager #2 MakeMove() Seat = %d, want 2 (Bob holds seat 2)", outcome.Seat)
	}
	if outcome.NextTurnSeat != 1 {
		t.Errorf("Manager #2 MakeMove() NextTurnSeat = %d, want 1 (flipped back from seat 2)", outcome.NextTurnSeat)
	}
	if outcome.Status != "active" {
		t.Errorf("Manager #2 MakeMove() Status = %q, want %q", outcome.Status, "active")
	}

	// And GetState on Manager #2 itself now reflects this new move too.
	snap, err := mgr2.GetState(ctx, roomCode, nil)
	if err != nil {
		t.Fatalf("Manager #2 GetState(%q) error = %v, want nil", roomCode, err)
	}
	if snap.MoveSeq != 2 {
		t.Errorf("Manager #2 GetState() MoveSeq = %d, want 2", snap.MoveSeq)
	}
	if snap.TurnSeat == nil {
		t.Fatalf("Manager #2 GetState() TurnSeat = nil, want pointer to 1")
	}
	if *snap.TurnSeat != 1 {
		t.Errorf("Manager #2 GetState() TurnSeat = %d, want 1 (flipped back from seat 2 by move 2)", *snap.TurnSeat)
	}
}

// --- 3. Multiple rooms survive independently: two games created under
//        Manager #1 (one with a move made, one left "waiting") both come
//        back correctly and distinctly on Manager #2, with no crosstalk and
//        the "waiting" one undisturbed by rehydration --------------------

func TestManagerRehydration_MultipleRoomsSurviveIndependently(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	mgr1, st1 := newRehydrationManagerAt(t, dbPath)

	// Room A: created, joined (activated), and one move made -- the same
	// non-trivial flow as the other tests in this file.
	roomA := createActivateAndMove(t, mgr1, ctx)

	// Room B: created only, never joined -- left in "waiting" status with an
	// untouched initial board state and no turn_seat.
	createdB, err := mgr1.CreateGame(ctx, rehydrationFixtureGameDefID, "Carol")
	if err != nil {
		t.Fatalf("CreateGame() [room B] error = %v, want nil", err)
	}
	roomB := createdB.RoomCode

	if roomA == roomB {
		t.Fatalf("fixture setup: room A code %q == room B code %q, want distinct room codes (precondition for this test)", roomA, roomB)
	}

	preA, err := mgr1.GetState(ctx, roomA, nil)
	if err != nil {
		t.Fatalf("Manager #1 GetState(room A = %q) error = %v, want nil", roomA, err)
	}
	preB, err := mgr1.GetState(ctx, roomB, nil)
	if err != nil {
		t.Fatalf("Manager #1 GetState(room B = %q) error = %v, want nil", roomB, err)
	}
	if preB.Status != "waiting" {
		t.Fatalf("fixture setup: Manager #1 GetState(room B) Status = %q, want %q (precondition for this test)", preB.Status, "waiting")
	}
	if preB.TurnSeat != nil {
		t.Fatalf("fixture setup: Manager #1 GetState(room B) TurnSeat = %v, want nil (precondition: never activated)", *preB.TurnSeat)
	}

	if err := st1.Close(); err != nil {
		t.Fatalf("Manager #1 store Close() error = %v, want nil", err)
	}

	mgr2, st2 := newRehydrationManagerAt(t, dbPath)
	t.Cleanup(func() { _ = st2.Close() })

	postA, err := mgr2.GetState(ctx, roomA, nil)
	if err != nil {
		t.Fatalf("Manager #2 (post-restart) GetState(room A = %q) error = %v, want nil", roomA, err)
	}
	postB, err := mgr2.GetState(ctx, roomB, nil)
	if err != nil {
		t.Fatalf("Manager #2 (post-restart) GetState(room B = %q) error = %v, want nil", roomB, err)
	}

	// Room A: active, post-move state, matching exactly what Manager #1 saw.
	if postA.BoardState != preA.BoardState {
		t.Errorf("Manager #2 GetState(room A) BoardState = %s, want %s", postA.BoardState, preA.BoardState)
	}
	if postA.TurnSeat == nil || preA.TurnSeat == nil {
		t.Fatalf("Manager #2 GetState(room A) TurnSeat = %v, preA.TurnSeat = %v, want both non-nil", postA.TurnSeat, preA.TurnSeat)
	}
	if *postA.TurnSeat != *preA.TurnSeat {
		t.Errorf("Manager #2 GetState(room A) TurnSeat = %d, want %d", *postA.TurnSeat, *preA.TurnSeat)
	}
	if postA.MoveSeq != preA.MoveSeq {
		t.Errorf("Manager #2 GetState(room A) MoveSeq = %d, want %d", postA.MoveSeq, preA.MoveSeq)
	}
	if postA.Status != "active" {
		t.Errorf("Manager #2 GetState(room A) Status = %q, want %q", postA.Status, "active")
	}

	// Room B: still "waiting", untouched by rehydration, and NOT contaminated
	// with room A's board state or turn_seat.
	if postB.Status != "waiting" {
		t.Errorf("Manager #2 GetState(room B) Status = %q, want %q (rehydration must not disturb a waiting game)", postB.Status, "waiting")
	}
	if postB.TurnSeat != nil {
		t.Errorf("Manager #2 GetState(room B) TurnSeat = %v, want nil (never activated, before or after restart)", *postB.TurnSeat)
	}
	if postB.MoveSeq != preB.MoveSeq {
		t.Errorf("Manager #2 GetState(room B) MoveSeq = %d, want %d", postB.MoveSeq, preB.MoveSeq)
	}
	if postB.BoardState != preB.BoardState {
		t.Errorf("Manager #2 GetState(room B) BoardState = %s, want %s (unchanged initial setup)", postB.BoardState, preB.BoardState)
	}

	// Cross-check: the two rooms' states genuinely differ from each other
	// (no crosstalk / accidental aliasing between rooms after rehydration).
	if postA.BoardState == postB.BoardState {
		t.Errorf("Manager #2 GetState(room A) and GetState(room B) returned identical BoardState %s, want distinct (room A has a move applied, room B doesn't)", postA.BoardState)
	}
	if postA.RoomCode == postB.RoomCode {
		t.Errorf("Manager #2 GetState(room A).RoomCode == GetState(room B).RoomCode == %q, want distinct", postA.RoomCode)
	}
}
