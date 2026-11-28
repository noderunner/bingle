package cpu

import (
	"context"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// take is Background that fails the test on an error.
func take(t *testing.T, b *Budget) *Slot {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := b.Background(ctx)
	if err != nil {
		t.Fatalf("Background: %v", err)
	}
	return s
}

// waitFor polls cond for up to five seconds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("waited five seconds for %s", what)
}

// queued starts a Background call in a goroutine and returns what it gets.
func queued(b *Budget, ctx context.Context) <-chan *Slot {
	got := make(chan *Slot, 1)
	go func() {
		s, _ := b.Background(ctx)
		got <- s
	}()
	return got
}

func TestTheDefaults(t *testing.T) {
	for size, want := range map[int]int{1: 0, 2: 1, 3: 1, 4: 1, 8: 2, 16: 4, 64: 16} {
		if got := DefaultReserve(size); got != want {
			t.Errorf("DefaultReserve(%d) = %d, want %d", size, got, want)
		}
	}
	// A reserve of everything still leaves background one slot.
	if b := New(2, 5); b.Reserve() != 1 || b.BackgroundShare() != 1 {
		t.Errorf("a reserve of 5 of 2 CPUs keeps %d and shares %d", b.Reserve(), b.BackgroundShare())
	}
	if b := New(4, -1); b.Reserve() != 0 || b.BackgroundShare() != 4 {
		t.Errorf("a reserve of -1 keeps %d", b.Reserve())
	}
}

// With nobody playing, background gets everything but the reserve, at once,
// and the next one waits for a slot to be given back.
func TestBackgroundGetsAllButTheReserve(t *testing.T) {
	b := New(4, 1)
	held := []*Slot{take(t, b), take(t, b), take(t, b)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	next := queued(b, ctx)
	waitFor(t, "the fourth to queue", func() bool { return b.Stats().Waiting == 1 })
	select {
	case <-next:
		t.Fatal("background took the reserve")
	case <-time.After(20 * time.Millisecond):
	}
	held[0].Release()
	if s := <-next; s == nil {
		t.Fatal("the waiter got nothing when a slot was given back")
	}
	if st := b.Stats(); st.Background != 3 || st.Waiting != 0 {
		t.Errorf("stats %+v", st)
	}
}

// Live work never waits, even with background holding all it may; and once
// it is past the reserve, background gives way at its next ply, as many slots
// as live work needs and no more, and takes them again when it is done.
func TestBackgroundGivesWayToLiveWork(t *testing.T) {
	b := New(4, 1)
	held := []*Slot{take(t, b), take(t, b), take(t, b)}
	ctx := context.Background()

	done1 := b.Live() // within the reserve: nobody needs to move
	for _, s := range held {
		if err := s.GiveWay(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if st := b.Stats(); st.Background != 3 {
		t.Fatalf("background gave way to live work within the reserve: %+v", st)
	}

	done2 := b.Live() // one past it: one background slot must go
	back := make(chan error, 3)
	for _, s := range held {
		go func() { back <- s.GiveWay(ctx) }()
	}
	waitFor(t, "one background job to give way", func() bool { st := b.Stats(); return st.Background == 2 && st.Waiting == 1 })
	done2()
	done2() // twice does nothing
	for range held {
		if err := <-back; err != nil {
			t.Fatal(err)
		}
	}
	if st := b.Stats(); st.Background != 3 || st.Live != 1 || st.Waiting != 0 {
		t.Errorf("after live work ended: %+v", st)
	}
	done1()
	for _, s := range held {
		s.Release()
		s.Release()
	}
	if st := b.Stats(); st != (Stats{}) {
		t.Errorf("everything given back, and still %+v", st)
	}
}

// One CPU keeps no reserve: background runs, and gives way to any live work.
func TestOneCPU(t *testing.T) {
	b := New(1, DefaultReserve(1))
	s := take(t, b)
	done := b.Live()
	gave := make(chan error, 1)
	go func() { gave <- s.GiveWay(context.Background()) }()
	waitFor(t, "background to give way", func() bool { return b.Stats().Background == 0 })
	done()
	if err := <-gave; err != nil {
		t.Fatal(err)
	}
	if st := b.Stats(); st.Background != 1 {
		t.Errorf("background did not get its slot back: %+v", st)
	}
	s.Release()
}

// A waiter whose context ends leaves the queue and holds nothing, and a slot
// handed to it as it was cancelled is given back once: the budget never goes
// below zero nor keeps a slot nobody holds.
func TestACancelledWaiterGivesNothingBackTwice(t *testing.T) {
	b := New(2, 1)
	s := take(t, b)
	ctx, cancel := context.WithCancel(context.Background())
	got := queued(b, ctx)
	waitFor(t, "the waiter to queue", func() bool { return b.Stats().Waiting == 1 })
	cancel()
	if w := <-got; w != nil {
		t.Fatal("a cancelled waiter was given a slot")
	}
	if st := b.Stats(); st.Background != 1 || st.Waiting != 0 {
		t.Fatalf("after cancelling: %+v", st)
	}
	s.Release()

	// A hand-over and a cancellation at the same moment, many times.
	for i := 0; i < 2000; i++ {
		s := take(t, b)
		ctx, cancel := context.WithCancel(context.Background())
		got := queued(b, ctx)
		for b.Stats().Waiting == 0 {
			time.Sleep(time.Microsecond)
		}
		go cancel()
		s.Release()
		if w := <-got; w != nil {
			w.Release()
			w.Release()
		}
		if st := b.Stats(); st != (Stats{}) {
			t.Fatalf("round %d left %+v", i, st)
		}
	}
}

// A job that gives its slot back and asks again at once does not pass one
// that is waiting: the queue is served in order.
func TestNoJobJumpsTheQueue(t *testing.T) {
	b := New(1, 0)
	s := take(t, b)
	ctx := context.Background()
	got := queued(b, ctx)
	waitFor(t, "the waiter to queue", func() bool { return b.Stats().Waiting == 1 })
	s.Release()
	again := queued(b, ctx) // the one that just gave it back
	w := <-got
	if w == nil {
		t.Fatal("the waiter got nothing")
	}
	select {
	case <-again:
		t.Fatal("a job jumped the queue")
	case <-time.After(20 * time.Millisecond):
	}
	w.Release()
	(<-again).Release()
}

// Background work that never ends a game still lets a waiting background job
// in, once it has had its turn.
func TestBackgroundTakesTurns(t *testing.T) {
	b := New(1, 0)
	var mu sync.Mutex
	now := time.Unix(0, 0)
	b.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	s := take(t, b)
	got := queued(b, context.Background())
	waitFor(t, "the waiter to queue", func() bool { return b.Stats().Waiting == 1 })
	if err := s.GiveWay(context.Background()); err != nil || !s.held {
		t.Fatalf("gave way before its turn was up: %v", err)
	}
	mu.Lock()
	now = now.Add(Turn)
	mu.Unlock()
	gave := make(chan error, 1)
	go func() { gave <- s.GiveWay(context.Background()) }()
	w := <-got
	if w == nil {
		t.Fatal("the waiter got nothing")
	}
	mu.Lock()
	now = now.Add(Turn)
	mu.Unlock()
	gave2 := make(chan error, 1)
	go func() { gave2 <- w.GiveWay(context.Background()) }()
	if err := <-gave; err != nil { // the first is back in, the second waits
		t.Fatal(err)
	}
	s.Release()
	if err := <-gave2; err != nil {
		t.Fatal(err)
	}
	w.Release()
	if st := b.Stats(); st != (Stats{}) {
		t.Errorf("left %+v", st)
	}
}

// Many background jobs and live jobs at once, some cancelled: background
// never holds more than its share, nothing is lost, and it all comes back.
// Worth a run with -race.
func TestTheBudgetUnderLoad(t *testing.T) {
	b := New(4, 1)
	var holding, most atomic.Int32
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := rand.New(rand.NewPCG(uint64(i), 1))
			for {
				select {
				case <-stop:
					return
				default:
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.IntN(3))*time.Millisecond)
				s, err := b.Background(ctx)
				if err != nil {
					cancel()
					continue
				}
				n := holding.Add(1)
				for m := most.Load(); n > m && !most.CompareAndSwap(m, n); m = most.Load() {
				}
				for p := 0; p < r.IntN(5); p++ {
					time.Sleep(time.Duration(r.IntN(200)) * time.Microsecond)
					holding.Add(-1)
					if s.GiveWay(ctx) != nil {
						break
					}
					holding.Add(1)
				}
				if s.held {
					holding.Add(-1)
				}
				s.Release()
				cancel()
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				done := b.Live()
				time.Sleep(300 * time.Microsecond)
				done()
				time.Sleep(time.Millisecond)
			}
		}()
	}
	time.Sleep(500 * time.Millisecond)
	close(stop)
	wg.Wait()
	if m := most.Load(); m > 3 || m < 1 {
		t.Errorf("background held %d slots at most, of a share of 3", m)
	}
	if st := b.Stats(); st != (Stats{}) {
		t.Errorf("after the load, %+v", st)
	}
}
