package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// A page that could not learn who it is (the server was away) does not keep
// that answer: it asks again, and when the tab comes back into view it is drawn
// again for whoever it is (a phone once showed a signed-in player as a guest for
// as long as the tab stayed open, though everything it sent went as them).
func TestWebPage_AFailedWhoAmIIsAskedAgain(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	// The first GET /auth/me of each page load fails, as if the server were restarting.
	flaky := `(function () {
		const real = window.fetch.bind(window);
		let failed = false;
		window.meCalls = 0;
		window.fetch = (input, init) => {
			const url = typeof input === "string" ? input : input.url;
			if (url.endsWith("/auth/me")) {
				window.meCalls++;
				if (!failed && !sessionStorage.getItem("meFailedOnce")) {
					failed = true;
					sessionStorage.setItem("meFailedOnce", "1");
					return Promise.reject(new TypeError("network"));
				}
			}
			return real(input, init);
		};
	})()`
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(flaky).Do(c)
		return err
	})); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the first ask", `window.meCalls >= 1`, 10*time.Second)
	// Asked again, the page gets an answer rather than the failure it had.
	var kind string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`import("/static/js/api.js").then((m) => m.whoAmI()).then((me) => me.kind)`, &kind, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil {
		t.Fatal(err)
	}
	if kind == "anonymous" {
		t.Fatalf("the failed answer was kept: %q", kind)
	}
	// Coming back into view after a failed first ask redraws the page.
	evalBool(t, ctx, `(window.beforeReload = true, document.dispatchEvent(new Event("visibilitychange")), true)`)
	waitJS(t, ctx, "the page drawn again", `window.beforeReload === undefined && window.meCalls >= 1`, 10*time.Second)
}
