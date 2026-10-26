package onitama

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Notation: the card, a colon, then the squares.
//
//	tiger:c1-c3     use the Tiger card to move the piece on c1 to c3
//	tiger:pass      nothing can move: pass, but still spend the Tiger card

// Format renders mv.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Move) string {
	if mv.Action == actPass {
		return mv.Piece + ":pass"
	}
	return fmt.Sprintf("%s:%s-%s", mv.Piece, mv.From, mv.To)
}

// Parse reads notation produced by Format (case-insensitively).
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Move, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	bad := func(why string) (state.Move, error) {
		return state.Move{}, fmt.Errorf("onitama: cannot read %q as a move (%s)", text, why)
	}
	i := strings.IndexByte(t, ':')
	if i < 0 {
		return bad("expected card:from-to, e.g. tiger:c1-c3")
	}
	card, rest := t[:i], t[i+1:]
	if _, ok := cardByID(card); !ok {
		return bad("unknown card " + card)
	}
	if rest == "pass" {
		return state.Move{Action: actPass, Piece: card}, nil
	}
	parts := strings.Split(rest, "-")
	if len(parts) != 2 {
		return bad("expected from-to")
	}
	square := func(s string) (board.PositionID, bool) {
		id := board.PositionID(s)
		_, ok := cellOf(id)
		return id, ok && game.Board.HasPosition(id)
	}
	from, ok1 := square(parts[0])
	to, ok2 := square(parts[1])
	if !ok1 || !ok2 {
		return bad("unknown square")
	}
	return state.Move{Action: actMove, Piece: card, From: from, To: to}, nil
}

// Steps: choose the card, then the piece and where it goes.
func (rs *Ruleset) Steps(game *catalog.CompiledGame, mv state.Move) []state.Step {
	act := func(id string) state.Step { return state.Step{Kind: state.StepAction, Value: id} }
	pos := func(id board.PositionID) state.Step { return state.Step{Kind: state.StepPos, Value: string(id)} }
	if mv.Action == actPass {
		return []state.Step{act("card-" + mv.Piece), act(actPass)}
	}
	return []state.Step{act("card-" + mv.Piece), pos(mv.From), pos(mv.To)}
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	info := state.RulesetInfo{
		WinRules: []string{
			"Win by capturing the opponent's Master (Way of the Stone), or by moving your Master onto the opponent's Temple Arch (Way of the Stream).",
		},
		NotationHelp: []string{
			"A move is the card, a colon and the squares: tiger:c1-c3 moves the piece on c1 to c3 with the Tiger card.",
			"When nothing can move you pass, spending a card all the same: tiger:pass.",
			"Cards: tiger dragon frog rabbit crab elephant goose rooster monkey mantis horse ox crane boar eel cobra.",
		},
	}
	if st, err := rs.Start(game, 1); err == nil {
		if d, err := dataOf(st); err == nil {
			if moves := rs.Legal(game, st, d.Turn); len(moves) > 0 {
				info.NotationHelp = append(info.NotationHelp, "Example of a legal first move: '"+rs.Format(game, moves[0])+"'.")
			}
		}
	}
	for _, c := range cards {
		info.Actions = append(info.Actions, state.RulesetAction{ID: "card-" + c.ID, Label: c.Name, Hint: "Move a piece as the " + c.Name + " card shows, then swap it for the card at the side."})
	}
	info.Actions = append(info.Actions, state.RulesetAction{ID: actPass, Label: "Pass", Hint: "Nothing can move: pass, but still swap the card."})
	return info
}
