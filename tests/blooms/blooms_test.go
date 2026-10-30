// Package blooms_test checks Blooms: the turn structure, fenced blooms and
// captures (including that a player's own fenced blooms wait for the
// opponent's turn), the capture target and the board variants.
package blooms_test

import (
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/blooms"
)

func gameFor(t testing.TB, id string) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(blooms.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("%s is not registered", id)
	}
	return g
}

func game(t testing.TB) *catalog.CompiledGame { return gameFor(t, "blooms") }

var dirs = [6][2]int{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}}

// neighbours lists the cells touching id.
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

// centre is a cell with six neighbours.
func centre(g *catalog.CompiledGame) board.PositionID {
	for _, id := range g.Board.Positions() {
		if len(neighbours(g, id)) == 6 {
			return id
		}
	}
	panic("no interior cell")
}

func build(g *catalog.CompiledGame, stones map[board.PositionID]string, d blooms.Data) *state.BoardState {
	st := state.NewBoardState()
	for id, c := range stones {
		st.Set(id, state.Occupant{Piece: c, Color: c})
	}
	st.SetExt(&d)
	return st
}

func play(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, seat int, text string) (*state.BoardState, *state.MoveResult) {
	t.Helper()
	rs := blooms.New()
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

func data(st *state.BoardState) *blooms.Data { d, _ := st.Ext().(*blooms.Data); return d }

func TestBoardSizesAndTargets(t *testing.T) {
	for id, want := range map[string][2]int{"blooms-4": {37, 15}, "blooms": {61, 20}, "blooms-6": {91, 25}} {
		g := gameFor(t, id)
		if n := len(g.Board.Positions()); n != want[0] {
			t.Errorf("%s: %d cells, want %d", id, n, want[0])
		}
		if n := g.ParamInt("capture_target", 0); n != want[1] {
			t.Errorf("%s: target %d, want %d", id, n, want[1])
		}
	}
}

func TestPlayerOneOpensWithOneStoneThenOneOrTwoEach(t *testing.T) {
	g := game(t)
	rs := blooms.New()
	st, _ := rs.Start(g, 0)
	if rs.ToMove(g, st) != 1 {
		t.Fatal("player 1 starts")
	}
	c := centre(g)
	st, res := play(t, g, st, 1, "red@"+string(c))
	if res.NextTurnSeat != 2 {
		t.Fatalf("player 1's first turn is a single stone: next %d", res.NextTurnSeat)
	}
	// Player 2 may place two stones of different colours.
	nb := neighbours(g, c)
	st, res = play(t, g, st, 2, "blue@"+string(nb[0]))
	if res.NextTurnSeat != 2 {
		t.Fatal("player 2 may place a second stone")
	}
	legal := rs.Legal(g, st, 2)
	for _, mv := range legal {
		if mv.Piece == "blue" {
			t.Fatalf("the second stone must be the other colour: %v", mv)
		}
	}
	hasDone := false
	for _, mv := range legal {
		hasDone = hasDone || mv.Action == "done"
	}
	if !hasDone {
		t.Fatal("the turn can end after one stone")
	}
	st, res = play(t, g, st, 2, "black@"+string(nb[2]))
	if res.NextTurnSeat != 1 {
		t.Fatalf("two stones end the turn: next %d", res.NextTurnSeat)
	}
	if st.Height(nb[2]) != 1 {
		t.Fatal("the stone is on the board")
	}
}

func TestSurroundedBloomIsCapturedAtTheEndOfTheTurn(t *testing.T) {
	g := game(t)
	c := centre(g)
	nb := neighbours(g, c)
	stones := map[board.PositionID]string{c: "blue"}
	for _, n := range nb[:5] {
		stones[n] = "red"
	}
	st := build(g, stones, blooms.Data{Turn: 1, Turns: 4})
	// The last neighbour is the enemy bloom's only liberty: red fills it.
	st, res := play(t, g, st, 1, "yellow@"+string(nb[5]))
	if res.NextTurnSeat == 1 {
		// stage second: end the turn to capture
		st, res = play(t, g, st, 1, "done")
	}
	if _, ok := st.Get(c); ok {
		t.Fatal("the fenced blue stone should be captured")
	}
	if data(st).Captured[0] != 1 {
		t.Errorf("one stone captured: %v", data(st).Captured)
	}
	if res.NextTurnSeat != 2 {
		t.Errorf("then player 2 moves")
	}
}

func TestOwnFencedBloomWaitsForTheOpponentsTurn(t *testing.T) {
	g := game(t)
	c := centre(g)
	nb := neighbours(g, c)
	// Red steps into a hole ringed by blue: it is Red's own bloom that is
	// fenced, so nothing is captured on Red's turn.
	stones := map[board.PositionID]string{}
	for _, n := range nb[:5] {
		stones[n] = "blue"
	}
	st := build(g, stones, blooms.Data{Turn: 1, Turns: 4})
	st, _ = play(t, g, st, 1, "red@"+string(c))
	st, _ = play(t, g, st, 1, "done")
	if _, ok := st.Get(c); !ok {
		t.Fatal("a player's own fenced bloom is not captured on their turn")
	}
	// Blue now closes the last liberty: the red stone is captured.
	st, res := play(t, g, st, 2, "black@"+string(nb[5]))
	if res.NextTurnSeat == 2 {
		st, _ = play(t, g, st, 2, "done")
	}
	if _, ok := st.Get(c); ok {
		t.Fatal("Blue captures the fenced red stone")
	}
	if data(st).Captured[1] != 1 {
		t.Errorf("captures %v", data(st).Captured)
	}
}

func TestOnlySameColourStonesFormABloom(t *testing.T) {
	g := game(t)
	c := centre(g)
	nb := neighbours(g, c)
	// A red stone beside a yellow stone are two blooms: surrounding only the
	// red one captures just it.
	stones := map[board.PositionID]string{c: "red", nb[0]: "yellow"}
	for _, n := range nb[1:] {
		stones[n] = "blue"
	}
	// The red stone's liberties are all filled (by blue and by yellow): but
	// the yellow stone has other empty neighbours, so only red falls.
	st := build(g, stones, blooms.Data{Turn: 2, Turns: 4})
	var spare board.PositionID
	for _, id := range g.Board.Positions() {
		if _, taken := stones[id]; !taken {
			spare = id
			break
		}
	}
	st, res := play(t, g, st, 2, "black@"+string(spare))
	if res.NextTurnSeat == 2 {
		st, _ = play(t, g, st, 2, "done")
	}
	if _, ok := st.Get(c); ok {
		t.Error("the red stone is fenced by blue stones and its yellow neighbour")
	}
	if _, ok := st.Get(nb[0]); !ok {
		t.Error("the yellow stone still has liberties")
	}
}

func TestReachingTheTargetWins(t *testing.T) {
	g := game(t)
	c := centre(g)
	nb := neighbours(g, c)
	stones := map[board.PositionID]string{c: "blue"}
	for _, n := range nb[:5] {
		stones[n] = "red"
	}
	st := build(g, stones, blooms.Data{Turn: 1, Turns: 8, Captured: [2]int{19, 0}})
	st, res := play(t, g, st, 1, "yellow@"+string(nb[5]))
	if !res.GameEnded {
		st, res = play(t, g, st, 1, "done")
	}
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != "captures" {
		t.Fatalf("the 20th captured stone wins: %+v", res)
	}
	if out := blooms.New().Outcome(g, st); !out.Ended {
		t.Error("outcome should report the end")
	}
}

// The pie rule is a choice the table makes when the game is set up
// (pie_rule: "optional"): the plain game never offers the swap, and a table
// that asks plays a game of its own, blooms~pie, in which the second player
// may take the opening side.
func TestPieRuleIsASetupChoice(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(blooms.Module()); err != nil {
		t.Fatal(err)
	}
	id, err := cat.Derive("blooms", map[string]any{"pie-rule": true})
	if err != nil || id != "blooms~pie" {
		t.Fatalf("derive: %q, %v", id, err)
	}
	if _, ok := cat.Get("blooms~pie"); !ok {
		t.Fatal("blooms~pie is not in the catalog")
	}
	// A variant inherits the choice.
	if id, err := cat.Derive("blooms-4", map[string]any{"pie-rule": true}); err != nil || id != "blooms-4~pie" {
		t.Errorf("derive blooms-4: %q, %v", id, err)
	}

	// The plain game has no swap to offer or to accept.
	g, _ := cat.Get("blooms")
	rs := blooms.New()
	st, _ := rs.Start(g, 0)
	st, _ = play(t, g, st, 1, "red@"+string(centre(g)))
	for _, mv := range rs.Legal(g, st, 2) {
		if mv.Action == "swap" {
			t.Error("the plain game offers the swap")
		}
	}
	if _, err := rs.Apply(g, st, 2, state.Move{Action: "swap"}); err == nil {
		t.Error("the plain game accepted a swap")
	}
}

// With the pie rule the second player may answer the opening stone by taking
// it: the stone changes to their colours where it stands, the first player
// replies with an ordinary turn, and the swap is spent.
func TestPieRuleTheSecondPlayerMayTakeTheOpeningSide(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(blooms.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get("blooms~pie")
	if !ok {
		t.Fatal("blooms~pie is not in the catalog")
	}
	rs := blooms.New()

	for first, want := range map[string]string{"red": "blue", "yellow": "black"} {
		st, _ := rs.Start(g, 0)
		c := centre(g)
		st, res := play(t, g, st, 1, first+"@"+string(c))
		if res.NextTurnSeat != 2 {
			t.Fatalf("%s: next %d, want 2", first, res.NextTurnSeat)
		}
		hasSwap := false
		for _, mv := range rs.Legal(g, st, 2) {
			hasSwap = hasSwap || mv.Action == "swap"
		}
		if !hasSwap {
			t.Fatalf("%s: the second player's first reply has no swap", first)
		}
		st, res = play(t, g, st, 2, "swap")
		if res.Notation != "swap" {
			t.Errorf("%s: notation %q, want swap", first, res.Notation)
		}
		occ, ok := st.Get(c)
		if !ok || occ.Color != want || occ.Piece != want {
			t.Errorf("%s: the opening stone is %v, want %s", first, occ, want)
		}
		if res.NextTurnSeat != 1 || rs.ToMove(g, st) != 1 {
			t.Errorf("%s: the first player replies (next %d)", first, res.NextTurnSeat)
		}
		if err := state.Validate(g, st); err != nil {
			t.Errorf("%s: %v", first, err)
		}
		// The swap is spent: for the first player now, and for the second
		// player ever after.
		for _, mv := range rs.Legal(g, st, 1) {
			if mv.Action == "swap" {
				t.Errorf("%s: the first player is offered the swap", first)
			}
		}
		// The reply is an ordinary turn: one stone, then a second or done.
		nb := neighbours(g, c)
		st, res = play(t, g, st, 1, "red@"+string(nb[0]))
		if res.NextTurnSeat != 1 {
			t.Errorf("%s: the reply after a swap is a one-or-two-stone turn (next %d)", first, res.NextTurnSeat)
		}
		st, res = play(t, g, st, 1, "done")
		if res.NextTurnSeat != 2 {
			t.Fatalf("%s: next %d, want 2", first, res.NextTurnSeat)
		}
		for _, mv := range rs.Legal(g, st, 2) {
			if mv.Action == "swap" {
				t.Errorf("%s: the swap is still on offer a turn later", first)
			}
		}
	}
}

// Once the second player has started placing, the swap is off the table.
func TestPieRuleOnlyBeforeTheFirstReply(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(blooms.Module()); err != nil {
		t.Fatal(err)
	}
	g, _ := cat.Get("blooms~pie")
	rs := blooms.New()
	st, _ := rs.Start(g, 0)
	c := centre(g)
	st, _ = play(t, g, st, 1, "red@"+string(c))
	st, res := play(t, g, st, 2, "blue@"+string(neighbours(g, c)[0]))
	if res.NextTurnSeat != 2 {
		t.Fatalf("the second stone of the reply is the second player's: next %d", res.NextTurnSeat)
	}
	for _, mv := range rs.Legal(g, st, 2) {
		if mv.Action == "swap" {
			t.Error("the swap is on offer after the reply has begun")
		}
	}
	if _, err := rs.Apply(g, st, 2, state.Move{Action: "swap"}); err == nil {
		t.Error("a swap was accepted after the reply had begun")
	}
}

func TestRandomGamesAreValidAndEnd(t *testing.T) {
	for _, id := range []string{"blooms-4", "blooms"} {
		g := gameFor(t, id)
		rs := blooms.New()
		rng := rand.New(rand.NewSource(5))
		ended := 0
		for n := 0; n < 30; n++ {
			st, _ := rs.Start(g, 0)
			seat := 1
			for ply := 0; ply < 700; ply++ {
				legal := rs.Legal(g, st, seat)
				if len(legal) == 0 {
					t.Fatalf("%s: no legal move for seat %d", id, seat)
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
					t.Fatal(err)
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
			t.Errorf("%s: only %d of 30 games ended", id, ended)
		}
	}
}
