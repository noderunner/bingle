// Package courtfall_test checks Courtfall's rules: the setup draft, the turn
// structure, movement, exchanges and shots, shoves and pushes, Momentum, the
// five Courts, hidden information and the engine contract.
package courtfall_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/courtfall"
)

const (
	light = 1
	dark  = 2
)

func game(t testing.TB) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(courtfall.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get("courtfall")
	if !ok {
		t.Fatal("courtfall is not registered")
	}
	return g
}

// pos describes a position to build: everything defaults to an ordinary
// Light turn in the main stage with nothing on the board.
type pos struct {
	phase  string
	stage  string
	turn   int
	streak int
	// units maps "white/queen" (Light) or "black/queen" (Dark) to a hex.
	units  map[string]string
	tiles  map[string]string
	cond   map[string]int
	mom    [2]int
	vp     [2]int
	courts [2]string
	rng    uint64

	stoneUsed [2]bool
	act       *courtfall.Activation
	con       *courtfall.Contest
	pushes    []courtfall.Push
}

func (p pos) build() *state.BoardState {
	st := state.NewBoardState()
	for k, cell := range p.units {
		parts := strings.SplitN(k, "/", 2)
		st.Set(board.PositionID(cell), state.Occupant{Color: parts[0], Piece: parts[1]})
	}
	d := &courtfall.Data{
		Phase:     orStr(p.phase, "play"),
		Stage:     orStr(p.stage, "main"),
		Turn:      orInt(p.turn, light),
		Streak:    orInt(p.streak, 1),
		Tiles:     p.tiles,
		Cond:      p.cond,
		Mom:       p.mom,
		VP:        p.vp,
		Courts:    p.courts,
		StoneUsed: p.stoneUsed,
		Act:       p.act,
		Con:       p.con,
		Pushes:    p.pushes,
		RNG:       p.rng,
	}
	if d.Phase != "play" {
		d.Stage = ""
		d.Turn = 0
	}
	if d.RNG == 0 {
		d.RNG = 12345
	}
	st.SetExt(d)
	return st
}

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

// data returns the Courtfall payload of a position produced by the engine.
func data(t testing.TB, st *state.BoardState) *courtfall.Data {
	t.Helper()
	d, ok := st.Ext().(*courtfall.Data)
	if !ok {
		t.Fatalf("position payload is %T, want *courtfall.Data", st.Ext())
	}
	return d
}

// play parses a notation and applies it for seat, failing the test on error.
func play(t testing.TB, g *catalog.CompiledGame, st *state.BoardState, seat int, notation string) (*state.BoardState, *state.MoveResult) {
	t.Helper()
	mv, err := state.ParseMove(g, notation)
	if err != nil {
		t.Fatalf("parse %q: %v", notation, err)
	}
	res, err := state.Apply(g, st, seat, mv)
	if err != nil {
		t.Fatalf("apply %q as seat %d: %v\n%s", notation, seat, err, state.RenderText(g, st))
	}
	return res.NewState, res
}

// refuse asserts that a move is rejected.
func refuse(t testing.TB, g *catalog.CompiledGame, st *state.BoardState, seat int, notation string) {
	t.Helper()
	mv, err := state.ParseMove(g, notation)
	if err != nil {
		return // unparsable counts as refused
	}
	if _, err := state.Apply(g, st, seat, mv); err == nil {
		t.Fatalf("%q was accepted for seat %d but must be illegal\n%s", notation, seat, state.RenderText(g, st))
	}
}

// notations lists seat's legal moves as notation, sorted.
func notations(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	var out []string
	for _, mv := range state.LegalMoves(g, st, seat) {
		out = append(out, state.FormatMove(g, mv))
	}
	sort.Strings(out)
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
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// seedWith finds a generator state whose first dice satisfy ok.
func seedWith(t testing.TB, n int, ok func(dice []int) bool) uint64 {
	t.Helper()
	for seed := uint64(1); seed < 2_000_000; seed++ {
		dice, _ := courtfall.RollDice(seed, n)
		if ok(dice) {
			return seed
		}
	}
	t.Fatal("no seed produces the wanted dice")
	return 0
}

// seedDice returns a generator state that rolls exactly want.
func seedDice(t testing.TB, want ...int) uint64 {
	return seedWith(t, len(want), func(d []int) bool {
		for i := range want {
			if d[i] != want[i] {
				return false
			}
		}
		return true
	})
}

// actFor starts an activation of piece with two actions left.
func actFor(piece string) *courtfall.Activation {
	return &courtfall.Activation{Unit: piece, Left: 2}
}

func dump(g *catalog.CompiledGame, st *state.BoardState) string { return state.RenderText(g, st) }
