package store

import (
	"context"
	"strings"
	"time"
)

// MarkTutorialDone records that an account has finished a tutorial ("family/id").
// Finishing it again keeps the first time.
func (s *Store) MarkTutorialDone(ctx context.Context, accountID int64, ref string, at time.Time) error {
	_, err := s.exec(ctx, "MarkTutorialDone",
		`INSERT INTO tutorials_done (account_id, tutorial, done_at) VALUES (?, ?, ?) ON CONFLICT (account_id, tutorial) DO NOTHING`,
		accountID, ref, stamp(at))
	return err
}

// TutorialsDone returns the ids of the tutorials of a family an account has finished.
func (s *Store) TutorialsDone(ctx context.Context, accountID int64, family string) (map[string]bool, error) {
	rows, err := s.query(ctx, "TutorialsDone",
		`SELECT tutorial FROM tutorials_done WHERE account_id = ? AND substr(tutorial, 1, ?) = ?`,
		accountID, len(family)+1, family+"/")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, err
		}
		out[strings.TrimPrefix(ref, family+"/")] = true
	}
	return out, rows.Err()
}
