package state

import (
	"errors"
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Impulses (ADR 0114). An impulse is the choices one seat makes before the game
// needs something from elsewhere, submitted at once and applied as one. Until
// the renames of ADR 0114's first phase, a Move here is one selection, and an
// impulse is the []Move of its selections.

// SelectionSeparator joins the selections of one impulse in its notation. No
// game uses it inside a selection: Hive and Mycelium have spaces there, and
// boards labelled xy have commas.
const SelectionSeparator = "; "

// IsPieSwap reports whether mv is the pie-rule swap of a game played with the
// rule: the seats' occupants exchange, so whoever chooses next is somebody else
// even though the seat on impulse may be the same.
func IsPieSwap(game *catalog.CompiledGame, mv Move) bool {
	return catalog.PieRuleOn(game.Definition) && mv.Action == catalog.ActionSwap
}

// ImpulseEnds reports whether the selection mv, which took before to res, ends
// seat's impulse: the game is over, the next choice is another seat's (a
// reaction included) or someone else's in the same seat (the pie-rule swap), or
// the selection showed the seat something it could not know before (a roll, a
// draw, a card turned up: the ruleset's UndoGuard). It depends on the position
// and the rules alone, never on a seat's settings.
func ImpulseEnds(game *catalog.CompiledGame, before *BoardState, seat int, mv Move, res *MoveResult) bool {
	return res.GameEnded || res.NextImpulseSeat != seat || IsPieSwap(game, mv) ||
		!CanTakeBack(game, before, res.NewState)
}

// AnySelectionMayEnd reports whether a seat may end its impulse after any
// selection: in a game for one seat nobody waits on it, and there is no clock or
// rating to game by pausing, so a game of patience can be sent move by move
// (ADR 0114). Elsewhere an impulse ends only where ImpulseEnds says.
func AnySelectionMayEnd(game *catalog.CompiledGame) bool { return len(game.Definition.Seats) == 1 }

// ErrImpulseOpen is returned for selections that stop short of the end of the
// impulse: a seat may not submit part of one.
var ErrImpulseOpen = errors.New("state: the impulse is not over: more selections are needed")

// ErrImpulseOver is returned for a selection after the one that ended the
// impulse.
var ErrImpulseOver = errors.New("state: the impulse is already over")

// Selections is what may come next in an impulse after the selections made.
type Selections struct {
	// Options are the legal next selections; empty when Complete.
	Options []Move
	// MayEnd says the selections made may be submitted as they are although
	// Options go on: only in a game for one seat (AnySelectionMayEnd), and
	// never before a first selection.
	MayEnd bool
	// Complete says the selections made end the impulse: they are a move,
	// ready to submit.
	Complete bool
	// Position is the position after the selections made, as the engine holds
	// it (a caller redacts it for the seat). Nil when Complete: the selection
	// that ends an impulse may roll or draw, and must not be tried in a query.
	Position *BoardState
}

// NextSelections lists what seat may select after the selections made, from
// st. It is pure and deterministic, like Legal; nothing is stored.
func NextSelections(game *catalog.CompiledGame, st *BoardState, seat int, made []Move) (Selections, error) {
	cur := st
	for i, mv := range made {
		res, err := Apply(game, cur, seat, mv)
		if err != nil {
			return Selections{}, fmt.Errorf("selection %d: %w", i+1, err)
		}
		if ImpulseEnds(game, cur, seat, mv, res) {
			if i < len(made)-1 {
				return Selections{}, fmt.Errorf("selection %d: %w", i+2, ErrImpulseOver)
			}
			return Selections{Complete: true}, nil
		}
		cur = res.NewState
	}
	return Selections{Options: LegalMoves(game, cur, seat), Position: cur,
		MayEnd: len(made) > 0 && AnySelectionMayEnd(game)}, nil
}

// ImpulseResult is the outcome of a whole impulse.
type ImpulseResult struct {
	// Selections are the results of each selection, in order; the last is the
	// impulse's own result (its NewState, NextImpulseSeat and ending).
	Selections []*MoveResult
}

// Last is the result of the selection that ended the impulse.
func (r *ImpulseResult) Last() *MoveResult { return r.Selections[len(r.Selections)-1] }

// ApplyImpulse applies a whole impulse: every selection but the last must keep
// it open, and the last must end it (in a game for one seat it may stop
// anywhere). Nothing is applied unless all are.
func ApplyImpulse(game *catalog.CompiledGame, st *BoardState, seat int, sels []Move) (*ImpulseResult, error) {
	if len(sels) == 0 {
		return nil, ErrImpulseOpen
	}
	out := &ImpulseResult{}
	cur := st
	for i, mv := range sels {
		res, err := Apply(game, cur, seat, mv)
		if err != nil {
			return nil, fmt.Errorf("selection %d: %w", i+1, err)
		}
		ends := ImpulseEnds(game, cur, seat, mv, res)
		if ends && i < len(sels)-1 {
			return nil, fmt.Errorf("selection %d: %w", i+2, ErrImpulseOver)
		}
		if !ends && i == len(sels)-1 && !AnySelectionMayEnd(game) {
			return nil, ErrImpulseOpen
		}
		out.Selections = append(out.Selections, res)
		cur = res.NewState
	}
	return out, nil
}

// FormatImpulse writes an impulse's selections as one move's notation.
func FormatImpulse(game *catalog.CompiledGame, sels []Move) string {
	parts := make([]string, len(sels))
	for i, mv := range sels {
		parts[i] = FormatMove(game, mv)
	}
	return strings.Join(parts, SelectionSeparator)
}

// ParseImpulse reads a move's notation into its selections.
func ParseImpulse(game *catalog.CompiledGame, text string) ([]Move, error) {
	var out []Move
	for i, part := range strings.Split(text, strings.TrimSpace(SelectionSeparator)) {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("state: selection %d of %q is empty", i+1, text)
		}
		mv, err := ParseMove(game, part)
		if err != nil {
			return nil, fmt.Errorf("selection %d: %w", i+1, err)
		}
		out = append(out, mv)
	}
	return out, nil
}
