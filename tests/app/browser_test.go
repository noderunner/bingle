// The one place the browser tests touch chromedp and the Chrome DevTools Protocol
// (cdproto). Everything else in the package says what to do in the browser through
// the helpers below: a test names a step (stepNavigate, stepWaitVisible,
// stepEvalTo) and runs it with runSteps, so that a new release of chromedp changes
// this file and nothing else. A step is the unit of "do this, then that": runSteps
// runs its steps in order and stops at the first error, as chromedp.Run does.

package app_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// browserStep is one thing to do in a browser.
type browserStep = chromedp.Action

// browserOption is a setting of the browser a test starts (a flag, say).
type browserOption = chromedp.ExecAllocatorOption

// evalOption is a setting of one evaluation in the page.
type evalOption = chromedp.EvaluateOption

// awaitPromise makes an evaluation wait for the promise its expression returns
// and give that promise's value.
var awaitPromise evalOption = func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
	return p.WithAwaitPromise(true)
}

// browserFlag is a command-line flag for the browser: browserFlag("mute-audio", true).
func browserFlag(name string, value any) browserOption {
	return chromedp.Flag(name, value)
}

// runSteps runs the steps in the browser behind ctx, in order, until one fails.
func runSteps(ctx context.Context, steps ...browserStep) error {
	return chromedp.Run(ctx, steps...)
}

// stepNavigate loads url and waits for the page to load.
func stepNavigate(url string) browserStep { return chromedp.Navigate(url) }

// stepReload reloads the page.
func stepReload() browserStep { return chromedp.Reload() }

// stepWaitVisible waits until an element matching the CSS selector is visible.
func stepWaitVisible(sel string) browserStep { return chromedp.WaitVisible(sel, chromedp.ByQuery) }

// stepWaitReady waits until an element matching the CSS selector is ready.
func stepWaitReady(sel string) browserStep { return chromedp.WaitReady(sel, chromedp.ByQuery) }

// stepSleep waits for d, or until the browser's context ends.
func stepSleep(d time.Duration) browserStep { return chromedp.Sleep(d) }

// stepClick clicks the middle of the first element matching the CSS selector, with a
// real mouse press.
func stepClick(sel string) browserStep { return chromedp.Click(sel, chromedp.ByQuery) }

// stepSendKeys types text into the first element matching the CSS selector.
func stepSendKeys(sel, text string) browserStep {
	return chromedp.SendKeys(sel, text, chromedp.ByQuery)
}

// stepSetValue sets the value of the first element matching the CSS selector.
func stepSetValue(sel, value string) browserStep {
	return chromedp.SetValue(sel, value, chromedp.ByQuery)
}

// The keys stepKey takes by name; any other key is its character.
const (
	keyArrowDown = kb.ArrowDown
	keyArrowUp   = kb.ArrowUp
	keyEnd       = kb.End
	keyEscape    = kb.Escape
)

// stepKey presses one key ("\x1b" is Escape) on whatever has the focus.
func stepKey(key string) browserStep { return chromedp.KeyEvent(key) }

// stepText stores the visible text of the first element matching the CSS selector in *out.
func stepText(sel string, out *string) browserStep { return chromedp.Text(sel, out, chromedp.ByQuery) }

// stepLocation stores the page's URL in *out.
func stepLocation(out *string) browserStep { return chromedp.Location(out) }

// stepViewport sizes the page to w by h CSS pixels.
func stepViewport(w, h int64) browserStep { return chromedp.EmulateViewport(w, h) }

// stepMobileViewport sizes the page to w by h CSS pixels, as a phone's.
func stepMobileViewport(w, h int64) browserStep {
	return chromedp.EmulateViewport(w, h, chromedp.EmulateMobile)
}

// stepPhone makes the page a portrait phone w by h CSS pixels at scale pixels per CSS
// pixel, with a touch screen.
func stepPhone(w, h int64, scale float64) browserStep {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		err := emulation.SetDeviceMetricsOverride(w, h, scale, true).
			WithScreenOrientation(&emulation.ScreenOrientation{Type: emulation.OrientationTypePortraitPrimary}).Do(ctx)
		if err != nil {
			return err
		}
		return emulation.SetTouchEmulationEnabled(true).Do(ctx)
	})
}

// stepDesktopMetrics fixes the page at w by h CSS pixels, scale pixels per CSS pixel,
// as a desktop screen and not a phone.
func stepDesktopMetrics(w, h int64, scale float64) browserStep {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetDeviceMetricsOverride(w, h, scale, false).Do(ctx)
	})
}

// stepTouchScreen gives the page a touch screen with up to maxPoints fingers.
func stepTouchScreen(maxPoints int64) browserStep {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetTouchEmulationEnabled(true).WithMaxTouchPoints(maxPoints).Do(ctx)
	})
}

// stepColorScheme makes the page's prefers-color-scheme "light" or "dark".
func stepColorScheme(scheme string) browserStep {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: scheme}}).Do(ctx)
	})
}

// stepMouseMove moves the pointer to x, y (CSS pixels from the page's top left).
func stepMouseMove(x, y float64) browserStep {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MouseMoved, x, y).Do(ctx)
	})
}

// stepTouchTap touches the page at x, y and lifts the finger: a tap.
func stepTouchTap(x, y float64) browserStep {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		if err := input.DispatchTouchEvent(input.TouchStart, []*input.TouchPoint{{X: x, Y: y}}).Do(ctx); err != nil {
			return err
		}
		return input.DispatchTouchEvent(input.TouchEnd, []*input.TouchPoint{}).Do(ctx)
	})
}

// stepScreenshot stores a PNG of the whole page in *out.
func stepScreenshot(out *[]byte) browserStep { return chromedp.FullScreenshot(out, 90) }

// stepViewportScreenshot stores a PNG of what the page's window shows in *out.
func stepViewportScreenshot(out *[]byte) browserStep { return chromedp.CaptureScreenshot(out) }

// stepNetworkEvents turns on the browser's network events, which listenWebSockets reads.
func stepNetworkEvents() browserStep {
	return chromedp.ActionFunc(func(ctx context.Context) error { return network.Enable().Do(ctx) })
}

// stepAddScript has the browser run js in every page it loads from now on, before
// the page's own scripts.
func stepAddScript(js string) browserStep {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(js).Do(ctx)
		return err
	})
}

// stepSetCookie gives the browser the cookie name=value for the site at baseURL,
// HttpOnly so that no page can read it.
func stepSetCookie(baseURL, name, value string) browserStep {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCookie(name, value).WithURL(baseURL).WithPath("/").WithHTTPOnly(true).Do(ctx)
	})
}

// stepCookie stores the value of the cookie called name in *out, or "" if there is none.
func stepCookie(name string, out *string) browserStep {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		cookies, err := network.GetCookies().Do(ctx)
		if err != nil {
			return err
		}
		*out = ""
		for _, c := range cookies {
			if c.Name == name {
				*out = c.Value
			}
		}
		return nil
	})
}

// stepEval evaluates the JavaScript expression js in the page and ignores its value.
func stepEval(js string, opts ...evalOption) browserStep {
	return chromedp.Evaluate(js, nil, opts...)
}

// stepEvalTo evaluates the JavaScript expression js in the page and stores its value
// in *out, which takes the type the value has (a string, an int, a bool, a slice, a
// struct for an object). An expression that gives undefined is an error; one that
// gives null leaves *out as it is.
func stepEvalTo[T any](js string, out *T, opts ...evalOption) browserStep {
	return chromedp.Evaluate(js, out, opts...)
}

// listenJSErrors calls fn with the text of every uncaught exception of the page.
// fn runs on the browser's event loop: it must be quick and must not drive the browser.
func listenJSErrors(ctx context.Context, fn func(text string)) {
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*runtime.EventExceptionThrown); ok && e.ExceptionDetails != nil {
			fn(e.ExceptionDetails.Error())
		}
	})
}

// listenWebSockets calls fn with the URL of every WebSocket the page opens, once
// stepNetworkEvents has run. fn runs on the browser's event loop: it must be quick and
// must not drive the browser.
func listenWebSockets(ctx context.Context, fn func(url string)) {
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*network.EventWebSocketCreated); ok {
			fn(e.URL)
		}
	})
}

// closeBrowser closes the browser behind ctx, as a person closing the window would.
func closeBrowser(ctx context.Context) error {
	return chromedp.Cancel(ctx)
}

// newChromedpContextWith is newChromedpContext with extra browser options (such as flags).
//
// Its pages start with the welcome tour seen (ADR 0139), so that it does not
// lie over the main page of a test about something else; newChromedpContextWithTour
// leaves it to be offered.
func newChromedpContextWith(t *testing.T, timeout time.Duration, extra ...browserOption) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := chromedpContext(t, timeout, extra...)
	if err := runSteps(ctx, stepAddScript(tourSeenScript)); err != nil {
		cancel()
		t.Fatalf("starting the browser: %v", err)
	}
	return ctx, cancel
}

func chromedpContext(t *testing.T, timeout time.Duration, extra ...browserOption) (context.Context, context.CancelFunc) {
	t.Helper()

	if testing.Short() {
		t.Skip("browser tests are skipped with -short")
	}
	skipOtherShards(t)
	browser := findBrowser()
	if browser == "" {
		t.Skip("no Chrome or Chromium found on $PATH (set BINGLE_BROWSER to its path): skipping the browser tests")
	}
	dir, err := os.MkdirTemp("", "chromedp-runner-")
	if err != nil {
		t.Fatalf("creating browser profile dir: %v", err)
	}
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.UserDataDir(dir), chromedp.ExecPath(browser))
	// The default 20s is too short for Chrome to start on a busy CI runner,
	// where the packages' tests run side by side.
	opts = append(opts, chromedp.WSURLReadTimeout(90*time.Second))
	opts = append(opts, extra...)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelChrome := chromedp.NewContext(allocCtx)
	ctx, cancelTimeout := context.WithTimeout(ctx, timeout)

	cancel := func() {
		cancelTimeout()
		cancelChrome()
		cancelAlloc()
		killLeftoverBrowser(dir)
	}
	return ctx, cancel
}

// pollLocation repeatedly asks the page for its URL until it matches re, or
// ctx's deadline is reached (a bounded wait, never an unbounded hang). It
// returns the first matching URL, or fails the test with the last-seen URL
// if the deadline is reached first.
func pollLocation(t *testing.T, ctx context.Context, re *regexp.Regexp) string {
	t.Helper()

	var lastURL string
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		var url string
		if err := runSteps(ctx, stepLocation(&url)); err == nil {
			lastURL = url
			if re.MatchString(url) {
				return url
			}
		}

		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for browser location to match %s; last-seen location: %q", re.String(), lastURL)
			return ""
		case <-ticker.C:
		}
	}
}

// submitClosestForm runs a small piece of JS in the page that finds the
// <form> ancestor of the element matching fieldSelector and clicks its
// submit button (a real button[type=submit]/input[type=submit] click, or
// failing that any <button> inside the form). Using a genuine click (rather
// than calling form.submit() directly) matters: form.submit() does NOT fire
// the form's "submit" event, so it would silently bypass any client-side JS
// (per docs/history/plan.md section 6.11) that intercepts submission to fetch()
// POST /api/games instead of letting the browser navigate there directly.
func submitClosestForm(ctx context.Context, fieldSelector string) browserStep {
	js := fmt.Sprintf(`(function() {
		var field = document.querySelector(%q);
		if (!field) { throw new Error(%q); }
		var form = field.closest('form');
		if (!form) { throw new Error('no <form> ancestor found for ' + %q); }
		var btn = form.querySelector('button[type="submit"], input[type="submit"]') || form.querySelector('button');
		if (!btn) { throw new Error('no submit button found inside the form'); }
		btn.click();
	})()`, fieldSelector, "no element matching "+fieldSelector+" found", fieldSelector)
	return stepEval(js)
}
