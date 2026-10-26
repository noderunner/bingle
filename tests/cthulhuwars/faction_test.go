package cthulhuwars_test

import (
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Spellbook indexes of the faction cards (mirrors factions.go).
const (
	bkAbsorb, bkDevolve, bkDreams, bkRegenerate, bkSubmerge, bkYha   = 0, 1, 2, 3, 4, 5
	bkAbduct, bkInvisibility, bkSeek, bkEmissary, bkMadness, bkForms = 0, 1, 2, 3, 4, 5
	bkBlood, bkFrenzy, bkGhroth, bkRedSign, bkNecro, bkThousand      = 0, 1, 2, 3, 4, 5
	bkPassion, bkHWINN, bkThirdEye, bkShriek, bkScreaming, bkZingaya = 0, 1, 2, 3, 4, 5
)

func TestSubmergeTakesCthulhuAndTheUnitsYouChooseOffTheMapAndEmergeBringsThemBack(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cthulhu", "south-pacific", 1}, {gc, "deep-one", "south-pacific", 2}, {gc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(4, 0, 0, 0), books: [5]uint8{0, books(bkSubmerge), 0, 0, 0},
	}
	st := p.build()
	if got := notations(g, st, 1); !has(got, "submerge") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, 1, "submerge")
	d := data(t, st)
	// Cthulhu is under at once; the rest of the area is the player's choice.
	if d.Power[gc] != 3 || d.Away["cthulhu"] != 1 || len(d.Away) != 1 {
		t.Fatalf("power %d away %v", d.Power[gc], d.Away)
	}
	if d.Stage != "prompt" || d.Prompts[0].Kind != "submerge" {
		t.Fatalf("stage %q prompts %+v", d.Stage, d.Prompts)
	}
	got := notations(g, st, 1)
	for _, want := range []string{"bring:deep-one", "bring:cultist", "done"} {
		if !has(got, want) {
			t.Fatalf("%q missing from %v", want, got)
		}
	}
	if hasPrefix(got, "bring:cthulhu") {
		t.Fatalf("Cthulhu is already under: %v", got)
	}
	// Take one Deep One along and leave the other and the Cultist behind.
	st = play(t, g, st, 1, "bring:deep-one")
	st = play(t, g, st, 1, "done")
	d = data(t, st)
	if d.Away["cthulhu"] != 1 || d.Away["deep-one"] != 1 || d.Away["cultist"] != 0 {
		t.Fatalf("away %v", d.Away)
	}
	if count(st, gc, "deep-one", "south-pacific") != 1 || count(st, gc, "cultist", "south-pacific") != 1 {
		t.Fatal("the units that were not chosen stay on the map")
	}
	if d.Gates["south-pacific"] != gc {
		t.Fatalf("the Cultist stayed, so the Gate is still ours: %v", d.Gates)
	}
	if n := count(st, gc, "cthulhu", "south-pacific"); n != 0 {
		t.Fatal("Cthulhu must leave the map")
	}
	// Submerging was the turn's action, so the turn is over (nobody else has
	// Power, so Great Cthulhu's next turn follows) and Cthulhu may emerge.
	if d.Stage != "turn" || d.Turn.Main {
		t.Fatalf("stage %q turn %+v", d.Stage, d.Turn)
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
	st = play(t, g, st, 1, "emerge@asia")
	d = data(t, st)
	if count(st, gc, "cthulhu", "asia") != 1 || count(st, gc, "deep-one", "asia") != 1 || len(d.Away) != 0 {
		t.Fatalf("emerge failed: away %v", d.Away)
	}
	if d.Power[gc] != 3 {
		t.Fatalf("emerging is free: power %d", d.Power[gc])
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
}

func TestSubmergeAbandonsAGateWhoseCultistsGoUnder(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cthulhu", "south-pacific", 1}, {gc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(4, 0, 0, 0), books: [5]uint8{0, books(bkSubmerge), 0, 0, 0},
	}
	st := play(t, g, p.build(), 1, "submerge")
	st = play(t, g, st, 1, "bring:cultist")
	d := data(t, st)
	// Nothing else to take: the choice ends by itself.
	if d.Stage != "turn" || d.Away["cultist"] != 1 {
		t.Fatalf("stage %q away %v", d.Stage, d.Away)
	}
	if d.Gates["south-pacific"] != 0 {
		t.Fatalf("a Gate left without a Cultist is abandoned: %v", d.Gates)
	}
}

func TestSubmergeWithNobodyElseThereJustSinksCthulhu(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cthulhu", "south-pacific", 1}},
		power: powers(4, 0, 0, 0), books: [5]uint8{0, books(bkSubmerge), 0, 0, 0},
	}
	st := play(t, g, p.build(), 1, "submerge")
	d := data(t, st)
	if d.Stage != "turn" || d.Away["cthulhu"] != 1 || len(d.Prompts) != 0 {
		t.Fatalf("stage %q away %v prompts %v", d.Stage, d.Away, d.Prompts)
	}
}

func TestSubmergeNeedsAnOcean(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cthulhu", "asia", 1}},
		power: powers(4, 0, 0, 0), books: [5]uint8{0, books(bkSubmerge), 0, 0, 0},
	}
	if got := notations(g, p.build(), 1); has(got, "submerge") {
		t.Fatalf("Cthulhu on land cannot submerge: %v", got)
	}
}

func TestDreamsSwapsAnEnemyCultistForOneOfYours(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 1}, {cc, "cultist", "asia", 2}},
		power: powers(5, 0, 0, 0), books: [5]uint8{0, books(bkDreams), 0, 0, 0},
	}
	st := p.build()
	got := notations(g, st, 1)
	if !has(got, "dreams@asia/cc") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, 1, "dreams@asia/cc")
	d := data(t, st)
	if count(st, cc, "cultist", "asia") != 1 || count(st, gc, "cultist", "asia") != 1 || d.Power[gc] != 3 {
		t.Fatalf("asia: cc %d gc %d power %d", count(st, cc, "cultist", "asia"), count(st, gc, "cultist", "asia"), d.Power[gc])
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
}

func TestDevolveTurnsACultistIntoADeepOneForFree(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "asia", 1}, {gc, "cultist", "europe", 1}},
		gates: map[string]int{"asia": gc},
		power: powers(1, 0, 0, 0), books: [5]uint8{0, books(bkDevolve), 0, 0, 0},
	}
	st := p.build()
	got := notations(g, st, 1)
	if !has(got, "devolve@asia") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, 1, "devolve@asia")
	d := data(t, st)
	if count(st, gc, "deep-one", "asia") != 1 || count(st, gc, "cultist", "asia") != 0 {
		t.Fatal("the Cultist did not turn into a Deep One")
	}
	if _, ok := d.Gates["asia"]; ok && d.Gates["asia"] == gc {
		t.Fatal("the Gate lost its Cultist and is abandoned")
	}
	if d.Stage != "turn" || d.Turn.Seat != gc || d.Power[gc] != 1 {
		t.Fatalf("devolve is unlimited and free: stage %q turn %+v power %d", d.Stage, d.Turn, d.Power[gc])
	}
}

func TestThousandFormsMakesEnemiesPayPower(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{cc, "nyarlathotep", "asia", 1}},
		power: powers(5, 3, 5, 5), books: [5]uint8{0, 0, books(bkForms), 0, 0}, rng: seedDice(t, 3),
	}
	p.turn = cc
	st := p.build()
	if got := notations(g, st, cc); !has(got, "forms") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, cc, "forms")
	d := data(t, st)
	lost := (5 - d.Power[gc]) + (5 - d.Power[bg]) + (5 - d.Power[ys])
	if lost != 3 {
		t.Fatalf("enemies lost %d Power in total, want 3 (power %v)", lost, d.Power)
	}
	if d.Power[cc] != 3 {
		t.Fatalf("The Thousand Forms is free: %d", d.Power[cc])
	}
}

func TestGhrothMakesEnemiesLoseCultistsOrPlacesOne(t *testing.T) {
	g := game(t)
	mk := func(rng uint64) pos {
		return pos{
			units: []u{
				{bg, "fungi", "africa", 1}, {bg, "fungi", "europe", 1}, {bg, "cultist", "africa", 1},
				{gc, "cultist", "asia", 2}, {cc, "cultist", "asia", 2}, {ys, "cultist", "asia", 2},
			},
			turn: bg, power: powers(0, 0, 4, 0), books: [5]uint8{0, 0, 0, books(bkGhroth), 0}, rng: rng,
		}
	}
	lost := func(st *state.BoardState) (total int, by map[int]int) {
		by = map[int]int{}
		for _, s := range []int{gc, cc, ys} {
			by[s] = 2 - count(st, s, "cultist", "asia")
			total += by[s]
		}
		return total, by
	}
	finish := func(st *state.BoardState) *state.BoardState {
		for guard := 0; guard < 10 && data(t, st).Stage == "prompt"; guard++ {
			pr := data(t, st).Prompts[0]
			legal := notations(g, st, pr.Seat)
			if len(legal) == 0 || !strings.HasPrefix(legal[0], "lose:cultist@") {
				t.Fatalf("seat %d must choose a Cultist to lose: %v", pr.Seat, legal)
			}
			st = play(t, g, st, pr.Seat, legal[0])
		}
		return st
	}
	// A 1 is within the two areas where Fungi stand: the enemies eliminate 1
	// Cultist between them — not 1 each.
	st := mk(seedDice(t, 1)).build()
	if got := notations(g, st, bg); !has(got, "ghroth") {
		t.Fatalf("options = %v", got)
	}
	st = finish(play(t, g, st, bg, "ghroth"))
	if total, by := lost(st); total != 1 {
		t.Errorf("a roll of 1 costs the enemies 1 Cultist between them: lost %d (%v)", total, by)
	}
	if data(t, st).Power[bg] != 2 {
		t.Errorf("Ghroth costs 2: power %d", data(t, st).Power[bg])
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
	// A 2 costs two, taken from two different enemies (the ones with the most).
	st = finish(play(t, g, mk(seedDice(t, 2)).build(), bg, "ghroth"))
	if total, by := lost(st); total != 2 {
		t.Errorf("a roll of 2: lost %d (%v)", total, by)
	} else {
		for s, n := range by {
			if n > 1 {
				t.Errorf("seat %d lost %d: the loss is shared out evenly (%v)", s, n, by)
			}
		}
	}
	// A 6 is too high: nobody loses a Cultist, and Black Goat places any
	// faction's Acolyte anywhere instead.
	st = play(t, g, mk(seedDice(t, 6)).build(), bg, "ghroth")
	if total, _ := lost(st); total != 0 {
		t.Errorf("a failed Ghroth costs the enemies nothing: lost %d", total)
	}
	d := data(t, st)
	if d.Stage != "prompt" || d.Prompts[0].Kind != "place" || d.Prompts[0].Seat != bg {
		t.Fatalf("stage %q prompts %+v", d.Stage, d.Prompts)
	}
	got := notations(g, st, bg)
	if !has(got, "place@africa/cc") || !has(got, "place@asia/gc") || !has(got, "place@europe/bg") {
		t.Fatalf("any faction's Acolyte may go anywhere: %v", got)
	}
	st = play(t, g, st, bg, "place@asia/gc")
	if n := count(st, gc, "cultist", "asia"); n != 3 {
		t.Errorf("the placed Acolyte belongs to Great Cthulhu: %d in Asia", n)
	}
	if data(t, st).Stage != "turn" && data(t, st).Turn.Seat == bg && data(t, st).Stage == "prompt" {
		t.Errorf("the choice is over: stage %q", data(t, st).Stage)
	}
}

func TestSacrificeTwoCultistsFulfilsBlackGoatsRequirement(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{bg, "cultist", "africa", 1}, {bg, "cultist", "europe", 1}},
		turn:  bg, power: powers(0, 0, 3, 0),
	}
	st := p.build()
	got := notations(g, st, bg)
	if !has(got, "sacrifice") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, bg, "sacrifice")
	d := data(t, st)
	if d.Stage != "prompt" || d.Prompts[0].Kind != "book" || d.Prompts[0].Seat != bg {
		t.Fatalf("a requirement earns a spellbook: stage %q prompts %+v", d.Stage, d.Prompts)
	}
	if count(st, bg, "cultist", "africa")+count(st, bg, "cultist", "europe") != 0 {
		t.Fatal("both Cultists are eliminated")
	}
	st = play(t, g, st, bg, "book:blood")
	d = data(t, st)
	if d.Books[bg]&(1<<bkBlood) == 0 {
		t.Fatal("the chosen spellbook was not taken")
	}
	// Sacrificing pays nothing: the Power per enemy belongs to the
	// shared-areas requirement.
	if d.Power[bg] != 3 {
		t.Fatalf("sacrificing two Cultists gives no Power: %d", d.Power[bg])
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
}

func TestCrawlingChaosPaysPowerForTwoSpellbooks(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{cc, "cultist", "asia", 1}},
		turn:  cc, power: powers(0, 8, 0, 0),
	}
	st := p.build()
	got := notations(g, st, cc)
	if !has(got, "pay/4") || !has(got, "pay/6") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, cc, "pay/4")
	d := data(t, st)
	if d.Power[cc] != 4 {
		t.Fatalf("power %d", d.Power[cc])
	}
	st = play(t, g, st, cc, "book:abduct")
	if got := notations(g, st, cc); has(got, "pay/4") {
		t.Fatalf("each payment can be made once: %v", got)
	}
}

func TestScreamingDeadMovesTheKingAndHisUndead(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{ys, "king-in-yellow", "europe", 1}, {ys, "undead", "europe", 2}, {ys, "undead", "asia", 1}},
		turn:  ys, power: powers(0, 0, 0, 3), books: [5]uint8{0, 0, 0, 0, books(bkScreaming)},
	}
	st := p.build()
	if got := notations(g, st, ys); !has(got, "scream@arctic-ocean") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, ys, "scream@arctic-ocean")
	if count(st, ys, "king-in-yellow", "arctic-ocean") != 1 {
		t.Fatal("the King moves")
	}
	// The Undead that were with him may come along — each of them, or all.
	got := notations(g, st, ys)
	if !has(got, "bring:undead") || !has(got, "bring:all") || !has(got, "done") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, ys, "bring:undead")
	st = play(t, g, st, ys, "done")
	if count(st, ys, "undead", "arctic-ocean") != 1 || count(st, ys, "undead", "europe") != 1 {
		t.Fatal("the player chose to take one Undead along")
	}
	if count(st, ys, "undead", "asia") != 1 {
		t.Fatal("Undead elsewhere stay")
	}
	if data(t, st).Power[ys] != 2 {
		t.Fatalf("power %d", data(t, st).Power[ys])
	}
	// A second, different action follows — not He Who is Not to be Named.
	got = notations(g, st, ys)
	if has(got, "scream@europe") || has(got, "done") && !hasPrefix(got, "recruit") {
		t.Fatalf("the turn goes on with a second action: %v", got)
	}
	if d := data(t, st); d.Turn == nil || !d.Turn.Second || d.Turn.SecondTaken {
		t.Fatalf("turn %+v", d.Turn)
	}
}

func TestShriekGathersByakheeInOneArea(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{ys, "byakhee", "europe", 1}, {ys, "byakhee", "africa", 2}, {ys, "byakhee", "asia", 1}},
		turn:  ys, power: powers(0, 0, 0, 2), books: [5]uint8{0, 0, 0, 0, books(bkShriek)},
	}
	st := p.build()
	if got := notations(g, st, ys); !has(got, "shriek@asia") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, ys, "shriek@asia")
	// Any or all of them: one Byakhee from Africa, then finished.
	got := notations(g, st, ys)
	if !has(got, "bring:byakhee@africa") || !has(got, "bring:byakhee@europe") || !has(got, "bring:all") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, ys, "bring:byakhee@africa")
	st = play(t, g, st, ys, "done")
	if n := count(st, ys, "byakhee", "asia"); n != 2 {
		t.Fatalf("%d Byakhee in Asia, want the one that was there and one more", n)
	}
	if count(st, ys, "byakhee", "africa") != 1 || count(st, ys, "byakhee", "europe") != 1 {
		t.Fatal("the others stay where they were")
	}
	// All of them.
	st = play(t, g, p.build(), ys, "shriek@asia")
	st = play(t, g, st, ys, "bring:all")
	if n := count(st, ys, "byakhee", "asia"); n != 4 {
		t.Fatalf("%d Byakhee in Asia", n)
	}
}

func TestZingayaReplacesAnEnemyCultistWithAnUndead(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{ys, "undead", "asia", 1}, {cc, "cultist", "asia", 2}},
		turn:  ys, power: powers(0, 0, 0, 2), books: [5]uint8{0, 0, 0, 0, books(bkZingaya)},
	}
	st := p.build()
	if got := notations(g, st, ys); !has(got, "zingaya@asia/cc") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, ys, "zingaya@asia/cc")
	if count(st, cc, "cultist", "asia") != 1 || count(st, ys, "undead", "asia") != 2 {
		t.Fatal("Zingaya must trade a Cultist for an Undead")
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
}

func TestDesecrateRollsAgainstTheUnitsAndFulfilsAGlyphRequirement(t *testing.T) {
	g := game(t)
	mk := func(rng uint64, area string) pos {
		return pos{
			units: []u{{ys, "king-in-yellow", area, 1}, {ys, "undead", area, 2}},
			turn:  ys, power: powers(0, 0, 0, 4), rng: rng,
		}
	}
	// 3 units (the King and two Undead): a 3 or less succeeds. The action is
	// one move that rolls the die; the unit it places is chosen after it.
	before := mk(seedDice(t, 2), "asia").build()
	got := notations(g, before, ys)
	if !has(got, "desecrate") || hasPrefix(got, "desecrate:") {
		t.Fatalf("Desecrate is a single action: %v", got)
	}
	st := play(t, g, before, ys, "desecrate")
	d := data(t, st)
	if !d.Desec["asia"] {
		t.Fatalf("Asia is desecrated: %v", d.Desec)
	}
	if d.Power[ys] != 2 {
		t.Fatalf("Desecrate costs 2: power %d", d.Power[ys])
	}
	// The die is cast: the game cannot be taken back across it.
	if state.CanTakeBack(g, before, st) {
		t.Fatal("a Desecration rolled a die: it cannot be taken back")
	}
	// Whatever the roll, a Cultist or a Monster of cost 2 or less is placed:
	// a choice of the three kinds, made after the roll.
	if d.Stage != "prompt" || d.Prompts[0].Kind != "desecrate" {
		t.Fatalf("stage %q prompts %+v", d.Stage, d.Prompts)
	}
	got = notations(g, st, ys)
	for _, want := range []string{"desecrate:cultist", "desecrate:undead", "desecrate:byakhee"} {
		if !has(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("only the three kinds of cost 2 or less: %v", got)
	}
	st = play(t, g, st, ys, "desecrate:cultist")
	d = data(t, st)
	if count(st, ys, "cultist", "asia") != 1 {
		t.Fatal("the chosen unit is placed")
	}
	// The Dragon glyph requirement (Asia) is met; a spellbook is offered.
	if d.Stage != "prompt" || d.Prompts[0].Kind != "book" {
		t.Fatalf("stage %q prompts %+v", d.Stage, d.Prompts)
	}
	// A 6 fails against 3 units — the unit is placed all the same.
	st = play(t, g, mk(seedDice(t, 6), "asia").build(), ys, "desecrate")
	if data(t, st).Desec["asia"] {
		t.Fatal("a 6 fails against 3 units")
	}
	if data(t, st).Power[ys] != 2 {
		t.Fatal("the Power is spent either way")
	}
	st = play(t, g, st, ys, "desecrate:byakhee")
	if count(st, ys, "byakhee", "asia") != 1 || data(t, st).Desec["asia"] {
		t.Fatal("a failed Desecration still places the unit")
	}
	// Only one Desecration token per area.
	desec := mk(seedDice(t, 1), "asia")
	desec.desec = []string{"asia"}
	if got := notations(g, desec.build(), ys); has(got, "desecrate") {
		t.Fatalf("an area holds one token: %v", got)
	}
}

// With one kind of unit left in the pool there is nothing to choose: it is
// placed at once; with none, nothing is placed.
func TestDesecratePlacesTheOnlyUnitLeftAndNothingWhenThePoolIsEmpty(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, false)
	build := func(pool map[string]int) *state.BoardState {
		return tb.build(xp{
			units: []xu{{"ys", "king-in-yellow", "south-pacific", 1}, {"ys", "undead", "south-pacific", 1}},
			turn:  "ys", power: map[string]int{"ys": 4}, rng: seedDice(t, 1), pool: pool,
		})
	}
	// Only Byakhee remain among the cost-2-or-less kinds.
	st := tb.as(build(map[string]int{"ys|cultist": 0, "ys|undead": 0}), "ys", "desecrate")
	if tb.n(st, "ys", "byakhee", "south-pacific") != 1 {
		t.Fatal("the only unit left is placed without asking")
	}
	if d := tb.data(st); d.Stage == "prompt" {
		t.Fatalf("nothing to choose: %+v", d.Prompts)
	} else if !d.Desec["south-pacific"] {
		t.Fatal("a 1 succeeds")
	}
	// None at all.
	st = tb.as(build(map[string]int{"ys|cultist": 0, "ys|undead": 0, "ys|byakhee": 0}), "ys", "desecrate")
	if d := tb.data(st); d.Stage == "prompt" {
		t.Fatalf("nothing to place, nothing asked: %+v", d.Prompts)
	}
	if tb.n(st, "ys", "undead", "south-pacific") != 1 || tb.n(st, "ys", "byakhee", "south-pacific") != 0 {
		t.Fatal("nothing was placed")
	}
}

func TestHastursSecondActionLetsYellowSignActTwice(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{ys, "hastur", "europe", 1}, {ys, "cultist", "asia", 1}, {gc, "cultist", "africa", 1}},
		gates: map[string]int{"asia": ys},
		turn:  ys, power: powers(0, 0, 0, 5), books: [5]uint8{0, 0, 0, 0, books(bkHWINN)},
	}
	st := p.build()
	if got := notations(g, st, ys); !has(got, "hwinn@africa") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, ys, "hwinn@africa")
	d := data(t, st)
	if count(st, ys, "hastur", "africa") != 1 {
		t.Fatal("Hastur appears in the chosen area")
	}
	if d.Turn == nil || d.Turn.Seat != ys || d.Stage != "turn" {
		t.Fatalf("the turn goes on: stage %q turn %+v", d.Stage, d.Turn)
	}
	// A second, different action: recruit works, "hwinn" does not repeat.
	got := notations(g, st, ys)
	if !has(got, "recruit@asia") {
		t.Fatalf("second action options = %v", got)
	}
	if has(got, "hwinn@africa") {
		t.Fatal("the second action must be a different action")
	}
	st = play(t, g, st, ys, "recruit@asia")
	if d := data(t, st); d.Turn != nil && d.Turn.Seat == ys && d.Stage == "turn" && d.Turn.Main {
		// Both actions used up: the turn passes unless unlimited actions remain.
		t.Logf("turn still with Yellow Sign: %+v", d.Turn)
	}
}
