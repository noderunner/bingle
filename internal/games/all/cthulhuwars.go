//go:build !no_cthulhuwars

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
	"github.com/noderunner/bingle/internal/games/registry"
)

func init() {
	registry.Register("cthulhu-wars", func() []catalog.Module { return []catalog.Module{cthulhuwars.Module()} })
}
