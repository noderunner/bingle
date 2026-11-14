package session

import (
	"context"
	"errors"

	"github.com/noderunner/bingle/internal/auth"
)

// ErrBadgesAccount answers a guest who asks to see badges: every registered
// user sees every account's badges, and guests none (ADR 0130).
var ErrBadgesAccount = errors.New("session: badges are seen by accounts: sign in")

// PlayerBadge is one badge an account holds, as others and the holder see it.
type PlayerBadge struct {
	Badge       string `json:"badge"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Family is the game family it was earned in, and Game the game's name.
	Family    string `json:"family,omitempty"`
	Game      string `json:"game,omitempty"`
	AwardedAt string `json:"awarded_at"`
	// Room is the game it was earned in, for the holder alone: a room code
	// lets whoever has it in.
	Room string `json:"room,omitempty"`
	// Hidden says only the holder sees it until its game ends.
	Hidden bool `json:"hidden,omitempty"`
}

// PlayerBadges lists the badges an account holds, oldest first. The holder
// sees them all; another account those shown to everyone; a guest none.
func (m *Manager) PlayerBadges(ctx context.Context, handle string) ([]PlayerBadge, error) {
	p, ok := auth.From(ctx)
	if !ok || p.Account == nil {
		return nil, ErrBadgesAccount
	}
	acc, err := m.activeAccount(ctx, handle)
	if err != nil {
		return nil, err
	}
	mine := p.Account.ID == acc.ID
	awards, err := m.store.Awards(ctx, acc.ID, mine)
	if err != nil {
		return nil, err
	}
	out := make([]PlayerBadge, 0, len(awards))
	for _, a := range awards {
		b := PlayerBadge{
			Badge: a.Badge, Name: a.Name, Description: a.Description,
			Family: a.Family, Game: a.Detail, AwardedAt: a.AwardedAt.UTC().Format("2006-01-02T15:04:05Z"),
			Hidden: !a.Visible,
		}
		if mine {
			b.Room = a.RoomCode
		}
		out = append(out, b)
	}
	return out, nil
}
