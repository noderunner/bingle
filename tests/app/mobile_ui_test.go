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

	// A phone: narrow, touch, mobile. The layout is chosen by the screen alone.
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(390, 844, chromedp.EmulateMobile, chromedp.EmulateTouch)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the phone layout", `document.body.classList.contains("m-mode") && document.getElementById("m-track") !== null`, 15*time.Second)
	waitJS(t, ctx, "the map", `document.querySelector("#m-page-map #board svg.area-map") !== null`, 15*time.Second)

	shoot := func(name string) {
		shot(t, ctx, name, func(b *[]byte) error { return chromedp.Run(ctx, chromedp.CaptureScreenshot(b)) })
	}
	shoot("phone-map")

	if got := evalString(t, ctx, `[...document.querySelectorAll("#m-track .m-page")].map(p => p.dataset.page).join(",")`); got != "map,actions,round,cult-1,cult-2,cult-3,cult-4,log,chat" {
		t.Errorf("pages %q", got)
	}
	if over := evalInt(t, ctx, `document.documentElement.scrollWidth - window.innerWidth`); over > 0 {
		t.Errorf("the page scrolls sideways by %d px", over)
	}
	// Nothing tappable is smaller than a thumb.
	if small := evalString(t, ctx, `[...document.querySelectorAll("#m-nav button, #m-page-map .move-bar button")].filter(b => b.getBoundingClientRect().height < 40).map(b => b.textContent.trim()).join("|")`); small != "" {
		t.Errorf("targets under 40px tall: %s", small)
	}

	// Playing works from the map page: the first player picks a direction with a thumb tap.
	if evalString(t, ctx, `document.querySelector("#m-page-map .status-bar .message").textContent`) == "" {
		t.Fatal("the map page shows the status")
	}
	evalString(t, ctx, `document.querySelector("#m-page-map .m-dock .move-bar button").click(); "ok"`)
	waitJS(t, ctx, "the move was played", `!/first player/.test(document.querySelector("#m-page-map .status-bar .message").textContent)`, 15*time.Second)

	// Each button of the bar brings its view to the front, and a swipe-sized scroll follows.
	for _, v := range []struct{ group, page, shot string }{
		{"actions", "actions", "phone-actions"},
		{"round", "round", "phone-round"},
		{"cults", "cult-1", "phone-cult"},
		{"log", "log", "phone-history"},
		{"chat", "chat", "phone-chat"},
		{"map", "map", "phone-map-again"},
	} {
		evalString(t, ctx, fmt.Sprintf(`document.querySelector('#m-nav [data-group="%s"]').click(); "ok"`, v.group))
		waitJS(t, ctx, v.page+" on screen", fmt.Sprintf(`(() => { const t = document.getElementById("m-track"); const p = document.getElementById("m-page-%s"); return Math.abs(p.getBoundingClientRect().left - t.getBoundingClientRect().left) < 2; })()`, v.page), 5*time.Second)
		waitJS(t, ctx, v.group+" lit", fmt.Sprintf(`document.querySelector('#m-nav [data-group="%s"]').classList.contains("on")`, v.group), 5*time.Second)
		time.Sleep(300 * time.Millisecond)
		shoot(v.shot)
	}

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
