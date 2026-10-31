package mycelium

import (
	"fmt"
	"math/rand"

	"github.com/noderunner/bingle/internal/engine/board"
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
	v := &View{Phase: "play", ToMove: d.Turn, Tints: map[string]string{}, Seats: map[string]*SeatView{}}
	if d.Last != "" {
		v.Tints[d.Last] = "last"
	}
	n := e.count()
	cf := d.Confluence
	switch {
	case d.Turn == 0 && d.Winner != 0:
		v.Prompt = fmt.Sprintf("%s wins, %d nodes to %d", seatLabel(d.Winner), n[d.Winner-1], n[other(d.Winner)-1])
	case d.Turn == 0:
		v.Prompt = "The game is drawn"
	case cf == nil:
		v.Prompt = fmt.Sprintf("%s: place a node", seatLabel(d.Turn))
	default:
		v.Stage = cf.Phase
		for _, c := range cf.Spent {
			v.Tints[c] = "spent"
		}
		v.Tints[string(e.g.ids[e.g.centre])] = "confluence"
		switch {
		case cf.Head != "":
			v.Prompt = fmt.Sprintf("%s: convert an enemy node touching %s (%d more at most), or stop", seatLabel(d.Turn), cf.Head, cf.Left)
		case cf.Phase == phaseSpore:
			v.Prompt = fmt.Sprintf("Confluence, spores of size %d: %s converts an enemy node in a line of sight", cf.Initiative, seatLabel(d.Turn))
		default:
			v.Prompt = fmt.Sprintf("Confluence, hyphae of size %d: %s spreads from the end of a strand", cf.Initiative, seatLabel(d.Turn))
		}
	}
	for s := 1; s <= 2; s++ {
		v.Seats[string(rune('0'+s))] = &SeatView{Counters: []CounterView{{ID: "nodes", Label: "Nodes", Value: n[s-1]}}}
	}
	return v
}

// Heuristic values a position for seat in [-1, 1] by its share of the nodes.
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
	return lead(newEng(game, st, d).count(), seat) / float64(len(geoOf(game).ids))
}

func lead(n [2]int, seat int) float64 { return float64(n[seat-1] - n[other(seat)-1]) }

// GuideMoves keeps a bot from throwing a confluence away. It takes the centre
// only when the confluence that follows (played out greedily) leaves it better
// off than a node placed anywhere else, and never when that confluence loses.
// In a confluence it converts as far as a spread allows, starts a hyphae spread
// from the end of its longest strand, and treats spreads that convert the same
// node as one.
func (rs *Ruleset) GuideMoves(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) []state.Move {
	d, err := dataOf(st)
	if err != nil {
		return legal
	}
	e := newEng(game, st, d)
	if d.Confluence != nil {
		return e.guideSpreads(legal)
	}
	centre := e.g.ids[e.g.centre]
	var rest []state.Move
	take := -1
	for i, mv := range legal {
		if mv.To == centre {
			take = i
		} else {
			rest = append(rest, mv)
		}
	}
	if take < 0 || len(rest) == 0 {
		return legal
	}
	elsewhere := lead(e.count(), seat) + 1
	switch after := rs.playOut(game, st, seat, legal[take]); {
	case after > elsewhere:
		return legal[take : take+1]
	case after < elsewhere:
		return rest
	}
	return legal
}

// guideSpreads narrows a confluence decision as GuideMoves describes.
func (e *eng) guideSpreads(legal []state.Move) []state.Move {
	// The length each hyphae start allows, and the longest for each target.
	reach := make([]int, len(legal))
	best := map[board.PositionID]int{}
	for k, mv := range legal {
		if mv.Action == actHyphae && mv.From != "" {
			i, j := e.g.index[mv.From], e.g.index[mv.To]
			reach[k] = e.strand(i, e.clusterAt(i), len(e.clusterAt(j)))
			best[mv.To] = max(best[mv.To], reach[k])
		}
	}
	seen := map[board.PositionID]bool{}
	var out []state.Move
	for k, mv := range legal {
		switch {
		case mv.Action == actDone && len(legal) > 1:
			continue
		case mv.Action == actHyphae && mv.From != "" && reach[k] < best[mv.To]:
			continue
		case mv.Action != actDone && seen[mv.To]:
			continue
		}
		seen[mv.To] = true
		out = append(out, mv)
	}
	return out
}

// playOut plays mv and the confluence it starts, each side taking its first
// guided spread, and returns seat's lead in nodes at the end (±100 for a won
// or lost game).
func (rs *Ruleset) playOut(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Move) float64 {
	for guard := 0; guard < 4*len(geoOf(game).ids); guard++ {
		res, err := rs.Apply(game, st, rs.ToMove(game, st), mv)
		if err != nil {
			return -100
		}
		st = res.NewState
		d, _ := dataOf(st)
		switch {
		case res.GameEnded && res.WinnerSeat == nil:
			return 0
		case res.GameEnded && *res.WinnerSeat == seat:
			return 100
		case res.GameEnded:
			return -100
		case d.Confluence == nil:
			return lead(newEng(game, st, d).count(), seat)
		}
		e := newEng(game, st, d)
		mv = e.guideSpreads(e.legal())[0]
	}
	return 0
}
