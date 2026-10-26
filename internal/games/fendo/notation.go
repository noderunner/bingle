package fendo

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Notation: a piece that moves is its cell, a dash, the cell it ends on, a
// plus and the side of that cell the fence goes on (d1-d4+n); a piece that
// stays leaves out the dash and the second cell (d4+e); an added piece is
// add@c3; pass.

func boardID(s string) board.PositionID { return board.PositionID(s) }

// Format renders mv.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Move) string {
	switch mv.Action {
	case actMove:
		if mv.From == mv.To {
			return fmt.Sprintf("%s+%s", mv.From, mv.Piece)
		}
		return fmt.Sprintf("%s-%s+%s", mv.From, mv.To, mv.Piece)
	case actAdd:
		return "add@" + string(mv.To)
	}
	return actPass
}

// Parse reads notation produced by Format (case-insensitively).
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Move, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	bad := func(why string) (state.Move, error) {
		return state.Move{}, fmt.Errorf("fendo: cannot read %q as a move (%s; expected d1-d4+n, d4+e, add@c3 or pass)", text, why)
	}
	cell := func(s string) (board.PositionID, bool) {
		id := board.PositionID(s)
		return id, game.Board.HasPosition(id)
	}
	switch {
	case t == actPass:
		return state.Move{Action: actPass}, nil
	case strings.HasPrefix(t, "add@"):
		to, ok := cell(t[4:])
		if !ok {
			return bad("unknown cell " + t[4:])
		}
		return state.Move{Action: actAdd, To: to}, nil
	case strings.Contains(t, "+"):
		path, side, _ := strings.Cut(t, "+")
		if _, ok := sideOf(side); !ok {
			return bad("the fence side must be n, e, s or w")
		}
		a, b, moves := strings.Cut(path, "-")
		from, ok := cell(a)
		if !ok {
			return bad("unknown cell " + a)
		}
		to := from
		if moves {
			if to, ok = cell(b); !ok {
				return bad("unknown cell " + b)
			}
		}
		return state.Move{Action: actMove, From: from, To: to, Piece: side}, nil
	}
	return bad("not a move")
}

// Steps: the piece, where it goes (if it moves) and the side of the fence; an
// added piece is the Add action and the cell.
func (rs *Ruleset) Steps(game *catalog.CompiledGame, mv state.Move) []state.Step {
	switch mv.Action {
	case actAdd:
		return []state.Step{{Kind: state.StepAction, Value: actAdd}, {Kind: state.StepPos, Value: string(mv.To)}}
	case actMove:
		steps := []state.Step{{Kind: state.StepPos, Value: string(mv.From)}}
		if mv.To != mv.From {
			steps = append(steps, state.Step{Kind: state.StepPos, Value: string(mv.To)})
		}
		return append(steps, state.Step{Kind: state.StepAction, Value: "fence-" + mv.Piece})
	}
	return []state.Step{{Kind: state.StepAction, Value: actPass}}
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	info := state.RulesetInfo{
		WinRules: []string{
			"The game ends when the last open area is closed; the player who owns more squares in areas closed round one piece wins (a tie is a draw).",
			"On the 9×9 and 11×11 boards the first player to own a majority of the squares wins at once.",
		}, NotationHelp: []string{
			"Moving a piece and fencing: its cell, a dash, the cell it ends on, a plus and the side of that cell for the fence (n, e, s or w): d1-d4+n. A piece that stays leaves out the dash and the second cell: d4+e.",
			"Adding a piece: add@c3. pass is allowed only when you have no other action.",
		}}
	if st, err := rs.Start(game, 0); err == nil {
		if moves := rs.Legal(game, st, 1); len(moves) > 0 {
			info.NotationHelp = append(info.NotationHelp, "Example of a legal first move: '"+rs.Format(game, moves[0])+"'.")
		}
	}
	info.Actions = []state.RulesetAction{{ID: actAdd, Label: "Add a piece", Hint: "Put a piece from your stock one move away from one of yours in the open area; no fence."}}
	for d := 0; d < 4; d++ {
		name := sideName[d]
		info.Actions = append(info.Actions, state.RulesetAction{
			ID:    "fence-" + sideLetter[d],
			Label: "Fence " + name,
			Hint:  "Build the fence on the " + name + " side of the piece's square.",
		})
	}
	info.Actions = append(info.Actions, state.RulesetAction{ID: actPass, Label: "Pass", Hint: "You have no action: pass the turn."})
	return info
}
