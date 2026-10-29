// Tests of what the store reports about itself: how statements are counted,
// what the gauges say about a real file, and the rule that every statement
// goes through the wrapper.

package store_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/store"
)

// newMeasuredStore opens a store on its own file, recording into its own
// registry, and returns both plus a scrape of the exposition.
func newMeasuredStore(t *testing.T) (*store.Store, func() string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "measured.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	mx := metrics.New("9.9.9")
	st.SetMetrics(mx)
	mx.SetDBSource(func() metrics.DBSnapshot { return st.Snapshot(context.Background()) })

	return st, func() string { return scrapeRegistry(t, mx) }
}

// A statement that works, one that finds nothing, and one that fails are three
// different outcomes, because "is there a row for this?" is what half of
// bingle's statements are for and is not a fault.
func TestStatements_AreCountedByHowTheyTurnedOut(t *testing.T) {
	st, scrape := newMeasuredStore(t)
	ctx := context.Background()

	// Works: a game is created and read back.
	g, _, err := newGame(ctx, st, 1)
	if err != nil {
		t.Fatalf("CreateGame: %v", err)
	}
	if _, err := st.GetGame(ctx, g.RoomCode); err != nil {
		t.Fatalf("GetGame: %v", err)
	}

	// Finds nothing: a room code nobody has.
	if _, err := st.GetGame(ctx, "ZZ9XYQ"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetGame of a missing room = %v, want ErrNotFound", err)
	}

	got := scrape()
	for _, want := range []string{
		`bingle_db_queries_total{result="ok",stmt="GetGame"} 1`,
		`bingle_db_queries_total{result="no-rows",stmt="GetGame"} 1`,
		`bingle_db_queries_total{result="busy",stmt="GetGame"} 0`,
		`bingle_db_queries_total{result="error",stmt="GetGame"} 0`,
		`bingle_db_queries_total{result="ok",stmt="CreateGameAt.insertGame"} 1`,
		`bingle_db_query_duration_seconds_count{stmt="GetGame"} 2`,
		`bingle_db_rows_total{stmt="GetGame"} 1`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the exposition does not say %s:\n%s", want, dbLines(got))
		}
	}
	// A transaction was opened and closed, and timed.
	if !strings.Contains(got, "bingle_db_tx_duration_seconds_count 1") {
		t.Errorf("the transaction was not timed:\n%s", dbLines(got))
	}
}

// A statement the database refuses counts as an error, and does not count as
// "no rows" or disappear.
func TestStatements_AFailingStatementCountsAsAnError(t *testing.T) {
	st, scrape := newMeasuredStore(t)
	ctx := context.Background()

	// Pull the table out from under a statement: the next read of it cannot
	// work, and is not a missing row either.
	if _, err := st.DB().ExecContext(ctx, `DROP TABLE games`); err != nil {
		t.Fatalf("dropping games: %v", err)
	}
	if _, err := st.GetGame(ctx, "AB12CD"); err == nil {
		t.Fatal("GetGame against a dropped table returned no error")
	}

	got := scrape()
	if !strings.Contains(got, `bingle_db_queries_total{result="error",stmt="GetGame"} 1`) {
		t.Errorf("a failed statement was not counted as an error:\n%s", dbLines(got))
	}
	// All four outcomes of a statement are published from the first time it
	// runs, so a rate() over errors works from the start instead of the
	// series appearing only once something has gone wrong. The one that did
	// not happen must therefore read 0, not be missing.
	if !strings.Contains(got, `bingle_db_queries_total{result="no-rows",stmt="GetGame"} 0`) {
		t.Errorf("a failed statement was counted as a missing row:\n%s", dbLines(got))
	}
	// And the figures that could not be read are left out rather than
	// published as zero, which would read as an empty database.
	if !strings.Contains(got, "bingle_db_snapshot_ok 0") {
		t.Errorf("the snapshot did not report that it failed:\n%s", dbLines(got))
	}
	if strings.Contains(got, "bingle_db_table_rows{") {
		t.Errorf("table counts were published from a failed snapshot:\n%s", dbLines(got))
	}
}

// The gauges about the file itself have to read back sanely on a real
// database, not just be present.
func TestSnapshot_ReadsBackSanelyOnARealDatabase(t *testing.T) {
	st, scrape := newMeasuredStore(t)
	ctx := context.Background()

	// Enough games that the file has something in it.
	for i := 0; i < 20; i++ {
		if _, _, err := newGame(ctx, st, i); err != nil {
			t.Fatalf("CreateGame: %v", err)
		}
	}

	got := scrape()
	if !strings.Contains(got, "bingle_db_snapshot_ok 1") {
		t.Fatalf("the snapshot failed:\n%s", dbLines(got))
	}

	pageCount := gaugeValue(t, got, "bingle_db_page_count")
	pageBytes := gaugeValue(t, got, "bingle_db_page_bytes")
	fileBytes := gaugeValue(t, got, "bingle_db_file_bytes")
	walBytes := gaugeValue(t, got, "bingle_db_wal_bytes")

	if pageBytes < 512 || pageBytes > 65536 {
		t.Errorf("page size = %v, which is not a SQLite page size", pageBytes)
	}
	if pageCount < 1 {
		t.Errorf("page count = %v", pageCount)
	}
	if fileBytes < pageBytes {
		t.Errorf("the file is %v bytes, less than one %v-byte page", fileBytes, pageBytes)
	}
	// In WAL mode — which store.Open sets — a page that has been written
	// lives in the log until a checkpoint moves it, so the main file can be
	// far smaller than page_count suggests while the log holds the rest.
	// That is exactly why both gauges exist and why WAL growth is the thing
	// the plan says will bite first. So the pages must be accounted for
	// across the two, not in the file alone.
	if onDisk, pages := fileBytes+walBytes, pageCount*pageBytes; onDisk < pages*0.5 {
		t.Errorf("%v pages of %v bytes is %v, but the file and the log hold only %v bytes together",
			pageCount, pageBytes, pages, onDisk)
	}
	if walBytes <= 0 {
		t.Errorf("twenty games were written and the write-ahead log is %v bytes", walBytes)
	}
	if freelist := gaugeValue(t, got, "bingle_db_freelist_pages"); freelist < 0 || freelist > pageCount {
		t.Errorf("freelist = %v of %v pages", freelist, pageCount)
	}

	// The tables hold what was just written, and every measured table is
	// reported even when it is empty.
	if rows := gaugeValue(t, got, `bingle_db_table_rows{table="games"}`); rows != 20 {
		t.Errorf("games holds %v rows, want 20", rows)
	}
	if rows := gaugeValue(t, got, `bingle_db_table_rows{table="moves"}`); rows != 0 {
		t.Errorf("moves holds %v rows, want 0", rows)
	}
	for _, table := range []string{"games", "players", "moves", "batches", "notify_contacts", "game_versions"} {
		if !strings.Contains(got, `bingle_db_table_rows{table="`+table+`"}`) {
			t.Errorf("no row count for %s", table)
		}
	}

	// And the gauge agrees with the log actually on disk.
	if st, err := os.Stat(dbPathOf(t, st) + "-wal"); err != nil {
		t.Errorf("no write-ahead log beside the database: %v", err)
	} else if float64(st.Size()) != walBytes {
		t.Errorf("the log is %d bytes on disk and the gauge says %v", st.Size(), walBytes)
	}

	// And the pool is reported whatever else happened.
	if gaugeValue(t, got, "bingle_db_pool_connections") < 1 {
		t.Error("the pool reports no connections")
	}
}

// The snapshot is cached, so a scraper asking every second cannot turn
// counting the rows of `moves` into a table scan every second.
func TestSnapshot_IsCachedBetweenScrapes(t *testing.T) {
	st, scrape := newMeasuredStore(t)
	ctx := context.Background()

	if _, _, err := newGame(ctx, st, 1); err != nil {
		t.Fatalf("CreateGame: %v", err)
	}
	first := gaugeValue(t, scrape(), `bingle_db_table_rows{table="games"}`)
	if first != 1 {
		t.Fatalf("games holds %v rows, want 1", first)
	}

	for i := 0; i < 5; i++ {
		if _, _, err := newGame(ctx, st, 100+i); err != nil {
			t.Fatalf("CreateGame: %v", err)
		}
	}
	// Within the cache's lifetime the figure is the one already read.
	if again := gaugeValue(t, scrape(), `bingle_db_table_rows{table="games"}`); again != first {
		t.Errorf("the snapshot was read again straight away: %v then %v", first, again)
	}
}

// Statements that fail because the database is locked are told apart from
// statements that fail, and counted on their own. This is the first thing to
// look at when writes start failing under load.
//
// It takes the ten seconds of the DSN's busy_timeout to provoke, so -short
// skips it.
func TestStatements_ABusyDatabaseIsCountedOnItsOwn(t *testing.T) {
	if testing.Short() {
		t.Skip("provoking SQLITE_BUSY means waiting out the ten-second busy timeout")
	}
	st, scrape := newMeasuredStore(t)
	path := dbPathOf(t, st)

	// A second connection that will not wait, holding the write lock.
	blocker, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(0)&_txlock=immediate")
	if err != nil {
		t.Fatalf("opening the blocking connection: %v", err)
	}
	defer func() { _ = blocker.Close() }()

	ctx := context.Background()
	tx, err := blocker.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("taking the write lock: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO game_versions (family, version) VALUES ('x','1')`); err != nil {
		_ = tx.Rollback()
		t.Fatalf("writing from the blocking connection: %v", err)
	}
	// The lock is held until this rolls back.
	defer func() { _ = tx.Rollback() }()

	start := time.Now()
	_, _, err = newGame(ctx, st, 1)
	waited := time.Since(start)
	if err == nil {
		t.Fatal("a write succeeded while another connection held the write lock")
	}
	t.Logf("the write waited %s before giving up: %v", waited.Round(time.Millisecond), err)

	got := scrape()
	if !strings.Contains(got, "bingle_db_busy_total 1") {
		t.Errorf("a locked database was not counted as busy:\n%s", dbLines(got))
	}
	if !strings.Contains(got, `result="busy"`) {
		t.Errorf("no statement was recorded with result=busy:\n%s", dbLines(got))
	}
}

// Every statement in internal/store goes through the wrapper in measure.go,
// so every one of them is named and counted. A new call site that reaches for
// the database directly fails here, the way the DOM-contract tests pin the
// interface the browser tests rely on.
func TestWrapperContract_NoStatementBypassesTheWrapper(t *testing.T) {
	// The direct calls, as they appear in source.
	direct := regexp.MustCompile(`\b(?:s\.db|t\.tx|tx)\.(?:Exec|Query|QueryRow)Context\b|\bs\.db\.BeginTx\b`)

	entries, err := os.ReadDir("../../internal/store")
	if err != nil {
		t.Fatalf("reading internal/store: %v", err)
	}
	seen := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// measure.go is the wrapper: it is the one file that may.
		if name == "measure.go" {
			continue
		}
		seen++
		src, err := os.ReadFile("../../internal/store/" + name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if direct.MatchString(line) {
				t.Errorf("internal/store/%s:%d reaches for the database directly:\n\t%s\n"+
					"Use s.exec / s.query / s.queryRow / s.begin (or tx.exec / tx.queryRow) with a "+
					"statement name, so the statement is counted — see internal/store/measure.go.",
					name, i+1, strings.TrimSpace(line))
			}
		}
	}
	if seen < 5 {
		t.Fatalf("only %d source files were checked, so this test is not looking where it thinks", seen)
	}
}

// ---- helpers ---------------------------------------------------------------

// newGame creates a game with a room code of its own, since room codes are
// unique and these tests make several.
func newGame(ctx context.Context, st *store.Store, n int) (store.Game, store.Player, error) {
	return st.CreateGame(ctx, fmt.Sprintf("RM%04d", n), "test-freeplay", `{"positions":[]}`, "Ana", "black")
}

// dbPathOf finds the file a store was opened on, by asking SQLite.
func dbPathOf(t *testing.T, st *store.Store) string {
	t.Helper()
	rows, err := st.DB().Query("PRAGMA database_list")
	if err != nil {
		t.Fatalf("PRAGMA database_list: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			t.Fatalf("scanning database_list: %v", err)
		}
		if name == "main" {
			return file
		}
	}
	t.Fatal("no main database")
	return ""
}

// dbLines is the bingle_db_ part of an exposition, for a readable failure.
func dbLines(exposition string) string {
	var out []string
	for _, line := range strings.Split(exposition, "\n") {
		if strings.HasPrefix(line, "bingle_db_") {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// scrapeRegistry renders a registry as Prometheus' text format.
func scrapeRegistry(t *testing.T, mx *metrics.Metrics) string {
	t.Helper()
	w := httptest.NewRecorder()
	mx.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("/metrics answered %d", w.Code)
	}
	return w.Body.String()
}

// gaugeValue pulls one value out of an exposition by its name (with labels, if
// it has any).
func gaugeValue(t *testing.T, exposition, name string) float64 {
	t.Helper()
	for _, line := range strings.Split(exposition, "\n") {
		rest, ok := strings.CutPrefix(line, name+" ")
		if !ok {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
		if err != nil {
			t.Fatalf("%s = %q, which is not a number: %v", name, rest, err)
		}
		return v
	}
	t.Fatalf("the exposition has no %s:\n%s", name, dbLines(exposition))
	return 0
}
