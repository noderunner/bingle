// Package classics ships a handful of well-known abstract games that are
// defined entirely in YAML using the engine's declarative actions and win
// rules — Gomoku, Hex (a hexagonal board
// with per-seat connection rules), English Draughts (mandatory multi-jumps,
// promotion, directional movement), Breakthrough (direction-specific capture
// and a "reach the far row" win) Reversi (bracketing flips and passing) and Go (liberty captures, ko, passing,
// area scoring with komi), plus Yavalath (lose by making three) and Havannah
// (ring, bridge and fork wins) on hexagon-shaped boards, and the Game of the
// Amazons (queen slides with a compulsory arrow shot) and Ataxx (clone or
// jump, infecting neighbours) and Pente (custodian pair captures and a capture
// count) and Brandubh (an asymmetric hunt game: rook slides, custodian
// captures against corners and throne, a king to escape or capture). They
// double as
// living proof that the DSL is not Tak-specific.
package classics

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

//go:embed *.yaml
var definitionFS embed.FS

// definitions are the definitions as JSON by file name, converted once.
var definitions = sync.OnceValue(func() map[string][]byte {
	files, err := fs.Glob(definitionFS, "*.yaml")
	if err != nil {
		panic(err)
	}
	out := map[string][]byte{}
	for _, f := range files {
		out[f] = dsl.MustLoad(definitionFS, f)
	}
	return out
})

//go:embed guide
var guideFS embed.FS

// guideOf is the folder of a game's rules page and pictures.
func guideOf(folder string) fs.FS {
	sub, err := fs.Sub(guideFS, "guide/"+folder)
	if err != nil {
		panic(err)
	}
	return sub
}

// Modules returns the classic games' catalog modules.
func Modules() []catalog.Module {
	return []catalog.Module{
		{ID: "gomoku", Definition: definitions()["gomoku.yaml"], Assets: guideOf("gomoku")},
		{ID: "hex-11", Definition: definitions()["hex.yaml"], Assets: guideOf("hex")},
		{ID: "draughts", Definition: definitions()["draughts.yaml"], Assets: guideOf("draughts")},
		{ID: "breakthrough", Definition: definitions()["breakthrough.yaml"], Assets: guideOf("breakthrough")},
		{ID: "reversi", Definition: definitions()["reversi.yaml"], Assets: guideOf("reversi")},
		{ID: "go-9x9", Definition: definitions()["go.yaml"], Assets: guideOf("go")},
		{ID: "yavalath", Definition: definitions()["yavalath.yaml"], Assets: guideOf("yavalath")},
		{ID: "havannah-8", Definition: definitions()["havannah.yaml"], Assets: guideOf("havannah")},
		{ID: "amazons", Definition: definitions()["amazons.yaml"], Assets: guideOf("amazons")},
		{ID: "ataxx", Definition: definitions()["ataxx.yaml"], Assets: guideOf("ataxx")},
		{ID: "pente", Definition: definitions()["pente.yaml"], Assets: guideOf("pente")},
		{ID: "brandubh", Definition: definitions()["brandubh.yaml"], Assets: guideOf("brandubh")},
	}
}
