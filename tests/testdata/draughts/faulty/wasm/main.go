// Command wasm is the faulty draughts rules as a rules module (ADR 0147),
// built by the tests like the fixture's own.
package main

import (
	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
	"github.com/noderunner/bingle/tests/testdata/draughts/faulty"
)

func init() { guest.Register(faulty.New) }

func main() {}
