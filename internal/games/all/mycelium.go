//go:build !no_mycelium

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/mycelium"
	"github.com/noderunner/bingle/internal/games/registry"
)

func init() {
	registry.Register("mycelium", func() []catalog.Module { return mycelium.Modules() })
}
