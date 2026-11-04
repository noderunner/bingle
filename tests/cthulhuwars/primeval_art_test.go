package cthulhuwars_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

func inPolygon(x, y float64, poly [][2]float64) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		if (poly[i][1] > y) != (poly[j][1] > y) && x < (poly[j][0]-poly[i][0])*(y-poly[i][1])/(poly[j][1]-poly[i][1])+poly[i][0] {
			in = !in
		}
	}
	return in
}

// The five-player Primeval Map as traced from the board.
func TestThePrimevalMapIsTheBoards(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get("cthulhu-wars-primeval-5p")
	if !ok {
		t.Fatal("no layout")
	}
	board := g.Definition.Board
	if len(board.Positions) != 22 {
		t.Fatalf("%d areas", len(board.Positions))
	}
	edges := map[[2]string]bool{}
	for _, e := range board.Edges {
		edges[[2]string{string(e.From), string(e.To)}] = true
	}
	if len(edges) != 56 {
		t.Errorf("%d borders, want 56", len(edges))
	}
	for _, want := range [][2]string{
		{"rheic-ocean", "upper-mu"}, {"paratethys-ocean", "poseidonis"}, // printed on the board, and missed when it was drawn by eye
		{"iapetus-ocean", "mezcalera-ocean"}, {"mezcalera-ocean", "mirovia-ocean"}, // round the map's edge
		{"atlantis", "paratethys-ocean"}, {"rutas", "tethys-ocean"}, // past the seam between the board's halves
	} {
		if !edges[want] && !edges[[2]string{want[1], want[0]}] {
			t.Errorf("no border between %s and %s", want[0], want[1])
		}
	}
	for _, none := range [][2]string{{"antarctica", "atlantis"}, {"atlantis", "lomar"}, {"lomar", "west-atlantis"}, {"east-lemuria", "thule"}, {"shasta", "thule"}} {
		if edges[none] || edges[[2]string{none[1], none[0]}] {
			t.Errorf("a border between %s and %s", none[0], none[1])
		}
	}

	// Each Glacier glyph stands in an area that has one, and each such area has one.
	marks, _ := g.Definition.Params["ice_glyphs"].([]any)
	if len(marks) != 12 {
		t.Fatalf("%d Glacier glyphs", len(marks))
	}
	found := map[string]int{}
	fills, _ := g.Definition.Params["area_fill"].(map[string]any)
	for _, p := range board.Positions {
		if _, ok := fills[string(p.ID)]; !ok {
			t.Errorf("%s has no colour", p.ID)
		}
		for _, poly := range p.Polygons {
			for _, pt := range poly {
				if pt[0] < 0 || pt[0] > 1000 || pt[1] < 0 || pt[1] > 500 {
					t.Errorf("%s has a point off the map: %v", p.ID, pt)
				}
			}
			for _, m := range marks {
				xy := m.([]any)
				if inPolygon(xy[0].(float64), xy[1].(float64), poly) {
					found[string(p.ID)]++
				}
			}
		}
		glacier := false
		for _, tag := range p.Tags {
			glacier = glacier || tag == "glacier"
		}
		if glacier != (found[string(p.ID)] == 1) {
			t.Errorf("%s: tagged %v, %d glyphs", p.ID, glacier, found[string(p.ID)])
		}
	}

	// The picture is in the build, and small enough to stay there.
	art, _ := g.Definition.Params["art"].(string)
	info, err := fs.Stat(g.Assets, art)
	if err != nil {
		t.Fatalf("the picture %q: %v", art, err)
	}
	if info.Size() > 200*1024 {
		t.Errorf("the picture is %d KB", info.Size()/1024)
	}
}

// The other three Primeval boards, each built from a three-player or five-player half on
// each side, have the areas and Glacier glyphs their print shows.
func TestThePrimevalBoardsAreTraced(t *testing.T) {
	for _, tc := range []struct {
		id           string
		areas, edges int
		glaciers     []string
		none         [][2]string
		borders      [][2]string
	}{
		{
			"cthulhu-wars-primeval-3p", 14, 33,
			[]string{"antarctica", "hyperborea", "iapetus-ocean", "lower-mu", "mirovia-ocean", "paratethys-ocean", "poseidonis", "terra-australis", "upper-mu"},
			nil,
			[][2]string{{"rheic-ocean", "upper-mu"}, {"atlantis", "terra-australis"}},
		},
		{
			"cthulhu-wars-primeval-3-5", 18, 44,
			[]string{"antarctica", "hyperborea", "iapetus-ocean", "lomar", "lower-mu", "mirovia-ocean", "paratethys-ocean", "poseidonis", "rheic-ocean", "terra-australis", "upper-mu"},
			[][2]string{{"atlantis", "mirovia-ocean"}, {"atlantis", "lomar"}},
			[][2]string{{"atlantis", "ocean-trench"}, {"atlantis", "rutas"}, {"lemuria", "tethys-ocean"}},
		},
		{
			"cthulhu-wars-primeval-5-3", 18, 45,
			[]string{"antarctica", "hyperborea", "iapetus-ocean", "lower-mu", "mirovia-ocean", "paratethys-ocean", "poseidonis", "south-lemuria", "terra-australis", "upper-mu"},
			[][2]string{{"atlantis", "mezcalera-ocean"}, {"shasta", "thule"}},
			[][2]string{{"atlantis", "west-atlantis"}, {"atlantis", "terra-australis"}, {"west-atlantis", "tethys-ocean"}},
		},
	} {
		t.Run(tc.id, func(t *testing.T) {
			g := variant(t, tc.id)
			board := g.Definition.Board
			if len(board.Positions) != tc.areas || len(board.Edges) != tc.edges {
				t.Errorf("%d areas and %d borders, want %d and %d", len(board.Positions), len(board.Edges), tc.areas, tc.edges)
			}
			edges := map[[2]string]bool{}
			for _, e := range board.Edges {
				edges[[2]string{string(e.From), string(e.To)}] = true
				edges[[2]string{string(e.To), string(e.From)}] = true
			}
			for _, want := range tc.borders {
				if !edges[want] {
					t.Errorf("no border between %s and %s", want[0], want[1])
				}
			}
			for _, none := range tc.none {
				if edges[none] {
					t.Errorf("a border between %s and %s", none[0], none[1])
				}
			}
			var tagged []string
			marks, _ := g.Definition.Params["ice_glyphs"].([]any)
			for _, p := range board.Positions {
				glacier := false
				for _, tag := range p.Tags {
					glacier = glacier || tag == "glacier"
				}
				if glacier {
					tagged = append(tagged, string(p.ID))
				}
				found := 0
				for _, poly := range p.Polygons {
					for _, m := range marks {
						xy := m.([]any)
						if inPolygon(xy[0].(float64), xy[1].(float64), poly) {
							found++
						}
					}
				}
				if glacier != (found == 1) {
					t.Errorf("%s: tagged %v, %d glyphs", p.ID, glacier, found)
				}
			}
			if strings.Join(tagged, " ") != strings.Join(tc.glaciers, " ") {
				t.Errorf("glaciers on %v, want %v", tagged, tc.glaciers)
			}
			if len(marks) != len(tc.glaciers) {
				t.Errorf("%d glyphs for %d glaciers", len(marks), len(tc.glaciers))
			}
			// Earth prints its glyphs in places of its own, which must not follow this map.
			if g.Definition.Params["glyph_marks"] != nil {
				t.Errorf("the map inherited Earth's glyph places")
			}
			art, _ := g.Definition.Params["art"].(string)
			info, err := fs.Stat(g.Assets, art)
			if err != nil {
				t.Fatalf("the picture %q: %v", art, err)
			}
			if info.Size() > 200*1024 {
				t.Errorf("the picture is %d KB", info.Size()/1024)
			}
		})
	}
}

// East Atlantis has no glyph on the 5-3 board, and has one on the 3-5, where Atlantis is whole.
func TestAtlantisGlyphFollowsItsBoard(t *testing.T) {
	for id, want := range map[string]bool{"cthulhu-wars-primeval-3-5": true, "cthulhu-wars-primeval-5-3": false} {
		for _, p := range variant(t, id).Definition.Board.Positions {
			if p.ID != "atlantis" {
				continue
			}
			got := false
			for _, tag := range p.Tags {
				got = got || tag == "glyph-thorns"
			}
			if got != want {
				t.Errorf("%s: Atlantis has a glyph: %v", id, got)
			}
		}
	}
}

// The two four-player Primeval maps play to the end, with four cults.
func TestTheFourPlayerPrimevalMapsPlayToTheEnd(t *testing.T) {
	for _, id := range []string{"cthulhu-wars-primeval-3-5", "cthulhu-wars-primeval-5-3"} {
		g := variant(t, id)
		if n := len(g.Definition.Seats); n != 4 {
			t.Fatalf("%s: %d seats", id, n)
		}
		if _, _, done := randomGame(t, g, 1, 4000, nil); !done {
			t.Errorf("%s did not finish", id)
		}
	}
}
