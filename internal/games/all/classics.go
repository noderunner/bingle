//go:build !no_classics

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/classics"
	"github.com/noderunner/bingle/internal/games/registry"
)

func init() {
	registry.Register("classics", func() []catalog.Module { return classics.Modules() })
}
