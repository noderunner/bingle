package cthulhuwars_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The 3-5 Earth layout as traced from the board: the same borders the game always
// had, the glyphs the board prints, each standing in its own area.
func TestTheEarthMapHasTheBoardsGlyphs(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get("cthulhu-wars-3-5")
	if !ok {
		t.Fatal("no layout")
	}
	board := g.Definition.Board
	if len(board.Positions) != 17 || len(board.Edges) != 44 {
		t.Errorf("%d areas and %d borders, want 17 and 44", len(board.Positions), len(board.Edges))
	}
	want := map[string]string{
		"north-america": "dragon", "south-america": "dragon",
		"europe": "thorns", "scandinavia": "thorns", "north-asia": "thorns", "south-asia": "thorns", "arabia": "thorns",
		"west-africa": "chevron", "east-africa": "chevron", "australia": "chevron",
	}
	marks, _ := g.Definition.Params["glyph_marks"].(map[string]any)
	if len(marks) != len(want) {
		t.Errorf("%d printed glyphs, want %d", len(marks), len(want))
	}
	for _, p := range board.Positions {
		id := string(p.ID)
		got := ""
		for _, tag := range p.Tags {
			if strings.HasPrefix(tag, "glyph-") {
				got = strings.TrimPrefix(tag, "glyph-")
			}
		}
		if got != want[id] {
			t.Errorf("%s has the glyph %q, the board prints %q", id, got, want[id])
		}
		mark, ok := marks[id].([]any)
		if ok != (want[id] != "") {
			t.Errorf("%s: printed at %v", id, marks[id])
		}
		if ok {
			inside := false
			for _, poly := range p.Polygons {
				inside = inside || inPolygon(mark[0].(float64), mark[1].(float64), poly)
			}
			if !inside {
				t.Errorf("%s's glyph stands outside it, at %v", id, mark)
			}
		}
	}
	art, _ := g.Definition.Params["art"].(string)
	info, err := fs.Stat(g.Assets, art)
	if err != nil {
		t.Fatalf("the picture %q: %v", art, err)
	}
	if info.Size() > 250*1024 {
		t.Errorf("the picture is %d KB", info.Size()/1024)
	}
	// A layout not yet traced has no picture.
	old, _ := cat.Get("cthulhu-wars-5p")
	if old.Definition.Params["art"] != nil || old.Definition.Params["glyph_marks"] != nil {
		t.Error("the five-player layout has a picture")
	}
}

// Every Earth layout has the glyphs and the start areas its printed board shows.
func TestEveryEarthMapFollowsItsBoard(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	glyphs := map[string]string{
		"north-america": "dragon", "north-america-west": "dragon", "north-america-east": "dragon", "central-america": "dragon",
		"south-america": "dragon", "south-america-west": "dragon", "south-america-east": "dragon",
		"africa": "chevron", "west-africa": "chevron", "east-africa": "chevron", "australia": "chevron", "new-zealand": "chevron",
		"europe": "thorns", "scandinavia": "thorns", "arabia": "thorns", "asia": "thorns", "north-asia": "thorns", "south-asia": "thorns",
	}
	for id, wantStart := range map[string][2]string{
		"cthulhu-wars":     {"asia", "africa"},
		"cthulhu-wars-3p":  {"asia", "africa"},
		"cthulhu-wars-3-5": {"south-asia", "west-africa"},
		"cthulhu-wars-5p":  {"south-asia", "west-africa"},
	} {
		g, ok := cat.Get(id)
		if !ok {
			t.Fatalf("no layout %s", id)
		}
		for _, p := range g.Definition.Board.Positions {
			got := ""
			for _, tag := range p.Tags {
				if strings.HasPrefix(tag, "glyph-") {
					got = strings.TrimPrefix(tag, "glyph-")
				}
			}
			if got != glyphs[string(p.ID)] {
				t.Errorf("%s: %s has the glyph %q, want %q", id, p.ID, got, glyphs[string(p.ID)])
			}
		}
		starts, _ := g.Definition.Params["starts"].(map[string]any)
		if starts["cc"] != wantStart[0] || starts["bg"] != wantStart[1] {
			t.Errorf("%s starts: %v", id, starts)
		}
	}
}
