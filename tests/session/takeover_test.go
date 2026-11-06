// Tests of taking over a seat: the vote to replace a player, and clocks at tables of
// three or more, where a bot takes the seat of a player who runs out of time.
package session_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/nyet"
	"github.com/noderunner/bingle/internal/games/tak"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// bigTableManager has Nyet, whose tables seat three to five, and Tak for two seats.
func bigTableManager(t *testing.T) (*session.Manager, *fakeClock) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "takeover.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cat := catalog.NewCatalog()
	for _, mod := range []catalog.Module{nyet.Module(), tak.Module()} {
		if err := cat.RegisterModule(mod); err != nil {
			t.Fatal(err)
		}
	}
	m := session.NewManager(st, cat)
	clock := &fakeClock{now: time.Now()}
	m.SetClock(clock.Now)
	m.SetConnectionTimes(session.ConnectionTimes{Grace: 10 * time.Millisecond})
	return m, clock
}

var tableNames = []string{"Ann", "Ben", "Cal", "Dee", "Eve"}

// table is a game of Nyet for len(names) with everybody seated, in the order of names.
func table(t *testing.T, m *session.Manager, opts session.CreateOptions, names ...string) string {
	t.Helper()
	ctx := context.Background()
	opts.CreatorName = names[0]
	if opts.GameDefID == "" {
		opts.GameDefID = map[int]string{3: "nyet-3", 4: "nyet", 5: "nyet-5"}[len(names)]
	}
	created, err := m.CreateGameWithOptions(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names[1:] {
		if _, err := m.JoinGame(ctx, created.RoomCode, name, session.RolePlayer); err != nil {
			t.Fatal(err)
		}
	}
	return created.RoomCode
}

// connect opens a socket for each seat, 1 up to n, and returns what closes each.
func connect(t *testing.T, m *session.Manager, room string, n int) map[int]func() {
	t.Helper()
	out := map[int]func(){}
	for seat := 1; seat <= n; seat++ {
		out[seat] = m.TrackConnection(room, seat)
		t.Cleanup(out[seat])
	}
	return out
}

func stateOf(t *testing.T, m *session.Manager, room string) session.StateSnapshot {
	t.Helper()
	snap, err := m.GetState(context.Background(), room, nil)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func seatOf(t *testing.T, snap session.StateSnapshot, seat int) session.SeatInfo {
	t.Helper()
	for _, s := range snap.Seats {
		if s.Seat == seat {
			return s
		}
	}
	t.Fatalf("no seat %d", seat)
	return session.SeatInfo{}
}

// waitDisconnected waits for a seat's grace period to end.
func waitDisconnected(t *testing.T, m *session.Manager, room string, seat int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		connected := false
		for _, s := range m.ConnectedSeats(room) {
			connected = connected || s == seat
		}
		if !connected {
			return
		}
	}
	t.Fatalf("seat %d still shows as connected", seat)
}

var realTime = &session.RealTimeSettings{}

// The table in the plan: how many votes it takes at each size.
func TestVoteKick_TakesMoreThanHalfOfThoseCounted(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		people int
		needed int
	}{{3, 2}, {4, 3}, {5, 3}} {
		m, _ := bigTableManager(t)
		room := table(t, m, session.CreateOptions{RealTime: realTime}, tableNames[:tc.people]...)
		connect(t, m, room, tc.people)
		// Ben is the target; everybody else votes in turn. Ann opens it.
		for i, voters := 0, []string{"Ann", "Cal", "Dee", "Eve"}; i < tc.needed; i++ {
			if bot := seatOf(t, stateOf(t, m, room), 2).Bot; bot != "" {
				t.Fatalf("%d people: replaced after %d votes, want %d", tc.people, i, tc.needed)
			}
			if _, err := m.VoteKick(ctx, room, voters[i], 2); err != nil {
				t.Fatalf("%d people: %s: %v", tc.people, voters[i], err)
			}
		}
		ben := seatOf(t, stateOf(t, m, room), 2)
		if ben.Bot != "tactical" || ben.Replaced == nil || ben.Replaced.Name != "Ben" || ben.Replaced.Reason != session.ReasonKicked {
			t.Errorf("%d people: after %d votes: %+v", tc.people, tc.needed, ben)
		}
	}
}

func TestVoteKick_WhoMayVote(t *testing.T) {
	ctx := context.Background()
	m, _ := bigTableManager(t)
	room := table(t, m, session.CreateOptions{RealTime: realTime}, "Ann", "Ben", "Cal")

	// Nobody is connected.
	if _, err := m.VoteKick(ctx, room, "Ann", 2); !errors.Is(err, session.ErrNotConnected) {
		t.Errorf("no socket: %v", err)
	}
	// With only the voter and the target connected, it could never pass.
	ann, ben := m.TrackConnection(room, 1), m.TrackConnection(room, 2)
	defer ann()
	defer ben()
	if _, err := m.VoteKick(ctx, room, "Ann", 2); !errors.Is(err, session.ErrKickNotAllowed) {
		t.Errorf("two connected: %v", err)
	}
	cal := m.TrackConnection(room, 3)
	defer cal()
	if _, err := m.VoteKick(ctx, room, "Ann", 1); !errors.Is(err, session.ErrKickNotAllowed) {
		t.Errorf("against oneself: %v", err)
	}
	if _, err := m.VoteKick(ctx, room, "Nobody", 2); !errors.Is(err, session.ErrNotAPlayer) {
		t.Errorf("a stranger: %v", err)
	}
	// A vote twice is one vote.
	for range 2 {
		if _, err := m.VoteKick(ctx, room, "Ann", 2); err != nil {
			t.Fatal(err)
		}
	}
	snap := stateOf(t, m, room)
	if v := snap.RealTime.KickVotes; len(v) != 1 || v[0].Seat != 2 || v[0].Votes != 1 || v[0].Needed != 2 {
		t.Fatalf("the vote: %+v", v)
	}
	// The target cannot vote against themselves, nor stop the others.
	if _, err := m.VoteKick(ctx, room, "Ben", 2); !errors.Is(err, session.ErrKickNotAllowed) {
		t.Errorf("the target's own vote: %v", err)
	}
	if _, err := m.VoteKick(ctx, room, "Cal", 2); err != nil {
		t.Fatal(err)
	}
	// A bot is nobody to vote against.
	if _, err := m.VoteKick(ctx, room, "Ann", 2); !errors.Is(err, session.ErrKickNotAllowed) {
		t.Errorf("against the bot now there: %v", err)
	}
}

func TestVoteKick_NotInEveryGame(t *testing.T) {
	ctx := context.Background()
	m, _ := bigTableManager(t)
	notRealTime := table(t, m, session.CreateOptions{}, "Ann", "Ben", "Cal")
	connect(t, m, notRealTime, 3)
	if _, err := m.VoteKick(ctx, notRealTime, "Ann", 2); !errors.Is(err, session.ErrKickNotAllowed) {
		t.Errorf("not real-time: %v", err)
	}
	// Two seats: only one person could ever vote.
	two := table(t, m, session.CreateOptions{GameDefID: "tak-3x3", RealTime: realTime}, "Ann", "Ben")
	connect(t, m, two, 2)
	if _, err := m.VoteKick(ctx, two, "Ann", 2); !errors.Is(err, session.ErrKickNotAllowed) {
		t.Errorf("two seats: %v", err)
	}
}

// Those counted are the connected people and the target, whether connected or not. A
// voter who goes stops counting, and somebody going can make a vote pass.
func TestVoteKick_CountsWhoIsConnectedNow(t *testing.T) {
	ctx := context.Background()
	m, _ := bigTableManager(t)
	room := table(t, m, session.CreateOptions{RealTime: realTime}, "Ann", "Ben", "Cal", "Dee")
	closers := connect(t, m, room, 4)

	if _, err := m.VoteKick(ctx, room, "Ann", 2); err != nil {
		t.Fatal(err)
	}
	if v := stateOf(t, m, room).RealTime.KickVotes; v[0].Needed != 3 {
		t.Fatalf("four connected: needs %d", v[0].Needed)
	}
	if _, err := m.VoteKick(ctx, room, "Cal", 2); err != nil {
		t.Fatal(err)
	}
	// The target goes: still counted. Two of three is enough, but of four it was not.
	closers[2]()
	waitDisconnected(t, m, room, 2)
	if v := stateOf(t, m, room).RealTime.KickVotes; len(v) != 1 || v[0].Needed != 3 {
		t.Fatalf("the target gone: %+v", v)
	}
	// Dee, who had not voted, goes: three are counted, and the two votes now pass.
	closers[4]()
	waitDisconnected(t, m, room, 4)
	m.RealTimeDue(ctx)
	if got := seatOf(t, stateOf(t, m, room), 2); got.Bot == "" {
		t.Errorf("the vote did not pass when somebody left: %+v", got)
	}
	if v := stateOf(t, m, room).RealTime.KickVotes; len(v) != 0 {
		t.Errorf("a vote stays open: %+v", v)
	}
}

func TestVoteKick_AVoterWhoGoesStopsCounting(t *testing.T) {
	ctx := context.Background()
	m, _ := bigTableManager(t)
	room := table(t, m, session.CreateOptions{RealTime: realTime}, "Ann", "Ben", "Cal", "Dee", "Eve")
	closers := connect(t, m, room, 5)
	if _, err := m.VoteKick(ctx, room, "Ann", 2); err != nil {
		t.Fatal(err)
	}
	closers[1]()
	waitDisconnected(t, m, room, 1)
	if v := stateOf(t, m, room).RealTime.KickVotes; len(v) != 1 || v[0].Votes != 0 || v[0].Needed != 3 {
		t.Fatalf("the voter gone: %+v", v)
	}
}

func TestVoteKick_LapsesAndCannotBeReopenedAtOnce(t *testing.T) {
	ctx := context.Background()
	m, clock := bigTableManager(t)
	room := table(t, m, session.CreateOptions{RealTime: realTime}, "Ann", "Ben", "Cal")
	connect(t, m, room, 3)
	if _, err := m.VoteKick(ctx, room, "Ann", 2); err != nil {
		t.Fatal(err)
	}
	events, stop := m.Subscribe(room)
	defer stop()
	clock.Advance(2*time.Minute + time.Second)
	m.RealTimeDue(ctx)
	if v := stateOf(t, m, room).RealTime.KickVotes; len(v) != 0 {
		t.Fatalf("after two minutes: %+v", v)
	}
	told := false
	for len(events) > 0 {
		told = told || (<-events).Type == "kick_vote"
	}
	if !told {
		t.Error("the room was not told the vote lapsed")
	}

	if _, err := m.VoteKick(ctx, room, "Ann", 2); !errors.Is(err, session.ErrKickNotAllowed) {
		t.Errorf("reopened at once: %v", err)
	}
	// Whoever opened it is the only one held back. Cal's vote lapses too.
	if _, err := m.VoteKick(ctx, room, "Cal", 2); err != nil {
		t.Errorf("somebody else: %v", err)
	}
	clock.Advance(2*time.Minute + time.Second)
	m.RealTimeDue(ctx)
	clock.Advance(3 * time.Minute)
	if _, err := m.VoteKick(ctx, room, "Ann", 2); err != nil {
		t.Errorf("after five minutes: %v", err)
	}
}

// A kick is final: the player has no seat, the game is off their list, and they may
// not come back, though they may watch. Who played before it stays on the record.
func TestVoteKick_IsFinal(t *testing.T) {
	ctx := context.Background()
	m, _ := bigTableManager(t)
	as := func(id string) context.Context {
		return auth.WithPrincipal(ctx, auth.Principal{ID: id, Kind: store.KindGuest})
	}
	created, err := m.CreateGameWithOptions(as("ann"), session.CreateOptions{GameDefID: "nyet-3", CreatorName: "Ann", RealTime: realTime})
	if err != nil {
		t.Fatal(err)
	}
	room := created.RoomCode
	for _, name := range []string{"Ben", "Cal"} {
		if _, err := m.JoinGame(as(strings.ToLower(name)), room, name, session.RolePlayer); err != nil {
			t.Fatal(err)
		}
	}
	connect(t, m, room, 3)
	if own, err := m.OwnGames(as("ben"), 10); err != nil || len(own) != 1 {
		t.Fatalf("Ben's games before: %+v, %v", own, err)
	}
	before := stateOf(t, m, room)
	for _, name := range []string{"Ann", "Cal"} {
		if _, err := m.VoteKick(as(strings.ToLower(name)), room, name, 2); err != nil {
			t.Fatal(err)
		}
	}

	snap := stateOf(t, m, room)
	seat := seatOf(t, snap, 2)
	if seat.Bot != "tactical" || seat.Name == "Ben" || seat.PrincipalID != "" {
		t.Errorf("the seat: %+v", seat)
	}
	if seat.Replaced == nil || seat.Replaced.Name != "Ben" || seat.Replaced.MoveSeq != before.MoveSeq {
		t.Errorf("the record: %+v, at move %d", seat.Replaced, before.MoveSeq)
	}
	if own, err := m.OwnGames(as("ben"), 10); err != nil || len(own) != 0 {
		t.Errorf("Ben's games after: %+v, %v", own, err)
	}
	if _, err := m.JoinGame(as("ben"), room, "Ben", session.RolePlayer); !errors.Is(err, session.ErrWasReplaced) {
		t.Errorf("Ben coming back: %v", err)
	}
	if _, err := m.JoinGame(as("ben"), room, "Someone Else", session.RolePlayer); !errors.Is(err, session.ErrWasReplaced) {
		t.Errorf("Ben under another name: %v", err)
	}
	if _, err := m.JoinGame(as("ben"), room, "Ben", session.RoleSpectator); err != nil {
		t.Errorf("Ben watching: %v", err)
	}
	if _, err := m.JoinGame(as("dee"), room, "Dee", session.RolePlayer); !errors.Is(err, session.ErrSeatsFull) {
		t.Errorf("somebody new: %v", err)
	}
}

// With nobody signed in, the ban is held by the name of the seat.
func TestVoteKick_TheBanOfASeatNobodyOwned(t *testing.T) {
	ctx := context.Background()
	m, _ := bigTableManager(t)
	room := table(t, m, session.CreateOptions{RealTime: realTime}, "Ann", "Ben", "Cal")
	connect(t, m, room, 3)
	for _, name := range []string{"Ann", "Cal"} {
		if _, err := m.VoteKick(ctx, room, name, 2); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.JoinGame(ctx, room, "BEN", session.RolePlayer); !errors.Is(err, session.ErrWasReplaced) {
		t.Errorf("Ben coming back: %v", err)
	}
}

// Somebody playing several seats is one person: one vote, and out of all of them.
func TestVoteKick_APersonWithSeveralSeats(t *testing.T) {
	ctx := context.Background()
	m, _ := bigTableManager(t)
	room := table(t, m, session.CreateOptions{GameDefID: "nyet", RealTime: realTime, Also: []int{3}}, "Ann", "Ben", "Cal")
	connect(t, m, room, 4)
	if _, err := m.VoteKick(ctx, room, "Ben", 3); err != nil {
		t.Fatal(err)
	}
	if v := stateOf(t, m, room).RealTime.KickVotes; v[0].Seat != 1 || v[0].Needed != 2 {
		t.Fatalf("three people, the target at seats 1 and 3: %+v", v)
	}
	if _, err := m.VoteKick(ctx, room, "Cal", 1); err != nil {
		t.Fatal(err)
	}
	snap := stateOf(t, m, room)
	if seatOf(t, snap, 1).Bot == "" || seatOf(t, snap, 3).Bot == "" {
		t.Errorf("not out of both: %+v", snap.Seats)
	}
}

// Replacing somebody else gives the player to move no time back; replacing the player
// to move hands the turn to the bot.
func TestVoteKick_AndTheClock(t *testing.T) {
	ctx := context.Background()
	m, clock := bigTableManager(t)
	room := table(t, m, session.CreateOptions{RealTime: realTime, ClockEach: 600}, "Ann", "Ben", "Cal")
	connect(t, m, room, 3)
	turn := *stateOf(t, m, room).ImpulseSeat
	names := map[int]string{1: "Ann", 2: "Ben", 3: "Cal"}
	other := turn%3 + 1
	voters := []string{names[turn], names[other%3+1]}

	clock.Advance(30 * time.Second)
	before := stateOf(t, m, room).ImpulseDeadline
	for _, name := range voters {
		if _, err := m.VoteKick(ctx, room, name, other); err != nil {
			t.Fatal(err)
		}
	}
	snap := stateOf(t, m, room)
	if snap.ImpulseDeadline == nil || !snap.ImpulseDeadline.Equal(*before) {
		t.Errorf("the deadline moved from %v to %v", before, snap.ImpulseDeadline)
	}
	for _, s := range snap.Clock.Seats {
		if s.Seat == other {
			t.Errorf("the bot's seat is on the clock: %+v", snap.Clock)
		}
	}
	if len(snap.Clock.Seats) != 2 {
		t.Errorf("the clock: %+v", snap.Clock)
	}
}

// The clock runs at any number of seats, charged to the seat the game waits on.
func TestLargeTableClock_ChargesTheSeatToMove(t *testing.T) {
	ctx := context.Background()
	m, clock := bigTableManager(t)
	room := table(t, m, session.CreateOptions{ClockSeconds: []int{100, 200, 300}}, "Ann", "Ben", "Cal")
	names := map[int]string{1: "Ann", 2: "Ben", 3: "Cal"}
	snap := stateOf(t, m, room)
	if snap.Clock == nil || len(snap.Clock.Seats) != 3 {
		t.Fatalf("the clock: %+v", snap.Clock)
	}
	for range 4 {
		seat := *snap.ImpulseSeat
		left := map[int]float64{}
		for _, s := range snap.Clock.Seats {
			left[s.Seat] = s.Left
		}
		if snap.Clock.Running != seat {
			t.Fatalf("running %d, seat to move %d", snap.Clock.Running, seat)
		}
		clock.Advance(5 * time.Second)
		_, legal, err := legalSelections(ctx, m, room)
		if err != nil || len(legal) == 0 {
			t.Fatalf("legal moves: %d, %v", len(legal), err)
		}
		if _, err := m.PlayNotation(ctx, room, names[seat], legal[0].Notation); err != nil {
			t.Fatal(err)
		}
		snap = stateOf(t, m, room)
		for _, s := range snap.Clock.Seats {
			want := left[s.Seat]
			if s.Seat == seat {
				want -= 5
			}
			if s.Left < want-1.5 || s.Left > want+0.5 {
				t.Errorf("seat %d has %v, want about %v (seat %d moved)", s.Seat, s.Left, want, seat)
			}
		}
	}
}

func TestLargeTableClock_IsForTwoOrMore(t *testing.T) {
	m, _ := bigTableManager(t)
	ctx := context.Background()
	if _, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "nyet-3", CreatorName: "Ann", ClockSeconds: []int{60, 60}}); !errors.Is(err, session.ErrInvalidClock) {
		t.Errorf("two times for three seats: %v", err)
	}
	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "nyet-3", CreatorName: "Ann", ClockEach: 60})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JoinGame(ctx, created.RoomCode, "Ben", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	if got := len(stateOf(t, m, created.RoomCode).Clock.Seats); got != 3 {
		t.Errorf("one time for all: %d seats", got)
	}
}

// waitForPerson waits for the bots to play until a person's seat is to move, or the game ends.
func waitForPerson(t *testing.T, m *session.Manager, room string) session.StateSnapshot {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		snap := stateOf(t, m, room)
		if snap.Status != "active" || snap.ImpulseSeat == nil || seatOf(t, snap, *snap.ImpulseSeat).Bot == "" {
			return snap
		}
	}
	t.Fatal("the bots never got to a person")
	return session.StateSnapshot{}
}

// Out of time at three seats, a bot takes the seat and the game goes on, with that
// seat off the clock, until nobody is left to play with: then it ends with no winner.
func TestLargeTableClock_ABotTakesTheSeat(t *testing.T) {
	for name, opts := range map[string]session.CreateOptions{
		"a clock":         {ClockEach: 60},
		"a time per move": {MoveTimeSeconds: 60},
	} {
		t.Run(name, func(t *testing.T) {
			m, clock := bigTableManager(t)
			room := table(t, m, opts, "Ann", "Ben", "Cal")
			first := *stateOf(t, m, room).ImpulseSeat
			clock.Advance(61 * time.Second)

			snap := waitForPerson(t, m, room)
			seat := seatOf(t, snap, first)
			if seat.Bot != "tactical" || seat.Replaced == nil || seat.Replaced.Reason != session.ReasonTimeout {
				t.Fatalf("after the flag: %s, %+v", snap.Status, seat)
			}
			if snap.Status == "active" && *snap.ImpulseSeat == first {
				t.Errorf("the game is still waiting on the seat that went")
			}
			if snap.Clock != nil {
				for _, s := range snap.Clock.Seats {
					if s.Seat == first {
						t.Errorf("the bot's seat is on the clock: %+v", snap.Clock)
					}
				}
			}

			// The next person runs out too, leaving one, and the last has nobody left
			// to play with. The database's own time does not follow the test's clock, so
			// with a time per move these have all happened already.
			for range 2 {
				if snap.Status == "active" {
					clock.Advance(61 * time.Second)
					snap = waitForPerson(t, m, room)
				}
			}
			if snap.Status != "finished" || snap.WinnerSeat != nil || snap.EndReason != "timeout" {
				t.Errorf("the last flag: %s, winner %v, %q", snap.Status, snap.WinnerSeat, snap.EndReason)
			}
		})
	}
}

// The watcher makes a flag fall with nobody looking, in a real-time game.
func TestLargeTableClock_TheWatcherSeesAFlag(t *testing.T) {
	m, clock := bigTableManager(t)
	room := table(t, m, session.CreateOptions{RealTime: realTime, ClockEach: 60}, "Ann", "Ben", "Cal")
	first := *stateOf(t, m, room).ImpulseSeat
	events, stop := m.Subscribe(room)
	defer stop()
	clock.Advance(61 * time.Second)
	m.RealTimeDue(context.Background())
	for deadline := time.After(5 * time.Second); ; {
		select {
		case ev := <-events:
			if ev.Type == "seat_replaced" {
				return
			}
		case <-deadline:
			t.Fatalf("no seat_replaced for seat %d", first)
		}
	}
}

// The person a bot replaced holds no seat, so nothing the seat goes on to do is theirs.
func TestTakeover_TheReplacedPersonHoldsNoSeat(t *testing.T) {
	m, clock := bigTableManager(t)
	room := table(t, m, session.CreateOptions{ClockEach: 60}, "Ann", "Ben", "Cal")
	first := *stateOf(t, m, room).ImpulseSeat
	clock.Advance(61 * time.Second)
	snap := waitForPerson(t, m, room)
	for _, name := range []string{"Ann", "Ben", "Cal"} {
		held := false
		for _, s := range snap.Seats {
			held = held || s.Name == name
		}
		if want := map[int]string{1: "Ann", 2: "Ben", 3: "Cal"}[first] != name; held != want {
			t.Errorf("%s holds a seat: %v", name, held)
		}
	}
}

// No vote in a rated game, where every move must be the player's own.
func TestVoteKick_NotInARatedGame(t *testing.T) {
	w := newRatedWorld(t)
	a, b := w.account("anna"), w.account("bruno")
	room := w.start(a, b)
	if _, err := w.m.VoteKick(a.ctx, room, "anna", 2); !errors.Is(err, session.ErrRatedGame) {
		t.Errorf("a rated game: %v", err)
	}
}
