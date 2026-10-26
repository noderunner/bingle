package envs

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Spec describes a game to a model builder: the board geometry, what the
// observation planes mean and the size of the fixed action space — what you
// need to size a network before playing a single move.
type Spec struct {
	GameDefID string         `json:"game_def_id"`
	Name      string         `json:"name"`
	Notation  string         `json:"notation"`
	Seats     []SeatSpec     `json:"seats"`
	Board     BoardSpec      `json:"board"`
	Pieces    []PieceSpec    `json:"pieces"`
	Reserves  map[string]int `json:"reserves,omitempty"`

	// Planes is absent for boards without grid geometry.
	Planes  *PlanesSpec `json:"planes,omitempty"`
	Actions ActionsSpec `json:"actions"`
	Rewards string      `json:"rewards"`
}

// SeatSpec is one seat.
type SeatSpec struct {
	Seat    int    `json:"seat"`
	Color   string `json:"color"`
	Label   string `json:"label,omitempty"`
	Forward string `json:"forward,omitempty"`
}

// BoardSpec is the board geometry.
type BoardSpec struct {
	Kind   string `json:"kind"` // grid, hex or explicit
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// Cells lists the position ID at each plane cell, row-major (y*width+x).
	Cells []string `json:"cells,omitempty"`
}

// PieceSpec is one piece type.
type PieceSpec struct {
	ID    string   `json:"id"`
	Code  string   `json:"code,omitempty"`
	Label string   `json:"label,omitempty"`
	Pool  string   `json:"pool,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}

// PlanesSpec describes the observation tensor.
type PlanesSpec struct {
	Shape    [3]int   `json:"shape"` // channels, height, width
	Depth    int      `json:"depth"`
	Channels []string `json:"channels"`
	Layout   string   `json:"layout"`
}

// ActionsSpec describes the action space.
type ActionsSpec struct {
	// Fixed is true when every move has a fixed integer ID (0..Count-1).
	Fixed bool `json:"fixed"`
	Count int  `json:"count,omitempty"`
	// Moves lists the notation of every action by ID (on request).
	Moves []string `json:"moves,omitempty"`
	Note  string   `json:"note"`
}

// Spec returns the specification of game gameID. includeMoves adds the full
// action list (large for big Tak boards).
func (m *Manager) Spec(gameID string, includeMoves bool, depth int) (Spec, error) {
	game, ok := m.catalog.Get(gameID)
	if !ok {
		return Spec{}, fmt.Errorf("%w: %q", ErrUnknownGame, gameID)
	}
	return BuildSpec(game, includeMoves, depth), nil
}

// BuildSpec builds the Spec of a compiled game.
func BuildSpec(game *catalog.CompiledGame, includeMoves bool, depth int) Spec {
	def := game.Definition
	sp := Spec{
		GameDefID: def.ID, Name: def.Name, Notation: def.Notation,
		Reserves: def.Reserves,
		Rewards:  "+1 for a win, -1 for a loss and 0 for a draw or a truncated episode, from the agent's seat; 0 until the episode ends",
	}
	if sp.Notation == "" {
		sp.Notation = "generic"
	}
	for _, s := range def.Seats {
		sp.Seats = append(sp.Seats, SeatSpec{Seat: s.Seat, Color: s.Color, Label: s.Label, Forward: s.Forward})
	}
	for _, id := range game.PieceTypes {
		pd := def.Pieces[id]
		sp.Pieces = append(sp.Pieces, PieceSpec{ID: id, Code: pd.Code, Label: pd.Label, Pool: game.Pool(id), Tags: pd.Tags})
	}
	g := game.Board
	sp.Board = BoardSpec{Kind: g.Kind(), Width: g.Width(), Height: g.Height()}
	if names, h, w, err := state.PlaneChannels(game, state.PlaneOptions{Depth: depth}); err == nil {
		d := depth
		if d <= 0 {
			d = 1
			for _, a := range game.Actions {
				if a.Type == catalog.ActionMoveStack {
					d = 4
				}
			}
		}
		sp.Planes = &PlanesSpec{
			Shape: [3]int{len(names), h, w}, Depth: d, Channels: names,
			Layout: "row-major [channel][y][x]; channels are relative to the agent (me / opp); y = 0 is the top row of the board as drawn",
		}
		if cells, err := state.PlaneCells(game); err == nil {
			sp.Board.Cells = cells
		}
	}
	as := state.ActionSpaceOf(game)
	sp.Actions = ActionsSpec{Fixed: as.Fixed}
	if as.Fixed {
		sp.Actions.Count = len(as.Moves)
		sp.Actions.Note = "action IDs index the full move list; an observation's legal_action_ids says which are legal now (mask the rest)"
		if includeMoves {
			sp.Actions.Moves = as.Moves
		}
	} else {
		sp.Actions.Note = "this game's moves (chained jumps) cannot be enumerated in advance; choose from the observation's legal_moves list"
	}
	return sp
}
