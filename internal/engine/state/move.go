package state

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Move is one player action in structured form. Which fields matter depends
// on the action:
//
//   - "step", "slide": From, To (and Aim when the slide has a follow-up)
//   - "place": Piece, To
//   - "move-stack" actions: From, Count, Path, Drops (To mirrors the last
//     path square once a move has been normalized)
//
// The zero Action means "infer from the other fields" (see Normalize).
type Move struct {
	Action string             `json:"action,omitempty"`
	Piece  string             `json:"piece,omitempty"`
	From   board.PositionID   `json:"from,omitempty"`
	To     board.PositionID   `json:"to,omitempty"`
	Count  int                `json:"count,omitempty"`
	Path   []board.PositionID `json:"path,omitempty"`
	Drops  []int              `json:"drops,omitempty"`
	// Aim is the target of a compound move's second half (the square an
	// Amazon's arrow lands on).
	Aim board.PositionID `json:"aim,omitempty"`
}

// Key returns a canonical, comparable string for the move, ignoring fields
// that are derived (To of a stack move).
func (m Move) Key() string {
	var b strings.Builder
	b.WriteString(m.Action)
	b.WriteByte('|')
	b.WriteString(m.Piece)
	b.WriteByte('|')
	b.WriteString(string(m.From))
	b.WriteByte('|')
	if len(m.Path) == 0 {
		b.WriteString(string(m.To))
	}
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(m.Count))
	b.WriteByte('|')
	for i, p := range m.Path {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(string(p))
	}
	b.WriteByte('|')
	for i, d := range m.Drops {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(d))
	}
	if m.Aim != "" {
		b.WriteString("|aim:")
		b.WriteString(string(m.Aim))
	}
	return b.String()
}

// Normalize returns m with its Action filled in (inferred from the fields
// when empty) and derived fields made canonical. It reports an error when
// the action cannot be determined or is not defined by game.
func (m Move) Normalize(game *catalog.CompiledGame) (Move, error) {
	if m.Action == "" {
		switch {
		case len(m.Path) > 0 || m.Count > 0:
			want := catalog.ActionMoveStack
			if m.Count == 0 {
				want = catalog.ActionJump
			}
			for _, a := range game.Actions {
				if a.Type == want {
					m.Action = a.ID
					break
				}
			}
			if m.Action == "" {
				return m, fmt.Errorf("state: game has no %s action", want)
			}
		case m.Piece != "" && m.From == "":
			for _, a := range game.Actions {
				if a.Type != catalog.ActionPlace {
					continue
				}
				for _, p := range a.Pieces {
					if p == m.Piece {
						m.Action = a.ID
						break
					}
				}
				if m.Action != "" {
					break
				}
			}
			if m.Action == "" {
				return m, fmt.Errorf("state: no place action can place piece %q", m.Piece)
			}
		case m.From != "" && m.To != "":
			m.Action = catalog.ActionStep
			// (A bare from/to move is resolved against the explicit step,
			// slide and leap actions by Apply.)
		default:
			return m, fmt.Errorf("state: move does not say what to do (need an action, or from/to)")
		}
	}

	act, ok := game.Action(m.Action)
	if !ok {
		if m.Action == catalog.ActionStep && hasStepActions(game) {
			// Resolved against the explicit step actions by Apply.
			return m, nil
		}
		return m, fmt.Errorf("state: unknown action %q", m.Action)
	}
	if act.Type == catalog.ActionPlace && m.Piece == "" && len(act.Pieces) > 0 {
		// A placement that names no piece uses the action's default (first).
		m.Piece = act.Pieces[0]
	}
	if (act.Type == catalog.ActionMoveStack || act.Type == catalog.ActionJump) && len(m.Path) > 0 {
		m.To = m.Path[len(m.Path)-1]
	}
	return m, nil
}

// hasStepActions reports whether game declares any action that a bare
// from/to move can name: an explicit step, a slide or a leap.
func hasStepActions(game *catalog.CompiledGame) bool {
	for _, a := range game.Actions {
		if (a.Type == catalog.ActionStep && a.Explicit) || a.Type == catalog.ActionSlide || a.Type == catalog.ActionLeap {
			return true
		}
	}
	return false
}

// clonePath returns a copy of p.
func clonePath(p []board.PositionID) []board.PositionID {
	if p == nil {
		return nil
	}
	out := make([]board.PositionID, len(p))
	copy(out, p)
	return out
}

// cloneInts returns a copy of d.
func cloneInts(d []int) []int {
	if d == nil {
		return nil
	}
	out := make([]int, len(d))
	copy(out, d)
	return out
}
