// Package testdata holds game definitions that only the tests use. They are
// not part of bingle: the server never registers them, the tests that need one
// register it themselves.
package testdata

import (
	_ "embed" // freeplay.json

	"github.com/noderunner/bingle/internal/engine/catalog"
)

//go:embed freeplay.json
var freeplayJSON []byte

// FreeplayID is the id of the Freeplay fixture.
const FreeplayID = "test-freeplay"

// Freeplay is a small rule-free sandbox game on a 9×9 Go-style board: stones
// step one square and capture by landing on an opponent's stone. It drives the
// platform's tests (the REST API, the store and the DSL) with a game whose
// every move and starting position is known, and is not a game to play.
func Freeplay() catalog.Module {
	return catalog.Module{ID: FreeplayID, Definition: freeplayJSON}
}
