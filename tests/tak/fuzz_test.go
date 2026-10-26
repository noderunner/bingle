package tak_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
)

// The inputs below arrive from untrusted clients (browsers, AI agents), so
// none of them may ever panic, whatever they contain.

func FuzzParseMove(f *testing.F) {
	for _, seed := range []string{
		"a1", "Sa1", "Ca1", "3c3>111", "c3+", "2d2<", "", " ", "zz9", "0a1+", "99a1>", "a1>>>>", "3c3>000", "Fa1*", "c3-!?", "ä1", "1a1+1111111111",
	} {
		f.Add(seed)
	}
	g := loadGame(f, "tak-5x5")
	f.Fuzz(func(t *testing.T, text string) {
		mv, err := state.ParseMove(g, text)
		if err != nil {
			return
		}
		// Whatever parsed must also be formattable and applicable (or
		// rejected) without panicking.
		_ = state.FormatMove(g, mv)
		_, _ = state.Apply(g, state.InitialState(g), 1, mv)
	})
}

func FuzzParseTPS(f *testing.F) {
	for _, seed := range []string{
		"x5/x5/x5/x5/x5 1 1", "x3,12,2S/x,22221,1/x5/x5/x5 1 7", "", "x/x/x 1 1", "1,2,3 1 1", "x5/x5/x5/x5/x5 3 1", "x99/x5/x5/x5/x5 1 1", "22222222222222222222222222,x4/x5/x5/x5/x5 2 5",
	} {
		f.Add(seed)
	}
	g := loadGame(f, "tak-5x5")
	f.Fuzz(func(t *testing.T, text string) {
		st, seat, err := state.ParseTPS(g, text)
		if err != nil {
			return
		}
		_, _ = state.FormatTPS(g, st, seat)
		_ = state.LegalMoves(g, st, seat)
	})
}

// FuzzApplyStructuredMove feeds arbitrary structured moves, as the REST API
// and the MCP tool accept them.
func FuzzApplyStructuredMove(f *testing.F) {
	f.Add("move", "", "c3", "", 2, "d3", 1, "e3", 1)
	f.Add("place", "wall", "", "c3", 0, "", 0, "", 0)
	f.Add("move", "", "a1", "", -1, "zz", -5, "a2", 1<<30)
	f.Add("", "", "", "", 0, "", 0, "", 0)
	g := loadGame(f, "tak-5x5")
	st, _, err := state.ParseTPS(g, "x5/x2,1S,x2/x,12,121C,x2/x5/x5 1 6")
	if err != nil {
		f.Fatalf("seed position: %v", err)
	}
	f.Fuzz(func(t *testing.T, action, piece, from, to string, count int, p1 string, d1 int, p2 string, d2 int) {
		mv := state.Move{
			Action: action, Piece: piece,
			From: board.PositionID(from), To: board.PositionID(to), Count: count,
		}
		if p1 != "" {
			mv.Path = append(mv.Path, board.PositionID(p1))
			mv.Drops = append(mv.Drops, d1)
		}
		if p2 != "" {
			mv.Path = append(mv.Path, board.PositionID(p2))
			mv.Drops = append(mv.Drops, d2)
		}
		for seat := 1; seat <= 2; seat++ {
			_, _ = state.Apply(g, st, seat, mv)
		}
	})
}

// FuzzRegisterDefinition throws arbitrary JSON at the catalog compiler.
func FuzzRegisterDefinition(f *testing.F) {
	f.Add([]byte(`{"id":"x","name":"X"}`))
	f.Add([]byte(`{"id":"x","name":"X","board":{"generator":{"type":"grid","width":-1,"height":0,"adjacency":"orthogonal4"}},"seats":[{"seat":1,"color":"a"}],"pieces":{"p":{}},"turn_order":{"type":"round-robin","seats":[]},"win_condition":{"type":"rules","rules":[{"type":"connect"}]}}`))
	f.Add([]byte(`{"id":"x","name":"X","board":{"generator":{"type":"hex","width":3,"height":3}},"seats":[{"seat":1,"color":"a","forward":"N"}],"pieces":{"p":{"movement":{"primitive":"step","occupancy":"capture-only","directions":["forward"]}}},"turn_order":{"type":"round-robin","seats":[1]},"actions":[{"id":"j","type":"jump"}],"win_condition":{"type":"none"}}`))
	f.Add([]byte(`not json`))
	f.Add([]byte(`{"variants":[{"id":"v","patch":null}],"id":"b","name":"B"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		cat := catalog.NewCatalog()
		_ = cat.Register("fuzz", data, nil)
		_, _ = dsl.Parse(data)
		_, _ = dsl.ExpandVariants(data)
	})
}
