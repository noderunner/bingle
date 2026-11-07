//go:build !no_solitaire

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/registry"
	"github.com/noderunner/bingle/internal/games/solitaire"
)

func init() {
	registry.Register("solitaire", func() []catalog.Module { return []catalog.Module{solitaire.Module(), solitaire.BowlingModule()} })
}
