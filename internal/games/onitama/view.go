package onitama

import (
	"math/rand"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

var (
	_ state.HiddenInfo    = (*Ruleset)(nil)
	_ state.Heuristic     = (*Ruleset)(nil)
	_ state.Describer     = (*Ruleset)(nil)
	_ state.BotGuide      = (*Ruleset)(nil)
	_ state.SeededRuleset = (*Ruleset)(nil)
)

// CardView is a Move card as clients draw it.
type CardView struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Moves [][2]int `json:"moves"` // (right, forward) for the holder
	Stamp string   `json:"stamp"`
}

// CardsView is the five cards in play.
type CardsView struct {
	Hands map[string][]CardView `json:"hands"`
	Side  CardView              `json:"side"`
}

// View is what clients draw beyond the pieces.
type View struct {
	Phase  string               `json:"phase"`
	ToMove int                  `json:"to_move,omitempty"`
	Prompt string               `json:"prompt"`
	Tints  map[string]string    `json:"tints,omitempty"`
	Seats  map[string]*SeatView `json:"seats"`
	Cards  *CardsView           `json:"cards,omitempty"`
}

// SeatView is one player's scoreboard.
type SeatView struct {
	Counters []CounterView `json:"counters"`
}

// CounterView is a labelled number.
type CounterView struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value int    `json:"value"`
	Max   int    `json:"max,omitempty"`
}

func cardView(id string) CardView {
	c, _ := cardByID(id)
	return CardView(c)
}

// Redact attaches the derived view: Onitama has nothing to hide.
func (rs *Ruleset) Redact(game *catalog.CompiledGame, st *state.BoardState, viewer int) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	out := st.Clone()
	out.SetExt(&publicExt{Data: d.clone(), View: newEng(st, d).buildView()})
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

func (e *eng) count(seat int) int {
	n := 0
	for _, p := range e.b {
		if p != nil && p.seat == seat {
			n++
		}
	}
	return n
}

func (e *eng) buildView() *View {
	d := e.d
	v := &View{Phase: "play", ToMove: d.Turn, Tints: map[string]string{}, Seats: map[string]*SeatView{}}
	v.Tints[cellName(arch(seatRed))] = "arch-red"
	v.Tints[cellName(arch(seatBlue))] = "arch-blue"
	if d.LastFrom != "" {
		v.Tints[d.LastFrom] = "last"
		v.Tints[d.LastTo] = "last"
	}
	v.Cards = &CardsView{Hands: map[string][]CardView{
		"1": {cardView(d.Hands[0][0]), cardView(d.Hands[0][1])},
		"2": {cardView(d.Hands[1][0]), cardView(d.Hands[1][1])},
	}, Side: cardView(d.Side)}
	if d.Turn == 0 {
		v.Prompt = reasonText(d.Reason)
	} else {
		v.Prompt = seatName(d.Turn) + " to move: choose a card, then a piece and where it goes"
	}
	for s := seatRed; s <= seatBlue; s++ {
		v.Seats[string(rune('0'+s))] = &SeatView{Counters: []CounterView{{ID: "pieces", Label: "Pieces", Value: e.count(s)}}}
	}
	return v
}

// Heuristic values a position for seat in [-1, 1]: pieces left, and how far
// each side's Master has come toward the opposing arch.
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
	e := newEng(st, d)
	x := 0.0
	for c, p := range e.b {
		if p == nil {
			continue
		}
		sign := 1.0
		if p.seat != seat {
			sign = -1
		}
		x += sign * 0.2
		if p.master {
			progress := float64(c / 5)
			if p.seat == seatBlue {
				progress = 4 - progress
			}
			x += sign * 0.05 * progress
		}
	}
	if x > 1 {
		x = 1
	}
	if x < -1 {
		x = -1
	}
	return x
}

// GuideMoves lets a bot win when it can, and not lose at once when it can
// help it: it keeps moves that win now, else moves after which the opponent
// has no winning reply.
func (rs *Ruleset) GuideMoves(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) []state.Move {
	d, err := dataOf(st)
	if err != nil {
		return legal
	}
	var winning, safe []state.Move
	for _, mv := range legal {
		e := newEng(st, d)
		nd := d.clone()
		e.d = nd
		e.execute(mv, seat)
		if nd.Turn == 0 {
			winning = append(winning, mv)
			continue
		}
		if !e.canWin(other(seat)) {
			safe = append(safe, mv)
		}
	}
	switch {
	case len(winning) > 0:
		return winning
	case len(safe) > 0:
		return safe
	}
	return legal
}

// canWin reports whether seat has a move that ends the game in its favour.
func (e *eng) canWin(seat int) bool {
	for _, mv := range e.legal(seat) {
		if mv.Action == actPass {
			continue
		}
		to, _ := cellOf(mv.To)
		from, _ := cellOf(mv.From)
		if v := e.b[to]; v != nil && v.master {
			return true
		}
		if p := e.b[from]; p != nil && p.master && to == arch(other(seat)) {
			return true
		}
	}
	return false
}
