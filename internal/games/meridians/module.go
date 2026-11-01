package meridians

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

//go:embed meridians.yaml
var definitionFS embed.FS

// definition is the definition as JSON, converted once.
var definition = sync.OnceValue(func() []byte { return dsl.MustLoad(definitionFS, "meridians.yaml") })

//go:embed guide
var guideFS embed.FS

// Module returns Meridians' catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, "guide")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "meridians", Definition: definition(), Rules: New(), Assets: sub}
}
