// Package solitaire_test checks the games of patience: the deals, what may be built
// on what, what ends a game, what a viewer may see, and Bowling Solitaire's pins,
// balls and score sheet. A position is written out by hand where a rule needs one.
package solitaire_test

import (
	"encoding/json"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/solitaire"
)

var allVariants = []string{"solitaire", "solitaire-draw-3", "spider", "spider-2", "spider-1", "freecell", "bowling-solitaire"}

func game(t testing.TB, id string) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	for _, m := range []catalog.Module{solitaire.Module(), solitaire.BowlingModule()} {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatal(err)
		}
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("%s is not registered", id)
	}
	return g
}

// doc is a position as it is stored.
type doc struct {
	Mode    string  `json:"mode"`
	Turn    int     `json:"turn"`
	Won     bool    `json:"won"`
	Reason  string  `json:"reason"`
	RNG     uint64  `json:"rng"`
	Frame   int     `json:"frame"`
	Ball    int     `json:"ball"`
	Knocked int     `json:"knocked"`
	Rolls   [][]int `json:"rolls"`
	Fresh   bool    `json:"fresh"`
	Table   struct {
		Zones map[string]*zone `json:"zones"`
	} `json:"table"`
}

type zone struct {
	Cards []card `json:"cards"`
}

type card struct {
	ID string `json:"id"`
	Up bool   `json:"up"`
}

func read(t *testing.T, st *state.BoardState) doc {
	t.Helper()
	b, err := st.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var o struct {
		Ext doc `json:"ext"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		t.Fatal(err)
	}
	return o.Ext
}

func count(d doc, z string) int {
	if d.Table.Zones[z] == nil {
		return 0
	}
	return len(d.Table.Zones[z].Cards)
}

// up writes a pile of face-up cards from the bottom up: "spades-K hearts-Q". A leading
// "?" on a card leaves it face down.
func pile(s string) *zone {
	z := &zone{}
	for _, c := range strings.Fields(s) {
		z.Cards = append(z.Cards, card{ID: strings.TrimPrefix(c, "?"), Up: !strings.HasPrefix(c, "?")})
	}
	return z
}

// position is g's opening position with the table replaced by these piles; every other
// zone of the opening stays and is emptied.
func position(t *testing.T, g *catalog.CompiledGame, piles map[string]string, change func(*map[string]any)) *state.BoardState {
	t.Helper()
	rs, _ := state.RulesetOf(g)
	st, err := rs.Start(g, 7)
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var whole map[string]any
	if err := json.Unmarshal(b, &whole); err != nil {
		t.Fatal(err)
	}
	ext := whole["ext"].(map[string]any)
	zones := map[string]any{}
	for k := range ext["table"].(map[string]any)["zones"].(map[string]any) {
		zones[k] = &zone{}
	}
	for k, v := range piles {
		zones[k] = pile(v)
	}
	ext["table"] = map[string]any{"zones": zones}
	if change != nil {
		change(&ext)
	}
	b, err = json.Marshal(whole)
	if err != nil {
		t.Fatal(err)
	}
	out := state.NewBoardState()
	if err := out.UnmarshalJSON(b); err != nil {
		t.Fatal(err)
	}
	return out
}

func legal(g *catalog.CompiledGame, st *state.BoardState) []string {
	rs, _ := state.RulesetOf(g)
	var out []string
	for _, m := range rs.Legal(g, st, 1) {
		out = append(out, m.Piece)
	}
	return out
}

func has(list []string, want string) bool {
	for _, m := range list {
		if m == want {
			return true
		}
	}
	return false
}

func wantMoves(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, yes, no []string) {
	t.Helper()
	got := legal(g, st)
	for _, m := range yes {
		if !has(got, m) {
			t.Errorf("%s should be legal; legal: %v", m, got)
		}
	}
	for _, m := range no {
		if has(got, m) {
			t.Errorf("%s should not be legal; legal: %v", m, got)
		}
	}
}

func play(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, moves ...string) *state.BoardState {
	t.Helper()
	rs, _ := state.RulesetOf(g)
	for _, text := range moves {
		mv, err := rs.Parse(g, text)
		if err != nil {
			t.Fatal(err)
		}
		res, err := rs.Apply(g, st, 1, mv)
		if err != nil {
			t.Fatalf("%s: %v (legal: %v)", text, err, legal(g, st))
		}
		st = res.NewState
	}
	return st
}

// Every one of them can wear a themed set of cards: that is what they were asked for.
func TestEveryVariantTakesAThemedSet(t *testing.T) {
	want := map[string]int{"solitaire": 52, "solitaire-draw-3": 52, "spider": 104, "spider-2": 104, "spider-1": 104, "freecell": 52, "bowling-solitaire": 20}
	suits := map[string]int{"solitaire": 4, "solitaire-draw-3": 4, "spider": 4, "spider-2": 2, "spider-1": 1, "freecell": 4, "bowling-solitaire": 2}
	for _, id := range allVariants {
		g := game(t, id)
		if !g.Cards.Standard() {
			t.Errorf("%s: the deck is not standard", id)
		}
		if n := len(g.Cards.Deck("main")); n != want[id] {
			t.Errorf("%s: %d cards, want %d", id, n, want[id])
		}
		if n := len(g.Cards.Suits); n != suits[id] {
			t.Errorf("%s: %d suits, want %d", id, n, suits[id])
		}
		if len(g.Definition.Seats) != 1 {
			t.Errorf("%s: %d seats", id, len(g.Definition.Seats))
		}
		if len(g.Definition.Rules) == 0 {
			t.Errorf("%s has no rules", id)
		}
	}
}

// Each variant explains its own game, not the base game's.
func TestEachVariantHasItsOwnRules(t *testing.T) {
	seen := map[string]string{}
	words := map[string]string{"solitaire": "one card at a time", "solitaire-draw-3": "three cards at a time", "spider": "two decks", "spider-2": "two suits", "spider-1": "one suit", "freecell": "free cell", "bowling-solitaire": "Sid Sackson"}
	for _, id := range allVariants {
		g := game(t, id)
		text := strings.Join(g.Definition.Rules, " ")
		if other, ok := seen[text]; ok {
			t.Errorf("%s has the same rules as %s", id, other)
		}
		seen[text] = id
		if !strings.Contains(text, words[id]) {
			t.Errorf("%s: the rules do not mention %q", id, words[id])
		}
	}
}

func TestTheDeals(t *testing.T) {
	sizes := func(d doc, n int) []int {
		var out []int
		for i := 1; i <= n; i++ {
			out = append(out, count(d, "t"+strconv.Itoa(i)))
		}
		return out
	}
	same := func(a, b []int) bool {
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
	cases := []struct {
		id      string
		columns []int
		stock   int
		allUp   bool
	}{
		{"solitaire", []int{1, 2, 3, 4, 5, 6, 7}, 24, false},
		{"spider", []int{6, 6, 6, 6, 5, 5, 5, 5, 5, 5}, 50, false},
		{"freecell", []int{7, 7, 7, 7, 6, 6, 6, 6}, 0, true},
	}
	for _, c := range cases {
		g := game(t, c.id)
		rs, _ := state.RulesetOf(g)
		st, err := rs.Start(g, 11)
		if err != nil {
			t.Fatal(err)
		}
		d := read(t, st)
		if got := sizes(d, len(c.columns)); !same(got, c.columns) {
			t.Errorf("%s: columns %v, want %v", c.id, got, c.columns)
		}
		if n := count(d, "stock"); n != c.stock {
			t.Errorf("%s: stock %d, want %d", c.id, n, c.stock)
		}
		for i := range c.columns {
			cards := d.Table.Zones["t"+strconv.Itoa(i+1)].Cards
			for k, card := range cards {
				wantUp := c.allUp || k == len(cards)-1
				if card.Up != wantUp {
					t.Errorf("%s: column %d card %d up=%v", c.id, i+1, k, card.Up)
				}
			}
		}
	}
}

// A deal comes from the seed: the same seed deals the same cards, another does not.
func TestTheDealFollowsTheSeed(t *testing.T) {
	g := game(t, "solitaire")
	rs, _ := state.RulesetOf(g)
	a, _ := rs.Start(g, 5)
	b, _ := rs.Start(g, 5)
	c, _ := rs.Start(g, 6)
	ja, _ := a.MarshalJSON()
	jb, _ := b.MarshalJSON()
	jc, _ := c.MarshalJSON()
	if string(ja) != string(jb) {
		t.Error("the same seed dealt differently")
	}
	if string(ja) == string(jc) {
		t.Error("another seed dealt the same")
	}
}

func TestKlondike_BuildingAndFoundations(t *testing.T) {
	g := game(t, "solitaire")
	st := position(t, g, map[string]string{
		"t1":    "?clubs-2 hearts-7",
		"t2":    "spades-8",
		"t3":    "clubs-8",
		"t4":    "hearts-8",
		"t5":    "",
		"t6":    "?clubs-5 spades-K",
		"waste": "hearts-A",
	}, nil)
	wantMoves(t, g, st,
		[]string{"t1-t2", "t1-t3", "w-f", "t6-t5", "recycle"},
		[]string{"t1-t4", "t1-t5", "t2-t5", "draw", "t6-f"},
	)
	st = play(t, g, st, "w-f")
	d := read(t, st)
	if count(d, "f1") != 1 {
		t.Errorf("the ace of hearts went to the first foundation? f1 has %d", count(d, "f1"))
	}
	// A foundation's top card can come back; moving off a face-down card turns it up.
	st = play(t, g, st, "t1-t2")
	d = read(t, st)
	if !d.Table.Zones["t1"].Cards[0].Up {
		t.Error("the card uncovered did not turn up")
	}
}

func TestKlondike_RunsMoveTogether(t *testing.T) {
	g := game(t, "solitaire")
	st := position(t, g, map[string]string{
		"t1": "?clubs-K spades-9 hearts-8 clubs-7",
		"t2": "hearts-10",
		"t3": "diamonds-8",
	}, nil)
	wantMoves(t, g, st, []string{"t1.3-t2", "t1-t3"}, []string{"t1.4-t2"})
	st = play(t, g, st, "t1.3-t2")
	d := read(t, st)
	if count(d, "t2") != 4 || !d.Table.Zones["t1"].Cards[0].Up {
		t.Errorf("after the run: %+v", d.Table.Zones)
	}
}

func TestKlondike_TheStockGoesRoundInOrder(t *testing.T) {
	g := game(t, "solitaire")
	st := position(t, g, map[string]string{"stock": "?clubs-3 ?clubs-4 ?clubs-5", "t1": "spades-K"}, nil)
	st = play(t, g, st, "draw", "draw", "draw")
	d := read(t, st)
	if got := d.Table.Zones["waste"].Cards; got[2].ID != "clubs-3" || !got[2].Up {
		t.Errorf("the waste: %+v", got)
	}
	st = play(t, g, st, "recycle")
	d = read(t, st)
	if got := d.Table.Zones["stock"].Cards; got[2].ID != "clubs-5" || got[2].Up {
		t.Errorf("the stock again: %+v", got)
	}
}

func TestKlondike_DrawThree(t *testing.T) {
	g := game(t, "solitaire-draw-3")
	st := position(t, g, map[string]string{"stock": "?clubs-3 ?clubs-4 ?clubs-5 ?clubs-6", "t1": "spades-K"}, nil)
	st = play(t, g, st, "draw")
	d := read(t, st)
	if count(d, "waste") != 3 || count(d, "stock") != 1 {
		t.Fatalf("waste %d, stock %d", count(d, "waste"), count(d, "stock"))
	}
	if top := d.Table.Zones["waste"].Cards[2]; top.ID != "clubs-4" {
		t.Errorf("the top of the waste is %s", top.ID)
	}
	st = play(t, g, st, "draw")
	if d = read(t, st); count(d, "waste") != 4 {
		t.Errorf("the last card: waste %d", count(d, "waste"))
	}
}

// Finish appears once everything can go home, and it wins.
func TestKlondike_FinishWins(t *testing.T) {
	g := game(t, "solitaire")
	all := func(suit string) string {
		var b []string
		for _, r := range []string{"K", "Q", "J", "10", "9", "8", "7", "6", "5", "4", "3", "2", "A"} {
			b = append(b, suit+"-"+r)
		}
		return strings.Join(b, " ")
	}
	st := position(t, g, map[string]string{"t1": all("hearts"), "t2": all("spades"), "t3": all("diamonds"), "t4": all("clubs")}, nil)
	wantMoves(t, g, st, []string{"finish"}, nil)
	st = play(t, g, st, "finish")
	rs, _ := state.RulesetOf(g)
	out := rs.Outcome(g, st)
	if !out.Ended || out.Winner == nil || *out.Winner != 1 || out.Reason != "cleared" {
		t.Errorf("outcome %+v", out)
	}
}

// Hidden cards keep Finish away, and a game with no move left ends without a win.
func TestKlondike_StuckEndsWithNoWinner(t *testing.T) {
	g := game(t, "solitaire")
	// Red cards only, nothing to build on, and the two of hearts is buried.
	st := position(t, g, map[string]string{
		"t1": "?clubs-K hearts-A", "t2": "?hearts-2 hearts-5", "t3": "hearts-9", "t4": "diamonds-5",
		"t5": "diamonds-9", "t6": "hearts-3", "t7": "diamonds-3",
	}, nil)
	wantMoves(t, g, st, []string{"t1-f"}, []string{"finish"})
	st = play(t, g, st, "t1-f")
	if got := legal(g, st); len(got) != 0 {
		t.Fatalf("legal: %v", got)
	}
	rs, _ := state.RulesetOf(g)
	out := rs.Outcome(g, st)
	if !out.Ended || out.Winner != nil || out.Reason != "stuck" || out.Detail != "1 of 52 cards home" {
		t.Errorf("outcome %+v", out)
	}
}

func TestSpider_OnlyOneSuitMovesTogether(t *testing.T) {
	g := game(t, "spider")
	st := position(t, g, map[string]string{
		"t1":    "spades-9#0 spades-8#0 hearts-7#0",
		"t2":    "clubs-8#0",
		"t3":    "hearts-10#0",
		"t4":    "diamonds-3#0",
		"t5":    "diamonds-4#0",
		"t6":    "clubs-5#0",
		"t7":    "clubs-6#0",
		"t8":    "clubs-7#0",
		"t9":    "hearts-9#0",
		"t10":   "hearts-J#0",
		"stock": "?clubs-A#0 ?clubs-2#0",
	}, nil)
	wantMoves(t, g, st, []string{"t1-t2", "deal"}, []string{"t1.2-t2", "t1.3-t3"})
	st = play(t, g, st, "deal")
	d := read(t, st)
	if count(d, "stock") != 0 || count(d, "t1") != 4 || count(d, "t2") != 2 {
		t.Errorf("after the deal: stock %d, t1 %d, t2 %d", count(d, "stock"), count(d, "t1"), count(d, "t2"))
	}
}

func TestSpider_NoDealWithAnEmptyColumn(t *testing.T) {
	g := game(t, "spider")
	st := position(t, g, map[string]string{"t1": "hearts-K#0", "stock": "?clubs-A#0"}, nil)
	wantMoves(t, g, st, nil, []string{"deal"})
}

func TestSpider_AFinishedSuitLeavesTheTable(t *testing.T) {
	g := game(t, "spider-1")
	run := func(copy string) string {
		var b []string
		for _, r := range []string{"K", "Q", "J", "10", "9", "8", "7", "6", "5", "4", "3", "2"} {
			b = append(b, "spades-"+r+"#"+copy)
		}
		return strings.Join(b, " ")
	}
	st := position(t, g, map[string]string{"t1": "?spades-5#1 " + run("0"), "t2": "spades-A#0"}, nil)
	st = play(t, g, st, "t2-t1")
	d := read(t, st)
	if count(d, "done") != 13 || count(d, "t1") != 1 || !d.Table.Zones["t1"].Cards[0].Up {
		t.Errorf("done %d, t1 %d: %+v", count(d, "done"), count(d, "t1"), d.Table.Zones["t1"])
	}
}

func TestSpider_EightSuitsWin(t *testing.T) {
	g := game(t, "spider-1")
	var done []string
	for c := 0; c < 8; c++ {
		for _, r := range []string{"K", "Q", "J", "10", "9", "8", "7", "6", "5", "4", "3", "2", "A"} {
			if c == 7 && r == "A" {
				continue
			}
			done = append(done, "spades-"+r+"#"+strconv.Itoa(c))
		}
	}
	// The last suit's run is on the table, but for the ace on the next column.
	st := position(t, g, map[string]string{
		"done": strings.Join(done[:13*7], " "),
		"t1":   strings.Join(done[13*7:], " "),
		"t2":   "spades-A#7",
	}, nil)
	st = play(t, g, st, "t2-t1")
	rs, _ := state.RulesetOf(g)
	if out := rs.Outcome(g, st); !out.Ended || out.Winner == nil {
		t.Errorf("outcome %+v", out)
	}
}

func TestFreeCell_CellsAndCapacity(t *testing.T) {
	g := game(t, "freecell")
	// Three cards in order, four free cells: four can move, so three can.
	st := position(t, g, map[string]string{
		"t1": "spades-9 hearts-8 clubs-7",
		"t2": "hearts-10",
		"t3": "spades-A",
	}, nil)
	wantMoves(t, g, st, []string{"t1.3-t2", "t1-c", "t3-f"}, []string{"t3-t2"})
	// With every cell full and nowhere empty, only one card moves at a time.
	full := position(t, g, map[string]string{
		"t1": "spades-9 hearts-8 clubs-7", "t2": "hearts-10", "t3": "spades-2", "t4": "hearts-3", "t5": "clubs-2",
		"t6": "clubs-3", "t7": "clubs-4", "t8": "clubs-5",
		"c1": "diamonds-2", "c2": "diamonds-3", "c3": "diamonds-4", "c4": "diamonds-5",
	}, nil)
	wantMoves(t, g, full, []string{"t3-t4"}, []string{"t1.3-t2", "t1.2-t2", "t1-c"})
}

func TestFreeCell_ACardInACellPlaysOut(t *testing.T) {
	g := game(t, "freecell")
	st := position(t, g, map[string]string{"t1": "spades-5", "c1": "hearts-4"}, nil)
	wantMoves(t, g, st, []string{"c1-t1", "c1-t2"}, nil)
	st = play(t, g, st, "t1-c")
	d := read(t, st)
	if count(d, "c2") != 1 {
		t.Errorf("the card did not take the next free cell: %+v", d.Table.Zones["c2"])
	}
}

// --- what can be seen ---------------------------------------------------------------

func TestNothingFaceDownLeavesTheServer(t *testing.T) {
	for _, id := range []string{"solitaire", "spider", "bowling-solitaire"} {
		g := game(t, id)
		rs, _ := state.RulesetOf(g)
		st, err := rs.Start(g, 3)
		if err != nil {
			t.Fatal(err)
		}
		pub := state.Public(g, st, 1)
		b, err := pub.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			Ext struct {
				RNG   uint64 `json:"rng"`
				Table any    `json:"table"`
				View  struct {
					Table []struct {
						ID    string `json:"id"`
						Count int    `json:"count"`
						Cards []card `json:"cards"`
					} `json:"table"`
				} `json:"view"`
			} `json:"ext"`
		}
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		if out.Ext.RNG != 0 || out.Ext.Table != nil {
			t.Errorf("%s: the generator or the raw table is public", id)
		}
		d := read(t, st)
		hidden := map[string]bool{}
		for _, z := range d.Table.Zones {
			for _, c := range z.Cards {
				if !c.Up {
					hidden[c.ID] = true
				}
			}
		}
		for _, z := range out.Ext.View.Table {
			for _, c := range z.Cards {
				if c.ID != "" && hidden[c.ID] && z.ID != "waste" {
					t.Errorf("%s: %s lies face down but is in the view of %s", id, c.ID, z.ID)
				}
			}
		}
		if strings.Contains(string(b), `"cards":[{"id":"","up":true}`) {
			t.Errorf("%s: a face-up card lost its name", id)
		}
	}
}

func TestDeterminizeKeepsWhatCanBeSeen(t *testing.T) {
	g := game(t, "solitaire")
	rs, _ := state.RulesetOf(g)
	hi := rs.(state.HiddenInfo)
	st, _ := rs.Start(g, 2)
	before := read(t, st)
	after := read(t, hi.Determinize(g, st, 1, rand.New(rand.NewSource(1))))
	changed := false
	for k, z := range before.Table.Zones {
		for i, c := range z.Cards {
			o := after.Table.Zones[k].Cards[i]
			if c.Up && o.ID != c.ID {
				t.Errorf("a card in view changed: %s", c.ID)
			}
			if c.Up != o.Up {
				t.Errorf("a card turned over")
			}
			if !c.Up && o.ID != c.ID {
				changed = true
			}
		}
	}
	if !changed {
		t.Error("nothing hidden moved")
	}
}

func TestParseRefusesNonsense(t *testing.T) {
	g := game(t, "solitaire")
	rs, _ := state.RulesetOf(g)
	for _, bad := range []string{"", "t1", "x1-t2", "t1-x9", "t.-t2", "e+h@2"} {
		if _, err := rs.Parse(g, bad); err == nil {
			t.Errorf("%q was read", bad)
		}
	}
	b := game(t, "bowling-solitaire")
	rb, _ := state.RulesetOf(b)
	for _, bad := range []string{"", "t1-t2", "e@9", "z@1", "a+b+c+d@1", "@1"} {
		if _, err := rb.Parse(b, bad); err == nil {
			t.Errorf("%q was read", bad)
		}
	}
}

// Random play in every game, against the bookkeeping: no card is ever lost or doubled,
// whatever is legal applies, and a game with no move left has ended.
func TestRandomPlayKeepsEveryCard(t *testing.T) {
	for _, id := range allVariants {
		g := game(t, id)
		rs, _ := state.RulesetOf(g)
		rng := rand.New(rand.NewSource(int64(len(id))))
		for seed := uint64(1); seed <= 6; seed++ {
			st, err := rs.Start(g, seed)
			if err != nil {
				t.Fatal(err)
			}
			total := len(g.Cards.Deck("main"))
			for ply := 0; ply < 400; ply++ {
				moves := rs.Legal(g, st, 1)
				if len(moves) == 0 {
					if !rs.Outcome(g, st).Ended {
						t.Fatalf("%s seed %d: no move, and the game goes on", id, seed)
					}
					break
				}
				res, err := rs.Apply(g, st, 1, moves[rng.Intn(len(moves))])
				if err != nil {
					t.Fatal(err)
				}
				st = res.NewState
				if res.GameEnded != rs.Outcome(g, st).Ended {
					t.Fatalf("%s: the move and the outcome disagree", id)
				}
				d := read(t, st)
				seen, n := map[string]bool{}, 0
				for _, z := range d.Table.Zones {
					for _, c := range z.Cards {
						if seen[c.ID] {
							t.Fatalf("%s: %s is in two places", id, c.ID)
						}
						seen[c.ID] = true
						n++
					}
				}
				if n != total {
					t.Fatalf("%s seed %d ply %d: %d cards of %d", id, seed, ply, n, total)
				}
				if err := rs.Validate(g, st); err != nil {
					t.Fatalf("%s: %v", id, err)
				}
			}
		}
	}
}

// The picture of a game in progress is really in progress, the same every time, and
// shows nothing face down.
func TestTheSampleIsAGameInProgress(t *testing.T) {
	for _, id := range allVariants {
		g := game(t, id)
		sample := state.SampleOf(g)
		b, err := json.Marshal(sample)
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Solitaire string `json:"solitaire"`
			Home      int    `json:"home"`
			Table     []struct {
				ID    string `json:"id"`
				Count int    `json:"count"`
				Cards []card `json:"cards"`
			} `json:"table"`
			Bowling *struct {
				Frame  int `json:"frame"`
				Frames []struct {
					Marks []string `json:"marks"`
				} `json:"frames"`
			} `json:"bowling"`
		}
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		if v.Solitaire != g.ParamString("mode", "") {
			t.Errorf("%s: a sample of %q", id, v.Solitaire)
		}
		switch v.Solitaire {
		case "klondike", "freecell":
			if v.Home == 0 {
				t.Errorf("%s: no card is home", id)
			}
		case "spider":
			up := 0
			for _, z := range v.Table {
				for _, c := range z.Cards {
					if c.ID != "" {
						up++
					}
				}
			}
			if up <= 10 {
				t.Errorf("%s: only %d cards are showing", id, up)
			}
		case "bowling":
			if v.Bowling == nil || v.Bowling.Frame < 2 || len(v.Bowling.Frames[0].Marks) == 0 {
				t.Errorf("%s: %+v", id, v.Bowling)
			}
		}
		// Worked out once, by a new ruleset too: the same.
		again, _ := json.Marshal(state.SampleOf(game(t, id)))
		if string(again) != string(b) {
			t.Errorf("%s: the sample changed", id)
		}
	}
}

// A play that shows the player a card they had not seen cannot be taken back,
// or Back would be a way to peek: drawing from the stock, or uncovering a card
// in a column. A play that shows nothing new can (ADR 0083, ADR 0114).
func TestKlondike_ACardSeenCannotBeTakenBack(t *testing.T) {
	g := game(t, "solitaire")
	st := position(t, g, map[string]string{
		"t1":    "?clubs-2 hearts-7",
		"t2":    "spades-8",
		"t3":    "clubs-8",
		"stock": "?diamonds-4",
		"waste": "hearts-A",
	}, nil)
	for _, c := range []struct {
		move     string
		takeBack bool
	}{
		{"draw", false},  // the four of diamonds comes up
		{"t1-t2", false}, // the two of clubs turns up
		{"w-f", true},    // the ace was already in view
	} {
		after := play(t, g, st, c.move)
		if got := state.CanTakeBack(g, st, after); got != c.takeBack {
			t.Errorf("%s: can be taken back = %v, want %v", c.move, got, c.takeBack)
		}
	}
	// Turning the waste back over hides cards; it shows none.
	empty := position(t, g, map[string]string{"t1": "spades-K", "waste": "hearts-5 clubs-9"}, nil)
	if after := play(t, g, empty, "recycle"); !state.CanTakeBack(g, empty, after) {
		t.Error("recycle: can no longer be taken back")
	}
}
