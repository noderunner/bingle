package journal

import (
	"context"
	"fmt"
	"time"
)

// Compaction and retention, which is what bounds the file's growth without a
// second process.
//
// Five-minute samples are kept for RetainDays, then become one hourly rollup
// each; hourly rollups are kept for a year, then become one daily rollup each.
// A year of one series is then 288 five-minute samples a day for two weeks,
// 24 hourly rows a day for the rest, and 365 daily rows for ever — a few
// thousand rows per series rather than a hundred thousand a year.

// hourlyFor is how long hourly rollups are kept before they become daily.
const hourlyFor = 365 * 24 * time.Hour

// CompactResult says what one compaction run did, for the log line and for a
// test.
type CompactResult struct {
	// HourlyRollups and DailyRollups are how many rollup rows were written.
	HourlyRollups, DailyRollups int64
	// SamplesCompacted and HourliesCompacted are how many rows were folded
	// into them and then deleted.
	SamplesCompacted, HourliesCompacted int64
}

// Any reports whether the run did anything worth a log line.
func (r CompactResult) Any() bool {
	return r.HourlyRollups+r.DailyRollups+r.SamplesCompacted+r.HourliesCompacted > 0
}

// Compact folds history older than its window into coarser rollups.
//
// It is one transaction per step, and each step writes the rollups before
// deleting what they came from, so an interrupted run loses no history: the
// worst case is a rollup that is written twice, which the upsert makes
// harmless.
//
// now is passed in rather than read, so a test can compact a seeded history.
func (d *DB) Compact(ctx context.Context, now time.Time, retainDays int) (CompactResult, error) {
	var out CompactResult
	if d == nil {
		return out, nil
	}
	if retainDays <= 0 {
		// History kept for ever at full resolution: the operator's choice,
		// and nothing to do.
		return out, nil
	}

	// Five-minute samples older than the window become hourly rollups.
	before := now.UTC().Add(-time.Duration(retainDays) * 24 * time.Hour).Unix()
	rollups, folded, err := d.foldSamples(ctx, before)
	if err != nil {
		return out, err
	}
	out.HourlyRollups, out.SamplesCompacted = rollups, folded

	// Hourly rollups older than a year become daily ones.
	beforeDay := now.UTC().Add(-hourlyFor).Unix()
	rollups, folded, err = d.foldHourlies(ctx, beforeDay)
	if err != nil {
		return out, err
	}
	out.DailyRollups, out.HourliesCompacted = rollups, folded
	return out, nil
}

// foldSamples turns every sample older than before into part of an hourly
// rollup, then deletes the samples.
func (d *DB) foldSamples(ctx context.Context, before int64) (rollups, folded int64, err error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("journal: beginning compaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 3600 seconds to the hour. The bucket is the hour the sample fell in.
	res, err := tx.ExecContext(ctx,
		`INSERT INTO rollups (bucket, at, name, labels, min, mean, max, n)
		 SELECT 'hour', at / 3600 * 3600, name, labels, MIN(value), AVG(value), MAX(value), COUNT(*)
		 FROM samples WHERE at < ?
		 GROUP BY name, labels, at / 3600
		 ON CONFLICT (bucket, name, labels, at) DO UPDATE SET
		   min  = MIN(min, excluded.min),
		   max  = MAX(max, excluded.max),
		   mean = (mean * n + excluded.mean * excluded.n) / (n + excluded.n),
		   n    = n + excluded.n`, before)
	if err != nil {
		return 0, 0, fmt.Errorf("journal: rolling samples up to the hour: %w", err)
	}
	rollups, _ = res.RowsAffected()

	res, err = tx.ExecContext(ctx, `DELETE FROM samples WHERE at < ?`, before)
	if err != nil {
		return 0, 0, fmt.Errorf("journal: deleting compacted samples: %w", err)
	}
	folded, _ = res.RowsAffected()

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("journal: committing compaction: %w", err)
	}
	return rollups, folded, nil
}

// foldHourlies turns every hourly rollup older than before into part of a
// daily one, then deletes the hourly rows.
func (d *DB) foldHourlies(ctx context.Context, before int64) (rollups, folded int64, err error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("journal: beginning compaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 86400 seconds to the day. The mean is weighted by n, so folding an
	// hour of twelve samples and an hour of one does not treat them alike.
	res, err := tx.ExecContext(ctx,
		`INSERT INTO rollups (bucket, at, name, labels, min, mean, max, n)
		 SELECT 'day', at / 86400 * 86400, name, labels,
		        MIN(min), SUM(mean * n) / SUM(n), MAX(max), SUM(n)
		 FROM rollups WHERE bucket = 'hour' AND at < ?
		 GROUP BY name, labels, at / 86400
		 ON CONFLICT (bucket, name, labels, at) DO UPDATE SET
		   min  = MIN(min, excluded.min),
		   max  = MAX(max, excluded.max),
		   mean = (mean * n + excluded.mean * excluded.n) / (n + excluded.n),
		   n    = n + excluded.n`, before)
	if err != nil {
		return 0, 0, fmt.Errorf("journal: rolling hours up to the day: %w", err)
	}
	rollups, _ = res.RowsAffected()

	res, err = tx.ExecContext(ctx, `DELETE FROM rollups WHERE bucket = 'hour' AND at < ?`, before)
	if err != nil {
		return 0, 0, fmt.Errorf("journal: deleting compacted hours: %w", err)
	}
	folded, _ = res.RowsAffected()

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("journal: committing compaction: %w", err)
	}
	return rollups, folded, nil
}

// PurgeSlowRequests drops journal rows older than before, so the journal does
// not grow without bound either. It keeps the same window as the samples.
func (d *DB) PurgeSlowRequests(ctx context.Context, before time.Time) (int64, error) {
	if d == nil {
		return 0, nil
	}
	res, err := d.db.ExecContext(ctx, `DELETE FROM slow_requests WHERE at < ?`, before.UTC().Unix())
	if err != nil {
		return 0, fmt.Errorf("journal: purging slow requests: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// Counts is how many rows each table holds, for the page and for a test.
func (d *DB) Counts(ctx context.Context) (map[string]int64, error) {
	if d == nil {
		return nil, nil
	}
	out := map[string]int64{}
	for _, q := range []struct{ key, sql string }{
		{"samples", `SELECT COUNT(*) FROM samples`},
		{"rollups_hour", `SELECT COUNT(*) FROM rollups WHERE bucket = 'hour'`},
		{"rollups_day", `SELECT COUNT(*) FROM rollups WHERE bucket = 'day'`},
		{"slow_requests", `SELECT COUNT(*) FROM slow_requests`},
	} {
		var n int64
		if err := d.db.QueryRowContext(ctx, q.sql).Scan(&n); err != nil {
			return nil, fmt.Errorf("journal: counting %s: %w", q.key, err)
		}
		out[q.key] = n
	}
	return out, nil
}
