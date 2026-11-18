package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// A game of patience is scored by the moves it was won in, the fewer the
// better (ADR 0135). The results worker writes an account's score with the
// game's counts, so a game is scored once, and the best is a lookup.

// Score is one account's score in one finished game.
type Score struct {
	GameID    int64
	AccountID int64
	// GameDefID is the game played, a variant by its own id.
	GameDefID string
	Score     int
	RoomCode  string
	ScoredAt  time.Time
}

func insertScores(ctx context.Context, tx *Tx, scores []Score) error {
	for _, sc := range scores {
		if _, err := tx.exec(ctx, "RecordCounts.score",
			`INSERT INTO scores (game_id, account_id, game_def_id, score, room_code, scored_at) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (game_id, account_id) DO NOTHING`,
			sc.GameID, sc.AccountID, sc.GameDefID, sc.Score, sc.RoomCode, sc.ScoredAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("writing a score: %w", err)
		}
	}
	return nil
}

// BestScore is an account's lowest score in a game, and the game it was made
// in; ok is false when the account has none there.
func (s *Store) BestScore(ctx context.Context, accountID int64, gameDefID string) (Score, bool, error) {
	row := s.queryRow(ctx, "BestScore",
		`SELECT game_id, account_id, game_def_id, score, room_code, scored_at FROM scores WHERE account_id = ? AND game_def_id = ? ORDER BY score, scored_at LIMIT 1`,
		accountID, gameDefID)
	var sc Score
	var at string
	if err := row.Scan(&sc.GameID, &sc.AccountID, &sc.GameDefID, &sc.Score, &sc.RoomCode, &at); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Score{}, false, nil
		}
		return Score{}, false, fmt.Errorf("reading the best score of account_id=%d in %s: %w", accountID, gameDefID, err)
	}
	sc.ScoredAt, _ = time.Parse(time.RFC3339Nano, at)
	return sc, true, nil
}

// Deal is one finished game of patience an account played, for its deal
// history (ADR 0136): the seed, how it ended and the position it ended in.
type Deal struct {
	RoomCode    string
	Seed        string
	StartChosen bool
	Won         bool
	BoardState  string
	Ply         int
	EndedAt     string
}

// AccountDeals lists the finished games of gameDefID that accountID played
// and that have a seed, the latest first, at most limit of them.
func (s *Store) AccountDeals(ctx context.Context, accountID int64, gameDefID string, limit int) ([]Deal, error) {
	rows, err := s.query(ctx, "AccountDeals",
		`SELECT DISTINCT g.id, g.room_code, g.seed, g.start_chosen, COALESCE(g.winner_seat = p.seat, 0), g.board_state, g.ply, g.updated_at
		 FROM games g JOIN players p ON p.game_id = g.id JOIN principals pr ON pr.id = p.principal_id
		 WHERE pr.account_id = ? AND pr.kind = ? AND g.game_def_id = ? AND g.status = 'finished' AND g.seed != '' AND g.tutorial = ''
		 ORDER BY g.updated_at DESC, g.id DESC LIMIT ?`,
		accountID, KindAccount, gameDefID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing the deals of account_id=%d in %s: %w", accountID, gameDefID, err)
	}
	defer func() { _ = rows.Close() }()
	out := []Deal{}
	for rows.Next() {
		var d Deal
		var id int64
		var chosen, won int
		if err := rows.Scan(&id, &d.RoomCode, &d.Seed, &chosen, &won, &d.BoardState, &d.Ply, &d.EndedAt); err != nil {
			return nil, err
		}
		d.StartChosen, d.Won = chosen != 0, won != 0
		out = append(out, d)
	}
	return out, rows.Err()
}
