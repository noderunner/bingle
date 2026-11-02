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
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Move) string { return mv.Piece }

// Parse reads the notation Format writes.
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Move, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	var err error
	if game.ParamString("mode", modeKlondike) == modeBowling {
		if _, _, ok := parsePlay(t); !ok && t != moveEnd {
			err = fmt.Errorf("expected pins@ball, such as e+h@2, or end")
		}
	} else {
		err = validPatience(t)
	}
	if err != nil {
		return state.Move{}, fmt.Errorf("solitaire: cannot read %q as a move (%w)", text, err)
	}
	return state.Move{Action: actMove, Piece: t}, nil
}

// Steps are the clicks of a move. A card game's page turns them into clicks on the
// table: the cards to lift and where they go, or the pins and then the ball.
func (rs *Ruleset) Steps(game *catalog.CompiledGame, mv state.Move) []state.Step {
	act := func(v string) state.Step { return state.Step{Kind: state.StepAction, Value: v} }
	m := mv.Piece
	switch m {
	case moveDraw, moveRecycle, moveDeal, moveFinish, moveEnd:
		return []state.Step{act(m)}
	}
	if group, ball, ok := parsePlay(m); ok {
		steps := make([]state.Step, 0, len(group)+1)
		for i := 0; i < len(group); i++ {
			steps = append(steps, act("pin-"+group[i:i+1]))
		}
		return append(steps, act(fmt.Sprintf("ball-%d", ball)))
	}
	from, to, _ := strings.Cut(m, "-")
	pile, n := parseFrom(from)
	return []state.Step{act(fmt.Sprintf("from-%s-%d", pile, n)), act("to-" + to)}
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	if game.ParamString("mode", modeKlondike) == modeBowling {
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
	return state.RulesetInfo{
		Actions: []state.RulesetAction{{ID: moveFinish, Label: "Finish", Hint: "Everything can go home from where it lies: send it"}},
		WinRules: []string{
			"You win when every card is home. A game with no move left ends without a win, and Give up ends one at any time.",
		},
		NotationHelp: []string{
			"A move is from-to, between the piles t1…t10 (columns), w (waste), c1…c4 (free cells) and f1…f4 (foundations). t3-t5 moves one card; t3.2-t5 moves two.",
			"t3-f sends a card to its foundation and t3-c to a free cell. draw and recycle use the stock, deal deals Spider's next row, and finish sends everything home once nothing can stop it. Example: t3.2-t5.",
		},
	}
}
