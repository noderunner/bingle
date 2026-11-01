// Package tak ships Tak (James Ernest and Patrick Rothfuss's abstract
// strategy game) as a pure-JSON game module: stacks, a reserve of stones,
// placement and multi-square stack moves, the opening swap, road and
// flat-count win rules and PTN notation are all expressed in tak.yaml using
// the engine's declarative actions — no Go callbacks.
//
// The module registers six sibling games (tak-3x3 … tak-8x8, with tak-5x5
// as the base document) via the DSL's variants mechanism.
package tak

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

//go:embed tak.yaml
var definitionFS embed.FS

// definition is the definition as JSON, converted once.
var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "tak.yaml") })

//go:embed guide
var guideFS embed.FS

// Module returns Tak's catalog.Module. Register it with
// Catalog.RegisterModule so its board-size variants are registered too.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, "guide")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "tak-5x5", Definition: definition(), Callbacks: nil, Assets: sub}
}
