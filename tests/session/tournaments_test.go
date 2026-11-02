// Tests of running a tournament: its states, its rounds and what ends them, and
// that its results outlast its games.

package session_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

func tournamentSettings(format string) session.TournamentSettings {
	return session.TournamentSettings{Name: "The cup", GameDefID: "yavalath", Format: format, ClockSeconds: 3600, GamesPerPairing: 1}
}

// openTournament makes a tournament with these players in it, ready to start.
func (w *ratedWorld) openTournament(s session.TournamentSettings, organiser player, players ...player) int64 {
	w.t.Helper()
	made, err := w.m.CreateTournament(organiser.ctx, s)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.m.OpenTournament(organiser.ctx, made.ID); err != nil {
		w.t.Fatal(err)
	}
	for _, p := range players {
		if _, err := w.m.EnterTournament(p.ctx, made.ID); err != nil {
			w.t.Fatal(err)
		}
	}
	return made.ID
}

func (w *ratedWorld) detail(ctx context.Context, id int64) session.TournamentDetail {
	w.t.Helper()
	d, err := w.m.Tournament(ctx, id)
	if err != nil {
		w.t.Fatal(err)
	}
	return d
}

// openGames lists the games of the tournament that are being played.
func openGames(d session.TournamentDetail) []session.TournamentGameInfo {
	var out []session.TournamentGameInfo
	for _, r := range d.Games {
		for _, g := range r.Games {
			if !g.Done {
				out = append(out, g)
			}
		}
	}
	return out
}

// finishGame plays a game to the first seat's win (or, with draw, to a draw).
func (w *ratedWorld) finishGame(g session.TournamentGameInfo, byHandle map[string]player, draw bool) {
	w.t.Helper()
	first, second := byHandle[g.First], byHandle[g.Second]
	if draw {
		if _, err := w.m.Draw(first.ctx, g.RoomCode, first.handle, "offer"); err != nil {
			w.t.Fatal(err)
		}
		if _, err := w.m.Draw(second.ctx, g.RoomCode, second.handle, "accept"); err != nil {
			w.t.Fatal(err)
		}
	} else {
		if _, err := w.m.Resign(second.ctx, g.RoomCode, second.handle); err != nil {
			w.t.Fatal(err)
		}
	}
	w.m.AdvanceTournaments(context.Background())
}

func handles(players ...player) map[string]player {
	out := map[string]player{}
	for _, p := range players {
		out[p.handle] = p
	}
	return out
}

func TestTournament_OnlyWhoRunsItSeesADraftOrChangesIt(t *testing.T) {
	w := newRatedWorld(t)
	org, ana := w.account("org"), w.account("ana")
	made, err := w.m.CreateTournament(org.ctx, tournamentSettings("swiss"))
	if err != nil {
		t.Fatal(err)
	}
	if made.Status != store.TournamentDraft || !made.You.Organiser {
		t.Errorf("a new tournament: %+v", made)
	}
	if _, err := w.m.Tournament(ana.ctx, made.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("somebody else reads a draft: %v", err)
	}
	if list, _ := w.m.ListTournaments(ana.ctx); len(list) != 0 {
		t.Errorf("somebody else lists a draft: %+v", list)
	}
	if list, _ := w.m.ListTournaments(org.ctx); len(list) != 1 {
		t.Errorf("the organiser's list: %+v", list)
	}
	changed := tournamentSettings("round_robin")
	changed.Name = "The better cup"
	got, err := w.m.UpdateTournament(org.ctx, made.ID, changed)
	if err != nil || got.Name != "The better cup" || got.Format != "round_robin" {
		t.Errorf("changing a draft: %+v, %v", got, err)
	}
	if _, err := w.m.UpdateTournament(ana.ctx, made.ID, changed); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("somebody else changes a draft: %v", err)
	}
	if _, err := w.m.EnterTournament(ana.ctx, made.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("entering a draft: %v", err)
	}

	if _, err := w.m.OpenTournament(org.ctx, made.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.UpdateTournament(org.ctx, made.ID, changed); !errors.Is(err, store.ErrWrongTournamentState) {
		t.Errorf("changing it once open: %v", err)
	}
	if _, err := w.m.StartTournament(ana.ctx, made.ID); !errors.Is(err, session.ErrNotOrganiser) {
		t.Errorf("somebody else starts it: %v", err)
	}
	if _, err := w.m.AbandonTournament(ana.ctx, made.ID); !errors.Is(err, session.ErrNotOrganiser) {
		t.Errorf("somebody else abandons it: %v", err)
	}
	if _, err := w.m.StartTournament(org.ctx, made.ID); !errors.Is(err, session.ErrTooFewEntries) {
		t.Errorf("starting with nobody: %v", err)
	}
}

func TestTournament_WhatSettingsAreRefused(t *testing.T) {
	w := newRatedWorld(t)
	org := w.account("org")
	for name, change := range map[string]func(*session.TournamentSettings){
		"no name":         func(s *session.TournamentSettings) { s.Name = "  " },
		"a long name":     func(s *session.TournamentSettings) { s.Name = strings.Repeat("x", 81) },
		"no format":       func(s *session.TournamentSettings) { s.Format = "bracket" },
		"no clock":        func(s *session.TournamentSettings) { s.ClockSeconds = 0 },
		"a long clock":    func(s *session.TournamentSettings) { s.ClockSeconds = session.MaxClockSeconds + 1 },
		"three games":     func(s *session.TournamentSettings) { s.GamesPerPairing = 3 },
		"one player":      func(s *session.TournamentSettings) { s.MinPlayers = 1 },
		"min over max":    func(s *session.TournamentSettings) { s.MinPlayers, s.MaxPlayers = 8, 4 },
		"too many":        func(s *session.TournamentSettings) { s.MaxPlayers = 1000 },
		"rounds in RR":    func(s *session.TournamentSettings) { s.Format, s.Rounds = "round_robin", 3 },
		"a bad increment": func(s *session.TournamentSettings) { s.ClockIncrementSeconds = -1 },
	} {
		s := tournamentSettings("swiss")
		change(&s)
		if _, err := w.m.CreateTournament(org.ctx, s); !errors.Is(err, session.ErrInvalidTournament) {
			t.Errorf("%s: %v", name, err)
		}
	}
	unknown := tournamentSettings("swiss")
	unknown.GameDefID = "no-such-game"
	if _, err := w.m.CreateTournament(org.ctx, unknown); !errors.Is(err, session.ErrGameDefNotFound) {
		t.Errorf("an unknown game: %v", err)
	}
	if _, err := w.m.CreateTournament(context.Background(), tournamentSettings("swiss")); !errors.Is(err, session.ErrTournamentAccount) {
		t.Errorf("no account: %v", err)
	}
}

func TestTournament_EntriesOpenAndClose(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea, cyd := w.account("org"), w.account("ana"), w.account("bea"), w.account("cyd")
	s := tournamentSettings("swiss")
	s.MaxPlayers = 2
	id := w.openTournament(s, org, ana, ana, bea)
	if got := w.detail(ana.ctx, id); got.Players != 2 || !got.You.Entered {
		t.Errorf("entering twice counts once: %+v", got.TournamentSummary)
	}
	if _, err := w.m.EnterTournament(cyd.ctx, id); !errors.Is(err, store.ErrTournamentFull) {
		t.Errorf("a full tournament: %v", err)
	}
	if _, err := w.m.LeaveTournament(ana.ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.EnterTournament(cyd.ctx, id); err != nil {
		t.Errorf("a place given up: %v", err)
	}
	if _, err := w.m.LeaveTournament(bea.ctx, id, "cyd"); !errors.Is(err, session.ErrNotOrganiser) {
		t.Errorf("a player removes another: %v", err)
	}
	if _, err := w.m.LeaveTournament(org.ctx, id, "cyd"); err != nil {
		t.Errorf("the organiser removes a player: %v", err)
	}
	if _, err := w.m.EnterTournament(context.Background(), id); !errors.Is(err, session.ErrTournamentAccount) {
		t.Errorf("no account: %v", err)
	}
}

func TestTournament_RoundRobinPlaysToItsEnd(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea, cyd := w.account("org"), w.account("ana"), w.account("bea"), w.account("cyd")
	who := handles(ana, bea, cyd)
	s := tournamentSettings("round_robin")
	s.GamesPerPairing = 2
	id := w.openTournament(s, org, ana, bea, cyd)
	d, err := w.m.StartTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != store.TournamentRunning || d.TotalRounds != 3 || d.CurrentRound != 1 {
		t.Fatalf("started: %+v", d.TournamentSummary)
	}
	if _, err := w.m.EnterTournament(w.account("dia").ctx, id); !errors.Is(err, store.ErrWrongTournamentState) {
		t.Errorf("entering late: %v", err)
	}
	met := map[string]int{}
	for round := 1; round <= 3; round++ {
		d = w.detail(org.ctx, id)
		if d.CurrentRound != round {
			t.Fatalf("after %d rounds, round %d", round-1, d.CurrentRound)
		}
		last := d.Games[len(d.Games)-1]
		// Three players: one bye and a pairing of two games, sides swapped.
		if len(last.Games) != 3 {
			t.Fatalf("round %d: %+v", round, last.Games)
		}
		open := openGames(d)
		if len(open) != 2 || open[0].First != open[1].Second || open[0].Second != open[1].First {
			t.Fatalf("round %d's pairing is not two games with sides swapped: %+v", round, open)
		}
		met[min(open[0].First, open[0].Second)+"-"+max(open[0].First, open[0].Second)]++
		// The round's games run together, and the round waits for both.
		w.moveEachIn(open[0], who)
		w.finishGame(open[0], who, false)
		if w.detail(org.ctx, id).CurrentRound != round {
			t.Fatalf("round %d ended with a game in play", round)
		}
		w.moveEachIn(open[1], who)
		w.finishGame(open[1], who, false)
	}
	if len(met) != 3 {
		t.Errorf("the pairings: %v", met)
	}
	d = w.detail(org.ctx, id)
	if d.Status != store.TournamentFinished || d.EndedAt == "" {
		t.Fatalf("after the last round: %+v", d.TournamentSummary)
	}
	// Each pairing split 1–1 and each player had a bye: all on four points, sharing first.
	for _, line := range d.Standings {
		if line.Score != 4 || line.Place != 1 || line.Played != 4 || line.Won != 2 || line.Lost != 2 {
			t.Errorf("standing: %+v", line)
		}
	}
	// The games were rated, and say which tournament they were part of.
	w.m.RecordResults(context.Background())
	r, ok := w.rating(ana)
	if !ok || r.Games != 4 {
		t.Errorf("Ana's rating: %+v (%v)", r, ok)
	}
	room := d.Games[0].Games[0].RoomCode
	if room == "" {
		room = d.Games[0].Games[1].RoomCode
	}
	var tid int64
	if err := w.st.DB().QueryRow(`SELECT tournament_id FROM rating_history WHERE room_code = ? LIMIT 1`, room).Scan(&tid); err != nil || tid != id {
		t.Errorf("the rating history's tournament: %d, %v", tid, err)
	}
	state, err := w.m.GetState(context.Background(), room, nil)
	if err != nil || state.Tournament == nil || state.Tournament.ID != id || state.Tournament.Name != "The cup" {
		t.Errorf("the game's tournament: %+v, %v", state.Tournament, err)
	}
}

// moveEachIn has each player move, so that a rated game counts.
func (w *ratedWorld) moveEachIn(g session.TournamentGameInfo, who map[string]player) {
	w.t.Helper()
	w.moveEach(g.RoomCode, who[g.First], who[g.Second])
}

func TestTournament_SwissPairsByScoreWithoutRematches(t *testing.T) {
	w := newRatedWorld(t)
	org := w.account("org")
	players := []player{w.account("ana"), w.account("bea"), w.account("cyd"), w.account("dia")}
	who := handles(players...)
	id := w.openTournament(tournamentSettings("swiss"), org, players...)
	d, err := w.m.StartTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if d.TotalRounds != 2 {
		t.Fatalf("four players take two rounds, not %d", d.TotalRounds)
	}
	first := openGames(d)
	if len(first) != 2 {
		t.Fatalf("round 1: %+v", first)
	}
	winners := map[string]bool{}
	for _, g := range first {
		winners[g.First] = true
		w.finishGame(g, who, false)
	}
	d = w.detail(org.ctx, id)
	second := openGames(d)
	if d.CurrentRound != 2 || len(second) != 2 {
		t.Fatalf("round 2: %+v", d.TournamentSummary)
	}
	for _, g := range second {
		if winners[g.First] != winners[g.Second] {
			t.Errorf("round 2 pairs %s and %s, on different scores", g.First, g.Second)
		}
		for _, old := range first {
			if (old.First == g.First && old.Second == g.Second) || (old.First == g.Second && old.Second == g.First) {
				t.Errorf("a rematch: %+v", g)
			}
		}
		w.finishGame(g, who, true)
	}
	d = w.detail(org.ctx, id)
	if d.Status != store.TournamentFinished {
		t.Fatalf("after two rounds: %+v", d.TournamentSummary)
	}
	if top := d.Standings[0]; top.Score != 1.5 || top.Place != 1 || top.Drawn != 1 || top.Won != 1 {
		t.Errorf("the top: %+v", top)
	}
}

func TestTournament_SwissGivesTheOddPlayerABye(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea, cyd := w.account("org"), w.account("ana"), w.account("bea"), w.account("cyd")
	s := tournamentSettings("swiss")
	s.GamesPerPairing = 2
	id := w.openTournament(s, org, ana, bea, cyd)
	d, err := w.m.StartTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	byes := 0
	for _, g := range d.Games[0].Games {
		if g.Second == "" {
			byes++
			if !g.Done || g.Scores[0] != 2 || g.Reason != store.ResultBye {
				t.Errorf("a bye is a win of the pairing's games: %+v", g)
			}
		}
	}
	if byes != 1 {
		t.Errorf("%d byes: %+v", byes, d.Games[0].Games)
	}
}

func TestTournament_GamesCannotBeTakenBackRatedOrNot(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea := w.account("org"), w.account("ana"), w.account("bea")
	unrated := false
	s := tournamentSettings("round_robin")
	s.Rated = &unrated
	id := w.openTournament(s, org, ana, bea)
	d, err := w.m.StartTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	g := openGames(d)[0]
	who := handles(ana, bea)
	first, second := who[g.First], who[g.Second]
	if _, err := w.m.PlayNotation(first.ctx, g.RoomCode, first.handle, "e5"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.Undo(first.ctx, g.RoomCode, first.handle); !errors.Is(err, session.ErrCannotUndo) {
		t.Errorf("an undo: %v", err)
	}
	if _, err := w.m.PlayNotation(second.ctx, g.RoomCode, second.handle, "d4"); err != nil {
		t.Fatal(err)
	}
	w.finishGame(g, who, false)
	w.m.RecordResults(context.Background())
	if _, ok := w.rating(ana); ok {
		t.Error("an unrated tournament moved a rating")
	}
	if got := w.detail(org.ctx, id); got.Status != store.TournamentFinished {
		t.Errorf("after its one game: %+v", got.TournamentSummary)
	}
}

func TestTournament_APieSwapCreditsThePersonNotTheSeat(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea := w.account("org"), w.account("ana"), w.account("bea")
	id := w.openTournament(tournamentSettings("round_robin"), org, ana, bea)
	d, err := w.m.StartTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	g := openGames(d)[0]
	who := handles(ana, bea)
	first, second := who[g.First], who[g.Second]
	if _, err := w.m.PlayNotation(first.ctx, g.RoomCode, first.handle, "e5"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.PlayNotation(second.ctx, g.RoomCode, second.handle, "swap"); err != nil {
		t.Fatalf("the pie rule is on by default: %v", err)
	}
	// The second player now holds the first seat, and the other resigns to them.
	if _, err := w.m.Resign(first.ctx, g.RoomCode, first.handle); err != nil {
		t.Fatal(err)
	}
	w.m.AdvanceTournaments(context.Background())
	d = w.detail(org.ctx, id)
	if d.Standings[0].Handle != second.handle || d.Standings[0].Score != 1 {
		t.Errorf("standings: %+v, want %s first", d.Standings, second.handle)
	}
}

func TestTournament_APlayerWhoWithdraws(t *testing.T) {
	w := newRatedWorld(t)
	org := w.account("org")
	players := []player{w.account("ana"), w.account("bea"), w.account("cyd"), w.account("dia")}
	who := handles(players...)
	id := w.openTournament(tournamentSettings("round_robin"), org, players...)
	d, err := w.m.StartTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	open := openGames(d)
	if len(open) != 2 {
		t.Fatalf("round 1: %+v", open)
	}
	// One of round 1's players leaves: their game goes to the other.
	leaver, opponent := who[open[0].First], who[open[0].Second]
	if _, err := w.m.LeaveTournament(leaver.ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	game, err := w.st.GetGame(context.Background(), open[0].RoomCode)
	if err != nil || game.Status != "finished" || game.EndReason != store.EndReasonForfeit {
		t.Fatalf("the game in progress: %+v, %v", game, err)
	}
	if _, err := w.m.LeaveTournament(leaver.ctx, id, ""); !errors.Is(err, session.ErrNotEntered) {
		t.Errorf("leaving twice: %v", err)
	}
	if got := w.detail(leaver.ctx, id); !got.You.Withdrawn {
		t.Errorf("the player who left: %+v", got.You)
	}
	w.finishGame(open[1], who, false)
	// The remaining rounds go on with a forfeit for each of the leaver's games.
	for {
		d = w.detail(org.ctx, id)
		games := openGames(d)
		if len(games) == 0 {
			break
		}
		for _, g := range games {
			if g.First == leaver.handle || g.Second == leaver.handle {
				t.Errorf("a game was made for a player who left: %+v", g)
			}
			w.finishGame(g, who, true)
		}
	}
	if d.Status != store.TournamentFinished {
		t.Fatalf("the end: %+v", d.TournamentSummary)
	}
	var left, other session.TournamentEntryInfo
	for _, s := range d.Standings {
		switch s.Handle {
		case leaver.handle:
			left = s
		case opponent.handle:
			other = s
		}
	}
	if left.Score != 0 || left.Lost != 3 || !left.Withdrawn {
		t.Errorf("the player who left: %+v", left)
	}
	if other.Won < 1 {
		t.Errorf("their opponent: %+v", other)
	}
}

func TestTournament_SwissStopsPairingWhoWithdrew(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea := w.account("org"), w.account("ana"), w.account("bea")
	s := tournamentSettings("swiss")
	s.Rounds = 5
	id := w.openTournament(s, org, ana, bea, w.account("cyd"), w.account("dia"))
	d, err := w.m.StartTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if d.TotalRounds != 3 {
		t.Errorf("rounds are capped at what can be played without a rematch: %d", d.TotalRounds)
	}
	// Everybody but one leaves, and there is nobody left to pair.
	for _, h := range []string{"ana", "bea", "cyd"} {
		if _, err := w.m.LeaveTournament(org.ctx, id, h); err != nil {
			t.Fatal(err)
		}
	}
	if got := w.detail(org.ctx, id); got.Status != store.TournamentFinished {
		t.Errorf("one player left: %+v", got.TournamentSummary)
	}
}

func TestTournament_AbandoningLeavesNoGamesAndNoRatings(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea := w.account("org"), w.account("ana"), w.account("bea")
	s := tournamentSettings("round_robin")
	s.GamesPerPairing = 2
	id := w.openTournament(s, org, ana, bea)
	d, err := w.m.StartTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	open := openGames(d)
	who := handles(ana, bea)
	w.moveEachIn(open[0], who)
	got, err := w.m.AbandonTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.TournamentAbandoned || len(openGames(got)) != 0 {
		t.Fatalf("abandoned: %+v", got)
	}
	for _, g := range open {
		game, err := w.st.GetGame(context.Background(), g.RoomCode)
		if err != nil || game.Status != "finished" || game.WinnerSeat != nil || game.EndReason != store.EndReasonAbandoned {
			t.Errorf("a zombie: %+v, %v", game, err)
		}
	}
	w.m.RecordResults(context.Background())
	if _, ok := w.rating(ana); ok {
		t.Error("an abandoned tournament moved a rating")
	}
	if _, err := w.m.AbandonTournament(org.ctx, id); !errors.Is(err, store.ErrWrongTournamentState) {
		t.Errorf("abandoning twice: %v", err)
	}
}

// The sweep is what ends a game whose flag fell with nobody looking.
func TestTournament_TheSweepEndsAFallenFlag(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea := w.account("org"), w.account("ana"), w.account("bea")
	s := tournamentSettings("round_robin")
	s.ClockSeconds = 60
	id := w.openTournament(s, org, ana, bea)
	d, err := w.m.StartTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	g := openGames(d)[0]
	w.clock.Advance(2 * time.Minute)
	w.m.AdvanceTournaments(context.Background())
	d = w.detail(org.ctx, id)
	if d.Status != store.TournamentFinished {
		t.Fatalf("after the flag: %+v", d.TournamentSummary)
	}
	if d.Standings[0].Handle != g.Second || d.Standings[0].Score != 1 {
		t.Errorf("the first player never moved, and lost on time: %+v", d.Standings)
	}
	// Nobody played a move, so it moved no rating.
	w.m.RecordResults(context.Background())
	if _, ok := w.rating(ana); ok {
		t.Error("a game nobody played moved a rating")
	}
}

// A long clock can outlast retention, and the results outlast the games.
func TestTournament_RetentionWaitsForTheEndAndTheResultsStay(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea := w.account("org"), w.account("ana"), w.account("bea")
	id := w.openTournament(tournamentSettings("round_robin"), org, ana, bea)
	d, err := w.m.StartTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	g := openGames(d)[0]
	ctx := context.Background()
	future := time.Now().Add(48 * time.Hour)
	n, err := w.st.DeleteInactiveGames(ctx, future)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("retention deleted %d games of a running tournament", n)
	}
	w.finishGame(g, handles(ana, bea), false)
	if d = w.detail(org.ctx, id); d.Status != store.TournamentFinished {
		t.Fatalf("after its game: %+v", d.TournamentSummary)
	}
	if n, err = w.st.DeleteInactiveGames(ctx, future); err != nil || n != 1 {
		t.Fatalf("retention after the end: %d, %v", n, err)
	}
	after := w.detail(org.ctx, id)
	if after.Standings[0].Handle != g.First || after.Standings[0].Score != 1 {
		t.Errorf("the standings lost their game: %+v", after.Standings)
	}
}

// A game that retention cannot touch while it runs still ends a round if it goes
// some other way: nobody scores, and the tournament is not stuck.
func TestTournament_AMissingGameDoesNotStopIt(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea := w.account("org"), w.account("ana"), w.account("bea")
	id := w.openTournament(tournamentSettings("round_robin"), org, ana, bea)
	if _, err := w.m.StartTournament(org.ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := w.st.DB().Exec(`DELETE FROM games`); err != nil {
		t.Fatal(err)
	}
	w.m.AdvanceTournaments(context.Background())
	if got := w.detail(org.ctx, id); got.Status != store.TournamentFinished || got.Standings[0].Score != 0 {
		t.Errorf("after losing its game: %+v", got)
	}
}

// A game no longer offered abandons its tournament, with nothing left behind.
func TestTournament_ARetiredGameAbandonsIt(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea := w.account("org"), w.account("ana"), w.account("bea")
	id := w.openTournament(tournamentSettings("round_robin"), org, ana, bea)
	d, err := w.m.StartTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	room := openGames(d)[0].RoomCode
	// The same database under a build without the game.
	session.NewManager(w.st, catalog.NewCatalog()).AdvanceTournaments(context.Background())
	got := w.detail(org.ctx, id)
	if got.Status != store.TournamentAbandoned || got.EndDetail == "" || len(openGames(got)) != 0 {
		t.Fatalf("after retiring its game: %+v", got)
	}
	game, err := w.st.GetGame(context.Background(), room)
	if err != nil || game.Status != "finished" {
		t.Errorf("its game: %+v, %v", game, err)
	}
}

// Somebody entering while the organiser presses Start must not throw the rounds out.
func TestTournament_StartingRefusesAStaleListOfEntries(t *testing.T) {
	w := newRatedWorld(t)
	org, ana, bea := w.account("org"), w.account("ana"), w.account("bea")
	id := w.openTournament(tournamentSettings("round_robin"), org, ana, bea)
	stale := []store.TournamentEntry{{AccountID: ana.id}}
	err := w.st.StartTournament(context.Background(), id, 1, 1, stale, time.Now())
	if !errors.Is(err, store.ErrWrongTournamentState) {
		t.Errorf("a stale list: %v", err)
	}
	if got := w.detail(org.ctx, id); got.Status != store.TournamentRegistration {
		t.Errorf("it started anyway: %+v", got.TournamentSummary)
	}
}
