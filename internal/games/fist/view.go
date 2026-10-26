package fist

import (
	"math/rand"
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

var (
	_ state.HiddenInfo = (*Ruleset)(nil)
	_ state.Heuristic  = (*Ruleset)(nil)
	_ state.Describer  = (*Ruleset)(nil)
	_ state.BotGuide   = (*Ruleset)(nil)
)

// View is what clients draw beyond the pieces: tints, badges, scoreboards and
// the prompt. It travels in the "view" key of a position's ext payload.
type View struct {
	Phase  string               `json:"phase"`
	ToMove int                  `json:"to_move,omitempty"`
	Prompt string               `json:"prompt"`
	Tints  map[string]string    `json:"tints,omitempty"`
	Badges map[string][]string  `json:"badges,omitempty"`
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

// Redact attaches the derived view: Fist has nothing to hide.
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

func (e *eng) buildView() *View {
	d := e.d
	v := &View{Phase: "play", ToMove: d.Turn, Tints: map[string]string{}, Badges: map[string][]string{}, Seats: map[string]*SeatView{}}
	for _, k := range keep {
		v.Tints[cellName(k)] = "keep"
	}
	if d.LastFrom != "" {
		v.Tints[d.LastFrom] = "last"
		v.Tints[d.LastTo] = "last"
	}
	for _, c := range d.LastTook {
		v.Tints[c] = "taken"
	}
	for c := range e.b {
		if e.emp[c] {
			v.Badges[cellName(c)] = append(v.Badges[cellName(c)], "empowered")
		}
	}
	pawn := e.pawnCell()
	var who []string
	if e.controls(seatChallenger) {
		who = append(who, "the challenger")
	}
	if e.controls(seatKing) {
		who = append(who, "the king's side")
	}
	pawnNote := "no one can see the verdigris pawn"
	if len(who) > 0 {
		pawnNote = "the verdigris pawn answers to " + strings.Join(who, " and ")
	}
	switch {
	case d.Turn == 0:
		v.Prompt = reasonText(d.Reason)
	case pawn < 0:
		v.Prompt = seatName(d.Turn) + " to move"
	default:
		v.Prompt = seatName(d.Turn) + " to move — " + pawnNote
		if e.controls(uint8(d.Turn)) {
			v.Badges[cellName(pawn)] = append(v.Badges[cellName(pawn)], "ctrl")
		}
	}
	c := func(id, label string, side, kind uint8) CounterView {
		return CounterView{ID: id, Label: label, Value: e.count(side, kind), Max: total[side][kind]}
	}
	empower := 0
	if d.KingEmpower {
		empower = 1
	}
	v.Seats["1"] = &SeatView{Counters: []CounterView{
		c("blue", "Blue guards", seatChallenger, kBlue),
		c("yellow", "Yellow guards", seatChallenger, kYellow),
		c("mage", "Mage", seatChallenger, kMage),
	}}
	v.Seats["2"] = &SeatView{Counters: []CounterView{
		c("blue", "Blue guards", seatKing, kBlue),
		c("yellow", "Yellow guards", seatKing, kYellow),
		{ID: "empower", Label: "King empowers", Value: empower, Max: 1},
	}}
	return v
}

// Heuristic values a position for seat in [-1, 1]: material left on each
// side, the challenger's blue guards (its life) and how close its strong
// pieces are to the king.
func (rs *Ruleset) Heuristic(game *catalog.CompiledGame, st *state.BoardState, seat int) float64 {
	d, err := dataOf(st)
	if err != nil {
		return 0
	}
	e := newEng(st, d)
	if d.Turn == 0 {
		switch d.Winner {
		case 0:
			return 0
		case seat:
			return 1
		}
		return -1
	}
	var mat [3]float64
	king := -1
	for c, p := range e.b {
		if p.side != 0 {
			mat[p.side] += float64(e.val[c])
		}
		if p.kind == kKing {
			king = c
		}
	}
	x := mat[seatChallenger]/18 - mat[seatKing]/30
	x -= 0.35 * float64(3-e.count(seatChallenger, kBlue))
	if king >= 0 {
		best := 8
		for c, p := range e.b {
			if (p.side == seatChallenger && (p.kind == kAce || p.kind == kMage)) || p.kind == kPawn {
				if dd := maxInt(absInt(c%8-king%8), absInt(c/8-king/8)); dd < best {
					best = dd
				}
			}
		}
		x += 0.05 * float64(8-best)
	}
	if seat == seatKing {
		x = -x
	}
	if x > 1 {
		x = 1
	}
	if x < -1 {
		x = -1
	}
	return x
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// GuideMoves keeps a bot from throwing pieces away: no capturing its own
// pieces, and no leaving an enemy it could take when it jumps.
func (rs *Ruleset) GuideMoves(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) []state.Move {
	d, err := dataOf(st)
	if err != nil {
		return legal
	}
	e := newEng(st, d)
	var out []state.Move
	for _, mv := range legal {
		if mv.Action == actJump && !e.sensibleJump(mv, uint8(seat)) {
			continue
		}
		if mv.Action == actMove && mv.Aim != "" {
			if t, ok := cellOf(mv.Aim); ok && e.b[t].side == uint8(seat) {
				continue
			}
		}
		out = append(out, mv)
	}
	return out
}

// sensibleJump is false for a chain that takes one of its own pieces or
// spares an enemy piece it jumped (the jump over a pawn is left alone).
func (e *eng) sensibleJump(mv state.Move, side uint8) bool {
	cur, _ := cellOf(mv.From)
	board := e.b
	board[cur] = pc{}
	for i, land := range mv.Path {
		to, _ := cellOf(land)
		mid := (cur/8+to/8)/2*8 + (cur%8+to%8)/2
		over := board[mid]
		switch {
		case mv.Drops[i] != jumpSpare && over.side == side:
			return false
		case mv.Drops[i] == jumpSpare && over.side == other(side) && over.kind != kPawn:
			return false
		}
		if mv.Drops[i] != jumpSpare {
			board[mid] = pc{}
		}
		cur = to
	}
	return true
}
