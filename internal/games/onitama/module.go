// Package onitama ships Onitama (Shimpei Sato, Arcane Wonders): a two-player
// abstract game on a 5×5 board where the moves a piece may make come from
// five Move cards; the card you use goes to the middle and you take the one
// that was waiting there. Win by capturing the opponent's Master, or by
// moving your own Master onto their Temple Arch.
//
// The rules are a Go state.Ruleset (a turn names a card, a piece and a
// square, and exchanges cards), with onitama.yaml describing board, seats and
// pieces. The game has no secrets; only the deal at the start is random.
package onitama

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/web/look"
)

//go:embed onitama.yaml web.yaml
var definitionFS embed.FS

// webFS is the page's own script and stylesheet for Onitama: its Move cards
// (ADR 0124).
//
//go:embed web
var webFS embed.FS

// definition is the definition as JSON, converted once.
var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "onitama.yaml") })

// Module returns Onitama's catalog.Module.
func Module() catalog.Module {
	scripts, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "onitama", Definition: definition(), Rules: New(), Assets: newGuideAssets(), Scripts: scripts, Web: look.MustLoad(definitionFS)}
}
