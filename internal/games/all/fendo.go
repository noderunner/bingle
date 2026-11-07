//go:build !no_fendo

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/fendo"
	"github.com/noderunner/bingle/internal/games/registry"
)

func init() {
	registry.Register("fendo", func() []catalog.Module { return []catalog.Module{fendo.Module()} })
}
