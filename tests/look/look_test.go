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
	if err := cat.RegisterModule(catalog.Module{ID: "g", Definition: []byte(src)}); err != nil {
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
// looks, apart from that of a private game whose look has not moved yet
// (Cthulhu Wars', in a private build, ADR 0127). A piece type's
// own colour stays, since the rules read it (ADR 0117); a zone's layout cannot
// be written at all, since the definition only says whether it is a pile.
func TestNoDefinitionCarriesItsLook(t *testing.T) {
	for _, m := range app.BuiltinModules() {
		if m.ID == "cthulhu-wars" {
			continue
		}
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
