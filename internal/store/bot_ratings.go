package store

import (
	"context"
	"fmt"
	"time"
)

// BotRating is a built-in bot's rating in a game family, on the bot ladder
// (ADR 0160). It lives in a table of its own, apart from people's ratings.
type BotRating struct {
	Bot        string
	Family     string
	FamilyName string
	Rating     float64
	Deviation  float64
	Volatility float64
	Games      int
	Won        int
	Drawn      int
	Lost       int
	UpdatedAt  time.Time
}

// BotRatings lists the bot ladder: every family's bots, the families by name
// and each family's bots the highest rating first.
func (s *Store) BotRatings(ctx context.Context) ([]BotRating, error) {
	rows, err := s.query(ctx, "BotRatings",
		`SELECT bot, family, family_name, rating, deviation, volatility, games, won, drawn, lost, updated_at
		 FROM bot_ratings ORDER BY family_name, family, rating DESC, bot`)
	if err != nil {
		return nil, fmt.Errorf("listing the bot ladder: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []BotRating
	for rows.Next() {
		var r BotRating
		var at string
		if err := rows.Scan(&r.Bot, &r.Family, &r.FamilyName, &r.Rating, &r.Deviation, &r.Volatility, &r.Games, &r.Won, &r.Drawn, &r.Lost, &at); err != nil {
			return nil, fmt.Errorf("scanning a bot's rating: %w", err)
		}
		r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetBotRatings writes a run of the bot ladder in one transaction: each row
// replaces that bot's rating in that family, games and results included.
func (s *Store) SetBotRatings(ctx context.Context, list []BotRating) error {
	tx, err := s.begin(ctx, "SetBotRatings")
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, r := range list {
		if _, err := tx.exec(ctx, "SetBotRatings.row",
			`INSERT INTO bot_ratings (bot, family, family_name, rating, deviation, volatility, games, won, drawn, lost, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (bot, family) DO UPDATE SET family_name = excluded.family_name, rating = excluded.rating,
			   deviation = excluded.deviation, volatility = excluded.volatility, games = excluded.games,
			   won = excluded.won, drawn = excluded.drawn, lost = excluded.lost, updated_at = excluded.updated_at`,
			r.Bot, r.Family, r.FamilyName, r.Rating, r.Deviation, r.Volatility, r.Games, r.Won, r.Drawn, r.Lost,
			r.UpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("writing a bot's rating: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
