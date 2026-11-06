package cthulhuwars_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The later packs: Underworld, Cosmic Terrors, Cats, the Satyr and the
// Independents of Great Old One Packs 2 to 4 and the Campbell Horrors.

// Card numbers of the later packs, bits of Data.Loyal.
const (
	cdGhast = iota + 15
	cdLeng
	cdGug
	cdYith
	cdDhole
	cdQuachil
	cdMercury
	cdVenus
	cdAsteroid
	cdNeptune
	cdSatyr
	cdAtlach
	cdBokrug
	cdDagon
	cdGhatan
	cdGobogeg
	cdByatis
	cdNyogtha
	cdTulzscha
	cdGlaaki
	cdDaoloth
	cdYgolonac
	cdHagarg
)

// Bits of Data.IBooks of the later Independents, after Pack 1's five.
const (
	ibAtlach = iota + 5
	ibBokrug
	ibDagon
	ibGhatan
	ibGobogeg
	ibByatis
	ibNyogtha
	ibTulzscha
	ibGlaaki
	ibDaoloth
	ibYgolonac
	ibHagarg
)

func later(t testing.TB) *table { return cardTable(t, fourPlus, allCards) }

func TestAGugGuardsButDoesNotCapture(t *testing.T) {
	tb := later(t)
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "gug", area: "africa"}, {who: "cc", typ: "cultist", area: "africa"}},
		power: map[string]int{"gc": 4},
		pool:  map[string]int{"gc|gug": 1},
		edit:  func(d *cthulhuwars.Data) { d.Loyal[1] |= 1 << cdGug },
	})
	if got := tb.can(st, "gc"); has(got, "capture@africa/cc") {
		t.Errorf("a Gug captured: %v", got)
	}
}

func TestTheGreatRaceCapturesPastAGuard(t *testing.T) {
	tb := later(t)
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "great-race-of-yith", area: "africa"}, {who: "cc", typ: "cultist", area: "africa"}, {who: "cc", typ: "nyarlathotep", area: "africa"}},
		power: map[string]int{"gc": 4},
		edit:  func(d *cthulhuwars.Data) { d.Loyal[1] |= 1 << cdYith },
	})
	if got := tb.can(st, "gc"); !has(got, "capture@africa/cc") {
		t.Errorf("Possession does not capture past a Great Old One: %v", got)
	}
}

func TestACatFromVenusSendsTheCaptiveHomeForAPower(t *testing.T) {
	tb := later(t)
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "cat-from-venus", area: "africa"}, {who: "cc", typ: "cultist", area: "africa"}},
		power: map[string]int{"gc": 4},
		pool:  map[string]int{"gc|cat-from-venus": 1},
		edit:  func(d *cthulhuwars.Data) { d.Loyal[1] |= 1 << cdVenus },
	})
	st = tb.as(st, "gc", "capture@africa/cc")
	d := tb.data(st)
	if d.Captured[1][2] != 0 || d.Power[1] != 4 {
		t.Errorf("captured %d, Power %d: the capture costs 1 and Honeymoon gives 1", d.Captured[1][2], d.Power[1])
	}
}

func TestAtlachNachaWinsWithSixWebs(t *testing.T) {
	tb := later(t)
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "atlach-nacha", area: "africa"}},
		power: map[string]int{"gc": 4},
		edit: func(d *cthulhuwars.Data) {
			d.Loyal[1] |= 1 << cdAtlach
			d.Webs = []string{"asia", "europe", "antarctica", "australia", "south-pacific"}
		},
	})
	if got := tb.can(st, "gc"); has(got, "card:cosmic-web") || !has(got, "card:spin") {
		t.Fatalf("five Webs: %v", got)
	}
	st = tb.as(st, "gc", "card:spin")
	d := tb.data(st)
	if len(d.Webs) != 6 || d.IBooks&(1<<ibAtlach) == 0 {
		t.Fatalf("webs %v, books %b", d.Webs, d.IBooks)
	}
}

func TestByatisStaysPut(t *testing.T) {
	tb := later(t)
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "byatis", area: "africa"}},
		power: map[string]int{"gc": 4},
		edit:  func(d *cthulhuwars.Data) { d.Loyal[1] |= 1 << cdByatis },
	})
	for _, mv := range tb.can(st, "gc") {
		if len(mv) > 8 && mv[:9] == "go:byatis" {
			t.Errorf("Byatis moves: %s", mv)
		}
	}
}

func TestBokrugIsHandedOverWithHisBook(t *testing.T) {
	tb := later(t)
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "bokrug", area: "africa"}},
		power: map[string]int{"gc": 4},
		edit:  func(d *cthulhuwars.Data) { d.Loyal[1] |= 1 << cdBokrug },
	})
	st = tb.as(st, "gc", "card:give-bokrug/cc")
	d := tb.data(st)
	if d.Loyal[1]&(1<<cdBokrug) != 0 || d.Loyal[2]&(1<<cdBokrug) == 0 || d.IBooks&(1<<ibBokrug) == 0 {
		t.Errorf("loyal %b %b books %b", d.Loyal[1], d.Loyal[2], d.IBooks)
	}
	if tb.n(st, "cc", "bokrug", "africa") != 1 {
		t.Error("the figure changes hands too")
	}
	if err := state.Validate(tb.g, st); err != nil {
		t.Error(err)
	}
}

func TestMummifiedAcolytesDoNotMove(t *testing.T) {
	tb := later(t)
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "ghatanothoa", area: "africa"}, {who: "cc", typ: "cultist", area: "africa"}},
		power: map[string]int{"gc": 4, "cc": 4},
		edit:  func(d *cthulhuwars.Data) { d.Loyal[1] |= 1 << cdGhatan },
	})
	st = tb.as(st, "gc", "card:mummify")
	if n := tb.data(st).Mummies["africa|2"]; n != 1 {
		t.Fatalf("mummies: %v", tb.data(st).Mummies)
	}
}

func TestNyogthaIsTwoFiguresAndHisCardGoesWithTheSecond(t *testing.T) {
	tb := later(t)
	st := tb.build(xp{
		units: []xu{{who: "gc", typ: "cthulhu", area: "south-pacific"}, {who: "gc", typ: "cultist", area: "south-pacific"}},
		gates: map[string]string{"south-pacific": "gc"},
		power: map[string]int{"gc": 10},
	})
	st = tb.as(st, "gc", "awaken:nyogtha@south-pacific")
	if n := tb.n(st, "gc", "nyogtha", "south-pacific"); n != 2 {
		t.Errorf("%d Nyogthas awake", n)
	}
	if err := state.Validate(tb.g, st); err != nil {
		t.Error(err)
	}
}

func TestGobogegWaitsForSixSpellbooksAndPaysEveryAwakening(t *testing.T) {
	tb := later(t)
	base := xp{
		units: []xu{{who: "gc", typ: "cthulhu", area: "south-pacific"}, {who: "gc", typ: "cultist", area: "south-pacific"}},
		gates: map[string]string{"south-pacific": "gc"},
		power: map[string]int{"gc": 10},
	}
	if got := tb.can(tb.build(base), "gc"); has(got, "awaken:gobogeg@south-pacific") {
		t.Errorf("no cult has six books: %v", got)
	}
	base.books = map[string][]int{"cc": {0, 1, 2, 3, 4, 5}}
	st := tb.build(base)
	st = tb.as(st, "gc", "awaken:gobogeg@south-pacific")
	if p := tb.data(st).Power[1]; p != 16 {
		t.Errorf("Power %d: 10, and 6 from the Book of Law", p)
	}
}

// arranged is a table where the four cults between them already hold every later
// card, with the figures on the map, half the spellbooks earned and Power to use.
func arranged(tb *table, seed int) *state.BoardState {
	type card struct {
		typ string
		bit uint
		n   int
	}
	all := []card{
		{"ghast", cdGhast, 4}, {"leng-spider", cdLeng, 3}, {"gug", cdGug, 2}, {"great-race-of-yith", cdYith, 1},
		{"dhole", cdDhole, 1}, {"quachil-uttaus", cdQuachil, 1}, {"cat-from-mercury", cdMercury, 2}, {"cat-from-venus", cdVenus, 2},
		{"asteroid-cat", cdAsteroid, 2}, {"cat-from-neptune", cdNeptune, 1}, {"satyr", cdSatyr, 3},
		{"atlach-nacha", cdAtlach, 1}, {"bokrug", cdBokrug, 1}, {"father-dagon", cdDagon, 1}, {"ghatanothoa", cdGhatan, 1},
		{"gobogeg", cdGobogeg, 1}, {"byatis", cdByatis, 1}, {"nyogtha", cdNyogtha, 2}, {"tulzscha", cdTulzscha, 1},
		{"glaaki", cdGlaaki, 1}, {"daoloth", cdDaoloth, 1}, {"ygolonac", cdYgolonac, 1}, {"hagarg-ryonis", cdHagarg, 1},
	}
	keys := []string{"gc", "cc", "bg", "ys"}
	areas := []string{"africa", "europe", "asia", "antarctica", "south-pacific", "north-atlantic", "indian-ocean", "south-atlantic", "australia", "arctic-ocean"}
	p := xp{
		units: []xu{
			{who: "gc", typ: "cultist", area: "south-pacific", n: 3}, {who: "cc", typ: "cultist", area: "asia", n: 3},
			{who: "bg", typ: "cultist", area: "africa", n: 3}, {who: "ys", typ: "cultist", area: "europe", n: 3},
		},
		gates: map[string]string{"south-pacific": "gc", "asia": "cc", "africa": "bg", "europe": "ys"},
		power: map[string]int{"gc": 14, "cc": 14, "bg": 14, "ys": 14},
		doom:  map[string]int{"gc": 6, "cc": 6, "bg": 6, "ys": 6},
		pool:  map[string]int{},
		rng:   uint64(seed*31 + 7),
	}
	loyal := [6]uint64{}
	for i, c := range all {
		k := (i + seed) % 4
		loyal[k+1] |= 1 << c.bit
		onMap := c.n
		if c.typ == "ghast" || c.typ == "satyr" || c.typ == "leng-spider" {
			onMap = 1
		}
		p.units = append(p.units, xu{who: keys[k], typ: c.typ, area: areas[(i+seed)%len(areas)], n: onMap})
		p.pool[keys[k]+"|"+c.typ] = c.n - onMap
	}
	p.edit = func(d *cthulhuwars.Data) {
		d.Loyal = [6]uint64(loyal)
		if seed%2 == 1 {
			d.IBooks = ^uint32(0)
			d.IBooks &^= 1 << ibAtlach
		}
	}
	return tb.build(p)
}

// From a table where every later card is in play, random decisions reach the
// abilities, each legal move is in the action space and reads back from its
// notation, and the positions stay valid.
func TestTheLaterCardsPlayFromAnArrangedTable(t *testing.T) {
	tb := later(t)
	g := tb.g
	space := state.ActionSpaceOf(g)
	seen := map[string]int{}
	seeds, plies := 40, 1500
	if testing.Short() {
		seeds, plies = 12, 800
	}
	for seed := 0; seed < seeds; seed++ {
		rng := rand.New(rand.NewSource(int64(seed)))
		st := arranged(tb, seed)
		if err := state.Validate(g, st); err != nil {
			t.Fatalf("seed %d: the arranged table: %v", seed, err)
		}
		seat := state.FirstSeat(g)
		for ply := 0; ply < plies; ply++ {
			legal := state.LegalSelections(g, st, seat)
			if len(legal) == 0 {
				t.Fatalf("seed %d ply %d: no legal moves for seat %d\n%s", seed, ply, seat, state.RenderText(g, st))
			}
			// Half the time prefer the ability on offer, so that they all come up.
			mv := legal[rng.Intn(len(legal))]
			if rng.Intn(2) == 0 {
				var cards []state.Selection
				for _, m := range legal {
					if m.Action == "card" {
						cards = append(cards, m)
					}
				}
				if len(cards) > 0 {
					mv = cards[rng.Intn(len(cards))]
				}
			}
			for _, m := range legal {
				text := state.FormatMove(g, m)
				if _, ok := space.ID(text); !ok {
					t.Fatalf("seed %d ply %d: %s is not in the action space", seed, ply, text)
				}
				back, err := state.ParseMove(g, text)
				if err != nil || state.FormatMove(g, back) != text {
					t.Fatalf("seed %d ply %d: %s does not read back: %v", seed, ply, text, err)
				}
			}
			if mv.Action == "card" {
				id, _, _ := strings.Cut(mv.Piece, ".")
				seen[id]++
			}
			res, err := state.Apply(g, st, seat, mv)
			if err != nil {
				t.Fatalf("seed %d ply %d: %s rejected: %v", seed, ply, state.FormatMove(g, mv), err)
			}
			if err := state.Validate(g, res.NewState); err != nil {
				t.Fatalf("seed %d ply %d after %s: %v\n%s", seed, ply, state.FormatMove(g, mv), err, state.RenderText(g, res.NewState))
			}
			if res.GameEnded {
				break
			}
			st, seat = res.NewState, res.NextImpulseSeat
		}
	}
	t.Logf("abilities reached: %v", seen)
	for _, id := range []string{"spin", "give-bokrug", "tsunami", "mummify", "ghatan-pay", "forget", "nightmare", "tulzscha-gift", "lust", "innsmouth", "subvert", "sarnath-unit", "ceremony"} {
		if seen[id] == 0 {
			t.Errorf("%s was never played", id)
		}
	}
}

// primeval is a four-cult table's worth of the Primeval Map's three-player layout.
func primevalGame(t testing.TB, base string, keys []string) *catalog.CompiledGame {
	return packTable(t, base, keys, map[string]any{})
}

func TestRandomGamesOnThePrimevalMap(t *testing.T) {
	for _, c := range []struct {
		base string
		keys []string
	}{
		{"cthulhu-wars-primeval-5p", []string{"gc", "cc", "bg", "ys", "ww"}},
		{"cthulhu-wars-primeval-3p", []string{"gc", "cc", "bg"}},
		{"cthulhu-wars-primeval-5p", []string{"sl", "ow", "tt", "gc", "ww"}},
	} {
		g := primevalGame(t, c.base, c.keys)
		glaciers := 0
		for seed := int64(1); seed <= 6; seed++ {
			st, _, _ := randomGame(t, g, seed, 9000, func(ply int, st *state.BoardState, seat int, mv state.Selection) {
				if err := state.Validate(g, st); err != nil {
					t.Fatalf("%s seed %d ply %d: %v", g.Definition.ID, seed, ply, err)
				}
			})
			if n := len(primevalData(t, st).Glaciers); n > glaciers {
				glaciers = n
			}
		}
		if glaciers < len(c.keys) {
			t.Errorf("%s: only %d Glaciers placed in any game", g.Definition.ID, glaciers)
		}
	}
}

func TestAGlacierAbandonsAGateForGood(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get("cthulhu-wars-primeval-3p")
	if !ok {
		t.Fatal("no Primeval layout")
	}
	st, err := state.InitialStateSeeded(g, 5)
	if err != nil {
		t.Fatal(err)
	}
	d := primevalData(t, st)
	if len(d.Glaciers) != 0 {
		t.Fatalf("Glaciers at the start: %v", d.Glaciers)
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
	// Run to the end of the first Doom phase by random play.
	rng := rand.New(rand.NewSource(1))
	seat := state.FirstSeat(g)
	for ply := 0; ply < 4000; ply++ {
		legal := state.LegalSelections(g, st, seat)
		res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
		if err != nil {
			t.Fatal(err)
		}
		st, seat = res.NewState, res.NextImpulseSeat
		d = primevalData(t, st)
		if len(d.Glaciers) > 0 {
			break
		}
	}
	if len(d.Glaciers) != 3 {
		t.Fatalf("the first Glaciers: %v", d.Glaciers)
	}
	for _, id := range d.Glaciers {
		if ctl, there := d.Gates[id]; there && ctl != 0 {
			t.Errorf("the Gate under the Glacier in %s is held by seat %d", id, ctl)
		}
	}
}

func primevalData(t testing.TB, st *state.BoardState) *cthulhuwars.Data { return data(t, st) }

// --- The Ancients -------------------------------------------------------------------------------

func ancients(t testing.TB) *table {
	return cardTable(t, []string{"an", "gc", "cc", "bg"}, map[string]any{})
}

func TestTheAncientsBeginAfterTheOthersInAnAreaWithNoStartGlyph(t *testing.T) {
	g := packTable(t, "cthulhu-wars", []string{"an", "gc", "cc", "bg"}, map[string]any{})
	st, err := state.InitialStateSeeded(g, 3)
	if err != nil {
		t.Fatal(err)
	}
	legal := state.LegalSelections(g, st, state.FirstSeat(g))
	if len(legal) == 0 || legal[0].Action != "start" {
		t.Fatalf("the first decision: %v", legal)
	}
	for _, mv := range legal {
		switch string(mv.To) {
		case "south-pacific", "asia", "africa", "europe":
			t.Errorf("%s holds a start glyph", mv.To)
		}
	}
}

func TestACathedralCostsOneAloneAndEarnsTheBookOfItsGlyph(t *testing.T) {
	tb := ancients(t)
	st := tb.build(xp{
		units: []xu{{who: "an", typ: "cultist", area: "africa", n: 2}, {who: "an", typ: "cultist", area: "north-atlantic", n: 2}},
		gates: map[string]string{"africa": "an"},
		power: map[string]int{"an": 8},
	})
	st = tb.as(st, "an", "card:cathedral@africa") // Africa has the Chevron
	d := tb.data(st)
	if d.Power[tb.seat("an")] != 7 {
		t.Errorf("Power %d: a Cathedral on its own costs 1", d.Power[tb.seat("an")])
	}
	if d.Reqs[tb.seat("an")]&(1<<2) == 0 {
		t.Errorf("the Chevron requirement is not met: %b", d.Reqs[tb.seat("an")])
	}
}

func TestACathedralNextToAnotherCostsThree(t *testing.T) {
	tb := ancients(t)
	st := tb.build(xp{
		units: []xu{{who: "an", typ: "cultist", area: "north-atlantic", n: 2}},
		power: map[string]int{"an": 8},
		edit:  func(d *cthulhuwars.Data) { d.Cathedrals = []string{"africa"} },
	})
	before := tb.data(st).Power[tb.seat("an")]
	st = tb.as(st, "an", "card:cathedral@north-atlantic")
	if got := tb.data(st).Power[tb.seat("an")]; before-got != 3 {
		t.Errorf("paid %d", before-got)
	}
}

func TestFestivalAndBrainlessChangeTheCosts(t *testing.T) {
	tb := ancients(t)
	st := tb.build(xp{
		units: []xu{{who: "an", typ: "cultist", area: "africa"}},
		gates: map[string]string{"africa": "an"},
		power: map[string]int{"an": 1},
		books: map[string][]int{"an": {0, 3}},
	})
	got := tb.can(st, "an")
	for _, want := range []string{"summon:un-men@africa", "summon:reanimated@africa"} {
		if !has(got, want) {
			t.Errorf("%s is not on offer with 1 Power: %v", want, got)
		}
	}
	if has(got, "summon:yothan@africa") {
		t.Errorf("a Yothan costs 6 without Extinction: %v", got)
	}
}

func TestBrainlessReanimatedDoNotMoveAlone(t *testing.T) {
	tb := ancients(t)
	st := tb.build(xp{
		units: []xu{{who: "an", typ: "reanimated", area: "africa"}, {who: "gc", typ: "cultist", area: "africa"}},
		power: map[string]int{"an": 6},
		books: map[string][]int{"an": {0}},
	})
	got := tb.can(st, "an")
	for _, mv := range got {
		if strings.HasPrefix(mv, "go:reanimated") || strings.HasPrefix(mv, "battle") || strings.HasPrefix(mv, "capture") {
			t.Errorf("a Reanimated alone did %s", mv)
		}
	}
}

func TestRandomGamesWithTheAncients(t *testing.T) {
	for _, c := range []struct {
		base string
		keys []string
	}{
		{"cthulhu-wars", []string{"an", "gc", "cc", "bg"}},
		{"cthulhu-wars-5p", []string{"an", "tt", "ww", "ow", "sl"}},
		{"cthulhu-wars-primeval-3p", []string{"an", "ys", "cc"}},
	} {
		g := packTable(t, c.base, c.keys, allCards)
		seen := 0
		for seed := int64(1); seed <= 8; seed++ {
			st, _, _ := randomGame(t, g, seed, 8000, func(ply int, st *state.BoardState, seat int, mv state.Selection) {
				if err := state.Validate(g, st); err != nil {
					t.Fatalf("%s seed %d ply %d after %s: %v", g.Definition.ID, seed, ply, state.FormatMove(g, mv), err)
				}
			})
			seen += len(data(t, st).Cathedrals)
		}
		if seen == 0 {
			t.Errorf("%s: no Cathedral was ever built", g.Definition.ID)
		}
	}
	checkEngineContract(t, packTable(t, "cthulhu-wars", []string{"an", "ww", "tt", "ow"}, allCards), 3, 1500)
}

func TestGuidedGamesWithTheAncients(t *testing.T) {
	g := packTable(t, "cthulhu-wars", []string{"an", "gc", "bg", "ys"}, map[string]any{})
	finished := 0
	for seed := int64(1); seed <= 6; seed++ {
		if playGuided(t, g, seed, 9000).ended {
			finished++
		}
	}
	if finished < 3 {
		t.Errorf("only %d of 6 guided games finished", finished)
	}
}

// --- Bubastis ------------------------------------------------------------------------------------

func TestBubastisBeginsOnTheMoonWithItsGate(t *testing.T) {
	g := packTable(t, "cthulhu-wars", []string{"bu", "gc", "cc", "bg"}, map[string]any{})
	st, err := state.InitialStateSeeded(g, 2)
	if err != nil {
		t.Fatal(err)
	}
	d := data(t, st)
	if d.Gates["the-moon"] != 1 {
		t.Errorf("gates: %v", d.Gates)
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
}

func TestRandomGamesWithBubastis(t *testing.T) {
	for _, c := range []struct {
		base string
		keys []string
		opts map[string]any
	}{
		{"cthulhu-wars", []string{"bu", "gc", "cc", "bg"}, map[string]any{}},
		{"cthulhu-wars-5p", []string{"bu", "an", "tt", "ww", "ow"}, allCards},
		{"cthulhu-wars-primeval-3p", []string{"bu", "ys", "sl"}, map[string]any{}},
	} {
		g := packTable(t, c.base, c.keys, c.opts)
		for seed := int64(1); seed <= 8; seed++ {
			randomGame(t, g, seed, 8000, func(ply int, st *state.BoardState, seat int, mv state.Selection) {
				if err := state.Validate(g, st); err != nil {
					t.Fatalf("%s seed %d ply %d after %s: %v", g.Definition.ID, seed, ply, state.FormatMove(g, mv), err)
				}
			})
		}
	}
	checkEngineContract(t, packTable(t, "cthulhu-wars", []string{"bu", "an", "tt", "ow"}, allCards), 3, 1500)
}

func TestGuidedGamesWithBubastis(t *testing.T) {
	g := packTable(t, "cthulhu-wars", []string{"bu", "gc", "bg", "ys"}, map[string]any{})
	finished := 0
	for seed := int64(1); seed <= 6; seed++ {
		if playGuided(t, g, seed, 9000).ended {
			finished++
		}
	}
	if finished < 3 {
		t.Errorf("only %d of 6 guided games finished", finished)
	}
}

func TestOnlyBubastisCanReachTheMoon(t *testing.T) {
	g := packTable(t, "cthulhu-wars", []string{"bu", "gc", "cc", "bg"}, map[string]any{})
	st, err := state.InitialStateSeeded(g, 2)
	if err != nil {
		t.Fatal(err)
	}
	seat := state.FirstSeat(g)
	rng := rand.New(rand.NewSource(1))
	for ply := 0; ply < 1500; ply++ {
		legal := state.LegalSelections(g, st, seat)
		if len(legal) == 0 {
			t.Fatalf("no legal moves at %d", ply)
		}
		for _, mv := range legal {
			if mv.Action == "go" && mv.To == "the-moon" && seat != 1 {
				t.Fatalf("seat %d can move to the Moon", seat)
			}
		}
		res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
		if err != nil {
			t.Fatal(err)
		}
		if res.GameEnded {
			break
		}
		st, seat = res.NewState, res.NextImpulseSeat
	}
}
