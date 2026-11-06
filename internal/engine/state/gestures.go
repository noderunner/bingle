package state

import (
	"strconv"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Gesture kinds.
const (
	GesturePiece = "piece" // choose a piece type to place
	GesturePos   = "pos"   // choose a board position
	GestureCount = "count" // choose how many pieces to pick up
	// GestureAction is a single-click action with no parameters (pass).
	GestureAction = "action"
)

// Gesture is one thing a player does in an interface to build a selection: a
// click on a piece, a square, a count. A client can turn the legal selections
// into a decision tree keyed by gestures and walk it click by click, so any
// game's selections are enterable without game-specific UI code. The rules
// never depend on gestures (ADR 0114).
type Gesture struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Gestures decomposes mv into the ordered gestures that build it:
//
//	step        pos(from), pos(to)
//	place       piece(type), pos(to)
//	move-stack  pos(from), count(n), then pos(p) once per piece dropped on p
//	            (so drops of 1,2 over squares d3,e3 are d3, e3, e3)
func Gestures(game *catalog.CompiledGame, mv Selection) []Gesture {
	if rs, ok := RulesetOf(game); ok {
		return rs.Gestures(game, mv)
	}
	act, ok := game.Action(mv.Action)
	if !ok {
		return nil
	}
	switch act.Type {
	case catalog.ActionPass, catalog.ActionSwap:
		return []Gesture{{GestureAction, act.ID}}
	case catalog.ActionPlace:
		return []Gesture{{GesturePiece, mv.Piece}, {GesturePos, string(mv.To)}}
	case catalog.ActionMoveStack:
		steps := []Gesture{{GesturePos, string(mv.From)}, {GestureCount, strconv.Itoa(mv.Count)}}
		for i, p := range mv.Path {
			for d := 0; d < mv.Drops[i]; d++ {
				steps = append(steps, Gesture{GesturePos, string(p)})
			}
		}
		return steps
	case catalog.ActionJump:
		steps := []Gesture{{GesturePos, string(mv.From)}}
		for _, p := range mv.Path {
			steps = append(steps, Gesture{GesturePos, string(p)})
		}
		return steps
	case catalog.ActionSlide:
		steps := []Gesture{{GesturePos, string(mv.From)}, {GesturePos, string(mv.To)}}
		if mv.Aim != "" {
			steps = append(steps, Gesture{GesturePos, string(mv.Aim)})
		}
		return steps
	default:
		return []Gesture{{GesturePos, string(mv.From)}, {GesturePos, string(mv.To)}}
	}
}
