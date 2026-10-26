// Package cthulhuwars_test checks the Cthulhu Wars ruleset: setup, the
// action phase, gathering Power, battles, spellbooks, the Doom phase and the
// engine contract.
package cthulhuwars_test

import (
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

func game(t testing.TB) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get("cthulhu-wars")
	if !ok {
		t.Fatal("cthulhu-wars is not registered")
	}
	return g
}

// randomGame plays random legal decisions and calls check after each.
func randomGame(t testing.TB, g *catalog.CompiledGame, seed int64, maxPlies int, check func(ply int, st *state.BoardState, seat int, mv state.Move)) (*state.BoardState, int, bool) {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	st, err := state.InitialStateSeeded(g, uint64(seed*7919+1))
	if err != nil {
		t.Fatal(err)
	}
	seat := state.FirstSeat(g)
	for ply := 0; ply < maxPlies; ply++ {
		legal := state.LegalMoves(g, st, seat)
		if len(legal) == 0 {
			t.Fatalf("seed %d ply %d: seat %d has no legal moves\n%s", seed, ply, seat, state.RenderText(g, st))
		}
		mv := legal[rng.Intn(len(legal))]
		res, err := state.Apply(g, st, seat, mv)
		if err != nil {
			t.Fatalf("seed %d ply %d: legal move %s rejected: %v\n%s", seed, ply, state.FormatMove(g, mv), err, state.RenderText(g, st))
		}
		if check != nil {
			check(ply, res.NewState, seat, res.Move)
		}
		if res.GameEnded {
			return res.NewState, ply, true
		}
		st, seat = res.NewState, res.NextTurnSeat
	}
	return st, maxPlies, false
}

// --- positions ------------------------------------------------------------------

var unitCount = map[string]int{
	"cultist": 6, "deep-one": 4, "shoggoth": 2, "starspawn": 2, "cthulhu": 1,
	"nightgaunt": 3, "flying-polyp": 3, "hunting-horror": 2, "nyarlathotep": 1,
	"ghoul": 2, "fungi": 4, "dark-young": 3, "shub-niggurath": 1,
	"undead": 6, "byakhee": 4, "king-in-yellow": 1, "hastur": 1,
}

var factionUnits = map[int][]string{
	1: {"cultist", "deep-one", "shoggoth", "starspawn", "cthulhu"},
	2: {"cultist", "nightgaunt", "flying-polyp", "hunting-horror", "nyarlathotep"},
	3: {"cultist", "ghoul", "fungi", "dark-young", "shub-niggurath"},
	4: {"cultist", "undead", "byakhee", "king-in-yellow", "hastur"},
}

var colors = [5]string{"", "green", "blue", "red", "yellow"}

const (
	gc = 1
	cc = 2
	bg = 3
	ys = 4
)

// book bits per faction, matching the spellbook order of each faction card.
func books(idx ...int) uint8 {
	var m uint8
	for _, i := range idx {
		m |= 1 << uint(i)
	}
	return m
}

type u struct {
	seat int
	typ  string
	area string
	n    int
}

// pos describes a position; everything defaults to the start of round 1
// with each cult's Cultists gone (pools full) unless units say otherwise.
type pos struct {
	units    []u
	gates    map[string]int
	desec    []string
	power    [5]int
	doom     [5]int
	books    [5]uint8
	reqs     [5]uint8
	stage    string
	turn     int
	main     bool
	phase    string
	round    int
	first    int
	dir      int
	rng      uint64
	ritual   int
	signs    map[int][]int
	captured [5][5]int
	away     map[string]int
	awoken   map[string]bool
	prompts  []cthulhuwars.Prompt
	bat      *cthulhuwars.Battle
	doomx    *cthulhuwars.DoomState
}

func (p pos) build() *state.BoardState {
	st := state.NewBoardState()
	placed := map[string]int{}
	for _, x := range p.units {
		for i := 0; i < x.n || (x.n == 0 && i < 1); i++ {
			st.Push(boardID(x.area), state.Occupant{Piece: x.typ, Color: colors[x.seat]})
			placed[colors[x.seat]+"|"+x.typ]++
		}
	}
	for seat := 1; seat <= 4; seat++ {
		for _, typ := range factionUnits[seat] {
			n := unitCount[typ] - placed[colors[seat]+"|"+typ]
			if typ == "cultist" {
				for by := 1; by <= 4; by++ {
					n -= p.captured[by][seat]
				}
			}
			if seat == 1 {
				n -= p.away[typ]
			}
			if n < 0 {
				n = 0
			}
			st.SetReserve(colors[seat], typ, n)
		}
	}
	d := &cthulhuwars.Data{
		Phase: orStr(p.phase, "action"), Round: orInt(p.round, 1), First: orInt(p.first, 1), Dir: orInt(p.dir, 1),
		Ritual: p.ritual,
		Gates:  p.gates, Bag: [3]int{18, 12, 6},
		Away: p.away, Awoken: p.awoken,
		Stage: orStr(p.stage, "turn"), RNG: p.rng, Prompts: p.prompts, Bat: p.bat, Doomx: p.doomx,
	}
	copy(d.Power[:], p.power[:])
	copy(d.Doom[:], p.doom[:])
	copy(d.Books[:], p.books[:])
	copy(d.Reqs[:], p.reqs[:])
	for i := range p.captured {
		copy(d.Captured[i][:], p.captured[i][:])
	}
	if d.Gates == nil {
		d.Gates = map[string]int{}
	}
	d.Desec = map[string]bool{}
	for _, a := range p.desec {
		d.Desec[a] = true
	}
	for s, vals := range p.signs {
		d.Signs[s] = vals
	}
	if d.RNG == 0 {
		d.RNG = 424242
	}
	if d.Phase == "action" && d.Stage == "turn" {
		d.Turn = &cthulhuwars.TurnState{Seat: orInt(p.turn, 1), Main: p.main}
	} else if p.turn != 0 {
		d.Turn = &cthulhuwars.TurnState{Seat: p.turn, Main: p.main}
	}
	st.SetExt(d)
	return st
}

func powers(a, b, c, d int) [5]int { return [5]int{0, a, b, c, d} }

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func orInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func data(t testing.TB, st *state.BoardState) *cthulhuwars.Data {
	t.Helper()
	d, ok := st.Ext().(*cthulhuwars.Data)
	if !ok {
		t.Fatalf("payload is %T", st.Ext())
	}
	return d
}

// play parses and applies a notation for seat.
func play(t testing.TB, g *catalog.CompiledGame, st *state.BoardState, seat int, notation string) *state.BoardState {
	t.Helper()
	mv, err := state.ParseMove(g, notation)
	if err != nil {
		t.Fatalf("parse %q: %v", notation, err)
	}
	res, err := state.Apply(g, st, seat, mv)
	if err != nil {
		t.Fatalf("apply %q as seat %d: %v\n%s", notation, seat, err, state.RenderText(g, st))
	}
	return res.NewState
}

func refuse(t testing.TB, g *catalog.CompiledGame, st *state.BoardState, seat int, notation string) {
	t.Helper()
	mv, err := state.ParseMove(g, notation)
	if err != nil {
		return
	}
	if _, err := state.Apply(g, st, seat, mv); err == nil {
		t.Fatalf("%q was accepted for seat %d but must be illegal\n%s", notation, seat, state.RenderText(g, st))
	}
}

func notations(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	var out []string
	for _, mv := range state.LegalMoves(g, st, seat) {
		out = append(out, state.FormatMove(g, mv))
	}
	return out
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func hasPrefix(list []string, prefix string) bool {
	for _, s := range list {
		if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// count returns how many units of seat/typ stand in area on a position.
func count(st *state.BoardState, seat int, typ, area string) int {
	n := 0
	for _, o := range st.Stack(boardID(area)) {
		if o.Piece == typ && o.Color == colors[seat] {
			n++
		}
	}
	return n
}

// seedDice finds a generator state whose first dice are exactly want.
func seedDice(t testing.TB, want ...int) uint64 {
	t.Helper()
	for seed := uint64(1); seed < 3_000_000; seed++ {
		dice, _ := cthulhuwars.RollDice(seed, len(want))
		ok := true
		for i := range want {
			if dice[i] != want[i] {
				ok = false
				break
			}
		}
		if ok {
			return seed
		}
	}
	t.Fatal("no seed produces the wanted dice")
	return 0
}

func boardID(s string) board.PositionID { return board.PositionID(s) }
