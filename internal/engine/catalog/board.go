package catalog

import (
	"fmt"
	"sort"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

// resolveBoard compiles def.Board into a *board.Graph, using the generator
// path (currently only "grid") if def.Board.Generator is set, or the
// explicit positions/edges path otherwise.
func resolveBoard(def *dsl.GameDefinition) (*board.Graph, error) {
	var g *board.Graph

	if def.Board.Generator != nil {
		gen := def.Board.Generator
		var err error
		switch gen.Type {
		case "grid":
			g, err = board.GenerateGrid(board.GridOptions{
				Width:  gen.Width,
				Height: gen.Height,
				Mode:   gen.Adjacency,
				Labels: board.Labeling(gen.Labels),
			})
		case "hex":
			g, err = board.GenerateHex(gen.Width, gen.Height)
		case "hexhex":
			if gen.Size2 > 0 && gen.Size2 != gen.Size {
				g, err = board.GenerateHexHexSides(gen.Size, gen.Size2)
			} else {
				g, err = board.GenerateHexHex(gen.Size)
			}
		default:
			return nil, fmt.Errorf("catalog: unsupported board generator type %q", gen.Type)
		}
		if err != nil {
			return nil, fmt.Errorf("catalog: %w", err)
		}
	} else {
		if len(def.Board.Positions) == 0 {
			return nil, fmt.Errorf("catalog: board has neither a generator nor positions")
		}

		positions := make([]board.PositionID, len(def.Board.Positions))
		for i, pos := range def.Board.Positions {
			positions[i] = board.PositionID(pos.ID)
		}

		edges := make([][2]board.PositionID, len(def.Board.Edges))
		for i, edge := range def.Board.Edges {
			edges[i] = [2]board.PositionID{board.PositionID(edge.From), board.PositionID(edge.To)}
		}

		var err error
		g, err = board.FromExplicit(positions, edges)
		if err != nil {
			return nil, fmt.Errorf("catalog: %w", err)
		}
	}

	switch def.Board.Style {
	case "", "squares", "points", "free", "none":
	case "areas":
		if def.Board.Canvas == nil || def.Board.Canvas.Width <= 0 || def.Board.Canvas.Height <= 0 {
			return nil, fmt.Errorf("catalog: an area map needs a canvas with a positive width and height")
		}
		for _, p := range def.Board.Positions {
			if len(p.Polygons) == 0 {
				return nil, fmt.Errorf("catalog: area %q has no polygon", p.ID)
			}
			for _, poly := range p.Polygons {
				if len(poly) < 3 {
					return nil, fmt.Errorf("catalog: area %q has a polygon with fewer than three corners", p.ID)
				}
			}
		}
	default:
		return nil, fmt.Errorf("catalog: unsupported board style %q", def.Board.Style)
	}

	for region := range def.Board.Borders {
		if _, ok := g.Region(region); !ok && def.Board.Regions[region] == nil {
			return nil, fmt.Errorf("catalog: board border references unknown region %q", region)
		}
	}

	names := make([]string, 0, len(def.Board.Regions))
	for name := range def.Board.Regions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ids := make([]board.PositionID, len(def.Board.Regions[name]))
		for i, p := range def.Board.Regions[name] {
			ids[i] = board.PositionID(p)
		}
		if !g.DefineRegion(name, ids) {
			return nil, fmt.Errorf("catalog: board region %q references an unknown position", name)
		}
	}

	return g, nil
}
