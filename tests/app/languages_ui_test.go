package app_test

// The Language setting in the page (ADR 0141), and the page right to left in
// Arabic with its board left to right.

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Choosing Arabic in the profile window loads the page again in Arabic, right
// to left: the top bar is mirrored, a game's board is not. Choosing the
// browser's own language brings English back.
func TestWebLanguages_ArabicIsRightToLeftAndTheBoardIsNot(t *testing.T) {
	base := newRESTTestServer(t)
	room := createRoom(t, base, "Alice")
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)

	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/"), chromedp.WaitReady("#open-profile", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, `document.documentElement.lang + " " + getComputedStyle(document.body).direction`); got != "en ltr" {
		t.Fatalf("a new browser: %s", got)
	}
	clickSel(t, ctx, "#open-profile")
	waitJS(t, ctx, "the Language setting", `document.querySelector("#profile-dialog[open] #profile-window-language")`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#profile-window-language").options[0].textContent`); got != "Browser (English)" {
		t.Errorf("the first choice: %q", got)
	}
	evalBool(t, ctx, `(() => { const s = document.querySelector("#profile-window-language"); s.value = "ar"; s.dispatchEvent(new Event("change")); return true; })()`)
	waitJS(t, ctx, "the page in Arabic", `document.documentElement.lang === "ar" && document.querySelector("#open-profile")`, 15*time.Second)

	if got := evalString(t, ctx, `document.documentElement.dir + " " + getComputedStyle(document.body).direction`); got != "rtl rtl" {
		t.Errorf("the page in Arabic: %s", got)
	}
	// Mirrored: the brand on the right, the icons on its left.
	if !evalBool(t, ctx, `(() => {
		const brand = document.querySelector(".topbar .brand").getBoundingClientRect();
		const icons = document.querySelector("#topbar-actions").getBoundingClientRect();
		return brand.left > icons.right && brand.right > window.innerWidth / 2;
	})()`) {
		t.Error("the top bar is not mirrored")
	}
	if got := evalString(t, ctx, `document.querySelector("#open-profile").getAttribute("aria-label")`); got != "ملفك الشخصي (تلعب كضيف)" && got != "ملفك الشخصي" {
		t.Errorf("the profile button in Arabic: %q", got)
	}

	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/game/"+room), chromedp.WaitReady("#board-svg", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, `getComputedStyle(document.body).direction + " " + getComputedStyle(document.getElementById("board")).direction + " " + getComputedStyle(document.getElementById("board-svg")).direction`); got != "rtl ltr ltr" {
		t.Errorf("the game page, the board and its picture: %s, want rtl ltr ltr", got)
	}

	clickSel(t, ctx, "#open-profile")
	waitJS(t, ctx, "the Language setting", `document.querySelector("#profile-dialog[open] #profile-window-language")`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#profile-window-language").value`); got != "ar" {
		t.Errorf("the setting shows %q, want ar", got)
	}
	evalBool(t, ctx, `(() => { const s = document.querySelector("#profile-window-language"); s.value = ""; s.dispatchEvent(new Event("change")); return true; })()`)
	waitJS(t, ctx, "the page in English again", `document.documentElement.lang === "en" && document.documentElement.dir === "ltr" && document.querySelector("#board-svg")`, 15*time.Second)

	mu.Lock()
	defer mu.Unlock()
	for _, e := range jsErrors {
		t.Errorf("JS error: %s", e)
	}
}

// On a phone in Arabic the pages run from the right: the page a button of the
// bar below opens is the one on the screen, and a swipe back to the first
// page is followed.
func TestWebLanguages_APhoneInArabicTurnsItsPagesTheOtherWay(t *testing.T) {
	base := newRESTTestServer(t)
	room := councilWithHistory(t, base, 2)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/healthz"), chromedp.Evaluate(`document.cookie = "bingle_lang=ar; path=/"; true`, nil)); err != nil {
		t.Fatal(err)
	}
	openOnPhone(t, ctx, base, "Alice", map[string]string{room: "test-council"}, room)
	if got := evalString(t, ctx, `document.documentElement.dir`); got != "rtl" {
		t.Fatalf("the page is %q", got)
	}
	onScreen := `(function (el) { const r = el && el.getBoundingClientRect(); return !!r && r.width > 0 && Math.abs(r.left) < 2 && Math.abs(r.right - document.documentElement.clientWidth) < 2; })(%s)`
	clickSel(t, ctx, `#m-nav [data-group="log"]`)
	waitJS(t, ctx, "the history on the screen", fmt.Sprintf(onScreen, `document.getElementById("m-page-log")`), 5*time.Second)
	if !evalBool(t, ctx, `document.getElementById("m-track").scrollLeft < 0`) {
		t.Error("the pages do not run from the right")
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("m-track").scrollTo({ left: 0, behavior: "instant" }); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the first page followed", `!document.querySelector('#m-nav [data-group="log"]').classList.contains("on")`, 5*time.Second)
}
