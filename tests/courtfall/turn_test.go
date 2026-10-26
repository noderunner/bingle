package courtfall_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

// A quiet position: Light's five units and Dark's queen, far apart.
func quiet() map[string]string {
	return map[string]string{
		"white/queen": "c6", "white/pawn": "d6", "white/knight": "b6", "white/bishop": "a6", "white/rook": "d7",
		"black/queen": "d2", "black/pawn": "e2", "black/knight": "f2", "black/bishop": "c2", "black/rook": "g2",
	}
}

func TestFreeRecoveryIsAutomaticWhenThereIsNoChoice(t *testing.T) {
	g := game(t)
	p := pos{stage: "seize", turn: dark, units: quiet(), cond: map[string]int{"white/pawn": 1, "black/rook": 1}}
	st, _ := play(t, g, p.build(), dark, "done")
	d := data(t, st)
	if d.Turn != light || d.Stage != "main" {
		t.Fatalf("turn %d stage %q, want Light's main stage", d.Turn, d.Stage)
	}
	if d.Cond["white/pawn"] != 0 {
		t.Errorf("Light's only white stone should have been removed: %v", d.Cond)
	}
	if d.Cond["black/rook"] != 1 {
		t.Errorf("Dark's stone must stay until Dark's own turn: %v", d.Cond)
	}
}

func TestFreeRecoveryAsksWhichUnitWhenSeveralAreFatigued(t *testing.T) {
	g := game(t)
	p := pos{stage: "seize", turn: dark, units: quiet(), cond: map[string]int{"white/pawn": 1, "white/knight": 1}}
	st, _ := play(t, g, p.build(), dark, "done")
	d := data(t, st)
	if d.Stage != "recover" || d.Turn != light {
		t.Fatalf("stage %q turn %d, want Light to choose a recovery", d.Stage, d.Turn)
	}
	got := notations(g, st, light)
	if len(got) != 2 || !has(got, "recover:P") || !has(got, "recover:N") {
		t.Fatalf("recovery choices = %v", got)
	}
	st, _ = play(t, g, st, light, "recover:N")
	d = data(t, st)
	if d.Stage != "main" || d.Cond["white/knight"] != 0 || d.Cond["white/pawn"] != 1 {
		t.Fatalf("after recovering the Outrider: stage %q cond %v", d.Stage, d.Cond)
	}
}

func TestRecoveryDoesNotTouchBlackStonesUnlessGale(t *testing.T) {
	g := game(t)
	p := pos{stage: "seize", turn: dark, units: quiet(), cond: map[string]int{"white/queen": 2}}
	st, _ := play(t, g, p.build(), dark, "done")
	if c := data(t, st).Cond["white/queen"]; c != 2 {
		t.Fatalf("a black stone must stay without Gale: cond %d", c)
	}
	p.courts = [2]string{"gale", ""}
	st, _ = play(t, g, p.build(), dark, "done")
	if c := data(t, st).Cond["white/queen"]; c != 1 {
		t.Fatalf("the Gale Court should turn the black stone white: cond %d", c)
	}
}

func TestGaleChoosesBetweenWhiteAndBlack(t *testing.T) {
	g := game(t)
	p := pos{stage: "seize", turn: dark, units: quiet(), courts: [2]string{"gale", ""},
		cond: map[string]int{"white/queen": 2, "white/pawn": 1}}
	st, _ := play(t, g, p.build(), dark, "done")
	if d := data(t, st); d.Stage != "recover" {
		t.Fatalf("stage %q, want a recovery choice", d.Stage)
	}
	got := notations(g, st, light)
	if !has(got, "recover:Q") || !has(got, "recover:P") {
		t.Fatalf("choices = %v", got)
	}
	st, _ = play(t, g, st, light, "recover:Q")
	d := data(t, st)
	if d.Cond["white/queen"] != 1 || d.Cond["white/pawn"] != 1 {
		t.Fatalf("cond = %v", d.Cond)
	}
}

func TestMainStageOffersActivateExertAndClear(t *testing.T) {
	g := game(t)
	p := pos{units: quiet(), cond: map[string]int{"white/pawn": 1, "white/queen": 2}}
	got := notations(g, p.build(), light)
	for _, want := range []string{"activate:N", "activate:B", "activate:R", "exert:P", "clear"} {
		if !has(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	for _, bad := range []string{"activate:P", "activate:Q", "exert:Q", "exert:N"} {
		if has(got, bad) {
			t.Errorf("%s must not be legal: %v", bad, got)
		}
	}
	if has(got, "surge:N>b5") || hasPrefix(got, "rouse") {
		t.Errorf("no Momentum, so no Surge or Rouse: %v", got)
	}
	// Not Dark's turn.
	if len(notations(g, p.build(), dark)) != 0 {
		t.Error("Dark has moves on Light's turn")
	}
}

func TestActivationIsTwoActionsThenAWhiteStone(t *testing.T) {
	g := game(t)
	p := pos{units: quiet()}
	st, _ := play(t, g, p.build(), light, "activate:N")
	d := data(t, st)
	if d.Stage != "act" || d.Act == nil || d.Act.Left != 2 || d.Act.Unit != "knight" {
		t.Fatalf("activation = %+v stage %q", d.Act, d.Stage)
	}
	st, _ = play(t, g, st, light, "Nb5")
	if d := data(t, st); d.Act.Left != 1 || d.Turn != light {
		t.Fatalf("after one action: %+v turn %d", d.Act, d.Turn)
	}
	st, res := play(t, g, st, light, "Nc4")
	d = data(t, st)
	if d.Turn != dark || d.Stage != "main" {
		t.Fatalf("after two actions: turn %d stage %q", d.Turn, d.Stage)
	}
	if d.Cond["white/knight"] != 1 {
		t.Fatalf("the Outrider should carry a white stone: %v", d.Cond)
	}
	if res.NextTurnSeat != dark {
		t.Fatalf("NextTurnSeat = %d", res.NextTurnSeat)
	}
}

func TestActivationCanEndEarlyButStillEarnsTheStone(t *testing.T) {
	g := game(t)
	st, _ := play(t, g, pos{units: quiet()}.build(), light, "activate:B")
	st, _ = play(t, g, st, light, "Bb5")
	st, _ = play(t, g, st, light, "end")
	d := data(t, st)
	if d.Turn != dark || d.Cond["white/bishop"] != 1 {
		t.Fatalf("turn %d cond %v", d.Turn, d.Cond)
	}
}

func TestExertIsOneActionThenABlackStone(t *testing.T) {
	g := game(t)
	p := pos{units: quiet(), cond: map[string]int{"white/rook": 1}}
	st, _ := play(t, g, p.build(), light, "exert:R")
	d := data(t, st)
	if d.Act == nil || d.Act.Left != 1 || !d.Act.Exert {
		t.Fatalf("exert activation = %+v", d.Act)
	}
	st, _ = play(t, g, st, light, "Rc7")
	d = data(t, st)
	if d.Turn != dark || d.Cond["white/rook"] != 2 {
		t.Fatalf("after an exert: turn %d cond %v (want black stone)", d.Turn, d.Cond)
	}
}

func TestExhaustedUnitsCannotBeActivatedOrExerted(t *testing.T) {
	// Exhausted units cannot be activated or exerted.
	g := game(t)
	p := pos{units: quiet(), cond: map[string]int{"white/rook": 2}}
	got := notations(g, p.build(), light)
	if has(got, "activate:R") || has(got, "exert:R") {
		t.Fatalf("an Exhausted unit acted: %v", got)
	}
}

func TestOnlyActionsOfTheActiveUnitAreOffered(t *testing.T) {
	g := game(t)
	st, _ := play(t, g, pos{units: quiet()}.build(), light, "activate:P")
	for _, n := range notations(g, st, light) {
		if n == "end" {
			continue
		}
		if n[0] != 'P' {
			t.Fatalf("move %s is not the Warden's", n)
		}
	}
}

func TestClearTurnBuysVPAtRisingPrice(t *testing.T) {
	g := game(t)
	cases := []struct {
		mom  int
		want []string
		not  []string
	}{
		{0, []string{"clear"}, []string{"clear+1"}},
		{1, []string{"clear"}, []string{"clear+1"}},
		{2, []string{"clear", "clear+1"}, []string{"clear+2"}},
		{4, []string{"clear", "clear+1"}, []string{"clear+2"}},
		{5, []string{"clear", "clear+1", "clear+2"}, []string{"clear+3"}},
	}
	for _, c := range cases {
		p := pos{units: quiet(), mom: [2]int{c.mom, 0}}
		got := notations(g, p.build(), light)
		for _, w := range c.want {
			if !has(got, w) {
				t.Errorf("mom %d: missing %s", c.mom, w)
			}
		}
		for _, n := range c.not {
			if has(got, n) {
				t.Errorf("mom %d: %s must not be affordable", c.mom, n)
			}
		}
	}
	p := pos{units: quiet(), mom: [2]int{5, 0}}
	st, _ := play(t, g, p.build(), light, "clear+2")
	d := data(t, st)
	if d.VP[0] != 2 || d.Mom[0] != 0 {
		t.Fatalf("buying 2 VP for 5 Momentum: vp %v mom %v", d.VP, d.Mom)
	}
}

func TestClearTurnRemovesEveryStoneAndRefreshesStoneCourt(t *testing.T) {
	g := game(t)
	p := pos{units: quiet(), cond: map[string]int{"white/queen": 2, "white/pawn": 1, "black/pawn": 1},
		courts: [2]string{"stone", ""}, stoneUsed: [2]bool{true, false}}
	st, _ := play(t, g, p.build(), light, "clear")
	d := data(t, st)
	if d.Cond["white/queen"] != 0 || d.Cond["white/pawn"] != 0 {
		t.Fatalf("Light's stones should be gone: %v", d.Cond)
	}
	if d.Cond["black/pawn"] != 0 && d.Turn == dark {
		// Dark's own recovery already took its single white stone.
		t.Fatalf("Dark's free recovery should remove its stone: %v", d.Cond)
	}
	if d.StoneUsed[0] {
		t.Error("a Clear Turn should refresh the Stone Court swap")
	}
}

func TestEyePaysOnlyWhileAUnitThatCanActHoldsIt(t *testing.T) {
	g := game(t)
	u := quiet()
	u["white/queen"] = "d4"
	st, _ := play(t, g, pos{units: u}.build(), light, "clear")
	if m := data(t, st).Mom[0]; m != 1 {
		t.Fatalf("holding the Eye should pay 1 Momentum, got %d", m)
	}
	st, _ = play(t, g, pos{units: u, cond: map[string]int{"white/queen": 2}}.build(), light, "clear")
	if m := data(t, st).Mom[0]; m != 0 {
		t.Fatalf("an Exhausted holder must not collect: %d", m)
	}
	// The payout counts before buying: 1 banked + 1 Eye = the first VP.
	p := pos{units: u, mom: [2]int{1, 0}}
	got := notations(g, p.build(), light)
	if !has(got, "clear+1") {
		t.Fatalf("the Eye payout should make clear+1 affordable: %v", got)
	}
	st, _ = play(t, g, p.build(), light, "clear+1")
	if d := data(t, st); d.VP[0] != 1 || d.Mom[0] != 0 {
		t.Fatalf("vp %v mom %v", d.VP, d.Mom)
	}
}

func TestMomentumIsCappedAtFive(t *testing.T) {
	g := game(t)
	u := quiet()
	u["white/queen"] = "d4"
	st, _ := play(t, g, pos{units: u, mom: [2]int{5, 0}}.build(), light, "clear")
	if m := data(t, st).Mom[0]; m != 5 {
		t.Fatalf("momentum %d, want it capped at 5", m)
	}
}

func TestFifthVPEndsTheGameOnTheClear(t *testing.T) {
	g := game(t)
	p := pos{units: quiet(), mom: [2]int{2, 0}, vp: [2]int{4, 0}}
	st, res := play(t, g, p.build(), light, "clear+1")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != light || res.EndReason != "victory" {
		t.Fatalf("result = %+v", res)
	}
	if d := data(t, st); d.Phase != "over" || d.VP[0] != 5 {
		t.Fatalf("data %+v", d)
	}
	if len(state.LegalMoves(g, st, dark)) != 0 || len(state.LegalMoves(g, st, light)) != 0 {
		t.Fatal("a finished game offers moves")
	}
	out, err := state.Evaluate(g, st, light)
	if err != nil || !out.Ended || out.Winner == nil || *out.Winner != light {
		t.Fatalf("Evaluate = %+v %v", out, err)
	}
}

func TestSurgeShiftsAUnitOneHexForOneMomentum(t *testing.T) {
	g := game(t)
	p := pos{units: quiet(), mom: [2]int{1, 0}, cond: map[string]int{"white/pawn": 2}}
	got := notations(g, p.build(), light)
	if !has(got, "surge:Q>c5") {
		t.Fatalf("surge missing: %v", got)
	}
	if hasPrefix(got, "surge:P") {
		t.Fatalf("an Exhausted unit cannot be surged: %v", got)
	}
	st, _ := play(t, g, p.build(), light, "surge:Q>c5")
	d := data(t, st)
	if d.Mom[0] != 0 || d.Stage != "main" || d.Turn != light {
		t.Fatalf("after a surge: mom %v stage %q turn %d", d.Mom, d.Stage, d.Turn)
	}
	if o, ok := st.Get("c5"); !ok || o.Piece != "queen" {
		t.Fatal("the Champion should stand on c5")
	}
	// Surge costs 1; with nothing banked it is not offered.
	if hasPrefix(notations(g, pos{units: quiet()}.build(), light), "surge") {
		t.Fatal("surge offered without Momentum")
	}
}

func TestRouseRemovesABlackStoneForTwo(t *testing.T) {
	g := game(t)
	p := pos{units: quiet(), mom: [2]int{2, 0}, cond: map[string]int{"white/rook": 2}}
	st, _ := play(t, g, p.build(), light, "rouse:R")
	d := data(t, st)
	if d.Mom[0] != 0 || d.Cond["white/rook"] != 0 || d.Stage != "main" {
		t.Fatalf("after rouse: %+v", d)
	}
	if hasPrefix(notations(g, pos{units: quiet(), mom: [2]int{1, 0}, cond: map[string]int{"white/rook": 2}}.build(), light), "rouse") {
		t.Fatal("rouse offered with one Momentum")
	}
	if hasPrefix(notations(g, pos{units: quiet(), mom: [2]int{3, 0}, cond: map[string]int{"white/rook": 1}}.build(), light), "rouse") {
		t.Fatal("rouse offered for a white stone")
	}
}

func TestSeizeTheTempoGivesAnotherTurnButNeverTwiceInARow(t *testing.T) {
	g := game(t)
	p := pos{units: quiet(), mom: [2]int{2, 0}}
	st, _ := play(t, g, p.build(), light, "clear")
	d := data(t, st)
	if d.Stage != "seize" || d.Turn != light {
		t.Fatalf("after the turn: stage %q turn %d, want the seize offer", d.Stage, d.Turn)
	}
	got := notations(g, st, light)
	if !has(got, "seize") || !has(got, "done") {
		t.Fatalf("offers = %v", got)
	}
	st, _ = play(t, g, st, light, "seize")
	d = data(t, st)
	if d.Turn != light || d.Stage != "main" || d.Mom[0] != 0 || d.Streak != 2 {
		t.Fatalf("seized turn: %+v", d)
	}
	// With 2 Momentum again, the seized turn may not seize once more.
	d.Mom[0] = 2
	st, _ = play(t, g, st, light, "clear")
	d = data(t, st)
	if d.Turn != dark {
		t.Fatalf("a second Seize in succession was offered: turn %d stage %q", d.Turn, d.Stage)
	}
}

func TestNoSeizeOfferWithoutMomentum(t *testing.T) {
	g := game(t)
	st, _ := play(t, g, pos{units: quiet(), mom: [2]int{1, 0}}.build(), light, "clear")
	if d := data(t, st); d.Turn != dark {
		t.Fatalf("turn %d, want Dark", d.Turn)
	}
}

func TestPlyLimitEndsTheGameOnVictoryPoints(t *testing.T) {
	g := game(t)
	p := pos{units: quiet(), vp: [2]int{2, 1}}
	st := p.build()
	st.SetPly(1999)
	_, res := play(t, g, st, light, "clear")
	if !res.GameEnded || res.EndReason != "ply-limit" || res.WinnerSeat == nil || *res.WinnerSeat != light {
		t.Fatalf("result = %+v", res)
	}
	p.vp = [2]int{1, 1}
	st = p.build()
	st.SetPly(1999)
	_, res = play(t, g, st, light, "clear")
	if !res.GameEnded || res.WinnerSeat != nil {
		t.Fatalf("equal VP should draw: %+v", res)
	}
}

func TestMovesBySeatOutOfTurnAreRejected(t *testing.T) {
	g := game(t)
	st := pos{units: quiet()}.build()
	mv, _ := state.ParseMove(g, "clear")
	if _, err := state.Apply(g, st, dark, mv); err == nil {
		t.Fatal("Dark moved on Light's turn")
	}
}
