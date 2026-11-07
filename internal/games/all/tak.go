//go:build !no_tak

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/registry"
	"github.com/noderunner/bingle/internal/games/tak"
)

func init() {
	registry.Register("tak", func() []catalog.Module { return []catalog.Module{tak.Module()} })
}
