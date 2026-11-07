//go:build !no_courtfall

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/courtfall"
	"github.com/noderunner/bingle/internal/games/registry"
)

func init() {
	registry.Register("courtfall", func() []catalog.Module { return courtfall.Modules() })
}
