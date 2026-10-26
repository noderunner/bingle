package state

import (
	"strconv"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Step kinds.
const (
	StepPiece = "piece" // choose a piece type to place
	StepPos   = "pos"   // choose a board position
	StepCount = "count" // choose how many pieces to pick up
	// StepAction is a single-click action with no parameters (pass).
	StepAction = "action"
)

// Step is one selection in the sequence of choices that builds a move
// interactively. A client can turn the full list of legal moves into a
// decision tree keyed by steps and walk it click by click, so any game's
// moves are enterable without game-specific UI code.
type Step struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Steps decomposes mv into the ordered selections that build it:
//
//	step        pos(from), pos(to)
//	place       piece(type), pos(to)
//	move-stack  pos(from), count(n), then pos(p) once per piece dropped on p
//	            (so drops of 1,2 over squares d3,e3 are d3, e3, e3)
func Steps(game *catalog.CompiledGame, mv Move) []Step {
	if rs, ok := RulesetOf(game); ok {
		return rs.Steps(game, mv)
	}
	act, ok := game.Action(mv.Action)
	if !ok {
		return nil
	}
	switch act.Type {
	case catalog.ActionPass, catalog.ActionSwap:
		return []Step{{StepAction, act.ID}}
	case catalog.ActionPlace:
		return []Step{{StepPiece, mv.Piece}, {StepPos, string(mv.To)}}
	case catalog.ActionMoveStack:
		steps := []Step{{StepPos, string(mv.From)}, {StepCount, strconv.Itoa(mv.Count)}}
		for i, p := range mv.Path {
			for d := 0; d < mv.Drops[i]; d++ {
				steps = append(steps, Step{StepPos, string(p)})
			}
		}
		return steps
	case catalog.ActionJump:
		steps := []Step{{StepPos, string(mv.From)}}
		for _, p := range mv.Path {
			steps = append(steps, Step{StepPos, string(p)})
		}
		return steps
	case catalog.ActionSlide:
		steps := []Step{{StepPos, string(mv.From)}, {StepPos, string(mv.To)}}
		if mv.Aim != "" {
			steps = append(steps, Step{StepPos, string(mv.Aim)})
		}
		return steps
	default:
		return []Step{{StepPos, string(mv.From)}, {StepPos, string(mv.To)}}
	}
}
