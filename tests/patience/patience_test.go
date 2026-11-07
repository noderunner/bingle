// Package patience_test covers the patience vocabulary (ADR 0122) on the
// families Solitaire does not play: Yukon, Scorpion, Baker's Dozen and Forty
// Thieves. Klondike, Spider and FreeCell are held to Solitaire's ruleset by the
// differential harness in internal/games/solitaire.
package patience_test

import (
	"encoding/json"
	"math/rand"
	"slices"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

func game(id string, copies int, block map[string]any) map[string]any {
	ranks := []any{}
	for i, r := range []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"} {
		ranks = append(ranks, map[string]any{"id": r, "value": i + 1, "count": copies})
	}
	return map[string]any{
		"id": id, "name": id, "version": "0.1.0",
		"board":  map[string]any{"generator": map[string]any{"type": "grid", "width": 1, "height": 1, "adjacency": "orthogonal4"}, "style": "none"},
		"seats":  []any{map[string]any{"seat": 1, "color": "red"}},
		"pieces": map[string]any{"token": map[string]any{}},
		"cards": map[string]any{
			"suits": map[string]any{
				"hearts": map[string]any{"label": "Hearts"}, "spades": map[string]any{"label": "Spades"},
				"diamonds": map[string]any{"label": "Diamonds"}, "clubs": map[string]any{"label": "Clubs"},
			},
			"suit_order": []string{"hearts", "spades", "diamonds", "clubs"},
			"decks":      map[string]any{"main": map[string]any{"ranks": ranks}},
		},
		"turn_order":    map[string]any{"type": "round-robin", "seats": []int{1}},
		"patience":      block,
		"win_condition": map[string]any{"type": "patience"},
	}
}

var families = map[string]map[string]any{
	"yukon": game("yukon", 1, map[string]any{
		"columns": map[string]any{"count": 7, "deal": map[string]any{"sizes": []int{1, 6, 7, 8, 9, 10, 11}, "down": []int{0, 1, 2, 3, 4, 5, 6}},
			"build": map[string]any{"suit": "alternate"}, "lift": "any-face-up", "empty": "king"},
		"foundations": map[string]any{"count": 4},
		"finish":      true,
	}),
	"scorpion": game("scorpion", 1, map[string]any{
		"columns": map[string]any{"count": 7, "deal": map[string]any{"sizes": 7, "down": []int{3, 3, 3, 3, 0, 0, 0}},
			"build": map[string]any{"suit": "same"}, "lift": "any-face-up", "empty": "king"},
		"stock": map[string]any{"deal": "row", "columns": 3},
		"off":   map[string]any{"run": "king-to-ace", "suit": "same"},
	}),
	"bakers-dozen": game("bakers-dozen", 1, map[string]any{
		"columns":     map[string]any{"count": 13, "deal": map[string]any{"sizes": 4, "down": 0, "kings_under": true}, "lift": "top", "empty": "none"},
		"foundations": map[string]any{"count": 4},
		"finish":      true,
	}),
	"forty-thieves": game("forty-thieves", 2, map[string]any{
		"columns":     map[string]any{"count": 10, "deal": map[string]any{"sizes": 4, "down": 0}, "build": map[string]any{"suit": "same"}, "lift": "top"},
		"foundations": map[string]any{"count": 8},
		"stock":       map[string]any{"draw": 1, "to": "waste"},
	}),
}

func compile(def map[string]any) (*catalog.CompiledGame, error) {
	raw, err := json.Marshal(def)
	if err != nil {
		return nil, err
	}
	cat := catalog.NewCatalog()
	if err := cat.Register(def["id"].(string), raw, nil); err != nil {
		return nil, err
	}
	g, _ := cat.Get(def["id"].(string))
	return g, nil
}

func mustCompile(t *testing.T, def map[string]any) *catalog.CompiledGame {
	t.Helper()
	g, err := compile(def)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func pile(st *state.BoardState, id string) (n, down int) {
	for _, c := range st.Cards().Zone(id, 0).Cards {
		n++
		if !c.Up {
			down++
		}
	}
	return
}

func notations(g *catalog.CompiledGame, st *state.BoardState) []string {
	var out []string
	for _, m := range state.LegalSelections(g, st, 1) {
		out = append(out, state.FormatMove(g, m))
	}
	return out
}

// Every family deals as declared, and random games play to their end: every
// move listed applies, reads back from its notation, and a hint is offered.
func TestEveryFamilyPlays(t *testing.T) {
	for id, def := range families {
		t.Run(id, func(t *testing.T) {
			g := mustCompile(t, def)
			for seed := uint64(1); seed <= 6; seed++ {
				st, err := state.InitialStateSeeded(g, seed)
				if err != nil {
					t.Fatal(err)
				}
				if err := state.Validate(g, st); err != nil {
					t.Fatal(err)
				}
				rng := rand.New(rand.NewSource(int64(seed)))
				for ply := 0; ply < 400; ply++ {
					legal := state.LegalSelections(g, st, 1)
					if len(legal) == 0 {
						t.Fatalf("seed %d ply %d: no move, and the game did not end", seed, ply)
					}
					if len(state.GuidedMoves(g, st, 1, legal)) == 0 {
						t.Fatal("no hint")
					}
					m := legal[rng.Intn(len(legal))]
					back, err := state.ParseMove(g, state.FormatMove(g, m))
					if err != nil || back.Key() != m.Key() {
						t.Fatalf("%s reads back as %+v (%v)", state.FormatMove(g, m), back, err)
					}
					res, err := state.Apply(g, st, 1, m)
					if err != nil {
						t.Fatalf("seed %d: %s: %v", seed, state.FormatMove(g, m), err)
					}
					st = res.NewState
					if res.GameEnded {
						break
					}
				}
			}
		})
	}
}

// The deals: Yukon's face-down cards under five up, Scorpion's three face down
// in the first four columns and three cards in the stock, Baker's Dozen's kings
// at the bottom, and Forty Thieves' two decks.
func TestTheDeals(t *testing.T) {
	st := func(id string) (*catalog.CompiledGame, *state.BoardState) {
		g := mustCompile(t, families[id])
		s, _ := state.InitialStateSeeded(g, 3)
		return g, s
	}
	_, y := st("yukon")
	if n, down := pile(y, "t7"); n != 11 || down != 6 {
		t.Fatalf("yukon t7: %d cards, %d down", n, down)
	}
	_, s := st("scorpion")
	if n, down := pile(s, "t1"); n != 7 || down != 3 {
		t.Fatalf("scorpion t1: %d cards, %d down", n, down)
	}
	if n, _ := pile(s, "stock"); n != 3 {
		t.Fatalf("scorpion stock: %d", n)
	}
	bg, b := st("bakers-dozen")
	for i := 1; i <= 13; i++ {
		cs := b.Cards().Zone(catalog.Column(i), 0).Cards
		king := true
		for _, c := range cs {
			isKing := bg.Cards.Type(c.ID).Value == 13
			if isKing && !king {
				t.Fatalf("column %d has a king above another card", i)
			}
			king = king && isKing
		}
	}
	_, f := st("forty-thieves")
	if n, _ := pile(f, "stock"); n != 104-40 {
		t.Fatalf("forty thieves stock: %d", n)
	}
}

// Forty Thieves never turns the waste over; Baker's Dozen's empty columns take
// nothing; Scorpion deals a row to three columns.
func TestTheRulesOfEach(t *testing.T) {
	g := mustCompile(t, families["forty-thieves"])
	st, _ := state.InitialStateSeeded(g, 2)
	for i := 0; i < 64; i++ {
		res, err := state.Apply(g, st, 1, state.Selection{Action: "patience", Piece: "draw"})
		if err != nil {
			t.Fatal(err)
		}
		st = res.NewState
	}
	if slices.Contains(notations(g, st), "recycle") {
		t.Fatal("forty thieves may turn the waste over")
	}
	sg := mustCompile(t, families["scorpion"])
	ss, _ := state.InitialStateSeeded(sg, 2)
	res, err := state.Apply(sg, ss, 1, state.Selection{Action: "patience", Piece: "deal"})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{8, 8, 8, 7} {
		if n, _ := pile(res.NewState, catalog.Column(i+1)); n != want {
			t.Fatalf("after the deal t%d has %d cards, want %d", i+1, n, want)
		}
	}
	bg := mustCompile(t, families["bakers-dozen"])
	bs, _ := state.InitialStateSeeded(bg, 2)
	bs.Cards().Zone("t1", 0).Clear()
	for _, n := range notations(bg, bs) {
		if len(n) > 3 && n[len(n)-3:] == "-t1" {
			t.Fatalf("%s goes to an empty column in Baker's Dozen", n)
		}
	}
}

// The catalog refuses a patience block it cannot play.
func TestTheCatalogRefusesBadPatience(t *testing.T) {
	block := func(edit func(map[string]any)) map[string]any {
		def := game("bad", 1, map[string]any{
			"columns":     map[string]any{"count": 7, "deal": map[string]any{"sizes": []int{1, 2, 3, 4, 5, 6, 7}}},
			"foundations": map[string]any{"count": 4},
		})
		edit(def)
		return def
	}
	cols := func(d map[string]any) map[string]any {
		return d["patience"].(map[string]any)["columns"].(map[string]any)
	}
	for name, edit := range map[string]func(map[string]any){
		"too many cards":    func(d map[string]any) { cols(d)["deal"] = map[string]any{"sizes": 9} },
		"bad lift":          func(d map[string]any) { cols(d)["lift"] = "all" },
		"bad empty":         func(d map[string]any) { cols(d)["empty"] = "queen" },
		"three foundations": func(d map[string]any) { d["patience"].(map[string]any)["foundations"] = map[string]any{"count": 3} },
		"no way to win":     func(d map[string]any) { delete(d["patience"].(map[string]any), "foundations") },
		"two seats": func(d map[string]any) {
			d["seats"] = append(d["seats"].([]any), map[string]any{"seat": 2, "color": "blue"})
		},
		"other win": func(d map[string]any) { d["win_condition"] = map[string]any{"type": "none"} },
		"stock both ways": func(d map[string]any) {
			d["patience"].(map[string]any)["stock"] = map[string]any{"draw": 1, "to": "waste", "deal": "row"}
		},
	} {
		if _, err := compile(block(edit)); err == nil {
			t.Errorf("%s: compiled", name)
		}
	}
}
