package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// A moderator may take a rated game out of the ratings (ADR 0157). Its rows in
// rating_history stay, marked by a row in rating_removals, and the ratings are
// written again: replayed without it, or with its own change reversed.

// RatingRemoved is the rating status of a game taken out of the ratings.
const RatingRemoved = "removed"

// ErrAlreadyRemoved is the answer to removing a game that is out already.
var ErrAlreadyRemoved = errors.New("store: that game is out of the ratings already")

// RatedSide is one player's row of a rated result, as rating_history keeps it.
type RatedSide struct {
	ID                 int64
	AccountID          int64
	OpponentID         int64
	Handle             string
	OpponentHandle     string
	Family, FamilyName string
	RoomCode           string
	Seat               int
	Score              float64
	Before, After      float64
	PlayedAt           time.Time
	TournamentID       int64
	Removed            bool
	RemovedBy          string // the moderator's handle
	RemovedAt          string
	Recomputed         bool
	// playedRaw is played_at as stored, which both sides of a game share.
	playedRaw string
}

const ratedSideCols = `h.id, h.account_id, h.opponent_id, a.handle, o.handle, h.family, h.family_name, h.room_code, h.seat, h.score,
	h.rating_before, h.rating_after, h.played_at, h.tournament_id, x.room_code IS NOT NULL, COALESCE(m.handle, ''), COALESCE(x.removed_at, ''), COALESCE(x.recomputed, 0)`

const ratedSideFrom = ` FROM rating_history h JOIN accounts a ON a.id = h.account_id JOIN accounts o ON o.id = h.opponent_id
	LEFT JOIN rating_removals x ON x.room_code = h.room_code LEFT JOIN accounts m ON m.id = x.removed_by`

func (s *Store) ratedSides(ctx context.Context, name, where string, args ...any) ([]RatedSide, error) {
	rows, err := s.query(ctx, name, `SELECT `+ratedSideCols+ratedSideFrom+` `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("reading rated results: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []RatedSide
	for rows.Next() {
		var r RatedSide
		var removed, recomputed int
		if err := rows.Scan(&r.ID, &r.AccountID, &r.OpponentID, &r.Handle, &r.OpponentHandle, &r.Family, &r.FamilyName, &r.RoomCode, &r.Seat, &r.Score,
			&r.Before, &r.After, &r.playedRaw, &r.TournamentID, &removed, &r.RemovedBy, &r.RemovedAt, &recomputed); err != nil {
			return nil, fmt.Errorf("scanning a rated result: %w", err)
		}
		r.PlayedAt, _ = time.Parse(time.RFC3339Nano, r.playedRaw)
		r.Removed, r.Recomputed = removed == 1, recomputed == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// AccountRatedGames lists an account's rated results, the newest first.
func (s *Store) AccountRatedGames(ctx context.Context, accountID int64, limit int) ([]RatedSide, error) {
	return s.ratedSides(ctx, "AccountRatedGames", `WHERE h.account_id = ? ORDER BY h.played_at DESC, h.id DESC LIMIT ?`, accountID, limit)
}

// RatedGame is both sides of the rated game played in a room, by seat, or
// ErrNotFound. A room code used twice names the later game.
func (s *Store) RatedGame(ctx context.Context, roomCode string) ([2]RatedSide, error) {
	rows, err := s.ratedSides(ctx, "RatedGame", `WHERE h.room_code = ? ORDER BY h.played_at DESC, h.id DESC LIMIT 2`, roomCode)
	if err != nil {
		return [2]RatedSide{}, err
	}
	if len(rows) != 2 || rows[0].playedRaw != rows[1].playedRaw {
		return [2]RatedSide{}, ErrNotFound
	}
	if rows[0].Seat > rows[1].Seat {
		rows[0], rows[1] = rows[1], rows[0]
	}
	return [2]RatedSide{rows[0], rows[1]}, nil
}

// FamilyHistory is every rated result of a family in the order it was written,
// removed or not: what a replay of its ratings reads.
func (s *Store) FamilyHistory(ctx context.Context, family string) ([]RatedSide, error) {
	return s.ratedSides(ctx, "FamilyHistory", `WHERE h.family = ? ORDER BY h.played_at, h.id`, family)
}

// FamilyRatings is every rating in a family, the deleted accounts' too.
func (s *Store) FamilyRatings(ctx context.Context, family string) ([]Rating, error) {
	return s.listRatings(ctx, "FamilyRatings",
		`SELECT `+ratingCols+` FROM ratings r JOIN accounts a ON a.id = r.account_id WHERE r.family = ? ORDER BY r.account_id`, family)
}

// OpponentsIn counts the different accounts an account has a result against in
// a family, leaving out the games removed and the one in room.
func (s *Store) OpponentsIn(ctx context.Context, accountID int64, family, room string) (int, error) {
	var n int
	err := s.queryRow(ctx, "OpponentsIn",
		`SELECT COUNT(DISTINCT opponent_id) FROM rating_history h WHERE account_id = ? AND family = ? AND room_code != ?
		 AND NOT EXISTS (SELECT 1 FROM rating_removals x WHERE x.room_code = h.room_code)`, accountID, family, room).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting opponents: %w", err)
	}
	return n, nil
}

// RatingRemoval is what taking a game out of the ratings writes.
type RatingRemoval struct {
	RoomCode   string
	Family     string
	By         int64
	At         time.Time
	Recomputed bool
	// Ratings are the family's ratings to write; Dropped the accounts left
	// with no rated game in it, whose ratings go.
	Ratings []Rating
	Dropped []int64
	// Counters are the counters the game added, one per player, to take off.
	Counters []Counter
}

// Counter is one of an account's counters.
type Counter struct {
	AccountID int64
	Key       string
}

// RemoveRatedGame writes a removal in one transaction: the mark, the ratings,
// the counters and the game's status if retention has kept it. A game that is
// out already is ErrAlreadyRemoved, and nothing is written.
func (s *Store) RemoveRatedGame(ctx context.Context, rm RatingRemoval) error {
	tx, err := s.begin(ctx, "RemoveRatedGame")
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	recomputed := 0
	if rm.Recomputed {
		recomputed = 1
	}
	res, err := tx.exec(ctx, "RemoveRatedGame.mark",
		`INSERT INTO rating_removals (room_code, removed_by, removed_at, recomputed) VALUES (?, ?, ?, ?) ON CONFLICT (room_code) DO NOTHING`,
		rm.RoomCode, rm.By, rm.At.UTC().Format(time.RFC3339Nano), recomputed)
	if err != nil {
		return fmt.Errorf("marking a game removed: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAlreadyRemoved
	}
	for _, r := range rm.Ratings {
		if _, err := tx.exec(ctx, "RemoveRatedGame.rating",
			`UPDATE ratings SET rating = ?, deviation = ?, volatility = ?, games = ?, opponents = ?, last_game_at = ? WHERE account_id = ? AND family = ?`,
			r.Rating, r.Deviation, r.Volatility, r.Games, r.Opponents, r.LastGameAt.UTC().Format(time.RFC3339Nano), r.AccountID, rm.Family); err != nil {
			return fmt.Errorf("writing a rating: %w", err)
		}
	}
	for _, id := range rm.Dropped {
		if _, err := tx.exec(ctx, "RemoveRatedGame.drop", `DELETE FROM ratings WHERE account_id = ? AND family = ?`, id, rm.Family); err != nil {
			return fmt.Errorf("dropping a rating: %w", err)
		}
	}
	for _, c := range rm.Counters {
		if _, err := tx.exec(ctx, "RemoveRatedGame.stat",
			`UPDATE stats SET value = MAX(value - 1, 0) WHERE account_id = ? AND key = ?`, c.AccountID, c.Key); err != nil {
			return fmt.Errorf("taking off a counter: %w", err)
		}
	}
	if _, err := tx.exec(ctx, "RemoveRatedGame.game",
		`UPDATE games SET rating_status = ? WHERE room_code = ? AND rated = 1 AND rating_status = ?`, RatingRemoved, rm.RoomCode, RatingRecorded); err != nil {
		return fmt.Errorf("setting the rating status of room %s: %w", rm.RoomCode, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// TournamentRatingRemovals are a tournament's results that were taken out of
// the ratings, one row per player, in the order they were played.
func (s *Store) TournamentRatingRemovals(ctx context.Context, tournamentID int64) ([]RatedSide, error) {
	return s.ratedSides(ctx, "TournamentRatingRemovals", `WHERE h.tournament_id = ? AND x.room_code IS NOT NULL ORDER BY h.played_at, h.id`, tournamentID)
}
