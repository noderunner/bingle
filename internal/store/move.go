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
	NextImpulseSeat     int     // ignored (games.impulse_seat set NULL) if GameEnded
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
	// SwapSeats says the move is the pie-rule swap: the occupants of the two
	// seats change places (the board itself the swap leaves untouched), in
	// the same transaction as the move.
	SwapSeats bool
	// EndsImpulse says this selection ends its impulse (state.ImpulseEnds):
	// the game's ply moves on with it. The row's own ply is the game's + 1.
	EndsImpulse bool
	// Facts are what the selection did that the rules report, as JSON
	// (ADR 0129); "" for none, stored as NULL.
	Facts string
}

// Move mirrors one row of the moves table (see schema.sql).
type Move struct {
	ID     int64
	GameID int64
	Seq    int
	// Ply is the impulse the row is a selection of (ADR 0114), from 1.
	Ply            int
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
	// Facts: what the selection did that the rules report, as JSON
	// (ADR 0129); "" for none.
	Facts string
}

// RecordMove inserts one moves row (seq = the game's current move_seq + 1,
// read and incremented inside the same transaction) and updates the parent
// games row's board_state/move_seq/impulse_seat/winner_seat/status to match, as
// a single transaction — both happen or neither does. If GameEnded, impulse_seat
// becomes NULL and status becomes "finished"; otherwise impulse_seat becomes
// NextImpulseSeat and status is left as whatever it already was (RecordMove
// doesn't itself manage the waiting→active transition — that's a session-
// layer concern, e.g. when a second player joins). Returns store.ErrNotFound
// if gameID doesn't exist.
func (s *Store) RecordMove(ctx context.Context, gameID int64, in MoveInput) (Move, error) {
	tx, err := s.begin(ctx, "RecordMove")
	if err != nil {
		return Move{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentMoveSeq, currentPly int
	var currentStatus string
	err = tx.queryRow(ctx, "RecordMove.readSeq", `SELECT move_seq, status, ply FROM games WHERE id = ?`, gameID).
		Scan(&currentMoveSeq, &currentStatus, &currentPly)
	if errors.Is(err, sql.ErrNoRows) {
		return Move{}, ErrNotFound
	}
	if err != nil {
		return Move{}, fmt.Errorf("reading current move_seq/status for game_id=%d: %w", gameID, err)
	}

	newSeq := currentMoveSeq + 1
	rowPly, gamePly := currentPly+1, currentPly
	if in.EndsImpulse {
		gamePly = rowPly
	}

	var captured sql.NullString
	if in.Captured != nil {
		captured = sql.NullString{String: *in.Captured, Valid: true}
	}

	res, err := tx.exec(ctx, "RecordMove.insertMove",
		`INSERT INTO moves (game_id, seq, seat, color, from_position, to_position, captured_position, resulting_state, notation, move_json, auto, locked, autoplay, ply, facts)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		gameID, newSeq, in.Seat, in.Color, in.From, in.To, captured, in.ResultingBoardState, in.Notation, in.MoveJSON, boolToInt(in.Auto), boolToInt(in.Locked), boolToInt(in.AutoPlay), rowPly, sql.NullString{String: in.Facts, Valid: in.Facts != ""},
	)
	if err != nil {
		return Move{}, fmt.Errorf("insert moves row: %w", err)
	}
	moveID, err := res.LastInsertId()
	if err != nil {
		return Move{}, fmt.Errorf("moves insert LastInsertId: %w", err)
	}

	var impulseSeat sql.NullInt64
	status := currentStatus
	if in.GameEnded {
		status = "finished"
	} else {
		impulseSeat = sql.NullInt64{Int64: int64(in.NextImpulseSeat), Valid: true}
	}

	var winnerSeat sql.NullInt64
	if in.WinnerSeat != nil {
		winnerSeat = sql.NullInt64{Int64: int64(*in.WinnerSeat), Valid: true}
	}

	if _, err := tx.exec(ctx, "RecordMove.updateGame",
		`UPDATE games SET board_state = ?, move_seq = ?, ply = ?, updated_at = CURRENT_TIMESTAMP, impulse_seat = ?, status = ?, winner_seat = ?, end_reason = ?, end_detail = ?, rating_status = CASE WHEN rated = 1 AND ? = 'finished' THEN 'pending' ELSE '' END, draw_offer_seat = NULL WHERE id = ?`,
		in.ResultingBoardState, newSeq, gamePly, impulseSeat, status, winnerSeat, in.EndReason, in.EndDetail, status, gameID,
	); err != nil {
		return Move{}, fmt.Errorf("updating games row for game_id=%d: %w", gameID, err)
	}
	if in.Clock != nil {
		if _, err := tx.exec(ctx, "RecordMove.updateClock", `UPDATE games SET clock = ? WHERE id = ?`, *in.Clock, gameID); err != nil {
			return Move{}, fmt.Errorf("updating the clock of game_id=%d: %w", gameID, err)
		}
	}
	if in.SwapSeats {
		if err := swapSeats(ctx, tx, gameID); err != nil {
			return Move{}, fmt.Errorf("swapping the seats of game_id=%d: %w", gameID, err)
		}
	}

	move, err := scanMoveByID(ctx, tx, "RecordMove.readMove", moveID)
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
	rows, err := s.query(ctx, "ListMoves",
		`SELECT id, game_id, seq, seat, color, from_position, to_position, captured_position, resulting_state, notation, move_json, auto, locked, autoplay, created_at, ply, facts
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
		var facts sql.NullString
		if err := rows.Scan(&m.ID, &m.GameID, &m.Seq, &m.Seat, &m.Color, &m.From, &m.To, &captured, &m.ResultingState, &m.Notation, &m.MoveJSON, &auto, &locked, &autoplay, &m.CreatedAt, &m.Ply, &facts); err != nil {
			return nil, fmt.Errorf("scanning moves row for game_id=%d: %w", gameID, err)
		}
		m.Auto = auto != 0
		m.Locked = locked != 0
		m.AutoPlay = autoplay != 0
		m.Facts = facts.String
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

func scanMoveByID(ctx context.Context, q rowQuerier, name string, id int64) (Move, error) {
	var m Move
	var captured sql.NullString
	var auto, locked, autoplay int
	var facts sql.NullString

	row := q.queryRow(ctx, name,
		`SELECT id, game_id, seq, seat, color, from_position, to_position, captured_position, resulting_state, notation, move_json, auto, locked, autoplay, created_at, ply, facts
		 FROM moves WHERE id = ?`,
		id,
	)
	if err := row.Scan(&m.ID, &m.GameID, &m.Seq, &m.Seat, &m.Color, &m.From, &m.To, &captured, &m.ResultingState, &m.Notation, &m.MoveJSON, &auto, &locked, &autoplay, &m.CreatedAt, &m.Ply, &facts); err != nil {
		return Move{}, err
	}
	m.Auto = auto != 0
	m.Locked = locked != 0
	m.AutoPlay = autoplay != 0
	m.Facts = facts.String

	if captured.Valid {
		v := captured.String
		m.Captured = &v
	}

	return m, nil
}

// UndoToSeq deletes every move with seq > keepSeq and rewinds the game to the
// position those moves came from, all in one transaction: boardState becomes the
// game's current snapshot, move_seq becomes keepSeq and impulse_seat impulseSeat.
// Returns ErrNotFound if gameID doesn't exist.
func (s *Store) UndoToSeq(ctx context.Context, gameID int64, keepSeq int, boardState string, impulseSeat int) error {
	tx, err := s.begin(ctx, "UndoToSeq")
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.exec(ctx, "UndoToSeq.deleteMoves", `DELETE FROM moves WHERE game_id = ? AND seq > ?`, gameID, keepSeq); err != nil {
		return fmt.Errorf("deleting moves for game_id=%d: %w", gameID, err)
	}
	res, err := tx.exec(ctx, "UndoToSeq.updateGame",
		`UPDATE games SET board_state = ?, move_seq = ?, impulse_seat = ?, ply = COALESCE((SELECT MAX(ply) FROM moves WHERE game_id = ?), 0) WHERE id = ?`,
		boardState, keepSeq, impulseSeat, gameID, gameID,
	)
	if err != nil {
		return fmt.Errorf("updating games row for game_id=%d: %w", gameID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) FinishGame(ctx context.Context, gameID int64, winnerSeat *int, reason string) error {
	var winner sql.NullInt64
	if winnerSeat != nil {
		winner = sql.NullInt64{Int64: int64(*winnerSeat), Valid: true}
	}
	res, err := s.exec(ctx, "FinishGame",
		`UPDATE games SET status = 'finished', impulse_seat = NULL, winner_seat = ?, end_reason = ?, rating_status = CASE rated WHEN 1 THEN 'pending' ELSE '' END, draw_offer_seat = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
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

// GamesToRegroup lists the rooms of the games with rows written before
// impulses were counted (ply 0), or with a declined reaction (auto) that does
// not end its impulse, as 0.80.0 numbered one followed by the seat's own turn,
// for session.Manager.RegroupMoves.
func (s *Store) GamesToRegroup(ctx context.Context) ([]string, error) {
	rows, err := s.query(ctx, "GamesToRegroup",
		`SELECT DISTINCT g.room_code FROM moves m JOIN games g ON g.id = m.game_id
		 WHERE m.ply = 0
		    OR (m.auto = 1 AND (g.ply < m.ply
		        OR EXISTS (SELECT 1 FROM moves n WHERE n.game_id = m.game_id AND n.ply = m.ply AND n.seq > m.seq)))`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var rooms []string
	for rows.Next() {
		var room string
		if err := rows.Scan(&room); err != nil {
			return nil, err
		}
		rooms = append(rooms, room)
	}
	return rooms, rows.Err()
}

// NumberPlies sets the ply of each of a game's rows (plies, by seq) and the
// game's own, in one transaction.
func (s *Store) NumberPlies(ctx context.Context, gameID int64, plies map[int]int, gamePly int) error {
	tx, err := s.begin(ctx, "NumberPlies")
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for seq, ply := range plies {
		if _, err := tx.exec(ctx, "NumberPlies.move", `UPDATE moves SET ply = ? WHERE game_id = ? AND seq = ?`, ply, gameID, seq); err != nil {
			return err
		}
	}
	if _, err := tx.exec(ctx, "NumberPlies.game", `UPDATE games SET ply = ? WHERE id = ?`, gamePly, gameID); err != nil {
		return err
	}
	return tx.Commit()
}
