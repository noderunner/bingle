//go:build !no_cthulhuwars

package all

import (
	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
	"github.com/noderunner/bingle/internal/games/registry"
)

func init() {
	registry.Register("cthulhu-wars", func() []catalog.Module { return []catalog.Module{cthulhuwars.Module()} })
	// Its practice batches' cults, measurements, report and guide knobs.
	batch.RegisterStudy("cthulhu-wars", cthulhuwars.Study{})
}
