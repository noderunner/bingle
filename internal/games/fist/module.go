// Package fist ships Fist, the abstract strategy game from Alysha
// Wishingrad's novel The Verdigris Pawn (Crab Fragment Labs, open beta
// rules 1.1): a challenger with a mage, an ace and a few guards against a
// king with an army, and one neutral verdigris pawn either side may seize.
//
// Its rules are written in Go (a state.Ruleset) because pinning with allies,
// jump chains with optional captures and the mage's Empower are not things
// the declarative DSL expresses. fist.json still describes the board, seats
// and pieces, so the lobby, UI, API, MCP server and bots treat it like any
// other game. The game has no secrets and no chance.
package fist

import (
	"embed"
	_ "embed" // fist.json
	"io/fs"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

//go:embed fist.json
var definitionJSON []byte

//go:embed guide
var guideFS embed.FS

// Module returns Fist's catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, ".")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "fist", Definition: definitionJSON, Rules: New(), Assets: sub}
}
