//go:build !no_meridians

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/meridians"
	"github.com/noderunner/bingle/internal/games/registry"
)

func init() {
	registry.Register("meridians", func() []catalog.Module { return []catalog.Module{meridians.Module()} })
}
