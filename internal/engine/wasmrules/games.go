package wasmrules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
)

// gameOf is a game as rules are told of it: its id, its definition as the
// catalog compiled it and its board as the catalog built it, so that rules
// never compile either.
func gameOf(game *catalog.CompiledGame) (*guest.Game, error) {
	def, err := json.Marshal(game.Definition)
	if err != nil {
		return nil, fmt.Errorf("wasmrules: the definition of %s cannot be written: %w", game.Definition.ID, err)
	}
	b := game.Board
	out := &guest.Game{ID: game.Definition.ID, Definition: def, Board: guest.Board{
		Kind: b.Kind(), Width: b.Width(), Height: b.Height(),
		Positions: make([]guest.Point, 0, b.Len()),
	}}
	for _, id := range b.Positions() {
		p := guest.Point{ID: string(id), Neighbors: ids(b.Neighbors(id))}
		if c, ok := b.Coord(id); ok {
			p.XY = &[2]int{c.X, c.Y}
		}
		out.Board.Positions = append(out.Board.Positions, p)
	}
	if names := b.RegionNames(); len(names) > 0 {
		out.Board.Regions = make(map[string][]string, len(names))
		for _, name := range names {
			region, _ := b.Region(name)
			out.Board.Regions[name] = ids(region)
		}
	}
	return out, nil
}

func ids(in []board.PositionID) []string {
	out := make([]string, len(in))
	for i, id := range in {
		out[i] = string(id)
	}
	return out
}

// toldGame is a game as an instance is told of it: the init request that
// carries it. An instance that was sent this very request knows the game.
type toldGame struct {
	id   string
	init []byte
}

// initRequest is the request of init.
type initRequest struct {
	ABI   int           `json:"abi"`
	Games []*guest.Game `json:"games"`
}

// games are the games a transport has been asked about. A game is learned
// the first time a call names it, which covers a game derived from a pack's
// own long after the pack was loaded. Two compiled copies of one game are
// told as one.
type games struct {
	mu    sync.RWMutex
	known map[*catalog.CompiledGame]*toldGame
	byID  map[string]*toldGame
}

func (g *games) of(game *catalog.CompiledGame) (*toldGame, error) {
	g.mu.RLock()
	told, ok := g.known[game]
	g.mu.RUnlock()
	if ok {
		return told, nil
	}
	gg, err := gameOf(game)
	if err != nil {
		return nil, err
	}
	init, err := json.Marshal(initRequest{ABI: guest.ABI, Games: []*guest.Game{gg}})
	if err != nil {
		return nil, fmt.Errorf("wasmrules: %s cannot be written for init: %w", gg.ID, err)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.known == nil {
		g.known, g.byID = map[*catalog.CompiledGame]*toldGame{}, map[string]*toldGame{}
	}
	told, ok = g.byID[gg.ID]
	if !ok || !bytes.Equal(told.init, init) {
		told = &toldGame{id: gg.ID, init: init}
		g.byID[gg.ID] = told
	}
	g.known[game] = told
	return told, nil
}
