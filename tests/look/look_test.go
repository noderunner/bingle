// Package look_test holds every shipped game's web.yaml (ADR 0124) to its
// definitions, and checks look.Check itself on small files.
package look_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/catalog"
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
	// Every game but Cthulhu Wars, whose look moves with it (ADR 0124).
	for _, id := range []string{"nyet", "solitaire", "bowling-solitaire", "blooms", "tak-5x5", "fendo", "fist", "meridians", "hive", "onitama", "yinsh", "courtfall-1.0.0", "mycelium-1.0.0",
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
