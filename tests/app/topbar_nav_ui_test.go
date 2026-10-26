package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// The bar at the top of a game: the active games, joining a game and the
// profile, and a voice call that stays up when the player goes to another game
// or to the main page.

// rememberGames tells the browser which games it has played (name and codes).
func rememberGames(t *testing.T, ctx context.Context, baseURL, name string, games map[string]string) {
	t.Helper()
	var list []map[string]any
	ts := time.Now().UnixMilli()
	for code, def := range games {
		list = append(list, map[string]any{"code": code, "def": def, "name": name, "ts": ts})
		ts--
	}
	raw, _ := json.Marshal(list)
	nameJSON, _ := json.Marshal(name)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/healthz"),
		chromedp.Evaluate(fmt.Sprintf(`localStorage.setItem("bingle.games", %q); localStorage.setItem("bingle.name", %q); true`, raw, nameJSON), nil),
	); err != nil {
		t.Fatal(err)
	}
}

func botRoom(t *testing.T, baseURL, name string) string {
	t.Helper()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": name, "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	return created.RoomCode
}

func TestWebGame_TheTopBarHasTheActiveGamesJoinAndProfile(t *testing.T) {
	baseURL := newRESTTestServer(t)
	a, b := botRoom(t, baseURL, "Alice"), botRoom(t, baseURL, "Alice")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	rememberGames(t, ctx, baseURL, "Alice", map[string]string{a: "tic-tac-toe", b: "tic-tac-toe"})
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/game/"+a), chromedp.WaitReady("#board-svg, #card-table", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"open-active-games", "open-join", "open-profile"} {
		if n := evalInt(t, ctx, `document.querySelectorAll('.topbar #`+id+`').length`); n != 1 {
			t.Errorf("%d #%s in the top bar of the game", n, id)
		}
	}
	// It is Alice's turn in both games: the badge counts them, including this one.
	waitJS(t, ctx, "the badge", `document.getElementById("active-badge") && !document.getElementById("active-badge").hidden && document.getElementById("active-badge").textContent === "2"`, 15*time.Second)
	// The title and the icon of the tab are this game's own.
	if got := evalString(t, ctx, `document.title`); strings.HasPrefix(got, "(") {
		t.Errorf("the tab title is taken by the count: %q", got)
	}
	// The list names the other game and has a way to open it.
	clickSel(t, ctx, "#open-active-games")
	waitJS(t, ctx, "the list", `document.querySelector('#my-games li[data-code="`+b+`"] a[href="/game/`+b+`"]') !== null`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#my-games li .badge.turn').length`); n != 2 {
		t.Errorf("%d games say it is your turn, want 2", n)
	}
	// Joining by code, from inside a game.
	clickSel(t, ctx, "#active-games-dialog .dialog-close")
	clickSel(t, ctx, "#open-join")
	waitJS(t, ctx, "the join form", `document.getElementById("room_code") !== null && document.getElementById("join-dialog").open`, 5*time.Second)
	// The profile is a link.
	if got := evalString(t, ctx, `document.getElementById("open-profile").getAttribute("href")`); got != "/profile" {
		t.Errorf("profile link = %q", got)
	}
}

// With no call, a link loads the next page as it always did.
func TestWebGame_WithNoCallALinkLoadsThePage(t *testing.T) {
	baseURL := newRESTTestServer(t)
	a := botRoom(t, baseURL, "Alice")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	rememberGames(t, ctx, baseURL, "Alice", map[string]string{a: "tic-tac-toe"})
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/game/"+a), chromedp.WaitReady("#board-svg", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.__marker = 1; true`, nil)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, ".topbar a.brand")
	waitJS(t, ctx, "the main page", `location.pathname === "/" && document.getElementById("create-game-form") !== null`, 15*time.Second)
	if evalBool(t, ctx, `window.__marker === 1`) {
		t.Error("the document was not reloaded by a plain link")
	}
}

func TestVoiceUI_ACallStaysUpWhenThePlayerGoesToAnotherGameOrTheMainPage(t *testing.T) {
	baseURL := newRESTTestServer(t)
	call := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	joinRoom(t, baseURL, call, "Bob")
	other := botRoom(t, baseURL, "Alice")

	alice, cancelA := newChromedpContextWith(t, 120*time.Second, fakeMic...)
	defer cancelA()
	bob, cancelB := newChromedpContextWith(t, 120*time.Second, fakeMic...)
	defer cancelB()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(alice, &mu, &jsErrors)
	rememberGames(t, alice, baseURL, "Alice", map[string]string{call: "tic-tac-toe", other: "tic-tac-toe"})
	openGameAs(t, bob, baseURL, call, "tic-tac-toe", "Bob")
	if err := chromedp.Run(alice, chromedp.Navigate(baseURL+"/game/"+call), chromedp.WaitReady("#board-svg", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, alice, "voice card", `document.getElementById("voice-join")`, 15*time.Second)
	waitJS(t, bob, "voice card", `document.getElementById("voice-join")`, 15*time.Second)
	clickSel(t, alice, "#voice-join")
	waitJS(t, alice, "Alice in the call", `document.getElementById("voice-badge").textContent === "in call"`, 15*time.Second)
	clickSel(t, bob, "#voice-join")
	connected := `(function(){ const r = document.querySelector('#voice-peers li[data-voice-seat="%d"]:not([data-me])'); return !!r && r.dataset.state === "connected"; })()`
	waitJS(t, alice, "connected to Bob", fmt.Sprintf(connected, 2), 45*time.Second)
	waitJS(t, bob, "connected to Alice", fmt.Sprintf(connected, 1), 45*time.Second)
	// In this very game the button of the call is there, without a room code.
	waitJS(t, alice, "the call's button", `document.getElementById("voice-chip") && !document.getElementById("voice-chip").hidden`, 5*time.Second)
	if got := evalString(t, alice, `document.getElementById("voice-chip-code").textContent`); got != "" {
		t.Errorf("the button names the room %q though the call is in this game", got)
	}

	// To another game, by the list of the active games.
	if err := chromedp.Run(alice, chromedp.Evaluate(`window.__marker = 42; true`, nil)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, alice, "#open-active-games")
	waitJS(t, alice, "the list", `document.querySelector('#my-games li[data-code="`+other+`"] a') !== null`, 10*time.Second)
	clickSel(t, alice, `#my-games li[data-code="`+other+`"] a`)
	waitJS(t, alice, "the other game", `location.pathname === "/game/`+other+`" && document.getElementById("board-svg") !== null && document.body.dataset.roomCode === "`+other+`"`, 20*time.Second)
	if !evalBool(t, alice, `window.__marker === 42`) {
		t.Fatal("the document was reloaded: the call is gone")
	}
	// The call is still up: its button names its game, the audio plays, the card says where the call is.
	waitJS(t, alice, "the room on the button", `document.getElementById("voice-chip-code").textContent === "`+call+`"`, 10*time.Second)
	if n := evalInt(t, alice, `document.querySelectorAll('#voice-audio audio').length`); n != 1 {
		t.Errorf("%d audio elements, want the one of Bob", n)
	}
	waitJS(t, alice, "the card says where the call is", `document.getElementById("voice-elsewhere") !== null && document.getElementById("voice-elsewhere").textContent.indexOf("`+call+`") >= 0`, 10*time.Second)
	// Bob's side did not notice: still connected.
	if !evalBool(t, bob, fmt.Sprintf(connected, 1)) {
		t.Error("Bob lost the connection to Alice when she went to another game")
	}
	// The page is the new game's: its board, its title, one socket (the call's old one is not the page's).
	if got := evalString(t, alice, `document.title`); !strings.Contains(got, other) {
		t.Errorf("title = %q", got)
	}
	if n := evalInt(t, alice, `document.querySelectorAll('.topbar #open-active-games').length`); n != 1 {
		t.Errorf("%d active-games buttons after the move", n)
	}

	// To the main page, by the logo.
	clickSel(t, alice, ".topbar a.brand")
	waitJS(t, alice, "the main page", `location.pathname === "/" && document.getElementById("create-game-form") !== null`, 20*time.Second)
	if !evalBool(t, alice, `window.__marker === 42`) {
		t.Fatal("the document was reloaded on the way to the main page")
	}
	waitJS(t, alice, "the button on the main page", `document.getElementById("voice-chip") && !document.getElementById("voice-chip").hidden && document.getElementById("voice-chip-code").textContent === "`+call+`"`, 10*time.Second)
	if !evalBool(t, bob, fmt.Sprintf(connected, 1)) {
		t.Error("Bob lost the connection to Alice on the main page")
	}
	// The button opens the controls of the call, with a way back to its game.
	clickSel(t, alice, "#voice-chip")
	waitJS(t, alice, "the panel", `document.getElementById("voice-pop") && !document.getElementById("voice-pop").hidden && document.getElementById("voice-pop-go") !== null`, 5*time.Second)
	clickSel(t, alice, "#voice-pop-mute")
	waitJS(t, alice, "muted from the main page", `document.getElementById("voice-pop-mute").textContent === "Unmute"`, 5*time.Second)
	clickSel(t, alice, "#voice-pop-mute")
	clickSel(t, alice, "#voice-pop-go")
	waitJS(t, alice, "back in the game of the call", `location.pathname === "/game/`+call+`" && document.getElementById("voice-badge") && document.getElementById("voice-badge").textContent === "in call"`, 20*time.Second)
	if !evalBool(t, alice, `window.__marker === 42`) {
		t.Fatal("the document was reloaded on the way back")
	}
	if got := evalString(t, alice, `document.getElementById("voice-chip-code").textContent`); got != "" {
		t.Errorf("the button names the room %q in the game of the call", got)
	}
	waitJS(t, alice, "still connected", fmt.Sprintf(connected, 2), 20*time.Second)

	// Leaving ends it everywhere: the button goes, Bob sees her go.
	clickSel(t, alice, "#voice-leave")
	waitJS(t, alice, "the button is gone", `document.getElementById("voice-chip").hidden`, 5*time.Second)
	waitJS(t, bob, "Alice gone", `!document.querySelector('#voice-peers li[data-voice-seat="1"]')`, 10*time.Second)
	// With the call over, a link loads the page again.
	if err := chromedp.Run(alice, chromedp.Evaluate(`window.__marker = 7; true`, nil)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, alice, ".topbar a.brand")
	waitJS(t, alice, "the main page", `location.pathname === "/"`, 15*time.Second)
	if evalBool(t, alice, `window.__marker === 7`) {
		t.Error("a link still navigates softly with no call up")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
