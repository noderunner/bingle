package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/metrics"
)

// measuredTables are the tables whose rows are counted for
// bingle_db_table_rows. The list is here rather than read from the schema at
// runtime because it is a metric label and a label is a closed set; a new
// table is a deliberate line here. `moves` is the one that grows without
// bound, and the reason this gauge exists.
var measuredTables = []string{
	"games", "players", "moves",
	"reaction_prefs", "chat_messages", "global_chat", "autoplay_prefs",
	"batches", "batch_games",
	"notify_contacts", "notify_prefs", "notify_suppress", "notify_sent",
	"game_versions", "principals", "guest_keys",
	"accounts", "credentials", "account_groups", "sessions", "account_tokens", "auth_events",
	"capabilities", "cap_limits", "api_tokens", "invites",
}

// snapshotTTL is how long a snapshot is reused.
//
// Counting the rows of `moves` is a table scan, so a scraper asking every
// second must not turn into a table scan every second. Ten seconds is shorter
// than a normal scrape interval, so a scrape still gets figures from this
// scrape rather than the last one, and a burst of them costs one read.
const snapshotTTL = 10 * time.Second

// snapshotTimeout bounds the reading: these are gauges on a monitoring
// endpoint, and a database busy serving a game matters more than they do.
const snapshotTimeout = 5 * time.Second

// snapshotCache holds the last reading and when it was taken.
type snapshotCache struct {
	mu   sync.Mutex
	at   time.Time
	snap metrics.DBSnapshot
}

// Snapshot is the state of the database for the metrics: its size, its pages,
// its rows and its connection pool. It is cached for snapshotTTL, so calling
// it on every scrape is cheap.
func (s *Store) Snapshot(ctx context.Context) metrics.DBSnapshot {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	if !s.stats.at.IsZero() && time.Since(s.stats.at) < snapshotTTL {
		return s.stats.snap
	}
	ctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()
	s.stats.snap = s.readSnapshot(ctx)
	s.stats.at = time.Now()
	return s.stats.snap
}

// readSnapshot does the work Snapshot caches. The pool figures are filled in
// whatever else happens, because they are database/sql's own counting and
// cannot fail.
func (s *Store) readSnapshot(ctx context.Context) metrics.DBSnapshot {
	snap := metrics.DBSnapshot{Pool: s.db.Stats()}

	// The sizes on disk. An in-memory database has no file, and a WAL only
	// appears once something has been written, so neither missing is a fault.
	if s.path != "" {
		if st, err := os.Stat(s.path); err == nil {
			snap.FileBytes = st.Size()
		}
		if st, err := os.Stat(s.path + "-wal"); err == nil {
			snap.WALBytes = st.Size()
		}
	}

	for _, p := range []struct {
		pragma string
		into   *int64
	}{
		{"page_count", &snap.PageCount},
		{"freelist_count", &snap.FreelistPages},
		{"page_size", &snap.PageSize},
	} {
		// A PRAGMA name is from the list above, never from data, so this is
		// not an injection site.
		if err := s.queryRowUnmeasured(ctx, "PRAGMA "+p.pragma).Scan(p.into); err != nil {
			snap.Err = fmt.Errorf("reading PRAGMA %s: %w", p.pragma, err)
			return snap
		}
	}

	snap.TableRows = make(map[string]int64, len(measuredTables))
	for _, table := range measuredTables {
		var n int64
		// Likewise: the table name comes from measuredTables.
		if err := s.queryRowUnmeasured(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
			snap.Err = fmt.Errorf("counting %s: %w", table, err)
			return snap
		}
		snap.TableRows[table] = n
	}
	return snap
}
