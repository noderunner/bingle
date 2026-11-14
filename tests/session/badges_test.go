// Badges checked during play and at the end of a game (ADR 0130): the platform's and a game's
// own, awarded once, to accounts only, from games that count.

package session_test

import (
	"context"
	"sort"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// badgeFixtureDefJSON is a game of placing stones with badges of its own: a
// stone placed reports "placed", and two passes open and close a window.
const badgeFixtureDefJSON = `{
  "id": "fixture-badges",
  "name": "Fixture badges",
  "board": { "generator": { "type": "grid", "width": 4, "height": 4, "adjacency": "orthogonal4", "labels": "algebraic" } },
  "seats": [ { "seat": 1, "color": "black" }, { "seat": 2, "color": "white" } ],
  "pieces": { "black": { "color": "black" }, "white": { "color": "white" } },
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "actions": [
    { "id": "place", "type": "place", "pieces": ["black", "white"], "source": "supply", "fact": "placed" },
    { "id": "open", "type": "pass", "fact": "opened" },
    { "id": "close", "type": "pass", "fact": "closed" },
    { "id": "hide", "type": "pass", "fact": "hid" }
  ],
  "facts": {
    "placed": { "description": "A stone is placed." },
    "opened": { "description": "A window opens." },
    "closed": { "description": "A window closes." },
    "hid": { "description": "Something is hidden.", "secret": true }
  },
  "badges": [
    { "id": "builder", "name": "Builder", "description": "Place three stones in a game.", "when": "end",
      "count": { "fact": "placed", "at_least": 3 } },
    { "id": "burst", "name": "Burst", "description": "Place two stones in one window.", "when": "end",
      "count": { "fact": "placed", "at_least": 2, "within": ["opened", "closed"] } },
    { "id": "mason", "name": "Mason", "description": "Place four stones across games.", "when": "end",
      "count": { "fact": "placed", "at_least": 4, "within": "games" } },
    { "id": "second-wins", "name": "Second Wins", "description": "Win as the second seat.", "when": "end",
      "won": true, "seat": { "side": 2 } },
    { "id": "beat-tactical", "name": "Beat Tactical", "description": "Beat the tactical bot or better.", "when": "end",
      "won": true, "bot": { "at_least": "tactical" } },
    { "id": "handful", "name": "Handful", "description": "Place two stones in a game.", "when": "during",
      "count": { "fact": "placed", "at_least": 2 } },
    { "id": "hider", "name": "Hider", "description": "Hide something.", "when": "during",
      "count": { "fact": "hid", "at_least": 1 } }
  ],
  "win_condition": { "type": "none" }
}`

func (w *ratedWorld) withBadgeFixture() {
	w.t.Helper()
	if err := w.cat.Register("fixture-badges", []byte(badgeFixtureDefJSON), nil); err != nil {
		w.t.Fatal(err)
	}
}

// badgeIDs is the ids of an account's awards, sorted.
func (w *ratedWorld) badgeIDs(p player) []string {
	w.t.Helper()
	awards, err := w.st.Awards(context.Background(), p.id, true)
	if err != nil {
		w.t.Fatal(err)
	}
	ids := []string{}
	for _, a := range awards {
		ids = append(ids, a.Badge)
	}
	sort.Strings(ids)
	return ids
}

func (w *ratedWorld) play(room string, p player, action, piece, to string) {
	w.t.Helper()
	sel := state.Selection{Action: action, Piece: piece}
	if to != "" {
		sel.To = board.PositionID(to)
	}
	if _, err := w.m.Play(p.ctx, room, p.handle, sel); err != nil {
		w.t.Fatalf("%s %s %s: %v", p.handle, action, to, err)
	}
}

// finish ends a game with seat as its winner (0 for a draw), as a rule would,
// and lets the results worker look at it.
func (w *ratedWorld) finish(room string, seat int) {
	w.t.Helper()
	ctx := context.Background()
	g, err := w.st.GetGame(ctx, room)
	if err != nil {
		w.t.Fatal(err)
	}
	var winner *int
	if seat != 0 {
		winner = &seat
	}
	if err := w.st.FinishGame(ctx, g.ID, winner, "test"); err != nil {
		w.t.Fatal(err)
	}
	w.m.RecordResults(ctx)
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTheFirstWinIsAwardedOnceToTheWinner(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	for range 2 {
		room := w.unrated("yavalath", ana, bea)
		w.moveEach(room, ana, bea)
		w.resign(room, bea)
	}
	awards, err := w.st.Awards(context.Background(), ana.id, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(awards) != 1 || awards[0].Badge != "first-win" || awards[0].Name != "First Win" || awards[0].Family != "yavalath" || !awards[0].Visible {
		t.Fatalf("ana's awards = %+v, want one visible First Win in yavalath", awards)
	}
	if got := w.badgeIDs(bea); len(got) != 0 {
		t.Errorf("bea's awards = %v, want none", got)
	}
}

// A game's own badges count its facts in the game, in a window and across
// games, and are known by the family's name.
func TestAGamesBadgesCountItsFacts(t *testing.T) {
	w := newRatedWorld(t)
	w.withBadgeFixture()
	ana, bea := w.account("ana"), w.account("bea")

	room := w.unrated("fixture-badges", ana, bea)
	w.play(room, ana, "place", "black", "a1") // outside any window
	w.play(room, bea, "open", "", "")
	w.play(room, ana, "place", "black", "b1") // in the window
	w.play(room, bea, "place", "white", "a4")
	w.play(room, ana, "place", "black", "c1") // in the window: two
	w.play(room, bea, "close", "", "")
	w.finish(room, 2)

	if got, want := w.badgeIDs(ana), []string{"fixture-badges.builder", "fixture-badges.burst"}; !equal(got, want) {
		t.Errorf("ana's badges = %v, want %v", got, want)
	}
	if got, want := w.badgeIDs(bea), []string{"first-win", "fixture-badges.second-wins"}; !equal(got, want) {
		t.Errorf("bea's badges = %v, want %v", got, want)
	}

	// A second game takes ana past four stones across games.
	room = w.unrated("fixture-badges", ana, bea)
	w.play(room, ana, "place", "black", "a1")
	w.play(room, bea, "place", "white", "a4")
	w.finish(room, 0)
	if got := w.badgeIDs(ana); !equal(got, []string{"fixture-badges.builder", "fixture-badges.burst", "fixture-badges.mason"}) {
		t.Errorf("ana's badges after a second game = %v", got)
	}
}

// A bot badge needs a game of one account against bots, the win, and a bot at
// least as strong as it names; and a game against a bot earns no other badge.
func TestABotBadgeNamesTheBotBeaten(t *testing.T) {
	w := newRatedWorld(t)
	w.withBadgeFixture()
	ana := w.account("ana")
	against := func(bot string) {
		t.Helper()
		created, err := w.m.CreateGameWithOptions(ana.ctx, session.CreateOptions{GameDefID: "fixture-badges", CreatorName: ana.handle, Opponent: bot})
		if err != nil {
			t.Fatal(err)
		}
		w.play(created.RoomCode, ana, "place", "black", "a1")
		w.finish(created.RoomCode, 1)
	}
	against("random")
	if got := w.badgeIDs(ana); len(got) != 0 {
		t.Fatalf("beating the random bot earned %v, want nothing", got)
	}
	against("tactical")
	if got, want := w.badgeIDs(ana), []string{"fixture-badges.beat-tactical"}; !equal(got, want) {
		t.Errorf("beating the tactical bot earned %v, want %v", got, want)
	}
	if got := w.stats(ana); len(got) != 0 {
		t.Errorf("games against bots counted %v, want nothing", got)
	}
}

// A game that counts for nobody earns nobody a badge.
func TestANoShowEarnsNoBadge(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	room := w.unrated("yavalath", ana, bea)
	if _, err := w.m.PlayNotation(ana.ctx, room, ana.handle, "e5"); err != nil {
		t.Fatal(err)
	}
	w.resign(room, bea)
	if got := w.badgeIDs(ana); len(got) != 0 {
		t.Errorf("ana's badges = %v, want none", got)
	}
	if got := w.countStatus(room); got != store.CountSkipped {
		t.Errorf("count status = %q, want %q", got, store.CountSkipped)
	}
}

// visibleIDs is the ids of the awards everyone can see an account hold.
func (w *ratedWorld) visibleIDs(p player) []string {
	w.t.Helper()
	awards, err := w.st.Awards(context.Background(), p.id, false)
	if err != nil {
		w.t.Fatal(err)
	}
	ids := []string{}
	for _, a := range awards {
		ids = append(ids, a.Badge)
	}
	sort.Strings(ids)
	return ids
}

// A during badge is awarded as soon as it holds, without the game ending, and
// once however often the worker looks.
func TestADuringBadgeIsAwardedInPlay(t *testing.T) {
	w := newRatedWorld(t)
	w.withBadgeFixture()
	ana, bea := w.account("ana"), w.account("bea")
	ctx := context.Background()

	room := w.unrated("fixture-badges", ana, bea)
	w.play(room, ana, "place", "black", "a1")
	w.play(room, bea, "place", "white", "a4")
	w.m.CheckBadges(ctx)
	if got := w.badgeIDs(ana); len(got) != 0 {
		t.Fatalf("ana's badges after one stone = %v, want none", got)
	}
	w.play(room, ana, "place", "black", "b1")
	w.m.CheckBadges(ctx)
	w.m.CheckBadges(ctx)
	if got := w.visibleIDs(ana); !equal(got, []string{"fixture-badges.handful"}) {
		t.Errorf("ana's badges in play = %v, want handful, shown", got)
	}
	if got := w.badgeIDs(bea); len(got) != 0 {
		t.Errorf("bea's badges = %v, want none", got)
	}
	if got := w.countStatus(room); got != "" {
		t.Errorf("count status = %q, want the game still going", got)
	}
}

// An award that counts a secret fact is shown to its holder at once and to
// everyone else when the game ends.
func TestASecretAwardWaitsForTheEnd(t *testing.T) {
	w := newRatedWorld(t)
	w.withBadgeFixture()
	ana, bea := w.account("ana"), w.account("bea")
	ctx := context.Background()

	room := w.unrated("fixture-badges", ana, bea)
	w.play(room, ana, "hide", "", "")
	w.play(room, bea, "place", "white", "a4")
	w.m.CheckBadges(ctx)
	if got := w.badgeIDs(ana); !equal(got, []string{"fixture-badges.hider"}) {
		t.Fatalf("ana's own badges = %v, want hider", got)
	}
	if got := w.visibleIDs(ana); len(got) != 0 {
		t.Fatalf("ana's badges as others see them = %v, want none before the end", got)
	}
	w.finish(room, 0)
	if got := w.visibleIDs(ana); !equal(got, []string{"fixture-badges.hider"}) {
		t.Errorf("ana's badges after the end = %v, want hider, shown", got)
	}
}

// A secret award checked only after its game was counted is shown at once:
// nothing would show it later.
func TestASecretAwardAfterTheEndIsShown(t *testing.T) {
	w := newRatedWorld(t)
	w.withBadgeFixture()
	ana, bea := w.account("ana"), w.account("bea")

	room := w.unrated("fixture-badges", ana, bea)
	w.play(room, ana, "hide", "", "")
	w.play(room, bea, "place", "white", "a4")
	w.finish(room, 0)
	w.m.CheckBadges(context.Background())
	if got := w.visibleIDs(ana); !equal(got, []string{"fixture-badges.hider"}) {
		t.Errorf("ana's badges = %v, want hider, shown", got)
	}
}
