// Tests of rules that run as a WebAssembly module (ADR 0147): the host in
// internal/engine/wasmrules, the guest glue it speaks to, and the Draughts
// fixture built into a module by the test itself.
package wasmrules_test

import (
	"context"
	"io"
	"log/slog"
	"math/rand"
	"sync"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/engine/wasmrules"
	"github.com/noderunner/bingle/tests/testdata"
	"github.com/noderunner/bingle/tests/testdata/draughts"
)

// quietLog keeps what a test provokes on purpose out of its output.
var quietLog = slog.New(slog.NewTextHandler(io.Discard, nil))

// newRuntime returns a runtime that closes with the test.
func newRuntime(t testing.TB, opt wasmrules.Options) *wasmrules.Runtime {
	t.Helper()
	if opt.Log == nil {
		opt.Log = quietLog
	}
	rt, err := wasmrules.New(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close(context.Background()) })
	return rt
}

// load loads a fixture's module into rt.
func load(t testing.TB, rt *wasmrules.Runtime, pkg string) *wasmrules.Module {
	t.Helper()
	mod, err := rt.Load(context.Background(), "fixture", testdata.BuildWasm(t, pkg))
	if err != nil {
		t.Fatal(err)
	}
	return mod
}

// shared is one runtime and one module of the Draughts fixture for the tests
// that only play: compiling a module takes a second or two.
var shared = sync.OnceValues(func() (*wasmrules.Runtime, error) {
	return wasmrules.New(context.Background(), wasmrules.Options{Log: quietLog})
})

var sharedModule struct {
	once sync.Once
	mod  *wasmrules.Module
	err  error
}

func draughtsModule(t testing.TB) *wasmrules.Module {
	t.Helper()
	wasm := testdata.BuildWasm(t, testdata.DraughtsWasm)
	sharedModule.once.Do(func() {
		rt, err := shared()
		if err != nil {
			sharedModule.err = err
			return
		}
		sharedModule.mod, sharedModule.err = rt.Load(context.Background(), "draughts", wasm)
	})
	if sharedModule.err != nil {
		t.Fatal(sharedModule.err)
	}
	return sharedModule.mod
}

// games compiles the Draughts fixture and its variants as rules play them.
func games(t testing.TB, rules state.Ruleset) map[string]*catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(testdata.DraughtsWith(rules)); err != nil {
		t.Fatal(err)
	}
	out := map[string]*catalog.CompiledGame{}
	for _, g := range cat.List() {
		out[g.Definition.ID] = g
	}
	return out
}

func nativeGames(t testing.TB) map[string]*catalog.CompiledGame {
	return games(t, wasmrules.Native(draughts.New))
}

// step is one selection of a played game: who made it, how it is written, and
// the position after it as it is stored.
type step struct {
	Seat     int
	Notation string
	Position string
}

// playOut plays one game from seed with moves chosen from choice, and returns
// what happened selection by selection. It stops after plies selections.
func playOut(t testing.TB, game *catalog.CompiledGame, seed uint64, choice int64, plies int) []step {
	t.Helper()
	rng := rand.New(rand.NewSource(choice))
	st, err := state.InitialStateSeeded(game, seed)
	if err != nil {
		t.Fatalf("%s does not start: %v", game.Definition.ID, err)
	}
	start, err := st.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	steps := []step{{Position: string(start)}}
	seat := state.StartSeat(game, st)
	for ply := 0; ply < plies; ply++ {
		legal := state.LegalSelections(game, st, seat)
		if len(legal) == 0 {
			t.Fatalf("%s: seat %d has no selection after %d, and the game is not over", game.Definition.ID, seat, ply)
		}
		mv := legal[rng.Intn(len(legal))]
		res, err := state.Apply(game, st, seat, mv)
		if err != nil {
			t.Fatalf("%s: a listed selection is refused: %v", game.Definition.ID, err)
		}
		doc, err := res.NewState.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		steps = append(steps, step{Seat: seat, Notation: res.Notation, Position: string(doc)})
		if res.GameEnded {
			break
		}
		st, seat = res.NewState, res.NextImpulseSeat
	}
	return steps
}
