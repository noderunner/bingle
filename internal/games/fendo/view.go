package fendo

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

// View is what clients draw beyond the pieces.
type View struct {
	Phase  string               `json:"phase"`
	ToMove int                  `json:"to_move,omitempty"`
	Prompt string               `json:"prompt"`
	Tints  map[string]string    `json:"tints,omitempty"`
	Walls  [][2]string          `json:"walls,omitempty"`
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

// Redact attaches the derived view: Fendo has nothing to hide.
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
	v := &View{Phase: "play", ToMove: d.Turn, Tints: map[string]string{}, Walls: d.Walls, Seats: map[string]*SeatView{}}
	for _, a := range e.areas() {
		if a.count() != 1 {
			continue
		}
		key := "area1"
		if a.pieces[2] == 1 {
			key = "area2"
		}
		for _, c := range a.cells {
			v.Tints[string(e.g.ids[c])] = key
		}
	}
	switch {
	case d.Turn == 0 && d.Winner != 0:
		v.Prompt = fmt.Sprintf("%s wins, %d squares to %d", seatLabel(d.Winner), d.Score[d.Winner-1], d.Score[other(d.Winner)-1])
	case d.Turn == 0:
		v.Prompt = "The game is drawn"
	default:
		v.Prompt = fmt.Sprintf("%s: move a piece and build a fence, or add a piece", seatLabel(d.Turn))
	}
	t := e.territory()
	for s := 1; s <= 2; s++ {
		v.Seats[string(rune('0'+s))] = &SeatView{Counters: []CounterView{
			{ID: "territory", Label: "Squares owned", Value: t[s], Max: len(e.occ)},
			{ID: "stock", Label: "In stock", Value: d.Stock[s-1]},
		}}
	}
	return v
}

// estimate is the squares seat owns for sure plus the share of the open area
// its pieces reach first (squares reached at the same time count for no one).
func (e *eng) estimate(seat int) float64 {
	total := float64(e.territory()[seat])
	open := map[int]bool{}
	for _, c := range e.openArea() {
		open[c] = true
	}
	dist := make([]int, len(e.occ))
	own := make([]int8, len(e.occ))
	var queue []int
	for i := range e.occ {
		dist[i] = -1
		if open[i] && e.occ[i] != 0 {
			dist[i], own[i] = 0, e.occ[i]
			queue = append(queue, i)
		}
	}
	for k := 0; k < len(queue); k++ {
		c := queue[k]
		for d := 0; d < 4; d++ {
			n := e.step(c, d)
			switch {
			case n < 0:
			case dist[n] < 0:
				dist[n], own[n] = dist[c]+1, own[c]
				queue = append(queue, n)
			case dist[n] == dist[c]+1 && own[n] != own[c]:
				own[n] = 3
			}
		}
	}
	for _, c := range queue {
		if int(own[c]) == seat {
			total++
		}
	}
	return total
}

// Heuristic values a position for seat in [-1, 1] from the squares each side
// owns or is nearest to.
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
	x := (e.estimate(seat) - e.estimate(other(seat))) / float64(len(e.occ))
	if x > 0.99 {
		x = 0.99
	}
	if x < -0.99 {
		x = -0.99
	}
	return x
}

// GuideMoves keeps a bot sensible: it considers the moves that leave it best
// off in squares owned or nearest.
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
		e.execute(mv, seat)
		scores[i] = e.estimate(seat) - e.estimate(other(seat))
		if scores[i] > best {
			best = scores[i]
		}
	}
	var out []state.Move
	for i, mv := range legal {
		if scores[i] >= best-1.5 {
			out = append(out, mv)
		}
	}
	return out
}
