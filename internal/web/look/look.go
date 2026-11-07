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
	"slices"
	"strconv"

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
	// queen, king), Yinsh's donut and chip, or one of Hive's bug-* tiles.
	Shape string `json:"shape,omitempty"`
}

// Board is how the board is drawn.
type Board struct {
	// Style is squares, points, none, free or areas.
	Style string `json:"style,omitempty"`
	// Borders tints board edges: region name -> seat colour.
	Borders map[string]string `json:"borders,omitempty"`
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

// Zone is how a zone of cards is laid out.
type Zone struct {
	Layout string `json:"layout,omitempty"`
}

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
		for id := range f.Cards.Zones {
			if !slices.ContainsFunc(def.Cards.Zones, func(z dsl.ZoneDef) bool { return z.ID == id }) {
				return fmt.Errorf("zone %q is not one of the game's", id)
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
