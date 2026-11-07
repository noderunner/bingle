package store

import (
	"context"
	"fmt"
	"strings"
)

// UpgradeStates rewrites the stored positions of the games whose definition
// ids are given (and the games derived from them, id~suffix): each game's
// board and initial state and every move's resulting state. upgrade gets a
// game's definition id and a document, and returns the document to store and
// whether it changed; it returns how many documents changed. A module
// upgrades its positions once its rules read them in a new form
// (catalog.Module.Upgrade), and a document already in that form is left
// alone, so it may run at every start.
func (s *Store) UpgradeStates(ctx context.Context, ids []string, upgrade func(defID string, doc []byte) ([]byte, bool)) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var where []string
	var args []any
	for _, id := range ids {
		where = append(where, "g.game_def_id = ? OR g.game_def_id LIKE ?")
		args = append(args, id, id+"~%")
	}
	type doc struct {
		table, column string
		id            int64
		def, text     string
	}
	var docs []doc
	read := func(name, q, table, column string) error {
		rows, err := s.query(ctx, name, q, args...)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			d := doc{table: table, column: column}
			if err := rows.Scan(&d.id, &d.def, &d.text); err != nil {
				return err
			}
			docs = append(docs, d)
		}
		return rows.Err()
	}
	cond := strings.Join(where, " OR ")
	// Most starts have nothing to convert, and a server that has none of
	// these games should not grow a statement in its metrics for looking:
	// this one check is not recorded (see queryRowUnmeasured).
	var found bool
	if err := s.queryRowUnmeasured(ctx, `SELECT EXISTS (SELECT 1 FROM games g WHERE `+cond+`)`, args...).Scan(&found); err != nil {
		return 0, fmt.Errorf("store: upgrade states: %w", err)
	}
	if !found {
		return 0, nil
	}
	if err := read("UpgradeStates", `SELECT g.id, g.game_def_id, g.board_state FROM games g WHERE `+cond, "games", "board_state"); err != nil {
		return 0, fmt.Errorf("store: upgrade states: %w", err)
	}
	if err := read("UpgradeStates", `SELECT g.id, g.game_def_id, g.initial_state FROM games g WHERE `+cond, "games", "initial_state"); err != nil {
		return 0, fmt.Errorf("store: upgrade states: %w", err)
	}
	if err := read("UpgradeStates", `SELECT m.id, g.game_def_id, m.resulting_state FROM moves m JOIN games g ON g.id = m.game_id WHERE `+cond, "moves", "resulting_state"); err != nil {
		return 0, fmt.Errorf("store: upgrade states: %w", err)
	}
	tx, err := s.begin(ctx, "UpgradeStates")
	if err != nil {
		return 0, fmt.Errorf("store: upgrade states: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	changed := 0
	for _, d := range docs {
		if d.text == "" {
			continue
		}
		out, ok := upgrade(d.def, []byte(d.text))
		if !ok {
			continue
		}
		// The table and column come from the three reads above, never from data.
		if _, err := tx.exec(ctx, "UpgradeStates", `UPDATE `+d.table+` SET `+d.column+` = ? WHERE id = ?`, string(out), d.id); err != nil {
			return 0, fmt.Errorf("store: upgrade states: %w", err)
		}
		changed++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: upgrade states: %w", err)
	}
	return changed, nil
}
