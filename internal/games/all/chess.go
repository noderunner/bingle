//go:build !no_chess

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/chess"
	"github.com/noderunner/bingle/internal/games/registry"
)

func init() {
	registry.Register("chess", func() []catalog.Module { return []catalog.Module{chess.Module()} })
}
