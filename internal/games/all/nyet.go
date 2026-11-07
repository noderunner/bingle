//go:build !no_nyet

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/nyet"
	"github.com/noderunner/bingle/internal/games/registry"
)

func init() {
	registry.Register("nyet", func() []catalog.Module { return []catalog.Module{nyet.Module()} })
}
