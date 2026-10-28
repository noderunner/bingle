package session

import (
	"context"
	"fmt"
	"sort"
)

// The list of games anybody can look in on: every game that has not ended,
// whether it is waiting for players, sitting in a lobby or being played. The
// *Active games* window of the page shows the browser's own games in one tab
// and this list in the other, so that a person can find a game to watch — or
// an open seat to take — without being sent a link.
//
// A room code is all it takes to join a game, so listing the rooms makes every
// unfinished game public to look at and, where a seat is open, to sit in. That
// is the point of the list (ADR 0058); a game that should not be found is one
// nobody should be able to join by code either.

// maxOpenGames bounds the list.
const maxOpenGames = 60

// OpenGame is one unfinished game as the list shows it.
type OpenGame struct {
	RoomCode  string `json:"room_code"`
	GameDefID string `json:"game_def_id"`
	// Name is the game's name from the catalog ("Hex (11×11)"), and Family
	// the id of the game it is a layout or a derived table of.
	Name   string `json:"name"`
	Family string `json:"family,omitempty"`
	// Status is "waiting" or "active"; Lobby says it waits for its host.
	Status string `json:"status"`
	Lobby  bool   `json:"lobby,omitempty"`
	// Seats is how many seats the game has, OpenSeats how many nobody holds.
	Seats     int `json:"seats"`
	OpenSeats int `json:"open_seats"`
	// Players are the seats that are taken, people and bots.
	Players  []SeatInfo `json:"players"`
	TurnSeat *int       `json:"turn_seat"`
	MoveSeq  int        `json:"move_seq"`
	// UpdatedAt is when the game last moved on (a database timestamp).
	UpdatedAt string `json:"updated_at"`
}

// ListOpenGames returns the games that are still going, newest change first:
// the games of this server that anybody may watch, and those with an open seat
// that anybody may join. Games whose definition is no longer registered are
// left out, as nothing could be shown or joined.
func (m *Manager) ListOpenGames(ctx context.Context, limit int) ([]OpenGame, error) {
	if limit <= 0 || limit > maxOpenGames {
		limit = maxOpenGames
	}
	games, err := m.store.ListUnfinishedGames(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}
	out := make([]OpenGame, 0, len(games))
	for _, g := range games {
		compiled, ok := m.catalog.Get(g.GameDefID)
		if !ok {
			continue
		}
		players, err := m.store.ListPlayers(ctx, g.GameID)
		if err != nil {
			return nil, fmt.Errorf("session: %w", err)
		}
		seats := len(compiled.Definition.Seats)
		entry := OpenGame{
			RoomCode:  g.RoomCode,
			GameDefID: g.GameDefID,
			Name:      compiled.Definition.Name,
			Family:    familyOf(compiled.Definition.Family, g.GameDefID),
			Status:    g.Status,
			Lobby:     g.Lobby,
			Seats:     seats,
			OpenSeats: seats - len(players),
			TurnSeat:  g.TurnSeat,
			MoveSeq:   g.MoveSeq,
			UpdatedAt: g.UpdatedAt,
		}
		if entry.OpenSeats < 0 {
			entry.OpenSeats = 0
		}
		for _, p := range players {
			entry.Players = append(entry.Players, SeatInfo{Seat: p.Seat, Color: p.Color, Name: p.Name, Bot: p.Bot})
		}
		sort.Slice(entry.Players, func(i, j int) bool { return entry.Players[i].Seat < entry.Players[j].Seat })
		out = append(out, entry)
	}
	return out, nil
}
