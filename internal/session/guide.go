package session

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// GuideTabInfo names one tab of a game's rules guide.
type GuideTabInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// GuideTab is one tab with its Markdown page.
type GuideTab struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Markdown string `json:"markdown"`
}

// Guide is a game's rules guide: Markdown pages for the rules window. Images
// in the pages are relative to /game-defs/{id}/assets/, and ![](piece:type)
// stands for a piece's icon.
type Guide struct {
	Game string     `json:"game"`
	Tabs []GuideTab `json:"tabs"`
	// Generated is true when the game ships no guide of its own and the
	// pages were assembled from its definition.
	Generated bool `json:"generated,omitempty"`
}

// ErrAssetNotFound reports a game asset that does not exist.
var ErrAssetNotFound = errors.New("session: asset not found")

// Guide returns the rules guide of the game definition id: the module's
// own pages when it has them, otherwise pages generated from the
// definition.
func (m *Manager) Guide(id string) (Guide, error) {
	cg, ok := m.catalog.Get(id)
	if !ok {
		return Guide{}, fmt.Errorf("%w: %q", ErrGameDefNotFound, id)
	}
	g := Guide{Game: id}
	if def := cg.Definition; def.Guide != nil && cg.Assets != nil {
		for _, t := range def.Guide.Tabs {
			b, err := fs.ReadFile(cg.Assets, t.File)
			if err != nil {
				return Guide{}, fmt.Errorf("session: guide page %q: %w", t.File, err)
			}
			g.Tabs = append(g.Tabs, GuideTab{ID: t.ID, Title: t.Title, Markdown: m.branded(string(b))})
		}
		return g, nil
	}
	g.Generated = true
	g.Tabs = generatedGuide(buildGameInfo(cg))
	return g, nil
}

// WebFile returns the web page's appearance file for the game's family
// (web.yaml as JSON, ADR 0124), or an empty object when it has none. The
// server does not read it: the web page resolves the game's variant in it.
func (m *Manager) WebFile(id string) ([]byte, error) {
	cg, ok := m.catalog.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrGameDefNotFound, id)
	}
	if cg.Web == nil {
		return []byte("{}"), nil
	}
	return cg.Web, nil
}

// GameScripts returns the web page's scripts of a game family (ADR 0124), to
// be served as they are; ok is false for a family with none.
func (m *Manager) GameScripts(family string) (fs.FS, bool) {
	cg, ok := m.catalog.Get(family)
	if !ok || cg.Scripts == nil {
		return nil, false
	}
	return cg.Scripts, true
}

// Asset returns a file of the game's module (guide images and so on). The
// server draws no pictures of its own (ADR 0124): a guide's map of an areas
// board is drawn by the game's page script (markdown's figure:).
func (m *Manager) Asset(id, name string) (data []byte, contentType string, err error) {
	cg, ok := m.catalog.Get(id)
	if !ok {
		return nil, "", fmt.Errorf("%w: %q", ErrGameDefNotFound, id)
	}
	clean := path.Clean("/" + name)[1:]
	if clean == "" || !fs.ValidPath(clean) {
		return nil, "", fmt.Errorf("%w: %q", ErrAssetNotFound, name)
	}
	if cg.Assets != nil {
		if b, err := fs.ReadFile(cg.Assets, clean); err == nil {
			return b, assetType(clean), nil
		}
	}
	return nil, "", fmt.Errorf("%w: %q", ErrAssetNotFound, name)
}

func assetType(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".md":
		return "text/markdown; charset=utf-8"
	case ".json":
		return "application/json"
	}
	return "application/octet-stream"
}

// generatedGuide assembles the page of a game that ships no guide: one Rules
// tab with the description, the rules, how the game ends and the move
// notation. A game that deserves more (a pieces page, diagrams) ships its own
// guide instead.
func generatedGuide(info GameInfo) []GuideTab {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", info.Name)
	if info.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", info.Description)
	}
	for _, r := range info.Rules {
		fmt.Fprintf(&b, "%s\n\n", r)
	}
	if len(info.WinRules) > 0 {
		b.WriteString("## How the game ends\n\n")
		for _, r := range info.WinRules {
			fmt.Fprintf(&b, "- %s\n", r)
		}
		b.WriteString("\n")
	}
	if len(info.NotationDoc) > 0 {
		b.WriteString("## Move notation\n\n")
		for _, r := range info.NotationDoc {
			fmt.Fprintf(&b, "- %s\n", escapeMD(r))
		}
	}
	return []GuideTab{{ID: "rules", Title: "Rules", Markdown: b.String()}}
}

// escapeMD makes text safe to embed as Markdown prose.
func escapeMD(s string) string {
	return strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "<", "\\<", ">", "\\>", "|", "\\|", "~", "\\~").Replace(s)
}
