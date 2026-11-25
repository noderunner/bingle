package difftest_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/engine/wasmrules"
	"github.com/noderunner/bingle/tests/difftest"
	"github.com/noderunner/bingle/tests/testdata"
	"github.com/noderunner/bingle/tests/testdata/draughts"
)

// draughtsGames compiles the Draughts fixture and its variants as rules play
// them.
func draughtsGames(t *testing.T, rules state.Ruleset) map[string]*catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(testdata.DraughtsWith(rules)); err != nil {
		t.Fatal(err)
	}
	games := map[string]*catalog.CompiledGame{}
	for _, g := range cat.List() {
		games[g.Definition.ID] = g
	}
	if len(games) != 3 {
		t.Fatalf("the fixture has %d games, want its three", len(games))
	}
	return games
}

// draughtsData is the game's own part of a position, which both sides must
// agree on to the byte once it is read as JSON: whose move it is, the chain of
// jumps in progress and the count towards a draw.
func draughtsData(st *state.BoardState) string {
	raw, err := json.Marshal(st)
	if err != nil {
		return err.Error()
	}
	var doc struct {
		Ply int             `json:"ply"`
		Ext json.RawMessage `json:"ext"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err.Error()
	}
	return string(doc.Ext)
}

// The same rules agree with themselves across the boundary of ADR 0147: linked
// in (the reference), through the module's JSON in process, and as a
// WebAssembly module, over a random battery of every game of the fixture.
func TestRulesAgreeNativelyAndThroughWasm(t *testing.T) {
	ctx := context.Background()
	rt, err := wasmrules.New(ctx, wasmrules.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rt.Close(ctx) }()
	mod, err := rt.Load(ctx, "draughts", testdata.BuildWasm(t, testdata.DraughtsWasm))
	if err != nil {
		t.Fatal(err)
	}

	native := draughtsGames(t, wasmrules.Native(draughts.New))
	candidates := map[string]map[string]*catalog.CompiledGame{
		"encoded": draughtsGames(t, wasmrules.Encoded(draughts.New)),
		"wasm":    draughtsGames(t, mod.Rules()),
	}
	// And the module as TinyGo builds it, where there is a TinyGo.
	if _, ok := testdata.TinyGo(); ok {
		tiny, err := rt.Load(ctx, "draughts-tinygo", testdata.BuildWasmTinyGo(t, testdata.DraughtsWasm))
		if err != nil {
			t.Fatal(err)
		}
		candidates["wasm-tinygo"] = draughtsGames(t, tiny.Rules())
	}
	for name, games := range candidates {
		// A call into the module takes about a millisecond, so it plays fewer
		// games than the JSON alone does.
		opt := difftest.Options{Games: 12, Plies: 400, Sample: 4, Seed: 1 << 60}
		if name != "encoded" {
			opt.Games = 5
		}
		if testing.Short() {
			opt.Games = 2
		}
		for id, ref := range native {
			t.Run(name+"/"+id, func(t *testing.T) {
				err := difftest.Compare(
					difftest.Side{Game: ref, Extra: draughtsData},
					difftest.Side{Game: games[id], Extra: draughtsData}, opt)
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
