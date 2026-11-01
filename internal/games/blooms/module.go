// Package blooms ships Blooms (Nick Bentley, 2018): a Go-like game on a
// hexagonal board where each player has two colours of stone, places one or
// two stones of different colours per turn, and wins by being the first to
// capture a set number of enemy stones. A "bloom" is a connected group of one
// colour; it is captured when it has no empty space beside it.
//
// The rules are a Go state.Ruleset (a turn is one or two placements and an end
// of turn that resolves the captures); blooms.yaml describes the board, seats
// and the four stone colours, and carries the base-4 and base-6 boards as
// variants. The game has no secrets and no chance.
package blooms

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

//go:embed blooms.yaml
var definitionFS embed.FS

// definition is the definition as JSON, converted once.
var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "blooms.yaml") })

//go:embed guide
var guideFS embed.FS

//go:embed tutorials
var tutorialsFS embed.FS

// Module returns Blooms' catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, "guide")
	if err != nil {
		panic(err)
	}
	tutorials, err := fs.Sub(tutorialsFS, "tutorials")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "blooms", Definition: definition(), Rules: New(), Assets: sub, Tutorials: tutorials}
}
