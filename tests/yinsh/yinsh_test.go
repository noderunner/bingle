// Package yinsh_test checks Yinsh's rules: the 85-point board, placing rings,
// moving a ring with the marker it leaves behind, jumping and turning over
// markers, and removing rows and rings.
package yinsh_test

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/yinsh"
)

func game(t testing.TB, id string) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(yinsh.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("%s is not registered", id)
	}
	return g
}

type stones struct {
	wRings, bRings, wMarkers, bMarkers []string
}

// build makes a position in the play stage: seat to move, with d's other fields.
func build(s stones, d yinsh.Data) *state.BoardState {
	st := state.NewBoardState()
	put := func(cells []string, piece, color string) {
		for _, c := range cells {
			st.Set(board.PositionID(c), state.Occupant{Piece: piece, Color: color})
		}
	}
	put(s.wRings, "ring", "white")
	put(s.bRings, "ring", "black")
	put(s.wMarkers, "marker", "white")
	put(s.bMarkers, "marker", "black")
	if d.Mover == 0 {
		d.Mover = d.Turn
	}
	st.SetExt(&d)
	return st
}

func movesFrom(g *catalog.CompiledGame, st *state.BoardState, seat int, from string) []string {
	var out []string
	for _, mv := range yinsh.New().Legal(g, st, seat) {
		if mv.Action == "move" && string(mv.From) == from {
			out = append(out, string(mv.To))
		}
	}
	sort.Strings(out)
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
	rs := yinsh.New()
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

func TestTheBoardHas85Points(t *testing.T) {
	g := game(t, "yinsh")
	if n := len(g.Board.Positions()); n != 85 {
		t.Fatalf("%d points, want 85", n)
	}
	for _, id := range []string{"a2", "a5", "b1", "b7", "e1", "e10", "k7", "k10", "g11", "f2"} {
		if !g.Board.HasPosition(board.PositionID(id)) {
			t.Errorf("%s should be on the board", id)
		}
	}
	for _, id := range []string{"a1", "a6", "k6", "k11", "f1", "g1", "b8"} {
		if g.Board.HasPosition(board.PositionID(id)) {
			t.Errorf("%s should not be on the board", id)
		}
	}
}

func TestSetupPlacesFiveRingsEachThenWhiteMoves(t *testing.T) {
	g := game(t, "yinsh")
	rs := yinsh.New()
	st, _ := rs.Start(g, 1)
	cells := []string{"e5", "f6", "d4", "g7", "e6", "f7", "d5", "g8", "e7", "f8"}
	seat := 1
	for i, c := range cells {
		if rs.ToMove(g, st) != seat {
			t.Fatalf("placement %d: %d to move, want %d", i, rs.ToMove(g, st), seat)
		}
		st, _ = play(t, g, st, seat, c)
		seat = 3 - seat
	}
	d := st.Ext().(*yinsh.Data)
	if d.Stage != "" || d.Turn != 1 {
		t.Fatalf("after setup: stage %q, turn %d", d.Stage, d.Turn)
	}
	// White's moves: each white ring may move.
	if len(rs.Legal(g, st, 1)) == 0 {
		t.Fatal("White has no move")
	}
	if o, _ := st.Get("e5"); o.Piece != "ring" || o.Color != "white" {
		t.Errorf("e5 = %v", o)
	}
}

func TestRingMovesAlongLinesLeavingAMarker(t *testing.T) {
	g := game(t, "yinsh")
	st := build(stones{wRings: []string{"e5"}, bRings: []string{"a2"}}, yinsh.Data{Turn: 1})
	got := movesFrom(g, st, 1, "e5")
	// Along the column (e): e1..e4 and e6..e10; along the row (5): b5..d5, f5..; diagonals.
	for _, want := range []string{"e1", "e10", "a5", "b5", "j10", "f6", "d4", "a1"} {
		if want == "a1" {
			if has(got, want) {
				t.Errorf("a1 is not on the board")
			}
			continue
		}
		if !has(got, want) {
			t.Errorf("the ring cannot reach %s: %v", want, got)
		}
	}
	// Not off the lines: f7 is two rows up from f5 and not on a line from e5.
	if has(got, "f7") || has(got, "g6") {
		t.Errorf("a point off the lines is reachable: %v", got)
	}
	next, _ := play(t, g, st, 1, "e5-e8")
	if o, _ := next.Get("e5"); o.Piece != "marker" || o.Color != "white" {
		t.Errorf("e5 should hold a white marker, got %v", o)
	}
	if o, _ := next.Get("e8"); o.Piece != "ring" || o.Color != "white" {
		t.Errorf("e8 should hold the white ring, got %v", o)
	}
}

func TestRingsAreNeverPassedOrLandedOn(t *testing.T) {
	g := game(t, "yinsh")
	st := build(stones{wRings: []string{"e5"}, bRings: []string{"e7", "c5"}}, yinsh.Data{Turn: 1})
	got := movesFrom(g, st, 1, "e5")
	if !has(got, "e6") || has(got, "e7") || has(got, "e8") {
		t.Errorf("column moves past a ring: %v", got)
	}
	if !has(got, "d5") || has(got, "c5") || has(got, "b5") {
		t.Errorf("row moves past a ring: %v", got)
	}
}

func TestJumpingMarkersTurnsThemOverAndMustLandRightAfter(t *testing.T) {
	g := game(t, "yinsh")
	// A white ring on e3 with black markers on e5 e6 e7 and e4 empty: it may go
	// to e4, or jump e5–e7 to land on e8 only (the first empty point after them).
	st := build(stones{wRings: []string{"e3"}, bRings: []string{"a2"}, bMarkers: []string{"e5", "e6", "e7"}}, yinsh.Data{Turn: 1})
	got := movesFrom(g, st, 1, "e3")
	if !has(got, "e4") || !has(got, "e8") {
		t.Errorf("moves = %v", got)
	}
	if has(got, "e5") || has(got, "e6") || has(got, "e9") || has(got, "e10") {
		t.Errorf("a ring landed on a marker or past the first empty point: %v", got)
	}
	next, _ := play(t, g, st, 1, "e3-e8")
	for _, c := range []string{"e5", "e6", "e7"} {
		if o, _ := next.Get(board.PositionID(c)); o.Piece != "marker" || o.Color != "white" {
			t.Errorf("%s should be a white marker now: %v", c, o)
		}
	}
	// Markers behind or beside the path are untouched, and the marker dropped on
	// e3 is white.
	if o, _ := next.Get("e3"); o.Piece != "marker" || o.Color != "white" {
		t.Errorf("e3 = %v", o)
	}
	// A ring may not jump a group that ends at a ring or at the board's edge.
	st = build(stones{wRings: []string{"e3"}, bRings: []string{"e8"}, bMarkers: []string{"e5", "e6", "e7"}}, yinsh.Data{Turn: 1})
	if got := movesFrom(g, st, 1, "e3"); has(got, "e8") || has(got, "e9") {
		t.Errorf("jumped onto or past a ring: %v", got)
	}
}

func TestEmptyPointsThenMarkersThenLanding(t *testing.T) {
	g := game(t, "yinsh")
	// e2 e3 empty, e4 e5 markers, e6 empty: from e1 the ring may stop on e2, e3
	// or jump to e6 but nowhere past.
	st := build(stones{wRings: []string{"e1"}, bRings: []string{"a2"}, wMarkers: []string{"e4", "e5"}}, yinsh.Data{Turn: 1})
	got := movesFrom(g, st, 1, "e1")
	for _, want := range []string{"e2", "e3", "e6"} {
		if !has(got, want) {
			t.Errorf("%s should be reachable: %v", want, got)
		}
	}
	if has(got, "e7") {
		t.Errorf("e7 is past the first empty point after the markers")
	}
}

func TestMakingARowRemovesItAndARing(t *testing.T) {
	g := game(t, "yinsh")
	rs := yinsh.New()
	st := build(stones{
		wRings:   []string{"f5"},
		bRings:   []string{"a2", "k9", "j9"},
		wMarkers: []string{"b5", "c5", "d5", "e5"},
	}, yinsh.Data{Turn: 1})
	// f5 drops a marker on f5 (completing b5–f5) and moves away.
	st, _ = play(t, g, st, 1, "f5-f8")
	d := st.Ext().(*yinsh.Data)
	if d.Stage != "row" || d.Turn != 1 {
		t.Fatalf("after completing a row: stage %q turn %d", d.Stage, d.Turn)
	}
	moves := rs.Legal(g, st, 1)
	if len(moves) != 1 || rs.Format(g, moves[0]) != "row:b5-f5" {
		var names []string
		for _, mv := range moves {
			names = append(names, rs.Format(g, mv))
		}
		t.Fatalf("row choices: %v", names)
	}
	st, _ = play(t, g, st, 1, "row:b5-f5")
	if st.Ext().(*yinsh.Data).Stage != "ring" {
		t.Fatalf("a ring must be chosen next")
	}
	if _, ok := st.Get("c5"); ok {
		t.Errorf("the row was not removed")
	}
	st, _ = play(t, g, st, 1, "ring:f8")
	d = st.Ext().(*yinsh.Data)
	if d.Score != [2]int{1, 0} {
		t.Errorf("score = %v", d.Score)
	}
	if d.Turn != 2 || d.Stage != "" {
		t.Errorf("the turn should pass to Black: turn %d stage %q", d.Turn, d.Stage)
	}
	if _, ok := st.Get("f8"); ok {
		t.Errorf("the ring was not removed")
	}
}

func TestALongRowLetsYouChooseWhichFive(t *testing.T) {
	g := game(t, "yinsh")
	rs := yinsh.New()
	st := build(stones{
		wRings:   []string{"g5"},
		bRings:   []string{"a2", "k9"},
		wMarkers: []string{"b5", "c5", "d5", "e5", "f5"},
	}, yinsh.Data{Turn: 1})
	// g5 drops a marker making six in a row (b5–g5), then moves off.
	st, _ = play(t, g, st, 1, "g5-g8")
	var names []string
	for _, mv := range rs.Legal(g, st, 1) {
		names = append(names, rs.Format(g, mv))
	}
	if len(names) != 2 || !has(names, "row:b5-f5") || !has(names, "row:c5-g5") {
		t.Fatalf("row choices = %v", names)
	}
}

func TestBothPlayersRowsMoverFirstThenOpponent(t *testing.T) {
	g := game(t, "yinsh")
	// White's marker completes b5–f5; Black already has five markers in a row
	// (c7–g7), as if White's jump had just turned them over. White removes its
	// row first, then Black removes its own.
	st := build(stones{
		wRings:   []string{"f5"},
		bRings:   []string{"a2", "k9", "j9"},
		wMarkers: []string{"b5", "c5", "d5", "e5"},
		bMarkers: []string{"c7", "d7", "e7", "f7", "g7"},
	}, yinsh.Data{Turn: 1})
	st, _ = play(t, g, st, 1, "f5-f3")
	if d := st.Ext().(*yinsh.Data); d.Turn != 1 || d.Stage != "row" {
		t.Fatalf("White should remove first: %+v", d)
	}
	st, _ = play(t, g, st, 1, "row:b5-f5")
	st, _ = play(t, g, st, 1, "ring:f3")
	d := st.Ext().(*yinsh.Data)
	if d.Turn != 2 || d.Stage != "row" || d.Mover != 1 {
		t.Fatalf("Black should now remove its row: %+v", d)
	}
	st, _ = play(t, g, st, 2, "row:c7-g7")
	st, _ = play(t, g, st, 2, "ring:k9")
	d = st.Ext().(*yinsh.Data)
	if d.Score != [2]int{1, 1} {
		t.Errorf("score = %v", d.Score)
	}
	if d.Turn != 2 || d.Mover != 2 || d.Stage != "" {
		t.Errorf("then Black's turn to move: %+v", d)
	}
}

func TestThirdRingWinsAndBlitzOneRingWins(t *testing.T) {
	for _, tc := range []struct {
		id    string
		score [2]int
	}{{"yinsh", [2]int{2, 0}}, {"yinsh-blitz", [2]int{0, 0}}} {
		g := game(t, tc.id)
		st := build(stones{
			wRings:   []string{"f5", "a3"},
			bRings:   []string{"k9", "j9", "i9"},
			wMarkers: []string{"b5", "c5", "d5", "e5"},
		}, yinsh.Data{Turn: 1, Score: tc.score})
		st, _ = play(t, g, st, 1, "f5-f8")
		st, _ = play(t, g, st, 1, "row:b5-f5")
		_, res := play(t, g, st, 1, "ring:f8")
		if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 {
			t.Errorf("%s: White should win: %+v", tc.id, res)
		}
	}
}

func TestRandomGamesAreValidAndTerminate(t *testing.T) {
	g := game(t, "yinsh")
	rs := yinsh.New()
	rng := rand.New(rand.NewSource(5))
	results := map[string]int{}
	for n := 0; n < 20; n++ {
		st, _ := rs.Start(g, 1)
		for ply := 0; ply < 1100; ply++ {
			seat := rs.ToMove(g, st)
			if seat == 0 {
				break
			}
			moves := rs.Legal(g, st, seat)
			if len(moves) == 0 {
				t.Fatalf("seat %d has no decision at ply %d", seat, ply)
			}
			mv := moves[rng.Intn(len(moves))]
			text := rs.Format(g, mv)
			back, err := rs.Parse(g, text)
			if err != nil || rs.Format(g, back) != text {
				t.Fatalf("notation %q does not round-trip: %v", text, err)
			}
			res, err := rs.Apply(g, st, seat, mv)
			if err != nil {
				t.Fatalf("legal move %q refused: %v", text, err)
			}
			if err := rs.Validate(g, res.NewState); err != nil {
				t.Fatalf("invalid position after %q: %v", text, err)
			}
			st = res.NewState
			if res.GameEnded {
				break
			}
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
		t.Errorf("%d games did not finish", results["unfinished"])
	}
	t.Logf("%v", results)
}
