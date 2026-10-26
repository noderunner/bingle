package classics_test

import (
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

func TestHexHexGeometry(t *testing.T) {
	g, err := board.GenerateHexHex(5)
	if err != nil {
		t.Fatal(err)
	}
	if g.Len() != 61 || g.Width() != 9 || g.Height() != 9 || !g.IsHex() {
		t.Fatalf("size-5 hexhex: len %d, %dx%d hex=%v", g.Len(), g.Width(), g.Height(), g.IsHex())
	}
	deg := map[int]int{}
	for _, id := range g.Positions() {
		deg[len(g.Neighbors(id))]++
	}
	// 6 corners (3 neighbours), 18 other rim cells (4), 37 interior (6).
	if deg[3] != 6 || deg[4] != 18 || deg[6] != 37 {
		t.Errorf("degree histogram = %v", deg)
	}
	count := func(name string) int { ids, _ := g.Region(name); return len(ids) }
	if count("edge") != 24 || count("corners") != 6 {
		t.Errorf("edge %d corners %d", count("edge"), count("corners"))
	}
	for _, side := range []string{"top", "bottom", "upper-left", "lower-left", "upper-right", "lower-right"} {
		if count(side) != 3 {
			t.Errorf("side %s has %d cells, want 3 (corners excluded)", side, count(side))
		}
	}
	for i := 0; i < 6; i++ {
		ids, ok := g.Region("corner-" + string(rune('0'+i)))
		if !ok || len(ids) != 1 {
			t.Errorf("corner-%d = %v", i, ids)
		}
	}
	if _, err := board.GenerateHexHex(1); err == nil {
		t.Error("size 1 should be rejected")
	}
}

func place(t testing.TB, g *catalog.CompiledGame, st *state.BoardState, seat int, cell string) *state.MoveResult {
	t.Helper()
	mv := mustParse(t, g, cell)
	res, err := state.Apply(g, st, seat, mv)
	if err != nil {
		t.Fatalf("place %s: %v", cell, err)
	}
	return res
}

func withPieces(g *catalog.CompiledGame, color string, cells ...string) *state.BoardState {
	st := state.InitialState(g)
	for _, c := range cells {
		st.Set(boardID(c), state.Occupant{Piece: "piece", Color: color})
	}
	return st
}

func TestYavalathFourWinsThreeLoses(t *testing.T) {
	g := load(t, "yavalath")

	// Three in a row loses for the one who makes it.
	st := withPieces(g, "white", "a5", "b5")
	res := place(t, g, st, 1, "c5")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 2 || res.EndReason != "three-in-a-row" {
		t.Errorf("three: ended=%v winner=%v reason=%q, want seat 2 by three-in-a-row", res.GameEnded, res.WinnerSeat, res.EndReason)
	}

	// Four wins — even though the four contains threes.
	st = withPieces(g, "white", "a5", "b5", "d5")
	res = place(t, g, st, 1, "c5")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != "four-in-a-row" {
		t.Errorf("four: ended=%v winner=%v reason=%q, want seat 1 by four-in-a-row", res.GameEnded, res.WinnerSeat, res.EndReason)
	}

	// The third hex axis counts: a9 b8 c7 run along (1,-1).
	st = withPieces(g, "black", "a9", "b8")
	res = place(t, g, st, 2, "c7")
	if !res.GameEnded || *res.WinnerSeat != 1 {
		t.Errorf("diagonal three: ended=%v winner=%v", res.GameEnded, res.WinnerSeat)
	}

	// The (1,1) offset is two cells apart on a hex board, not a neighbour.
	st = withPieces(g, "white", "e1", "f2")
	res = place(t, g, st, 1, "g3")
	if res.GameEnded {
		t.Error("e1, f2, g3 is not a line on a hex board")
	}

	// Two in a row and an unrelated stone is just play.
	st = withPieces(g, "white", "a5")
	if res := place(t, g, st, 1, "b5"); res.GameEnded {
		t.Error("two in a row must not end the game")
	}
}

func TestYavalathRandomGamesTerminate(t *testing.T) {
	g := load(t, "yavalath")
	rng := rand.New(rand.NewSource(3))
	for n := 0; n < 40; n++ {
		st, seat := state.InitialState(g), 1
		ended := false
		for ply := 0; ply < 80 && !ended; ply++ {
			legal := state.LegalMoves(g, st, seat)
			if len(legal) == 0 {
				break
			}
			res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
			if err != nil {
				t.Fatal(err)
			}
			ended = res.GameEnded
			st, seat = res.NewState, res.NextTurnSeat
		}
		if !ended {
			t.Fatalf("game %d did not end within 80 plies (61 cells)", n)
		}
	}
}

func TestHavannahWinningShapes(t *testing.T) {
	g := load(t, "havannah-4") // base 4: 37 cells, coordinates a1..g7

	cases := []struct {
		name   string
		pre    []string
		last   string
		reason string
	}{
		// Six pieces around d4; the centre stays empty.
		{"ring", []string{"e4", "c4", "d5", "d3", "e3"}, "c5", "ring"},
		// A chain along the top rim joins corners d1 and g1.
		{"bridge", []string{"d1", "e1", "f1"}, "g1", "bridge"},
		// A Y from the centre to the top, lower-left and upper-right sides.
		{"fork", []string{"d4", "d3", "e2", "e1", "c4", "b5", "a5", "e4", "f4"}, "g3", "fork"},
	}
	for _, tc := range cases {
		st := withPieces(g, "white", tc.pre...)
		// Nothing has been decided before the last stone.
		out, err := state.Evaluate(g, st, 1)
		if err != nil {
			t.Fatal(err)
		}
		if out.Ended {
			t.Errorf("%s: game already ended before the last stone (%s)", tc.name, out.Reason)
		}
		res := place(t, g, st, 1, tc.last)
		if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != tc.reason {
			t.Errorf("%s: ended=%v winner=%v reason=%q, want seat 1 by %s", tc.name, res.GameEnded, res.WinnerSeat, res.EndReason, tc.reason)
		}
	}

	// A group touching only two sides is not a fork, and a group touching a
	// corner and a side is not a bridge.
	st := withPieces(g, "white", "e1", "f1", "g2", "c4", "b5")
	if out, _ := state.Evaluate(g, st, 1); out.Ended {
		t.Errorf("partial shapes ended the game: %s", out.Reason)
	}
}

func TestHavannahSurroundedOpponentCountsAsARing(t *testing.T) {
	g := load(t, "havannah-4")
	st := withPieces(g, "white", "e4", "c4", "d5", "d3", "e3")
	st.Set(boardID("d4"), state.Occupant{Piece: "piece", Color: "black"}) // enclosed
	res := place(t, g, st, 1, "c5")
	if !res.GameEnded || res.EndReason != "ring" || *res.WinnerSeat != 1 {
		t.Errorf("ring around an enemy stone: ended=%v reason=%q", res.GameEnded, res.EndReason)
	}
}

func TestHavannahRandomGamesTerminateConsistently(t *testing.T) {
	g := load(t, "havannah-4")
	rng := rand.New(rand.NewSource(5))
	for n := 0; n < 30; n++ {
		st, seat := state.InitialState(g), 1
		for ply := 0; ply < 60; ply++ {
			legal := state.LegalMoves(g, st, seat)
			if len(legal) == 0 {
				break
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
