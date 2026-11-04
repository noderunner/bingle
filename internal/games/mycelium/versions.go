// Package mycelium lists the versions of Mycelium's rules. Mycelium is a
// prototype: each version is a folder holding the whole game, the definition,
// the Go ruleset, the guide and any tutorials, and is only added to, so that a
// game in progress keeps the rules it started with (ADR 0098).
package mycelium

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	v100 "github.com/noderunner/bingle/internal/games/mycelium/v1.0.0"
)

// Modules returns one catalog.Module per version of the rules.
func Modules() []catalog.Module {
	return []catalog.Module{v100.Module()}
}
