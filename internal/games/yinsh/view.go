package yinsh

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

// Redact attaches the derived view: Yinsh has nothing to hide.
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
	for _, c := range d.Flipped {
		v.Tints[c] = "flipped"
	}
	if d.Stage == stageRow {
		// The rows the seat to move may remove.
		for _, r := range e.rows(d.Turn) {
			dir := e.dirBetween(r[0], r[1])
			for c := r[0]; ; c = e.g.nbr[c][dir] {
				v.Tints[string(e.g.ids[c])] = "row"
				if c == r[1] {
					break
				}
			}
		}
	}
	switch {
	case d.Turn == 0 && d.Winner != 0:
		v.Prompt = fmt.Sprintf("%s wins with %d rings removed", seatLabel(d.Winner), d.Score[d.Winner-1])
	case d.Turn == 0:
		v.Prompt = "The game is drawn"
	case d.Stage == stageSetup:
		v.Prompt = fmt.Sprintf("%s: place a ring on any empty point", seatLabel(d.Turn))
	case d.Stage == stageRow:
		v.Prompt = fmt.Sprintf("%s: choose a row of five to remove (its two end markers)", seatLabel(d.Turn))
	case d.Stage == stageRing:
		v.Prompt = fmt.Sprintf("%s: choose one of your rings to remove", seatLabel(d.Turn))
	default:
		v.Prompt = fmt.Sprintf("%s: choose a ring, then the point it moves to (a marker stays behind)", seatLabel(d.Turn))
	}
	pool := e.supply() - e.markersOnBoard()
	for s := 1; s <= 2; s++ {
		v.Seats[string(rune('0'+s))] = &SeatView{Counters: []CounterView{
			{ID: "rings", Label: "Rings removed", Value: d.Score[s-1], Max: e.target()},
			{ID: "pool", Label: "Markers in the pool", Value: pool},
		}}
	}
	return v
}

// score values the position for seat: rings removed, with credit for long
// runs of markers of its colour and a debit for the opponent's.
func (e *eng) score(seat int) float64 {
	runs := func(s int) float64 {
		isMine := func(i int) bool { return i >= 0 && e.kind[i] == marker && int(e.owner[i]) == s }
		total := 0.0
		for i := range e.kind {
			if !isMine(i) {
				continue
			}
			for _, d := range []int{0, 2, 4} {
				if isMine(e.g.nbr[i][d+1]) {
					continue
				}
				n := 1
				for c := e.g.nbr[i][d]; isMine(c); c = e.g.nbr[c][d] {
					n++
				}
				if n >= 3 && n < 5 {
					total += float64(n * n)
				} else if n >= 5 {
					total += 40
				}
			}
		}
		return total
	}
	d := e.d
	x := 60*float64(d.Score[seat-1]-d.Score[other(seat)-1]) + runs(seat) - runs(other(seat))
	return x
}

// Heuristic values a position for seat in [-1, 1].
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
	x := newEng(game, st, d).score(seat) / 200
	if x > 0.99 {
		x = 0.99
	}
	if x < -0.99 {
		x = -0.99
	}
	return x
}

// GuideMoves keeps a bot from wasting its play: it takes the decisions that
// leave it best off by rings removed and runs of markers.
func (rs *Ruleset) GuideMoves(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) []state.Move {
	d, err := dataOf(st)
	if err != nil || d.Stage == stageSetup {
		return legal
	}
	best := -1e9
	scores := make([]float64, len(legal))
	for i, mv := range legal {
		e := newEng(game, st, d)
		e.d = d.clone()
		e.execute(mv, seat)
		scores[i] = e.score(seat)
		if scores[i] > best {
			best = scores[i]
		}
	}
	var out []state.Move
	for i, mv := range legal {
		if scores[i] >= best-3 {
			out = append(out, mv)
		}
	}
	return out
}
