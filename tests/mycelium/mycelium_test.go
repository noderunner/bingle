// Package mycelium_test checks Mycelium: placement, the confluence's order of
// spreads, spore and hyphae spreads, the end of a confluence, and the bots'
// guide.
//
// Cells are a letter (the column) and a number (the row). Row 1 runs e1–i1 and
// row 5 a5–i5; e5 is the centre. A cell touches the cells beside it in its
// row and column, and the cell one column right and one row down (f4 from e5)
// and one column left and one row up (d6 from e5).
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

// build sets out black and green nodes with the given payload.
func build(t *testing.T, g *catalog.CompiledGame, black, green []board.PositionID, d mycelium.Data) *state.BoardState {
	t.Helper()
	st := state.NewBoardState()
	for _, id := range black {
		st.Set(id, state.Occupant{Piece: "black", Color: "black"})
	}
	for _, id := range green {
		st.Set(id, state.Occupant{Piece: "green", Color: "green"})
	}
	st.SetExt(&d)
	if err := state.Validate(g, st); err != nil {
		t.Fatalf("test position: %v", err)
	}
	return st
}

// confluence is a confluence in phase at initiative, started by initiator,
// with seat to choose.
func confluence(seat, initiator int, phase string, initiative int) mycelium.Data {
	return mycelium.Data{Turn: seat, Confluence: &mycelium.Confluence{Initiator: initiator, Phase: phase, Initiative: initiative}}
}

func play(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, text string) (*state.BoardState, *state.MoveResult) {
	t.Helper()
	rs := mycelium.New()
	mv, err := rs.Parse(g, text)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rs.Apply(g, st, rs.ToMove(g, st), mv)
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	if err := state.Validate(g, res.NewState); err != nil {
		t.Fatalf("after %s: %v", text, err)
	}
	return res.NewState, res
}

func data(st *state.BoardState) *mycelium.Data { d, _ := st.Ext().(*mycelium.Data); return d }

func legalTexts(g *catalog.CompiledGame, st *state.BoardState) []string {
	rs := mycelium.New()
	var out []string
	for _, mv := range rs.Legal(g, st, rs.ToMove(g, st)) {
		out = append(out, rs.Format(g, mv))
	}
	return out
}

func wantLegal(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, want ...string) {
	t.Helper()
	got := legalTexts(g, st)
	if len(got) != len(want) {
		t.Fatalf("legal moves %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("legal moves %v, want %v", got, want)
		}
	}
}

func wantColor(t *testing.T, st *state.BoardState, color string, cells ...board.PositionID) {
	t.Helper()
	for _, id := range cells {
		occ, ok := st.Get(id)
		switch {
		case color == "" && ok:
			t.Errorf("%s holds a %s node, want it empty", id, occ.Color)
		case color != "" && (!ok || occ.Color != color):
			t.Errorf("%s holds %q, want a %s node", id, occ.Color, color)
		}
	}
}

func TestBoardHas61Cells(t *testing.T) {
	if n := len(game(t).Board.Positions()); n != 61 {
		t.Errorf("%d cells, want 61", n)
	}
}

func TestPlacementsAlternate(t *testing.T) {
	g := game(t)
	st, _ := mycelium.New().Start(g, 0)
	st, res := play(t, g, st, "black@d4")
	if res.NextTurnSeat != 2 || data(st).Confluence != nil {
		t.Fatalf("after a placement off the centre: next %d, confluence %v", res.NextTurnSeat, data(st).Confluence)
	}
	if _, err := mycelium.New().Parse(g, "black@d4"); err != nil {
		t.Fatal(err)
	}
	_, res = play(t, g, st, "green@f6")
	if res.NextTurnSeat != 1 {
		t.Fatalf("turn should pass back to Black, got %d", res.NextTurnSeat)
	}
}

// Taking the centre with nothing else on the board starts a confluence with
// nothing to spread: the centre is cleared, neither colony has a node, and the
// player who took the centre loses.
func TestOpeningOnTheCentreLoses(t *testing.T) {
	g := game(t)
	st, _ := mycelium.New().Start(g, 0)
	_, res := play(t, g, st, "black@e5")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 2 {
		t.Fatalf("Green should win when Black opens on the centre, got %+v", res)
	}
}

// The spreading spore stays put and the node it sees changes colour. The
// converted node is a new cluster and spreads in turn; a cluster that has
// spread does not spread again, so a player with nothing left lets the other
// spread twice running.
func TestSporeSpreadsConvertAndChain(t *testing.T) {
	g := game(t)
	// Column e: green e2, then black e5 (the centre, Black took it) and e9.
	st := build(t, g, []board.PositionID{"e5", "e9"}, []board.PositionID{"e2"}, confluence(2, 1, "spore", 1))
	wantLegal(t, g, st, "spore e2 e5")

	st, _ = play(t, g, st, "spore e2 e5")
	wantColor(t, st, "green", "e2", "e5")
	// Black chooses next; the new green node on e5 now sees e9 as well.
	wantLegal(t, g, st, "spore e9 e5")

	st, _ = play(t, g, st, "spore e9 e5")
	wantColor(t, st, "black", "e5", "e9")
	// Green's e2 has spread; Black's new node on e5 has not, and sees e2.
	if seat := mycelium.New().ToMove(g, st); seat != 1 {
		t.Fatalf("Black should spread again, %d is to move", seat)
	}
	wantLegal(t, g, st, "spore e5 e2")

	_, res := play(t, g, st, "spore e5 e2")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 {
		t.Fatalf("Black should win with Green wiped out, got %+v", res)
	}
}

// A confluence that wipes out one colony ends the game for it, even though
// the winner's node on the centre is removed.
func TestConfluenceWipesOutAColony(t *testing.T) {
	g := game(t)
	st := build(t, g, []board.PositionID{"e5"}, []board.PositionID{"e2"}, confluence(2, 1, "spore", 1))
	st, res := play(t, g, st, "spore e2 e5")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 2 || res.EndDetail != "0–1 nodes" {
		t.Fatalf("Green should win 1 node to 0, got %+v", res)
	}
	wantColor(t, st, "", "e5")
	wantColor(t, st, "green", "e2")
}

// A hyphae spread converts one node at a time along a winding chain, from the
// end of a bent strand, as many as the strand is long.
func TestHyphaeSpreadWinds(t *testing.T) {
	g := game(t)
	// Black's strand a5–a6–b6 bends; a5 touches green b4, and green winds on
	// b4–c3–d3. Green took the centre, so Black chooses first.
	black := []board.PositionID{"a5", "a6", "b6"}
	green := []board.PositionID{"b4", "c3", "d3", "e5"}
	st := build(t, g, black, green, confluence(1, 2, "hyphae", 3))
	wantLegal(t, g, st, "hyphae a5 b4")
	st, _ = play(t, g, st, "hyphae a5 b4")
	wantLegal(t, g, st, "hyphae c3", "done")
	st, _ = play(t, g, st, "hyphae c3")
	wantLegal(t, g, st, "hyphae d3", "done")
	st, _ = play(t, g, st, "hyphae d3")
	// Green is left with the centre alone, which is removed: Black wins.
	if d := data(st); d.Turn != 0 || d.Winner != 1 {
		t.Fatalf("Black should have won, got turn %d winner %d", d.Turn, d.Winner)
	}
	wantColor(t, st, "black", "b4", "c3", "d3")
}

// A spread is as long as the longest strand ending where it starts, which can
// be shorter than the cluster.
func TestHyphaeSpreadIsAsLongAsTheStrand(t *testing.T) {
	g := game(t)
	// Black's star: c7 with d7, c6 and b8 around it, none touching another, so
	// its longest strand from d7 is three nodes (d7–c7–c6) of the four. Green's
	// e7–f7–g7–g6 touches d7.
	black := []board.PositionID{"c7", "d7", "c6", "b8"}
	green := []board.PositionID{"e7", "f7", "g7", "g6", "e5"}
	st := build(t, g, black, green, confluence(1, 2, "hyphae", 4))
	wantLegal(t, g, st, "hyphae d7 e7")
	st, _ = play(t, g, st, "hyphae d7 e7")
	st, _ = play(t, g, st, "hyphae f7")
	st, _ = play(t, g, st, "hyphae g7")
	wantColor(t, st, "black", "e7", "f7", "g7")
	wantColor(t, st, "green", "g6")
	// The confluence is over: the centre is cleared and Black, who did not take
	// it, places next.
	if d := data(st); d.Confluence != nil || d.Turn != 1 {
		t.Fatalf("normal play should resume with Black, got turn %d confluence %+v", d.Turn, d.Confluence)
	}
	wantColor(t, st, "", "e5")
}

func TestHyphaeSpreadMayStopEarly(t *testing.T) {
	g := game(t)
	black := []board.PositionID{"a5", "a6", "b6"}
	green := []board.PositionID{"b4", "c3", "d3", "e5"}
	st := build(t, g, black, green, confluence(1, 2, "hyphae", 3))
	st, _ = play(t, g, st, "hyphae a5 b4")
	st, _ = play(t, g, st, "done")
	wantColor(t, st, "green", "c3", "d3")
	if d := data(st); d.Confluence != nil || d.Turn != 1 {
		t.Fatalf("the confluence should be over with Black to place, got turn %d confluence %+v", d.Turn, d.Confluence)
	}
}

func TestValidateRejectsAStalledConfluence(t *testing.T) {
	g := game(t)
	st := state.NewBoardState()
	st.Set("e5", state.Occupant{Piece: "black", Color: "black"})
	d := confluence(2, 1, "spore", 1)
	st.SetExt(&d)
	if err := state.Validate(g, st); err == nil {
		t.Fatal("a confluence where the seat to move has no spread should not validate")
	}
}

func TestBotsDoNotThrowTheGameOnTheCentre(t *testing.T) {
	g := game(t)
	rs := mycelium.New()
	st, _ := rs.Start(g, 0)
	for _, mv := range state.GuidedMoves(g, st, 1, rs.Legal(g, st, 1)) {
		if mv.To == "e5" {
			t.Fatal("the guide offers the opening on the centre, which loses at once")
		}
	}
}

// Random games, bots guided and not, play through confluences: every move
// round-trips through its notation, no two legal moves share notation or
// steps, every position validates, and the games end.
func TestRandomGames(t *testing.T) {
	g := game(t)
	rs := mycelium.New()
	rng := rand.New(rand.NewSource(7))
	ended, confluences := 0, 0
	for n := 0; n < 40; n++ {
		st, _ := rs.Start(g, 0)
		guided := n%2 == 0
		for ply := 0; ply < 600; ply++ {
			seat := rs.ToMove(g, st)
			legal := rs.Legal(g, st, seat)
			if len(legal) == 0 {
				t.Fatalf("no legal move for seat %d", seat)
			}
			texts, steps := map[string]bool{}, map[string]bool{}
			for _, mv := range legal {
				text := rs.Format(g, mv)
				key := ""
				for _, s := range rs.Steps(g, mv) {
					if s.Kind == state.StepPos && !g.Board.HasPosition(board.PositionID(s.Value)) {
						t.Fatalf("%s: step %q is not a cell", text, s.Value)
					}
					key += s.Kind + ":" + s.Value + " "
				}
				if texts[text] || steps[key] {
					t.Fatalf("%s is listed twice", text)
				}
				texts[text], steps[key] = true, true
			}
			if guided {
				legal = state.GuidedMoves(g, st, seat, legal)
			}
			mv := legal[rng.Intn(len(legal))]
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
			if data(st).Confluence == nil && data(res.NewState).Confluence != nil {
				confluences++
			}
			st = res.NewState
			if res.GameEnded {
				ended++
				break
			}
		}
	}
	if ended < 40 || confluences < 20 {
		t.Errorf("%d of 40 games ended, with %d confluences resolved by choice", ended, confluences)
	}
}

// The length of a hyphae spread is the longest strand ending at its start,
// found by brute force here: a random black cluster among green nodes on every
// other cell.
func TestHyphaeSpreadLengthMatchesBruteForce(t *testing.T) {
	g := game(t)
	rs := mycelium.New()
	nbrs := func(id board.PositionID) []board.PositionID {
		c, _ := g.Board.Coord(id)
		var out []board.PositionID
		for _, d := range [6][2]int{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}} {
			if n, ok := g.Board.At(board.Coord{X: c.X + d[0], Y: c.Y + d[1]}); ok {
				out = append(out, n)
			}
		}
		return out
	}
	// longest is the longest path from id through the cells of in.
	var longest func(id board.PositionID, in, on map[board.PositionID]bool) int
	longest = func(id board.PositionID, in, on map[board.PositionID]bool) int {
		best := 1
		on[id] = true
		for _, n := range nbrs(id) {
			if in[n] && !on[n] {
				best = max(best, 1+longest(n, in, on))
			}
		}
		on[id] = false
		return best
	}
	// size is the number of cells of in connected to id.
	size := func(id board.PositionID, in map[board.PositionID]bool) int {
		seen, queue := map[board.PositionID]bool{id: true}, []board.PositionID{id}
		for k := 0; k < len(queue); k++ {
			for _, n := range nbrs(queue[k]) {
				if in[n] && !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
		return len(queue)
	}
	rng := rand.New(rand.NewSource(11))
	cells := g.Board.Positions()
	for trial := 0; trial < 300; trial++ {
		isBlack := map[board.PositionID]bool{}
		blackCells := []board.PositionID{cells[rng.Intn(len(cells))]}
		isBlack[blackCells[0]] = true
		for want := 2 + rng.Intn(9); len(blackCells) < want; {
			ns := nbrs(blackCells[rng.Intn(len(blackCells))])
			if n := ns[rng.Intn(len(ns))]; !isBlack[n] {
				isBlack[n] = true
				blackCells = append(blackCells, n)
			}
		}
		isGreen := map[board.PositionID]bool{}
		var greenCells []board.PositionID
		for _, id := range cells {
			if !isBlack[id] {
				isGreen[id] = true
				greenCells = append(greenCells, id)
			}
		}
		st := build(t, g, blackCells, greenCells, confluence(1, 2, "hyphae", len(blackCells)))
		mv := rs.Legal(g, st, 1)[0]
		want := min(longest(mv.From, isBlack, map[board.PositionID]bool{}), size(mv.To, isGreen))
		st, _ = play(t, g, st, rs.Format(g, mv))
		// The spread goes on while the strand allows and a green node touches
		// the one just converted.
		touches := false
		for _, n := range nbrs(mv.To) {
			touches = touches || (isGreen[n] && n != mv.To)
		}
		cf := data(st).Confluence
		switch {
		case want > 1 && touches:
			if cf == nil || cf.Head != string(mv.To) || cf.Left != want-1 {
				t.Fatalf("trial %d, %s: %+v, want %d more conversions", trial, rs.Format(g, mv), cf, want-1)
			}
		case cf != nil && cf.Head != "":
			t.Fatalf("trial %d, %s: the spread should be over, got %+v", trial, rs.Format(g, mv), cf)
		}
	}
}
