// Package chess ships Chess, with the rules of the FIDE Laws of Chess:
// castling, en passant, promotion, checkmate and stalemate, and the draws by
// repetition, the fifty-move rule and insufficient material.
//
// Its rules are written in Go (a state.Ruleset) because check, castling and
// en passant are not things the declarative DSL expresses. chess.yaml still
// describes the board, seats and pieces, so the lobby, UI, API, MCP server and
// bots treat it like any other game. The game has no secrets and no chance.
package chess

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/web/look"
)

//go:embed chess.yaml web.yaml
var definitionFS embed.FS

// definition is the definition as JSON, converted once.
var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "chess.yaml") })

//go:embed guide
var guideFS embed.FS

//go:embed tutorials
var tutorialsFS embed.FS

// Module returns Chess's catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, ".")
	if err != nil {
		panic(err)
	}
	tutorials, err := fs.Sub(tutorialsFS, "tutorials")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "chess", Definition: definition(), Rules: New(), Assets: sub, Tutorials: tutorials, Web: look.MustLoad(definitionFS)}
}
