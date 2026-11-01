// Package cthulhuwars implements Cthulhu Wars (Petersen Games), the core
// set with its four factions — Great Cthulhu, Crawling Chaos, Black Goat and
// Yellow Sign — on a simplified world map of straight-edged areas.
//
// The rules are written in Go (a state.Ruleset): four cults spend Power on
// recruiting, moving, summoning and battling, gather Power from Cultists and
// Gates, score Doom for the Gates they control, earn spellbooks and race to
// 30 Doom with all six books. definition/cthulhuwars.yaml describes the board, the
// pieces and the legend; guide/ holds the long-form rules shown in the
// rules window.
package cthulhuwars

import (
	crand "crypto/rand"
	"embed"
	"encoding/binary"
	"io/fs"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

//go:embed definition
var definitionFS embed.FS

// definition is the definition as JSON, converted once.
var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "definition/cthulhuwars.yaml") })

//go:embed guide
var guideFS embed.FS

// Module returns Cthulhu Wars' catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, ".")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "cthulhu-wars", Definition: definition(), Rules: New(), Assets: sub}
}

// freshSeed draws a dice generator state from the system's entropy.
func freshSeed() uint64 {
	var b [8]byte
	if _, err := crand.Read(b[:]); err != nil {
		return 0x9e3779b97f4a7c15
	}
	v := binary.LittleEndian.Uint64(b[:])
	if v == 0 {
		v = 1
	}
	return v
}
