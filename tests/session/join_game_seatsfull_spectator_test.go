// Tests of joining a full game: a third player is refused with ErrSeatsFull, and a spectator can always watch without taking a seat.

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
// Reuses the same shape as join_game_test.go's / join_game_reconnect_test.go's
// fixture (a self-contained 3x3 orthogonal4 grid, 2 seats 1=black/2=white, one
// "stone" piece via "step"/"capture-or-empty", round-robin turn order,
// win_condition "none") — deliberately re-declared here (not imported/shared)
// so this file stands on its own, per those files' own note that other test
// files in this package are free to build their own fixtures.

const seatsFullFixtureGameDefID = "fixture-3x3-seatsfull-spectator"

const seatsFullFixtureDefJSON = `{
  "id": "fixture-3x3-seatsfull-spectator",
  "name": "Fixture 3x3 (seats-full/spectator tests)",
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

// newSeatsFullTestManager builds a fresh *catalog.Catalog with the
// seats-full/spectator-tests fixture registered, a fresh *store.Store against
// a temp-dir SQLite file, and returns a session.NewManager(st, cat) wired to
// both. Same pattern as join_game_test.go's newJoinTestManager, reproduced
// locally per this file's own self-containment choice.
func newSeatsFullTestManager(t *testing.T) *session.Manager {
	t.Helper()

	cat := catalog.NewCatalog()
	if err := cat.Register(seatsFullFixtureGameDefID, []byte(seatsFullFixtureDefJSON), nil); err != nil {
		t.Fatalf("fixture setup: catalog.Register(%q) error = %v, want nil", seatsFullFixtureGameDefID, err)
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

// seatNamesBySeat3 is a small local helper turning []session.SeatInfo into a
// seat-number-keyed map for terser assertions. (Named with a "3" suffix to
// avoid colliding with join_game_test.go's and join_game_reconnect_test.go's
// identically-purposed helpers in this same package.)
func seatNamesBySeat3(t *testing.T, seats []session.SeatInfo) map[int]string {
	t.Helper()
	m := make(map[int]string, len(seats))
	for _, s := range seats {
		m[s.Seat] = s.Name
	}
	return m
}

// --- 1 & 2. The AC itself, seats-full: a third distinct name joining as
//            role=player when both seats are taken returns ErrSeatsFull and
//            a zero-value JoinResult; the roster is left unchanged ----------

func TestJoinGame_ThirdDistinctPlayer_SeatsFull_ReturnsErrSeatsFull(t *testing.T) {
	mgr := newSeatsFullTestManager(t)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, seatsFullFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	bobJoin, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(%q, RolePlayer) error = %v, want nil", "Bob", err)
	}
	if bobJoin.Status != "active" {
		t.Fatalf("JoinGame(Bob) Status = %q, want %q (precondition: both seats now full)", bobJoin.Status, "active")
	}

	result, err := mgr.JoinGame(ctx, created.RoomCode, "Charlie", session.RolePlayer)
	if err == nil {
		t.Fatalf("JoinGame(%q, RolePlayer) error = nil, want non-nil (both seats already taken)", "Charlie")
	}
	if !errors.Is(err, session.ErrSeatsFull) {
		t.Errorf("JoinGame(%q, RolePlayer) error = %v, want errors.Is(err, session.ErrSeatsFull)", "Charlie", err)
	}

	want := session.JoinResult{}
	if result != want {
		t.Errorf("JoinGame(%q, RolePlayer) = %+v, want zero-value %+v", "Charlie", result, want)
	}

	// --- 2. GetState shows the roster unchanged: Charlie was not added
	//        anywhere, seat assignments and status are untouched -------------

	snap, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}

	if snap.Status != "active" {
		t.Errorf("GetState() Status = %q, want %q (failed join must not alter status)", snap.Status, "active")
	}
	if len(snap.Seats) != 2 {
		t.Fatalf("GetState() Seats has %d entries, want 2 (failed join must not add a seat)", len(snap.Seats))
	}

	names := seatNamesBySeat3(t, snap.Seats)
	if names[1] != "Alice" {
		t.Errorf("GetState() Seats[seat=1].Name = %q, want %q", names[1], "Alice")
	}
	if names[2] != "Bob" {
		t.Errorf("GetState() Seats[seat=2].Name = %q, want %q", names[2], "Bob")
	}
	for seat, name := range names {
		if name == "Charlie" {
			t.Errorf("GetState() Seats[seat=%d].Name = %q, Charlie must not appear anywhere in the roster after a failed seats-full join", seat, name)
		}
	}
}

// --- 3. The AC itself, spectator: joining as role=spectator on a "waiting"
//        game succeeds without occupying a seat -----------------------------

func TestJoinGame_Spectator_OnWaitingGame_SucceedsWithoutSeat(t *testing.T) {
	mgr := newSeatsFullTestManager(t)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, seatsFullFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}
	if created.Status != "waiting" {
		t.Fatalf("CreateGame() Status = %q, want %q (precondition for this test)", created.Status, "waiting")
	}

	result, err := mgr.JoinGame(ctx, created.RoomCode, "Dana", session.RoleSpectator)
	if err != nil {
		t.Fatalf("JoinGame(%q, RoleSpectator) error = %v, want nil", "Dana", err)
	}

	want := session.JoinResult{
		Role:   session.RoleSpectator,
		Seat:   0,
		Color:  "",
		Status: "waiting",
	}
	if result != want {
		t.Errorf("JoinGame(%q, RoleSpectator) = %+v, want %+v", "Dana", result, want)
	}
}

// --- 4. Spectator join on a FULL/active game also succeeds: spectators are
//        never blocked by seats being full ----------------------------------

func TestJoinGame_Spectator_OnFullActiveGame_StillSucceeds(t *testing.T) {
	mgr := newSeatsFullTestManager(t)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, seatsFullFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}
	bobJoin, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(%q, RolePlayer) error = %v, want nil", "Bob", err)
	}
	if bobJoin.Status != "active" {
		t.Fatalf("JoinGame(Bob) Status = %q, want %q (precondition: both seats now full)", bobJoin.Status, "active")
	}

	result, err := mgr.JoinGame(ctx, created.RoomCode, "Eve", session.RoleSpectator)
	if err != nil {
		t.Fatalf("JoinGame(%q, RoleSpectator) error = %v, want nil (spectators must never be blocked by full seats)", "Eve", err)
	}

	want := session.JoinResult{
		Role:   session.RoleSpectator,
		Seat:   0,
		Color:  "",
		Status: "active",
	}
	if result != want {
		t.Errorf("JoinGame(%q, RoleSpectator) = %+v, want %+v", "Eve", result, want)
	}
}

// --- 5. Spectators are never persisted as players: after one or more
//        spectator joins, GetState's Seats list is unaffected ---------------

func TestJoinGame_Spectator_NeverPersistedAsPlayer(t *testing.T) {
	mgr := newSeatsFullTestManager(t)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, seatsFullFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	spectatorNames := []string{"Dana", "Frank", "Grace"}
	for _, name := range spectatorNames {
		if _, err := mgr.JoinGame(ctx, created.RoomCode, name, session.RoleSpectator); err != nil {
			t.Fatalf("JoinGame(%q, RoleSpectator) error = %v, want nil", name, err)
		}
	}

	snap, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}

	if snap.Status != "waiting" {
		t.Errorf("GetState() Status = %q, want %q (spectator joins must not change status)", snap.Status, "waiting")
	}
	if len(snap.Seats) != 2 {
		t.Fatalf("GetState() Seats has %d entries, want 2 (spectator joins must not add seats)", len(snap.Seats))
	}

	names := seatNamesBySeat3(t, snap.Seats)
	if names[1] != "Alice" {
		t.Errorf("GetState() Seats[seat=1].Name = %q, want %q", names[1], "Alice")
	}
	if names[2] != "" {
		t.Errorf("GetState() Seats[seat=2].Name = %q, want \"\" (seat 2 must remain open; spectators never occupy a seat)", names[2])
	}
	for seat, name := range names {
		for _, spectatorName := range spectatorNames {
			if name == spectatorName {
				t.Errorf("GetState() Seats[seat=%d].Name = %q, spectator names must never appear in Seats", seat, name)
			}
		}
	}
}

// --- 6. Multiple different spectators can join the same game freely: no
//        "spectators full" concept exists ------------------------------------

func TestJoinGame_MultipleSpectators_AllSucceed(t *testing.T) {
	mgr := newSeatsFullTestManager(t)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, seatsFullFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	spectatorNames := []string{"Dana", "Frank", "Grace"}
	for _, name := range spectatorNames {
		result, err := mgr.JoinGame(ctx, created.RoomCode, name, session.RoleSpectator)
		if err != nil {
			t.Fatalf("JoinGame(%q, RoleSpectator) error = %v, want nil", name, err)
		}
		want := session.JoinResult{
			Role:   session.RoleSpectator,
			Seat:   0,
			Color:  "",
			Status: "waiting",
		}
		if result != want {
			t.Errorf("JoinGame(%q, RoleSpectator) = %+v, want %+v", name, result, want)
		}
	}
}
