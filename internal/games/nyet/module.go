// Package nyet ships Nyet! (Stefan Dorra, 1997; IELLO): a trick-taking game
// for two to five players in which the terms of every round — who leads, what
// is discarded, the trump colour, the super trump and what a trick is worth —
// are fixed by the players taking turns to cross choices off a board, each
// naming what they do NOT want. The leader then picks a partner (or a team)
// and the round is played as a trick-taking game with trumps, super trumps
// and loot.
//
// It is the first game built on the card framework: nyet.json defines the
// deck (four colours, thirteen ranks, three 1s of each) and the zones of the
// table in the DSL's "cards" section, and the rules are a Go state.Ruleset
// that keeps its cards in a cards.Table, so hands are private, the trick is a
// public play area and the draw pile stays face down without game code for it.
package nyet

import (
	"embed"
	_ "embed" // nyet.json
	"io/fs"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

//go:embed nyet.json
var definitionJSON []byte

//go:embed guide
var guideFS embed.FS

// Module returns Nyet!'s catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, "guide")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "nyet", Definition: definitionJSON, Rules: New(), Assets: sub}
}
