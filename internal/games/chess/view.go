package chess

import (
	"math/rand"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

var (
	_ state.HiddenInfo = (*Ruleset)(nil)
	_ state.Open       = (*Ruleset)(nil)
	_ state.Heuristic  = (*Ruleset)(nil)
	_ state.Describer  = (*Ruleset)(nil)
)

// View is what clients draw beyond the pieces: a king in check. It travels
// in the "view" key of a position's ext payload.
type View struct {
	Phase       string            `json:"phase"`
	ImpulseSeat int               `json:"impulse_seat,omitempty"`
	Prompt      string            `json:"prompt"`
	Impulse     string            `json:"impulse,omitempty"`
	Tints       map[string]string `json:"tints,omitempty"`
}

// Redact attaches the derived view: chess has nothing to hide.
func (rs *Ruleset) Redact(game *catalog.CompiledGame, st *state.BoardState, viewer int) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	out := st.Clone()
	out.SetExt(&publicExt{Data: d.clone(), View: buildView(st, d)})
	return out
}

// HidesNothing: Redact only attaches the view (state.Open).
func (rs *Ruleset) HidesNothing() bool { return true }

// RedactMove: moves reveal nothing.
func (rs *Ruleset) RedactMove(game *catalog.CompiledGame, st *state.BoardState, mv state.Selection, notation string) (state.Selection, string) {
	return mv, notation
}

// Determinize: there is nothing unknown to sample.
func (rs *Ruleset) Determinize(game *catalog.CompiledGame, st *state.BoardState, viewer int, rng *rand.Rand) *state.BoardState {
	return st
}

func buildView(st *state.BoardState, d *Data) *View {
	v := &View{Phase: "play", ImpulseSeat: d.Turn, Tints: map[string]string{}}
	if d.Turn == 0 {
		v.Prompt = reasonText(d.Reason)
		if d.Reason == "checkmate" {
			p := decode(st, &Data{Turn: int(other(uint8(d.Winner)))})
			if k := p.king(p.side); k >= 0 {
				v.Tints[cellName(k)] = "check"
			}
		}
		return v
	}
	p := decode(st, d)
	name := map[int]string{seatWhite: "White", seatBlack: "Black"}[d.Turn]
	v.Prompt = name + " to move"
	v.Impulse = "Choose a piece to move."
	if p.inCheck(p.side) {
		v.Tints[cellName(p.king(p.side))] = "check"
		v.Prompt = name + " to move, in check"
		v.Impulse = "Choose a piece to move: your king is in check."
	}
	return v
}

// value is each kind's worth in pawns, for the heuristic.
var value = [...]float64{0, 1, 3, 3, 5, 9, 0}

// Heuristic values a position for seat in [-1, 1] by the material on each
// side, with a little for pawns that have come forward.
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
	p := decode(st, d)
	var x float64
	for c, q := range p.b {
		w := value[q.kind]
		if q.kind == kPawn {
			ranks := c / 8
			if q.side == seatBlack {
				ranks = 7 - ranks
			}
			w += 0.05 * float64(ranks-1)
		}
		if q.side == uint8(seat) {
			x += w
		} else {
			x -= w
		}
	}
	x /= 20
	if x > 1 {
		x = 1
	}
	if x < -1 {
		x = -1
	}
	return x
}
