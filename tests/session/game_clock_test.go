// Tests of the game clock (a chess clock): the time each player has left, the
// charge their turn takes, and the forfeit when a clock runs out.

package session_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/blooms"
	"github.com/noderunner/bingle/internal/games/classics"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// clockLeft is the seconds the snapshot says a seat has left.
func clockLeft(t *testing.T, snap session.StateSnapshot, seat int) float64 {
	t.Helper()
	if snap.Clock == nil {
		t.Fatalf("the game has no clock")
	}
	for _, s := range snap.Clock.Seats {
		if s.Seat == seat {
			return s.Left
		}
	}
	t.Fatalf("the clock holds no seat %d (%+v)", seat, snap.Clock.Seats)
	return 0
}

// A game clock gives each player their own store of time: it is reported per
// seat, only the player to move counts down, and a turn costs the mover the
// time they took over it.
func TestGameClock_OnlyTheMoversClockRuns(t *testing.T) {
	m, clock := newTimedManager(t)
	ctx := context.Background()

	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{
		GameDefID: "tak-3x3", CreatorName: "Alice", ClockSeconds: []int{300, 600},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Waiting for an opponent: the clock is reported but nothing runs.
	snap, _ := m.GetState(ctx, created.RoomCode, nil)
	if snap.Clock == nil || snap.Clock.Running != 0 || snap.ImpulseDeadline != nil {
		t.Fatalf("waiting: clock %+v deadline %v", snap.Clock, snap.ImpulseDeadline)
	}
	if got := clockLeft(t, snap, 1); got != 300 {
		t.Errorf("seat 1 starts with %v seconds, want 300", got)
	}
	if got := clockLeft(t, snap, 2); got != 600 {
		t.Errorf("seat 2 starts with %v seconds, want 600 (the two sides may differ)", got)
	}

	clock.Set(time.Now())
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	snap, _ = m.GetState(ctx, created.RoomCode, nil)
	if snap.Clock.Running != 1 || snap.ImpulseDeadline == nil {
		t.Fatalf("after the game starts: running %d deadline %v", snap.Clock.Running, snap.ImpulseDeadline)
	}
	if left := snap.ImpulseDeadline.Sub(snap.ServerTime); left < 290*time.Second || left > 301*time.Second {
		t.Errorf("seat 1 has %v to move, want about its whole clock", left)
	}

	// Alice thinks for a minute: her clock, and only hers, pays for it. (The
	// game's first turn began when the database stamped the join, by the real
	// clock, so the time it took is measured from now.)
	clock.Set(time.Now().Add(60 * time.Second))
	if _, err := m.PlayNotation(ctx, created.RoomCode, "Alice", "a1"); err != nil {
		t.Fatal(err)
	}
	snap, _ = m.GetState(ctx, created.RoomCode, nil)
	if got := clockLeft(t, snap, 1); got < 235 || got > 245 {
		t.Errorf("seat 1 has %v seconds left after a minute's thought, want about 240", got)
	}
	if got := clockLeft(t, snap, 2); got != 600 {
		t.Errorf("seat 2 has %v seconds left without having moved, want 600", got)
	}
	if snap.Clock.Running != 2 {
		t.Errorf("seat %d's clock runs after Alice moved, want seat 2's", snap.Clock.Running)
	}
	// And the deadline is now Bob's whole clock, not Alice's remainder.
	if left := snap.ImpulseDeadline.Sub(snap.ServerTime); left < 590*time.Second || left > 601*time.Second {
		t.Errorf("seat 2 has %v to move, want about 600s", left)
	}
}

// The first player whose clock runs out forfeits: the game ends on time, the
// opponent wins, and a move that arrives late is refused.
func TestGameClock_RunningOutForfeitsTheGame(t *testing.T) {
	m, clock := newTimedManager(t)
	ctx := context.Background()
	created, _ := m.CreateGameWithOptions(ctx, session.CreateOptions{
		GameDefID: "tak-3x3", CreatorName: "Alice", ClockSeconds: []int{60, 60},
	})
	clock.Set(time.Now())
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	// Two quick moves each way leave both clocks nearly full.
	clock.Set(time.Now().Add(5 * time.Second))
	if _, err := m.PlayNotation(ctx, created.RoomCode, "Alice", "a1"); err != nil {
		t.Fatal(err)
	}
	clock.Advance(5 * time.Second)
	if _, err := m.PlayNotation(ctx, created.RoomCode, "Bob", "c3"); err != nil {
		t.Fatal(err)
	}
	snap, _ := m.GetState(ctx, created.RoomCode, nil)
	if got := clockLeft(t, snap, 1); got < 50 || got > 56 {
		t.Fatalf("seat 1 has %v seconds left, want about 55", got)
	}

	// Alice then lets her clock run out.
	clock.Advance(56 * time.Second)
	if _, err := m.PlayNotation(ctx, created.RoomCode, "Alice", "b2"); !errors.Is(err, session.ErrGameNotActive) {
		t.Fatalf("a move after the clock ran out: %v, want ErrGameNotActive", err)
	}
	snap, _ = m.GetState(ctx, created.RoomCode, nil)
	if snap.Status != "finished" || snap.EndReason != "timeout" || snap.WinnerSeat == nil || *snap.WinnerSeat != 2 {
		t.Errorf("after the flag fell: status %q reason %q winner %v, want finished/timeout/seat 2", snap.Status, snap.EndReason, snap.WinnerSeat)
	}
	if snap.Clock == nil || snap.Clock.Running != 0 {
		t.Errorf("a finished game still has a clock running: %+v", snap.Clock)
	}
	if snap.ImpulseDeadline != nil {
		t.Error("a finished game has a deadline")
	}
	// Bob's clock was never touched by Alice's loss.
	if got := clockLeft(t, snap, 2); got < 50 || got > 56 {
		t.Errorf("seat 2 has %v seconds left, want about 55", got)
	}
}

// A clock is for two players, must be a sensible length, and is not asked for
// together with a time per move.
func TestGameClock_IsValidated(t *testing.T) {
	m, _ := newTimedManager(t)
	ctx := context.Background()
	for name, opts := range map[string]session.CreateOptions{
		"no time at all":      {GameDefID: "tak-3x3", CreatorName: "X", ClockSeconds: []int{0, 300}},
		"a negative time":     {GameDefID: "tak-3x3", CreatorName: "X", ClockSeconds: []int{-1, 300}},
		"longer than the max": {GameDefID: "tak-3x3", CreatorName: "X", ClockSeconds: []int{session.MaxClockSeconds + 1, 300}},
		"one time for two":    {GameDefID: "tak-3x3", CreatorName: "X", ClockSeconds: []int{300}},
		"three times":         {GameDefID: "tak-3x3", CreatorName: "X", ClockSeconds: []int{300, 300, 300}},
		"and a time per move": {GameDefID: "tak-3x3", CreatorName: "X", ClockSeconds: []int{300, 300}, MoveTimeSeconds: 60},
	} {
		if _, err := m.CreateGameWithOptions(ctx, opts); !errors.Is(err, session.ErrInvalidClock) {
			t.Errorf("%s: %v, want ErrInvalidClock", name, err)
		}
	}
	// A game without a clock says so, and never runs out.
	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "tak-3x3", CreatorName: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	snap, _ := m.GetState(ctx, created.RoomCode, nil)
	if snap.Clock != nil {
		t.Errorf("a game without a clock reports %+v", snap.Clock)
	}
}

// What a bot takes over its turn comes off the bot's own clock, and an
// observation tells the player to move how long they have left.
func TestGameClock_BotsAndObservations(t *testing.T) {
	m, clock := newTimedManager(t)
	ctx := context.Background()
	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{
		GameDefID: "tak-3x3", CreatorName: "Alice", Opponent: "random", ClockSeconds: []int{120, 120},
	})
	if err != nil {
		t.Fatal(err)
	}
	clock.Set(time.Now().Add(10 * time.Second))
	out, err := m.PlayNotation(ctx, created.RoomCode, "Alice", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.BotMoves) != 1 {
		t.Fatalf("bot replies = %d, want 1", len(out.BotMoves))
	}
	snap, _ := m.GetState(ctx, created.RoomCode, nil)
	if got := clockLeft(t, snap, 1); got < 105 || got > 112 {
		t.Errorf("Alice has %v seconds left after ten seconds' thought, want about 110", got)
	}
	// The bot answered at once, so its clock is barely touched.
	if got := clockLeft(t, snap, 2); got < 115 {
		t.Errorf("the bot has %v seconds left, want nearly all of 120", got)
	}
	obs, err := m.Observe(ctx, created.RoomCode, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if obs.SecondsLeft == nil {
		t.Fatal("the observation says nothing about the time left")
	}
	if *obs.SecondsLeft < 100 || *obs.SecondsLeft > 112 {
		t.Errorf("the observation says %d seconds left, want about 110", *obs.SecondsLeft)
	}
}

// A pie-rule swap moves each player to the other seat, and their clock goes
// with them: the stores the seats show after the swap are the ones the two
// players had before it.
func TestGameClock_PieRuleSwapMovesTheClocksWithThePlayers(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "pieclock.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cat := catalog.NewCatalog()
	for _, mod := range classics.Modules() {
		if err := cat.RegisterModule(mod); err != nil {
			t.Fatal(err)
		}
	}
	m := session.NewManager(st, cat)
	clock := &fakeClock{now: time.Now()}
	m.SetClock(clock.Now)
	ctx := context.Background()

	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{
		GameDefID: "yavalath", CreatorName: "Alice", ClockSeconds: []int{300, 600},
		Options: map[string]any{"pie-rule": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	// Alice thinks a minute over her opening; Bob half a minute over the swap.
	clock.Set(time.Now().Add(60 * time.Second))
	if _, err := m.PlayNotation(ctx, created.RoomCode, "Alice", "e5"); err != nil {
		t.Fatal(err)
	}
	clock.Set(time.Now().Add(90 * time.Second))
	if _, err := m.PlayNotation(ctx, created.RoomCode, "Bob", "swap"); err != nil {
		t.Fatal(err)
	}
	snap, _ := m.GetState(ctx, created.RoomCode, nil)
	if len(snap.Seats) != 2 || snap.Seats[0].Name != "Bob" || snap.Seats[1].Name != "Alice" {
		t.Fatalf("after the swap the seats are %+v, want Bob in 1 and Alice in 2", snap.Seats)
	}
	if got := clockLeft(t, snap, 1); got < 565 || got > 575 {
		t.Errorf("seat 1 (Bob) has %v seconds, want his 600 less the half-minute swap", got)
	}
	if got := clockLeft(t, snap, 2); got < 235 || got > 245 {
		t.Errorf("seat 2 (Alice) has %v seconds, want her 300 less the minute's opening", got)
	}
	if snap.Clock.Running != 2 {
		t.Errorf("the clock running is seat %d's, want 2 (Alice answers)", snap.Clock.Running)
	}
}

// clockGame starts Alice and Bob on a clock, made by options, in a manager over
// the classics.
func clockGame(t *testing.T, opts session.CreateOptions) (*session.Manager, *fakeClock, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "clock.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cat := catalog.NewCatalog()
	for _, mod := range classics.Modules() {
		if err := cat.RegisterModule(mod); err != nil {
			t.Fatal(err)
		}
	}
	m := session.NewManager(st, cat)
	clock := &fakeClock{now: time.Now()}
	m.SetClock(clock.Now)
	opts.CreatorName = "Alice"
	created, err := m.CreateGameWithOptions(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JoinGame(context.Background(), created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	return m, clock, created.RoomCode
}

// near fails unless the seat has about want seconds left.
func near(t *testing.T, snap session.StateSnapshot, seat int, want float64) {
	t.Helper()
	if got := clockLeft(t, snap, seat); got < want-5 || got > want+5 {
		t.Errorf("seat %d has %v seconds, want about %v", seat, got, want)
	}
}

// A restart costs nobody time: a player whose turn was running when the server
// went down does not lose on time because of it, and is charged only from when
// the server came back.
func TestGameClock_ARestartCostsNobodyTime(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "restart.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cat := catalog.NewCatalog()
	for _, mod := range classics.Modules() {
		if err := cat.RegisterModule(mod); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	before := session.NewManager(st, cat)
	clock := &fakeClock{now: time.Now()}
	before.SetClock(clock.Now)
	created, err := before.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "yavalath", CreatorName: "Alice", ClockSeconds: []int{300, 300}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := before.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}

	// Down for ten minutes, twice Alice's whole clock.
	clock.Advance(10 * time.Minute)
	after := session.NewManager(st, cat)
	after.SetClock(clock.Now)
	snap, err := after.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Status != "active" {
		t.Fatalf("after the restart the game is %q (%s), want it still going", snap.Status, snap.EndReason)
	}
	if left := snap.ImpulseDeadline.Sub(clock.Now()); left < 295*time.Second || left > 300*time.Second {
		t.Errorf("Alice has %v to move after the restart, want her whole five minutes", left)
	}

	// Time after the restart counts as usual.
	clock.Advance(time.Minute)
	if _, err := after.PlayNotation(ctx, created.RoomCode, "Alice", "e5"); err != nil {
		t.Fatal(err)
	}
	snap, _ = after.GetState(ctx, created.RoomCode, nil)
	near(t, snap, 1, 240)

	// And a flag that falls after the restart still falls.
	clock.Advance(6 * time.Minute)
	snap, _ = after.GetState(ctx, created.RoomCode, nil)
	if snap.Status != "finished" || snap.EndReason != "timeout" {
		t.Errorf("Bob's clock ran out and the game is %q (%s)", snap.Status, snap.EndReason)
	}
}

// A per-move limit is measured the same way.
func TestTimeLimit_ARestartCostsNobodyTime(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "restart.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cat := catalog.NewCatalog()
	for _, mod := range classics.Modules() {
		if err := cat.RegisterModule(mod); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	before := session.NewManager(st, cat)
	clock := &fakeClock{now: time.Now()}
	before.SetClock(clock.Now)
	created, err := before.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "yavalath", CreatorName: "Alice", MoveTimeSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := before.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	clock.Advance(10 * time.Minute)
	after := session.NewManager(st, cat)
	after.SetClock(clock.Now)
	snap, err := after.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Status != "active" {
		t.Errorf("after the restart the game is %q (%s), want it still going", snap.Status, snap.EndReason)
	}
}

// The increment is added when a player hands the turn on, after their time is
// charged, and is reported with the clock.
func TestGameClock_IncrementIsAddedWhenTheTurnPasses(t *testing.T) {
	m, clock, room := clockGame(t, session.CreateOptions{GameDefID: "yavalath", ClockSeconds: []int{300, 300}, ClockIncrementSeconds: 30})
	ctx := context.Background()

	snap, _ := m.GetState(ctx, room, nil)
	if snap.Clock.Increment != 30 {
		t.Errorf("the clock reports an increment of %d, want 30", snap.Clock.Increment)
	}
	clock.Advance(time.Minute)
	if _, err := m.PlayNotation(ctx, room, "Alice", "e5"); err != nil {
		t.Fatal(err)
	}
	snap, _ = m.GetState(ctx, room, nil)
	near(t, snap, 1, 270)
	near(t, snap, 2, 300)
	if left := snap.ImpulseDeadline.Sub(clock.Now()); left < 295*time.Second || left > 300*time.Second {
		t.Errorf("Bob has %v to move, want his own five minutes", left)
	}
}

// A clock can run past what it started with, and a game without an increment is
// unchanged.
func TestGameClock_NoIncrementAddsNothing(t *testing.T) {
	m, clock, room := clockGame(t, session.CreateOptions{GameDefID: "yavalath", ClockSeconds: []int{300, 300}})
	ctx := context.Background()
	clock.Advance(time.Minute)
	if _, err := m.PlayNotation(ctx, room, "Alice", "e5"); err != nil {
		t.Fatal(err)
	}
	snap, _ := m.GetState(ctx, room, nil)
	near(t, snap, 1, 240)
	if snap.Clock.Increment != 0 {
		t.Errorf("a clock without an increment reports %d", snap.Clock.Increment)
	}
}

// An increment needs a clock and a sensible size.
func TestGameClock_IncrementIsValidated(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "inc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cat := catalog.NewCatalog()
	for _, mod := range classics.Modules() {
		if err := cat.RegisterModule(mod); err != nil {
			t.Fatal(err)
		}
	}
	m := session.NewManager(st, cat)
	for name, opts := range map[string]session.CreateOptions{
		"no clock": {ClockIncrementSeconds: 5},
		"negative": {ClockSeconds: []int{60, 60}, ClockIncrementSeconds: -1},
		"too long": {ClockSeconds: []int{60, 60}, ClockIncrementSeconds: session.MaxClockIncrementSeconds + 1},
	} {
		opts.GameDefID, opts.CreatorName = "yavalath", "Alice"
		if _, err := m.CreateGameWithOptions(context.Background(), opts); !errors.Is(err, session.ErrInvalidClock) {
			t.Errorf("%s: %v, want ErrInvalidClock", name, err)
		}
	}
}

// A turn of several decisions earns the increment once, on the decision that
// hands the turn on.
func TestGameClock_ATurnOfSeveralDecisionsEarnsOnce(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "blooms.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(blooms.Module()); err != nil {
		t.Fatal(err)
	}
	m := session.NewManager(st, cat)
	ctx := context.Background()
	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "blooms", CreatorName: "Alice", ClockSeconds: []int{300, 300}, ClockIncrementSeconds: 30})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	playFirst := func(who string, seat int) session.MoveOutcome {
		t.Helper()
		_, legal, err := m.LegalMoves(ctx, created.RoomCode, seat)
		if err != nil || len(legal) == 0 {
			t.Fatalf("legal moves of seat %d: %d, %v", seat, len(legal), err)
		}
		out, err := m.PlayNotation(ctx, created.RoomCode, who, legal[0].Notation)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	// Alice's opening is a single stone, and hands the turn on.
	if out := playFirst("Alice", 1); out.NextImpulseSeat != 2 {
		t.Fatalf("after the opening seat %d is next", out.NextImpulseSeat)
	}
	// Bob's first stone leaves the turn with him: nothing earned yet.
	if out := playFirst("Bob", 2); out.NextImpulseSeat != 2 {
		t.Fatalf("after one stone of two seat %d is next", out.NextImpulseSeat)
	}
	snap, _ := m.GetState(ctx, created.RoomCode, nil)
	near(t, snap, 2, 300)
	// His second does.
	if out := playFirst("Bob", 2); out.NextImpulseSeat != 1 {
		t.Fatalf("after two stones seat %d is next", out.NextImpulseSeat)
	}
	snap, _ = m.GetState(ctx, created.RoomCode, nil)
	near(t, snap, 2, 330)
}
