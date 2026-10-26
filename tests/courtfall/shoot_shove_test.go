package courtfall_test

import (
	"testing"
)

func TestOnlyTheArbalestShootsAndOnlyAlongClearLines(t *testing.T) {
	g := game(t)
	base := func(extra map[string]string, tiles map[string]string) []string {
		u := map[string]string{"white/bishop": "b4", "black/queen": "e4"}
		for k, v := range extra {
			u[k] = v
		}
		return notations(g, pos{stage: "act", units: u, tiles: tiles, act: actFor("bishop")}.build(), light)
	}
	if got := base(nil, nil); !has(got, "B*e4") {
		t.Fatalf("a clear line three hexes long should be shootable: %v", got)
	}
	if got := base(map[string]string{"black/pawn": "c4"}, nil); hasPrefix(got, "B*e4") {
		t.Fatalf("a unit in the way must block the shot: %v", got)
	}
	if got := base(map[string]string{"white/pawn": "d4"}, nil); hasPrefix(got, "B*e4") {
		t.Fatalf("even a friend blocks the line: %v", got)
	}
	if got := base(nil, map[string]string{"d4": "green"}); hasPrefix(got, "B*e4") {
		t.Fatalf("Overgrowth blocks line of sight through it: %v", got)
	}
	if got := base(nil, map[string]string{"e4": "green"}); !has(got, "B*e4") {
		t.Fatalf("a target standing in Overgrowth is still a target: %v", got)
	}
	// Adjacent targets cannot be shot, but can be struck.
	if got := base(map[string]string{"black/queen": "c4"}, nil); hasPrefix(got, "B*c4") || !hasPrefix(got, "Bxc4") {
		t.Fatalf("no adjacent shots: %v", got)
	}
	// Four hexes is the limit: b4 → f4 is four, a4 → f4 five.
	u := map[string]string{"white/bishop": "b4", "black/queen": "f4"}
	if got := notations(g, pos{stage: "act", units: u, act: actFor("bishop")}.build(), light); !has(got, "B*f4") {
		t.Fatalf("four hexes should be in range: %v", got)
	}
	u = map[string]string{"white/bishop": "a4", "black/queen": "f4"}
	if got := notations(g, pos{stage: "act", units: u, act: actFor("bishop")}.build(), light); hasPrefix(got, "B*") {
		t.Fatalf("five hexes is out of range: %v", got)
	}
	// Off the line.
	u = map[string]string{"white/bishop": "b4", "black/queen": "d5"}
	if got := notations(g, pos{stage: "act", units: u, act: actFor("bishop")}.build(), light); hasPrefix(got, "B*") {
		t.Fatalf("not on a hex line: %v", got)
	}
	// Other units never shoot.
	u = map[string]string{"white/queen": "b4", "black/queen": "e4"}
	if got := notations(g, pos{stage: "act", units: u, act: actFor("queen")}.build(), light); hasPrefix(got, "Q*") {
		t.Fatalf("only the Arbalest shoots: %v", got)
	}
}

func TestShotRollsThreeDiceAgainstTheDefendersTwo(t *testing.T) {
	g := game(t)
	u := map[string]string{"white/bishop": "b4", "black/queen": "e4"}
	p := pos{stage: "act", units: u, act: actFor("bishop"), rng: seedDice(t, 6, 6, 1, 1, 1)}
	st, _ := play(t, g, p.build(), light, "B*e4")
	d := data(t, st)
	if d.Last == nil || d.Last.Kind != "shoot" || len(d.Last.AD) != 3 || len(d.Last.TF) != 2 {
		t.Fatalf("contest = %+v", d.Last)
	}
	if d.Cond["black/queen"] != 1 || d.Mom[0] != 1 {
		t.Fatalf("a successful shot should Fatigue the target and pay Momentum: %v %v", d.Cond, d.Mom)
	}
	// No counter-fire: the shooter is untouched.
	if d.Cond["white/bishop"] != 0 {
		t.Fatalf("the Arbalest was hit: %v", d.Cond)
	}
	// Two successes to none: the target is pushed, the shooter chooses.
	if d.Stage != "push" || d.Turn != light {
		t.Fatalf("stage %q: a shot won by two pushes the target", d.Stage)
	}
}

func TestOvergrowthGivesTheTargetADefensiveEdgeAgainstShots(t *testing.T) {
	g := game(t)
	u := map[string]string{"white/bishop": "b4", "black/queen": "e4"}
	p := pos{stage: "act", units: u, tiles: map[string]string{"e4": "green"}, act: actFor("bishop"), rng: seedDice(t, 1, 1, 1, 1, 1)}
	st, _ := play(t, g, p.build(), light, "B*e4")
	if d := data(t, st); d.Last.TFN != 5 {
		t.Fatalf("a defender in Overgrowth needs %d, want 5", d.Last.TFN)
	}
	// With a Warden beside it too the Edges stack but never beyond 4+.
	u = map[string]string{"white/bishop": "b4", "black/pawn": "e4", "black/rook": "f4"}
	p = pos{stage: "act", units: u, tiles: map[string]string{"e4": "green"}, act: actFor("bishop"), rng: seedDice(t, 1, 1, 1, 1, 1)}
	st, _ = play(t, g, p.build(), light, "B*e4")
	if d := data(t, st); d.Last.TFN != 4 {
		t.Fatalf("Overgrowth + Shieldwall should reach 4+, got %d", d.Last.TFN)
	}
}

func TestFatiguedShooterAndMireCostDice(t *testing.T) {
	g := game(t)
	u := map[string]string{"white/bishop": "b4", "black/queen": "e4"}
	p := pos{stage: "act", units: u, cond: map[string]int{"white/bishop": 1}, tiles: map[string]string{"b4": "blue"}, act: actFor("bishop"), rng: 99}
	st, _ := play(t, g, p.build(), light, "B*e4")
	if d := data(t, st); len(d.Last.AD) != 1 {
		t.Fatalf("a Fatigued Arbalest in Mire rolls 3-1-1 = 1 die, got %d", len(d.Last.AD))
	}
	// A Fatigued defender rolls one die, a Fresh one two.
	p = pos{stage: "act", units: u, cond: map[string]int{"black/queen": 1}, act: actFor("bishop"), rng: 99}
	st, _ = play(t, g, p.build(), light, "B*e4")
	if d := data(t, st); len(d.Last.TF) != 1 {
		t.Fatalf("a Fatigued defender rolls 1 die, got %d", len(d.Last.TF))
	}
}

// shoveScene: Light's Juggernaut on b4 shoving Dark's Warden on c4 (east).
func shoveScene(extra map[string]string, tiles map[string]string, cond map[string]int, courts [2]string, dice ...int) pos {
	u := map[string]string{"white/rook": "b4", "black/pawn": "c4"}
	for k, v := range extra {
		u[k] = v
	}
	p := pos{stage: "act", units: u, tiles: tiles, cond: cond, courts: courts, act: actFor("rook")}
	if len(dice) > 0 {
		p.rng = seedDice(&testing.T{}, dice...)
	}
	return p
}

func TestShoveWinsOnFivePlusAndPushesAJuggernautTwoHexes(t *testing.T) {
	g := game(t)
	p := shoveScene(nil, nil, nil, [2]string{})
	p.rng = seedDice(t, 5, 5, 5, 1, 1, 1)
	st, _ := play(t, g, p.build(), light, "R^c4")
	d := data(t, st)
	if d.Last == nil || d.Last.ADN != 5 || d.Last.TFN != 6 {
		t.Fatalf("shove numbers = %+v", d.Last)
	}
	if o, ok := st.Get("e4"); !ok || o.Piece != "pawn" {
		t.Fatalf("Bulldoze should push two hexes to e4\n%s", dump(g, st))
	}
	if d.Cond["black/pawn"] != 0 {
		t.Fatalf("a shove never inflicts a Hit: %v", d.Cond)
	}
	if d.Mom != [2]int{} {
		t.Fatalf("an ordinary shove pays nothing: %v", d.Mom)
	}
	if d.Act.Left != 1 || d.Stage != "act" {
		t.Fatalf("a shove costs one action: %+v %q", d.Act, d.Stage)
	}
}

func TestShoveNeedsStrictlyMoreSuccessesAndFlankingHelps(t *testing.T) {
	g := game(t)
	// One success each: no push.
	p := shoveScene(nil, nil, nil, [2]string{})
	p.rng = seedDice(t, 5, 1, 1, 6, 1, 1)
	st, _ := play(t, g, p.build(), light, "R^c4")
	if o, ok := st.Get("c4"); !ok || o.Piece != "pawn" {
		t.Fatal("a tied shove must not move the target")
	}
	// A flanking friend lowers the number to 4+.
	p = shoveScene(map[string]string{"white/pawn": "c3"}, nil, nil, [2]string{})
	p.rng = seedDice(t, 4, 1, 1, 1, 1, 1)
	st, _ = play(t, g, p.build(), light, "R^c4")
	d := data(t, st)
	if d.Last.ADN != 4 {
		t.Fatalf("flanked shove needs %d, want 4", d.Last.ADN)
	}
	if _, ok := st.Get("c4"); ok {
		t.Fatal("the Warden should have been shoved off c4")
	}
}

func TestExhaustedUnitsLoseShovesAutomatically(t *testing.T) {
	g := game(t)
	p := shoveScene(nil, nil, map[string]int{"black/pawn": 2}, [2]string{})
	st, _ := play(t, g, p.build(), light, "R^c4")
	d := data(t, st)
	if len(d.Last.AD) != 0 || !d.Last.Auto {
		t.Fatalf("no dice should be rolled: %+v", d.Last)
	}
	if o, ok := st.Get("e4"); !ok || o.Piece != "pawn" {
		t.Fatal("the Exhausted Warden should be shoved two hexes")
	}
}

func TestShoveIsBlockedByUnitsAndWardensHoldTheirGround(t *testing.T) {
	g := game(t)
	// A unit right behind the target: nothing to do.
	p := shoveScene(map[string]string{"black/queen": "d4"}, nil, nil, [2]string{})
	if got := notations(g, p.build(), light); hasPrefix(got, "R^c4") {
		t.Fatalf("a shove that cannot move anyone should not be offered: %v", got)
	}
	// A Warden beside a non-Exhausted friend cannot be pushed at all.
	p = shoveScene(map[string]string{"black/knight": "c3"}, nil, nil, [2]string{})
	if got := notations(g, p.build(), light); hasPrefix(got, "R^c4") {
		t.Fatalf("an un-pushable Warden should not be a shove target: %v", got)
	}
	// Beside only an Exhausted friend, it can be pushed.
	p = shoveScene(map[string]string{"black/knight": "c3"}, nil, map[string]int{"black/knight": 2}, [2]string{})
	if got := notations(g, p.build(), light); !has(got, "R^c4") {
		t.Fatalf("an Exhausted friend does not hold the Warden: %v", got)
	}
}

func TestPushedIntoAHazardIsAHitAndAPushBonus(t *testing.T) {
	g := game(t)
	// Rook shoves two hexes: c4 → d4 is Hazard: the push stops there.
	p := shoveScene(nil, map[string]string{"d4": "red"}, nil, [2]string{})
	p.rng = seedDice(t, 5, 5, 5, 1, 1, 1)
	st, _ := play(t, g, p.build(), light, "R^c4")
	d := data(t, st)
	if o, ok := st.Get("d4"); !ok || o.Piece != "pawn" {
		t.Fatalf("the push should stop on the Hazard\n%s", dump(g, st))
	}
	if d.Cond["black/pawn"] != 1 {
		t.Fatalf("the Hazard Hits: %v", d.Cond)
	}
	if d.Mom[0] != 1 {
		t.Fatalf("pushing an enemy into a Hazard pays 1 Momentum: %v", d.Mom)
	}
	// The Cinder Court pays one more and pushes one further (3 hexes, but
	// the Hazard stops it anyway).
	p = shoveScene(nil, map[string]string{"d4": "red"}, nil, [2]string{"cinder", ""})
	p.rng = seedDice(t, 5, 5, 5, 1, 1, 1)
	st, _ = play(t, g, p.build(), light, "R^c4")
	if d := data(t, st); d.Mom[0] != 2 {
		t.Fatalf("Cinder pays 2 Momentum for the Hazard push: %v", d.Mom)
	}
}

func TestPushedOffTheEdgeIsAHitAndTheUnitStays(t *testing.T) {
	g := game(t)
	u := map[string]string{"white/rook": "f4", "black/pawn": "g4"}
	p := pos{stage: "act", units: u, act: actFor("rook"), rng: seedDice(t, 5, 5, 5, 1, 1, 1)}
	st, _ := play(t, g, p.build(), light, "R^g4")
	d := data(t, st)
	if o, ok := st.Get("g4"); !ok || o.Piece != "pawn" {
		t.Fatal("the unit stays in the hex it left")
	}
	if d.Cond["black/pawn"] != 1 || d.Mom[0] != 1 {
		t.Fatalf("edge: cond %v mom %v", d.Cond, d.Mom)
	}
}

func TestCinderShovesPushOneExtraHex(t *testing.T) {
	g := game(t)
	u := map[string]string{"white/pawn": "a4", "black/knight": "b4"}
	p := pos{stage: "act", units: u, courts: [2]string{"cinder", ""}, act: actFor("pawn"), rng: seedDice(t, 5, 5, 5, 1, 1, 1)}
	st, _ := play(t, g, p.build(), light, "P^b4")
	if o, ok := st.Get("d4"); !ok || o.Piece != "knight" {
		t.Fatalf("a Cinder shove by a Warden pushes 1+1 hexes, to d4\n%s", dump(g, st))
	}
	p.courts = [2]string{}
	st, _ = play(t, g, p.build(), light, "P^b4")
	if o, ok := st.Get("c4"); !ok || o.Piece != "knight" {
		t.Fatalf("without Cinder a Warden's shove pushes exactly 1 hex, to c4\n%s", dump(g, st))
	}
}

func TestChainPushOfExhaustedUnitIntoAHazard(t *testing.T) {
	g := game(t)
	// Exhausted Dark pawn shoved into a Hazard: Hit on an Exhausted unit
	// pushes it on, by the shover's choice, never off the edge.
	u := map[string]string{"white/pawn": "a4", "black/knight": "b4"}
	p := pos{stage: "act", units: u, cond: map[string]int{"black/knight": 2}, tiles: map[string]string{"c4": "red"}, act: actFor("pawn")}
	st, _ := play(t, g, p.build(), light, "P^b4")
	d := data(t, st)
	if d.Stage != "push" {
		t.Fatalf("stage %q: the Hazard's Hit should push the Exhausted unit on", d.Stage)
	}
	for _, n := range notations(g, st, light) {
		if n == "push>W" {
			// west of c4 is b4 (empty now): allowed
			continue
		}
	}
	if d.Mom[0] != 1 {
		t.Fatalf("the Hazard push pays 1: %v", d.Mom)
	}
	// A chain push may not go off the edge.
	u = map[string]string{"white/pawn": "e4", "black/knight": "f4"}
	p = pos{stage: "act", units: u, cond: map[string]int{"black/knight": 2}, tiles: map[string]string{"g4": "red"}, act: actFor("pawn")}
	st, _ = play(t, g, p.build(), light, "P^f4")
	for _, n := range notations(g, st, light) {
		if n == "push>E" {
			t.Fatalf("the chain push must not leave the arena (g4 is on the edge): %v", notations(g, st, light))
		}
	}
}

func TestHitAndRunShiftsTheOutriderAfterItsOwnStrike(t *testing.T) {
	g := game(t)
	u := map[string]string{"white/knight": "d5", "black/pawn": "d4"}
	p := pos{stage: "act", units: u, act: actFor("knight"), rng: seedDice(t, 1, 1, 1, 1, 1, 1)}
	st, _ := play(t, g, p.build(), light, "Nxd4/0")
	st, _ = play(t, g, st, dark, "respond/0")
	d := data(t, st)
	if d.Stage != "shift" || d.Turn != light {
		t.Fatalf("stage %q: Hit and Run should offer a free shift", d.Stage)
	}
	got := notations(g, st, light)
	if !has(got, "stay") || !hasPrefix(got, "shift>") {
		t.Fatalf("options = %v", got)
	}
	st, _ = play(t, g, st, light, "shift>d6")
	if o, ok := st.Get("d6"); !ok || o.Piece != "knight" {
		t.Fatal("the Outrider should stand on d6")
	}
	if d := data(t, st); d.Act.Left != 1 {
		t.Fatalf("the shift is free: %+v", d.Act)
	}
	// Other units do not get Hit and Run, and neither does a shove.
	u = map[string]string{"white/pawn": "d5", "black/pawn": "d4"}
	p = pos{stage: "act", units: u, act: actFor("pawn"), rng: seedDice(t, 1, 1, 1, 1, 1, 1)}
	st, _ = play(t, g, p.build(), light, "Pxd4/0")
	st, _ = play(t, g, st, dark, "respond/0")
	if d := data(t, st); d.Stage == "shift" {
		t.Fatal("only the Outrider has Hit and Run")
	}
}

func TestSurgeIntoAHazardIsAHit(t *testing.T) {
	g := game(t)
	u := map[string]string{"white/pawn": "d6", "black/queen": "d2"}
	p := pos{units: u, mom: [2]int{1, 0}, tiles: map[string]string{"d5": "red"}}
	st, _ := play(t, g, p.build(), light, "surge:P>d5")
	if c := data(t, st).Cond["white/pawn"]; c != 1 {
		t.Fatalf("surging into fire should Fatigue the Warden: %d", c)
	}
}

func TestStoneCourtSwapsTwoEmptyTilesOncePerClear(t *testing.T) {
	g := game(t)
	u := map[string]string{"white/pawn": "d6", "black/queen": "d2"}
	p := pos{units: u, courts: [2]string{"stone", ""}, tiles: map[string]string{"c4": "green", "e4": "blue"}}
	got := notations(g, p.build(), light)
	if !has(got, "tiles:c4<>d3") && !has(got, "tiles:c4<>c3") {
		t.Fatalf("a tile should be swappable with a neighbouring open hex: %v", got)
	}
	// The Eye never moves, nor does a tile enter a deployment zone.
	for _, n := range got {
		if len(n) > 6 && (contains(n, "d4") || contains(n, "d6") || contains(n, "c6") || contains(n, "d2")) && n[:6] == "tiles:" {
			t.Fatalf("illegal swap offered: %s", n)
		}
	}
	st, _ := play(t, g, p.build(), light, "tiles:c4<>c3")
	d := data(t, st)
	if d.Tiles["c3"] != "green" || d.Tiles["c4"] != "" {
		t.Fatalf("tiles after the swap: %v", d.Tiles)
	}
	if !d.StoneUsed[0] || hasPrefix(notations(g, st, light), "tiles:") {
		t.Fatal("the swap is once per Clear Turn")
	}
	// Units block a swap, and other Courts cannot swap at all.
	p2 := pos{units: map[string]string{"white/pawn": "c3", "black/queen": "d2"}, courts: [2]string{"stone", ""}, tiles: map[string]string{"c4": "green"}}
	if has(notations(g, p2.build(), light), "tiles:c3<>c4") {
		t.Fatal("an occupied hex cannot be swapped")
	}
	p3 := pos{units: u, tiles: map[string]string{"c4": "green"}}
	if hasPrefix(notations(g, p3.build(), light), "tiles:") {
		t.Fatal("only the Stone Court swaps tiles")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestIsolationIsAnEdgeOnlyInExchanges(t *testing.T) {
	g := game(t)
	// Light is the Veiled Court; Dark's Warden stands alone. A strike gets
	// the Isolation Edge, a shove does not.
	p := shoveScene(nil, nil, nil, [2]string{"veiled", ""})
	p.rng = seedDice(t, 1, 1, 1, 1, 1, 1)
	st, _ := play(t, g, p.build(), light, "R^c4")
	if d := data(t, st); d.Last.ADN != 5 {
		t.Fatalf("an isolated target still needs 5+ for a shove: %d", d.Last.ADN)
	}
	strike := pos{stage: "act", units: map[string]string{"white/rook": "b4", "black/pawn": "c4"}, courts: [2]string{"veiled", ""}, act: actFor("rook")}
	st, _ = play(t, g, strike.build(), light, "Rxc4/1")
	st, _ = play(t, g, st, dark, "respond/0")
	if d := data(t, st); d.Last.ADN != 5 {
		t.Fatalf("Isolation should lower a strike to 5+: %d", d.Last.ADN)
	}
}
