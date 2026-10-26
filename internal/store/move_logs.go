package store

import (
	"context"
	"fmt"
)

// MoveLog is the event log a position carried after one move (the raw JSON
// array at ext.log of its resulting state; empty for a game without one).
type MoveLog struct {
	Seq int
	Log string
}

// ListMoveLogs returns, for every move of the game, the event log of the
// position after it, in ascending seq order. Only that part of each state is
// read, so a long game does not pull every full snapshot into memory.
func (s *Store) ListMoveLogs(ctx context.Context, gameID int64) ([]MoveLog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT seq, COALESCE(json_extract(resulting_state, '$.ext.log'), '') FROM moves WHERE game_id = ? ORDER BY seq`,
		gameID)
	if err != nil {
		return nil, fmt.Errorf("listing move logs (game_id=%d): %w", gameID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []MoveLog
	for rows.Next() {
		var l MoveLog
		if err := rows.Scan(&l.Seq, &l.Log); err != nil {
			return nil, fmt.Errorf("scanning move log (game_id=%d): %w", gameID, err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
