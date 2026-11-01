// Command genschema regenerates docs/game-definition.schema.json, the JSON
// Schema of a game definition, from the Go types of internal/engine/dsl:
//
//	go run ./scripts/genschema
//
// Run it from the repository root after changing those types; a test fails
// while the checked-in schema is out of date.
package main

import (
	"fmt"
	"os"

	"github.com/noderunner/bingle/internal/engine/dsl/schemagen"
)

func main() {
	data, err := schemagen.Generate("internal/engine/dsl")
	if err != nil {
		fmt.Fprintln(os.Stderr, "genschema:", err)
		os.Exit(1)
	}
	if err := os.WriteFile("docs/game-definition.schema.json", data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "genschema:", err)
		os.Exit(1)
	}
}
