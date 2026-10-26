// Tests of joining a game: the second player fills seat 2 and starts the game, the join is stored, and games do not affect each other.

package session_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// --- fixture game definition -------------------------------------------------
//
// Reuses the same shape as manager_test.go's fixture (a self-contained 3x3
// orthogonal4 grid, 2 seats 1=black/2=white, one "stone" piece via
// "step"/"capture-or-empty", round-robin turn order, win_condition "none") —
// deliberately re-declared here (not imported/shared) so this file stands on
// its own and doesn't couple to manager_test.go's internals, per that file's
// own note that other test files in this package are free to build their own
// fixtures.

const joinFixtureGameDefID = "fixture-3x3-join"

const joinFixtureDefJSON = `{
  "id": "fixture-3x3-join",
  "name": "Fixture 3x3 (join tests)",
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

// newJoinTestManager builds a fresh *catalog.Catalog with the join-tests
// fixture registered, a fresh *store.Store against a temp-dir SQLite file,
// and returns a session.NewManager(st, cat) wired to both. Same pattern as
// manager_test.go's newTestManager, reproduced locally per this file's own
// self-containment choice.
func newJoinTestManager(t *testing.T) *session.Manager {
	t.Helper()

	cat := catalog.NewCatalog()
	if err := cat.Register(joinFixtureGameDefID, []byte(joinFixtureDefJSON), nil); err != nil {
		t.Fatalf("fixture setup: catalog.Register(%q) error = %v, want nil", joinFixtureGameDefID, err)
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

// --- 1. The AC itself: a second distinct name joining as role=player fills
//        seat 2 and flips status to active ----------------------------------

func TestJoinGame_SecondDistinctPlayer_FillsSeat2AndActivatesGame(t *testing.T) {
	mgr := newJoinTestManager(t)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, joinFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}
	if created.Status != "waiting" {
		t.Fatalf("CreateGame() Status = %q, want %q (precondition for this test)", created.Status, "waiting")
	}

	result, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(%q, %q, RolePlayer) error = %v, want nil", created.RoomCode, "Bob", err)
	}

	want := session.JoinResult{
		Role:   session.RolePlayer,
		Seat:   2,
		Color:  "white",
		Status: "active",
	}
	if result != want {
		t.Errorf("JoinGame(%q, %q, RolePlayer) = %+v, want %+v", created.RoomCode, "Bob", result, want)
	}
}

// --- 2. GetState after that join shows status=active and BOTH seats filled,
//        confirming the DB (not just the return value) was updated ----------

func TestJoinGame_SecondDistinctPlayer_PersistsToStore(t *testing.T) {
	mgr := newJoinTestManager(t)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, joinFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	if _, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatalf("JoinGame(%q, %q, RolePlayer) error = %v, want nil", created.RoomCode, "Bob", err)
	}

	snap, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}

	if snap.Status != "active" {
		t.Errorf("GetState() Status = %q, want %q (after second player joined)", snap.Status, "active")
	}

	if len(snap.Seats) != 2 {
		t.Fatalf("GetState() Seats has %d entries, want 2", len(snap.Seats))
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
		t.Errorf("GetState() Seats[seat=1].Name = %q, want %q", seat1.Name, "Alice")
	}
	if seat2.Name != "Bob" {
		t.Errorf("GetState() Seats[seat=2].Name = %q, want %q", seat2.Name, "Bob")
	}
	if seat2.Color != "white" {
		t.Errorf("GetState() Seats[seat=2].Color = %q, want %q", seat2.Color, "white")
	}
}

// --- 3. JoinGame on a nonexistent room code propagates store.ErrNotFound ----

func TestJoinGame_UnknownRoomCode_ReturnsStoreErrNotFound(t *testing.T) {
	mgr := newJoinTestManager(t)
	ctx := context.Background()

	_, err := mgr.JoinGame(ctx, "ZZZZZZ", "Bob", session.RolePlayer)
	if err == nil {
		t.Fatalf("JoinGame() error = nil, want non-nil (room code never created)")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("JoinGame() error = %v, want errors.Is(err, store.ErrNotFound)", err)
	}
}

// --- 4. Two independent fresh games each accept their own second player and
//        reach "active" with no crosstalk between games' seat assignments --

func TestJoinGame_TwoIndependentGames_NoCrosstalk(t *testing.T) {
	mgr := newJoinTestManager(t)
	ctx := context.Background()

	game1, err := mgr.CreateGame(ctx, joinFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() #1 error = %v, want nil", err)
	}
	game2, err := mgr.CreateGame(ctx, joinFixtureGameDefID, "Carol")
	if err != nil {
		t.Fatalf("CreateGame() #2 error = %v, want nil", err)
	}
	if game1.RoomCode == game2.RoomCode {
		t.Fatalf("CreateGame() produced the same RoomCode %q twice, want distinct codes", game1.RoomCode)
	}

	joinResult1, err := mgr.JoinGame(ctx, game1.RoomCode, "Bob", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(game1, %q, RolePlayer) error = %v, want nil", "Bob", err)
	}
	joinResult2, err := mgr.JoinGame(ctx, game2.RoomCode, "Dave", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(game2, %q, RolePlayer) error = %v, want nil", "Dave", err)
	}

	wantJoin1 := session.JoinResult{Role: session.RolePlayer, Seat: 2, Color: "white", Status: "active"}
	wantJoin2 := session.JoinResult{Role: session.RolePlayer, Seat: 2, Color: "white", Status: "active"}
	if joinResult1 != wantJoin1 {
		t.Errorf("JoinGame(game1, %q, RolePlayer) = %+v, want %+v", "Bob", joinResult1, wantJoin1)
	}
	if joinResult2 != wantJoin2 {
		t.Errorf("JoinGame(game2, %q, RolePlayer) = %+v, want %+v", "Dave", joinResult2, wantJoin2)
	}

	snap1, err := mgr.GetState(ctx, game1.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(game1) error = %v, want nil", err)
	}
	snap2, err := mgr.GetState(ctx, game2.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(game2) error = %v, want nil", err)
	}

	if snap1.Status != "active" {
		t.Errorf("GetState(game1) Status = %q, want %q", snap1.Status, "active")
	}
	if snap2.Status != "active" {
		t.Errorf("GetState(game2) Status = %q, want %q", snap2.Status, "active")
	}

	names1 := seatNamesBySeat(t, snap1.Seats)
	names2 := seatNamesBySeat(t, snap2.Seats)

	if names1[1] != "Alice" || names1[2] != "Bob" {
		t.Errorf("GetState(game1) seat names = %+v, want {1:Alice, 2:Bob} (no crosstalk from game2)", names1)
	}
	if names2[1] != "Carol" || names2[2] != "Dave" {
		t.Errorf("GetState(game2) seat names = %+v, want {1:Carol, 2:Dave} (no crosstalk from game1)", names2)
	}
}

// seatNamesBySeat is a small local helper turning []session.SeatInfo into a
// seat-number-keyed map for terser crosstalk assertions.
func seatNamesBySeat(t *testing.T, seats []session.SeatInfo) map[int]string {
	t.Helper()
	m := make(map[int]string, len(seats))
	for _, s := range seats {
		m[s.Seat] = s.Name
	}
	return m
}
