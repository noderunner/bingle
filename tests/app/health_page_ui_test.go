// A browser test of the Health page: it is drawn entirely by a script, so the
// only way to know it works is to let a browser run it.

package app_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/journal"
	"github.com/noderunner/bingle/internal/metrics"
)

// newHealthServer builds a server with a seeded metric history and serves its
// **private** handler, which is the only place the Health page exists.
func newHealthServer(t *testing.T) string {
	t.Helper()

	db, err := journal.Open(t.TempDir() + "/bingle-metrics.db")
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mx := metrics.New("9.9.9")
	a, err := app.New(newTestConfig(t),
		app.WithMetrics(mx),
		app.WithHistory(journal.PageOptions{
			DB: db, Version: "9.9.9", Every: 5 * time.Minute, RetainDays: 14,
		}),
	)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	registerFixtures(t, a)

	// A history with something in it: several ticks, so the counters have
	// moved and the page has rates to draw.
	ctx := context.Background()
	now := time.Now()
	for i := 0; i < 8; i++ {
		samples, err := mx.Samples()
		if err != nil {
			t.Fatalf("gathering: %v", err)
		}
		// Nudge the counters so the page has something other than flat lines.
		mx.Move("tak", metrics.MoveApplied, time.Millisecond)
		mx.Finished("tak", "resignation")
		if err := db.WriteSamples(ctx, now.Add(time.Duration(i-8)*5*time.Minute), samples); err != nil {
			t.Fatalf("seeding the history: %v", err)
		}
	}

	return servePrivate(t, a)
}

// servePrivate wraps an App's private handler — the metrics listener's — in an
// httptest server. The Health page exists only there.
func servePrivate(t *testing.T, a *app.App) string {
	t.Helper()
	srv := httptest.NewServer(a.MetricsHandler())
	t.Cleanup(srv.Close)
	return srv.URL
}

// The page draws itself from the history, without throwing.
func TestWebHealthPage_DrawsFromTheHistory(t *testing.T) {
	baseURL := newHealthServer(t)

	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()

	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)

	var cards, text string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/metrics/health"),
		// The script replaces the "Loading…" shell with cards.
		chromedp.WaitVisible(`#health-root section.card`, chromedp.ByQuery),
		chromedp.Evaluate(`String(document.querySelectorAll("#health-root section.card").length)`, &cards),
		chromedp.Text(`#health-root`, &text, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("driving the health page: %v", err)
	}

	mu.Lock()
	errs := append([]string(nil), jsErrors...)
	mu.Unlock()
	if len(errs) > 0 {
		t.Fatalf("the health page threw:\n%s", strings.Join(errs, "\n"))
	}

	if cards == "0" || cards == "" {
		t.Fatalf("the page drew %q cards", cards)
	}
	// The figures it is for: the database, and what the history holds.
	for _, want := range []string{"database", "samples"} {
		if !strings.Contains(strings.ToLower(text), want) {
			t.Errorf("the page does not mention %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "There is no history yet") {
		t.Errorf("the page says there is no history, but it was seeded:\n%s", text)
	}
}

// With no history at all the page says so rather than throwing or showing an
// empty frame.
func TestWebHealthPage_SaysWhenThereIsNoHistory(t *testing.T) {
	db, err := journal.Open(t.TempDir() + "/bingle-metrics.db")
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	a, err := app.New(newTestConfig(t),
		app.WithMetrics(metrics.New("9.9.9")),
		app.WithHistory(journal.PageOptions{DB: db, Version: "9.9.9", Every: 5 * time.Minute, RetainDays: 14}),
	)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	baseURL := servePrivate(t, a)

	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()

	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)

	var text string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/metrics/health"),
		chromedp.WaitVisible(`#health-root section.card`, chromedp.ByQuery),
		chromedp.Text(`#health-root`, &text, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("driving the health page: %v", err)
	}

	mu.Lock()
	errs := append([]string(nil), jsErrors...)
	mu.Unlock()
	if len(errs) > 0 {
		t.Fatalf("the empty health page threw:\n%s", strings.Join(errs, "\n"))
	}
	if !strings.Contains(text, "no history yet") {
		t.Errorf("the page does not say the history is empty:\n%s", text)
	}
}
