// Package hive_test checks Hive's rules: placement, the One Hive rule, the
// movement of all eight insects (with the three expansion insects), the
// Pillbug's throw, and the ways a game ends.
package hive_test

import (
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/hive"
)

func game(t testing.TB, id string) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(hive.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("%s is not registered", id)
	}
	return g
}

// The field has size 10: the middle cell is j10; the neighbours of (q,r) are
// (q±1,r), (q,r±1), (q+1,r-1) and (q-1,r+1). cell(dq, dr) names the cell that
// far from the middle.
func cell(dq, dr int) string { return string(rune('a'+9+dq)) + itoa(10+dr) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// tiles describe a position: "w:ant" at a cell, bottom first for stacks.
type tile struct {
	at   string
	seat int
	kind string
}

func build(t testing.TB, g *catalog.CompiledGame, turn int, d hive.Data, tiles ...tile) *state.BoardState {
	t.Helper()
	st := state.NewBoardState()
	color := map[int]string{1: "white", 2: "black"}
	used := map[string]int{}
	for _, tl := range tiles {
		st.Push(board.PositionID(tl.at), state.Occupant{Piece: tl.kind, Color: color[tl.seat]})
		used[color[tl.seat]+"/"+tl.kind]++
	}
	for _, s := range []string{"white", "black"} {
		for pool, n := range g.Definition.Reserves {
			st.SetReserve(s, pool, n-used[s+"/"+pool])
		}
	}
	d.Turn = turn
	st.SetExt(&d)
	return st
}

func movesFrom(g *catalog.CompiledGame, st *state.BoardState, seat int, from string) []string {
	rs := hive.New()
	var out []string
	for _, mv := range rs.Legal(g, st, seat) {
		if mv.Action == "move" && string(mv.From) == from {
			out = append(out, string(mv.To))
		}
	}
	sort.Strings(out)
	return out
}

// notations lists the legal moves by their cells (kind@cell, from-to, pass),
// whatever the notation shown to players is.
func notations(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	var out []string
	for _, mv := range hive.New().Legal(g, st, seat) {
		switch mv.Action {
		case "place":
			out = append(out, mv.Piece+"@"+string(mv.To))
		case "move":
			out = append(out, string(mv.From)+"-"+string(mv.To))
		default:
			out = append(out, mv.Action)
		}
	}
	return out
}

// protocol lists the legal moves in the notation players see.
func protocol(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	rs := hive.New()
	var out []string
	for _, mv := range rs.Legal(g, st, seat) {
		out = append(out, rs.Format(g, mv))
	}
	return out
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func play(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, seat int, text string) (*state.BoardState, *state.MoveResult) {
	t.Helper()
	rs := hive.New()
	mv, err := rs.Parse(g, text)
	if err != nil {
		t.Fatalf("parse %q: %v", text, err)
	}
	res, err := rs.Apply(g, st, seat, mv)
	if err != nil {
		t.Fatalf("apply %q: %v", text, err)
	}
	return res.NewState, res
}

func sameSet(got []string, want ...string) bool {
	sort.Strings(want)
	return strings.Join(got, ",") == strings.Join(want, ",")
}

func TestOpeningPlacementAndTheEightTiles(t *testing.T) {
	g := game(t, "hive")
	rs := hive.New()
	st, err := rs.Start(g, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"queen", "beetle", "spider", "grasshopper", "ant", "mosquito", "ladybug", "pillbug"} {
		if !has(notations(g, st, 1), kind+"@j10") {
			t.Errorf("White cannot open with %s in the middle", kind)
		}
	}
	if n := len(notations(g, st, 1)); n != 8 {
		t.Errorf("White has %d openings, want 8 tiles on one cell", n)
	}
	if st.Reserve("white", "ant") != 3 || st.Reserve("black", "pillbug") != 1 {
		t.Errorf("supplies: %d ants, %d pillbugs", st.Reserve("white", "ant"), st.Reserve("black", "pillbug"))
	}
	st, _ = play(t, g, st, 1, "ant@j10")
	// Black's first tile touches White's: any of the six neighbours.
	if n := len(notations(g, st, 2)); n != 6*8 {
		t.Errorf("Black has %d replies, want 48", n)
	}
	st, _ = play(t, g, st, 2, "queen@k10")
	if st.Reserve("white", "ant") != 2 {
		t.Errorf("White's supply of ants is %d after one placed", st.Reserve("white", "ant"))
	}
	// White's second tile may not touch Black's queen.
	for _, m := range notations(g, st, 1) {
		if strings.HasPrefix(m, "ant@") {
			c := m[4:]
			for _, bad := range []string{cell(2, 0), cell(1, 1), cell(1, -1)} {
				if c == bad {
					t.Errorf("White placed next to a black tile: %s", m)
				}
			}
		}
	}
}

func TestBaseVariantHasNoExpansionTiles(t *testing.T) {
	g := game(t, "hive-base")
	st, _ := hive.New().Start(g, 1)
	for _, m := range notations(g, st, 1) {
		if strings.HasPrefix(m, "mosquito") || strings.HasPrefix(m, "ladybug") || strings.HasPrefix(m, "pillbug") {
			t.Errorf("the base game offers %s", m)
		}
	}
}

func TestQueenMustBePlacedByTheFourthTurn(t *testing.T) {
	g := game(t, "hive")
	d := hive.Data{Turns: [2]int{3, 3}}
	st := build(t, g, 1, d,
		tile{cell(0, 0), 1, "ant"}, tile{cell(-1, 0), 1, "spider"}, tile{cell(-2, 0), 1, "beetle"},
		tile{cell(1, 0), 2, "ant"}, tile{cell(2, 0), 2, "spider"}, tile{cell(3, 0), 2, "beetle"})
	for _, m := range notations(g, st, 1) {
		if !strings.HasPrefix(m, "queen@") {
			t.Errorf("fourth turn without a queen offers %s", m)
		}
	}
	if len(notations(g, st, 1)) == 0 {
		t.Fatal("no queen placements")
	}
}

func TestNothingMovesBeforeTheQueenIsDown(t *testing.T) {
	g := game(t, "hive")
	st := build(t, g, 1, hive.Data{Turns: [2]int{1, 1}},
		tile{cell(0, 0), 1, "ant"}, tile{cell(1, 0), 2, "ant"})
	for _, m := range notations(g, st, 1) {
		if strings.Contains(m, "-") {
			t.Errorf("a tile moved before the queen: %s", m)
		}
	}
}

func TestQueenSlidesOneSpaceKeepingTouch(t *testing.T) {
	g := game(t, "hive")
	// A row of three: white queen at the end can only slide round the row.
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 1, "queen"}, tile{cell(1, 0), 2, "ant"}, tile{cell(2, 0), 2, "queen"})
	got := movesFrom(g, st, 1, cell(0, 0))
	// Around the ant at (1,0): up to (1,-1) and down to (0,1).
	if !sameSet(got, cell(1, -1), cell(0, 1)) {
		t.Errorf("queen moves = %v", got)
	}
}

func TestOneHiveRuleStopsAPieceThatHoldsTheHiveTogether(t *testing.T) {
	g := game(t, "hive")
	// white ant in the middle of a line: lifting it splits the hive.
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(-1, 0), 1, "queen"}, tile{cell(0, 0), 1, "ant"}, tile{cell(1, 0), 2, "queen"})
	if got := movesFrom(g, st, 1, cell(0, 0)); len(got) != 0 {
		t.Errorf("a pinned ant moved: %v", got)
	}
}

func TestFreedomToMoveBlocksATightGap(t *testing.T) {
	g := game(t, "hive")
	// Tiles at (1,-1) and (0,1) stand either side of the step to (1,0): the
	// queen cannot squeeze through that gate.
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 1, "queen"}, tile{cell(1, -1), 2, "ant"}, tile{cell(0, 1), 2, "ant"}, tile{cell(-1, 1), 2, "queen"})
	got := movesFrom(g, st, 1, cell(0, 0))
	if has(got, cell(1, 0)) {
		t.Errorf("queen squeezed through a gate: %v", got)
	}
}

func TestSoldierAntWalksTheWholeOutside(t *testing.T) {
	g := game(t, "hive")
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 1, "queen"}, tile{cell(1, 0), 1, "ant"}, tile{cell(2, 0), 2, "queen"})
	got := movesFrom(g, st, 1, cell(1, 0))
	// The hive is a line of three; the ant is the middle, so it is pinned.
	if len(got) != 0 {
		t.Errorf("the middle ant moved: %v", got)
	}
	st = build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 1, "queen"}, tile{cell(1, 0), 2, "queen"}, tile{cell(0, 1), 1, "ant"})
	got = movesFrom(g, st, 1, cell(0, 1))
	// The hive is a triangle of three: the ant can walk all the way round it,
	// but never ends where it started.
	if len(got) < 6 || has(got, cell(0, 1)) {
		t.Errorf("ant moves = %v", got)
	}
}

func TestSpiderMovesExactlyThree(t *testing.T) {
	g := game(t, "hive")
	// A straight line of four tiles; the spider is at the left end.
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 1, "spider"}, tile{cell(1, 0), 1, "queen"}, tile{cell(2, 0), 2, "ant"}, tile{cell(3, 0), 2, "queen"})
	got := movesFrom(g, st, 1, cell(0, 0))
	// Along the top of the line: (0,0)->(1,-1)->(2,-1)->(3,-1); and
	// underneath: (0,1)->(1,1)->(2,1).
	if !sameSet(got, cell(3, -1), cell(2, 1)) {
		t.Errorf("spider moves = %v", got)
	}
}

func TestGrasshopperJumpsOverTheLine(t *testing.T) {
	g := game(t, "hive")
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 1, "grasshopper"}, tile{cell(1, 0), 1, "queen"}, tile{cell(2, 0), 2, "ant"}, tile{cell(3, 0), 2, "queen"})
	got := movesFrom(g, st, 1, cell(0, 0))
	if !sameSet(got, cell(4, 0)) {
		t.Errorf("grasshopper moves = %v", got)
	}
}

func TestBeetleClimbsAndMovesAsATopTile(t *testing.T) {
	g := game(t, "hive")
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 1, "beetle"}, tile{cell(1, 0), 2, "queen"}, tile{cell(0, 1), 1, "queen"})
	got := movesFrom(g, st, 1, cell(0, 0))
	if !has(got, cell(1, 0)) || !has(got, cell(0, 1)) {
		t.Errorf("beetle cannot climb: %v", got)
	}
	st, _ = play(t, g, st, 1, cell(0, 0)+"-"+cell(1, 0))
	stack := st.Stack(board.PositionID(cell(1, 0)))
	if len(stack) != 2 || stack[1].Piece != "beetle" || stack[1].Color != "white" {
		t.Fatalf("stack = %v", stack)
	}
	// The covered queen cannot move; the beetle can drop back down.
	if got := movesFrom(g, st, 2, cell(1, 0)); len(got) != 0 {
		t.Errorf("a covered tile moved (black to move): %v", got)
	}
	// New tiles can't touch a stack the enemy holds from above.
	for _, m := range notations(g, st, 2) {
		if strings.Contains(m, "@") {
			c := m[strings.IndexByte(m, '@')+1:]
			if c == cell(2, 0) || c == cell(2, -1) || c == cell(1, 1) || c == cell(1, -1) {
				// all touch the white-topped stack
				t.Errorf("black placed beside a white-topped stack: %s", m)
			}
		}
	}
}

func TestLadybugTwoOverThenDown(t *testing.T) {
	g := game(t, "hive")
	// A line of three tiles; the ladybug stands at one end.
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 1, "ladybug"}, tile{cell(1, 0), 1, "queen"}, tile{cell(2, 0), 2, "ant"}, tile{cell(3, 0), 2, "queen"})
	got := movesFrom(g, st, 1, cell(0, 0))
	// Over (1,0) and (2,0), then down on a cell beside (2,0) that is empty and
	// is not the start: (3,-1), (2,1)... (2,0)'s empties: (2,-1),(1,1),(2,1),(3,-1).
	// (1,0) then (2,0) is the only walk (the line), so these four.
	want := []string{cell(2, -1), cell(1, 1), cell(2, 1), cell(3, -1)}
	if !sameSet(got, want...) {
		t.Errorf("ladybug moves = %v, want %v", got, want)
	}
}

func TestMosquitoCopiesWhatItTouches(t *testing.T) {
	g := game(t, "hive")
	// A mosquito in the middle of a line is pinned.
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 1, "queen"}, tile{cell(1, 0), 1, "mosquito"}, tile{cell(2, 0), 2, "grasshopper"}, tile{cell(3, 0), 2, "queen"})
	if got := movesFrom(g, st, 1, cell(1, 0)); len(got) != 0 {
		t.Errorf("a pinned mosquito moved: %v", got)
	}
	// It touches the white queen and the black grasshopper: it slides like a
	// queen and jumps like a grasshopper (over (1,0), landing on (2,-1)).
	st = build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 1, "queen"}, tile{cell(1, 0), 2, "grasshopper"}, tile{cell(2, 0), 2, "queen"}, tile{cell(0, 1), 1, "mosquito"})
	got := movesFrom(g, st, 1, cell(0, 1))
	if !has(got, cell(2, -1)) {
		t.Errorf("the mosquito did not jump like a grasshopper: %v", got)
	}
	if !has(got, cell(-1, 1)) {
		t.Errorf("the mosquito did not slide like a queen: %v", got)
	}
	// Touching only another mosquito, it can do nothing.
	st = build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 2, "mosquito"}, tile{cell(1, 0), 1, "mosquito"}, tile{cell(-1, 0), 2, "queen"}, tile{cell(2, 0), 1, "queen"})
	if got := movesFrom(g, st, 1, cell(1, 0)); len(got) != 0 {
		t.Errorf("a mosquito beside a mosquito moved: %v", got)
	}
}

func TestMosquitoOnTopMovesLikeABeetle(t *testing.T) {
	g := game(t, "hive")
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 2, "queen"}, tile{cell(0, 0), 1, "mosquito"}, tile{cell(1, 0), 1, "queen"})
	got := movesFrom(g, st, 1, cell(0, 0))
	if !has(got, cell(1, 0)) {
		t.Errorf("a mosquito on the hive cannot step like a beetle: %v", got)
	}
}

func TestPillbugThrowsATileOverItself(t *testing.T) {
	g := game(t, "hive")
	// White pillbug at (0,0), black ant at (1,0) alone beside it (white queen far
	// at (-1,0) gives touch).
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(-1, 0), 1, "queen"}, tile{cell(0, 0), 1, "pillbug"}, tile{cell(1, 0), 2, "ant"}, tile{cell(1, -1), 2, "queen"})
	got := movesFrom(g, st, 1, cell(1, 0))
	// Thrown to empty cells beside the pillbug: (0,1), (-1,1), (0,-1)?
	// the pillbug's neighbours: (1,0)t, (1,-1)q, (0,-1), (-1,0)q, (-1,1), (0,1)
	if !sameSet(got, cell(0, -1), cell(-1, 1), cell(0, 1)) {
		t.Errorf("thrown ant lands on %v", got)
	}
}

func TestPillbugCannotThrowTheTileJustMoved(t *testing.T) {
	g := game(t, "hive")
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}, Moved: cell(1, 0)},
		tile{cell(-1, 0), 1, "queen"}, tile{cell(0, 0), 1, "pillbug"}, tile{cell(1, 0), 2, "ant"}, tile{cell(1, -1), 2, "queen"})
	for _, m := range notations(g, st, 1) {
		if strings.HasPrefix(m, cell(1, 0)+"-") {
			// ants cannot walk by themselves (white to move) so any move is a throw
			t.Errorf("threw the tile that was just moved: %s", m)
		}
	}
}

func TestPillbugCannotThrowACoveredPillbugOrAPinnedTile(t *testing.T) {
	g := game(t, "hive")
	// The black ant at (1,0) is the only link to the black queen: lifting it
	// would split the hive.
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 1, "pillbug"}, tile{cell(1, 0), 2, "ant"}, tile{cell(2, 0), 2, "queen"}, tile{cell(-1, 0), 1, "queen"})
	if got := movesFrom(g, st, 1, cell(1, 0)); len(got) != 0 {
		t.Errorf("threw a tile that holds the hive together: %v", got)
	}
	// A covered pillbug cannot throw.
	st = build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 1, "pillbug"}, tile{cell(0, 0), 2, "beetle"}, tile{cell(1, 0), 2, "ant"}, tile{cell(0, 1), 2, "queen"}, tile{cell(-1, 0), 1, "queen"})
	for _, m := range notations(g, st, 1) {
		if strings.HasPrefix(m, cell(1, 0)+"-") {
			t.Errorf("a covered pillbug threw: %s", m)
		}
	}
}

func ring(seat int, kind string, skip int, q [2]int) []tile {
	var out []tile
	dirs := [][2]int{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}}
	for i, d := range dirs {
		if i != skip {
			out = append(out, tile{cell(q[0]+d[0], q[1]+d[1]), seat, kind})
		}
	}
	return out
}

func TestSurroundingTheQueenWins(t *testing.T) {
	g := game(t, "hive")
	// Black's queen in the middle, five white tiles round it; White's queen
	// slides in to close the sixth side.
	tiles := append([]tile{{cell(0, 0), 2, "queen"}, {cell(1, 1), 1, "queen"}}, ring(1, "beetle", 0, [2]int{0, 0})...)
	st := build(t, g, 1, hive.Data{Turns: [2]int{4, 4}}, tiles...)
	want := cell(1, 1) + "-" + cell(1, 0)
	if !has(notations(g, st, 1), want) {
		t.Fatalf("no closing move %s", want)
	}
	next, res := play(t, g, st, 1, want)
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 {
		t.Errorf("surrounding the queen should win for White: %+v", res)
	}
	if out := hive.New().Outcome(game(t, "hive"), next); !out.Ended || out.Winner == nil || *out.Winner != 1 {
		t.Errorf("outcome = %+v", out)
	}
}

func TestSurroundingBothQueensIsADraw(t *testing.T) {
	g := game(t, "hive")
	// Both queens are completely ringed (they share two of the ring tiles), so
	// any move that leaves the rings alone ends the game as a draw.
	tiles := []tile{
		{cell(0, 0), 2, "queen"}, {cell(1, 0), 1, "queen"},
		{cell(1, -1), 1, "ant"}, {cell(0, -1), 1, "ant"}, {cell(-1, 0), 1, "ant"}, {cell(-1, 1), 1, "ant"}, {cell(0, 1), 1, "ant"},
		{cell(2, 0), 2, "ant"}, {cell(2, -1), 2, "ant"}, {cell(1, 1), 2, "grasshopper"},
	}
	st := build(t, g, 2, hive.Data{Turns: [2]int{5, 5}}, tiles...)
	for _, m := range notations(g, st, 2) {
		if !strings.Contains(m, "@") {
			continue
		}
		_, res := play(t, g, st, 2, m)
		if !res.GameEnded || res.WinnerSeat != nil {
			t.Fatalf("%s: both queens surrounded should draw: %+v", m, res)
		}
		return
	}
	t.Skip("no placement available in this position")
}

func TestRandomGamesAreValidAndTerminate(t *testing.T) {
	for _, id := range []string{"hive", "hive-base"} {
		g := game(t, id)
		rs := hive.New()
		rng := rand.New(rand.NewSource(7))
		results := map[string]int{}
		for n := 0; n < 40; n++ {
			st, _ := rs.Start(g, 1)
			seat := 1
			for ply := 0; ply < 400; ply++ {
				moves := rs.Legal(g, st, seat)
				if len(moves) == 0 {
					break
				}
				mv := moves[rng.Intn(len(moves))]
				text := rs.Format(g, mv)
				back, err := rs.Parse(g, text)
				if err != nil || rs.Format(g, back) != text {
					t.Fatalf("%s: notation %q does not round-trip: %v", id, text, err)
				}
				res, err := rs.Apply(g, st, seat, mv)
				if err != nil {
					t.Fatalf("%s: a legal move %q was refused: %v", id, text, err)
				}
				if err := rs.Validate(g, res.NewState); err != nil {
					t.Fatalf("%s: invalid position after %q: %v", id, text, err)
				}
				st = res.NewState
				if res.GameEnded {
					break
				}
				seat = res.NextTurnSeat
			}
			o := rs.Outcome(g, st)
			switch {
			case !o.Ended:
				results["unfinished"]++
			case o.Winner == nil:
				results["draw"]++
			default:
				results["win"]++
			}
		}
		if results["unfinished"] > 0 {
			t.Errorf("%s: %d games did not finish within 400 plies", id, results["unfinished"])
		}
		t.Logf("%s: %v", id, results)
	}
}

func TestNotationIsTheUniversalHiveProtocol(t *testing.T) {
	g := game(t, "hive")
	rs := hive.New()
	st, _ := rs.Start(g, 1)
	first := protocol(g, st, 1)
	if !has(first, "wA1") || !has(first, "wQ") || !has(first, "wM") {
		t.Errorf("openings = %v", first)
	}
	st, res := play(t, g, st, 1, "wA1")
	if res.Notation != "wA1" {
		t.Errorf("notation = %q", res.Notation)
	}
	// Black lands beside wA1 on each of its six sides, written from wA1.
	reply := protocol(g, st, 2)
	for _, want := range []string{"bQ wA1-", "bQ wA1/", `bQ wA1\`, "bQ -wA1", "bQ /wA1", `bQ \wA1`} {
		if !has(reply, want) {
			t.Errorf("black cannot play %q; moves: %v", want, reply[:6])
		}
	}
	st, _ = play(t, g, st, 2, "bQ wA1-")
	if _, ok := st.Get("k10"); !ok {
		t.Errorf("bQ wA1- should land on the right of the ant (k10)")
	}
	// The second ant is A2; numbers follow the supply, not the cell.
	st, _ = play(t, g, st, 1, `wA2 \wA1`)
	if o, ok := st.Get("j9"); !ok || o.Piece != "ant" {
		t.Errorf("wA2 \\wA1 should be the ant above and left of wA1 (j9)")
	}
	// Text is read in any case.
	if _, err := rs.Parse(g, "bq wa1-"); err != nil {
		t.Errorf("lower case should read: %v", err)
	}
}

func TestBeetleClimbIsWrittenWithJustTheTileUnderneath(t *testing.T) {
	g := game(t, "hive")
	st := build(t, g, 1, hive.Data{Turns: [2]int{2, 2}},
		tile{cell(0, 0), 1, "beetle"}, tile{cell(1, 0), 2, "queen"}, tile{cell(0, 1), 1, "queen"})
	var found bool
	for _, m := range protocol(g, st, 1) {
		if m == "wB1 bQ" {
			found = true
		}
	}
	if !found {
		t.Errorf("a climbing beetle should read 'wB1 bQ': %v", protocol(g, st, 1))
	}
	// A move by its protocol text applies.
	next, _ := play(t, g, st, 1, "wB1 bQ")
	if stack := next.Stack(board.PositionID(cell(1, 0))); len(stack) != 2 {
		t.Errorf("stack = %v", stack)
	}
}
