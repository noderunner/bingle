package catalog

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/dsl"
)

// The pie rule (also the swap rule): the second player, instead of replying to
// the opening move, may take the first player's side. It evens out the
// advantage of moving first, and whether a table plays with it is a choice of
// the people playing, not a property of the game — so a definition says that
// the game *can* be played with it (`pie_rule`) and the creator of a game
// decides. The engine already has the move (the "swap" action, which recolours
// every piece and gives the turn back to the first player), so a game that
// declares the pie rule needs nothing else: this file brings the action, the
// setup option clients draw, and the Deriver that turns the choice into a game
// of its own (see ADR 0056 and ADR 0028).

// The values of dsl.GameDefinition.PieRule.
const (
	// PieOptional offers the rule in the setup, off unless the table asks.
	PieOptional = "optional"
	// PieStandard offers it in the setup, on unless the table turns it off:
	// for games whose players normally use it (Hex, Gomoku).
	PieStandard = "standard"
	// PieOn and PieOff are settled: the rule is in play, or it is not, and
	// there is nothing left to choose. A derived game carries one of them.
	PieOn  = "on"
	PieOff = "off"
)

// PieOption is the id of the setup option a game with a pie rule offers, and
// the key of its value in a create call's options.
const PieOption = "pie-rule"

// pieChoice reports whether the creator of a game may choose the pie rule.
func pieChoice(v string) bool { return v == PieOptional || v == PieStandard }

// PieRuleOn reports whether a definition is played with the pie rule.
func PieRuleOn(def *dsl.GameDefinition) bool {
	return def != nil && (def.PieRule == PieStandard || def.PieRule == PieOn)
}

// applyPieRule checks a definition's pie_rule and expands it before the rules
// are compiled: the swap action when the game is played with the rule, and the
// setup option when its creator may still choose. A game that declares no pie
// rule is left alone.
func applyPieRule(id string, def *dsl.GameDefinition) error {
	switch def.PieRule {
	case "":
		return nil
	case PieOptional, PieStandard, PieOn, PieOff:
	default:
		return fmt.Errorf("catalog: game %q has pie_rule %q (optional, standard, on or off)", id, def.PieRule)
	}
	// The pie rule is the engine's swap action, which only the generic engine
	// plays; a ruleset builds its own moves and would have to offer it itself.
	if def.Engine == "ruleset" {
		return fmt.Errorf("catalog: game %q declares pie_rule, which the generic engine's swap action implements: a ruleset game has to offer swapping sides itself", id)
	}
	if len(def.Seats) != 2 {
		return fmt.Errorf("catalog: game %q declares pie_rule but seats %d players (the pie rule is for two)", id, len(def.Seats))
	}
	// Swapping recolours the pieces where they stand, so a game in which a
	// seat has a direction of play would come out backwards.
	for _, s := range def.Seats {
		if s.Forward != "" {
			return fmt.Errorf("catalog: game %q declares pie_rule but seat %d plays towards %q: swapping sides would leave its pieces facing the wrong way", id, s.Seat, s.Forward)
		}
	}
	for _, a := range def.Actions {
		if a.Type == ActionSwap {
			return fmt.Errorf("catalog: game %q declares pie_rule and the swap action %q: the pie rule brings the action itself", id, a.ID)
		}
	}
	if PieRuleOn(def) {
		def.Actions = append(def.Actions, dsl.ActionDef{ID: ActionSwap, Type: ActionSwap, Label: "Swap sides"})
	}
	if pieChoice(def.PieRule) {
		def.Options = append(def.Options, dsl.OptionDef{
			ID:    PieOption,
			Label: "Pie rule (the second player may swap sides)",
			Kind:  "bool",
			Text:  "Instead of answering the first move, the second player may take the first player's side. It evens out the advantage of moving first.",
			// A game played with the rule offers to leave it out, and the
			// other way round, so the box starts at the game's own practice.
			Default: def.PieRule == PieStandard,
		})
	}
	return nil
}

// pieDeriver builds the derived games of the pie-rule option: "base~pie" is
// the game played with the rule where it is normally left out, and
// "base~nopie" the game without it where it is normal. A game that offers
// nothing else needs no Deriver of its own.
type pieDeriver struct{}

// Derive reads the pie-rule option. Choosing what the game does anyway returns
// no suffix, so the plain game is used.
func (pieDeriver) Derive(base *dsl.GameDefinition, options map[string]any) (string, map[string]any, error) {
	on := PieRuleOn(base)
	for k, v := range options {
		if k != PieOption {
			return "", nil, fmt.Errorf("catalog: game %q has no setup option %q", base.ID, k)
		}
		b, ok := v.(bool)
		if !ok {
			return "", nil, fmt.Errorf("catalog: option %q must be true or false", k)
		}
		on = b
	}
	if on == PieRuleOn(base) {
		return "", nil, nil
	}
	return pieSuffix(on), piePatch(on), nil
}

// FromSuffix rebuilds the patch of a suffix Derive returned.
func (pieDeriver) FromSuffix(base *dsl.GameDefinition, suffix string) (map[string]any, error) {
	switch suffix {
	case pieSuffix(true):
		return piePatch(true), nil
	case pieSuffix(false):
		return piePatch(false), nil
	}
	return nil, fmt.Errorf("catalog: game %q has no setup options named %q", base.ID, suffix)
}

func pieSuffix(on bool) string {
	if on {
		return "pie"
	}
	return "nopie"
}

func piePatch(on bool) map[string]any {
	if on {
		return map[string]any{"pie_rule": PieOn}
	}
	return map[string]any{"pie_rule": PieOff}
}
