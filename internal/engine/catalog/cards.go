package catalog

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/dsl"
)

// CardType is one kind of card: a suit and rank of a template deck, or an
// explicit card. Several identical cards (a rank with count 3) share a type.
type CardType struct {
	ID      string
	Deck    string
	Suit    string
	Rank    string
	Label   string
	Value   int
	Text    string
	Symbols []string
	Tags    []string
}

// HasTag reports whether the card type carries tag.
func (t *CardType) HasTag(tag string) bool {
	for _, x := range t.Tags {
		if x == tag {
			return true
		}
	}
	return false
}

// CompiledCards is a game's decks, expanded: every card type, and for each
// deck the instance ids of its cards in a canonical order. An instance id is
// the type id, followed by "#n" when the deck has several copies of it.
type CompiledCards struct {
	Def       *dsl.CardsDef
	Types     map[string]*CardType
	Instances map[string][]string // deck name → instance ids, canonical order
	Suits     []string            // suit ids in display order
	Zones     map[string]dsl.ZoneDef
}

// Deck returns the instance ids of a deck in canonical (unshuffled) order.
func (c *CompiledCards) Deck(name string) []string {
	if c == nil {
		return nil
	}
	return append([]string(nil), c.Instances[name]...)
}

// TypeID strips the copy number from an instance id.
func TypeID(instance string) string {
	if i := strings.IndexByte(instance, '#'); i >= 0 {
		return instance[:i]
	}
	return instance
}

// Type returns the card type of an instance id (nil if unknown).
func (c *CompiledCards) Type(instance string) *CardType {
	if c == nil {
		return nil
	}
	return c.Types[TypeID(instance)]
}

func compileCards(cg *CompiledGame) error {
	def := cg.Definition.Cards
	if def == nil {
		return nil
	}
	cc := &CompiledCards{Def: def, Types: map[string]*CardType{}, Instances: map[string][]string{}, Zones: map[string]dsl.ZoneDef{}}
	if len(def.Decks) == 0 {
		return fmt.Errorf("catalog: cards define no decks")
	}
	for id := range def.Suits {
		cc.Suits = append(cc.Suits, id)
	}
	sort.Strings(cc.Suits)
	if len(def.SuitOrder) > 0 {
		seen := map[string]bool{}
		var ordered []string
		for _, id := range def.SuitOrder {
			if _, ok := def.Suits[id]; !ok {
				return fmt.Errorf("catalog: suit_order names unknown suit %q", id)
			}
			if !seen[id] {
				seen[id] = true
				ordered = append(ordered, id)
			}
		}
		for _, id := range cc.Suits {
			if !seen[id] {
				ordered = append(ordered, id)
			}
		}
		cc.Suits = ordered
	}
	rank := map[string]int{}
	for i, id := range cc.Suits {
		rank[id] = i
	}
	deckNames := make([]string, 0, len(def.Decks))
	for name := range def.Decks {
		deckNames = append(deckNames, name)
	}
	sort.Strings(deckNames)
	for _, name := range deckNames {
		deck := def.Decks[name]
		removed := map[string]bool{}
		for _, id := range deck.Remove {
			removed[id] = true
		}
		type entry struct {
			t     *CardType
			count int
		}
		var entries []entry
		add := func(t *CardType, count int) error {
			if count < 1 {
				count = 1
			}
			if t.ID == "" {
				return fmt.Errorf("catalog: deck %q has a card without an id", name)
			}
			if _, dup := cc.Types[t.ID]; dup {
				return fmt.Errorf("catalog: card %q is defined twice (decks share one namespace of card ids)", t.ID)
			}
			if t.Suit != "" {
				if _, ok := def.Suits[t.Suit]; !ok {
					return fmt.Errorf("catalog: card %q has unknown suit %q", t.ID, t.Suit)
				}
			}
			if removed[t.ID] {
				return nil
			}
			cc.Types[t.ID] = t
			entries = append(entries, entry{t, count})
			return nil
		}
		if len(deck.Ranks) > 0 {
			suits := deck.Suits
			if len(suits) == 0 {
				suits = cc.Suits
			}
			for _, s := range suits {
				if _, ok := def.Suits[s]; !ok {
					return fmt.Errorf("catalog: deck %q lists unknown suit %q", name, s)
				}
			}
			sort.SliceStable(suits, func(i, j int) bool { return rank[suits[i]] < rank[suits[j]] })
			seenRank := map[string]bool{}
			for _, r := range deck.Ranks {
				if r.ID == "" || seenRank[r.ID] {
					return fmt.Errorf("catalog: deck %q has a missing or repeated rank %q", name, r.ID)
				}
				seenRank[r.ID] = true
			}
			for _, s := range suits {
				for _, r := range deck.Ranks {
					label := r.Label
					if label == "" {
						label = r.ID
					}
					t := &CardType{ID: s + "-" + r.ID, Deck: name, Suit: s, Rank: r.ID, Label: label, Value: r.Value, Text: r.Text, Symbols: r.Symbols, Tags: r.Tags}
					if err := add(t, r.Count); err != nil {
						return err
					}
				}
			}
		}
		for _, c := range deck.Extra {
			label := c.Label
			if label == "" {
				label = c.ID
			}
			t := &CardType{ID: c.ID, Deck: name, Suit: c.Suit, Rank: c.ID, Label: label, Value: c.Value, Text: c.Text, Symbols: c.Symbols, Tags: c.Tags}
			if err := add(t, c.Count); err != nil {
				return err
			}
		}
		for id := range removed {
			if _, ok := cc.Types[id]; !ok && !knownRemoval(deck, id) {
				return fmt.Errorf("catalog: deck %q removes unknown card %q", name, id)
			}
		}
		for _, e := range entries {
			for n := 0; n < e.count; n++ {
				id := e.t.ID
				if e.count > 1 {
					id += "#" + strconv.Itoa(n)
				}
				cc.Instances[name] = append(cc.Instances[name], id)
			}
		}
		if len(cc.Instances[name]) == 0 {
			return fmt.Errorf("catalog: deck %q has no cards", name)
		}
	}
	zones := map[string]bool{}
	for _, z := range def.Zones {
		if z.ID == "" || zones[z.ID] {
			return fmt.Errorf("catalog: cards have a missing or repeated zone id %q", z.ID)
		}
		zones[z.ID] = true
		switch z.Scope {
		case "seat", "table":
		default:
			return fmt.Errorf("catalog: zone %q has scope %q (seat or table)", z.ID, z.Scope)
		}
		switch z.Visibility {
		case "", "owner", "all", "flag", "none":
		default:
			return fmt.Errorf("catalog: zone %q has unknown visibility %q", z.ID, z.Visibility)
		}
		switch z.Layout {
		case "", "fan", "row", "stack", "splay", "grid":
		default:
			return fmt.Errorf("catalog: zone %q has unknown layout %q", z.ID, z.Layout)
		}
		cc.Zones[z.ID] = z
	}
	cg.Cards = cc
	return nil
}

// knownRemoval reports whether id could name a card of the deck (so that
// removing it from a variant that already stripped it is not an error).
func knownRemoval(deck dsl.DeckDef, id string) bool {
	for _, c := range deck.Extra {
		if c.ID == id {
			return true
		}
	}
	for _, r := range deck.Ranks {
		if strings.HasSuffix(id, "-"+r.ID) {
			return true
		}
	}
	return false
}
