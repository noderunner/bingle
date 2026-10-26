// Package courtfall ships Courtfall (Third Edition), a two-player arena
// skirmish of five exiles per side: fatigue as action economy and health,
// secret simultaneous dice allocation in every exchange, momentum as a
// spendable resource and a personal Clear Turn that cashes it in.
//
// Its rules are written in Go (a state.Ruleset) because they need things
// the declarative DSL does not have — per-unit conditions, dice, hidden
// choices, turns made of many small decisions. courtfall.json still
// describes the board, seats, pieces and legend, so the lobby, the web UI,
// the API, the MCP server and the bots treat it like any other game.
package courtfall

import (
	crand "crypto/rand"
	"embed"
	"encoding/binary"
	"io/fs"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

//go:embed courtfall.json
var definitionJSON []byte

// guideFS holds the rules guide pages shown in the rules window.
//
//go:embed guide
var guideFS embed.FS

// Module returns Courtfall's catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, ".")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "courtfall", Definition: definitionJSON, Rules: New(), Assets: sub}
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
