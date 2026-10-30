// Package mycelium_test checks Mycelium: normal placement, the confluence turn,
// spore and hyphae spreads, and end-of-confluence scoring.
package mycelium_test

import (
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/mycelium"
)

func game(t testing.TB) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(mycelium.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get("mycelium")
	if !ok {
		t.Fatal("mycelium is not registered")
	}
	return g
}

var dirs = [6][2]int{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}}

func neighbours(g *catalog.CompiledGame, id board.PositionID) []board.PositionID {
	c, _ := g.Board.Coord(id)
	var out []board.PositionID
	for _, d := range dirs {
		if n, ok := g.Board.At(board.Coord{X: c.X + d[0], Y: c.Y + d[1]}); ok {
			out = append(out, n)
		}
	}
	return out
}

func centre(g *catalog.CompiledGame) board.PositionID {
	if id, ok := g.Board.At(board.Coord{X: g.Board.Width() / 2, Y: g.Board.Height() / 2}); ok {
		return id
	}
	panic("no centre cell")
}

func build(g *catalog.CompiledGame, stones map[board.PositionID]string, d mycelium.Data) *state.BoardState {
	st := state.NewBoardState()
	for id, c := range stones {
		st.Set(id, state.Occupant{Piece: c, Color: c})
	}
	st.SetExt(&d)
	return st
}

func play(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, seat int, text string) (*state.BoardState, *state.MoveResult) {
	t.Helper()
	rs := mycelium.New()
	mv, err := rs.Parse(g, text)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rs.Apply(g, st, seat, mv)
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	return res.NewState, res
}

func data(st *state.BoardState) *mycelium.Data { d, _ := st.Ext().(*mycelium.Data); return d }

func TestBoardHas61Cells(t *testing.T) {
	g := game(t)
	if n := len(g.Board.Positions()); n != 61 {
		t.Errorf("%d cells, want 61", n)
	}
}

func TestBlackOpensAndTurnsAlternate(t *testing.T) {
	g := game(t)
	rs := mycelium.New()
	st, _ := rs.Start(g, 0)
	if rs.ToMove(g, st) != 1 {
		t.Fatal("black moves first")
	}
	c := centre(g)
	nb := neighbours(g, c)
	st, res := play(t, g, st, 1, "black@"+string(nb[0]))
	if res.NextTurnSeat != 2 {
		t.Fatalf("turn should pass to green, got %d", res.NextTurnSeat)
	}
	_, res = play(t, g, st, 2, "green@"+string(nb[1]))
	if res.NextTurnSeat != 1 {
		t.Fatalf("turn should pass back to black, got %d", res.NextTurnSeat)
	}
}

func TestCentrePlacementTriggersConfluence(t *testing.T) {
	g := game(t)
	rs := mycelium.New()
	st, _ := rs.Start(g, 0)
	c := centre(g)
	nb := neighbours(g, c)
	// Make a non-centre move first so the confluence does not end immediately.
	st, _ = play(t, g, st, 1, "black@"+string(nb[0]))
	st, _ = play(t, g, st, 2, "green@"+string(nb[1]))
	st, _ = play(t, g, st, 1, "black@"+string(c))
	if data(st).Confluence == nil {
		t.Fatal("centre placement should trigger a confluence")
	}
	if data(st).Confluence.Initiator != 1 {
		t.Errorf("initiator %d, want 1", data(st).Confluence.Initiator)
	}
	if rs.ToMove(g, st) != 2 {
		t.Fatalf("green should choose first in the confluence, got %d", rs.ToMove(g, st))
	}
	if data(st).Confluence.Phase != "spore" && data(st).Confluence.Phase != "hyphae" {
		t.Errorf("phase %q, want spore or hyphae", data(st).Confluence.Phase)
	}
}

func TestSporeSpreadConvertsEnemyNode(t *testing.T) {
	g := game(t)
	c := centre(g)
	// Green has a size-1 spore cluster at d3 with line of sight to a black node
	// at d5: d3 -> d4 (empty) -> d5 (black). The confluence is in the spore phase
	// at initiative 1, with green to move first.
	stones := map[board.PositionID]string{
		c:    "black",
		"d5": "black",
		"d3": "green",
	}
	st := build(g, stones, mycelium.Data{Turn: 2, Confluence: &mycelium.Confluence{
		Initiator: 1, Phase: "spore", Initiative: 1, NextSeat: 2, Spread: map[string]bool{},
	}})
	rs := mycelium.New()
	legal := rs.Legal(g, st, 2)
	if len(legal) == 0 {
		t.Fatal("green should have a spore spread")
	}
	found := false
	for _, mv := range legal {
		if rs.Format(g, mv) == "spore d3 d5" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected spore d3 d5 among %v", legal)
	}
	st, _ = play(t, g, st, 2, "spore d3 d5")
	if _, ok := st.Get("d3"); ok {
		t.Error("the source node should have left d3")
	}
	occ, ok := st.Get("d5")
	if !ok || occ.Color != "green" {
		t.Errorf("d5 should hold the converted green node, got %v", occ)
	}
}

func TestHyphaeSpreadConvertsAdjacentEnemyNodes(t *testing.T) {
	g := game(t)
	c := centre(g)
	// Black has a size-2 hyphae cluster d5-e5; the strand d5-e5 can convert
	// green nodes f5 and g5 along the (1,0) direction.
	stones := map[board.PositionID]string{
		c:    "black",
		"d5": "black",
		"e5": "black",
		"f5": "green",
		"g5": "green",
	}
	st := build(g, stones, mycelium.Data{Turn: 1, Confluence: &mycelium.Confluence{
		Initiator: 2, Phase: "hyphae", Initiative: 2, NextSeat: 1, Spread: map[string]bool{},
	}})
	rs := mycelium.New()
	legal := rs.Legal(g, st, 1)
	want := "hyphae d5-e5 f5 g5"
	found := false
	for _, mv := range legal {
		if rs.Format(g, mv) == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s among %v", want, legal)
	}
	st, _ = play(t, g, st, 1, want)
	// The targets are converted; the centre is cleared when the confluence ends.
	for _, id := range []board.PositionID{"d5", "f5", "g5"} {
		occ, ok := st.Get(id)
		if !ok || occ.Color != "black" {
			t.Errorf("%s should be black, got %v", id, occ)
		}
	}
}

func TestConfluenceEliminationEndsGame(t *testing.T) {
	g := game(t)
	c := centre(g)
	// Black initiates the confluence with only the centre node; green has a
	// size-1 spore cluster at c7 that can leap over d6 to the centre and
	// eliminate black's only node. The centre is removed after the spread,
	// leaving black with nothing.
	stones := map[board.PositionID]string{
		c:    "black",
		"c7": "green",
	}
	st := build(g, stones, mycelium.Data{Turn: 2, Confluence: &mycelium.Confluence{
		Initiator: 1, Phase: "spore", Initiative: 1, NextSeat: 2, Spread: map[string]bool{},
	}})
	_, res := play(t, g, st, 2, "spore c7 e5")
	if !res.GameEnded {
		t.Fatal("the game should end after black is eliminated")
	}
	if res.WinnerSeat == nil || *res.WinnerSeat != 2 {
		t.Fatalf("green should win, got %+v", res)
	}
}

func TestConfluenceDrawWhenBothColoniesWiped(t *testing.T) {
	g := game(t)
	c := centre(g)
	// Black initiates with the centre; green has a spore cluster that can leap
	// to the centre, but the centre removal then also removes the just-moved
	// green node, leaving both sides empty. The initiator loses, so green wins.
	stones := map[board.PositionID]string{
		c:    "black",
		"c7": "green",
	}
	st := build(g, stones, mycelium.Data{Turn: 2, Confluence: &mycelium.Confluence{
		Initiator: 1, Phase: "spore", Initiative: 1, NextSeat: 2, Spread: map[string]bool{},
	}})
	_, res := play(t, g, st, 2, "spore c7 e5")
	if res.WinnerSeat == nil || *res.WinnerSeat != 2 {
		t.Fatalf("non-initiator should win when both colonies are wiped, got %+v", res)
	}
}

func TestRandomGamesAreValidAndEnd(t *testing.T) {
	g := game(t)
	rs := mycelium.New()
	rng := rand.New(rand.NewSource(7))
	ended := 0
	for n := 0; n < 30; n++ {
		st, _ := rs.Start(g, 0)
		seat := 1
		for ply := 0; ply < 700; ply++ {
			legal := rs.Legal(g, st, seat)
			if len(legal) == 0 {
				t.Fatalf("no legal move for seat %d", seat)
			}
			guided := state.GuidedMoves(g, st, seat, legal)
			mv := guided[rng.Intn(len(guided))]
			text := rs.Format(g, mv)
			back, err := rs.Parse(g, text)
			if err != nil || rs.Format(g, back) != text {
				t.Fatalf("round trip %q: %v", text, err)
			}
			res, err := rs.Apply(g, st, seat, back)
			if err != nil {
				t.Fatalf("ply %d %s: %v", ply, text, err)
			}
			if err := state.Validate(g, res.NewState); err != nil {
				t.Fatal(err)
			}
			st, seat = res.NewState, res.NextTurnSeat
			if res.GameEnded {
				ended++
				break
			}
		}
	}
	if ended < 25 {
		t.Errorf("only %d of 30 games ended", ended)
	}
}
