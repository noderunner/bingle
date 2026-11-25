// Package fendo_test checks Fendo's rules: moving with one turn, building
// fences that may not close off an empty area, adding pieces, and scoring the
// closed areas.
package fendo_test

import (
	"encoding/json"
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

func TestTheGameEndsWhenOnlyOneSeatHasPiecesInTheOpenArea(t *testing.T) {
	g := game(t, "fendo")
	// Orange's only piece is fenced in on g7 but for its west side, and it
	// has no stock to add: once white closes that side, white's two pieces
	// are alone in the open area, which can only end up white's. The game
	// ends there, the open area counted white's: 48 squares to 1.
	st := build([]string{"f1", "b1"}, []string{"g7"}, [][2]string{{"g6", "g7"}}, 1)
	st.Ext().(*fendo.Data).Stock = [2]int{2, 0}
	next, res := play(t, g, st, 1, "f1-f7+e")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 {
		t.Fatalf("white owns the open area and should win: %+v", res)
	}
	d := next.Ext().(*fendo.Data)
	if d.Score != [2]int{48, 1} {
		t.Errorf("score = %v", d.Score)
	}
	if !strings.Contains(strings.Join(d.Events, " "), "Only White's pieces are left in the open area") {
		t.Errorf("events = %v", d.Events)
	}
	view, err := json.Marshal(fendo.New().Redact(g, next, 1).Ext())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(view), `"a4":"area1"`) || !strings.Contains(string(view), `"g7":"area2"`) {
		t.Errorf("the open area should be tinted white's: %s", view)
	}
}

// TestASealedInSeatDoesNotPassUntilTheEnd replays an 11×11 game in which
// all of orange's pieces were fenced in while white's still shared the open
// area: it went on with orange passing every other ply. It now ends there.
func TestASealedInSeatDoesNotPassUntilTheEnd(t *testing.T) {
	g := game(t, "fendo-11")
	rs := fendo.New()
	st, _ := rs.Start(g, 1)
	moves := strings.Fields("add@f10 add@g3 add@g4 add@f6 f1-e11+e add@f9 g4-g11+w add@h11 add@h10 add@h9 " +
		"h10-i11+w add@d11 g11-h10+n g3-j10+s f10-g10+n f6-g7+w e11-f10+n g7-g9+w i11-g8+n h9-g7+n " +
		"h10-h9+w f9-f8+w g10-i10+e g7-h8+w f10-g10+s d11-f10+w h9-k10+w h8-j8+s g10-h8+e j8-i6+s " +
		"k10-j11+s i6-h7+w i10-h6+n f8-h5+n h6-i7+w f10-f8+s h8-h9+s f8+e add@e6 h5-f4+s " +
		"e6-f9+s f4-h5+s j11-k4+s h5-e9+e k4-e8+n e9-d5+w i7-d6+w d5-e4+s add@f4 e4-c6+w " +
		"f4-c5+n c6-f7+w d6-f6+n add@i2 e8-d9+s")
	seat := 1
	for n, text := range moves {
		next, res := play(t, g, st, seat, text)
		st = next
		if res.GameEnded {
			d := st.Ext().(*fendo.Data)
			if res.WinnerSeat == nil || *res.WinnerSeat != 1 || d.Score[0]+d.Score[1] != 121 {
				t.Fatalf("after %s: winner %v, score %v", text, res.WinnerSeat, d.Score)
			}
			return
		}
		if got := notations(g, st, res.NextImpulseSeat); len(got) == 1 && got[0] == "pass" {
			t.Fatalf("after move %d, %s, the game goes on with only a pass for seat %d", n+1, text, res.NextImpulseSeat)
		}
		seat = res.NextImpulseSeat
	}
	t.Fatal("the game did not end")
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
				seat = res.NextImpulseSeat
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
