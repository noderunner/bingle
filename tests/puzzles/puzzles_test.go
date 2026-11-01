package puzzles_test

import (
	"context"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/puzzles"
	"github.com/noderunner/bingle/tests/testdata"
)

func testCatalog(t testing.TB) *catalog.Catalog {
	t.Helper()
	cat := catalog.NewCatalog()
	for _, m := range append(app.BuiltinModules(), testdata.TicTacToe()) {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatal(err)
		}
	}
	return cat
}

func TestSolverFindsAndRefutes(t *testing.T) {
	cat := testCatalog(t)
	g, _ := cat.Get("tak-5x5")
	// White to move; e3 completes a road along row 3.
	st, seat, err := state.ParseTPS(g, "x5/x5/1,1,1,1,x/x5/x5 1 6")
	if err != nil {
		t.Fatal(err)
	}
	s := puzzles.NewSolver(g)
	wins := s.WinningMoves(context.Background(), st, seat, 1)
	var got []string
	for _, mv := range wins {
		got = append(got, state.FormatMove(g, mv))
	}
	// A flat stone or the capstone on e3 both complete the road.
	if len(got) != 2 || got[0] != "e3" || got[1] != "Ce3" {
		t.Fatalf("win-in-1 moves = %v, want [e3 Ce3]", got)
	}

	// With nothing close to a road there is no forced win in one move.
	empty, seat2, _ := state.ParseTPS(g, "x5/x5/x5/x5/x5 1 3")
	if none := puzzles.NewSolver(g).WinningMoves(context.Background(), empty, seat2, 1); len(none) != 0 {
		t.Errorf("empty-ish board has winning moves %v", none)
	}
}

func TestSolverRespectsTheDefender(t *testing.T) {
	cat := testCatalog(t)
	g, _ := cat.Get("tic-tac-toe")
	// X X . / O O . / . . . with X to move: X wins at once at c3? (a3 b3 c3).
	st := state.InitialState(g)
	x, o := "x", "o"
	if sd, ok := g.Seat(1); ok {
		x = sd.Color
	}
	if sd, ok := g.Seat(2); ok {
		o = sd.Color
	}
	for _, p := range []string{"a3", "b3"} {
		st.Set(board.PositionID(p), state.Occupant{Piece: "mark", Color: x})
	}
	for _, p := range []string{"a2", "b2"} {
		st.Set(board.PositionID(p), state.Occupant{Piece: "mark", Color: o})
	}
	s := puzzles.NewSolver(g)
	if wins := s.WinningMoves(context.Background(), st, 1, 1); len(wins) != 1 || state.FormatMove(g, wins[0]) != "c3" {
		t.Fatalf("X's immediate win = %v", wins)
	}
	// With O to move the threat is symmetric; O's win at c2 beats X's.
	if wins := s.WinningMoves(context.Background(), st, 2, 1); len(wins) != 1 || state.FormatMove(g, wins[0]) != "c2" {
		t.Fatalf("O's immediate win = %v", wins)
	}
}

// Every shipped puzzle must be sound: the stored first moves win by force
// within the stated plies, the stored line is legal and ends in a win, and a
// win in fewer plies does not exist (the puzzle is as hard as it claims).
func TestShippedPuzzlesAreSound(t *testing.T) {
	cat := testCatalog(t)
	all, err := puzzles.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Skip("no puzzles shipped")
	}
	seen := map[string]bool{}
	for _, p := range all {
		if seen[p.ID] {
			t.Errorf("duplicate puzzle id %s", p.ID)
		}
		seen[p.ID] = true
		g, ok := cat.Get(p.GameDefID)
		if !ok {
			t.Errorf("%s: unknown game %q", p.ID, p.GameDefID)
			continue
		}
		st := state.NewBoardState()
		if err := st.UnmarshalJSON(p.State); err != nil {
			t.Errorf("%s: bad state: %v", p.ID, err)
			continue
		}
		if err := state.Validate(g, st); err != nil {
			t.Errorf("%s: invalid position: %v", p.ID, err)
			continue
		}
		if p.WinIn < 1 || p.Plies != 2*p.WinIn-1 || len(p.FirstMoves) == 0 || len(p.Solution) == 0 {
			t.Errorf("%s: malformed metadata %+v", p.ID, p)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		solver := puzzles.NewSolver(g)
		for _, text := range p.FirstMoves {
			mv, err := state.ParseMove(g, text)
			if err != nil {
				t.Errorf("%s: first move %q: %v", p.ID, text, err)
				continue
			}
			if !solver.Proves(ctx, st, p.ToMove, mv, p.Plies) {
				t.Errorf("%s: %q does not win by force within %d plies", p.ID, text, p.Plies)
			}
		}
		if p.Plies > 1 {
			if got := solver.WinningMoves(ctx, st, p.ToMove, p.Plies-2); len(got) > 0 {
				t.Errorf("%s: a faster win exists (%d plies): %v", p.ID, p.Plies-2, got)
			}
		}
		cancel()

		// The line replays legally and the attacker wins at its end.
		cur, seat := st, p.ToMove
		var last *state.MoveResult
		for i, text := range p.Solution {
			mv, err := state.ParseMove(g, text)
			if err != nil {
				t.Errorf("%s: solution move %d %q: %v", p.ID, i+1, text, err)
				break
			}
			res, err := state.Apply(g, cur, seat, mv)
			if err != nil {
				t.Errorf("%s: solution move %d %q illegal: %v", p.ID, i+1, text, err)
				break
			}
			last = res
			cur, seat = res.NewState, res.NextTurnSeat
		}
		if last == nil || !last.GameEnded || last.WinnerSeat == nil || *last.WinnerSeat != p.ToMove {
			t.Errorf("%s: the solution line does not end in a win for seat %d", p.ID, p.ToMove)
		}
		if p.TPS != "" {
			if _, _, err := state.ParseTPS(g, p.TPS); err != nil {
				t.Errorf("%s: TPS %q does not parse: %v", p.ID, p.TPS, err)
			}
		}
	}
}

// The principal variation of a win in two ends with the attacker's winning
// move and alternates the two sides.
func TestLineProducesAWinningVariation(t *testing.T) {
	all, err := puzzles.All()
	if err != nil || len(all) == 0 {
		t.Skip("no puzzles")
	}
	cat := testCatalog(t)
	checked := 0
	for _, p := range all {
		if p.WinIn != 2 {
			continue
		}
		g, _ := cat.Get(p.GameDefID)
		st := state.NewBoardState()
		if err := st.UnmarshalJSON(p.State); err != nil {
			t.Fatal(err)
		}
		mv, err := state.ParseMove(g, p.FirstMoves[0])
		if err != nil {
			t.Fatal(err)
		}
		line := puzzles.NewSolver(g).Line(context.Background(), st, p.ToMove, mv, p.Plies)
		if len(line) != 3 || line[0] != p.FirstMoves[0] {
			t.Errorf("%s: line = %v, want 3 plies starting with %q", p.ID, line, p.FirstMoves[0])
		}
		if checked++; checked >= 5 {
			break
		}
	}
	if checked == 0 {
		t.Skip("no win-in-two puzzles")
	}
}
