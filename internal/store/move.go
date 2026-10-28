package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// MoveInput carries everything RecordMove needs to persist the result of an
// already-computed state.ApplyMove call (see docs/history/plan.md section 6.6b).
// Positions are plain strings here, not board.PositionID, keeping Store
// decoupled from internal/engine/*.
type MoveInput struct {
	Seat                int
	Color               string
	From                string
	To                  string
	Captured            *string // nil ⇒ no capture on this move
	ResultingBoardState string  // the JSON board_state after this move
	NextTurnSeat        int     // ignored (games.turn_seat set NULL) if GameEnded
	GameEnded           bool
	WinnerSeat          *int   // nil unless GameEnded (nil + GameEnded = draw)
	EndReason           string // why the game ended; "" unless GameEnded
	EndDetail           string // specifics such as the final score
	Notation            string // the move in the game's canonical notation
	MoveJSON            string // the structured move, as JSON
	// Auto says the server played the move on the seat's behalf (a reaction
	// the player chose to pass automatically).
	Auto bool
	// AutoPlay says a tactical bot played the move for a person whose
	// Auto-Play Timeout ran out.
	AutoPlay bool
	// Locked says the move rolled dice (or drew a hidden token): the game
	// cannot be taken back across it.
	Locked bool
	// Clock, when set, replaces the game's clock: the mover's time, charged
	// with the turn they have just taken, written in the same transaction as
	// the move so that the two can never disagree.
	Clock *string
}

// Move mirrors one row of the moves table (see schema.sql).
type Move struct {
	ID             int64
	GameID         int64
	Seq            int
	Seat           int
	Color          string
	From           string
	To             string
	Captured       *string
	ResultingState string
	Notation       string
	MoveJSON       string
	// Auto: played by the server for the seat (an auto-passed reaction).
	Auto bool
	// AutoPlay: played by a tactical bot for a person whose Auto-Play Timeout
	// ran out.
	AutoPlay bool
	// Locked: the move rolled dice (or drew a hidden token).
	Locked    bool
	CreatedAt string
}

// RecordMove inserts one moves row (seq = the game's current move_seq + 1,
// read and incremented inside the same transaction) and updates the parent
// games row's board_state/move_seq/turn_seat/winner_seat/status to match, as
// a single transaction — both happen or neither does. If GameEnded, turn_seat
// becomes NULL and status becomes "finished"; otherwise turn_seat becomes
// NextTurnSeat and status is left as whatever it already was (RecordMove
// doesn't itself manage the waiting→active transition — that's a session-
// layer concern, e.g. when a second player joins). Returns store.ErrNotFound
// if gameID doesn't exist.
func (s *Store) RecordMove(ctx context.Context, gameID int64, in MoveInput) (Move, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Move{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentMoveSeq int
	var currentStatus string
	err = tx.QueryRowContext(ctx, `SELECT move_seq, status FROM games WHERE id = ?`, gameID).
		Scan(&currentMoveSeq, &currentStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return Move{}, ErrNotFound
	}
	if err != nil {
		return Move{}, fmt.Errorf("reading current move_seq/status for game_id=%d: %w", gameID, err)
	}

	newSeq := currentMoveSeq + 1

	var captured sql.NullString
	if in.Captured != nil {
		captured = sql.NullString{String: *in.Captured, Valid: true}
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO moves (game_id, seq, seat, color, from_position, to_position, captured_position, resulting_state, notation, move_json, auto, locked, autoplay)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		gameID, newSeq, in.Seat, in.Color, in.From, in.To, captured, in.ResultingBoardState, in.Notation, in.MoveJSON, boolToInt(in.Auto), boolToInt(in.Locked), boolToInt(in.AutoPlay),
	)
	if err != nil {
		return Move{}, fmt.Errorf("insert moves row: %w", err)
	}
	moveID, err := res.LastInsertId()
	if err != nil {
		return Move{}, fmt.Errorf("moves insert LastInsertId: %w", err)
	}

	var turnSeat sql.NullInt64
	status := currentStatus
	if in.GameEnded {
		status = "finished"
	} else {
		turnSeat = sql.NullInt64{Int64: int64(in.NextTurnSeat), Valid: true}
	}

	var winnerSeat sql.NullInt64
	if in.WinnerSeat != nil {
		winnerSeat = sql.NullInt64{Int64: int64(*in.WinnerSeat), Valid: true}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE games SET board_state = ?, move_seq = ?, updated_at = CURRENT_TIMESTAMP, turn_seat = ?, status = ?, winner_seat = ?, end_reason = ?, end_detail = ?, draw_offer_seat = NULL, takeback_seat = NULL, takeback_seq = 0 WHERE id = ?`,
		in.ResultingBoardState, newSeq, turnSeat, status, winnerSeat, in.EndReason, in.EndDetail, gameID,
	); err != nil {
		return Move{}, fmt.Errorf("updating games row for game_id=%d: %w", gameID, err)
	}
	if in.Clock != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE games SET clock = ? WHERE id = ?`, *in.Clock, gameID); err != nil {
			return Move{}, fmt.Errorf("updating the clock of game_id=%d: %w", gameID, err)
		}
	}

	move, err := scanMoveByID(ctx, tx, moveID)
	if err != nil {
		return Move{}, fmt.Errorf("reading back inserted move: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Move{}, fmt.Errorf("commit transaction: %w", err)
	}

	return move, nil
}

// ListMoves returns every moves row for gameID with seq > sinceSeq, in
// ascending seq order (see docs/history/plan.md section 6.8). A gameID with no
// matching rows (whether because it has no moves yet, or because it doesn't
// exist at all) is not an error here — it's a normal empty result; the
// caller (session.Manager.ListMoves) is what checks the game exists via
// GetGame first. Always returns a non-nil (possibly empty) slice, so JSON
// encoding produces "[]", never "null".
func (s *Store) ListMoves(ctx context.Context, gameID int64, sinceSeq int) ([]Move, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, game_id, seq, seat, color, from_position, to_position, captured_position, resulting_state, notation, move_json, auto, locked, autoplay, created_at
		 FROM moves WHERE game_id = ? AND seq > ? ORDER BY seq`,
		gameID, sinceSeq,
	)
	if err != nil {
		return nil, fmt.Errorf("querying moves for game_id=%d: %w", gameID, err)
	}
	defer func() { _ = rows.Close() }()

	moves := make([]Move, 0)
	for rows.Next() {
		var m Move
		var captured sql.NullString
		var auto, locked, autoplay int
		if err := rows.Scan(&m.ID, &m.GameID, &m.Seq, &m.Seat, &m.Color, &m.From, &m.To, &captured, &m.ResultingState, &m.Notation, &m.MoveJSON, &auto, &locked, &autoplay, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning moves row for game_id=%d: %w", gameID, err)
		}
		m.Auto = auto != 0
		m.Locked = locked != 0
		m.AutoPlay = autoplay != 0
		if captured.Valid {
			v := captured.String
			m.Captured = &v
		}
		moves = append(moves, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating moves rows for game_id=%d: %w", gameID, err)
	}

	return moves, nil
}

func scanMoveByID(ctx context.Context, q queryRowContexter, id int64) (Move, error) {
	var m Move
	var captured sql.NullString
	var auto, locked, autoplay int

	row := q.QueryRowContext(ctx,
		`SELECT id, game_id, seq, seat, color, from_position, to_position, captured_position, resulting_state, notation, move_json, auto, locked, autoplay, created_at
		 FROM moves WHERE id = ?`,
		id,
	)
	if err := row.Scan(&m.ID, &m.GameID, &m.Seq, &m.Seat, &m.Color, &m.From, &m.To, &captured, &m.ResultingState, &m.Notation, &m.MoveJSON, &auto, &locked, &autoplay, &m.CreatedAt); err != nil {
		return Move{}, err
	}
	m.Auto = auto != 0
	m.Locked = locked != 0
	m.AutoPlay = autoplay != 0

	if captured.Valid {
		v := captured.String
		m.Captured = &v
	}

	return m, nil
}

// UndoToSeq deletes every move with seq > keepSeq and rewinds the game to
// the position those moves came from: boardState becomes the game's current
// snapshot, move_seq becomes keepSeq, turn_seat becomes turnSeat, and the
// game is active again with no winner. All of it happens in one transaction.
// Returns ErrNotFound if gameID doesn't exist.
func (s *Store) UndoToSeq(ctx context.Context, gameID int64, keepSeq int, boardState string, turnSeat int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM moves WHERE game_id = ? AND seq > ?`, gameID, keepSeq); err != nil {
		return fmt.Errorf("deleting moves for game_id=%d: %w", gameID, err)
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE games SET board_state = ?, move_seq = ?, turn_seat = ?, status = 'active', winner_seat = NULL, end_reason = '', end_detail = '', draw_offer_seat = NULL, takeback_seat = NULL, takeback_seq = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		boardState, keepSeq, turnSeat, gameID,
	)
	if err != nil {
		return fmt.Errorf("updating games row for game_id=%d: %w", gameID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// FinishGame ends a game without a move (resignation, agreed draw, ...):
// status becomes "finished", turn_seat NULL, winner_seat the given seat (nil
// = draw) and end_reason the given reason. Returns ErrNotFound if gameID
// doesn't exist.
func (s *Store) FinishGame(ctx context.Context, gameID int64, winnerSeat *int, reason string) error {
	var winner sql.NullInt64
	if winnerSeat != nil {
		winner = sql.NullInt64{Int64: int64(*winnerSeat), Valid: true}
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE games SET status = 'finished', turn_seat = NULL, winner_seat = ?, end_reason = ?, draw_offer_seat = NULL, takeback_seat = NULL, takeback_seq = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		winner, reason, gameID,
	)
	if err != nil {
		return fmt.Errorf("finishing game_id=%d: %w", gameID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
