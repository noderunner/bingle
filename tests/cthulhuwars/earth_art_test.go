package cthulhuwars_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// Every Earth layout as traced from its board: the borders it prints, the glyphs it
// prints, each standing in its own area, and a picture small enough to stay in the build.
func TestTheEarthMapsHaveTheBoardsGlyphs(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	const (
		drag = "dragon"
		chev = "chevron"
		thor = "thorns"
	)
	left3 := map[string]string{"north-america": drag, "south-america": drag, "australia": chev}
	left5 := map[string]string{
		"north-america-west": drag, "north-america-east": drag, "central-america": drag,
		"south-america-west": drag, "south-america-east": drag, "australia": chev, "new-zealand": chev,
	}
	right3 := map[string]string{"europe": thor, "asia": thor, "africa": chev}
	right5 := map[string]string{
		"europe": thor, "scandinavia": thor, "north-asia": thor, "south-asia": thor, "arabia": thor,
		"west-africa": chev, "east-africa": chev,
	}
	for _, tc := range []struct {
		id           string
		areas, edges int
		left, right  map[string]string
	}{
		{"cthulhu-wars-3p", 13, 32, left3, right3},
		{"cthulhu-wars-3-5", 17, 44, left3, right5},
		{"cthulhu-wars", 17, 43, left5, right3},
		{"cthulhu-wars-5p", 21, 55, left5, right5},
	} {
		t.Run(tc.id, func(t *testing.T) {
			g, ok := cat.Get(tc.id)
			if !ok {
				t.Fatal("no layout")
			}
			board := g.Definition.Board
			if len(board.Positions) != tc.areas || len(board.Edges) != tc.edges {
				t.Errorf("%d areas and %d borders, want %d and %d", len(board.Positions), len(board.Edges), tc.areas, tc.edges)
			}
			want := map[string]string{}
			for _, half := range []map[string]string{tc.left, tc.right} {
				for id, glyph := range half {
					want[id] = glyph
				}
			}
			marks, _ := g.Definition.Params["glyph_marks"].(map[string]any)
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
		})
	}
}

// Central America runs on along the north of South America, as the board prints it,
// with no line across the isthmus. The two layouts with that left half agree.
func TestTheSplitAmericasFollowTheBoard(t *testing.T) {
	for _, id := range []string{"cthulhu-wars", "cthulhu-wars-5p"} {
		edges := map[[2]string]bool{}
		for _, e := range variant(t, id).Definition.Board.Edges {
			edges[[2]string{string(e.From), string(e.To)}] = true
			edges[[2]string{string(e.To), string(e.From)}] = true
		}
		for _, want := range [][2]string{{"central-america", "south-america-east"}, {"central-america", "south-america-west"}, {"south-america-west", "south-atlantic"}} {
			if !edges[want] {
				t.Errorf("%s: no border between %s and %s", id, want[0], want[1])
			}
		}
		if edges[[2]string{"north-atlantic", "south-america-west"}] {
			t.Errorf("%s: South America West reaches the North Atlantic", id)
		}
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
