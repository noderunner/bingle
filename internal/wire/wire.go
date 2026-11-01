// Package wire holds the JSON shapes that the REST API and the MCP server
// both send, so a move or a game definition looks the same to every client
// and is defined once.
package wire

import "github.com/noderunner/bingle/internal/session"

// Move is one move as clients see it: the "move" object of a make-move
// response, an entry of the move history, and the last move of a game.
type Move struct {
	Seq      int     `json:"seq"`
	Seat     int     `json:"seat"`
	Color    string  `json:"color"`
	From     string  `json:"from"`
	To       string  `json:"to"`
	Captured *string `json:"captured"`

	Action   string   `json:"action,omitempty"`
	Piece    string   `json:"piece,omitempty"`
	Count    int      `json:"count,omitempty"`
	Path     []string `json:"path,omitempty"`
	Drops    []int    `json:"drops,omitempty"`
	Aim      string   `json:"aim,omitempty"`
	Notation string   `json:"notation,omitempty"`

	CreatedAt string `json:"created_at,omitempty"`
	// Auto: the server played the move for a person (a reaction they chose
	// to pass automatically). AutoPlay: a tactical bot played it for a person
	// whose Auto-Play Timeout ran out. Locked: the move rolled dice or drew a
	// hidden token, so nothing can be taken back across it.
	Auto     bool `json:"auto,omitempty"`
	AutoPlay bool `json:"auto_play,omitempty"`
	Locked   bool `json:"locked,omitempty"`
}

// MoveFromOutcome is the Move of a move that was just played.
func MoveFromOutcome(o session.MoveOutcome) Move {
	return Move{
		Seq: o.Seq, Seat: o.Seat, Color: o.Color, From: o.From, To: o.To, Captured: o.Captured,
		Action: o.Action, Piece: o.Piece, Count: o.Count, Path: o.Path, Drops: o.Drops, Aim: o.Aim, Notation: o.Notation,
		Auto: o.Auto, AutoPlay: o.AutoPlay, Locked: o.Locked,
	}
}

// MovesFromOutcomes is MoveFromOutcome for each of a list (nil for none).
func MovesFromOutcomes(os []session.MoveOutcome) []Move {
	if len(os) == 0 {
		return nil
	}
	out := make([]Move, len(os))
	for i, o := range os {
		out[i] = MoveFromOutcome(o)
	}
	return out
}

// MoveFromRecord is the Move of a move read back from the game's history.
func MoveFromRecord(rec session.MoveRecord) Move {
	return Move{
		Seq: rec.Seq, Seat: rec.Seat, Color: rec.Color, From: rec.From, To: rec.To, Captured: rec.Captured,
		Action: rec.Action, Piece: rec.Piece, Count: rec.Count, Path: rec.Path, Drops: rec.Drops, Aim: rec.Aim, Notation: rec.Notation,
		CreatedAt: rec.CreatedAt, Auto: rec.Auto, AutoPlay: rec.AutoPlay, Locked: rec.Locked,
	}
}

// GameDef is one entry of the list of games.
type GameDef struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	SeatsRequired int    `json:"seats_required"`
	Description   string `json:"description,omitempty"`
	BoardWidth    int    `json:"board_width,omitempty"`
	BoardHeight   int    `json:"board_height,omitempty"`
	Family        string `json:"family,omitempty"`
	// Version is the game implementation's semantic version; a version below
	// 1.0.0 is a beta.
	Version   string   `json:"version,omitempty"`
	Designers []string `json:"designers,omitempty"`
	Publisher string   `json:"publisher,omitempty"`
	// Tutorials is how many tutorials the game's family has (see
	// GET /game-defs/{id}/tutorials).
	Tutorials int `json:"tutorials,omitempty"`
}

// GameDefs converts the manager's list of games.
func GameDefs(defs []session.GameDefSummary) []GameDef {
	out := make([]GameDef, len(defs))
	for i, d := range defs {
		out[i] = GameDef{
			ID: d.ID, Name: d.Name, SeatsRequired: d.SeatsRequired, Description: d.Description,
			BoardWidth: d.BoardWidth, BoardHeight: d.BoardHeight, Family: d.Family,
			Version: d.Version, Designers: d.Designers, Publisher: d.Publisher, Tutorials: d.Tutorials,
		}
	}
	return out
}
