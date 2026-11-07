package solitaire

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// actMove is the action of every move: the move itself is its notation, in Piece.
const actMove = "move"

// Format writes a move: it is its own notation.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Selection) string { return mv.Piece }

// Parse reads the notation Format writes.
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Selection, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	if _, _, ok := parsePlay(t); !ok && t != moveEnd {
		return state.Selection{}, fmt.Errorf("solitaire: cannot read %q as a move (expected pins@ball, such as e+h@2, or end)", text)
	}
	return state.Selection{Action: actMove, Piece: t}, nil
}

// Gestures are the clicks of a move. A card game's page turns them into clicks on the
// table: the pins and then the ball.
func (rs *Ruleset) Gestures(game *catalog.CompiledGame, mv state.Selection) []state.Gesture {
	act := func(v string) state.Gesture { return state.Gesture{Kind: state.GestureAction, Value: v} }
	m := mv.Piece
	if m == moveEnd {
		return []state.Gesture{act(m)}
	}
	if group, ball, ok := parsePlay(m); ok {
		steps := make([]state.Gesture, 0, len(group)+1)
		for i := 0; i < len(group); i++ {
			steps = append(steps, act("pin-"+group[i:i+1]))
		}
		return append(steps, act(fmt.Sprintf("ball-%d", ball)))
	}
	return nil
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	return state.RulesetInfo{
		Actions: []state.RulesetAction{{ID: moveEnd, Label: "End this ball", Hint: "Stop here and go on to the next ball, though a play is left"}},
		WinRules: []string{
			"Ten frames are bowled and scored as in ten-pin bowling; 300 is a perfect game. There is no winning or losing, only the score.",
		},
		NotationHelp: []string{
			"A play names the pins and the ball pile: e+h@2 knocks down pins e and h with the top card of pile 2. The back row is a b c d, then e f g, then h i, and j at the front.",
			"end stops the ball though a play is left. Example: e+h@2.",
		},
	}
}
