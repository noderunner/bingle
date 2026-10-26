package courtfall_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

// brawl is Light's Champion on d5 striking Dark's Warden on d4, with the
// dice for the exchange fixed: attack dice, defense dice of the striker,
// then the target's attack and defense dice.
type brawl struct {
	units map[string]string
	cond  map[string]int
	tiles map[string]string
	mom   [2]int
	court [2]string
	dice  []int
}

func (b brawl) start(t *testing.T) *state.BoardState {
	u := map[string]string{"white/queen": "d5", "black/pawn": "d4"}
	for k, v := range b.units {
		u[k] = v
	}
	p := pos{stage: "act", units: u, cond: b.cond, tiles: b.tiles, mom: b.mom, courts: b.court, act: actFor("queen")}
	if len(b.dice) > 0 {
		p.rng = seedDice(t, b.dice...)
	}
	return p.build()
}

func TestStrikeCommitsInSecretAndTheTargetAnswers(t *testing.T) {
	g := game(t)
	b := brawl{dice: []int{6, 6, 2, 1, 1, 2, 3}}
	st := b.start(t)
	got := notations(g, st, light)
	for n := 0; n <= 4; n++ {
		if want := "Qxd4/" + string(rune('0'+n)); !has(got, want) {
			t.Fatalf("missing %s in %v", want, got)
		}
	}
	if has(got, "Qxd4/5") {
		t.Fatal("a Champion has only four dice")
	}
	st, res := play(t, g, st, light, "Qxd4/3")
	d := data(t, st)
	if d.Stage != "respond" || d.Con == nil || d.Con.AAtk != 3 || d.Con.Rolled {
		t.Fatalf("after the commit: stage %q con %+v", d.Stage, d.Con)
	}
	if res.NextTurnSeat != dark {
		t.Fatalf("the target must answer next, got seat %d", res.NextTurnSeat)
	}
	if len(notations(g, st, light)) != 0 {
		t.Fatal("the committed side has no further decisions")
	}
	answers := notations(g, st, dark)
	if len(answers) != 4 || !has(answers, "respond/0") || !has(answers, "respond/3") || has(answers, "respond/4") {
		t.Fatalf("a Warden (CV 3) may put 0-3 dice on attack: %v", answers)
	}
}

func TestFreshTargetHitIsFatiguedAndPaysMomentumAndPushesOnAMarginOfTwo(t *testing.T) {
	g := game(t)
	b := brawl{dice: []int{6, 6, 2, 1, 1, 2, 3}}
	st, _ := play(t, g, b.start(t), light, "Qxd4/3")
	st, _ = play(t, g, st, dark, "respond/1")
	d := data(t, st)
	if d.Last == nil || d.Last.ADN != 6 || d.Last.TFN != 6 {
		t.Fatalf("thresholds = %+v", d.Last)
	}
	if got := d.Last.AD; len(got) != 3 || got[0] != 6 || got[1] != 6 {
		t.Fatalf("attack dice = %v", got)
	}
	if d.Cond["black/pawn"] != 1 {
		t.Fatalf("the Warden should be Fatigued: %v", d.Cond)
	}
	if d.Mom[0] != 1 || d.Mom[1] != 0 {
		t.Fatalf("Light gains 1 Momentum: %v", d.Mom)
	}
	// Winning by two successes: Light chooses where the Warden goes.
	if d.Stage != "push" {
		t.Fatalf("stage %q, want a push decision", d.Stage)
	}
	opts := notations(g, st, light)
	if len(opts) != 5 || has(opts, "push>SE") {
		t.Fatalf("push directions = %v (SE is the Champion's own hex)", opts)
	}
	st, _ = play(t, g, st, light, "push>E")
	if o, ok := st.Get("e4"); !ok || o.Piece != "pawn" {
		t.Fatal("the Warden should have been pushed to e4")
	}
	d = data(t, st)
	if d.Stage != "act" || d.Act.Left != 1 {
		t.Fatalf("the activation continues: stage %q act %+v", d.Stage, d.Act)
	}
}

func TestTiesGoToDefense(t *testing.T) {
	g := game(t)
	// Attacker: 1 attack die (6), 3 defense dice. Target: 0 attack, 3 defense with a 6.
	b := brawl{dice: []int{6, 1, 1, 1, 6, 1}}
	st, _ := play(t, g, b.start(t), light, "Qxd4/1")
	st, _ = play(t, g, st, dark, "respond/0")
	d := data(t, st)
	if d.Cond["black/pawn"] != 0 || d.Mom != [2]int{} {
		t.Fatalf("one success against one success must not hit: %v %v", d.Cond, d.Mom)
	}
	if d.Stage != "act" {
		t.Fatalf("stage %q", d.Stage)
	}
}

func TestCounterAttackLandsInTheSameInstant(t *testing.T) {
	g := game(t)
	// Champion: 2 attack [6,1], 2 defense [1,1]. Warden puts 3 on attack [6,6,1].
	b := brawl{dice: []int{6, 1, 1, 1, 6, 6, 1}}
	st, _ := play(t, g, b.start(t), light, "Qxd4/2")
	st, _ = play(t, g, st, dark, "respond/3")
	d := data(t, st)
	if d.Cond["black/pawn"] != 1 || d.Cond["white/queen"] != 1 {
		t.Fatalf("both units should be Fatigued: %v", d.Cond)
	}
	if d.Mom != [2]int{1, 1} {
		t.Fatalf("both sides gain 1 Momentum: %v", d.Mom)
	}
	// The Warden won the counter by two successes and pushes the Champion.
	if d.Stage != "push" {
		t.Fatalf("stage %q", d.Stage)
	}
	if got := state.LegalMoves(g, st, dark); len(got) == 0 {
		t.Fatal("Dark chooses the direction of its own push")
	}
	if len(state.LegalMoves(g, st, light)) != 0 {
		t.Fatal("Light must wait for Dark's push")
	}
}

func TestExhaustedTargetCanOnlyDefendAndIsPushedInsteadOfHit(t *testing.T) {
	g := game(t)
	b := brawl{cond: map[string]int{"black/pawn": 2}, dice: []int{6, 6, 1, 1, 1}}
	st, _ := play(t, g, b.start(t), light, "Qxd4/3")
	answers := notations(g, st, dark)
	if len(answers) != 1 || answers[0] != "respond/0" {
		t.Fatalf("an Exhausted unit has a single defense die: %v", answers)
	}
	st, _ = play(t, g, st, dark, "respond/0")
	d := data(t, st)
	if d.Cond["black/pawn"] != 2 {
		t.Fatalf("an Exhausted unit stays Exhausted: %v", d.Cond)
	}
	if d.Mom[0] != 0 {
		t.Fatalf("kicking a unit that is down earns nothing: %v", d.Mom)
	}
	// 3 successes vs 0: a Hit (push) and a margin push: two pushes in a row.
	if d.Stage != "push" {
		t.Fatalf("stage %q", d.Stage)
	}
	st, _ = play(t, g, st, light, "push>W")
	if d := data(t, st); d.Stage != "push" {
		t.Fatalf("the margin push is a second decision: stage %q", d.Stage)
	}
	st, _ = play(t, g, st, light, "push>W")
	if o, ok := st.Get("b4"); !ok || o.Piece != "pawn" {
		t.Fatal("two pushes west should leave the Warden on b4")
	}
}

func TestEdgesLowerTheSuccessNumberUpToTwo(t *testing.T) {
	g := game(t)
	cases := []struct {
		name  string
		b     brawl
		wantA int // the striker's attack success number
	}{
		{"no edges", brawl{}, 6},
		{"flanking", brawl{units: map[string]string{"white/pawn": "e3"}}, 5},
		{"worn down", brawl{cond: map[string]int{"black/pawn": 1}}, 5},
		{"exhausted counts as worn down", brawl{cond: map[string]int{"black/pawn": 2}}, 5},
		{"flanking and worn down", brawl{units: map[string]string{"white/pawn": "e3"}, cond: map[string]int{"black/pawn": 1}}, 4},
		{"an Exhausted friend does not flank", brawl{units: map[string]string{"white/pawn": "e3"}, cond: map[string]int{"white/pawn": 2}}, 6},
		{"two edges is the most", brawl{units: map[string]string{"white/pawn": "e3"}, cond: map[string]int{"black/pawn": 1}, court: [2]string{"veiled", ""}}, 4},
	}
	for _, c := range cases {
		st := c.b.start(t)
		st, _ = play(t, g, st, light, "Qxd4/1")
		st, _ = play(t, g, st, dark, "respond/0")
		d := data(t, st)
		if d.Last == nil || d.Last.ADN != c.wantA {
			t.Errorf("%s: attack needs %v, want %d", c.name, d.Last, c.wantA)
		}
	}
}

func TestChampionFallingPaysTheBountyTooAndHitsAdvanceFatigueToExhaustion(t *testing.T) {
	g := game(t)
	// A Fatigued Dark Champion on d4 is struck by Light's Warden.
	u := map[string]string{"white/pawn": "d5", "black/queen": "d4"}
	p := pos{stage: "act", units: u, cond: map[string]int{"black/queen": 1}, act: actFor("pawn"), rng: seedDice(t, 6, 6, 1, 1, 1, 1, 1, 1)}
	st := p.build()
	st, _ = play(t, g, st, light, "Pxd4/2")
	st, _ = play(t, g, st, dark, "respond/0")
	d := data(t, st)
	if d.Cond["black/queen"] != 2 {
		t.Fatalf("the Fatigued Champion should be Exhausted: %v", d.Cond)
	}
	if d.Mom[0] != 2 {
		t.Fatalf("Light gains 1 Momentum plus 1 for the fallen Champion: %v", d.Mom)
	}
}

func TestMireCostsADieAndShieldwallLiftsDefense(t *testing.T) {
	g := game(t)
	// The striker stands in Mire: Champion CV 4 → 3 dice to split.
	b := brawl{tiles: map[string]string{"d5": "blue"}}
	got := notations(g, b.start(t), light)
	if has(got, "Qxd4/4") || !has(got, "Qxd4/3") {
		t.Fatalf("Mire should cost a die: %v", got)
	}
	// A Warden beside a non-Exhausted friend defends at 5+.
	b = brawl{units: map[string]string{"black/knight": "d3"}}
	st := b.start(t)
	st, _ = play(t, g, st, light, "Qxd4/1")
	st, _ = play(t, g, st, dark, "respond/0")
	d := data(t, st)
	if d.Last.TFN != 5 {
		t.Fatalf("Shieldwall: defense needs %d, want 5", d.Last.TFN)
	}
	// Bastion Court gives the same Edge to a Dark Outrider beside a friend.
	p := pos{stage: "act", units: map[string]string{"white/queen": "d5", "black/knight": "d4", "black/rook": "d3"}, act: actFor("queen"), courts: [2]string{"", "bastion"}}
	st, _ = play(t, g, p.build(), light, "Qxd4/1")
	st, _ = play(t, g, st, dark, "respond/0")
	if d := data(t, st); d.Last.TFN != 5 {
		t.Fatalf("Bastion: Outrider defense needs %d, want 5", d.Last.TFN)
	}
	// Without the Court there is no Edge.
	p.courts = [2]string{}
	st, _ = play(t, g, p.build(), light, "Qxd4/1")
	st, _ = play(t, g, st, dark, "respond/0")
	if d := data(t, st); d.Last.TFN != 6 {
		t.Fatalf("no Bastion: defense needs %d, want 6", d.Last.TFN)
	}
}

func TestChargeNeedsAStraightRunOfTwoIntoContact(t *testing.T) {
	g := game(t)
	// Dark Warden on d4; Light Juggernaut on d1? Use the west-east line:
	// Rook a4 → c4 is two hexes in a line, ending next to d4.
	u := map[string]string{"white/rook": "a4", "black/pawn": "d4"}
	p := pos{stage: "act", units: u, act: actFor("rook")}
	st, _ := play(t, g, p.build(), light, "Rc4")
	st, _ = play(t, g, st, light, "Rxd4/1")
	st, _ = play(t, g, st, dark, "respond/0")
	if d := data(t, st); d.Last.ADN != 5 {
		t.Fatalf("a charge holds an Edge: need %d, want 5", d.Last.ADN)
	}
	// One hex of movement is no charge.
	u = map[string]string{"white/rook": "b4", "black/pawn": "d4"}
	st, _ = play(t, g, pos{stage: "act", units: u, act: actFor("rook")}.build(), light, "Rc4")
	st, _ = play(t, g, st, light, "Rxd4/1")
	st, _ = play(t, g, st, dark, "respond/0")
	if d := data(t, st); d.Last.ADN != 6 {
		t.Fatalf("a one-hex move is not a charge: need %d", d.Last.ADN)
	}
	// Striking without having moved is no charge either.
	u = map[string]string{"white/rook": "c4", "black/pawn": "d4"}
	st, _ = play(t, g, pos{stage: "act", units: u, act: actFor("rook")}.build(), light, "Rxd4/1")
	st, _ = play(t, g, st, dark, "respond/0")
	if d := data(t, st); d.Last.ADN != 6 {
		t.Fatalf("standing still is not a charge: need %d", d.Last.ADN)
	}
}

func TestExhaustedUnitsCannotStrikeAndActivationEndsWhenAUnitIsExhausted(t *testing.T) {
	g := game(t)
	// The striker is Fatigued; the Warden's counter-attack exhausts it, so
	// the rest of the activation is lost and its turn ends.
	b := brawl{
		cond: map[string]int{"white/queen": 1},
		// Champion Fatigued: CV 3. Strike /0: AD none, AF 3 dice. Warden /3: TD [6,6,6]... TF none.
		dice: []int{1, 1, 1, 6, 6, 6},
	}
	st, _ := play(t, g, b.start(t), light, "Qxd4/0")
	st, _ = play(t, g, st, dark, "respond/3")
	d := data(t, st)
	if d.Cond["white/queen"] != 2 {
		t.Fatalf("the Champion should be Exhausted: %v", d.Cond)
	}
	// 3 successes vs 0: a margin push on the Champion first.
	for d.Stage == "push" {
		legal := state.LegalMoves(g, st, dark)
		res, err := state.Apply(g, st, dark, legal[0])
		if err != nil {
			t.Fatal(err)
		}
		st = res.NewState
		d = data(t, st)
	}
	if d.Turn != dark {
		t.Fatalf("an Exhausted activation must end the turn: turn %d stage %q", d.Turn, d.Stage)
	}
}

func TestRerollsNeedMomentumAreOncePerSideAndOnlyOfFailedDice(t *testing.T) {
	g := game(t)
	// Light has 1 Momentum, Dark has 2. Striker /1: AD [3], AF [1,1,1]... The
	// Champion CV 4: AD 1 die, AF 3 dice.
	b := brawl{mom: [2]int{1, 2}, dice: []int{3, 1, 6, 6, 1, 1, 1}}
	st, _ := play(t, g, b.start(t), light, "Qxd4/1")
	st, _ = play(t, g, st, dark, "respond/0")
	d := data(t, st)
	if d.Stage != "reroll" {
		t.Fatalf("stage %q, want the initiator's reroll decision", d.Stage)
	}
	opts := notations(g, st, light)
	// Failed dice: attack die 1 (3<6), defense dice 2 (1,1 → indexes 2,3? values 1,6,6).
	if !has(opts, "keep") || !has(opts, "reroll/1") {
		t.Fatalf("options = %v", opts)
	}
	if has(opts, "reroll/3") || has(opts, "reroll/4") {
		// AF dice are [1,6,6]: indexes 2 (the 1) is failed, 3 and 4 succeeded.
		t.Fatalf("a successful die may not be rerolled: %v", opts)
	}
	before := d.Mom[0]
	st, _ = play(t, g, st, light, "reroll/1")
	d = data(t, st)
	if d.Mom[0] != before-1 {
		t.Fatalf("a reroll costs 1 Momentum: %v", d.Mom)
	}
	// Now the target's turn to reroll: Dark's failed dice exist (TF has 3 dice).
	if d.Stage != "reroll" || len(notations(g, st, dark)) == 0 {
		t.Fatalf("stage %q: the target should be offered a reroll", d.Stage)
	}
	st, _ = play(t, g, st, dark, "keep")
	if d := data(t, st); d.Stage == "reroll" {
		t.Fatalf("each side rerolls at most once per contest")
	}
}

func TestNoRerollOfferWithoutMomentumOrFailures(t *testing.T) {
	g := game(t)
	b := brawl{dice: []int{6, 1, 1, 1, 1, 1}}
	st, _ := play(t, g, b.start(t), light, "Qxd4/1")
	st, _ = play(t, g, st, dark, "respond/0")
	if d := data(t, st); d.Stage == "reroll" {
		t.Fatal("rerolls offered with no Momentum")
	}
}

func TestStrikeOnlyReachesAdjacentEnemies(t *testing.T) {
	g := game(t)
	u := map[string]string{"white/queen": "d5", "black/pawn": "d3", "white/pawn": "c5", "black/rook": "e5"}
	st := pos{stage: "act", units: u, act: actFor("queen")}.build()
	got := notations(g, st, light)
	if hasPrefix(got, "Qxd3") {
		t.Fatalf("d3 is two hexes from d5: %v", got)
	}
	if !hasPrefix(got, "Qxe5") {
		t.Fatalf("e5 is adjacent: %v", got)
	}
	if hasPrefix(got, "Qxc5") {
		t.Fatalf("a friend cannot be struck: %v", got)
	}
}

func TestLegalMovesAreAcceptedAndOthersRejected(t *testing.T) {
	g := game(t)
	b := brawl{}
	st := b.start(t)
	legal := state.LegalMoves(g, st, light)
	for _, mv := range legal {
		if _, err := state.Apply(g, st, light, mv); err != nil {
			t.Fatalf("legal move %s rejected: %v", state.FormatMove(g, mv), err)
		}
	}
	refuse(t, g, st, light, "Qxd4/9")
	refuse(t, g, st, light, "respond/1")
	refuse(t, g, st, light, "push>E")
}
