package mycelium

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Notation: a placement is the colour, an @ and the cell (black@d4). In a
// confluence, "spore d3 d7" is a spore spread from d3 converting d7; "hyphae
// d4 e4" starts a hyphae spread at the strand's end d4 by converting e4, and
// "hyphae f4" converts f4 next in the spread under way; done ends that spread
// early.

// Format renders mv.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Selection) string {
	switch mv.Action {
	case actPlace:
		return fmt.Sprintf("%s@%s", mv.Piece, mv.To)
	case actSpore:
		return fmt.Sprintf("spore %s %s", mv.From, mv.To)
	case actHyphae:
		if mv.From == "" {
			return fmt.Sprintf("hyphae %s", mv.To)
		}
		return fmt.Sprintf("hyphae %s %s", mv.From, mv.To)
	}
	return mv.Action
}

// Parse reads notation produced by Format (case-insensitively).
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Selection, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == actDone {
		return state.Selection{Action: actDone}, nil
	}
	if i := strings.IndexByte(t, '@'); i >= 0 {
		color, cell := t[:i], board.PositionID(t[i+1:])
		if ownerOf(color) == 0 {
			return state.Selection{}, fmt.Errorf("mycelium: unknown colour %q (black, green)", color)
		}
		if !game.Board.HasPosition(cell) {
			return state.Selection{}, fmt.Errorf("mycelium: unknown cell %q", cell)
		}
		return state.Selection{Action: actPlace, Piece: color, To: cell}, nil
	}
	f := strings.Fields(t)
	if len(f) < 2 || (f[0] != actSpore && f[0] != actHyphae) {
		return state.Selection{}, fmt.Errorf("mycelium: cannot read %q as a move (expected black@d4, spore d3 d7, hyphae d4 e4, hyphae f4 or done)", text)
	}
	cells := make([]board.PositionID, len(f)-1)
	for i, c := range f[1:] {
		cells[i] = board.PositionID(c)
		if !game.Board.HasPosition(cells[i]) {
			return state.Selection{}, fmt.Errorf("mycelium: unknown cell %q", c)
		}
	}
	switch {
	case len(cells) == 2:
		return state.Selection{Action: f[0], From: cells[0], To: cells[1]}, nil
	case len(cells) == 1 && f[0] == actHyphae:
		return state.Selection{Action: actHyphae, To: cells[0]}, nil
	}
	return state.Selection{}, fmt.Errorf("mycelium: %q needs a node and the cell it converts", text)
}

// Gestures: a placement is the colour then the cell; a spread is the spreading
// node then the cell it converts (only the cell, for the next conversion of a
// hyphae spread); done is an action. Which kind of spread a click makes is
// never in doubt, because each phase of a confluence has only one kind.
func (rs *Ruleset) Gestures(game *catalog.CompiledGame, mv state.Selection) []state.Gesture {
	switch mv.Action {
	case actPlace:
		return []state.Gesture{{Kind: state.GesturePiece, Value: mv.Piece}, {Kind: state.GesturePos, Value: string(mv.To)}}
	case actSpore, actHyphae:
		if mv.From == "" {
			return []state.Gesture{{Kind: state.GesturePos, Value: string(mv.To)}}
		}
		return []state.Gesture{{Kind: state.GesturePos, Value: string(mv.From)}, {Kind: state.GesturePos, Value: string(mv.To)}}
	}
	return []state.Gesture{{Kind: state.GestureAction, Value: mv.Action}}
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	info := state.RulesetInfo{
		WinRules: []string{
			"Wipe out the other colony in a confluence: a player left with no nodes when it ends loses.",
			"If a confluence wipes out both colonies, the player who took the centre loses.",
			fmt.Sprintf("If no confluence has decided the game after %d plies, it is drawn.", game.ParamInt("max_plies", 500)),
		},
		NotationHelp: []string{
			"Place a node: black@d4 (Black) or green@d4 (Green). Taking the centre, e5, starts a confluence.",
			"A spore spread is the spreading node and the enemy node it converts: spore d3 d7.",
			"A hyphae spread converts one node at a time: hyphae d4 e4 starts from the strand's end d4, then hyphae f4 converts the next; done stops early.",
		},
	}
	if st, err := rs.Start(game, 0); err == nil {
		if moves := rs.Legal(game, st, 1); len(moves) > 0 {
			info.NotationHelp = append(info.NotationHelp, "Example of a legal first move: '"+rs.Format(game, moves[0])+"'.")
		}
	}
	return info
}
