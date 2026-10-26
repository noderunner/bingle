// Tests of the session manager's creation and state reads: a new game waits with one seat filled, results carry their fields, and unknown definitions are refused.

package session_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// --- fixture game definition -------------------------------------------------
//
// A small, self-contained 3x3 orthogonal4 grid, 2 seats (1=black, 2=white),
// one piece type "stone" via "step"/"capture-or-empty", round-robin turn
// order over [1,2], win_condition "none", and a setup with exactly one black
// stone and one white stone at distinct, known positions — so the initial
// BoardState can be checked against setup precisely, not just "non-empty".

const fixtureGameDefID = "fixture-3x3"

const fixtureBlackPos = "0,0"
const fixtureWhitePos = "2,2"

const fixtureDefJSON = `{
  "id": "fixture-3x3",
  "name": "Fixture 3x3",
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
    { "position": "2,2", "piece": "stone", "color": "white" }
  ],
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "win_condition": { "type": "none" }
}`

// newTestManager builds a fresh *catalog.Catalog with the fixture def
// registered, a fresh *store.Store against a temp-dir SQLite file, and
// returns a session.NewManager(st, cat) wired to both, along with the store
// itself (for direct inspection where useful) and a cleanup-free context.
func newTestManager(t *testing.T) (*session.Manager, *store.Store) {
	t.Helper()

	cat := catalog.NewCatalog()
	if err := cat.Register(fixtureGameDefID, []byte(fixtureDefJSON), nil); err != nil {
		t.Fatalf("fixture setup: catalog.Register(%q) error = %v, want nil", fixtureGameDefID, err)
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

	return mgr, st
}

// --- 1. The AC itself: CreateGame returns a room code; GetState immediately
//        reports status=waiting with exactly one seat filled --------------------

func TestCreateGame_ThenGetState_ReportsWaitingWithExactlyOneSeatFilled(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	result, err := mgr.CreateGame(ctx, fixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	if len(result.RoomCode) != 6 {
		t.Errorf("CreateGame() RoomCode = %q (len %d), want length 6", result.RoomCode, len(result.RoomCode))
	}
	for _, r := range result.RoomCode {
		if !strings.ContainsRune(roomcodeAlphabet, r) {
			t.Errorf("CreateGame() RoomCode = %q contains rune %q not in roomcode.Alphabet %q", result.RoomCode, r, roomcodeAlphabet)
			break
		}
	}

	snap, err := mgr.GetState(ctx, result.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", result.RoomCode, err)
	}

	if snap.Status != "waiting" {
		t.Errorf("GetState() Status = %q, want %q", snap.Status, "waiting")
	}

	if len(snap.Seats) != 2 {
		t.Fatalf("GetState() Seats has %d entries, want 2 (one per declared seat)", len(snap.Seats))
	}

	var seat1, seat2 *session.SeatInfo
	for i := range snap.Seats {
		switch snap.Seats[i].Seat {
		case 1:
			seat1 = &snap.Seats[i]
		case 2:
			seat2 = &snap.Seats[i]
		}
	}
	if seat1 == nil {
		t.Fatalf("GetState() Seats missing seat 1: %+v", snap.Seats)
	}
	if seat2 == nil {
		t.Fatalf("GetState() Seats missing seat 2: %+v", snap.Seats)
	}

	if seat1.Name != "Alice" {
		t.Errorf("GetState() Seats[seat=1].Name = %q, want %q (the creator)", seat1.Name, "Alice")
	}
	if seat2.Name != "" {
		t.Errorf("GetState() Seats[seat=2].Name = %q, want \"\" (open seat)", seat2.Name)
	}

	// Exactly one seat filled: count non-empty names.
	filled := 0
	for _, s := range snap.Seats {
		if s.Name != "" {
			filled++
		}
	}
	if filled != 1 {
		t.Errorf("GetState() Seats has %d filled seats, want exactly 1", filled)
	}
}

// roomcodeAlphabet mirrors internal/session/roomcode.Alphabet's character
// set, duplicated here (rather than importing the roomcode subpackage) so
// this assertion is a black-box check on Manager's output, not a dependency
// on roomcode's internals from this file specifically. (Other files in this
// same test package do import roomcode directly; that's fine, this is just
// this file's own choice for a self-contained check.)
const roomcodeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// --- 2. CreateGameResult itself: Seat, Color, Status -------------------------

func TestCreateGame_ResultFields(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	result, err := mgr.CreateGame(ctx, fixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	if result.Seat != 1 {
		t.Errorf("CreateGame() Seat = %d, want 1 (creator always takes seat 1)", result.Seat)
	}
	if result.Color != "black" {
		t.Errorf("CreateGame() Color = %q, want %q (seat 1's declared color in the fixture)", result.Color, "black")
	}
	if result.Status != "waiting" {
		t.Errorf("CreateGame() Status = %q, want %q", result.Status, "waiting")
	}
}

// --- 3. GetState's BoardState reflects the fixture's setup -------------------

func TestGetState_BoardStateReflectsSetup(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	result, err := mgr.CreateGame(ctx, fixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	snap, err := mgr.GetState(ctx, result.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", result.RoomCode, err)
	}

	if snap.BoardState == "" {
		t.Fatalf("GetState() BoardState = \"\", want non-empty JSON reflecting setup")
	}

	// state.BoardState.MarshalJSON produces a JSON object keyed by position
	// string, valued by {"Piece":..., "Color":...}. Decode generically to
	// avoid depending on internal/engine/state's exact Go type from this
	// black-box test (this test package may only import internal/session,
	// internal/engine/catalog, and internal/store) — a raw map[string]map[string]string is enough to check the two
	// setup entries landed at their configured positions.
	var occupants map[string]struct {
		Piece string
		Color string
	}
	if err := json.Unmarshal([]byte(snap.BoardState), &occupants); err != nil {
		t.Fatalf("json.Unmarshal(BoardState) error = %v, want nil; BoardState = %s", err, snap.BoardState)
	}

	black, ok := occupants[fixtureBlackPos]
	if !ok {
		t.Fatalf("BoardState missing entry at %q (fixture's black stone position); BoardState = %s", fixtureBlackPos, snap.BoardState)
	}
	if black.Piece != "stone" || black.Color != "black" {
		t.Errorf("BoardState[%q] = %+v, want {Piece: \"stone\", Color: \"black\"}", fixtureBlackPos, black)
	}

	white, ok := occupants[fixtureWhitePos]
	if !ok {
		t.Fatalf("BoardState missing entry at %q (fixture's white stone position); BoardState = %s", fixtureWhitePos, snap.BoardState)
	}
	if white.Piece != "stone" || white.Color != "white" {
		t.Errorf("BoardState[%q] = %+v, want {Piece: \"stone\", Color: \"white\"}", fixtureWhitePos, white)
	}

	if len(occupants) != 2 {
		t.Errorf("BoardState has %d occupied positions, want exactly 2 (fixture's setup has 2 entries); BoardState = %s", len(occupants), snap.BoardState)
	}
}

// --- 4. CreateGame with an unregistered game def id ---------------------------

func TestCreateGame_UnknownGameDefID_ReturnsErrGameDefNotFound(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.CreateGame(ctx, "does-not-exist", "Alice")
	if err == nil {
		t.Fatalf("CreateGame() error = nil, want non-nil (unregistered game def id)")
	}
	if !errors.Is(err, session.ErrGameDefNotFound) {
		t.Errorf("CreateGame() error = %v, want errors.Is(err, session.ErrGameDefNotFound)", err)
	}
}

// --- 5. GetState for a room code that was never created -----------------------

func TestGetState_UnknownRoomCode_ReturnsStoreErrNotFound(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.GetState(ctx, "ZZZZZZ", nil)
	if err == nil {
		t.Fatalf("GetState() error = nil, want non-nil (room code never created)")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetState() error = %v, want errors.Is(err, store.ErrNotFound)", err)
	}
}

// --- 6. GetState with atSeq=1 immediately after CreateGame (no moves yet) -----

func TestGetState_AtSeqBeforeAnyMoves_ReturnsStoreErrNotFound(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	result, err := mgr.CreateGame(ctx, fixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	seq := 1
	_, err = mgr.GetState(ctx, result.RoomCode, &seq)
	if err == nil {
		t.Fatalf("GetState(roomCode, atSeq=1) error = nil, want non-nil (no move 1 recorded yet)")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetState(roomCode, atSeq=1) error = %v, want errors.Is(err, store.ErrNotFound)", err)
	}
}

// --- 7. Two separate CreateGame calls produce distinct room codes, no
//        crosstalk between GetState results ----------------------------------

func TestCreateGame_TwoGames_DistinctRoomCodesAndNoCrosstalk(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	result1, err := mgr.CreateGame(ctx, fixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() #1 error = %v, want nil", err)
	}
	result2, err := mgr.CreateGame(ctx, fixtureGameDefID, "Bob")
	if err != nil {
		t.Fatalf("CreateGame() #2 error = %v, want nil", err)
	}

	if result1.RoomCode == result2.RoomCode {
		t.Fatalf("CreateGame() produced the same RoomCode %q twice, want distinct codes", result1.RoomCode)
	}

	snap1, err := mgr.GetState(ctx, result1.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", result1.RoomCode, err)
	}
	snap2, err := mgr.GetState(ctx, result2.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", result2.RoomCode, err)
	}

	if snap1.RoomCode != result1.RoomCode {
		t.Errorf("GetState(%q).RoomCode = %q, want %q", result1.RoomCode, snap1.RoomCode, result1.RoomCode)
	}
	if snap2.RoomCode != result2.RoomCode {
		t.Errorf("GetState(%q).RoomCode = %q, want %q", result2.RoomCode, snap2.RoomCode, result2.RoomCode)
	}

	var game1Seat1, game2Seat1 *session.SeatInfo
	for i := range snap1.Seats {
		if snap1.Seats[i].Seat == 1 {
			game1Seat1 = &snap1.Seats[i]
		}
	}
	for i := range snap2.Seats {
		if snap2.Seats[i].Seat == 1 {
			game2Seat1 = &snap2.Seats[i]
		}
	}
	if game1Seat1 == nil {
		t.Fatalf("GetState(%q) missing seat 1: %+v", result1.RoomCode, snap1.Seats)
	}
	if game2Seat1 == nil {
		t.Fatalf("GetState(%q) missing seat 1: %+v", result2.RoomCode, snap2.Seats)
	}

	if game1Seat1.Name != "Alice" {
		t.Errorf("GetState(%q) seat 1 Name = %q, want %q (no crosstalk from game 2)", result1.RoomCode, game1Seat1.Name, "Alice")
	}
	if game2Seat1.Name != "Bob" {
		t.Errorf("GetState(%q) seat 1 Name = %q, want %q (no crosstalk from game 1)", result2.RoomCode, game2Seat1.Name, "Bob")
	}
}
