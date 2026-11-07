//go:build !no_onitama

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/onitama"
	"github.com/noderunner/bingle/internal/games/registry"
)

func init() {
	registry.Register("onitama", func() []catalog.Module { return []catalog.Module{onitama.Module()} })
}
