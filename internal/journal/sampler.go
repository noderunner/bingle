package journal

import (
	"context"
	"log/slog"
	"time"
)

// Sampler writes the registry's history and keeps the file from growing
// without bound.
//
// One goroutine, one small transaction every SampleEvery, and compaction in
// the same job — which is the whole reason the interval is five minutes rather
// than fifteen seconds: the point is history that survives into the backup,
// not a time-series database. Prometheus, if anyone runs one, scrapes
// /metrics as often as it likes and this does not change.
type Sampler struct {
	// DB is where the samples go. A nil Sampler, or a nil DB, does nothing.
	DB *DB
	// Source gathers the registry. internal/metrics.Metrics.Samples is it.
	Source func() ([]Sample, error)
	// Every is how often a tick happens.
	Every time.Duration
	// RetainDays is how long five-minute samples are kept before they are
	// compacted to hourly ones. Zero keeps everything at full resolution.
	RetainDays int
	// Logger is the server's logger; nil means slog's default.
	Logger *slog.Logger
	// Now is the clock, for tests. Nil means time.Now.
	Now func() time.Time
}

// tickTimeout bounds one tick. A tick is a few hundred small inserts in one
// transaction; if it cannot finish in a minute something is badly wrong and
// the next tick is a better bet than a pile-up.
const tickTimeout = time.Minute

func (s *Sampler) now() time.Time {
	if s == nil || s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

func (s *Sampler) log() *slog.Logger {
	if s == nil || s.Logger == nil {
		return slog.Default()
	}
	return s.Logger
}

// Run samples until ctx ends. It returns at once when there is nothing to do,
// so main can call it unconditionally.
func (s *Sampler) Run(ctx context.Context) {
	if s == nil || s.DB == nil || s.Source == nil || s.Every <= 0 {
		return
	}
	s.log().Info("the metric history is on",
		"db", s.DB.Path(), "every", s.Every, "full_resolution_days", s.RetainDays)

	t := time.NewTicker(s.Every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			// One last tick on the way out, so a restart does not lose the
			// window since the previous one.
			s.Tick(context.WithoutCancel(ctx))
			return
		case <-t.C:
			s.Tick(ctx)
		}
	}
}

// Tick writes one sample of every series and compacts what has aged out. It is
// exported so a test can drive it without waiting for a ticker, and so the
// Health page can ask for a tick on demand.
func (s *Sampler) Tick(ctx context.Context) {
	if s == nil || s.DB == nil || s.Source == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, tickTimeout)
	defer cancel()
	lg := s.log()

	samples, err := s.Source()
	if err != nil {
		lg.Error("the metric history could not read the registry", "err", err)
		return
	}
	at := s.now()
	if err := s.DB.WriteSamples(ctx, at, samples); err != nil {
		lg.Error("the metric history could not be written", "err", err)
		return
	}
	lg.Debug("the metric history took a sample", "series", len(samples))

	res, err := s.DB.Compact(ctx, at, s.RetainDays)
	if err != nil {
		lg.Error("the metric history could not be compacted", "err", err)
		return
	}
	if res.Any() {
		lg.Info("the metric history was compacted",
			"samples_folded", res.SamplesCompacted, "hourly_written", res.HourlyRollups,
			"hours_folded", res.HourliesCompacted, "daily_written", res.DailyRollups)
	}

	// The journal keeps the same window as the samples: a slow request from
	// a fortnight ago has already been acted on or never will be.
	if s.RetainDays > 0 {
		before := at.Add(-time.Duration(s.RetainDays) * 24 * time.Hour)
		if n, err := s.DB.PurgeSlowRequests(ctx, before); err != nil {
			lg.Error("the slow-request journal could not be purged", "err", err)
		} else if n > 0 {
			lg.Info("old slow requests were purged", "rows", n, "older_than_days", s.RetainDays)
		}
		// A fault nobody has hit for a fortnight has been fixed or has
		// stopped mattering.
		if n, err := s.DB.PurgeClientErrors(ctx, before); err != nil {
			lg.Error("old client errors could not be purged", "err", err)
		} else if n > 0 {
			lg.Info("old client errors were purged", "rows", n, "older_than_days", s.RetainDays)
		}
	}
}
