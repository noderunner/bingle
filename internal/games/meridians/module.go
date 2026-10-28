package meridians

import (
	"embed"
	_ "embed" // the definition
	"io/fs"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

//go:embed meridians.json
var definitionJSON []byte

//go:embed guide
var guideFS embed.FS

// Module returns Meridians' catalog.Module.
func Module() catalog.Module {
	sub, err := fs.Sub(guideFS, "guide")
	if err != nil {
		panic(err)
	}
	return catalog.Module{ID: "meridians", Definition: definitionJSON, Rules: New(), Assets: sub}
}
