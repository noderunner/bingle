package fist

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Notation (squares are a1–h8, no spaces):
//
//	e5-e6        the piece on e5 steps to e6
//	e5-e6xf6     …and pins the piece on f6
//	a1^c1        a jump from a1 to c1, over b1, taking nothing
//	a1^c1x       …capturing the piece on b1
//	a1^c1c       …capturing it and (the Ace) replacing it with one of its own
//	a1^c1x^e3c   a chain of jumps, each with its own choice

// Format renders mv.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Move) string {
	switch mv.Action {
	case actMove:
		s := fmt.Sprintf("%s-%s", mv.From, mv.To)
		if mv.Aim != "" {
			s += "x" + string(mv.Aim)
		}
		return s
	case actJump:
		var b strings.Builder
		b.WriteString(string(mv.From))
		for i, p := range mv.Path {
			b.WriteByte('^')
			b.WriteString(string(p))
			if i < len(mv.Drops) {
				switch mv.Drops[i] {
				case jumpCapture:
					b.WriteByte('x')
				case jumpConvert:
					b.WriteByte('c')
				}
			}
		}
		return b.String()
	}
	return mv.Action
}

// Parse reads notation produced by Format (case-insensitively).
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Move, error) {
	t := strings.ToLower(strings.TrimSpace(text))
	bad := func(why string) (state.Move, error) {
		return state.Move{}, fmt.Errorf("fist: cannot read %q as a move (%s)", text, why)
	}
	square := func(s string) (board.PositionID, bool) {
		id := board.PositionID(s)
		_, ok := cellOf(id)
		return id, ok && game.Board.HasPosition(id)
	}
	if t == "" {
		return bad("empty")
	}
	if strings.Contains(t, "^") {
		parts := strings.Split(t, "^")
		from, ok := square(parts[0])
		if !ok {
			return bad("unknown square " + parts[0])
		}
		mv := state.Move{Action: actJump, From: from}
		for _, leg := range parts[1:] {
			drop := jumpSpare
			switch {
			case strings.HasSuffix(leg, "x"):
				drop, leg = jumpCapture, leg[:len(leg)-1]
			case strings.HasSuffix(leg, "c"):
				drop, leg = jumpConvert, leg[:len(leg)-1]
			}
			land, ok := square(leg)
			if !ok {
				return bad("unknown square " + leg)
			}
			mv.Path = append(mv.Path, land)
			mv.Drops = append(mv.Drops, drop)
		}
		if len(mv.Path) == 0 {
			return bad("a jump needs a landing square")
		}
		mv.To = mv.Path[len(mv.Path)-1]
		return mv, nil
	}
	dash := strings.IndexByte(t, '-')
	if dash < 0 {
		return bad("expected e5-e6, e5-e6xf6 or a1^c1x")
	}
	from, ok := square(t[:dash])
	if !ok {
		return bad("unknown square " + t[:dash])
	}
	rest := t[dash+1:]
	aim := ""
	if i := strings.IndexByte(rest, 'x'); i >= 0 {
		rest, aim = rest[:i], rest[i+1:]
	}
	to, ok := square(rest)
	if !ok {
		return bad("unknown square " + rest)
	}
	mv := state.Move{Action: actMove, From: from, To: to}
	if aim != "" {
		a, ok := square(aim)
		if !ok {
			return bad("unknown square " + aim)
		}
		mv.Aim = a
	}
	return mv, nil
}

// The action steps of a move that builds on the board's clicks.
const (
	stepPlain   = "move"
	stepSpare   = "spare"
	stepCapture = "capture"
	stepConvert = "convert"
	stepEnd     = "end"
)

// Steps decomposes a move into clicks: the piece, then where it goes; a step
// ends with the square pinned or the choice "move"; a chain gives each
// landing square and what happens to the piece jumped, then "end".
func (rs *Ruleset) Steps(game *catalog.CompiledGame, mv state.Move) []state.Step {
	pos := func(id board.PositionID) state.Step { return state.Step{Kind: state.StepPos, Value: string(id)} }
	act := func(id string) state.Step { return state.Step{Kind: state.StepAction, Value: id} }
	switch mv.Action {
	case actMove:
		if mv.Aim != "" {
			return []state.Step{pos(mv.From), pos(mv.To), pos(mv.Aim)}
		}
		return []state.Step{pos(mv.From), pos(mv.To), act(stepPlain)}
	case actJump:
		out := []state.Step{pos(mv.From)}
		for i, p := range mv.Path {
			out = append(out, pos(p))
			switch mv.Drops[i] {
			case jumpCapture:
				out = append(out, act(stepCapture))
			case jumpConvert:
				out = append(out, act(stepConvert))
			default:
				out = append(out, act(stepSpare))
			}
		}
		return append(out, act(stepEnd))
	}
	return nil
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	info := state.RulesetInfo{
		WinRules: []string{
			"The challenger wins by capturing the king; the king's side wins by capturing all three of the challenger's blue guards.",
			"A side with no legal move draws, and the game is drawn after 400 plies.",
		},
		NotationHelp: []string{
			"Squares are a1–h8. A step is e5-e6; add x and a square to pin: e5-e6xf6.",
			"A jump is a1^c1 (over b1); add x to capture the piece jumped, or c to capture and replace it (the Ace): a1^c1x^e3.",
			"Pieces: Y yellow guard, B blue guard, K king, A ace, M mage, P verdigris pawn.",
		},
	}
	if st, err := rs.Start(game, 1); err == nil {
		if moves := rs.Legal(game, st, seatChallenger); len(moves) > 0 {
			info.NotationHelp = append(info.NotationHelp, "Example of a legal first move: '"+rs.Format(game, moves[0])+"'.")
		}
	}
	add := func(id, label, hint string) {
		info.Actions = append(info.Actions, state.RulesetAction{ID: id, Label: label, Hint: hint})
	}
	add(stepPlain, "Move (no capture)", "End the move here without pinning anything.")
	add(stepSpare, "Leave it", "Jump over the piece without capturing it.")
	add(stepCapture, "Capture it", "Remove the piece you jumped.")
	add(stepConvert, "Capture and replace", "Ace: capture it and put one of your own of that kind in its place.")
	add(stepEnd, "End the move", "Stop jumping here.")
	return info
}
