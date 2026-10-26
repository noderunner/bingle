// Package blooms ships Blooms (Nick Bentley, 2018): a Go-like game on a
// hexagonal board where each player has two colours of stone, places one or
// two stones of different colours per turn, and wins by being the first to
// capture a set number of enemy stones. A "bloom" is a connected group of one
// colour; it is captured when it has no empty space beside it.
//
// The rules are a Go state.Ruleset (a turn is one or two placements and an end
// of turn that resolves the captures); blooms.json describes the board, seats
// and the four stone colours, and carries the base-4 and base-6 boards as
// variants. The game has no secrets and no chance.
package blooms

import (
	"embed"
	_ "embed" // the definition
	"io/fs"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

//go:embed blooms.json
var definitionJSON []byte

//go:embed guide
var guideFS embed.FS

// Module returns Blooms' catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, "guide")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "blooms", Definition: definitionJSON, Rules: New(), Assets: sub}
}
