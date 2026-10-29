// Package trace records where a request spent its time.
//
// It is not distributed tracing and does not try to be. One request's spans go
// into one capped, pooled collector; when the request finishes they are thrown
// away unless it was slow or it failed, in which case internal/journal writes
// them down. That answers "which paths are slow or error-prone" on one machine
// with nothing else installed, which is what bingle needs (see
// docs/history/observability-plan.md section 3).
//
// The API is shaped like OpenTelemetry's on purpose:
//
//	ctx, span := trace.Start(ctx, "store.RecordMove")
//	defer span.End()
//	span.Set("stmt", "RecordMove")
//
// so that if traces ever need to leave the machine, the OTEL SDK replaces this
// package and no call site changes.
//
// **Nothing outside a request collects anything.** A bot search, a batch of
// five hundred games and every call in a test run with no collector in the
// context, where Start returns a span whose methods do nothing: one map-free
// context lookup and no allocation.
package trace

import (
	"context"
	"sync"
	"time"
)

// maxSpans is how many spans one request may record.
//
// A request that opens more than this is in a loop, and the sixty-fourth span
// is already past the point where a person reads the breakdown; the rest are
// counted and dropped so that a runaway handler cannot grow the collector
// without bound.
const maxSpans = 64

// maxAttrs is how many attributes one span may carry, for the same reason.
const maxAttrs = 8

// Attr is one thing worth knowing about a span. The value is formatted when
// the span is written down, not here.
type Attr struct {
	Key   string
	Value any
}

// Record is one finished span, as internal/journal writes it.
type Record struct {
	// Name is the boundary: "handler", "session.ApplyMove", "room.lock",
	// "store.RecordMove", "engine.Legal", "engine.Apply", "bot.search",
	// "web.render", "ws.publish".
	Name string
	// Parent is the index in this request's spans of the span this one is
	// inside, or -1 for the outermost.
	Parent int
	// Offset is how long after the request started this span began, so a
	// breakdown can be read in order without absolute times.
	Offset time.Duration
	// Duration is how long the span took. A span that never ended — a
	// handler that panicked, a goroutine still running — has a zero
	// duration, which is how it is told apart from a fast one.
	Duration time.Duration
	Ended    bool
	Attrs    []Attr
}

// Collector holds one request's spans.
//
// It is taken from a pool, so a request that turns out to be ordinary costs
// one reused slice and no allocation at all.
type Collector struct {
	mu      sync.Mutex
	start   time.Time
	spans   []Record
	dropped int
}

var collectorPool = sync.Pool{New: func() any {
	return &Collector{spans: make([]Record, 0, maxSpans)}
}}

type collectorKey struct{}
type parentKey struct{}

// Collect attaches a fresh collector to ctx and returns it with the function
// that gives it back to the pool. The caller must call release once it has
// finished with the spans — after writing them down, not before.
func Collect(ctx context.Context) (context.Context, *Collector, func()) {
	c := collectorPool.Get().(*Collector)
	c.reset()
	return context.WithValue(ctx, collectorKey{}, c), c, func() {
		c.reset()
		collectorPool.Put(c)
	}
}

func (c *Collector) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Keep each span's attribute array: resetting the length rather than the
	// slice is what makes a reused collector allocation-free.
	for i := range c.spans {
		c.spans[i].Attrs = c.spans[i].Attrs[:0]
		c.spans[i].Name = ""
	}
	c.spans = c.spans[:0]
	c.dropped = 0
	c.start = time.Now()
}

// FromContext returns the collector ctx carries, or nil when nothing is being
// collected.
func FromContext(ctx context.Context) *Collector {
	c, _ := ctx.Value(collectorKey{}).(*Collector)
	return c
}

// Spans returns the spans recorded so far, in the order they started.
func (c *Collector) Spans() []Record {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Record, len(c.spans))
	copy(out, c.spans)
	// The attribute slices are the collector's, and it is about to be reused,
	// so they are copied too.
	for i := range out {
		if n := len(out[i].Attrs); n > 0 {
			attrs := make([]Attr, n)
			copy(attrs, out[i].Attrs)
			out[i].Attrs = attrs
		}
	}
	return out
}

// Dropped is how many spans were refused because the request was already at
// maxSpans. It is reported with the breakdown, so a truncated one says so.
func (c *Collector) Dropped() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dropped
}

// Span is one boundary being timed. A span whose collector is nil does
// nothing, which is what Start returns outside a request.
type Span struct {
	c   *Collector
	idx int
}

// noop is handed out when nothing is being collected. Every method on it is a
// nil check, and nothing about it is ever written, so one shared value is
// safe for every goroutine.
var noop = &Span{}

// Start begins a span named name. The returned context makes it the parent of
// any span started inside it, and the span must be ended — `defer span.End()`
// on the line after is the shape to copy.
func Start(ctx context.Context, name string) (context.Context, *Span) {
	c := FromContext(ctx)
	if c == nil {
		return ctx, noop
	}
	parent := -1
	if p, ok := ctx.Value(parentKey{}).(int); ok {
		parent = p
	}

	c.mu.Lock()
	if len(c.spans) >= maxSpans {
		c.dropped++
		c.mu.Unlock()
		return ctx, noop
	}
	idx := len(c.spans)
	c.spans = append(c.spans, Record{
		Name:   name,
		Parent: parent,
		Offset: time.Since(c.start),
	})
	c.mu.Unlock()

	return context.WithValue(ctx, parentKey{}, idx), &Span{c: c, idx: idx}
}

// Set adds something worth knowing to the span: the statement a store span
// ran, the game an engine span was for. Values are never player data — a span
// ends up in the journal, which is read like a log (ADR 0063).
func (s *Span) Set(key string, value any) {
	if s.c == nil {
		return
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	r := &s.c.spans[s.idx]
	if len(r.Attrs) >= maxAttrs {
		return
	}
	r.Attrs = append(r.Attrs, Attr{Key: key, Value: value})
}

// End stops the span's clock. Ending twice is harmless; the first one counts.
func (s *Span) End() {
	if s.c == nil {
		return
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	r := &s.c.spans[s.idx]
	if r.Ended {
		return
	}
	r.Ended = true
	r.Duration = time.Since(s.c.start) - r.Offset
}
