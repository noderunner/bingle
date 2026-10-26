// Tests of making moves through the session manager: outcomes, persistence, turn changes and unknown players.

package session_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// --- fixture game definition -------------------------------------------------
//
// A self-contained 3x3 orthogonal4 grid (same style as manager_test.go's and
// join_game_test.go's fixtures, deliberately re-declared here rather than
// shared, per those files' own note that each file in this package is free
// to build its own fixture), 2 seats (1=black, 2=white), one "stone" piece
// via "step"/"capture-or-empty", round-robin turn order [1, 2],
// win_condition "none".
//
// Layout, chosen so both a capturing first move AND a non-capturing second
// move are exercisable with known, hand-verified adjacency:
//
//	         y=0    y=1    y=2
//	x=0:   black-A   .      .
//	x=1:   white-B white-C  .
//	x=2:     .       .      .
//
//   - "0,0": black stone (seat 1)
//   - "1,0": white stone (seat 2) — orthogonally adjacent to "0,0" (dx=1,
//     dy=0), so black's very first move "0,0" -> "1,0" is legal AND captures
//     this stone.
//   - "1,1": white stone (seat 2) — a second white stone that survives the
//     capture, giving seat 2 a piece to make its own, non-capturing, legal
//     move with afterward (its neighbors "1,0", "1,2", "0,1", "2,1" include
//     the empty "1,2", "0,1", "2,1").

const makeMoveFixtureGameDefID = "fixture-3x3-makemove"

const (
	makeMoveFixtureBlackPos        = "0,0" // seat 1 (black)'s only stone
	makeMoveFixtureWhiteCapturePos = "1,0" // seat 2 (white)'s stone black captures on move 1
	makeMoveFixtureWhiteFreePos    = "1,1" // seat 2 (white)'s surviving stone, moved on move 2
	makeMoveFixtureWhiteFreeDest   = "1,2" // empty, adjacent to "1,1" -- white's non-capture destination
)

const makeMoveFixtureDefJSON = `{
  "id": "fixture-3x3-makemove",
  "name": "Fixture 3x3 (make-move tests)",
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

// newMakeMoveTestManager builds a fresh *catalog.Catalog with the
// make-move-tests fixture registered, a fresh *store.Store against a
// temp-dir SQLite file, and returns a session.NewManager(st, cat) wired to
// both. Same pattern as manager_test.go's newTestManager / join_game_test.go's
// newJoinTestManager, reproduced locally per this package's established
// self-containment convention.
func newMakeMoveTestManager(t *testing.T) *session.Manager {
	t.Helper()

	cat := catalog.NewCatalog()
	if err := cat.Register(makeMoveFixtureGameDefID, []byte(makeMoveFixtureDefJSON), nil); err != nil {
		t.Fatalf("fixture setup: catalog.Register(%q) error = %v, want nil", makeMoveFixtureGameDefID, err)
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

// createAndActivateMakeMoveGame runs the standard flow: CreateGame(Alice)
// then JoinGame(Bob, RolePlayer), which (per docs/history/plan.md section 6.4c/6.4d)
// activates the game and sets turn_seat to seat 1 (the first entry in the
// fixture's round-robin turn order). Returns the resulting room code.
func createAndActivateMakeMoveGame(t *testing.T, mgr *session.Manager, ctx context.Context) string {
	t.Helper()

	created, err := mgr.CreateGame(ctx, makeMoveFixtureGameDefID, "Alice")
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

	return created.RoomCode
}

// boardOccupants decodes a state.BoardState JSON string generically (a raw
// map keyed by position string) so this black-box test package doesn't need
// to import internal/engine/state's exact Go type.
type boardOccupant struct {
	Piece string
	Color string
}

func decodeBoardOccupants(t *testing.T, boardStateJSON string) map[string]boardOccupant {
	t.Helper()
	var occupants map[string]boardOccupant
	if err := json.Unmarshal([]byte(boardStateJSON), &occupants); err != nil {
		t.Fatalf("json.Unmarshal(BoardState) error = %v, want nil; BoardState = %s", err, boardStateJSON)
	}
	return occupants
}

// --- 1. The AC itself: a legal, capturing move by the correct seat's turn
//        succeeds and returns the full MoveOutcome -------------------------

func TestMakeMove_LegalCaptureByCorrectSeat_ReturnsExpectedOutcome(t *testing.T) {
	mgr := newMakeMoveTestManager(t)
	ctx := context.Background()

	roomCode := createAndActivateMakeMoveGame(t, mgr, ctx)

	outcome, err := mgr.MakeMove(ctx, roomCode, "Alice",
		board.PositionID(makeMoveFixtureBlackPos), board.PositionID(makeMoveFixtureWhiteCapturePos))
	if err != nil {
		t.Fatalf("MakeMove(%q, %q, %q->%q) error = %v, want nil",
			roomCode, "Alice", makeMoveFixtureBlackPos, makeMoveFixtureWhiteCapturePos, err)
	}

	if outcome.Seq != 1 {
		t.Errorf("MakeMove() Seq = %d, want 1", outcome.Seq)
	}
	if outcome.Seat != 1 {
		t.Errorf("MakeMove() Seat = %d, want 1 (Alice holds seat 1)", outcome.Seat)
	}
	if outcome.Color != "black" {
		t.Errorf("MakeMove() Color = %q, want %q", outcome.Color, "black")
	}
	if outcome.From != makeMoveFixtureBlackPos {
		t.Errorf("MakeMove() From = %q, want %q", outcome.From, makeMoveFixtureBlackPos)
	}
	if outcome.To != makeMoveFixtureWhiteCapturePos {
		t.Errorf("MakeMove() To = %q, want %q", outcome.To, makeMoveFixtureWhiteCapturePos)
	}
	if outcome.Captured == nil {
		t.Errorf("MakeMove() Captured = nil, want pointer to %q", makeMoveFixtureWhiteCapturePos)
	} else if *outcome.Captured != makeMoveFixtureWhiteCapturePos {
		t.Errorf("MakeMove() Captured = %q, want %q", *outcome.Captured, makeMoveFixtureWhiteCapturePos)
	}
	if outcome.NextTurnSeat != 2 {
		t.Errorf("MakeMove() NextTurnSeat = %d, want 2 (flipped from seat 1)", outcome.NextTurnSeat)
	}
	if outcome.GameEnded {
		t.Errorf("MakeMove() GameEnded = true, want false (win_condition: none)")
	}
	if outcome.WinnerSeat != nil {
		t.Errorf("MakeMove() WinnerSeat = %v, want nil", *outcome.WinnerSeat)
	}
	if outcome.Status != "active" {
		t.Errorf("MakeMove() Status = %q, want %q", outcome.Status, "active")
	}
	if outcome.BoardState == "" {
		t.Errorf("MakeMove() BoardState = \"\", want non-empty JSON")
	}
}

// --- 2. Immediately after that move, GetState confirms exactly one move was
//        persisted: turn_seat flipped to 2, move_seq == 1, and the board
//        state itself shows black at the destination and nothing at the
//        origin (via direct occupant inspection, not string matching) ------

func TestMakeMove_LegalCapture_PersistsAndUpdatesGameRow(t *testing.T) {
	mgr := newMakeMoveTestManager(t)
	ctx := context.Background()

	roomCode := createAndActivateMakeMoveGame(t, mgr, ctx)

	if _, err := mgr.MakeMove(ctx, roomCode, "Alice",
		board.PositionID(makeMoveFixtureBlackPos), board.PositionID(makeMoveFixtureWhiteCapturePos)); err != nil {
		t.Fatalf("MakeMove() error = %v, want nil", err)
	}

	snap, err := mgr.GetState(ctx, roomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", roomCode, err)
	}

	if snap.MoveSeq != 1 {
		t.Errorf("GetState() MoveSeq = %d, want 1 (exactly one move persisted)", snap.MoveSeq)
	}
	if snap.TurnSeat == nil {
		t.Fatalf("GetState() TurnSeat = nil, want pointer to 2")
	}
	if *snap.TurnSeat != 2 {
		t.Errorf("GetState() TurnSeat = %d, want 2 (flipped from seat 1 by the move)", *snap.TurnSeat)
	}
	if snap.Status != "active" {
		t.Errorf("GetState() Status = %q, want %q", snap.Status, "active")
	}

	occupants := decodeBoardOccupants(t, snap.BoardState)

	// Black stone landed at the capture destination.
	dest, ok := occupants[makeMoveFixtureWhiteCapturePos]
	if !ok {
		t.Fatalf("BoardState missing entry at %q (expected black's captured-into stone); BoardState = %s",
			makeMoveFixtureWhiteCapturePos, snap.BoardState)
	}
	if dest.Piece != "stone" || dest.Color != "black" {
		t.Errorf("BoardState[%q] = %+v, want {Piece: \"stone\", Color: \"black\"}", makeMoveFixtureWhiteCapturePos, dest)
	}

	// The origin square is now empty.
	if occ, ok := occupants[makeMoveFixtureBlackPos]; ok {
		t.Errorf("BoardState still has an occupant at origin %q: %+v, want no entry (piece moved away)",
			makeMoveFixtureBlackPos, occ)
	}

	// White's surviving stone is untouched.
	survivor, ok := occupants[makeMoveFixtureWhiteFreePos]
	if !ok {
		t.Fatalf("BoardState missing entry at %q (white's surviving stone); BoardState = %s",
			makeMoveFixtureWhiteFreePos, snap.BoardState)
	}
	if survivor.Piece != "stone" || survivor.Color != "white" {
		t.Errorf("BoardState[%q] = %+v, want {Piece: \"stone\", Color: \"white\"}", makeMoveFixtureWhiteFreePos, survivor)
	}

	// Exactly two occupants total: the captured stone is gone, nothing else
	// changed.
	if len(occupants) != 2 {
		t.Errorf("BoardState has %d occupied positions, want exactly 2 (captured stone removed); BoardState = %s",
			len(occupants), snap.BoardState)
	}
}

// --- 3. A second legal move, now by the other seat whose turn it has
//        become, succeeds with Seq == 2 and flips NextTurnSeat back to 1 --

func TestMakeMove_SecondLegalMoveByOtherSeat_AdvancesSeqAndFlipsTurnBack(t *testing.T) {
	mgr := newMakeMoveTestManager(t)
	ctx := context.Background()

	roomCode := createAndActivateMakeMoveGame(t, mgr, ctx)

	if _, err := mgr.MakeMove(ctx, roomCode, "Alice",
		board.PositionID(makeMoveFixtureBlackPos), board.PositionID(makeMoveFixtureWhiteCapturePos)); err != nil {
		t.Fatalf("MakeMove() [move 1, Alice] error = %v, want nil", err)
	}

	outcome, err := mgr.MakeMove(ctx, roomCode, "Bob",
		board.PositionID(makeMoveFixtureWhiteFreePos), board.PositionID(makeMoveFixtureWhiteFreeDest))
	if err != nil {
		t.Fatalf("MakeMove() [move 2, Bob] error = %v, want nil", err)
	}

	if outcome.Seq != 2 {
		t.Errorf("MakeMove() [move 2] Seq = %d, want 2", outcome.Seq)
	}
	if outcome.Seat != 2 {
		t.Errorf("MakeMove() [move 2] Seat = %d, want 2 (Bob holds seat 2)", outcome.Seat)
	}
	if outcome.Color != "white" {
		t.Errorf("MakeMove() [move 2] Color = %q, want %q", outcome.Color, "white")
	}
	if outcome.Captured != nil {
		t.Errorf("MakeMove() [move 2] Captured = %q, want nil (destination was empty)", *outcome.Captured)
	}
	if outcome.NextTurnSeat != 1 {
		t.Errorf("MakeMove() [move 2] NextTurnSeat = %d, want 1 (flipped back from seat 2)", outcome.NextTurnSeat)
	}
	if outcome.Status != "active" {
		t.Errorf("MakeMove() [move 2] Status = %q, want %q", outcome.Status, "active")
	}

	snap, err := mgr.GetState(ctx, roomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", roomCode, err)
	}
	if snap.MoveSeq != 2 {
		t.Errorf("GetState() MoveSeq = %d, want 2 (two moves persisted)", snap.MoveSeq)
	}
	if snap.TurnSeat == nil {
		t.Errorf("GetState() TurnSeat = nil, want pointer to 1")
	} else if *snap.TurnSeat != 1 {
		t.Errorf("GetState() TurnSeat = %d, want 1 (flipped back from seat 2 by move 2)", *snap.TurnSeat)
	}
}

// --- 4. A quick smoke check: MakeMove by a name that holds no seat at all
//        in this game returns ErrNotYourTurn (not a panic, not some other
//        error) -- item 21 covers rejection paths thoroughly; this is just a
//        sanity check on the "not found among players" branch that the
//        happy-path lookup above also exercises. -----------------------------

func TestMakeMove_UnknownPlayerName_ReturnsErrNotYourTurn(t *testing.T) {
	mgr := newMakeMoveTestManager(t)
	ctx := context.Background()

	roomCode := createAndActivateMakeMoveGame(t, mgr, ctx)

	_, err := mgr.MakeMove(ctx, roomCode, "Zed",
		board.PositionID(makeMoveFixtureBlackPos), board.PositionID(makeMoveFixtureWhiteCapturePos))
	if err == nil {
		t.Fatalf("MakeMove(%q, ...) error = nil, want non-nil ( %q holds no seat in this game)", "Zed", "Zed")
	}
	if !errors.Is(err, session.ErrNotYourTurn) {
		t.Errorf("MakeMove(%q, ...) error = %v, want errors.Is(err, session.ErrNotYourTurn)", "Zed", err)
	}
}
