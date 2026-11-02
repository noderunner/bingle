// Package solitaire ships the games one person plays with a deck of cards:
// Klondike (what most people call Solitaire), Spider, FreeCell and Sid Sackson's
// Bowling Solitaire. solitaire.yaml defines the decks and the zones of the table,
// and one Go ruleset plays all of them, each variant naming its rules with the
// param "mode". The decks are plain suits and ranks, so every one of them can
// wear a themed set of cards (ADR 0085); seating one player is ADR 0086.
package solitaire

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

//go:embed solitaire.yaml
var definitionFS embed.FS

// definition is the definition as JSON, converted once.
var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "solitaire.yaml") })

//go:embed guide
var guideFS embed.FS

//go:embed tutorials
var tutorialsFS embed.FS

// Module returns Solitaire's catalog.Module.
func Module() catalog.Module {
	guide, err := fs.Sub(guideFS, "guide")
	if err != nil {
		panic(err)
	}
	tutorials, err := fs.Sub(tutorialsFS, "tutorials")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "solitaire", Definition: definition(), Rules: New(), Assets: guide, Tutorials: tutorials}
}
