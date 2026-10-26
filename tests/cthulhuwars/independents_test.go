package cthulhuwars_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The five Independent Great Old Ones of Great Old One Pack 1.

var allIndependents = map[string]any{
	"igoo-abhoth": true, "igoo-chaugnar": true, "igoo-cthugha": true, "igoo-hydra": true, "igoo-yig": true,
}

// indep is a table with the Independents in the box.
func indep(t testing.TB) *table { return cardTable(t, fourPlus, allIndependents) }

// holdsBook gives the cult the spellbook that lies on its Independent's card.
func holdsBook(d *cthulhuwars.Data, book int) { d.IBooks |= 1 << uint(book) }

func TestAnIndependentIsAwakenedAtAGateWithAGreatOldOneOfOneOwn(t *testing.T) {
	tb := indep(t)
	st := tb.build(xp{
		units: []xu{
			{who: "gc", typ: "cthulhu", area: "south-pacific"}, {who: "gc", typ: "cultist", area: "south-pacific"},
			{who: "gc", typ: "cultist", area: "europe"}, {who: "cc", typ: "cultist", area: "asia"}, {who: "cc", typ: "nyarlathotep", area: "asia"},
		},
		gates: map[string]string{"south-pacific": "gc", "europe": "gc", "asia": "cc"},
		power: map[string]int{"gc": 10, "cc": 10},
		pool:  map[string]int{"gc|abhoth": 0},
	})
	got := tb.can(st, "gc")
	for _, want := range []string{"awaken:abhoth@south-pacific", "awaken:chaugnar-faugn@south-pacific", "awaken:mother-hydra@south-pacific", "awaken:yig@south-pacific", "awaken:cthugha@south-pacific/cthulhu"} {
		if !has(got, want) {
			t.Errorf("%s is not offered: %v", want, got)
		}
	}
	// A Gate with no Great Old One beside it is no place for one.
	if has(got, "awaken:abhoth@europe") {
		t.Errorf("Abhoth awakens only where the cult has a Great Old One: %v", got)
	}
	st = tb.as(st, "gc", "awaken:abhoth@south-pacific")
	d := tb.data(st)
	if d.Power[1] != 6 {
		t.Errorf("an Independent costs 4 Power: %d", d.Power[1])
	}
	if d.Loyal[1]&(1<<cdAbhoth) == 0 || tb.n(st, "gc", "abhoth", "south-pacific") != 1 {
		t.Error("Abhoth is on the map and his card is the cult's")
	}
	if tb.pool(st, "gc", "filth") != 12 {
		t.Errorf("the twelve Filth Tokens come with him: %d", tb.pool(st, "gc", "filth"))
	}
	// Immortal: any Great Old One awakened gives Great Cthulhu an Elder Sign.
	if len(d.Signs[1]) != 1 {
		t.Errorf("Immortal gives an Elder Sign for an Independent: %v", d.Signs[1])
	}
	// The same card cannot be awakened again, by anyone.
	if got := tb.can(st, "gc"); hasPrefix(got, "awaken:abhoth") {
		t.Errorf("Abhoth is awake: %v", got)
	}
	if got := tb.can(st, "cc"); hasPrefix(got, "awaken:abhoth") {
		t.Errorf("Abhoth is controlled by another cult: %v", got)
	}
	tb.valid(st)
}

func TestIndependentsGiveNoElderSignsInARitual(t *testing.T) {
	tb := indep(t)
	st := doomPhase(tb, "gc", xp{
		units: []xu{{who: "gc", typ: "cthulhu", area: "south-pacific"}, {who: "gc", typ: "yig", area: "south-pacific"}},
		gates: map[string]string{"south-pacific": "gc"},
		power: map[string]int{"gc": 10},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdYig) },
		pool:  map[string]int{"gc|yig": 0},
	})
	st = tb.as(st, "gc", "ritual")
	if n := len(tb.data(st).Signs[1]); n != 1 {
		t.Errorf("one Elder Sign for Cthulhu, none for Yig: %d", n)
	}
}

func TestAbhothsFilthIsPlacedAnywhereButNeverMoves(t *testing.T) {
	tb := indep(t)
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "abhoth", area: "south-pacific"}, {who: "gc", typ: "cultist", area: "south-pacific"}, {who: "gc", typ: "filth", area: "asia", n: 2}},
		gates: map[string]string{"south-pacific": "gc"},
		power: map[string]int{"gc": 6},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdAbhoth) },
		pool:  map[string]int{"gc|abhoth": 0, "gc|filth": 10},
	})
	got := tb.can(st, "gc")
	if !has(got, "filth@europe") || !has(got, "filth@australia") {
		t.Fatalf("Filth goes in any area: %v", got)
	}
	if hasPrefix(got, "go:filth") || hasPrefix(got, "summon:filth") {
		t.Fatalf("Filth is neither moved nor summoned: %v", got)
	}
	st = tb.as(st, "gc", "filth@europe")
	if tb.power(st, "gc") != 5 || tb.n(st, "gc", "filth", "europe") != 1 || tb.pool(st, "gc", "filth") != 9 {
		t.Errorf("a Filth Token for 1 Power: Power %d", tb.power(st, "gc"))
	}
	tb.valid(st)
}

func TestAbhothsCombatIsTheNumberOfFilthTokensInPlay(t *testing.T) {
	tb := indep(t)
	st := tb.build(xp{
		units: []xu{
			{who: "gc", typ: "abhoth", area: "asia"}, {who: "gc", typ: "filth", area: "europe", n: 5}, {who: "cc", typ: "cultist", area: "asia"},
		},
		power: map[string]int{"gc": 6},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdAbhoth) },
		pool:  map[string]int{"gc|abhoth": 0, "gc|filth": 7},
		rng:   seedDice(t, 1, 1, 1, 1, 1),
	})
	st = tb.as(st, "gc", "battle@asia/cc")
	if d := tb.data(st); d.Last == nil || d.Last.Combat[1] != 5 {
		t.Fatalf("Abhoth rolls one die for each of the five Filth Tokens: %+v", d.Last)
	}
}

func TestTheBroodSpoilsGatesWithFilthButNotAbhothsOwn(t *testing.T) {
	tb := indep(t)
	st := tb.build(xp{
		units: []xu{
			{who: "gc", typ: "abhoth", area: "south-pacific"}, {who: "gc", typ: "cultist", area: "south-pacific"},
			{who: "gc", typ: "filth", area: "south-pacific"}, {who: "gc", typ: "filth", area: "asia"},
			{who: "cc", typ: "cultist", area: "asia"}, {who: "cc", typ: "cultist", area: "europe"},
		},
		gates: map[string]string{"south-pacific": "gc", "asia": "cc", "europe": "cc"},
		stage: "direction", phase: "action",
		edit: func(d *cthulhuwars.Data) {
			holds(d, tb, "gc", cdAbhoth)
			holdsBook(d, ibBrood)
		},
		pool: map[string]int{"gc|abhoth": 0, "gc|filth": 9},
	})
	st = tb.as(st, "gc", "cw")
	d := tb.data(st)
	if d.Doom[1] != 1 {
		t.Errorf("Abhoth's own Gate counts although Filth lies there: Doom %d", d.Doom[1])
	}
	if d.Doom[2] != 1 {
		t.Errorf("the Gate in Asia does not count (Filth) but the one in Europe does: Doom %d", d.Doom[2])
	}
}

func TestAbhothsSpellbookIsEarnedWithFourKindsOfMonsterOrEight(t *testing.T) {
	tb := indep(t)
	three := xp{
		units: []xu{
			{who: "gc", typ: "abhoth", area: "south-pacific"}, {who: "gc", typ: "cultist", area: "south-pacific"},
			{who: "gc", typ: "deep-one", area: "south-pacific"}, {who: "gc", typ: "shoggoth", area: "south-pacific"}, {who: "gc", typ: "filth", area: "asia"},
		},
		gates: map[string]string{"south-pacific": "gc"},
		power: map[string]int{"gc": 6},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdAbhoth) },
		pool:  map[string]int{"gc|abhoth": 0, "gc|filth": 11},
	}
	// Deep One, Shoggoth, Filth: three kinds. A Star Spawn is the fourth.
	st := tb.build(three)
	st = tb.as(st, "gc", "recruit@south-pacific")
	if tb.data(st).IBooks&(1<<ibBrood) != 0 {
		t.Fatal("three kinds of Monster do not earn The Brood")
	}
	four := three
	four.units = append(append([]xu(nil), three.units...), xu{who: "gc", typ: "starspawn", area: "south-pacific"})
	st = tb.build(four)
	st = tb.as(st, "gc", "recruit@south-pacific")
	if tb.data(st).IBooks&(1<<ibBrood) == 0 {
		t.Fatal("four kinds of Monster earn The Brood")
	}
	// Eight Monsters of one or two kinds do, too — the Filth counts.
	eight := three
	eight.units = []xu{
		{who: "gc", typ: "abhoth", area: "south-pacific"}, {who: "gc", typ: "cultist", area: "south-pacific"},
		{who: "gc", typ: "deep-one", area: "south-pacific", n: 3}, {who: "gc", typ: "filth", area: "asia", n: 5},
	}
	eight.pool = map[string]int{"gc|abhoth": 0, "gc|filth": 7}
	st = tb.build(eight)
	st = tb.as(st, "gc", "recruit@south-pacific")
	if tb.data(st).IBooks&(1<<ibBrood) == 0 {
		t.Fatal("eight Monsters earn The Brood")
	}
}

func TestAnIndependentThatDiesGoesBackToThePoolWithItsSpellbook(t *testing.T) {
	tb := indep(t)
	base := xp{
		units: []xu{{who: "gc", typ: "abhoth", area: "asia"}, {who: "gc", typ: "filth", area: "europe", n: 3}, {who: "cc", typ: "hunting-horror", area: "asia", n: 2}},
		power: map[string]int{"cc": 6, "gc": 6},
		turn:  "cc",
		edit: func(d *cthulhuwars.Data) {
			holds(d, tb, "gc", cdAbhoth)
			holdsBook(d, ibBrood)
		},
		pool: map[string]int{"gc|abhoth": 0, "gc|filth": 8},
		rng:  seedDice(t, 6, 6, 6, 6),
	}
	st := tb.build(base)
	st = tb.as(st, "cc", "battle@asia/gc")
	d := tb.data(st)
	if d.Loyal[1]&(1<<cdAbhoth) != 0 || d.IBooks&(1<<ibBrood) != 0 {
		t.Fatalf("Abhoth's card and his spellbook are gone: loyal %b books %b", d.Loyal[1], d.IBooks)
	}
	if tb.n(st, "gc", "filth", "europe") != 3 {
		t.Error("the tokens already on the map remain there")
	}
	if tb.pool(st, "gc", "filth") != 0 {
		t.Errorf("the unused tokens go back to the general pool: %d", tb.pool(st, "gc", "filth"))
	}
	tb.valid(st)
	// Someone else may awaken him, with the Filth that is not on the map.
	st = tb.build(xp{
		units: []xu{
			{who: "bg", typ: "shub-niggurath", area: "south-america-west"}, {who: "bg", typ: "cultist", area: "south-america-west"},
			{who: "gc", typ: "filth", area: "europe", n: 3},
		},
		gates: map[string]string{"south-america-west": "bg"},
		power: map[string]int{"bg": 8}, turn: "bg",
		pool: map[string]int{"gc|abhoth": 0, "gc|filth": 0},
	})
	st = tb.as(st, "bg", "awaken:abhoth@south-america-west")
	if tb.pool(st, "bg", "filth") != 9 {
		t.Errorf("twelve less the three on the map: %d", tb.pool(st, "bg", "filth"))
	}
	tb.valid(st)
}

func TestChaugnarFaugnAddsThreeDiceInBattlesAtGatesOfHisOwner(t *testing.T) {
	tb := indep(t)
	build := func(gates map[string]string) *state.BoardState {
		return tb.build(xp{
			units: []xu{{who: "gc", typ: "chaugnar-faugn", area: "south-pacific"}, {who: "gc", typ: "deep-one", area: "asia", n: 2}, {who: "cc", typ: "cultist", area: "asia"}},
			gates: gates,
			power: map[string]int{"gc": 6},
			edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdChaugnar) },
			pool:  map[string]int{"gc|chaugnar-faugn": 0},
			rng:   seedDice(t, 1, 1, 1, 1, 1),
		})
	}
	st := tb.as(build(map[string]string{"asia": "gc", "south-pacific": "gc"}), "gc", "battle@asia/cc")
	if d := tb.data(st); d.Last.Combat[1] != 5 {
		t.Errorf("two Deep Ones and Miri Nigri at the Gate: %d dice", d.Last.Combat[1])
	}
	st = tb.as(build(map[string]string{"south-pacific": "gc"}), "gc", "battle@asia/cc")
	if d := tb.data(st); d.Last.Combat[1] != 2 {
		t.Errorf("no Gate there, no Miri Nigri: %d dice", d.Last.Combat[1])
	}
}

func TestChaugnarsRequirementDiscardsTwoElderSignsAndTheCurseChoosesTheValues(t *testing.T) {
	tb := indep(t)
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "chaugnar-faugn", area: "south-pacific"}, {who: "gc", typ: "cultist", area: "south-pacific"}},
		gates: map[string]string{"south-pacific": "gc"},
		power: map[string]int{"gc": 6},
		signs: map[string][]int{"gc": {3, 1, 2}},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdChaugnar) },
		pool:  map[string]int{"gc|chaugnar-faugn": 0},
	})
	got := tb.can(st, "gc")
	for _, want := range []string{"discard:12", "discard:13", "discard:23"} {
		if !has(got, want) {
			t.Errorf("%s is not offered: %v", want, got)
		}
	}
	if has(got, "discard:11") || has(got, "discard:22") {
		t.Errorf("pairs the cult cannot make: %v", got)
	}
	st = tb.as(st, "gc", "discard:12")
	d := tb.data(st)
	if len(d.Signs[1]) != 1 || d.Signs[1][0] != 3 {
		t.Errorf("a 1 and a 2 were discarded: %v", d.Signs[1])
	}
	if d.IBooks&(1<<ibCurse) == 0 {
		t.Fatal("the Curse is earned")
	}
	if d.Doom[1] != 0 {
		t.Errorf("no Doom for discarded signs: %d", d.Doom[1])
	}
	// Anyone who now receives an Elder Sign gets the value Chaugnar's holder
	// picks: here Great Cthulhu gives Crawling Chaos a 1.
	var st2 *state.BoardState
	st2 = doomPhase(tb, "cc", xp{
		units: []xu{{who: "gc", typ: "chaugnar-faugn", area: "south-pacific"}, {who: "cc", typ: "nyarlathotep", area: "asia"}, {who: "cc", typ: "cultist", area: "asia"}},
		gates: map[string]string{"south-pacific": "gc", "asia": "cc"},
		power: map[string]int{"cc": 10},
		edit: func(d *cthulhuwars.Data) {
			holds(d, tb, "gc", cdChaugnar)
			holdsBook(d, ibCurse)
		},
		pool: map[string]int{"gc|chaugnar-faugn": 0},
	})
	st2 = tb.as(st2, "cc", "ritual")
	d = tb.data(st2)
	if d.Stage != "prompt" || d.Prompts[0].Kind != "pick" || d.Prompts[0].Seat != 1 || d.Prompts[0].By != 2 {
		t.Fatalf("the holder of the Curse chooses: stage %q %+v", d.Stage, d.Prompts)
	}
	if got := tb.can(st2, "gc"); !has(got, "pick:1") || !has(got, "pick:2") || !has(got, "pick:3") {
		t.Fatalf("the values in the pool: %v", got)
	}
	st2 = tb.as(st2, "gc", "pick:1")
	d = tb.data(st2)
	if len(d.Signs[2]) != 1 || d.Signs[2][0] != 1 {
		t.Errorf("Crawling Chaos receives the 1 that was picked: %v", d.Signs[2])
	}
	if d.Stage == "prompt" {
		t.Errorf("nothing else to decide: %+v", d.Prompts)
	}
}

func TestCthughaReplacesAGreatOldOneAndCostsSixLessItsCost(t *testing.T) {
	tb := indep(t)
	build := func(awoke bool, goo string) *state.BoardState {
		return tb.build(xp{
			units: []xu{{who: "gc", typ: goo, area: "south-pacific"}, {who: "gc", typ: "cultist", area: "south-pacific"}},
			gates: map[string]string{"south-pacific": "gc"},
			power: map[string]int{"gc": 10},
			edit: func(d *cthulhuwars.Data) {
				if awoke {
					d.Awoken = map[string]bool{"1|0": true}
				}
				if goo == "abhoth" {
					holds(d, tb, "gc", cdAbhoth)
				}
			},
			pool: map[string]int{"gc|cthulhu": map[bool]int{true: 0, false: 1}[goo == "cthulhu"], "gc|abhoth": 0, "gc|cthugha": 0},
		})
	}
	// Cthulhu, who has awakened before, costs 4: Cthugha costs 2.
	st := build(true, "cthulhu")
	if got := tb.can(st, "gc"); !has(got, "awaken:cthugha@south-pacific/cthulhu") {
		t.Fatalf("Cthugha may replace Cthulhu: %v", got)
	}
	st = tb.as(st, "gc", "awaken:cthugha@south-pacific/cthulhu")
	if tb.power(st, "gc") != 8 {
		t.Errorf("6 minus 4 is 2 Power: %d left", tb.power(st, "gc"))
	}
	if tb.n(st, "gc", "cthugha", "south-pacific") != 1 || tb.n(st, "gc", "cthulhu", "south-pacific") != 0 || tb.pool(st, "gc", "cthulhu") != 1 {
		t.Error("Cthulhu goes back to the pool and Cthugha takes his place")
	}
	tb.valid(st)
	// The first time Cthulhu costs 10: replacing him pays 4 Power.
	st = tb.as(build(false, "cthulhu"), "gc", "awaken:cthugha@south-pacific/cthulhu")
	if tb.power(st, "gc") != 14 {
		t.Errorf("a negative cost is gained: %d Power", tb.power(st, "gc"))
	}
	// Replacing another Independent loses its card.
	st = tb.as(build(false, "abhoth"), "gc", "awaken:cthugha@south-pacific/abhoth")
	d := tb.data(st)
	if tb.power(st, "gc") != 8 || d.Loyal[1]&(1<<cdAbhoth) != 0 || d.Loyal[1]&(1<<cdCthugha) == 0 {
		t.Errorf("Abhoth (4) is replaced for 2 Power and his card goes back: power %d loyal %b", tb.power(st, "gc"), d.Loyal[1])
	}
	tb.valid(st)
}

// cthughaBattle is Cthugha's seat against Crawling Chaos' lone Nyarlathotep
// in Asia, the dice giving Great Cthulhu one Kill.
func cthughaBattle(t *testing.T, tb *table, ccPower int, firestorm bool) *state.BoardState {
	t.Helper()
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "cthugha", area: "asia"}, {who: "gc", typ: "deep-one", area: "asia", n: 2}, {who: "cc", typ: "nyarlathotep", area: "asia"}},
		power: map[string]int{"gc": 6, "cc": ccPower},
		edit: func(d *cthulhuwars.Data) {
			holds(d, tb, "gc", cdCthugha)
			if firestorm {
				holdsBook(d, ibFirestorm)
			}
		},
		pool: map[string]int{"gc|cthugha": 0},
		rng:  seedDice(t, 6, 1),
	})
	return tb.as(st, "gc", "battle@asia/cc")
}

func TestFireVampiresSpareAKilledUnitForPower(t *testing.T) {
	tb := indep(t)
	st := cthughaBattle(t, tb, 5, false)
	d := tb.data(st)
	if d.Stage != "prompt" || d.Prompts[0].Kind != "spare" || d.Prompts[0].Seat != 1 {
		t.Fatalf("Cthugha's owner may spare: stage %q %+v", d.Stage, d.Prompts)
	}
	if got := tb.can(st, "gc"); !has(got, "spare:nyarlathotep/cc") || !has(got, "done") {
		t.Fatalf("moves %v", got)
	}
	st = tb.as(st, "gc", "spare:nyarlathotep/cc")
	d = tb.data(st)
	if d.Power[1] != 6 {
		t.Errorf("1 Power for the spared unit (6 - 1 + 1): %d", d.Power[1])
	}
	if tb.pool(st, "cc", "nyarlathotep") != 0 {
		t.Error("the spared unit lives")
	}
	if d.IBooks&(1<<ibFirestorm) != 0 {
		t.Error("a Great Old One that was spared was not killed: no spellbook")
	}
	// Pained, it retreats.
	if d.Stage != "retreat" && tb.n(st, "cc", "nyarlathotep", "asia") != 0 {
		t.Errorf("Nyarlathotep takes a Pain and must retreat: stage %q", d.Stage)
	}
	tb.valid(st)
}

func TestWithoutSparingTheKillStandsAndCthughaEarnsHisSpellbook(t *testing.T) {
	tb := indep(t)
	st := cthughaBattle(t, tb, 5, false)
	st = tb.as(st, "gc", "done")
	d := tb.data(st)
	if tb.pool(st, "cc", "nyarlathotep") != 1 || tb.n(st, "cc", "nyarlathotep", "asia") != 0 {
		t.Error("Nyarlathotep is killed")
	}
	if d.IBooks&(1<<ibFirestorm) == 0 {
		t.Error("killing an enemy Great Old One earns Firestorm")
	}
	tb.valid(st)
}

func TestFireVampiresDoNotWorkAgainstAnEnemyWithoutPower(t *testing.T) {
	tb := indep(t)
	st := cthughaBattle(t, tb, 0, false)
	d := tb.data(st)
	if d.Stage == "prompt" && len(d.Prompts) > 0 && d.Prompts[0].Kind == "spare" {
		t.Fatal("an enemy with no Power cannot be spared")
	}
	if tb.pool(st, "cc", "nyarlathotep") != 1 {
		t.Error("Nyarlathotep is killed")
	}
}

func TestFirestormGivesAnElderSignForEachUnitSpared(t *testing.T) {
	tb := indep(t)
	st := cthughaBattle(t, tb, 5, true)
	st = tb.as(st, "gc", "spare:nyarlathotep/cc")
	if n := len(tb.data(st).Signs[1]); n != 2 {
		// One for Cthugha's spare, none other: Immortal only on awakening.
		if n != 1 {
			t.Errorf("an Elder Sign for the spared unit: %d", n)
		}
	}
}

func TestCthughaRollsTheCombatOfAnEnemyGreatOldOne(t *testing.T) {
	tb := indep(t)
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "cthugha", area: "asia"}, {who: "cc", typ: "nyarlathotep", area: "asia"}, {who: "cc", typ: "cultist", area: "asia"}},
		power: map[string]int{"gc": 6, "cc": 5},
		books: map[string][]int{"cc": {0, 1, 2}, "gc": {0}},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdCthugha) },
		pool:  map[string]int{"gc|cthugha": 0},
		rng:   seedDice(t, 1, 1, 1, 1, 1),
	})
	st = tb.as(st, "gc", "battle@asia/cc")
	// Nyarlathotep rolls the spellbooks on both faction cards: 3 + 1.
	if d := tb.data(st); d.Last.Combat[1] != 4 {
		t.Errorf("Cthugha rolls what Nyarlathotep does: %d", d.Last.Combat[1])
	}
	// Alone against Cultists he rolls nothing: no battle is offered.
	st = tb.build(xp{
		units: []xu{{who: "gc", typ: "cthugha", area: "asia"}, {who: "cc", typ: "cultist", area: "asia"}},
		power: map[string]int{"gc": 6},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdCthugha) },
		pool:  map[string]int{"gc|cthugha": 0},
	})
	if got := tb.can(st, "gc"); has(got, "battle@asia/cc") {
		t.Errorf("Cthugha has no Combat without an enemy Great Old One: %v", got)
	}
}

func TestMotherHydraRollsSixLessTheEnemyUnitsAtLeastOne(t *testing.T) {
	tb := indep(t)
	for _, c := range []struct{ enemies, want int }{{2, 4}, {5, 1}, {8, 1}} {
		st := tb.build(xp{
			units: []xu{{who: "gc", typ: "mother-hydra", area: "asia"}, {who: "cc", typ: "cultist", area: "asia", n: c.enemies}},
			power: map[string]int{"gc": 6},
			edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdHydra) },
			pool:  map[string]int{"gc|mother-hydra": 0},
			rng:   seedDice(t, 1, 1, 1, 1, 1),
		})
		st = tb.as(st, "gc", "battle@asia/cc")
		if d := tb.data(st); d.Last.Combat[1] != c.want {
			t.Errorf("%d enemy units: %d dice, want %d", c.enemies, d.Last.Combat[1], c.want)
		}
	}
}

func TestTheAgonySting(t *testing.T) {
	tb := indep(t)
	st := tb.build(xp{
		units: []xu{
			{who: "gc", typ: "mother-hydra", area: "asia"}, {who: "gc", typ: "cultist", area: "north-atlantic"},
			{who: "cc", typ: "cultist", area: "north-atlantic", n: 2}, {who: "bg", typ: "cultist", area: "north-atlantic"},
		},
		power: map[string]int{"gc": 6},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdHydra) },
		pool:  map[string]int{"gc|mother-hydra": 0},
	})
	got := tb.can(st, "gc")
	if !has(got, "sting@north-atlantic") {
		t.Fatalf("an ocean with enemy Cultists: %v", got)
	}
	if has(got, "sting@south-pacific") {
		t.Errorf("an ocean with none: %v", got)
	}
	st = tb.as(st, "gc", "sting@north-atlantic")
	d := tb.data(st)
	if d.Power[1] != 5 {
		t.Errorf("1 Power: %d", d.Power[1])
	}
	if d.Stage != "prompt" || d.Prompts[0].Kind != "sting" || d.Prompts[0].Seat != 2 {
		t.Fatalf("the owners move their Cultists: stage %q %+v", d.Stage, d.Prompts)
	}
	got = tb.can(st, "cc")
	if !has(got, "sting:cultist>europe") || !has(got, "sting:cultist>africa") || has(got, "sting:cultist>arctic-ocean") {
		t.Fatalf("only to adjacent land areas: %v", got)
	}
	st = tb.as(st, "cc", "sting:cultist>europe")
	st = tb.as(st, "cc", "sting:cultist>africa")
	d = tb.data(st)
	if d.Stage != "prompt" || d.Prompts[0].Seat != 3 {
		t.Fatalf("next the Black Goat: stage %q %+v", d.Stage, d.Prompts)
	}
	st = tb.as(st, "bg", "sting:cultist>africa")
	if tb.n(st, "cc", "cultist", "north-atlantic")+tb.n(st, "bg", "cultist", "north-atlantic") != 0 {
		t.Error("the enemy Cultists are gone from the ocean")
	}
	if tb.n(st, "gc", "cultist", "north-atlantic") != 1 {
		t.Error("the stinger's own Cultists are immune")
	}
	tb.valid(st)
}

func TestTheZygotePlacesEveryAcolyteOfThePoolWhereTheCultHasUnits(t *testing.T) {
	tb := indep(t)
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "mother-hydra", area: "asia"}, {who: "gc", typ: "cultist", area: "europe"}},
		power: map[string]int{"gc": 6},
		edit: func(d *cthulhuwars.Data) {
			holds(d, tb, "gc", cdHydra)
			holdsBook(d, ibZygote)
		},
		pool: map[string]int{"gc|mother-hydra": 0},
	})
	if got := tb.can(st, "gc"); !has(got, "zygote") {
		t.Fatalf("The Zygote is an Action: %v", got)
	}
	st = tb.as(st, "gc", "zygote")
	if tb.power(st, "gc") != 5 {
		t.Errorf("1 Power: %d", tb.power(st, "gc"))
	}
	got := tb.can(st, "gc")
	if !has(got, "zygote@asia") || !has(got, "zygote@europe") || has(got, "zygote@africa") {
		t.Fatalf("only where the cult has a unit: %v", got)
	}
	for _, a := range []string{"asia", "asia", "europe", "europe", "europe"} {
		st = tb.as(st, "gc", "zygote@"+a)
	}
	if tb.pool(st, "gc", "cultist") != 0 {
		t.Errorf("all the Acolytes were placed: %d left", tb.pool(st, "gc", "cultist"))
	}
	if tb.n(st, "gc", "cultist", "asia") != 2 || tb.n(st, "gc", "cultist", "europe") != 4 {
		t.Error("two in Asia, four in Europe (one was there)")
	}
	tb.valid(st)
}

func TestMotherHydrasSpellbookIsEarnedWhenOceansHoldNoGreatOldOnes(t *testing.T) {
	tb := indep(t)
	hydra := func(units ...xu) xp {
		return xp{
			units: append([]xu{{who: "gc", typ: "mother-hydra", area: "asia"}, {who: "gc", typ: "cultist", area: "asia"}}, units...),
			power: map[string]int{"gc": 6},
			edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdHydra) },
			pool:  map[string]int{"gc|mother-hydra": 0},
		}
	}
	// Neither side has a Great Old One in an ocean.
	st := tb.as(tb.build(hydra()), "gc", "recruit@asia")
	if tb.data(st).IBooks&(1<<ibZygote) == 0 {
		t.Error("no Great Old One in an ocean: The Zygote")
	}
	// Both do: not yet.
	st = tb.as(tb.build(hydra(xu{who: "gc", typ: "cthulhu", area: "south-pacific"}, xu{who: "cc", typ: "nyarlathotep", area: "north-atlantic"})), "gc", "recruit@asia")
	if tb.data(st).IBooks&(1<<ibZygote) != 0 {
		t.Error("each side has one in an ocean")
	}
	// Only the enemies' do — and the cult's own are on land: it has none in oceans.
	st = tb.as(tb.build(hydra(xu{who: "cc", typ: "nyarlathotep", area: "north-atlantic"})), "gc", "recruit@asia")
	if tb.data(st).IBooks&(1<<ibZygote) == 0 {
		t.Error("the cult controls no Great Old One in an ocean")
	}
	// Only its own are in an ocean: the enemies have none.
	st = tb.as(tb.build(hydra(xu{who: "gc", typ: "cthulhu", area: "south-pacific"})), "gc", "recruit@asia")
	if tb.data(st).IBooks&(1<<ibZygote) == 0 {
		t.Error("the enemies control no Great Old One in an ocean")
	}
}

func TestYigsSnakebiteGivesTheEnemyAnExtraKillWhenCultistsDie(t *testing.T) {
	tb := indep(t)
	st := tb.build(xp{
		units: []xu{
			{who: "gc", typ: "yig", area: "europe"}, {who: "gc", typ: "cultist", area: "asia", n: 2},
			{who: "cc", typ: "hunting-horror", area: "asia", n: 2},
		},
		power: map[string]int{"cc": 6}, turn: "cc",
		edit: func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdYig) },
		pool: map[string]int{"gc|yig": 0},
		rng:  seedDice(t, 6, 1, 1, 1),
	})
	st = tb.as(st, "cc", "battle@asia/gc")
	if n := tb.n(st, "cc", "hunting-horror", "asia"); n != 1 {
		t.Errorf("one Cultist died, so the poison killed a Hunting Horror: %d left", n)
	}
	// Without Yig in play there is no poison.
	st = tb.build(xp{
		units: []xu{{who: "gc", typ: "cultist", area: "asia", n: 2}, {who: "cc", typ: "hunting-horror", area: "asia", n: 2}},
		power: map[string]int{"cc": 6}, turn: "cc", rng: seedDice(t, 6, 1, 1, 1),
	})
	st = tb.as(st, "cc", "battle@asia/gc")
	if n := tb.n(st, "cc", "hunting-horror", "asia"); n != 2 {
		t.Errorf("no Yig, no poison: %d left", n)
	}
}

func TestYigsRequirementRemovesAGateAndMessengerOfYigCollects(t *testing.T) {
	tb := indep(t)
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "yig", area: "south-pacific"}, {who: "gc", typ: "cultist", area: "south-pacific"}, {who: "gc", typ: "cultist", area: "asia"}},
		gates: map[string]string{"south-pacific": "gc", "asia": "gc"},
		power: map[string]int{"gc": 6},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdYig) },
		pool:  map[string]int{"gc|yig": 0},
	})
	got := tb.can(st, "gc")
	if !has(got, "gateoff@south-pacific") || !has(got, "gateoff@asia") {
		t.Fatalf("remove one of the controlled Gates: %v", got)
	}
	st = tb.as(st, "gc", "gateoff@asia")
	d := tb.data(st)
	if _, ok := d.Gates["asia"]; ok {
		t.Error("the Gate is gone from the map")
	}
	if d.IBooks&(1<<ibMessenger) == 0 {
		t.Fatal("Messenger of Yig is earned")
	}
	// In the Doom phase every other cult gives 1 Power — or Yig's owner gains 1 Doom.
	st = tb.build(xp{
		units: []xu{{who: "gc", typ: "yig", area: "south-pacific"}, {who: "gc", typ: "cultist", area: "south-pacific"}},
		gates: map[string]string{"south-pacific": "gc"},
		power: map[string]int{"gc": 2, "cc": 5, "bg": 5, "ys": 0},
		reqs:  map[string][]int{"gc": {0}},
		stage: "direction", phase: "action",
		edit: func(d *cthulhuwars.Data) {
			holds(d, tb, "gc", cdYig)
			holdsBook(d, ibMessenger)
		},
		pool: map[string]int{"gc|yig": 0},
	})
	st = tb.as(st, "gc", "cw")
	d = tb.data(st)
	if d.Stage != "prompt" || d.Prompts[0].Kind != "donate" || d.Prompts[0].Seat != 2 {
		t.Fatalf("Crawling Chaos is asked first: stage %q %+v", d.Stage, d.Prompts)
	}
	st = tb.as(st, "cc", "donate:give")
	st = tb.as(st, "bg", "donate:refuse")
	d = tb.data(st)
	// Yellow Sign has no Power to give: it cannot give, so Yig's owner gains the Doom.
	if d.Power[1] != 3 || d.Power[2] != 4 {
		t.Errorf("1 Power moved from Crawling Chaos to Great Cthulhu: %d %d", d.Power[1], d.Power[2])
	}
	if d.Doom[1] != 1+2 {
		t.Errorf("1 Doom for the Gate, 1 for the refusal and 1 for the cult that could not give: %d", d.Doom[1])
	}
}

func TestAnIndependentHelpsToAwakenAnother(t *testing.T) {
	tb := indep(t)
	// A cult with no Great Old One of its own but Abhoth at its Gate.
	st := tb.build(xp{
		units: []xu{{who: "bg", typ: "abhoth", area: "south-america-west"}, {who: "bg", typ: "cultist", area: "south-america-west"}},
		gates: map[string]string{"south-america-west": "bg"},
		power: map[string]int{"bg": 8}, turn: "bg",
		edit: func(d *cthulhuwars.Data) { holds(d, tb, "bg", cdAbhoth) },
		pool: map[string]int{"bg|abhoth": 0},
	})
	got := tb.can(st, "bg")
	if !has(got, "awaken:yig@south-america-west") || !has(got, "awaken:chaugnar-faugn@south-america-west") {
		t.Fatalf("one Independent helps to awaken another: %v", got)
	}
	if has(got, "awaken:abhoth@south-america-west") {
		t.Errorf("Abhoth is awake already: %v", got)
	}
	// Without a Great Old One at the Gate nothing can be awakened.
	st = tb.build(xp{
		units: []xu{{who: "bg", typ: "cultist", area: "south-america-west"}},
		gates: map[string]string{"south-america-west": "bg"},
		power: map[string]int{"bg": 8}, turn: "bg",
	})
	if got := tb.can(st, "bg"); hasPrefix(got, "awaken:") {
		t.Errorf("no Great Old One, no awakening: %v", got)
	}
}

func TestAnElderThingSilencesAnIndependentsAbility(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"azathoth": true, "igoo-abhoth": true, "igoo-hydra": true})
	build := func(elder bool) *state.BoardState {
		units := []xu{{who: "gc", typ: "abhoth", area: "south-pacific"}, {who: "gc", typ: "mother-hydra", area: "indian-ocean"}, {who: "cc", typ: "cultist", area: "indian-ocean"}}
		if elder {
			units = append(units, xu{who: "cc", typ: "elder-thing", area: "south-pacific"})
		}
		return tb.build(xp{
			units: units,
			power: map[string]int{"gc": 5},
			edit: func(d *cthulhuwars.Data) {
				holds(d, tb, "gc", cdAbhoth)
				holds(d, tb, "gc", cdHydra)
				holds(d, tb, "cc", cdElderThing)
			},
			pool: map[string]int{"gc|abhoth": 0, "gc|mother-hydra": 0, "gc|filth": 12, "cc|elder-thing": 3 - map[bool]int{true: 1, false: 0}[elder]},
		})
	}
	if got := tb.can(build(false), "gc"); !has(got, "filth@asia") || !has(got, "sting@indian-ocean") {
		t.Fatalf("both abilities work: %v", got)
	}
	got := tb.can(build(true), "gc")
	if has(got, "filth@asia") {
		t.Errorf("an Elder Thing in Abhoth's area: no Filth: %v", got)
	}
	if !has(got, "sting@indian-ocean") {
		t.Errorf("Mother Hydra has none beside her: %v", got)
	}
}

func TestATerrorIsNeitherAMonsterNorACultistForDevourAndShriveling(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"azathoth": true, "beyond": true})
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "cthulhu", area: "asia"}, {who: "cc", typ: "hound-of-tindalos", area: "asia"}, {who: "cc", typ: "cultist", area: "asia"}},
		power: map[string]int{"gc": 5},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "cc", cdHound) },
		pool:  map[string]int{"cc|hound-of-tindalos": 0},
		rng:   seedDice(t, 1, 1, 1, 1, 1, 1),
	})
	st = tb.as(st, "gc", "battle@asia/cc")
	// Only the Cultist is a Monster or Cultist, so there is nothing to choose.
	if tb.n(st, "cc", "cultist", "asia") != 0 || tb.n(st, "cc", "hound-of-tindalos", "asia") != 1 {
		t.Error("Devour takes the Cultist, never the Terror")
	}
}
