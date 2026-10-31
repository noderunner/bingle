package blooms

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

// View is what clients draw beyond the stones.
type View struct {
	Phase  string               `json:"phase"`
	Stage  string               `json:"stage,omitempty"`
	ToMove int                  `json:"to_move,omitempty"`
	Prompt string               `json:"prompt"`
	Tints  map[string]string    `json:"tints,omitempty"`
	Seats  map[string]*SeatView `json:"seats"`
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

// Redact attaches the derived view: Blooms has nothing to hide.
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
	v := &View{Phase: "play", Stage: d.Stage, ToMove: d.Turn, Tints: map[string]string{}, Seats: map[string]*SeatView{}}
	for _, c := range d.Last {
		v.Tints[c] = "last"
	}
	for _, c := range d.Placed {
		v.Tints[c] = "last"
	}
	// Blooms already fenced are captured when the opponent's turn ends.
	for owner := 1; owner <= 2; owner++ {
		for _, group := range e.fenced(owner) {
			for _, i := range group {
				v.Tints[string(e.g.ids[i])] = "fenced"
			}
		}
	}
	switch {
	case d.Turn == 0 && d.Winner != 0:
		v.Prompt = fmt.Sprintf("%s wins with %d captures", seatLabel(d.Winner), d.Captured[d.Winner-1])
	case d.Turn == 0:
		v.Prompt = "The game is drawn"
	case d.Stage == stageSecond:
		v.Prompt = fmt.Sprintf("%s: place a second stone (not %s), or end the turn", seatLabel(d.Turn), d.First)
	case d.Turns == 0:
		v.Prompt = "Red & Yellow: place one stone"
	case d.Turn == 2 && d.Turns == 1 && d.Stage == stageFirst && e.pieOn():
		v.Prompt = fmt.Sprintf("%s: answer the opening stone, or swap sides and take it (the pie rule)", seatLabel(d.Turn))
	default:
		v.Prompt = fmt.Sprintf("%s: place a stone", seatLabel(d.Turn))
	}
	for s := 1; s <= 2; s++ {
		v.Seats[string(rune('0'+s))] = &SeatView{Counters: []CounterView{{ID: "captured", Label: "Captured", Value: d.Captured[s-1], Max: e.target()}}}
	}
	return v
}

// score values the position for seat: captures banked, plus the blooms that
// would fall if the turn ended now, less the stones that are in danger.
func (e *eng) score(seat int) float64 {
	d := e.d
	mine, theirs := float64(d.Captured[seat-1]), float64(d.Captured[other(seat)-1])
	for _, g := range e.fenced(other(seat)) {
		mine += float64(len(g))
	}
	for _, g := range e.fenced(seat) {
		theirs += float64(len(g))
	}
	return mine - theirs
}

// Heuristic values a position for seat in [-1, 1] from the captures.
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
	x := e.score(seat) / float64(e.target())
	if x > 1 {
		x = 1
	}
	if x < -1 {
		x = -1
	}
	return x
}

// GuideMoves keeps a bot from throwing stones away: it takes the placements
// that leave it best off by the captures the turn would bring, less the
// stones it leaves fenced.
func (rs *Ruleset) GuideMoves(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) []state.Move {
	d, err := dataOf(st)
	if err != nil {
		return legal
	}
	best := -1e9
	scores := make([]float64, len(legal))
	for i, mv := range legal {
		e := newEng(game, st, d)
		e.d = d.clone()
		e.apply(mv)
		scores[i] = e.score(seat)
		if scores[i] > best {
			best = scores[i]
		}
	}
	var out []state.Move
	for i, mv := range legal {
		if scores[i] >= best-0.5 {
			out = append(out, mv)
		}
	}
	return out
}

// apply plays mv on e without ending the turn's bookkeeping when it only
// places a stone: for a first stone the captures are scored as if the turn
// ended there.
func (e *eng) apply(mv state.Move) {
	seat := e.d.Turn
	if mv.Action == actPlace {
		e.cells[e.g.index[mv.To]] = mv.Piece
	}
	// Score as if the turn ended now.
	for _, g := range e.fenced(other(seat)) {
		for _, i := range g {
			e.cells[i] = ""
			e.d.Captured[seat-1]++
		}
	}
}

func seatLabel(seat int) string {
	if seat == 1 {
		return "Red & Yellow"
	}
	return "Blue & Black"
}
