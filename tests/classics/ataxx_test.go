package classics_test

import (
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

func TestAtaxxOpeningMoves(t *testing.T) {
	g := load(t, "ataxx")
	moves := state.LegalMoves(g, state.InitialState(g), 1)
	// Two red blobs in opposite corners: each has 3 clones and 5 jumps.
	if len(moves) != 2*(3+5) {
		t.Errorf("Red's opening moves = %d, want 16", len(moves))
	}
}

func TestAtaxxCloneJumpAndInfection(t *testing.T) {
	g := load(t, "ataxx")
	st := state.InitialState(g)

	// Clone a1 -> b2: both a1 and b2 are red afterwards.
	mv, err := state.ParseMove(g, "a1-b2")
	if err != nil {
		t.Fatal(err)
	}
	res, err := state.Apply(g, st, 1, mv)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a1", "b2", "g7"} {
		if top, ok := res.NewState.Get(boardID(id)); !ok || top.Color != "red" {
			t.Errorf("%s should be red after the clone, got %+v ok=%v", id, top, ok)
		}
	}

	// Jump a1 -> c3 leaves a1 empty.
	jm, _ := state.ParseMove(g, "a1-c3")
	jr, err := state.Apply(g, st, 1, jm)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := jr.NewState.Get(boardID("a1")); ok {
		t.Error("a jump should vacate its origin")
	}

	// Three squares is too far; occupied squares are refused.
	for _, bad := range []string{"a1-d4", "a1-a7", "a1-a1"} {
		m, err := state.ParseMove(g, bad)
		if err != nil {
			continue
		}
		if _, err := state.Apply(g, st, 1, m); err == nil {
			t.Errorf("%s was accepted", bad)
		}
	}

	// Infection: a blue blob next to the landing square turns red; blobs
	// two away or already red are untouched.
	pos := state.NewBoardState()
	put := func(id, color string) { pos.Set(boardID(id), state.Occupant{Piece: "blob", Color: color}) }
	put("a1", "red")
	put("b3", "blue") // adjacent to b2 (landing)
	put("c3", "blue") // adjacent to b2 diagonally
	put("d4", "blue") // two away from b2
	put("g7", "blue")
	cl, _ := state.ParseMove(g, "a1-b2")
	r2, err := state.Apply(g, pos, 1, cl)
	if err != nil {
		t.Fatal(err)
	}
	colors := map[string]string{}
	for _, id := range []string{"b3", "c3", "d4", "g7"} {
		top, _ := r2.NewState.Get(boardID(id))
		colors[id] = top.Color
	}
	if colors["b3"] != "red" || colors["c3"] != "red" || colors["d4"] != "blue" || colors["g7"] != "blue" {
		t.Errorf("infection result = %v", colors)
	}
}

func TestAtaxxPassesAndCounts(t *testing.T) {
	g := load(t, "ataxx")
	rng := rand.New(rand.NewSource(8))
	finished := 0
	for n := 0; n < 12; n++ {
		st, seat := state.InitialState(g), 1
		for ply := 0; ply < 300; ply++ {
			legal := state.LegalMoves(g, st, seat)
			if len(legal) == 0 {
				t.Fatalf("game %d: seat %d has no move yet the game continues", n, seat)
			}
			res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
			if err != nil {
				t.Fatal(err)
			}
			if res.GameEnded {
				finished++
				// Winner has the most blobs.
				counts := map[string]int{}
				for _, id := range res.NewState.Occupied() {
					top, _ := res.NewState.Get(id)
					counts[top.Color]++
				}
				if res.WinnerSeat != nil {
					w := "red"
					if *res.WinnerSeat == 2 {
						w = "blue"
					}
					o := "blue"
					if w == "blue" {
						o = "red"
					}
					if counts[w] <= counts[o] {
						t.Fatalf("game %d: winner %s has %d vs %d", n, w, counts[w], counts[o])
					}
				} else if counts["red"] != counts["blue"] {
					t.Fatalf("game %d: draw with %d vs %d", n, counts["red"], counts["blue"])
				}
				break
			}
			st, seat = res.NewState, res.NextTurnSeat
		}
	}
	if finished != 12 {
		t.Errorf("only %d of 12 random games finished", finished)
	}
}

func TestAtaxxEnumerationMatchesValidation(t *testing.T) {
	g := load(t, "ataxx")
	rng := rand.New(rand.NewSource(2))
	st, seat := state.InitialState(g), 1
	ids := g.Board.Positions()
	for ply := 0; ply < 20; ply++ {
		legal := state.LegalMoves(g, st, seat)
		if len(legal) == 0 {
			break
		}
		listed := map[string]bool{}
		for _, mv := range legal {
			listed[state.FormatMove(g, mv)] = true
		}
		accepted := 0
		for _, from := range ids {
			for _, to := range ids {
				mv := state.Move{Action: "move", From: from, To: to}
				if _, err := state.Apply(g, st, seat, mv); err == nil {
					accepted++
					if !listed[state.FormatMove(g, mv)] {
						t.Fatalf("ply %d: %s accepted but not listed", ply, state.FormatMove(g, mv))
					}
				}
			}
		}
		if accepted != len(legal) {
			t.Fatalf("ply %d: validation accepted %d, enumeration listed %d", ply, accepted, len(legal))
		}
		res, _ := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
		st, seat = res.NewState, res.NextTurnSeat
	}
}
