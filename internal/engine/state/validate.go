package state

import (
	"fmt"
	"sort"

	"github.com/noderunner/bingle/internal/engine/cards"
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
	for _, s := range game.Definition.Seats {
		for _, c := range s.Colors {
			onBoard[c] = true
		}
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
	if err := validateCards(game, st); err != nil {
		return err
	}
	if rs, ok := RulesetOf(game); ok {
		return rs.Validate(game, st)
	}
	if t := st.turn; t != nil {
		parts := turnParts(game, st)
		if parts == nil || t.Part < 0 || t.Part >= len(parts) || t.Made < 1 || t.Made > parts[t.Part].Max {
			return fmt.Errorf("state: a turn in progress at part %d with %d selections does not fit the game's turn", t.Part+1, t.Made)
		}
	}
	return nil
}

// validateCards checks a declared game's table: every zone is one of the
// game's, and every card is one of its cards, once.
func validateCards(game *catalog.CompiledGame, st *BoardState) error {
	if st.table == nil {
		return nil
	}
	if game.Cards == nil {
		return fmt.Errorf("state: cards in a game without cards")
	}
	known := map[string]bool{}
	for _, ids := range game.Cards.Instances {
		for _, id := range ids {
			known[id] = true
		}
	}
	seen := map[string]bool{}
	for key, z := range st.table.Zones {
		zone, _ := cards.SplitKey(key)
		if _, ok := game.Cards.Zones[zone]; !ok {
			return fmt.Errorf("state: unknown card zone %q", zone)
		}
		for _, c := range z.Cards {
			if c.Hidden() {
				continue // a redacted card
			}
			if !known[c.ID] {
				return fmt.Errorf("state: unknown card %q in %s", c.ID, key)
			}
			if seen[c.ID] {
				return fmt.Errorf("state: card %q lies in two places", c.ID)
			}
			seen[c.ID] = true
		}
	}
	return nil
}
