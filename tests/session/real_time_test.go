// Tests of real-time games: the setting, the warnings, the bot on offer, and the flag
// that falls with nobody looking.
package session_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/session"
)

// rtGame is a game of Tak between Alice and Bob, in real time with no timer.
func rtGame(t *testing.T, m *session.Manager) string {
	t.Helper()
	ctx := context.Background()
	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "tak-3x3", CreatorName: "Alice", RealTime: &session.RealTimeSettings{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	return created.RoomCode
}

// rtPlay plays name's first legal move at seat, and returns how many moves the game
// then holds, the bots' answers included.
func rtPlay(t *testing.T, m *session.Manager, room, name string, seat int) int {
	t.Helper()
	ctx := context.Background()
	_, legal, err := legalSelections(ctx, m, room)
	if err != nil || len(legal) == 0 {
		t.Fatalf("legal moves of seat %d: %d, %v", seat, len(legal), err)
	}
	if _, err := m.PlayNotation(ctx, room, name, legal[0].Notation); err != nil {
		t.Fatal(err)
	}
	snap, err := m.GetState(ctx, room, nil)
	if err != nil {
		t.Fatal(err)
	}
	return snap.MoveSeq
}

// slowTurns reads the warnings waiting on a subscription, as stage numbers.
func slowTurns(events <-chan session.Event) []session.Event {
	var out []session.Event
	for {
		select {
		case ev := <-events:
			if ev.Type == "slow_turn" {
				out = append(out, ev)
			}
		default:
			return out
		}
	}
}

func rtStage(t *testing.T, m *session.Manager, room string) *session.RealTimeWait {
	t.Helper()
	snap, err := m.GetState(context.Background(), room, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.RealTime == nil {
		t.Fatal("the game is not real-time")
	}
	return snap.RealTime.Wait
}

func TestRealTime_SettingsTakeDefaultsAndBounds(t *testing.T) {
	m, _ := newTimedManager(t)
	ctx := context.Background()
	room := rtGame(t, m)
	snap, err := m.GetState(ctx, room, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.RealTime.FirstWarningSeconds != 120 || snap.RealTime.SecondWarningSeconds != 300 {
		t.Errorf("defaults = %+v, want 120 and 300", snap.RealTime.RealTimeSettings)
	}
	for _, bad := range []session.RealTimeSettings{{FirstWarningSeconds: 29}, {SecondWarningSeconds: 3601}, {FirstWarningSeconds: -1}} {
		_, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "tak-3x3", CreatorName: "Alice", RealTime: &bad})
		if !errors.Is(err, session.ErrInvalidRealTime) {
			t.Errorf("%+v: err = %v, want ErrInvalidRealTime", bad, err)
		}
	}
	plain, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "tak-3x3", CreatorName: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	if snap, _ := m.GetState(ctx, plain.RoomCode, nil); snap.RealTime != nil {
		t.Errorf("an ordinary game has real_time %+v", snap.RealTime)
	}
}

// Each warning comes once, at its time, and a decision the game accepts restarts them.
func TestRealTime_WarningsComeOnceAndAMoveRestartsThem(t *testing.T) {
	m, clock := newTimedManager(t)
	ctx := context.Background()
	room := rtGame(t, m)
	events, stop := m.Subscribe(room)
	defer stop()

	clock.Advance(100 * time.Second)
	m.RealTimeDue(ctx)
	if got := slowTurns(events); len(got) != 0 {
		t.Fatalf("a warning before its time: %+v", got)
	}
	clock.Advance(30 * time.Second)
	m.RealTimeDue(ctx)
	m.RealTimeDue(ctx)
	if got := slowTurns(events); len(got) != 1 {
		t.Fatalf("%d warnings after the first time, want 1", len(got))
	}
	if w := rtStage(t, m, room); w == nil || w.Seat != 1 || w.Stage != 1 {
		t.Fatalf("wait = %+v, want seat 1 at stage 1", w)
	}
	clock.Advance(5 * time.Minute)
	m.RealTimeDue(ctx)
	m.RealTimeDue(ctx)
	if got := slowTurns(events); len(got) != 1 {
		t.Fatalf("%d warnings after the second time, want 1", len(got))
	}
	if w := rtStage(t, m, room); w.Stage != 2 {
		t.Fatalf("stage = %d, want 2", w.Stage)
	}

	// The database stamps a move with the real clock, so put ours back to match.
	clock.Set(time.Now())
	rtPlay(t, m, room, "Alice", 1)
	if w := rtStage(t, m, room); w == nil || w.Seat != 2 || w.Stage != 0 {
		t.Fatalf("after the move wait = %+v, want seat 2 at stage 0", w)
	}
}

// The warnings are for a game with no timer, and a table with somebody else at it.
func TestRealTime_NoWarningsWithATimerOrAlone(t *testing.T) {
	m, clock := newTimedManager(t)
	ctx := context.Background()
	timed, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "tak-3x3", CreatorName: "Alice", ClockSeconds: []int{3600, 3600}, RealTime: &session.RealTimeSettings{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JoinGame(ctx, timed.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	alone, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "tak-3x3", CreatorName: "Alice", Opponent: "random", RealTime: &session.RealTimeSettings{}})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(30 * time.Minute)
	for _, room := range []string{timed.RoomCode, alone.RoomCode} {
		if w := rtStage(t, m, room); w != nil {
			t.Errorf("room %s waits on %+v", room, w)
		}
	}
}

// A flag falls with nobody asking about the game, because the game is real-time.
func TestRealTime_AFlagFallsWithNobodyLooking(t *testing.T) {
	m, clock := newTimedManager(t)
	ctx := context.Background()
	for _, realTime := range []bool{true, false} {
		opts := session.CreateOptions{GameDefID: "tak-3x3", CreatorName: "Alice", ClockSeconds: []int{60, 60}}
		if realTime {
			opts.RealTime = &session.RealTimeSettings{}
		}
		created, err := m.CreateGameWithOptions(ctx, opts)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
			t.Fatal(err)
		}
		events, stop := m.Subscribe(created.RoomCode)
		defer stop()
		clock.Advance(2 * time.Minute)
		m.RealTimeDue(ctx)
		told := false
		for len(events) > 0 {
			if ev := <-events; ev.Type == "state" {
				told = true
			}
		}
		if told != realTime {
			t.Errorf("real-time %v: the room heard of the flag: %v", realTime, told)
		}
		clock.Set(time.Now())
	}
}

// The offer: past the second warning, for another connected person, for one turn.
func TestRealTime_TheBotIsOnOfferToAnotherConnectedPlayer(t *testing.T) {
	m, clock := newTimedManager(t)
	ctx := context.Background()
	room := rtGame(t, m)
	defer m.TrackConnection(room, 1)()

	if _, err := m.AutoPlayImpulse(ctx, room, "Bob"); !errors.Is(err, session.ErrNoAutoPlayOffer) {
		t.Errorf("before any warning: %v, want ErrNoAutoPlayOffer", err)
	}
	clock.Advance(3 * time.Minute)
	if _, err := m.AutoPlayImpulse(ctx, room, "Bob"); !errors.Is(err, session.ErrNoAutoPlayOffer) {
		t.Errorf("past the first warning only: %v, want ErrNoAutoPlayOffer", err)
	}
	clock.Advance(5 * time.Minute)
	if _, err := m.AutoPlayImpulse(ctx, room, "Alice"); !errors.Is(err, session.ErrOwnTurnOffer) {
		t.Errorf("the slow player asking: %v, want ErrOwnTurnOffer", err)
	}
	if _, err := m.AutoPlayImpulse(ctx, room, "Bob"); !errors.Is(err, session.ErrNotConnected) {
		t.Errorf("Bob with no socket: %v, want ErrNotConnected", err)
	}
	defer m.TrackConnection(room, 2)()
	if _, err := m.AutoPlayImpulse(ctx, room, "Nobody"); !errors.Is(err, session.ErrNotAPlayer) {
		t.Errorf("a stranger: %v, want ErrNotAPlayer", err)
	}
	snap, err := m.AutoPlayImpulse(ctx, room, "Bob")
	if err != nil {
		t.Fatal(err)
	}
	if snap.MoveSeq != 1 || snap.ImpulseSeat == nil || *snap.ImpulseSeat != 2 {
		t.Fatalf("after the offer: seq %d, turn %v, want one move and Bob to play", snap.MoveSeq, snap.ImpulseSeat)
	}
	moves, err := m.ListMoves(ctx, room, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !moves[0].AutoPlay || moves[0].Seat != 1 {
		t.Errorf("the move = %+v, want seat 1's and marked auto-play", moves[0])
	}
}

// A connected player who is only slow gets one turn played, and the wait starts again.
func TestRealTime_AConnectedPlayerGetsOneTurnPlayed(t *testing.T) {
	m, clock := newTimedManager(t)
	ctx := context.Background()
	room := rtGame(t, m)
	defer m.TrackConnection(room, 1)()
	defer m.TrackConnection(room, 2)()
	clock.Advance(8 * time.Minute)
	if _, err := m.AutoPlayImpulse(ctx, room, "Bob"); err != nil {
		t.Fatal(err)
	}
	clock.Set(time.Now())
	if seq := rtPlay(t, m, room, "Bob", 2); seq != 2 {
		t.Errorf("after Bob's move seq = %d, want 2: the bot went on playing for a connected Alice", seq)
	}
}

// An absent player's turns are played with no wait, until they come back.
func TestRealTime_ABotGoesOnPlayingForAnAbsentPlayerUntilTheyReconnect(t *testing.T) {
	m, clock := newTimedManager(t)
	ctx := context.Background()
	room := rtGame(t, m)
	defer m.TrackConnection(room, 2)()
	clock.Advance(8 * time.Minute)
	if _, err := m.AutoPlayImpulse(ctx, room, "Bob"); err != nil {
		t.Fatal(err)
	}
	clock.Set(time.Now())

	if seq := rtPlay(t, m, room, "Bob", 2); seq != 3 {
		t.Fatalf("after Bob's move seq = %d, want 3: the bot answers for Alice at once", seq)
	}
	stop := m.TrackConnection(room, 1)
	defer stop()
	if seq := rtPlay(t, m, room, "Bob", 2); seq != 4 {
		t.Fatalf("after Alice came back seq = %d, want 4: her turn is hers again", seq)
	}
}
