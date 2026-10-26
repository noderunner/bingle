// Package fendo ships Fendo (Dieter Stein, 2014): two players share a 7×7
// board and a supply of fences. On your turn you move one of your pieces and
// build a fence beside it, or add a piece; fences wall off areas, and an area
// closed round exactly one piece belongs to that piece's owner. When the
// last open area is closed the player who owns the most squares wins.
//
// The rules are a Go state.Ruleset (a turn is a single decision: a move with
// its fence, an added piece or a pass); fendo.json describes the board, the
// seats and the pieces, and carries the 9×9 and 11×11 boards as variants. The
// game has no secrets and no chance.
package fendo

import (
	"embed"
	_ "embed" // the definition
	"io/fs"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

//go:embed fendo.json
var definitionJSON []byte

//go:embed guide
var guideFS embed.FS

// Module returns Fendo's catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, "guide")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "fendo", Definition: definitionJSON, Rules: New(), Assets: sub}
}
