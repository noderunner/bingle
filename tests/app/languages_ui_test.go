package app_test

// The Language setting in the page (ADR 0141), and the page right to left in
// Arabic with its board left to right.

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/auth"
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

// plainEnglish lists the words shown inside sel that are not in the
// pseudo-locale (en-XA, whose every word is accented and bracketed): words left
// out of the catalogs. Names, codes and the brand are not words to translate.
const plainEnglish = `((sel) => {
	const keep = /^(Bingle|GitHub|API|bingle|v?[0-9.]+|[A-Z0-9]{4,8}|bgg_…|https?:\/\/\S+)$/;
	const out = [];
	for (const root of document.querySelectorAll(sel)) {
		const walk = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
		for (let n = walk.nextNode(); n; n = walk.nextNode()) {
			const el = n.parentElement;
			if (!el || el.closest("[hidden], .help-changes-body, code, .voice-name, .handle-badge, script, style") || !el.getClientRects().length) continue;
			for (const word of n.textContent.split(/[\s·,:()\[\]]+/)) {
				if (/[A-Za-z]{2,}/.test(word) && !word.startsWith("@") && !keep.test(word) && !/[À-ɏḀ-ỿ]/.test(word)) out.push(word);
			}
		}
		for (const el of root.querySelectorAll("[title], [aria-label], [placeholder]")) {
			if (el.closest("[hidden]")) continue;
			for (const a of ["title", "aria-label", "placeholder"]) {
				const v = el.getAttribute(a);
				if (v && /[A-Za-z]{2,}/.test(v) && !keep.test(v) && !/[À-ɏḀ-ỿ]/.test(v)) out.push(a + "=" + v);
			}
		}
	}
	return [...new Set(out)].join(" | ");
})`

// In the pseudo-locale every word of the top bar and its windows comes from the
// catalogs: none is plain English.
func TestWebLanguages_TheTopBarIsTranslated(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1200, 900), chromedp.Navigate(base+"/healthz"), chromedp.Evaluate(`document.cookie = "bingle_lang=en-XA; path=/"; true`, nil), chromedp.Navigate(base+"/"), chromedp.WaitReady("#open-profile", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, plainEnglish+`(".topbar-actions")`); got != "" {
		t.Errorf("the top bar: %s", got)
	}
	for _, w := range []struct{ button, dialog string }{
		{"#open-active-games", "#active-games-dialog"},
		{"#open-join", "#join-dialog"},
		{"#open-profile", "#profile-dialog"},
		{"#open-help", "#help-dialog"},
	} {
		clickSel(t, ctx, w.button)
		waitJS(t, ctx, w.dialog, `document.querySelector("`+w.dialog+`[open]")`, 10*time.Second)
		time.Sleep(500 * time.Millisecond) // what the window reads from the server
		if got := evalString(t, ctx, plainEnglish+`("`+w.dialog+`")`); got != "" {
			t.Errorf("%s: %s", w.dialog, got)
		}
		evalBool(t, ctx, `(document.querySelector("`+w.dialog+`").close(), true)`)
	}
}

// So are the windows of a signed-in account: the profile and the friends,
// with a friend, a request and a correspondence open.
func TestWebLanguages_TheTopBarIsTranslatedForAnAccount(t *testing.T) {
	a, base := membersServer(t)
	anaB, bobB, caraB := member(t, a, base, "ana"), member(t, a, base, "bob"), member(t, a, base, "cara")
	befriend(t, anaB, bobB, "bob", "ana")
	if code := caraB.do("POST", "/api/friends/request", map[string]any{"handle": "ana"}, nil); code != 200 {
		t.Fatalf("@cara asks @ana: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, anaB.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1200, 900), chromedp.Navigate(base+"/healthz"), chromedp.Evaluate(`document.cookie = "bingle_lang=en-XA; path=/"; true`, nil), chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the friends icon", `getComputedStyle(document.getElementById("open-friends")).display !== "none"`, 20*time.Second)
	clickSel(t, ctx, "#open-profile")
	waitJS(t, ctx, "the profile window", `document.querySelector("#profile-dialog[open] #profile-signout")`, 10*time.Second)
	if got := evalString(t, ctx, plainEnglish+`("#profile-dialog")`); got != "" {
		t.Errorf("the profile window: %s", got)
	}
	evalBool(t, ctx, `(document.getElementById("profile-dialog").close(), true)`)
	clickSel(t, ctx, "#open-friends")
	waitJS(t, ctx, "the friend and the request", `document.querySelector("#friends-list .dm-person") && document.querySelector("[data-accept]")`, 15*time.Second)
	if got := evalString(t, ctx, plainEnglish+`("#friends-dialog")`); got != "" {
		t.Errorf("the friends window: %s", got)
	}
	clickSel(t, ctx, "#friends-list .dm-person")
	clickSel(t, ctx, "#friend-invite")
	waitJS(t, ctx, "the correspondence", `document.getElementById("dm-title").textContent.includes("bob") && !document.getElementById("friend-invite-row").hidden`, 10*time.Second)
	if got := evalString(t, ctx, plainEnglish+`("#friends-dialog")`); got != "" {
		t.Errorf("a correspondence: %s", got)
	}
}
