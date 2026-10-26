package tak_test

import (
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

// The fixed action space of Tak must contain every legal move of random
// games (all board sizes), with unique IDs.
func TestTakActionSpaceCoversRandomGames(t *testing.T) {
	for _, id := range []string{"tak-3x3", "tak-4x4", "tak-5x5", "tak-6x6"} {
		g := loadGame(t, id)
		as := state.ActionSpaceOf(g)
		if !as.Fixed || len(as.Moves) == 0 {
			t.Fatalf("%s: action space not fixed", id)
		}
		t.Logf("%s: %d actions", id, len(as.Moves))
		seen := map[string]bool{}
		for _, m := range as.Moves {
			if seen[m] {
				t.Fatalf("%s: duplicate action %q", id, m)
			}
			seen[m] = true
		}
		rng := rand.New(rand.NewSource(11))
		games := 30
		if testing.Short() {
			games = 8
		}
		for n := 0; n < games; n++ {
			st, seat := state.InitialState(g), 1
			for ply := 0; ply < 200; ply++ {
				legal := state.LegalMoves(g, st, seat)
				if len(legal) == 0 {
					break
				}
				for _, mv := range legal {
					text := state.FormatMove(g, mv)
					if _, ok := as.ID(text); !ok {
						t.Fatalf("%s: legal move %q missing from the action space", id, text)
					}
				}
				res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
				if err != nil {
					t.Fatal(err)
				}
				if res.GameEnded {
					break
				}
				st, seat = res.NewState, res.NextTurnSeat
			}
		}
	}
}

func TestTakPlanesShowStackDepthAndReserves(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	st, seat := position(t, g, "x5/x5/x2,12,x2/x5/x5 1 3")
	p, err := state.EncodePlanes(g, st, seat, seat, state.PlaneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Shape[1] != 5 || p.Shape[2] != 5 {
		t.Fatalf("shape %v", p.Shape)
	}
	cells := 25
	idx := map[string]int{}
	for i, n := range p.Channels {
		idx[n] = i
	}
	at := func(ch string, y, x int) float32 { return p.Data[idx[ch]*cells+y*5+x] }
	// "12" at c3 (x=2, y=2): bottom white(1), top black(2). Viewer = seat 1.
	if at("top/opp/flat", 2, 2) != 1 || at("depth2/me/flat", 2, 2) != 1 {
		t.Errorf("stack not encoded top=opp flat, 2nd=me flat")
	}
	if at("height", 2, 2) != 2 {
		t.Errorf("height = %v, want 2", at("height", 2, 2))
	}
	if at("to-move", 0, 0) != 1 {
		t.Errorf("to-move should be 1 for the viewer on move")
	}
	// Seen from the other seat the roles swap and to-move is 0.
	q, err := state.EncodePlanes(g, st, 2, seat, state.PlaneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if q.Data[idx["top/me/flat"]*cells+2*5+2] != 1 || q.Data[idx["to-move"]*cells] != 0 {
		t.Errorf("viewer-relative encoding broken for seat 2")
	}
}
