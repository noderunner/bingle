package app_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/auth"
)

// cwTable starts a table with Alice in the first seat and bots in the others, and opens it.
func cwTable(t *testing.T, ctx context.Context, baseURL, def string, options map[string]any, bots map[string]string) string {
	t.Helper()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": def, "player_name": "Alice", "seat_bots": bots}
	if options != nil {
		body["options"] = options
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 && code != 201 {
		t.Fatalf("create: %d", code)
	}
	var game struct {
		GameDefID string `json:"game_def_id"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &game)
	openGameAs(t, ctx, baseURL, created.RoomCode, game.GameDefID, "Alice")
	waitJS(t, ctx, "the game", `document.getElementById("board-svg") !== null`, 30*time.Second)
	return created.RoomCode
}

var threeBots = map[string]string{"2": "tactical", "3": "tactical", "4": "tactical"}

const cardsJS = `[...document.querySelectorAll('#cult-stack .seat-strip')].map(s => s.getAttribute("data-seat")).join(",")`

// The cult cards follow the order of play and say who they are with a glyph, and
// the note about the order is gone.
func TestWebCthulhuWars_TheCultCardsFollowTheOrderOfPlay(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	cwTable(t, ctx, baseURL, "cthulhu-wars", nil, threeBots)
	waitJS(t, ctx, "the choice of direction", `document.getElementById("action-ccw") !== null`, 30*time.Second)
	if got := evalString(t, ctx, cardsJS); got != "1,2,3,4" {
		t.Errorf("cards before the choice: %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll("#seat-order-note").length`); n != 0 {
		t.Error("the note about the order is still there")
	}
	if n := evalInt(t, ctx, `document.querySelectorAll("#cult-stack .seat-strip .seat-glyph").length`); n != 4 {
		t.Errorf("%d glyphs on four cards", n)
	}
	waitJS(t, ctx, "the glyphs to load", `[...document.querySelectorAll(".seat-glyph")].every(i => i.complete && i.naturalWidth > 0)`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#seat-1 .seat-head").firstElementChild.className`); got != "seat-glyph" {
		t.Errorf("first on the card: %q", got)
	}
	// A human's card no longer spells out the faction; the glyph says it on hover.
	if n := evalInt(t, ctx, `document.querySelectorAll("#seat-1 .seat-faction").length`); n != 0 {
		t.Error("Alice's card still names her faction")
	}
	if got := evalString(t, ctx, `document.querySelector("#seat-1 .seat-glyph").title`); got != "Great Cthulhu" {
		t.Errorf("the glyph says %q", got)
	}
	clickSel(t, ctx, "#action-ccw")
	waitJS(t, ctx, "the cards turned round", cardsJS+` === "1,4,3,2"`, 15*time.Second)
}

// A cult that chooses where to begin is told so plainly; the Tcho-Tcho are pink and
// Ubbo-Sathla's Growth is an icon.
func TestWebCthulhuWars_ChoosingAStartPinkAndGrowth(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	cwTable(t, ctx, baseURL, "cthulhu-wars", map[string]any{"factions": []any{"tt", "gc", "cc", "bg"}}, threeBots)
	waitJS(t, ctx, "the prompt", `document.getElementById("impulse-card") && document.getElementById("impulse-card").textContent.includes("Choose an empty area marked with a faction glyph where your cult begins.")`, 30*time.Second)
	if got := evalString(t, ctx, `document.getElementById("seat-1").style.getPropertyValue("--seat-ink")`); !strings.Contains(got, "--piece-brown-fill") {
		t.Errorf("the Tcho-Tcho's colour is %s", got)
	}
	el := `document.querySelector('#seat-1 [data-counter="growth"]')`
	if !evalBool(t, ctx, el+`.querySelector("svg.stat-growth") !== null`) {
		t.Error("Growth has no icon")
	}
	if got := evalString(t, ctx, el+`.textContent`); strings.Contains(got, "Growth") {
		t.Errorf("Growth still has words: %q", got)
	}
	if got := evalString(t, ctx, el+`.title`); !strings.Contains(got, "Growth") {
		t.Errorf("the icon's title: %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

const logAtEndJS = `(function () { var e = document.getElementById("events") || document.getElementById("move-list"); return e.scrollHeight > e.clientHeight && e.scrollHeight - e.scrollTop - e.clientHeight < 3; })()`

// Coming back to the history or the moves shows what is newest.
func TestWebCthulhuWars_TheLogOpensOnItsEnd(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	cwTable(t, ctx, baseURL, "cthulhu-wars", nil, threeBots)
	clickSel(t, ctx, "#action-cw")
	waitJS(t, ctx, "some history", `document.getElementById("history-notation") !== null`, 30*time.Second)
	// Small enough that even a log of one line has somewhere to scroll.
	evalString(t, ctx, `(function(){ var s = document.createElement("style"); s.textContent = "#events, #move-list { height: 6px !important; min-height: 0 !important; max-height: 6px !important; overflow-y: auto !important; }"; document.head.append(s); return "ok"; })()`)
	evalString(t, ctx, `(function(){ var b = document.getElementById("rail-tab-history"); if (b) b.click(); return "ok"; })()`)
	clickSel(t, ctx, "#history-notation")
	waitJS(t, ctx, "the moves at their end", logAtEndJS, 5*time.Second)
	clickSel(t, ctx, "#history-friendly")
	waitJS(t, ctx, "the history at its end", logAtEndJS, 5*time.Second)
}

// The rules open on the rules, not on a keyboard, fit a phone, and Back closes them.
func TestWebRules_OnAPhone(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(390, 844)); err != nil {
		t.Fatal(err)
	}
	cwTable(t, ctx, baseURL, "cthulhu-wars", nil, threeBots)
	clickSel(t, ctx, "#show-rules")
	waitJS(t, ctx, "the rules", `document.querySelector("#rules-dialog .rules-panel") !== null`, 15*time.Second)
	if got := evalString(t, ctx, `document.activeElement.id || document.activeElement.className`); got == "rules-search" {
		t.Error("the search box took the focus")
	}
	if n := evalInt(t, ctx, `(function(){ var b = document.querySelector(".rules-close").getBoundingClientRect(); return b.left >= 0 && b.right <= innerWidth && b.top >= 0 && b.height >= 44 && b.width >= 44 ? 1 : 0; })()`); n != 1 {
		t.Errorf("the close button: %s", evalString(t, ctx, `JSON.stringify(document.querySelector(".rules-close").getBoundingClientRect())`))
	}
	if got := evalInt(t, ctx, `(function(){ var b = document.querySelector(".rules-body"); return b.scrollWidth - b.clientWidth; })()`); got > 1 {
		t.Errorf("the page is %d px wider than the phone", got)
	}
	path := evalString(t, ctx, `location.pathname`)
	// Back closes the window and stays in the game.
	evalString(t, ctx, `history.back(); "ok"`)
	waitJS(t, ctx, "the rules closed", `document.getElementById("rules-dialog") === null`, 5*time.Second)
	if got := evalString(t, ctx, `location.pathname`); got != path {
		t.Errorf("Back left for %q", got)
	}
	// So does the button, and it leaves no step behind for Back to trip on.
	before := evalInt(t, ctx, `history.length`)
	clickSel(t, ctx, "#show-rules")
	waitJS(t, ctx, "the rules again", `document.querySelector("#rules-dialog .rules-close") !== null`, 15*time.Second)
	clickSel(t, ctx, ".rules-close")
	waitJS(t, ctx, "the rules closed again", `document.getElementById("rules-dialog") === null`, 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	if got := evalString(t, ctx, `location.pathname`); got != path {
		t.Errorf("closing left for %q (history %d)", got, before)
	}
}

// Chat lines: the name, then the message under it, on alternating backgrounds; a
// badge counts what comes in while another tab or view is up.
func TestWebCthulhuWars_ChatLinesAndTheirBadge(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	room := cwTable(t, ctx, baseURL, "cthulhu-wars", nil, threeBots)
	say := func(text string) {
		t.Helper()
		if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/chat", map[string]any{"player_name": "Visitor", "text": text}, nil); code != 200 && code != 201 {
			t.Fatalf("chat: %d", code)
		}
	}
	waitJS(t, ctx, "the tabs", `document.getElementById("rail-tab-talk") !== null`, 15*time.Second)
	clickSel(t, ctx, "#rail-tab-cults")
	say("first")
	waitJS(t, ctx, "the badge", `!document.getElementById("rail-chat-unread").hidden && document.getElementById("rail-chat-unread").textContent === "1"`, 10*time.Second)
	say("second")
	waitJS(t, ctx, "the badge counting", `document.getElementById("rail-chat-unread").textContent === "2"`, 10*time.Second)
	clickSel(t, ctx, "#rail-tab-talk")
	waitJS(t, ctx, "the badge gone", `document.getElementById("rail-chat-unread").hidden`, 5*time.Second)
	// With the chat on screen a message is read as it comes.
	say("third")
	waitJS(t, ctx, "the third line", `document.querySelectorAll("#chat-log .chat-line").length === 3`, 10*time.Second)
	if !evalBool(t, ctx, `document.getElementById("rail-chat-unread").hidden`) {
		t.Error("a badge for a message on screen")
	}
	// A guest is marked as one, on the name's line, above the message.
	if err := chromedp.Run(ctx, chromedp.SendKeys("#chat-input", "from a guest\r", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the guest's line", `document.querySelector("#chat-log .chat-line .chat-guest") !== null`, 10*time.Second)
	if !evalBool(t, ctx, `(function(){ var g = document.querySelector("#chat-log .chat-guest"), l = g.closest(".chat-line"); return g.textContent === "guest" && g.getBoundingClientRect().bottom <= l.querySelector(".chat-text").getBoundingClientRect().top + 1; })()`) {
		t.Error("a guest's line does not say so above the message")
	}
	// The text stands under the name, and neighbours differ in background.
	if !evalBool(t, ctx, `(function(){ var l = document.querySelector("#chat-log .chat-line"); return l.querySelector(".chat-text").getBoundingClientRect().top >= l.querySelector(".chat-who").getBoundingClientRect().bottom - 1; })()`) {
		t.Error("the message is not under the name")
	}
	if !evalBool(t, ctx, `(function(){ var l = document.querySelectorAll("#chat-log .chat-line"); return getComputedStyle(l[0]).backgroundColor !== getComputedStyle(l[1]).backgroundColor; })()`) {
		t.Error("neighbouring lines have the same background")
	}

	// On a phone the bottom bar counts them.
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(390, 844)); err != nil {
		t.Fatal(err)
	}
	evalString(t, ctx, `localStorage.setItem("bingle.layout", JSON.stringify("phone")); location.reload(); "ok"`)
	waitJS(t, ctx, "the phone layout", `document.getElementById("m-nav") !== null`, 20*time.Second)
	say("fourth")
	waitJS(t, ctx, "the bar's badge", `(function(){ var b = document.querySelector('#m-nav [data-group="chat"] .m-nav-badge'); return b && !b.hidden && b.textContent === "1"; })()`, 10*time.Second)
	clickSel(t, ctx, `#m-nav [data-group="chat"]`)
	waitJS(t, ctx, "the bar's badge gone", `(function(){ var b = document.querySelector('#m-nav [data-group="chat"] .m-nav-badge'); return !b || b.hidden; })()`, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// An account's chat line shows its handle beside the name, above the message.
func TestWebCthulhuWars_ChatLineShowsTheHandle(t *testing.T) {
	a, base := membersServer(t)
	ana := member(t, a, base, "ana")
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := ana.do("POST", "/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Ana the Bold", "seat_bots": threeBots}, &created); code != 200 && code != 201 {
		t.Fatalf("create: %d", code)
	}
	if code := ana.do("POST", "/api/games/"+created.RoomCode+"/chat", map[string]any{"player_name": "Ana the Bold", "text": "hello"}, nil); code != 200 && code != 201 {
		t.Fatalf("chat: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, ana.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/game/"+created.RoomCode)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the line", `document.querySelector("#chat-log .chat-line .chat-text") !== null`, 30*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#chat-log .chat-line .chat-handle").textContent`); got != "@ana" {
		t.Errorf("the handle reads %q", got)
	}
	if got := evalString(t, ctx, `document.querySelector("#chat-log .chat-line .chat-who").textContent`); got != "Ana the Bold" {
		t.Errorf("the name reads %q", got)
	}
}
