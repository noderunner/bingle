// Tests of real-time tournaments: the start time and check-in, a pairing's games one
// after the other, the break between rounds, and the first-move deadline.

package session_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// realTimeSettings is a tournament starting an hour from now by w's clock, with a
// quarter of an hour of check-in, a minute's break and two minutes for a first move.
// The clock is put back an hour for it, so that the start comes at the real time: the
// database stamps a new game with the real clock, and the deadlines count from that.
func (w *ratedWorld) realTimeSettings(format string, games int) session.TournamentSettings {
	s := tournamentSettings(format)
	s.GamesPerPairing = games
	s.RealTime = true
	w.clock.Set(time.Now().Add(-time.Hour))
	s.StartsAt = time.Now().UTC().Format(time.RFC3339)
	return s
}

// sweep runs the sweep, as the watcher does every few seconds.
func (w *ratedWorld) sweep() { w.m.AdvanceRealTimeTournaments(context.Background()) }

// runningWith starts a tournament of these players, all checked in, and leaves the
// clock at the real time.
func (w *ratedWorld) runningWith(s session.TournamentSettings, org player, players ...player) int64 {
	w.t.Helper()
	id := w.openTournament(s, org, players...)
	w.clock.Advance(50 * time.Minute)
	w.sweep()
	for _, p := range players {
		if _, err := w.m.CheckInTournament(p.ctx, id); err != nil {
			w.t.Fatal(err)
		}
	}
	w.clock.Set(time.Now())
	w.sweep()
	if d := w.detail(org.ctx, id); d.Status != store.TournamentRunning {
		w.t.Fatalf("status %q, want running", d.Status)
	}
	return id
}

func TestRealTimeTournament_Settings(t *testing.T) {
	w := newRatedWorld(t)
	org := w.account("org")
	good := w.realTimeSettings("swiss", 1)
	made, err := w.m.CreateTournament(org.ctx, good)
	if err != nil {
		t.Fatal(err)
	}
	if !made.RealTime || made.CheckInSeconds != 900 || made.RoundBreakSeconds != 60 || made.FirstMoveSeconds != 120 {
		t.Errorf("the defaults: %+v", made.TournamentSummary)
	}
	shortClock := w.realTimeSettings("swiss", 1)
	shortClock.ClockSeconds = 60
	if d, err := w.m.CreateTournament(org.ctx, shortClock); err != nil || d.FirstMoveSeconds != 30 {
		t.Errorf("a short clock: first move %d, %v, want half the clock", d.FirstMoveSeconds, err)
	}
	for name, change := range map[string]func(*session.TournamentSettings){
		"no start time":          func(s *session.TournamentSettings) { s.StartsAt = "" },
		"a first move as long":   func(s *session.TournamentSettings) { s.FirstMoveSeconds = s.ClockSeconds },
		"a first move too short": func(s *session.TournamentSettings) { s.FirstMoveSeconds = 5 },
		"a check-in too short":   func(s *session.TournamentSettings) { s.CheckInSeconds = 10 },
		"a break too long":       func(s *session.TournamentSettings) { b := 3601; s.RoundBreakSeconds = &b },
	} {
		s := w.realTimeSettings("swiss", 1)
		change(&s)
		if _, err := w.m.CreateTournament(org.ctx, s); !errors.Is(err, session.ErrInvalidTournament) {
			t.Errorf("%s: %v, want ErrInvalidTournament", name, err)
		}
	}
	// A start time already gone cannot be opened, and nobody starts one by hand.
	w.clock.Advance(2 * time.Hour)
	if _, err := w.m.OpenTournament(org.ctx, made.ID); !errors.Is(err, session.ErrInvalidTournament) {
		t.Errorf("opening one whose start has passed: %v", err)
	}
	w.clock.Set(time.Now())
	id := w.openTournament(w.realTimeSettings("swiss", 1), org, w.account("ana"), w.account("bea"))
	if _, err := w.m.StartTournament(org.ctx, id); !errors.Is(err, store.ErrWrongTournamentState) {
		t.Errorf("starting one by hand: %v", err)
	}
}

func TestRealTimeTournament_CheckInAndTheStart(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea, cal, dee := w.account("org"), w.account("ana"), w.account("bea"), w.account("cal"), w.account("dee")
	id := w.openTournament(w.realTimeSettings("swiss", 1), org, ana, bea, cal)

	if _, err := w.m.CheckInTournament(ana.ctx, id); !errors.Is(err, store.ErrWrongTournamentState) {
		t.Errorf("checking in too early: %v", err)
	}
	w.clock.Advance(44 * time.Minute)
	w.sweep()
	if d := w.detail(org.ctx, id); d.Status != store.TournamentRegistration {
		t.Fatalf("status %q before the check-in opens", d.Status)
	}
	w.clock.Advance(2 * time.Minute)
	w.sweep()
	if d := w.detail(org.ctx, id); d.Status != store.TournamentCheckIn {
		t.Fatalf("status %q, want check_in", d.Status)
	}
	if _, err := w.m.EnterTournament(dee.ctx, id); !errors.Is(err, store.ErrWrongTournamentState) {
		t.Errorf("entering after the check-in opened: %v", err)
	}
	if _, err := w.m.CheckInTournament(dee.ctx, id); !errors.Is(err, session.ErrNotEntered) {
		t.Errorf("checking in with no entry: %v", err)
	}
	d, err := w.m.CheckInTournament(ana.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !d.You.CheckedIn {
		t.Error("Ana is not told she is checked in")
	}
	if _, err := w.m.CheckInTournament(ana.ctx, id); err != nil {
		t.Errorf("checking in twice: %v", err)
	}
	if _, err := w.m.CheckInTournament(bea.ctx, id); err != nil {
		t.Fatal(err)
	}

	w.clock.Advance(13 * time.Minute)
	w.sweep()
	if d := w.detail(org.ctx, id); d.Status != store.TournamentCheckIn {
		t.Fatalf("status %q before the start", d.Status)
	}
	w.clock.Set(time.Now())
	w.sweep()
	d = w.detail(ana.ctx, id)
	if d.Status != store.TournamentRunning || d.Players != 2 {
		t.Fatalf("status %q with %d players, want running with 2: Cal did not check in", d.Status, d.Players)
	}
	if d.You.Game == "" {
		t.Error("Ana is not told of her game")
	}
	game, err := w.m.GetState(context.Background(), d.You.Game, nil)
	if err != nil {
		t.Fatal(err)
	}
	if game.RealTime == nil || game.RealTime.Wait != nil || game.RealTime.FirstMove == nil || game.RealTime.FirstMove.Seat != 1 {
		t.Errorf("the game's real_time: %+v", game.RealTime)
	}
}

// With too few checked in there is no tournament, and those who entered can see why.
func TestRealTimeTournament_TooFewCheckIn(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea := w.account("org"), w.account("ana"), w.account("bea")
	id := w.openTournament(w.realTimeSettings("swiss", 1), org, ana, bea)
	w.clock.Advance(50 * time.Minute)
	w.sweep()
	if _, err := w.m.CheckInTournament(ana.ctx, id); err != nil {
		t.Fatal(err)
	}
	w.clock.Set(time.Now())
	w.sweep()
	d := w.detail(ana.ctx, id)
	if d.Status != store.TournamentAbandoned || !strings.Contains(d.EndDetail, "checked in") {
		t.Errorf("status %q, %q: want abandoned, saying why", d.Status, d.EndDetail)
	}
	if list, _ := w.m.ListTournaments(bea.ctx); len(list) != 1 {
		t.Errorf("somebody who entered sees %d tournaments, want it still listed", len(list))
	}
}

// A pairing's games are one after the other, with sides swapped, and the next round
// comes after the break.
func TestRealTimeTournament_OneGameAtATimeThenABreak(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea := w.account("org"), w.account("ana"), w.account("bea")
	cal, dee := w.account("cal"), w.account("dee")
	who := handles(ana, bea, cal, dee)
	id := w.runningWith(w.realTimeSettings("swiss", 2), org, ana, bea, cal, dee)

	d := w.detail(org.ctx, id)
	if len(d.Games) != 1 || len(d.Games[0].Games) != 4 {
		t.Fatalf("round one: %+v", d.Games)
	}
	first := d.Games[0].Games[0]
	second := d.Games[0].Games[1]
	if first.RoomCode == "" || second.RoomCode != "" || second.Done {
		t.Fatalf("the first pairing: %+v, %+v: only the first game starts", first, second)
	}
	if first.First != second.Second || first.Second != second.First {
		t.Errorf("sides are not swapped: %+v, %+v", first, second)
	}

	// Whoever is on the old page hears that the next game has started.
	events, stop := w.m.Subscribe(first.RoomCode)
	defer stop()
	w.finishGame(first, who, false)
	w.sweep()
	d = w.detail(org.ctx, id)
	second = d.Games[0].Games[1]
	if second.RoomCode == "" || second.Done {
		t.Fatalf("the second game has not started: %+v", second)
	}
	told := 0
	for len(events) > 0 {
		if ev := <-events; ev.Type == "next_game" {
			told++
		}
	}
	if told != 2 {
		t.Errorf("%d told of the next game, want both players", told)
	}

	for _, g := range openGames(d) {
		if g.RoomCode != "" {
			w.finishGame(g, who, false)
		}
	}
	w.sweep()
	for _, g := range openGames(w.detail(org.ctx, id)) {
		w.finishGame(g, who, false)
	}
	w.sweep()

	// The round is over: the break, and then the next round.
	d = w.detail(org.ctx, id)
	if d.CurrentRound != 1 || d.NextRoundAt == "" {
		t.Fatalf("round %d, next at %q: want the break", d.CurrentRound, d.NextRoundAt)
	}
	w.clock.Advance(30 * time.Second)
	w.sweep()
	if d = w.detail(org.ctx, id); d.CurrentRound != 1 {
		t.Errorf("round %d during the break", d.CurrentRound)
	}
	w.clock.Advance(40 * time.Second)
	w.sweep()
	if d = w.detail(org.ctx, id); d.CurrentRound != 2 || d.NextRoundAt != "" {
		t.Errorf("round %d, next at %q: want round 2", d.CurrentRound, d.NextRoundAt)
	}
}

// A withdrawal during the first game settles the second without playing it.
func TestRealTimeTournament_WithdrawingSettlesTheGameToCome(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea := w.account("org"), w.account("ana"), w.account("bea")
	id := w.runningWith(w.realTimeSettings("round_robin", 2), org, ana, bea)
	if _, err := w.m.LeaveTournament(bea.ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	d := w.detail(org.ctx, id)
	if d.Status != store.TournamentFinished {
		t.Fatalf("status %q, want finished", d.Status)
	}
	if d.Standings[0].Handle != "ana" || d.Standings[0].Score != 2 {
		t.Errorf("standings: %+v, want Ana with both games", d.Standings)
	}
}

// oneGame is a real-time tournament of a single game between ana and bea.
func (w *ratedWorld) oneGame(t *testing.T) (id int64, org, ana, bea player, room string) {
	t.Helper()
	org, ana, bea = w.account("org"), w.account("ana"), w.account("bea")
	id = w.runningWith(w.realTimeSettings("round_robin", 1), org, ana, bea)
	g := w.detail(org.ctx, id).Games[0].Games[0]
	first, second := ana, bea
	if g.First != "ana" {
		first, second = bea, ana
	}
	return id, org, first, second, g.RoomCode
}

func TestRealTimeTournament_TheFirstPlayerDoesNotCome(t *testing.T) {
	w := newRatedWorld(t)
	id, org, first, second, room := w.oneGame(t)
	defer w.m.TrackConnection(room, 2)()
	w.clock.Advance(119 * time.Second)
	w.sweep()
	if d := w.detail(org.ctx, id); d.Status != store.TournamentRunning {
		t.Fatalf("status %q before the deadline", d.Status)
	}
	w.clock.Advance(2 * time.Second)
	w.sweep()
	d := w.detail(org.ctx, id)
	if d.Status != store.TournamentFinished || d.Standings[0].Handle != second.handle || d.Standings[0].Score != 1 {
		t.Errorf("status %q, standings %+v: want %s to have won, not %s", d.Status, d.Standings, second.handle, first.handle)
	}
	game, err := w.m.GetState(context.Background(), room, nil)
	if err != nil {
		t.Fatal(err)
	}
	if game.EndReason != store.EndReasonNoShow {
		t.Errorf("the game ended by %q", game.EndReason)
	}
	w.m.RecordResults(context.Background())
	if game, _ = w.m.GetState(context.Background(), room, nil); game.RatingStatus != store.RatingNoMoves {
		t.Errorf("rating status %q, want no change to anyone's rating", game.RatingStatus)
	}
}

func TestRealTimeTournament_NeitherPlayerComes(t *testing.T) {
	w := newRatedWorld(t)
	id, org, _, _, room := w.oneGame(t)
	w.clock.Advance(121 * time.Second)
	w.sweep()
	d := w.detail(org.ctx, id)
	if d.Status != store.TournamentFinished {
		t.Fatalf("status %q", d.Status)
	}
	for _, s := range d.Standings {
		if s.Score != 0 {
			t.Errorf("%s scored %v, want nothing", s.Handle, s.Score)
		}
	}
	if game, _ := w.m.GetState(context.Background(), room, nil); game.EndReason != store.EndReasonNoShows || game.WinnerSeat != nil {
		t.Errorf("the game ended by %q, winner %v", game.EndReason, game.WinnerSeat)
	}
}

// The second player gets their own deadline, from the first player's move, and once
// both have moved only the clock matters.
func TestRealTimeTournament_TheSecondPlayersDeadline(t *testing.T) {
	w := newRatedWorld(t)
	_, _, first, second, room := w.oneGame(t)
	move := func(p player, seat int) {
		t.Helper()
		_, legal, err := legalSelections(context.Background(), w.m, room)
		if err != nil || len(legal) == 0 {
			t.Fatalf("legal moves of seat %d: %d, %v", seat, len(legal), err)
		}
		if _, err := w.m.PlayNotation(p.ctx, room, p.handle, legal[0].Notation); err != nil {
			t.Fatal(err)
		}
	}
	move(first, 1)
	game, err := w.m.GetState(context.Background(), room, nil)
	if err != nil {
		t.Fatal(err)
	}
	if game.RealTime.FirstMove == nil || game.RealTime.FirstMove.Seat != 2 {
		t.Fatalf("first move after the opening: %+v, want seat 2's", game.RealTime.FirstMove)
	}
	w.clock.Set(time.Now())
	w.clock.Advance(100 * time.Second)
	w.sweep()
	move(second, 2)
	w.clock.Set(time.Now())
	if game, _ = w.m.GetState(context.Background(), room, nil); game.RealTime.FirstMove != nil {
		t.Errorf("a deadline with both moved: %+v", game.RealTime.FirstMove)
	}
	w.clock.Advance(10 * time.Minute)
	w.sweep()
	if game, _ = w.m.GetState(context.Background(), room, nil); game.Status != "active" {
		t.Errorf("the game ended by %q with both having moved", game.EndReason)
	}
}

func TestRealTimeTournament_TheSecondPlayerDoesNotCome(t *testing.T) {
	w := newRatedWorld(t)
	id, org, first, _, room := w.oneGame(t)
	_, legal, err := legalSelections(context.Background(), w.m, room)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.PlayNotation(first.ctx, room, first.handle, legal[0].Notation); err != nil {
		t.Fatal(err)
	}
	w.clock.Set(time.Now())
	w.clock.Advance(121 * time.Second)
	w.sweep()
	d := w.detail(org.ctx, id)
	if d.Status != store.TournamentFinished || d.Standings[0].Handle != first.handle || d.Standings[0].Score != 1 {
		t.Errorf("status %q, standings %+v: want %s to have won", d.Status, d.Standings, first.handle)
	}
}

// A correspondence tournament has none of this.
func TestRealTimeTournament_ACorrespondenceGameHasNoDeadline(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea := w.account("org"), w.account("ana"), w.account("bea")
	id := w.openTournament(tournamentSettings("round_robin"), org, ana, bea)
	d, err := w.m.StartTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	room := d.Games[0].Games[0].RoomCode
	w.clock.Advance(10 * time.Minute)
	w.m.AdvanceTournaments(context.Background())
	game, err := w.m.GetState(context.Background(), room, nil)
	if err != nil {
		t.Fatal(err)
	}
	if game.Status != "active" || game.RealTime != nil {
		t.Errorf("status %q, real_time %+v", game.Status, game.RealTime)
	}
}
