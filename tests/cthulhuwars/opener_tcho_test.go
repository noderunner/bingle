package cthulhuwars_test

import (
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The Opener of the Way and the Tcho-Tcho, on tables that seat them.

// Spellbook numbers of the two cults, in the order of their faction cards.
const (
	owChannel = iota
	owAscend
	owDescend
	owCurse
	owMillion
	owBreak
)

const (
	ttHiero = iota
	ttSoulless
	ttTerror
	ttIdolatry
	ttMartyr
	ttTablets
)

const (
	rqOWGates8 = iota
	rqOWGates12
	rqOWEnemyGates
	rqOWLose
	rqOWMeet
	rqOWAwaken
)

const (
	rqTTRitual = iota
	rqTTSign
	rqTTThree
	rqTTGate
	rqTTKilled
	rqTTAwaken
)

func owTable(t testing.TB) *table {
	return newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ow"}, false)
}

func ttTable(t testing.TB, hp bool) *table {
	return newTable(t, "cthulhu-wars", []string{"gc", "cc", "ys", "tt"}, hp)
}

// --- the Opener of the Way ----------------------------------------------------------------

func TestOpenerSetsUpLastInAnEmptyArea(t *testing.T) {
	tb := owTable(t)
	st := tb.start(3)
	d := tb.data(st)
	ow := tb.seat("ow")
	if d.Stage != "setup" || d.Setup != ow {
		t.Fatalf("stage %q, setup seat %d, want the Opener (%d)", d.Stage, d.Setup, ow)
	}
	got := tb.can(st, "ow")
	// The others already stand in their start areas: they are not on offer.
	for _, busy := range []string{"south-pacific", "asia", "south-america-west"} {
		if has(got, "start@"+busy) {
			t.Errorf("start@%s offered although it holds a cult", busy)
		}
	}
	if !has(got, "start@africa") || !has(got, "start@antarctica") {
		t.Fatalf("empty areas not offered: %v", got)
	}
	st = tb.as(st, "ow", "start@africa")
	d = tb.data(st)
	if d.Stage != "direction" || d.Gates["africa"] != ow || tb.n(st, "ow", "cultist", "africa") != 6 || d.Power[ow] != 8 {
		t.Errorf("after the start: stage %q gates %v cultists %d power %d", d.Stage, d.Gates, tb.n(st, "ow", "cultist", "africa"), d.Power[ow])
	}
	tb.valid(st)
}

func TestTheBeyondOneMovesAUnitTheGateAndItsKeeper(t *testing.T) {
	tb := owTable(t)
	build := func(extra ...xu) *state.BoardState {
		units := append([]xu{{"ow", "abomination", "europe", 1}, {"ow", "mutant", "europe", 1}, {"gc", "cultist", "asia", 1}, {"ow", "cultist", "asia", 1}}, extra...)
		return tb.build(xp{
			units: units, gates: map[string]string{"europe": "ow", "asia": "gc"},
			power: map[string]int{"ow": 4, "cc": 5}, turn: "ow",
			edit: func(d *cthulhuwars.Data) {
				d.Holders = map[string]string{"asia": "cultist"}
			},
		})
	}
	st := build(xu{"ow", "cultist", "europe", 1})
	got := tb.can(st, "ow")
	if !has(got, "beyond:abomination@europe>africa") {
		t.Fatalf("The Beyond One not offered: %v", got)
	}
	if has(got, "beyond:mutant@europe>africa") || has(got, "beyond:abomination@europe>asia") {
		t.Errorf("a Mutant costs 2; Asia already has a Gate: %v", got)
	}
	st = tb.as(st, "ow", "beyond:abomination@europe>africa")
	d := tb.data(st)
	if _, still := d.Gates["europe"]; still || d.Gates["africa"] != tb.seat("ow") || d.Power[tb.seat("ow")] != 3 {
		t.Errorf("gates %v power %d", d.Gates, d.Power[tb.seat("ow")])
	}
	if tb.n(st, "ow", "abomination", "africa") != 1 || tb.n(st, "ow", "cultist", "africa") != 1 || tb.n(st, "ow", "mutant", "europe") != 1 {
		t.Errorf("units: abomination %d cultist %d in Africa, mutant %d left in Europe", tb.n(st, "ow", "abomination", "africa"), tb.n(st, "ow", "cultist", "africa"), tb.n(st, "ow", "mutant", "europe"))
	}
	tb.valid(st)

	// An enemy's Gate goes too, with the unit that holds it, when the Opener
	// has a unit of cost 3 in its area.
	st = tb.build(xp{
		units: []xu{{"ow", "abomination", "asia", 1}, {"gc", "cultist", "asia", 1}},
		gates: map[string]string{"asia": "gc"}, power: map[string]int{"ow": 4, "cc": 5}, turn: "ow",
		edit: func(d *cthulhuwars.Data) { d.Holders = map[string]string{"asia": "cultist"} },
	})
	st = tb.as(st, "ow", "beyond:abomination@asia>europe")
	d = tb.data(st)
	if d.Gates["europe"] != tb.seat("gc") || tb.n(st, "gc", "cultist", "europe") != 1 || tb.n(st, "gc", "cultist", "asia") != 0 {
		t.Errorf("the enemy Gate and its Acolyte did not move: gates %v", d.Gates)
	}
	tb.valid(st)

	// Not where an enemy Great Old One stands.
	st = tb.build(xp{
		units: []xu{{"ow", "abomination", "asia", 1}, {"gc", "cthulhu", "asia", 1}, {"gc", "cultist", "asia", 1}},
		gates: map[string]string{"asia": "gc"}, power: map[string]int{"ow": 4, "cc": 5}, turn: "ow",
	})
	if got := tb.can(st, "ow"); hasPrefix(got, "beyond:") {
		t.Errorf("The Beyond One beside Cthulhu: %v", got)
	}
}

func TestYogSothothIsAGate(t *testing.T) {
	tb := owTable(t)
	ow := tb.seat("ow")
	st := tb.build(xp{
		units: []xu{{"ow", "yog-sothoth", "africa", 1}, {"ow", "cultist", "europe", 1}},
		gates: map[string]string{"europe": "ow"}, power: map[string]int{"ow": 6, "cc": 5}, turn: "ow",
	})
	got := tb.can(st, "ow")
	// Monsters are summoned at him, but no Gate can be built where he stands.
	if !has(got, "summon:mutant@africa") {
		t.Errorf("no summoning at Yog-Sothoth: %v", got)
	}
	// He is a Gate for Doom: a Gate and Yog-Sothoth are 2 Doom in the Doom phase.
	final := tb.build(xp{
		units: []xu{{"ow", "yog-sothoth", "africa", 1}, {"ow", "cultist", "europe", 1}},
		gates: map[string]string{"europe": "ow"}, power: map[string]int{"cc": 1}, turn: "cc",
	})
	final = tb.as(final, "cc", "pass")
	final = toDoom(t, tb, final)
	if d := tb.data(final); d.Doom[ow] != 2 {
		t.Errorf("Doom of the Opener %d in the Doom phase, want 2 (a Gate and Yog-Sothoth)", d.Doom[ow])
	}
	// He is no Gate for The Beyond One: Africa has none, so it is a free area.
	st = tb.build(xp{
		units: []xu{{"ow", "yog-sothoth", "africa", 1}, {"ow", "abomination", "europe", 1}, {"ow", "cultist", "europe", 1}},
		gates: map[string]string{"europe": "ow"}, power: map[string]int{"ow": 6, "cc": 5}, turn: "ow",
	})
	if got := tb.can(st, "ow"); !has(got, "beyond:abomination@europe>africa") {
		t.Errorf("Africa, with only Yog-Sothoth, is an area without a Gate: %v", got)
	}
	// His Combat is twice the enemy-controlled Great Old Ones in play.
	st = tb.build(xp{
		units: []xu{{"ow", "yog-sothoth", "africa", 1}, {"gc", "cthulhu", "south-pacific", 1}, {"cc", "nyarlathotep", "asia", 1}, {"gc", "cultist", "africa", 1}},
		power: map[string]int{"ow": 3, "cc": 5}, turn: "ow", rng: seedDice(t, 1, 1, 1, 1, 1),
	})
	st = tb.as(st, "ow", "battle@africa/gc")
	if last := tb.data(st).Last; last == nil || last.Combat[tb.seat("ow")] != 4 {
		t.Errorf("Yog-Sothoth rolled %+v, want 4 dice (two enemy Great Old Ones)", last)
	}
}

func TestYogSothothIsAwakenedByReplacingASpawn(t *testing.T) {
	tb := owTable(t)
	st := tb.build(xp{
		units: []xu{{"ow", "spawn-of-yog-sothoth", "africa", 1}, {"ow", "cultist", "africa", 1}},
		gates: map[string]string{"africa": "ow"}, power: map[string]int{"ow": 7, "cc": 5}, turn: "ow",
	})
	got := tb.can(st, "ow")
	if !has(got, "awaken:yog-sothoth@africa") {
		t.Fatalf("Yog-Sothoth cannot be awakened: %v", got)
	}
	st = tb.as(st, "ow", "awaken:yog-sothoth@africa")
	d := tb.data(st)
	if tb.n(st, "ow", "yog-sothoth", "africa") != 1 || tb.n(st, "ow", "spawn-of-yog-sothoth", "africa") != 0 || d.Power[tb.seat("ow")] != 1 {
		t.Errorf("Yog %d, Spawn %d, Power %d", tb.n(st, "ow", "yog-sothoth", "africa"), tb.n(st, "ow", "spawn-of-yog-sothoth", "africa"), d.Power[tb.seat("ow")])
	}
	if !reqDone(d, tb.seat("ow"), rqOWAwaken) {
		t.Errorf("the awakening requirement is not met")
	}
	tb.valid(st)
}

func TestTheyBreakThroughSummonsAtAnEnemyGate(t *testing.T) {
	tb := owTable(t)
	build := func(book bool) *state.BoardState {
		return tb.build(xp{
			units: []xu{{"ow", "cultist", "europe", 1}, {"gc", "cultist", "asia", 1}},
			gates: map[string]string{"europe": "ow", "asia": "gc"}, power: map[string]int{"ow": 4, "cc": 5}, turn: "ow",
			books: map[string][]int{"ow": {owBreak}},
			edit: func(d *cthulhuwars.Data) {
				if !book {
					d.Books[tb.seat("ow")] = 0
				}
			},
		})
	}
	if got := tb.can(build(false), "ow"); has(got, "summon:mutant@asia") {
		t.Errorf("summoning at an enemy Gate without the book: %v", got)
	}
	st := build(true)
	if got := tb.can(st, "ow"); !has(got, "summon:mutant@asia") {
		t.Fatalf("They Break Through: %v", got)
	}
	st = tb.as(st, "ow", "summon:mutant@asia")
	if tb.n(st, "ow", "mutant", "asia") != 1 {
		t.Errorf("no Mutant in Asia")
	}
}

func TestTheMillionFavoredOnesPromotesSurvivorsAfterABattle(t *testing.T) {
	tb := owTable(t)
	ow := tb.seat("ow")
	st := tb.build(xp{
		units: []xu{{"ow", "cultist", "africa", 2}, {"ow", "mutant", "africa", 1}, {"gc", "deep-one", "africa", 1}},
		power: map[string]int{"ow": 4, "cc": 5}, turn: "ow", books: map[string][]int{"ow": {owMillion}},
		rng: seedDice(t, 1, 1, 1, 1),
	})
	st = tb.as(st, "ow", "battle@africa/gc")
	d := tb.data(st)
	if d.Stage != "post" || d.Bat == nil || d.Bat.Who != ow {
		t.Fatalf("stage %q: the post-battle window of the Opener", d.Stage)
	}
	got := tb.can(st, "ow")
	if !has(got, "upgrade:cultist") || !has(got, "upgrade:mutant") || !has(got, "done") {
		t.Fatalf("upgrades %v", got)
	}
	st = tb.as(st, "ow", "upgrade:mutant")
	st = tb.as(st, "ow", "upgrade:cultist")
	if tb.n(st, "ow", "abomination", "africa") != 1 || tb.n(st, "ow", "mutant", "africa") != 1 || tb.n(st, "ow", "cultist", "africa") != 1 {
		t.Errorf("after two promotions: abomination %d mutant %d cultist %d", tb.n(st, "ow", "abomination", "africa"), tb.n(st, "ow", "mutant", "africa"), tb.n(st, "ow", "cultist", "africa"))
	}
	tb.valid(st)
}

func TestChannelPowerRerollsTheDiceThatMissed(t *testing.T) {
	tb := owTable(t)
	ow := tb.seat("ow")
	// Three Abominations roll 6 dice: 1 1 1 1 1 1, all misses.
	st := tb.build(xp{
		units: []xu{{"ow", "abomination", "africa", 3}, {"gc", "deep-one", "africa", 1}},
		power: map[string]int{"ow": 4, "cc": 5}, turn: "ow", books: map[string][]int{"ow": {owChannel}},
		rng: seedDice(t, 1, 1, 1, 1, 1, 1, 1),
	})
	st = tb.as(st, "ow", "battle@africa/gc")
	d := tb.data(st)
	if d.Stage != "prompt" || d.Prompts[0].Kind != "channel" || d.Prompts[0].Seat != ow {
		t.Fatalf("stage %q prompts %+v", d.Stage, d.Prompts)
	}
	before := d.Power[ow]
	st = tb.as(st, "ow", "channel")
	d = tb.data(st)
	if d.Power[ow] != before-1 {
		t.Errorf("Power %d, want %d", d.Power[ow], before-1)
	}
	// Done closes the window: the results are applied.
	if d.Stage == "prompt" && d.Prompts[0].Kind == "channel" {
		st = tb.as(st, "ow", "done")
	}
	if d = tb.data(st); d.Stage == "prompt" && len(d.Prompts) > 0 && d.Prompts[0].Kind == "channel" {
		t.Errorf("still channelling: %+v", d.Prompts)
	}
	tb.valid(st)
}

func TestDragonAscendingAndDescending(t *testing.T) {
	tb := owTable(t)
	ow := tb.seat("ow")
	st := tb.build(xp{
		power: map[string]int{"ow": 3, "gc": 9, "cc": 5}, turn: "ow", books: map[string][]int{"ow": {owAscend}},
		units: []xu{{"ow", "cultist", "europe", 1}},
	})
	if got := tb.can(st, "ow"); !has(got, "ascend:gc") || has(got, "ascend:cc") == false {
		// Both enemies have more Power than the Opener.
		t.Fatalf("moves %v", got)
	}
	st = tb.as(st, "ow", "ascend:gc")
	if d := tb.data(st); d.Power[ow] != 9 {
		t.Errorf("Power %d, want 9", d.Power[ow])
	}
	// Once only.
	if got := tb.can(st, "ow"); hasPrefix(got, "ascend:") {
		t.Errorf("a second Ascending: %v", got)
	}

	// Descending: the Ritual's Doom twice, once.
	st = tb.build(xp{
		phase: "doom", stage: "doom", turn: "ow", power: map[string]int{"ow": 9},
		units: []xu{{"ow", "cultist", "europe", 1}, {"ow", "cultist", "asia", 1}},
		gates: map[string]string{"europe": "ow", "asia": "ow"}, books: map[string][]int{"ow": {owDescend}},
		edit: func(d *cthulhuwars.Data) { d.Doomx = &cthulhuwars.DoomState{Order: []int{ow}} },
	})
	got := tb.can(st, "ow")
	if !has(got, "ritual") || !has(got, "ritual:descend") {
		t.Fatalf("moves %v", got)
	}
	st = tb.as(st, "ow", "ritual:descend")
	if d := tb.data(st); d.Doom[ow] != 4 {
		t.Errorf("Doom %d after Descending with 2 Gates, want 4", d.Doom[ow])
	}
}

func TestDreadCurseAppliesResultsToAnEnemyInAnArea(t *testing.T) {
	tb := owTable(t)
	ow := tb.seat("ow")
	// An Abomination and a Spawn roll 2 dice: a Kill and a Pain.
	st := tb.build(xp{
		units: []xu{{"ow", "abomination", "europe", 1}, {"ow", "spawn-of-yog-sothoth", "europe", 1}, {"gc", "deep-one", "asia", 2}},
		power: map[string]int{"ow": 5, "cc": 5}, turn: "ow", books: map[string][]int{"ow": {owCurse}},
		rng: seedDice(t, 6, 5),
	})
	got := tb.can(st, "ow")
	if !has(got, "curse@asia") || has(got, "curse@europe") {
		t.Fatalf("areas to curse: %v", got)
	}
	st = tb.as(st, "ow", "curse@asia")
	d := tb.data(st)
	// The Kill and the Pain fall on the only enemy, on its only kind of unit; the
	// Pained Deep One is moved by the Opener.
	if tb.n(st, "gc", "deep-one", "asia") != 1 {
		t.Errorf("%d Deep Ones left in Asia, want 1 (one killed, one Pained)", tb.n(st, "gc", "deep-one", "asia"))
	}
	if d.Stage != "prompt" || d.Prompts[0].Kind != "curse-move" || d.Prompts[0].Seat != ow {
		t.Fatalf("stage %q prompts %+v: the Opener places the Pained unit", d.Stage, d.Prompts)
	}
	got = tb.can(st, "ow")
	if !has(got, "retreat:deep-one>europe/gc") || !has(got, "retreat:deep-one>africa/gc") {
		t.Fatalf("destinations %v", got)
	}
	st = tb.as(st, "ow", "retreat:deep-one>africa/gc")
	if tb.n(st, "gc", "deep-one", "africa") != 1 || tb.n(st, "gc", "deep-one", "asia") != 0 || tb.data(st).Power[ow] != 3 {
		t.Errorf("the Pained Deep One: %d in Africa, %d in Asia; Power %d", tb.n(st, "gc", "deep-one", "africa"), tb.n(st, "gc", "deep-one", "asia"), tb.data(st).Power[ow])
	}
	tb.valid(st)
}

func TestOpenerRequirements(t *testing.T) {
	tb := owTable(t)
	ow := tb.seat("ow")
	gates := map[string]string{}
	for _, a := range []string{"europe", "asia", "africa", "antarctica", "australia", "new-zealand", "central-america", "north-america-east"} {
		gates[a] = "ow"
	}
	st := tb.build(xp{
		units: []xu{{"ow", "cultist", "europe", 1}}, gates: gates, power: map[string]int{"ow": 3, "cc": 5}, turn: "ow",
	})
	// Any action checks the requirements: 8 Gates meet the first.
	st = tb.as(st, "ow", "recruit@europe")
	d := tb.data(st)
	if !reqDone(d, ow, rqOWGates8) || reqDone(d, ow, rqOWGates12) {
		t.Errorf("reqs %b: 8 Gates are on the map, not 12", d.Reqs[ow])
	}
	// Meeting a Great Old One with Yog-Sothoth.
	st = tb.build(xp{
		units: []xu{{"ow", "yog-sothoth", "asia", 1}, {"gc", "cthulhu", "asia", 1}, {"ow", "cultist", "europe", 1}},
		power: map[string]int{"ow": 3, "cc": 5}, turn: "ow",
	})
	st = tb.as(st, "ow", "recruit@europe")
	if !reqDone(tb.data(st), ow, rqOWMeet) {
		t.Errorf("Yog-Sothoth with Cthulhu does not meet the requirement")
	}
}

// --- the Tcho-Tcho -------------------------------------------------------------------------------

func TestTchoTchoBeginsInAnEmptyGlyphAreaWithAHighPriest(t *testing.T) {
	tb := ttTable(t, true)
	st := tb.start(3)
	tt := tb.seat("tt")
	got := tb.can(st, "tt")
	if !has(got, "start@africa") || has(got, "start@antarctica") || has(got, "start@south-pacific") {
		t.Fatalf("glyph areas on offer: %v", got)
	}
	st = tb.as(st, "tt", "start@africa")
	d := tb.data(st)
	if d.TTStart != "africa" || d.Gates["africa"] != tt || d.Growth != 0 {
		t.Errorf("start %q gates %v growth %d", d.TTStart, d.Gates, d.Growth)
	}
	// With the High Priests option one Acolyte is a High Priest.
	if tb.n(st, "tt", "cultist", "africa") != 5 || tb.n(st, "tt", "high-priest", "africa") != 1 || tb.pool(st, "tt", "high-priest") != 2 {
		t.Errorf("Acolytes %d, High Priests %d (pool %d)", tb.n(st, "tt", "cultist", "africa"), tb.n(st, "tt", "high-priest", "africa"), tb.pool(st, "tt", "high-priest"))
	}
	tb.valid(st)
	// Without the option the High Priests wait in the pool: three of them.
	tb = ttTable(t, false)
	st = tb.as(tb.start(3), "tt", "start@africa")
	if tb.n(st, "tt", "cultist", "africa") != 6 || tb.pool(st, "tt", "high-priest") != 3 {
		t.Errorf("without the option: Acolytes %d, High Priests in the pool %d", tb.n(st, "tt", "cultist", "africa"), tb.pool(st, "tt", "high-priest"))
	}
	tb.valid(st)
}

func TestSycophancyGivesTheTchoTchoDoomOrCostsTheRitual(t *testing.T) {
	tb := ttTable(t, false)
	gc, tt := tb.seat("gc"), tb.seat("tt")
	build := func() *state.BoardState {
		return tb.build(xp{
			phase: "doom", stage: "doom", turn: "gc", power: map[string]int{"gc": 9},
			units: []xu{{"gc", "cultist", "south-pacific", 1}, {"gc", "cultist", "asia", 1}},
			gates: map[string]string{"south-pacific": "gc", "asia": "gc"},
			edit:  func(d *cthulhuwars.Data) { d.Doomx = &cthulhuwars.DoomState{Order: []int{gc}} },
		})
	}
	st := tb.as(build(), "gc", "ritual")
	d := tb.data(st)
	if d.Stage != "prompt" || d.Prompts[0].Kind != "syco" || d.Prompts[0].Seat != gc {
		t.Fatalf("stage %q prompts %+v", d.Stage, d.Prompts)
	}
	if !reqDone(d, tt, rqTTRitual) {
		t.Errorf("another player's Ritual does not meet the Tcho-Tcho's first requirement")
	}
	got := tb.can(st, "gc")
	if !has(got, "syco:give/tt") || !has(got, "syco:less/tt") {
		t.Fatalf("answers %v", got)
	}
	gave := tb.as(st, "gc", "syco:give/tt")
	if d := tb.data(gave); d.Doom[tt] != 1 || d.Doom[gc] != 2 {
		t.Errorf("give: Tcho-Tcho %d Doom, Great Cthulhu %d (want 1 and 2)", d.Doom[tt], d.Doom[gc])
	}
	less := tb.as(st, "gc", "syco:less/tt")
	if d := tb.data(less); d.Doom[tt] != 0 || d.Doom[gc] != 1 {
		t.Errorf("less: Tcho-Tcho %d Doom, Great Cthulhu %d (want 0 and 1)", d.Doom[tt], d.Doom[gc])
	}
}

func TestHierophantsPlacesAHighPriestWithEverySpellbook(t *testing.T) {
	tb := ttTable(t, false)
	tt := tb.seat("tt")
	st := tb.build(xp{
		stage: "prompt", turn: "tt", units: []xu{{"tt", "cultist", "africa", 1}}, gates: map[string]string{"africa": "tt"},
		edit: func(d *cthulhuwars.Data) {
			d.Prompts = []cthulhuwars.Prompt{{Kind: "book", Seat: tt}}
			d.Resume = "turn"
			d.Turn = &cthulhuwars.TurnState{Seat: tt}
		},
	})
	st.SetReserve(tb.colour["tt"], "high-priest", 3)
	// Taking Hierophants itself brings one.
	st = tb.as(st, "tt", "book:hierophants")
	if tb.n(st, "tt", "high-priest", "africa") != 1 || tb.pool(st, "tt", "high-priest") != 2 {
		t.Errorf("High Priests at the Gate: %d (pool %d)", tb.n(st, "tt", "high-priest", "africa"), tb.pool(st, "tt", "high-priest"))
	}
	// With none left in the pool Ubbo-Sathla grows instead.
	st = tb.build(xp{
		stage: "prompt", turn: "tt", units: []xu{{"tt", "cultist", "africa", 1}}, gates: map[string]string{"africa": "tt"},
		books: map[string][]int{"tt": {ttHiero}},
		edit: func(d *cthulhuwars.Data) {
			d.Prompts = []cthulhuwars.Prompt{{Kind: "book", Seat: tt}}
			d.Resume = "turn"
			d.Turn = &cthulhuwars.TurnState{Seat: tt}
			d.Growth = 2
		},
	})
	st.SetReserve(tb.colour["tt"], "high-priest", 0)
	st = tb.as(st, "tt", "book:soulless")
	if d := tb.data(st); d.Growth != 3 {
		t.Errorf("Growth %d, want 3", d.Growth)
	}
}

func TestUbboSathlaIsAwakenedByAHighPriestAndFeedsOnTheDoomPhase(t *testing.T) {
	tb := ttTable(t, false)
	tt := tb.seat("tt")
	build := func(phase string) *state.BoardState {
		x := xp{
			units: []xu{{"tt", "cultist", "africa", 1}, {"tt", "high-priest", "asia", 1}}, gates: map[string]string{"africa": "tt"},
			power: map[string]int{"tt": 7, "gc": 5}, turn: "tt",
		}
		if phase == "doom" {
			x.phase, x.stage = "doom", "doom"
			x.edit = func(d *cthulhuwars.Data) { d.Doomx = &cthulhuwars.DoomState{Order: []int{tt}} }
		}
		return tb.build(x)
	}
	// The Action phase: 6 Power.
	st := build("action")
	st = tb.as(st, "tt", "awaken:ubbo-sathla@africa")
	d := tb.data(st)
	if tb.n(st, "tt", "ubbo-sathla", "africa") != 1 || tb.n(st, "tt", "high-priest", "asia") != 0 || d.Power[tt] != 1 {
		t.Errorf("Ubbo %d, High Priest %d, Power %d", tb.n(st, "tt", "ubbo-sathla", "africa"), tb.n(st, "tt", "high-priest", "asia"), d.Power[tt])
	}
	if !reqDone(d, tt, rqTTAwaken) {
		t.Errorf("the awakening requirement is not met")
	}
	// The Doom phase: free.
	st = build("doom")
	st.SetReserve(tb.colour["tt"], "ubbo-sathla", 1)
	got := tb.can(st, "tt")
	if !has(got, "awaken:ubbo-sathla@africa") {
		t.Fatalf("Doom-phase awakening: %v", got)
	}
	st = tb.as(st, "tt", "awaken:ubbo-sathla@africa")
	if d = tb.data(st); d.Power[tt] != 7 {
		t.Errorf("a free awakening cost %d Power", 7-d.Power[tt])
	}
	tb.valid(st)
	// His Combat is the Growth counter.
	st = tb.build(xp{
		units: []xu{{"tt", "ubbo-sathla", "africa", 1}, {"gc", "cultist", "africa", 1}}, power: map[string]int{"tt": 3, "gc": 5}, turn: "tt",
		rng:  seedDice(t, 1, 1, 1, 1, 1),
		edit: func(d *cthulhuwars.Data) { d.Growth = 4 },
	})
	st = tb.as(st, "tt", "battle@africa/gc")
	if last := tb.data(st).Last; last == nil || last.Combat[tt] != 4 {
		t.Errorf("Ubbo-Sathla rolled %+v, want 4 dice", last)
	}
}

func TestHellsBanquetGrowsUbboEveryDoomPhaseAfterHeWasAwakened(t *testing.T) {
	tb := ttTable(t, false)
	tt := tb.seat("tt")
	// The Action phase ends at once, and the Doom phase begins.
	st := tb.build(xp{
		units: []xu{{"tt", "cultist", "africa", 1}, {"gc", "cultist", "asia", 1}}, gates: map[string]string{"africa": "tt"},
		power: map[string]int{"gc": 1}, turn: "gc", rng: seedDice(t, 5),
		edit: func(d *cthulhuwars.Data) {
			d.Awoken = map[string]bool{"4|0": true}
			d.Growth = 2
		},
	})
	_ = tt
	st = tb.as(st, "gc", "pass")
	st = toDoom(t, tb, st)
	d := tb.data(st)
	if d.Phase != "doom" {
		t.Fatalf("phase %q", d.Phase)
	}
	if d.Growth <= 2 || d.Growth > 8 {
		t.Errorf("Growth %d after Hell's Banquet from 2, want 3..8", d.Growth)
	}
}

func TestSoullessCapturesGiveNoPower(t *testing.T) {
	tb := ttTable(t, false)
	gc := tb.seat("gc")
	build := func(soulless bool) *state.BoardState {
		return tb.build(xp{
			units: []xu{{"gc", "cultist", "europe", 1}}, power: map[string]int{"gc": 1, "cc": 0}, turn: "gc",
			books: map[string][]int{"tt": nil},
			edit: func(d *cthulhuwars.Data) {
				d.Captured[gc][tb.seat("tt")] = 2
				if soulless {
					d.Books[tb.seat("tt")] = 1 << ttSoulless
				}
			},
		})
	}
	gained := func(st *state.BoardState) int {
		st = tb.as(st, "gc", "pass")
		for i := 0; i < 8; i++ {
			d := tb.data(st)
			if d.Stage == "direction" || d.Phase == "doom" {
				break
			}
			seat := toMove(tb.g, st)
			legal := state.LegalMoves(tb.g, st, seat)
			res, err := state.Apply(tb.g, st, seat, legal[0])
			if err != nil {
				t.Fatal(err)
			}
			st = res.NewState
		}
		return tb.data(st).Power[gc]
	}
	plain, soulless := gained(build(false)), gained(build(true))
	if plain-soulless != 2 {
		t.Errorf("two captives gave %d Power without Soulless and %d with it, want 2 fewer", plain, soulless)
	}
}

func TestTerrorLowersTheEnemyOrRaisesTheTchoTcho(t *testing.T) {
	tb := ttTable(t, false)
	tt := tb.seat("tt")
	build := func() *state.BoardState {
		return tb.build(xp{
			units: []xu{{"tt", "proto-shoggoth", "africa", 2}, {"gc", "shoggoth", "africa", 1}},
			power: map[string]int{"tt": 3, "gc": 5}, turn: "tt", books: map[string][]int{"tt": {ttTerror}},
			rng: seedDice(t, 1, 1, 1, 1, 1, 1),
		})
	}
	st := tb.as(build(), "tt", "battle@africa/gc")
	got := tb.can(st, "tt")
	if !has(got, "terror:foe") || !has(got, "terror:self") {
		t.Fatalf("pre-battle moves %v", got)
	}
	self := tb.as(st, "tt", "terror:self")
	if last := tb.data(self).Last; last == nil || last.Combat[tt] != 4 {
		t.Errorf("Terror for oneself: Combat %+v, want 4 (2 Protos + 2)", last)
	}
	foe := tb.as(st, "tt", "terror:foe")
	if last := tb.data(foe).Last; last == nil || last.Combat[tb.seat("gc")] != 0 {
		t.Errorf("Terror against the enemy: Combat %+v, want a Shoggoth's 2 − 2 = 0", last)
	}
}

func TestMartyrdomTurnsKillsIntoPainsWhenTheHighPriestFalls(t *testing.T) {
	tb := ttTable(t, false)
	tt := tb.seat("tt")
	// Great Cthulhu's four Deep Ones roll 6 6 1 1: two Kills against the Tcho-Tcho.
	st := tb.build(xp{
		units: []xu{{"gc", "deep-one", "africa", 4}, {"tt", "high-priest", "africa", 1}, {"tt", "proto-shoggoth", "africa", 1}},
		power: map[string]int{"gc": 3, "tt": 0, "cc": 5}, turn: "gc", books: map[string][]int{"tt": {ttMartyr}},
		rng: seedDice(t, 6, 6, 1, 1, 1),
	})
	st = tb.as(st, "gc", "battle@africa/tt")
	d := tb.data(st)
	if d.Stage != "assign" || d.Bat.Queue[0].Seat != tt {
		t.Fatalf("stage %q: the Tcho-Tcho assigns", d.Stage)
	}
	st = tb.as(st, "tt", "assign:high-priest")
	if tb.n(st, "tt", "high-priest", "africa") != 0 {
		t.Errorf("the High Priest should be dead")
	}
	// The second Kill became a Pain: the Proto-Shoggoth lives and retreats.
	for i := 0; i < 6 && tb.data(st).Stage != "turn" && tb.data(st).Phase == "action"; i++ {
		seat := toMove(tb.g, st)
		legal := state.LegalMoves(tb.g, st, seat)
		if len(legal) == 0 {
			break
		}
		res, err := state.Apply(tb.g, st, seat, legal[0])
		if err != nil {
			t.Fatal(err)
		}
		st = res.NewState
		if tb.data(st).Stage == "turn" {
			break
		}
	}
	protos := 0
	for _, a := range []string{"africa", "europe", "asia", "indian-ocean", "north-atlantic", "south-atlantic"} {
		protos += tb.n(st, "tt", "proto-shoggoth", a)
	}
	if protos != 1 {
		t.Errorf("%d Proto-Shoggoths left, want 1 (Pained, not killed)", protos)
	}
}

func TestTabletsOfTheGodsAddsElderSignsAndEliminatesTheHighPriests(t *testing.T) {
	tb := ttTable(t, false)
	tt := tb.seat("tt")
	st := tb.build(xp{
		phase: "doom", stage: "doom", turn: "tt", power: map[string]int{"tt": 9},
		units: []xu{{"tt", "cultist", "africa", 1}, {"tt", "high-priest", "africa", 1}, {"tt", "cultist", "europe", 1}, {"tt", "high-priest", "asia", 1}},
		gates: map[string]string{"africa": "tt", "europe": "tt"}, books: map[string][]int{"tt": {ttTablets}},
		edit: func(d *cthulhuwars.Data) { d.Doomx = &cthulhuwars.DoomState{Order: []int{tt}} },
	})
	st = tb.as(st, "tt", "ritual")
	d := tb.data(st)
	// No Great Old One: no Elder Signs of the Ritual itself; Tablets: 1 per Gate with a High Priest.
	if len(d.Signs[tt]) != 1 {
		t.Errorf("%d Elder Signs, want 1 (one Gate has a High Priest)", len(d.Signs[tt]))
	}
	if tb.n(st, "tt", "high-priest", "africa") != 0 || tb.n(st, "tt", "high-priest", "asia") != 0 {
		t.Errorf("the High Priests were not eliminated")
	}
	tb.valid(st)
}

func TestIdolatryCallsUnitsToAnotherCultsGlyph(t *testing.T) {
	tb := ttTable(t, false)
	tt := tb.seat("tt")
	st := tb.build(xp{
		units: []xu{{"tt", "proto-shoggoth", "asia", 2}, {"tt", "cultist", "africa", 1}, {"tt", "cultist", "europe", 1}},
		gates: map[string]string{"europe": "tt"}, power: map[string]int{"tt": 4, "gc": 5}, turn: "tt",
		books: map[string][]int{"tt": {ttIdolatry}},
		edit:  func(d *cthulhuwars.Data) { d.TTStart = "europe" },
	})
	got := tb.can(st, "tt")
	// Asia is a start area (Crawling Chaos'); Europe is the Tcho-Tcho's own here.
	if !has(got, "idolatry@asia") {
		t.Fatalf("Idolatry: %v", got)
	}
	st = tb.as(st, "tt", "idolatry@asia")
	d := tb.data(st)
	if d.Stage != "prompt" || d.Prompts[0].Kind != "idol" {
		t.Fatalf("stage %q prompts %+v", d.Stage, d.Prompts)
	}
	got = tb.can(st, "tt")
	if !has(got, "bring:cultist@africa") || !has(got, "bring:cultist@europe") || !has(got, "bring:all") || !has(got, "done") {
		t.Fatalf("stacks to call: %v", got)
	}
	st = tb.as(st, "tt", "bring:all")
	if tb.n(st, "tt", "cultist", "asia") != 2 || d.Power[tt] != 3 {
		t.Errorf("cultists in Asia %d, power %d", tb.n(st, "tt", "cultist", "asia"), tb.data(st).Power[tt])
	}
	tb.valid(st)
}

func TestTchoTchoRemovesItsGateForARequirement(t *testing.T) {
	tb := ttTable(t, false)
	tt := tb.seat("tt")
	st := tb.build(xp{
		units: []xu{{"tt", "cultist", "africa", 1}}, gates: map[string]string{"africa": "tt"}, power: map[string]int{"tt": 4, "gc": 5}, turn: "tt",
		edit: func(d *cthulhuwars.Data) { d.TTStart = "africa" },
	})
	got := tb.can(st, "tt")
	if !has(got, "dismantle@africa") {
		t.Fatalf("moves %v", got)
	}
	st = tb.as(st, "tt", "dismantle@africa")
	d := tb.data(st)
	if _, has := d.Gates["africa"]; has || !reqDone(d, tt, rqTTGate) {
		t.Errorf("gates %v, reqs %b", d.Gates, d.Reqs[tt])
	}
	if d.Stage != "prompt" || d.Prompts[0].Kind != "book" {
		t.Errorf("the book of the requirement: stage %q prompts %+v", d.Stage, d.Prompts)
	}
}

func TestTchoTchoRequirements(t *testing.T) {
	tb := ttTable(t, false)
	tt := tb.seat("tt")
	// Another player with 15 Doom, and three Elder Signs.
	st := tb.build(xp{
		units: []xu{{"tt", "cultist", "africa", 1}}, power: map[string]int{"tt": 3, "gc": 5}, turn: "tt",
		doom: map[string]int{"gc": 15}, signs: map[string][]int{"tt": {1, 2, 3}},
	})
	st = tb.as(st, "tt", "recruit@africa")
	d := tb.data(st)
	if !reqDone(d, tt, rqTTRitual) || !reqDone(d, tt, rqTTThree) {
		t.Errorf("reqs %b: 15 Doom elsewhere and 3 Elder Signs", d.Reqs[tt])
	}
	if !strings.Contains(strings.Join(d.Events, "|"), "spellbook requirement") {
		t.Errorf("events %v", d.Events)
	}
}

func reqDone(d *cthulhuwars.Data, seat, req int) bool { return d.Reqs[seat]&(1<<uint(req)) != 0 }

// toDoom plays the choices after the last Action (Gather Power, the first
// player, the direction) until the Doom phase begins.
func toDoom(t testing.TB, tb *table, st *state.BoardState) *state.BoardState {
	t.Helper()
	for i := 0; i < 12; i++ {
		d := tb.data(st)
		if d.Phase == "doom" {
			return st
		}
		seat := toMove(tb.g, st)
		legal := state.LegalMoves(tb.g, st, seat)
		if len(legal) == 0 {
			t.Fatalf("no legal move at %q\n%s", d.Stage, state.RenderText(tb.g, st))
		}
		mv := legal[0]
		for _, m := range legal {
			if state.FormatMove(tb.g, m) == "cw" {
				mv = m
			}
		}
		res, err := state.Apply(tb.g, st, seat, mv)
		if err != nil {
			t.Fatal(err)
		}
		st = res.NewState
	}
	t.Fatalf("the Doom phase did not begin")
	return st
}
