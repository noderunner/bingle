// The core tutorials of Cthulhu Wars, followed in a browser by clicking only
// what the page marks. Verify proves their moves are legal; this proves a
// learner can make them, and that what each step picks out is on the page.

package app_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// The step in view: its number, and the parts of its spotlight the page does not have.
const tutorialWalkState = `(async () => {
	const count = (document.getElementById("tutorial-count") || {}).textContent || "";
	const found = /^Lesson (\d+) of/.exec(count);
	const parts = found ? (window.__steps[found[1] - 1].spotlight || []) : [];
	const { pagePart } = await import("/static/js/tutorial.js");
	const missing = parts.filter((p) => !document.querySelector(pagePart(p, 1).selector + ".tutorial-spot"));
	return JSON.stringify({ count, missing, off: !!document.getElementById("tutorial-off"), done: !!document.getElementById("tutorial-done") });
})()`

// One click on what the tutorial marks: Next, else a destination, a unit, or a button.
const tutorialWalkClick = `(() => {
	const fire = (el) => el.dispatchEvent(new MouseEvent("click", { bubbles: true }));
	const next = document.getElementById("tutorial-next");
	if (next && !next.disabled) return next.click(), true;
	const area = document.querySelector("#board .cell.area.clickable");
	if (area) return fire(area), true;
	const token = document.querySelector("#board .token.token-target");
	if (token) return fire(token), true;
	const button = document.querySelector("button.tutorial-target");
	if (button) return button.click(), true;
	return false;
})()`

func TestWebCthulhuWars_TheCoreTutorialsCanBeFollowed(t *testing.T) {
	base := newRESTTestServer(t)
	for _, id := range []string{"how-to-win", "power", "units", "movement", "spellbooks", "combat", "sequence-of-play", "ui-features"} {
		t.Run(id, func(t *testing.T) {
			ctx, cancel := newChromedpContext(t, 150*time.Second)
			defer cancel()
			var mu sync.Mutex
			var jsErrors []string
			collectJSErrors(ctx, &mu, &jsErrors)
			if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 900)); err != nil {
				t.Fatal(err)
			}
			openTutorialOf(t, ctx, base, "cthulhu-wars", id)
			await := func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) }
			load := `fetch("/api/game-defs/cthulhu-wars/tutorials/` + id + `").then(r => r.json()).then(d => { window.__steps = d.steps; return d.steps.length; })`
			var steps int
			if err := chromedp.Run(ctx, chromedp.Evaluate(load, &steps, await)); err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			checked := ""
			deadline := time.Now().Add(120 * time.Second)
			for {
				if time.Now().After(deadline) {
					t.Fatalf("stuck at %q; the impulse says %q", checked, evalString(t, ctx, `(document.getElementById("impulse-text") || {}).textContent || ""`))
				}
				var raw string
				if err := chromedp.Run(ctx, chromedp.Evaluate(tutorialWalkState, &raw, await)); err != nil {
					t.Fatal(err)
				}
				var st struct {
					Count   string   `json:"count"`
					Missing []string `json:"missing"`
					Off     bool     `json:"off"`
					Done    bool     `json:"done"`
				}
				if err := json.Unmarshal([]byte(raw), &st); err != nil {
					t.Fatal(err)
				}
				if st.Off {
					t.Fatalf("left the script at %q", checked)
				}
				if st.Done {
					break
				}
				// A step's parts are checked once the page has settled on it.
				if st.Count != checked {
					checked = st.Count
					time.Sleep(400 * time.Millisecond)
					continue
				}
				if !seen[st.Count] {
					seen[st.Count] = true
					if len(st.Missing) > 0 {
						t.Errorf("%s picks out what is not on the page: %s", st.Count, strings.Join(st.Missing, ", "))
					}
				}
				var clicked bool
				if err := chromedp.Run(ctx, chromedp.Evaluate(tutorialWalkClick, &clicked)); err != nil {
					t.Fatal(err)
				}
				time.Sleep(200 * time.Millisecond)
			}
			if len(seen) != steps {
				t.Errorf("saw %d of %d steps", len(seen), steps)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(jsErrors) > 0 {
				t.Errorf("uncaught JS exceptions: %v", jsErrors)
			}
		})
	}
}
