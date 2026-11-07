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
func families(t *testing.T) map[string]struct {
	web  []byte
	defs map[string]*dsl.GameDefinition
} {
	t.Helper()
	cat := catalog.NewCatalog()
	out := map[string]struct {
		web  []byte
		defs map[string]*dsl.GameDefinition
	}{}
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatalf("RegisterModule(%s): %v", m.ID, err)
		}
		if m.Web != nil {
			out[m.ID] = struct {
				web  []byte
				defs map[string]*dsl.GameDefinition
			}{m.Web, map[string]*dsl.GameDefinition{}}
		}
	}
	for _, cg := range cat.List() {
		if f, ok := out[cg.Definition.Family]; ok {
			f.defs[cg.Definition.ID] = cg.Definition
		}
	}
	return out
}

// Every shipped look names only its own games' seats, pieces, tints, badges,
// zones, suits and variants, and previews only pieces in played colours.
func TestEveryLookFitsItsGame(t *testing.T) {
	fams := families(t)
	if _, ok := fams["blooms"]; !ok {
		t.Fatal("Blooms brings no look")
	}
	for id, f := range fams {
		if err := look.Check(f.web, id, f.defs); err != nil {
			t.Error(err)
		}
	}
}

// Check catches what a look gets wrong.
func TestCheckCatchesAMistake(t *testing.T) {
	def := &dsl.GameDefinition{
		ID:      "g",
		Seats:   []dsl.SeatDef{{Seat: 1, Color: "red"}, {Seat: 2, Color: "blue"}},
		Pieces:  map[string]dsl.PieceDef{"stone": {}},
		Display: &dsl.DisplayDef{Tints: map[string]dsl.OverlayDef{"hot": {Label: "Hot"}}},
	}
	defs := map[string]*dsl.GameDefinition{"g": def, "g-big": def}
	for file, want := range map[string]string{
		`{"seats":{"1":{"blend":["red","blue"]}},"pieces":{"stone":{"shape":"ring"}},"display":{"tints":{"hot":{"color":"#f00"}}},"variants":{"g-big":{"board":{"style":"points"}}}}`: "",
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
	} {
		err := look.Check([]byte(file), "g", defs)
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
