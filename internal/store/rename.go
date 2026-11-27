package store

import (
	"context"
	"fmt"
)

// GameRename is a game whose id changed: what was stored under From (and the
// games derived from it, From~suffix, and its tutorial rooms, From/id) moves
// to To, and what named its family by name is named Name.
type GameRename struct {
	From, To, Name string
}

// RenameGames moves everything stored under the former id of each game to its
// id now, in one transaction: the games, their batches, tournaments, setup
// profiles, scores, ratings and rating history, badges and the versions seen.
// It finds nothing to do once it has run. It returns how many rows moved.
func (s *Store) RenameGames(ctx context.Context, renames []GameRename) (int64, error) {
	if len(renames) == 0 {
		return 0, nil
	}
	tx, err := s.begin(ctx, "RenameGames")
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var total int64
	run := func(query string, args ...any) error {
		res, err := tx.exec(ctx, "RenameGames", query, args...)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		total += n
		return nil
	}
	for _, r := range renames {
		// A stored game id is From itself or From~suffix: the new one keeps
		// the suffix.
		if err := run(`UPDATE tournaments SET game_def_id = ? || substr(game_def_id, ?), game_name = ? WHERE game_def_id = ? OR game_def_id GLOB ?`,
			r.To, len(r.From)+1, r.Name, r.From, r.From+"~*"); err != nil {
			return 0, fmt.Errorf("renaming %q in tournaments: %w", r.From, err)
		}
		for _, table := range []string{"games", "batches", "setup_profiles", "scores", "rating_history"} {
			if err := run(fmt.Sprintf(`UPDATE %s SET game_def_id = ? || substr(game_def_id, ?) WHERE game_def_id = ? OR game_def_id GLOB ?`, table),
				r.To, len(r.From)+1, r.From, r.From+"~*"); err != nil {
				return 0, fmt.Errorf("renaming %q in %s: %w", r.From, table, err)
			}
		}
		for _, stmt := range []string{
			`UPDATE games SET tutorial = ? || substr(tutorial, ?) WHERE tutorial GLOB ?`,
			`UPDATE tutorials_done SET tutorial = ? || substr(tutorial, ?) WHERE tutorial GLOB ?`,
		} {
			if err := run(stmt, r.To, len(r.From)+1, r.From+"/*"); err != nil {
				return 0, fmt.Errorf("renaming the tutorials of %q: %w", r.From, err)
			}
		}
		for _, table := range []string{"ratings", "rating_history"} {
			if err := run(fmt.Sprintf(`UPDATE %s SET family = ?, family_name = ? WHERE family = ?`, table), r.To, r.Name, r.From); err != nil {
				return 0, fmt.Errorf("renaming %q in %s: %w", r.From, table, err)
			}
		}
		for _, table := range []string{"setup_profiles", "badge_awards", "game_versions"} {
			if err := run(fmt.Sprintf(`UPDATE %s SET family = ? WHERE family = ?`, table), r.To, r.From); err != nil {
				return 0, fmt.Errorf("renaming %q in %s: %w", r.From, table, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return total, nil
}
