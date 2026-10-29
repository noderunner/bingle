// Tests of the span collector: what it records, what it does outside a
// request, and that neither a cap nor a pool makes it lie.

package trace_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/trace"
)

func TestStart_OutsideARequestDoesNothing(t *testing.T) {
	ctx := context.Background()

	// This is the common case by far: a bot search, a batch of five hundred
	// games, every call in a test.
	ctx2, span := trace.Start(ctx, "engine.Apply")
	span.Set("game", "hex")
	span.End()
	span.End() // and again, harmlessly

	if trace.FromContext(ctx2) != nil {
		t.Error("a collector appeared from nowhere")
	}
	if got := trace.FromContext(ctx).Spans(); got != nil {
		t.Errorf("Spans() of no collector = %v, want nil", got)
	}
	if n := trace.FromContext(ctx).Dropped(); n != 0 {
		t.Errorf("Dropped() of no collector = %d", n)
	}
}

// Nothing is allocated when nothing is collecting, because that is the path
// every move that nobody is watching takes.
func TestStart_OutsideARequestAllocatesNothing(t *testing.T) {
	ctx := context.Background()
	allocs := testing.AllocsPerRun(1000, func() {
		_, span := trace.Start(ctx, "engine.Apply")
		span.Set("game", "hex")
		span.End()
	})
	if allocs != 0 {
		t.Errorf("%v allocations per span outside a request, want 0", allocs)
	}
}

func TestCollect_RecordsNamesParentsAndOrder(t *testing.T) {
	ctx, c, release := trace.Collect(context.Background())
	defer release()

	ctx, handler := trace.Start(ctx, "handler")
	inner, apply := trace.Start(ctx, "session.ApplyMove")
	_, store := trace.Start(inner, "store.RecordMove")
	store.Set("stmt", "RecordMove")
	store.End()
	apply.End()
	// A sibling of session.ApplyMove, not a child: started from ctx.
	_, publish := trace.Start(ctx, "ws.publish")
	publish.End()
	handler.End()

	spans := c.Spans()
	if len(spans) != 4 {
		t.Fatalf("recorded %d spans, want 4: %+v", len(spans), spans)
	}
	for i, want := range []struct {
		name   string
		parent int
	}{
		{"handler", -1},
		{"session.ApplyMove", 0},
		{"store.RecordMove", 1},
		{"ws.publish", 0},
	} {
		if spans[i].Name != want.name || spans[i].Parent != want.parent {
			t.Errorf("span %d = %q parent %d, want %q parent %d",
				i, spans[i].Name, spans[i].Parent, want.name, want.parent)
		}
		if !spans[i].Ended {
			t.Errorf("span %d (%s) is not ended", i, spans[i].Name)
		}
	}
	if len(spans[2].Attrs) != 1 || spans[2].Attrs[0].Key != "stmt" || spans[2].Attrs[0].Value != "RecordMove" {
		t.Errorf("the store span's attributes = %+v", spans[2].Attrs)
	}
	// Offsets are in order, and a parent lasts at least as long as its child.
	if spans[1].Offset < spans[0].Offset || spans[2].Offset < spans[1].Offset {
		t.Errorf("offsets are out of order: %v", []time.Duration{spans[0].Offset, spans[1].Offset, spans[2].Offset})
	}
	if spans[0].Duration < spans[1].Duration {
		t.Errorf("the handler (%v) is shorter than the move inside it (%v)", spans[0].Duration, spans[1].Duration)
	}
}

// A span that never ends — a handler that panicked, a goroutine still going —
// is told apart from a fast one rather than reported as taking no time.
func TestCollect_AnUnendedSpanSaysSo(t *testing.T) {
	ctx, c, release := trace.Collect(context.Background())
	defer release()

	_, span := trace.Start(ctx, "handler")
	_ = span // never ended

	spans := c.Spans()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	if spans[0].Ended {
		t.Error("an unended span is reported as ended")
	}
	if spans[0].Duration != 0 {
		t.Errorf("an unended span has duration %v", spans[0].Duration)
	}
}

// A handler in a loop cannot grow the collector without bound, and a
// breakdown that was cut short says how much it lost.
func TestCollect_CapsTheSpansAndCountsWhatItDropped(t *testing.T) {
	ctx, c, release := trace.Collect(context.Background())
	defer release()

	const tried = 200
	for i := 0; i < tried; i++ {
		_, span := trace.Start(ctx, "store.RecordMove")
		span.End()
	}

	spans := c.Spans()
	if len(spans) == 0 || len(spans) >= tried {
		t.Fatalf("recorded %d spans of %d tried, want a cap in between", len(spans), tried)
	}
	if got, want := c.Dropped(), tried-len(spans); got != want {
		t.Errorf("dropped %d, want %d", got, want)
	}
}

// Attributes are capped too, and the ones that fit are kept.
func TestSet_CapsTheAttributes(t *testing.T) {
	ctx, c, release := trace.Collect(context.Background())
	defer release()

	_, span := trace.Start(ctx, "handler")
	for i := 0; i < 50; i++ {
		span.Set("k", i)
	}
	span.End()

	attrs := c.Spans()[0].Attrs
	if len(attrs) == 0 || len(attrs) >= 50 {
		t.Fatalf("kept %d attributes of 50, want a cap in between", len(attrs))
	}
	if attrs[0].Value != 0 {
		t.Errorf("the first attribute kept is %v, want the first one set", attrs[0].Value)
	}
}

// The collector is pooled, so the spans a caller is handed must not change
// under them when the next request reuses it.
func TestCollect_SpansSurviveTheCollectorGoingBackToThePool(t *testing.T) {
	ctx, c, release := trace.Collect(context.Background())
	_, span := trace.Start(ctx, "handler")
	span.Set("route", "/api/games/{code}")
	span.End()
	kept := c.Spans()
	release()

	// Whatever the pool hands out next, and whatever is recorded into it.
	for i := 0; i < 20; i++ {
		ctx2, _, release2 := trace.Collect(context.Background())
		_, s := trace.Start(ctx2, "something.else")
		s.Set("other", "value")
		s.End()
		release2()
	}

	if len(kept) != 1 || kept[0].Name != "handler" {
		t.Fatalf("the spans changed after release: %+v", kept)
	}
	if len(kept[0].Attrs) != 1 || kept[0].Attrs[0].Key != "route" {
		t.Errorf("the attributes changed after release: %+v", kept[0].Attrs)
	}
}

// Spans from several goroutines land in one collector without racing (run
// under -race to mean anything).
func TestCollect_IsSafeFromSeveralGoroutines(t *testing.T) {
	ctx, c, release := trace.Collect(context.Background())
	defer release()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_, span := trace.Start(ctx, "ws.publish")
				span.Set("kind", "page")
				span.End()
			}
		}()
	}
	wg.Wait()

	if n := len(c.Spans()) + c.Dropped(); n != 8*20 {
		t.Errorf("recorded %d spans and dropped %d, want %d together", len(c.Spans()), c.Dropped(), 8*20)
	}
}
