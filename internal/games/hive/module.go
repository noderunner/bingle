// Package hive ships Hive (John Yianni, 2001) with the three expansion
// insects: a game of eight kinds of insect per player, laid as tiles on a
// table with no board. The first player whose Queen Bee is completely
// surrounded loses.
//
// Hive has no board, so the game is played on a large hexagonal field whose
// middle is where the first tile goes; its edge is far beyond where a hive
// normally spreads. The rules are a Go state.Ruleset (placement, the One Hive
// rule, freedom to move, and each insect's movement, with stacks for the
// Beetle and Mosquito on top of the hive); hive.json carries the field, the
// pieces and the starting supply, and a variant without the expansion
// insects. The game has no secrets and no chance.
package hive

import (
	"embed"
	_ "embed" // hive.json
	"io/fs"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

//go:embed hive.json
var definitionJSON []byte

//go:embed guide
var guideFS embed.FS

// Module returns Hive's catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, ".")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "hive", Definition: definitionJSON, Rules: New(), Assets: sub}
}
