package state

import (
	"fmt"
	"sort"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Validate checks that st is a position game could actually contain: every
// occupied square exists, every piece type and colour is known, reserves name
// real pools and colours, and no stack is taller than the board has pieces
// (a guard against absurd documents). It does not check reachability.
func Validate(game *catalog.CompiledGame, st *BoardState) error {
	g := game.Board
	colors := map[string]bool{}
	for _, s := range game.Definition.Seats {
		colors[s.Color] = true
	}
	onBoard := map[string]bool{}
	for c := range colors {
		onBoard[c] = true
	}
	for _, c := range game.Definition.ExtraColors {
		onBoard[c] = true
	}
	ids := st.Occupied()
	for _, id := range ids {
		if !g.HasPosition(id) {
			return fmt.Errorf("state: position %q is not on the board", id)
		}
		stack := st.Stack(id)
		if len(stack) > 256 {
			return fmt.Errorf("state: the stack at %s is implausibly tall (%d pieces)", id, len(stack))
		}
		for _, o := range stack {
			if _, ok := game.Pieces[o.Piece]; !ok {
				return fmt.Errorf("state: unknown piece type %q at %s", o.Piece, id)
			}
			if !onBoard[o.Color] {
				return fmt.Errorf("state: unknown colour %q at %s", o.Color, id)
			}
		}
	}
	res := st.Reserves()
	colorNames := make([]string, 0, len(res))
	for c := range res {
		colorNames = append(colorNames, c)
	}
	sort.Strings(colorNames)
	for _, c := range colorNames {
		if !colors[c] {
			return fmt.Errorf("state: reserves for unknown colour %q", c)
		}
		for pool, n := range res[c] {
			if _, ok := game.Definition.Reserves[pool]; !ok {
				return fmt.Errorf("state: reserve pool %q does not exist in this game", pool)
			}
			if n < 0 {
				return fmt.Errorf("state: negative reserve for %s/%s", c, pool)
			}
		}
	}
	if st.Ply() < 0 {
		return fmt.Errorf("state: negative ply")
	}
	if rs, ok := RulesetOf(game); ok {
		return rs.Validate(game, st)
	}
	return nil
}
