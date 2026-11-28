// Package cpu shares the server's CPUs between live play and background work
// (ADR 0162). Go gives no goroutine priority over another, so the order is
// kept here, in one budget that every CPU pool touches:
//
//   - Live work (a bot's reply in a room, a hint, a bot match people watch)
//     never waits for the budget. It only counts itself while it searches.
//   - Background work (a batch's games, the bot ladder, match_bots, the
//     training environments' opponents) takes a slot before it searches,
//     from a first-come queue. It may hold at most the budget's size less the
//     reserve kept for live play, and is given no slot while live and
//     background together would pass the size.
//   - Between plies a background game gives way: when live work has gone
//     past the reserve, or another background job has waited its turn, it
//     hands its slot back and queues again, keeping its position, and plays
//     on when its turn comes. Nothing is stopped in the middle of a search.
//
// A slot is taken and given back only here, and nothing is held while waiting
// for a slot, so the budget cannot deadlock; and since the queue is served in
// order and every holder gives way within one ply, no background job waits
// forever while live work leaves room.
package cpu

import (
	"container/list"
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/noderunner/bingle/internal/metrics"
)

// Turn is how long a background job holds its slot before it gives way to
// another background job that is waiting. It bounds how long background work
// waits behind other background work, and keeps the hand-overs few.
const Turn = 100 * time.Millisecond

// Budget is the CPUs, shared between live play and background work.
type Budget struct {
	mu      sync.Mutex
	size    int       // the CPUs
	reserve int       // what background may not take
	live    int       // live jobs counting themselves
	bg      int       // background slots held
	queue   list.List // of *waiter, first come first served
	now     func() time.Time
	mx      atomic.Pointer[metrics.Metrics]
}

// waiter is one background job waiting for a slot. granted is set, under the
// budget's lock, when it is given one; ready is closed then.
type waiter struct {
	ready   chan struct{}
	granted bool
	elem    *list.Element
}

// New makes a budget of size CPUs that keeps reserve of them for live play
// (see SetReserve).
func New(size, reserve int) *Budget {
	b := &Budget{size: max(size, 1), now: time.Now}
	b.SetReserve(reserve)
	return b
}

// DefaultReserve is what a budget of size CPUs keeps for live play when the
// operator does not say: what three quarters of them leave over, as a batch's
// default workers always did (4 of 16, 1 of 2, none of 1).
func DefaultReserve(size int) int {
	return size - max(size*3/4, 1)
}

var shared = New(runtime.GOMAXPROCS(0), DefaultReserve(runtime.GOMAXPROCS(0)))

// Shared is the server's budget, the one every pool uses.
func Shared() *Budget { return shared }

// SetReserve keeps n CPUs for live play. Background work always keeps one
// slot, so a server of one CPU still runs it (it then gives way to all live
// work). A background job holding a slot past a smaller share gives it back
// at its next ply.
func (b *Budget) SetReserve(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reserve = min(max(n, 0), b.size-1)
	b.grant()
	b.report()
	b.mx.Load().SetSlotSizes(map[metrics.Slot]int{metrics.SlotLive: b.reserve, metrics.SlotBackground: b.size - b.reserve})
}

// Size is the CPUs the budget shares.
func (b *Budget) Size() int { return b.size }

// Reserve is the CPUs kept for live play.
func (b *Budget) Reserve() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reserve
}

// BackgroundShare is the most slots background work may hold at once: the
// size less the reserve, and never less than one.
func (b *Budget) BackgroundShare() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.size - b.reserve
}

// SetMetrics hands the budget the registry to record into.
func (b *Budget) SetMetrics(mx *metrics.Metrics) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mx.Store(mx)
	b.report()
	mx.SetSlotSizes(map[metrics.Slot]int{metrics.SlotLive: b.reserve, metrics.SlotBackground: b.size - b.reserve})
}

// Live counts one live job until done is called; it never waits. Calling
// done again does nothing.
func (b *Budget) Live() (done func()) {
	b.mu.Lock()
	b.live++
	b.report()
	b.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.live--
			b.grant()
			b.report()
			b.mu.Unlock()
		})
	}
}

// Slot is a background job's hold on the budget. It belongs to the one
// goroutine that took it.
type Slot struct {
	b     *Budget
	held  bool
	since time.Time
}

// Background waits for a background slot, in turn, until ctx ends. The slot
// is given back with Release.
func (b *Budget) Background(ctx context.Context) (*Slot, error) {
	s := &Slot{b: b}
	if err := s.take(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// GiveWay is called by a background job between plies. When live work needs
// the CPU (live and background together are past the size), when the share
// has shrunk below what background holds, or when another background job is
// waiting and this one has had its Turn, it gives its slot back and waits for
// it again at the back of the queue. An error means ctx ended while it
// waited: the slot is then not held, and Release does nothing.
func (s *Slot) GiveWay(ctx context.Context) error {
	if !s.held {
		return context.Canceled
	}
	b := s.b
	b.mu.Lock()
	must := b.live+b.bg > b.size || b.bg > b.size-b.reserve
	turn := b.queue.Len() > 0 && b.now().Sub(s.since) >= Turn
	if !must && !turn {
		b.mu.Unlock()
		return nil
	}
	b.bg--
	s.held = false
	w := b.enqueue()
	b.grant()
	b.report()
	b.mu.Unlock()
	return s.wait(ctx, w)
}

// Release gives the slot back. Calling it again, or on a slot whose wait was
// cancelled, does nothing.
func (s *Slot) Release() {
	if s == nil || !s.held {
		return
	}
	b := s.b
	b.mu.Lock()
	s.held = false
	b.bg--
	b.grant()
	b.report()
	b.mu.Unlock()
}

// take gets the slot: at once when nobody waits and there is room, else in
// turn.
func (s *Slot) take(ctx context.Context) error {
	b := s.b
	b.mu.Lock()
	if b.queue.Len() == 0 && b.room() {
		b.bg++
		s.held, s.since = true, b.now()
		b.report()
		b.mu.Unlock()
		b.mx.Load().SlotWait(metrics.SlotBackground, 0)
		return nil
	}
	w := b.enqueue()
	b.mu.Unlock()
	return s.wait(ctx, w)
}

// wait waits for w to be given a slot, or for ctx to end. A slot given to a
// waiter whose ctx ended at the same moment is given back, once.
func (s *Slot) wait(ctx context.Context, w *waiter) error {
	b := s.b
	queued := b.now()
	select {
	case <-w.ready:
	case <-ctx.Done():
		b.mu.Lock()
		if w.granted {
			b.bg--
			b.grant()
		} else {
			b.queue.Remove(w.elem)
		}
		b.report()
		b.mu.Unlock()
		return ctx.Err()
	}
	b.mx.Load().SlotWait(metrics.SlotBackground, b.now().Sub(queued))
	s.held, s.since = true, b.now()
	return nil
}

// room reports whether background may take one more slot. The caller holds
// the lock.
func (b *Budget) room() bool {
	return b.bg < b.size-b.reserve && b.live+b.bg < b.size
}

// enqueue puts a new waiter at the back of the queue. The caller holds the
// lock.
func (b *Budget) enqueue() *waiter {
	w := &waiter{ready: make(chan struct{})}
	w.elem = b.queue.PushBack(w)
	return w
}

// grant gives slots to the waiters at the front of the queue, in order, while
// there is room. The caller holds the lock.
func (b *Budget) grant() {
	for b.queue.Len() > 0 && b.room() {
		w := b.queue.Remove(b.queue.Front()).(*waiter)
		w.granted = true
		b.bg++
		close(w.ready)
	}
}

// report sets the gauges of what is in use. The caller holds the lock.
func (b *Budget) report() {
	mx := b.mx.Load()
	mx.SlotInUse(metrics.SlotLive, b.live)
	mx.SlotInUse(metrics.SlotBackground, b.bg)
}

// Stats is what the budget holds right now, for tests and the curious.
type Stats struct {
	Live, Background, Waiting int
}

// Stats says what the budget holds right now.
func (b *Budget) Stats() Stats {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Stats{Live: b.live, Background: b.bg, Waiting: b.queue.Len()}
}
