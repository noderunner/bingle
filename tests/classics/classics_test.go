// Package classics_test exercises the JSON-only classic games (Tic-Tac-Toe,
// Connect Four, Gomoku): they use the same engine primitives as Tak but
// different ones — supply placement, gravity and "line" win rules.
package classics_test

import (
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/classics"
)

func boardID(s string) board.PositionID { return board.PositionID(s) }

func load(t testing.TB, id string) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	for _, m := range classics.Modules() {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatalf("RegisterModule(%s): %v", m.ID, err)
		}
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("no game %q", id)
	}
	return g
}

// playAll applies moves alternately starting with seat 1 and returns the
// last result.
func playAll(t testing.TB, g *catalog.CompiledGame, moves ...string) *state.MoveResult {
	t.Helper()
	st, seat := state.InitialState(g), 1
	var res *state.MoveResult
	for i, text := range moves {
		mv, err := state.ParseMove(g, text)
		if err != nil {
			t.Fatalf("move %d %q: parse: %v", i+1, text, err)
		}
		res, err = state.Apply(g, st, seat, mv)
		if err != nil {
			t.Fatalf("move %d %q: %v", i+1, text, err)
		}
		st, seat = res.NewState, res.NextTurnSeat
	}
	return res
}

func TestTicTacToeDiagonalWin(t *testing.T) {
	g := load(t, "tic-tac-toe")
	res := playAll(t, g, "a3", "b3", "b2", "c3", "c1")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != "line" {
		t.Errorf("result = ended %v winner %v reason %q, want seat 1 by line", res.GameEnded, res.WinnerSeat, res.EndReason)
	}
}

func TestTicTacToeDraw(t *testing.T) {
	g := load(t, "tic-tac-toe")
	//  X O X
	//  X O O
	//  O X X
	res := playAll(t, g, "a3", "b3", "c3", "b2", "a2", "c2", "b1", "a1", "c1")
	if !res.GameEnded || res.WinnerSeat != nil || res.EndReason != "draw" {
		t.Errorf("result = ended %v winner %v reason %q, want a draw", res.GameEnded, res.WinnerSeat, res.EndReason)
	}
}

func TestTicTacToeNotationIsBareSquare(t *testing.T) {
	g := load(t, "tic-tac-toe")
	mv, err := state.ParseMove(g, "b2")
	if err != nil {
		t.Fatal(err)
	}
	if mv.Action != "place" || mv.Piece != "mark" || mv.To != "b2" {
		t.Errorf("parsed %+v", mv)
	}
	if got := state.FormatMove(g, mv); got != "b2" {
		t.Errorf("FormatMove = %q, want b2", got)
	}
}

func TestConnectFourGravity(t *testing.T) {
	g := load(t, "connect-four")
	st := state.InitialState(g)
	if n := len(state.LegalMoves(g, st, 1)); n != 7 {
		t.Errorf("legal moves at the start = %d, want 7 (one per column)", n)
	}
	// Only the bottom square of an empty column may be played.
	mv, _ := state.ParseMove(g, "a3")
	if _, err := state.Apply(g, st, 1, mv); err == nil {
		t.Error("placing at a3 over an empty column should be rejected")
	}
	// After a1 the next square up is available.
	res := playAll(t, g, "a1")
	if n := len(state.LegalMoves(g, res.NewState, 2)); n != 7 {
		t.Errorf("legal moves after a1 = %d, want 7", n)
	}
	mv, _ = state.ParseMove(g, "a2")
	if _, err := state.Apply(g, res.NewState, 2, mv); err != nil {
		t.Errorf("a2 should be playable on top of a1: %v", err)
	}
}

func TestConnectFourVerticalWin(t *testing.T) {
	g := load(t, "connect-four")
	res := playAll(t, g, "a1", "b1", "a2", "b2", "a3", "b3", "a4")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 {
		t.Errorf("ended %v winner %v, want seat 1", res.GameEnded, res.WinnerSeat)
	}
}

func TestConnectFourDiagonalWin(t *testing.T) {
	g := load(t, "connect-four")
	// Red climbs a diagonal: a1, b2, c3, d4. Yellow supplies the filler
	// discs underneath: b1, c1, c2, d1, d2, d3.
	res := playAll(t, g,
		"a1", "b1", // red a1, yellow b1
		"b2", "c1", // red b2, yellow c1
		"d1", "c2", // red d1 (filler), yellow c2
		"c3", "d2", // red c3, yellow d2
		"e1", "d3", // red e1 (filler), yellow d3
		"d4", // red d4 completes a1-b2-c3-d4
	)
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 {
		t.Errorf("ended %v winner %v, want red to win on the a1-d4 diagonal\n", res.GameEnded, res.WinnerSeat)
	}
}

func TestGomokuFiveInARow(t *testing.T) {
	g := load(t, "gomoku")
	res := playAll(t, g, "h8", "a1", "i8", "a2", "j8", "a3", "k8", "a4", "l8")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != "five" {
		t.Errorf("ended %v winner %v reason %q", res.GameEnded, res.WinnerSeat, res.EndReason)
	}
}

// TestRandomPlayoutsAreConsistent plays random games of each classic and
// checks LegalMoves against Apply and that every game terminates.
func TestRandomPlayoutsAreConsistent(t *testing.T) {
	for _, id := range []string{"tic-tac-toe", "connect-four", "gomoku"} {
		g := load(t, id)
		rng := rand.New(rand.NewSource(7))
		games := 40
		if id == "gomoku" {
			games = 4
		}
		for n := 0; n < games; n++ {
			st, seat := state.InitialState(g), 1
			for ply := 0; ply < 400; ply++ {
				moves := state.LegalMoves(g, st, seat)
				if len(moves) == 0 {
					t.Fatalf("%s: no legal moves in an unfinished game", id)
				}
				mv := moves[rng.Intn(len(moves))]
				text := state.FormatMove(g, mv)
				back, err := state.ParseMove(g, text)
				if err != nil || back.Key() != mv.Key() {
					t.Fatalf("%s: notation %q round trip: %+v vs %+v (%v)", id, text, back, mv, err)
				}
				res, err := state.Apply(g, st, seat, mv)
				if err != nil {
					t.Fatalf("%s: legal move %q rejected: %v", id, text, err)
				}
				st, seat = res.NewState, res.NextTurnSeat
				if res.GameEnded {
					break
				}
				if ply == 399 {
					t.Fatalf("%s: game did not finish in 400 plies", id)
				}
			}
		}
	}
}

func TestHexBoardTopology(t *testing.T) {
	g := load(t, "hex-11")
	if g.Board.Kind() != "hex" || g.Board.Len() != 121 {
		t.Fatalf("board kind %q with %d cells, want an 11x11 hex board", g.Board.Kind(), g.Board.Len())
	}
	// Interior cells have six neighbours, edge cells four; the acute corners
	// (top-left a1, bottom-right k11) have two and the obtuse corners
	// (top-right k1, bottom-left a11) three.
	want := map[string]int{"f6": 6, "f1": 4, "a6": 4, "a1": 2, "k11": 2, "k1": 3, "a11": 3}
	for id, n := range want {
		if got := len(g.Board.Neighbors(boardID(id))); got != n {
			t.Errorf("%s has %d neighbours, want %d", id, got, n)
		}
	}
	for _, region := range []string{"top", "bottom", "left", "right"} {
		ids, ok := g.Board.Region(region)
		if !ok || len(ids) != 11 {
			t.Errorf("region %s = %d cells, want 11", region, len(ids))
		}
	}
}

func TestHexEachPlayerConnectsTheirOwnEdges(t *testing.T) {
	g := load(t, "hex-7")
	// Red (seat 1) plays down column d (top to bottom); Blue answers
	// elsewhere without connecting left-right.
	moves := []string{"d1", "a1", "d2", "a2", "d3", "a3", "d4", "a4", "d5", "a5", "d6", "a6", "d7"}
	res := playAll(t, g, moves...)
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != "connection" {
		t.Fatalf("ended %v winner %v reason %q, want Red by connection", res.GameEnded, res.WinnerSeat, res.EndReason)
	}

	// Blue connecting left-right wins for Blue; Red's top-bottom pair does
	// not apply to Blue and vice versa.
	moves = []string{"a1", "a4", "b1", "b4", "c1", "c4", "d1", "d4", "e1", "e4", "f1", "f4", "g1", "g4"}
	// Red tries a left-right chain along row 1 (a1..g1): that must NOT win
	// for Red; Blue completes row 4 left-right and wins.
	res = playAll(t, g, moves...)
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 2 {
		t.Errorf("ended %v winner %v, want Blue to win by connecting left and right", res.GameEnded, res.WinnerSeat)
	}
}

func TestHexRandomGamesAlwaysHaveAWinner(t *testing.T) {
	g := load(t, "hex-7")
	rng := rand.New(rand.NewSource(3))
	for n := 0; n < 30; n++ {
		st, seat := state.InitialState(g), 1
		for {
			moves := state.LegalMoves(g, st, seat)
			if len(moves) == 0 {
				t.Fatal("no moves in an unfinished game")
			}
			res, err := state.Apply(g, st, seat, moves[rng.Intn(len(moves))])
			if err != nil {
				t.Fatal(err)
			}
			st, seat = res.NewState, res.NextTurnSeat
			if res.GameEnded {
				if res.WinnerSeat == nil {
					t.Fatal("Hex cannot end in a draw")
				}
				break
			}
		}
	}
}

// ---- draughts ---------------------------------------------------------------

func draughtsPosition(t testing.TB, g *catalog.CompiledGame, cells map[string][]string) *state.BoardState {
	t.Helper()
	st := state.NewBoardState()
	for pos, pieces := range cells {
		for _, p := range pieces {
			// "bm" = black man, "rk" = red king.
			color := map[byte]string{'b': "black", 'r': "red"}[p[0]]
			piece := map[byte]string{'m': "man", 'k': "king"}[p[1]]
			st.Push(boardID(pos), state.Occupant{Piece: piece, Color: color})
		}
	}
	return st
}

func TestDraughtsStartingPositionAndOpeningMoves(t *testing.T) {
	g := load(t, "draughts")
	st := state.InitialState(g)
	if n := len(st.Occupied()); n != 24 {
		t.Fatalf("pieces at the start = %d, want 24", n)
	}
	// Black (bottom) has 7 opening moves in English draughts.
	moves := state.LegalMoves(g, st, 1)
	if len(moves) != 7 {
		t.Errorf("opening moves = %d, want 7: %v", len(moves), notations(g, moves))
	}
	// Men never move backwards or sideways: only forward diagonals.
	for _, m := range moves {
		if m.Action != "step" {
			t.Errorf("unexpected action %q", m.Action)
		}
	}
	// Red's reply count is symmetric.
	res, err := state.Apply(g, st, 1, moves[0])
	if err != nil {
		t.Fatal(err)
	}
	if n := len(state.LegalMoves(g, res.NewState, 2)); n != 7 {
		t.Errorf("red's replies = %d, want 7", n)
	}
}

func notations(g *catalog.CompiledGame, moves []state.Move) []string {
	out := make([]string, len(moves))
	for i, m := range moves {
		out[i] = state.FormatMove(g, m)
	}
	return out
}

func TestDraughtsCaptureIsMandatory(t *testing.T) {
	g := load(t, "draughts")
	// Black man on c3 can jump the red man on d4 (landing e5); another black
	// man on a1 has an ordinary step available but must not take it.
	st := draughtsPosition(t, g, map[string][]string{
		"c3": {"bm"}, "d4": {"rm"}, "a1": {"bm"}, "h8": {"rm"},
	})
	moves := state.LegalMoves(g, st, 1)
	got := notations(g, moves)
	if len(got) != 1 || got[0] != "c3xe5" {
		t.Fatalf("legal moves = %v, want only the forced capture c3xe5", got)
	}
	if _, err := state.Apply(g, st, 1, state.Move{From: "a1", To: "b2"}); err == nil {
		t.Error("a plain step was accepted although a capture is available")
	}
	res, err := state.Apply(g, st, 1, moves[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.NewState.Get("d4"); ok {
		t.Error("the jumped piece was not removed")
	}
	if occ, ok := res.NewState.Get("e5"); !ok || occ.Color != "black" {
		t.Errorf("e5 = %+v ok=%v, want the capturing man", occ, ok)
	}
	if len(res.Captures) != 1 || res.Captures[0] != "d4" {
		t.Errorf("captures = %v, want [d4]", res.Captures)
	}
}

func TestDraughtsMultiJumpMustBePlayedToTheEnd(t *testing.T) {
	g := load(t, "draughts")
	// Black man c1 jumps d2 -> e3, then f4 -> g5 in one move.
	st := draughtsPosition(t, g, map[string][]string{
		"c1": {"bm"}, "d2": {"rm"}, "f4": {"rm"}, "h8": {"rm"},
	})
	got := notations(g, state.LegalMoves(g, st, 1))
	if len(got) != 1 || got[0] != "c1xe3xg5" {
		t.Fatalf("legal moves = %v, want only the full chain c1xe3xg5", got)
	}
	// Stopping after the first hop is illegal.
	short, _ := state.ParseMove(g, "c1xe3")
	if _, err := state.Apply(g, st, 1, short); err == nil {
		t.Error("a chain that stops early was accepted")
	}
	full, err := state.ParseMove(g, "c1xe3xg5")
	if err != nil {
		t.Fatal(err)
	}
	res, err := state.Apply(g, st, 1, full)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Captures) != 2 {
		t.Errorf("captures = %v, want two", res.Captures)
	}
	// Steps for the UI: from then each landing.
	steps := state.Steps(g, res.Move)
	if len(steps) != 3 || steps[0].Value != "c1" || steps[2].Value != "g5" {
		t.Errorf("steps = %+v", steps)
	}
}

func TestDraughtsManPromotesAndKingMovesBackwards(t *testing.T) {
	g := load(t, "draughts")
	st := draughtsPosition(t, g, map[string][]string{"b7": {"bm"}, "h2": {"rm"}})
	mv, _ := state.ParseMove(g, "b7-a8")
	res, err := state.Apply(g, st, 1, mv)
	if err != nil {
		t.Fatal(err)
	}
	if occ, _ := res.NewState.Get("a8"); occ.Piece != "king" {
		t.Fatalf("a8 holds %+v, want a king after reaching the far row", occ)
	}
	// Red replies elsewhere; the new king may then step backwards (a8-b7).
	st2 := res.NewState
	r2, err := state.Apply(g, st2, 2, mustParse(t, g, "h2-g1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.Apply(g, r2.NewState, 1, mustParse(t, g, "a8-b7")); err != nil {
		t.Errorf("a king moving backwards was rejected: %v", err)
	}
	// A man may not step backwards.
	st3 := draughtsPosition(t, g, map[string][]string{"c3": {"bm"}, "h8": {"rm"}})
	if _, err := state.Apply(g, st3, 1, mustParse(t, g, "c3-b2")); err == nil {
		t.Error("a man stepped backwards")
	}
}

func mustParse(t testing.TB, g *catalog.CompiledGame, text string) state.Move {
	t.Helper()
	mv, err := state.ParseMove(g, text)
	if err != nil {
		t.Fatalf("ParseMove(%q): %v", text, err)
	}
	return mv
}

func TestDraughtsBlockedPlayerLoses(t *testing.T) {
	g := load(t, "draughts")
	// Red's only man on a8... is already against the edge with its way
	// blocked: black captures nothing, but red has no legal move afterwards.
	st := draughtsPosition(t, g, map[string][]string{
		"a8": {"rm"}, "b7": {"bm"}, "c6": {"bm"}, "h2": {"bm"},
	})
	// Black makes a harmless move; red's a8 man (moving down: forward-right
	// is b7, occupied, with c6 behind it occupied so no jump) is stuck.
	res, err := state.Apply(g, st, 1, mustParse(t, g, "h2-g3"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 {
		t.Errorf("ended %v winner %v, want black to win because red cannot move", res.GameEnded, res.WinnerSeat)
	}
}

func TestDraughtsRandomGamesTerminateConsistently(t *testing.T) {
	g := load(t, "draughts")
	rng := rand.New(rand.NewSource(11))
	for n := 0; n < 20; n++ {
		st, seat := state.InitialState(g), 1
		for ply := 0; ply < 250; ply++ {
			moves := state.LegalMoves(g, st, seat)
			if len(moves) == 0 {
				t.Fatal("no moves in an unfinished game")
			}
			// Every listed move must be accepted, and notation must round-trip.
			for i, mv := range moves {
				if i > 6 {
					break
				}
				back, err := state.ParseMove(g, state.FormatMove(g, mv))
				if err != nil {
					t.Fatalf("parse %q: %v", state.FormatMove(g, mv), err)
				}
				back, _ = back.Normalize(g)
				if back.Key() != mv.Key() {
					t.Fatalf("notation round trip: %+v vs %+v", back, mv)
				}
				if _, err := state.Apply(g, st, seat, mv); err != nil {
					t.Fatalf("legal move %s rejected: %v", state.FormatMove(g, mv), err)
				}
			}
			res, err := state.Apply(g, st, seat, moves[rng.Intn(len(moves))])
			if err != nil {
				t.Fatal(err)
			}
			st, seat = res.NewState, res.NextTurnSeat
			if res.GameEnded {
				break
			}
			if ply == 249 {
				t.Fatal("a game never ended despite the ply limit")
			}
		}
	}
}

// ---- breakthrough -------------------------------------------------------------

func TestBreakthroughMovesAndCaptures(t *testing.T) {
	g := load(t, "breakthrough")
	st := state.InitialState(g)
	if n := len(st.Occupied()); n != 32 {
		t.Fatalf("pieces = %d, want 32", n)
	}
	// White's opening moves: each of 8 pawns on rank 2 can step straight (8)
	// or diagonally (14 on-board diagonals) onto the empty rank 3: 8 + 14.
	if n := len(state.LegalMoves(g, st, 1)); n != 22 {
		t.Errorf("white opening moves = %d, want 22", n)
	}

	// Straight ahead never captures; diagonal does.
	cells := state.NewBoardState()
	cells.Push("d4", state.Occupant{Piece: "pawn", Color: "white"})
	cells.Push("d5", state.Occupant{Piece: "pawn", Color: "black"})
	cells.Push("e5", state.Occupant{Piece: "pawn", Color: "black"})
	if _, err := state.Apply(g, cells, 1, mustParse(t, g, "d4-d5")); err == nil {
		t.Error("a pawn captured straight ahead")
	}
	res, err := state.Apply(g, cells, 1, mustParse(t, g, "d4-e5"))
	if err != nil {
		t.Fatalf("diagonal capture rejected: %v", err)
	}
	if res.Captured == nil || *res.Captured != "e5" {
		t.Errorf("captured = %v, want e5", res.Captured)
	}
	// Backwards and sideways are never allowed.
	if _, err := state.Apply(g, cells, 1, mustParse(t, g, "d4-c4")); err == nil {
		t.Error("a pawn moved sideways")
	}
	if _, err := state.Apply(g, cells, 1, mustParse(t, g, "d4-d3")); err == nil {
		t.Error("a pawn moved backwards")
	}
}

func TestBreakthroughReachingTheFarRowWins(t *testing.T) {
	g := load(t, "breakthrough")
	st := state.NewBoardState()
	st.Push("c7", state.Occupant{Piece: "pawn", Color: "white"})
	st.Push("h1", state.Occupant{Piece: "pawn", Color: "black"})
	res, err := state.Apply(g, st, 1, mustParse(t, g, "c7-c8"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != "breakthrough" {
		t.Errorf("ended %v winner %v reason %q", res.GameEnded, res.WinnerSeat, res.EndReason)
	}
}

func TestBreakthroughRandomGamesHaveAWinner(t *testing.T) {
	g := load(t, "breakthrough")
	rng := rand.New(rand.NewSource(5))
	for n := 0; n < 20; n++ {
		st, seat := state.InitialState(g), 1
		for ply := 0; ply < 400; ply++ {
			moves := state.LegalMoves(g, st, seat)
			if len(moves) == 0 {
				t.Fatal("no legal moves in an unfinished game")
			}
			res, err := state.Apply(g, st, seat, moves[rng.Intn(len(moves))])
			if err != nil {
				t.Fatal(err)
			}
			st, seat = res.NewState, res.NextTurnSeat
			if res.GameEnded {
				if res.WinnerSeat == nil {
					t.Fatal("Breakthrough cannot be drawn")
				}
				break
			}
			if ply == 399 {
				t.Fatal("game did not end")
			}
		}
	}
}

// ---- reversi ----------------------------------------------------------------

func TestReversiOpeningMovesAndFlips(t *testing.T) {
	g := load(t, "reversi")
	st := state.InitialState(g)
	moves := state.LegalMoves(g, st, 1)
	got := map[string]bool{}
	for _, m := range moves {
		got[state.FormatMove(g, m)] = true
	}
	// Black's four classic opening moves.
	for _, want := range []string{"d3", "c4", "f5", "e6"} {
		if !got[want] {
			t.Errorf("missing opening move %s (have %v)", want, got)
		}
	}
	if len(moves) != 4 {
		t.Errorf("opening moves = %d, want 4", len(moves))
	}

	res, err := state.Apply(g, st, 1, mustParse(t, g, "d3"))
	if err != nil {
		t.Fatal(err)
	}
	// d3 brackets d4 (white) against d5 (black): d4 flips.
	if occ, _ := res.NewState.Get("d4"); occ.Color != "black" {
		t.Errorf("d4 = %+v, want flipped to black", occ)
	}
	if n := len(res.NewState.Occupied()); n != 5 {
		t.Errorf("discs = %d, want 5", n)
	}
	// A placement that flips nothing is illegal.
	if _, err := state.Apply(g, st, 1, mustParse(t, g, "a1")); err == nil {
		t.Error("a1 flips nothing but was accepted")
	}
}

func TestReversiEndsWhenNobodyCanMove(t *testing.T) {
	g := load(t, "reversi")
	// Row 1 = _ W B and a far-away black disc. Black plays a1, flipping b1;
	// white is left with no discs and black has nothing left to flip, so the
	// game ends and black wins on count (4 discs to 0).
	st := state.NewBoardState()
	st.Push("b1", state.Occupant{Piece: "disc", Color: "white"})
	st.Push("c1", state.Occupant{Piece: "disc", Color: "black"})
	st.Push("h8", state.Occupant{Piece: "disc", Color: "black"})
	res, err := state.Apply(g, st, 1, mustParse(t, g, "a1"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != "discs" {
		t.Errorf("ended %v winner %v reason %q", res.GameEnded, res.WinnerSeat, res.EndReason)
	}
}

func TestReversiRandomGamesFinish(t *testing.T) {
	g := load(t, "reversi")
	rng := rand.New(rand.NewSource(8))
	passes := 0
	for n := 0; n < 20; n++ {
		st, seat := state.InitialState(g), 1
		for ply := 0; ply < 200; ply++ {
			moves := state.LegalMoves(g, st, seat)
			if len(moves) == 0 {
				t.Fatal("the seat to move has no legal move in an unfinished game")
			}
			res, err := state.Apply(g, st, seat, moves[rng.Intn(len(moves))])
			if err != nil {
				t.Fatal(err)
			}
			if !res.GameEnded && res.NextTurnSeat == seat {
				// A pass: the opponent really had no move and the mover does.
				passes++
				opp, _ := g.Opponent(seat)
				if state.HasLegalMove(g, res.NewState, opp) {
					t.Fatal("the turn passed although the opponent could move")
				}
				if !state.HasLegalMove(g, res.NewState, seat) {
					t.Fatal("the turn stayed with a seat that cannot move, but the game did not end")
				}
			}
			st, seat = res.NewState, res.NextTurnSeat
			if res.GameEnded {
				break
			}
			if ply == 199 {
				t.Fatal("game did not end")
			}
		}
	}
	t.Logf("%d passes observed across 20 random games", passes)
}

// ---- go -----------------------------------------------------------------------

// goBoard builds a 9x9 Go position from rows of "." / "x" (black) / "o"
// (white), top row first.
func goBoard(t testing.TB, rows ...string) *state.BoardState {
	t.Helper()
	st := state.NewBoardState()
	for r, row := range rows {
		for c, ch := range row {
			id := boardID(string(rune('a'+c)) + string(rune('0'+9-r)))
			switch ch {
			case 'x':
				st.Push(id, state.Occupant{Piece: "stone", Color: "black"})
			case 'o':
				st.Push(id, state.Occupant{Piece: "stone", Color: "white"})
			}
		}
	}
	return st
}

func TestGoCapturesAGroupWithoutLiberties(t *testing.T) {
	g := load(t, "go-9x9")
	// White b9 sits on the top edge between black a9 and an empty c9.
	st := goBoard(t,
		"xo.......",
	)
	res, err := state.Apply(g, st, 1, mustParse(t, g, "c9"))
	if err != nil {
		t.Fatal(err)
	}
	// b9 still has a liberty at b8, so it survives for now.
	if _, ok := res.NewState.Get("b9"); !ok {
		t.Fatal("b9 was captured while it still had a liberty")
	}
	res2, err := state.Apply(g, res.NewState, 2, mustParse(t, g, "pass"))
	if err != nil {
		t.Fatal(err)
	}
	res3, err := state.Apply(g, res2.NewState, 1, mustParse(t, g, "b8"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res3.NewState.Get("b9"); ok {
		t.Error("b9 should have been captured")
	}
	if len(res3.Captures) != 1 || res3.Captures[0] != "b9" {
		t.Errorf("captures = %v, want [b9]", res3.Captures)
	}
}

func TestGoSuicideIsIllegalUnlessItCaptures(t *testing.T) {
	g := load(t, "go-9x9")
	// A white eye at a9: black stones on b9 and a8. White playing a9 would
	// have no liberties: suicide.
	st := goBoard(t,
		".x.......",
		"x........",
	)
	if _, err := state.Apply(g, st, 2, mustParse(t, g, "a9")); err == nil {
		t.Error("suicide was accepted")
	}
	// But it is fine if it captures: black b9 has only a9 and c9... give
	// black b9 no other liberties by surrounding with white.
	st2 := goBoard(t,
		".xo......",
		"xo.......",
	)
	// White a9: black b9 has neighbours a9 (white now), c9 (white), b8
	// (white): captured, so a9 is legal even though a9 itself touches only
	// black a8 and the captured b9.
	res, err := state.Apply(g, st2, 2, mustParse(t, g, "a9"))
	if err != nil {
		t.Fatalf("a capturing move was rejected as suicide: %v", err)
	}
	if _, ok := res.NewState.Get("b9"); ok {
		t.Error("b9 should have been captured")
	}
}

func TestGoKoRuleBlocksRecreatingThePreviousPosition(t *testing.T) {
	g := load(t, "go-9x9")
	// The textbook ko on the top-left (rank 9 first):
	//   . x o
	//   x o . o
	//   . x o
	// White b8 has a single liberty (c8). Black captures there.
	st := goBoard(t,
		".xo......",
		"xo.o.....",
		".xo......",
	)
	// White b8 neighbours: a8 x, c8 empty, b9 x, b7 x -> one liberty (c8).
	// Black plays c8: neighbours of c8: b8 (white), d8 (white), c9 (white),
	// c7 (white): black c8 would have no liberties but captures b8, so legal.
	res, err := state.Apply(g, st, 1, mustParse(t, g, "c8"))
	if err != nil {
		t.Fatalf("black c8: %v", err)
	}
	if _, ok := res.NewState.Get("b8"); ok {
		t.Fatal("b8 was not captured")
	}
	// White may not immediately retake at b8 (which would capture c8 and
	// recreate the previous position).
	if _, err := state.Apply(g, res.NewState, 2, mustParse(t, g, "b8")); err == nil {
		t.Error("the ko recapture was accepted")
	}
	// After a pass by white and one by black elsewhere, the ban is lifted.
	r2, err := state.Apply(g, res.NewState, 2, mustParse(t, g, "h1"))
	if err != nil {
		t.Fatal(err)
	}
	r3, err := state.Apply(g, r2.NewState, 1, mustParse(t, g, "h3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.Apply(g, r3.NewState, 2, mustParse(t, g, "b8")); err != nil {
		t.Errorf("recapture after a tenuki was rejected: %v", err)
	}
}

func TestGoTwoPassesEndTheGameAndAreaScoringWithKomi(t *testing.T) {
	g := load(t, "go-9x9")
	// Black owns the left 3 columns (stones on column c, territory a-b);
	// white has one stone far away.
	st := state.NewBoardState()
	for r := 1; r <= 9; r++ {
		st.Push(boardID("c"+string(rune('0'+r))), state.Occupant{Piece: "stone", Color: "black"})
	}
	st.Push("h5", state.Occupant{Piece: "stone", Color: "white"})
	// h5 white; black c-file wall: territory a,b columns (18 points) belongs
	// to black (empty region a-b bordered only by black). The big region d-i
	// is bordered by both colors: neutral.
	r1, err := state.Apply(g, st, 1, mustParse(t, g, "pass"))
	if err != nil {
		t.Fatal(err)
	}
	if r1.GameEnded {
		t.Fatal("one pass must not end the game")
	}
	r2, err := state.Apply(g, r1.NewState, 2, mustParse(t, g, "pass"))
	if err != nil {
		t.Fatal(err)
	}
	if !r2.GameEnded || r2.EndReason != "score" {
		t.Fatalf("ended %v reason %q, want game over by score", r2.GameEnded, r2.EndReason)
	}
	// Black: 9 stones + 18 points = 27. White: 1 stone + 6.5 komi = 7.5.
	if r2.WinnerSeat == nil || *r2.WinnerSeat != 1 {
		t.Errorf("winner = %v, want black", r2.WinnerSeat)
	}
	if want := "Black 27 – White 7.5"; r2.EndDetail != want {
		t.Errorf("detail = %q, want %q", r2.EndDetail, want)
	}

	// A move in between resets the pass counter.
	st2 := state.NewBoardState()
	a, _ := state.Apply(g, st2, 1, mustParse(t, g, "pass"))
	b, _ := state.Apply(g, a.NewState, 2, mustParse(t, g, "e5"))
	c, _ := state.Apply(g, b.NewState, 1, mustParse(t, g, "pass"))
	if c.GameEnded || c.NewState.Passes() != 1 {
		t.Errorf("pass counter = %d ended=%v, want 1 and not ended", c.NewState.Passes(), c.GameEnded)
	}
}

func TestGoRandomGamesTerminateAndNotationRoundTrips(t *testing.T) {
	g := load(t, "go-9x9")
	rng := rand.New(rand.NewSource(21))
	for n := 0; n < 8; n++ {
		st, seat := state.InitialState(g), 1
		for ply := 0; ply < 600; ply++ {
			moves := state.LegalMoves(g, st, seat)
			if len(moves) == 0 {
				t.Fatal("no legal moves (pass should always exist)")
			}
			// Bias towards passing late so games end.
			mv := moves[rng.Intn(len(moves))]
			if ply > 200 {
				mv = moves[len(moves)-1] // the pass action is listed last
			}
			text := state.FormatMove(g, mv)
			back, err := state.ParseMove(g, text)
			if err != nil {
				t.Fatalf("parse %q: %v", text, err)
			}
			back, _ = back.Normalize(g)
			if back.Key() != mv.Key() {
				t.Fatalf("notation round trip %q: %+v vs %+v", text, back, mv)
			}
			res, err := state.Apply(g, st, seat, mv)
			if err != nil {
				t.Fatalf("legal move %s rejected: %v", text, err)
			}
			st, seat = res.NewState, res.NextTurnSeat
			if res.GameEnded {
				if res.EndDetail == "" {
					t.Error("a scored game has no detail")
				}
				break
			}
			if ply == 599 {
				t.Fatal("game did not end")
			}
		}
	}
}

// Move0 is the bare swap move.
func Move0(g *catalog.CompiledGame) state.Move {
	for _, a := range g.Actions {
		if a.Type == catalog.ActionSwap {
			return state.Move{Action: a.ID}
		}
	}
	return state.Move{}
}

func TestPieRuleSwapOnTheSecondMoveOnly(t *testing.T) {
	for _, id := range []string{"hex-11", "gomoku"} {
		g := load(t, id)
		st := state.InitialState(g)

		// Not available before anything was played.
		for _, mv := range state.LegalMoves(g, st, 1) {
			if mv.Action == "swap" {
				t.Fatalf("%s: swap offered on the first move", id)
			}
		}
		first := state.LegalMoves(g, st, 1)[0]
		res, err := state.Apply(g, st, 1, first)
		if err != nil {
			t.Fatal(err)
		}

		// Offered to the second player exactly now.
		var swap *state.Move
		for _, mv := range state.LegalMoves(g, res.NewState, 2) {
			if mv.Action == "swap" {
				m := mv
				swap = &m
			}
		}
		if swap == nil {
			t.Fatalf("%s: swap not offered on the second move", id)
		}
		sw, err := state.Apply(g, res.NewState, 2, *swap)
		if err != nil {
			t.Fatal(err)
		}
		if sw.Notation != "swap" {
			t.Errorf("%s: notation = %q", id, sw.Notation)
		}
		// The stone stays as it was — the players change seats — and the turn
		// stays with the second seat (the first player holds it now).
		top, ok := sw.NewState.Get(first.To)
		if !ok || top.Color == "" {
			t.Fatalf("%s: stone vanished", id)
		}
		orig, _ := res.NewState.Get(first.To)
		if top.Color != orig.Color {
			t.Errorf("%s: the stone changed colour", id)
		}
		if sw.NextTurnSeat != 2 {
			t.Errorf("%s: next seat = %d, want 2", id, sw.NextTurnSeat)
		}

		// And never again.
		second, err := state.Apply(g, res.NewState, 2, state.LegalMoves(g, res.NewState, 2)[0])
		if err != nil {
			t.Fatal(err)
		}
		for _, mv := range state.LegalMoves(g, second.NewState, 1) {
			if mv.Action == "swap" {
				t.Fatalf("%s: swap offered on the third move", id)
			}
		}
		if _, err := state.Apply(g, second.NewState, 1, Move0(g)); err == nil {
			t.Errorf("%s: a late swap was accepted", id)
		}
	}
}
