package session

import (
	"context"
	"strings"
	"sync"

	"github.com/noderunner/bingle/internal/badges"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/store"
)

// Badges are earned during play or at the end of a game, and awarded once
// (ADR 0130). This is the end: it runs in the results worker as a finished
// game is counted, and its awards are written with the counts.

// platformBadges is the platform's own list, read once.
var platformBadges = sync.OnceValue(badges.Platform)

// gameBadge is a badge with the id it is known by.
type gameBadge struct {
	id  string
	def dsl.BadgeDef
	// own says the badge is the game's, which counts in its family only.
	own bool
}

// badgesOf is every badge a game of compiled can earn: the platform's and the
// game's own, whose ids are its family's.
func badgesOf(compiled *catalog.CompiledGame) []gameBadge {
	var out []gameBadge
	for _, b := range platformBadges() {
		out = append(out, gameBadge{id: b.ID, def: b})
	}
	family := gameFamily(compiled)
	for _, b := range compiled.Definition.Badges {
		out = append(out, gameBadge{id: badges.GameID(family, b.ID), def: b, own: true})
	}
	return out
}

// endGame is what an end badge is checked against.
type endGame struct {
	compiled *catalog.CompiledGame
	game     store.Game
	family   string
	facts    []state.Fact
	// bots is set in a game of one account against bots: the place of the
	// weakest of them in the server's list, weakest first.
	bots      bool
	weakest   int
	botPlaces map[string]int
}

// endAwards is the end badges a finished game earns the accounts at its
// seats. counts is what the game adds to their counts (nil in a game against
// bots), which the badges see as already added.
func (m *Manager) endAwards(ctx context.Context, compiled *catalog.CompiledGame, game store.Game, at table, facts []state.Fact, counts map[int64]map[string]int) ([]store.Award, error) {
	eg := endGame{compiled: compiled, game: game, family: gameFamily(compiled), facts: facts, bots: len(at.bots) > 0}
	if eg.bots {
		eg.botPlaces = map[string]int{}
		for i, b := range m.bots.List() {
			eg.botPlaces[b.ID] = i
		}
		eg.weakest = -1
		for _, id := range at.bots {
			place, ok := eg.botPlaces[id]
			if !ok {
				place = -1 // a bot the list does not know beats no badge
			}
			if eg.weakest == -1 || place < eg.weakest {
				eg.weakest = place
			}
		}
	}
	list := badgesOf(compiled)
	now := m.now()
	var out []store.Award
	for seat, account := range at.accounts {
		stats, err := m.store.Stats(ctx, account)
		if err != nil {
			return nil, err
		}
		for key, n := range counts[account] {
			stats[key] += n
		}
		for _, b := range list {
			if b.def.When != badges.WhenEnd || !m.earnsAtEnd(b, eg, seat, stats) {
				continue
			}
			out = append(out, store.Award{
				AccountID: account, Badge: b.id, Name: b.def.Name, Description: b.def.Description,
				Family: eg.family, GameID: game.ID, RoomCode: game.RoomCode, Detail: compiled.Definition.Name,
				AwardedAt: now, Visible: true,
			})
		}
	}
	return out, nil
}

// earnsAtEnd says whether seat earns b in the finished game eg, with stats
// the seat's account's counts after it.
func (m *Manager) earnsAtEnd(gb gameBadge, eg endGame, seat int, stats map[string]int) bool {
	b := gb.def
	// A bot badge counts games against bots only, and every other badge
	// games against people only.
	if (b.Bot != nil) != eg.bots {
		return false
	}
	if b.Bot != nil {
		need, ok := eg.botPlaces[b.Bot.AtLeast]
		if !ok {
			m.logger().Error("a bot badge names a bot the server does not have", "bot", b.Bot.AtLeast)
			return false
		}
		if eg.weakest < need {
			return false
		}
	}
	if b.Won && outcomeOf(eg.game, seat) != "won" {
		return false
	}
	if b.Rated && !ratedResult(eg.game) {
		return false
	}
	if s := b.Seat; s != nil {
		if s.Side != 0 && s.Side != seat {
			return false
		}
		if s.Faction != "" && state.SeatFaction(eg.compiled, seat) != s.Faction {
			return false
		}
	}
	if g := b.Game; g != nil {
		if g.Family != "" && g.Family != eg.family {
			return false
		}
		if g.Variant != "" && g.Variant != eg.compiled.Definition.ID {
			return false
		}
	}
	// A game's badge counts in its family, and a platform badge across
	// every family unless it names one.
	oneFamily := gb.own || (b.Game != nil && b.Game.Family != "")
	if c := b.Count; c != nil {
		var n int
		switch c.Within.Place() {
		case dsl.WithinGames:
			n = acrossGames(stats, resultKey(c.Fact, b.Rated), eg.family, oneFamily)
		case "window":
			open, closing, _ := c.Within.Window()
			n = inWindows(eg.facts, c.Fact, seat, open, closing)
		default:
			n = inWindows(eg.facts, c.Fact, seat, "", "")
		}
		if n < c.AtLeast {
			return false
		}
	}
	if d := b.Distinct; d != nil {
		prefix := store.CountMet
		if b.Rated {
			prefix = store.CountMetRated
		}
		if d.Of == badges.OfFamilies {
			prefix = resultKey("played", b.Rated) + ":"
		}
		if withPrefix(stats, prefix) < d.AtLeast {
			return false
		}
	}
	return true
}

// resultKey is the name a count across games is kept under: a result of
// every game that counts, a rated game's own result, or a fact.
func resultKey(fact string, rated bool) string {
	switch {
	case !badges.IsResult(fact):
		return store.CountFact + fact
	case rated:
		return fact
	}
	return store.CountGame + fact
}

// acrossGames is the count of what in family, or in every family.
func acrossGames(stats map[string]int, what, family string, oneFamily bool) int {
	if oneFamily {
		return stats[store.StatKey(what, family)]
	}
	n := 0
	for k, v := range stats {
		if strings.HasPrefix(k, what+":") {
			n += v
		}
	}
	return n
}

// withPrefix counts the keys of stats that start with prefix and are set.
func withPrefix(stats map[string]int, prefix string) int {
	n := 0
	for k, v := range stats {
		if v > 0 && strings.HasPrefix(k, prefix) {
			n++
		}
	}
	return n
}

// inWindows is the most times fact was reported about seat within one window:
// between a fact named open and the next named closing, whoever they name.
// With no window, the whole game is one.
func inWindows(facts []state.Fact, fact string, seat int, open, closing string) int {
	inside := open == ""
	best, n := 0, 0
	for _, f := range facts {
		switch {
		case open != "" && f.Name == open:
			inside, n = true, 0
		case open != "" && f.Name == closing:
			inside, n = false, 0
		case inside && f.Name == fact && f.Seat == seat:
			n++
			best = max(best, n)
		}
	}
	return best
}
