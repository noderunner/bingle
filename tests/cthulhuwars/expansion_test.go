package cthulhuwars_test

import (
	"fmt"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// derived registers the module and returns the game that base's options
// describe: the cults in seat order and whether the High Priests are in.
func derived(t testing.TB, base string, factions []string, hp bool) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	opts := map[string]any{}
	if factions != nil {
		var l []any
		for _, f := range factions {
			l = append(l, f)
		}
		opts["factions"] = l
	}
	if hp {
		opts["high-priests"] = true
	}
	id, err := cat.Derive(base, opts)
	if err != nil {
		t.Fatalf("derive %s %v hp=%v: %v", base, factions, hp, err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("derived game %q is not registered", id)
	}
	return g
}

func TestDerivedGamesSeatTheChosenCults(t *testing.T) {
	g := derived(t, "cthulhu-wars-5p", []string{"gc", "ww", "sl", "ys", "bg"}, true)
	if g.Definition.ID != "cthulhu-wars-5p~gc-ww-sl-ys-bg-hp" {
		t.Errorf("id = %q", g.Definition.ID)
	}
	want := []struct{ color, label string }{{"green", "Great Cthulhu"}, {"white", "Windwalker"}, {"orange", "Sleeper"}, {"yellow", "Yellow Sign"}, {"red", "Black Goat"}}
	for i, s := range g.Definition.Seats {
		if s.Seat != i+1 || s.Color != want[i].color || s.Label != want[i].label {
			t.Errorf("seat %d = %+v, want %v", i+1, s, want[i])
		}
	}
	// Windwalker (seat 2) sets up first: it leads the turn order.
	if first := state.FirstSeat(g); first != 2 {
		t.Errorf("the first to move is seat %d, want Windwalker's seat 2", first)
	}
	st := state.InitialState(g)
	d := data(t, st)
	if d.Stage != "setup" || d.Setup != 2 || d.Players != 5 {
		t.Errorf("stage %q setup %d players %d", d.Stage, d.Setup, d.Players)
	}
	// Every cult but Windwalker already stands in its start area; each box has
	// its High Priest in the pool.
	if len(d.Gates) != 4 {
		t.Errorf("gates %v", d.Gates)
	}
	for _, c := range []struct{ color, typ string }{{"green", "high-priest"}, {"white", "wendigo"}, {"orange", "formless-spawn"}} {
		if st.Reserve(c.color, c.typ) == 0 {
			t.Errorf("%s has no %s in its pool", c.color, c.typ)
		}
	}
	if err := state.Validate(g, st); err != nil {
		t.Errorf("the start is invalid: %v", err)
	}
	// Refused: a faction twice, an unknown one, the wrong number of seats.
	cat := catalog.NewCatalog()
	_ = cat.RegisterModule(cthulhuwars.Module())
	for name, opts := range map[string]map[string]any{
		"twice":   {"factions": []any{"gc", "gc", "bg", "ys", "ww"}},
		"unknown": {"factions": []any{"gc", "cc", "bg", "ys", "xx"}},
		"count":   {"factions": []any{"gc", "cc", "bg"}},
		"option":  {"colour": "red"},
	} {
		if _, err := cat.Derive("cthulhu-wars-5p", opts); err == nil {
			t.Errorf("%s: the options were accepted", name)
		}
	}
	// The same options name the same game, and the id loads again.
	again, ok := cat.Get("cthulhu-wars-5p~gc-ww-sl-ys-bg-hp")
	if !ok || len(again.Definition.Seats) != 5 {
		t.Errorf("the derived id does not load")
	}
}

// Random games of every kind of table stay valid and finish.
func TestRandomGamesWithTheExpansionCults(t *testing.T) {
	tables := []struct {
		base     string
		factions []string
		hp       bool
	}{
		{"cthulhu-wars-5p", nil, false},
		{"cthulhu-wars-5p", []string{"gc", "cc", "ww", "sl", "bg"}, true},
		{"cthulhu-wars-5p", []string{"sl", "ww", "ys", "cc", "gc"}, false},
		{"cthulhu-wars", []string{"ww", "sl", "gc", "bg"}, true},
		{"cthulhu-wars-3p", []string{"sl", "ww", "cc"}, false},
		{"cthulhu-wars-3-5", []string{"sl", "gc", "bg", "ys"}, true},
	}
	seeds := 3
	if testing.Short() {
		seeds = 1
	}
	for _, tb := range tables {
		g := derived(t, tb.base, tb.factions, tb.hp)
		for seed := int64(1); seed <= int64(seeds); seed++ {
			name := fmt.Sprintf("%s seed %d", g.Definition.ID, seed)
			st, ply, done := randomGame(t, g, seed, 6000, func(ply int, st *state.BoardState, seat int, mv state.Move) {
				if err := state.Validate(g, st); err != nil {
					t.Fatalf("%s ply %d after %s: %v\n%s", name, ply, state.FormatMove(g, mv), err, state.RenderText(g, st))
				}
			})
			if !done {
				t.Errorf("%s: no end after %d plies\n%s", name, ply, state.RenderText(g, st))
			}
		}
	}
}

// The bots' guide plays the expansion cults and the High Priests legally, never
// abandons a Gate and brings most games to an end.
func TestGuidedGamesWithTheExpansionCultsAreLegalAndReachAnEnd(t *testing.T) {
	tables := []struct {
		base     string
		factions []string
		hp       bool
	}{
		{"cthulhu-wars-5p", []string{"gc", "cc", "bg", "ys", "ww"}, false},
		{"cthulhu-wars-5p", []string{"sl", "ww", "gc", "cc", "bg"}, true},
		{"cthulhu-wars", []string{"ww", "sl", "gc", "ys"}, true},
		{"cthulhu-wars-3p", []string{"sl", "cc", "ww"}, false},
	}
	games := 6
	if testing.Short() {
		games = 2
	}
	for _, tb := range tables {
		g := derived(t, tb.base, tb.factions, tb.hp)
		ended, abandons := 0, 0
		for seed := int64(1); seed <= int64(games); seed++ {
			s := playGuided(t, g, seed, 8000)
			if s.ended {
				ended++
			}
			abandons += s.abandons
			t.Logf("%s seed %d: plies %d rounds %d ended %v winner %d", g.Definition.ID, seed, s.plies, s.rounds, s.ended, s.winner)
		}
		if abandons != 0 {
			t.Errorf("%s: bots abandoned %d Gates", g.Definition.ID, abandons)
		}
		if ended < games/2 {
			t.Errorf("%s: only %d of %d guided games ended", g.Definition.ID, ended, games)
		}
	}
}
