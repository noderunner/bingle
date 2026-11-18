package session

import (
	"context"
	"errors"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/store"
)

// ErrDealsAccount answers a guest who asks for a deal history: it is kept
// for accounts (ADR 0136).
var ErrDealsAccount = errors.New("session: a deal history is kept for accounts: sign in")

// maxDeals bounds a deal history.
const maxDeals = 200

// DealRecord is one finished game of patience in an account's deal history.
type DealRecord struct {
	// Seed is what the game was dealt from, to play it again.
	Seed string `json:"seed"`
	// Won says the game was won; Moves is the moves it used (state.Moves),
	// which for a win is its score.
	Won   bool `json:"won"`
	Moves int  `json:"moves"`
	// Chosen says the player chose the deal: it kept no best score.
	Chosen   bool   `json:"chosen,omitempty"`
	RoomCode string `json:"room_code"`
	EndedAt  string `json:"ended_at"`
}

// Deals is the signed-in account's finished games of gameDefID that were
// dealt from a seed, the latest first.
func (m *Manager) Deals(ctx context.Context, gameDefID string) ([]DealRecord, error) {
	p, ok := auth.From(ctx)
	if !ok || p.Account == nil {
		return nil, ErrDealsAccount
	}
	if _, ok := m.catalog.Get(gameDefID); !ok {
		return nil, ErrGameDefNotFound
	}
	rows, err := m.store.AccountDeals(ctx, p.Account.ID, gameDefID, maxDeals)
	if err != nil {
		return nil, err
	}
	out := make([]DealRecord, 0, len(rows))
	for _, d := range rows {
		out = append(out, DealRecord{
			Seed: d.Seed, Won: d.Won, Moves: movesOf(store.Game{BoardState: d.BoardState, Ply: d.Ply}),
			Chosen: d.StartChosen, RoomCode: d.RoomCode, EndedAt: d.EndedAt,
		})
	}
	return out, nil
}
