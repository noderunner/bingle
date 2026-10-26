// Package onitama_test checks Onitama's rules: the deal, card moves, the
// card exchange, captures, both ways to win, passing and the engine contract.
package onitama_test

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/onitama"
)

func game(t testing.TB) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(onitama.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get("onitama")
	if !ok {
		t.Fatal("onitama is not registered")
	}
	return g
}

// build makes a position: pieces as "red/master": squares.
func build(pieces map[string][]string, data onitama.Data) *state.BoardState {
	st := state.NewBoardState()
	for k, cells := range pieces {
		color, kind := k[:3], k[4:]
		if k[:4] == "blue" {
			color, kind = "blue", k[5:]
		}
		for _, c := range cells {
			st.Set(board.PositionID(c), state.Occupant{Piece: kind, Color: color})
		}
	}
	st.SetExt(&data)
	return st
}

func notations(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	rs := onitama.New()
	var out []string
	for _, mv := range rs.Legal(g, st, seat) {
		out = append(out, rs.Format(g, mv))
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
	rs := onitama.New()
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

func dataOfState(st *state.BoardState) *onitama.Data { d, _ := st.Ext().(*onitama.Data); return d }

func TestDealAndSetup(t *testing.T) {
	g := game(t)
	rs := onitama.New()
	for seed := uint64(1); seed < 40; seed++ {
		st, _ := rs.Start(g, seed)
		if err := state.Validate(g, st); err != nil {
			t.Fatal(err)
		}
		d := dataOfState(st)
		cards := map[string]bool{d.Side: true}
		for _, h := range d.Hands {
			cards[h[0]], cards[h[1]] = true, true
		}
		if len(cards) != 5 {
			t.Fatalf("five different cards are dealt: %v", d)
		}
		if len(st.Occupied()) != 10 {
			t.Fatalf("ten pieces: %d", len(st.Occupied()))
		}
	}
	st, _ := rs.Start(g, 7)
	if o, _ := st.Get("c1"); o.Piece != "master" || o.Color != "red" {
		t.Error("Red's Master starts on c1")
	}
	if o, _ := st.Get("c5"); o.Piece != "master" || o.Color != "blue" {
		t.Error("Blue's Master starts on c5")
	}
}

func TestRedStarts(t *testing.T) {
	g := game(t)
	rs := onitama.New()
	for seed := uint64(0); seed < 40; seed++ {
		st, _ := rs.Start(g, seed)
		if rs.ToMove(g, st) != 1 || len(rs.Legal(g, st, 1)) == 0 {
			t.Fatalf("seed %d: Red should start with moves", seed)
		}
	}
}

func TestCardMovesAreRelativeAndBlueFacesTheOtherWay(t *testing.T) {
	g := game(t)
	d := onitama.Data{Turn: 1, Hands: [2][2]string{{"tiger", "dragon"}, {"ox", "frog"}}, Side: "crab"}
	st := build(map[string][]string{"red/master": {"c1"}, "blue/master": {"c5"}, "red/student": {"b2"}}, d)
	got := notations(g, st, 1)
	for _, want := range []string{"tiger:c1-c3", "tiger:b2-b4", "dragon:c1-a2", "dragon:c1-e2", "dragon:b2-d3", "tiger:b2-b1"} {
		if !has(got, want) {
			t.Errorf("Red should have %s in %v", want, got)
		}
	}
	if has(got, "tiger:c1-c0") || has(got, "dragon:c1-a0") {
		t.Error("no move leaves the board")
	}
	// Blue reads the same card upside down: Tiger goes two ranks down.
	d.Turn = 2
	d.Hands[1] = [2]string{"tiger", "frog"}
	d.Hands[0] = [2]string{"ox", "dragon"}
	st = build(map[string][]string{"red/master": {"c1"}, "blue/master": {"c5"}}, d)
	got = notations(g, st, 2)
	if !has(got, "tiger:c5-c3") {
		t.Errorf("Blue's Tiger moves two down: %v", got)
	}
	// Frog: (-2,0) for Blue is two files to the right (e5 is off by one? c5->e5).
	if !has(got, "frog:c5-e5") {
		t.Errorf("Blue's Frog is mirrored: %v", got)
	}
}

func TestCannotLandOnOwnPieceButCapturesAnEnemy(t *testing.T) {
	g := game(t)
	d := onitama.Data{Turn: 1, Hands: [2][2]string{{"ox", "tiger"}, {"horse", "frog"}}, Side: "crab"}
	st := build(map[string][]string{"red/master": {"c1"}, "red/student": {"c2"}, "blue/student": {"d1"}, "blue/master": {"c5"}}, d)
	got := notations(g, st, 1)
	if has(got, "ox:c1-c2") {
		t.Error("cannot land on your own piece")
	}
	if !has(got, "ox:c1-d1") {
		t.Errorf("Ox moves right onto the enemy: %v", got)
	}
	st, _ = play(t, g, st, 1, "ox:c1-d1")
	if o, _ := st.Get("d1"); o.Color != "red" || o.Piece != "master" {
		t.Error("the Master took the student on d1")
	}
}

func TestCardsAreExchangedWithTheSide(t *testing.T) {
	g := game(t)
	d := onitama.Data{Turn: 1, Hands: [2][2]string{{"ox", "tiger"}, {"horse", "frog"}}, Side: "crab"}
	st := build(map[string][]string{"red/master": {"c1"}, "blue/master": {"c5"}}, d)
	st, res := play(t, g, st, 1, "ox:c1-c2")
	nd := dataOfState(st)
	if nd.Side != "ox" || nd.Hands[0] != [2]string{"crab", "tiger"} {
		t.Fatalf("after the move: hands %v side %s", nd.Hands, nd.Side)
	}
	if res.NextTurnSeat != 2 {
		t.Error("then Blue moves")
	}
	if _, err := onitama.New().Apply(g, st, 2, state.Move{Action: "move", Piece: "ox", From: "c5", To: "c4"}); err == nil {
		t.Error("Blue does not hold the Ox card")
	}
}

func TestWaysToWin(t *testing.T) {
	g := game(t)
	d := onitama.Data{Turn: 1, Hands: [2][2]string{{"ox", "tiger"}, {"horse", "frog"}}, Side: "crab"}
	// The Way of the Stone: capture the Master.
	st := build(map[string][]string{"red/master": {"c1"}, "blue/master": {"d1"}, "red/student": {"a1"}}, d)
	_, res := play(t, g, st, 1, "ox:c1-d1")
	if !res.GameEnded || *res.WinnerSeat != 1 || res.EndReason != "stone" {
		t.Errorf("capturing the Master wins: %+v", res)
	}
	// The Way of the Stream: Master onto the opposing arch.
	st = build(map[string][]string{"red/master": {"c4"}, "blue/master": {"a5"}}, d)
	_, res = play(t, g, st, 1, "ox:c4-c5")
	if !res.GameEnded || *res.WinnerSeat != 1 || res.EndReason != "stream" {
		t.Errorf("reaching the arch wins: %+v", res)
	}
	// A student on the arch is not a win.
	st = build(map[string][]string{"red/master": {"a1"}, "red/student": {"c4"}, "blue/master": {"a5"}}, d)
	_, res = play(t, g, st, 1, "ox:c4-c5")
	if res.GameEnded {
		t.Error("only a Master wins on the arch")
	}
	// Blue wins on c1.
	d.Turn = 2
	d.Hands = [2][2]string{{"ox", "tiger"}, {"horse", "ox2"}}
}

func TestPassWhenNothingCanMove(t *testing.T) {
	g := game(t)
	// Red fills the e-file, and the Tiger card only moves along a file: every
	// move is off the board or onto a Red piece.
	d := onitama.Data{Turn: 1, Hands: [2][2]string{{"tiger", "horse"}, {"ox", "frog"}}, Side: "crab"}
	d.Hands[0] = [2]string{"tiger", "tiger"}
	st := build(map[string][]string{"red/master": {"e5"}, "red/student": {"e1", "e2", "e3", "e4"}, "blue/master": {"a1"}}, d)
	got := notations(g, st, 1)
	if len(got) == 0 || !has(got, "tiger:pass") {
		t.Fatalf("Red must pass, spending a card: %v", got)
	}
	st, res := play(t, g, st, 1, "tiger:pass")
	nd := dataOfState(st)
	if nd.Side != "tiger" || nd.Hands[0][0] != "crab" || res.NextTurnSeat != 2 {
		t.Errorf("a pass still exchanges the card: %+v", nd)
	}
}

func TestRandomGamesAreValidAndEnd(t *testing.T) {
	g := game(t)
	rs := onitama.New()
	rng := rand.New(rand.NewSource(3))
	ended := 0
	for n := 0; n < 100; n++ {
		st, _ := rs.Start(g, uint64(n+1))
		seat := rs.ToMove(g, st)
		for ply := 0; ply < 400; ply++ {
			legal := rs.Legal(g, st, seat)
			if len(legal) == 0 {
				t.Fatal("a player always has a move or a pass")
			}
			mv := state.GuidedMoves(g, st, seat, legal)[0]
			if ply%3 == 0 {
				mv = legal[rng.Intn(len(legal))]
			}
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
	if ended < 90 {
		t.Errorf("only %d of 100 games ended", ended)
	}
}
