// Package testdata holds game definitions that only the tests use. They are
// not part of bingle: the server never registers them, the tests that need one
// register it themselves.
package testdata

import (
	"embed"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

//go:embed freeplay.json
var freeplayJSON []byte

// FreeplayID is the id of the Freeplay fixture.
const FreeplayID = "test-freeplay"

// Freeplay is a small rule-free sandbox game on a 9×9 Go-style board: stones
// step one square and capture by landing on an opponent's stone. It drives the
// platform's tests (the REST API, the store and the DSL) with a game whose
// every move and starting position is known, and is not a game to play.
func Freeplay() catalog.Module {
	return catalog.Module{ID: FreeplayID, Definition: freeplayJSON}
}

//go:embed tictactoe.yaml connect4.yaml
var retiredFS embed.FS

var (
	ticTacToe   = sync.OnceValue(func() []byte { return dsl.MustLoad(retiredFS, "tictactoe.yaml") })
	connectFour = sync.OnceValue(func() []byte { return dsl.MustLoad(retiredFS, "connect4.yaml") })
)

// TicTacToe and ConnectFour are the two games bingle retired (ADR 0076),
// kept for the tests under their old ids: Tic-Tac-Toe is the smallest
// complete game, which many tests play to the end, and Connect Four is the
// only game that drops its pieces (the place action's gravity).
func TicTacToe() catalog.Module {
	return catalog.Module{ID: "tic-tac-toe", Definition: ticTacToe()}
}

// ConnectFour: see TicTacToe.
func ConnectFour() catalog.Module {
	return catalog.Module{ID: "connect-four", Definition: connectFour()}
}

// Fixtures are every game only the tests use.
func Fixtures() []catalog.Module {
	return []catalog.Module{Freeplay(), TicTacToe(), ConnectFour()}
}
