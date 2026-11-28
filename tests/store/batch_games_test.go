package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/store"
)

// A batch's finished games are written together (ADR 0162): every game of the
// group is kept, and each batch in it counts its own.
func TestBatchGamesAreWrittenTogether(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "batch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	a, err := st.CreateBatch(ctx, "a", "hex-7", "{}", 10, "x")
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateBatch(ctx, "b", "hex-7", "{}", 10, "x")
	if err != nil {
		t.Fatal(err)
	}
	game := func(batch int64, i int) store.BatchGame {
		return store.BatchGame{BatchID: batch, Index: i, Seed: int64(i), GameID: "hex-7", Seating: "[]", Winner: 1, EndReason: "win", Plies: 30, Metrics: "null"}
	}
	if err := st.AddBatchGames(ctx, []store.BatchGame{game(a, 0), game(a, 1), game(b, 0), game(a, 2)}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddBatchGames(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.AddBatchGame(ctx, game(b, 1)); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]int{a: 3, b: 2} {
		got, err := st.GetBatch(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		games, err := st.BatchGames(ctx, id, false)
		if err != nil {
			t.Fatal(err)
		}
		if got.Done != want || len(games) != want {
			t.Errorf("batch %d: done %d, %d games stored, want %d", id, got.Done, len(games), want)
		}
	}
}
