// Package badges holds the platform's badges and checks every badge, the
// platform's and each game's, against the parts a badge may be built from
// (ADR 0130). It knows nothing of games in play: the session works out who
// earns what.
package badges

import (
	"embed"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/noderunner/bingle/internal/engine/dsl"
)

//go:embed platform.yaml
var files embed.FS

// The results the platform counts in every game that counts, which a badge
// may count across games, and which no game may declare as a fact.
var Results = []string{"played", "won", "drawn", "lost"}

// The places a badge is checked.
const (
	WhenDuring = "during"
	WhenEnd    = "end"
)

// What a badge's distinct part counts.
const (
	OfOpponents = "opponents"
	OfFamilies  = "families"
)

// IsResult reports whether name is one of the platform's results.
func IsResult(name string) bool {
	for _, r := range Results {
		if r == name {
			return true
		}
	}
	return false
}

// Platform is the platform's badges, checked. A mistake in the embedded file
// is a build mistake, so it panics.
func Platform() []dsl.BadgeDef {
	raw, err := dsl.LoadAs(files, "platform.yaml", reflect.TypeFor[dsl.PlatformBadges]())
	if err != nil {
		panic(err)
	}
	var p dsl.PlatformBadges
	if err := json.Unmarshal(raw, &p); err != nil {
		panic(fmt.Errorf("badges: platform.yaml: %w", err))
	}
	if err := CheckAll(p.Badges, nil); err != nil {
		panic(fmt.Errorf("badges: platform.yaml: %w", err))
	}
	return p.Badges
}

// CheckAll checks a list of badges: the platform's when facts is nil, or a
// game's, which may count the facts it declares.
func CheckAll(list []dsl.BadgeDef, facts map[string]dsl.FactDef) error {
	seen := map[string]bool{}
	for _, b := range list {
		if seen[b.ID] {
			return fmt.Errorf("badge %q is listed twice", b.ID)
		}
		seen[b.ID] = true
		if err := Check(b, facts); err != nil {
			return err
		}
	}
	return nil
}

// Check checks one badge. facts is nil for a platform badge, which knows no
// game's facts, and the game's facts for a game's badge.
func Check(b dsl.BadgeDef, facts map[string]dsl.FactDef) error {
	game := facts != nil
	fail := func(format string, args ...any) error {
		return fmt.Errorf("badge %q: "+format, append([]any{b.ID}, args...)...)
	}
	if !badgeID(b.ID) {
		return fmt.Errorf("badge %q: an id is lower case letters, digits and dashes, starting with a letter", b.ID)
	}
	if strings.TrimSpace(b.Name) == "" || strings.TrimSpace(b.Description) == "" {
		return fail("a badge needs a name and a description")
	}
	if b.When != WhenDuring && b.When != WhenEnd {
		return fail("when is %q, not during or end", b.When)
	}
	// What depends on the end can only be checked at the end.
	endOnly := []struct {
		on   bool
		what string
	}{
		{b.Won, "won"}, {b.Rated, "rated"}, {b.Distinct != nil, "distinct"}, {b.Bot != nil, "bot"},
		{b.Count != nil && b.Count.Within.Place() == dsl.WithinGames, "a count across games"},
	}
	for _, e := range endOnly {
		if e.on && b.When != WhenEnd {
			return fail("%s is known only at the end: when must be end", e.what)
		}
	}
	declared := func(name string) bool { _, ok := facts[name]; return ok }
	if c := b.Count; c != nil {
		if c.AtLeast < 1 {
			return fail("count.at_least must be at least 1")
		}
		switch place := c.Within.Place(); place {
		case dsl.WithinGame, dsl.WithinGames:
		case "window":
			open, closing, _ := c.Within.Window()
			if !declared(open) || !declared(closing) {
				return fail("a window opens and closes on facts the game declares, not %q and %q", open, closing)
			}
		default:
			return fail("within is %q, not game, games or two facts", place)
		}
		switch {
		case IsResult(c.Fact):
			if c.Within.Place() != dsl.WithinGames {
				return fail("%s is counted across games: within must be games", c.Fact)
			}
		case !game:
			return fail("a platform badge counts only played, won, drawn or lost, not %q", c.Fact)
		case !declared(c.Fact):
			return fail("counts %q, which the game does not declare", c.Fact)
		}
	}
	if d := b.Distinct; d != nil {
		if d.Of != OfOpponents && d.Of != OfFamilies {
			return fail("distinct.of is %q, not opponents or families", d.Of)
		}
		if d.AtLeast < 1 {
			return fail("distinct.at_least must be at least 1")
		}
	}
	if b.Bot != nil {
		if b.Bot.AtLeast == "" {
			return fail("bot.at_least names the weakest bot that will do")
		}
		if !b.Won {
			return fail("a bot badge needs the win: won must be true")
		}
		if b.Rated || b.Distinct != nil {
			return fail("bots play no rated games, and a bot badge names a bot, never a number of them")
		}
	}
	if s := b.Seat; s != nil && s.Faction == "" && s.Side == 0 {
		return fail("seat names a faction or a side")
	}
	if g := b.Game; g != nil {
		if g.Family == "" && g.Variant == "" {
			return fail("game names a family or a variant")
		}
		if game && g.Family != "" {
			return fail("a game's badge is already its family's")
		}
	}
	return nil
}

func badgeID(s string) bool {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c == '-' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// GameID is the id a game's badge is known by: family.id.
func GameID(family, id string) string { return family + "." + id }
