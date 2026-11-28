package app_test

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// tapSel touches the middle of the element sel finds, as a finger would: a touch
// down and up at one place, which the browser turns into pointer events and a click.
func tapSel(t *testing.T, ctx context.Context, sel string) {
	t.Helper()
	var at []float64
	if err := runSteps(ctx, stepEvalTo(fmt.Sprintf(`(function(){ var e = document.querySelector(%q); if (!e) return []; var r = e.getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2]; })()`, sel), &at)); err != nil {
		t.Fatal(err)
	}
	if len(at) != 2 {
		t.Fatalf("nothing to tap at %s", sel)
	}
	tapAt(t, ctx, at[0], at[1])
}

func tapAt(t *testing.T, ctx context.Context, x, y float64) {
	t.Helper()
	if err := runSteps(ctx, stepTouchTap(x, y)); err != nil {
		t.Fatal(err)
	}
}

// A touch screen has no hover: a tap on something with a hover card pins the card
// open, a tap outside it (or on the thing again) closes it, and a button that acts
// keeps acting on a tap rather than opening a card. A mouse still hovers. The
// cards are made by js/tooltip.js on a page of their own, so that what is tapped
// is known: an icon that only shows what it is, and a button that does something.
func TestWebTouch_ATapPinsAHoverCardAndATapOutsideClosesIt(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := runSteps(ctx,
		stepViewport(800, 900),
		stepTouchScreen(5),
		stepNavigate(base+"/"),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the page", `document.readyState === "complete"`, 10*time.Second)
	if err := runSteps(ctx, stepEval(`(async function () {
		const m = await import("/static/js/tooltip.js");
		const box = document.createElement("div");
		box.style.cssText = "position:fixed;top:200px;left:200px;z-index:200;display:flex;gap:40px;background:#fff;padding:20px";
		const icon = document.createElement("span");
		icon.id = "t-icon"; icon.tabIndex = 0; icon.textContent = "ICON"; icon.style.cssText = "display:inline-block;width:80px;height:40px";
		const btn = document.createElement("button");
		btn.id = "t-btn"; btn.textContent = "ACT"; btn.style.cssText = "width:80px;height:40px";
		btn.addEventListener("click", () => { window.__acted = (window.__acted || 0) + 1; });
		box.append(icon, btn);
		document.body.append(box);
		m.hoverCard(icon, () => { const d = document.createElement("div"); d.id = "t-card"; d.textContent = "what the icon is"; return d; });
		m.hoverCard(btn, () => { const d = document.createElement("div"); d.textContent = "what the button does"; return d; });
		window.__ready = true;
		return true;
	})()`, awaitPromise)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the test's icon and button", `window.__ready === true`, 5*time.Second)

	tapSel(t, ctx, "#t-icon")
	waitJS(t, ctx, "a pinned card", `document.querySelector(".hover-card.pinned #t-card") !== null`, 5*time.Second)
	time.Sleep(400 * time.Millisecond)
	if !evalBool(t, ctx, `document.querySelector(".hover-card.pinned") !== null`) {
		t.Fatal("the card closed by itself after the tap")
	}
	// A touch on the card itself leaves it open.
	tapSel(t, ctx, ".hover-card.pinned")
	time.Sleep(200 * time.Millisecond)
	if !evalBool(t, ctx, `document.querySelector(".hover-card.pinned") !== null`) {
		t.Error("a touch on the card closed it")
	}
	// A touch anywhere else closes it.
	tapAt(t, ctx, 5, 880)
	waitJS(t, ctx, "the card closed by a touch outside", `document.querySelector(".hover-card") === null`, 5*time.Second)
	// A second tap on the icon closes the card the first one opened.
	tapSel(t, ctx, "#t-icon")
	waitJS(t, ctx, "pinned again", `document.querySelector(".hover-card.pinned") !== null`, 5*time.Second)
	tapSel(t, ctx, "#t-icon")
	waitJS(t, ctx, "closed by a second tap", `document.querySelector(".hover-card") === null`, 5*time.Second)

	// A button acts on a tap, and opens no card.
	tapSel(t, ctx, "#t-btn")
	waitJS(t, ctx, "the button acted", `window.__acted === 1`, 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	if evalBool(t, ctx, `document.querySelector(".hover-card") !== null`) {
		t.Error("a tap on a button opened a card")
	}

	// A mouse still hovers, and the card it opens is not pinned.
	var at []float64
	if err := runSteps(ctx, stepEvalTo(`(function(){ var r = document.getElementById("t-icon").getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2]; })()`, &at)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // past the time in which a mouse event counts as made up from a touch
	if err := runSteps(ctx, stepMouseMove(at[0], at[1])); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "a hover card", `document.querySelector(".hover-card:not(.pinned)") !== null`, 5*time.Second)
	if err := runSteps(ctx, stepMouseMove(5, 880)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the hover card gone with the pointer", `document.querySelector(".hover-card") === null`, 5*time.Second)
}
