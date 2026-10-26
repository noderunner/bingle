package cthulhuwars_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The neutral content of the Azathoth faction: six spellbooks and four
// Monsters, wanted at the table as an option, and — without the faction in the
// game — as a replacement for the books that need a clock.

// Neutral spellbook numbers (the bits of Data.Neutral).
const (
	nbShrivel = iota
	nbStars
	nbRecrim
	nbMao
	nbUmr
	nbUndim
)

// azTable derives a game with the Azathoth faction (or not) and the swaps.
func azTable(t testing.TB, base string, keys []string, az bool, swaps map[string]string) *table {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	var l []any
	for _, k := range keys {
		l = append(l, k)
	}
	opts := map[string]any{"factions": l}
	if az {
		opts["azathoth"] = true
	}
	if len(swaps) > 0 {
		m := map[string]any{}
		for k, v := range swaps {
			m[k] = v
		}
		opts["book-swaps"] = m
	}
	id, err := cat.Derive(base, opts)
	if err != nil {
		t.Fatalf("derive %v az=%v swaps=%v: %v", keys, az, swaps, err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("derived game %q is not registered", id)
	}
	tb := &table{t: t, g: g, keys: keys, colour: map[string]string{}}
	for i, k := range keys {
		tb.colour[k] = g.Definition.Seats[i].Color
	}
	return tb
}

var fourCults = []string{"gc", "cc", "bg", "ys"}

func TestAzathothOptionsNameTheGame(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	id, err := cat.Derive("cthulhu-wars", map[string]any{
		"factions": []any{"gc", "cc", "bg", "ys"}, "azathoth": true,
		"book-swaps": map[string]any{"cc": "mao", "bg": "shrivel"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "cthulhu-wars~gc-cc-bg-ys-az-bg.shrivel-cc.mao" {
		t.Errorf("id %q", id)
	}
	g, ok := cat.Get(id)
	if !ok || !strings.Contains(g.Definition.Name, "Azathoth") {
		t.Fatalf("the derived game does not load: %v", ok)
	}
	if g.Definition.Params["azathoth"] != true {
		t.Errorf("params %v", g.Definition.Params)
	}
	// The swap alone needs no Azathoth faction.
	id2, err := cat.Derive("cthulhu-wars", map[string]any{"factions": []any{"gc", "cc", "bg", "ys"}, "book-swaps": map[string]any{"bg": "umr"}})
	if err != nil || id2 != "cthulhu-wars~gc-cc-bg-ys-bg.umr" {
		t.Errorf("swap only: %q %v", id2, err)
	}
	// A swap for a cult that is not at the table is left out.
	id3, err := cat.Derive("cthulhu-wars", map[string]any{"factions": []any{"gc", "cc", "ys", "ww"}, "book-swaps": map[string]any{"bg": "umr"}})
	if err != nil || strings.Contains(id3, "umr") {
		t.Errorf("a swap for an absent cult: %q %v", id3, err)
	}
	for name, opts := range map[string]map[string]any{
		"unknown book": {"book-swaps": map[string]any{"bg": "ghroth"}},
		"wrong cult":   {"book-swaps": map[string]any{"gc": "mao"}},
		"same book":    {"book-swaps": map[string]any{"bg": "mao", "cc": "mao"}},
		"not a bool":   {"azathoth": "yes"},
	} {
		opts["factions"] = []any{"gc", "cc", "bg", "ys"}
		if _, err := cat.Derive("cthulhu-wars", opts); err == nil {
			t.Errorf("%s: the options were accepted", name)
		}
	}
	// The id loads again by itself.
	if _, ok := cat.Get("cthulhu-wars~gc-cc-bg-ys-az-bg.shrivel-cc.mao"); !ok {
		t.Error("the derived id does not load")
	}
	// Changing a cult in the lobby keeps the neutral options.
	opts, ok, err := state.SeatCultOptions(g, 4, "ww", map[int]bool{})
	if err != nil || !ok {
		t.Fatalf("changing a cult: %v %v", ok, err)
	}
	if opts["azathoth"] != true {
		t.Errorf("the Azathoth option was lost: %v", opts)
	}
	swaps, _ := opts["book-swaps"].(map[string]any)
	if swaps["bg"] != "shrivel" || swaps["cc"] != "mao" || len(swaps) != 2 {
		t.Errorf("the swaps after Yellow Sign gave way to Windwalker: %v", swaps)
	}
}

func TestASwappedTimerBookIsReplacedByTheNeutralOne(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, false, map[string]string{"bg": "shrivel", "cc": "undim"})
	bgSeat := tb.seat("bg")
	// Black Goat has met a requirement: the choice offers its five other books
	// and Shriveling, never Ghroth, and no other neutral book (no Azathoth).
	st := tb.build(xp{stage: "prompt", edit: func(d *cthulhuwars.Data) {
		d.Prompts = []cthulhuwars.Prompt{{Kind: "book", Seat: bgSeat}}
		d.Resume = "turn"
		d.Turn = &cthulhuwars.TurnState{Seat: bgSeat}
	}})
	got := tb.can(st, "bg")
	if len(got) != 6 || !has(got, "book:shriveling") || has(got, "book:ghroth") || has(got, "book:umr-at-tawil") || has(got, "book:undimensioned") {
		t.Fatalf("book choices %v", got)
	}
	// Crawling Chaos: The Thousand Forms is out, Undimensioned in.
	st2 := tb.build(xp{stage: "prompt", edit: func(d *cthulhuwars.Data) {
		d.Prompts = []cthulhuwars.Prompt{{Kind: "book", Seat: tb.seat("cc")}}
		d.Resume = "turn"
		d.Turn = &cthulhuwars.TurnState{Seat: tb.seat("cc")}
	}})
	got = tb.can(st2, "cc")
	if len(got) != 6 || !has(got, "book:undimensioned") || has(got, "book:the-thousand-forms") {
		t.Fatalf("Crawling Chaos' choices %v", got)
	}
	for _, g := range got {
		if strings.Contains(g, "forms") {
			t.Errorf("a Thousand Forms choice: %q", g)
		}
	}
	// Taking it counts as a spellbook, and Ghroth cannot be used.
	st = tb.as(st, "bg", "book:shriveling")
	d := tb.data(st)
	if d.Neutral[bgSeat] != 1<<nbShrivel || d.Books[bgSeat] != 0 {
		t.Errorf("books %b neutral %b", d.Books[bgSeat], d.Neutral[bgSeat])
	}
	// The six books of the card: five own and the neutral one.
	own := []int{0, 1, 3, 4, 5}
	st = tb.build(xp{books: map[string][]int{"bg": own}, edit: func(d *cthulhuwars.Data) { d.Neutral[bgSeat] = 1 << nbShrivel }})
	if n := tb.data(st).Neutral[bgSeat]; n == 0 {
		t.Fatal("no neutral book")
	}
}

func TestWithAzathothAnyCultMayTakeAFreeNeutralBook(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, map[string]string{"bg": "shrivel"})
	gcSeat := tb.seat("gc")
	st := tb.build(xp{stage: "prompt", edit: func(d *cthulhuwars.Data) {
		d.Prompts = []cthulhuwars.Prompt{{Kind: "book", Seat: gcSeat}}
		d.Resume = "turn"
		d.Turn = &cthulhuwars.TurnState{Seat: gcSeat}
	}})
	got := tb.can(st, "gc")
	// Six of its own, five neutral (Shriveling is set aside for Black Goat).
	if len(got) != 11 || has(got, "book:shriveling") || !has(got, "book:mao-ceremony") || !has(got, "book:recriminations") {
		t.Fatalf("choices %v", got)
	}
	st = tb.as(st, "gc", "book:umr-at-tawil")
	if n := tb.data(st).Neutral[gcSeat]; n != 1<<nbUmr {
		t.Fatalf("neutral %b", n)
	}
	// A neutral book can be held by one cult only.
	st = tb.build(xp{stage: "prompt", edit: func(d *cthulhuwars.Data) {
		d.Neutral[gcSeat] = 1 << nbUmr
		d.Prompts = []cthulhuwars.Prompt{{Kind: "book", Seat: tb.seat("ys")}}
		d.Resume = "turn"
		d.Turn = &cthulhuwars.TurnState{Seat: tb.seat("ys")}
	}})
	if got := tb.can(st, "ys"); has(got, "book:umr-at-tawil") || len(got) != 10 {
		t.Errorf("another cult's choices %v", got)
	}
}

func TestUmrAtTawilMakesGatesCostTwo(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	bgSeat := tb.seat("bg")
	build := func(umr bool) *state.BoardState {
		return tb.build(xp{
			units: []xu{{"bg", "cultist", "europe", 1}},
			power: map[string]int{"bg": 3, "ys": 5}, turn: "bg",
			edit: func(d *cthulhuwars.Data) {
				if umr {
					d.Neutral[bgSeat] = 1 << nbUmr
				}
			},
		})
	}
	st := build(false)
	st = tb.as(st, "bg", "gate@europe")
	if p := tb.power(st, "bg"); p != 0 {
		t.Errorf("a Gate cost %d Power, want 3", 3-p)
	}
	st = build(true)
	st = tb.as(st, "bg", "gate@europe")
	if p := tb.power(st, "bg"); p != 1 {
		t.Errorf("with Umr at-Tawil a Gate cost %d Power, want 2", 3-p)
	}
}

func TestStarsAreRightPaysPowerForElderSignsInTheDoomPhase(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	ccSeat := tb.seat("cc")
	st := tb.build(xp{phase: "doom", stage: "doom", signs: map[string][]int{"cc": {2, 3}}, edit: func(d *cthulhuwars.Data) {
		d.Neutral[ccSeat] = 1 << nbStars
		d.Doomx = &cthulhuwars.DoomState{Order: []int{ccSeat}}
	}})
	st = tb.as(st, "cc", "reveal")
	if p, doom := tb.power(st, "cc"), tb.data(st).Doom[ccSeat]; p != 5 || doom != 5 {
		t.Errorf("Power %d and Doom %d after revealing 2 and 3, want 5 and 5", p, doom)
	}
	// Without the book the Doom comes alone.
	st = tb.build(xp{phase: "doom", stage: "doom", signs: map[string][]int{"cc": {2, 3}}, edit: func(d *cthulhuwars.Data) {
		d.Doomx = &cthulhuwars.DoomState{Order: []int{ccSeat}}
	}})
	st = tb.as(st, "cc", "reveal")
	if p := tb.power(st, "cc"); p != 0 {
		t.Errorf("Power %d without the book", p)
	}
}

func TestShrivelingEliminatesAMonsterAndPaysItsCost(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	bgSeat := tb.seat("bg")
	st := tb.build(xp{
		units: []xu{{"bg", "fungi", "europe", 1}, {"gc", "shoggoth", "europe", 1}, {"gc", "deep-one", "europe", 1}},
		power: map[string]int{"bg": 4, "gc": 0}, turn: "bg", rng: seedDice(t, 1, 1, 1, 1, 1, 1),
		edit: func(d *cthulhuwars.Data) { d.Neutral[bgSeat] = 1 << nbShrivel },
	})
	st = tb.as(st, "bg", "battle@europe/gc")
	got := tb.can(st, "bg")
	if !has(got, "shrivel:shoggoth") || !has(got, "shrivel:deep-one") {
		t.Fatalf("pre-battle moves %v", got)
	}
	st = tb.as(st, "bg", "shrivel:shoggoth")
	if n := tb.n(st, "gc", "shoggoth", "europe"); n != 0 {
		t.Errorf("the Shoggoth is still there")
	}
	if p := tb.power(st, "gc"); p != 2 {
		t.Errorf("Great Cthulhu received %d Power for a Shoggoth, want 2 (its cost)", p)
	}
	if got := tb.can(st, "bg"); has(got, "shrivel:deep-one") {
		t.Errorf("Shriveling twice in one battle: %v", got)
	}
	tb.valid(st)
}

func TestRecriminationsSwapsABook(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	gcSeat := tb.seat("gc")
	st := tb.build(xp{
		power: map[string]int{"gc": 3, "ys": 5}, turn: "gc", books: map[string][]int{"gc": {0}},
		edit: func(d *cthulhuwars.Data) { d.Neutral[gcSeat] = 1 << nbRecrim },
	})
	got := tb.can(st, "gc")
	if !has(got, "recriminate:absorb") || !has(got, "recriminate:recriminations") {
		t.Fatalf("moves %v", got)
	}
	st = tb.as(st, "gc", "recriminate:absorb")
	d := tb.data(st)
	if d.Books[gcSeat] != 0 || d.Power[gcSeat] != 2 {
		t.Errorf("books %b power %d", d.Books[gcSeat], d.Power[gcSeat])
	}
	// The replacement is a different book.
	got = tb.can(st, "gc")
	if has(got, "book:absorb") || !has(got, "book:devolve") || !has(got, "book:umr-at-tawil") {
		t.Fatalf("replacements %v", got)
	}
	st = tb.as(st, "gc", "book:devolve")
	if d = tb.data(st); d.Books[gcSeat] != 1<<1 || d.Neutral[gcSeat] != 1<<nbRecrim {
		t.Errorf("books %b neutral %b", d.Books[gcSeat], d.Neutral[gcSeat])
	}
	if d.Turn == nil || d.Turn.Seat != tb.seat("ys") {
		t.Errorf("Recriminations is an action, so the turn passes on: %+v", d.Turn)
	}
	// It can replace itself.
	st = tb.build(xp{power: map[string]int{"gc": 3, "ys": 5}, turn: "gc", edit: func(d *cthulhuwars.Data) { d.Neutral[gcSeat] = 1 << nbRecrim }})
	st = tb.as(st, "gc", "recriminate:recriminations")
	if got := tb.can(st, "gc"); has(got, "book:recriminations") {
		t.Errorf("the book that was set aside is offered again: %v", got)
	}
}

func TestTheMaoCeremonyTradesCultistsForPowerAtTheEndOfGatherPower(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	ysSeat := tb.seat("ys")
	st := tb.build(xp{
		units: []xu{{"ys", "cultist", "europe", 3}, {"gc", "cultist", "asia", 1}},
		gates: map[string]string{"europe": "ys"},
		power: map[string]int{"gc": 1}, turn: "gc",
		edit: func(d *cthulhuwars.Data) { d.Neutral[ysSeat] = 1 << nbMao },
	})
	// Everybody but Great Cthulhu is out of Power: it passes, Power is gathered.
	st = tb.as(st, "gc", "pass")
	d := tb.data(st)
	if d.Stage != "prompt" || len(d.Prompts) == 0 || d.Prompts[0].Kind != "mao" || d.Prompts[0].Seat != ysSeat {
		t.Fatalf("stage %q prompts %+v", d.Stage, d.Prompts)
	}
	before := d.Power[ysSeat]
	got := tb.can(st, "ys")
	if !has(got, "mao:cultist@europe") || !has(got, "done") {
		t.Fatalf("moves %v", got)
	}
	st = tb.as(st, "ys", "mao:cultist@europe")
	st = tb.as(st, "ys", "mao:cultist@europe")
	d = tb.data(st)
	if d.Power[ysSeat] != before+2 {
		t.Errorf("Power %d, want %d", d.Power[ysSeat], before+2)
	}
	if n := tb.n(st, "ys", "cultist", "europe"); n != 1 {
		t.Errorf("%d Cultists left in Europe, want 1", n)
	}
	// Done closes the window and play goes on to the direction of play.
	st = tb.as(st, "ys", "done")
	if d = tb.data(st); d.Stage == "prompt" {
		t.Errorf("still asking: %+v", d.Prompts)
	}
	tb.valid(st)
}

func TestUndimensionedRearrangesUnitsWithinTheirAreas(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	gcSeat := tb.seat("gc")
	st := tb.build(xp{
		units: []xu{{"gc", "deep-one", "europe", 2}, {"gc", "shoggoth", "asia", 1}, {"gc", "cultist", "africa", 1}},
		power: map[string]int{"gc": 5, "ys": 5}, turn: "gc",
		edit: func(d *cthulhuwars.Data) { d.Neutral[gcSeat] = 1 << nbUndim },
	})
	st = tb.as(st, "gc", "undimensioned")
	d := tb.data(st)
	if d.Power[gcSeat] != 3 || d.Stage != "prompt" || d.Prompts[0].Kind != "undim" {
		t.Fatalf("power %d stage %q prompts %+v", d.Power[gcSeat], d.Stage, d.Prompts)
	}
	got := tb.can(st, "gc")
	if !has(got, "rearrange:deep-one@europe>asia") || !has(got, "rearrange:shoggoth@asia>africa") || !has(got, "done") {
		t.Fatalf("moves %v", got)
	}
	// No new areas.
	for _, m := range got {
		if strings.Contains(m, ">north-america") || strings.Contains(m, ">south-pacific") {
			t.Errorf("a new area: %q", m)
		}
	}
	st = tb.as(st, "gc", "rearrange:deep-one@europe>asia")
	st = tb.as(st, "gc", "rearrange:deep-one@europe>asia")
	if tb.n(st, "gc", "deep-one", "asia") != 2 || tb.n(st, "gc", "deep-one", "europe") != 0 {
		t.Errorf("the Deep Ones did not move")
	}
	// An area may be emptied, and Done ends the action.
	st = tb.as(st, "gc", "done")
	if d = tb.data(st); d.Stage == "prompt" || d.Turn == nil || d.Turn.Seat != tb.seat("ys") {
		t.Errorf("stage %q turn %+v", d.Stage, d.Turn)
	}
	tb.valid(st)
}

func TestUndimensionedLeavesTheUnitOnAGate(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	gcSeat := tb.seat("gc")
	st := tb.build(xp{
		units: []xu{{"gc", "cultist", "europe", 1}, {"gc", "cultist", "asia", 1}},
		gates: map[string]string{"europe": "gc"},
		power: map[string]int{"gc": 5, "ys": 5}, turn: "gc",
		edit: func(d *cthulhuwars.Data) {
			d.Neutral[gcSeat] = 1 << nbUndim
			d.Holders = map[string]string{"europe": "cultist"}
		},
	})
	st = tb.as(st, "gc", "undimensioned")
	got := tb.can(st, "gc")
	if has(got, "rearrange:cultist@europe>asia") {
		t.Errorf("the Cultist on the Gate left it: %v", got)
	}
	if !has(got, "rearrange:cultist@asia>europe") {
		t.Errorf("the Cultist beside the Gate cannot move: %v", got)
	}
}

func TestLoyaltyCardsAreBoughtWithDoomInTheDoomPhase(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	bgSeat := tb.seat("bg")
	doomPhase := func(doom int, units ...xu) *state.BoardState {
		return tb.build(xp{
			phase: "doom", stage: "doom", turn: "bg", units: units,
			gates: map[string]string{"europe": "bg"}, doom: map[string]int{"bg": doom},
			edit: func(d *cthulhuwars.Data) { d.Doomx = &cthulhuwars.DoomState{Order: []int{bgSeat}} },
		})
	}
	st := doomPhase(1, xu{"bg", "cultist", "europe", 1})
	if got := tb.can(st, "bg"); hasPrefix(got, "buy:") {
		t.Errorf("a card for 1 Doom: %v", got)
	}
	st = doomPhase(5, xu{"bg", "cultist", "europe", 1})
	got := tb.can(st, "bg")
	for _, want := range []string{"buy:dimensional-shambler", "buy:star-vampire@europe", "buy:elder-thing@europe", "buy:servitor/gc", "buy:servitor/cc", "buy:servitor/ys"} {
		if !has(got, want) {
			t.Errorf("%q is not offered: %v", want, got)
		}
	}
	// A Star Vampire appears at the Gate; the card is Black Goat's, three in all.
	st = tb.as(st, "bg", "buy:star-vampire@europe")
	d := tb.data(st)
	if d.Doom[bgSeat] != 3 || d.Loyal[bgSeat] != 1<<1 {
		t.Errorf("Doom %d loyal %b", d.Doom[bgSeat], d.Loyal[bgSeat])
	}
	if tb.n(st, "bg", "star-vampire", "europe") != 1 || tb.pool(st, "bg", "star-vampire") != 2 {
		t.Errorf("the Star Vampire: %d in Europe, %d in the pool", tb.n(st, "bg", "star-vampire", "europe"), tb.pool(st, "bg", "star-vampire"))
	}
	// One card a Doom phase.
	if got := tb.can(st, "bg"); hasPrefix(got, "buy:") {
		t.Errorf("a second card: %v", got)
	}
	tb.valid(st)
}

func TestShamblersWaitOnTheCardAndWalkAfterAnyAction(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	bgSeat, gcSeat := tb.seat("bg"), tb.seat("gc")
	st := tb.build(xp{
		phase: "doom", stage: "doom", turn: "bg", units: []xu{{"bg", "cultist", "europe", 1}},
		gates: map[string]string{"europe": "bg"}, doom: map[string]int{"bg": 4},
		edit: func(d *cthulhuwars.Data) { d.Doomx = &cthulhuwars.DoomState{Order: []int{bgSeat}} },
	})
	st = tb.as(st, "bg", "buy:dimensional-shambler")
	d := tb.data(st)
	if d.Card[bgSeat] != 1 || d.Loyal[bgSeat] != 1<<0 || tb.pool(st, "bg", "dimensional-shambler") != 2 {
		t.Fatalf("card %d loyal %b pool %d", d.Card[bgSeat], d.Loyal[bgSeat], tb.pool(st, "bg", "dimensional-shambler"))
	}
	tb.valid(st)
	// In the Action phase another player's action opens the window for the owner.
	st = tb.build(xp{
		units: []xu{{"bg", "cultist", "europe", 1}, {"gc", "cultist", "asia", 1}}, gates: map[string]string{"europe": "bg"},
		power: map[string]int{"gc": 3, "bg": 3}, turn: "gc",
		edit: func(d *cthulhuwars.Data) {
			d.Loyal[bgSeat] = 1 << 0
			d.Card[bgSeat] = 2
		},
	})
	st.SetReserve(tb.colour["bg"], "dimensional-shambler", 1)
	st = tb.as(st, "gc", "recruit@asia")
	d = tb.data(st)
	if d.Stage != "prompt" || d.Prompts[0].Kind != "react" || d.Prompts[0].Why != "walk" || d.Prompts[0].Seat != bgSeat {
		t.Fatalf("stage %q prompts %+v (gc %d)", d.Stage, d.Prompts, gcSeat)
	}
	st = tb.as(st, "bg", "walk@asia")
	st = tb.as(st, "bg", "walk@europe")
	if tb.n(st, "bg", "dimensional-shambler", "asia") != 1 || tb.n(st, "bg", "dimensional-shambler", "europe") != 1 {
		t.Errorf("the Shamblers did not arrive")
	}
	if d = tb.data(st); d.Card[bgSeat] != 0 || d.Stage == "prompt" {
		t.Errorf("card %d stage %q", d.Card[bgSeat], d.Stage)
	}
	tb.valid(st)
}

func TestServitorCardGoesToAnotherPlayerWhoMayOnlySummonServitors(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	bgSeat, ysSeat := tb.seat("bg"), tb.seat("ys")
	st := tb.build(xp{
		phase: "doom", stage: "doom", turn: "bg", units: []xu{{"bg", "cultist", "europe", 1}},
		gates: map[string]string{"europe": "bg"}, doom: map[string]int{"bg": 4},
		edit: func(d *cthulhuwars.Data) { d.Doomx = &cthulhuwars.DoomState{Order: []int{bgSeat}} },
	})
	st = tb.as(st, "bg", "buy:servitor/ys")
	d := tb.data(st)
	if d.Loyal[ysSeat] != 1<<3 || d.Loyal[bgSeat] != 0 || d.Doom[bgSeat] != 2 {
		t.Fatalf("loyal ys %b bg %b, Doom %d", d.Loyal[ysSeat], d.Loyal[bgSeat], d.Doom[bgSeat])
	}
	if tb.pool(st, "ys", "servitor") != 3 {
		t.Errorf("the Servitors went to %d", tb.pool(st, "ys", "servitor"))
	}
	// Yellow Sign: Adulation — only Servitors while some are in the pool.
	st = tb.build(xp{
		units: []xu{{"ys", "cultist", "asia", 1}}, gates: map[string]string{"asia": "ys"},
		power: map[string]int{"ys": 6}, turn: "ys",
		edit: func(d *cthulhuwars.Data) { d.Loyal[ysSeat] = 1 << 3 },
	})
	st.SetReserve(tb.colour["ys"], "servitor", 3)
	got := tb.can(st, "ys")
	if !has(got, "summon:servitor@asia") || hasPrefix(got, "summon:undead") || hasPrefix(got, "summon:byakhee") {
		t.Fatalf("moves %v", got)
	}
	// Once the Servitors are all out the other Monsters come back.
	st = tb.build(xp{
		units: []xu{{"ys", "cultist", "asia", 1}, {"ys", "servitor", "asia", 3}}, gates: map[string]string{"asia": "ys"},
		power: map[string]int{"ys": 6}, turn: "ys",
		edit: func(d *cthulhuwars.Data) { d.Loyal[ysSeat] = 1 << 3 },
	})
	st.SetReserve(tb.colour["ys"], "servitor", 0)
	if got := tb.can(st, "ys"); !hasPrefix(got, "summon:undead") {
		t.Errorf("moves %v", got)
	}
}

func TestStarVampireDrainsPowerAndDoomWithItsPainsAndKills(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	bgSeat, gcSeat := tb.seat("bg"), tb.seat("gc")
	// Three Star Vampires roll 6, 5, 4 (one Kill, two Pains); the Deep One 1.
	st := tb.build(xp{
		units: []xu{{"bg", "star-vampire", "europe", 3}, {"gc", "deep-one", "europe", 1}},
		power: map[string]int{"bg": 3, "gc": 5}, doom: map[string]int{"gc": 4, "bg": 1}, turn: "bg",
		rng:  seedDice(t, 6, 5, 4, 1),
		edit: func(d *cthulhuwars.Data) { d.Loyal[bgSeat] = 1 << 1 },
	})
	st.SetReserve(tb.colour["bg"], "star-vampire", 0)
	st = tb.as(st, "bg", "battle@europe/gc")
	d := tb.data(st)
	if d.Power[gcSeat] != 3 || d.Doom[gcSeat] != 3 {
		t.Errorf("Great Cthulhu has Power %d, Doom %d; want 3 and 3", d.Power[gcSeat], d.Doom[gcSeat])
	}
	// 3 − 1 (the battle) + 2 drained.
	if d.Power[bgSeat] != 4 || d.Doom[bgSeat] != 2 {
		t.Errorf("Black Goat has Power %d, Doom %d; want 4 and 2", d.Power[bgSeat], d.Doom[bgSeat])
	}
}

func TestServitorsCostTheirOwnerADie(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	ysSeat := tb.seat("ys")
	// Two Byakhee (2 + 1 = 3 dice) and a Servitor: 2 dice.
	st := tb.build(xp{
		units: []xu{{"ys", "byakhee", "europe", 2}, {"ys", "servitor", "europe", 1}, {"gc", "deep-one", "europe", 1}},
		power: map[string]int{"ys": 4}, turn: "ys", rng: seedDice(t, 1, 1, 1, 1),
		edit: func(d *cthulhuwars.Data) { d.Loyal[ysSeat] = 1 << 3 },
	})
	st.SetReserve(tb.colour["ys"], "servitor", 2)
	st = tb.as(st, "ys", "battle@europe/gc")
	last := tb.data(st).Last
	if last == nil || last.Combat[ysSeat] != 2 {
		t.Fatalf("Yellow Sign rolled for %+v, want 2 dice", last)
	}
}

func TestElderThingSilencesAnEnemyGreatOldOne(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	bgSeat := tb.seat("bg")
	build := func(thing bool) *state.BoardState {
		units := []xu{{"bg", "fungi", "europe", 1}, {"gc", "cthulhu", "europe", 1}}
		if thing {
			units = append(units, xu{"bg", "elder-thing", "europe", 1})
		}
		st := tb.build(xp{
			units: units, power: map[string]int{"bg": 4}, turn: "bg", rng: seedDice(t, 1, 1, 1, 1, 1, 1, 1, 1),
			edit: func(d *cthulhuwars.Data) { d.Loyal[bgSeat] = 1 << 2 },
		})
		st.SetReserve(tb.colour["bg"], "elder-thing", 2)
		return st
	}
	// Without it Cthulhu devours the Fungi as the battle opens.
	st := tb.as(build(false), "bg", "battle@europe/gc")
	if n := tb.n(st, "bg", "fungi", "europe"); n != 0 {
		t.Fatalf("no Devour: %d Fungi left", n)
	}
	// With an Elder Thing in the area it cannot: nothing is eliminated.
	st = tb.as(build(true), "bg", "battle@europe/gc")
	if tb.n(st, "bg", "fungi", "europe") != 1 || tb.n(st, "bg", "elder-thing", "europe") != 1 {
		t.Errorf("Devour with an Elder Thing in the area: %d Fungi, %d Elder Things", tb.n(st, "bg", "fungi", "europe"), tb.n(st, "bg", "elder-thing", "europe"))
	}
	if d := tb.data(st); d.Stage == "prompt" && d.Prompts[0].Why == "devour" {
		t.Errorf("a Devour prompt: %+v", d.Prompts)
	}
}

// Random games with the Azathoth faction stay valid and finish; the swaps and
// the neutral content are used along the way.
func TestRandomGamesWithAzathoth(t *testing.T) {
	tables := []struct {
		base  string
		keys  []string
		az    bool
		swaps map[string]string
	}{
		{"cthulhu-wars", fourCults, true, nil},
		{"cthulhu-wars", fourCults, true, map[string]string{"bg": "shrivel", "cc": "mao"}},
		{"cthulhu-wars", fourCults, false, map[string]string{"bg": "umr", "cc": "stars"}},
		{"cthulhu-wars-5p", []string{"sl", "ww", "gc", "cc", "bg"}, true, map[string]string{"cc": "undim"}},
	}
	seeds := 3
	if testing.Short() {
		seeds = 1
	}
	for _, c := range tables {
		tb := azTable(t, c.base, c.keys, c.az, c.swaps)
		for seed := int64(1); seed <= int64(seeds); seed++ {
			name := fmt.Sprintf("%s seed %d", tb.g.Definition.ID, seed)
			st, ply, done := randomGame(t, tb.g, seed, 8000, func(ply int, st *state.BoardState, seat int, mv state.Move) {
				if err := state.Validate(tb.g, st); err != nil {
					t.Fatalf("%s ply %d after %s: %v\n%s", name, ply, state.FormatMove(tb.g, mv), err, state.RenderText(tb.g, st))
				}
			})
			if !done {
				t.Errorf("%s: no end after %d plies\n%s", name, ply, state.RenderText(tb.g, st))
			}
		}
	}
}

// The guide plays tables with the neutral content legally and finishes games.
func TestGuidedGamesWithAzathoth(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, map[string]string{"bg": "shrivel", "cc": "umr"})
	finished := 0
	for seed := int64(1); seed <= 4; seed++ {
		stats := playGuided(t, tb.g, seed, 6000)
		if stats.ended {
			finished++
		}
	}
	if finished < 2 {
		t.Errorf("only %d of 4 guided games finished", finished)
	}
}

// "A Neutral Spellbook counts as one of the 6 Spellbooks needed on your Faction
// Card to allow you to perform Unlimited Battle and win the game, as well as
// counting for any other rules or abilities that reference your Faction Card's
// Spellbooks (such as Nyarlathotep's Combat rating or the Moonbeasts' ability)."
func TestANeutralSpellbookCountsAsOneOfTheSix(t *testing.T) {
	tb := azTable(t, "cthulhu-wars", fourCults, true, nil)
	ccSeat := tb.seat("cc")
	// Five own books and a neutral one: Battle is unlimited, taken after the
	// turn's main action.
	build := func(neutral bool) *state.BoardState {
		return tb.build(xp{
			units: []xu{{who: "cc", typ: "flying-polyp", area: "asia", n: 2}, {who: "gc", typ: "cultist", area: "asia"}},
			power: map[string]int{"cc": 5}, turn: "cc", main: true,
			books: map[string][]int{"cc": {0, 1, 2, 3, 4}},
			edit: func(d *cthulhuwars.Data) {
				if neutral {
					d.Neutral[ccSeat] |= 1 << nbMao
				}
			},
		})
	}
	if got := tb.can(build(false), "cc"); has(got, "battle@asia/gc") {
		t.Fatalf("five books are not Unlimited Battle: %v", got)
	}
	got := tb.can(build(true), "cc")
	if !has(got, "battle@asia/gc") {
		t.Errorf("five books and a neutral one are six: %v", got)
	}
	// Nyarlathotep rolls one die for each spellbook on the cards of both sides,
	// a neutral one included.
	st := tb.build(xp{
		units: []xu{{who: "cc", typ: "nyarlathotep", area: "asia"}, {who: "gc", typ: "cultist", area: "asia"}},
		power: map[string]int{"cc": 5}, turn: "cc",
		books: map[string][]int{"cc": {0, 1}, "gc": {0}},
		edit:  func(d *cthulhuwars.Data) { d.Neutral[ccSeat] |= 1 << nbUmr; d.Neutral[tb.seat("gc")] |= 1 << nbShrivel },
		rng:   seedDice(t, 1, 1, 1, 1, 1),
	})
	st = tb.as(st, "cc", "battle@asia/gc")
	if d := tb.data(st); d.Last == nil || d.Last.Combat[ccSeat] != 5 {
		t.Errorf("2 + 1 own books and 1 + 1 of the enemy's: %+v", d.Last)
	}
}
