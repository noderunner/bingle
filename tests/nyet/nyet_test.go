// Package nyet_test checks Nyet!: the deals, the Nyet! phase that sets the
// terms of a round, the teams, discards, the trick rules with trump and super
// trump, loot, scoring and what each viewer may see.
package nyet_test

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/nyet"
)

func game(t testing.TB, id string) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(nyet.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("%s is not registered", id)
	}
	return g
}

func names(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	rs := nyet.New()
	var out []string
	for _, mv := range rs.Legal(g, st, seat) {
		out = append(out, rs.Format(g, mv))
	}
	return out
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func play(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, text string) *state.BoardState {
	t.Helper()
	rs := nyet.New()
	seat := rs.ToMove(g, st)
	mv, err := rs.Parse(g, text)
	if err != nil {
		t.Fatalf("parse %q: %v", text, err)
	}
	res, err := rs.Apply(g, st, seat, mv)
	if err != nil {
		t.Fatalf("apply %q (seat %d): %v", text, seat, err)
	}
	return res.NewState
}

// plays lists seat, card pairs as a trick.
func plays(args ...any) []nyet.Play {
	var out []nyet.Play
	for i := 0; i+1 < len(args); i += 2 {
		out = append(out, nyet.Play{Seat: args[i].(int), Card: args[i+1].(string)})
	}
	return out
}

func data(st *state.BoardState) *nyet.Data { return st.Ext().(*nyet.Data) }

// trick builds a position in the trick phase: hands by seat (card instance
// ids), the terms, and the plays already in the trick.
func trick(g *catalog.CompiledGame, turn, first int, trump, super string, team []int, hands map[int][]string, plays []nyet.Play) *state.BoardState {
	d := &nyet.Data{Phase: "trick", Turn: turn, Round: 1, Dealer: 1, First: first, Trump: trump, Super: super, Points: 1, Disc: "none", Team: team, Table: cards.NewTable(), Crossed: map[string]int{}}
	for s, ids := range hands {
		d.Table.Zone("hand", s).Fill(ids, false)
	}
	for _, p := range plays {
		d.Table.Zone("trick", 0).Push(cards.Card{ID: p.Card, Up: true})
	}
	d.Plays = plays
	st := state.NewBoardState()
	st.SetExt(d)
	return st
}

func TestTheDealsForEveryPlayerCount(t *testing.T) {
	for _, tc := range []struct {
		id           string
		players, per int
		deck         int
	}{{"nyet", 4, 15, 0}, {"nyet-2", 2, 15, 30}, {"nyet-3", 3, 16, 0}, {"nyet-5", 5, 12, 0}} {
		g := game(t, tc.id)
		st, err := nyet.New().Start(g, 42)
		if err != nil {
			t.Fatal(err)
		}
		d := data(st)
		seen := map[string]bool{}
		for s := 1; s <= tc.players; s++ {
			z := d.Table.Zone("hand", s)
			if z.Len() != tc.per {
				t.Errorf("%s: seat %d has %d cards, want %d", tc.id, s, z.Len(), tc.per)
			}
			for _, c := range z.Cards {
				seen[c.ID] = true
			}
		}
		if got := d.Table.Zone("deck", 0).Len(); got != tc.deck {
			t.Errorf("%s: deck has %d cards, want %d", tc.id, got, tc.deck)
		}
		for _, c := range d.Table.Zone("deck", 0).Cards {
			seen[c.ID] = true
		}
		want := 60
		if tc.id == "nyet-3" {
			want = 48
		}
		if len(seen) != want {
			t.Errorf("%s: %d distinct cards, want %d", tc.id, len(seen), want)
		}
		if d.Phase != "nyet" || d.Turn != 1 {
			t.Errorf("%s: phase %q turn %d", tc.id, d.Phase, d.Turn)
		}
	}
}

func TestTheDeckHasThreeOnesOfEachColour(t *testing.T) {
	g := game(t, "nyet")
	deck := g.Cards.Deck("main")
	if len(deck) != 60 {
		t.Fatalf("%d cards", len(deck))
	}
	ones := map[string]int{}
	for _, id := range deck {
		if ty := g.Cards.Type(id); ty.Value == 1 {
			ones[ty.Suit]++
		}
	}
	for _, c := range []string{"red", "blue", "yellow", "green"} {
		if ones[c] != 3 {
			t.Errorf("%s has %d ones", c, ones[c])
		}
	}
}

func TestCrossingBoxesFixesTheTerms(t *testing.T) {
	g := game(t, "nyet")
	rs := nyet.New()
	st, _ := rs.Start(g, 7)
	// Cross everything but a chosen box on each line, taking turns.
	keep := map[string]string{"first": "tiger", "discard": "none", "trump": "blue", "super": "red", "points": "2"}
	for guard := 0; guard < 60; guard++ {
		seat := rs.ToMove(g, st)
		d := data(st)
		if d.Phase != "nyet" {
			break
		}
		var pick string
		for _, m := range names(g, st, seat) {
			parts := strings.Split(m, ":")
			if keep[parts[1]] != parts[2] {
				pick = m
				break
			}
		}
		if pick == "" {
			t.Fatalf("no box to cross for seat %d: %v", seat, names(g, st, seat))
		}
		st = play(t, g, st, pick)
	}
	d := data(st)
	if d.Phase != "team" || d.First != 3 || d.Trump != "blue" || d.Super != "red" || d.Points != 2 || d.Disc != "none" {
		t.Fatalf("terms: %+v", d)
	}
	if d.Turn != 3 {
		t.Errorf("the first player (Tiger) picks the team, turn = %d", d.Turn)
	}
	// 4 players: the leader picks one partner of three.
	if got := names(g, st, 3); len(got) != 3 || !has(got, "team:wolf") || !has(got, "team:crane") || !has(got, "team:bear") {
		t.Errorf("team choices = %v", got)
	}
	// Choosing a partner starts the tricks (no discards): Tiger leads.
	st = play(t, g, st, "team:wolf")
	d = data(st)
	if d.Phase != "trick" || d.Turn != 3 {
		t.Errorf("after the team: phase %q turn %d", d.Phase, d.Turn)
	}
	if len(d.Team) != 2 || d.Team[0] != 1 || d.Team[1] != 3 {
		t.Errorf("team = %v", d.Team)
	}
}

func TestAnyLineStopsAtOneBox(t *testing.T) {
	g := game(t, "nyet")
	rs := nyet.New()
	st, _ := rs.Start(g, 3)
	// The trump line has four boxes: after three are crossed it offers no more.
	for _, c := range []string{"red", "blue", "yellow"} {
		st = play(t, g, st, "nyet:trump:"+c)
	}
	for _, m := range names(g, st, rs.ToMove(g, st)) {
		if strings.HasPrefix(m, "nyet:trump:") {
			t.Errorf("a decided line still offers %s", m)
		}
	}
}

func TestTeamChoicesForThreeAndFivePlayers(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want int
	}{{"nyet-3", 3}, {"nyet-5", 4 + 6}} {
		g := game(t, tc.id)
		d := &nyet.Data{Phase: "team", Turn: 1, First: 1, Round: 1, Dealer: 1, Table: cards.NewTable(), Crossed: map[string]int{}}
		st := state.NewBoardState()
		st.SetExt(d)
		if got := names(g, st, 1); len(got) != tc.want {
			t.Errorf("%s: %d team choices %v, want %d", tc.id, len(got), got, tc.want)
		}
	}
}

func TestMustFollowTheColourLed(t *testing.T) {
	g := game(t, "nyet")
	st := trick(g, 2, 1, "blue", "none", []int{1, 3},
		map[int][]string{
			1: {"red-5"}, 2: {"green-2", "green-9", "red-7", "blue-4"}, 3: {"red-1#0"}, 4: {"red-2"},
		},
		[]nyet.Play{{Seat: 1, Card: "green-5"}})
	got := names(g, st, 2)
	if len(got) != 2 || !has(got, "green-2") || !has(got, "green-9") {
		t.Errorf("must follow green: %v", got)
	}
	// With no green, any card may be played (a trump included).
	st = trick(g, 2, 1, "blue", "none", []int{1, 3},
		map[int][]string{1: {"red-5"}, 2: {"red-7", "blue-4"}, 3: {"red-1#0"}, 4: {"red-2"}},
		[]nyet.Play{{Seat: 1, Card: "green-5"}})
	if got := names(g, st, 2); len(got) != 2 {
		t.Errorf("no green: any card, got %v", got)
	}
}

func TestSuperTrumpIsTheTrumpColourWhateverItsOwn(t *testing.T) {
	g := game(t, "nyet")
	// Trump blue, super trump red: the red 1s count as blue. Red is led: a player
	// holding the red 1 and a red 5 must play the red 5 (the 1 is not red).
	st := trick(g, 2, 1, "blue", "red", []int{1, 3},
		map[int][]string{1: {"green-1#0"}, 2: {"red-1#0", "red-5", "green-2"}, 3: {"green-3"}, 4: {"green-4"}},
		[]nyet.Play{{Seat: 1, Card: "red-9"}})
	got := names(g, st, 2)
	if len(got) != 1 || got[0] != "red-5" {
		t.Errorf("only the real red may follow red: %v", got)
	}
	// Trump (blue) led: the red 1 super trump follows.
	st = trick(g, 2, 1, "blue", "red", []int{1, 3},
		map[int][]string{1: {"green-1#0"}, 2: {"red-1#0", "red-5", "blue-3", "green-2"}, 3: {"green-3"}, 4: {"green-4"}},
		[]nyet.Play{{Seat: 1, Card: "blue-9"}})
	got = names(g, st, 2)
	if len(got) != 2 || !has(got, "red-1") || !has(got, "blue-3") {
		t.Errorf("trump led: the trump and the super trump follow: %v", got)
	}
}

func finishTrick(t *testing.T, g *catalog.CompiledGame, trump, super string, team []int, plays []nyet.Play, hands map[int][]string) *nyet.Data {
	t.Helper()
	// Each seat plays its single card in order, starting with plays[0].Seat.
	hs := map[int][]string{}
	for _, p := range plays {
		hs[p.Seat] = []string{p.Card}
	}
	for s, h := range hands {
		hs[s] = append(hs[s], h...)
	}
	st := trick(g, plays[0].Seat, plays[0].Seat, trump, super, team, hs, nil)
	for _, p := range plays {
		st = play(t, g, st, catalog.TypeID(p.Card))
	}
	return data(st)
}

func TestWhoTakesTheTrick(t *testing.T) {
	g := game(t, "nyet")
	// Spare yellow cards keep the round going (yellow is neither led nor trump here).
	extra := map[int][]string{1: {"yellow-13"}, 2: {"yellow-12"}, 3: {"yellow-11"}, 4: {"yellow-10"}}
	// Highest of the colour led wins with no trump.
	d := finishTrick(t, g, "blue", "none", []int{1, 3}, plays(1, "green-9", 2, "green-4", 3, "green-12", 4, "red-13"), extra)
	if d.Tricks[3] != 1 {
		t.Errorf("seat 3 should win with green 12: %v", d.Tricks)
	}
	// A trump beats the colour led; the highest trump wins.
	d = finishTrick(t, g, "blue", "none", []int{1, 3}, plays(1, "green-9", 2, "blue-2", 3, "green-12", 4, "blue-6"), extra)
	if d.Tricks[4] != 1 {
		t.Errorf("seat 4 should win with blue 6: %v", d.Tricks)
	}
	// A super trump beats every trump, and the last one played wins.
	d = finishTrick(t, g, "blue", "red", []int{1, 3}, plays(1, "green-9", 2, "red-1#0", 3, "blue-13", 4, "red-1#1"), extra)
	if d.Tricks[4] != 1 {
		t.Errorf("seat 4 played the last super trump: %v", d.Tricks)
	}
	// Equal cards: the last played takes it (two threes of ... same colour cannot exist, but 1s can).
	d = finishTrick(t, g, "blue", "none", []int{1, 3}, plays(1, "green-1#0", 2, "green-1#1", 3, "green-1#2", 4, "red-9"), extra)
	if d.Tricks[3] != 1 {
		t.Errorf("the last equal card wins: %v", d.Tricks)
	}
}

func TestLootIsAnOpponentsOneInATrickYouWin(t *testing.T) {
	g := game(t, "nyet")
	extra := map[int][]string{1: {"yellow-13"}, 2: {"yellow-12"}, 3: {"yellow-11"}, 4: {"yellow-10"}}
	// Team {1,3}. Seat 3 wins; the green 1 of seat 2 (opponent) is loot, the green 1
	// of seat 1 (a teammate) is not.
	d := finishTrick(t, g, "blue", "none", []int{1, 3}, plays(1, "green-1#0", 2, "green-1#1", 3, "green-9", 4, "green-2"), extra)
	if d.Tricks[3] != 1 || d.Loot[3] != 1 {
		t.Errorf("tricks %v loot %v: seat 3 should have 1 trick and 1 loot", d.Tricks, d.Loot)
	}
	// Seat 2 (opponent) wins with a 1 of their own? not loot for themselves.
	d = finishTrick(t, g, "blue", "none", []int{1, 3}, plays(1, "green-5", 2, "green-9", 3, "green-1#0", 4, "green-1#1"), extra)
	if d.Tricks[2] != 1 || d.Loot[2] != 1 {
		t.Errorf("seat 2 wins and captures the teammate's... %v %v", d.Tricks, d.Loot)
	}
}

func TestScoringAndTheBonusCard(t *testing.T) {
	g := game(t, "nyet-3")
	// Three players, one trick left each: seat 1 and 2 are a team, seat 3 alone
	// with the bonus card. Points per trick: 3. Team {1,2} wins the last trick
	// (their tricks so far: 4; seat 3 had 2).
	d := &nyet.Data{Phase: "trick", Turn: 1, Round: 1, Dealer: 1, First: 1, Trump: "blue", Super: "none", Points: 3, Disc: "none",
		Team: []int{1, 2}, Bonus: 3, Table: cards.NewTable(), Crossed: map[string]int{}}
	d.Table.Zone("hand", 1).Fill([]string{"green-9"}, false)
	d.Table.Zone("hand", 2).Fill([]string{"green-3"}, false)
	d.Table.Zone("hand", 3).Fill([]string{"green-4"}, false)
	d.Tricks = [6]int{0, 2, 2, 2}
	// The rest of the deck lies in the deck zone, ready for the next deal.
	inPlay := map[string]bool{"green-9": true, "green-3": true, "green-4": true}
	var rest []string
	for _, id := range g.Cards.Deck("main") {
		if !inPlay[id] {
			rest = append(rest, id)
		}
	}
	d.Table.Zone("deck", 0).Fill(rest, false)
	st := state.NewBoardState()
	st.SetExt(d)
	st = play(t, g, st, "green-9")
	st = play(t, g, st, "green-3")
	st = play(t, g, st, "green-4")
	nd := data(st)
	// Team {1,2}: tricks 2+2+1 = 5, x3 = 15 each. Seat 3: 2 tricks x3 = 6, doubled = 12.
	if nd.Scores[1] != 15 || nd.Scores[2] != 15 || nd.Scores[3] != 12 {
		t.Errorf("scores = %v", nd.Scores)
	}
	if nd.Round != 2 || nd.Phase != "nyet" || nd.Dealer != 2 {
		t.Errorf("a new round should start: round %d phase %q dealer %d", nd.Round, nd.Phase, nd.Dealer)
	}
	if nd.Table.Zone("hand", 1).Len() != 16 {
		t.Errorf("the next deal gives 16 cards, got %d", nd.Table.Zone("hand", 1).Len())
	}
}

func TestPassingLeftExchangesTheCardsTogether(t *testing.T) {
	g := game(t, "nyet-3")
	d := &nyet.Data{Phase: "discard", Turn: 1, Round: 1, Dealer: 1, First: 1, Disc: "passleft", Trump: "blue", Super: "none", Points: 1,
		Team: []int{1}, Bonus: 1, Table: cards.NewTable(), Crossed: map[string]int{}, Pending: map[int]string{}}
	d.Table.Zone("hand", 1).Fill([]string{"red-2", "red-3"}, false)
	d.Table.Zone("hand", 2).Fill([]string{"blue-2", "blue-3"}, false)
	d.Table.Zone("hand", 3).Fill([]string{"green-2", "green-3"}, false)
	st := state.NewBoardState()
	st.SetExt(d)
	st = play(t, g, st, "pass:red-2")
	st = play(t, g, st, "pass:blue-2")
	// Nothing has moved yet: seat 3 may still choose, and seat 2's choice is not in seat 3's hand.
	if data(st).Table.Zone("hand", 3).Len() != 2 || data(st).Phase != "discard" {
		t.Fatal("cards moved before everyone chose")
	}
	st = play(t, g, st, "pass:green-3")
	nd := data(st)
	if nd.Phase != "trick" {
		t.Fatalf("phase %q", nd.Phase)
	}
	// Seat 1 gave red-2 to seat 2, seat 2 gave blue-2 to seat 3, seat 3 gave green-3 to seat 1.
	if !nd.Table.Zone("hand", 2).Has("red-2") || !nd.Table.Zone("hand", 3).Has("blue-2") || !nd.Table.Zone("hand", 1).Has("green-3") {
		t.Errorf("hands after passing: %v %v %v", nd.Table.Zone("hand", 1).IDs(), nd.Table.Zone("hand", 2).IDs(), nd.Table.Zone("hand", 3).IDs())
	}
}

func TestDiscardExcept1NeverOffersAOne(t *testing.T) {
	g := game(t, "nyet")
	d := &nyet.Data{Phase: "discard", Turn: 1, Round: 1, Dealer: 1, First: 1, Disc: "except1", Trump: "blue", Super: "none", Points: 1,
		Team: []int{1, 2}, Table: cards.NewTable(), Crossed: map[string]int{}, Pending: map[int]string{}}
	d.Table.Zone("hand", 1).Fill([]string{"red-1#0", "red-4", "green-1#0"}, false)
	st := state.NewBoardState()
	st.SetExt(d)
	got := names(g, st, 1)
	if len(got) != 1 || got[0] != "discard:red-4" {
		t.Errorf("discards = %v", got)
	}
}

func TestEachViewerSeesOnlyTheirOwnHand(t *testing.T) {
	g := game(t, "nyet")
	rs := nyet.New()
	st, _ := rs.Start(g, 11)
	for viewer := 0; viewer <= 4; viewer++ {
		pub := rs.Redact(g, st, viewer)
		raw, _ := json.Marshal(pub.Ext())
		var out struct {
			RNG  uint64 `json:"rng"`
			View struct {
				Table []struct {
					ID    string       `json:"id"`
					Owner int          `json:"owner"`
					Count int          `json:"count"`
					Cards []cards.Card `json:"cards"`
				} `json:"table"`
			} `json:"view"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		if out.RNG != 0 {
			t.Errorf("viewer %d sees the shuffling state", viewer)
		}
		for _, z := range out.View.Table {
			for _, c := range z.Cards {
				if c.ID != "" && (z.ID != "hand" || z.Owner != viewer) {
					t.Errorf("viewer %d sees a card %q in %s:%d", viewer, c.ID, z.ID, z.Owner)
				}
			}
			if z.ID == "hand" && z.Owner == viewer && viewer != 0 {
				if len(z.Cards) != 15 {
					t.Errorf("viewer %d sees %d of their own cards", viewer, len(z.Cards))
				}
				for _, c := range z.Cards {
					if c.ID == "" {
						t.Errorf("viewer %d cannot see one of their cards", viewer)
					}
				}
			}
			if z.ID == "hand" && z.Count != 15 {
				t.Errorf("hand count %d", z.Count)
			}
		}
		if strings.Contains(string(raw), `"table":{"zones"`) {
			t.Errorf("viewer %d is sent the whole table", viewer)
		}
	}
}

func TestRandomGamesAreValidAndTerminate(t *testing.T) {
	for _, id := range []string{"nyet", "nyet-2", "nyet-3", "nyet-5"} {
		g := game(t, id)
		rs := nyet.New()
		rng := rand.New(rand.NewSource(9))
		games := 3
		for n := 0; n < games; n++ {
			st, _ := rs.Start(g, uint64(n+1))
			for ply := 0; ply < 5000; ply++ {
				seat := rs.ToMove(g, st)
				if seat == 0 {
					break
				}
				moves := rs.Legal(g, st, seat)
				if len(moves) == 0 {
					t.Fatalf("%s: seat %d has no decision (ply %d, phase %s)", id, seat, ply, data(st).Phase)
				}
				mv := moves[rng.Intn(len(moves))]
				text := rs.Format(g, mv)
				back, err := rs.Parse(g, text)
				if err != nil || rs.Format(g, back) != text {
					t.Fatalf("%s: %q does not round-trip: %v", id, text, err)
				}
				res, err := rs.Apply(g, st, seat, mv)
				if err != nil {
					t.Fatalf("%s: legal move %q refused: %v", id, text, err)
				}
				if err := rs.Validate(g, res.NewState); err != nil {
					t.Fatalf("%s: invalid position after %q: %v", id, text, err)
				}
				st = res.NewState
			}
			if rs.ToMove(g, st) != 0 {
				t.Errorf("%s: game %d did not finish", id, n)
			}
			if o := rs.Outcome(g, st); !o.Ended {
				t.Errorf("%s: no outcome", id)
			}
		}
	}
}

// viewOf is what a viewer's page is sent beyond the board: the table, the board
// of terms and the scoreboard.
type viewOf struct {
	Grid struct {
		Lines []struct {
			ID    string `json:"id"`
			Icon  string `json:"icon"`
			Boxes []struct {
				ID     string `json:"id"`
				Label  string `json:"label"`
				Icon   string `json:"icon"`
				State  string `json:"state"`
				By     int    `json:"by"`
				ByIcon string `json:"by_icon"`
			} `json:"boxes"`
		} `json:"lines"`
	} `json:"grid"`
	Seats map[string]struct {
		Counters []struct {
			ID    string `json:"id"`
			Value int    `json:"value"`
		} `json:"counters"`
	} `json:"seats"`
}

func viewFor(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, viewer int) viewOf {
	t.Helper()
	raw, err := json.Marshal(nyet.New().Redact(g, st, viewer).Ext())
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		View viewOf `json:"view"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out.View
}

// Every box of the board and every line names a picture for the page to draw,
// and a box that has been crossed off names the animal of the player who did it.
func TestTheBoardNamesAPictureForEveryBoxAndLine(t *testing.T) {
	g := game(t, "nyet")
	rs := nyet.New()
	st, _ := rs.Start(g, 5)
	st = play(t, g, st, "nyet:trump:red")  // the wolf (seat 1)
	st = play(t, g, st, "nyet:first:wolf") // the crane (seat 2)
	v := viewFor(t, g, st, 1)
	want := map[string][]string{
		"first":   {"animal:wolf:red", "animal:crane:blue", "animal:tiger:green", "animal:bear:yellow", "animal:eagle:orange"},
		"discard": {"discard:none", "discard:1", "discard:2", "discard:except1", "discard:passleft"},
		"trump":   {"suit:red", "suit:blue", "suit:yellow", "suit:green"},
		"super":   {"super:red", "super:blue", "super:yellow", "super:green", "super:none"},
		"points":  {"points:1", "points:2", "points:3", "points:4", "points:-2"},
	}
	if len(v.Grid.Lines) != len(want) {
		t.Fatalf("%d lines, want %d", len(v.Grid.Lines), len(want))
	}
	for _, line := range v.Grid.Lines {
		if line.Icon != "line:"+line.ID {
			t.Errorf("line %s is pictured as %q", line.ID, line.Icon)
		}
		var got []string
		for _, b := range line.Boxes {
			got = append(got, b.Icon)
		}
		if strings.Join(got, ",") != strings.Join(want[line.ID], ",") {
			t.Errorf("%s boxes are pictured as %v, want %v", line.ID, got, want[line.ID])
		}
	}
	// The wolf crossed hearts off the trump line, the crane the wolf off the first-player line.
	for _, line := range v.Grid.Lines {
		for _, b := range line.Boxes {
			switch {
			case line.ID == "trump" && b.ID == "red":
				if b.State != "crossed" || b.By != 1 || b.ByIcon != "animal:wolf:red" {
					t.Errorf("hearts: %+v, want crossed by the wolf", b)
				}
				if b.Label != "Hearts" {
					t.Errorf("the hearts box says %q", b.Label)
				}
			case line.ID == "first" && b.ID == "1":
				if b.State != "crossed" || b.By != 2 || b.ByIcon != "animal:crane:blue" {
					t.Errorf("the wolf's box: %+v, want crossed by the crane", b)
				}
			case b.ByIcon != "":
				t.Errorf("%s:%s names a player though nobody crossed it: %+v", line.ID, b.ID, b)
			}
		}
	}
}

// How many cards each player holds is public, and goes down as cards are played.
func TestEachPlayersCardCountIsInTheScoreboard(t *testing.T) {
	g := game(t, "nyet")
	st := trick(g, 1, 1, "blue", "none", []int{1, 3},
		map[int][]string{1: {"red-5", "red-6", "red-7"}, 2: {"green-2", "green-9"}, 3: {"red-1#0"}, 4: {"red-2", "blue-4"}},
		nil)
	counts := func(v viewOf) string {
		var out []string
		for s := 1; s <= 4; s++ {
			for _, c := range v.Seats[string(rune('0'+s))].Counters {
				if c.ID == "cards" {
					out = append(out, string(rune('0'+c.Value)))
				}
			}
		}
		return strings.Join(out, "")
	}
	// Seen by any viewer, a spectator included.
	for _, viewer := range []int{0, 2} {
		if got := counts(viewFor(t, g, st, viewer)); got != "3212" {
			t.Errorf("viewer %d sees hand sizes %s, want 3212", viewer, got)
		}
	}
	st = play(t, g, st, "red-5")
	if got := counts(viewFor(t, g, st, 0)); got != "2212" {
		t.Errorf("after a card is played: %s, want 2212", got)
	}
}

// The deck's four colours are the four classic suits to the players: what the
// game says about a card, a trump or a super trump names the suit, while the
// notation keeps the colour's own name.
func TestWhatThePlayersReadNamesTheSuits(t *testing.T) {
	g := game(t, "nyet")
	st := trick(g, 1, 1, "blue", "none", []int{1, 3},
		map[int][]string{1: {"red-9", "red-6"}, 2: {"green-2"}, 3: {"red-1#0"}, 4: {"blue-4"}},
		nil)
	st = play(t, g, st, "red-9")
	events := strings.Join(data(st).Events, " ")
	if !strings.Contains(events, "plays the 9 of hearts") {
		t.Errorf("the history says %q, want it to name the 9 of hearts", events)
	}
	for _, colour := range []string{"red", "blue", "yellow", "green"} {
		if strings.Contains(strings.ToLower(events), colour) {
			t.Errorf("the history names a colour (%s): %q", colour, events)
		}
	}
	// The terms of the round, said when the last box is crossed.
	rs := nyet.New()
	start, _ := rs.Start(g, 7)
	keep := map[string]string{"first": "tiger", "discard": "none", "trump": "blue", "super": "red", "points": "2"}
	for guard := 0; guard < 60 && data(start).Phase == "nyet"; guard++ {
		seat := rs.ToMove(g, start)
		var pick string
		for _, m := range names(g, start, seat) {
			if parts := strings.Split(m, ":"); keep[parts[1]] != parts[2] {
				pick = m
				break
			}
		}
		start = play(t, g, start, pick)
	}
	terms := strings.Join(data(start).Events, " ")
	if !strings.Contains(terms, "trump spades") || !strings.Contains(terms, "super trump the 1s of hearts") {
		t.Errorf("the terms read %q, want trump spades and the 1s of hearts as super trumps", terms)
	}
}
