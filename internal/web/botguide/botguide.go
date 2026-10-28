// Package botguide holds the guide to bingle's bots: what each of them does,
// the algorithms behind them, and how to use a batch of bot games to study a
// game itself. It is one Markdown document with a few drawings, embedded in the
// binary and served to the Analysis tab of the home page (see
// internal/api/botguide.go); the drawings are made by scripts/botdiagrams.py.
package botguide

import (
	"embed"
	"io/fs"
)

//go:embed bots.md assets
var files embed.FS

// Text is the guide itself, Markdown as js/markdown.js renders it.
func Text() ([]byte, error) { return files.ReadFile("bots.md") }

// Assets are the drawings the guide refers to, under "assets/".
func Assets() (fs.FS, error) { return fs.Sub(files, "assets") }
