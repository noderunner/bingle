package blooms

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Notation: a stone is its colour, an @ and the cell (red@d4); a second stone
// of the same turn is written the same way; done ends the turn after one stone.

// Format renders mv.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Move) string {
	if mv.Action == actDone {
		return actDone
	}
	return fmt.Sprintf("%s@%s", mv.Piece, mv.To)
}

// Parse reads notation produced by Format (case-insensitively).
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Move, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == actDone {
		return state.Move{Action: actDone}, nil
	}
	i := strings.IndexByte(t, '@')
	if i < 0 {
		return state.Move{}, fmt.Errorf("blooms: cannot read %q as a move (expected red@d4 or done)", text)
	}
	color, cell := t[:i], board.PositionID(t[i+1:])
	if ownerOf(color) == 0 {
		return state.Move{}, fmt.Errorf("blooms: unknown colour %q (red, yellow, blue, black)", color)
	}
	if !game.Board.HasPosition(cell) {
		return state.Move{}, fmt.Errorf("blooms: unknown cell %q", cell)
	}
	return state.Move{Action: actPlace, Piece: color, To: cell}, nil
}

// Steps: the stone's colour, then the cell; done is an action.
func (rs *Ruleset) Steps(game *catalog.CompiledGame, mv state.Move) []state.Step {
	if mv.Action == actDone {
		return []state.Step{{Kind: state.StepAction, Value: actDone}}
	}
	return []state.Step{{Kind: state.StepPiece, Value: mv.Piece}, {Kind: state.StepPos, Value: string(mv.To)}}
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	info := state.RulesetInfo{
		WinRules: []string{
			fmt.Sprintf("Be the first to capture %d enemy stones (blooms captured when the turn ends).", game.ParamInt("capture_target", 20)),
			fmt.Sprintf("After %d plies the player with more captures wins (equal is a draw).", game.ParamInt("max_plies", 500)),
		}, NotationHelp: []string{
			"A stone is its colour, @ and a cell: red@d4. Player 1 has red and yellow, player 2 blue and black.",
			"A turn is one stone, or two stones of different colours (the second is another move); done ends the turn after one.",
		}}
	if st, err := rs.Start(game, 0); err == nil {
		if moves := rs.Legal(game, st, 1); len(moves) > 0 {
			info.NotationHelp = append(info.NotationHelp, "Example of a legal first move: '"+rs.Format(game, moves[0])+"'.")
		}
	}
	info.Actions = []state.RulesetAction{{ID: actDone, Label: "End turn", Hint: "Place no second stone: end the turn and capture any fenced enemy blooms."}}
	return info
}
