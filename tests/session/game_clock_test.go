// Tests of the game clock (a chess clock): the time each player has left, the
// charge their turn takes, and the forfeit when a clock runs out.

package session_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/session"
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
	if snap.Clock == nil || snap.Clock.Running != 0 || snap.TurnDeadline != nil {
		t.Fatalf("waiting: clock %+v deadline %v", snap.Clock, snap.TurnDeadline)
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
	if snap.Clock.Running != 1 || snap.TurnDeadline == nil {
		t.Fatalf("after the game starts: running %d deadline %v", snap.Clock.Running, snap.TurnDeadline)
	}
	if left := snap.TurnDeadline.Sub(snap.ServerTime); left < 290*time.Second || left > 301*time.Second {
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
	if left := snap.TurnDeadline.Sub(snap.ServerTime); left < 590*time.Second || left > 601*time.Second {
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
	if snap.TurnDeadline != nil {
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
