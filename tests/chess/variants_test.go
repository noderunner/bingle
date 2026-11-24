package chess_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/chess"
)

func variant(t testing.TB, id string) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(chess.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("%s is not registered", id)
	}
	return g
}

// cells builds a position from "color piece" by square; White moves.
func cells(pieces map[string]string, d chess.Data) *state.BoardState {
	st := state.NewBoardState()
	for sq, p := range pieces {
		color, piece, _ := strings.Cut(p, " ")
		st.Set(board.PositionID(sq), state.Occupant{Piece: piece, Color: color})
	}
	if d.Turn == 0 {
		d.Turn = 1
	}
	if d.Fullmove == 0 {
		d.Fullmove = 1
	}
	st.SetExt(&d)
	return st
}

func legalIn(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	rs := chess.New()
	var out []string
	for _, mv := range rs.Legal(g, st, seat) {
		out = append(out, rs.Format(g, mv))
	}
	return out
}

// Move generation matches the published perft counts of Chess960,
// Antichess and Atomic positions, and Crazyhouse's start (no drop can come
// before the fifth ply).
func TestChessVariants_PerftCounts(t *testing.T) {
	rs := chess.New()
	cases := []struct {
		game, fen string
		want      []int
	}{
		{"chess-960", "bqnb1rkr/pp3ppp/3ppn2/2p5/5P2/P2P4/NPP1P1PP/BQ1BNRKR w HFhf - 2 9", []int{21, 528, 12189}},
		{"chess-960", "2nnrbkr/p1qppppp/8/1ppb4/6PP/3PP3/PPP2P2/BQNNRBKR w HEhe - 1 9", []int{21, 807, 18002}},
		{"chess-960", "b1q1rrkb/pppppppp/3nn3/8/P7/1PPP4/4PPPP/BQNNRKRB w GE - 1 9", []int{20, 479, 10471}},
		{"chess-antichess", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w - -", []int{20, 400, 8067, 153299}},
		{"chess-atomic", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq -", []int{20, 400, 8902, 197326}},
		{"chess-crazyhouse", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq -", []int{20, 400, 8902, 197281}},
	}
	for _, c := range cases {
		g := variant(t, c.game)
		st, seat := fen(t, c.fen)
		if err := rs.Validate(g, st); err != nil {
			t.Fatalf("%s %s: %v", c.game, c.fen, err)
		}
		for i, want := range c.want {
			if testing.Short() && want > 10000 {
				break
			}
			if got := perft(t, g, rs, st, seat, i+1); got != want {
				t.Errorf("%s %s perft(%d) = %d, want %d", c.game, c.fen, i+1, got, want)
			}
		}
	}
}

// A Chess960 table is drawn from the seed: 518 is chess's, 0 is BBQNNRKR.
// Castling puts king and rook on chess's squares, and is clicked king then
// rook, even when the king does not move.
func TestChessVariants_Chess960(t *testing.T) {
	g := variant(t, "chess-960")
	rs := chess.New()
	st, _ := rs.Start(g, 0)
	for sq, want := range map[string]string{"a1": "bishop", "b1": "bishop", "c1": "queen", "d1": "knight", "e1": "knight", "f1": "rook", "g1": "king", "h1": "rook", "g8": "king"} {
		if o, _ := st.Get(board.PositionID(sq)); o.Piece != want {
			t.Errorf("seed 0: %s holds %s, want %s", sq, o.Piece, want)
		}
	}
	std, _ := rs.Start(g, 518)
	if o, _ := std.Get("e1"); o.Piece != "king" {
		t.Errorf("seed 518 is not chess: e1 holds %s", o.Piece)
	}
	seen := map[string]bool{}
	for seed := uint64(0); seed < 960; seed++ {
		st, _ := rs.Start(g, seed)
		var row []byte
		for f := byte('a'); f <= 'h'; f++ {
			o, _ := st.Get(board.PositionID([]byte{f, '1'}))
			row = append(row, o.Piece...)
			row = append(row, ' ')
		}
		seen[string(row)] = true
		if err := rs.Validate(g, st); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
	if len(seen) != 960 {
		t.Errorf("%d different tables, want 960", len(seen))
	}

	// The king on g1 castles short by moving only the rook, to f1.
	st = cells(map[string]string{"g1": "white king", "h1": "white rook", "a1": "white rook", "g8": "black king"}, chess.Data{Castling: "KQ"})
	var castle state.Selection
	for _, mv := range rs.Legal(g, st, 1) {
		if rs.Format(g, mv) == "O-O" {
			castle = mv
		}
	}
	if castle.Action == "" {
		t.Fatalf("O-O missing: %v", legalIn(g, st, 1))
	}
	if gs := rs.Gestures(g, castle); len(gs) != 2 || gs[0].Value != "g1" || gs[1].Value != "h1" {
		t.Errorf("O-O is clicked %+v, want g1 then h1", gs)
	}
	after, _ := play(t, g, st, 1, "O-O")
	for sq, want := range map[string]string{"g1": "king", "f1": "rook"} {
		if o, _ := after.Get(board.PositionID(sq)); o.Piece != want {
			t.Errorf("after O-O %s holds %+v", sq, o)
		}
	}
	// O-O-O: the king crosses f1..c1, all of which must be empty and safe.
	if _, res := play(t, g, st, 1, "g1a1"); res.Notation != "O-O-O" {
		t.Errorf("king takes rook is %s", res.Notation)
	}
	blocked := cells(map[string]string{"g1": "white king", "a1": "white rook", "d1": "white knight", "g8": "black king"}, chess.Data{Castling: "Q"})
	if has(legalIn(g, blocked, 1), "O-O-O") {
		t.Error("O-O-O over a knight on d1")
	}
	attacked := cells(map[string]string{"g1": "white king", "a1": "white rook", "g8": "black king", "e8": "black rook"}, chess.Data{Castling: "Q"})
	if has(legalIn(g, attacked, 1), "O-O-O") {
		t.Error("O-O-O across the attacked e1")
	}
}

// A capture goes to the hand and can be dropped back, a promoted piece
// returns as a pawn, and pawns never drop on the first or last rank.
func TestChessVariants_Crazyhouse(t *testing.T) {
	g := variant(t, "chess-crazyhouse")
	rs := chess.New()
	start, _ := rs.Start(g, 1)
	st, res := play(t, g, start, 1, "e4", "d5", "exd5")
	if st.Reserve("white", "pawn") != 1 || res.Notation != "e4xd5" {
		t.Fatalf("White holds %d pawns after exd5", st.Reserve("white", "pawn"))
	}
	st, _ = play(t, g, st, 2, "Qxd5")
	if st.Reserve("black", "pawn") != 1 {
		t.Errorf("Black holds %d pawns after Qxd5", st.Reserve("black", "pawn"))
	}
	l := legalIn(g, st, 1)
	if !has(l, "P@e4") || has(l, "P@e8") || has(l, "N@f3") {
		t.Errorf("White's drops: %v", l)
	}
	st, res = play(t, g, st, 1, "P@e4")
	if res.Notation != "P@e4" || st.Reserve("white", "pawn") != 0 {
		t.Errorf("after P@e4: %s, %d in hand", res.Notation, st.Reserve("white", "pawn"))
	}
	if o, _ := st.Get("e4"); o.Piece != "pawn" || o.Color != "white" {
		t.Errorf("e4 holds %+v", o)
	}
	if gs := rs.Gestures(g, res.Move); len(gs) != 2 || gs[0].Kind != state.GesturePiece || gs[0].Value != "pawn" || gs[1].Value != "e4" {
		t.Errorf("a drop is clicked %+v", gs)
	}

	// A promoted queen taken goes to the hand as a pawn.
	st = cells(map[string]string{"e1": "white king", "h8": "black king", "b8": "black rook", "c8": "black rook", "a7": "white pawn"}, chess.Data{})
	st, _ = play(t, g, st, 1, "axb8=Q")
	if st.Reserve("white", "rook") != 1 {
		t.Errorf("White holds %d rooks after axb8=Q", st.Reserve("white", "rook"))
	}
	st, _ = play(t, g, st, 2, "Rxb8")
	if st.Reserve("black", "pawn") != 1 || st.Reserve("black", "queen") != 0 {
		t.Errorf("a promoted queen came back as %d pawns, %d queens", st.Reserve("black", "pawn"), st.Reserve("black", "queen"))
	}
	if err := rs.Validate(g, st); err != nil {
		t.Errorf("after Rxb8: %v", err)
	}
	// A drop can mate: a rook dropped on the back rank of a king boxed in by its pawns.
	st = cells(map[string]string{"e1": "white king", "g8": "black king", "f7": "black pawn", "g7": "black pawn", "h7": "black pawn", "e6": "white pawn"}, chess.Data{})
	st.SetReserve("white", "rook", 1)
	if _, res := play(t, g, st, 1, "R@e8"); !res.GameEnded || res.EndReason != "checkmate" {
		t.Errorf("R@e8: %+v", res)
	}
}

// Captures are compulsory, the king is an ordinary piece, and losing
// everything or having no move wins.
func TestChessVariants_Antichess(t *testing.T) {
	g := variant(t, "chess-antichess")
	rs := chess.New()
	start, _ := rs.Start(g, 1)
	if d := start.Ext().(*chess.Data); d.Castling != "" {
		t.Errorf("castling rights %q", d.Castling)
	}
	st, _ := play(t, g, start, 1, "e3", "b5")
	if l := legalIn(g, st, 1); len(l) != 1 || l[0] != "Bf1xb5" {
		t.Errorf("White must take: %v", l)
	}
	// The king may be taken, and a pawn may become a king.
	st = cells(map[string]string{"e4": "white king", "d5": "black pawn", "a2": "black pawn", "h8": "black rook"}, chess.Data{Turn: 2})
	if l := legalIn(g, st, 2); len(l) != 1 || l[0] != "d5xe4" {
		t.Errorf("Black must take the king: %v", l)
	}
	st = cells(map[string]string{"c4": "white knight", "a2": "black pawn", "h8": "black rook"}, chess.Data{Turn: 2})
	if l := legalIn(g, st, 2); !has(l, "a2-a1=K") {
		t.Errorf("no promotion to a king: %v", l)
	}
	// Black's last piece taken: Black wins.
	st = cells(map[string]string{"a1": "white rook", "a5": "black knight", "h1": "white king"}, chess.Data{})
	_, res := play(t, g, st, 1, "Ra1xa5")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 2 || res.EndReason != "no-pieces" {
		t.Errorf("Black has nothing left: %+v", res)
	}
	// Black stuck: Black wins.
	st = cells(map[string]string{"a2": "white pawn", "a4": "black pawn", "h2": "white pawn", "h5": "white pawn"}, chess.Data{})
	_, res = play(t, g, st, 1, "a3")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 2 || res.EndReason != "no-moves" {
		t.Errorf("Black has no move: %+v", res)
	}
}

// A capture blows up the pieces around but pawns; blowing up the king wins;
// kings cannot capture, nor blow up their own king, and side by side they
// give no check.
func TestChessVariants_Atomic(t *testing.T) {
	g := variant(t, "chess-atomic")
	rs := chess.New()
	st, _ := fen(t, "4k3/3np3/8/8/8/8/8/3QK3 w - -")
	after, res := play(t, g, st, 1, "Qxd7")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != "king-exploded" {
		t.Errorf("the king is blown up: %+v", res)
	}
	for _, sq := range []string{"d7", "d1", "e8"} {
		if _, ok := after.Get(board.PositionID(sq)); ok {
			t.Errorf("%s survived the explosion", sq)
		}
		if !has(func() []string {
			var s []string
			for _, c := range res.Captures {
				s = append(s, string(c))
			}
			return s
		}(), sq) {
			t.Errorf("captures %v lack %s", res.Captures, sq)
		}
	}
	if o, _ := after.Get("e7"); o.Piece != "pawn" {
		t.Error("the pawn on e7 was blown up")
	}
	// The king may not take, and a queen may not take beside her own king.
	st, _ = fen(t, "4k3/8/8/8/8/8/3np3/3QK3 w - -")
	for _, l := range legalIn(g, st, 1) {
		if strings.Contains(l, "x") {
			t.Errorf("%s is legal", l)
		}
	}
	// Kings side by side: no check from the rook.
	st, _ = fen(t, "8/8/8/8/8/8/3kK2r/8 w - -")
	if err := rs.Validate(g, st); err != nil {
		t.Errorf("kings side by side: %v", err)
	}
	if doc, _ := rs.Redact(g, st, 0).MarshalJSON(); strings.Contains(string(doc), `"check"`) {
		t.Errorf("a king beside the other is shown in check: %s", doc)
	}
	if !has(legalIn(g, st, 1), "Ke2-e3") {
		t.Errorf("White's king can step away: %v", legalIn(g, st, 1))
	}
}

// Every move crosses to the other board, onto an empty square; check comes
// only from a king's own board; and a move must be legal on its own board.
func TestChessVariants_Alice(t *testing.T) {
	g := variant(t, "chess-alice")
	rs := chess.New()
	start, _ := rs.Start(g, 1)
	l := legalIn(g, start, 1)
	if len(l) != 20 || !has(l, "Ng1-f3") || !has(l, "e2-e4") {
		t.Fatalf("first moves: %v", l)
	}
	st, res := play(t, g, start, 1, "e4")
	if res.Notation != "e2-e4" {
		t.Errorf("e4 is %s", res.Notation)
	}
	if o, _ := st.Get("m4"); o.Piece != "pawn" {
		t.Error("the pawn did not cross to m4")
	}
	// Black's e5 lands on m5, in front of White's pawn on board B.
	st, _ = play(t, g, st, 2, "e5")
	if has(legalIn(g, st, 1), "m4-m5") {
		t.Error("the pawn on m4 moved through the pawn on m5")
	}
	// The twin square must be empty.
	st = cells(map[string]string{"e1": "white king", "a1": "white rook", "i8": "black pawn", "e8": "black king"}, chess.Data{})
	if has(legalIn(g, st, 1), "Ra1-a8") {
		t.Error("the rook landed on a piece on board B")
	}
	// A rook on board B does not check a king on board A.
	st = cells(map[string]string{"e1": "white king", "m3": "white rook", "e8": "black king"}, chess.Data{Turn: 2})
	if err := rs.Validate(g, st); err != nil {
		t.Errorf("a rook on m3 checks e8: %v", err)
	}
	// The king may not step onto d1, attacked on board A, though it would
	// cross to the safe l1.
	st = cells(map[string]string{"e1": "white king", "d8": "black rook", "h8": "black king"}, chess.Data{})
	if l := legalIn(g, st, 1); has(l, "Ke1-d1") || !has(l, "Ke1-f1") {
		t.Errorf("king moves: %v", l)
	}
	// A capture on board B is clicked on the piece taken (game BTE3DX): the
	// bishop on j5 takes the knight on i6 and crosses to a6.
	st = cells(map[string]string{"e1": "white king", "e8": "black king", "j5": "white bishop", "i6": "black knight", "a7": "black pawn"}, chess.Data{})
	var take state.Selection
	for _, mv := range rs.Legal(g, st, 1) {
		if rs.Format(g, mv) == "Bj5xi6" {
			take = mv
		}
	}
	if gs := rs.Gestures(g, take); len(gs) != 2 || gs[0].Value != "j5" || gs[1].Value != "i6" {
		t.Fatalf("Bj5xi6 is clicked %+v (moves %v)", gs, legalIn(g, st, 1))
	}
	after, _ := play(t, g, st, 1, "Bxi6")
	if o, _ := after.Get("a6"); o.Piece != "bishop" {
		t.Errorf("the bishop is not on a6: %+v", o)
	}
	if _, ok := after.Get("i6"); ok {
		t.Error("the knight is still on i6")
	}
	// The older form, naming the twin, is the same move.
	if _, res := play(t, g, start, 1, "Ng1-n3"); res.Notation != "Ng1-f3" {
		t.Errorf("Ng1-n3 is recorded as %s", res.Notation)
	}

	// Castling is on board A, and both cross.
	st = cells(map[string]string{"e1": "white king", "h1": "white rook", "e8": "black king"}, chess.Data{Castling: "K"})
	after, _ = play(t, g, st, 1, "O-O")
	for sq, want := range map[string]string{"o1": "king", "n1": "rook"} {
		if o, _ := after.Get(board.PositionID(sq)); o.Piece != want {
			t.Errorf("after O-O %s holds %+v", sq, o)
		}
	}
}

// Random games of every variant: each position is valid, and each move's
// notation reads back as itself and plays the same move.
func TestChessVariants_RandomGames(t *testing.T) {
	rs := chess.New()
	for _, id := range []string{"chess", "chess-960", "chess-crazyhouse", "chess-antichess", "chess-atomic", "chess-alice"} {
		g := variant(t, id)
		rng := rand.New(rand.NewSource(7))
		for n := 0; n < 12; n++ {
			st, err := rs.Start(g, rng.Uint64())
			if err != nil {
				t.Fatal(err)
			}
			seat := 1
			for ply := 0; ply < 300; ply++ {
				legal := rs.Legal(g, st, seat)
				if len(legal) == 0 {
					t.Fatalf("%s: no move in a live game", id)
				}
				mv := legal[rng.Intn(len(legal))]
				text := rs.Format(g, mv)
				parsed, err := rs.Parse(g, text)
				if err != nil {
					t.Fatalf("%s %s: %v", id, text, err)
				}
				if again := rs.Format(g, parsed); again != text {
					t.Fatalf("%s: %s reads back as %s", id, text, again)
				}
				res, err := rs.Apply(g, st, seat, parsed)
				if err != nil {
					t.Fatalf("%s %s: %v", id, text, err)
				}
				if res.Notation != text {
					t.Fatalf("%s: %s played as %s", id, text, res.Notation)
				}
				if err := state.Validate(g, res.NewState); err != nil && !res.GameEnded {
					t.Fatalf("%s after %s: %v", id, text, err)
				}
				if res.GameEnded {
					break
				}
				st, seat = res.NewState, res.NextImpulseSeat
			}
		}
	}
}
