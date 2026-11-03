package app_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestWebCthulhuWarsPhoneLayoutSwipesBetweenViews(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode

	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)

	// A phone held upright: narrow, touch, mobile. The layout is chosen by the screen alone.
	upright := func() {
		t.Helper()
		if err := chromedp.Run(ctx, chromedp.EmulateViewport(390, 844, chromedp.EmulateMobile, chromedp.EmulateTouch, chromedp.EmulatePortrait)); err != nil {
			t.Fatal(err)
		}
	}
	// On its side, as wide as a large phone: wider than a phone held upright could be.
	sideways := func() {
		t.Helper()
		if err := chromedp.Run(ctx, chromedp.EmulateViewport(932, 430, chromedp.EmulateMobile, chromedp.EmulateTouch, chromedp.EmulateLandscape)); err != nil {
			t.Fatal(err)
		}
	}
	sideways()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	upright()
	waitJS(t, ctx, "the phone layout", `document.body.classList.contains("m-mode") && document.getElementById("m-track") !== null`, 15*time.Second)

	shoot := func(name string) {
		shot(t, ctx, name, func(b *[]byte) error { return chromedp.Run(ctx, chromedp.CaptureScreenshot(b)) })
	}
	shoot("phone-actions")

	// Upright there are the cards, and no map.
	if got := evalString(t, ctx, `[...document.querySelectorAll("#m-track .m-page")].map(p => p.dataset.page).join(",")`); got != "actions,round,cult-1,cult-2,cult-3,cult-4,log,chat" {
		t.Errorf("pages %q", got)
	}
	if evalBool(t, ctx, `document.getElementById("board") !== null`) {
		t.Error("the map is drawn on an upright phone")
	}
	if over := evalInt(t, ctx, `document.documentElement.scrollWidth - window.innerWidth`); over > 0 {
		t.Errorf("the page scrolls sideways by %d px", over)
	}
	// Nothing tappable is smaller than a thumb.
	if small := evalString(t, ctx, `[...document.querySelectorAll("#m-nav button, #m-page-actions .move-bar button")].filter(b => b.getBoundingClientRect().height < 40).map(b => b.textContent).join("; ")`); small != "" {
		t.Errorf("targets under 40px tall: %s", small)
	}

	// The buttons are played from here: the first player picks a direction with a thumb tap.
	if evalString(t, ctx, `document.querySelector("#m-page-actions .status-bar .message").textContent`) == "" {
		t.Fatal("the actions page shows the status")
	}
	evalString(t, ctx, `document.querySelector("#m-page-actions .move-bar button").click(); "ok"`)
	waitJS(t, ctx, "the move was played", `!/first player/.test(document.querySelector("#m-page-actions .status-bar .message").textContent)`, 15*time.Second)

	// Each button of the bar brings its view to the front, and a swipe-sized scroll follows.
	for _, v := range []struct{ group, page, shot string }{
		{"round", "round", "phone-round"},
		{"cults", "cult-1", "phone-cult"},
		{"log", "log", "phone-history"},
		{"chat", "chat", "phone-chat"},
		{"actions", "actions", "phone-actions-again"},
	} {
		evalString(t, ctx, fmt.Sprintf(`document.querySelector('#m-nav [data-group="%s"]').click(); "ok"`, v.group))
		waitJS(t, ctx, v.page+" on screen", fmt.Sprintf(`(() => { const t = document.getElementById("m-track"); const p = document.getElementById("m-page-%s"); return Math.abs(p.getBoundingClientRect().left - t.getBoundingClientRect().left) < 2; })()`, v.page), 5*time.Second)
		waitJS(t, ctx, v.group+" lit", fmt.Sprintf(`document.querySelector('#m-nav [data-group="%s"]').classList.contains("on")`, v.group), 5*time.Second)
		time.Sleep(300 * time.Millisecond)
		shoot(v.shot)
	}

	// Turned on its side, the whole screen is the map: no bars, and no cards.
	sideways()
	waitJS(t, ctx, "the map", `document.querySelector("#board svg.area-map") !== null`, 10*time.Second)
	shoot("phone-map")
	if !evalBool(t, ctx, `document.body.classList.contains("m-mode") && document.getElementById("m-track") === null && document.getElementById("m-nav") === null && document.querySelector(".move-bar") === null`) {
		t.Error("something other than the map on a phone on its side")
	}
	if evalBool(t, ctx, `document.querySelector(".topbar").offsetParent !== null`) {
		t.Error("the top bar takes room from the map")
	}
	// All of the screen, and all of the svg: a view taller than the map would draw it small in the middle.
	if !evalBool(t, ctx, `(function () { var r = document.querySelector("#board svg").getBoundingClientRect(); return r.height >= innerHeight - 2 && r.width >= innerWidth - 2; })()`) {
		t.Error("the map does not fill the screen")
	}
	if got := evalString(t, ctx, `document.querySelector("#board svg").getAttribute("viewBox")`); got != "-8 -8 1016 506" {
		t.Errorf("the map's view is %q, not the whole map", got)
	}
	// Whose turn it is can be told from here, without covering the taps.
	waitJS(t, ctx, "the turn", `document.getElementById("m-turn-note") !== null`, 15*time.Second)
	if got := evalString(t, ctx, `getComputedStyle(document.getElementById("m-turn-note")).pointerEvents`); got != "none" {
		t.Errorf("the note takes taps: %q", got)
	}
	// And back: the cards return, on the page the player left.
	upright()
	waitJS(t, ctx, "the cards again", `document.getElementById("m-track") !== null && document.getElementById("board") === null`, 10*time.Second)

	// A window made wider by an on-screen keyboard is not a phone turned: the device says it is upright.
	clickSel(t, ctx, `#m-nav [data-group="chat"]`)
	waitJS(t, ctx, "the chat", `document.getElementById("chat-input") !== null`, 5*time.Second)
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(390, 300, chromedp.EmulateMobile, chromedp.EmulateTouch, chromedp.EmulatePortrait)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if !evalBool(t, ctx, `document.getElementById("m-track") !== null && document.getElementById("board") === null`) {
		t.Error("the keyboard's shape turned the page to the map")
	}
	upright()

	// Forcing the desktop layout in the options brings the normal page back.
	evalString(t, ctx, `localStorage.setItem("bingle.layout", JSON.stringify("desktop")); "ok"`)
	if err := chromedp.Run(ctx, chromedp.Reload(), chromedp.WaitReady("#board-svg", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the desktop layout", `!document.body.classList.contains("m-mode") && document.getElementById("m-track") === null && document.querySelector(".game-layout") !== null`, 15*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
