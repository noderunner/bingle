package session

import (
	"context"
	"errors"

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
