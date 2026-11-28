// Tests of the bot ladder (ADR 0160): who plays, what a run writes, and that
// it never reaches people's ratings.

package session_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/botladder"
	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/classics"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// weak keeps a test's bots quick: the ladder itself plays them at the
// strength players meet.
var weak = bots.Config{MCTSIterations: 20, MinimaxNodes: 300}

// newLadderWorld is a rated world of three small games, so that a run with
// weak bots takes a moment.
func newLadderWorld(t *testing.T) *ratedWorld {
	t.Helper()
	w := newRatedWorld(t)
	w.cat = catalog.NewCatalog()
	for _, mod := range classics.Modules() {
		if mod.ID == "yavalath" || mod.ID == "hex-11" || mod.ID == "breakthrough" {
			if err := w.cat.RegisterModule(mod); err != nil {
				t.Fatal(err)
			}
		}
	}
	w.m = session.NewManager(w.st, w.cat)
	w.m.SetClock(w.clock.Now)
	return w
}

func newLadder(w *ratedWorld, hold func(context.Context) (func(), error), limit time.Duration) *botladder.Ladder {
	return botladder.New(w.st, w.cat, hold, botladder.Options{Games: 2, Limit: limit, Strength: weak}, nil)
}

// peoplesRatings is everything about people's ratings a reader can see.
type peoplesRatings struct {
	A, B     store.Rating
	History  int
	Families []session.RatedFamily
	Board    []session.LeaderboardEntry
	Profile  []session.PlayerRating
}

func readPeoplesRatings(w *ratedWorld, a, b player) peoplesRatings {
	w.t.Helper()
	ctx := context.Background()
	var p peoplesRatings
	p.A, _ = w.rating(a)
	p.B, _ = w.rating(b)
	if err := w.st.DB().QueryRow(`SELECT COUNT(*) FROM rating_history`).Scan(&p.History); err != nil {
		w.t.Fatal(err)
	}
	var err error
	if p.Families, err = w.m.RatedFamilies(ctx); err != nil {
		w.t.Fatal(err)
	}
	if p.Board, err = w.m.Leaderboard(ctx, "yavalath", 0); err != nil {
		w.t.Fatal(err)
	}
	if p.Profile, err = w.m.PlayerRatings(a.ctx, a.handle); err != nil {
		w.t.Fatal(err)
	}
	return p
}

// A run rates Tactical, Minimax and MCTS in every family of two seats, in a
// table of their own: no person's rating, history, leaderboard or profile
// moves, and no bot appears among people.
func TestBotLadderKeepsApartFromPeople(t *testing.T) {
	w := newLadderWorld(t)
	a, b := w.account("ann"), w.account("bob")
	w.decisive(a, b)
	w.decisive(b, a)
	before := readPeoplesRatings(w, a, b)
	if before.A.Games != 2 || before.History != 4 {
		t.Fatalf("the rated games were not recorded: %+v", before)
	}

	games := botladder.Games(w.cat)
	sum, err := newLadder(w, nil, time.Minute).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := len(games) * 3 * 2; sum.Games != want || sum.Cut {
		t.Fatalf("a run played %+v, want %d games", sum, want)
	}
	if after := readPeoplesRatings(w, a, b); !reflect.DeepEqual(after, before) {
		t.Errorf("people's ratings moved:\nbefore %+v\nafter  %+v", before, after)
	}

	ladder, err := w.m.BotLadder(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ladder) != len(games) {
		t.Fatalf("%d games on the ladder, want %d: %+v", len(ladder), len(games), ladder)
	}
	for _, g := range ladder {
		if len(g.Bots) != 3 {
			t.Errorf("%s: bots %+v, want tactical, minimax and mcts", g.Family, g.Bots)
		}
		seen := map[string]bool{}
		for i, e := range g.Bots {
			seen[e.Bot] = true
			if e.Rank != i+1 || e.Games != 4 || e.Won+e.Drawn+e.Lost != e.Games || e.Label == "" {
				t.Errorf("%s: %+v", g.Family, e)
			}
			if i > 0 && e.Rating > g.Bots[i-1].Rating {
				t.Errorf("%s: not the highest first: %+v", g.Family, g.Bots)
			}
		}
		if !seen["tactical"] || !seen["minimax"] || !seen["mcts"] || seen["random"] {
			t.Errorf("%s: the wrong bots: %+v", g.Family, g.Bots)
		}
	}

	// A second run moves the same ratings on, from where the first left them.
	if _, err := newLadder(w, nil, time.Minute).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	again, _ := w.m.BotLadder(context.Background())
	if again[0].Bots[0].Games != 8 {
		t.Errorf("a second run: %+v", again[0])
	}
	if after := readPeoplesRatings(w, a, b); !reflect.DeepEqual(after, before) {
		t.Errorf("people's ratings moved on a second run")
	}
}

// The ladder plays every family of two seats that is not a prototype, as the
// game of the family's own id where there is one.
func TestBotLadderGames(t *testing.T) {
	w := newRatedWorld(t)
	games := botladder.Games(w.cat)
	if len(games) == 0 {
		t.Fatal("no games")
	}
	byFamily := map[string]botladder.Game{}
	for _, g := range games {
		byFamily[g.Family] = g
		cg, ok := w.cat.Get(g.GameID)
		if !ok || len(cg.Definition.Seats) != 2 || cg.Definition.Prototype {
			t.Errorf("%+v is not a game of two seats", g)
		}
	}
	if g := byFamily["hex-11"]; g.GameID != "hex-11" || g.Name != "Hex" {
		t.Errorf("Hex is played as %+v", g)
	}
}

// One run at a time: a run asked for while another waits or plays is
// refused, and a run that cannot have the batches' turn within its time
// limit is skipped and writes nothing.
func TestBotLadderOneRunAtATime(t *testing.T) {
	w := newLadderWorld(t)
	waiting := make(chan struct{})
	hold := func(ctx context.Context) (func(), error) {
		close(waiting)
		<-ctx.Done() // a batch that never finishes
		return nil, ctx.Err()
	}
	l := newLadder(w, hold, 200*time.Millisecond)
	done := make(chan error, 1)
	go func() {
		_, err := l.RunOnce(context.Background())
		done <- err
	}()
	<-waiting
	if _, err := l.RunOnce(context.Background()); !errors.Is(err, botladder.ErrBusy) {
		t.Errorf("a second run at once: %v, want ErrBusy", err)
	}
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a run behind a batch that never ends: %v", err)
	}
	if ladder, _ := w.m.BotLadder(context.Background()); len(ladder) != 0 {
		t.Errorf("a skipped run wrote %+v", ladder)
	}
}

// A run's time limit ends it, and what it played by then counts.
func TestBotLadderTimeLimit(t *testing.T) {
	w := newLadderWorld(t)
	l := botladder.New(w.st, w.cat, nil, botladder.Options{Games: 50, Limit: 300 * time.Millisecond, Strength: weak}, nil)
	start := time.Now()
	sum, err := l.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Cut || time.Since(start) > 10*time.Second {
		t.Errorf("a run of %d games a pairing was not cut short: %+v", 50, sum)
	}
}

// A run plays the families whose ratings are oldest first, those never
// played before all, so that a run cut short is followed by one that starts
// where it stopped.
func TestBotLadderOldestFirst(t *testing.T) {
	w := newLadderWorld(t)
	now := time.Now()
	var rows []store.BotRating
	for _, bot := range botladder.Bots {
		rows = append(rows,
			store.BotRating{Bot: bot, Family: "breakthrough", FamilyName: "Breakthrough", Rating: 1500, Deviation: 300, Volatility: 0.06, UpdatedAt: now},
			store.BotRating{Bot: bot, Family: "hex-11", FamilyName: "Hex", Rating: 1500, Deviation: 300, Volatility: 0.06, UpdatedAt: now.Add(-24 * time.Hour)})
	}
	if err := w.st.SetBotRatings(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	l := botladder.New(w.st, w.cat, nil, botladder.Options{Games: 1, Limit: time.Minute, Strength: weak}, nil)
	sum, err := l.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(sum.Order); got != "[yavalath hex-11 breakthrough]" {
		t.Errorf("played in the order %s", got)
	}
}

func TestBotLadderNext(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, c := range []struct {
		now     string
		minutes int
		want    string
	}{
		{"2026-11-28T10:00:00Z", 0, "2026-11-29T00:00:00Z"},
		{"2026-11-28T00:00:00Z", 0, "2026-11-29T00:00:00Z"},
		{"2026-11-28T23:59:59Z", 0, "2026-11-29T00:00:00Z"},
		{"2026-11-28T02:00:00Z", 210, "2026-11-28T03:30:00Z"},
		{"2026-11-28T02:00:00+05:00", 0, "2026-11-28T00:00:00Z"}, // 21:00 UTC the day before
	} {
		if got := botladder.Next(at(c.now), c.minutes); !got.Equal(at(c.want)) {
			t.Errorf("Next(%s, %d) = %s, want %s", c.now, c.minutes, got, c.want)
		}
	}
}
