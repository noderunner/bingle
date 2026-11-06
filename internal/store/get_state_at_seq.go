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

	row := s.queryRow(ctx, "GetStateAtSeq",
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

// SeqAtPly is the seq of the last row of ply (ADR 0114): the position after
// that many impulses is the one stored on it. 0 for ply 0, and ErrNotFound
// for a ply the game has not played.
func (s *Store) SeqAtPly(ctx context.Context, gameID int64, ply int) (int, error) {
	if ply < 0 {
		return 0, ErrNotFound
	}
	var played int
	var seq sql.NullInt64
	err := s.queryRow(ctx, "SeqAtPly",
		`SELECT g.ply, (SELECT MAX(seq) FROM moves WHERE game_id = g.id AND ply <= ?) FROM games g WHERE g.id = ?`,
		ply, gameID).Scan(&played, &seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("seq at ply %d (game_id=%d): %w", ply, gameID, err)
	}
	if ply > played {
		return 0, ErrNotFound
	}
	return int(seq.Int64), nil
}
