// Package council_test checks the Council fixture (tests/testdata/council)
// itself, so the platform tests that lean on it lean on something sound.
package council_test

import (
	"errors"
	"math/rand"
	"reflect"
	"slices"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/tutorial"
	"github.com/noderunner/bingle/tests/testdata/council"
)

func newCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(council.Module()); err != nil {
		t.Fatal(err)
	}
	return cat
}

func game(t *testing.T, cat *catalog.Catalog, id string) *catalog.CompiledGame {
	t.Helper()
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("no game %q", id)
	}
	return g
}

func play(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, seat int, notation string) *state.MoveResult {
	t.Helper()
	mv, err := state.ParseMove(g, notation)
	if err != nil {
		t.Fatalf("parse %q: %v", notation, err)
	}
	r, err := state.Apply(g, st, seat, mv)
	if err != nil {
		t.Fatalf("seat %d %q: %v", seat, notation, err)
	}
	return r
}

// It compiles as a board of five areas for four, and random games finish, every
// move surviving its notation and every position its document.
func TestRandomGamesFinish(t *testing.T) {
	cat := newCatalog(t)
	g := game(t, cat, council.ID)
	if len(g.Definition.Seats) != 4 || g.Board.Len() != 5 {
		t.Fatalf("the board: %d seats, %d areas", len(g.Definition.Seats), g.Board.Len())
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 20; i++ {
		st := state.InitialState(g)
		seat := state.FirstSeat(g)
		ended := false
		for ply := 0; ply < 1000 && !ended; ply++ {
			legal := state.LegalSelections(g, st, seat)
			if len(legal) == 0 {
				t.Fatalf("game %d ply %d: seat %d has no move", i, ply, seat)
			}
			mv := legal[rng.Intn(len(legal))]
			back, err := state.ParseMove(g, state.FormatMove(g, mv))
			if err != nil || state.FormatMove(g, back) != state.FormatMove(g, mv) {
				t.Fatalf("%q does not survive its notation: %v", state.FormatMove(g, mv), err)
			}
			r, err := state.Apply(g, st, seat, mv)
			if err != nil {
				t.Fatal(err)
			}
			doc, _ := r.NewState.MarshalJSON()
			var again state.BoardState
			if err := again.UnmarshalJSON(doc); err != nil {
				t.Fatal(err)
			}
			st, seat, ended = r.NewState, r.NextImpulseSeat, r.GameEnded
		}
		if !ended {
			t.Fatalf("game %d did not end", i)
		}
	}
}

// After another house's turn, a house that may Object is asked, aimed at
// when the turn ended where its envoy stands; an objection costs the claim.
func TestObjectIsAReaction(t *testing.T) {
	cat := newCatalog(t)
	g := game(t, cat, council.ID)
	rs, _ := state.RulesetOf(g)
	reactor := rs.(state.Reactor)
	st := state.InitialState(g)
	// Crow (seat 1) may Object; Stag (2) may not.
	if a := reactor.ReactionAbilities(g, st, 1); len(a) != 1 || a[0].Name != "Object" || !a[0].Held {
		t.Fatalf("Crow's abilities %+v", a)
	}
	if a := reactor.ReactionAbilities(g, st, 2); len(a) != 0 {
		t.Fatalf("Stag's abilities %+v", a)
	}
	// Crow claims North; nobody else may object, so Stag moves next.
	r := play(t, g, st, 1, "claim@north")
	if r.NextImpulseSeat != 2 || reactor.PendingReaction(g, r.NewState) != nil {
		t.Fatalf("after Crow's claim: next %d", r.NextImpulseSeat)
	}
	// Stag walks into North, where Crow's envoy stands: Crow is asked, aimed at.
	r = play(t, g, r.NewState, 2, "go@north")
	p := reactor.PendingReaction(g, r.NewState)
	if r.NextImpulseSeat != 1 || p == nil || p.Seat != 1 || p.Trigger != 2 || !p.Targeted {
		t.Fatalf("after Stag's move: next %d, pending %+v", r.NextImpulseSeat, p)
	}
	r = play(t, g, r.NewState, 1, "decline")
	if r.NextImpulseSeat != 3 {
		t.Fatalf("after Crow declines: next %d", r.NextImpulseSeat)
	}
	// Wolf moves where Crow is not: Crow is asked, not aimed at.
	r = play(t, g, r.NewState, 3, "go@hall")
	if p := reactor.PendingReaction(g, r.NewState); p == nil || p.Targeted {
		t.Fatalf("after Wolf's move: %+v", p)
	}
}

// Every table is a game of its own: a house per seat, the Court and a pool,
// named by a suffix it is rebuilt from; houses swap in the lobby.
func TestTables(t *testing.T) {
	cat := newCatalog(t)
	id, err := cat.Derive(council.ID, map[string]any{"houses": []any{"owl", "stag", "wolf", "crow"}, "court": true})
	if err != nil || id != council.ID+catalog.DerivedSep+"owl-stag-wolf-crow-court" {
		t.Fatalf("derive: %q %v", id, err)
	}
	g := game(t, cat, id)
	if g.Definition.Seats[0].Label != "Owl" || g.Definition.Seats[0].Color != "white" {
		t.Errorf("seat 1 is %+v", g.Definition.Seats[0])
	}
	rs, _ := state.RulesetOf(g)
	// With the Court, Stag may Object once it holds a claim: listed, not held.
	if a := rs.(state.Reactor).ReactionAbilities(g, state.InitialState(g), 2); len(a) != 1 || a[0].Held {
		t.Errorf("Stag at the Court: %+v", a)
	}
	// A house swap: Ram for seat 2 (from nobody), and Crow for seat 1 (from seat 4).
	sc := rs.(state.SeatFactions)
	opts, err := sc.SeatFactionOptions(g, 1, "crow", map[int]bool{})
	if err != nil || !reflect.DeepEqual(opts["houses"], []any{"crow", "stag", "wolf", "owl"}) {
		t.Errorf("crow to seat 1: %v %v", opts, err)
	}
	if _, err := sc.SeatFactionOptions(g, 1, "crow", map[int]bool{4: true}); !errors.Is(err, state.ErrFactionHeld) {
		t.Errorf("a taken seat's house: %v", err)
	}
	// A pool: the players choose in the lobby, from the pool only.
	pid, err := cat.Derive(council.ID, map[string]any{"houses": []any{"crow", "stag", "wolf", "ram"}, "house-pool": []any{"crow", "stag", "wolf", "ram", "owl"}})
	if err != nil {
		t.Fatal(err)
	}
	pg := game(t, cat, pid)
	prs, _ := state.RulesetOf(pg)
	if !prs.(state.OwnSeatFactions).PlayersChooseFactions(pg) {
		t.Error("a table with a pool lets its players choose")
	}
	for _, bad := range []map[string]any{
		{"houses": []any{"crow", "crow", "wolf", "ram"}},
		{"houses": []any{"crow", "stag", "wolf"}},
		{"houses": []any{"crow", "stag", "wolf", "bear"}},
		{"house-pool": []any{"owl"}},
		{"court": "yes"},
		{"dragons": true},
	} {
		if _, err := cat.Derive(council.ID, bad); err == nil {
			t.Errorf("%v was accepted", bad)
		}
	}
	// The catalog lists every house.
	raw, ok, err := state.SetupCatalogOf(g)
	if !ok || err != nil {
		t.Fatalf("catalog: %v %v", ok, err)
	}
	tabs := raw.(map[string]any)["tabs"].([]map[string]any)
	items := tabs[0]["items"].([]council.CatalogItem)
	var keys []string
	for _, it := range items {
		keys = append(keys, it.ID)
	}
	if !slices.Equal(keys, []string{"crow", "stag", "wolf", "ram", "owl"}) {
		t.Errorf("catalog houses %v", keys)
	}
}

// Council's tutorials load and play out as written: a series of two, and one
// of its own from a written start at a derived table.
func TestTutorialsPlayOut(t *testing.T) {
	cat := newCatalog(t)
	list, err := tutorial.Load(council.Module().Tutorials)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("%d tutorials, want 3", len(list))
	}
	for _, tu := range list {
		if err := tutorial.Verify(cat, council.ID, tu); err != nil {
			t.Errorf("%s: %v", tu.ID, err)
		}
	}
}
