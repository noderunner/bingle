package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrBatchNotFound is returned when a batch does not exist.
var ErrBatchNotFound = errors.New("store: batch not found")

// Batch statuses.
const (
	BatchQueued      = "queued"
	BatchRunning     = "running"
	BatchDone        = "done"
	BatchCancelled   = "cancelled"
	BatchFailed      = "failed"
	BatchInterrupted = "interrupted"
)

// Batch is a bot practice batch.
type Batch struct {
	ID         int64
	Name       string
	GameDefID  string
	Spec       string // JSON
	Status     string
	NGames     int
	Done       int
	Error      string
	CreatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// BatchGame is one game of a batch.
type BatchGame struct {
	BatchID   int64
	Index     int
	Seed      int64
	GameID    string
	Seating   string // JSON list of cult keys
	Winner    int
	EndReason string
	Plies     int
	Rounds    int
	Metrics   string // JSON
	Events    string // JSON, "" when not kept
}

// CreateBatch stores a new queued batch.
func (s *Store) CreateBatch(ctx context.Context, name, gameDefID, spec string, nGames int) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO batches (name, game_def_id, spec, n_games) VALUES (?, ?, ?, ?)`,
		name, gameDefID, spec, nGames)
	if err != nil {
		return 0, fmt.Errorf("creating batch: %w", err)
	}
	return res.LastInsertId()
}

// SetBatchStatus moves a batch to a status; running stamps started_at and the
// final statuses finished_at.
func (s *Store) SetBatchStatus(ctx context.Context, id int64, status, errText string) error {
	q := `UPDATE batches SET status = ?, error = ? WHERE id = ?`
	switch status {
	case BatchRunning:
		q = `UPDATE batches SET status = ?, error = ?, started_at = CURRENT_TIMESTAMP WHERE id = ?`
	case BatchDone, BatchCancelled, BatchFailed, BatchInterrupted:
		q = `UPDATE batches SET status = ?, error = ?, finished_at = CURRENT_TIMESTAMP WHERE id = ?`
	}
	if _, err := s.db.ExecContext(ctx, q, status, errText, id); err != nil {
		return fmt.Errorf("setting batch %d status: %w", id, err)
	}
	return nil
}

// InterruptBatches marks the batches a stopped server left queued or running.
func (s *Store) InterruptBatches(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE batches SET status = 'interrupted', finished_at = CURRENT_TIMESTAMP WHERE status IN ('queued', 'running')`)
	return err
}

// AddBatchGame stores a finished game and counts it in its batch.
func (s *Store) AddBatchGame(ctx context.Context, g BatchGame) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`INSERT OR REPLACE INTO batch_games (batch_id, idx, seed, game_id, seating, winner, end_reason, plies, rounds, metrics, events)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.BatchID, g.Index, g.Seed, g.GameID, g.Seating, g.Winner, g.EndReason, g.Plies, g.Rounds, g.Metrics, g.Events); err != nil {
		return fmt.Errorf("storing batch game: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE batches SET done = (SELECT COUNT(*) FROM batch_games WHERE batch_id = ?) WHERE id = ?`, g.BatchID, g.BatchID); err != nil {
		return err
	}
	return tx.Commit()
}

const batchCols = `id, name, game_def_id, spec, status, n_games, done, error, created_at, started_at, finished_at`

func scanBatch(sc interface{ Scan(...any) error }) (Batch, error) {
	var b Batch
	var started, finished sql.NullTime
	if err := sc.Scan(&b.ID, &b.Name, &b.GameDefID, &b.Spec, &b.Status, &b.NGames, &b.Done, &b.Error, &b.CreatedAt, &started, &finished); err != nil {
		return Batch{}, err
	}
	if started.Valid {
		b.StartedAt = &started.Time
	}
	if finished.Valid {
		b.FinishedAt = &finished.Time
	}
	return b, nil
}

// GetBatch returns one batch.
func (s *Store) GetBatch(ctx context.Context, id int64) (Batch, error) {
	b, err := scanBatch(s.db.QueryRowContext(ctx, `SELECT `+batchCols+` FROM batches WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Batch{}, ErrBatchNotFound
	}
	if err != nil {
		return Batch{}, fmt.Errorf("reading batch %d: %w", id, err)
	}
	return b, nil
}

// ListBatches returns the batches, newest first.
func (s *Store) ListBatches(ctx context.Context, limit int) ([]Batch, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+batchCols+` FROM batches ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing batches: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Batch
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BatchGames returns a batch's games in order; withEvents also loads the
// (large) event lists.
func (s *Store) BatchGames(ctx context.Context, id int64, withEvents bool) ([]BatchGame, error) {
	ev := `''`
	if withEvents {
		ev = `events`
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT idx, seed, game_id, seating, winner, end_reason, plies, rounds, metrics, `+ev+` FROM batch_games WHERE batch_id = ? ORDER BY idx`, id)
	if err != nil {
		return nil, fmt.Errorf("listing games of batch %d: %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	var out []BatchGame
	for rows.Next() {
		g := BatchGame{BatchID: id}
		if err := rows.Scan(&g.Index, &g.Seed, &g.GameID, &g.Seating, &g.Winner, &g.EndReason, &g.Plies, &g.Rounds, &g.Metrics, &g.Events); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// BatchGame returns one game of a batch with its events.
func (s *Store) BatchGame(ctx context.Context, id int64, index int) (BatchGame, error) {
	g := BatchGame{BatchID: id}
	err := s.db.QueryRowContext(ctx,
		`SELECT idx, seed, game_id, seating, winner, end_reason, plies, rounds, metrics, events FROM batch_games WHERE batch_id = ? AND idx = ?`, id, index).
		Scan(&g.Index, &g.Seed, &g.GameID, &g.Seating, &g.Winner, &g.EndReason, &g.Plies, &g.Rounds, &g.Metrics, &g.Events)
	if errors.Is(err, sql.ErrNoRows) {
		return BatchGame{}, ErrBatchNotFound
	}
	return g, err
}

// DeleteBatch removes a batch and its games.
func (s *Store) DeleteBatch(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM batches WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting batch %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrBatchNotFound
	}
	return nil
}
