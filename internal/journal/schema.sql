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
