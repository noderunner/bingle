//go:build !no_hive

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/hive"
	"github.com/noderunner/bingle/internal/games/registry"
)

func init() {
	registry.Register("hive", func() []catalog.Module { return []catalog.Module{hive.Module()} })
}
