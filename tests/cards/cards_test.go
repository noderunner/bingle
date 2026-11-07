// Package cards_test checks the card framework: decks in the DSL (suits, ranks
// with counts, explicit cards, removals, errors) and the runtime that deals,
// shuffles, moves and flips cards and says what each viewer may see.
package cards_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// register compiles a minimal ruleset game with the given "cards" section.
func register(t *testing.T, cardsJSON string) (*catalog.CompiledGame, error) {
	t.Helper()
	def := fmt.Sprintf(`{"id":"cardtest","name":"Card test","engine":"ruleset",
	 "board":{"generator":{"type":"grid","width":1,"height":1,"adjacency":"orthogonal4"},"style":"none"},
	 "seats":[{"seat":1,"color":"white"},{"seat":2,"color":"black"}],
	 "pieces":{"token":{"label":"Token","shape":"disc"}},
	 "turn_order":{"type":"round-robin","seats":[1,2]},
	 "win_condition":{"type":"ruleset"},
	 "cards":%s}`, cardsJSON)
	cat := catalog.NewCatalog()
	if err := cat.RegisterWith("cardtest", []byte(def), nil, struct{}{}); err != nil {
		return nil, err
	}
	g, _ := cat.Get("cardtest")
	return g, nil
}

const goodCards = `{
 "suits":{"hearts":{"label":"Hearts","color":"#c00","symbol":"♥"},"spades":{"label":"Spades","color":"#000","symbol":"♠"}},
 "suit_order":["spades","hearts"],
 "decks":{"main":{
   "ranks":[{"id":"A","value":14},{"id":"K","value":13,"count":2,"text":"Royal"},{"id":"2","value":2}],
   "extra":[{"id":"joker","label":"Joker","value":0,"count":2,"text":"Wild","symbols":["★"]}]
 }},
 "back":{"color":"#336","text":"BACK"},
 "zones":[
  {"id":"deck","scope":"table","visibility":"none","pile":true,"counts":true},
  {"id":"hand","scope":"seat","visibility":"owner"},
  {"id":"table","scope":"table","visibility":"flag"},
  {"id":"open","scope":"table","visibility":"all"}
 ]}`

func TestDecksExpandSuitsTimesRanksPlusExtraCards(t *testing.T) {
	g, err := register(t, goodCards)
	if err != nil {
		t.Fatal(err)
	}
	cc := g.Cards
	// 2 suits x (A + 2 K + 2) + 2 jokers = 10 cards.
	deck := cc.Deck("main")
	if len(deck) != 10 {
		t.Fatalf("%d cards: %v", len(deck), deck)
	}
	// Suits come in suit_order: spades before hearts; ranks in the order listed.
	if deck[0] != "spades-A" || deck[1] != "spades-K#0" || deck[2] != "spades-K#1" || deck[len(deck)-1] != "joker#1" {
		t.Errorf("deck order %v", deck)
	}
	k := cc.Type("hearts-K#1")
	if k == nil || k.Suit != "hearts" || k.Value != 13 || k.Text != "Royal" || k.Label != "K" {
		t.Errorf("hearts K = %+v", k)
	}
	j := cc.Type("joker#0")
	if j == nil || j.Label != "Joker" || len(j.Symbols) != 1 || j.Suit != "" {
		t.Errorf("joker = %+v", j)
	}
	if catalog.TypeID("spades-K#1") != "spades-K" || catalog.TypeID("joker") != "joker" {
		t.Error("TypeID")
	}
}

func TestDeckRemovalsAndErrors(t *testing.T) {
	g, err := register(t, strings.Replace(goodCards, `"ranks":`, `"remove":["hearts-2","joker"],"ranks":`, 1))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(g.Cards.Deck("main")); n != 10-1-2 {
		t.Errorf("after removals: %d cards", n)
	}
	for name, bad := range map[string]string{
		"unknown suit":          strings.Replace(goodCards, `"suits":{"hearts"`, `"suits":{"hearts2"`, 1),
		"repeated rank":         strings.Replace(goodCards, `{"id":"2","value":2}`, `{"id":"A","value":2}`, 1),
		"bad zone scope":        strings.Replace(goodCards, `"scope":"seat"`, `"scope":"nowhere"`, 1),
		"bad visibility":        strings.Replace(goodCards, `"visibility":"owner"`, `"visibility":"sometimes"`, 1),
		"a layout (the look's)": strings.Replace(goodCards, `"pile":true`, `"layout":"stack"`, 1),
		"unknown removal":       strings.Replace(goodCards, `"ranks":`, `"remove":["clubs-Q"],"ranks":`, 1),
		"no decks":              `{"decks":{}}`,
	} {
		if _, err := register(t, bad); err == nil {
			t.Errorf("%s: the definition was accepted", name)
		}
	}
}

func TestShuffleIsSeededAndKeepsEveryCard(t *testing.T) {
	g, _ := register(t, goodCards)
	build := func(seed uint64) *cards.Zone {
		z := &cards.Zone{}
		z.Fill(g.Cards.Deck("main"), false)
		z.Shuffle(&cards.RNG{State: seed})
		return z
	}
	a, b, c := build(5), build(5), build(6)
	if strings.Join(a.IDs(), ",") != strings.Join(b.IDs(), ",") {
		t.Error("the same seed gave two orders")
	}
	if strings.Join(a.IDs(), ",") == strings.Join(c.IDs(), ",") {
		t.Error("two seeds gave one order")
	}
	seen := map[string]bool{}
	for _, id := range a.IDs() {
		seen[id] = true
	}
	if len(seen) != 10 {
		t.Errorf("a shuffle lost cards: %d distinct", len(seen))
	}
}

func TestDealMoveFlipAndDraw(t *testing.T) {
	g, _ := register(t, goodCards)
	tb := cards.NewTable()
	deck := tb.Zone("deck", 0)
	deck.Fill(g.Cards.Deck("main"), false)
	h1, h2 := tb.Zone("hand", 1), tb.Zone("hand", 2)
	if n := cards.Deal(deck, []*cards.Zone{h1, h2}, 3); n != 6 {
		t.Fatalf("dealt %d", n)
	}
	if h1.Len() != 3 || h2.Len() != 3 || deck.Len() != 4 {
		t.Fatalf("hands %d %d, deck %d", h1.Len(), h2.Len(), deck.Len())
	}
	id := h1.Cards[0].ID
	if !tb.Move(id, "hand", 1, "table", 0, true) || h1.Len() != 2 || tb.Count("table", 0) != 1 {
		t.Fatal("move failed")
	}
	if c, _ := tb.Zone("table", 0).Top(); c.ID != id || !c.Up {
		t.Errorf("top = %+v", c)
	}
	tb.Zone("table", 0).Flip(id, false)
	if c, _ := tb.Zone("table", 0).Top(); c.Up {
		t.Error("flip did not turn the card face down")
	}
	if got := deck.Draw(2); len(got) != 2 || deck.Len() != 2 {
		t.Errorf("draw: %d cards, deck %d", len(got), deck.Len())
	}
	if tb.Move("nonexistent", "hand", 1, "table", 0, true) {
		t.Error("moved a card that is not there")
	}
	tb.MoveAll("hand", 2, "open", 0, true)
	if h2.Len() != 0 || tb.Count("open", 0) != 3 {
		t.Error("MoveAll")
	}
	clone := tb.Clone()
	clone.Zone("deck", 0).Clear()
	if deck.Len() != 2 {
		t.Error("a clone shares cards with the original")
	}
}

func TestEachViewerSeesOnlyWhatTheZonesShowThem(t *testing.T) {
	g, _ := register(t, goodCards)
	tb := cards.NewTable()
	tb.Zone("deck", 0).Fill([]string{"spades-A", "spades-2"}, false)
	tb.Zone("hand", 1).Fill([]string{"hearts-A", "hearts-2"}, false)
	tb.Zone("hand", 2).Fill([]string{"spades-K#0"}, false)
	tb.Zone("table", 0).Cards = []cards.Card{{ID: "hearts-K#0", Up: true}, {ID: "hearts-K#1", Up: false}}
	tb.Zone("open", 0).Fill([]string{"joker#0"}, false)
	visible := func(red *cards.Table, key string) []string {
		var out []string
		for _, c := range red.Zones[key].Cards {
			out = append(out, c.ID)
		}
		return out
	}
	// Seat 1: its own hand, the face-up table card and the always-open card.
	r1 := tb.Redact(g.Cards, 1)
	if got := visible(r1, "hand:1"); got[0] != "hearts-A" || got[1] != "hearts-2" {
		t.Errorf("seat 1 own hand: %v", got)
	}
	if got := visible(r1, "hand:2"); got[0] != "" {
		t.Errorf("seat 1 sees seat 2's hand: %v", got)
	}
	if got := visible(r1, "table"); got[0] != "hearts-K#0" || got[1] != "" {
		t.Errorf("flag zone: %v", got)
	}
	if got := visible(r1, "open"); got[0] != "joker#0" {
		t.Errorf("an 'all' zone hides a card: %v", got)
	}
	if got := visible(r1, "deck"); got[0] != "" || got[1] != "" {
		t.Errorf("seat 1 sees the deck: %v", got)
	}
	// The public sees no hand at all.
	r0 := tb.Redact(g.Cards, 0)
	if got := visible(r0, "hand:1"); got[0] != "" {
		t.Errorf("the public sees a hand: %v", got)
	}
	// A card in an owner zone shown face up is seen by everyone.
	tb.Zone("hand", 2).Flip("spades-K#0", true)
	if got := visible(tb.Redact(g.Cards, 1), "hand:2"); got[0] != "spades-K#0" {
		t.Errorf("a revealed card in a hand stays hidden: %v", got)
	}
}

func TestViewSendsNoCardsForAHiddenPile(t *testing.T) {
	g, _ := register(t, goodCards)
	tb := cards.NewTable()
	tb.Zone("deck", 0).Fill(g.Cards.Deck("main"), false)
	tb.Zone("hand", 1).Fill([]string{"hearts-A"}, false)
	views := tb.View(g.Cards, 2)
	for _, z := range views {
		switch z.ID {
		case "deck":
			if z.Count != 10 || z.Cards != nil {
				t.Errorf("the deck view = %+v: a hidden pile should send only its count", z)
			}
		case "hand":
			if z.Count != 1 || len(z.Cards) != 1 || z.Cards[0].ID != "" {
				t.Errorf("seat 2 sees seat 1's hand as %+v", z)
			}
		}
	}
}

// A player's own hand comes out in order: by suit, in the game's own order of
// suits, then by the value of the rank, with the cards of no suit (the jokers)
// last. Two cards of one type keep the order they were dealt in. The table
// itself is untouched, and a shared zone keeps the order of play.
func TestOwnHandComesOutBySuitAndValue(t *testing.T) {
	g, _ := register(t, goodCards)
	tb := cards.NewTable()
	dealt := []string{"hearts-2", "joker#0", "spades-K#1", "hearts-A", "spades-2", "spades-K#0"}
	tb.Zone("hand", 1).Fill(dealt, false)
	tb.Zone("table", 0).Cards = []cards.Card{{ID: "hearts-A", Up: true}, {ID: "spades-2", Up: true}}
	ids := func(zones []cards.ZoneView, id string) []string {
		for _, z := range zones {
			if z.ID == id {
				var out []string
				for _, c := range z.Cards {
					out = append(out, c.ID)
				}
				return out
			}
		}
		t.Fatalf("no zone %q in the view", id)
		return nil
	}
	own := ids(tb.View(g.Cards, 1), "hand")
	want := []string{"spades-2", "spades-K#1", "spades-K#0", "hearts-2", "hearts-A", "joker#0"}
	if strings.Join(own, ",") != strings.Join(want, ",") {
		t.Errorf("the hand reads %v, want %v", own, want)
	}
	// The cards themselves did not move: only the view is sorted.
	if got := strings.Join(tb.Zone("hand", 1).IDs(), ","); got != strings.Join(dealt, ",") {
		t.Errorf("the hand on the table is now %s, want the cards as they were dealt", got)
	}
	// A zone everybody sees keeps the order it was played in.
	if got := ids(tb.View(g.Cards, 1), "table"); got[0] != "hearts-A" || got[1] != "spades-2" {
		t.Errorf("a shared zone was sorted: %v", got)
	}
	// Somebody else's hand is hidden, so there is nothing to sort.
	for _, c := range ids(tb.View(g.Cards, 2), "hand") {
		if c != "" {
			t.Errorf("seat 2 sees %q of seat 1's hand", c)
		}
	}
}

// A deck can list its ranks in one line, integers and letters: an integer is
// worth itself and a letter one more than the rank before it.
func TestRankListGivesRanksTheirValues(t *testing.T) {
	cases := map[string]map[string]int{
		"A 2 3 10 J Q K": {"A": 1, "2": 2, "3": 3, "10": 10, "J": 11, "Q": 12, "K": 13},
		"2 3 4 J Q K A":  {"2": 2, "4": 4, "J": 5, "K": 7, "A": 8},
		"9 10 J Q K A":   {"9": 9, "A": 14},
		"I II III":       {"I": 1, "II": 2, "III": 3},
		"0 1 2":          {"0": 0, "2": 2},
	}
	for list, want := range cases {
		g, err := register(t, fmt.Sprintf(`{"suits":{"a":{"label":"A"},"b":{"label":"B"}},"decks":{"main":{"rank_list":%q}}}`, list))
		if err != nil {
			t.Errorf("%q: %v", list, err)
			continue
		}
		for rank, value := range want {
			if ty := g.Cards.Types["b-"+rank]; ty == nil || ty.Value != value || ty.Label != rank {
				t.Errorf("%q: card b-%s is %+v, want value %d", list, rank, ty, value)
			}
		}
	}
}

func TestRankListIsRefusedWhenItCannotBeRead(t *testing.T) {
	for name, deck := range map[string]string{
		"both":      `{"rank_list":"1 2","ranks":[{"id":"3","value":3}]}`,
		"repeated":  `{"rank_list":"1 2 1"}`,
		"empty":     `{"rank_list":"   "}`,
		"not ranks": `{"rank_list":"1 a-b"}`,
	} {
		if _, err := register(t, fmt.Sprintf(`{"suits":{"a":{"label":"A"}},"decks":{"main":%s}}`, deck)); err == nil {
			t.Errorf("%s: a deck with %s compiled", name, deck)
		}
	}
}

// What may wear a themed set: up to eight suits of plain cards, with jokers.
func TestStandardDecksAreSuitsRanksAndJokers(t *testing.T) {
	suits := func(n int) string {
		var parts []string
		for i := 0; i < n; i++ {
			parts = append(parts, fmt.Sprintf(`"s%d":{"label":"S%d"}`, i, i))
		}
		return strings.Join(parts, ",")
	}
	joker := `"extra":[{"id":"joker","label":"Joker","tags":["joker"],"text":"Wild","count":2}]`
	for name, c := range map[string]struct {
		json string
		want bool
	}{
		"four suits":   {fmt.Sprintf(`{"suits":{%s},"decks":{"m":{"rank_list":"A 2 K"}}}`, suits(4)), true},
		"eight suits":  {fmt.Sprintf(`{"suits":{%s},"decks":{"m":{"rank_list":"A 2 K"}}}`, suits(8)), true},
		"jokers":       {fmt.Sprintf(`{"suits":{%s},"decks":{"m":{"rank_list":"A 2 K",%s}}}`, suits(4), joker), true},
		"nine suits":   {fmt.Sprintf(`{"suits":{%s},"decks":{"m":{"rank_list":"A 2 K"}}}`, suits(9)), false},
		"no suits":     {`{"decks":{"m":{"extra":[{"id":"x","label":"X"}]}}}`, false},
		"rule text":    {fmt.Sprintf(`{"suits":{%s},"decks":{"m":{"ranks":[{"id":"1","value":1,"text":"Draw two"}]}}}`, suits(2)), false},
		"special card": {fmt.Sprintf(`{"suits":{%s},"decks":{"m":{"rank_list":"1 2","extra":[{"id":"x","label":"Special"}]}}}`, suits(2)), false},
	} {
		g, err := register(t, c.json)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := g.Cards.Standard(); got != c.want {
			t.Errorf("%s: Standard() = %v, want %v", name, got, c.want)
		}
	}
}
