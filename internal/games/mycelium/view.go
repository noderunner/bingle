package mycelium

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

// View is what clients draw beyond the nodes.
type View struct {
	Phase  string            `json:"phase"`
	ToMove int               `json:"to_move,omitempty"`
	Prompt string            `json:"prompt"`
	Tints  map[string]string `json:"tints,omitempty"`
}

// Redact attaches the derived view: Mycelium has nothing to hide.
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
	if d.Last != "" {
		v.Tints[d.Last] = "last"
	}
	if d.Confluence != nil {
		v.Tints[string(e.g.center)] = "confluence"
	}
	switch {
	case d.Turn == 0 && d.Winner != 0:
		v.Prompt = fmt.Sprintf("%s wins (%s).", seatLabel(d.Winner), d.Reason)
	case d.Turn == 0:
		v.Prompt = "The game is drawn."
	case d.Confluence == nil:
		v.Prompt = fmt.Sprintf("%s: place a node.", seatLabel(d.Turn))
	default:
		cf := d.Confluence
		v.Prompt = fmt.Sprintf("Confluence (initiative %d, %s phase). %s chooses a %s spread.",
			cf.Initiative, cf.Phase, seatLabel(cf.NextSeat), cf.Phase)
	}
	return v
}

// Heuristic values a position for seat in [-1, 1] from material count.
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
	count := [2]int{}
	for _, c := range e.cells {
		if c == seatColors[1] {
			count[0]++
		}
		if c == seatColors[2] {
			count[1]++
		}
	}
	diff := float64(count[seat-1] - count[other(seat)-1])
	// Normalise roughly by board size.
	if diff > 30 {
		diff = 30
	}
	if diff < -30 {
		diff = -30
	}
	return diff / 30
}

// GuideMoves keeps a bot from useless spreads: it prefers placements that
// create or enlarge clusters toward the centre, and confluence spreads that
// gain material.
func (rs *Ruleset) GuideMoves(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) []state.Move {
	d, err := dataOf(st)
	if err != nil {
		return legal
	}
	if d.Confluence != nil {
		return legal
	}
	best := -1e9
	scores := make([]float64, len(legal))
	g := geoOf(game)
	centerI := g.centerI
	for i, mv := range legal {
		scores[i] = -1e9
		if mv.Action != actPlace {
			continue
		}
		e := newEng(game, st, d)
		e.cells[g.index[mv.To]] = mv.Piece
		// Prefer the centre, then cells with friendly neighbours.
		score := 0.0
		if g.index[mv.To] == centerI {
			score += 10
		}
		for _, n := range g.nbr[g.index[mv.To]] {
			if e.cells[n] == mv.Piece {
				score += 2
			}
		}
		scores[i] = score
		if score > best {
			best = score
		}
	}
	var out []state.Move
	for i, mv := range legal {
		if scores[i] >= best-0.5 {
			out = append(out, mv)
		}
	}
	if len(out) == 0 {
		return legal
	}
	return out
}
