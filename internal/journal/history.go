package journal

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/noderunner/bingle/internal/metrics"
)

// The metric history.
//
// One row per series per tick, kept as Prometheus keeps it: a counter is its
// monotonic total and rates are worked out on read, so a restart shows as the
// total going back to zero rather than as a spike. Older history is compacted
// into hourly and then daily rollups, so the file's growth is bounded without
// a second process.

// Sample is one series at one moment. It is internal/metrics' type: that
// package knows what Prometheus' gathered form looks like and flattens it, and
// this one stores the result, so there is one shape and no conversion between
// them.
type Sample = metrics.Sample

// CanonicalLabels renders a label set the one way this package stores it, so
// the same series is the same row whatever order the labels arrived in.
func CanonicalLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
	}
	return b.String()
}

// WriteSamples records one tick: every series as it stood, in a single
// transaction, so a reader never sees half a tick.
func (d *DB) WriteSamples(ctx context.Context, at time.Time, samples []Sample) error {
	if d == nil || len(samples) == 0 {
		return nil
	}
	secs := at.UTC().Unix()

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("journal: beginning a sample tick: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO samples (at, name, labels, value) VALUES (?, ?, ?, ?)
		 ON CONFLICT (name, labels, at) DO UPDATE SET value = excluded.value`)
	if err != nil {
		return fmt.Errorf("journal: preparing the sample insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, s := range samples {
		if _, err := stmt.ExecContext(ctx, secs, s.Name, s.Labels, s.Value); err != nil {
			return fmt.Errorf("journal: writing sample %s{%s}: %w", s.Name, s.Labels, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("journal: committing a sample tick: %w", err)
	}
	return nil
}

// Point is one value of one series at one moment, as a reader sees it. A point
// that came from a rollup carries its spread as well.
type Point struct {
	At    time.Time `json:"at"`
	Value float64   `json:"value"`
	// Min, Max and N are set for a point read from a rollup, where Value is
	// the mean of N samples. They are zero for a raw sample.
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	N   int      `json:"n,omitempty"`
	// Bucket says where the point came from: "" for a raw sample, "hour" or
	// "day" for a rollup, so a reader knows what it is looking at.
	//
	// A rollup is stamped with the **start** of its bucket, which can fall
	// before the window that asked for it: the day bucket holding noon begins
	// at midnight. A reader wanting a window of old history should round its
	// start down to the bucket, or ask for a little more than it needs.
	Bucket string `json:"bucket,omitempty"`
}

// Series is one series' history.
type Series struct {
	Name   string            `json:"name"`
	Labels string            `json:"labels"`
	Label  map[string]string `json:"label,omitempty"`
	Points []Point           `json:"points"`
}

// Query asks for history.
type Query struct {
	// Name is the series to read; required.
	Name string
	// Since and Until bound the window. A zero Until means now.
	Since, Until time.Time
	// Labels, when set, is the exact canonical label set wanted; empty
	// returns every label set of the name.
	Labels string
	// Limit caps the points per series (0 means a sensible default).
	Limit int
}

// maxHistoryPoints is the most points one series may return. A chart cannot
// use more, and a window of a year of daily rollups is under four hundred.
const maxHistoryPoints = 5000

// History reads a series back, newest window last: the raw samples that are
// still kept, and the rollups for whatever is older.
func (d *DB) History(ctx context.Context, q Query) ([]Series, error) {
	if d == nil {
		return nil, nil
	}
	if q.Name == "" {
		return nil, fmt.Errorf("journal: a history query needs a name")
	}
	until := q.Until
	if until.IsZero() {
		until = time.Now()
	}
	limit := q.Limit
	if limit <= 0 || limit > maxHistoryPoints {
		limit = maxHistoryPoints
	}

	byLabels := map[string]*Series{}
	order := []string{}
	add := func(labels string, p Point) {
		s, ok := byLabels[labels]
		if !ok {
			s = &Series{Name: q.Name, Labels: labels, Label: ParseLabels(labels)}
			byLabels[labels] = s
			order = append(order, labels)
		}
		if len(s.Points) < limit {
			s.Points = append(s.Points, p)
		}
	}

	// The rollups first, because they are the older half of the window.
	rollupSQL := `SELECT bucket, at, labels, min, mean, max, n FROM rollups
	              WHERE name = ? AND at >= ? AND at <= ?`
	args := []any{q.Name, q.Since.UTC().Unix(), until.UTC().Unix()}
	if q.Labels != "" {
		rollupSQL += ` AND labels = ?`
		args = append(args, q.Labels)
	}
	// Daily before hourly at the same instant: coarse first, then finer.
	rollupSQL += ` ORDER BY at, bucket DESC`
	rows, err := d.db.QueryContext(ctx, rollupSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("journal: reading rollups of %s: %w", q.Name, err)
	}
	for rows.Next() {
		var bucket, labels string
		var at int64
		var min, mean, max float64
		var n int
		if err := rows.Scan(&bucket, &at, &labels, &min, &mean, &max, &n); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("journal: scanning a rollup: %w", err)
		}
		lo, hi := min, max
		add(labels, Point{At: time.Unix(at, 0).UTC(), Value: mean, Min: &lo, Max: &hi, N: n, Bucket: bucket})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("journal: reading rollups of %s: %w", q.Name, err)
	}
	_ = rows.Close()

	sampleSQL := `SELECT at, labels, value FROM samples WHERE name = ? AND at >= ? AND at <= ?`
	args = []any{q.Name, q.Since.UTC().Unix(), until.UTC().Unix()}
	if q.Labels != "" {
		sampleSQL += ` AND labels = ?`
		args = append(args, q.Labels)
	}
	sampleSQL += ` ORDER BY at`
	rows, err = d.db.QueryContext(ctx, sampleSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("journal: reading samples of %s: %w", q.Name, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var labels string
		var at int64
		var value float64
		if err := rows.Scan(&at, &labels, &value); err != nil {
			return nil, fmt.Errorf("journal: scanning a sample: %w", err)
		}
		add(labels, Point{At: time.Unix(at, 0).UTC(), Value: value})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("journal: reading samples of %s: %w", q.Name, err)
	}

	sort.Strings(order)
	out := make([]Series, 0, len(order))
	for _, labels := range order {
		out = append(out, *byLabels[labels])
	}
	return out, nil
}

// ParseLabels turns the canonical form back into a map, for a reader that
// wants the pairs rather than the string.
func ParseLabels(canonical string) map[string]string {
	if canonical == "" {
		return nil
	}
	out := map[string]string{}
	for _, pair := range strings.Split(canonical, ",") {
		if k, v, ok := strings.Cut(pair, "="); ok {
			out[k] = v
		}
	}
	return out
}

// Names lists the series the history holds, for the page's picker.
func (d *DB) Names(ctx context.Context) ([]string, error) {
	if d == nil {
		return nil, nil
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT name FROM samples UNION SELECT name FROM rollups ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("journal: listing series: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("journal: scanning a series name: %w", err)
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
