package session

import (
	"context"
	"errors"
	"sort"

	"github.com/noderunner/bingle/internal/badges"
	"github.com/noderunner/bingle/internal/engine/dsl"

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

// Badge is one badge there is to earn, as the catalog declares it: what it is
// for, and not who holds it.
type Badge struct {
	Badge       string `json:"badge"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Platform says the badge is the platform's, not one game's.
	Platform bool `json:"platform,omitempty"`
	// Family is the game family it is earned in, a game's badge's or a
	// platform badge's that names one, and Game that family's name.
	Family string `json:"family,omitempty"`
	Game   string `json:"game,omitempty"`
	// Kind is "player" (games against other people), "rated" (rated games
	// only) or "bot" (a game against bots), and Bot the weakest bot that will
	// do, by its label.
	Kind string `json:"kind"`
	Bot  string `json:"bot,omitempty"`
	// When is "during" (awarded as soon as it holds) or "end".
	When string `json:"when"`
}

// Badges is every badge there is to earn: the platform's first, as its file
// lists them, then each family's in the order of its name and its definition.
// A badge declared by two versions of a family is one badge, and listed once.
// What a badge is for is no secret, so anybody may read the list.
func (m *Manager) Badges() []Badge {
	labels := map[string]string{}
	for _, b := range m.bots.List() {
		labels[b.ID] = b.Label
	}
	info := func(id string, def dsl.BadgeDef) Badge {
		b := Badge{Badge: id, Name: def.Name, Description: def.Description, Kind: "player", When: def.When}
		switch {
		case def.Bot != nil:
			b.Kind, b.Bot = "bot", labels[def.Bot.AtLeast]
			if b.Bot == "" {
				b.Bot = def.Bot.AtLeast
			}
		case def.Rated:
			b.Kind = "rated"
		}
		return b
	}
	out := []Badge{}
	for _, def := range platformBadges() {
		b := info(def.ID, def)
		b.Platform = true
		if def.Game != nil && def.Game.Family != "" {
			b.Family = def.Game.Family
			if base, ok := m.catalog.Get(def.Game.Family); ok {
				b.Game = m.familyName(base)
			}
		}
		out = append(out, b)
	}
	var games []Badge
	seen := map[string]bool{}
	for _, compiled := range m.catalog.List() {
		family := gameFamily(compiled)
		for _, def := range compiled.Definition.Badges {
			id := badges.GameID(family, def.ID)
			if seen[id] {
				continue
			}
			seen[id] = true
			b := info(id, def)
			b.Family, b.Game = family, m.familyName(compiled)
			games = append(games, b)
		}
	}
	// The catalog lists games by id; a family's badges keep their order.
	sort.SliceStable(games, func(i, j int) bool { return games[i].Game < games[j].Game })
	return append(out, games...)
}
