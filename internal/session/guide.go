package session

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path"
	"sort"
	"strings"

	"github.com/noderunner/bingle/internal/engine/dsl"
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
			g.Tabs = append(g.Tabs, GuideTab{ID: t.ID, Title: t.Title, Markdown: string(b)})
		}
		return g, nil
	}
	g.Generated = true
	g.Tabs = generatedGuide(buildGameInfo(cg))
	return g, nil
}

// Asset returns a file of the game's module (guide images and so on). The
// virtual file "board.svg" draws an area map from its polygons.
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
	if clean == "board.svg" && cg.Definition.Board.Style == "areas" {
		return []byte(areaMapSVG(cg.Definition)), "image/svg+xml", nil
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

// areasOf converts an areas board's positions to the client form.
func areasOf(def *dsl.GameDefinition) (*CanvasDTO, []AreaDTO) {
	var canvas *CanvasDTO
	if c := def.Board.Canvas; c != nil {
		canvas = &CanvasDTO{Width: c.Width, Height: c.Height}
	}
	var out []AreaDTO
	for _, p := range def.Board.Positions {
		a := AreaDTO{ID: p.ID, Name: p.Name, Polygons: p.Polygons, Tags: p.Tags}
		if a.Name == "" {
			a.Name = p.ID
		}
		if p.Anchor != nil {
			a.Anchor = *p.Anchor
		} else {
			a.Anchor = polygonCenter(p.Polygons)
		}
		out = append(out, a)
	}
	return canvas, out
}

// polygonCenter is the centroid of the largest polygon.
func polygonCenter(polys [][][2]float64) [2]float64 {
	var best [][2]float64
	bestArea := -1.0
	for _, poly := range polys {
		if a := math.Abs(polygonArea(poly)); a > bestArea {
			best, bestArea = poly, a
		}
	}
	if len(best) == 0 {
		return [2]float64{}
	}
	var cx, cy, a2 float64
	for i := range best {
		p, q := best[i], best[(i+1)%len(best)]
		cross := p[0]*q[1] - q[0]*p[1]
		a2 += cross
		cx += (p[0] + q[0]) * cross
		cy += (p[1] + q[1]) * cross
	}
	if a2 == 0 {
		return best[0]
	}
	return [2]float64{cx / (3 * a2), cy / (3 * a2)}
}

func polygonArea(poly [][2]float64) float64 {
	var a2 float64
	for i := range poly {
		p, q := poly[i], poly[(i+1)%len(poly)]
		a2 += p[0]*q[1] - q[0]*p[1]
	}
	return a2 / 2
}

// mapPalette is the gradient of each kind of area on the map picture:
// light/dark stops for light mode, then for dark mode.
var mapPalette = []struct{ kind, a, b, da, db string }{
	{"pacific", "#8cbfe4", "#5f95c8", "#3b6a96", "#1f4468"},
	{"atlantic", "#98c9ec", "#6aa5d3", "#447aa6", "#25507a"},
	{"indian", "#82c4dc", "#509fc4", "#377f9a", "#1c5878"},
	{"arctic", "#f2f9fd", "#c4deef", "#8aa9bd", "#5d7f96"},
	{"c-na", "#d7e6a5", "#a9c276", "#7f8f55", "#5a6a37"},
	{"c-sa", "#c1e38f", "#86b55f", "#6f9a46", "#4a7330"},
	{"c-eu", "#efe2b3", "#c9b982", "#9d9065", "#766a45"},
	{"c-me", "#f5d69e", "#d8aa6b", "#a98550", "#7f5f33"},
	{"c-af", "#f6dc93", "#dcaf5d", "#ad9142", "#876c27"},
	{"c-as", "#e3d9a8", "#bcaf79", "#958c62", "#6f6840"},
	{"c-au", "#f3c896", "#d49d61", "#b27c47", "#85582b"},
	{"c-ice", "#ffffff", "#d1dfec", "#cfdbe6", "#98aabd"},
}

// glyphPaths are the glyph symbols in a 24x24 box (the client draws the same).
var glyphPaths = map[string]string{
	"dragon":  "M -9 4 Q -6 -9 -1 -1 T 8 -3 M 8 -3 l 3 -2 M 6 -6 l 0 3",
	"chevron": "M -9 5 L 0 -6 L 9 5 M -9 -1 L 0 -12 L 9 -1",
	"thorns":  "M -10 8 L -7 -6 L -4 8 M -4 8 L -1 -9 L 2 8 M 2 8 L 5 -5 L 8 8",
}

// areaMapSVG draws an areas board as a standalone SVG picture (for guides):
// graded seas with waves, a pale shore round every coast, lands in their own
// colours, names on top and a badge for each glyph.
func areaMapSVG(def *dsl.GameDefinition) string {
	canvas, areas := areasOf(def)
	w, h := 1000.0, 490.0
	if canvas != nil {
		w, h = canvas.Width, canvas.Height
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="-8 -8 %g %g" role="img" aria-label="%s map">`, w+16, h+16, escapeXML(def.Name))
	b.WriteString("<style>")
	for _, p := range mapPalette {
		fmt.Fprintf(&b, ".s-%[1]s-0{stop-color:%[2]s}.s-%[1]s-1{stop-color:%[3]s}", p.kind, p.a, p.b)
	}
	b.WriteString(".sea{stroke:#ffffffb3;stroke-width:1.5;stroke-dasharray:8 6}.land{stroke:#5a4a2b;stroke-width:1.6}")
	b.WriteString(".shore{fill:none;stroke:#ffffff99;stroke-width:10;stroke-linejoin:round}.frame{fill:#8a6a43}")
	b.WriteString(".waves{fill:url(#waves);opacity:.3}.name{font:700 12.5px system-ui,sans-serif;fill:#1f2a1a;stroke:#ffffffd9;stroke-width:3.2px;paint-order:stroke;text-anchor:middle}")
	b.WriteString(".name.o{font:italic 600 13px Georgia,serif;letter-spacing:.1em;fill:#123a5e;stroke:#ffffff8c}")
	b.WriteString(".gb circle{fill:#2b2a26;stroke:#fff;stroke-width:1.5}.gb path{fill:none;stroke:#ffe9a8;stroke-width:2;stroke-linecap:round;stroke-linejoin:round}")
	b.WriteString("@media (prefers-color-scheme:dark){")
	for _, p := range mapPalette {
		fmt.Fprintf(&b, ".s-%[1]s-0{stop-color:%[2]s}.s-%[1]s-1{stop-color:%[3]s}", p.kind, p.da, p.db)
	}
	b.WriteString(".land{stroke:#14110a}.sea{stroke:#bedcff59}.shore{stroke:#96c8ff47}.name{fill:#f2f5ee;stroke:#000000bf}.name.o{fill:#cfe6fb;stroke:#00000099}}")
	b.WriteString("</style><defs>")
	for _, p := range mapPalette {
		if strings.HasPrefix(p.kind, "c-") {
			fmt.Fprintf(&b, `<radialGradient id="g-%[1]s" cx=".5" cy=".42" r=".82"><stop offset="0" class="s-%[1]s-0"/><stop offset="1" class="s-%[1]s-1"/></radialGradient>`, p.kind)
		} else {
			fmt.Fprintf(&b, `<linearGradient id="g-%[1]s" x1="0" y1="0" x2=".45" y2="1"><stop offset="0" class="s-%[1]s-0"/><stop offset="1" class="s-%[1]s-1"/></linearGradient>`, p.kind)
		}
	}
	b.WriteString(`<pattern id="waves" width="30" height="16" patternUnits="userSpaceOnUse"><path d="M0 8 q7.5 -7 15 0 t15 0" fill="none" stroke="#fff" stroke-width="1.1" stroke-linecap="round"/></pattern>`)
	fmt.Fprintf(&b, `<clipPath id="clip"><rect width="%g" height="%g"/></clipPath></defs>`, w, h)
	fmt.Fprintf(&b, `<rect class="frame" x="-8" y="-8" width="%g" height="%g" rx="12"/>`, w+16, h+16)

	kind := func(a AreaDTO) (string, bool) {
		ocean := false
		for _, t := range a.Tags {
			switch t {
			case "ocean":
				ocean = true
			case "pacific", "atlantic", "indian", "arctic", "c-na", "c-sa", "c-eu", "c-me", "c-af", "c-as", "c-au", "c-ice":
				return t, ocean
			}
		}
		if ocean {
			return "pacific", true
		}
		return "c-eu", false
	}
	area := func(poly [][2]float64) float64 {
		var a2 float64
		for i := range poly {
			p, q := poly[i], poly[(i+1)%len(poly)]
			a2 += p[0]*q[1] - q[0]*p[1]
		}
		return math.Abs(a2) / 2
	}
	size := func(a AreaDTO) float64 {
		var sum float64
		for _, p := range a.Polygons {
			sum += area(p)
		}
		return sum
	}
	ordered := make([]AreaDTO, len(areas))
	copy(ordered, areas)
	sort.SliceStable(ordered, func(i, j int) bool {
		if si, sj := size(ordered[i]), size(ordered[j]); si != sj {
			return si > sj
		}
		return ordered[i].ID < ordered[j].ID
	})
	points := func(poly [][2]float64) string {
		pts := make([]string, len(poly))
		for i, p := range poly {
			pts[i] = fmt.Sprintf("%g,%g", p[0], p[1])
		}
		return strings.Join(pts, " ")
	}
	draw := func(a AreaDTO, ocean bool) {
		k, _ := kind(a)
		class := "land"
		if ocean {
			class = "sea"
		}
		for _, poly := range a.Polygons {
			fmt.Fprintf(&b, `<polygon class="%s" fill="url(#g-%s)" points="%s"/>`, class, k, points(poly))
			if ocean {
				fmt.Fprintf(&b, `<polygon class="waves" points="%s"/>`, points(poly))
			}
		}
	}
	for _, a := range ordered {
		if _, ocean := kind(a); ocean {
			draw(a, true)
		}
	}
	b.WriteString(`<g class="shore" clip-path="url(#clip)">`)
	for _, a := range ordered {
		if _, ocean := kind(a); !ocean {
			for _, poly := range a.Polygons {
				fmt.Fprintf(&b, `<polygon points="%s"/>`, points(poly))
			}
		}
	}
	b.WriteString(`</g>`)
	for _, a := range ordered {
		if _, ocean := kind(a); !ocean {
			draw(a, false)
		}
	}
	for _, a := range ordered {
		_, ocean := kind(a)
		cls := "name"
		if ocean {
			cls = "name o"
		}
		fmt.Fprintf(&b, `<text class="%s" x="%g" y="%g">%s</text>`, cls, a.Anchor[0], a.Anchor[1]+4, escapeXML(a.Name))
		for _, t := range a.Tags {
			if g, ok := strings.CutPrefix(t, "glyph-"); ok && glyphPaths[g] != "" {
				x := a.Anchor[0] + float64(len(a.Name))*3.7 + 15
				fmt.Fprintf(&b, `<g class="gb" transform="translate(%g %g)"><circle r="11"/><path d="%s" transform="scale(.8)"/></g>`, x, a.Anchor[1], glyphPaths[g])
			}
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func escapeXML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
