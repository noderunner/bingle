// Package fist_test checks Fist's rules: the setup, stepping, pinning with
// allies and divided attention, jumping, the Ace's Convert, the mage's
// Empower, the verdigris pawn and the two ways to win.
package fist_test

import (
	"sort"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/fist"
)

func game(t testing.TB) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(fist.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get("fist")
	if !ok {
		t.Fatal("fist is not registered")
	}
	return g
}

// pos builds a position from "color/piece" → squares; the challenger moves.
type pos struct {
	pieces map[string][]string
	turn   int
	data   fist.Data
}

func (p pos) build(t testing.TB) *state.BoardState {
	st := state.NewBoardState()
	for k, cells := range p.pieces {
		color, piece := k[:5], k[6:]
		if k[:5] == "green" {
			color, piece = "green", k[6:]
		}
		for _, c := range cells {
			st.Set(board.PositionID(c), state.Occupant{Piece: piece, Color: color})
		}
	}
	d := p.data
	d.Turn = p.turn
	if d.Turn == 0 {
		d.Turn = 1
	}
	st.SetExt(&d)
	return st
}

func notations(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	rs := fist.New()
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
	rs := fist.New()
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

func at(st *state.BoardState, sq string) (state.Occupant, bool) { return st.Get(board.PositionID(sq)) }

func TestStartingPosition(t *testing.T) {
	g := game(t)
	rs := fist.New()
	st, _ := rs.Start(g, 1)
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, id := range st.Occupied() {
		o, _ := st.Get(id)
		counts[o.Color+"/"+o.Piece]++
	}
	want := map[string]int{"white/ace": 1, "white/mage": 1, "white/blue": 3, "white/yellow": 6, "black/king": 1, "black/blue": 7, "black/yellow": 12, "green/pawn": 1}
	for k, n := range want {
		if counts[k] != n {
			t.Errorf("%s: %d, want %d", k, counts[k], n)
		}
	}
	if rs.ToMove(g, st) != 1 {
		t.Error("the challenger moves first")
	}
	if len(rs.Legal(g, st, 1)) == 0 {
		t.Error("the challenger has opening moves")
	}
}

func TestYellowGuardStepsOneAndPinsAnEqualPiece(t *testing.T) {
	g := game(t)
	p := pos{pieces: map[string][]string{"white/yellow": {"c3"}, "black/yellow": {"e4"}, "black/king": {"h8"}}}
	got := notations(g, p.build(t), 1)
	if !has(got, "c3-d3") || has(got, "c3-e3") {
		t.Errorf("a yellow guard steps one square: %v", got)
	}
	// d4 touches e4: value 1 against value 1 captures.
	if !has(got, "c3-d4xe4") && !has(got, "c3-d3xe3") {
		// d4 touches d5/e4/c4/d3: e4 is east of d4.
		t.Errorf("a yellow guard pins an equal piece after stepping: %v", got)
	}
}

func TestPinNeedsStrengthAndDividedAttentionNeedsAll(t *testing.T) {
	g := game(t)
	// A yellow guard (1) cannot pin a blue guard (2) alone…
	p := pos{pieces: map[string][]string{"white/yellow": {"c4"}, "black/blue": {"e4"}, "black/king": {"h8"}}}
	for _, n := range notations(g, p.build(t), 1) {
		if len(n) > 5 {
			t.Errorf("a lone yellow guard cannot pin a blue guard: %s", n)
		}
	}
	// …but with a blue guard beside the target it can (1+2 ≥ 2).
	p.pieces["white/blue"] = []string{"e3"}
	if !has(notations(g, p.build(t), 1), "c4-d4xe4") {
		t.Error("an ally touching the target lends its value")
	}
	// Divided attention: the king (4) beside value 1 + 2 + 2 targets (5) can take nothing.
	p = pos{pieces: map[string][]string{"black/king": {"d4"}, "white/yellow": {"d5"}, "white/blue": {"c4", "e4"}}, turn: 2}
	for _, n := range notations(g, p.build(t), 2) {
		if len(n) == 7 && n[2] == '-' && n[5] == 'x' {
			t.Errorf("the king cannot take anything from five points of targets: %s", n)
		}
	}
	// 1 + 1 + 2 = 4: it can take any one.
	p = pos{pieces: map[string][]string{"black/king": {"d3"}, "white/yellow": {"d5", "c4"}, "white/blue": {"e4"}}, turn: 2}
	got := notations(g, p.build(t), 2)
	for _, want := range []string{"d3-d4xd5", "d3-d4xc4", "d3-d4xe4"} {
		if !has(got, want) {
			t.Errorf("%s should be legal with strength 4 against 4: %v", want, got)
		}
	}
}

func TestBlueGuardJumpsAndMayChooseToCapture(t *testing.T) {
	g := game(t)
	p := pos{pieces: map[string][]string{"white/blue": {"a1"}, "black/yellow": {"b1", "a2"}, "black/king": {"h8"}}}
	got := notations(g, p.build(t), 1)
	for _, want := range []string{"a1^c1", "a1^c1x", "a1^a3", "a1^a3x"} {
		if !has(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	st, _ := play(t, g, p.build(t), 1, "a1^c1x")
	if _, ok := at(st, "b1"); ok {
		t.Error("the jumped piece should be captured")
	}
	if o, ok := at(st, "c1"); !ok || o.Piece != "blue" {
		t.Error("the blue guard lands on c1")
	}
	st, _ = play(t, g, p.build(t), 1, "a1^c1")
	if _, ok := at(st, "b1"); !ok {
		t.Error("an unchosen capture leaves the piece")
	}
}

func TestJumpsCannotPassHigherValueOrLandOnPieces(t *testing.T) {
	g := game(t)
	p := pos{pieces: map[string][]string{"white/mage": {"a1"}, "black/king": {"b1"}, "white/yellow": {"a2"}}}
	got := notations(g, p.build(t), 1)
	if has(got, "a1^c1") || has(got, "a1^c1x") {
		t.Errorf("the mage (3) cannot jump the king (4): %v", got)
	}
	if !has(got, "a1^a3") {
		t.Errorf("the mage may jump an ally: %v", got)
	}
	// A chain may not land twice on a square.
	p = pos{pieces: map[string][]string{"white/ace": {"a1"}, "black/yellow": {"b1", "b2", "a2"}, "black/king": {"h8"}}}
	for _, n := range notations(g, p.build(t), 1) {
		if n == "a1^c1^a1" {
			t.Errorf("a piece cannot return to its start: %s", n)
		}
	}
}

func TestAceConvertsACapturedGuardWhenOneIsOutOfPlay(t *testing.T) {
	g := game(t)
	p := pos{pieces: map[string][]string{"white/ace": {"a1"}, "black/yellow": {"b1"}, "black/king": {"h8"}}}
	got := notations(g, p.build(t), 1)
	if !has(got, "a1^c1c") {
		t.Fatalf("the ace may replace a captured guard: %v", got)
	}
	st, _ := play(t, g, p.build(t), 1, "a1^c1c")
	if o, ok := at(st, "b1"); !ok || o.Color != "white" || o.Piece != "yellow" {
		t.Errorf("a white yellow guard should stand on b1: %v %v", o, ok)
	}
	// With all six yellow guards in play there is nothing to convert to.
	p.pieces["white/yellow"] = []string{"e1", "e2", "e3", "e4", "e5", "e6"}
	if has(notations(g, p.build(t), 1), "a1^c1c") {
		t.Error("no replacement is available")
	}
}

func TestMageEmpowersAlliesItSees(t *testing.T) {
	g := game(t)
	// The mage sees the yellow guard along the rank: it is empowered (value 2,
	// two squares).
	p := pos{pieces: map[string][]string{"white/mage": {"a1"}, "white/yellow": {"d1"}, "black/king": {"h8"}}}
	got := notations(g, p.build(t), 1)
	if !has(got, "d1-d3") {
		t.Errorf("an empowered yellow guard moves two squares: %v", got)
	}
	// Blocked sight: no empowerment.
	p.pieces["black/yellow"] = []string{"b1"}
	got = notations(g, p.build(t), 1)
	if has(got, "d1-d3") {
		t.Errorf("a blocked mage does not empower: %v", got)
	}
}

func TestPawnIsControlledByWhoeverSeesItAndCannotBeCaptured(t *testing.T) {
	g := game(t)
	// Only the challenger's yellow guard sees the pawn.
	p := pos{pieces: map[string][]string{"green/pawn": {"d1"}, "white/yellow": {"a1"}, "black/king": {"h8"}}}
	if !has(notations(g, p.build(t), 1), "d1-d2") {
		t.Error("the challenger sees the pawn and may move it")
	}
	p.turn = 2
	if has(notations(g, p.build(t), 2), "d1-d2") {
		t.Error("the king's side cannot see the pawn")
	}
	// A king's-side piece in the Keep always sees it.
	p.pieces["black/yellow"] = []string{"e4"}
	if !has(notations(g, p.build(t), 2), "d1-d2") {
		t.Error("a piece in the Keep sees the pawn")
	}
	// Jumping over the pawn never captures it.
	p = pos{pieces: map[string][]string{"white/ace": {"a1"}, "green/pawn": {"b1"}, "black/king": {"h8"}}}
	got := notations(g, p.build(t), 1)
	if has(got, "a1^c1x") {
		t.Errorf("the pawn cannot be captured: %v", got)
	}
}

func TestPawnCannotReverseItsLastMove(t *testing.T) {
	g := game(t)
	p := pos{pieces: map[string][]string{"green/pawn": {"d4"}, "white/yellow": {"d3"}, "black/king": {"h8"}}, data: fist.Data{PawnFrom: "d5"}}
	if has(notations(g, p.build(t), 1), "d4-d5") {
		t.Error("the pawn cannot go straight back")
	}
	if !has(notations(g, p.build(t), 1), "d4-d6") {
		t.Error("it may pass the square it left")
	}
}

func TestPawnPinsTheKingAndTheChallengerWins(t *testing.T) {
	g := game(t)
	p := pos{pieces: map[string][]string{"green/pawn": {"a1"}, "white/yellow": {"b1"}, "black/king": {"d3"}}}
	st, res := play(t, g, p.build(t), 1, "a1-c3xd3")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 {
		t.Fatalf("capturing the king wins: %+v", res)
	}
	if out := fist.New().Outcome(g, st); !out.Ended || out.Reason != "king-captured" {
		t.Errorf("outcome %+v", out)
	}
}

func TestKingsSideWinsWhenTheLastBlueGuardFalls(t *testing.T) {
	g := game(t)
	p := pos{pieces: map[string][]string{"white/blue": {"c3"}, "white/mage": {"a8"}, "black/king": {"e3"}}, turn: 2}
	_, res := play(t, g, p.build(t), 2, "e3-d3xc3")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 2 {
		t.Errorf("the last blue guard fell: %+v", res)
	}
}

func TestKingsSideTakingTheMageGivesTheKingEmpower(t *testing.T) {
	g := game(t)
	p := pos{pieces: map[string][]string{"white/mage": {"c4"}, "black/king": {"d3"}, "white/blue": {"h8"}}, turn: 2}
	st, _ := play(t, g, p.build(t), 2, "d3-d4xc4")
	d, _ := st.Ext().(*fist.Data)
	if d == nil || !d.KingEmpower {
		t.Fatal("the king should now Empower")
	}
}

func TestNotationRoundTripAndRandomGamesFinish(t *testing.T) {
	g := game(t)
	rs := fist.New()
	st, _ := rs.Start(g, 1)
	seat := 1
	for ply := 0; ply < 400; ply++ {
		legal := rs.Legal(g, st, seat)
		if len(legal) == 0 {
			break
		}
		mv := legal[(ply*7919)%len(legal)]
		text := rs.Format(g, mv)
		back, err := rs.Parse(g, text)
		if err != nil || rs.Format(g, back) != text {
			t.Fatalf("round trip %q: %v %v", text, back, err)
		}
		res, err := rs.Apply(g, st, seat, back)
		if err != nil {
			t.Fatalf("ply %d %s: %v", ply, text, err)
		}
		if err := state.Validate(g, res.NewState); err != nil {
			t.Fatalf("ply %d %s: %v", ply, text, err)
		}
		st = res.NewState
		if res.GameEnded {
			return
		}
		seat = res.NextTurnSeat
	}
}
