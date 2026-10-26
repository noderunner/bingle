// Package fendo_test checks Fendo's rules: moving with one turn, building
// fences that may not close off an empty area, adding pieces, and scoring the
// closed areas.
package fendo_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/fendo"
)

func game(t testing.TB, id string) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(fendo.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("%s is not registered", id)
	}
	return g
}

// build makes a position: white and orange piece squares, fences as pairs of
// touching squares, and the seat to move. Stocks default to 2 each.
func build(white, orange []string, walls [][2]string, turn int) *state.BoardState {
	st := state.NewBoardState()
	for _, c := range white {
		st.Set(board.PositionID(c), state.Occupant{Piece: "piece", Color: "white"})
	}
	for _, c := range orange {
		st.Set(board.PositionID(c), state.Occupant{Piece: "piece", Color: "orange"})
	}
	st.SetExt(&fendo.Data{Turn: turn, Stock: [2]int{2, 2}, Walls: walls})
	return st
}

func notations(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	rs := fendo.New()
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
	rs := fendo.New()
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

func TestStartingPosition(t *testing.T) {
	g := game(t, "fendo")
	st, err := fendo.New().Start(g, 1)
	if err != nil {
		t.Fatal(err)
	}
	if o, ok := st.Get("d1"); !ok || o.Color != "white" {
		t.Errorf("d1 = %v %v", o, ok)
	}
	if o, ok := st.Get("d7"); !ok || o.Color != "orange" {
		t.Errorf("d7 = %v %v", o, ok)
	}
	d := st.Ext().(*fendo.Data)
	if d.Stock != [2]int{6, 6} {
		t.Errorf("stock = %v", d.Stock)
	}
	moves := notations(g, st, 1)
	for _, want := range []string{"d1-d6+n", "d1-a1+e", "d1-a5+e", "d1-c3+n", "d1+n", "add@d2", "add@g6"} {
		if !has(moves, want) {
			t.Errorf("White cannot play %s", want)
		}
	}
	// A fence cannot go on the outer edge, which is a wall already.
	for _, bad := range []string{"d1+s", "d1-a1+w", "d1-g1+s"} {
		if has(moves, bad) {
			t.Errorf("fence on the outer edge allowed: %s", bad)
		}
	}
	// d7 is occupied: nobody can land on it or pass it.
	for _, m := range moves {
		if strings.Contains(m, "-d7") {
			t.Errorf("moved onto a piece: %s", m)
		}
	}
}

func TestPiecesAreBlockedByPiecesAndFences(t *testing.T) {
	g := game(t, "fendo")
	// a1 is hemmed in by pieces on a2 and b1: it cannot go anywhere (but it
	// may still stay and fence), and nothing may pass over them.
	st := build([]string{"a1", "g7"}, []string{"a2", "b1"}, nil, 1)
	for _, m := range notations(g, st, 1) {
		if strings.HasPrefix(m, "a1-") {
			t.Errorf("a hemmed-in piece moved: %s", m)
		}
	}
	// A fence between a1 and b1 stops the walk right; it also blocks the turn.
	st = build([]string{"a1", "g7"}, nil, [][2]string{{"a1", "b1"}}, 1)
	moves := notations(g, st, 1)
	if has(moves, "a1-b1+n") || has(moves, "a1-c1+n") {
		t.Errorf("walked through a fence")
	}
	// But up and over is fine: a1 up to a2 then right along rank 2.
	if !has(moves, "a1-c2+n") {
		t.Errorf("one turn round the fence should be allowed")
	}
}

func TestFencesCannotCloseOffAnEmptyAreaOrASecondOpenArea(t *testing.T) {
	g := game(t, "fendo")
	// 7x7 with white on a1 and orange on a7, and a fence line along the top of
	// row 3 from a to f (between rank 3 and 4) except g, so g3-g4 is the only
	// gap. Pieces at a1 and a2 below; orange alone above.
	var walls [][2]string
	for _, c := range []string{"a", "b", "c", "d", "e", "f"} {
		walls = append(walls, [2]string{c + "3", c + "4"})
	}
	st := build([]string{"a1", "a2"}, []string{"a7"}, walls, 1)
	moves := notations(g, st, 1)
	// Closing the gap g3-g4 leaves the lower part with two white pieces (still
	// open) and the upper part with one orange piece (closed).
	// A piece on g3 fencing north closes the upper region (one orange piece).
	if !has(moves, "a1-g3+n") {
		t.Errorf("closing off an area with exactly one piece should be legal")
	}
	// With no piece above, closing it would enclose an empty area.
	st = build([]string{"a1", "a2"}, []string{"b1"}, walls, 1)
	for _, m := range notations(g, st, 1) {
		if strings.HasSuffix(m, "+n") && strings.Contains(m, "-g3") {
			t.Errorf("closed off an empty area: %s", m)
		}
	}
}

func TestClosingTheLastOpenAreaEndsAndScores(t *testing.T) {
	g := game(t, "fendo")
	// White alone in the bottom three ranks, orange alone above: a fence at the
	// gap g3-g4 closes both and ends the game: 21 squares to 28.
	var walls [][2]string
	for _, c := range []string{"a", "b", "c", "d", "e", "f"} {
		walls = append(walls, [2]string{c + "3", c + "4"})
	}
	st := build([]string{"a1"}, []string{"a7"}, walls, 1)
	next, res := play(t, g, st, 1, "a1-g3+n")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 2 {
		t.Fatalf("orange owns 28 squares to 21 and should win: %+v", res)
	}
	d := next.Ext().(*fendo.Data)
	if d.Score != [2]int{21, 28} {
		t.Errorf("score = %v", d.Score)
	}
	if out := fendo.New().Outcome(g, next); !out.Ended || out.Winner == nil || *out.Winner != 2 {
		t.Errorf("outcome = %+v", out)
	}
}

func TestAPlayerWithNoPieceInTheOpenAreaPasses(t *testing.T) {
	g := game(t, "fendo")
	// A piece closed into its own room cannot act: white's a1 has fences above
	// and to the right.
	walls := [][2]string{{"a1", "a2"}, {"a1", "b1"}}
	st := build([]string{"a1"}, []string{"d7", "d6"}, walls, 1)
	// white's only piece is closed in: nothing but a pass.
	if got := notations(g, st, 1); len(got) != 1 || got[0] != "pass" {
		t.Errorf("a closed-in player should only pass: %v", got)
	}
}

func TestAddedPieceMustBeOneMoveAwayAndCostsStock(t *testing.T) {
	g := game(t, "fendo")
	st := build([]string{"a1"}, []string{"g7"}, [][2]string{{"a1", "b1"}}, 1)
	moves := notations(g, st, 1)
	if !has(moves, "add@a7") || !has(moves, "add@c2") {
		t.Errorf("additions missing: %v", moves)
	}
	// The fence stops a1 going right, so b1 is not one move away.
	if has(moves, "add@b1") || has(moves, "add@c1") {
		t.Errorf("a piece was added across a fence")
	}
	next, _ := play(t, g, st, 1, "add@a3")
	if d := next.Ext().(*fendo.Data); d.Stock[0] != 1 {
		t.Errorf("stock = %v", d.Stock)
	}
	if _, ok := next.Get("a3"); !ok {
		t.Errorf("no piece on a3")
	}
	if next.Ext().(*fendo.Data).Turn != 2 {
		t.Errorf("turn did not pass")
	}
}

func TestMajorityWinsOnTheBigBoard(t *testing.T) {
	g := game(t, "fendo-9")
	st, _ := fendo.New().Start(g, 1)
	if o, ok := st.Get("e1"); !ok || o.Color != "white" {
		t.Errorf("white should start on e1: %v", o)
	}
	if g.ParamInt("majority", 0) != 41 {
		t.Errorf("majority = %d", g.ParamInt("majority", 0))
	}
}

func TestRandomGamesAreValidAndTerminate(t *testing.T) {
	for _, id := range []string{"fendo", "fendo-9"} {
		g := game(t, id)
		rs := fendo.New()
		rng := rand.New(rand.NewSource(11))
		results := map[string]int{}
		for n := 0; n < 40; n++ {
			st, _ := rs.Start(g, 1)
			seat := 1
			for ply := 0; ply < 500; ply++ {
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
					t.Fatalf("%s: legal move %q refused: %v", id, text, err)
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
			t.Errorf("%s: %d games did not finish", id, results["unfinished"])
		}
		t.Logf("%s: %v", id, results)
	}
}
