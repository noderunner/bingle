// What naming and recording every statement costs, per statement.
//
// The move benchmark in tests/app goes through HTTP and an fsync, which vary
// by more than this does; a read answers from SQLite's cache, so the figure
// here is the wrapper and the recording and almost nothing else.

package store_test

import (
	"context"
	"testing"

	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/store"
)

func benchGetGame(b *testing.B, mx *metrics.Metrics) {
	path := b.TempDir() + "/bench.db"
	st, err := store.Open(path)
	if err != nil {
		b.Fatalf("store.Open: %v", err)
	}
	defer func() { _ = st.Close() }()
	st.SetMetrics(mx)

	ctx := context.Background()
	g, _, err := st.CreateGame(ctx, "RM0001", "test-freeplay", `{"positions":[]}`, "Ana", "black")
	if err != nil {
		b.Fatalf("CreateGame: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.GetGame(ctx, g.RoomCode); err != nil {
			b.Fatalf("GetGame: %v", err)
		}
	}
}

// One read, not recorded: the wrapper's own cost.
func BenchmarkStoreGetGame_MetricsOff(b *testing.B) { benchGetGame(b, nil) }

// And recorded, which is what Stage 3 adds.
func BenchmarkStoreGetGame_MetricsOn(b *testing.B) { benchGetGame(b, metrics.New("9.9.9")) }
