// Package look_test holds every shipped game's web.yaml (ADR 0124) to its
// definitions, and checks look.Check itself on small files.
package look_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/web/look"
	"github.com/noderunner/bingle/tests/testdata"
)

// families registers every shipped module and returns, for each family whose
// module brings a look, the look and its games' definitions.
type family struct {
	web   []byte
	games map[string]*catalog.CompiledGame
}

func families(t *testing.T) map[string]family {
	t.Helper()
	out := map[string]family{}
	for _, m := range app.BuiltinModules() {
		if m.Web == nil {
			continue
		}
		// The module's games alone: its own and its variants.
		cat := catalog.NewCatalog()
		if err := cat.RegisterModule(m); err != nil {
			t.Fatalf("RegisterModule(%s): %v", m.ID, err)
		}
		f := family{m.Web, map[string]*catalog.CompiledGame{}}
		for _, cg := range cat.List() {
			f.games[cg.Definition.ID] = cg
		}
		out[m.ID] = f
	}
	return out
}

// Every shipped look names only its own games' seats, pieces, tints, badges,
// zones, suits and variants, and previews only pieces in played colours.
func TestEveryLookFitsItsGame(t *testing.T) {
	fams := families(t)
	for _, id := range []string{"solitaire", "bowling-solitaire", "blooms", "tak-5x5", "fendo", "fist", "meridians", "courtfall-1.0.0", "mycelium-1.0.0",
		"gomoku", "hex-11", "draughts", "breakthrough", "reversi", "go-9x9", "yavalath", "havannah-8", "amazons", "ataxx", "pente", "brandubh"} {
		if _, ok := fams[id]; !ok {
			t.Errorf("%s brings no look", id)
		}
	}
	for id, f := range fams {
		if len(f.games) == 0 {
			t.Errorf("%s: no game of the family", id)
		}
		if err := look.Check(f.web, id, f.games); err != nil {
			t.Error(err)
		}
	}
}

// Check catches what a look gets wrong.
func TestCheckCatchesAMistake(t *testing.T) {
	cat := catalog.NewCatalog()
	src := `{"id":"g","name":"G","board":{"generator":{"type":"grid","width":3,"height":3,"adjacency":"orthogonal4"}},
		"seats":[{"seat":1,"color":"red"},{"seat":2,"color":"blue"}],"pieces":{"stone":{"movement":{}}},
		"turn_order":{"type":"round-robin","seats":[1,2]},"actions":[{"id":"place","type":"place","pieces":["stone"]}],
		"win_condition":{"type":"none"},"display":{"tints":{"hot":{"label":"Hot"}}},
		"variants":[{"id":"g-big","patch":{"board":{"generator":{"width":5,"height":5}}}}]}`
	files := fstest.MapFS{"art/board.webp": {Data: []byte("RIFF")}, "art/big.svg": {Data: []byte("<svg/>")}, "notes.txt": {Data: []byte("x")}}
	if err := cat.RegisterModule(catalog.Module{ID: "g", Definition: []byte(src), Assets: files}); err != nil {
		t.Fatal(err)
	}
	games := map[string]*catalog.CompiledGame{}
	for _, id := range []string{"g", "g-big"} {
		cg, ok := cat.Get(id)
		if !ok {
			t.Fatalf("no %s", id)
		}
		games[id] = cg
	}
	for file, want := range map[string]string{
		`{"seats":{"1":{"blend":["red","blue"]}},"pieces":{"stone":{"shape":"ring"}},"display":{"tints":{"hot":{"color":"#f00"}}},"variants":{"g-big":{"board":{"style":"points"}}}}`: "",
		`{"board":{"borders":{"top":"red"}}}`:                            "",
		`{"board":{"borders":{"moat":"red"}}}`:                           `border "moat"`,
		`{"seats":{"3":{}}}`:                                             `seat "3"`,
		`{"seats":{"1":{"blend":["green"]}}}`:                            `"green", which nobody plays`,
		`{"pieces":{"pawn":{}}}`:                                         `piece "pawn"`,
		`{"display":{"tints":{"cold":{}}}}`:                              `tint "cold"`,
		`{"display":{"badges":{"hot":{}}}}`:                              `badge "hot"`,
		`{"cards":{}}`:                                                   "the game has none",
		`{"preview":[{"position":"a1","piece":"pawn","color":"red"}]}`:   `piece "pawn"`,
		`{"preview":[{"position":"a1","piece":"stone","color":"pink"}]}`: `"pink"`,
		`{"variants":{"h":{}}}`:                                          `variant "h"`,
		`{"variants":{"g-big":{"variants":{}}}}`:                         "", // an empty map is no variants
		`{"variants":{"g-big":{"pieces":{"pawn":{}}}}}`:                  `variant "g-big": piece "pawn"`,
		`{"script":"page.js"}`:                                           `script "page.js", but the module brings no scripts`,
		`{"style":"page.css"}`:                                           `style "page.css", but the module brings no scripts`,
		// A picture of the board, for the game's page script: the game's own file.
		`{"board":{"art":"art/board.webp"},"variants":{"g-big":{"board":{"art":"art/big.svg"}}}}`: "",
		`{"board":{"art":"art/none.png"}}`:                        `board art "art/none.png"`,
		`{"variants":{"g-big":{"board":{"art":"art/none.png"}}}}`: `variant "g-big": board art "art/none.png"`,
		`{"board":{"art":"notes.txt"}}`:                           "has to be a PNG, JPEG, WebP or SVG",
		`{"board":{"art":"../art/board.webp"}}`:                   "has to be a PNG, JPEG, WebP or SVG",
		`{"board":{"art":"https://example.com/board.png"}}`:       "has to be a PNG, JPEG, WebP or SVG",
	} {
		err := look.Check([]byte(file), "g", games)
		switch {
		case want == "" && err != nil:
			t.Errorf("%s: %v", file, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("%s: got %v, want an error with %s", file, err, want)
		}
	}
}

// A map of areas is the look's: an outline for every position, in a canvas,
// each variant with its own.
func TestCheckCatchesAnAreaMap(t *testing.T) {
	cat := catalog.NewCatalog()
	src := `{"id":"m","name":"M","board":{"positions":[{"id":"a","name":"A","tags":["land"]},{"id":"b"}],"edges":[{"from":"a","to":"b"}]},
		"seats":[{"seat":1,"color":"red"},{"seat":2,"color":"blue"}],"pieces":{"stone":{"movement":{}}},
		"turn_order":{"type":"round-robin","seats":[1,2]},"actions":[{"id":"place","type":"place","pieces":["stone"]}],
		"win_condition":{"type":"none"},
		"variants":[{"id":"m-3","patch":{"board":{"positions":[{"id":"a"},{"id":"b"},{"id":"c"}],"edges":[{"from":"a","to":"b"},{"from":"b","to":"c"}]}}}]}`
	if err := cat.RegisterModule(catalog.Module{ID: "m", Definition: []byte(src)}); err != nil {
		t.Fatal(err)
	}
	games := map[string]*catalog.CompiledGame{}
	for _, id := range []string{"m", "m-3"} {
		cg, ok := cat.Get(id)
		if !ok {
			t.Fatalf("no %s", id)
		}
		games[id] = cg
	}
	const sq = `[[[0,0],[1,0],[1,1],[0,1]]]`
	two := `[{"id":"a","polygons":` + sq + `,"anchor":[0.5,0.5]},{"id":"b","polygons":` + sq + `}]`
	three := `[{"id":"a","polygons":` + sq + `},{"id":"b","polygons":` + sq + `},{"id":"c","polygons":` + sq + `}]`
	for file, want := range map[string]string{
		`{"board":{"style":"areas","canvas":{"width":2,"height":1},"areas":` + two + `},"variants":{"m-3":{"board":{"areas":` + three + `}}}}`: "",
		// The family's outlines do not draw the variant's third area.
		`{"board":{"style":"areas","canvas":{"width":2,"height":1},"areas":` + two + `}}`:                      `variant "m-3": area "c" has no outline`,
		`{"board":{"style":"areas","areas":` + two + `},"variants":{"m-3":{"board":{"areas":` + three + `}}}}`: "needs a canvas",
		`{"board":{"style":"areas","canvas":{"width":0,"height":1},"areas":` + two + `}}`:                      "positive width and height",
		`{"board":{"areas":[{"id":"z","polygons":` + sq + `}]}}`:                                               `area "z" is not a position`,
		`{"board":{"areas":[{"id":"a","polygons":[]}]}}`:                                                       `area "a" has no polygon`,
		`{"board":{"areas":[{"id":"a","polygons":[[[0,0],[1,1]]]}]}}`:                                          "fewer than three corners",
		`{"board":{"areas":[{"id":"a","polygons":` + sq + `},{"id":"a","polygons":` + sq + `}]}}`:              `area "a" is outlined twice`,
		// Outlines with no areas style are only checked, not required.
		`{"board":{"areas":[{"id":"a","polygons":` + sq + `}]}}`: "",
		// The page script's data is the script's own.
		`{"data":{"fill":{"a":"#123456"}}}`: "",
	} {
		err := look.Check([]byte(file), "m", games)
		switch {
		case want == "" && err != nil:
			t.Errorf("%s: %v", file, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("%s: got %v, want an error with %s", file, err, want)
		}
	}
	// A definition cannot outline its areas any more.
	bad := strings.Replace(src, `{"id":"b"}]`, `{"id":"b","polygons":[[[0,0],[1,0],[1,1]]]}]`, 1)
	if _, err := dsl.Parse([]byte(bad)); err == nil {
		t.Error("a definition with an outline was accepted")
	}
}

// The test fixtures' looks fit their games too: Council's map of areas.
func TestEveryFixtureLookFitsItsGame(t *testing.T) {
	for _, m := range testdata.Fixtures() {
		if m.Web == nil {
			continue
		}
		cat := catalog.NewCatalog()
		if err := cat.RegisterModule(m); err != nil {
			t.Fatalf("RegisterModule(%s): %v", m.ID, err)
		}
		games := map[string]*catalog.CompiledGame{}
		for _, cg := range cat.List() {
			games[cg.Definition.ID] = cg
		}
		if err := look.Check(m.Web, m.ID, games); err != nil {
			t.Error(err)
		}
	}
}

// Load reads web.yaml as JSON and refuses a key the file does not have, such
// as a rule; a folder without one has no look.
func TestLoadChecksEveryKey(t *testing.T) {
	if data, err := look.Load(fstest.MapFS{}); err != nil || data != nil {
		t.Errorf("no file: %s %v", data, err)
	}
	good := fstest.MapFS{look.FileName: {Data: []byte("seats:\n  1: {blend: [red, yellow]}\nvariants:\n  g-big: {board: {style: points}}\n")}}
	if data, err := look.Load(good); err != nil || !strings.Contains(string(data), `"blend":["red","yellow"]`) {
		t.Errorf("a good file: %s %v", data, err)
	}
	for _, bad := range []string{"pieces:\n  stone: {movement: {}}\n", "turn_order: {}\n", "variants:\n  g-big: {rules: []}\n"} {
		if _, err := look.Load(fstest.MapFS{look.FileName: {Data: []byte(bad)}}); err == nil {
			t.Errorf("%q: loaded", bad)
		}
	}
}

// A zone's layout is the look's, and one the page does not draw is caught.
func TestCheckCatchesAZoneLayout(t *testing.T) {
	f := families(t)["solitaire"]
	if err := look.Check([]byte(`{"cards":{"zones":{"waste":{"layout":"fan"}}}}`), "solitaire", f.games); err != nil {
		t.Error(err)
	}
	if err := look.Check([]byte(`{"cards":{"zones":{"waste":{"layout":"heap"}}}}`), "solitaire", f.games); err == nil || !strings.Contains(err.Error(), `unknown layout "heap"`) {
		t.Errorf("a bad layout: %v", err)
	}
	if err := look.Check([]byte(`{"cards":{"zones":{"pocket":{}}}}`), "solitaire", f.games); err == nil {
		t.Error("a zone the game lacks was accepted")
	}
}

// The line of ADR 0124, held: no definition of a shipped game says how it
// looks, the private games' included (ADR 0127). A piece type's
// own colour stays, since the rules read it (ADR 0117); a zone's layout and
// an area's outline cannot be written at all, since the definition only says
// whether a zone is a pile and which areas there are.
func TestNoDefinitionCarriesItsLook(t *testing.T) {
	for _, m := range app.BuiltinModules() {
		cat := catalog.NewCatalog()
		if err := cat.RegisterModule(m); err != nil {
			t.Fatalf("RegisterModule(%s): %v", m.ID, err)
		}
		for _, cg := range cat.List() {
			def, id := cg.Definition, cg.Definition.ID
			if def.Board.Style != "" || len(def.Board.Borders) > 0 {
				t.Errorf("%s: the definition draws the board (style %q, borders %v)", id, def.Board.Style, def.Board.Borders)
			}
			if len(def.Preview) > 0 {
				t.Errorf("%s: the definition has a preview", id)
			}
			for _, s := range def.Seats {
				if len(s.Blend) > 0 {
					t.Errorf("%s: seat %d blends %v", id, s.Seat, s.Blend)
				}
			}
			for name, p := range def.Pieces {
				if p.Shape != "" {
					t.Errorf("%s: piece %s has shape %q", id, name, p.Shape)
				}
			}
			if d := def.Display; d != nil {
				if d.NoHands {
					t.Errorf("%s: the definition hides the hands", id)
				}
				for kind, marks := range map[string]map[string]dsl.OverlayDef{"tint": d.Tints, "badge": d.Badges} {
					for key, o := range marks {
						if o.Color != "" || o.Glyph != "" {
							t.Errorf("%s: %s %s is drawn in the definition (%q %q)", id, kind, key, o.Color, o.Glyph)
						}
					}
				}
			}
			if c := def.Cards; c != nil {
				if c.Back.Color != "" || c.Back.Text != "" {
					t.Errorf("%s: the definition draws the backs of the cards", id)
				}
				for sid, s := range c.Suits {
					if s.Color != "" {
						t.Errorf("%s: suit %s has colour %q", id, sid, s.Color)
					}
				}
			}
		}
	}
}
