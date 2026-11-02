package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// What became of a rated game, in games.rating_status. A game that is not rated
// has none; one that ended is pending until its result has been written, and
// then says whether the ratings moved.
const (
	RatingPending    = "pending"
	RatingRecorded   = "rated"
	RatingNoMoves    = "no_moves"    // somebody never played a move
	RatingDailyLimit = "daily_limit" // the same two accounts have played enough today
	RatingVoid       = "void"        // a seat was not an account's, or the game has gone
)

// Rating is an account's rating in a game family.
type Rating struct {
	AccountID  int64
	Handle     string
	Family     string
	FamilyName string
	Rating     float64
	Deviation  float64
	Volatility float64
	Games      int
	Opponents  int
	LastGameAt time.Time
}

const ratingCols = `r.account_id, a.handle, r.family, r.family_name, r.rating, r.deviation, r.volatility, r.games, r.opponents, r.last_game_at`

func scanRating(row rowScanner) (Rating, error) {
	var r Rating
	var last string
	if err := row.Scan(&r.AccountID, &r.Handle, &r.Family, &r.FamilyName, &r.Rating, &r.Deviation, &r.Volatility, &r.Games, &r.Opponents, &last); err != nil {
		return Rating{}, err
	}
	r.LastGameAt, _ = time.Parse(time.RFC3339Nano, last)
	return r, nil
}

// SetRated marks a game as rated.
func (s *Store) SetRated(ctx context.Context, gameID int64) error {
	if _, err := s.exec(ctx, "SetRated", `UPDATE games SET rated = 1 WHERE id = ?`, gameID); err != nil {
		return fmt.Errorf("marking game_id=%d as rated: %w", gameID, err)
	}
	return nil
}

// PendingRatedGames lists the rated games that have ended and whose results are
// still to be written. A game ending marks itself in the transaction that ends
// it, so a crash can delay a result and never lose one.
func (s *Store) PendingRatedGames(ctx context.Context) ([]Game, error) {
	rows, err := s.query(ctx, "PendingRatedGames", `SELECT `+gameColumns+` FROM games WHERE rating_status = ?`, RatingPending)
	if err != nil {
		return nil, fmt.Errorf("listing rated games to record: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Game
	for rows.Next() {
		g, err := scanGameRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning a rated game: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SetRatingStatus says what became of a pending game. It changes nothing for a
// game that is no longer pending.
func (s *Store) SetRatingStatus(ctx context.Context, gameID int64, status string) error {
	if _, err := s.exec(ctx, "SetRatingStatus", `UPDATE games SET rating_status = ? WHERE id = ? AND rating_status = ?`, status, gameID, RatingPending); err != nil {
		return fmt.Errorf("setting the rating status of game_id=%d: %w", gameID, err)
	}
	return nil
}

// AccountsOfPrincipals maps principals to the accounts they are.
func (s *Store) AccountsOfPrincipals(ctx context.Context, ids []string) (map[string]int64, error) {
	out := map[string]int64{}
	for _, id := range ids {
		if id == "" {
			continue
		}
		var account sql.NullInt64
		err := s.queryRow(ctx, "AccountsOfPrincipals", `SELECT account_id FROM principals WHERE id = ? AND kind = ?`, id, KindAccount).Scan(&account)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("finding the account of a principal: %w", err)
		}
		if account.Valid {
			out[id] = account.Int64
		}
	}
	return out, nil
}

// SeatsThatMoved counts the seats with a move of their own in a game: one the
// server or a bot made for them does not count.
func (s *Store) SeatsThatMoved(ctx context.Context, gameID int64) (int, error) {
	var n int
	err := s.queryRow(ctx, "SeatsThatMoved", `SELECT COUNT(DISTINCT seat) FROM moves WHERE game_id = ? AND auto = 0 AND autoplay = 0`, gameID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting the seats that moved in game_id=%d: %w", gameID, err)
	}
	return n, nil
}

// RatedGamesBetween counts the rated results two accounts have had against each
// other in a family since a moment.
func (s *Store) RatedGamesBetween(ctx context.Context, a, b int64, family string, since time.Time) (int, error) {
	var n int
	err := s.queryRow(ctx, "RatedGamesBetween",
		`SELECT COUNT(*) FROM rating_history WHERE account_id = ? AND opponent_id = ? AND family = ? AND played_at >= ?`,
		a, b, family, since.UTC().Format(time.RFC3339Nano)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting rated games between two accounts: %w", err)
	}
	return n, nil
}

// RatingOf is an account's rating in a family; ok is false before its first
// rated game there.
func (s *Store) RatingOf(ctx context.Context, accountID int64, family string) (Rating, bool, error) {
	r, err := scanRating(s.queryRow(ctx, "RatingOf",
		`SELECT `+ratingCols+` FROM ratings r JOIN accounts a ON a.id = r.account_id WHERE r.account_id = ? AND r.family = ?`, accountID, family))
	if errors.Is(err, sql.ErrNoRows) {
		return Rating{}, false, nil
	}
	if err != nil {
		return Rating{}, false, fmt.Errorf("reading a rating: %w", err)
	}
	return r, true, nil
}

// RatedResult is one player's side of a rated game, with their new rating.
type RatedResult struct {
	AccountID, OpponentID int64
	Seat                  int
	Score                 float64
	Before                float64
	Rating                float64
	Deviation             float64
	Volatility            float64
}

// RecordRatedGame writes both sides of a rated game in one transaction: the
// ratings, the history and the counters, and the game's status. It writes
// nothing for a game that is no longer pending, so a result cannot count twice.
func (s *Store) RecordRatedGame(ctx context.Context, game Game, family, familyName string, results [2]RatedResult, at time.Time) error {
	tx, err := s.begin(ctx, "RecordRatedGame")
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.exec(ctx, "RecordRatedGame.status", `UPDATE games SET rating_status = ? WHERE id = ? AND rating_status = ?`, RatingRecorded, game.ID, RatingPending)
	if err != nil {
		return fmt.Errorf("setting the rating status of game_id=%d: %w", game.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	stamp := at.UTC().Format(time.RFC3339Nano)
	for _, r := range results {
		var met int
		if err := tx.queryRow(ctx, "RecordRatedGame.met",
			`SELECT COUNT(*) FROM rating_history WHERE account_id = ? AND opponent_id = ? AND family = ?`, r.AccountID, r.OpponentID, family).Scan(&met); err != nil {
			return fmt.Errorf("looking for an earlier game: %w", err)
		}
		newOpponent := 0
		if met == 0 {
			newOpponent = 1
		}
		if _, err := tx.exec(ctx, "RecordRatedGame.rating",
			`INSERT INTO ratings (account_id, family, family_name, rating, deviation, volatility, games, opponents, last_game_at)
			 VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
			 ON CONFLICT (account_id, family) DO UPDATE SET family_name = excluded.family_name, rating = excluded.rating, deviation = excluded.deviation,
			   volatility = excluded.volatility, games = games + 1, opponents = opponents + excluded.opponents, last_game_at = excluded.last_game_at`,
			r.AccountID, family, familyName, r.Rating, r.Deviation, r.Volatility, newOpponent, stamp); err != nil {
			return fmt.Errorf("writing a rating: %w", err)
		}
		if _, err := tx.exec(ctx, "RecordRatedGame.history",
			`INSERT INTO rating_history (account_id, opponent_id, family, family_name, game_def_id, room_code, seat, score, rating_before, rating_after, deviation_after, played_at, tournament_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.AccountID, r.OpponentID, family, familyName, game.GameDefID, game.RoomCode, r.Seat, r.Score, r.Before, r.Rating, r.Deviation, stamp, game.TournamentID); err != nil {
			return fmt.Errorf("writing the rating history: %w", err)
		}
		outcome := "drawn"
		switch r.Score {
		case 1:
			outcome = "won"
		case 0:
			outcome = "lost"
		}
		for _, key := range []string{StatKey("played", family), StatKey(outcome, family)} {
			if _, err := tx.exec(ctx, "RecordRatedGame.stat",
				`INSERT INTO stats (account_id, key, value) VALUES (?, ?, 1) ON CONFLICT (account_id, key) DO UPDATE SET value = value + 1`, r.AccountID, key); err != nil {
				return fmt.Errorf("writing a counter: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// StatKey is the counter of a thing per game family: "won:hex".
func StatKey(what, family string) string { return what + ":" + family }

// Stats is an account's counters.
func (s *Store) Stats(ctx context.Context, accountID int64) (map[string]int, error) {
	rows, err := s.query(ctx, "Stats", `SELECT key, value FROM stats WHERE account_id = ?`, accountID)
	if err != nil {
		return nil, fmt.Errorf("reading counters: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var v int
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scanning a counter: %w", err)
		}
		out[k] = v
	}
	return out, rows.Err()
}

// RatingChange is what one rated game did to one seat's rating.
type RatingChange struct {
	Seat          int
	Before, After float64
}

// RatingChanges lists what a game did to its players' ratings, by seat.
func (s *Store) RatingChanges(ctx context.Context, roomCode string) ([]RatingChange, error) {
	rows, err := s.query(ctx, "RatingChanges", `SELECT seat, rating_before, rating_after FROM rating_history WHERE room_code = ? ORDER BY seat`, roomCode)
	if err != nil {
		return nil, fmt.Errorf("reading rating changes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []RatingChange
	for rows.Next() {
		var c RatingChange
		if err := rows.Scan(&c.Seat, &c.Before, &c.After); err != nil {
			return nil, fmt.Errorf("scanning a rating change: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Leaderboard lists the ratings of a family that have earned their place, the
// highest first. A deleted account is left out.
func (s *Store) Leaderboard(ctx context.Context, family string, minGames, minOpponents, limit int) ([]Rating, error) {
	return s.listRatings(ctx, "Leaderboard",
		`SELECT `+ratingCols+` FROM ratings r JOIN accounts a ON a.id = r.account_id
		 WHERE r.family = ? AND r.games >= ? AND r.opponents >= ? AND a.deleted_at IS NULL
		 ORDER BY r.rating DESC, a.handle_lower LIMIT ?`, family, minGames, minOpponents, limit)
}

// AccountRatings lists every rating an account has, by the game's name.
func (s *Store) AccountRatings(ctx context.Context, accountID int64) ([]Rating, error) {
	return s.listRatings(ctx, "AccountRatings",
		`SELECT `+ratingCols+` FROM ratings r JOIN accounts a ON a.id = r.account_id WHERE r.account_id = ? ORDER BY r.family_name`, accountID)
}

func (s *Store) listRatings(ctx context.Context, name, q string, args ...any) ([]Rating, error) {
	rows, err := s.query(ctx, name, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing ratings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Rating
	for rows.Next() {
		r, err := scanRating(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning a rating: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RatedFamily is a game family with a leaderboard, and how many are on it.
type RatedFamily struct {
	Family, Name string
	Players      int
}

// RatedFamilies lists the families whose leaderboards have anybody on them.
func (s *Store) RatedFamilies(ctx context.Context, minGames, minOpponents int) ([]RatedFamily, error) {
	rows, err := s.query(ctx, "RatedFamilies",
		`SELECT r.family, MAX(r.family_name), COUNT(*) FROM ratings r JOIN accounts a ON a.id = r.account_id
		 WHERE r.games >= ? AND r.opponents >= ? AND a.deleted_at IS NULL GROUP BY r.family ORDER BY 2`, minGames, minOpponents)
	if err != nil {
		return nil, fmt.Errorf("listing rated games: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []RatedFamily
	for rows.Next() {
		var f RatedFamily
		if err := rows.Scan(&f.Family, &f.Name, &f.Players); err != nil {
			return nil, fmt.Errorf("scanning a rated game: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FamilyNameOf is a game's name without its layout: "Hex (11×11)" is Hex.
func FamilyNameOf(name string) string {
	if i := strings.Index(name, " ("); i > 0 {
		return name[:i]
	}
	return name
}
