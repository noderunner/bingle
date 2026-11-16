// Command genschema regenerates the JSON Schemas of the hand-written
// documents from their Go types: docs/game-definition.schema.json (from
// internal/engine/dsl) and docs/tutorial.schema.json (from internal/tutorial),
// and that of the API's messages, docs/api.schema.json (from the handlers of
// internal/api and the types they send):
//
//	go run ./scripts/genschema
//
// Run it from the repository root after changing those types; a test fails
// while a checked-in schema is out of date.
package main

import (
	"fmt"
	"os"

	"github.com/noderunner/bingle/internal/api"
	"github.com/noderunner/bingle/internal/engine/dsl/schemagen"
	"github.com/noderunner/bingle/internal/tutorial"
)

func main() {
	def, err := schemagen.Generate("internal/engine/dsl")
	if err != nil {
		fail(err)
	}
	tut, err := tutorial.Schema("internal/tutorial")
	if err != nil {
		fail(err)
	}
	msgs, err := api.Schema(".")
	if err != nil {
		fail(err)
	}
	for path, data := range map[string][]byte{"docs/game-definition.schema.json": def, "docs/tutorial.schema.json": tut, "docs/api.schema.json": msgs} {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "genschema:", err)
	os.Exit(1)
}
