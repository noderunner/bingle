// Package fist ships Fist, the abstract strategy game from Alysha
// Wishingrad's novel The Verdigris Pawn (Crab Fragment Labs, open beta
// rules 1.1): a challenger with a mage, an ace and a few guards against a
// king with an army, and one neutral verdigris pawn either side may seize.
//
// Its rules are written in Go (a state.Ruleset) because pinning with allies,
// jump chains with optional captures and the mage's Empower are not things
// the declarative DSL expresses. fist.yaml still describes the board, seats
// and pieces, so the lobby, UI, API, MCP server and bots treat it like any
// other game. The game has no secrets and no chance.
package fist

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

//go:embed fist.yaml
var definitionFS embed.FS

// definition is the definition as JSON, converted once.
var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "fist.yaml") })

//go:embed guide
var guideFS embed.FS

// Module returns Fist's catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, ".")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "fist", Definition: definition(), Rules: New(), Assets: sub}
}
