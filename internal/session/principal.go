package session

import (
	"context"
	"errors"
	"strings"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/store"
)

// ErrNotYourSeat is the answer to a request that names a seat owned by
// another principal (ADR 0067): a name is how a request says which seat it
// means, never proof that the seat is its own.
var ErrNotYourSeat = errors.New("session: that seat is somebody else's (it was taken in another browser)")

// mayActFor reports whether the request's principal may act for the person
// at seat p, and binds a seat nobody owns yet to it.
//
//   - A seat owned by a principal is that principal's alone.
//   - A seat nobody owns (taken by an anonymous client, or before principals
//     existed) is acted for by name, as every seat once was; the first
//     principal to do so becomes its owner.
//   - An anonymous request may act only for a seat nobody owns.
func (m *Manager) mayActFor(ctx context.Context, p *store.Player) error {
	id := auth.IDFrom(ctx)
	switch {
	case p.PrincipalID == id:
		return nil
	case p.PrincipalID != "":
		return ErrNotYourSeat
	}
	ok, err := m.store.BindPlayer(ctx, p.ID, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotYourSeat
	}
	p.PrincipalID = id
	return nil
}

// mayView reports whether the request may see seat p's private view (its
// hand, its secrets). It is mayActFor without the error: a request that may
// not is shown what a spectator sees.
func (m *Manager) mayView(ctx context.Context, p *store.Player) bool {
	return m.mayActFor(ctx, p) == nil
}

// mayViewSeat is mayView for the person at seat of the game.
func (m *Manager) mayViewSeat(ctx context.Context, gameID int64, seat int) bool {
	players, err := m.store.ListPlayers(ctx, gameID)
	if err != nil {
		return false
	}
	for i := range players {
		if players[i].Seat == seat {
			return m.mayView(ctx, &players[i])
		}
	}
	return false
}

// identify says who holds each seat: an account's handle, or a guest.
func (m *Manager) identify(ctx context.Context, seats []SeatInfo) {
	ids := make([]string, 0, len(seats))
	for _, s := range seats {
		ids = append(ids, s.PrincipalID)
	}
	who, err := m.store.Identities(ctx, ids)
	if err != nil {
		return
	}
	for i := range seats {
		if id, ok := who[seats[i].PrincipalID]; ok {
			seats[i].Handle, seats[i].Guest = id.Handle, id.Guest
		}
	}
}

// ErrNameIsAHandle is the answer to a name that is an account's handle (or
// one nobody may have) from anybody but that account (stage 7 of the
// accounts plan): a guest cannot pass for @ana by typing "ana".
var ErrNameIsAHandle = errors.New("session: that name is an account's: choose another")

// mayUseName reports whether the request may go by name in a game.
func (m *Manager) mayUseName(ctx context.Context, name string) error {
	h, ok := auth.HandleOf(name)
	if !ok {
		return nil
	}
	p, signedIn := auth.From(ctx)
	mine := signedIn && p.Account != nil && strings.EqualFold(p.Account.Handle, h)
	if auth.IsReserved(h) {
		if mine {
			return nil
		}
		return ErrNameIsAHandle
	}
	owner, err := m.store.HandleHeldBy(ctx, h, m.now())
	if err != nil || owner == 0 {
		return err
	}
	if signedIn && p.Account != nil && p.Account.ID == owner {
		return nil
	}
	return ErrNameIsAHandle
}

// OwnGame is one of the request's principal's games, for "My games".
type OwnGame struct {
	RoomCode  string   `json:"room_code"`
	GameDefID string   `json:"game_def_id"`
	Name      string   `json:"name"`
	Status    string   `json:"status"`
	Seats     []string `json:"seats"` // the names the principal plays under
	UpdatedAt string   `json:"updated_at"`
}

// OwnGames lists the games the request's principal has a seat in, the
// latest first: "My games", on any device the principal is on.
func (m *Manager) OwnGames(ctx context.Context, limit int) ([]OwnGame, error) {
	id := auth.IDFrom(ctx)
	if id == "" {
		return []OwnGame{}, nil
	}
	rows, err := m.store.OwnGames(ctx, id, limit)
	if err != nil {
		return nil, err
	}
	out := make([]OwnGame, 0, len(rows))
	for _, g := range rows {
		name := g.GameDefID
		if c, ok := m.catalog.Get(g.GameDefID); ok {
			name = c.Definition.Name
		}
		out = append(out, OwnGame{RoomCode: g.RoomCode, GameDefID: g.GameDefID, Name: name, Status: g.Status, Seats: g.Names, UpdatedAt: g.UpdatedAt})
	}
	return out, nil
}
