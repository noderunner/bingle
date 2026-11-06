// Package deal_test covers the seeded deal at the start of a declared game
// (ADR 0119): the same seed deals the same cards, the table is stored with the
// position, and a viewer sees only the cards their zones show them.
package deal_test

import (
	"encoding/json"
	"math/rand"
	"slices"
	"testing"

	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// game is a 3×3 board with sixteen numbered cards: two to each player's hand
// and one to the side, as Onitama deals. handSeen is who sees a hand.
func game(handSeen string, edit func(map[string]any)) map[string]any {
	def := map[string]any{
		"id": "dealt", "name": "Dealt", "version": "0.1.0",
		"board":  map[string]any{"generator": map[string]any{"type": "grid", "width": 3, "height": 3, "adjacency": "orthogonal4", "labels": "algebraic"}},
		"seats":  []any{map[string]any{"seat": 1, "color": "red"}, map[string]any{"seat": 2, "color": "blue"}},
		"pieces": map[string]any{"stone": map[string]any{}},
		"cards": map[string]any{
			"suits": map[string]any{"move": map[string]any{"label": "Move"}},
			"decks": map[string]any{"main": map[string]any{"rank_list": "1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16"}},
			"zones": []any{
				map[string]any{"id": "deck", "scope": "table", "visibility": "none", "layout": "stack"},
				map[string]any{"id": "hand", "scope": "seat", "visibility": handSeen, "layout": "row"},
				map[string]any{"id": "side", "scope": "table", "visibility": "all", "layout": "row"},
			},
		},
		"deal": []any{
			map[string]any{"shuffle": "deck"},
			map[string]any{"from": "deck", "to": "hand", "each": 2, "face": "up"},
			map[string]any{"from": "deck", "to": "side", "count": 1, "face": "up"},
		},
		"turn_order": map[string]any{"type": "round-robin", "seats": []int{1, 2}},
		"actions":    []any{map[string]any{"id": "place", "type": "place", "pieces": []string{"stone"}, "source": "supply"}},
		"win_condition": map[string]any{"type": "rules", "rules": []any{
			map[string]any{"id": "full", "type": "end", "when": []string{"board-full"}, "decide": map[string]any{"type": "draw"}, "reason": "draw"},
		}},
	}
	if edit != nil {
		edit(def)
	}
	return def
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

func start(t *testing.T, g *catalog.CompiledGame, seed uint64) *state.BoardState {
	t.Helper()
	st, err := state.InitialStateSeeded(g, seed)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// ids lists the cards of a zone, bottom first ("" for a hidden one).
func ids(st *state.BoardState, zone string, owner int) []string {
	var out []string
	for _, c := range st.Cards().Zone(zone, owner).Cards {
		out = append(out, c.ID)
	}
	return out
}

// The deal follows the definition, face up, and is the same for the same seed.
func TestTheSeedDecidesTheDeal(t *testing.T) {
	g := mustCompile(t, game("all", nil))
	a, b := start(t, g, 42), start(t, g, 42)
	for _, z := range []struct {
		zone  string
		owner int
		n     int
	}{{"hand", 1, 2}, {"hand", 2, 2}, {"side", 0, 1}, {"deck", 0, 11}} {
		got := ids(a, z.zone, z.owner)
		if len(got) != z.n {
			t.Fatalf("%s:%d holds %d cards, want %d", z.zone, z.owner, len(got), z.n)
		}
		if !slices.Equal(got, ids(b, z.zone, z.owner)) {
			t.Fatalf("%s:%d differs between two deals with one seed", z.zone, z.owner)
		}
	}
	for _, c := range a.Cards().Zone("hand", 1).Cards {
		if !c.Up {
			t.Fatal("a hand card was dealt face down")
		}
	}
	differs := false
	for seed := uint64(1); seed < 6 && !differs; seed++ {
		differs = !slices.Equal(ids(start(t, g, seed), "hand", 1), ids(a, "hand", 1))
	}
	if !differs {
		t.Fatal("five other seeds all dealt the same first hand")
	}
	// The undealt deck is face down, so it is a secret; with every zone open
	// there is none.
	if !state.HasSecrets(g) {
		t.Fatal("a face-down deck is no secret")
	}
	open := mustCompile(t, game("all", func(d map[string]any) {
		d["cards"].(map[string]any)["zones"].([]any)[0].(map[string]any)["visibility"] = "all"
	}))
	if state.HasSecrets(open) {
		t.Fatal("a game whose cards are all open has secrets")
	}
}

// The table travels with the position document and is checked when loaded.
func TestTheTableIsStored(t *testing.T) {
	g := mustCompile(t, game("all", nil))
	st := start(t, g, 7)
	raw, err := st.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	back := state.NewBoardState()
	if err := back.UnmarshalJSON(raw); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(back, "side", 0), ids(st, "side", 0)) || !slices.Equal(ids(back, "hand", 2), ids(st, "hand", 2)) {
		t.Fatal("the cards changed in a round trip")
	}
	if err := state.Validate(g, back); err != nil {
		t.Fatal(err)
	}
	// A card in two places is refused.
	back.Cards().Zone("side", 0).Push(cards.Card{ID: ids(st, "hand", 1)[0], Up: true})
	if err := state.Validate(g, back); err == nil {
		t.Fatal("a card dealt twice passed validation")
	}
}

// A hand only its owner sees is hidden from the other seat and the public,
// and a determinized position deals the hidden cards again among themselves.
func TestAHiddenHandIsRedacted(t *testing.T) {
	// Hands dealt face down: a face-up card is seen by everyone.
	g := mustCompile(t, game("owner", func(d map[string]any) { d["deal"].([]any)[1].(map[string]any)["face"] = "down" }))
	if !state.HasSecrets(g) {
		t.Fatal("hidden hands are no secret")
	}
	st := start(t, g, 3)
	seen := state.Public(g, st, 1)
	if got := ids(seen, "hand", 1); !slices.Equal(got, ids(st, "hand", 1)) {
		t.Fatalf("seat 1 does not see its own hand: %v", got)
	}
	for _, id := range ids(seen, "hand", 2) {
		if id != "" {
			t.Fatalf("seat 1 sees seat 2's card %q", id)
		}
	}
	if slices.Equal(ids(state.Public(g, st, 0), "side", 0), []string{""}) {
		t.Fatal("the open side card is hidden")
	}
	hiddenOf := func(s *state.BoardState) []string {
		out := append(ids(s, "hand", 2), ids(s, "deck", 0)...)
		slices.Sort(out)
		return out
	}
	rng := rand.New(rand.NewSource(1))
	moved := false
	for i := 0; i < 5; i++ {
		d := state.Determinize(g, st, 1, rng)
		if !slices.Equal(ids(d, "hand", 1), ids(st, "hand", 1)) || !slices.Equal(ids(d, "side", 0), ids(st, "side", 0)) {
			t.Fatal("determinizing changed cards seat 1 can see")
		}
		if !slices.Equal(hiddenOf(d), hiddenOf(st)) {
			t.Fatal("determinizing changed which cards are hidden")
		}
		moved = moved || !slices.Equal(ids(d, "hand", 2), ids(st, "hand", 2))
	}
	if !moved {
		t.Fatal("five determinizations left seat 2's hand as it was")
	}
}

// The catalog refuses a deal it cannot play.
func TestTheCatalogRefusesABadDeal(t *testing.T) {
	step := func(i int, k string, v any) func(map[string]any) {
		return func(d map[string]any) { d["deal"].([]any)[i].(map[string]any)[k] = v }
	}
	for name, edit := range map[string]func(map[string]any){
		"unknown zone":       step(1, "to", "pocket"),
		"each to the table":  step(2, "each", 1),
		"count to a seat":    step(1, "count", 1),
		"more than the deck": step(1, "each", 9),
		"bad face":           step(2, "face", "sideways"),
		"shuffle a hand":     step(0, "shuffle", "hand"),
		"no cards":           func(d map[string]any) { delete(d, "cards") },
	} {
		if _, err := compile(game("all", edit)); err == nil {
			t.Errorf("%s: compiled", name)
		}
	}
}
