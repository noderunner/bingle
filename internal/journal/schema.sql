-- The metrics database: measurements, kept beside the game database rather
-- than in it. See docs/history/observability-plan.md section 4.
--
-- Stage 4 creates slow_requests. Stage 5 adds samples, rollups and
-- client_errors to this same file.

CREATE TABLE IF NOT EXISTS slow_requests (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  at          INTEGER NOT NULL,          -- unix seconds
  request_id  TEXT    NOT NULL,          -- the X-Request-Id the response carried
  route       TEXT    NOT NULL,          -- chi's pattern, never the path
  method      TEXT    NOT NULL,
  status      INTEGER NOT NULL,
  duration_ms REAL    NOT NULL,
  spans       TEXT    NOT NULL,          -- the phase breakdown, as JSON
  err         TEXT    NOT NULL DEFAULT '',
  span_count  INTEGER NOT NULL DEFAULT 0,
  spans_lost  INTEGER NOT NULL DEFAULT 0 -- spans refused because the request hit the cap
);

CREATE INDEX IF NOT EXISTS idx_slow_requests_at ON slow_requests (at);
CREATE INDEX IF NOT EXISTS idx_slow_requests_route ON slow_requests (route, at);

-- Stage 5: the metric history.
--
-- One row per series per tick. Counters are kept as their monotonic total and
-- rates are computed on read, as Prometheus does; gauges as their value;
-- histograms as _count, _sum and one row per bucket (the bucket in labels).
CREATE TABLE IF NOT EXISTS samples (
  at     INTEGER NOT NULL,          -- unix seconds
  name   TEXT    NOT NULL,          -- "bingle_http_requests_total"
  labels TEXT    NOT NULL,          -- canonical "method=POST,route=/api/games", '' for none
  value  REAL    NOT NULL,
  PRIMARY KEY (name, labels, at)
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_samples_at ON samples (at);

-- Older history, compacted. bucket is "hour" or "day"; at is the start of the
-- bucket. min/mean/max/n describe the samples that went into it.
CREATE TABLE IF NOT EXISTS rollups (
  bucket TEXT    NOT NULL,
  at     INTEGER NOT NULL,
  name   TEXT    NOT NULL,
  labels TEXT    NOT NULL,
  min    REAL    NOT NULL,
  mean   REAL    NOT NULL,
  max    REAL    NOT NULL,
  n      INTEGER NOT NULL,
  PRIMARY KEY (bucket, name, labels, at)
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_rollups_at ON rollups (bucket, at);
