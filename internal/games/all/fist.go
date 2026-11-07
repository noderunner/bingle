//go:build !no_fist

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/fist"
	"github.com/noderunner/bingle/internal/games/registry"
)

func init() {
	registry.Register("fist", func() []catalog.Module { return []catalog.Module{fist.Module()} })
}
