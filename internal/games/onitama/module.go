// Package onitama ships Onitama (Shimpei Sato, Arcane Wonders): a two-player
// abstract game on a 5×5 board where the moves a piece may make come from
// five Move cards; the card you use goes to the middle and you take the one
// that was waiting there. Win by capturing the opponent's Master, or by
// moving your own Master onto their Temple Arch.
//
// The rules are a Go state.Ruleset (a turn names a card, a piece and a
// square, and exchanges cards), with onitama.json describing board, seats and
// pieces. The game has no secrets; only the deal at the start is random.
package onitama

import (
	_ "embed"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

//go:embed onitama.json
var definitionJSON []byte

// Module returns Onitama's catalog.Module.
func Module() catalog.Module {
	return catalog.Module{ID: "onitama", Definition: definitionJSON, Rules: New(), Assets: newGuideAssets()}
}
