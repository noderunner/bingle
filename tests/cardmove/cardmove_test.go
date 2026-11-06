// Package cardmove_test covers move cards (ADR 0120): a card's offsets read
// through the seat's forward, the card passed on after the move, a card spent
// when nothing moves, and absent counting one seat's pieces.
package cardmove_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// mini is Onitama in small: five cards that all move a piece one square
// forward or one square forward-right, two to each seat and one to the side.
func mini(setup []any, edit func(map[string]any)) map[string]any {
	var extra []any
	for _, id := range []string{"ox", "eel", "boar", "crab", "frog"} {
		extra = append(extra, map[string]any{"id": id, "label": id, "moves": [][2]int{{0, 1}, {1, 1}}})
	}
	if setup == nil {
		setup = []any{
			map[string]any{"position": "c1", "piece": "master", "color": "red"},
			map[string]any{"position": "b1", "piece": "student", "color": "red"},
			map[string]any{"position": "c5", "piece": "master", "color": "blue"},
		}
	}
	def := map[string]any{
		"id": "mini", "name": "Mini", "version": "0.1.0",
		"board": map[string]any{"generator": map[string]any{"type": "grid", "width": 5, "height": 5, "adjacency": "orthogonal8", "labels": "algebraic"}},
		"seats": []any{
			map[string]any{"seat": 1, "color": "red", "forward": "N"},
			map[string]any{"seat": 2, "color": "blue", "forward": "S"},
		},
		"pieces": map[string]any{"master": map[string]any{}, "student": map[string]any{}},
		"setup":  setup,
		"cards": map[string]any{
			"decks": map[string]any{"move": map[string]any{"extra": extra}},
			"zones": []any{
				map[string]any{"id": "deck", "scope": "table", "visibility": "all"},
				map[string]any{"id": "hand", "scope": "seat", "visibility": "all"},
				map[string]any{"id": "side", "scope": "table", "visibility": "all"},
			},
		},
		"deal": []any{
			map[string]any{"shuffle": "deck"},
			map[string]any{"from": "deck", "to": "hand", "each": 2, "face": "up"},
			map[string]any{"from": "deck", "to": "side", "count": 1, "face": "up"},
		},
		"turn_order": map[string]any{"type": "round-robin", "seats": []int{1, 2}},
		"actions":    []any{map[string]any{"id": "move", "type": "card-move", "hand": "hand", "pieces": []string{"master", "student"}, "pass_on": "side", "stuck": "spend"}},
		"win_condition": map[string]any{"type": "rules", "rules": []any{
			map[string]any{"id": "red-master", "type": "absent", "piece": "master", "of_seat": 2, "seat": 1, "reason": "master-captured"},
			map[string]any{"id": "blue-master", "type": "absent", "piece": "master", "of_seat": 1, "seat": 2, "reason": "master-captured"},
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

func start(t *testing.T, g *catalog.CompiledGame) *state.BoardState {
	t.Helper()
	st, err := state.InitialStateSeeded(g, 9)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func legal(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	var out []string
	for _, mv := range state.LegalSelections(g, st, seat) {
		out = append(out, state.FormatMove(g, mv))
	}
	return out
}

func cardsIn(st *state.BoardState, zone string, owner int) []string {
	var out []string
	for _, c := range st.Cards().Zone(zone, owner).Cards {
		out = append(out, c.ID)
	}
	return out
}

// Red's cards move up the board and Blue's down, as each player sees them;
// a piece may not land on its own side.
func TestACardIsReadByTheSeatsForward(t *testing.T) {
	g := mustCompile(t, mini(nil, nil))
	st := start(t, g)
	card := cardsIn(st, "hand", 1)[0]
	got := legal(g, st, 1)
	for _, want := range []string{card + ":c1-c2", card + ":c1-d2", card + ":b1-b2"} {
		if !slices.Contains(got, want) {
			t.Fatalf("red cannot play %s: %v", want, got)
		}
	}
	// With a student in front of it, the master may not land there.
	blocked := mustCompile(t, mini([]any{
		map[string]any{"position": "c1", "piece": "master", "color": "red"},
		map[string]any{"position": "c2", "piece": "student", "color": "red"},
		map[string]any{"position": "c5", "piece": "master", "color": "blue"},
	}, nil))
	bst := start(t, blocked)
	if slices.Contains(legal(blocked, bst, 1), cardsIn(bst, "hand", 1)[0]+":c1-c2") {
		t.Fatal("red may land on its own student")
	}
	blue := cardsIn(st, "hand", 2)[0]
	got = legal(g, st, 2)
	// Blue faces south: forward is down the board and its right is west.
	for _, want := range []string{blue + ":c5-c4", blue + ":c5-b4"} {
		if !slices.Contains(got, want) {
			t.Fatalf("blue cannot play %s: %v", want, got)
		}
	}
}

// After the move the mover takes the side card and the card used goes to the
// side; the notation reads back to the same selection.
func TestTheCardIsPassedOn(t *testing.T) {
	g := mustCompile(t, mini(nil, nil))
	st := start(t, g)
	hand, side := cardsIn(st, "hand", 1), cardsIn(st, "side", 0)
	text := hand[0] + ":c1-c2"
	mv, err := state.ParseMove(g, text)
	if err != nil || state.FormatMove(g, mv) != text {
		t.Fatalf("%s reads as %+v (%v)", text, mv, err)
	}
	if gs := state.Gestures(g, mv); len(gs) != 3 || gs[0].Kind != state.GestureCard || gs[0].Value != hand[0] {
		t.Fatalf("gestures %+v", gs)
	}
	res, err := state.Apply(g, st, 1, mv)
	if err != nil {
		t.Fatal(err)
	}
	ns := res.NewState
	if top, ok := ns.Get("c2"); !ok || top.Piece != "master" {
		t.Fatal("the master did not move")
	}
	if got := cardsIn(ns, "side", 0); !slices.Equal(got, []string{hand[0]}) {
		t.Fatalf("side holds %v, want the used %s", got, hand[0])
	}
	got := cardsIn(ns, "hand", 1)
	slices.Sort(got)
	want := []string{hand[1], side[0]}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("hand holds %v, want %v", got, want)
	}
	if res.NextImpulseSeat != 2 {
		t.Fatalf("next seat %d", res.NextImpulseSeat)
	}
}

// Landing on the other master captures it, and absent counts that seat's
// masters only.
func TestCapturingTheMasterWins(t *testing.T) {
	g := mustCompile(t, mini([]any{
		map[string]any{"position": "c1", "piece": "master", "color": "red"},
		map[string]any{"position": "c2", "piece": "master", "color": "blue"},
	}, nil))
	st := start(t, g)
	res, err := state.Apply(g, st, 1, state.Selection{Card: cardsIn(st, "hand", 1)[0], From: "c1", To: "c2"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != "master-captured" {
		t.Fatalf("ended %v winner %v reason %q", res.GameEnded, res.WinnerSeat, res.EndReason)
	}
}

// With no move to make, each card may be spent, and it is passed on.
func TestAStuckSeatSpendsACard(t *testing.T) {
	// Red's master stands on the top rank: every card moves it off the board.
	g := mustCompile(t, mini([]any{
		map[string]any{"position": "e5", "piece": "master", "color": "red"},
		map[string]any{"position": "a1", "piece": "master", "color": "blue"},
	}, nil))
	st := start(t, g)
	hand := cardsIn(st, "hand", 1)
	got := legal(g, st, 1)
	slices.Sort(got)
	want := []string{hand[0] + ":pass", hand[1] + ":pass"}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("legal %v, want %v", got, want)
	}
	mv, _ := state.ParseMove(g, hand[1]+":pass")
	res, err := state.Apply(g, st, 1, mv)
	if err != nil {
		t.Fatal(err)
	}
	if got := cardsIn(res.NewState, "side", 0); !slices.Equal(got, []string{hand[1]}) {
		t.Fatalf("the spent card is not at the side: %v", got)
	}
	// Spending a card is refused while a card moves a piece.
	free := mustCompile(t, mini(nil, nil))
	fst := start(t, free)
	if _, err := state.Apply(free, fst, 1, state.Selection{Card: cardsIn(fst, "hand", 1)[0]}); err == nil {
		t.Fatal("a card was spent while a piece could move")
	}
}

// The catalog refuses a card move it cannot read or play.
func TestTheCatalogRefusesABadCardMove(t *testing.T) {
	act := func(k string, v any) func(map[string]any) {
		return func(d map[string]any) { d["actions"].([]any)[0].(map[string]any)[k] = v }
	}
	for name, edit := range map[string]func(map[string]any){
		"no forward":     func(d map[string]any) { delete(d["seats"].([]any)[1].(map[string]any), "forward") },
		"a table hand":   act("hand", "side"),
		"a seat pass_on": act("pass_on", "hand"),
		"a bad stuck":    act("stuck", "sulk"),
		"absent of nobody": func(d map[string]any) {
			d["win_condition"].(map[string]any)["rules"].([]any)[0].(map[string]any)["of_seat"] = 3
		},
	} {
		if _, err := compile(mini(nil, edit)); err == nil {
			t.Errorf("%s: compiled", name)
		}
	}
}
