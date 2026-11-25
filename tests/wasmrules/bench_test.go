package wasmrules_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/engine/wasmrules"
	"github.com/noderunner/bingle/tests/testdata"
	"github.com/noderunner/bingle/tests/testdata/draughts"
)

// midgame is a position of the 8×8 game twenty selections in, the seat to
// move there and its first legal selection.
func midgame(b *testing.B, game *catalog.CompiledGame) (*state.BoardState, int, state.Selection) {
	b.Helper()
	steps := playOut(b, nativeGames(b)[draughts.ID], 7, 7, 20)
	st := state.NewBoardState()
	if err := st.UnmarshalJSON([]byte(steps[len(steps)-1].Position)); err != nil {
		b.Fatal(err)
	}
	seat := state.StartSeat(game, st)
	legal := state.LegalSelections(game, st, seat)
	if len(legal) == 0 {
		b.Fatal("the benchmark's position has no legal selection")
	}
	return st, seat, legal[0]
}

// What the boundary of ADR 0147 costs, call by call, on the same rules and
// the same position: linked in (native), through the module's JSON without a
// module (encoded), as a WebAssembly module (wasm), and as a module whose
// calls have no deadline (wasm-no-deadline), which tells the cost of the
// deadline apart. Format carries next to nothing, so it is the cost of a call
// as such. The plugins plan refused a WebAssembly tier on these numbers. Run
// with
//
//	go test ./tests/wasmrules -run xxx -bench . -benchmem
func BenchmarkRules(b *testing.B) {
	free := load(b, newRuntime(b, wasmrules.Options{Deadline: -1}), testdata.DraughtsWasm)
	sides := []struct {
		name  string
		rules state.Ruleset
	}{
		{"native", wasmrules.Native(draughts.New)},
		{"encoded", wasmrules.Encoded(draughts.New)},
		{"wasm", draughtsModule(b).Rules()},
		{"wasm-no-deadline", free.Rules()},
	}
	for _, side := range sides {
		game := games(b, side.rules)[draughts.ID]
		st, seat, mv := midgame(b, game)
		b.Run("Legal/"+side.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if len(state.LegalSelections(game, st, seat)) == 0 {
					b.Fatal("no legal selections")
				}
			}
		})
		b.Run("Apply/"+side.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := state.Apply(game, st, seat, mv); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("Format/"+side.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if state.FormatMove(game, mv) == "" {
					b.Fatal("no notation")
				}
			}
		})
	}
}
