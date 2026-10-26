package store_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/store"
)

// TestConcurrentReadersAndWriters_NoBusyErrors hammers one database file
// with concurrent readers and writers — what happens when several browser
// tabs refresh while players submit moves. SQLite in its default journal
// mode and without a busy timeout fails such commits immediately with
// SQLITE_BUSY ("database is locked").
func TestConcurrentReadersAndWriters_NoBusyErrors(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "busy.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()

	const games = 6
	const movesPerGame = 25

	var ids []int64
	var codes []string
	for i := 0; i < games; i++ {
		code := fmt.Sprintf("ROOM%02d", i)
		g, _, err := st.CreateGame(ctx, code, "test-freeplay", "{}", fmt.Sprintf("p%d", i), "black")
		if err != nil {
			t.Fatalf("CreateGame: %v", err)
		}
		ids = append(ids, g.ID)
		codes = append(codes, code)
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup
	readErrs := make(chan error, 64)
	for r := 0; r < 8; r++ {
		readers.Add(1)
		go func(r int) {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				code := codes[r%games]
				if _, err := st.GetGame(ctx, code); err != nil {
					readErrs <- fmt.Errorf("GetGame(%s): %w", code, err)
					return
				}
				if _, err := st.ListMoves(ctx, ids[r%games], 0); err != nil {
					readErrs <- fmt.Errorf("ListMoves: %w", err)
					return
				}
			}
		}(r)
	}

	var writers sync.WaitGroup
	writeErrs := make(chan error, games)
	deadline := time.Now().Add(60 * time.Second)
	for i := 0; i < games; i++ {
		writers.Add(1)
		go func(i int) {
			defer writers.Done()
			for m := 0; m < movesPerGame; m++ {
				if time.Now().After(deadline) {
					writeErrs <- fmt.Errorf("game %d: timed out", i)
					return
				}
				_, err := st.RecordMove(ctx, ids[i], store.MoveInput{
					Seat: 1 + m%2, Color: "black", From: "0,0", To: "0,1",
					ResultingBoardState: "{}", NextTurnSeat: 1 + (m+1)%2,
				})
				if err != nil {
					writeErrs <- fmt.Errorf("game %d move %d: %w", i, m, err)
					return
				}
			}
		}(i)
	}

	writers.Wait()
	close(stop)
	readers.Wait()
	close(writeErrs)
	close(readErrs)
	for err := range writeErrs {
		t.Errorf("writer: %v", err)
	}
	for err := range readErrs {
		t.Errorf("reader: %v", err)
	}
}
