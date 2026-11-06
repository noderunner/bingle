// Package difftest plays two implementations of the same game side by side and
// reports the first place they disagree. It is how a game moves from a Go
// ruleset to a declaration (the DSL coverage audit, decision 4): the Go ruleset
// stays for one release as the reference, and the declaration must match it
// over a random battery — the same legal selections, the same impulses, the
// same positions and the same results, selection for selection.
//
// Both implementations are driven only through the engine's public calls
// (InitialStateSeeded, LegalSelections, Apply, ImpulseEnds, FormatMove), so it
// makes no difference which of them is the ruleset and which the declaration.
// Selections are compared by their notation, which a conversion keeps, and
// positions by what is on the board plus whatever else each side is asked to
// report in Extra. Where an impulse ends is compared too (ADR 0114): a
// selection that keeps the seat may still end its impulse, when it rolled or
// drew, and a declaration must end it in the same places.
package difftest

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Side is one implementation of the game.
type Side struct {
	Game *catalog.CompiledGame
	// Extra reports what else of a position both sides must agree on, in words
	// both produce alike: each seat's captures, say, which a ruleset keeps in
	// its own data and a declaration in a reserve pool. Nil compares the board,
	// the impulse and the results only.
	Extra func(st *state.BoardState) string
}

// Options size the battery. Zero values take the defaults.
type Options struct {
	Games  int    // random games played (default 20)
	Plies  int    // plies at most in each (default 200)
	Sample int    // legal moves besides the one played that are applied and compared each ply (default 8)
	Seed   uint64 // the first game's seed; game i uses Seed+i, for chance and for the choice of moves
}

func (o Options) withDefaults() Options {
	if o.Games <= 0 {
		o.Games = 20
	}
	if o.Plies <= 0 {
		o.Plies = 200
	}
	if o.Sample < 0 {
		o.Sample = 0
	} else if o.Sample == 0 {
		o.Sample = 8
	}
	return o
}

// Compare plays the battery and returns the first disagreement between ref and
// cand, naming the game's seed and the moves that led there so it can be
// replayed, or nil when they agree throughout.
func Compare(ref, cand Side, opt Options) error {
	opt = opt.withDefaults()
	if a, b := ref.Game.Board.Positions(), cand.Game.Board.Positions(); !samePositions(a, b) {
		return fmt.Errorf("the boards differ: %d positions against %d, or not the same ids", len(a), len(b))
	}
	for i := 0; i < opt.Games; i++ {
		if err := play(ref, cand, opt, opt.Seed+uint64(i)); err != nil {
			return err
		}
	}
	return nil
}

func play(ref, cand Side, opt Options, seed uint64) error {
	rng := rand.New(rand.NewSource(int64(seed)))
	st := [2]*state.BoardState{}
	var err error
	if st[0], err = state.InitialStateSeeded(ref.Game, seed); err != nil {
		return fmt.Errorf("seed %d: the reference does not start: %w", seed, err)
	}
	if st[1], err = state.InitialStateSeeded(cand.Game, seed); err != nil {
		return fmt.Errorf("seed %d: the candidate does not start: %w", seed, err)
	}
	seat := state.FirstSeat(ref.Game)
	if s := state.FirstSeat(cand.Game); s != seat {
		return fmt.Errorf("seed %d: the first impulse is seat %d in the reference, %d in the candidate", seed, seat, s)
	}
	var path []string
	where := func() string {
		if len(path) == 0 {
			return fmt.Sprintf("seed %d, at the start", seed)
		}
		return fmt.Sprintf("seed %d, after %s", seed, strings.Join(path, " "))
	}

	for ply := 0; ply < opt.Plies; ply++ {
		if d := diffPosition(ref, cand, st[0], st[1]); d != "" {
			return fmt.Errorf("%s: the positions differ: %s", where(), d)
		}
		legal := [2]map[string]state.Selection{moves(ref.Game, st[0], seat), moves(cand.Game, st[1], seat)}
		if d := diffMoves(legal[0], legal[1]); d != "" {
			return fmt.Errorf("%s: seat %d's legal moves differ: %s", where(), seat, d)
		}
		if len(legal[0]) == 0 {
			return nil
		}
		names := make([]string, 0, len(legal[0]))
		for n := range legal[0] {
			names = append(names, n)
		}
		sort.Strings(names)
		played := names[rng.Intn(len(names))]

		// The move played and a sample of the others must lead to the same place.
		tried := []string{played}
		for k := 0; k < opt.Sample && k < len(names)-1; k++ {
			tried = append(tried, names[rng.Intn(len(names))])
		}
		var next [2]*state.MoveResult
		for _, name := range tried {
			res, d := applyBoth(ref, cand, st, seat, legal[0][name], legal[1][name])
			if d != "" {
				return fmt.Errorf("%s: seat %d plays %s: %s", where(), seat, name, d)
			}
			if name == played && next[0] == nil {
				next = res
			}
		}
		path = append(path, played)
		if next[0].GameEnded {
			return nil
		}
		st[0], st[1], seat = next[0].NewState, next[1].NewState, next[0].NextImpulseSeat
	}
	return nil
}

// applyBoth plays the same move on both sides and compares what comes of it.
func applyBoth(ref, cand Side, st [2]*state.BoardState, seat int, m0, m1 state.Selection) ([2]*state.MoveResult, string) {
	var res [2]*state.MoveResult
	var err [2]error
	res[0], err[0] = state.Apply(ref.Game, st[0], seat, m0)
	res[1], err[1] = state.Apply(cand.Game, st[1], seat, m1)
	switch {
	case err[0] != nil && err[1] != nil:
		return res, fmt.Sprintf("both reject a move they list (%v; %v)", err[0], err[1])
	case err[0] != nil:
		return res, fmt.Sprintf("the reference rejects a move it lists: %v", err[0])
	case err[1] != nil:
		return res, fmt.Sprintf("the candidate rejects a move it lists: %v", err[1])
	}
	a, b := res[0], res[1]
	if a.Notation != b.Notation {
		return res, fmt.Sprintf("it is written %q in the reference, %q in the candidate", a.Notation, b.Notation)
	}
	if a.GameEnded != b.GameEnded || seatOf(a.WinnerSeat) != seatOf(b.WinnerSeat) || a.EndReason != b.EndReason {
		return res, fmt.Sprintf("the results differ: %s in the reference, %s in the candidate", result(a), result(b))
	}
	if !a.GameEnded && a.NextImpulseSeat != b.NextImpulseSeat {
		return res, fmt.Sprintf("the next impulse is seat %d in the reference, %d in the candidate", a.NextImpulseSeat, b.NextImpulseSeat)
	}
	if ea, eb := state.ImpulseEnds(ref.Game, st[0], seat, m0, a), state.ImpulseEnds(cand.Game, st[1], seat, m1, b); ea != eb {
		return res, fmt.Sprintf("it %s seat %d's impulse in the reference and %s it in the candidate", ends(ea), seat, ends(eb))
	}
	if d := diffPosition(ref, cand, a.NewState, b.NewState); d != "" {
		return res, "the positions after it differ: " + d
	}
	return res, ""
}

func ends(yes bool) string {
	if yes {
		return "ends"
	}
	return "does not end"
}

// moves lists a seat's legal moves by their notation.
func moves(game *catalog.CompiledGame, st *state.BoardState, seat int) map[string]state.Selection {
	out := map[string]state.Selection{}
	for _, mv := range state.LegalSelections(game, st, seat) {
		out[state.FormatMove(game, mv)] = mv
	}
	return out
}

func diffMoves(a, b map[string]state.Selection) string {
	var onlyA, onlyB []string
	for n := range a {
		if _, ok := b[n]; !ok {
			onlyA = append(onlyA, n)
		}
	}
	for n := range b {
		if _, ok := a[n]; !ok {
			onlyB = append(onlyB, n)
		}
	}
	if len(onlyA) == 0 && len(onlyB) == 0 {
		return ""
	}
	return fmt.Sprintf("only the reference has %s; only the candidate has %s", few(onlyA), few(onlyB))
}

// diffPosition compares the board, cell by cell, and each side's Extra.
func diffPosition(ref, cand Side, a, b *state.BoardState) string {
	var cells []string
	for _, p := range ref.Game.Board.Positions() {
		if x, y := stack(a, p), stack(b, p); x != y {
			cells = append(cells, fmt.Sprintf("%s holds %s against %s", p, x, y))
		}
	}
	if len(cells) > 0 {
		return few(cells)
	}
	var x, y string
	if ref.Extra != nil {
		x = ref.Extra(a)
	}
	if cand.Extra != nil {
		y = cand.Extra(b)
	}
	if x != y {
		return fmt.Sprintf("the reference reports %q, the candidate %q", x, y)
	}
	return ""
}

func stack(st *state.BoardState, p board.PositionID) string {
	s := st.Stack(p)
	if len(s) == 0 {
		return "nothing"
	}
	parts := make([]string, len(s))
	for i, o := range s {
		parts[i] = o.Color + " " + o.Piece
	}
	return strings.Join(parts, ", ")
}

func samePositions(a, b []board.PositionID) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[board.PositionID]bool, len(a))
	for _, p := range a {
		set[p] = true
	}
	for _, p := range b {
		if !set[p] {
			return false
		}
	}
	return true
}

func seatOf(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func result(r *state.MoveResult) string {
	if !r.GameEnded {
		return "play goes on"
	}
	if r.WinnerSeat == nil {
		return fmt.Sprintf("a draw (%s)", r.EndReason)
	}
	return fmt.Sprintf("seat %d wins (%s)", *r.WinnerSeat, r.EndReason)
}

// few lists at most five items, sorted, and says how many more there are.
func few(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	sort.Strings(items)
	if len(items) <= 5 {
		return strings.Join(items, "; ")
	}
	return fmt.Sprintf("%s; and %d more", strings.Join(items[:5], "; "), len(items)-5)
}
