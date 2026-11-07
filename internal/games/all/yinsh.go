//go:build !no_yinsh

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/registry"
	"github.com/noderunner/bingle/internal/games/yinsh"
)

func init() {
	registry.Register("yinsh", func() []catalog.Module { return []catalog.Module{yinsh.Module()} })
}
