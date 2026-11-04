package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// SetupProfile is a setup an account saved. Options is the JSON of the options
// of a create call.
type SetupProfile struct {
	ID        int64
	AccountID int64
	Owner     string // the account's handle
	Family    string
	Name      string
	GameDefID string
	Options   string
	Shared    bool
}

const setupProfileColumns = `p.id, p.account_id, a.handle, p.family, p.name, p.game_def_id, p.options, p.shared`

func scanSetupProfile(scan func(...any) error) (SetupProfile, error) {
	var p SetupProfile
	var shared int
	err := scan(&p.ID, &p.AccountID, &p.Owner, &p.Family, &p.Name, &p.GameDefID, &p.Options, &shared)
	p.Shared = shared != 0
	return p, err
}

// SaveSetupProfile stores p, replacing what its account saved under the same
// name (whatever its case), and returns its id.
func (s *Store) SaveSetupProfile(ctx context.Context, p SetupProfile) (int64, error) {
	shared := 0
	if p.Shared {
		shared = 1
	}
	var id int64
	err := s.queryRow(ctx, "SaveSetupProfile", `
		INSERT INTO setup_profiles (account_id, family, name, name_lower, game_def_id, options, shared)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (account_id, family, name_lower) DO UPDATE SET
			name = excluded.name, game_def_id = excluded.game_def_id, options = excluded.options,
			shared = excluded.shared, updated_at = CURRENT_TIMESTAMP
		RETURNING id`,
		p.AccountID, p.Family, p.Name, strings.ToLower(p.Name), p.GameDefID, p.Options, shared).Scan(&id)
	return id, err
}

// CountSetupProfiles counts what an account saved for a family, and says whether
// one of them already has the name.
func (s *Store) CountSetupProfiles(ctx context.Context, accountID int64, family, name string) (n int, exists bool, err error) {
	var same int
	err = s.queryRow(ctx, "CountSetupProfiles", `
		SELECT COUNT(*), COALESCE(SUM(name_lower = ?), 0) FROM setup_profiles WHERE account_id = ? AND family = ?`,
		strings.ToLower(name), accountID, family).Scan(&n, &same)
	return n, same > 0, err
}

// SetupProfiles lists a family's setups that account (0: a guest) may see: its
// own, by name, then at most sharedLimit others' shared ones, newest first.
func (s *Store) SetupProfiles(ctx context.Context, family string, account int64, sharedLimit int) ([]SetupProfile, error) {
	rows, err := s.query(ctx, "SetupProfiles", `
		SELECT `+setupProfileColumns+` FROM setup_profiles p JOIN accounts a ON a.id = p.account_id
		WHERE p.family = ? AND p.account_id = ? AND a.deleted_at IS NULL
		ORDER BY p.name_lower`, family, account)
	if err != nil {
		return nil, err
	}
	out, err := readSetupProfiles(rows)
	if err != nil {
		return nil, err
	}
	rows, err = s.query(ctx, "SetupProfilesShared", `
		SELECT `+setupProfileColumns+` FROM setup_profiles p JOIN accounts a ON a.id = p.account_id
		WHERE p.family = ? AND p.shared = 1 AND p.account_id != ? AND a.deleted_at IS NULL
		ORDER BY p.updated_at DESC, p.id DESC LIMIT ?`, family, account, sharedLimit)
	if err != nil {
		return nil, err
	}
	shared, err := readSetupProfiles(rows)
	if err != nil {
		return nil, err
	}
	return append(out, shared...), nil
}

func readSetupProfiles(rows *Rows) ([]SetupProfile, error) {
	defer func() { _ = rows.Close() }()
	var out []SetupProfile
	for rows.Next() {
		p, err := scanSetupProfile(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetupProfile reads one saved setup.
func (s *Store) SetupProfile(ctx context.Context, id int64) (SetupProfile, error) {
	p, err := scanSetupProfile(s.queryRow(ctx, "SetupProfile", `
		SELECT `+setupProfileColumns+` FROM setup_profiles p JOIN accounts a ON a.id = p.account_id WHERE p.id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// DeleteSetupProfile removes a saved setup.
func (s *Store) DeleteSetupProfile(ctx context.Context, id int64) error {
	_, err := s.exec(ctx, "DeleteSetupProfile", `DELETE FROM setup_profiles WHERE id = ?`, id)
	return err
}
