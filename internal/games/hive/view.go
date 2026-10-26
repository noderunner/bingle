package hive

import (
	"fmt"
	"math/rand"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

var (
	_ state.HiddenInfo = (*Ruleset)(nil)
	_ state.Heuristic  = (*Ruleset)(nil)
	_ state.Describer  = (*Ruleset)(nil)
	_ state.BotGuide   = (*Ruleset)(nil)
)

// View is what clients draw beyond the tiles.
type View struct {
	Phase  string            `json:"phase"`
	ToMove int               `json:"to_move,omitempty"`
	Prompt string            `json:"prompt"`
	Tints  map[string]string `json:"tints,omitempty"`
}

// Redact attaches the derived view: Hive has nothing to hide.
func (rs *Ruleset) Redact(game *catalog.CompiledGame, st *state.BoardState, viewer int) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	out := st.Clone()
	out.SetExt(&publicExt{Data: d.clone(), View: newEng(game, st, d).buildView()})
	return out
}

// RedactMove: moves reveal nothing.
func (rs *Ruleset) RedactMove(game *catalog.CompiledGame, st *state.BoardState, mv state.Move, notation string) (state.Move, string) {
	return mv, notation
}

// Determinize: there is nothing unknown to sample.
func (rs *Ruleset) Determinize(game *catalog.CompiledGame, st *state.BoardState, viewer int, rng *rand.Rand) *state.BoardState {
	return st
}

func (e *eng) buildView() *View {
	d := e.d
	v := &View{Phase: "play", ToMove: d.Turn, Tints: map[string]string{}}
	for _, c := range d.Last {
		v.Tints[c] = "last"
	}
	// A Queen with four or more neighbours is in danger.
	for i, stack := range e.cells {
		for _, p := range stack {
			if int(p.kind) != kQueen {
				continue
			}
			n := 0
			for _, nb := range e.g.nbr[i] {
				if e.occupied(nb) {
					n++
				}
			}
			if n >= 4 {
				v.Tints[string(e.g.ids[i])] = "danger"
			}
		}
	}
	switch {
	case d.Turn == 0 && d.Winner != 0:
		v.Prompt = fmt.Sprintf("%s wins: the other Queen Bee is surrounded", seatLabel(d.Winner))
	case d.Turn == 0:
		v.Prompt = "The game is drawn"
	case e.res[d.Turn][kQueen] > 0 && d.Turns[d.Turn-1] >= 3:
		v.Prompt = fmt.Sprintf("%s: this is your fourth turn, so you must place your Queen Bee", seatLabel(d.Turn))
	case e.pieces() == 0:
		v.Prompt = "White: place your first tile in the middle"
	default:
		v.Prompt = fmt.Sprintf("%s: place a tile or move one", seatLabel(d.Turn))
	}
	return v
}

// Heuristic values a position for seat in [-1, 1] by how far each Queen
// Bee is surrounded, with a little for the tiles each side can still move.
func (rs *Ruleset) Heuristic(game *catalog.CompiledGame, st *state.BoardState, seat int) float64 {
	d, err := dataOf(st)
	if err != nil {
		return 0
	}
	if d.Turn == 0 {
		switch d.Winner {
		case 0:
			return 0
		case seat:
			return 1
		}
		return -1
	}
	e := newEng(game, st, d)
	pressure := func(s int) float64 {
		n := e.surrounded(s)
		if n < 0 {
			return 0
		}
		return float64(n) / 6
	}
	x := 0.8 * (pressure(other(seat)) - pressure(seat))
	// Free tiles are worth having: count the tiles that can move at all.
	free := func(s int) float64 {
		if e.res[s][kQueen] > 0 {
			return 0
		}
		n := 0
		for i := range e.cells {
			if len(e.cells[i]) > 0 && int(e.top(i).seat) == s && len(e.destsOf(i)) > 0 {
				n++
			}
		}
		return float64(n) / 14
	}
	x += 0.2 * (free(seat) - free(other(seat)))
	if x > 0.99 {
		x = 0.99
	}
	if x < -0.99 {
		x = -0.99
	}
	return x
}

// GuideMoves keeps a bot from losing at once: it takes a move that wins on the
// spot if there is one, and drops the moves that surround its own Queen.
func (rs *Ruleset) GuideMoves(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) []state.Move {
	d, err := dataOf(st)
	if err != nil {
		return legal
	}
	var safe []state.Move
	for _, mv := range legal {
		e := newEng(game, st, d)
		e.d = d.clone()
		e.execute(mv, seat)
		switch e.d.Winner {
		case seat:
			return []state.Move{mv}
		case 0:
			safe = append(safe, mv)
		}
	}
	if len(safe) == 0 {
		return legal
	}
	return safe
}
