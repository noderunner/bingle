package cthulhuwars_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// Rules the wiki's current faction pages state and an earlier reconstruction
// got wrong.

func gameID(t testing.TB, id string) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("%s is not registered", id)
	}
	return g
}

// toMoveSeat is whose decision the position waits for.
func toMoveSeat(t testing.TB, g *catalog.CompiledGame, st *state.BoardState) int {
	t.Helper()
	seat, err := state.TurnAfter(g, st, 0)
	if err != nil {
		t.Fatal(err)
	}
	return seat
}

// onBoard counts seat's units of a type anywhere on the map.
func onBoard(st *state.BoardState, seat int, typ string) int {
	n := 0
	for _, id := range st.Occupied() {
		for _, o := range st.Stack(id) {
			if o.Piece == typ && o.Color == colors[seat] {
				n++
			}
		}
	}
	return n
}

// seedWhere finds a generator state whose first n dice satisfy ok.
func seedWhere(t testing.TB, n int, ok func(dice []int) bool) uint64 {
	t.Helper()
	for seed := uint64(1); seed < 3_000_000; seed++ {
		dice, _ := cthulhuwars.RollDice(seed, n)
		if ok(dice) {
			return seed
		}
	}
	t.Fatal("no seed produces the wanted dice")
	return 0
}

func TestTheRitualTrackIsAsLongAsTheTableNeeds(t *testing.T) {
	// Four players: eight spaces, 5 6 7 7 8 8 9 10, then Instant Death.
	g := game(t)
	want := []int{5, 6, 7, 7, 8, 8, 9, 10}
	for i, cost := range want {
		p := inDoom(pos{
			units: []u{{gc, "cultist", "south-pacific", 1}},
			gates: map[string]int{"south-pacific": gc},
			power: powers(10, 0, 0, 0), ritual: i,
			books: [5]uint8{0, 63, 63, 63, 63},
		})
		st := play(t, g, p.build(), 1, "ritual")
		d := data(t, st)
		if d.Power[1] != 10-cost {
			t.Errorf("space %d: the Ritual cost %d Power, want %d", i, 10-d.Power[1], cost)
		}
		if d.Ritual != i+1 {
			t.Errorf("space %d: the marker is on %d", i, d.Ritual)
		}
		if d.Instant != (i == len(want)-1) {
			t.Errorf("space %d: instant death %v", i, d.Instant)
		}
	}
	// Three players: six spaces, 5 6 7 8 9 10.
	g3 := gameID(t, "cthulhu-wars-3p")
	want3 := []int{5, 6, 7, 8, 9, 10}
	for i, cost := range want3 {
		p := inDoom(pos{
			units: []u{{gc, "cultist", "south-pacific", 1}},
			gates: map[string]int{"south-pacific": gc},
			power: powers(10, 0, 0, 0), ritual: i,
			books: [5]uint8{0, 63, 63, 63, 0},
		})
		p.doomx = &cthulhuwars.DoomState{Order: []int{1, 2, 3}}
		st := play(t, g3, p.build(), 1, "ritual")
		d := data(t, st)
		if d.Power[1] != 10-cost || d.Instant != (i == len(want3)-1) {
			t.Errorf("3 players, space %d: power %d (cost %d) instant %v", i, d.Power[1], cost, d.Instant)
		}
	}
}

func TestInstantDeathEndsTheGameWhenTheDoomPhaseIsScored(t *testing.T) {
	g := game(t)
	p := inDoom(pos{
		units: []u{{gc, "cultist", "south-pacific", 1}, {cc, "cultist", "asia", 1}},
		gates: map[string]int{"south-pacific": gc, "asia": cc},
		power: powers(10, 12, 0, 0), ritual: 7,
		books: [5]uint8{0, 63, 63, 0, 0},
	})
	p.doom = [5]int{0, 3, 0, 0, 0}
	st := play(t, g, p.build(), 1, "ritual")
	if d := data(t, st); d.Phase == "over" {
		t.Fatal("the game ends when the phase is scored, not at the Ritual")
	}
	// The next player may still perform a Ritual (at 10); then it is over.
	if got := notations(g, st, 2); !has(got, "ritual") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, 2, "ritual")
	d := data(t, st)
	if d.Phase != "over" || d.Reason != "instant-death" {
		t.Fatalf("phase %q reason %q", d.Phase, d.Reason)
	}
	// The one with the most Doom and six books wins.
	if d.Winner != gc && d.Winner != cc {
		t.Fatalf("winner %d", d.Winner)
	}
}

func TestAwakeningAnyGreatOldOneGivesGreatCthulhuAnElderSign(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(10, 0, 0, 0),
	}
	st := play(t, g, p.build(), gc, "awaken:cthulhu@south-pacific")
	d := data(t, st)
	if len(d.Signs[gc]) != 1 {
		t.Fatalf("Immortal: an Elder Sign for awakening: %v", d.Signs[gc])
	}
	// Re-awakening costs 4 and gives another.
	q := pos{
		units: []u{{gc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(4, 0, 0, 0), awoken: map[string]bool{"1|0": true},
	}
	st = play(t, g, q.build(), gc, "awaken:cthulhu@south-pacific")
	d = data(t, st)
	if len(d.Signs[gc]) != 1 || d.Power[gc] != 0 {
		t.Fatalf("signs %v power %d", d.Signs[gc], d.Power[gc])
	}
	// Other cults do not gain one for awakening theirs.
	r := pos{
		units: []u{{cc, "cultist", "asia", 1}},
		gates: map[string]int{"asia": cc},
		power: powers(0, 10, 0, 0), turn: cc,
	}
	st = play(t, g, r.build(), cc, "awaken:nyarlathotep@asia")
	if n := len(data(t, st).Signs[cc]); n != 0 {
		t.Fatalf("Crawling Chaos gained %d Elder Signs", n)
	}
}

func TestCthulhuAwakensOnlyInTheStartingAreaWhoseGateMayBeAnyones(t *testing.T) {
	g := game(t)
	mk := func(units []u, gates map[string]int) pos {
		return pos{units: units, gates: gates, power: powers(10, 0, 0, 0)}
	}
	// An ocean Gate elsewhere is no use.
	st := mk([]u{{gc, "cultist", "north-pacific", 1}}, map[string]int{"north-pacific": gc}).build()
	if got := notations(g, st, gc); hasPrefix(got, "awaken:cthulhu") {
		t.Fatalf("Cthulhu wakes only in the South Pacific: %v", got)
	}
	// An abandoned Gate there is enough.
	st = mk([]u{{gc, "cultist", "north-pacific", 1}}, map[string]int{"south-pacific": 0}).build()
	if got := notations(g, st, gc); !has(got, "awaken:cthulhu@south-pacific") {
		t.Fatalf("an abandoned Gate will do: %v", got)
	}
	// So is an enemy's.
	st = mk([]u{{gc, "cultist", "north-pacific", 1}, {cc, "cultist", "south-pacific", 1}}, map[string]int{"south-pacific": cc}).build()
	if got := notations(g, st, gc); !has(got, "awaken:cthulhu@south-pacific") {
		t.Fatalf("an enemy's Gate will do: %v", got)
	}
	st = play(t, g, st, gc, "awaken:cthulhu@south-pacific")
	if count(st, gc, "cthulhu", "south-pacific") != 1 {
		t.Fatal("Cthulhu rises in the South Pacific")
	}
}

func TestTheLastSpellbookComesInADoomPhaseWithAnElderSign(t *testing.T) {
	g := game(t)
	// Five books, the first-Doom requirement already used: the sixth is taken
	// at the start of the Doom phase, with an Elder Sign.
	p := lastPass(pos{
		units: []u{{gc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		books: [5]uint8{0, 31, 0, 0, 0}, reqs: [5]uint8{0, 1 | 2 | 4 | 8 | 16, 0, 0, 0},
	})
	st := play(t, g, p.build(), 1, "pass")
	d := data(t, st)
	if d.First != gc {
		t.Skipf("first player %d: the walk to the Doom phase differs", d.First)
	}
	st = play(t, g, st, gc, "cw")
	d = data(t, st)
	if d.Books[gc] != 63 || len(d.Signs[gc]) != 1 {
		t.Fatalf("books %06b signs %v", d.Books[gc], d.Signs[gc])
	}
}

func TestTwoKillsMeetOneRequirementThreeMeetBoth(t *testing.T) {
	g := game(t)
	// Cthulhu devours one Nightgaunt and rolls kills: two units gone in all →
	// one spellbook; three gone → both.
	run := func(dice []int, nightgaunts int) *cthulhuwars.Data {
		units := []u{{gc, "cthulhu", "asia", 1}, {cc, "nightgaunt", "asia", nightgaunts}}
		p := pos{units: units, power: powers(8, 0, 0, 0), rng: seedDice(t, dice...)}
		st := play(t, g, p.build(), gc, "battle@asia/cc")
		for guard := 0; guard < 12; guard++ {
			d := data(t, st)
			if d.Stage != "prompt" && d.Stage != "assign" {
				break
			}
			seat := toMoveSeat(t, g, st)
			legal := notations(g, st, seat)
			if len(legal) == 0 {
				break
			}
			pick := legal[0]
			for _, n := range legal {
				if len(n) > 5 && n[:5] == "book:" {
					pick = n
					break
				}
			}
			st = play(t, g, st, seat, pick)
		}
		return data(t, st)
	}
	const req1, req2 = uint8(1 << 1), uint8(1 << 2) // kill-1, kill-2 of Great Cthulhu's card
	// Devour + 1 kill.
	d := run([]int{6, 1, 1, 1, 1, 1}, 3)
	if d.Reqs[gc]&req2 == 0 || d.Reqs[gc]&req1 != 0 {
		t.Errorf("two units: only the two-unit requirement is met: %06b", d.Reqs[gc])
	}
	// Devour + 2 kills.
	d = run([]int{6, 6, 1, 1, 1, 1}, 4)
	if d.Reqs[gc]&req2 == 0 || d.Reqs[gc]&req1 == 0 {
		t.Errorf("three units: both requirements are met: %06b", d.Reqs[gc])
	}
}

func TestAvatarSwapsShubNiggurathWithAUnitItsOwnerChooses(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{bg, "shub-niggurath", "africa", 1}, {bg, "cultist", "africa", 1}, {gc, "cultist", "asia", 1}, {gc, "deep-one", "asia", 1}},
		gates: map[string]int{"africa": bg},
		power: powers(0, 0, 3, 0), turn: bg,
	}
	st := p.build()
	if got := notations(g, st, bg); !has(got, "avatar@asia/gc") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, bg, "avatar@asia/gc")
	d := data(t, st)
	if d.Stage != "prompt" || d.Prompts[0].Kind != "avatar" || d.Prompts[0].Seat != gc {
		t.Fatalf("the owner chooses the unit: stage %q prompts %+v", d.Stage, d.Prompts)
	}
	if got := notations(g, st, gc); !has(got, "swap:cultist") || !has(got, "swap:deep-one") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, gc, "swap:deep-one")
	if count(st, bg, "shub-niggurath", "asia") != 1 || count(st, gc, "deep-one", "africa") != 1 || count(st, gc, "deep-one", "asia") != 0 {
		t.Fatal("Shub-Niggurath and the Deep One trade places")
	}
	if d := data(t, st); d.Power[bg] != 2 {
		t.Fatalf("Avatar costs 1: power %d", d.Power[bg])
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
}

func TestTheRedSignGivesPowerAndShubNiggurathCombatPerDarkYoung(t *testing.T) {
	g := game(t)
	// Gather Power: a Cultist (1), a Gate (2) and two Dark Young (1 each).
	p := lastPass(pos{
		units: []u{{bg, "cultist", "africa", 1}, {bg, "dark-young", "africa", 2}},
		gates: map[string]int{"africa": bg},
		books: [5]uint8{0, 0, 0, books(bkRedSign), 0},
	})
	st := play(t, g, p.build(), 1, "pass")
	if d := data(t, st); d.Power[bg] != 5 {
		t.Fatalf("Power %d, want 1 + 2 + 2", d.Power[bg])
	}
	// Combat: Shub-Niggurath = Gates (1) + Cultists (1) + Dark Young (2), and
	// each Dark Young rolls 2.
	q := pos{
		units: []u{{bg, "shub-niggurath", "asia", 1}, {bg, "cultist", "asia", 1}, {bg, "dark-young", "asia", 2}, {cc, "cultist", "asia", 1}},
		gates: map[string]int{"asia": bg},
		power: powers(0, 0, 4, 0), turn: bg, rng: seedDice(t, 1, 1, 1, 1, 1, 1, 1, 1),
		books: [5]uint8{0, 0, 0, books(bkRedSign), 0},
	}
	st = play(t, g, q.build(), bg, "battle@asia/cc")
	d := data(t, st)
	if d.Last == nil || d.Last.Combat[bg] != 8 {
		t.Fatalf("last %+v", d.Last)
	}
}

func TestSharingAreasPaysOnePowerPerEnemyButSacrificingDoesNot(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{
			{bg, "cultist", "africa", 1}, {gc, "cultist", "africa", 1},
			{bg, "cultist", "asia", 1}, {cc, "cultist", "asia", 1},
			{bg, "cultist", "europe", 1}, {ys, "cultist", "europe", 1},
		},
		power: powers(0, 0, 5, 0), turn: bg,
	}
	st := play(t, g, p.build(), bg, "recruit@africa")
	d := data(t, st)
	// 5 - 1 for the Cultist + 3 for sharing areas with all three enemies.
	if d.Power[bg] != 7 {
		t.Fatalf("Power %d, want 7", d.Power[bg])
	}
	if d.Stage != "prompt" || d.Prompts[0].Kind != "book" {
		t.Fatalf("a spellbook is earned: stage %q prompts %+v", d.Stage, d.Prompts)
	}
}

func TestPayingTenPowerMeetsBothCrawlingChaosRequirements(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{cc, "cultist", "asia", 1}}, turn: cc, power: powers(0, 10, 0, 0)}
	st := p.build()
	if got := notations(g, st, cc); !has(got, "pay/10") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, cc, "pay/10")
	d := data(t, st)
	if d.Power[cc] != 0 || len(d.Prompts) != 2 {
		t.Fatalf("power %d prompts %+v", d.Power[cc], d.Prompts)
	}
	st = play(t, g, st, cc, "book:abduct")
	st = play(t, g, st, cc, "book:seek")
	if d := data(t, st); d.Books[cc] != books(0, 2) {
		t.Fatalf("books %06b", d.Books[cc])
	}
}

func TestTheThousandFormsIsAllOrNothingAndOncePerActionPhase(t *testing.T) {
	g := game(t)
	mk := func() pos {
		return pos{
			units: []u{{cc, "nyarlathotep", "asia", 1}},
			power: powers(1, 5, 1, 1), books: [5]uint8{0, 0, books(bkForms), 0, 0},
			turn: cc, rng: seedDice(t, 6),
		}
	}
	// The foes hold 3 Power between them and the die shows 6: they cannot pay
	// it, so Crawling Chaos receives 6.
	st := play(t, g, mk().build(), cc, "forms")
	d := data(t, st)
	if d.Power[cc] != 11 || d.Power[gc] != 1 || d.Power[bg] != 1 || d.Power[ys] != 1 {
		t.Fatalf("power %v", d.Power)
	}
	if !d.FormsUsed {
		t.Fatal("the spellbook is turned face down")
	}
	// Face down it cannot be used again this Action phase.
	st = mk().build()
	data(t, st).FormsUsed = true
	if got := notations(g, st, cc); has(got, "forms") {
		t.Fatalf("used already: %v", got)
	}
}

func TestTheHarbingerPaysForGreatOldOnesThatAreHit(t *testing.T) {
	g := game(t)
	mk := func() pos {
		return pos{
			units: []u{{cc, "nyarlathotep", "asia", 1}, {gc, "cthulhu", "asia", 1}},
			power: powers(0, 8, 0, 0), turn: cc,
			books:  [5]uint8{0, 0, 15, 0, 0},
			awoken: map[string]bool{"1|0": true},
			// Nyarlathotep rolls 4 dice (his 4 spellbooks): a Pain and no
			// Kill; Cthulhu's 6 dice score no Kill on him.
			rng: seedWhere(t, 10, func(d []int) bool {
				for i, v := range d {
					if i == 0 && v != 4 || i > 0 && i < 4 && v >= 4 || i >= 4 && v == 6 {
						return false
					}
				}
				return true
			}),
		}
	}
	for _, c := range []struct {
		pick       string
		power, ess int
	}{{"harbinger:power", 8 - 1 + 2, 0}, {"harbinger:signs", 8 - 1, 2}} {
		st := play(t, g, mk().build(), cc, "battle@asia/gc")
		d := data(t, st)
		// Cthulhu's Devour finds nothing to take; the dice are rolled, the
		// Pain is assigned to Cthulhu and the Harbinger is asked.
		if d.Stage != "post" || d.Bat == nil || d.Bat.Who != cc {
			t.Fatalf("stage %q battle %+v", d.Stage, d.Bat)
		}
		if got := notations(g, st, cc); len(got) != 2 || !has(got, "harbinger:power") || !has(got, "harbinger:signs") {
			t.Fatalf("options = %v", got)
		}
		st = play(t, g, st, cc, c.pick)
		d = data(t, st)
		if d.Power[cc] != c.power || len(d.Signs[cc]) != c.ess {
			t.Errorf("%s: power %d (want %d), signs %v (want %d)", c.pick, d.Power[cc], c.power, d.Signs[cc], c.ess)
		}
	}
}

func TestMadnessArrangesTheRetreatsOfABattleItIsNotIn(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{
			{gc, "deep-one", "asia", 2}, {ys, "byakhee", "asia", 1},
			{cc, "cultist", "africa", 1},
		},
		power: powers(8, 0, 0, 0), turn: gc,
		books: [5]uint8{0, 0, books(bkMadness), 0, 0},
		// Great Cthulhu rolls 2 dice (4, 4): two Pains on the Byakhee; the
		// Byakhee's 2 dice: a Pain on a Deep One.
		rng: seedDice(t, 4, 4, 4, 1),
	}
	st := play(t, g, p.build(), gc, "battle@asia/ys")
	d := data(t, st)
	if d.Stage != "retreat" || d.Bat == nil || d.Bat.Retreater != cc {
		t.Fatalf("Crawling Chaos arranges the retreats: stage %q battle %+v", d.Stage, d.Bat)
	}
	r := state.PendingReaction(g, st)
	if r == nil || r.Seat != cc || r.Trigger != gc || r.Ability != "Madness" {
		t.Fatalf("it is a reaction: %+v", r)
	}
	got := notations(g, st, cc)
	// Units of both sides, in any order — an enemy's named by its faction.
	if !hasPrefix(got, "retreat:deep-one>") || !hasPrefix(got, "retreat:byakhee>") || !has(got, "done") {
		t.Fatalf("options = %v", got)
	}
	for _, n := range got {
		if len(n) > 16 && n[:16] == "retreat:byakhee>" && n[len(n)-3:] != "/ys" {
			t.Fatalf("another faction's unit names its owner: %s", n)
		}
	}
	// Declining leaves the choice to the owners.
	st = play(t, g, st, cc, "done")
	d = data(t, st)
	if d.Stage != "retreat" || d.Bat.Retreater != 0 {
		t.Fatalf("stage %q retreater %d", d.Stage, d.Bat.Retreater)
	}
}

func TestNecrophagyFollowsTheResultsAndTheGhoulsItCallsTakeNoPain(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{
			{bg, "fungi", "asia", 1}, {gc, "deep-one", "asia", 1},
			{bg, "ghoul", "africa", 2},
		},
		power: powers(0, 0, 4, 0), turn: bg,
		books: [5]uint8{0, 0, 0, books(bkNecro), 0},
		rng:   seedDice(t, 1, 1),
	}
	st := play(t, g, p.build(), bg, "battle@asia/gc")
	d := data(t, st)
	// The dice are rolled and applied (nothing happens), then Necrophagy.
	if d.Stage != "post" || d.Bat.Who != bg || !d.Bat.Assigned {
		t.Fatalf("stage %q battle %+v", d.Stage, d.Bat)
	}
	st = play(t, g, st, bg, "necro")
	// Each side takes 2 extra Pains, but the Ghouls just called take none
	// and the only other unit of each side takes one.
	d = data(t, st)
	if d.Stage != "retreat" {
		t.Fatalf("stage %q: the pained Fungus and Deep One retreat", d.Stage)
	}
	if count(st, bg, "ghoul", "asia") != 2 {
		t.Fatalf("the called Ghouls stay: %d in Asia", count(st, bg, "ghoul", "asia"))
	}
}

func TestDevolveIsOfferedAfterAnotherPlayersAction(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 2}, {cc, "cultist", "asia", 1}},
		gates: map[string]int{"south-pacific": gc, "asia": cc},
		power: powers(2, 3, 0, 0), turn: cc,
		books: [5]uint8{0, books(bkDevolve), 0, 0, 0},
	}
	st := play(t, g, p.build(), cc, "recruit@asia")
	d := data(t, st)
	if d.Stage != "prompt" || len(d.Prompts) != 1 || d.Prompts[0].Kind != "react" || d.Prompts[0].Seat != gc || d.Prompts[0].By != cc {
		t.Fatalf("stage %q prompts %+v", d.Stage, d.Prompts)
	}
	r := state.PendingReaction(g, st)
	if r == nil || r.Seat != gc || r.Trigger != cc || r.Ability != "Devolve" || r.Pass.Action != "done" {
		t.Fatalf("reaction %+v", r)
	}
	if got := notations(g, st, gc); !has(got, "devolve@south-pacific") || !has(got, "done") {
		t.Fatalf("options = %v", got)
	}
	// Use it twice (any number), then the window closes by itself: no Cultist
	// is left to replace.
	st = play(t, g, st, gc, "devolve@south-pacific")
	if d := data(t, st); d.Stage != "prompt" {
		t.Fatalf("the window stays open: stage %q", d.Stage)
	}
	st = play(t, g, st, gc, "devolve@south-pacific")
	d = data(t, st)
	if d.Stage != "turn" || count(st, gc, "deep-one", "south-pacific") != 2 {
		t.Fatalf("stage %q, deep ones %d", d.Stage, count(st, gc, "deep-one", "south-pacific"))
	}
	if r := state.PendingReaction(g, st); r != nil {
		t.Fatalf("no reaction pending: %+v", r)
	}
	// Declining is a single move.
	st = play(t, g, p.build(), cc, "recruit@asia")
	st = play(t, g, st, gc, "done")
	if d := data(t, st); d.Stage != "turn" || d.Turn.Seat != gc {
		t.Fatalf("stage %q turn %+v", d.Stage, d.Turn)
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
}

func TestDevolveAnswersABattleDeclaredAgainstGreatCthulhu(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "asia", 2}, {cc, "hunting-horror", "asia", 1}},
		power: powers(0, 3, 0, 0), turn: cc,
		books: [5]uint8{0, books(bkDevolve), 0, 0, 0},
		rng:   seedDice(t, 1, 1, 1, 1),
	}
	st := play(t, g, p.build(), cc, "battle@asia/gc")
	d := data(t, st)
	if d.Stage != "prompt" || d.Prompts[0].Kind != "react" || d.Prompts[0].Seat != gc || d.Prompts[0].Area != "asia" {
		t.Fatalf("stage %q prompts %+v", d.Stage, d.Prompts)
	}
	st = play(t, g, st, gc, "devolve@asia")
	st = play(t, g, st, gc, "done")
	d = data(t, st)
	// One Cultist is now a Deep One: GC rolls 1 die; the battle goes on.
	if d.Last == nil || d.Last.Combat[gc] != 1 {
		t.Fatalf("last %+v", d.Last)
	}
}

func TestRegenerateLetsAStarSpawnTakeTwoResults(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{cc, "hunting-horror", "asia", 1}, {gc, "starspawn", "asia", 1}, {gc, "deep-one", "asia", 1}},
		power: powers(0, 3, 0, 0), turn: cc,
		books: [5]uint8{0, books(bkRegenerate), 0, 0, 0},
		// The Horror rolls a Kill and a Pain; Great Cthulhu's 4 dice miss.
		rng: seedDice(t, 6, 4, 1, 1, 1, 1),
	}
	st := play(t, g, p.build(), cc, "battle@asia/gc")
	d := data(t, st)
	if d.Stage != "assign" {
		t.Fatalf("stage %q", d.Stage)
	}
	st = play(t, g, st, gc, "assign:starspawn") // the Kill: only a Pain for it
	d = data(t, st)
	if d.Stage != "assign" {
		t.Fatalf("stage %q: the Pain is still to assign", d.Stage)
	}
	got := notations(g, st, gc)
	if !has(got, "assign:starspawn") {
		t.Fatalf("the Star Spawn can take the Pain too: %v", got)
	}
	st = play(t, g, st, gc, "assign:starspawn")
	// Nothing else was hurt: the Deep One stays, the Star Spawn retreats.
	for guard := 0; guard < 4 && data(t, st).Phase == "action" && data(t, st).Bat != nil; guard++ {
		seat := toMoveSeat(t, g, st)
		legal := notations(g, st, seat)
		st = play(t, g, st, seat, legal[0])
	}
	if count(st, gc, "deep-one", "asia") != 1 {
		t.Fatal("the Deep One took no result")
	}
	if count(st, gc, "starspawn", "asia") != 0 {
		t.Fatal("the Star Spawn retreated")
	}
	if n := onBoard(st, gc, "starspawn"); n != 1 {
		t.Fatalf("the Star Spawn lives: %d on the map", n)
	}
}

func TestInvisibilityMayHideAnEnemyMonsterOrCultist(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "deep-one", "asia", 2}, {cc, "flying-polyp", "asia", 1}},
		power: powers(0, 4, 0, 0), turn: cc,
		books: [5]uint8{0, 0, books(bkInvisibility), 0, 0},
		rng:   seedDice(t, 1, 1, 1, 1),
	}
	st := play(t, g, p.build(), cc, "battle@asia/gc")
	got := notations(g, st, cc)
	if !has(got, "invisible:deep-one/gc") || !has(got, "invisible:flying-polyp") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, cc, "invisible:deep-one/gc")
	d := data(t, st)
	// Great Cthulhu's side is down one Deep One: 1 die.
	if d.Last == nil || d.Last.Combat[gc] != 1 {
		t.Fatalf("last %+v", d.Last)
	}
}

func TestYellowSignsFourthRequirementGivesAnotherPlayerThreeDoom(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{ys, "cultist", "europe", 1}},
		power: powers(0, 0, 0, 3), turn: ys,
	}
	st := p.build()
	got := notations(g, st, ys)
	if !has(got, "grant:gc") || !has(got, "grant:cc") || !has(got, "grant:bg") || has(got, "grant:ys") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, ys, "grant:cc")
	d := data(t, st)
	if d.Doom[cc] != 3 || d.Stage != "prompt" || d.Prompts[0].Kind != "book" {
		t.Fatalf("doom %v stage %q prompts %+v", d.Doom, d.Stage, d.Prompts)
	}
}
