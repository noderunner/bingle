package app_test

// The welcome tour in the page (ADR 0139): offered once on the main page,
// every slide's picture drawn in both themes, Skip remembered, and the help
// window taking it again.

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// awaitBool evaluates expr, a promise of a boolean.
func awaitBool(t *testing.T, ctx context.Context, expr string) bool {
	t.Helper()
	var v bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &v, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil {
		t.Fatalf("evaluate %s: %v", expr, err)
	}
	return v
}

const serverSaysTourSeen = `fetch("/auth/me").then((r) => r.json()).then((me) => me.tour_seen === true)`

// walkTour goes through every slide of the open tour from the first, checking
// that each draws its picture and leaves no id the page has twice, and returns
// the slides' ids.
func walkTour(t *testing.T, ctx context.Context) []string {
	t.Helper()
	waitJS(t, ctx, "the tour", `document.querySelector("#welcome-tour[open]")`, 15*time.Second)
	n := evalInt(t, ctx, `document.querySelectorAll("#welcome-tour .tour-dot").length`)
	if !evalBool(t, ctx, `document.querySelector("#tour-count").textContent.startsWith("1 of ")`) {
		t.Fatalf("the tour does not start at its first slide: %q", evalString(t, ctx, `document.querySelector("#tour-count").textContent`))
	}
	var ids []string
	for i := 0; i < n; i++ {
		id := evalString(t, ctx, `document.getElementById("welcome-tour").dataset.slide`)
		ids = append(ids, id)
		waitJS(t, ctx, "the picture of "+id, `(() => { const st = document.querySelector("#tour-picture .tour-stage"); return st && st.childElementCount > 0 && st.inert; })()`, 15*time.Second)
		if dup := evalString(t, ctx, `(() => {
			const seen = new Set();
			for (const el of document.querySelectorAll("[id]")) {
				if (el instanceof SVGElement) continue;
				if (seen.has(el.id)) return el.id;
				seen.add(el.id);
			}
			return "";
		})()`); dup != "" {
			t.Errorf("slide %s: the page has the id %q twice", id, dup)
		}
		if i < n-1 {
			clickSel(t, ctx, "#tour-next")
		}
	}
	if got := evalString(t, ctx, `document.getElementById("tour-next").textContent`); got != "Done" {
		t.Errorf("the last slide's button: %q, want Done", got)
	}
	return ids
}

// A newcomer is offered the tour on the main page: every slide draws, in the
// light theme and the dark, the guest's slide says what an account adds, and
// Skip is remembered by the server. The help window takes it again.
func TestWebTour_ANewcomerIsOfferedItOnce(t *testing.T) {
	_, base := accountServer(t)
	ctx, cancel := newChromedpContextWithTour(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	if err := chromedp.Run(ctx, emulation.SetDeviceMetricsOverride(1100, 900, 1, false), chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	ids := walkTour(t, ctx)
	want := "welcome opponents learn table puzzles analysis games compete friends muster guest"
	if got := strings.Join(ids, " "); got != want {
		t.Errorf("a guest's slides:\n got %s\nwant %s", got, want)
	}
	if adds := evalString(t, ctx, `document.getElementById("tour-adds").textContent`); !strings.Contains(adds, "friends") || !strings.Contains(adds, "musters") || !strings.Contains(adds, " and ") {
		t.Errorf("what an account adds: %q", adds)
	}
	clickSel(t, ctx, `#welcome-tour .tour-dot[data-slide="analysis"]`)
	waitJS(t, ctx, "the analysis slide, for an account", `document.getElementById("tour-account-note")`, 5*time.Second)
	clickSel(t, ctx, "#tour-skip")
	waitJS(t, ctx, "the tour gone", `!document.getElementById("welcome-tour")`, 5*time.Second)
	if !awaitBool(t, ctx, serverSaysTourSeen) {
		t.Fatal("after Skip the server does not know the tour was seen")
	}

	// Seen: not offered again.
	if err := chromedp.Run(ctx, chromedp.Reload()); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the games", `document.querySelector("#game-gallery .game-card")`, 15*time.Second)
	time.Sleep(time.Second)
	if evalBool(t, ctx, `document.getElementById("welcome-tour")`) {
		t.Fatal("the tour is offered again after Skip")
	}

	// The help window takes it again, in the dark theme too; Done closes it.
	if err := chromedp.Run(ctx, emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: "dark"}})); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#open-help")
	clickSel(t, ctx, "#help-tour")
	walkTour(t, ctx)
	clickSel(t, ctx, "#tour-next")
	waitJS(t, ctx, "the tour closed by Done", `!document.getElementById("welcome-tour")`, 5*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("errors in the page: %v", jsErrors)
	}
}

// An account that has not seen the tour is offered it, without the guest's
// slide or a word about needing an account; Esc closes it and it is seen.
func TestWebTour_AnAccountIsOfferedItWithoutTheGuestsSlide(t *testing.T) {
	base, session := newMemberServer(t)
	ctx, cancel := newChromedpContextWithTour(t, 90*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, session)
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	ids := walkTour(t, ctx)
	if got, want := strings.Join(ids, " "), "welcome opponents learn table puzzles analysis games compete friends muster"; got != want {
		t.Errorf("an account's slides:\n got %s\nwant %s", got, want)
	}
	if evalBool(t, ctx, `document.getElementById("tour-account-note")`) {
		t.Error("an account is told a slide needs an account")
	}
	if err := chromedp.Run(ctx, chromedp.KeyEvent("\x1b")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the tour closed by Esc", `!document.getElementById("welcome-tour")`, 5*time.Second)
	if !awaitBool(t, ctx, serverSaysTourSeen) {
		t.Error("after Esc the server does not know the account saw the tour")
	}
}

// A slide whose feature this server offers nobody is left out, and one only
// an account has says so to a guest.
func TestWebTour_TheSlidesFollowWhatTheServerOffers(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the page", `document.querySelector("#open-help")`, 15*time.Second)
	cases := []struct{ me, want string }{
		// A server without accounts: nothing of an account's, but the guest's key.
		{`{kind: "guest", caps: ["play"], accounts: false}`, "welcome opponents learn table puzzles games guest"},
		// Accounts that may not write to one another.
		{`{kind: "guest", caps: ["play"], account_caps: ["play", "run_batch"], accounts: true}`, "welcome opponents learn table puzzles analysis* games compete* muster* guest"},
		// Guests allowed batches, as an administrator may.
		{`{kind: "guest", caps: ["play", "run_batch"], account_caps: ["run_batch", "dm"], accounts: true}`, "welcome opponents learn table puzzles analysis games compete* friends* muster* guest"},
		{`{kind: "account", caps: ["play", "dm"], accounts: true}`, "welcome opponents learn table puzzles games compete friends muster"},
	}
	for _, c := range cases {
		got := evalStringAwait(t, ctx, `import("/static/js/welcome-tour.js").then((m) => m.slidesFor(`+c.me+`).map((s) => s.id + (s.accountOnly ? "*" : "")).join(" "))`)
		if got != c.want {
			t.Errorf("slides for %s:\n got %s\nwant %s", c.me, got, c.want)
		}
	}
}

func evalStringAwait(t *testing.T, ctx context.Context, expr string) string {
	t.Helper()
	var s string
	if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &s, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil {
		t.Fatalf("evaluate %s: %v", expr, err)
	}
	return s
}
