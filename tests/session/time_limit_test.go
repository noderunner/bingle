package session_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/tak"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// fakeClock is a manually advanced time source.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Set(t time.Time) {
	c.mu.Lock()
	c.now = t
	c.mu.Unlock()
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// newTimedManager returns a manager over a Tak catalog whose clock starts
// at the real current time (the database stamps rows with the real clock).
func newTimedManager(t *testing.T) (*session.Manager, *fakeClock) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "timed.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(tak.Module()); err != nil {
		t.Fatal(err)
	}
	m := session.NewManager(st, cat)
	clock := &fakeClock{now: time.Now()}
	m.SetClock(clock.Now)
	return m, clock
}

func TestTimeLimit_ClockRunsOnlyWhileActive(t *testing.T) {
	m, clock := newTimedManager(t)
	ctx := context.Background()

	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "tak-3x3", CreatorName: "Alice", MoveTimeSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	// Waiting for an opponent: no deadline, and no amount of waiting ends it.
	clock.Advance(10 * time.Minute)
	snap, err := m.GetState(ctx, created.RoomCode, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Status != "waiting" || snap.TurnDeadline != nil || snap.MoveTimeLimit != 60 {
		t.Fatalf("waiting snapshot = status %q deadline %v limit %d", snap.Status, snap.TurnDeadline, snap.MoveTimeLimit)
	}

	// Bob joins (the database stamps the turn start with the real time, so
	// bring the fake clock back in step first): the clock starts.
	clock.Set(time.Now())
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	snap, _ = m.GetState(ctx, created.RoomCode, nil)
	if snap.Status != "active" || snap.TurnDeadline == nil {
		t.Fatalf("active snapshot = status %q deadline %v", snap.Status, snap.TurnDeadline)
	}
	if left := snap.TurnDeadline.Sub(snap.ServerTime); left < 50*time.Second || left > 61*time.Second {
		t.Errorf("deadline is %v after server time, want about 60s", left)
	}
}

func TestTimeLimit_PlayerWhoRunsOutLosesOnTime(t *testing.T) {
	m, clock := newTimedManager(t)
	ctx := context.Background()

	created, _ := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "tak-3x3", CreatorName: "Alice", MoveTimeSeconds: 60})
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}

	// Alice moves in time...
	clock.Advance(30 * time.Second)
	if _, err := m.PlayNotation(ctx, created.RoomCode, "Alice", "a1"); err != nil {
		t.Fatalf("move within the limit: %v", err)
	}

	// ... but Bob lets the clock run out.
	clock.Advance(2 * time.Minute)
	sub, unsub := m.Subscribe(created.RoomCode)
	defer unsub()

	if _, err := m.PlayNotation(ctx, created.RoomCode, "Bob", "c3"); !errors.Is(err, session.ErrGameNotActive) {
		t.Fatalf("late move: %v, want ErrGameNotActive", err)
	}
	snap, _ := m.GetState(ctx, created.RoomCode, nil)
	if snap.Status != "finished" || snap.EndReason != "timeout" || snap.WinnerSeat == nil || *snap.WinnerSeat != 1 {
		t.Errorf("after timeout: status %q reason %q winner %v, want finished/timeout/seat 1", snap.Status, snap.EndReason, snap.WinnerSeat)
	}
	if snap.TurnDeadline != nil {
		t.Error("a finished game has no deadline")
	}

	// Subscribers learn about it.
	select {
	case ev := <-sub:
		if ev.Type != "state" {
			t.Errorf("event type = %q, want state", ev.Type)
		}
	case <-time.After(time.Second):
		t.Error("no state event was broadcast for the timeout")
	}
}

func TestTimeLimit_ReadingAnOverdueGameEndsIt(t *testing.T) {
	m, clock := newTimedManager(t)
	ctx := context.Background()
	created, _ := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "tak-3x3", CreatorName: "Alice", MoveTimeSeconds: 30})
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	clock.Advance(31 * time.Second)

	obs, err := m.Observe(ctx, created.RoomCode, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	// Alice (seat 1) was to move and ran out: Bob wins.
	if !obs.Done || obs.EndReason != "timeout" || obs.Result != "loss" || obs.Reward != -1 {
		t.Errorf("observation = done %v reason %q result %q reward %v", obs.Done, obs.EndReason, obs.Result, obs.Reward)
	}
}

func TestTimeLimit_UntimedGamesNeverExpireAndLimitsAreValidated(t *testing.T) {
	m, clock := newTimedManager(t)
	ctx := context.Background()

	created, _ := m.CreateGame(ctx, "tak-3x3", "Alice")
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	clock.Advance(1000 * time.Hour)
	snap, _ := m.GetState(ctx, created.RoomCode, nil)
	if snap.Status != "active" || snap.TurnDeadline != nil {
		t.Errorf("untimed game = status %q deadline %v", snap.Status, snap.TurnDeadline)
	}

	for _, bad := range []int{-1, session.MaxMoveTimeSeconds + 1} {
		_, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "tak-3x3", CreatorName: "X", MoveTimeSeconds: bad})
		if !errors.Is(err, session.ErrInvalidTimeLimit) {
			t.Errorf("limit %d: %v, want ErrInvalidTimeLimit", bad, err)
		}
	}
}

func TestTimeLimit_BotsAreNotSlowedByTheClock(t *testing.T) {
	m, _ := newTimedManager(t)
	ctx := context.Background()
	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{
		GameDefID: "tak-3x3", CreatorName: "Alice", Opponent: "random", MoveTimeSeconds: 120,
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := m.PlayNotation(ctx, created.RoomCode, "Alice", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.BotMoves) != 1 {
		t.Errorf("bot replies = %d, want 1", len(out.BotMoves))
	}
}
