// Command wasm is the draughts fixture as a rules module (ADR 0147). The
// tests build it with GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared,
// so no binary is committed. Built for anything else it does nothing.
package main

import (
	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
	"github.com/noderunner/bingle/tests/testdata/draughts"
)

func init() { guest.Register(draughts.New) }

func main() {}
