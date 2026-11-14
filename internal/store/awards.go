package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Award is a badge an account has earned (ADR 0130), with the badge's name and
// description as they were when it was earned.
type Award struct {
	AccountID   int64
	Badge       string
	Name        string
	Description string
	Family      string
	GameID      int64
	RoomCode    string
	Detail      string
	AwardedAt   time.Time
	// Visible says the award is shown to everyone; one that waits for its
	// game to end is shown only to its holder until then.
	Visible bool
}

func insertAward(ctx context.Context, tx *Tx, a Award) error {
	var visible sql.NullString
	if a.Visible {
		visible = sql.NullString{String: stamp(a.AwardedAt), Valid: true}
	}
	if _, err := tx.exec(ctx, "insertAward",
		`INSERT INTO badge_awards (account_id, badge, name, description, family, game_id, room_code, detail, awarded_at, visible_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (account_id, badge) DO NOTHING`,
		a.AccountID, a.Badge, a.Name, a.Description, a.Family, a.GameID, a.RoomCode, a.Detail, stamp(a.AwardedAt), visible); err != nil {
		return fmt.Errorf("writing an award: %w", err)
	}
	return nil
}

// Awards lists an account's awards, oldest first: every one when all is
// true (the holder's own view), and otherwise only those shown to everyone.
func (s *Store) Awards(ctx context.Context, accountID int64, all bool) ([]Award, error) {
	q := `SELECT account_id, badge, name, description, family, game_id, room_code, detail, awarded_at, visible_at
	      FROM badge_awards WHERE account_id = ?`
	if !all {
		q += ` AND visible_at IS NOT NULL`
	}
	rows, err := s.query(ctx, "Awards", q+` ORDER BY awarded_at, id`, accountID)
	if err != nil {
		return nil, fmt.Errorf("reading awards: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Award
	for rows.Next() {
		var a Award
		var at string
		var visible sql.NullString
		if err := rows.Scan(&a.AccountID, &a.Badge, &a.Name, &a.Description, &a.Family, &a.GameID, &a.RoomCode, &a.Detail, &at, &visible); err != nil {
			return nil, fmt.Errorf("scanning an award: %w", err)
		}
		a.AwardedAt, _ = time.Parse(sqlTime, at)
		a.Visible = visible.Valid
		out = append(out, a)
	}
	return out, rows.Err()
}
