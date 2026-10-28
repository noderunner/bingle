package metrics

import (
	"database/sql"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// DBResult is how a statement turned out. The set is closed, because it is a
// label: "no rows" is deliberately not an error, since asking whether a row
// exists is what half of bingle's statements are for, and a database that
// could not get its lock is told apart from one that refused the statement.
type DBResult string

// The outcomes of a statement.
const (
	DBOK     DBResult = "ok"
	DBNoRows DBResult = "no-rows"
	DBBusy   DBResult = "busy"
	DBError  DBResult = "error"
)

// DBSnapshot is the state of the database as a whole, read when the registry
// is gathered rather than kept up to date as things happen. The store reads it
// behind a short cache, so a scraper cannot make counting rows expensive.
type DBSnapshot struct {
	// FileBytes and WALBytes are the sizes on disk of the database and its
	// write-ahead log. WAL growth is the first thing that bites.
	FileBytes, WALBytes int64
	// PageCount, FreelistPages and PageSize are what SQLite says about its
	// own pages: a freelist that keeps growing is a database that wants a
	// VACUUM.
	PageCount, FreelistPages, PageSize int64
	// TableRows is how many rows each table holds, by table name. The names
	// are a closed set: internal/store has the list.
	TableRows map[string]int64
	// Pool is database/sql's own account of the connection pool.
	Pool sql.DBStats
	// Err is set when the snapshot could not be read — a closed database, a
	// statement that failed. The gauges are then left alone rather than
	// written as zero, which would read as "the database is empty".
	Err error
}

// dbCollector reports DBSnapshot at scrape time. It is a Collector rather than
// a set of gauges updated on a ticker so that what a scrape shows is what was
// true when it was scraped, and so that no goroutine runs for a server nobody
// is watching.
type dbCollector struct {
	mu     sync.Mutex
	source func() DBSnapshot

	fileBytes     *prometheus.Desc
	walBytes      *prometheus.Desc
	pageCount     *prometheus.Desc
	freelistPages *prometheus.Desc
	pageSize      *prometheus.Desc
	tableRows     *prometheus.Desc
	poolOpen      *prometheus.Desc
	poolInUse     *prometheus.Desc
	poolIdle      *prometheus.Desc
	poolWaited    *prometheus.Desc
	poolWaitTime  *prometheus.Desc
	snapshotOK    *prometheus.Desc
}

func newDBCollector() *dbCollector {
	name := func(s string) string { return Namespace + "_db_" + s }
	return &dbCollector{
		fileBytes:     prometheus.NewDesc(name("file_bytes"), "The database file, in bytes.", nil, nil),
		walBytes:      prometheus.NewDesc(name("wal_bytes"), "The write-ahead log, in bytes.", nil, nil),
		pageCount:     prometheus.NewDesc(name("page_count"), "Pages in the database.", nil, nil),
		freelistPages: prometheus.NewDesc(name("freelist_pages"), "Pages on the freelist, waiting to be reused or vacuumed away.", nil, nil),
		pageSize:      prometheus.NewDesc(name("page_bytes"), "The size of a page, in bytes.", nil, nil),
		tableRows:     prometheus.NewDesc(name("table_rows"), "Rows in each table.", []string{"table"}, nil),
		poolOpen:      prometheus.NewDesc(name("pool_connections"), "Connections the pool holds.", nil, nil),
		poolInUse:     prometheus.NewDesc(name("pool_in_use"), "Connections in use right now.", nil, nil),
		poolIdle:      prometheus.NewDesc(name("pool_idle"), "Connections sitting idle.", nil, nil),
		poolWaited:    prometheus.NewDesc(name("pool_waited_total"), "Times a caller had to wait for a connection.", nil, nil),
		poolWaitTime:  prometheus.NewDesc(name("pool_wait_seconds_total"), "Time spent waiting for a connection.", nil, nil),
		snapshotOK:    prometheus.NewDesc(name("snapshot_ok"), "1 when the last attempt to read the figures below succeeded, 0 when it did not.", nil, nil),
	}
}

func (c *dbCollector) setSource(f func() DBSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.source = f
}

// Describe implements prometheus.Collector.
func (c *dbCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		c.fileBytes, c.walBytes, c.pageCount, c.freelistPages, c.pageSize, c.tableRows,
		c.poolOpen, c.poolInUse, c.poolIdle, c.poolWaited, c.poolWaitTime, c.snapshotOK,
	} {
		ch <- d
	}
}

// Collect implements prometheus.Collector.
func (c *dbCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	source := c.source
	c.mu.Unlock()
	if source == nil {
		return // no store yet: a server still starting up
	}
	snap := source()

	gauge := func(d *prometheus.Desc, v float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, labels...)
	}
	counter := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v)
	}

	ok := 0.0
	if snap.Err == nil {
		ok = 1
	}
	gauge(c.snapshotOK, ok)

	// The pool is database/sql's own counting and is always true, even when
	// reading the file's figures failed.
	gauge(c.poolOpen, float64(snap.Pool.OpenConnections))
	gauge(c.poolInUse, float64(snap.Pool.InUse))
	gauge(c.poolIdle, float64(snap.Pool.Idle))
	counter(c.poolWaited, float64(snap.Pool.WaitCount))
	counter(c.poolWaitTime, snap.Pool.WaitDuration.Seconds())

	if snap.Err != nil {
		// Leave the rest out rather than publish zeros, which would read as
		// an empty database.
		return
	}
	gauge(c.fileBytes, float64(snap.FileBytes))
	gauge(c.walBytes, float64(snap.WALBytes))
	gauge(c.pageCount, float64(snap.PageCount))
	gauge(c.freelistPages, float64(snap.FreelistPages))
	gauge(c.pageSize, float64(snap.PageSize))
	for table, rows := range snap.TableRows {
		gauge(c.tableRows, float64(rows), table)
	}
}

// SetDBSource gives the registry something to ask about the database when it
// is gathered. internal/store calls this; a nil *Metrics ignores it.
func (m *Metrics) SetDBSource(f func() DBSnapshot) {
	if m == nil {
		return
	}
	m.db.setSource(f)
}

// dbHandles are one statement's metrics, resolved once.
//
// Looking a label value up in a *Vec hashes it and takes a lock, and a move
// runs ten statements: doing that three times for each of them was, measured,
// the whole of what put this stage over the plan's 1% budget. Statement names
// are a closed set, so each one's metrics can be found once and kept.
type dbHandles struct {
	duration prometheus.Observer
	rows     prometheus.Counter
	// results is indexed by dbResultIndex, so picking the counter for an
	// outcome is an array index rather than a map lookup.
	results [4]prometheus.Counter
}

// dbResultIndex numbers the outcomes, for dbHandles.results.
func dbResultIndex(r DBResult) int {
	switch r {
	case DBNoRows:
		return 1
	case DBBusy:
		return 2
	case DBError:
		return 3
	default:
		return 0 // DBOK
	}
}

// handles finds (and the first time, resolves) the metrics of one statement.
func (m *Metrics) handles(stmt string) *dbHandles {
	if h, ok := m.dbHandles.Load(stmt); ok {
		return h.(*dbHandles)
	}
	h := &dbHandles{
		duration: m.dbDuration.WithLabelValues(stmt),
		rows:     m.dbRows.WithLabelValues(stmt),
	}
	for _, r := range []DBResult{DBOK, DBNoRows, DBBusy, DBError} {
		h.results[dbResultIndex(r)] = m.dbQueries.WithLabelValues(stmt, string(r))
	}
	// Two callers racing here both resolve, and both get metrics that are the
	// same objects underneath: a *Vec hands out one metric per label set.
	actual, _ := m.dbHandles.LoadOrStore(stmt, h)
	return actual.(*dbHandles)
}

// DBStatement records one statement: which one, how it turned out, how long it
// took and how many rows it touched.
func (m *Metrics) DBStatement(stmt string, result DBResult, d time.Duration, rows int64) {
	if m == nil {
		return
	}
	h := m.handles(stmt)
	h.results[dbResultIndex(result)].Inc()
	h.duration.Observe(d.Seconds())
	if rows > 0 {
		h.rows.Add(float64(rows))
	}
}

// DBTransaction records how long a transaction was open.
func (m *Metrics) DBTransaction(d time.Duration) {
	if m == nil {
		return
	}
	m.dbTxDuration.Observe(d.Seconds())
}

// DBBusy records a statement that waited out the busy timeout and still could
// not get the lock.
func (m *Metrics) DBBusy() {
	if m == nil {
		return
	}
	m.dbBusy.Inc()
}
