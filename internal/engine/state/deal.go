package state

import (
	"math/rand"
	"slices"
	"strings"

	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// A declared game's cards (ADR 0119): dealt before the first move with the
// game's seed, and shown to each viewer as their zones allow.

// deal plays game's deal with seed: each shuffle fills its zone with a deck
// in a seeded random order (cards.RNG, Zone.Shuffle), and each deal draws
// from the top of its zone, one card to each seat in turn for a seat zone.
func deal(game *catalog.CompiledGame, seed uint64) *cards.Table {
	t := cards.NewTable()
	rng := &cards.RNG{State: seed}
	for _, d := range game.Deal {
		if d.Shuffle != "" {
			z := t.Zone(d.Shuffle, 0)
			z.Fill(game.Cards.Deck(d.Deck), false)
			z.Shuffle(rng)
			continue
		}
		from := t.Zone(d.From, 0)
		if d.ToSeats {
			hands := make([]*cards.Zone, len(game.Definition.Seats))
			before := make([]int, len(hands))
			for i, s := range game.Definition.Seats {
				hands[i] = t.Zone(d.To, s.Seat)
				before[i] = hands[i].Len()
			}
			cards.Deal(from, hands, d.Each)
			for i, h := range hands {
				for k := before[i]; k < h.Len(); k++ {
					h.Cards[k].Up = d.Up
				}
			}
			continue
		}
		to := t.Zone(d.To, 0)
		for _, c := range from.Draw(d.Count) {
			c.Up = d.Up
			to.Push(c)
		}
	}
	return t
}

// cardsSecret reports whether some zone of a declared game hides a card's
// face from somebody.
func cardsSecret(game *catalog.CompiledGame) bool {
	if game.Cards == nil || (len(game.Deal) == 0 && game.Patience == nil) {
		return false
	}
	for _, z := range game.Cards.Zones {
		if z.Visibility != "all" {
			return true
		}
	}
	return false
}

// redactCards returns st with the cards viewer may not see blanked out.
func redactCards(game *catalog.CompiledGame, st *BoardState, viewer int) *BoardState {
	if st == nil || st.table == nil || !cardsSecret(game) {
		return st
	}
	out := st.Clone()
	out.table = st.table.Redact(game.Cards, viewer)
	return out
}

// determinizeCards deals again, at random, every card viewer cannot see
// among the places such cards lie, so a search plans on a position it could
// be in.
func determinizeCards(game *catalog.CompiledGame, st *BoardState, viewer int, rng *rand.Rand) *BoardState {
	if st == nil || st.table == nil || !cardsSecret(game) {
		return st
	}
	seen := st.table.Redact(game.Cards, viewer)
	type slot struct {
		key string
		i   int
	}
	var slots []slot
	for key, z := range seen.Zones {
		for i, c := range z.Cards {
			if c.Hidden() {
				slots = append(slots, slot{key, i})
			}
		}
	}
	if len(slots) == 0 {
		return st
	}
	// Map order is random; a fixed order keeps a given rng reproducible.
	slices.SortFunc(slots, func(a, b slot) int {
		if c := strings.Compare(a.key, b.key); c != 0 {
			return c
		}
		return a.i - b.i
	})
	hidden := make([]string, len(slots))
	for k, s := range slots {
		hidden[k] = st.table.Zones[s.key].Cards[s.i].ID
	}
	rng.Shuffle(len(hidden), func(a, b int) { hidden[a], hidden[b] = hidden[b], hidden[a] })
	out := st.Clone()
	for k, s := range slots {
		out.table.Zones[s.key].Cards[s.i].ID = hidden[k]
	}
	return out
}

// cardsTurnedUp reports whether after lets some seat see the face of a card
// that before hid from it (ADR 0121): such a selection cannot be taken back.
func cardsTurnedUp(game *catalog.CompiledGame, before, after *BoardState) bool {
	if game.Cards == nil || before == nil || after == nil || after.table == nil || !cardsSecret(game) {
		return false
	}
	seen := func(t *cards.Table, viewer int) map[string]bool {
		out := map[string]bool{}
		if t == nil {
			return out
		}
		for _, z := range t.Redact(game.Cards, viewer).Zones {
			for _, c := range z.Cards {
				if !c.Hidden() {
					out[c.ID] = true
				}
			}
		}
		return out
	}
	for _, s := range game.Definition.Seats {
		was := seen(before.table, s.Seat)
		for id := range seen(after.table, s.Seat) {
			if !was[id] {
				return true
			}
		}
	}
	return false
}
