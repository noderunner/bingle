//go:build !no_blooms

package all

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/blooms"
	"github.com/noderunner/bingle/internal/games/registry"
)

func init() {
	registry.Register("blooms", func() []catalog.Module { return []catalog.Module{blooms.Module()} })
}
