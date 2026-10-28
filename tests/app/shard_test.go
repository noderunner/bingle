package app_test

import (
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"strings"
	"testing"
)

// cwUIEnv turns the Cthulhu Wars browser tests back on. They are 56 of the 172
// browser tests — a third of the longest part of the suite — and what they
// drive is one game's interface, so they are run deliberately when that game
// or the shared UI changes rather than on every pull request:
//
//	make test-cw-ui
//
// Everything else about Cthulhu Wars still runs everywhere: its rules
// (tests/cthulhuwars, 276 tests), its REST and MCP surface, and the browser
// tests that merely use a Cthulhu Wars table to exercise shared code.
const cwUIEnv = "BINGLE_TEST_CW_UI"

// skipCthulhuWarsUI skips a TestWebCthulhuWars* test unless $BINGLE_TEST_CW_UI
// is set.
//
// The rule is the test's name, not the file it is in, and deliberately so: ten
// of the browser tests in the cthulhuwars_*_ui_test.go files are about shared
// code that happens to be reached through a Cthulhu Wars table — the Markdown
// renderer's safety, the rules window, the lobby, the home page's layouts and
// seat choices — and they keep running everywhere. A new Cthulhu Wars UI test
// therefore has to be named TestWebCthulhuWars... to be covered by this, which
// is also how the phone-layout test in mobile_ui_test.go is covered.
func skipCthulhuWarsUI(t *testing.T) {
	t.Helper()

	if !strings.HasPrefix(t.Name(), "TestWebCthulhuWars") {
		return
	}
	if os.Getenv(cwUIEnv) == "" {
		t.Skipf("the Cthulhu Wars UI tests are off by default: set %s=1, or run `make test-cw-ui`", cwUIEnv)
	}
}

// shardSpec is the "i/n" in $BINGLE_TEST_SHARD: this process runs the browser
// tests in bucket i of n and skips the rest. CI uses it to spread the browser
// suite over several runners at once, which is the only part of the suite long
// enough to be worth splitting (the rest of the packages together take about a
// minute).
//
// The split is by test name rather than by file or by a list in the workflow,
// so a new browser test joins a shard by itself and the buckets stay even as
// the suite grows. The hash is FNV-1a: any stable hash would do, but it must
// not be Go's map hash, which is seeded per process and would put a test in a
// different shard on every run.
func shardSpec(t *testing.T) (i, n int, set bool) {
	t.Helper()

	spec := os.Getenv("BINGLE_TEST_SHARD")
	if spec == "" {
		return 0, 0, false
	}
	// A malformed spec fails loudly: silently running every test would turn a
	// typo in the workflow into four runners doing the same work.
	num, den, ok := strings.Cut(spec, "/")
	if !ok {
		t.Fatalf("BINGLE_TEST_SHARD = %q, want \"i/n\" (for example \"2/4\")", spec)
	}
	i, errI := strconv.Atoi(strings.TrimSpace(num))
	n, errN := strconv.Atoi(strings.TrimSpace(den))
	if errI != nil || errN != nil || n < 1 || i < 1 || i > n {
		t.Fatalf("BINGLE_TEST_SHARD = %q, want 1 <= i <= n with n >= 1", spec)
	}
	return i, n, true
}

// shardOf is the 1-based shard a test name belongs to. Subtests travel with
// their parent, so a shard never runs half of a table-driven test.
func shardOf(name string, n int) int {
	if k := strings.IndexByte(name, '/'); k >= 0 {
		name = name[:k]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return int(h.Sum32()%uint32(n)) + 1
}

// skipOtherShards skips the test unless it belongs to this process's shard. It
// is called from newChromedpContextWith, so every browser test is covered
// without each one having to remember.
func skipOtherShards(t *testing.T) {
	t.Helper()

	i, n, set := shardSpec(t)
	if !set {
		return
	}
	if mine := shardOf(t.Name(), n); mine != i {
		t.Skipf("browser test is in shard %d of %d; this run is shard %d", mine, n, i)
	}
}

// TestShardOfIsStableAndBalanced pins the two properties the split needs: the
// same name always lands in the same shard (so a failure can be reproduced in
// the shard that found it), and the real browser-test names spread evenly
// enough that no shard carries the suite.
func TestShardOfIsStableAndBalanced(t *testing.T) {
	if got, want := shardOf("TestWebGameShowsTheBoard", 4), shardOf("TestWebGameShowsTheBoard", 4); got != want {
		t.Fatalf("shardOf is not stable: %d then %d", got, want)
	}
	if got := shardOf("TestWebGameShowsTheBoard/sub", 4); got != shardOf("TestWebGameShowsTheBoard", 4) {
		t.Errorf("a subtest went to shard %d, not its parent's", got)
	}
	for _, n := range []int{1, 2, 3, 4, 6} {
		counts := make([]int, n+1)
		for _, name := range browserTestNames(t) {
			s := shardOf(name, n)
			if s < 1 || s > n {
				t.Fatalf("shardOf(%q, %d) = %d, out of range", name, n, s)
			}
			counts[s]++
		}
		total := 0
		for _, c := range counts[1:] {
			total += c
		}
		if total == 0 {
			t.Fatal("found no browser tests to check the split against")
		}
		// Even by luck a hash will not divide perfectly; twice the fair share
		// is loose enough never to flake and tight enough to catch a split
		// that has stopped spreading the work.
		limit := 2 * total / n
		for s, c := range counts[1:] {
			if c > limit {
				t.Errorf("shards=%d: shard %d holds %d of %d browser tests (limit %d)", n, s+1, c, total, limit)
			}
		}
		t.Logf("shards=%d: %v", n, counts[1:])
	}
}

// browserTestNames reads this package's own sources for the tests that drive a
// browser: the ones that call newChromedpContext. Reading the sources keeps the
// check honest as tests are added and removed.
func browserTestNames(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the test sources: %v", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		for _, fn := range strings.Split(string(src), "\nfunc ") {
			if !strings.HasPrefix(fn, "Test") || !strings.Contains(fn, "newChromedpContext") {
				continue
			}
			if name, _, ok := strings.Cut(fn, "("); ok {
				names = append(names, name)
			}
		}
	}
	if len(names) == 0 {
		t.Fatalf("no browser tests found in %s", must(os.Getwd))
	}
	return names
}

func must(f func() (string, error)) string {
	s, err := f()
	if err != nil {
		return fmt.Sprint(err)
	}
	return s
}
