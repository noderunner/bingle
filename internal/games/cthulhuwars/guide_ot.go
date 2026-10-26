package cthulhuwars

import (
	"github.com/noderunner/bingle/internal/engine/state"
)

// What the bots do with the Opener of the Way and the Tcho-Tcho. They use
// the spellbooks that reward plain play — promoting units, Channel Power,
// Terror, Dragon Descending with many Gates — and Hell's Banquet; they leave
// The Beyond One and Idolatry to people.

// scoreOT values the turn actions of the two cults.
func (g *guide) scoreOT(mv state.Move, area int) (float64, bool) {
	e, d, seat := g.e, g.e.d, g.seat
	switch mv.Action {
	case actBeyond:
		// Dragging a Gate and its keeper away from an enemy is a way to take it;
		// the bots do not plan that far.
		return 6, true
	case actIdolatry:
		return 5, true
	case actDismantle:
		// The requirement is worth a Gate once the other books are earned.
		if d.bookCount(seat) >= 4 {
			return 58, true
		}
		return 4, true
	case actCurse:
		s := 20.0 + 9*float64(e.curseCount(seat))
		for _, o := range e.others(seat) {
			s += 3 * float64(e.total(area, o))
			s += 8 * float64(e.classCount(area, o, classGOO))
		}
		if e.total(area, seat) > 0 {
			s -= 15
		}
		return s, true
	case actAscend:
		if d.Power[seat] <= 1 && d.Power[mv.Count] >= d.Power[seat]+4 {
			return 85, true
		}
		return 2, true
	}
	return 0, false
}

// promptOT answers the prompts of the two cults.
func (g *guide) promptOT(p Prompt, legal []state.Move) ([]state.Move, bool) {
	e, d, seat := g.e, g.e.d, g.seat
	var list []scoredMove
	switch p.Kind {
	case "channel":
		for _, mv := range legal {
			s := scoreDoneTurn
			if mv.Action == actChannel && d.Power[seat] >= 3 {
				s = 70
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "hiero":
		for _, mv := range legal {
			a := e.areaIdx(string(mv.To))
			list = append(list, scoredMove{mv, float64(e.total(a, seat)) - 3*float64(g.enemyRank(a))})
		}
		return top(list, 0.01), true
	case "syco":
		for _, mv := range legal {
			s := 0.0
			goal := e.game.ParamInt("doom_to_win", 30)
			switch mv.Piece {
			case "give":
				s = 10
				if d.Doom[mv.Count]+1 >= goal-3 {
					s = -10
				}
			case "less":
				s = 5
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "curse-target":
		for _, mv := range legal {
			list = append(list, scoredMove{mv, float64(d.Doom[mv.Count]) + float64(d.bookCount(mv.Count))})
		}
		return top(list, 0.01), true
	case "curse-hit":
		for _, mv := range legal {
			a := e.areaIdx(p.Area)
			list = append(list, scoredMove{mv, -g.unitValue(seat, mv.Piece, a)})
		}
		return top(list, 0.01), true
	case "curse-move":
		for _, mv := range legal {
			a := e.areaIdx(string(mv.To))
			s := -float64(e.total(a, mv.Count)) // the lonelier the better
			if e.d.Curse != nil {
				s += 2 * float64(e.total(a, e.d.Curse.Seat))
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "idol":
		for _, mv := range legal {
			if mv.Action == actDone {
				return []state.Move{mv}, true
			}
		}
	}
	return nil, false
}
