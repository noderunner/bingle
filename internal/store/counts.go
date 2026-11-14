package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"
)

// Every game that counts adds to its players' lasting counts when it ends
// (ADR 0130), in stats under keys of their own: the rated keys keep their
// meaning. A finished game is marked pending in the transaction that ends it,
// and counted, or skipped, after its rated result if it has one.
const (
	CountPending = "pending"
	CountCounted = "counted"
	CountSkipped = "skipped" // a guest or bot at a seat, or a seat that never moved
)

// The keys of the lasting counts.
const (
	// CountGame prefixes the results of every game that counts:
	// "game.played:hex", "game.won:hex", "game.drawn:hex", "game.lost:hex".
	CountGame = "game."
	// CountFact prefixes the facts about a seat: "fact.spellbook:cthulhu-wars".
	CountFact = "fact."
	// CountMet and CountMetRated prefix the opponents an account has played,
	// in any game that counts and in rated games: "met:42", "met.rated:42".
	CountMet      = "met:"
	CountMetRated = "met.rated:"
)

// MetKey is the count of the games an account has played against another.
func MetKey(opponent int64, rated bool) string {
	if rated {
		return CountMetRated + strconv.FormatInt(opponent, 10)
	}
	return CountMet + strconv.FormatInt(opponent, 10)
}

// PendingCountGames lists the finished games still to be counted whose rated
// result, if any, has been written.
func (s *Store) PendingCountGames(ctx context.Context) ([]Game, error) {
	rows, err := s.query(ctx, "PendingCountGames",
		`SELECT `+gameColumns+` FROM games WHERE count_status = ? AND rating_status != ?`, CountPending, RatingPending)
	if err != nil {
		return nil, fmt.Errorf("listing games to count: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Game
	for rows.Next() {
		g, err := scanGameRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning a game to count: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SkipCount says a pending game counts for nobody. The awards it gave during
// play that waited for its end are shown, as RecordCounts shows them.
func (s *Store) SkipCount(ctx context.Context, gameID int64, now time.Time) error {
	tx, err := s.begin(ctx, "SkipCount")
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.exec(ctx, "SkipCount", `UPDATE games SET count_status = ? WHERE id = ? AND count_status = ?`, CountSkipped, gameID, CountPending); err != nil {
		return fmt.Errorf("skipping the count of game_id=%d: %w", gameID, err)
	}
	if err := revealAwards(ctx, tx, gameID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// RecordCounts adds counts, per account and key, writes the awards the game's
// end earned, shows the awards that waited for the end, and marks the game
// counted, in one transaction. A game no longer
// pending is left alone and nothing is added, so counting twice adds once; an
// account that already holds a badge keeps the award it had.
func (s *Store) RecordCounts(ctx context.Context, gameID int64, counts map[int64]map[string]int, awards []Award, now time.Time) error {
	tx, err := s.begin(ctx, "RecordCounts")
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.exec(ctx, "RecordCounts.status", `UPDATE games SET count_status = ? WHERE id = ? AND count_status = ?`, CountCounted, gameID, CountPending)
	if err != nil {
		return fmt.Errorf("marking game_id=%d counted: %w", gameID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	for account, keys := range counts {
		for key, n := range keys {
			if n == 0 {
				continue
			}
			if _, err := tx.exec(ctx, "RecordCounts.stat",
				`INSERT INTO stats (account_id, key, value) VALUES (?, ?, ?) ON CONFLICT (account_id, key) DO UPDATE SET value = value + excluded.value`,
				account, key, n); err != nil {
				return fmt.Errorf("writing a count: %w", err)
			}
		}
	}
	for _, a := range awards {
		if err := insertAward(ctx, tx, a); err != nil {
			return err
		}
	}
	if err := revealAwards(ctx, tx, gameID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// GameFacts is the facts of every selection of a game, as stored (JSON), in
// the order they were played.
func (s *Store) GameFacts(ctx context.Context, gameID int64) ([]string, error) {
	rows, err := s.query(ctx, "GameFacts", `SELECT facts FROM moves WHERE game_id = ? AND facts IS NOT NULL ORDER BY seq`, gameID)
	if err != nil {
		return nil, fmt.Errorf("reading the facts of game_id=%d: %w", gameID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var f sql.NullString
		if err := rows.Scan(&f); err != nil {
			return nil, fmt.Errorf("scanning facts: %w", err)
		}
		out = append(out, f.String)
	}
	return out, rows.Err()
}
