// Package testdata holds game definitions that only the tests use. They are
// not part of bingle: the server never registers them, the tests that need one
// register it themselves.
package testdata

import (
	"embed"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/engine/wasmrules"
	"github.com/noderunner/bingle/internal/web/look"
	"github.com/noderunner/bingle/tests/testdata/council"
	"github.com/noderunner/bingle/tests/testdata/draughts"
	"github.com/noderunner/bingle/tests/testdata/tricks"
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

//go:embed table
var tableFS embed.FS

var table = sync.OnceValue(func() []byte { return dsl.MustLoad(tableFS, "table/table.yaml") })

// TableID is the id of the Table fixture.
const TableID = "test-table"

// Table is a small game of tiles laid on a table with no board, which grows
// as they are laid (the board style "free"), with the ring and marker shapes:
// the public suite's vehicle for what only commercial board games drew.
func Table() catalog.Module {
	return catalog.Module{ID: TableID, Definition: table(), Web: look.MustLoadFile(tableFS, "table/web.yaml")}
}

// Fixtures are every game only the tests use.
func Fixtures() []catalog.Module {
	return []catalog.Module{Freeplay(), TicTacToe(), ConnectFour(), council.Module(), tricks.Module(), Table()}
}

//go:embed draughts/draughts.yaml
var draughtsFS embed.FS

var draughtsDefinition = sync.OnceValue(func() []byte { return dsl.MustLoad(draughtsFS, "draughts/draughts.yaml") })

// Draughts is English draughts, the fixture for rules that run as a
// WebAssembly module (ADR 0147), with its rules linked in. It is not one of
// Fixtures: the tests that want it register it themselves.
func Draughts() catalog.Module { return DraughtsWith(wasmrules.Native(draughts.New)) }

// DraughtsWith is the Draughts fixture played by other rules than the ones
// linked in: the same rules as a module, say.
func DraughtsWith(rules state.Ruleset) catalog.Module {
	return catalog.Module{ID: draughts.ID, Definition: draughtsDefinition(), Rules: rules}
}
