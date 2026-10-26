// Package classics ships a handful of well-known abstract games that are
// defined entirely in JSON using the engine's declarative actions and win
// rules — Tic-Tac-Toe, Connect Four (gravity), Gomoku, Hex (a hexagonal board
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
	_ "embed" // the definitions
	"io/fs"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

//go:embed tictactoe.json
var ticTacToeJSON []byte

//go:embed connect4.json
var connectFourJSON []byte

//go:embed gomoku.json
var gomokuJSON []byte

//go:embed hex.json
var hexJSON []byte

//go:embed draughts.json
var draughtsJSON []byte

//go:embed breakthrough.json
var breakthroughJSON []byte

//go:embed reversi.json
var reversiJSON []byte

//go:embed go.json
var goJSON []byte

//go:embed yavalath.json
var yavalathJSON []byte

//go:embed havannah.json
var havannahJSON []byte

//go:embed amazons.json
var amazonsJSON []byte

//go:embed ataxx.json
var ataxxJSON []byte

//go:embed pente.json
var penteJSON []byte

//go:embed brandubh.json
var brandubhJSON []byte

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
		{ID: "tic-tac-toe", Definition: ticTacToeJSON, Assets: guideOf("tictactoe")},
		{ID: "connect-four", Definition: connectFourJSON, Assets: guideOf("connect4")},
		{ID: "gomoku", Definition: gomokuJSON, Assets: guideOf("gomoku")},
		{ID: "hex-11", Definition: hexJSON, Assets: guideOf("hex")},
		{ID: "draughts", Definition: draughtsJSON, Assets: guideOf("draughts")},
		{ID: "breakthrough", Definition: breakthroughJSON, Assets: guideOf("breakthrough")},
		{ID: "reversi", Definition: reversiJSON, Assets: guideOf("reversi")},
		{ID: "go-9x9", Definition: goJSON, Assets: guideOf("go")},
		{ID: "yavalath", Definition: yavalathJSON, Assets: guideOf("yavalath")},
		{ID: "havannah-8", Definition: havannahJSON, Assets: guideOf("havannah")},
		{ID: "amazons", Definition: amazonsJSON, Assets: guideOf("amazons")},
		{ID: "ataxx", Definition: ataxxJSON, Assets: guideOf("ataxx")},
		{ID: "pente", Definition: penteJSON, Assets: guideOf("pente")},
		{ID: "brandubh", Definition: brandubhJSON, Assets: guideOf("brandubh")},
	}
}
