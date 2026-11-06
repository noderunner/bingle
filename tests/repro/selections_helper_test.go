package repro_test

import (
	"context"

	"github.com/noderunner/bingle/internal/session"
)

// legalSelections lists the first selections of the seat on impulse, and that seat.
func legalSelections(ctx context.Context, m *session.Manager, room string) (int, []session.SelectionOption, error) {
	v, err := m.Selections(ctx, room, "", nil)
	return v.Seat, v.Options, err
}
