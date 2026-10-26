// Package demo ships bingle's first concrete game — "Demo: Free Movement &
// Capture" — as a catalog.Module built from the embedded demo.json
// definition, per docs/history/plan.md section 6.5 and docs/history/backlog.md item 09.
package demo

import (
	_ "embed"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

//go:embed demo.json
var definitionJSON []byte

// Module returns the demo game's catalog.Module, ready to be registered via
// Catalog.Register(mod.ID, mod.Definition, mod.Callbacks). Callbacks is nil;
// the demo game uses no Go-callback escape hatch, so Catalog.Register
// substitutes callback.NoopCallbacks.
func Module() catalog.Module {
	return catalog.Module{ID: "demo-freeplay", Definition: definitionJSON, Callbacks: nil}
}
