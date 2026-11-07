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

//go:embed solitaire.yaml bowling.yaml
var definitionFS embed.FS

// definition is the definition as JSON, converted once.
var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "solitaire.yaml") })

// bowlingDefinition is Bowling Solitaire's, a game of its own since the
// patience vocabulary (ADR 0122) is to take over the others.
var bowlingDefinition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "bowling.yaml") })

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
	return catalog.Module{ID: "solitaire", Definition: definition(), Rules: New(), Assets: guide}
}

// BowlingModule returns Bowling Solitaire's catalog.Module: the same ruleset,
// in its bowling mode, with its own definition and tutorial.
func BowlingModule() catalog.Module {
	guide, err := fs.Sub(guideFS, "guide")
	if err != nil {
		panic(err)
	}
	tutorials, err := fs.Sub(tutorialsFS, "tutorials")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "bowling-solitaire", Definition: bowlingDefinition(), Rules: New(), Assets: guide, Tutorials: tutorials}
}
