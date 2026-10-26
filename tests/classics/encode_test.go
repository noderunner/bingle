package classics_test

import (
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/classics"
)

// For every game with a fixed action space, any legal move reached in random
// play must have an action ID (the space is a superset of the legal moves),
// IDs must be unique, and the planes must keep a constant, correct shape.
func TestActionSpaceCoversLegalMovesAndPlanesAreWellFormed(t *testing.T) {
	for _, mod := range classics.Modules() {
		g := load(t, mod.ID)
		as := state.ActionSpaceOf(g)
		names, h, w, err := state.PlaneChannels(g, state.PlaneOptions{})
		if err != nil {
			t.Fatalf("%s: PlaneChannels: %v", mod.ID, err)
		}
		if w != g.Board.Width() || h != g.Board.Height() {
			t.Errorf("%s: plane size %dx%d, board %dx%d", mod.ID, w, h, g.Board.Width(), g.Board.Height())
		}
		t.Logf("%s: %d channels, fixed=%v actions=%d", mod.ID, len(names), as.Fixed, len(as.Moves))

		if as.Fixed {
			seen := map[string]bool{}
			for _, m := range as.Moves {
				if seen[m] {
					t.Fatalf("%s: duplicate action %q", mod.ID, m)
				}
				seen[m] = true
			}
		}

		rng := rand.New(rand.NewSource(7))
		for game := 0; game < 4; game++ {
			st, seat := state.InitialState(g), state.FirstSeat(g)
			for ply := 0; ply < 400; ply++ {
				legal := state.LegalMoves(g, st, seat)
				if len(legal) == 0 {
					break
				}
				if as.Fixed {
					for _, mv := range legal {
						text := state.FormatMove(g, mv)
						if _, ok := as.ID(text); !ok {
							t.Fatalf("%s ply %d: legal move %q has no action id", mod.ID, ply, text)
						}
					}
				}
				p, err := state.EncodePlanes(g, st, seat, seat, state.PlaneOptions{})
				if err != nil {
					t.Fatalf("%s: EncodePlanes: %v", mod.ID, err)
				}
				if got := p.Shape[0] * p.Shape[1] * p.Shape[2]; got != len(p.Data) || p.Shape[0] != len(names) {
					t.Fatalf("%s: shape %v data %d channels %d", mod.ID, p.Shape, len(p.Data), len(names))
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

// The encoding is viewer-relative: the first two board channels count my
// pieces and the opponent's, whichever colour I am.
func TestPlanesAreRelativeToTheViewer(t *testing.T) {
	g := load(t, "tic-tac-toe")
	res := playAll(t, g, "a3", "b2", "c1") // x at a3,c1; o at b2
	for seat, want := range map[int][2]int{1: {2, 1}, 2: {1, 2}} {
		p, err := state.EncodePlanes(g, res.NewState, seat, res.NextTurnSeat, state.PlaneOptions{})
		if err != nil {
			t.Fatal(err)
		}
		cells := p.Shape[1] * p.Shape[2]
		count := func(ch int) int {
			n := 0
			for _, v := range p.Data[ch*cells : (ch+1)*cells] {
				if v == 1 {
					n++
				}
			}
			return n
		}
		if p.Channels[0] != "top/me/mark" || p.Channels[1] != "top/opp/mark" {
			t.Fatalf("channels = %v", p.Channels[:2])
		}
		if got := [2]int{count(0), count(1)}; got != want {
			t.Errorf("seat %d sees me/opp = %v, want %v", seat, got, want)
		}
	}
}
