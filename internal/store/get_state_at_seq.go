package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetStateAtSeq returns the resulting_state JSON stored on the moves row for
// (gameID, seq) — see docs/history/plan.md section 6.6c. Returns ErrNotFound
// (checkable via errors.Is) if no such row exists — whether because gameID
// itself doesn't exist, or because that seq was never recorded for it
// (including seq=0, which is never a real moves row).
func (s *Store) GetStateAtSeq(ctx context.Context, gameID int64, seq int) (string, error) {
	var resultingState string

	row := s.db.QueryRowContext(ctx,
		`SELECT resulting_state FROM moves WHERE game_id = ? AND seq = ?`,
		gameID, seq,
	)
	err := row.Scan(&resultingState)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("scanning moves row (game_id=%d, seq=%d): %w", gameID, seq, err)
	}

	return resultingState, nil
}
