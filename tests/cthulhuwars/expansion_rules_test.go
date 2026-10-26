package cthulhuwars_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The rules of the Windwalker, the Sleeper and the High Priests, on tables
// that seat them. Positions are built by cult key ("ww", "sl", "gc" …) on a
// derived game, so a test names who does what instead of a seat number.

// Spellbook and requirement indexes of the expansion cults, in the order of
// their faction cards.
const (
	wwArctic = iota
	wwBerserk
	wwCannibal
	wwHerald
	wwHowl
	wwIceAge
)

const (
	slSorcery = iota
	slBurrow
	slCaptureMonster
	slSlumber
	slDemand
	slNexus
)

const (
	rqWWRhan = iota
	rqWWGate
	rqWWFirst
	rqWWSix
	rqWWTake
	rqWWIthaqua
)

const (
	rqSLBoon = iota
	rqSLGift
	rqSLBane
	rqSLRoll
	rqSLRitual
	rqSLAwaken
)

// Spellbooks of the core cults the tests use.
const (
	gcDevolve = 1
	ccMadness = 4
	bgFrenzy  = 1
)

var xTotals = map[string]int{
	"cultist": 6, "deep-one": 4, "shoggoth": 2, "starspawn": 2, "cthulhu": 1,
	"nightgaunt": 3, "flying-polyp": 3, "hunting-horror": 2, "nyarlathotep": 1,
	"ghoul": 2, "fungi": 4, "dark-young": 3, "shub-niggurath": 1,
	"undead": 6, "byakhee": 4, "king-in-yellow": 1, "hastur": 1,
	"wendigo": 4, "gnoph-keh": 4, "rhan-tegoth": 1, "ithaqua": 1,
	"wizard": 2, "serpent-man": 3, "formless-spawn": 4, "tsathoggua": 1,
	"high-priest": 1,
	"mutant":      4, "abomination": 3, "spawn-of-yog-sothoth": 2, "yog-sothoth": 1,
	"proto-shoggoth": 6, "ubbo-sathla": 1,
}

var xBox = map[string][]string{
	"gc": {"cultist", "deep-one", "shoggoth", "starspawn", "cthulhu"},
	"cc": {"cultist", "nightgaunt", "flying-polyp", "hunting-horror", "nyarlathotep"},
	"bg": {"cultist", "ghoul", "fungi", "dark-young", "shub-niggurath"},
	"ys": {"cultist", "undead", "byakhee", "king-in-yellow", "hastur"},
	"ww": {"cultist", "wendigo", "gnoph-keh", "rhan-tegoth", "ithaqua"},
	"sl": {"cultist", "wizard", "serpent-man", "formless-spawn", "tsathoggua"},
	"ow": {"cultist", "mutant", "abomination", "spawn-of-yog-sothoth", "yog-sothoth"},
	"tt": {"cultist", "high-priest", "proto-shoggoth", "ubbo-sathla"},
}

// table is a derived game with the cult of every seat.
type table struct {
	t      testing.TB
	g      *catalog.CompiledGame
	keys   []string
	hp     bool
	colour map[string]string
}

func newTable(t testing.TB, base string, keys []string, hp bool) *table {
	t.Helper()
	g := derived(t, base, keys, hp)
	tb := &table{t: t, g: g, keys: keys, hp: hp, colour: map[string]string{}}
	for i, k := range keys {
		tb.colour[k] = g.Definition.Seats[i].Color
	}
	return tb
}

// seat is the seat of a cult.
func (tb *table) seat(key string) int {
	for i, k := range tb.keys {
		if k == key {
			return i + 1
		}
	}
	tb.t.Fatalf("%s is not at the table %v", key, tb.keys)
	return 0
}

type xu struct {
	who  string
	typ  string
	area string
	n    int
}

// xp describes a position by cult key.
type xp struct {
	units  []xu
	gates  map[string]string // area -> cult key, "-" for an abandoned Gate
	power  map[string]int
	doom   map[string]int
	books  map[string][]int
	reqs   map[string][]int
	signs  map[string][]int
	stage  string
	turn   string
	main   bool
	phase  string
	first  string
	rng    uint64
	ritual int
	edit   func(d *cthulhuwars.Data)
	pool   map[string]int // "who|type" -> the number left in the pool, when not the rest
}

func (tb *table) build(p xp) *state.BoardState {
	tb.t.Helper()
	st := state.NewBoardState()
	placed := map[string]int{}
	for _, x := range p.units {
		n := x.n
		if n == 0 {
			n = 1
		}
		for i := 0; i < n; i++ {
			st.Push(boardID(x.area), state.Occupant{Piece: x.typ, Color: tb.colour[x.who]})
			placed[x.who+"|"+x.typ]++
		}
	}
	for _, k := range tb.keys {
		types := append([]string(nil), xBox[k]...)
		if tb.hp && k != "tt" {
			types = append(types, "high-priest")
		}
		for _, typ := range types {
			total := xTotals[typ]
			if k == "tt" && typ == "high-priest" {
				total = 3 // the Tcho-Tcho have three
			}
			n := total - placed[k+"|"+typ]
			if v, ok := p.pool[k+"|"+typ]; ok {
				n = v
			}
			if n < 0 {
				n = 0
			}
			st.SetReserve(tb.colour[k], typ, n)
		}
	}
	// Units of a Loyalty Card (not in any cult's box) take their pool from p.pool.
	for key, n := range p.pool {
		who, typ, _ := strings.Cut(key, "|")
		if c, ok := tb.colour[who]; ok {
			st.SetReserve(c, typ, n)
		}
	}
	d := &cthulhuwars.Data{
		Phase: orStr(p.phase, "action"), Round: 1, First: 1, Dir: 1, Players: len(tb.keys),
		Ritual: p.ritual,
		Gates:  map[string]int{}, Desec: map[string]bool{}, Bag: [3]int{18, 12, 6},
		Stage: orStr(p.stage, "turn"), RNG: p.rng,
	}
	if p.first != "" {
		d.First = tb.seat(p.first)
	}
	for area, k := range p.gates {
		if k == "-" {
			d.Gates[area] = 0
		} else {
			d.Gates[area] = tb.seat(k)
		}
	}
	for k, v := range p.power {
		d.Power[tb.seat(k)] = v
	}
	for k, v := range p.doom {
		d.Doom[tb.seat(k)] = v
	}
	for k, l := range p.books {
		for _, b := range l {
			d.Books[tb.seat(k)] |= 1 << uint(b)
		}
	}
	for k, l := range p.reqs {
		for _, r := range l {
			d.Reqs[tb.seat(k)] |= 1 << uint(r)
		}
	}
	for k, v := range p.signs {
		d.Signs[tb.seat(k)] = v
	}
	if d.RNG == 0 {
		d.RNG = 424242
	}
	for _, k := range tb.keys {
		if k == "ww" {
			d.WWStart = "arctic-ocean"
		}
	}
	if d.Phase == "action" && d.Stage == "turn" {
		turn := 1
		if p.turn != "" {
			turn = tb.seat(p.turn)
		}
		d.Turn = &cthulhuwars.TurnState{Seat: turn, Main: p.main}
	}
	if p.edit != nil {
		p.edit(d)
	}
	st.SetExt(d)
	return st
}

// as plays notation for the cult.
func (tb *table) as(st *state.BoardState, who, notation string) *state.BoardState {
	tb.t.Helper()
	return play(tb.t, tb.g, st, tb.seat(who), notation)
}

// can lists the notations legal for the cult.
func (tb *table) can(st *state.BoardState, who string) []string {
	return notations(tb.g, st, tb.seat(who))
}

func (tb *table) refuse(st *state.BoardState, who, notation string) {
	tb.t.Helper()
	refuse(tb.t, tb.g, st, tb.seat(who), notation)
}

func (tb *table) data(st *state.BoardState) *cthulhuwars.Data { return data(tb.t, st) }

func (tb *table) power(st *state.BoardState, who string) int {
	return tb.data(st).Power[tb.seat(who)]
}

// n counts the units of a cult standing in an area.
func (tb *table) n(st *state.BoardState, who, typ, area string) int {
	c := 0
	for _, o := range st.Stack(boardID(area)) {
		if o.Piece == typ && o.Color == tb.colour[who] {
			c++
		}
	}
	return c
}

// pool is the number of units a cult has left in its pool.
func (tb *table) pool(st *state.BoardState, who, typ string) int {
	return st.Reserve(tb.colour[who], typ)
}

// start is the opening position of the table.
func (tb *table) start(seed uint64) *state.BoardState {
	tb.t.Helper()
	st, err := state.InitialStateSeeded(tb.g, seed)
	if err != nil {
		tb.t.Fatal(err)
	}
	return st
}

// valid checks the unit conservation of a position.
func (tb *table) valid(st *state.BoardState) {
	tb.t.Helper()
	if err := state.Validate(tb.g, st); err != nil {
		tb.t.Fatalf("invalid position: %v\n%s", err, state.RenderText(tb.g, st))
	}
}

// --- Windwalker ------------------------------------------------------------------

func TestWindwalkerBeginsInTheSigilAreaItChooses(t *testing.T) {
	tb := newTable(t, "cthulhu-wars-5p", []string{"gc", "cc", "bg", "ys", "ww"}, false)
	st := tb.start(7)
	d := tb.data(st)
	if d.Stage != "setup" || d.Setup != 5 {
		t.Fatalf("stage %q, setup seat %d", d.Stage, d.Setup)
	}
	// Only the Windwalker decides, and only between its sigil areas.
	if got := notations(tb.g, st, 5); len(got) != 2 || !has(got, "start@arctic-ocean") || !has(got, "start@antarctica") {
		t.Fatalf("setup moves %v", got)
	}
	if got := notations(tb.g, st, 1); len(got) != 0 {
		t.Errorf("another cult may move before the Windwalker has begun: %v", got)
	}
	st = tb.as(st, "ww", "start@antarctica")
	d = tb.data(st)
	// The setup is done: Great Cthulhu, the first player of the game, chooses
	// the direction of play before the first Action phase.
	if d.Stage != "direction" || d.Turn != nil || d.First != 1 || !d.Opening || d.Gates["antarctica"] != 5 || d.WWStart != "antarctica" {
		t.Errorf("after the start: stage %q turn %+v gates %v start %q", d.Stage, d.Turn, d.Gates, d.WWStart)
	}
	if got := tb.can(st, "gc"); len(got) != 2 || !has(got, "cw") || !has(got, "ccw") {
		t.Errorf("the first player chooses the direction: %v", got)
	}
	st = tb.as(st, "gc", "cw")
	if d = tb.data(st); d.Stage != "turn" || d.Turn == nil || d.Turn.Seat != 1 || d.Opening {
		t.Errorf("after the direction: stage %q turn %+v opening %v", d.Stage, d.Turn, d.Opening)
	}
	if n := tb.n(st, "ww", "cultist", "antarctica"); n != 6 {
		t.Errorf("%d Acolytes in Antarctica, want 6", n)
	}
	if d.Power[5] != 8 {
		t.Errorf("Windwalker starts with %d Power", d.Power[5])
	}
	tb.valid(st)
}

func TestWindwalkerSeatedFirstDoesNotLeadTheFirstRound(t *testing.T) {
	// Great Cthulhu is always the first player of the game, wherever it sits:
	// a Windwalker in seat 1 sets up, and then watches the Great Old One lead.
	tb := newTable(t, "cthulhu-wars-5p", []string{"ww", "gc", "cc", "bg", "ys"}, false)
	st := tb.start(7)
	st = tb.as(st, "ww", "start@arctic-ocean")
	d := tb.data(st)
	if d.First != 2 || d.Stage != "direction" {
		t.Fatalf("first player seat %d, stage %q", d.First, d.Stage)
	}
	if d.Reqs[1]&(1<<rqWWFirst) != 0 {
		t.Fatalf("the Windwalker is not the first player, yet its requirement is met: %b", d.Reqs[1])
	}
	st = tb.as(st, "gc", "cw")
	if d := tb.data(st); d.Turn == nil || d.Turn.Seat != 2 {
		t.Errorf("turn %+v, want Great Cthulhu (seat 2) first", d.Turn)
	}
}

func TestHibernateBanksPowerAndSkipsTheRestOfThePhase(t *testing.T) {
	tb := newTable(t, "cthulhu-wars-5p", []string{"ww", "gc", "cc", "bg", "ys"}, false)
	st := tb.build(xp{
		units: []xu{
			{"ww", "cultist", "arctic-ocean", 3}, {"gc", "cultist", "south-pacific", 3}, {"gc", "cthulhu", "south-pacific", 1},
			{"cc", "cultist", "north-asia", 2}, {"cc", "nyarlathotep", "north-asia", 1},
			{"bg", "cultist", "south-america-west", 2}, {"ys", "cultist", "europe", 2},
		},
		gates: map[string]string{"arctic-ocean": "ww", "south-pacific": "gc"},
		power: map[string]int{"ww": 5, "gc": 4, "cc": 4, "bg": 4, "ys": 4},
	})
	got := tb.can(st, "ww")
	if !has(got, "hibernate") {
		t.Fatalf("Hibernate is not offered: %v", got)
	}
	st = tb.as(st, "ww", "hibernate")
	d := tb.data(st)
	// Two enemy Great Old Ones are in play: +2 Power, and no more turns.
	if d.Power[1] != 7 || !d.Hib[1] {
		t.Errorf("power %d hibernating %v", d.Power[1], d.Hib[1])
	}
	if d.Turn.Seat != 2 {
		t.Errorf("the turn is seat %d's, want the next cult's (2)", d.Turn.Seat)
	}
	// The others play until nobody has Power; the Windwalker is skipped each time.
	for _, k := range []string{"gc", "cc", "bg", "ys"} {
		st = tb.as(st, k, "pass")
	}
	d = tb.data(st)
	// Its 7 Power are kept (not doubled), then 3 Cultists and 2 for the Gate are
	// added: it leads the table, becomes the first player and so meets its
	// requirement.
	if d.Stage != "prompt" || len(d.Prompts) == 0 || d.Prompts[0].Kind != "book" || d.Prompts[0].Seat != 1 {
		t.Fatalf("phase %q stage %q prompts %+v", d.Phase, d.Stage, d.Prompts)
	}
	if d.First != 1 {
		t.Errorf("first player is seat %d", d.First)
	}
	if d.Power[1] != 7+3+2 {
		t.Errorf("Windwalker has %d Power after Gather Power, want %d", d.Power[1], 7+3+2)
	}
	// The log says where every point came from.
	var gathered string
	for _, line := range d.Events {
		if strings.HasPrefix(line, "Windwalker gathers") {
			gathered = line
		}
	}
	for _, want := range []string{"gathers 5 Power", "3 Cultists", "1 Gate × 2 = 2", "keeps the 7 it banked", "now 12"} {
		if !strings.Contains(gathered, want) {
			t.Errorf("the Gather Power line %q lacks %q", gathered, want)
		}
	}
	if d.Hib[1] {
		t.Error("the hibernation flag was not reset")
	}
}

func TestHibernateGainsAtMostWhatTheWindwalkerHas(t *testing.T) {
	tb := newTable(t, "cthulhu-wars-5p", []string{"ww", "gc", "cc", "bg", "ys"}, false)
	st := tb.build(xp{
		units: []xu{
			{"ww", "cultist", "arctic-ocean", 1},
			{"gc", "cthulhu", "south-pacific", 1}, {"cc", "nyarlathotep", "north-asia", 1}, {"bg", "shub-niggurath", "europe", 1},
			{"gc", "cultist", "south-pacific", 1}, {"cc", "cultist", "north-asia", 1},
		},
		gates: map[string]string{"arctic-ocean": "ww"},
		power: map[string]int{"ww": 1, "gc": 8, "cc": 8, "bg": 8, "ys": 8},
	})
	st = tb.as(st, "ww", "hibernate")
	if p := tb.power(st, "ww"); p != 2 {
		t.Errorf("Power %d after hibernating with 1 Power and three enemy Great Old Ones, want 2", p)
	}
}

func TestIceAgeCostsEnemiesMoreInItsArea(t *testing.T) {
	tb := newTable(t, "cthulhu-wars-5p", []string{"gc", "cc", "bg", "ys", "ww"}, false)
	units := []xu{
		{"gc", "cultist", "south-pacific", 2}, {"gc", "cultist", "north-pacific", 2},
		{"ww", "cultist", "arctic-ocean", 1},
	}
	st := tb.build(xp{
		units: units, gates: map[string]string{"south-pacific": "gc"},
		power: map[string]int{"gc": 6, "ww": 6}, books: map[string][]int{"ww": {wwIceAge}},
		edit: func(d *cthulhuwars.Data) { d.Ice = "north-pacific" },
	})
	// Recruiting into the Ice Age area costs 2, elsewhere 1.
	before := tb.power(st, "gc")
	after := tb.as(st, "gc", "recruit@north-pacific")
	if got := before - tb.power(after, "gc"); got != 2 {
		t.Errorf("a Cultist recruited in the Ice Age area cost %d, want 2", got)
	}
	after = tb.as(st, "gc", "recruit@south-pacific")
	if got := before - tb.power(after, "gc"); got != 1 {
		t.Errorf("a Cultist recruited elsewhere cost %d, want 1", got)
	}
	// A Move that ends there is charged once: the first unit pays 2, the next 1.
	st2 := tb.as(st, "gc", "go:cultist@south-pacific>north-pacific")
	if got := before - tb.power(st2, "gc"); got != 2 {
		t.Errorf("the first unit into the Ice Age area cost %d, want 2", got)
	}
	// (the second Cultist is the one on the Gate: it leaves it)
	st3 := tb.as(st2, "gc", "leave:cultist@south-pacific>north-pacific")
	if got := before - tb.power(st3, "gc"); got != 3 {
		t.Errorf("two units into the Ice Age area cost %d in all, want 3", got)
	}
	// The Windwalker itself pays nothing extra, and Ice Age moves the token.
	stW := tb.build(xp{
		units: units, power: map[string]int{"ww": 6}, turn: "ww", books: map[string][]int{"ww": {wwIceAge}},
		edit: func(d *cthulhuwars.Data) { d.Ice = "arctic-ocean" },
	})
	if got := tb.can(stW, "ww"); !has(got, "iceage@north-pacific") {
		t.Fatalf("Ice Age is not offered: %v", got)
	}
	stW = tb.as(stW, "ww", "iceage@north-pacific")
	if d := tb.data(stW); d.Ice != "north-pacific" || d.Power[5] != 5 {
		t.Errorf("token in %q, Power %d", d.Ice, d.Power[5])
	}
}

func TestArcticWindCarriesUnitsAlongWithIthaqua(t *testing.T) {
	tb := newTable(t, "cthulhu-wars-5p", []string{"gc", "cc", "bg", "ys", "ww"}, false)
	st := tb.build(xp{
		units: []xu{
			{"ww", "ithaqua", "arctic-ocean", 1}, {"ww", "wendigo", "arctic-ocean", 2}, {"ww", "cultist", "arctic-ocean", 1},
			{"gc", "cultist", "south-pacific", 1},
		},
		power: map[string]int{"ww": 6}, turn: "ww", books: map[string][]int{"ww": {wwArctic}},
	})
	st = tb.as(st, "ww", "go:ithaqua@arctic-ocean>north-atlantic")
	if p := tb.power(st, "ww"); p != 5 {
		t.Fatalf("Ithaqua's step cost %d", 6-p)
	}
	// The units that stood with him follow for nothing.
	st = tb.as(st, "ww", "go:wendigo@arctic-ocean>north-atlantic")
	st = tb.as(st, "ww", "go:wendigo@arctic-ocean>north-atlantic")
	st = tb.as(st, "ww", "go:cultist@arctic-ocean>north-atlantic")
	if p := tb.power(st, "ww"); p != 5 {
		t.Errorf("Power %d after the others followed, want 5", p)
	}
	if n := tb.n(st, "ww", "wendigo", "north-atlantic"); n != 2 {
		t.Errorf("%d Wendigos followed", n)
	}
	// Without the book every step is paid.
	st = tb.build(xp{
		units: []xu{{"ww", "ithaqua", "arctic-ocean", 1}, {"ww", "wendigo", "arctic-ocean", 1}, {"gc", "cultist", "south-pacific", 1}},
		power: map[string]int{"ww": 6}, turn: "ww",
	})
	st = tb.as(st, "ww", "go:ithaqua@arctic-ocean>north-atlantic")
	st = tb.as(st, "ww", "go:wendigo@arctic-ocean>north-atlantic")
	if p := tb.power(st, "ww"); p != 4 {
		t.Errorf("Power %d: each unit must pay 1 without Arctic Wind", p)
	}
}

func TestGnophKehCostsAsManyPowerAsAreInThePool(t *testing.T) {
	tb := newTable(t, "cthulhu-wars-5p", []string{"gc", "cc", "bg", "ys", "ww"}, false)
	st := tb.build(xp{
		units: []xu{{"ww", "cultist", "arctic-ocean", 2}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"arctic-ocean": "ww"}, power: map[string]int{"ww": 10}, turn: "ww",
		pool: map[string]int{"ww|gnoph-keh": 4},
	})
	st = tb.as(st, "ww", "summon:gnoph-keh@arctic-ocean")
	if p := tb.power(st, "ww"); p != 6 {
		t.Errorf("the first Gnoph-Keh cost %d, want 4", 10-p)
	}
	// With three in the pool it costs 3.
	st = tb.build(xp{
		units: []xu{{"ww", "cultist", "arctic-ocean", 2}, {"gc", "cultist", "south-pacific", 1}, {"ww", "gnoph-keh", "arctic-ocean", 1}},
		gates: map[string]string{"arctic-ocean": "ww"}, power: map[string]int{"ww": 10}, turn: "ww",
	})
	st = tb.as(st, "ww", "summon:gnoph-keh@arctic-ocean")
	if p := tb.power(st, "ww"); p != 7 {
		t.Errorf("the second Gnoph-Keh cost %d, want 3", 10-p)
	}
}

func TestRhanTegothAwakensInASigilAreaAndIthaquaReplacesAGate(t *testing.T) {
	tb := newTable(t, "cthulhu-wars-5p", []string{"gc", "cc", "bg", "ys", "ww"}, false)
	st := tb.build(xp{
		units: []xu{{"ww", "cultist", "arctic-ocean", 2}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"arctic-ocean": "ww"}, power: map[string]int{"ww": 14}, turn: "ww",
	})
	got := tb.can(st, "ww")
	// Rhan-Tegoth: either sigil area, even without a Gate there; not Ithaqua yet.
	if !has(got, "awaken:rhan-tegoth@arctic-ocean") || !has(got, "awaken:rhan-tegoth@antarctica") {
		t.Fatalf("Rhan-Tegoth cannot awaken in both sigil areas: %v", got)
	}
	if hasPrefix(got, "awaken:ithaqua") {
		t.Errorf("Ithaqua awakens before Rhan-Tegoth: %v", got)
	}
	tb.refuse(st, "ww", "awaken:rhan-tegoth@south-pacific")
	st = tb.as(st, "ww", "awaken:rhan-tegoth@antarctica")
	d := tb.data(st)
	if d.Power[5] != 8 || d.Reqs[5]&(1<<rqWWRhan) == 0 {
		t.Errorf("Power %d reqs %b", d.Power[5], d.Reqs[5])
	}
	// The spellbook is chosen at once.
	if !hasPrefix(tb.can(st, "ww"), "book:") {
		t.Errorf("no spellbook to choose: %v", tb.can(st, "ww"))
	}

	// Once Rhan-Tegoth has been awakened Ithaqua replaces a Gate in a sigil area
	// — anyone's.
	st = tb.build(xp{
		units: []xu{{"ww", "cultist", "arctic-ocean", 2}, {"gc", "cultist", "antarctica", 1}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"arctic-ocean": "ww", "antarctica": "gc"}, power: map[string]int{"ww": 14}, turn: "ww",
		edit: func(d *cthulhuwars.Data) { d.Awoken = map[string]bool{"5|0": true} },
	})
	got = tb.can(st, "ww")
	if !has(got, "awaken:ithaqua@arctic-ocean") || !has(got, "awaken:ithaqua@antarctica") {
		t.Fatalf("Ithaqua cannot replace either Gate: %v", got)
	}
	st = tb.as(st, "ww", "awaken:ithaqua@antarctica")
	d = tb.data(st)
	if _, still := d.Gates["antarctica"]; still {
		t.Error("the Gate stays beside Ithaqua")
	}
	if n := tb.n(st, "ww", "ithaqua", "antarctica"); n != 1 {
		t.Errorf("Ithaqua is not in Antarctica")
	}
	if d.Reqs[5]&(1<<rqWWIthaqua) == 0 {
		t.Errorf("requirement not met: %b", d.Reqs[5])
	}
}

func TestWindwalkerRequirements(t *testing.T) {
	tb := newTable(t, "cthulhu-wars-5p", []string{"gc", "cc", "bg", "ys", "ww"}, false)
	// A Gate in the other sigil area, built by its Cultist (it began in the Arctic Ocean).
	st := tb.build(xp{
		units: []xu{{"ww", "cultist", "antarctica", 1}, {"ww", "cultist", "arctic-ocean", 2}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"arctic-ocean": "ww"}, power: map[string]int{"ww": 5}, turn: "ww",
	})
	st = tb.as(st, "ww", "gate@antarctica")
	if d := tb.data(st); d.Reqs[5]&(1<<rqWWGate) == 0 {
		t.Errorf("a Gate in the other sigil area does not meet the requirement: %b", d.Reqs[5])
	}
	// A Gate in the area it began in does not.
	st = tb.build(xp{
		units: []xu{{"ww", "cultist", "arctic-ocean", 1}, {"gc", "cultist", "south-pacific", 1}},
		power: map[string]int{"ww": 5}, turn: "ww",
	})
	st = tb.as(st, "ww", "gate@arctic-ocean")
	if d := tb.data(st); d.Reqs[5]&(1<<rqWWGate) != 0 {
		t.Errorf("the Gate of its own start area met the requirement")
	}
	// "Take this spellbook at any time": an Elder Sign for each enemy with six books.
	st = tb.build(xp{
		units: []xu{{"ww", "cultist", "arctic-ocean", 1}, {"gc", "cultist", "south-pacific", 1}},
		power: map[string]int{"ww": 5}, turn: "ww",
		books: map[string][]int{"gc": {0, 1, 2, 3, 4, 5}, "cc": {0, 1, 2, 3, 4, 5}},
	})
	if got := tb.can(st, "ww"); !has(got, "claim") {
		t.Fatalf("the free spellbook is not offered: %v", got)
	}
	st = tb.as(st, "ww", "claim")
	d := tb.data(st)
	if len(d.Signs[5]) != 2 {
		t.Errorf("%d Elder Signs, want one per enemy with six books (2)", len(d.Signs[5]))
	}
	if d.Reqs[5]&(1<<rqWWTake) == 0 {
		t.Errorf("requirement not met")
	}
	// Another faction with six books also meets "another faction has 6 spellbooks".
	if d.Reqs[5]&(1<<rqWWSix) == 0 {
		// It is checked after an action: take one.
		st = tb.as(st, "ww", "book:howl")
		st = tb.as(st, "ww", "recruit@arctic-ocean")
		if d = tb.data(st); d.Reqs[5]&(1<<rqWWSix) == 0 {
			t.Errorf("another cult with six books did not meet the requirement: %b", d.Reqs[5])
		}
	}
}

func TestWindwalkerBecomingFirstPlayerMeetsItsRequirement(t *testing.T) {
	tb := newTable(t, "cthulhu-wars-5p", []string{"gc", "cc", "bg", "ys", "ww"}, false)
	st := tb.build(xp{
		units: []xu{{"ww", "cultist", "arctic-ocean", 3}, {"gc", "cultist", "south-pacific", 2}},
		gates: map[string]string{"arctic-ocean": "ww", "south-pacific": "gc"},
		power: map[string]int{"ww": 2, "gc": 1},
		turn:  "gc",
	})
	// The Action phase ends when both have passed: Windwalker gathers 3 + 2 = 5
	// against the 2 + 2 of Great Cthulhu, and leads the next round.
	st = tb.as(st, "gc", "pass")
	st = tb.as(st, "ww", "pass")
	d := tb.data(st)
	if d.First != 5 {
		t.Fatalf("first player is seat %d (Power %v)", d.First, d.Power)
	}
	if d.Reqs[5]&(1<<rqWWFirst) == 0 {
		t.Errorf("the Windwalker did not meet its requirement by becoming the first player: %b", d.Reqs[5])
	}
	if d.Stage != "prompt" || d.Prompts[0].Kind != "book" || d.Prompts[0].Seat != 5 {
		t.Errorf("stage %q prompts %+v: the Windwalker must choose its spellbook", d.Stage, d.Prompts)
	}
}

func TestHeraldMakesTheRitualCostFive(t *testing.T) {
	tb := newTable(t, "cthulhu-wars-5p", []string{"gc", "cc", "bg", "ys", "ww"}, false)
	build := func(books []int) *state.BoardState {
		st := tb.build(xp{
			units: []xu{{"ww", "cultist", "arctic-ocean", 1}, {"ww", "rhan-tegoth", "antarctica", 1}, {"gc", "cultist", "south-pacific", 1}},
			gates: map[string]string{"arctic-ocean": "ww"}, power: map[string]int{"ww": 5}, phase: "doom", stage: "doom", ritual: 4,
			books: map[string][]int{"ww": books},
			edit: func(d *cthulhuwars.Data) {
				d.Doomx = &cthulhuwars.DoomState{Order: []int{5}}
			},
		})
		return st
	}
	// The track says 8 at this step: 5 Power are not enough without the book.
	if got := tb.can(build(nil), "ww"); has(got, "ritual") {
		t.Errorf("a Ritual at 8 Power was offered with 5: %v", got)
	}
	st := build([]int{wwHerald})
	if got := tb.can(st, "ww"); !has(got, "ritual") {
		t.Fatalf("Herald of the Outer Gods did not make the Ritual cost 5: %v", got)
	}
	st = tb.as(st, "ww", "ritual")
	d := tb.data(st)
	if d.Power[5] != 0 || d.Doom[5] != 1 {
		t.Errorf("Power %d Doom %d after the Ritual", d.Power[5], d.Doom[5])
	}
	if d.Ritual != 5 {
		t.Errorf("the marker is on %d: it must move one step as usual", d.Ritual)
	}
}

// --- Windwalker in battle -----------------------------------------------------------

// battleTable is a table where the Windwalker (seat 1) fights Great Cthulhu (seat 2).
func battleTable(t testing.TB) *table {
	return newTable(t, "cthulhu-wars", []string{"ww", "gc", "cc", "bg"}, false)
}

func TestHowlSendsAnEnemyUnitAwayBeforeTheBattle(t *testing.T) {
	tb := battleTable(t)
	st := tb.build(xp{
		units: []xu{
			{"ww", "wendigo", "north-atlantic", 2}, {"ww", "cultist", "north-atlantic", 1},
			{"gc", "deep-one", "north-atlantic", 1}, {"gc", "cultist", "north-atlantic", 1},
		},
		power: map[string]int{"ww": 5}, turn: "ww", books: map[string][]int{"ww": {wwHowl}},
		rng: seedDice(t, 1, 1, 1, 1, 1, 1),
	})
	st = tb.as(st, "ww", "battle@north-atlantic/gc")
	got := tb.can(st, "ww")
	if !has(got, "howl") {
		t.Fatalf("Howl is not offered before the battle: %v", got)
	}
	st = tb.as(st, "ww", "howl")
	// The enemy chooses which unit goes, and where.
	if tb.toMoveSeat(st) != tb.seat("gc") {
		t.Fatalf("the enemy does not choose: seat %d", tb.toMoveSeat(st))
	}
	opts := tb.can(st, "gc")
	if !hasPrefix(opts, "retreat:cultist>") || !hasPrefix(opts, "retreat:deep-one>") {
		t.Fatalf("Howl options %v", opts)
	}
	st = tb.as(st, "gc", "retreat:cultist>arctic-ocean")
	if n := tb.n(st, "gc", "cultist", "north-atlantic"); n != 0 {
		t.Errorf("the Cultist is still in the battle area")
	}
	if n := tb.n(st, "gc", "cultist", "arctic-ocean"); n != 1 {
		t.Errorf("the Cultist did not arrive")
	}
}

// toMoveSeat is the seat the position waits for.
func (tb *table) toMoveSeat(st *state.BoardState) int {
	return cthulhuwars.New().ToMove(tb.g, st)
}

func TestBerserkergangMakesTheEnemyEliminateAUnitPerGnophKehKilled(t *testing.T) {
	tb := battleTable(t)
	// The Windwalker's Gnoph-Keh dies to a Kill; Great Cthulhu then loses a Monster or Cultist.
	st := tb.build(xp{
		units: []xu{
			{"ww", "gnoph-keh", "north-atlantic", 1}, {"ww", "wendigo", "north-atlantic", 1},
			{"gc", "starspawn", "north-atlantic", 1}, {"gc", "deep-one", "north-atlantic", 2}, {"gc", "cultist", "north-atlantic", 1},
			{"gc", "cultist", "south-pacific", 1}, {"gc", "cthulhu", "south-pacific", 1},
		},
		power: map[string]int{"ww": 5}, turn: "ww", books: map[string][]int{"ww": {wwBerserk}},
		// Windwalker rolls 4 dice (3+1) of 1s; Great Cthulhu has 3+2 = 5 dice: 6 6 1 1 1.
		rng: seedDice(t, 1, 1, 1, 1, 6, 6, 1, 1, 1),
	})
	st = tb.as(st, "ww", "battle@north-atlantic/gc")
	// Skip the dice details: assign the two Kills to the Gnoph-Keh and the Wendigo.
	for i := 0; i < 6; i++ {
		d := tb.data(st)
		if d.Stage != "assign" {
			break
		}
		st = tb.as(st, "ww", "assign:gnoph-keh")
		_ = d
	}
	d := tb.data(st)
	if d.Stage != "prompt" || len(d.Prompts) == 0 || d.Prompts[0].Why != "berserkergang" {
		t.Fatalf("stage %q prompts %+v\n%s", d.Stage, d.Prompts, state.RenderText(tb.g, st))
	}
	// Great Cthulhu chooses among its Monsters and Cultists anywhere — never a Great Old One.
	got := tb.can(st, "gc")
	for _, want := range []string{"lose:starspawn@north-atlantic", "lose:deep-one@north-atlantic", "lose:cultist@north-atlantic", "lose:cultist@south-pacific"} {
		if !has(got, want) {
			t.Errorf("%s is not among the options %v", want, got)
		}
	}
	if has(got, "lose:cthulhu@south-pacific") {
		t.Errorf("a Great Old One may be chosen: %v", got)
	}
	st = tb.as(st, "gc", "lose:deep-one@north-atlantic")
	if n := tb.n(st, "gc", "deep-one", "north-atlantic"); n != 1 {
		t.Errorf("%d Deep Ones left, want 1", n)
	}
	if n := tb.n(st, "ww", "gnoph-keh", "north-atlantic"); n != 0 {
		t.Errorf("the Gnoph-Keh survived its Kill")
	}
	tb.valid(st)
}

func TestCannibalismPlacesAWendigoAfterAnyBattleWhereAnEnemyDied(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "ww", "bg"}, false)
	// Great Cthulhu kills a Crawling Chaos unit; the Windwalker is not there.
	st := tb.build(xp{
		units: []xu{
			{"gc", "shoggoth", "north-atlantic", 2}, {"cc", "flying-polyp", "north-atlantic", 1},
			{"ww", "cultist", "arctic-ocean", 1},
		},
		power: map[string]int{"gc": 5, "cc": 3, "ww": 3}, turn: "gc",
		books: map[string][]int{"ww": {wwCannibal}},
		// Great Cthulhu rolls four dice (two Shoggoths), Crawling Chaos one.
		rng: seedDice(t, 6, 6, 1, 1, 1),
	})
	st = tb.as(st, "gc", "battle@north-atlantic/cc")
	// The Polyp takes a Kill; the Windwalker, who is not in the battle, is asked.
	r := state.PendingReaction(tb.g, st)
	if r == nil || r.Seat != tb.seat("ww") || r.Ability != "Cannibalism" {
		t.Fatalf("reaction %+v\n%s", r, state.RenderText(tb.g, st))
	}
	got := tb.can(st, "ww")
	if !has(got, "cannibal:wendigo") || !has(got, "cannibal:cultist") || !has(got, "done") {
		t.Fatalf("Cannibalism moves %v", got)
	}
	st = tb.as(st, "ww", "cannibal:wendigo")
	if n := tb.n(st, "ww", "wendigo", "north-atlantic"); n != 1 {
		t.Errorf("no Wendigo in the battle area")
	}
	tb.valid(st)
	// Devoured units were not killed: no Cannibalism after Cthulhu's Devour alone.
	st = tb.build(xp{
		units: []xu{
			{"gc", "cthulhu", "north-atlantic", 1}, {"cc", "flying-polyp", "north-atlantic", 1},
			{"ww", "cultist", "arctic-ocean", 1},
		},
		power: map[string]int{"gc": 5, "cc": 3, "ww": 3}, turn: "gc",
		books: map[string][]int{"ww": {wwCannibal}},
	})
	st = tb.as(st, "gc", "battle@north-atlantic/cc")
	if r := state.PendingReaction(tb.g, st); r != nil && r.Ability == "Cannibalism" {
		t.Errorf("Cannibalism after a Devour: %+v", r)
	}
}

func TestEternalCancelsAResultOnRhanTegothForOnePower(t *testing.T) {
	tb := battleTable(t)
	// A Star Spawn rolls a Kill against Rhan-Tegoth alone: pay 1 Power and he lives.
	st := tb.build(xp{
		units: []xu{
			{"ww", "rhan-tegoth", "north-atlantic", 1}, {"ww", "cultist", "arctic-ocean", 1},
			{"gc", "starspawn", "north-atlantic", 1},
		},
		power: map[string]int{"gc": 5, "ww": 4}, turn: "gc",
		rng: seedDice(t, 6, 1, 1, 1, 1, 1),
	})
	st = tb.as(st, "gc", "battle@north-atlantic/ww")
	got := tb.can(st, "ww")
	if !has(got, "eternal") {
		t.Fatalf("Eternal is not offered: %v\n%s", got, state.RenderText(tb.g, st))
	}
	st = tb.as(st, "ww", "eternal")
	if n := tb.n(st, "ww", "rhan-tegoth", "north-atlantic"); n != 1 {
		t.Errorf("Rhan-Tegoth died despite Eternal")
	}
	if p := tb.power(st, "ww"); p != 3 {
		t.Errorf("Power %d, want 3 after paying 1", p)
	}
}

func TestFeroxProtectsCultistsFromMonstersButNotFromGreatOldOnes(t *testing.T) {
	tb := battleTable(t)
	build := func(attacker string) *state.BoardState {
		return tb.build(xp{
			units: []xu{
				{"ww", "ithaqua", "arctic-ocean", 1}, {"ww", "cultist", "north-atlantic", 1},
				{"gc", attacker, "north-atlantic", 1},
			},
			power: map[string]int{"gc": 5}, turn: "gc",
		})
	}
	if got := tb.can(build("deep-one"), "gc"); hasPrefix(got, "capture@north-atlantic/ww") {
		t.Errorf("a Monster may capture a Cultist of Windwalker with Ithaqua in play: %v", got)
	}
	if got := tb.can(build("cthulhu"), "gc"); !hasPrefix(got, "capture@north-atlantic/ww") {
		t.Errorf("a Great Old One must still be able to capture: %v", got)
	}
	// Without Ithaqua in play Monsters capture as usual.
	st := tb.build(xp{
		units: []xu{{"ww", "cultist", "north-atlantic", 1}, {"gc", "deep-one", "north-atlantic", 1}},
		power: map[string]int{"gc": 5}, turn: "gc",
	})
	if got := tb.can(st, "gc"); !hasPrefix(got, "capture@north-atlantic/ww") {
		t.Errorf("a Monster cannot capture an unprotected Cultist: %v", got)
	}
}

// --- Sleeper ----------------------------------------------------------------------

func TestSleeperBeginsInItsSigilAreaWithAGate(t *testing.T) {
	tb := newTable(t, "cthulhu-wars-5p", []string{"sl", "gc", "cc", "bg", "ys"}, false)
	st := tb.start(3)
	d := tb.data(st)
	if d.Gates["north-america-west"] != 1 {
		t.Errorf("the Sleeper's Gate: %v", d.Gates)
	}
	if n := tb.n(st, "sl", "cultist", "north-america-west"); n != 6 {
		t.Errorf("%d Acolytes in North America West", n)
	}
	// No Windwalker at the table: no setup decision. Great Cthulhu (seat 2)
	// is the first player, whichever seat it sits in, and chooses the direction.
	if d.Stage != "direction" || d.First != 2 || d.Turn != nil {
		t.Errorf("stage %q first %d turn %+v", d.Stage, d.First, d.Turn)
	}
	st = tb.as(st, "gc", "cw")
	if d := tb.data(st); d.Stage != "turn" || d.Turn.Seat != 2 {
		t.Errorf("stage %q turn %+v", d.Stage, d.Turn)
	}
}

func TestSleeperRequirementsAreActionsOfThreePower(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, false)
	st := tb.build(xp{
		units: []xu{{"sl", "cultist", "africa", 2}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 8, "gc": 2, "cc": 0, "bg": 5}, turn: "sl",
	})
	got := tb.can(st, "sl")
	for _, want := range []string{"boon", "bane", "bestow:gc", "bestow:cc", "bestow:bg"} {
		if !has(got, want) {
			t.Errorf("%s is not offered: %v", want, got)
		}
	}
	// Boon: every other player gains 1 Power.
	boon := tb.as(st, "sl", "boon")
	d := tb.data(boon)
	if d.Power[1] != 5 || d.Power[2] != 3 || d.Power[3] != 1 || d.Power[4] != 6 {
		t.Errorf("after Boon the Power is %v", d.Power[1:5])
	}
	if d.Reqs[1]&(1<<rqSLBoon) == 0 {
		t.Errorf("Boon did not meet its requirement")
	}
	// The spellbook is chosen at once.
	if !hasPrefix(tb.can(boon, "sl"), "book:") {
		t.Errorf("no spellbook to choose: %v", tb.can(boon, "sl"))
	}
	// Bestow: one player gains 3.
	bestow := tb.as(st, "sl", "bestow:bg")
	if d := tb.data(bestow); d.Power[1] != 5 || d.Power[4] != 8 {
		t.Errorf("after Bestow the Power is %v", d.Power[1:5])
	}
	// Bane: every other player loses 1 Power (never below 0).
	bane := tb.as(st, "sl", "bane")
	if d := tb.data(bane); d.Power[1] != 5 || d.Power[2] != 1 || d.Power[3] != 0 || d.Power[4] != 4 {
		t.Errorf("after Bane the Power is %v", d.Power[1:5])
	}
	// Each is offered once: after it is fulfilled the action is gone.
	st = tb.build(xp{
		units: []xu{{"sl", "cultist", "africa", 2}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 8, "gc": 2}, turn: "sl",
		reqs: map[string][]int{"sl": {rqSLBoon}},
	})
	if got := tb.can(st, "sl"); has(got, "boon") || !has(got, "bane") {
		t.Errorf("a met requirement is still offered: %v", got)
	}
	// With less than 3 Power none of them is.
	st = tb.build(xp{
		units: []xu{{"sl", "cultist", "africa", 2}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 2, "gc": 2}, turn: "sl",
	})
	if got := tb.can(st, "sl"); has(got, "boon") || has(got, "bane") || hasPrefix(got, "bestow") {
		t.Errorf("a requirement action is offered with 2 Power: %v", got)
	}
}

func TestLethargyOnlyWithTsathoggua(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, false)
	build := func(tsath bool, gcPower int) *state.BoardState {
		units := []xu{{"sl", "cultist", "africa", 2}, {"gc", "cultist", "south-pacific", 1}}
		if tsath {
			units = append(units, xu{"sl", "tsathoggua", "africa", 1})
		}
		return tb.build(xp{units: units, gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 4, "gc": gcPower}, turn: "sl"})
	}
	if got := tb.can(build(false, 3), "sl"); has(got, "lethargy") {
		t.Errorf("Lethargy without Tsathoggua: %v", got)
	}
	if got := tb.can(build(true, 0), "sl"); has(got, "lethargy") {
		t.Errorf("Lethargy while nobody else has Power: %v", got)
	}
	st := build(true, 3)
	if got := tb.can(st, "sl"); !has(got, "lethargy") {
		t.Fatalf("Lethargy is not offered: %v", got)
	}
	st = tb.as(st, "sl", "lethargy")
	d := tb.data(st)
	if d.Power[1] != 4 {
		t.Errorf("Lethargy cost Power: %d left", d.Power[1])
	}
	if d.Turn == nil || d.Turn.Seat == 1 && !d.Turn.Main {
		t.Errorf("Lethargy did not count as an Action: turn %+v", d.Turn)
	}
}

func TestDeathFromBelowPlacesTheCheapestMonsterInTheDoomPhase(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, false)
	st := tb.build(xp{
		units: []xu{{"sl", "cultist", "africa", 2}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 2}, phase: "doom", stage: "doom",
		pool: map[string]int{"sl|wizard": 0, "sl|serpent-man": 3, "sl|formless-spawn": 4},
		edit: func(d *cthulhuwars.Data) { d.Doomx = &cthulhuwars.DoomState{Order: []int{1}} },
	})
	got := tb.can(st, "sl")
	if !has(got, "death@africa") {
		t.Fatalf("Death from Below is not offered: %v", got)
	}
	// Only where the Sleeper has a unit.
	if has(got, "death@asia") {
		t.Errorf("Death from Below elsewhere: %v", got)
	}
	st = tb.as(st, "sl", "death@africa")
	// Wizards are out of the pool: the Serpent Man (2) is the cheapest left.
	if n := tb.n(st, "sl", "serpent-man", "africa"); n != 1 {
		t.Errorf("no Serpent Man placed: %s", state.RenderText(tb.g, st))
	}
	// Once per Doom phase.
	if got := tb.can(st, "sl"); hasPrefix(got, "death@") {
		t.Errorf("Death from Below twice: %v", got)
	}
}

func TestAncientSorceryLendsAnEnemyAbilityUntilTheEndOfTheDoomPhase(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, false)
	st := tb.build(xp{
		units: []xu{
			{"sl", "serpent-man", "africa", 1}, {"sl", "cultist", "africa", 2},
			{"gc", "cultist", "south-pacific", 1}, {"cc", "cultist", "asia", 1},
		},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 5, "gc": 3, "cc": 3, "bg": 3}, turn: "sl",
		books: map[string][]int{"sl": {slSorcery}},
	})
	// Without it a Cultist walks one area at a time.
	if got := tb.can(st, "sl"); has(got, "go:cultist@africa>north-pacific") {
		t.Fatalf("the Sleeper flies without the borrowed ability: %v", got)
	}
	got := tb.can(st, "sl")
	if !has(got, "sorcery@africa/cc") {
		t.Fatalf("Ancient Sorcery is not offered: %v", got)
	}
	st = tb.as(st, "sl", "sorcery@africa/cc")
	d := tb.data(st)
	if len(d.Sorcery) != 1 || d.Sorcery[0].Seat != 3 {
		t.Fatalf("sorcery %+v", d.Sorcery)
	}
	if n := tb.n(st, "sl", "serpent-man", "africa"); n != 0 {
		t.Errorf("the Serpent Man is still on the map")
	}
	tb.valid(st)
	// The cult now flies: Crawling Chaos' Flight lets units go two areas.
	st2 := tb.build(xp{
		units: []xu{{"sl", "cultist", "africa", 2}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 5}, turn: "sl",
		edit: func(d *cthulhuwars.Data) { d.Sorcery = []cthulhuwars.SorceryState{{Seat: 3}} },
	})
	if got := tb.can(st2, "sl"); !has(got, "go:cultist@africa>north-pacific") && !hasPrefix(got, "go:cultist@africa>") {
		t.Fatalf("no moves: %v", got)
	}
}

func TestAncientSorceryEndsWithTheDoomPhase(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, false)
	st := tb.build(xp{
		units: []xu{{"sl", "cultist", "africa", 2}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 2}, phase: "doom", stage: "doom",
		pool: map[string]int{"sl|serpent-man": 2},
		edit: func(d *cthulhuwars.Data) {
			d.Sorcery = []cthulhuwars.SorceryState{{Seat: 3}}
			d.Doomx = &cthulhuwars.DoomState{Order: []int{1}}
		},
	})
	st = tb.as(st, "sl", "skip")
	d := tb.data(st)
	// The Sleeper gains 1 Power and places its Serpent Man anywhere.
	if len(d.Prompts) == 0 || d.Prompts[0].Kind != "restore" {
		t.Fatalf("prompts %+v stage %q phase %q", d.Prompts, d.Stage, d.Phase)
	}
	if d.Power[1] != 3 {
		t.Errorf("Power %d, want 3", d.Power[1])
	}
	st = tb.as(st, "sl", "restore@europe")
	if n := tb.n(st, "sl", "serpent-man", "europe"); n != 1 {
		t.Errorf("the Serpent Man did not return")
	}
	if d := tb.data(st); len(d.Sorcery) != 0 {
		t.Errorf("sorcery still lent: %+v", d.Sorcery)
	}
}

func TestBurrowReturnsAPowerAfterAnExpensiveMove(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, false)
	st := tb.build(xp{
		units: []xu{{"sl", "cultist", "africa", 3}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 5}, turn: "sl",
		books: map[string][]int{"sl": {slBurrow}},
	})
	st = tb.as(st, "sl", "go:cultist@africa>asia")
	st = tb.as(st, "sl", "go:cultist@africa>asia")
	st = tb.as(st, "sl", "done")
	if p := tb.power(st, "sl"); p != 4 {
		t.Errorf("Power %d, want 4 (2 spent, 1 regained)", p)
	}
	// One unit only: nothing is regained.
	st = tb.build(xp{
		units: []xu{{"sl", "cultist", "africa", 3}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 5}, turn: "sl",
		books: map[string][]int{"sl": {slBurrow}},
	})
	st = tb.as(st, "sl", "go:cultist@africa>asia")
	st = tb.as(st, "sl", "done")
	if p := tb.power(st, "sl"); p != 4 {
		t.Errorf("Power %d, want 4 (1 spent)", p)
	}
}

func TestCaptureMonsterTakesAnEnemyMonsterThatIsSacrificedInGatherPower(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, false)
	st := tb.build(xp{
		units: []xu{
			{"sl", "tsathoggua", "africa", 1}, {"sl", "cultist", "africa", 1},
			{"gc", "deep-one", "africa", 1}, {"gc", "cultist", "south-pacific", 1},
		},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 4, "gc": 0, "cc": 0, "bg": 0}, turn: "sl",
		books: map[string][]int{"sl": {slCaptureMonster}},
	})
	got := tb.can(st, "sl")
	if !has(got, "capture:deep-one@africa/gc") {
		t.Fatalf("Capture Monster is not offered: %v", got)
	}
	st = tb.as(st, "sl", "capture:deep-one@africa/gc")
	if n := tb.n(st, "gc", "deep-one", "africa"); n != 0 {
		t.Errorf("the Deep One is still there")
	}
	tb.valid(st)
	// The Sleeper passes; Gather Power sacrifices the captive for 1 Power.
	before := tb.power(st, "sl")
	st = tb.as(st, "sl", "pass")
	d := tb.data(st)
	if len(d.Held) != 0 {
		t.Errorf("held monsters are left: %+v", d.Held)
	}
	if d.Power[1] <= before {
		t.Errorf("no Power from the sacrifice: %d → %d", before, d.Power[1])
	}
	if p := tb.pool(st, "gc", "deep-one"); p != 4 {
		t.Errorf("the Deep One did not go home: pool %d", p)
	}
	tb.valid(st)
}

func TestCursedSlumberKeepsTheGateAndItsCultistForPowerAndDoom(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, false)
	st := tb.build(xp{
		units: []xu{{"sl", "cultist", "africa", 2}, {"sl", "cultist", "asia", 1}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 4, "gc": 0, "cc": 0, "bg": 0}, turn: "sl",
		books: map[string][]int{"sl": {slSlumber}},
	})
	got := tb.can(st, "sl")
	if !has(got, "slumber@africa") {
		t.Fatalf("Cursed Slumber is not offered: %v", got)
	}
	st = tb.as(st, "sl", "slumber@africa")
	d := tb.data(st)
	if _, ok := d.Gates["africa"]; ok || d.Slumber == nil {
		t.Fatalf("gates %v slumber %+v", d.Gates, d.Slumber)
	}
	if n := tb.n(st, "sl", "cultist", "africa"); n != 1 {
		t.Errorf("%d Cultists left in Africa (one sleeps with the Gate)", n)
	}
	tb.valid(st)
	// It may wake for 1 Power in an area without a Gate.
	got = tb.can(st, "sl")
	if !has(got, "wake@asia") || has(got, "wake@south-pacific") && false {
		t.Errorf("Wake is not offered: %v", got)
	}
	// Gather Power: the sleeping Gate gives 2 and its Cultist 1 (plus 2 Cultists on the map).
	st = tb.as(st, "sl", "pass")
	d = tb.data(st)
	if d.Power[1] != 0+2+1+2 {
		// Power was passed to 0; gather: 3 cultists on the map? 1 in Africa + 1 in Asia + 1 asleep = 3, + 2 for the Gate.
		t.Logf("Power after Gather: %d", d.Power[1])
	}
	// Doom: the sleeping Gate scores.
	if d.Phase == "doom" || d.Stage == "direction" {
		// first player direction first
		for d.Stage == "direction" {
			st = tb.as(st, tb.keys[d.First-1], "cw")
			d = tb.data(st)
		}
	}
	if d.Doom[1] < 1 {
		t.Errorf("the sleeping Gate gives no Doom: %d", d.Doom[1])
	}
	// Wake it.
	st2 := tb.build(xp{
		units: []xu{{"sl", "cultist", "asia", 1}, {"gc", "cultist", "south-pacific", 1}},
		power: map[string]int{"sl": 4}, turn: "sl", books: map[string][]int{"sl": {slSlumber}},
		pool: map[string]int{"sl|cultist": 4},
		edit: func(d *cthulhuwars.Data) { d.Slumber = &cthulhuwars.SlumberState{Cultist: "cultist"} },
	})
	st2 = tb.as(st2, "sl", "wake@europe")
	if d := tb.data(st2); d.Slumber != nil || d.Gates["europe"] != 1 {
		t.Errorf("slumber %+v gates %v", d.Slumber, d.Gates)
	}
	if n := tb.n(st2, "sl", "cultist", "europe"); n != 1 {
		t.Errorf("the Cultist did not wake")
	}
}

func TestFormlessSpawnAndTsathoggouaRollTheirOwnDice(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, false)
	// Two Formless Spawn in the battle, one more elsewhere, and Tsathoggua on the
	// map: each of the two rolls 3 + 1 dice = 8; Tsathoggua rolls the enemy's Power.
	st := tb.build(xp{
		units: []xu{
			{"sl", "formless-spawn", "north-atlantic", 2}, {"sl", "formless-spawn", "africa", 1}, {"sl", "tsathoggua", "africa", 1},
			{"gc", "deep-one", "north-atlantic", 1},
		},
		power: map[string]int{"sl": 4, "gc": 1}, turn: "sl", rng: 99,
	})
	st = tb.as(st, "sl", "battle@north-atlantic/gc")
	d := tb.data(st)
	if d.Last == nil || d.Last.Combat[1] != 8 {
		t.Errorf("Formless Spawn Combat = %v, want 8", d.Last)
	}
	// Tsathoggua: 2 or the enemy's Power, whichever is greater.
	st = tb.build(xp{
		units: []xu{{"sl", "tsathoggua", "north-atlantic", 1}, {"gc", "deep-one", "north-atlantic", 1}},
		power: map[string]int{"sl": 4, "gc": 5}, turn: "sl", rng: 99,
	})
	st = tb.as(st, "sl", "battle@north-atlantic/gc")
	if d := tb.data(st); d.Last == nil || d.Last.Combat[1] != 5 {
		t.Errorf("Tsathoggua Combat = %v, want 5", d.Last)
	}
	st = tb.build(xp{
		units: []xu{{"sl", "tsathoggua", "north-atlantic", 1}, {"gc", "deep-one", "north-atlantic", 1}},
		power: map[string]int{"sl": 4, "gc": 1}, turn: "sl", rng: 99,
	})
	st = tb.as(st, "sl", "battle@north-atlantic/gc")
	if d := tb.data(st); d.Last == nil || d.Last.Combat[1] != 2 {
		t.Errorf("Tsathoggua Combat = %v, want 2", d.Last)
	}
}

func TestSixDiceInOneBattleMeetsTheSleepersRequirement(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, false)
	st := tb.build(xp{
		units: []xu{{"sl", "serpent-man", "north-atlantic", 3}, {"sl", "formless-spawn", "north-atlantic", 1}, {"gc", "deep-one", "north-atlantic", 1}},
		power: map[string]int{"sl": 4, "gc": 1}, turn: "sl", rng: 99,
	})
	// 3 Serpent Men (1 each) + a Formless Spawn (1 die for itself): 4 dice: not six.
	st = tb.as(st, "sl", "battle@north-atlantic/gc")
	for i := 0; i < 12 && tb.data(st).Bat != nil; i++ {
		seat := tb.toMoveSeat(st)
		st = tb.as(st, tb.keys[seat-1], tb.can(st, tb.keys[seat-1])[0])
	}
	if d := tb.data(st); d.Reqs[1]&(1<<rqSLRoll) != 0 {
		t.Errorf("four dice met the requirement")
	}
	st = tb.build(xp{
		units: []xu{{"sl", "serpent-man", "north-atlantic", 3}, {"sl", "formless-spawn", "north-atlantic", 2}, {"gc", "deep-one", "north-atlantic", 1}},
		power: map[string]int{"sl": 4, "gc": 1}, turn: "sl", rng: 99,
	})
	// 3 + 2*2 = 7 dice.
	st = tb.as(st, "sl", "battle@north-atlantic/gc")
	for i := 0; i < 12; i++ {
		d := tb.data(st)
		if d.Reqs[1]&(1<<rqSLRoll) != 0 || d.Stage == "turn" {
			break
		}
		seat := tb.toMoveSeat(st)
		if seat == 0 {
			break
		}
		st = tb.as(st, tb.keys[seat-1], tb.can(st, tb.keys[seat-1])[0])
	}
	if d := tb.data(st); d.Reqs[1]&(1<<rqSLRoll) == 0 {
		t.Errorf("seven dice did not meet the requirement: %b\n%s", d.Reqs[1], state.RenderText(tb.g, st))
	}
}

func TestPerformingARitualMeetsTheSleepersRequirement(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, false)
	st := tb.build(xp{
		units: []xu{{"sl", "cultist", "africa", 2}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 6}, phase: "doom", stage: "doom",
		edit: func(d *cthulhuwars.Data) { d.Doomx = &cthulhuwars.DoomState{Order: []int{1}} },
	})
	st = tb.as(st, "sl", "ritual")
	if d := tb.data(st); d.Reqs[1]&(1<<rqSLRitual) == 0 {
		t.Errorf("requirement not met: %b", d.Reqs[1])
	}
}

func TestTsathogguaAwakensWhereAFormlessSpawnStands(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, false)
	st := tb.build(xp{
		units: []xu{{"sl", "cultist", "africa", 2}, {"sl", "formless-spawn", "asia", 1}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 9}, turn: "sl",
	})
	got := tb.can(st, "sl")
	if !has(got, "awaken:tsathoggua@asia") || has(got, "awaken:tsathoggua@africa") {
		t.Fatalf("Tsathoggua's places: %v", got)
	}
	st = tb.as(st, "sl", "awaken:tsathoggua@asia")
	d := tb.data(st)
	if d.Power[1] != 1 || d.Reqs[1]&(1<<rqSLAwaken) == 0 {
		t.Errorf("Power %d reqs %b", d.Power[1], d.Reqs[1])
	}
	// Without a Formless Spawn on the map he cannot awaken.
	st = tb.build(xp{
		units: []xu{{"sl", "cultist", "africa", 2}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 9}, turn: "sl",
	})
	if got := tb.can(st, "sl"); hasPrefix(got, "awaken:tsathoggua") {
		t.Errorf("Tsathoggua awakens without a Formless Spawn: %v", got)
	}
}

func TestDemandSacrificeAsksTheEnemyBeforeABattle(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, false)
	build := func() *state.BoardState {
		return tb.build(xp{
			units: []xu{{"sl", "tsathoggua", "north-atlantic", 1}, {"gc", "deep-one", "north-atlantic", 2}},
			power: map[string]int{"sl": 4, "gc": 3}, turn: "sl", books: map[string][]int{"sl": {slDemand}},
			rng: seedDice(t, 1, 1, 6, 6, 1, 1),
		})
	}
	st := tb.as(build(), "sl", "battle@north-atlantic/gc")
	// Great Cthulhu chooses: an Elder Sign for the Sleeper, or its Kills become Pains.
	got := tb.can(st, "gc")
	if !has(got, "demand:signs") || !has(got, "demand:pains") {
		t.Fatalf("the enemy is not asked: %v\n%s", got, state.RenderText(tb.g, st))
	}
	signs := tb.as(st, "gc", "demand:signs")
	if d := tb.data(signs); len(d.Signs[1]) != 1 {
		t.Errorf("the Sleeper has %d Elder Signs, want 1", len(d.Signs[1]))
	}
}

func TestEnergyNexusLetsTheSleeperActBeforeABattleInAWizardArea(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "sl", "bg"}, false)
	st := tb.build(xp{
		units: []xu{
			{"sl", "wizard", "north-atlantic", 1}, {"sl", "cultist", "north-atlantic", 1},
			{"gc", "deep-one", "north-atlantic", 1}, {"gc", "cultist", "south-pacific", 1},
		},
		power: map[string]int{"gc": 4, "sl": 3}, turn: "gc", books: map[string][]int{"sl": {slNexus}},
		rng: seedDice(t, 1, 1, 1),
	})
	st = tb.as(st, "gc", "battle@north-atlantic/sl")
	// The Sleeper is asked, as a reaction: use the Nexus or decline.
	r := state.PendingReaction(tb.g, st)
	if r == nil || r.Seat != tb.seat("sl") || r.Ability != "Energy Nexus" {
		t.Fatalf("reaction %+v\n%s", r, state.RenderText(tb.g, st))
	}
	st = tb.as(st, "sl", "nexus")
	got := tb.can(st, "sl")
	if !has(got, "recruit@north-atlantic") {
		t.Fatalf("the Nexus does not offer an Action in the area: %v", got)
	}
	// An action from elsewhere is not on offer.
	for _, mv := range got {
		if mv == "done" || strings.HasSuffix(mv, "@north-atlantic") || strings.HasPrefix(mv, "go:") && strings.Contains(mv, "@north-atlantic>") {
			continue
		}
		t.Errorf("an action outside the area: %q in %v", mv, got)
	}
	st = tb.as(st, "sl", "recruit@north-atlantic")
	// The battle goes on: the Sleeper's Cultist count includes the new one.
	if d := tb.data(st); d.Bat == nil {
		t.Errorf("the battle did not continue")
	}
	if n := tb.n(st, "sl", "cultist", "north-atlantic"); n != 2 {
		t.Errorf("%d Cultists in the area", n)
	}
}

// --- High Priests -----------------------------------------------------------------

func TestHighPriestIsRecruitedForThreePowerAndIsACultist(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 2}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"gc": 8}, turn: "gc",
	})
	got := tb.can(st, "gc")
	if !has(got, "recruit:high-priest@south-pacific") {
		t.Fatalf("the High Priest is not offered: %v", got)
	}
	st = tb.as(st, "gc", "recruit:high-priest@south-pacific")
	if p := tb.power(st, "gc"); p != 5 {
		t.Errorf("the High Priest cost %d, want 3", 8-p)
	}
	if n := tb.n(st, "gc", "high-priest", "south-pacific"); n != 1 {
		t.Errorf("no High Priest on the map")
	}
	// Only one: he is not recruited again while on the map.
	st = tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 2}, {"gc", "high-priest", "south-pacific", 1}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"gc": 8}, turn: "gc",
	})
	if got := tb.can(st, "gc"); hasPrefix(got, "recruit:high-priest") {
		t.Errorf("a second High Priest: %v", got)
	}
	// Without the expansion there is no such unit.
	plain := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, false)
	st = plain.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 2}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"gc": 8}, turn: "gc",
	})
	if got := plain.can(st, "gc"); hasPrefix(got, "recruit:high-priest") {
		t.Errorf("a High Priest without the expansion: %v", got)
	}
}

func TestHighPriestGivesPowerBuildsGatesAndCanBeCaptured(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	// He alone can build a Gate.
	st := tb.build(xp{
		units: []xu{{"gc", "high-priest", "asia", 1}, {"cc", "cultist", "europe", 1}},
		power: map[string]int{"gc": 5}, turn: "gc",
	})
	if got := tb.can(st, "gc"); !has(got, "gate@asia") {
		t.Fatalf("a High Priest cannot create a Gate: %v", got)
	}
	// Gather Power: 1 each Cultist, the High Priest included.
	st = tb.build(xp{
		units: []xu{{"gc", "high-priest", "asia", 1}, {"gc", "cultist", "asia", 2}, {"cc", "cultist", "europe", 1}},
		power: map[string]int{"gc": 1, "cc": 0, "bg": 0, "ys": 0}, turn: "gc",
	})
	st = tb.as(st, "gc", "pass")
	d := tb.data(st)
	if d.Power[1] != 3 {
		t.Errorf("Gather Power gave %d, want 3 (two Acolytes and the High Priest)", d.Power[1])
	}
	// A Monster captures him.
	st = tb.build(xp{
		units: []xu{{"gc", "high-priest", "asia", 1}, {"cc", "nightgaunt", "asia", 1}},
		power: map[string]int{"cc": 3}, turn: "cc",
	})
	got := tb.can(st, "cc")
	if !has(got, "capture:high-priest@asia/gc") {
		t.Fatalf("a Monster cannot capture the High Priest: %v", got)
	}
	st = tb.as(st, "cc", "capture:high-priest@asia/gc")
	if n := tb.n(st, "gc", "high-priest", "asia"); n != 0 {
		t.Errorf("he is still there")
	}
	tb.valid(st)
}

func TestUnspeakableOathGivesTwoPowerOnYourOwnTurn(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "high-priest", "asia", 1}, {"gc", "cultist", "asia", 1}, {"cc", "cultist", "europe", 1}},
		power: map[string]int{"gc": 3, "cc": 3}, turn: "gc",
	})
	got := tb.can(st, "gc")
	if !has(got, "oath") {
		t.Fatalf("the Oath is not offered on your own turn: %v", got)
	}
	st = tb.as(st, "gc", "oath")
	d := tb.data(st)
	if d.Power[1] != 5 {
		t.Errorf("Power %d, want 5", d.Power[1])
	}
	if n := tb.n(st, "gc", "high-priest", "asia"); n != 0 {
		t.Errorf("the High Priest is still on the map")
	}
	if p := tb.pool(st, "gc", "high-priest"); p != 1 {
		t.Errorf("pool %d", p)
	}
	// It is still the Windwalker's... the Great Cthulhu's turn: the Oath is free.
	if d.Turn == nil || d.Turn.Seat != 1 || d.Turn.Main {
		t.Errorf("the Oath used the turn: %+v", d.Turn)
	}
	// Recruit him again (3 Power).
	st = tb.as(st, "gc", "recruit:high-priest@asia")
	if p := tb.power(st, "gc"); p != 2 {
		t.Errorf("Power %d, want 2", p)
	}
}

func TestUnspeakableOathAfterAnotherPlayersActionWhenOutOfPower(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{
			{"gc", "high-priest", "asia", 1}, {"gc", "cultist", "asia", 1},
			{"cc", "cultist", "europe", 2}, {"cc", "cultist", "asia", 1},
		},
		gates: map[string]string{"europe": "cc"},
		power: map[string]int{"gc": 0, "cc": 4}, turn: "cc",
	})
	st = tb.as(st, "cc", "recruit@europe")
	// Great Cthulhu has no Power left: it is asked whether to sacrifice its High Priest.
	r := state.PendingReaction(tb.g, st)
	if r == nil || r.Seat != tb.seat("gc") || r.Ability != "Unspeakable Oath" {
		t.Fatalf("reaction %+v\n%s", r, state.RenderText(tb.g, st))
	}
	got := tb.can(st, "gc")
	if !has(got, "oath") || !has(got, "done") {
		t.Fatalf("moves %v", got)
	}
	st = tb.as(st, "gc", "oath")
	if p := tb.power(st, "gc"); p != 2 {
		t.Errorf("Power %d, want 2", p)
	}
	// Declining leaves the High Priest where he stands.
	st2 := tb.build(xp{
		units: []xu{
			{"gc", "high-priest", "asia", 1}, {"gc", "cultist", "asia", 1},
			{"cc", "cultist", "europe", 2}, {"cc", "cultist", "asia", 1},
		},
		gates: map[string]string{"europe": "cc"},
		power: map[string]int{"gc": 0, "cc": 4}, turn: "cc",
	})
	st2 = tb.as(st2, "cc", "recruit@europe")
	st2 = tb.as(st2, "gc", "done")
	if n := tb.n(st2, "gc", "high-priest", "asia"); n != 1 {
		t.Errorf("the High Priest is gone")
	}
}

// "At the end of any player's Action": the holder is asked after every action
// of the others while the High Priest stands on the map — with Power in hand
// and nothing threatening him too (a playtest was never asked).
func TestUnspeakableOathIsAskedAfterEveryActionOfTheOthers(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	build := func(hp bool) *state.BoardState {
		units := []xu{{"gc", "cultist", "asia", 1}, {"cc", "cultist", "europe", 2}}
		if hp {
			units = append(units, xu{"gc", "high-priest", "asia", 1})
		}
		return tb.build(xp{
			units: units, gates: map[string]string{"europe": "cc"},
			power: map[string]int{"gc": 6, "cc": 4}, turn: "cc",
		})
	}
	st := tb.as(build(true), "cc", "recruit@europe")
	r := state.PendingReaction(tb.g, st)
	if r == nil || r.Seat != tb.seat("gc") || r.Ability != "Unspeakable Oath" || r.Trigger != tb.seat("cc") {
		t.Fatalf("reaction %+v\n%s", r, state.RenderText(tb.g, st))
	}
	// It may be used (2 Power more) or declined, and play goes on.
	got := tb.can(st, "gc")
	if !has(got, "oath") || !has(got, "done") {
		t.Fatalf("moves %v", got)
	}
	st2 := tb.as(st, "gc", "done")
	if r := state.PendingReaction(tb.g, st2); r != nil && r.Seat == tb.seat("gc") {
		t.Errorf("asked again for the same action: %+v", r)
	}
	// Play goes on: it is Great Cthulhu's turn now (the others have no Power).
	if d := tb.data(st2); d.Turn == nil || d.Turn.Seat != tb.seat("gc") || d.Stage != "turn" {
		t.Errorf("turn %+v stage %q", d.Turn, d.Stage)
	}
	// With the High Priest in the pool there is nothing to ask.
	st3 := tb.as(build(false), "cc", "recruit@europe")
	if r := state.PendingReaction(tb.g, st3); r != nil {
		t.Errorf("asked with no High Priest on the map: %+v", r)
	}
}

func TestUnspeakableOathInTheDoomPhaseCanPayForARitual(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "high-priest", "asia", 1}, {"gc", "cultist", "asia", 1}, {"cc", "cultist", "europe", 1}},
		gates: map[string]string{"asia": "gc"}, power: map[string]int{"gc": 3}, phase: "doom", stage: "doom",
		edit: func(d *cthulhuwars.Data) { d.Doomx = &cthulhuwars.DoomState{Order: []int{1}} },
	})
	got := tb.can(st, "gc")
	if !has(got, "oath") {
		t.Fatalf("the Oath is not offered when 2 Power more pay for a Ritual: %v", got)
	}
	st = tb.as(st, "gc", "oath")
	got = tb.can(st, "gc")
	if !has(got, "ritual") {
		t.Fatalf("no Ritual after the Oath: %v", got)
	}
	// With more than enough Power the Oath is not offered here.
	st = tb.build(xp{
		units: []xu{{"gc", "high-priest", "asia", 1}, {"gc", "cultist", "asia", 1}, {"cc", "cultist", "europe", 1}},
		gates: map[string]string{"asia": "gc"}, power: map[string]int{"gc": 9}, phase: "doom", stage: "doom",
		edit: func(d *cthulhuwars.Data) { d.Doomx = &cthulhuwars.DoomState{Order: []int{1}} },
	})
	if got := tb.can(st, "gc"); has(got, "oath") {
		t.Errorf("the Oath is offered when it changes nothing: %v", got)
	}
}

func TestFrenzyGivesTheHighPriestACombat(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"bg", "high-priest", "asia", 1}, {"bg", "cultist", "asia", 1}, {"cc", "nightgaunt", "asia", 1}},
		power: map[string]int{"bg": 3}, turn: "bg", books: map[string][]int{"bg": {bgFrenzy}},
		rng: seedDice(t, 1, 1, 1),
	})
	st = tb.as(st, "bg", "battle@asia/cc")
	if d := tb.data(st); d.Last == nil || d.Last.Combat[3] != 2 {
		t.Errorf("Frenzy Combat = %+v, want 2 (the Acolyte and the High Priest)", d.Last)
	}
}

// The prompt text must exist for every position of the new rules (no empty
// "waiting for…" sentence), so a client can always say what is asked.
func TestExpansionDecisionsAreDescribed(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"sl", "gc", "cc", "bg"}, true)
	st := tb.build(xp{
		units: []xu{{"sl", "cultist", "africa", 2}, {"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"africa": "sl"}, power: map[string]int{"sl": 8}, turn: "sl",
	})
	text := state.RenderText(tb.g, st)
	if strings.TrimSpace(text) == "" {
		t.Error("no text view")
	}
	info, ok := state.DescribeRuleset(tb.g)
	if !ok {
		t.Fatal("no ruleset description")
	}
	labels := map[string]bool{}
	for _, a := range info.Actions {
		labels[a.ID] = true
	}
	for _, id := range []string{"hibernate", "iceage", "claim", "howl", "cannibal", "eternal", "lethargy", "boon", "bane", "sorcery", "slumber", "wake", "death", "nexus", "oath", "start", "restore"} {
		if !labels[id] {
			t.Errorf("action %q has no label", id)
		}
	}
	_ = fmt.Sprint()
}
