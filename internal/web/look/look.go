// Package look is the web page's appearance file for a game: web.yaml, beside
// the game's definition (ADR 0124). The definition says what things are; this
// file says how the web page draws them. Only the web page reads it. Another
// client ignores it and draws the game its own way, and the server serves its
// bytes without reading them.
//
// The types here are the file's schema. A game's module loads its web.yaml
// with Load, which checks every key against them, and Check holds what the
// file names to the game's definition.
package look

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"reflect"
	"regexp"
	"slices"
	"strconv"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

// FileName is the appearance file's name in a game's folder.
const FileName = "web.yaml"

// File is a game family's web.yaml. Each field mirrors the definition's field
// it replaces, keyed the same way, so moving a field from the definition into
// this file keeps its shape.
type File struct {
	// Seats is keyed by seat number ("1").
	Seats   map[string]Seat  `json:"seats,omitempty"`
	Pieces  map[string]Piece `json:"pieces,omitempty"`
	Board   *Board           `json:"board,omitempty"`
	Cards   *Cards           `json:"cards,omitempty"`
	Display *Display         `json:"display,omitempty"`
	// Preview is a sample position drawn in the list of games and on the
	// setup page, in place of an empty start. Squares a smaller variant's
	// board lacks are left out.
	Preview []dsl.SetupEntry `json:"preview,omitempty"`
	// Data is what the game's page script reads and nothing else does, in a
	// shape of the script's own: Cthulhu Wars' colour for each area and where
	// its glyphs are printed. Nothing checks it but the script; variants
	// patch it like the rest.
	Data map[string]any `json:"data,omitempty"`
	// Script is the game's own page script, a path among the module's
	// Scripts (served under /static/games/{family}/): an ES module whose
	// exports the page asks for the parts it draws for this game alone.
	Script string `json:"script,omitempty"`
	// Style is the game's own stylesheet, a path among the module's Scripts
	// like Script: the rules for what its page script draws, which the page
	// links once it has asked for one of the family's games.
	Style string `json:"style,omitempty"`
	// Variants patches the file for one game of the family, by its id, as
	// a JSON merge patch (the definition's variants work the same way).
	Variants map[string]File `json:"variants,omitempty"`
}

// Seat is how a seat is drawn.
type Seat struct {
	// Blend is the two colours a seat that plays both is drawn in, as a
	// gradient on its swatch and label.
	Blend []string `json:"blend,omitempty"`
}

// Piece is how a piece type is drawn. A piece type's own colour (Blooms'
// stones) is not here: the rules read it to say whose the piece is (ADR 0117),
// so it stays in the definition.
type Piece struct {
	// Shape names what the page draws: disc, square, flat, wall, cap, ring,
	// diamond, triangle, cross, a chess piece (pawn, knight, bishop, rook,
	// queen, king), donut and chip, or a shape the game's page script adds
	// (Hive's bug-* tiles).
	Shape string `json:"shape,omitempty"`
}

// Board is how the board is drawn.
type Board struct {
	// Style is squares, points, none, free or areas.
	Style string `json:"style,omitempty"`
	// Borders tints board edges: region name -> seat colour.
	Borders map[string]string `json:"borders,omitempty"`
	// Art is a picture of the printed board, a path among the game's own
	// files, for the game's page script to draw it behind an areas board (the
	// shared page draws no areas board itself). A variant names its own.
	Art string `json:"art,omitempty"`
	// Canvas is the coordinate space of an areas board's outlines.
	Canvas *Canvas `json:"canvas,omitempty"`
	// Areas outlines each position of an areas board, in the canvas. It is a
	// list, as the definition's positions are, so a variant's list replaces
	// the family's whole: each map has areas of its own.
	Areas []Area `json:"areas,omitempty"`
}

// Canvas is the width and height of an areas board's coordinate space.
type Canvas struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Area is how one position of an areas board is drawn.
type Area struct {
	// ID is the position's.
	ID string `json:"id"`
	// Polygons outline the area: each is a list of [x, y] corners in the
	// canvas. More than one draws one area in several pieces (an ocean
	// that wraps round the edge of the map).
	Polygons [][][2]float64 `json:"polygons"`
	// Anchor is where the area's name and its tokens are centred; without
	// one, the centre of its largest polygon.
	Anchor *[2]float64 `json:"anchor,omitempty"`
}

// Cards is how a card game's cards and zones are drawn.
type Cards struct {
	Back  *CardBack            `json:"back,omitempty"`
	Zones map[string]Zone      `json:"zones,omitempty"`
	Suits map[string]SuitColor `json:"suits,omitempty"`
}

// CardBack is how the backs of the cards look: a colour, and a word printed
// on them (Nyet!'s).
type CardBack struct {
	Color string `json:"color,omitempty"`
	Text  string `json:"text,omitempty"`
}

// Zone is how a zone of cards is laid out: "fan" (a hand, overlapping, all
// faces), "row" (side by side), "stack" (its top card and a count), "splay"
// (a cascade, each card partly covering the last) or "grid". Without one the
// page draws a pile (the definition's pile: true) as a stack and anything
// else as a row.
type Zone struct {
	Layout string `json:"layout,omitempty"`
}

// layouts are the zone layouts the page draws.
// pictureFile is what board art may be.
var pictureFile = regexp.MustCompile(`(?i)\.(png|jpe?g|webp|svg)$`)

var layouts = map[string]bool{"fan": true, "row": true, "stack": true, "splay": true, "grid": true}

// SuitColor is the colour a suit is drawn in, where the rules do not read it.
type SuitColor struct {
	Color string `json:"color,omitempty"`
}

// Display is how the facts a game's positions carry are drawn. Each key is a
// tint or badge the definition's display names, with its label and text.
type Display struct {
	Tints  map[string]Mark `json:"tints,omitempty"`
	Badges map[string]Mark `json:"badges,omitempty"`
	// NoHands hides the page's generic reserve ("hand") panels.
	NoHands *bool `json:"no_hands,omitempty"`
}

// Mark is how one tint or badge is drawn.
type Mark struct {
	// Color is a CSS colour.
	Color string `json:"color,omitempty"`
	// Glyph is a short text mark.
	Glyph string `json:"glyph,omitempty"`
}

// Load reads the web.yaml at the root of fsys and returns it as JSON, with
// every key checked against File. A family without one gets nil.
func Load(fsys fs.FS) ([]byte, error) { return LoadFile(fsys, FileName) }

// LoadFile is Load for a look file of another name, for a folder that holds
// several games (the classics keep theirs in web/<game>.yaml).
func LoadFile(fsys fs.FS, name string) ([]byte, error) {
	if _, err := fs.Stat(fsys, name); err != nil {
		return nil, nil
	}
	return dsl.LoadAs(fsys, name, reflect.TypeFor[File]())
}

// MustLoad is Load for a file embedded in the binary: an error is a build
// mistake, so it panics.
func MustLoad(fsys fs.FS) []byte { return MustLoadFile(fsys, FileName) }

// MustLoadFile is LoadFile for a file embedded in the binary.
func MustLoadFile(fsys fs.FS, name string) []byte {
	data, err := LoadFile(fsys, name)
	if err != nil {
		panic(err)
	}
	return data
}

// Check holds the file (as Load returned it) to the definitions of the
// family's games: every seat, piece, tint, badge, zone and suit it names must
// be the game's, every variant it patches one of the family, and the preview
// made of the game's pieces in colours that are played. games maps each of
// the family's games, by id, to the game compiled.
func Check(data []byte, family string, games map[string]*catalog.CompiledGame) error {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("look: %s: %w", family, err)
	}
	base, ok := games[family]
	if !ok {
		return fmt.Errorf("look: %s: no definition for the family", family)
	}
	if err := f.check(base); err != nil {
		return fmt.Errorf("look: %s: %w", family, err)
	}
	for _, file := range []struct{ kind, path string }{{"script", f.Script}, {"style", f.Style}} {
		if file.path == "" {
			continue
		}
		if base.Scripts == nil {
			return fmt.Errorf("look: %s: %s %q, but the module brings no scripts", family, file.kind, file.path)
		}
		if _, err := fs.Stat(base.Scripts, file.path); err != nil {
			return fmt.Errorf("look: %s: %s %q: %w", family, file.kind, file.path, err)
		}
	}
	for id, v := range f.Variants {
		def, ok := games[id]
		if !ok {
			return fmt.Errorf("look: %s: variant %q is not a game of the family", family, id)
		}
		if len(v.Variants) > 0 {
			return fmt.Errorf("look: %s: variant %q has variants of its own", family, id)
		}
		if err := v.check(def); err != nil {
			return fmt.Errorf("look: %s: variant %q: %w", family, id, err)
		}
	}
	// Each game drawn as a map of areas has an outline for every position,
	// in a canvas: the family's board with the game's variant over it.
	for id, cg := range games {
		b := f.Board
		if v, ok := f.Variants[id]; ok && v.Board != nil {
			b = mergeBoard(b, v.Board)
		}
		if err := b.complete(cg); err != nil {
			if id == family {
				return fmt.Errorf("look: %s: %w", family, err)
			}
			return fmt.Errorf("look: %s: variant %q: %w", family, id, err)
		}
	}
	return nil
}

// mergeBoard is variant's board over base's, as a merge patch lays it.
func mergeBoard(base, variant *Board) *Board {
	if base == nil {
		return variant
	}
	out := *base
	if variant.Style != "" {
		out.Style = variant.Style
	}
	if variant.Art != "" {
		out.Art = variant.Art
	}
	if variant.Canvas != nil {
		out.Canvas = variant.Canvas
	}
	if variant.Areas != nil {
		out.Areas = variant.Areas
	}
	return &out
}

// complete says that a board drawn as areas outlines every position of the
// game in a canvas.
func (b *Board) complete(cg *catalog.CompiledGame) error {
	if b == nil || b.Style != "areas" {
		return nil
	}
	if b.Canvas == nil {
		return fmt.Errorf("a map of areas needs a canvas")
	}
	drawn := map[string]bool{}
	for _, a := range b.Areas {
		drawn[a.ID] = true
	}
	for _, id := range cg.Board.Positions() {
		if !drawn[string(id)] {
			return fmt.Errorf("area %q has no outline", id)
		}
	}
	return nil
}

func (f File) check(cg *catalog.CompiledGame) error {
	def := cg.Definition
	seats := map[string]bool{}
	colors := map[string]bool{}
	for _, s := range def.Seats {
		seats[strconv.Itoa(s.Seat)] = true
		for _, c := range s.Palette() {
			colors[c] = true
		}
	}
	for _, c := range def.ExtraColors {
		colors[c] = true
	}
	for key, s := range f.Seats {
		if !seats[key] {
			return fmt.Errorf("seat %q is not one of the game's", key)
		}
		for _, c := range s.Blend {
			if !colors[c] {
				return fmt.Errorf("seat %s blends %q, which nobody plays", key, c)
			}
		}
	}
	for name := range f.Pieces {
		if _, ok := def.Pieces[name]; !ok {
			return fmt.Errorf("piece %q is not one of the game's", name)
		}
	}
	if f.Board != nil {
		// A border may be any colour: a seat's (Hex's edges) or none's
		// (Brandubh's corners).
		for region := range f.Board.Borders {
			if _, ok := cg.Board.Region(region); !ok && def.Board.Regions[region] == nil {
				return fmt.Errorf("border %q is not a region of the board", region)
			}
		}
		if c := f.Board.Canvas; c != nil && (c.Width <= 0 || c.Height <= 0) {
			return fmt.Errorf("the canvas needs a positive width and height")
		}
		seen := map[string]bool{}
		for _, a := range f.Board.Areas {
			if !cg.Board.HasPosition(board.PositionID(a.ID)) {
				return fmt.Errorf("area %q is not a position of the board", a.ID)
			}
			if seen[a.ID] {
				return fmt.Errorf("area %q is outlined twice", a.ID)
			}
			seen[a.ID] = true
			if len(a.Polygons) == 0 {
				return fmt.Errorf("area %q has no polygon", a.ID)
			}
			for _, poly := range a.Polygons {
				if len(poly) < 3 {
					return fmt.Errorf("area %q has a polygon with fewer than three corners", a.ID)
				}
			}
		}
		if art := f.Board.Art; art != "" {
			if !fs.ValidPath(art) || !pictureFile.MatchString(art) {
				return fmt.Errorf("board art %q has to be a PNG, JPEG, WebP or SVG among the game's files", art)
			}
			if cg.Assets == nil {
				return fmt.Errorf("board art %q, but the game brings no files", art)
			}
			if _, err := fs.Stat(cg.Assets, art); err != nil {
				return fmt.Errorf("board art %q: %w", art, err)
			}
		}
	}
	if f.Display != nil {
		var tints, badges map[string]dsl.OverlayDef
		if def.Display != nil {
			tints, badges = def.Display.Tints, def.Display.Badges
		}
		for key := range f.Display.Tints {
			if _, ok := tints[key]; !ok {
				return fmt.Errorf("tint %q is not in the definition's display, with its label", key)
			}
		}
		for key := range f.Display.Badges {
			if _, ok := badges[key]; !ok {
				return fmt.Errorf("badge %q is not in the definition's display, with its label", key)
			}
		}
	}
	if f.Cards != nil {
		if def.Cards == nil {
			return fmt.Errorf("cards are drawn, but the game has none")
		}
		for id, z := range f.Cards.Zones {
			if !slices.ContainsFunc(def.Cards.Zones, func(z dsl.ZoneDef) bool { return z.ID == id }) {
				return fmt.Errorf("zone %q is not one of the game's", id)
			}
			if z.Layout != "" && !layouts[z.Layout] {
				return fmt.Errorf("zone %q has unknown layout %q", id, z.Layout)
			}
		}
		for id := range f.Cards.Suits {
			if _, ok := def.Cards.Suits[id]; !ok {
				return fmt.Errorf("suit %q is not one of the game's", id)
			}
		}
	}
	for i, e := range f.Preview {
		if _, ok := def.Pieces[e.Piece]; !ok {
			return fmt.Errorf("preview entry %d is of piece %q, which the game lacks", i, e.Piece)
		}
		if !colors[e.Color] {
			return fmt.Errorf("preview entry %d is %q, which nobody plays", i, e.Color)
		}
	}
	return nil
}
