// Package cards is the runtime of card games: the cards of a game in progress
// live in zones (a hand, a draw pile, a trick on the table), and a Table
// holds them all. It deals, shuffles, moves and flips cards, and says what
// each viewer may see of them, so a ruleset game keeps its cards in a Table
// inside its position and gets private hands, face-down cards and public
// play areas right without code of its own.
//
// A card is only its instance id (see catalog.CompiledCards) and whether it is
// face up; everything else about it (suit, value, text) is looked up from the
// game definition. A redacted card — one the viewer may not see — is the
// empty Card, so neither the position nor the view reveals it.
package cards

import (
	"sort"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Card is one card of a game in progress.
type Card struct {
	// ID is the card's instance id; empty for a card the viewer cannot see.
	ID string `json:"id,omitempty"`
	// Up is whether the card lies face up (visible to everyone, unless its
	// zone shows it to all or to none).
	Up bool `json:"up,omitempty"`
}

// Hidden reports whether this is a card the viewer cannot see.
func (c Card) Hidden() bool { return c.ID == "" }

// Zone is an ordered pile of cards. The end of the slice is the top: Push adds
// there and Draw takes from there.
type Zone struct {
	Cards []Card `json:"cards,omitempty"`
}

// Len is the number of cards.
func (z *Zone) Len() int { return len(z.Cards) }

// Find returns the position of the card with instance id, or -1.
func (z *Zone) Find(id string) int {
	for i, c := range z.Cards {
		if c.ID == id {
			return i
		}
	}
	return -1
}

// Has reports whether the zone holds the card.
func (z *Zone) Has(id string) bool { return z.Find(id) >= 0 }

// Push puts a card on top.
func (z *Zone) Push(c Card) { z.Cards = append(z.Cards, c) }

// Take removes the card with the given id and returns it.
func (z *Zone) Take(id string) (Card, bool) {
	i := z.Find(id)
	if i < 0 {
		return Card{}, false
	}
	c := z.Cards[i]
	z.Cards = append(z.Cards[:i], z.Cards[i+1:]...)
	return c, true
}

// Draw removes up to n cards from the top and returns them, top card first.
func (z *Zone) Draw(n int) []Card {
	if n > len(z.Cards) {
		n = len(z.Cards)
	}
	out := make([]Card, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, z.Cards[len(z.Cards)-1])
		z.Cards = z.Cards[:len(z.Cards)-1]
	}
	return out
}

// Top is the top card (false for an empty zone).
func (z *Zone) Top() (Card, bool) {
	if len(z.Cards) == 0 {
		return Card{}, false
	}
	return z.Cards[len(z.Cards)-1], true
}

// Flip turns the card with the given id face up or down.
func (z *Zone) Flip(id string, up bool) bool {
	i := z.Find(id)
	if i < 0 {
		return false
	}
	z.Cards[i].Up = up
	return true
}

// FlipAll turns every card face up or down.
func (z *Zone) FlipAll(up bool) {
	for i := range z.Cards {
		z.Cards[i].Up = up
	}
}

// Clear empties the zone and returns what it held.
func (z *Zone) Clear() []Card {
	out := z.Cards
	z.Cards = nil
	return out
}

// IDs lists the instance ids of the cards, bottom first.
func (z *Zone) IDs() []string {
	out := make([]string, len(z.Cards))
	for i, c := range z.Cards {
		out[i] = c.ID
	}
	return out
}

// RNG is a small splitmix64 generator whose whole state is one number, so a
// game position can carry it (and keep it secret in public views).
type RNG struct{ State uint64 }

// Next advances the generator and returns 64 random bits.
func (r *RNG) Next() uint64 {
	r.State += 0x9e3779b97f4a7c15
	z := r.State
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// Intn returns a number in [0, n).
func (r *RNG) Intn(n int) int {
	if n <= 1 {
		return 0
	}
	return int(r.Next() % uint64(n))
}

// Shuffle puts the cards in a random order (Fisher–Yates).
func (z *Zone) Shuffle(r *RNG) {
	for i := len(z.Cards) - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		z.Cards[i], z.Cards[j] = z.Cards[j], z.Cards[i]
	}
}

// Fill replaces the zone's cards with the given instance ids, all face up or
// all face down.
func (z *Zone) Fill(ids []string, up bool) {
	z.Cards = make([]Card, len(ids))
	for i, id := range ids {
		z.Cards[i] = Card{ID: id, Up: up}
	}
}

// Deal draws each hand's cards from the top of from, one card to each in turn
// until every hand has each cards (or from runs out), and returns how many
// cards were dealt.
func Deal(from *Zone, hands []*Zone, each int) int {
	n := 0
	for round := 0; round < each; round++ {
		for _, h := range hands {
			if from.Len() == 0 {
				return n
			}
			h.Push(from.Draw(1)[0])
			n++
		}
	}
	return n
}

// Table is all the zones of a game. Zones are made when first used; a zone is
// named by its id in the definition and, for a seat's own zone, the seat.
type Table struct {
	Zones map[string]*Zone `json:"zones,omitempty"`
}

// NewTable returns an empty table.
func NewTable() *Table { return &Table{Zones: map[string]*Zone{}} }

// Key is the name of a zone in a Table: the id for a table zone, id:seat for a
// seat's own.
func Key(zone string, owner int) string {
	if owner == 0 {
		return zone
	}
	return zone + ":" + strconv.Itoa(owner)
}

// SplitKey is the inverse of Key.
func SplitKey(key string) (zone string, owner int) {
	if i := strings.LastIndexByte(key, ':'); i >= 0 {
		if n, err := strconv.Atoi(key[i+1:]); err == nil {
			return key[:i], n
		}
	}
	return key, 0
}

// Zone returns the zone, making it empty if it does not exist.
func (t *Table) Zone(zone string, owner int) *Zone {
	if t.Zones == nil {
		t.Zones = map[string]*Zone{}
	}
	k := Key(zone, owner)
	z := t.Zones[k]
	if z == nil {
		z = &Zone{}
		t.Zones[k] = z
	}
	return z
}

// Count is the number of cards in the zone (0 if it does not exist).
func (t *Table) Count(zone string, owner int) int {
	if z := t.Zones[Key(zone, owner)]; z != nil {
		return z.Len()
	}
	return 0
}

// Move takes the card with the given id out of one zone and puts it on top of
// another, face up or down; it reports whether the card was there.
func (t *Table) Move(id, fromZone string, fromOwner int, toZone string, toOwner int, up bool) bool {
	c, ok := t.Zone(fromZone, fromOwner).Take(id)
	if !ok {
		return false
	}
	c.Up = up
	t.Zone(toZone, toOwner).Push(c)
	return true
}

// MoveAll moves every card of one zone onto another, in order.
func (t *Table) MoveAll(fromZone string, fromOwner int, toZone string, toOwner int, up bool) {
	from := t.Zone(fromZone, fromOwner)
	to := t.Zone(toZone, toOwner)
	for _, c := range from.Clear() {
		c.Up = up
		to.Push(c)
	}
}

// Clone copies the table.
func (t *Table) Clone() *Table {
	if t == nil {
		return nil
	}
	out := &Table{Zones: make(map[string]*Zone, len(t.Zones))}
	for k, z := range t.Zones {
		out.Zones[k] = &Zone{Cards: append([]Card(nil), z.Cards...)}
	}
	return out
}

// Visible reports whether viewer sees the face of card c lying in zone z of
// owner (viewer 0 is the public: spectators and the event log).
func Visible(def ZoneVisibility, c Card, owner, viewer int) bool {
	switch def {
	case "all":
		return true
	case "none":
		return false
	case "flag":
		return c.Up
	default: // "owner": the owner sees all, everyone else only what is face up
		return c.Up || (viewer != 0 && viewer == owner)
	}
}

// ZoneVisibility names who sees a zone's faces (see dsl.ZoneDef.Visibility).
type ZoneVisibility = string

func visibilityOf(cc *catalog.CompiledCards, zone string) ZoneVisibility {
	if cc != nil {
		if def, ok := cc.Zones[zone]; ok && def.Visibility != "" {
			return def.Visibility
		}
	}
	return "owner"
}

// Redact returns a copy of the table as viewer may know it: every card whose
// face the viewer cannot see is replaced by the empty Card.
func (t *Table) Redact(cc *catalog.CompiledCards, viewer int) *Table {
	out := &Table{Zones: make(map[string]*Zone, len(t.Zones))}
	for k, z := range t.Zones {
		zone, owner := SplitKey(k)
		vis := visibilityOf(cc, zone)
		nz := &Zone{Cards: make([]Card, len(z.Cards))}
		for i, c := range z.Cards {
			if Visible(vis, c, owner, viewer) {
				nz.Cards[i] = c
			} else {
				nz.Cards[i] = Card{Up: false}
			}
		}
		out.Zones[k] = nz
	}
	return out
}

// ZoneView is one zone as a client draws it.
type ZoneView struct {
	ID    string `json:"id"`
	Owner int    `json:"owner,omitempty"`
	Count int    `json:"count"`
	// Cards are the cards in order (bottom first), a redacted card having no
	// id. A zone of which the viewer can see nothing, drawn as a stack or only
	// counted, sends no cards.
	Cards []Card `json:"cards,omitempty"`
	// Captions label the cards (who played each one), when the game sets them.
	Captions []string `json:"captions,omitempty"`
}

// View describes the table for viewer: one entry per zone that exists, in a
// stable order. A zone that is the viewer's own private one — their hand — is
// sorted (see sortHand), so that every card game hands a player their cards in
// an order they can read.
func (t *Table) View(cc *catalog.CompiledCards, viewer int) []ZoneView {
	keys := make([]string, 0, len(t.Zones))
	for k := range t.Zones {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var out []ZoneView
	for _, k := range keys {
		zone, owner := SplitKey(k)
		z := t.Zones[k]
		vis := visibilityOf(cc, zone)
		zv := ZoneView{ID: zone, Owner: owner, Count: z.Len()}
		seen := 0
		cards := make([]Card, len(z.Cards))
		for i, c := range z.Cards {
			if Visible(vis, c, owner, viewer) {
				cards[i] = c
				seen++
			} else {
				cards[i] = Card{}
			}
		}
		def := cc.Zones[zone]
		if vis == "owner" && owner != 0 && owner == viewer && def.Layout != "stack" && seen == len(cards) {
			sortHand(cc, cards)
		}
		if seen > 0 || (def.Layout != "stack" && !def.Counts) {
			zv.Cards = cards
		}
		out = append(out, zv)
	}
	return out
}

// sortHand puts a hand in the order a player holds their cards in: by suit, in
// the game's own order of suits, then by the value of the rank, then by the
// card's id, so that a hand reads the same way however it was dealt. Cards of
// no suit (jokers and the like) come last. A player may rearrange the cards in
// their client afterwards; this is the order a hand starts in.
func sortHand(cc *catalog.CompiledCards, hand []Card) {
	if cc == nil {
		return
	}
	suits := make(map[string]int, len(cc.Suits))
	for i, id := range cc.Suits {
		suits[id] = i
	}
	key := func(c Card) (int, int, string) {
		t := cc.Type(c.ID)
		if t == nil {
			return len(suits) + 1, 0, c.ID
		}
		s, ok := suits[t.Suit]
		if !ok {
			s = len(suits)
		}
		return s, t.Value, t.ID
	}
	sort.SliceStable(hand, func(i, j int) bool {
		si, vi, idi := key(hand[i])
		sj, vj, idj := key(hand[j])
		if si != sj {
			return si < sj
		}
		if vi != vj {
			return vi < vj
		}
		return idi < idj
	})
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
