// Package onitama ships Onitama (Shimpei Sato, Arcane Wonders): a two-player
// abstract game on a 5×5 board where the moves a piece may make come from
// five Move cards; the card you use goes to the middle and you take the one
// that was waiting there. Win by capturing the opponent's Master, or by
// moving your own Master onto their Temple Arch.
//
// The rules are declared in onitama.yaml: the sixteen cards and their moves,
// the deal (ADR 0119), and the card-move that plays a card and passes it on
// (ADR 0120). Until 0.2.0 they were a Go ruleset; upgrade.go converts the
// positions it stored. cards.go keeps the cards for the guide's pages. The
// game has no secrets; only the deal at the start is random.
package onitama

import (
	"embed"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

//go:embed onitama.yaml
var definitionFS embed.FS

// definition is the definition as JSON, converted once.
var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "onitama.yaml") })

// Module returns Onitama's catalog.Module.
func Module() catalog.Module {
	return catalog.Module{ID: "onitama", Definition: definition(), Assets: newGuideAssets(), Upgrade: Upgrade}
}
