// Package table_test checks the Table fixture (tests/testdata/table) itself,
// so the platform tests that lean on it lean on something sound.
package table_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/tests/testdata"
)

func game(t *testing.T) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(testdata.Table()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get(testdata.TableID)
	if !ok {
		t.Fatal("no Table")
	}
	return g
}

// A tile from the hand goes in the middle only; after that the colony grows
// next to itself, and a game played at random ends with the table full.
func TestTable_TilesAreLaidInTheMiddleAndGrowFromThere(t *testing.T) {
	g := game(t)
	st, seat := state.InitialState(g), state.FirstSeat(g)
	middle := map[string]bool{"e5": true, "e4": true, "f4": true, "d5": true, "f5": true, "e6": true, "d6": true}
	for _, mv := range state.LegalSelections(g, st, seat) {
		if mv.Action != "lay" || !middle[string(mv.To)] {
			t.Fatalf("a first move %+v is not a tile laid in the middle", mv)
		}
	}
	rng := rand.New(rand.NewSource(3))
	for plies := 0; ; plies++ {
		if plies > 500 {
			t.Fatal("the game does not end")
		}
		legal := state.LegalSelections(g, st, seat)
		if len(legal) == 0 {
			t.Fatalf("no move at ply %d", plies)
		}
		res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
		if err != nil {
			t.Fatal(err)
		}
		if res.GameEnded {
			if !strings.Contains(res.EndReason, "count") {
				t.Errorf("the game ended by %q", res.EndReason)
			}
			break
		}
		st, seat = res.NewState, res.NextImpulseSeat
	}
}
