package app_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/auth"
)

// The home page is a fun fact over one tabbed card; the games, joining and
// the profile live behind the icons of the top bar.
func TestHomePage_IsTabbedAndTidy(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/"),
		chromedp.WaitVisible(`.game-card[data-family="tak-5x5"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatal(err)
	}
	// What moved away.
	for _, gone := range []string{".hero", "#botwatch-card", "#import-card", "#my-games-card", "#puzzles-card", "main #join-game-form", ".tagline"} {
		if evalBool(t, ctx, fmt.Sprintf(`!!document.querySelector(%q)`, gone)) {
			t.Errorf("%s is still on the home page", gone)
		}
	}
	// The fun fact, in italics.
	if got := evalString(t, ctx, `document.getElementById("fun-fact").textContent`); len(got) < 40 {
		t.Errorf("fun fact = %q, want a sentence", got)
	}
	if got := evalString(t, ctx, `getComputedStyle(document.getElementById("fun-fact")).fontStyle`); got != "italic" {
		t.Errorf("fun fact font-style = %q, want italic", got)
	}
	// The tabs of the page itself, Games first. (The windows of the top bar
	// have tabs of their own, in their own tablists.)
	if got := evalString(t, ctx, `[...document.getElementById("tab-games").parentElement.querySelectorAll('[role="tab"]')].map((x) => x.firstChild.textContent.trim()).join("|")`); got != "Games|Puzzles|Analysis|Global Chat" {
		t.Errorf("tabs = %q", got)
	}
	if !evalBool(t, ctx, `document.getElementById("tab-games").getAttribute("aria-selected") === "true" && document.getElementById("panel-puzzles").hidden`) {
		t.Error("the Games tab should be showing")
	}
	// The icons of the top bar.
	for _, id := range []string{"open-active-games", "open-join", "open-profile"} {
		if !evalBool(t, ctx, fmt.Sprintf(`!!document.getElementById(%q)`, id)) {
			t.Errorf("the top bar has no #%s", id)
		}
	}
	// No floating zoom/reset buttons on the previews of the maps.
	waitJS(t, ctx, "map preview", `document.querySelector('.game-card[data-family="cthulhu-wars"] .game-card-preview svg')`, 20*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll(".map-zoom").length`); n != 0 {
		t.Errorf("%d zoom button groups on the home page, want none", n)
	}
	// Tabs switch (with the keyboard too).
	clickSel(t, ctx, "#tab-puzzles")
	waitJS(t, ctx, "puzzles panel", `!document.getElementById("panel-puzzles").hidden && document.getElementById("panel-games").hidden`, 5*time.Second)
	clickSel(t, ctx, "#tab-chat")
	waitJS(t, ctx, "chat panel", `!document.getElementById("panel-chat").hidden && document.getElementById("global-chat-input")`, 5*time.Second)
}

// Picking a game shows its setup in the same tab; "All games" goes back.
func TestHomePage_SetupAppearsAfterPickingAGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/"),
		chromedp.WaitVisible(`.game-card[data-family="hex-11"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatal(err)
	}
	if !evalBool(t, ctx, `document.getElementById("game-setup").hidden`) {
		t.Fatal("the setup shows before a game is picked")
	}
	clickSel(t, ctx, `.game-card[data-family="hex-11"]`)
	waitJS(t, ctx, "setup", `!document.getElementById("game-setup").hidden && document.getElementById("gallery-view").hidden`, 5*time.Second)
	waitJS(t, ctx, "title and preview", `document.getElementById("setup-title").textContent.indexOf("Hex") === 0 && document.querySelector("#setup-preview svg")`, 10*time.Second)
	clickSel(t, ctx, "#setup-back")
	waitJS(t, ctx, "gallery", `document.getElementById("game-setup").hidden && !document.getElementById("gallery-view").hidden`, 5*time.Second)
}

// A different fun fact turns up now and then when the page is loaded again.
func TestHomePage_FunFactVariesOnReload(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	seen := map[string]bool{}
	for i := 0; i < 8; i++ {
		if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/"), chromedp.WaitVisible("#fun-fact", chromedp.ByID)); err != nil {
			t.Fatal(err)
		}
		waitJS(t, ctx, "fact", `document.getElementById("fun-fact").textContent.length > 20`, 5*time.Second)
		seen[evalString(t, ctx, `document.getElementById("fun-fact").textContent`)] = true
	}
	if len(seen) < 2 {
		t.Errorf("8 loads showed %d distinct fun facts, want a random one each time", len(seen))
	}
}

// The active-games window lists the games not finished; finished ones are on
// the profile page, where the settings are kept too.
func TestHomePage_ActiveGamesAndProfile(t *testing.T) {
	baseURL := newRESTTestServer(t)
	live := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	joinRoom(t, baseURL, live, "Bob")
	done := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	joinRoom(t, baseURL, done, "Bob")
	if code := doJSON(t, "POST", baseURL+"/api/games/"+done+"/resign", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatalf("resign: %d", code)
	}
	games, _ := json.Marshal([]map[string]any{
		{"code": live, "def": "tic-tac-toe", "name": "Alice", "ts": time.Now().UnixMilli()},
		{"code": done, "def": "tic-tac-toe", "name": "Alice", "ts": time.Now().UnixMilli() - 1000},
	})
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/healthz"),
		chromedp.Evaluate(fmt.Sprintf(`localStorage.setItem("bingle.games", %q); localStorage.setItem("bingle.name", JSON.stringify("Alice")); true`, games), nil),
		chromedp.Navigate(baseURL+"/"),
	); err != nil {
		t.Fatal(err)
	}
	// One game waits for Alice: the badge says 1 (the finished game is not counted).
	waitJS(t, ctx, "badge", `(function(){ const b = document.getElementById("active-badge"); return b && !b.hidden && b.textContent === "1"; })()`, 10*time.Second)
	clickSel(t, ctx, "#open-active-games")
	waitJS(t, ctx, "list", `document.querySelectorAll("#active-games-dialog[open] #my-games li").length === 1`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#my-games li").getAttribute("data-code")`); got != live {
		t.Errorf("active game = %q, want %q", got, live)
	}

	// The profile page: finished games ...
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/profile"), chromedp.WaitVisible("#profile-form", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "finished list", `document.querySelectorAll("#finished-games li").length === 1`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#finished-games li").getAttribute("data-code")`); got != done {
		t.Errorf("finished game = %q, want %q", got, done)
	}
	if got := evalString(t, ctx, `document.getElementById("profile_name").value`); got != "Alice" {
		t.Errorf("profile name = %q, want Alice", got)
	}
	// ... and settings that stay.
	evalString(t, ctx, `(function(){
		document.getElementById("profile_name").value = "Alicia";
		document.getElementById("profile_email").value = "alicia@example.com";
		document.getElementById("profile_notify").checked = true;
		return "ok";
	})()`)
	clickSel(t, ctx, "#profile-save")
	waitJS(t, ctx, "saved", `document.getElementById("profile-status").textContent === "Saved."`, 5*time.Second)
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/profile"), chromedp.WaitVisible("#profile-form", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, `[document.getElementById("profile_name").value, document.getElementById("profile_email").value, document.getElementById("profile_notify").checked].join("|")`); got != "Alicia|alicia@example.com|true" {
		t.Errorf("settings after reload = %q", got)
	}
	// The name is the one offered when joining a game.
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/"), chromedp.WaitVisible("#open-join", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#open-join")
	waitJS(t, ctx, "join name", `document.getElementById("join_player_name").value === "Alicia"`, 5*time.Second)
	// A bad address is not saved.
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/profile"), chromedp.WaitVisible("#profile-form", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	evalString(t, ctx, `document.getElementById("profile_email").value = "not-an-address"; "ok"`)
	clickSel(t, ctx, "#profile-save")
	waitJS(t, ctx, "rejected", `document.getElementById("profile-status").textContent.indexOf("email") >= 0`, 5*time.Second)
}

// Joining through the dialog of the top bar puts the player in the game.
func TestHomePage_JoinDialogJoinsAGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/"), chromedp.WaitVisible("#open-join", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#open-join")
	if err := chromedp.Run(ctx,
		chromedp.WaitVisible("#room_code", chromedp.ByID),
		chromedp.SendKeys("#room_code", strings.ToLower(room), chromedp.ByID),
		chromedp.SendKeys("#join_player_name", "Bob", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, `#join-game-form button[type="submit"]`)
	waitJS(t, ctx, "game page", `location.pathname === "/game/`+room+`"`, 15*time.Second)
	var game struct {
		Seats []struct {
			Name string `json:"name"`
		} `json:"seats"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game)
	if len(game.Seats) != 2 || game.Seats[1].Name != "Bob" {
		t.Errorf("seats = %+v, want Bob in seat 2", game.Seats)
	}
}

// The global chat (ADR 0073): accounts write, under their handles; everyone
// reads it, live.
func TestGlobalChat_RESTAndLive(t *testing.T) {
	a, baseURL := membersServer(t)
	ann, cy := member(t, a, baseURL, "ann"), member(t, a, baseURL, "cyd")

	var out map[string]any
	if code := ann.do("POST", "/api/chat", map[string]any{"player_name": "Ann", "text": "   "}, &out); code != 400 {
		t.Errorf("an empty message: status %d, want 400", code)
	}
	if code := newBrowser(t, baseURL).do("POST", "/api/chat", map[string]any{"player_name": "Gus", "text": "hi"}, &out); code != 403 {
		t.Errorf("a guest writes: status %d, want 403", code)
	}
	var first struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Text string `json:"text"`
	}
	// The name is the account's handle, whatever was typed.
	if code := ann.do("POST", "/api/chat", map[string]any{"player_name": "Somebody Else", "text": "hello  all"}, &first); code != 200 || first.ID == 0 || first.Text != "hello  all" || first.Name != "@ann" {
		t.Fatalf("say: %d %+v", code, first)
	}
	var log []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Text string `json:"text"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/chat", nil, &log); code != 200 || len(log) != 1 || log[0].Name != "@ann" {
		t.Fatalf("log: %d %+v", code, log)
	}
	if code := doJSON(t, "GET", baseURL+"/api/chat?after="+fmt.Sprint(first.ID), nil, &log); code != 200 || len(log) != 0 {
		t.Errorf("log after the last message: %d %+v", code, log)
	}

	// A guest's page shows the room and says writing is for accounts.
	guestCtx, cancelG := newChromedpContext(t, 60*time.Second)
	defer cancelG()
	if err := chromedp.Run(guestCtx, chromedp.Navigate(baseURL+"/#chat"), chromedp.WaitVisible("#global-chat-input", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, guestCtx, "a guest reads but does not write", `document.querySelector("#global-chat-log .chat-line .chat-text") && document.getElementById("global-chat-input").disabled`, 10*time.Second)

	// An account's page writes.
	dee := member(t, a, baseURL, "dee")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	signInChrome(t, ctx, baseURL, dee.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/#chat"), chromedp.WaitVisible("#global-chat-input", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	// The history is there ...
	waitJS(t, ctx, "history", `document.querySelector("#global-chat-log .chat-line .chat-text").textContent === "hello  all"`, 10*time.Second)
	waitJS(t, ctx, "the handle as the name", `document.getElementById("global-chat-name").value === "@dee" && !document.getElementById("global-chat-input").disabled`, 10*time.Second)
	// ... a message from somebody else arrives by itself ...
	time.Sleep(500 * time.Millisecond) // let the live feed connect
	if code := cy.do("POST", "/api/chat", map[string]any{"player_name": "Cy", "text": "pushed"}, &out); code != 200 {
		t.Fatalf("say: %d", code)
	}
	waitJS(t, ctx, "live line", `[...document.querySelectorAll("#global-chat-log .chat-text")].some((e) => e.textContent === "pushed")`, 10*time.Second)
	// ... and the page can write too.
	if err := chromedp.Run(ctx, chromedp.SendKeys("#global-chat-input", "from the page", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#global-chat-send")
	waitJS(t, ctx, "own line", `[...document.querySelectorAll("#global-chat-log .chat-line")].some((e) => e.textContent.indexOf("@dee") === 0 && e.textContent.indexOf("from the page") > 0)`, 10*time.Second)
	doJSON(t, "GET", baseURL+"/api/chat", nil, &log)
	if len(log) != 3 || log[2].Text != "from the page" {
		t.Errorf("stored log = %+v", log)
	}
}

// Messages that come in while another tab shows are counted on the chat tab.
func TestGlobalChat_UnreadCountOnTheTab(t *testing.T) {
	a, baseURL := membersServer(t)
	cy := member(t, a, baseURL, "cyd")
	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/"), chromedp.WaitVisible("#tab-chat", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(600 * time.Millisecond)
	cy.do("POST", "/api/chat", map[string]any{"player_name": "Cy", "text": "psst"}, nil)
	waitJS(t, ctx, "unread", `(function(){ const b = document.getElementById("chat-tab-badge"); return !b.hidden && b.textContent === "1"; })()`, 10*time.Second)
	clickSel(t, ctx, "#tab-chat")
	waitJS(t, ctx, "read", `document.getElementById("chat-tab-badge").hidden`, 5*time.Second)
}
