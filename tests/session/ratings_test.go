// Tests of rated games: who may play one, what it allows, and what it does to
// its players when it ends.

package session_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/classics"
	"github.com/noderunner/bingle/internal/rating"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// ratedWorld is a server's worth of rated play over the classics.
type ratedWorld struct {
	t     *testing.T
	st    *store.Store
	cat   *catalog.Catalog
	m     *session.Manager
	clock *fakeClock
	// seconds is the clock of the games it starts, when it matters.
	seconds int
}

func newRatedWorld(t *testing.T) *ratedWorld {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "rated.db"))
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
	return &ratedWorld{t: t, st: st, cat: cat, m: m, clock: clock}
}

// player is a request context signed in as an account, with its handle.
type player struct {
	ctx    context.Context
	handle string
	id     int64
}

func (w *ratedWorld) account(handle string) player {
	w.t.Helper()
	ctx := context.Background()
	principal := "p-" + handle
	id, err := w.st.CreateAccount(ctx, store.NewAccount{
		Handle: handle, DisplayName: handle, Email: handle + "@example.test", PrincipalID: principal, NewPrincipal: true, Verified: true,
	})
	if err != nil {
		w.t.Fatal(err)
	}
	acc, err := w.st.AccountByID(ctx, id)
	if err != nil {
		w.t.Fatal(err)
	}
	return player{
		ctx:    auth.WithPrincipal(ctx, auth.Principal{ID: acc.PrincipalID, Kind: store.KindAccount, Account: &acc}),
		handle: handle, id: id,
	}
}

func ratedOpts(by player) session.CreateOptions {
	return session.CreateOptions{GameDefID: "yavalath", CreatorName: by.handle, ClockSeconds: []int{600, 600}, Rated: true}
}

// start makes a rated game between two accounts.
func (w *ratedWorld) start(a, b player) string {
	w.t.Helper()
	opts := ratedOpts(a)
	if w.seconds > 0 {
		opts.ClockSeconds = []int{w.seconds, w.seconds}
	}
	created, err := w.m.CreateGameWithOptions(a.ctx, opts)
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.m.JoinGame(b.ctx, created.RoomCode, b.handle, session.RolePlayer); err != nil {
		w.t.Fatal(err)
	}
	return created.RoomCode
}

// moveEach has each player make a move, so that the game counts.
func (w *ratedWorld) moveEach(room string, a, b player) {
	w.t.Helper()
	for _, mv := range []struct {
		p player
		n string
	}{{a, "e5"}, {b, "d4"}} {
		if _, err := w.m.PlayNotation(mv.p.ctx, room, mv.p.handle, mv.n); err != nil {
			w.t.Fatal(err)
		}
	}
}

// decisive plays a game to a's win and records it.
func (w *ratedWorld) decisive(a, b player) string {
	w.t.Helper()
	room := w.start(a, b)
	w.moveEach(room, a, b)
	if _, err := w.m.Resign(b.ctx, room, b.handle); err != nil {
		w.t.Fatal(err)
	}
	w.m.RecordResults(context.Background())
	return room
}

func (w *ratedWorld) rating(p player) (store.Rating, bool) {
	w.t.Helper()
	r, ok, err := w.st.RatingOf(context.Background(), p.id, "yavalath")
	if err != nil {
		w.t.Fatal(err)
	}
	return r, ok
}

func (w *ratedWorld) status(room string) string {
	w.t.Helper()
	g, err := w.st.GetGame(context.Background(), room)
	if err != nil {
		w.t.Fatal(err)
	}
	return g.RatingStatus
}

func TestRated_WhatCannotBeRated(t *testing.T) {
	w := newRatedWorld(t)
	ana := w.account("ana")
	guest := auth.WithPrincipal(context.Background(), auth.Principal{ID: "a-guest", Kind: store.KindGuest})
	for name, c := range map[string]struct {
		ctx    context.Context
		change func(*session.CreateOptions)
	}{
		"a guest":          {guest, func(*session.CreateOptions) {}},
		"nobody":           {context.Background(), func(*session.CreateOptions) {}},
		"a bot":            {ana.ctx, func(o *session.CreateOptions) { o.Opponent = "random" }},
		"a bot in a seat":  {ana.ctx, func(o *session.CreateOptions) { o.SeatBots = map[int]string{2: "random"} }},
		"oneself":          {ana.ctx, func(o *session.CreateOptions) { o.Also = []int{2} }},
		"no clock":         {ana.ctx, func(o *session.CreateOptions) { o.ClockSeconds = nil }},
		"a time per move":  {ana.ctx, func(o *session.CreateOptions) { o.ClockSeconds, o.MoveTimeSeconds = nil, 60 }},
		"a handicap":       {ana.ctx, func(o *session.CreateOptions) { o.ClockSeconds = []int{60, 600} }},
		"a chosen opening": {ana.ctx, func(o *session.CreateOptions) { o.StartState, o.StartSeat = "{}", 1 }},
	} {
		opts := ratedOpts(ana)
		opts.CreatorName = "Somebody"
		c.change(&opts)
		if _, err := w.m.CreateGameWithOptions(c.ctx, opts); !errors.Is(err, session.ErrCannotBeRated) {
			t.Errorf("%s: %v, want ErrCannotBeRated", name, err)
		}
	}
}

func TestRated_WhoMayJoin(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	created, err := w.m.CreateGameWithOptions(ana.ctx, ratedOpts(ana))
	if err != nil {
		t.Fatal(err)
	}
	room := created.RoomCode
	guest := auth.WithPrincipal(context.Background(), auth.Principal{ID: "a-guest", Kind: store.KindGuest})
	if _, err := w.m.JoinGame(guest, room, "Gus", session.RolePlayer); !errors.Is(err, session.ErrCannotBeRated) {
		t.Errorf("a guest joined: %v", err)
	}
	if _, err := w.m.JoinGame(ana.ctx, room, "Ana's other name", session.RolePlayer); !errors.Is(err, session.ErrCannotBeRated) {
		t.Errorf("the same account took both seats: %v", err)
	}
	if _, err := w.m.AddBot(ana.ctx, room, ana.handle, "random"); !errors.Is(err, session.ErrRatedGame) {
		t.Errorf("a bot joined: %v", err)
	}
	// Watching is anybody's, and coming back to one's own seat is fine.
	if _, err := w.m.JoinGame(guest, room, "Gus", session.RoleSpectator); err != nil {
		t.Errorf("a guest could not watch: %v", err)
	}
	if _, err := w.m.JoinGame(ana.ctx, room, ana.handle, session.RolePlayer); err != nil {
		t.Errorf("ana could not come back: %v", err)
	}
	if _, err := w.m.JoinGame(bea.ctx, room, bea.handle, session.RolePlayer); err != nil {
		t.Errorf("a second account could not join: %v", err)
	}
}

// Nothing in a rated game can change a result, because a rating is written once.
func TestRated_MovesStand(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	room := w.start(ana, bea)
	w.moveEach(room, ana, bea)
	if _, err := w.m.Undo(ana.ctx, room, ana.handle); !errors.Is(err, session.ErrCannotUndo) {
		t.Errorf("undo: %v", err)
	}
	on := true
	if _, err := w.m.SetAutoPlay(ana.ctx, room, ana.handle, &on, nil); !errors.Is(err, session.ErrRatedGame) {
		t.Errorf("auto-play: %v", err)
	}
	if _, err := w.m.Resign(bea.ctx, room, bea.handle); err != nil {
		t.Fatal(err)
	}
	w.m.RecordResults(context.Background())
	if _, err := w.m.Undo(bea.ctx, room, bea.handle); !errors.Is(err, session.ErrCannotUndo) {
		t.Errorf("undo after the game: %v", err)
	}
}

func TestRated_AWinMovesBothAndWritesItAllDown(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	room := w.decisive(ana, bea)

	if got := w.status(room); got != store.RatingRecorded {
		t.Errorf("the game's status is %q", got)
	}
	ra, okA := w.rating(ana)
	rb, okB := w.rating(bea)
	if !okA || !okB {
		t.Fatalf("ratings: %v %v", okA, okB)
	}
	if ra.Rating <= rating.InitialRating || rb.Rating >= rating.InitialRating {
		t.Errorf("the winner has %v and the loser %v", ra.Rating, rb.Rating)
	}
	if ra.Games != 1 || ra.Opponents != 1 || ra.FamilyName != "Yavalath" {
		t.Errorf("the winner's record: %+v", ra)
	}
	stats, err := w.st.Stats(context.Background(), ana.id)
	if err != nil {
		t.Fatal(err)
	}
	if stats["played:yavalath"] != 1 || stats["won:yavalath"] != 1 || stats["lost:yavalath"] != 0 {
		t.Errorf("the counters: %v", stats)
	}
	snap, err := w.m.GetState(ana.ctx, room, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Rated || len(snap.RatingChanges) != 2 {
		t.Fatalf("the snapshot: rated %v, changes %+v", snap.Rated, snap.RatingChanges)
	}
	if c := snap.RatingChanges[0]; c.Seat != 1 || c.Before != 1500 || c.After <= c.Before {
		t.Errorf("what it did to seat 1: %+v", c)
	}
	if snap.Seats[0].Rating == nil || snap.Seats[0].Rating.Rating != snap.RatingChanges[0].After || !snap.Seats[0].Rating.Provisional {
		t.Errorf("the seat shows %+v", snap.Seats[0].Rating)
	}
}

func TestRated_EveryWayOfEnding(t *testing.T) {
	t.Run("an agreed draw moves new players nowhere", func(t *testing.T) {
		w := newRatedWorld(t)
		ana, bea := w.account("ana"), w.account("bea")
		room := w.start(ana, bea)
		w.moveEach(room, ana, bea)
		if _, err := w.m.Draw(ana.ctx, room, ana.handle, "offer"); err != nil {
			t.Fatal(err)
		}
		if _, err := w.m.Draw(bea.ctx, room, bea.handle, "accept"); err != nil {
			t.Fatal(err)
		}
		w.m.RecordResults(context.Background())
		ra, ok := w.rating(ana)
		if !ok || ra.Rating < 1499.9 || ra.Rating > 1500.1 || ra.Games != 1 {
			t.Errorf("after a draw: %+v, %v", ra, ok)
		}
		stats, _ := w.st.Stats(context.Background(), ana.id)
		if stats["drawn:yavalath"] != 1 {
			t.Errorf("the counters: %v", stats)
		}
	})
	t.Run("a flag", func(t *testing.T) {
		w := newRatedWorld(t)
		ana, bea := w.account("ana"), w.account("bea")
		room := w.start(ana, bea)
		w.moveEach(room, ana, bea)
		// Ana is to move, and runs out.
		w.clock.Advance(11 * time.Minute)
		if _, err := w.m.GetState(ana.ctx, room, nil); err != nil {
			t.Fatal(err)
		}
		w.m.RecordResults(context.Background())
		rb, ok := w.rating(bea)
		if !ok || rb.Rating <= rating.InitialRating {
			t.Errorf("the player who kept their time: %+v, %v", rb, ok)
		}
	})
	t.Run("on the board", func(t *testing.T) {
		w := newRatedWorld(t)
		ana, bea := w.account("ana"), w.account("bea")
		room := w.start(ana, bea)
		who := map[int]player{1: ana, 2: bea}
		for range 400 {
			seat, legal, err := w.m.LegalMoves(ana.ctx, room, 0)
			if err != nil {
				t.Fatal(err)
			}
			mover := who[seat]
			out, err := w.m.PlayNotation(mover.ctx, room, mover.handle, legal[0].Notation)
			if err != nil {
				t.Fatal(err)
			}
			if out.GameEnded {
				break
			}
		}
		w.m.RecordResults(context.Background())
		if got := w.status(room); got != store.RatingRecorded {
			t.Errorf("after a game played out, the status is %q", got)
		}
	})
}

// A game somebody never moved in is not a game: a no-show, or a way to hand
// somebody a win.
func TestRated_AGameNobodyPlayedCountsForNothing(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")

	room := w.start(ana, bea)
	if _, err := w.m.Resign(bea.ctx, room, bea.handle); err != nil {
		t.Fatal(err)
	}
	w.m.RecordResults(context.Background())
	if got := w.status(room); got != store.RatingNoMoves {
		t.Errorf("resigning at once: %q", got)
	}

	// Ana moved, and Bea never came.
	room = w.start(ana, bea)
	if _, err := w.m.PlayNotation(ana.ctx, room, ana.handle, "e5"); err != nil {
		t.Fatal(err)
	}
	w.clock.Advance(11 * time.Minute)
	if _, err := w.m.GetState(ana.ctx, room, nil); err != nil {
		t.Fatal(err)
	}
	w.m.RecordResults(context.Background())
	if got := w.status(room); got != store.RatingNoMoves {
		t.Errorf("a no-show: %q", got)
	}
	if _, ok := w.rating(ana); ok {
		t.Error("a rating came of games nobody played")
	}
}

// Two accounts cannot feed each other wins all day.
func TestRated_ADailyLimitBetweenTheSameTwo(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	for range rating.MaxGamesPerDay {
		w.decisive(ana, bea)
	}
	full, _ := w.rating(ana)
	room := w.decisive(ana, bea)
	if got := w.status(room); got != store.RatingDailyLimit {
		t.Errorf("one game too many: %q", got)
	}
	if after, _ := w.rating(ana); after.Games != full.Games || after.Rating != full.Rating {
		t.Errorf("it changed the rating: %+v against %+v", after, full)
	}
	// Somebody else is a different matter, and so is tomorrow.
	cy := w.account("cy")
	if got := w.status(w.decisive(ana, cy)); got != store.RatingRecorded {
		t.Errorf("against a third player: %q", got)
	}
	// The database stamps its rows with the real clock, so this game's clock
	// has to outlast the jump.
	w.seconds = session.MaxClockSeconds
	w.clock.Advance(25 * time.Hour)
	if got := w.status(w.decisive(ana, bea)); got != store.RatingRecorded {
		t.Errorf("the next day: %q", got)
	}
}

// A game that ends is marked in the transaction that ends it, so a crash before
// the result is written delays it and loses nothing, and it counts only once.
func TestRated_ACrashDelaysAResultAndNeverLosesOne(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	room := w.start(ana, bea)
	w.moveEach(room, ana, bea)
	if err := w.st.FinishGame(context.Background(), mustGame(t, w, room).ID, intp(1), "resignation"); err != nil {
		t.Fatal(err)
	}
	if got := w.status(room); got != store.RatingPending {
		t.Fatalf("a game just ended is %q", got)
	}
	// Another server, after the crash.
	next := session.NewManager(w.st, w.cat)
	next.SetClock(w.clock.Now)
	next.RecordResults(context.Background())
	next.RecordResults(context.Background())
	w.m.RecordResults(context.Background())
	if ra, ok := w.rating(ana); !ok || ra.Games != 1 {
		t.Errorf("after the crash: %+v, %v", ra, ok)
	}
}

// Retention deletes games. A rating must not care.
func TestRated_ARatingOutlivesItsGames(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	w.decisive(ana, bea)
	before, _ := w.rating(ana)
	if n, err := w.st.DeleteInactiveGames(context.Background(), time.Now().Add(time.Hour)); err != nil || n == 0 {
		t.Fatalf("retention removed %d: %v", n, err)
	}
	after, ok := w.rating(ana)
	if !ok || after.Rating != before.Rating {
		t.Errorf("after retention: %+v, %v", after, ok)
	}
}

// The seats are the players' after a pie-rule swap, so the rating follows the
// person who won rather than the seat they began in.
func TestRated_ASwapDoesNotConfuseTheResult(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	opts := ratedOpts(ana)
	opts.Options = map[string]any{"pie-rule": true}
	created, err := w.m.CreateGameWithOptions(ana.ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	room := created.RoomCode
	if _, err := w.m.JoinGame(bea.ctx, room, bea.handle, session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.PlayNotation(ana.ctx, room, ana.handle, "e5"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.PlayNotation(bea.ctx, room, bea.handle, "swap"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.Resign(ana.ctx, room, ana.handle); err != nil {
		t.Fatal(err)
	}
	w.m.RecordResults(context.Background())
	rb, ok := w.rating(bea)
	if !ok || rb.Rating <= rating.InitialRating {
		t.Errorf("Bea, who swapped and won, has %+v (%v)", rb, ok)
	}
	if ra, _ := w.rating(ana); ra.Rating >= rating.InitialRating {
		t.Errorf("Ana, who resigned, has %+v", ra)
	}
}

// One rating covers a family: every size of a game, and a game made with options.
func TestRated_OneRatingAcrossAFamily(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	for _, id := range []string{"hex-7", "hex-11"} {
		opts := ratedOpts(ana)
		opts.GameDefID = id
		created, err := w.m.CreateGameWithOptions(ana.ctx, opts)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.m.JoinGame(bea.ctx, created.RoomCode, bea.handle, session.RolePlayer); err != nil {
			t.Fatal(err)
		}
		_, legal, err := w.m.LegalMoves(ana.ctx, created.RoomCode, 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.m.PlayNotation(ana.ctx, created.RoomCode, ana.handle, legal[0].Notation); err != nil {
			t.Fatal(err)
		}
		_, legal, err = w.m.LegalMoves(ana.ctx, created.RoomCode, 2)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.m.PlayNotation(bea.ctx, created.RoomCode, bea.handle, legal[0].Notation); err != nil {
			t.Fatal(err)
		}
		if _, err := w.m.Resign(bea.ctx, created.RoomCode, bea.handle); err != nil {
			t.Fatal(err)
		}
		w.m.RecordResults(context.Background())
	}
	r, ok, err := w.st.RatingOf(context.Background(), ana.id, "hex-11")
	if err != nil || !ok {
		t.Fatalf("the rating: %v, %v", ok, err)
	}
	if r.Games != 2 || r.FamilyName != "Hex" {
		t.Errorf("across the family: %+v", r)
	}
}

// A leaderboard takes enough games against enough people, and says nothing of
// when or whom. An account always sees its own.
func TestRated_WhoIsShown(t *testing.T) {
	w := newRatedWorld(t)
	ctx := context.Background()
	ana := w.account("ana")
	rivals := make([]player, rating.MinOpponents)
	for i := range rivals {
		rivals[i] = w.account(fmt.Sprintf("rival%d", i))
	}
	stranger := w.account("zed")

	// Not yet: too few opponents, however many games.
	for range rating.MinGames {
		w.decisive(ana, rivals[0])
	}
	if board, _ := w.m.Leaderboard(ctx, "yavalath", 0); len(board) != 0 {
		t.Errorf("too few opponents, and the board shows %+v", board)
	}
	if own, _ := w.m.PlayerRatings(ana.ctx, "ana"); len(own) != 1 || own[0].Listed {
		t.Errorf("ana's own view: %+v", own)
	}
	if theirs, _ := w.m.PlayerRatings(stranger.ctx, "ana"); len(theirs) != 0 {
		t.Errorf("a stranger sees %+v", theirs)
	}

	for _, r := range rivals[1:] {
		w.decisive(ana, r)
	}
	board, err := w.m.Leaderboard(ctx, "yavalath", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(board) != 1 || board[0].Handle != "ana" || board[0].Rank != 1 || board[0].Games != rating.MinGames+rating.MinOpponents-1 {
		t.Errorf("the board: %+v", board)
	}
	games, err := w.m.RatedFamilies(ctx)
	if err != nil || len(games) != 1 || games[0].Family != "yavalath" || games[0].Name != "Yavalath" || games[0].Players != 1 {
		t.Errorf("the games with a board: %+v, %v", games, err)
	}
	theirs, err := w.m.PlayerRatings(stranger.ctx, "ana")
	if err != nil {
		t.Fatal(err)
	}
	if len(theirs) != 1 || !theirs[0].Listed || theirs[0].Won != board[0].Games || theirs[0].Lost != 0 {
		t.Errorf("a stranger's view once listed: %+v", theirs)
	}
	if _, err := w.m.PlayerRatings(ctx, "nobody"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("an unknown handle: %v", err)
	}
}

func mustGame(t *testing.T, w *ratedWorld, room string) store.Game {
	t.Helper()
	g, err := w.st.GetGame(context.Background(), room)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func intp(v int) *int { return &v }
