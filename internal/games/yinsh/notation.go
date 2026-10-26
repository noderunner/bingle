package yinsh

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Notation, with the points of the published board (a2–k10):
//
//	e5         place a ring on e5 (the setup)
//	e5-e9      drop a marker in the ring on e5 and move the ring to e9
//	row:b2-b6  remove the five markers from b2 to b6 …
//	ring:e9    … and the ring on e9 with them
//	pass       no ring can move

// Format renders mv.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Move) string {
	switch mv.Action {
	case actPlace:
		return string(mv.To)
	case actMove:
		return fmt.Sprintf("%s-%s", mv.From, mv.To)
	case actRow:
		return fmt.Sprintf("row:%s-%s", mv.From, mv.To)
	case actRemove:
		return "ring:" + string(mv.From)
	}
	return actPass
}

// Parse reads notation produced by Format (case-insensitively).
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Move, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	bad := func(why string) (state.Move, error) {
		return state.Move{}, fmt.Errorf("yinsh: cannot read %q as a move (%s; expected e5, e5-e9, row:b2-b6, ring:e9 or pass)", text, why)
	}
	cell := func(s string) (board.PositionID, bool) {
		id := board.PositionID(s)
		return id, game.Board.HasPosition(id)
	}
	span := func(s string) (from, to board.PositionID, ok bool) {
		a, b, found := strings.Cut(s, "-")
		if !found {
			return "", "", false
		}
		from, ok1 := cell(a)
		to, ok2 := cell(b)
		return from, to, ok1 && ok2
	}
	switch {
	case t == actPass:
		return state.Move{Action: actPass}, nil
	case strings.HasPrefix(t, "row:"):
		from, to, ok := span(t[4:])
		if !ok {
			return bad("two points expected after row:")
		}
		return state.Move{Action: actRow, From: from, To: to}, nil
	case strings.HasPrefix(t, "ring:"):
		from, ok := cell(t[5:])
		if !ok {
			return bad("unknown point " + t[5:])
		}
		return state.Move{Action: actRemove, From: from}, nil
	case strings.Contains(t, "-"):
		from, to, ok := span(t)
		if !ok {
			return bad("unknown point")
		}
		return state.Move{Action: actMove, From: from, To: to}, nil
	}
	to, ok := cell(t)
	if !ok {
		return bad("unknown point " + t)
	}
	return state.Move{Action: actPlace, To: to}, nil
}

// Steps: a point, or two points; pass is an action.
func (rs *Ruleset) Steps(game *catalog.CompiledGame, mv state.Move) []state.Step {
	switch mv.Action {
	case actPlace:
		return []state.Step{{Kind: state.StepPos, Value: string(mv.To)}}
	case actMove, actRow:
		return []state.Step{{Kind: state.StepPos, Value: string(mv.From)}, {Kind: state.StepPos, Value: string(mv.To)}}
	case actRemove:
		return []state.Step{{Kind: state.StepPos, Value: string(mv.From)}}
	}
	return []state.Step{{Kind: state.StepAction, Value: actPass}}
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	info := state.RulesetInfo{
		WinRules: []string{
			fmt.Sprintf("Be the first to remove %d of your rings: each time you make a row of five markers of your colour, you remove the row and one of your rings.", game.ParamInt("rings_to_win", 3)),
			"If the markers run out (all 51 on the board), the player who has removed more rings wins; equal is a draw.",
		},
		NotationHelp: []string{
			"Points are named like the published board: a column letter a–k and a row number (a2–k10). Setup: e5 places a ring there.",
			"e5-e9 drops a marker in your ring on e5 and moves the ring to e9 (turning over the markers it jumps). row:b2-b6 removes five markers from b2 to b6 and ring:e9 then removes your ring on e9. pass is allowed only when no ring can move.",
		},
	}
	if st, err := rs.Start(game, 0); err == nil {
		if moves := rs.Legal(game, st, 1); len(moves) > 0 {
			info.NotationHelp = append(info.NotationHelp, "Example of a legal first move: '"+rs.Format(game, moves[0])+"'.")
		}
	}
	info.Actions = []state.RulesetAction{{ID: actPass, Label: "Pass", Hint: "None of your rings can move: pass the turn."}}
	return info
}
