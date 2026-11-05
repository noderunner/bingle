package session

import (
	"context"
	"fmt"
	"sort"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/store"
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
	// Tags are the short badges the game's rules give it in a list.
	Tags []string `json:"tags,omitempty"`
	// FriendsOnly says its seats are all held or kept for friends; ReservedForYou that one is the
	// viewer's. OpenSeats counts only the seats the viewer could sit at.
	FriendsOnly    bool `json:"friends_only,omitempty"`
	ReservedForYou bool `json:"reserved_for_you,omitempty"`
	// Status is "waiting" or "active"; Lobby says it waits for its host.
	Status string `json:"status"`
	Lobby  bool   `json:"lobby,omitempty"`
	Rated  bool   `json:"rated,omitempty"`
	// Seats is how many seats the game has, OpenSeats how many nobody holds.
	Seats     int `json:"seats"`
	OpenSeats int `json:"open_seats"`
	// Players are the seats that are taken, people and bots.
	Players     []SeatInfo `json:"players"`
	ImpulseSeat *int       `json:"impulse_seat"`
	MoveSeq     int        `json:"move_seq"`
	// UpdatedAt is when the game last moved on (a database timestamp).
	UpdatedAt string `json:"updated_at"`
}

// OpenGameFilter narrows the list: to one family of games, or to those that have
// not started and have a seat for somebody.
type OpenGameFilter struct {
	Family    string
	OpenSeats bool
	// Viewer is the account asking, 0 for anybody else. It decides which games are found (ADR 0107).
	Viewer int64
}

// openGamesScanned is how many unfinished games a filtered list looks through.
const openGamesScanned = 1000

// listLabel is what a list of games calls a game: its name, unless its rules say otherwise.
func listLabel(compiled *catalog.CompiledGame) (string, []string) {
	if l, ok := compiled.Rules.(state.ListLabeler); ok {
		return l.ListLabel(compiled)
	}
	return compiled.Definition.Title(), nil
}

// GameListLabel is the title and badges a game's rules give it in a list, if they give it any.
func (m *Manager) GameListLabel(id string) (string, []string) {
	compiled, ok := m.catalog.Get(id)
	if !ok {
		return "", nil
	}
	if l, ok := compiled.Rules.(state.ListLabeler); ok {
		return l.ListLabel(compiled)
	}
	return "", nil
}

// ListOpenGames returns the games that are still going, newest change first:
// the games of this server that anybody may watch, and those with an open seat
// that anybody may join. Games whose definition is no longer registered are
// left out, as nothing could be shown or joined.
func (m *Manager) ListOpenGames(ctx context.Context, limit int, filter OpenGameFilter) ([]OpenGame, error) {
	if limit <= 0 || limit > maxOpenGames {
		limit = maxOpenGames
	}
	// Some games are not for everybody, so the list cannot stop at the first few.
	scan := openGamesScanned
	friends := func() map[int64]bool {
		out := map[int64]bool{}
		rows, err := m.store.Friendships(ctx, filter.Viewer)
		if err != nil {
			return out
		}
		for _, f := range rows {
			if f.Status == store.FriendAccepted {
				out[f.OtherID] = true
			}
		}
		return out
	}
	var known map[int64]bool
	viewerFriends := func() map[int64]bool {
		if known == nil {
			known = friends()
		}
		return known
	}
	games, err := m.store.ListUnfinishedGames(ctx, scan)
	if err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}
	out := make([]OpenGame, 0, len(games))
	for _, g := range games {
		compiled, ok := m.catalog.Get(g.GameDefID)
		if !ok {
			continue
		}
		if filter.Family != "" && familyOf(compiled.Definition.Family, g.GameDefID) != filter.Family {
			continue
		}
		if filter.OpenSeats && g.Status != "waiting" {
			continue
		}
		if len(out) == limit {
			break
		}
		players, err := m.store.ListPlayers(ctx, g.GameID)
		if err != nil {
			return nil, fmt.Errorf("session: %w", err)
		}
		seats := len(compiled.Definition.Seats)
		seatNumbers := make([]int, 0, seats)
		for _, sd := range compiled.Definition.Seats {
			seatNumbers = append(seatNumbers, sd.Seat)
		}
		access, err := m.listedAccess(ctx, g.GameID, players, seatNumbers, filter.Viewer, viewerFriends)
		if err != nil {
			return nil, fmt.Errorf("session: %w", err)
		}
		if !access.Show {
			continue
		}
		title, tags := listLabel(compiled)
		if access.FriendsOnly {
			tags = append([]string{"Friends only"}, tags...)
		}
		if access.Yours {
			tags = append([]string{"Seat reserved for you"}, tags...)
		}
		entry := OpenGame{
			RoomCode:    g.RoomCode,
			GameDefID:   g.GameDefID,
			Name:        title,
			Tags:        tags,
			Family:      familyOf(compiled.Definition.Family, g.GameDefID),
			Status:      g.Status,
			Lobby:       g.Lobby,
			Rated:       g.Rated,
			Seats:       seats,
			OpenSeats:   access.Open,
			FriendsOnly: access.FriendsOnly,
			ImpulseSeat: g.ImpulseSeat,
			MoveSeq:     g.MoveSeq,
			UpdatedAt:   g.UpdatedAt,
		}
		entry.ReservedForYou = access.Yours
		for _, p := range players {
			entry.Players = append(entry.Players, SeatInfo{Seat: p.Seat, Color: p.Color, Name: p.Name, Bot: p.Bot})
		}
		sort.Slice(entry.Players, func(i, j int) bool { return entry.Players[i].Seat < entry.Players[j].Seat })
		// A friend's friends-only game is offered even with no seat for them: they can watch.
		if filter.OpenSeats && entry.OpenSeats == 0 && !entry.FriendsOnly {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}
