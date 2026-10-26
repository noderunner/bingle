// Tests that rejoining under the same name (in any case) returns to the same seat without making a duplicate.

package session_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// --- fixture game definition -------------------------------------------------
//
// Reuses the same shape as join_game_test.go's fixture (a self-contained 3x3
// orthogonal4 grid, 2 seats 1=black/2=white, one "stone" piece via
// "step"/"capture-or-empty", round-robin turn order, win_condition "none") —
// deliberately re-declared here (not imported/shared) so this file stands on
// its own, per that file's own note that other test files in this package are
// free to build their own fixtures.

const reconnectFixtureGameDefID = "fixture-3x3-reconnect"

const reconnectFixtureDefJSON = `{
  "id": "fixture-3x3-reconnect",
  "name": "Fixture 3x3 (reconnect tests)",
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

// newReconnectTestManager builds a fresh *catalog.Catalog with the
// reconnect-tests fixture registered, a fresh *store.Store against a
// temp-dir SQLite file, and returns a session.NewManager(st, cat) wired to
// both. Same pattern as join_game_test.go's newJoinTestManager, reproduced
// locally per this file's own self-containment choice.
func newReconnectTestManager(t *testing.T) *session.Manager {
	t.Helper()

	cat := catalog.NewCatalog()
	if err := cat.Register(reconnectFixtureGameDefID, []byte(reconnectFixtureDefJSON), nil); err != nil {
		t.Fatalf("fixture setup: catalog.Register(%q) error = %v, want nil", reconnectFixtureGameDefID, err)
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

// seatNamesBySeat2 is a small local helper turning []session.SeatInfo into a
// seat-number-keyed map for terser assertions. (Named with a "2" suffix to
// avoid colliding with join_game_test.go's identically-purposed helper in
// this same package.)
func seatNamesBySeat2(t *testing.T, seats []session.SeatInfo) map[int]string {
	t.Helper()
	m := make(map[int]string, len(seats))
	for _, s := range seats {
		m[s.Seat] = s.Name
	}
	return m
}

// --- 1. The AC itself: re-joining with the exact name already on seat 1 is
//        reattached to seat 1, not assigned a new seat, not rejected --------

func TestJoinGame_ReconnectExactName_ReattachesToSeat1(t *testing.T) {
	mgr := newReconnectTestManager(t)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, reconnectFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}
	if created.Status != "waiting" {
		t.Fatalf("CreateGame() Status = %q, want %q (precondition for this test)", created.Status, "waiting")
	}
	if created.Seat != 1 {
		t.Fatalf("CreateGame() Seat = %d, want 1 (precondition for this test)", created.Seat)
	}

	result, err := mgr.JoinGame(ctx, created.RoomCode, "Alice", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(%q, %q, RolePlayer) error = %v, want nil", created.RoomCode, "Alice", err)
	}

	want := session.JoinResult{
		Role:   session.RolePlayer,
		Seat:   1,
		Color:  created.Color,
		Status: "waiting",
	}
	if result != want {
		t.Errorf("JoinGame(%q, %q, RolePlayer) = %+v, want %+v", created.RoomCode, "Alice", result, want)
	}
}

// --- 2. Case-insensitivity: "ALICE" or "alice" also reattaches to seat 1,
//        not treated as a new/different player ------------------------------

func TestJoinGame_ReconnectCaseInsensitive_ReattachesToSeat1(t *testing.T) {
	cases := []struct {
		name       string
		joinAsName string
	}{
		{name: "all_uppercase", joinAsName: "ALICE"},
		{name: "all_lowercase", joinAsName: "alice"},
		{name: "mixed_case", joinAsName: "aLiCe"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newReconnectTestManager(t)
			ctx := context.Background()

			created, err := mgr.CreateGame(ctx, reconnectFixtureGameDefID, "Alice")
			if err != nil {
				t.Fatalf("CreateGame() error = %v, want nil", err)
			}

			result, err := mgr.JoinGame(ctx, created.RoomCode, tc.joinAsName, session.RolePlayer)
			if err != nil {
				t.Fatalf("JoinGame(%q, %q, RolePlayer) error = %v, want nil", created.RoomCode, tc.joinAsName, err)
			}

			want := session.JoinResult{
				Role:   session.RolePlayer,
				Seat:   1,
				Color:  created.Color,
				Status: "waiting",
			}
			if result != want {
				t.Errorf("JoinGame(%q, %q, RolePlayer) = %+v, want %+v (case-insensitive reconnect to seat 1)", created.RoomCode, tc.joinAsName, result, want)
			}
		})
	}
}

// --- 3. No duplicate row: after one or several reconnects, GetState still
//        shows exactly seat 1 filled with "Alice" and seat 2 still open ----

func TestJoinGame_ReconnectMultipleTimes_NoDuplicateSeatOrPlayer(t *testing.T) {
	mgr := newReconnectTestManager(t)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, reconnectFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	// Reconnect several times, with varying case, to make sure repeated
	// reconnects never mutate seat assignment or create extra rows.
	reconnectNames := []string{"Alice", "ALICE", "alice", "Alice"}
	for _, name := range reconnectNames {
		if _, err := mgr.JoinGame(ctx, created.RoomCode, name, session.RolePlayer); err != nil {
			t.Fatalf("JoinGame(%q, %q, RolePlayer) error = %v, want nil", created.RoomCode, name, err)
		}
	}

	snap, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}

	if snap.Status != "waiting" {
		t.Errorf("GetState() Status = %q, want %q (no new seat should have been filled by reconnects)", snap.Status, "waiting")
	}

	if len(snap.Seats) != 2 {
		t.Fatalf("GetState() Seats has %d entries, want 2 (one per declared seat, regardless of reconnect count)", len(snap.Seats))
	}

	names := seatNamesBySeat2(t, snap.Seats)
	if names[1] != "Alice" {
		t.Errorf("GetState() Seats[seat=1].Name = %q, want %q", names[1], "Alice")
	}
	if names[2] != "" {
		t.Errorf("GetState() Seats[seat=2].Name = %q, want \"\" (seat 2 must remain open; reconnects must not create a new player row)", names[2])
	}
}

// --- 4. Reconnect works after a second player has joined too: the second
//        player's own reconnect reattaches to seat 2 (not seat 1, not
//        rejected, not treated as seats-full), Status stays "active" -------

func TestJoinGame_ReconnectAfterSecondPlayerJoined_ReattachesToSeat2(t *testing.T) {
	mgr := newReconnectTestManager(t)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, reconnectFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	bobJoin, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(%q, %q, RolePlayer) error = %v, want nil", created.RoomCode, "Bob", err)
	}
	if bobJoin.Status != "active" {
		t.Fatalf("JoinGame(Bob) Status = %q, want %q (precondition for this test)", bobJoin.Status, "active")
	}
	if bobJoin.Seat != 2 {
		t.Fatalf("JoinGame(Bob) Seat = %d, want 2 (precondition for this test)", bobJoin.Seat)
	}

	// Bob reconnects.
	result, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(%q, %q, RolePlayer) [reconnect] error = %v, want nil", created.RoomCode, "Bob", err)
	}

	want := session.JoinResult{
		Role:   session.RolePlayer,
		Seat:   2,
		Color:  bobJoin.Color,
		Status: "active",
	}
	if result != want {
		t.Errorf("JoinGame(%q, %q, RolePlayer) [reconnect] = %+v, want %+v", created.RoomCode, "Bob", result, want)
	}

	// Also confirm Alice can still reconnect to seat 1 in this same
	// now-active game, and the roster remains exactly {Alice, Bob}.
	aliceResult, err := mgr.JoinGame(ctx, created.RoomCode, "alice", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(%q, %q, RolePlayer) [reconnect] error = %v, want nil", created.RoomCode, "alice", err)
	}
	wantAlice := session.JoinResult{
		Role:   session.RolePlayer,
		Seat:   1,
		Color:  created.Color,
		Status: "active",
	}
	if aliceResult != wantAlice {
		t.Errorf("JoinGame(%q, %q, RolePlayer) [reconnect] = %+v, want %+v", created.RoomCode, "alice", aliceResult, wantAlice)
	}

	snap, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}
	names := seatNamesBySeat2(t, snap.Seats)
	if names[1] != "Alice" || names[2] != "Bob" {
		t.Errorf("GetState() seat names = %+v, want {1:Alice, 2:Bob} (reconnects must not alter roster)", names)
	}
}

// --- 5. A non-matching name, with an open seat remaining, is NOT treated as
//        a reconnect — it goes through normal new-seat assignment instead --

func TestJoinGame_NonMatchingNameWithOpenSeat_IsNotReconnect(t *testing.T) {
	mgr := newReconnectTestManager(t)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, reconnectFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	result, err := mgr.JoinGame(ctx, created.RoomCode, "Charlie", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(%q, %q, RolePlayer) error = %v, want nil", created.RoomCode, "Charlie", err)
	}

	want := session.JoinResult{
		Role:   session.RolePlayer,
		Seat:   2,
		Color:  "white",
		Status: "active",
	}
	if result != want {
		t.Errorf("JoinGame(%q, %q, RolePlayer) = %+v, want %+v (unmatched name with an open seat must be assigned a NEW seat, not treated as a reconnect)", created.RoomCode, "Charlie", result, want)
	}

	snap, err := mgr.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", created.RoomCode, err)
	}
	names := seatNamesBySeat2(t, snap.Seats)
	if names[1] != "Alice" || names[2] != "Charlie" {
		t.Errorf("GetState() seat names = %+v, want {1:Alice, 2:Charlie}", names)
	}
}
