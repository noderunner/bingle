package courtfall

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/dsl"
)

// The standard arena: a terrain layout a table can take instead of drafting
// one (ADR 0061). The draft is ten decisions before anybody moves, which is a
// long way to start a quick game and pure noise in a batch of bot games — two
// batches of the same bots are not comparable when every game begins with a
// different arena. The option lays this one instead and the game starts at the
// choice of Courts.
//
// The layout is the draft's own rules obeyed strictly (nothing in either
// deployment zone, the Eye open, no two Hazards adjacent, no tile touching
// more than two special tiles) and is **symmetric under a half turn** of the
// board about the Eye, so neither side has the better ground:
//
//	      ·   ·   ·   ·          row 1  Dark's deployment zone
//	    ·   ·   ·   ·   ·        row 2
//	  G   G   ·   H   ·   M      row 3  (b3 c3 d3 e3 f3 g3)
//	M   ·   ·   ◎   ·   ·   H    row 4  (a4 … d4 is the Eye … g4)
//	  M   ·   H   ·   G   G      row 5  (a5 b5 c5 d5 e5 f5)
//	    ·   ·   ·   ·   ·        row 6  Light's deployment zone
//	      ·   ·   ·   ·          row 7
//
// Overgrowth comes in two pairs in opposite corners (a pair is a screen worth
// shooting round), the two Hazards beside the Eye make the centre cost
// something to hold, and the odd Mire and Hazard — the two a half turn cannot
// pair off, as three of each is an odd number — sit on the middle row, the one
// row equally far from both players.
var standardArena = map[string]string{
	"b3": tileGreen, "c3": tileGreen, "e5": tileGreen, "f5": tileGreen,
	"g3": tileBlue, "a4": tileBlue, "a5": tileBlue,
	"e3": tileRed, "c5": tileRed, "g4": tileRed,
}

// optArena is the setup option that lays the standard arena, and arenaSuffix
// the derived game it makes (courtfall~arena).
const (
	optArena    = "standard-arena"
	arenaSuffix = "arena"
)

// arenaParam is the parameter the derived document carries.
const arenaParam = "arena"

// standardArenaOn reports whether this definition is played on the standard
// arena rather than a drafted one.
func standardArenaOn(def *dsl.GameDefinition) bool {
	if def == nil {
		return false
	}
	v, _ := def.Params[arenaParam].(string)
	return v == "standard"
}

// Derive implements catalog.Deriver: the one setup option Courtfall offers.
// Choosing to draft the arena — what the game does anyway — returns no suffix,
// so the plain game keeps its plain id.
func (rs *Ruleset) Derive(base *dsl.GameDefinition, options map[string]any) (string, map[string]any, error) {
	standard := false
	for k, v := range options {
		if k != optArena {
			return "", nil, fmt.Errorf("courtfall: no setup option %q", k)
		}
		b, ok := v.(bool)
		if !ok {
			return "", nil, fmt.Errorf("courtfall: option %q must be true or false", k)
		}
		standard = b
	}
	if !standard {
		return "", nil, nil
	}
	return arenaSuffix, arenaPatch(), nil
}

// FromSuffix implements catalog.Deriver.
func (rs *Ruleset) FromSuffix(base *dsl.GameDefinition, suffix string) (map[string]any, error) {
	if suffix != arenaSuffix {
		return nil, fmt.Errorf("courtfall: no setup options named %q", suffix)
	}
	return arenaPatch(), nil
}

func arenaPatch() map[string]any {
	return map[string]any{"params": map[string]any{arenaParam: "standard"}}
}

// layStandardArena puts the standard terrain down and leaves the game at the
// choice of Courts, as a finished draft does.
func layStandardArena(d *Data) {
	d.Tiles = make(map[string]string, len(standardArena))
	for hex, tile := range standardArena {
		d.Tiles[hex] = tile
	}
	d.Left = [3]int{}
	d.Phase = phaseCourt
	d.N = 0
}
