// Package mycelium ships Mycelium (Miles Stevenson, 2024): a game of dueling
// fungal colonies on a hexagonal board. Players place nodes; a node placed in
// the centre triggers a confluence turn in which clusters spread and convert
// enemy nodes. Mycelium has no secrets and no chance.
//
// mycelium.yaml describes the board, seats and the two node colours; the Go
// ruleset implements the confluence turn and the conversion mechanics.
package mycelium

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

//go:embed mycelium.yaml
var definitionFS embed.FS

// definition is the definition as JSON, converted once.
var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "mycelium.yaml") })

//go:embed guide
var guideFS embed.FS

// Module returns Mycelium's catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, "guide")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "mycelium", Definition: definition(), Rules: New(), Assets: sub}
}
