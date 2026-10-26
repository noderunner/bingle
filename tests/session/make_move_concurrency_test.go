// Tests that concurrent moves in one room are serialised: exactly one wins, and the move sequence never skips.

package session_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// --- fixture game definition -------------------------------------------------
//
// Same style/shape as make_move_test.go's fixture (self-contained 3x3
// orthogonal4 grid, 2 seats, one "stone" piece via "step"/"capture-or-empty",
// round-robin turn order [1, 2]) but declared independently under its own
// game-def ID per this package's established self-containment convention.
//
// Only seat 1 (black)'s single legal move matters for this test: both
// goroutines race to play it.

const concurrencyFixtureGameDefID = "fixture-3x3-concurrency"

const (
	concurrencyFixtureBlackPos = "0,0" // seat 1 (black)'s only stone
	concurrencyFixtureWhitePos = "1,0" // seat 2 (white)'s stone; black's only legal move captures it
)

const concurrencyFixtureDefJSON = `{
  "id": "fixture-3x3-concurrency",
  "name": "Fixture 3x3 (concurrency tests)",
  "board": {
    "generator": { "type": "grid", "width": 3, "height": 3, "adjacency": "orthogonal4" }
  },
  "seats": [
    { "seat": 1, "color": "black" },
    { "seat": 2, "color": "white" }
  ],
  "pieces": {
    "stone": { "movement": { "primitive": "step", "occupancy": "capture-or-empty" } }
  },
  "setup": [
    { "position": "0,0", "piece": "stone", "color": "black" },
    { "position": "1,0", "piece": "stone", "color": "white" }
  ],
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "win_condition": { "type": "none" }
}`

// newConcurrencyTestManager builds a fresh *catalog.Catalog with the
// concurrency-tests fixture registered, a fresh *store.Store against a
// temp-dir SQLite file, and returns a session.NewManager(st, cat) wired to
// both. Reproduced locally per this package's self-containment convention
// (see make_move_test.go's identical note).
func newConcurrencyTestManager(t *testing.T) *session.Manager {
	t.Helper()

	cat := catalog.NewCatalog()
	if err := cat.Register(concurrencyFixtureGameDefID, []byte(concurrencyFixtureDefJSON), nil); err != nil {
		t.Fatalf("fixture setup: catalog.Register(%q) error = %v, want nil", concurrencyFixtureGameDefID, err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("fixture setup: store.Open(%q) error = %v, want nil", dbPath, err)
	}
	t.Cleanup(func() { _ = st.Close() })

	mgr := session.NewManager(st, cat)
	if mgr == nil {
		t.Fatalf("session.NewManager(...) = nil, want non-nil *Manager")
	}

	return mgr
}

// createAndActivateConcurrencyGame runs CreateGame(Alice) + JoinGame(Bob,
// RolePlayer), which activates the game and sets turn_seat to seat 1 (the
// first entry in the fixture's round-robin turn order). Returns the
// resulting room code.
func createAndActivateConcurrencyGame(t *testing.T, mgr *session.Manager, ctx context.Context) string {
	t.Helper()

	created, err := mgr.CreateGame(ctx, concurrencyFixtureGameDefID, "Alice")
	if err != nil {
		t.Fatalf("CreateGame() error = %v, want nil", err)
	}

	joined, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer)
	if err != nil {
		t.Fatalf("JoinGame(%q, RolePlayer) error = %v, want nil", "Bob", err)
	}
	if joined.Status != "active" {
		t.Fatalf("JoinGame(%q, RolePlayer) Status = %q, want %q (precondition for this test)", "Bob", joined.Status, "active")
	}

	return created.RoomCode
}

// raceResult captures one goroutine's MakeMove outcome for inspection after
// both have completed.
type raceResult struct {
	outcome session.MoveOutcome
	err     error
}

// runConcurrentMakeMoveTrial sets up one fresh game and fires two goroutines
// at the exact same legal move for seat 1, released as simultaneously as
// possible via a shared starting gate (a channel closed once both goroutines
// have registered they're ready to go, maximizing the actual overlap window
// rather than relying on two sequential calls that merely don't happen to
// serialize on their own). It returns the two results plus the room code so
// the caller can inspect GetState afterward.
func runConcurrentMakeMoveTrial(t *testing.T, mgr *session.Manager, ctx context.Context) (roomCode string, results [2]raceResult) {
	t.Helper()

	roomCode = createAndActivateConcurrencyGame(t, mgr, ctx)

	from := board.PositionID(concurrencyFixtureBlackPos)
	to := board.PositionID(concurrencyFixtureWhitePos)

	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			ready.Done()
			<-start // block until both goroutines are ready, then race
			outcome, err := mgr.MakeMove(ctx, roomCode, "Alice", from, to)
			results[i] = raceResult{outcome: outcome, err: err}
		}()
	}

	ready.Wait() // both goroutines are parked on <-start
	close(start) // release them as close to simultaneously as possible
	wg.Wait()

	return roomCode, results
}

// --- 1. Single-trial shape of the AC: exactly one success, one rejection,
//        and move_seq == 1 (not 2) afterward -------------------------------

func TestMakeMove_Concurrency_SingleTrial_ExactlyOneSucceeds(t *testing.T) {
	mgr := newConcurrencyTestManager(t)
	ctx := context.Background()

	roomCode, results := runConcurrentMakeMoveTrial(t, mgr, ctx)

	successCount := 0
	failureCount := 0
	for i, r := range results {
		if r.err == nil {
			successCount++
		} else {
			failureCount++
		}
		t.Logf("goroutine %d: err = %v, outcome.Seq = %d", i, r.err, r.outcome.Seq)
	}

	if successCount != 1 {
		t.Errorf("successCount = %d, want exactly 1 (the other call must be rejected)", successCount)
	}
	if failureCount != 1 {
		t.Errorf("failureCount = %d, want exactly 1", failureCount)
	}

	snap, err := mgr.GetState(ctx, roomCode, nil)
	if err != nil {
		t.Fatalf("GetState(%q, nil) error = %v, want nil", roomCode, err)
	}
	if snap.MoveSeq != 1 {
		t.Errorf("GetState() MoveSeq = %d, want exactly 1 (the core AC: both concurrent calls racing "+
			"the same legal move must not both succeed and advance move_seq by 2)", snap.MoveSeq)
	}
}

// --- 2. Repeat across many trials, since a single trial might not reliably
//        expose a race depending on goroutine scheduling. Reports how many
//        of the trials showed the move_seq == 2 bug. ------------------------

func TestMakeMove_Concurrency_ManyTrials_MoveSeqNeverAdvancesByTwo(t *testing.T) {
	const trials = 30

	buggyTrials := 0   // MoveSeq == 2 (both calls incorrectly succeeded)
	correctTrials := 0 // MoveSeq == 1
	otherTrials := 0   // anything else (unexpected)
	bothSucceeded := 0 // both goroutines returned nil error
	bothFailed := 0    // both goroutines returned non-nil error

	for trial := 0; trial < trials; trial++ {
		mgr := newConcurrencyTestManager(t)
		ctx := context.Background()

		roomCode, results := runConcurrentMakeMoveTrial(t, mgr, ctx)

		successCount := 0
		for _, r := range results {
			if r.err == nil {
				successCount++
			}
		}
		switch successCount {
		case 2:
			bothSucceeded++
		case 0:
			bothFailed++
		}

		snap, err := mgr.GetState(ctx, roomCode, nil)
		if err != nil {
			t.Fatalf("trial %d: GetState(%q, nil) error = %v, want nil", trial, roomCode, err)
		}

		switch snap.MoveSeq {
		case 1:
			correctTrials++
		case 2:
			buggyTrials++
		default:
			otherTrials++
			t.Errorf("trial %d: GetState() MoveSeq = %d, want 1 or 2", trial, snap.MoveSeq)
		}
	}

	t.Logf("concurrency trial summary over %d trials: MoveSeq==1 (correct) = %d, MoveSeq==2 (bug) = %d, "+
		"other = %d; both-goroutines-succeeded = %d, both-goroutines-failed = %d",
		trials, correctTrials, buggyTrials, otherTrials, bothSucceeded, bothFailed)

	if buggyTrials != 0 {
		t.Errorf("%d of %d trials showed MoveSeq == 2 (both concurrent MakeMove calls for the same room's "+
			"turn incorrectly succeeded) — want 0; this is backlog item 22's AC: MakeMove must serialize "+
			"per-room via a mutex so only one of two racing calls for the same turn can ever succeed",
			buggyTrials, trials)
	}
	if correctTrials != trials {
		t.Errorf("correctTrials = %d, want %d (every trial should show MoveSeq == 1)", correctTrials, trials)
	}
}
