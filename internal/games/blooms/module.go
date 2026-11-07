// Package blooms ships Blooms (Nick Bentley, 2018): a Go-like game on a
// hexagonal board where each player has two colours of stone, places one or
// two stones of different colours per turn, and wins by being the first to
// capture a set number of enemy stones. A "bloom" is a connected group of one
// colour; it is captured when it has no empty space beside it.
//
// The rules are declared in blooms.yaml: two colours a seat (ADR 0117), a turn
// of one or two stones (ADR 0115), captures when a turn ends (ADR 0116) and a
// move limit decided on captures (ADR 0118). Until 0.6.0 they were a Go
// ruleset; upgrade.go converts the positions it stored. The game has no
// secrets and no chance.
package blooms

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/web/look"
)

//go:embed blooms.yaml web.yaml
var definitionFS embed.FS

// definition is the definition as JSON, converted once.
var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "blooms.yaml") })

// web is how the web page draws Blooms (web.yaml, ADR 0124), as JSON.
var web = sync.OnceValue(func() []byte { return look.MustLoad(definitionFS) })

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
	return catalog.Module{ID: "blooms", Definition: definition(), Assets: sub, Tutorials: tutorials, Upgrade: Upgrade, Web: web()}
}
