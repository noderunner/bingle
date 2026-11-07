// Package onitama_test covers Onitama as it is declared since 0.2.0 (ADRs
// 0119, 0120): the deal, the cards read from each side, the exchange, the two
// ways to win, and the conversion of positions the Go ruleset stored.
package onitama_test

import (
	"encoding/json"
	"math/rand"
	"slices"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/onitama"
)

func game(t *testing.T) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(onitama.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get("onitama")
	if !ok {
		t.Fatal("onitama is not registered")
	}
	return g
}

func cardsIn(st *state.BoardState, zone string, owner int) []string {
	var out []string
	for _, c := range st.Cards().Zone(zone, owner).Cards {
		out = append(out, c.ID)
	}
	return out
}

func legal(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	var out []string
	for _, m := range state.LegalSelections(g, st, seat) {
		out = append(out, state.FormatMove(g, m))
	}
	return out
}

// Five of the sixteen cards are dealt, face up: two to each player and one to
// the side; the pieces stand on their home ranks, the Masters on the Arches.
func TestDealAndSetup(t *testing.T) {
	g := game(t)
	for seed := uint64(1); seed < 30; seed++ {
		st, err := state.InitialStateSeeded(g, seed)
		if err != nil {
			t.Fatal(err)
		}
		inPlay := append(append(cardsIn(st, "hand", 1), cardsIn(st, "hand", 2)...), cardsIn(st, "side", 0)...)
		if len(inPlay) != 5 || len(cardsIn(st, "box", 0)) != 11 {
			t.Fatalf("seed %d: %v in play", seed, inPlay)
		}
		if err := state.Validate(g, st); err != nil {
			t.Fatal(err)
		}
		if len(st.Occupied()) != 10 {
			t.Fatalf("ten pieces: %d", len(st.Occupied()))
		}
	}
	st := state.InitialState(g)
	if o, _ := st.Get("c1"); o.Piece != "master" || o.Color != "red" {
		t.Error("Red's Master starts on c1")
	}
	if o, _ := st.Get("c5"); o.Piece != "master" || o.Color != "blue" {
		t.Error("Blue's Master starts on c5")
	}
	if state.HasSecrets(g) {
		t.Error("Onitama has secrets")
	}
}

// Red always starts, and has a move whatever the deal.
func TestRedStarts(t *testing.T) {
	g := game(t)
	for seed := uint64(1); seed < 40; seed++ {
		st, _ := state.InitialStateSeeded(g, seed)
		if state.FirstSeat(g) != 1 || len(state.LegalSelections(g, st, 1)) == 0 {
			t.Fatalf("seed %d: Red should start with moves", seed)
		}
	}
}

// The card used goes to the side and the side card comes into the hand.
func TestCardsAreExchangedWithTheSide(t *testing.T) {
	g := game(t)
	st, _ := state.InitialStateSeeded(g, 3)
	side := cardsIn(st, "side", 0)[0]
	mv := state.LegalSelections(g, st, 1)[0]
	res, err := state.Apply(g, st, 1, mv)
	if err != nil {
		t.Fatal(err)
	}
	if got := cardsIn(res.NewState, "side", 0); !slices.Equal(got, []string{mv.Card}) {
		t.Fatalf("side %v, want the used %s", got, mv.Card)
	}
	if !slices.Contains(cardsIn(res.NewState, "hand", 1), side) {
		t.Fatal("the side card did not come into Red's hand")
	}
}

// Random games keep valid positions, read their notation back, and end by the
// Way of the Stone, the Way of the Stream or the move limit.
func TestRandomGamesAreValidAndEnd(t *testing.T) {
	g := game(t)
	reasons := map[string]int{}
	for n := 0; n < 40; n++ {
		rng := rand.New(rand.NewSource(int64(n)))
		st, _ := state.InitialStateSeeded(g, uint64(n+1))
		seat := 1
		for ply := 0; ply < 400; ply++ {
			moves := state.LegalSelections(g, st, seat)
			if len(moves) == 0 {
				t.Fatalf("no move for seat %d", seat)
			}
			text := state.FormatMove(g, moves[rng.Intn(len(moves))])
			mv, err := state.ParseMove(g, text)
			if err != nil {
				t.Fatal(err)
			}
			res, err := state.Apply(g, st, seat, mv)
			if err != nil {
				t.Fatalf("%s: %v", text, err)
			}
			if err := state.Validate(g, res.NewState); err != nil {
				t.Fatal(err)
			}
			st, seat = res.NewState, res.NextImpulseSeat
			if res.GameEnded {
				reasons[res.EndReason]++
				break
			}
		}
	}
	for r := range reasons {
		if r != "stone" && r != "stream" && r != "ply-limit" {
			t.Errorf("a game ended for %q", r)
		}
	}
	if reasons["stone"] == 0 {
		t.Errorf("no game was won by the Way of the Stone: %v", reasons)
	}
}

// A position the Go ruleset stored converts: its cards go into the table,
// and the game plays on from it.
func TestARulesetPositionConverts(t *testing.T) {
	g := game(t)
	st := state.InitialState(g)
	raw, _ := st.MarshalJSON()
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	delete(doc, "cards")
	doc["ext"] = map[string]any{"turn": 1, "hands": [][]string{{"tiger", "ox"}, {"crab", "eel"}}, "side": "boar"}
	raw, _ = json.Marshal(doc)
	out, changed := onitama.Upgrade(raw)
	if !changed {
		t.Fatal("nothing converted")
	}
	if again, ch := onitama.Upgrade(out); ch || string(again) != string(out) {
		t.Fatal("a converted position converts again")
	}
	back := state.NewBoardState()
	if err := back.UnmarshalJSON(out); err != nil {
		t.Fatal(err)
	}
	if err := state.Validate(g, back); err != nil {
		t.Fatal(err)
	}
	if got := cardsIn(back, "hand", 1); !slices.Equal(got, []string{"tiger", "ox"}) || cardsIn(back, "side", 0)[0] != "boar" || len(cardsIn(back, "box", 0)) != 11 {
		t.Fatalf("hands %v, side %v", got, cardsIn(back, "side", 0))
	}
	l := legal(g, back, 1)
	if !slices.Contains(l, "tiger:c1-c3") {
		t.Fatalf("Red cannot play the Tiger from c1: %v", l)
	}
}
