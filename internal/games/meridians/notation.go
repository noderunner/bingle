package meridians

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Notation: a stone is simply the point it goes on (g7), as in Go or Hex; a
// turn that can lay no stone is "pass".

// Format renders mv.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Move) string {
	if mv.Action == actPass {
		return actPass
	}
	return string(mv.To)
}

// Parse reads notation produced by Format (case-insensitively).
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Move, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == actPass {
		return state.Move{Action: actPass}, nil
	}
	point := board.PositionID(t)
	if !game.Board.HasPosition(point) {
		return state.Move{}, fmt.Errorf("meridians: %q is not a point of this board", text)
	}
	return state.Move{Action: actPlace, Piece: "stone", To: point}, nil
}

// Steps: a stone is one click on a point; a pass is a button.
func (rs *Ruleset) Steps(game *catalog.CompiledGame, mv state.Move) []state.Step {
	if mv.Action == actPass {
		return []state.Step{{Kind: state.StepAction, Value: actPass}}
	}
	return []state.Step{{Kind: state.StepPos, Value: string(mv.To)}}
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	info := state.RulesetInfo{
		WinRules: []string{
			"Win when your opponent's turn would begin with no stones of theirs on the board.",
			fmt.Sprintf("After %d plies an unfinished game is a draw, as it is when neither player can lay a stone.", game.ParamInt("max_plies", 1200)),
		},
		NotationHelp: []string{
			"A move is the point the stone goes on: g7. Columns are letters from the left, rows numbers from the top.",
			"A turn with nowhere to lay a stone is 'pass' — the only time a turn may pass.",
		},
	}
	if st, err := rs.Start(game, 0); err == nil {
		if moves := rs.Legal(game, st, 1); len(moves) > 0 {
			info.NotationHelp = append(info.NotationHelp, "Example of a legal first move: '"+rs.Format(game, moves[0])+"'.")
		}
	}
	info.Actions = []state.RulesetAction{{ID: actPass, Label: "Pass", Hint: "There is nowhere your stones can see: the turn passes."}}
	return info
}
