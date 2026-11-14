// Package badges_test covers how badges are declared (ADR 0130): the platform's
// list loads, every badge shipped names real bots, and a badge built wrongly is
// refused before the server starts.
package badges_test

import (
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/badges"
	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

func TestThePlatformsBadgesLoad(t *testing.T) {
	list := badges.Platform()
	if len(list) == 0 {
		t.Fatal("the platform has no badges")
	}
	for _, b := range list {
		if b.When != badges.WhenEnd && b.When != badges.WhenDuring {
			t.Errorf("%s: when %q", b.ID, b.When)
		}
	}
}

// A bot badge names a bot the server has, in the platform's list and in every
// game's.
func TestEveryBotBadgeNamesARealBot(t *testing.T) {
	known := map[string]bool{}
	for _, b := range bots.NewRegistry().List() {
		known[b.ID] = true
	}
	check := func(where string, list []dsl.BadgeDef) {
		for _, b := range list {
			if b.Bot != nil && !known[b.Bot.AtLeast] {
				t.Errorf("%s: badge %q names bot %q, which the server does not have", where, b.ID, b.Bot.AtLeast)
			}
		}
	}
	check("platform", badges.Platform())
	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatal(err)
		}
	}
	for _, g := range cat.List() {
		check(g.Definition.ID, g.Definition.Badges)
	}
}

func TestABadgeBuiltWronglyIsRefused(t *testing.T) {
	facts := map[string]dsl.FactDef{"spellbook": {Description: "x"}, "battle_start": {Description: "x"}, "battle_end": {Description: "x"}}
	ok := func() dsl.BadgeDef {
		return dsl.BadgeDef{ID: "deep-call", Name: "The Deep Calls", Description: "Two spellbooks in a battle.", When: "during",
			Count: &dsl.BadgeCount{Fact: "spellbook", AtLeast: 2, Within: dsl.Within{"battle_start", "battle_end"}}}
	}
	if err := badges.Check(ok(), facts); err != nil {
		t.Fatalf("the plan's example is refused: %v", err)
	}
	for name, c := range map[string]struct {
		edit     func(*dsl.BadgeDef)
		platform bool
		want     string
	}{
		"bad id":             {func(b *dsl.BadgeDef) { b.ID = "Deep_Call" }, false, "lower case"},
		"no name":            {func(b *dsl.BadgeDef) { b.Name = "" }, false, "name and a description"},
		"bad when":           {func(b *dsl.BadgeDef) { b.When = "later" }, false, "not during or end"},
		"won during play":    {func(b *dsl.BadgeDef) { b.Won = true }, false, "won is known only at the end"},
		"rated during play":  {func(b *dsl.BadgeDef) { b.Rated = true }, false, "rated is known only at the end"},
		"undeclared fact":    {func(b *dsl.BadgeDef) { b.Count.Fact = "gate" }, false, `"gate", which the game does not declare`},
		"undeclared window":  {func(b *dsl.BadgeDef) { b.Count.Within = dsl.Within{"turn_start", "battle_end"} }, false, "a window opens and closes"},
		"zero count":         {func(b *dsl.BadgeDef) { b.Count.AtLeast = 0 }, false, "at least 1"},
		"result in a game":   {func(b *dsl.BadgeDef) { b.When = "end"; b.Count = &dsl.BadgeCount{Fact: "won", AtLeast: 3} }, false, "within must be games"},
		"across during play": {func(b *dsl.BadgeDef) { b.Count.Within = dsl.Within{"games"} }, false, "only at the end"},
		"platform fact":      {func(b *dsl.BadgeDef) { b.Count.Within = nil }, true, "counts only played, won, drawn or lost"},
		"bad distinct": {func(b *dsl.BadgeDef) {
			b.When, b.Count, b.Distinct = "end", nil, &dsl.BadgeDistinct{Of: "games", AtLeast: 2}
		}, false, "not opponents or families"},
		"bot without win": {func(b *dsl.BadgeDef) {
			b.When, b.Count, b.Bot = "end", nil, &dsl.BadgeBot{AtLeast: "mcts"}
		}, false, "needs the win"},
		"bot across games": {func(b *dsl.BadgeDef) {
			b.When, b.Won, b.Bot = "end", true, &dsl.BadgeBot{AtLeast: "mcts"}
			b.Count = &dsl.BadgeCount{Fact: "spellbook", AtLeast: 3, Within: dsl.Within{"games"}}
		}, false, "games against bots add to no count"},
		"family in a game": {func(b *dsl.BadgeDef) { b.Game = &dsl.BadgeGame{Family: "hex"} }, false, "already its family's"},
		"empty seat":       {func(b *dsl.BadgeDef) { b.Seat = &dsl.BadgeSeat{} }, false, "faction or a side"},
	} {
		b := ok()
		c.edit(&b)
		f := facts
		if c.platform {
			f = nil
		}
		err := badges.Check(b, f)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want one saying %q", name, err, c.want)
		}
	}
}

// A game's badges are checked when it is registered.
func TestTheCatalogChecksAGamesBadges(t *testing.T) {
	def := `{"id":"bx","name":"Bx","board":{"generator":{"type":"grid","width":3,"height":3,"adjacency":"orthogonal4"}},
	  "seats":[{"seat":1,"color":"black"},{"seat":2,"color":"white"}],"pieces":{"stone":{"movement":{"primitive":"step","occupancy":"empty-only"}}},
	  "turn_order":{"type":"round-robin","seats":[1,2]},"win_condition":{"type":"none"},
	  "badges":[{"id":"x","name":"X","description":"X.","when":"end","count":{"fact":"spellbook","at_least":1}}]}`
	err := catalog.NewCatalog().Register("bx", []byte(def), nil)
	if err == nil || !strings.Contains(err.Error(), "spellbook") {
		t.Fatalf("error %v, want the undeclared fact refused", err)
	}
}
