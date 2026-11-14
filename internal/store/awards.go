package store

import (
	"context"
	"database/sql"
	"errors"
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

// revealAwards shows everyone the awards of a game that waited for its end.
func revealAwards(ctx context.Context, tx *Tx, gameID int64, now time.Time) error {
	if _, err := tx.exec(ctx, "revealAwards", `UPDATE badge_awards SET visible_at = ? WHERE game_id = ? AND visible_at IS NULL`, stamp(now), gameID); err != nil {
		return fmt.Errorf("showing the awards of game_id=%d: %w", gameID, err)
	}
	return nil
}

// BadgesDue is a game with moves that reported facts since its badges were
// last checked during play, and the last of those moves.
type BadgesDue struct {
	Game Game
	Seq  int
}

// GamesWithBadgesDue lists the games whose badges are to be checked during
// play (ADR 0130): those with a move that reported facts past the last move
// the check looked at.
func (s *Store) GamesWithBadgesDue(ctx context.Context) ([]BadgesDue, error) {
	rows, err := s.query(ctx, "GamesWithBadgesDue", `SELECT id, facts_seq FROM games WHERE facts_seq > badges_seq`)
	if err != nil {
		return nil, fmt.Errorf("listing games with badges due: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	var seqs []int
	for rows.Next() {
		var id int64
		var seq int
		if err := rows.Scan(&id, &seq); err != nil {
			return nil, fmt.Errorf("scanning a game with badges due: %w", err)
		}
		ids, seqs = append(ids, id), append(seqs, seq)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = rows.Close()
	out := make([]BadgesDue, 0, len(ids))
	for i, id := range ids {
		g, err := scanGameByID(ctx, s, "GamesWithBadgesDue.game", id)
		if errors.Is(err, sql.ErrNoRows) {
			continue // deleted meanwhile
		}
		if err != nil {
			return nil, fmt.Errorf("reading game_id=%d: %w", id, err)
		}
		out = append(out, BadgesDue{Game: g, Seq: seqs[i]})
	}
	return out, nil
}

// RecordDuring writes the awards a game earned during play and says its
// badges have been checked up to move seq, in one transaction, so a check
// that is repeated awards once. An award not yet visible is shown at once if
// the game has already ended, since nothing will show it later.
func (s *Store) RecordDuring(ctx context.Context, gameID int64, seq int, awards []Award) error {
	tx, err := s.begin(ctx, "RecordDuring")
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// The watermark is written first, which takes the write lock before the
	// game's status is read. An undo may have moved facts_seq back meanwhile.
	if _, err := tx.exec(ctx, "RecordDuring.seq", `UPDATE games SET badges_seq = MIN(?, facts_seq) WHERE id = ?`, seq, gameID); err != nil {
		return fmt.Errorf("moving the badges watermark of game_id=%d: %w", gameID, err)
	}
	var countStatus string
	if err := tx.queryRow(ctx, "RecordDuring.status", `SELECT count_status FROM games WHERE id = ?`, gameID).Scan(&countStatus); err != nil {
		return fmt.Errorf("reading the status of game_id=%d: %w", gameID, err)
	}
	for _, a := range awards {
		if countStatus != "" && countStatus != CountPending {
			a.Visible = true
		}
		if err := insertAward(ctx, tx, a); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
