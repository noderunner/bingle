package cthulhuwars_test

import (
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The Loyalty Cards beyond the Azathoth faction's: the Dreamlands Surface
// Monsters, Beyond Time and Space and the Independent Great Old Ones of Great
// Old One Pack 1. Positions are built by cult key on a table that has the
// packs switched on.

// The numbers of the cards (the bits of Data.Loyal).
const (
	cdShambler = iota
	cdStarVampire
	cdElderThing
	cdServitor
	cdMoonbeast
	cdGnorri
	cdShantak
	cdWamp
	cdVoonith
	cdHound
	cdAbhoth
	cdChaugnar
	cdCthugha
	cdHydra
	cdYig
)

// The Independents' spellbooks (the bits of Data.IBooks).
const (
	ibBrood = iota
	ibCurse
	ibFirestorm
	ibZygote
	ibMessenger
)

// cardTable is a table of the cults in keys with the packs in opts.
func cardTable(t testing.TB, keys []string, opts map[string]any) *table {
	t.Helper()
	g := packTable(t, "cthulhu-wars", keys, opts)
	tb := &table{t: t, g: g, keys: keys, colour: map[string]string{}}
	for i, k := range keys {
		tb.colour[k] = g.Definition.Seats[i].Color
	}
	return tb
}

// holds gives the cult the card.
func holds(d *cthulhuwars.Data, tb *table, who string, card int) {
	d.Loyal[tb.seat(who)] |= 1 << uint(card)
}

var fourPlus = []string{"gc", "cc", "bg", "ys"}

func TestPacksNameTheGameAndSurviveALobbyChange(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	id, err := cat.Derive("cthulhu-wars", map[string]any{
		"factions": []any{"gc", "cc", "bg", "ys"}, "dreamlands": true, "beyond": true,
		"igoo-yig": true, "igoo-abhoth": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "cthulhu-wars~gc-cc-bg-ys-dl-bt-g1ay" {
		t.Errorf("id %q", id)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatal("the derived game does not load")
	}
	for _, want := range []string{"Dreamlands Surface Monsters", "Beyond Time and Space", "Abhoth", "Yig"} {
		if !strings.Contains(g.Definition.Name, want) {
			t.Errorf("the name %q lacks %q", g.Definition.Name, want)
		}
	}
	if g.Definition.Params["dreamlands"] != true || g.Definition.Params["beyond"] != true {
		t.Errorf("params %v", g.Definition.Params)
	}
	// The id loads again by itself, with the same table.
	if again, ok := cat.Get("cthulhu-wars~gc-cc-bg-ys-dl-bt-g1ay"); !ok || again.Definition.Name != g.Definition.Name {
		t.Error("the id does not load again")
	}
	// A cult changed in the lobby keeps the packs.
	opts, ok, err := state.SeatCultOptions(g, 4, "ww", map[int]bool{})
	if err != nil || !ok {
		t.Fatalf("changing a cult: %v %v", ok, err)
	}
	if opts["dreamlands"] != true || opts["beyond"] != true || opts["igoo-yig"] != true || opts["igoo-abhoth"] != true || opts["igoo-hydra"] != false {
		t.Errorf("the packs were lost: %v", opts)
	}
	if _, err := cat.Derive("cthulhu-wars", map[string]any{"factions": []any{"gc", "cc", "bg", "ys"}, "igoo-yig": "yes"}); err == nil {
		t.Error("a option that is not a bool was accepted")
	}
}

// doomPhase is a position in a Doom phase where who decides.
func doomPhase(tb *table, who string, p xp) *state.BoardState {
	p.phase, p.stage = "doom", "doom"
	inner := p.edit
	p.edit = func(d *cthulhuwars.Data) {
		d.Doomx = &cthulhuwars.DoomState{Order: []int{tb.seat(who)}}
		d.Round = 3
		if inner != nil {
			inner(d)
		}
	}
	return tb.build(p)
}

func TestLoyaltyCardsAreBoughtOnePerDoomPhaseAndOnlyFromPacksInTheBox(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"dreamlands": true})
	st := doomPhase(tb, "gc", xp{
		units: []xu{{who: "gc", typ: "cultist", area: "south-pacific", n: 2}},
		gates: map[string]string{"south-pacific": "gc"},
		power: map[string]int{"gc": 5}, doom: map[string]int{"gc": 6},
		pool: map[string]int{"gc|gnorri": 0, "gc|moonbeast": 0, "gc|shantak": 0},
	})
	got := tb.can(st, "gc")
	for _, want := range []string{"buy:gnorri@south-pacific", "buy:shantak@south-pacific", "buy:moonbeast"} {
		if !has(got, want) {
			t.Errorf("%s is not offered: %v", want, got)
		}
	}
	// Nothing of the packs that are not in the box, and no Azathoth faction.
	for _, no := range []string{"buy:voonith@south-pacific", "buy:hound-of-tindalos@south-pacific", "buy:wamp", "buy:star-vampire@south-pacific", "buy:elder-thing@south-pacific"} {
		if has(got, no) {
			t.Errorf("%s is offered but its pack is not in the game", no)
		}
	}
	st = tb.as(st, "gc", "buy:gnorri@south-pacific")
	d := tb.data(st)
	if d.Doom[1] != 4 {
		t.Errorf("the card costs 2 Doom: %d", d.Doom[1])
	}
	if tb.n(st, "gc", "gnorri", "south-pacific") != 1 || tb.pool(st, "gc", "gnorri") != 2 {
		t.Errorf("one Gnorri appears at the Gate and two wait in the pool")
	}
	// One Loyalty Card in a Doom phase.
	if got := tb.can(st, "gc"); hasPrefix(got, "buy:") {
		t.Errorf("a second card in the same Doom phase: %v", got)
	}
	tb.valid(st)
}

func TestATerrorCostsDoomAndPowerAndAppearsAtAnyGate(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"beyond": true})
	build := func(power int) *state.BoardState {
		return doomPhase(tb, "gc", xp{
			units: []xu{{who: "gc", typ: "cultist", area: "south-pacific"}, {who: "cc", typ: "cultist", area: "asia"}},
			gates: map[string]string{"south-pacific": "gc", "asia": "cc", "europe": "-"},
			power: map[string]int{"gc": power}, doom: map[string]int{"gc": 6},
		})
	}
	if got := tb.can(build(1), "gc"); hasPrefix(got, "buy:hound") {
		t.Errorf("1 Power cannot buy a Terror: %v", got)
	}
	st := build(3)
	got := tb.can(st, "gc")
	for _, want := range []string{"buy:hound-of-tindalos@south-pacific", "buy:hound-of-tindalos@asia", "buy:hound-of-tindalos@europe"} {
		if !has(got, want) {
			t.Errorf("%s is not offered (a Gate of any kind will do): %v", want, got)
		}
	}
	st = tb.as(st, "gc", "buy:hound-of-tindalos@asia")
	d := tb.data(st)
	if d.Power[1] != 1 || d.Doom[1] != 4 {
		t.Errorf("a Terror costs 2 Power and 2 Doom: Power %d Doom %d", d.Power[1], d.Doom[1])
	}
	if tb.n(st, "gc", "hound-of-tindalos", "asia") != 1 {
		t.Error("the Hound stands at the enemy Gate")
	}
	tb.valid(st)
}

func TestACardOfAnotherKindIsNotBoughtTwiceInOneDoomPhase(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"beyond": true, "dreamlands": true})
	st := doomPhase(tb, "gc", xp{
		units: []xu{{who: "gc", typ: "cultist", area: "south-pacific"}},
		gates: map[string]string{"south-pacific": "gc"},
		power: map[string]int{"gc": 6}, doom: map[string]int{"gc": 9},
	})
	st = tb.as(st, "gc", "buy:hound-of-tindalos@south-pacific")
	// A Terror and a Neutral Monster are both Loyalty Cards: one a Doom phase.
	if got := tb.can(st, "gc"); hasPrefix(got, "buy:") {
		t.Errorf("a Monster card after the Terror card: %v", got)
	}
}

func TestGnorriEarnExtraDoomWhenTwoOrThreeAreInPlay(t *testing.T) {
	for _, c := range []struct{ n, want int }{{1, 0}, {2, 1}, {3, 2}} {
		tb := cardTable(t, fourPlus, map[string]any{"dreamlands": true})
		st := tb.build(xp{
			units: []xu{{who: "gc", typ: "cultist", area: "south-pacific"}, {who: "gc", typ: "gnorri", area: "south-pacific", n: c.n}},
			gates: map[string]string{"south-pacific": "gc"},
			stage: "direction", phase: "action",
			edit: func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdGnorri) },
			pool: map[string]int{"gc|gnorri": 3 - c.n},
		})
		st = tb.as(st, "gc", "cw")
		d := tb.data(st)
		if d.Phase != "doom" {
			t.Fatalf("%d Gnorri: phase %q", c.n, d.Phase)
		}
		if d.Doom[1] != 1+c.want {
			t.Errorf("%d Gnorri earn %d extra Doom: Doom %d, want %d", c.n, c.want, d.Doom[1], 1+c.want)
		}
	}
}

func TestAShantakReachesAnyAreaAndCarriesACultistForNothing(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"dreamlands": true})
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "cultist", area: "south-pacific", n: 2}, {who: "gc", typ: "shantak", area: "south-pacific"}},
		gates: map[string]string{"south-pacific": "gc"},
		power: map[string]int{"gc": 5},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdShantak) },
		pool:  map[string]int{"gc|shantak": 1},
	})
	if got := tb.can(st, "gc"); !has(got, "go:shantak@south-pacific>europe") || !has(got, "go:shantak@south-pacific>new-zealand") {
		t.Fatalf("a Shantak reaches any area: %v", got)
	}
	// A Cultist cannot go to Europe from there: the Shantak takes it.
	if got := tb.can(st, "gc"); has(got, "go:cultist@south-pacific>europe") {
		t.Fatalf("a Cultist on its own does not reach Europe: %v", got)
	}
	st = tb.as(st, "gc", "go:shantak@south-pacific>europe")
	if tb.power(st, "gc") != 4 {
		t.Errorf("the Shantak's step costs 1 Power: %d", tb.power(st, "gc"))
	}
	if got := tb.can(st, "gc"); !has(got, "go:cultist@south-pacific>europe") {
		t.Fatalf("the Cultist may ride along: %v", got)
	}
	st = tb.as(st, "gc", "go:cultist@south-pacific>europe")
	if tb.power(st, "gc") != 4 {
		t.Errorf("the ride is free: Power %d", tb.power(st, "gc"))
	}
	if tb.n(st, "gc", "cultist", "europe") != 1 || tb.n(st, "gc", "shantak", "europe") != 1 {
		t.Error("both stand in Europe")
	}
	// One Shantak carries one Cultist: the next one walks (and cannot reach Europe).
	if got := tb.can(st, "gc"); has(got, "go:cultist@south-pacific>europe") {
		t.Errorf("a second ride: %v", got)
	}
	tb.valid(st)
}

func TestViciousVoonithsMakeUpTheKillsTheDiceDidNotGive(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"beyond": true})
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "voonith", area: "asia", n: 2}, {who: "cc", typ: "cultist", area: "asia", n: 3}},
		power: map[string]int{"gc": 5},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdVoonith) },
		pool:  map[string]int{"gc|voonith": 0},
		rng:   seedDice(t, 1, 2),
	})
	st = tb.as(st, "gc", "battle@asia/cc")
	if n := tb.n(st, "cc", "cultist", "asia"); n != 1 {
		t.Fatalf("two Vooniths kill two Cultists whatever the dice: %d left", n)
	}
	// With a Kill rolled, one more makes the two.
	st = tb.build(xp{
		units: []xu{{who: "gc", typ: "voonith", area: "asia", n: 2}, {who: "cc", typ: "cultist", area: "asia", n: 3}},
		power: map[string]int{"gc": 5},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdVoonith) },
		pool:  map[string]int{"gc|voonith": 0},
		rng:   seedDice(t, 6, 1),
	})
	st = tb.as(st, "gc", "battle@asia/cc")
	if n := tb.n(st, "cc", "cultist", "asia"); n != 1 {
		t.Fatalf("one rolled Kill and one made up: %d left", n)
	}
}

func TestAMoonbeastLiesOnAnEnemySpellbookAndFreesItAtTheNextDoomPhase(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"dreamlands": true})
	frenzy := 1 // Black Goat's Frenzy: its Cultists have 1 Combat
	base := xp{
		units: []xu{
			{who: "gc", typ: "cultist", area: "south-pacific"}, {who: "bg", typ: "cultist", area: "south-pacific"},
		},
		gates: map[string]string{"south-pacific": "gc"},
		power: map[string]int{"gc": 6, "bg": 6},
		books: map[string][]int{"bg": {frenzy, 0}},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdMoonbeast) },
		pool:  map[string]int{"gc|moonbeast": 4},
	}
	// Frenzy lets Black Goat's Cultists fight.
	if got := tb.can(tb.build(func() xp { p := base; p.turn = "bg"; return p }()), "bg"); !has(got, "battle@south-pacific/gc") {
		t.Fatalf("Frenzy gives the Cultists a die: %v", got)
	}
	st := tb.build(base)
	st = tb.as(st, "gc", "summon:moonbeast@south-pacific")
	d := tb.data(st)
	if d.Stage != "prompt" || d.Prompts[0].Kind != "moon" {
		t.Fatalf("the Moonbeast asks where it lies: stage %q %+v", d.Stage, d.Prompts)
	}
	got := tb.can(st, "gc")
	if !has(got, "moon:frenzy/bg") || !has(got, "moon:blood/bg") {
		t.Fatalf("the Moonbeast may lie on any spellbook an enemy holds: %v", got)
	}
	tb.refuse(st, "gc", "moon:the-thousand-young/bg") // not held
	st = tb.as(st, "gc", "moon:frenzy/bg")
	d = tb.data(st)
	if len(d.Moon) != 1 || d.Moon[0].Owner != 1 || d.Moon[0].Victim != 3 {
		t.Fatalf("the Moonbeast lies on Frenzy: %+v", d.Moon)
	}
	if tb.pool(st, "gc", "moonbeast") != 3 || tb.n(st, "gc", "moonbeast", "south-pacific") != 0 {
		t.Error("the Moonbeast is on the spellbook, not on the map")
	}
	// The book still counts, but Black Goat cannot use it.
	if d.Books[3]&(1<<uint(frenzy)) == 0 {
		t.Error("the spellbook is still on the faction card")
	}
	st = tb.build(func() xp {
		p := base
		p.turn = "bg"
		p.edit = func(d *cthulhuwars.Data) {
			holds(d, tb, "gc", cdMoonbeast)
			d.Moon = []cthulhuwars.MoonState{{Owner: 1, Victim: 3, Book: frenzy}}
		}
		p.pool = map[string]int{"gc|moonbeast": 3}
		return p
	}())
	if got := tb.can(st, "bg"); has(got, "battle@south-pacific/gc") {
		t.Errorf("Frenzy cannot be used while a Moonbeast lies on it: %v", got)
	}
	// The victim may pay 1 Doom, at any time of its turn, to free it.
	st2 := tb.build(func() xp {
		p := base
		p.turn = "bg"
		p.doom = map[string]int{"bg": 3}
		p.edit = func(d *cthulhuwars.Data) {
			holds(d, tb, "gc", cdMoonbeast)
			d.Moon = []cthulhuwars.MoonState{{Owner: 1, Victim: 3, Book: frenzy}}
		}
		p.pool = map[string]int{"gc|moonbeast": 3}
		return p
	}())
	if got := tb.can(st2, "bg"); !has(got, "ransom:frenzy") {
		t.Fatalf("Black Goat may pay 1 Doom: %v", got)
	}
	st2 = tb.as(st2, "bg", "ransom:frenzy")
	d = tb.data(st2)
	if d.Doom[3] != 2 || len(d.Moon) != 0 {
		t.Errorf("1 Doom frees the book: Doom %d, Moon %+v", d.Doom[3], d.Moon)
	}
	if tb.n(st2, "gc", "moonbeast", "south-pacific") != 1 {
		t.Error("the freed Moonbeast comes to the owner's only Gate")
	}
	// Without paying, it goes home at the start of the next Doom phase.
	st = tb.build(func() xp {
		p := base
		p.stage, p.phase = "direction", "action"
		p.edit = func(d *cthulhuwars.Data) {
			holds(d, tb, "gc", cdMoonbeast)
			d.Moon = []cthulhuwars.MoonState{{Owner: 1, Victim: 3, Book: frenzy}}
		}
		p.pool = map[string]int{"gc|moonbeast": 3}
		return p
	}())
	st = tb.as(st, "gc", "cw")
	d = tb.data(st)
	if len(d.Moon) != 0 || tb.n(st, "gc", "moonbeast", "south-pacific") != 1 {
		t.Errorf("the Moonbeast returns in the Doom phase: %+v", d.Moon)
	}
	tb.valid(st)
}

func TestWampsArePlacedByAnEnemyWhereNoGateStands(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"beyond": true})
	st := doomPhase(tb, "gc", xp{
		units: []xu{{who: "gc", typ: "cultist", area: "south-pacific"}},
		gates: map[string]string{"south-pacific": "gc", "asia": "cc"},
		power: map[string]int{"gc": 5}, doom: map[string]int{"gc": 6},
	})
	st = tb.as(st, "gc", "buy:wamp")
	d := tb.data(st)
	if d.Stage != "prompt" || d.Prompts[0].Kind != "crypt-pick" {
		t.Fatalf("the buyer chooses the enemy: %q %+v", d.Stage, d.Prompts)
	}
	st = tb.as(st, "gc", "crypt:cc")
	d = tb.data(st)
	if d.Prompts[0].Kind != "crypt" || d.Prompts[0].Seat != 2 || d.Prompts[0].Left != 4 {
		t.Fatalf("Crypt Dweller: %+v", d.Prompts)
	}
	got := tb.can(st, "cc")
	if has(got, "crypt@asia") || has(got, "crypt@south-pacific") || !has(got, "crypt@europe") {
		t.Fatalf("Wamps go where no Gate stands: %v", got)
	}
	for _, a := range []string{"europe", "europe", "africa", "australia"} {
		st = tb.as(st, "cc", "crypt@"+a)
	}
	if tb.n(st, "gc", "wamp", "europe") != 2 || tb.n(st, "gc", "wamp", "africa") != 1 || tb.n(st, "gc", "wamp", "australia") != 1 {
		t.Error("the four Wamps belong to the buyer, wherever the enemy put them")
	}
	if tb.pool(st, "gc", "wamp") != 0 {
		t.Errorf("none is left in the pool: %d", tb.pool(st, "gc", "wamp"))
	}
	tb.valid(st)
}

func TestTheHoundOfTindalosHopsBetweenGatesWithTheOthersAndCannotBeKilled(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"beyond": true})
	base := xp{
		units: []xu{
			{who: "gc", typ: "hound-of-tindalos", area: "south-pacific"}, {who: "gc", typ: "cultist", area: "south-pacific", n: 2},
		},
		gates: map[string]string{"south-pacific": "gc", "asia": "cc", "europe": "-"},
		power: map[string]int{"gc": 5, "cc": 5},
		edit:  func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdHound) },
		pool:  map[string]int{"gc|hound-of-tindalos": 0},
	}
	st := tb.build(base)
	// It cannot move by itself…
	if got := tb.can(st, "gc"); hasPrefix(got, "go:hound-of-tindalos") || hasPrefix(got, "leave:hound-of-tindalos") {
		t.Fatalf("the Hound moves only with the others: %v", got)
	}
	// …but moves for free when another unit does, from Gate to Gate.
	st = tb.as(st, "gc", "go:cultist@south-pacific>antarctica")
	got := tb.can(st, "gc")
	if !has(got, "go:hound-of-tindalos@south-pacific>asia") || !has(got, "go:hound-of-tindalos@south-pacific>europe") {
		t.Fatalf("the Hound hops to any Gate: %v", got)
	}
	if has(got, "go:hound-of-tindalos@south-pacific>antarctica") {
		t.Fatalf("not to an area without a Gate: %v", got)
	}
	st = tb.as(st, "gc", "go:hound-of-tindalos@south-pacific>asia")
	if tb.power(st, "gc") != 4 {
		t.Errorf("the hop is free: Power %d", tb.power(st, "gc"))
	}
	if tb.n(st, "gc", "hound-of-tindalos", "asia") != 1 {
		t.Error("the Hound is in Asia")
	}
	// Angles of Time: no Kill can be assigned to it; the Cultist takes it.
	st = tb.build(func() xp {
		p := base
		p.units = []xu{
			{who: "gc", typ: "hound-of-tindalos", area: "south-pacific"}, {who: "gc", typ: "cultist", area: "south-pacific"},
			{who: "cc", typ: "hunting-horror", area: "south-pacific", n: 2},
		}
		p.turn = "cc"
		p.rng = seedDice(t, 6, 6, 6, 6)
		return p
	}())
	st = tb.as(st, "cc", "battle@south-pacific/gc")
	if tb.n(st, "gc", "hound-of-tindalos", "south-pacific") != 1 {
		t.Error("the Hound survives any number of Kills")
	}
	if tb.n(st, "gc", "cultist", "south-pacific") != 0 {
		t.Error("the Cultist took the one Kill it could")
	}
	// Angles of Time: a Hound in an area without a Gate is eliminated at once.
	st = tb.build(func() xp {
		p := base
		p.gates = map[string]string{"asia": "cc"}
		return p
	}())
	st = tb.as(st, "gc", "recruit@south-pacific")
	if tb.n(st, "gc", "hound-of-tindalos", "south-pacific") != 0 {
		t.Error("a Hound with no Gate under it is eliminated")
	}
	tb.valid(st)
}

func TestATerrorGuardsCultistsLikeAMonsterAndShrivelingCannotTouchIt(t *testing.T) {
	tb := cardTable(t, fourPlus, map[string]any{"beyond": true})
	p := xp{
		units: []xu{
			{who: "gc", typ: "hound-of-tindalos", area: "asia"}, {who: "gc", typ: "cultist", area: "asia"},
			{who: "cc", typ: "nightgaunt", area: "asia"},
		},
		gates: map[string]string{"asia": "gc"},
		power: map[string]int{"cc": 5}, turn: "cc",
		edit: func(d *cthulhuwars.Data) { holds(d, tb, "gc", cdHound) },
		pool: map[string]int{"gc|hound-of-tindalos": 0},
	}
	if got := tb.can(tb.build(p), "cc"); hasPrefix(got, "capture@asia/gc") {
		t.Errorf("a Monster cannot capture Cultists a Terror guards: %v", got)
	}
	p.units = []xu{{who: "gc", typ: "cultist", area: "asia"}, {who: "cc", typ: "nightgaunt", area: "asia"}}
	if got := tb.can(tb.build(p), "cc"); !hasPrefix(got, "capture@asia/gc") {
		t.Errorf("the Cultist alone can be captured: %v", got)
	}
}
