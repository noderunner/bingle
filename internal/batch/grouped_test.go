package batch

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/store"
)

// Records that end within the window with nothing after them are still
// written within about the window, not when the next game ends; and nothing
// is written after the batch's last write.
func TestGroupedRecordsWaitNoLongerThanTheWindow(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "grouped.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	id, err := st.CreateBatch(ctx, "g", "hex-7", "{}", 10, "x")
	if err != nil {
		t.Fatal(err)
	}
	game := func(i int) store.BatchGame {
		return store.BatchGame{BatchID: id, Index: i, GameID: "hex-7", Seating: "[]", EndReason: "win", Metrics: "null"}
	}
	stored := func() int {
		b, err := st.GetBatch(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return b.Done
	}
	const every = 100 * time.Millisecond
	g := newGrouped(st, every)
	began := time.Now()
	g.add(game(0))
	g.add(game(1))
	if n := stored(); n != 0 {
		t.Fatalf("%d written before the window was out", n)
	}
	for stored() < 2 {
		if time.Since(began) > 20*every {
			t.Fatalf("after %s only %d of 2 records are written", time.Since(began), stored())
		}
		time.Sleep(5 * time.Millisecond)
	}
	// A record still waiting when the batch ends is written by its last
	// write, and its timer writes nothing after that.
	g.add(game(2))
	g.flush()
	g.add(game(3)) // after the end: never written
	time.Sleep(3 * every)
	if n := stored(); n != 3 {
		t.Errorf("%d records written, want 3", n)
	}
}
