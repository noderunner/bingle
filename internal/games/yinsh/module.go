// Package yinsh ships Yinsh (Kris Burm, 2003, the third game of the GIPF
// project): each player has five rings and the two share a pool of markers
// that are white on one side and black on the other. On your turn you drop a
// marker of your colour inside one of your rings and then move that ring in a
// straight line, turning over every marker it jumps. A row of five markers of
// your colour is removed, with one of your rings; remove three rings to win.
//
// The rules are a Go state.Ruleset (the ring placements, the moves, and the
// choices of which row and which ring to remove are separate decisions, the
// opponent deciding for their own rows within a turn); definition/yinsh.yaml carries the
// 85-point board as an explicit graph, the pieces and a one-ring "blitz"
// variant. The game has no secrets and no chance.
package yinsh

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

//go:embed definition
var definitionFS embed.FS

// definition is the definition as JSON, converted once.
var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "definition/yinsh.yaml") })

//go:embed guide
var guideFS embed.FS

// Module returns Yinsh's catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, "guide")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "yinsh", Definition: definition(), Rules: New(), Assets: sub}
}
