package store

import (
	"context"
	"fmt"
)

// SeatReplacement says a bot took a seat from its player during a game: who they
// were, why, and how many moves of the game were played up to then.
type SeatReplacement struct {
	Seat        int
	Name        string
	PrincipalID string
	Reason      string
	MoveSeq     int
	At          string
}

// SeatTakeover is one seat passing to a bot.
type SeatTakeover struct {
	Replaced SeatReplacement
	BotName  string
	Bot      string
}

// ReplaceSeats gives each seat to its bot and records who held it, in one
// transaction, with the game's clock (JSON; "" for no clock) as it stands without
// them. An outstanding draw offer goes, as a bot never answers one. restartTurn says
// the game was waiting on one of the seats, so that the turn starts again for the
// bot: a takeover of anybody else must not give the player to move their time back.
func (s *Store) ReplaceSeats(ctx context.Context, gameID int64, takeovers []SeatTakeover, clock string, restartTurn bool) error {
	tx, err := s.begin(ctx, "ReplaceSeats")
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, t := range takeovers {
		r := t.Replaced
		if _, err := tx.exec(ctx, "ReplaceSeats.record",
			`INSERT INTO seat_replacements (game_id, seat, name, principal_id, reason, move_seq) VALUES (?, ?, ?, ?, ?, ?)`,
			gameID, r.Seat, r.Name, r.PrincipalID, r.Reason, r.MoveSeq); err != nil {
			return fmt.Errorf("recording the replacement at seat %d: %w", r.Seat, err)
		}
		if _, err := tx.exec(ctx, "ReplaceSeats.player",
			`UPDATE players SET name = ?, bot = ?, principal_id = '' WHERE game_id = ? AND seat = ?`,
			t.BotName, t.Bot, gameID, r.Seat); err != nil {
			return fmt.Errorf("seating the bot at seat %d: %w", r.Seat, err)
		}
		if _, err := tx.exec(ctx, "ReplaceSeats.autoplay",
			`DELETE FROM autoplay_prefs WHERE game_id = ? AND seat = ?`, gameID, r.Seat); err != nil {
			return fmt.Errorf("clearing the Auto-Play Timeout at seat %d: %w", r.Seat, err)
		}
	}
	update := `UPDATE games SET clock = ?, draw_offer_seat = NULL WHERE id = ?`
	if restartTurn {
		update = `UPDATE games SET clock = ?, draw_offer_seat = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	}
	if _, err := tx.exec(ctx, "ReplaceSeats.game", update, clock, gameID); err != nil {
		return fmt.Errorf("updating game_id=%d: %w", gameID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// SeatReplacements lists who bots have replaced in a game, by seat.
func (s *Store) SeatReplacements(ctx context.Context, gameID int64) ([]SeatReplacement, error) {
	rows, err := s.query(ctx, "SeatReplacements",
		`SELECT seat, name, principal_id, reason, move_seq, replaced_at FROM seat_replacements WHERE game_id = ? ORDER BY seat`, gameID)
	if err != nil {
		return nil, fmt.Errorf("listing seat replacements: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SeatReplacement
	for rows.Next() {
		var r SeatReplacement
		if err := rows.Scan(&r.Seat, &r.Name, &r.PrincipalID, &r.Reason, &r.MoveSeq, &r.At); err != nil {
			return nil, fmt.Errorf("scanning a seat replacement: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// WasReplaced says a bot has taken a seat of this game from this person: the
// principal that held it, or the name of a seat nobody owned.
func (s *Store) WasReplaced(ctx context.Context, gameID int64, principalID, name string) (bool, error) {
	var n int
	err := s.queryRow(ctx, "WasReplaced",
		`SELECT COUNT(*) FROM seat_replacements WHERE game_id = ?
		 AND ((principal_id != '' AND principal_id = ?) OR (principal_id = '' AND name = ? COLLATE NOCASE))`,
		gameID, principalID, name).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("reading seat replacements: %w", err)
	}
	return n > 0, nil
}
