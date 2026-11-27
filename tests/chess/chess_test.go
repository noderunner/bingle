// Package chess_test checks the rules of chess: move generation against the
// published perft counts, castling, en passant, promotion, the endings and
// the notation.
package chess_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/chess"
)

func game(t testing.TB) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(chess.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get("chess")
	if !ok {
		t.Fatal("chess is not registered")
	}
	return g
}

// fen builds a position from Forsyth–Edwards Notation.
func fen(t testing.TB, text string) (*state.BoardState, int) {
	t.Helper()
	f := strings.Fields(text)
	for len(f) < 6 {
		f = append(f, map[int]string{2: "-", 3: "-", 4: "0", 5: "1"}[len(f)])
	}
	names := map[byte]string{'k': "king", 'q': "queen", 'r': "rook", 'b': "bishop", 'n': "knight", 'p': "pawn"}
	st := state.NewBoardState()
	for i, row := range strings.Split(f[0], "/") {
		rank := 8 - i
		file := 0
		for j := 0; j < len(row); j++ {
			c := row[j]
			if c >= '1' && c <= '8' {
				file += int(c - '0')
				continue
			}
			color, lower := "white", c
			if c >= 'a' {
				color = "black"
			} else {
				lower = c + 'a' - 'A'
			}
			st.Set(board.PositionID(string([]byte{byte('a' + file), byte('0' + rank)})), state.Occupant{Piece: names[lower], Color: color})
			file++
		}
	}
	d := &chess.Data{Turn: 1, Fullmove: 1}
	if f[1] == "b" {
		d.Turn = 2
	}
	if f[2] != "-" {
		d.Castling = f[2]
	}
	if f[3] != "-" {
		d.EnPassant = f[3]
	}
	for _, c := range f[4] {
		d.Halfmove = d.Halfmove*10 + int(c-'0')
	}
	st.SetExt(d)
	return st, d.Turn
}

func play(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, seat int, moves ...string) (*state.BoardState, *state.MoveResult) {
	t.Helper()
	rs := chess.New()
	var res *state.MoveResult
	for _, text := range moves {
		mv, err := rs.Parse(g, text)
		if err != nil {
			t.Fatalf("parse %q: %v", text, err)
		}
		res, err = rs.Apply(g, st, seat, mv)
		if err != nil {
			t.Fatalf("%q: %v", text, err)
		}
		st, seat = res.NewState, res.NextImpulseSeat
	}
	return st, res
}

func notations(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	rs := chess.New()
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

func perft(t *testing.T, g *catalog.CompiledGame, rs *chess.Ruleset, st *state.BoardState, seat, depth int) int {
	legal := rs.Legal(g, st, seat)
	if depth == 1 {
		return len(legal)
	}
	n := 0
	for _, mv := range legal {
		res, err := rs.Apply(g, st, seat, mv)
		if err != nil {
			t.Fatalf("%s: %v", rs.Format(g, mv), err)
		}
		if !res.GameEnded {
			n += perft(t, g, rs, res.NewState, res.NextImpulseSeat, depth-1)
		}
	}
	return n
}

// The moves found match the published perft counts of the standard test
// positions, which exercise every special move and check.
func TestChess_PerftCounts(t *testing.T) {
	g := game(t)
	rs := chess.New()
	start, err := rs.Start(g, 1)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		fen  string
		want []int
	}{
		{"start", "", []int{20, 400, 8902}},
		{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq -", []int{48, 2039, 97862}},
		{"position 3", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - -", []int{14, 191, 2812, 43238}},
		{"position 4", "r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1", []int{6, 264, 9467}},
		{"position 5", "rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8", []int{44, 1486, 62379}},
	}
	for _, c := range cases {
		st, seat := start, 1
		if c.fen != "" {
			st, seat = fen(t, c.fen)
			if err := rs.Validate(g, st); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
		}
		for i, want := range c.want {
			if testing.Short() && want > 10000 {
				break
			}
			if got := perft(t, g, rs, st, seat, i+1); got != want {
				t.Errorf("%s perft(%d) = %d, want %d", c.name, i+1, got, want)
			}
		}
	}
}

// Castling moves king and rook, needs the way clear and safe, and the
// rights go with the king or the rook.
func TestChess_Castling(t *testing.T) {
	g := game(t)
	st, seat := fen(t, "r3k2r/8/8/8/8/8/8/R3K2R w KQkq -")
	if l := notations(g, st, seat); !has(l, "O-O") || !has(l, "O-O-O") {
		t.Fatalf("both castles should be legal: %v", l)
	}
	after, _ := play(t, g, st, seat, "O-O")
	if k, _ := after.Get("g1"); k.Piece != "king" {
		t.Errorf("g1 holds %+v after O-O", k)
	}
	if r, _ := after.Get("f1"); r.Piece != "rook" {
		t.Errorf("f1 holds %+v after O-O", r)
	}
	if l := notations(g, after, 2); !has(l, "O-O-O") {
		t.Errorf("Black may still castle: %v", l)
	}
	// Moving the rook loses that side's right only; the UCI king move castles too.
	after, _ = play(t, g, st, seat, "Ra1-b1", "Ke8-d8", "Rb1-a1", "Kd8-e8")
	if l := notations(g, after, 1); has(l, "O-O-O") || !has(l, "O-O") {
		t.Errorf("after the a1 rook moved: %v", l)
	}
	if l := notations(g, after, 2); has(l, "O-O") || has(l, "O-O-O") {
		t.Errorf("after Black's king moved: %v", l)
	}
	if _, res := play(t, g, after, 1, "e1g1"); res.Notation != "O-O" {
		t.Errorf("e1g1 is recorded as %s", res.Notation)
	}
	// Not out of check, through an attacked square or into one.
	for _, c := range []struct{ fen, not string }{
		{"4k3/4r3/8/8/8/8/8/R3K2R w KQ -", "O-O"},
		{"4k3/5r2/8/8/8/8/8/R3K2R w KQ -", "O-O"},
		{"4k3/6r1/8/8/8/8/8/R3K2R w KQ -", "O-O"},
		{"4k3/3r4/8/8/8/8/8/R3K2R w KQ -", "O-O-O"},
		{"4k3/8/8/8/8/8/8/RN2K2R w KQ -", "O-O-O"},
	} {
		st, seat := fen(t, c.fen)
		if has(notations(g, st, seat), c.not) {
			t.Errorf("%s should not be legal in %s", c.not, c.fen)
		}
	}
	// The b1 square may be attacked: only the king's path counts.
	st, seat = fen(t, "4k3/1r6/8/8/8/8/8/R3K3 w Q -")
	if !has(notations(g, st, seat), "O-O-O") {
		t.Error("O-O-O is legal with only b1 attacked")
	}
}

// A pawn's double step may be taken en passant on the next move only.
func TestChess_EnPassant(t *testing.T) {
	g := game(t)
	st, _ := fen(t, "4k3/3p4/8/4P3/8/8/8/4K3 b - -")
	after, _ := play(t, g, st, 2, "d7-d5")
	if l := notations(g, after, 1); !has(l, "e5xd6") {
		t.Fatalf("e5xd6 should be legal: %v", l)
	}
	taken, res := play(t, g, after, 1, "exd6")
	if _, ok := taken.Get("d5"); ok {
		t.Error("the pawn on d5 is still there")
	}
	if len(res.Captures) != 1 || res.Captures[0] != "d5" {
		t.Errorf("captures %v, want [d5]", res.Captures)
	}
	late, _ := play(t, g, after, 1, "Ke1-f1", "Ke8-f8")
	if has(notations(g, late, 1), "e5xd6") {
		t.Error("en passant is still legal a move later")
	}
	// Not when it would leave the king in check along the rank.
	st, _ = fen(t, "8/8/8/K2Pp2r/8/8/8/7k w - e6")
	if has(notations(g, st, 1), "d5xe6") {
		t.Error("en passant that exposes the king is legal")
	}
}

// A pawn on the last rank becomes the piece named, chosen with a third click.
func TestChess_Promotion(t *testing.T) {
	g := game(t)
	rs := chess.New()
	st, seat := fen(t, "3rk3/2P5/8/8/8/8/8/4K3 w - -")
	l := notations(g, st, seat)
	for _, want := range []string{"c7-c8=Q", "c7-c8=R", "c7-c8=B", "c7-c8=N", "c7xd8=Q", "c7xd8=N"} {
		if !has(l, want) {
			t.Errorf("%s missing from %v", want, l)
		}
	}
	if mv, err := rs.Parse(g, "c8"); err != nil {
		t.Fatal(err)
	} else if _, err := rs.Apply(g, st, seat, mv); err == nil {
		t.Error("c8 without a piece was accepted")
	}
	after, res := play(t, g, st, seat, "cxd8=N+")
	if p, _ := after.Get("d8"); p.Piece != "knight" || p.Color != "white" {
		t.Errorf("d8 holds %+v", p)
	}
	if res.Notation != "c7xd8=N" {
		t.Errorf("recorded as %s", res.Notation)
	}
	for _, mv := range rs.Legal(g, st, seat) {
		if rs.Format(g, mv) == "c7-c8=R" {
			gs := rs.Gestures(g, mv)
			if len(gs) != 3 || gs[0].Value != "c7" || gs[1].Value != "c8" || gs[2].Kind != state.GestureAction || gs[2].Value != "rook" {
				t.Errorf("gestures %+v", gs)
			}
		}
	}
}

// Checkmate wins, and the other endings are draws.
func TestChess_Endings(t *testing.T) {
	g := game(t)
	rs := chess.New()
	start, _ := rs.Start(g, 1)
	_, res := play(t, g, start, 1, "f3", "e5", "g4", "Qh4#")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 2 || res.EndReason != "checkmate" {
		t.Errorf("fool's mate: %+v", res)
	}
	if doc, _ := rs.Redact(g, res.NewState, 0).MarshalJSON(); !strings.Contains(string(doc), `"tints":{"e1":"check"}`) {
		t.Errorf("the mated king is not tinted: %s", doc)
	}
	checked, _ := play(t, g, start, 1, "e4", "f5", "Qh5")
	if doc, _ := rs.Redact(g, checked, 0).MarshalJSON(); !strings.Contains(string(doc), `"tints":{"e8":"check"}`) || !strings.Contains(string(doc), "in check") {
		t.Errorf("Black's king in check is not shown: %s", doc)
	}
	draw := func(name, position, move, reason string) {
		t.Helper()
		st, seat := fen(t, position)
		_, res := play(t, g, st, seat, move)
		if !res.GameEnded || res.WinnerSeat != nil || res.EndReason != reason {
			t.Errorf("%s: ended %v, winner %v, reason %q; want a draw by %s", name, res.GameEnded, res.WinnerSeat, res.EndReason, reason)
		}
	}
	draw("stalemate", "7k/8/6Q1/8/8/8/8/K7 w - -", "Qg6-f7", "stalemate")
	draw("king and bishop", "4k3/8/8/8/8/8/3r4/2B1K3 w - -", "Kxd2", "insufficient-material")
	draw("fifty moves", "4k3/8/8/8/8/8/8/R3K3 w - - 99 80", "Ra1-a2", "fifty-moves")
	// Same-colored bishops cannot mate; opposite ones can.
	st, seat := fen(t, "4k3/8/8/8/8/2b5/8/1B2Kr2 w - -")
	if _, res := play(t, g, st, seat, "Kxf1"); res.GameEnded {
		t.Errorf("bishops on both colors ended the game: %s", res.EndReason)
	}
	st, seat = fen(t, "4k3/8/8/8/8/1b6/8/1B2Kr2 w - -")
	_, res = play(t, g, st, seat, "Kxf1")
	if !res.GameEnded || res.EndReason != "insufficient-material" {
		t.Errorf("bishops on one color: %+v", res)
	}
	// The start position a third time.
	_, res = play(t, g, start, 1, "Nf3", "Nf6", "Ng1", "Ng8", "Nf3", "Nf6", "Ng1")
	if res.GameEnded {
		t.Fatal("drawn too soon")
	}
	_, res = play(t, g, res.NewState, 2, "Ng8")
	if !res.GameEnded || res.EndReason != "repetition" {
		t.Errorf("threefold repetition: %+v", res)
	}
	if o := rs.Outcome(g, res.NewState); !o.Ended || o.Winner != nil {
		t.Errorf("outcome %+v", o)
	}
}

// Every legal move's notation reads back as itself, and short algebraic
// notation finds the one move it names.
func TestChess_Notation(t *testing.T) {
	g := game(t)
	rs := chess.New()
	st, seat := fen(t, "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq -")
	for _, mv := range rs.Legal(g, st, seat) {
		text := rs.Format(g, mv)
		parsed, err := rs.Parse(g, text)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		if again := rs.Format(g, parsed); again != text {
			t.Errorf("%s reads back as %s", text, again)
		}
		if res, err := rs.Apply(g, st, seat, parsed); err != nil || res.Notation != text {
			t.Errorf("%s: %v %v", text, res, err)
		}
	}
	for short, long := range map[string]string{"Nxd7": "Ne5xd7", "dxe6": "d5xe6", "O-O-O": "O-O-O", "0-0": "O-O", "Qxf6+": "Qf3xf6", "e5d7": "Ne5xd7", "a4": "a2-a4", "Bb5": "Be2-b5"} {
		mv, err := rs.Parse(g, short)
		if err != nil {
			t.Errorf("%s: %v", short, err)
			continue
		}
		if res, err := rs.Apply(g, st, seat, mv); err != nil || res.Notation != long {
			t.Errorf("%s: %v %v, want %s", short, res, err, long)
		}
	}
	// Two knights reach d2 here: the file must be given.
	st, seat = fen(t, "4k3/8/8/8/8/8/8/1N2KN2 w - -")
	mv, _ := rs.Parse(g, "Nd2")
	if _, err := rs.Apply(g, st, seat, mv); err == nil || !strings.Contains(err.Error(), "say which") {
		t.Errorf("Nd2 with two knights: %v", err)
	}
	if _, res := play(t, g, st, seat, "Nbd2"); res.Notation != "Nb1-d2" {
		t.Errorf("Nbd2 is %s", res.Notation)
	}
	if _, err := rs.Parse(g, "hello"); err == nil {
		t.Error("nonsense parsed")
	}
}

// A position document must make sense.
func TestChess_Validate(t *testing.T) {
	g := game(t)
	rs := chess.New()
	for _, bad := range []string{
		"4k3/8/8/8/8/8/8/8 w - -",    // no white king
		"4k3/8/8/8/8/8/8/3KK3 w - -", // two
		"P3k3/8/8/8/8/8/8/4K3 w - -", // a pawn on the last rank
		"4k3/8/8/8/8/8/8/4K3 w K -",  // a right without the rook
		"4k3/8/8/8/8/8/8/R3K3 w X -", // not a right
		"4k3/8/8/8/8/8/8/r3K3 b - -", // White in check with Black to move
		"4k3/8/8/8/8/8/8/4K3 w - e3", // en passant on the wrong rank
	} {
		st, _ := fen(t, bad)
		if err := rs.Validate(g, st); err == nil {
			t.Errorf("%s was accepted", bad)
		}
	}
	st, _ := rs.Start(g, 1)
	if err := rs.Validate(g, st); err != nil {
		t.Errorf("the start: %v", err)
	}
}

// A seat number out of range is nobody's: it does not wrap round to White's.
func TestChess_HeuristicSeatOutOfRange(t *testing.T) {
	g := game(t)
	rs := chess.New()
	st, _ := fen(t, "4k3/8/8/8/8/8/8/Q3K3 w - -")
	white := rs.Heuristic(g, st, 1)
	if white <= 0 {
		t.Fatalf("White a queen up values the position at %v", white)
	}
	if got := rs.Heuristic(g, st, 257); got == white {
		t.Errorf("seat 257 is valued as White: %v", got)
	}
}

// A pawn that takes a pawn leaves a pawn on the square in the other color,
// which is a capture and not a conversion (#293).
func TestChess_ACaptureOfALikePieceConvertsNothing(t *testing.T) {
	g := game(t)
	st, seat := fen(t, "4k3/8/8/3p4/4P3/8/8/4K3 w")
	mv, err := chess.New().Parse(g, "exd5")
	if err != nil {
		t.Fatal(err)
	}
	captures, converts := state.SelectionEffects(g, st, seat, mv)
	if !slices.Equal(captures, []board.PositionID{"d5"}) || converts != nil {
		t.Errorf("exd5 captures %v and converts %v, want [d5] and nothing", captures, converts)
	}
}
