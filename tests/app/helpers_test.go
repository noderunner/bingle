// Helpers the browser tests share, from files that left with Cthulhu Wars (ADR 0127).

package app_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// jsErrorCollector records uncaught exceptions of a page.
func collectJSErrors(ctx context.Context, mu *sync.Mutex, out *[]string) {
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*runtime.EventExceptionThrown); ok && e.ExceptionDetails != nil {
			mu.Lock()
			*out = append(*out, e.ExceptionDetails.Error())
			mu.Unlock()
		}
	})
}

func evalBool(t *testing.T, ctx context.Context, expr string) bool {
	t.Helper()
	var v bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(`Boolean(`+expr+`)`, &v)); err != nil {
		t.Fatal(err)
	}
	return v
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// shot saves a full-page screenshot when BINGLE_SHOTS names a directory.
func shot(t *testing.T, ctx interface{ Done() <-chan struct{} }, name string, run func(*[]byte) error) {
	t.Helper()
	dir := os.Getenv("BINGLE_SHOTS")
	if dir == "" {
		return
	}
	var buf []byte
	if err := run(&buf); err != nil {
		t.Logf("screenshot %s: %v", name, err)
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name+".png"), buf, 0o644)
}
