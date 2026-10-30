package mycelium

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Notation: normal placement is colour@cell (black@e5). A spore spread is
// "spore <src> <dst>". A hyphae spread is "hyphae <strand> <targets...>",
// where a multi-cell strand is written with dashes (e5-e6) and targets are
// space-separated cells.

// Format renders mv.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Move) string {
	switch mv.Action {
	case actPlace:
		return fmt.Sprintf("%s@%s", mv.Piece, mv.To)
	case actSpore:
		return fmt.Sprintf("spore %s %s", mv.From, mv.To)
	case actHyphae:
		strand := string(mv.From)
		if mv.From != mv.To {
			strand = fmt.Sprintf("%s-%s", mv.From, mv.To)
		}
		targets := make([]string, len(mv.Path))
		for i, p := range mv.Path {
			targets[i] = string(p)
		}
		return fmt.Sprintf("hyphae %s %s", strand, strings.Join(targets, " "))
	}
	return ""
}

// Parse reads notation produced by Format (case-insensitively).
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Move, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	if i := strings.IndexByte(t, '@'); i >= 0 {
		// colour@cell
		color, cell := t[:i], board.PositionID(t[i+1:])
		if ownerOf(color) == 0 {
			return state.Move{}, fmt.Errorf("mycelium: unknown colour %q (black, green)", color)
		}
		if !game.Board.HasPosition(cell) {
			return state.Move{}, fmt.Errorf("mycelium: unknown cell %q", cell)
		}
		return state.Move{Action: actPlace, Piece: color, To: cell}, nil
	}
	fields := strings.Fields(t)
	if len(fields) == 0 {
		return state.Move{}, fmt.Errorf("mycelium: empty move")
	}
	switch fields[0] {
	case actSpore:
		if len(fields) != 3 {
			return state.Move{}, fmt.Errorf("mycelium: spore spread needs source and target (spore e5 e7)")
		}
		from, to := board.PositionID(fields[1]), board.PositionID(fields[2])
		if !game.Board.HasPosition(from) || !game.Board.HasPosition(to) {
			return state.Move{}, fmt.Errorf("mycelium: unknown cell in %q", text)
		}
		return state.Move{Action: actSpore, From: from, To: to}, nil
	case actHyphae:
		if len(fields) < 3 {
			return state.Move{}, fmt.Errorf("mycelium: hyphae spread needs a strand and at least one target")
		}
		strandParts := strings.Split(fields[1], "-")
		from := board.PositionID(strandParts[0])
		to := from
		if len(strandParts) == 2 {
			to = board.PositionID(strandParts[1])
		} else if len(strandParts) > 2 {
			return state.Move{}, fmt.Errorf("mycelium: strand %q is not a single cell or dash pair", fields[1])
		}
		if !game.Board.HasPosition(from) || !game.Board.HasPosition(to) {
			return state.Move{}, fmt.Errorf("mycelium: unknown strand cell in %q", text)
		}
		path := make([]board.PositionID, 0, len(fields)-2)
		for _, f := range fields[2:] {
			p := board.PositionID(f)
			if !game.Board.HasPosition(p) {
				return state.Move{}, fmt.Errorf("mycelium: unknown target cell %q", f)
			}
			path = append(path, p)
		}
		return state.Move{Action: actHyphae, From: from, To: to, Path: path}, nil
	}
	return state.Move{}, fmt.Errorf("mycelium: cannot read %q as a move", text)
}

// Steps decomposes mv into the ordered selections that build it.
func (rs *Ruleset) Steps(game *catalog.CompiledGame, mv state.Move) []state.Step {
	switch mv.Action {
	case actPlace:
		return []state.Step{{Kind: state.StepPiece, Value: mv.Piece}, {Kind: state.StepPos, Value: string(mv.To)}}
	case actSpore:
		return []state.Step{
			{Kind: state.StepAction, Value: actSpore},
			{Kind: state.StepPos, Value: string(mv.From)},
			{Kind: state.StepPos, Value: string(mv.To)},
		}
	case actHyphae:
		steps := []state.Step{{Kind: state.StepAction, Value: actHyphae}}
		strand := string(mv.From)
		if mv.From != mv.To {
			strand = fmt.Sprintf("%s-%s", mv.From, mv.To)
		}
		steps = append(steps, state.Step{Kind: state.StepPos, Value: strand})
		for _, p := range mv.Path {
			steps = append(steps, state.Step{Kind: state.StepPos, Value: string(p)})
		}
		return steps
	}
	return nil
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	info := state.RulesetInfo{
		WinRules: []string{
			"Trigger a confluence from the centre and spread clusters to eliminate the opponent's nodes.",
			"If both colonies are wiped out in a confluence, the player who did not initiate it wins.",
			"If no confluence ends the game, it is drawn after 500 plies.",
		},
		NotationHelp: []string{
			"Place a node: black@e5 or green@f5.",
			"Spore spread: spore e5 e7 (source then target).",
			"Hyphae spread: hyphae e5-e6 e7 e8 (strand then target cells).",
		},
	}
	if st, err := rs.Start(game, 0); err == nil {
		if moves := rs.Legal(game, st, 1); len(moves) > 0 {
			info.NotationHelp = append(info.NotationHelp, "Example of a legal first move: '"+rs.Format(game, moves[0])+"'.")
		}
	}
	return info
}
