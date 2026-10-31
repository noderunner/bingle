// Package mycelium ships Mycelium (Miles Stevenson, 2024): a game of dueling
// fungal colonies on a hexagonal board. Players place nodes; a node placed in
// the centre triggers a confluence turn in which clusters spread and convert
// enemy nodes. Mycelium has no secrets and no chance.
//
// mycelium.json describes the board, seats and the two node colours; the Go
// ruleset implements the confluence turn and the conversion mechanics.
package mycelium

import (
	"embed"
	_ "embed" // the definition
	"io/fs"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

//go:embed mycelium.json
var definitionJSON []byte

//go:embed guide
var guideFS embed.FS

// Module returns Mycelium's catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, "guide")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "mycelium", Definition: definitionJSON, Rules: New(), Assets: sub}
}
