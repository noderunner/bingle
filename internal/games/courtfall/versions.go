// Package courtfall lists the versions of Courtfall's rules. Courtfall is a
// prototype: each version is a folder holding the whole game, the definition,
// the Go ruleset, the guide and any tutorials, and is only added to, so that a
// game in progress keeps the rules it started with (ADR 0098).
package courtfall

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	v100 "github.com/noderunner/bingle/internal/games/courtfall/v1.0.0"
)

// Modules returns one catalog.Module per version of the rules.
func Modules() []catalog.Module {
	return []catalog.Module{v100.Module()}
}
