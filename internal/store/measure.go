package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"modernc.org/sqlite"

	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/trace"
)

// Every statement in this package goes through one of the four helpers below —
// exec, query, queryRow and begin — and every one of them carries a *name*.
// The name is what the metrics are grouped by, so it has to be a closed set:
// it is the method the statement belongs to ("GetGame"), with a short suffix
// when a method runs more than one ("RecordMove.insertMove"). A name is never
// built from data.
//
// Nothing in this package calls s.db.ExecContext and friends directly. That is
// not a convention to remember: tests/store/wrapper_contract_test.go reads
// this package's source and fails if a new call site bypasses a helper, the
// way the DOM-contract tests pin the interface the browser tests rely on.
//
// Stage 3 of docs/history/observability-plan.md.

// SetMetrics hands the store the registry to record into. Without it every
// helper below still runs the statement and records nothing, which is what a
// server with BINGLE_METRICS_ADDR empty does and what most tests want.
func (s *Store) SetMetrics(m *metrics.Metrics) { s.mx = m }

// exec runs a statement that returns no rows.
func (s *Store) exec(ctx context.Context, name, query string, args ...any) (sql.Result, error) {
	ctx, span := trace.Start(ctx, "store")
	span.Set("stmt", name)
	defer span.End()

	start := time.Now()
	res, err := s.db.ExecContext(ctx, query, args...)
	s.record(name, start, affected(res), err)
	return res, err
}

// query runs a statement that returns rows. The duration covers reading them
// as well as asking for them, because for a list that is the number that
// matters, and the rows are counted as the caller takes them.
func (s *Store) query(ctx context.Context, name, query string, args ...any) (*Rows, error) {
	// The span ends when the rows are closed, so it covers reading them.
	ctx, span := trace.Start(ctx, "store")
	span.Set("stmt", name)

	start := time.Now()
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		s.record(name, start, 0, err)
		span.End()
		return nil, err
	}
	return &Rows{Rows: rows, s: s, name: name, start: start, span: span}, nil
}

// queryRow runs a statement that returns one row. What happened is only known
// once the caller has scanned it, so that is where it is recorded.
func (s *Store) queryRow(ctx context.Context, name, query string, args ...any) *Row {
	ctx, span := trace.Start(ctx, "store")
	span.Set("stmt", name)

	start := time.Now()
	return &Row{row: s.db.QueryRowContext(ctx, query, args...), s: s, name: name, start: start, span: span}
}

// queryRowUnmeasured runs a statement that is deliberately not recorded. Its
// only caller is the snapshot in stats.go: those statements exist *because*
// something is watching, so counting them would make
// bingle_db_queries_total partly a measure of the monitoring rather than of
// bingle, and would add a statement name per scrape to a label that is
// supposed to be a closed set.
func (s *Store) queryRowUnmeasured(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

// begin starts a transaction. name is the transaction as a whole, for
// bingle_db_tx_duration_seconds; the statements inside it carry their own.
func (s *Store) begin(ctx context.Context, name string) (*Tx, error) {
	// The span ends with the transaction, so everything inside it nests.
	ctx, span := trace.Start(ctx, "store.tx")
	span.Set("tx", name)

	start := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.record(name+".begin", start, 0, err)
		span.End()
		return nil, err
	}
	return &Tx{tx: tx, s: s, ctx: ctx, start: time.Now(), span: span}, nil
}

// Tx is a transaction whose statements are measured. Its Commit and Rollback
// are what record how long the transaction held open.
type Tx struct {
	tx *sql.Tx
	s  *Store
	// ctx carries the transaction's span, so the statements inside it nest
	// under it however the caller passes its own context around.
	ctx   context.Context
	start time.Time
	span  trace.Span
	ended bool
}

func (t *Tx) exec(ctx context.Context, name, query string, args ...any) (sql.Result, error) {
	_, span := trace.Start(t.spanCtx(ctx), "store")
	span.Set("stmt", name)
	defer span.End()

	start := time.Now()
	res, err := t.tx.ExecContext(ctx, query, args...)
	t.s.record(name, start, affected(res), err)
	return res, err
}

func (t *Tx) queryRow(ctx context.Context, name, query string, args ...any) *Row {
	_, span := trace.Start(t.spanCtx(ctx), "store")
	span.Set("stmt", name)

	start := time.Now()
	return &Row{row: t.tx.QueryRowContext(ctx, query, args...), s: t.s, name: name, start: start, span: span}
}

// spanCtx is the context the transaction's statements nest their spans under.
func (t *Tx) spanCtx(ctx context.Context) context.Context {
	if t.ctx != nil {
		return t.ctx
	}
	return ctx
}

// Commit ends the transaction, and records how long it was open.
func (t *Tx) Commit() error {
	err := t.tx.Commit()
	t.end()
	return err
}

// Rollback ends the transaction. Every caller defers it, so it runs on the way
// out of a committed transaction too and must not record that one twice.
func (t *Tx) Rollback() error {
	err := t.tx.Rollback()
	t.end()
	return err
}

func (t *Tx) end() {
	if t.ended {
		return
	}
	t.ended = true
	t.s.mx.DBTransaction(time.Since(t.start))
	t.span.End()
}

// Row is one row, measured when it is scanned.
type Row struct {
	row   *sql.Row
	s     *Store
	name  string
	start time.Time
	span  trace.Span
	done  bool
}

// Scan reads the row, and records the statement.
func (r *Row) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	if !r.done {
		r.done = true
		r.span.End()
		rows := int64(1)
		if err != nil {
			rows = 0
		}
		r.s.record(r.name, r.start, rows, err)
	}
	return err
}

// Rows are rows being read, counted as the caller takes them. It is an
// *sql.Rows in every respect except Next, which counts, and Close, which
// records.
type Rows struct {
	*sql.Rows
	s     *Store
	name  string
	start time.Time
	span  trace.Span
	n     int64
	done  bool
}

// Next advances to the next row, counting it.
func (r *Rows) Next() bool {
	ok := r.Rows.Next()
	if ok {
		r.n++
	}
	return ok
}

// Close finishes with the rows, and records the statement. Every caller defers
// it, which is what makes it the right place to record from.
func (r *Rows) Close() error {
	err := r.Rows.Close()
	if !r.done {
		r.done = true
		r.span.End()
		// Err, not err: a read that stopped halfway is the interesting
		// failure, and Close itself rarely has anything to say.
		r.s.record(r.name, r.start, r.n, firstError(r.Err(), err))
	}
	return err
}

// record reports one statement. A nil registry makes this a few nil checks.
func (s *Store) record(name string, start time.Time, rows int64, err error) {
	if s.mx == nil {
		return
	}
	result := metrics.DBOK
	switch {
	case err == nil:
	case errors.Is(err, sql.ErrNoRows):
		// Not a fault: "is there a row for this?" is a question half these
		// statements exist to ask.
		result = metrics.DBNoRows
	case isBusy(err):
		result = metrics.DBBusy
		s.mx.DBBusy()
	default:
		result = metrics.DBError
	}
	s.mx.DBStatement(name, result, time.Since(start), rows)
}

// isBusy reports whether SQLite could not get the lock it wanted. The DSN sets
// busy_timeout, so this means the statement waited the whole ten seconds and
// still lost — the first thing to look at when writes start failing under
// load.
func isBusy(err error) bool {
	var e *sqlite.Error
	if !errors.As(err, &e) {
		return false
	}
	// The low byte is the primary result code; the rest is the extended one
	// (SQLITE_BUSY_SNAPSHOT and friends), which is still a busy database.
	switch e.Code() & 0xff {
	case 5: // SQLITE_BUSY
		return true
	case 6: // SQLITE_LOCKED
		return true
	}
	return false
}

// affected is how many rows a statement changed, or 0 when the driver will not
// say.
func affected(res sql.Result) int64 {
	if res == nil {
		return 0
	}
	n, err := res.RowsAffected()
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
