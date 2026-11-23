package app_test

// The Language setting in the page (ADR 0141), and the page right to left in
// Arabic with its board left to right.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/session"
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
	// Intl writes the pseudo-locale's dates in English: their months, days and AM/PM pass.
	// Ana is a player the welcome tour's pictures show.
	const dates = /^(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\.?$|^(Mon|Tues?|Wed(nes)?|Thu(rs)?|Fri|Sat(ur)?|Sun)(day)?$|^[AP]M$/;
	const keep = /^(Ana|Bingle|GitHub|API|bingle|Buchholz|v?[0-9.]+|[A-Z0-9]{4,8}|bgg_…|https?:\/\/\S+|\[Size .*)$/s;
	// A board's or a card table's picture is the renderer's to describe (the game page's part).
	// A rules guide is a game's own content (#253), the welcome tour's sample of one too.
	const pictures = ".game-card-preview, .setup-preview, .puzzle-preview, #card-set-preview, .tour-rules .md, .tour-board";
	const accented = /[\u00C0-\u024F\u1E00-\u1EFF]/;
	const words = (text) => text.split(/[\s·,:()\[\]]+/);
	// A plain English word: not accented, not a handle, not the games' own (window.__content).
	const english = (word) =>
		/[A-Za-z]{2,}/.test(word) && !word.startsWith("@") && !keep.test(word) && !dates.test(word) && !accented.test(word) &&
		!(window.__content && window.__content.has(word.replace(/[.!?;"“”'’]+$/, "")));
	const out = [];
	for (const root of document.querySelectorAll(sel)) {
		const walk = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
		for (let n = walk.nextNode(); n; n = walk.nextNode()) {
			const el = n.parentElement;
			if (!el || el.closest("[hidden], [translate=no], .help-changes-body, code, .voice-name, .handle-badge, script, style, " + pictures) || !el.getClientRects().length) continue;
			for (const word of words(n.textContent)) if (english(word)) out.push(word);
		}
		for (const el of root.querySelectorAll("[title], [aria-label], [placeholder]")) {
			if (el.closest("[hidden], [translate=no], " + pictures)) continue;
			for (const a of ["title", "aria-label", "placeholder"]) {
				const v = el.getAttribute(a);
				if (v && !keep.test(v) && !accented.test(v) && words(v).some(english)) out.push(a + "=" + v);
			}
		}
	}
	return [...new Set(out)].join(" | ");
})`

// learnContent gathers into window.__content every word of the games' own content
// (names, descriptions, variants, seats, tutorials) and of the bots: it stays as
// it is in every language (#253), so plainEnglish passes it.
func learnContent(t *testing.T, ctx context.Context) {
	t.Helper()
	if !awaitBool(t, ctx, `(async () => {
		const words = new Set();
		const add = (v) => {
			if (typeof v === "string") for (const w of v.split(/[\s·,:()\[\]]+/)) words.add(w.replace(/[.!?;"“”'’]+$/, ""));
			else if (Array.isArray(v)) v.forEach(add);
			else if (v && typeof v === "object") Object.values(v).forEach(add);
		};
		const defs = await (await fetch("/api/game-defs")).json();
		add(defs);
		for (const d of defs) {
			add(await (await fetch("/api/game-defs/" + encodeURIComponent(d.id))).json().catch(() => null));
			if (d.tutorials) add(await (await fetch("/api/game-defs/" + encodeURIComponent(d.id) + "/tutorials")).json().catch(() => null));
		}
		add(await (await fetch("/api/bots")).json());
		add(await (await fetch("/api/badges")).json().catch(() => null));
		add(await (await fetch("/api/puzzles")).json().catch(() => null));
		window.__content = words;
		return words.size > 0;
	})()`) {
		t.Fatal("no game content learnt")
	}
}

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

// So is every word of the main page: the gallery, a game's setup with its
// options, the deal history, tutorials and active games, and the puzzles.
func TestWebLanguages_TheMainPageIsTranslated(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1200, 900), chromedp.Navigate(base+"/healthz"), chromedp.Evaluate(`document.cookie = "bingle_lang=en-XA; path=/"; true`, nil), chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the gallery", `document.querySelector(".game-card")`, 20*time.Second)
	learnContent(t, ctx)
	check := func(what string) {
		t.Helper()
		time.Sleep(400 * time.Millisecond)
		if got := evalString(t, ctx, plainEnglish+`("main")`); got != "" {
			t.Errorf("%s: %s", what, got)
		}
	}
	check("the gallery")
	families := strings.Fields(evalString(t, ctx, `[...document.querySelectorAll(".game-card")].map((c) => c.dataset.family).join(" ")`))
	if len(families) < 10 {
		t.Fatalf("the gallery has %d games", len(families))
	}
	for _, family := range families {
		clickSel(t, ctx, `.game-card[data-family="`+family+`"]`)
		waitJS(t, ctx, "the setup of "+family, `!document.getElementById("game-setup").hidden`, 10*time.Second)
		check("the setup of " + family)
		// The opponents' choices change the form: the bots, and importing a move log.
		if evalBool(t, ctx, `!!document.getElementById("opponent-bots")`) {
			evalBool(t, ctx, `(document.querySelector("#opponent-bots input").click(), true)`)
			check(family + " with two bots")
			evalBool(t, ctx, `(document.querySelector('input[name="opponent"][value=""]').click(), true)`)
		}
		for _, tab := range []string{"tutorials", "active", "deals"} {
			if evalBool(t, ctx, `!document.getElementById("setup-tab-`+tab+`").hidden`) {
				clickSel(t, ctx, "#setup-tab-"+tab)
				check(family + "'s " + tab)
			}
		}
		clickSel(t, ctx, "#setup-tab-new")
		clickSel(t, ctx, "#setup-back")
	}
	clickSel(t, ctx, "#tab-puzzles")
	waitJS(t, ctx, "the puzzles", `document.querySelector(".puzzle-card")`, 15*time.Second)
	check("the puzzles")
}

// So is every word of the main page's other tabs (Analysis, Ratings,
// Tournaments with its form, Global Chat), a tournament's page while it is
// played, and the shelf of badges.
func TestWebLanguages_TheOtherTabsAreTranslated(t *testing.T) {
	a, base := membersServer(t)
	org := administrator(t, a, base, "org")
	ana, bea := member(t, a, base, "ana"), member(t, a, base, "bea")
	var made session.TournamentDetail
	if code := org.do("POST", "/api/tournaments", cupBody(), &made); code != http.StatusCreated {
		t.Fatalf("making the tournament: %d", code)
	}
	path := fmt.Sprintf("/api/tournaments/%d", made.ID)
	org.do("POST", path+"/open", nil, nil)
	ana.do("POST", path+"/enter", nil, nil)
	bea.do("POST", path+"/enter", nil, nil)
	if code := org.do("POST", path+"/start", nil, nil); code != 200 {
		t.Fatalf("starting it: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, org.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1200, 900), chromedp.Navigate(base+"/healthz"), chromedp.Evaluate(`document.cookie = "bingle_lang=en-XA; path=/"; true`, nil), chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the tabs", `document.getElementById("tab-analysis")`, 20*time.Second)
	learnContent(t, ctx)
	check := func(what, sel string) {
		t.Helper()
		time.Sleep(600 * time.Millisecond)
		if !evalBool(t, ctx, `!!window.__content`) {
			learnContent(t, ctx)
		}
		if got := evalString(t, ctx, plainEnglish+`("`+sel+`")`); got != "" {
			t.Errorf("%s: %s", what, got)
		}
	}
	for _, tab := range []string{"analysis", "ratings", "tournaments", "chat"} {
		clickSel(t, ctx, "#tab-"+tab)
		check("the "+tab+" tab", "#panel-"+tab)
	}
	clickSel(t, ctx, "#tab-tournaments")
	clickSel(t, ctx, "#tournament-new")
	waitJS(t, ctx, "the form", `document.getElementById("t-game") && document.getElementById("t-game").options.length > 1`, 10*time.Second)
	check("a new tournament's form", "#panel-tournaments")
	if err := chromedp.Run(ctx, chromedp.Navigate(fmt.Sprintf("%s/tournament/%d", base, made.ID))); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the tournament", `document.getElementById("tournament-standings")`, 20*time.Second)
	check("a tournament being played", "main")
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/profile#badges")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the badges", `document.querySelector("#badges-list .badge-item")`, 20*time.Second)
	check("the badges", "#panel-badges")
}

// So is every word of the welcome tour, its pictures included, for a guest on a
// server with accounts (whose slides say what an account adds).
func TestWebLanguages_TheWelcomeTourIsTranslated(t *testing.T) {
	_, base := accountServer(t)
	ctx, cancel := newChromedpContextWithTour(t, 120*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1200, 900), chromedp.Navigate(base+"/healthz"), chromedp.Evaluate(`document.cookie = "bingle_lang=en-XA; path=/"; true`, nil), chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the tour", `document.querySelector("#welcome-tour[open]")`, 20*time.Second)
	learnContent(t, ctx)
	n := evalInt(t, ctx, `document.querySelectorAll("#welcome-tour .tour-dot").length`)
	if n < 8 {
		t.Fatalf("the tour has %d slides", n)
	}
	for i := 0; i < n; i++ {
		id := evalString(t, ctx, `document.getElementById("welcome-tour").dataset.slide`)
		waitJS(t, ctx, "the picture of "+id, `(() => { const st = document.querySelector("#tour-picture .tour-stage"); return st && st.childElementCount > 0; })()`, 15*time.Second)
		time.Sleep(300 * time.Millisecond)
		if got := evalString(t, ctx, plainEnglish+`("#welcome-tour")`); got != "" {
			t.Errorf("slide %s: %s", id, got)
		}
		if i < n-1 {
			clickSel(t, ctx, "#tour-next")
		}
	}
}

// So is every word around a game's board: the lobby of a table that waits for
// its host, and at a game, the chat (with its emoji) and the rules window's
// tabs of the platform's own (Controls and Key).
func TestWebLanguages_TheLobbyChatAndRulesAreTranslated(t *testing.T) {
	base := newRESTTestServer(t)
	var lobby, game createGameResponse
	doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "test-council", "player_name": "Alice", "lobby": true}, &lobby)
	doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "tak-5x5", "player_name": "Alice"}, &game)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 1000), chromedp.Navigate(base+"/healthz"), chromedp.Evaluate(`document.cookie = "bingle_lang=en-XA; path=/"; true`, nil)); err != nil {
		t.Fatal(err)
	}
	check := func(what, sel string) {
		t.Helper()
		time.Sleep(500 * time.Millisecond)
		if !evalBool(t, ctx, `!!window.__content`) {
			learnContent(t, ctx)
		}
		if got := evalString(t, ctx, plainEnglish+`("`+sel+`")`); got != "" {
			t.Errorf("%s: %s", what, got)
		}
	}
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`localStorage.setItem("bingle.games", JSON.stringify([{code: "`+lobby.RoomCode+`", def: "test-council", name: "Alice", ts: Date.now()}])); localStorage.setItem("bingle.name", '"Alice"'); true`, nil),
		chromedp.Navigate(base+"/game/"+lobby.RoomCode)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the lobby", `document.getElementById("lobby-seats")`, 15*time.Second)
	check("the lobby", "#lobby")

	openGameAs(t, ctx, base, game.RoomCode, "tak-5x5", "Alice")
	waitJS(t, ctx, "the chat", `document.getElementById("chat-card")`, 15*time.Second)
	clickSel(t, ctx, "#emoji-button")
	waitJS(t, ctx, "the emoji", `!document.getElementById("emoji-panel").hidden`, 5*time.Second)
	check("the chat and its emoji", "#chat-card")
	if !evalBool(t, ctx, `[...document.querySelectorAll("#emoji-panel .emoji-cell")].every((b) => /[À-ɏ]/.test(b.title))`) {
		t.Error("an emoji's name is not in the pseudo-locale")
	}
	clickSel(t, ctx, "#show-rules")
	waitJS(t, ctx, "the rules", `document.querySelector("#rules-dialog[open] .rules-tab")`, 15*time.Second)
	check("the rules window's head", "#rules-dialog .rules-head")
	for _, tab := range []string{"controls", "key"} {
		if !evalBool(t, ctx, `!!document.getElementById("rules-tab-`+tab+`")`) {
			continue
		}
		clickSel(t, ctx, "#rules-tab-"+tab)
		check("the rules' "+tab, "#rules-panel-"+tab+", #rules-tab-"+tab)
	}
}
