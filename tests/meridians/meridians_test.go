// Package meridians_test checks Meridians' rules: the board with no middle
// point, the opening, placing where your stones can see, the death of a group
// that can see no friend, and winning by taking every enemy stone off.
package meridians_test

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/meridians"
)

const (
	light = 1
	dark  = 2
)

func game(t testing.TB) *catalog.CompiledGame {
	t.Helper()
	return gameID(t, "meridians")
}

func gameID(t testing.TB, id string) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(meridians.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("%s is not registered", id)
	}
	return g
}

// position builds a position: the stones of each side, whose turn it is and how
// many turns each has taken (which the opening rules and the removal depend on).
type position struct {
	light, dark []string
	turn        int
	turns       [2]int
	first       [2]string
}

func build(t testing.TB, g *catalog.CompiledGame, p position) *state.BoardState {
	t.Helper()
	st := state.NewBoardState()
	for _, id := range p.light {
		st.Set(board.PositionID(id), state.Occupant{Piece: "stone", Color: "white"})
	}
	for _, id := range p.dark {
		st.Set(board.PositionID(id), state.Occupant{Piece: "stone", Color: "black"})
	}
	turn := p.turn
	if turn == 0 {
		turn = light
	}
	st.SetExt(&meridians.Data{Turn: turn, Turns: p.turns, First: p.first})
	if err := state.Validate(g, st); err != nil {
		t.Fatalf("the position is not valid: %v", err)
	}
	return st
}

func moves(t testing.TB, g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	t.Helper()
	var out []string
	for _, mv := range state.LegalMoves(g, st, seat) {
		out = append(out, state.FormatMove(g, mv))
	}
	sort.Strings(out)
	return out
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func play(t testing.TB, g *catalog.CompiledGame, st *state.BoardState, seat int, notation string) *state.MoveResult {
	t.Helper()
	mv, err := state.ParseMove(g, notation)
	if err != nil {
		t.Fatalf("parse %q: %v", notation, err)
	}
	res, err := state.Apply(g, st, seat, mv)
	if err != nil {
		t.Fatalf("play %q: %v", notation, err)
	}
	return res
}

func stones(st *state.BoardState, color string) []string {
	var out []string
	for _, id := range st.Occupied() {
		if occ, _ := st.Get(id); occ.Color == color {
			out = append(out, string(id))
		}
	}
	sort.Strings(out)
	return out
}

// The board is a hexagon of triangles with no middle point: 114 points, six
// intersections along two sides and seven along the other four, and the two
// smaller boards are there as variants.
func TestBoard_IsACentrelessHexagon(t *testing.T) {
	for id, want := range map[string]int{"meridians": 114, "meridians-5": 80, "meridians-7": 154} {
		g := gameID(t, id)
		if n := len(g.Board.Positions()); n != want {
			t.Errorf("%s has %d points, want %d", id, n, want)
		}
	}
	g := game(t)
	// Rows of 6, 7, 8, 9, 10, 11, 12, 11, 10, 9, 8, 7, 6.
	rows := map[int]int{}
	for _, id := range g.Board.Positions() {
		c, _ := g.Board.Coord(id)
		rows[c.Y]++
	}
	var sizes []string
	for r := 0; r < len(rows); r++ {
		sizes = append(sizes, fmt.Sprint(rows[r]))
	}
	if got := strings.Join(sizes, ","); got != "6,7,8,9,10,11,12,11,10,9,8,7,6" {
		t.Errorf("the rows are %s", got)
	}
	// No point is the middle: the half turn of the board has no fixed point,
	// which is what "centreless" means.
	for _, id := range g.Board.Positions() {
		c, _ := g.Board.Coord(id)
		if 2*c.X == 11 && 2*c.Y == 12 {
			t.Errorf("%s is a middle point", id)
		}
	}
	// Every point touches at most six others, and the corners only three.
	for _, id := range g.Board.Positions() {
		if n := len(g.Board.Neighbors(id)); n < 3 || n > 6 {
			t.Errorf("%s touches %d points", id, n)
		}
	}
}

// The first stone goes anywhere; the second must not touch the first; after
// that a stone goes where one of your own can see along a straight line.
func TestOpening_FirstAnywhereThenNotTouching(t *testing.T) {
	g := game(t)
	st := state.InitialState(g)
	if got := len(state.LegalMoves(g, st, light)); got != 114 {
		t.Fatalf("Light may open on %d points, want every one of the 114", got)
	}
	if len(state.LegalMoves(g, st, dark)) != 0 {
		t.Error("Dark may move before Light has")
	}
	res := play(t, g, st, light, "g7")
	st = res.NewState
	// Dark has no stones, so Dark too may play anywhere (but not on g7).
	dmoves := moves(t, g, st, dark)
	if len(dmoves) != 113 || has(dmoves, "g7") {
		t.Fatalf("Dark may open on %d points (g7 among them: %v)", len(dmoves), has(dmoves, "g7"))
	}
	st = play(t, g, st, dark, "a7").NewState

	// Light's second stone: in sight of g7 (any straight line, nothing in the
	// way) and not touching it.
	lmoves := moves(t, g, st, light)
	for _, touching := range g.Board.Neighbors(board.PositionID("g7")) {
		if has(lmoves, string(touching)) {
			t.Errorf("Light may lay a second stone on %s, which touches the first", touching)
		}
	}
	// Along the row from g7: h7 touches it, so i7 is the nearest legal point.
	if !has(lmoves, "i7") {
		t.Errorf("i7 is not offered: %v", lmoves)
	}
	// Dark's stone on a7 blocks the line to the left of g7 beyond it.
	if has(lmoves, "a7") {
		t.Error("Light may lay a stone where Dark's already is")
	}
	// A point off every line from g7 is not offered: h8 is a neighbour of g7's
	// neighbour but not on a line through g7.
	for _, off := range []string{"i8", "h9"} {
		if has(lmoves, off) {
			t.Errorf("%s is offered, but no stone of Light's can see it", off)
		}
	}
}

// A stone may not be laid past an enemy stone: the line of sight stops there.
func TestSight_EnemyStonesBlockTheLine(t *testing.T) {
	g := game(t)
	// Both sides are well past their opening, so only sight is being tested.
	st := build(t, g, position{
		light: []string{"d7"},
		dark:  []string{"f7"},
		turn:  light,
		turns: [2]int{3, 3},
	})
	lmoves := moves(t, g, st, light)
	if has(lmoves, "e7") == false {
		t.Error("e7, the empty point before the enemy stone, is not offered")
	}
	for _, past := range []string{"g7", "h7", "i7"} {
		if has(lmoves, past) {
			t.Errorf("%s is offered, but Dark's stone on f7 is in the way", past)
		}
	}
	// Along another line from d7 nothing blocks: c7 and b7 are both in sight.
	for _, open := range []string{"c7", "b7", "a7"} {
		if !has(lmoves, open) {
			t.Errorf("%s is not offered, though the line is clear", open)
		}
	}
}

// A group that can see no other group of its colour comes off the board when
// its owner's turn ends — and a group the opponent cut off can be saved.
func TestDeath_AGroupWithNoPathComesOff(t *testing.T) {
	g := game(t)
	// Light has two groups that see each other along the row (d7 … g7) and a
	// third stone on no line with either of them: a8 lies on none of the three
	// lines through d7 or g7, so it can see no friend at all.
	st := build(t, g, position{
		light: []string{"d7", "g7", "a8"},
		dark:  []string{"a7"},
		turn:  light,
		turns: [2]int{3, 3},
	})
	// Light lays a stone that does not save k3: its own turn takes k3 off.
	res := play(t, g, st, light, "e7")
	left := stones(res.NewState, "white")
	if has(left, "a8") {
		t.Errorf("a8 can see no friend but is still on the board: %v", left)
	}
	if !has(left, "d7") || !has(left, "g7") || !has(left, "e7") {
		t.Errorf("the groups that see each other were taken off: %v", left)
	}
	// Saving it instead: a stone on a line between k3 and the others keeps it.
	st2 := build(t, g, position{
		light: []string{"d7", "g7", "a8"},
		dark:  []string{"a7"},
		turn:  light,
		turns: [2]int{3, 3},
	})
	// A stone that gives a8 a path keeps it: there is such a move, and after it
	// every one of Light's four stones is still on the board.
	saved := ""
	for _, mv := range state.LegalMoves(g, st2, light) {
		if mv.Action != "place" {
			continue
		}
		res := play(t, g, st2, light, state.FormatMove(g, mv))
		if has(stones(res.NewState, "white"), "a8") && len(stones(res.NewState, "white")) == 4 {
			saved = string(mv.To)
			break
		}
	}
	if saved == "" {
		t.Error("no stone of Light's could save a8")
	}
}

// Nothing comes off before a player has had two turns: a single stone can see
// no friend, which is why the opening is exempt.
func TestDeath_TheOpeningStonesAreExempt(t *testing.T) {
	g := game(t)
	st := state.InitialState(g)
	st = play(t, g, st, light, "g7").NewState
	if got := stones(st, "white"); len(got) != 1 {
		t.Fatalf("Light's first stone came off at once: %v", got)
	}
	st = play(t, g, st, dark, "a7").NewState
	if got := stones(st, "black"); len(got) != 1 {
		t.Fatalf("Dark's first stone came off at once: %v", got)
	}
	// Light's second stone must leave both alive: it is in sight and not
	// touching, so the two groups see each other.
	st = play(t, g, st, light, "i7").NewState
	if got := stones(st, "white"); len(got) != 2 {
		t.Errorf("after Light's second stone: %v", got)
	}
}

// The game is won when the loser's turn would begin with no stones of theirs.
func TestWinning_TakingEveryStoneOff(t *testing.T) {
	g := game(t)
	// Dark has one group of two stones with no friend to see; Light cuts the
	// only line it could have used... in fact a lone group is already dead, so
	// Dark's own turn takes it off and Dark has nothing left.
	st := build(t, g, position{
		light: []string{"d7", "g7"},
		dark:  []string{"a11", "b11"},
		turn:  dark,
		turns: [2]int{3, 3},
	})
	// Dark lays a stone that does not give the group a path: everything of
	// Dark's comes off and Light wins.
	var ended *state.MoveResult
	for _, mv := range state.LegalMoves(g, st, dark) {
		res := play(t, g, st, dark, state.FormatMove(g, mv))
		if res.GameEnded {
			ended = res
			break
		}
	}
	if ended == nil {
		t.Fatal("no move of Dark's ended the game, though every group of Dark's is dead")
	}
	if ended.EndReason != "wiped-out" || ended.WinnerSeat == nil || *ended.WinnerSeat != light {
		t.Errorf("the game ended as %q, winner %v", ended.EndReason, ended.WinnerSeat)
	}
	if got := stones(ended.NewState, "black"); len(got) != 0 {
		t.Errorf("Dark still has %v", got)
	}
}

// A game of random moves ends, and ends the way the rules say: one side with no
// stones left (or the move limit, which is a draw).
func TestRandomGamesEnd(t *testing.T) {
	g := gameID(t, "meridians-5")
	for seed := int64(1); seed <= 20; seed++ {
		rng := rand.New(rand.NewSource(seed))
		st := state.InitialState(g)
		seat := light
		ended := false
		for ply := 0; ply < 1200 && !ended; ply++ {
			legal := state.LegalMoves(g, st, seat)
			if len(legal) == 0 {
				t.Fatalf("seed %d, ply %d: seat %d has no move at all", seed, ply, seat)
			}
			res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
			if err != nil {
				t.Fatalf("seed %d, ply %d: %v", seed, ply, err)
			}
			st, seat, ended = res.NewState, res.NextTurnSeat, res.GameEnded
			if ended {
				switch res.EndReason {
				case "wiped-out":
					if res.WinnerSeat == nil {
						t.Errorf("seed %d: wiped-out with no winner", seed)
					}
				case "ply-limit", "stuck":
				default:
					t.Errorf("seed %d ended as %q", seed, res.EndReason)
				}
			}
		}
		if !ended {
			t.Errorf("seed %d did not end in 1200 plies", seed)
		}
		if err := state.Validate(g, st); err != nil {
			t.Errorf("seed %d: the final position is invalid: %v", seed, err)
		}
	}
}

// The notation is the point, and it reads back.
func TestNotation_IsThePoint(t *testing.T) {
	g := game(t)
	st := state.InitialState(g)
	mv, err := state.ParseMove(g, "G7")
	if err != nil {
		t.Fatal(err)
	}
	if got := state.FormatMove(g, mv); got != "g7" {
		t.Errorf("G7 reads as %q", got)
	}
	if _, err := state.ParseMove(g, "z9"); err == nil {
		t.Error("z9 was accepted, though it is off the board")
	}
	res := play(t, g, st, light, "g7")
	if res.Notation != "g7" {
		t.Errorf("the move was recorded as %q", res.Notation)
	}
}
