// Tests of the list of active games and the ways to find a game: the ✕ that
// resigns, a game's own Active Games tab, the tables in the Join Game window.

package app_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func gameStatus(t *testing.T, baseURL, room string) (status, reason string) {
	t.Helper()
	var g struct {
		Status    string `json:"status"`
		EndReason string `json:"end_reason"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &g); code != 200 {
		t.Fatalf("read %s: %d", room, code)
	}
	return g.Status, g.EndReason
}

// gameOfThisBrowser makes a game from the page, so that its guest holds the seat.
func gameOfThisBrowser(t *testing.T, ctx context.Context, baseURL, body string) string {
	t.Helper()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/healthz")); err != nil {
		t.Fatal(err)
	}
	var room string
	create := `fetch("/api/games", {method: "POST", headers: {"Content-Type": "application/json", "X-Bingle-Session": "1"}, body: JSON.stringify(` + body + `)}).then((r) => r.json()).then((g) => g.room_code)`
	if err := chromedp.Run(ctx, chromedp.Evaluate(create, &room, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil || room == "" {
		t.Fatalf("create: %q %v", room, err)
	}
	return room
}

func openActiveGames(t *testing.T, ctx context.Context, baseURL, name string, rooms map[string]string) {
	t.Helper()
	rememberGames(t, ctx, baseURL, name, rooms)
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the top bar", `document.getElementById("open-active-games") !== null`, 20*time.Second)
	clickSel(t, ctx, "#open-active-games")
}

// The ✕ of a game in play resigns from it, once it has asked; answered no, nothing happens.
func TestWebActiveGames_TheXResignsAfterAsking(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	room := gameOfThisBrowser(t, ctx, baseURL, `{game_def_id: "gomoku", player_name: "Ana", opponent: "random"}`)
	openActiveGames(t, ctx, baseURL, "Ana", map[string]string{room: "gomoku"})
	row := `document.querySelector('#my-games li[data-code="` + room + `"]')`
	waitJS(t, ctx, "the row", row+` !== null`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll("#active-games-dialog .tab").length`); n != 0 {
		t.Errorf("the window still has %d tabs", n)
	}

	click := row + `.querySelector("button.icon").click()`
	evalBool(t, ctx, `(window.asked = "", window.confirm = (q) => { window.asked = q; return false; }, `+click+`, true)`)
	waitJS(t, ctx, "the question", `window.asked.indexOf("Resign") === 0`, 5*time.Second)
	if status, _ := gameStatus(t, baseURL, room); status != "active" {
		t.Errorf("answering no left the game %q", status)
	}
	if !evalBool(t, ctx, row+` !== null`) {
		t.Error("answering no took the game off the list")
	}

	evalBool(t, ctx, `(window.confirm = () => true, `+click+`, true)`)
	waitJS(t, ctx, "the row gone", row+` === null`, 10*time.Second)
	if status, reason := gameStatus(t, baseURL, room); status != "finished" || reason != "resignation" {
		t.Errorf("the game is %q, %q", status, reason)
	}
}

// A game that has not started has nothing to resign: it only leaves the list.
func TestWebActiveGames_TheXLeavesAWaitingGameAsItIs(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	room := gameOfThisBrowser(t, ctx, baseURL, `{game_def_id: "nyet", player_name: "Ana", lobby: true}`)
	openActiveGames(t, ctx, baseURL, "Ana", map[string]string{room: "nyet"})
	row := `document.querySelector('#my-games li[data-code="` + room + `"]')`
	waitJS(t, ctx, "the row", row+` !== null`, 10*time.Second)
	evalBool(t, ctx, `(window.confirm = () => true, `+row+`.querySelector("button.icon").click(), true)`)
	waitJS(t, ctx, "the row gone", row+` === null`, 10*time.Second)
	if status, _ := gameStatus(t, baseURL, room); status != "waiting" {
		t.Errorf("the game is %q", status)
	}
	// The server still knows the seat, and does not bring the game back to the list.
	clickSel(t, ctx, "#active-games-dialog .dialog-close")
	clickSel(t, ctx, "#open-active-games")
	waitJS(t, ctx, "the list read again", `document.getElementById("my-games-empty").hidden === false`, 10*time.Second)
	time.Sleep(500 * time.Millisecond)
	if evalBool(t, ctx, row+` !== null`) {
		t.Error("the game came back to the list")
	}
}

// The setup of a game lists the games of that game, each to watch.
func TestWebSetup_ActiveGamesListsThatGamesGames(t *testing.T) {
	baseURL := newRESTTestServer(t)
	nyet := func(name string) string {
		var made struct {
			RoomCode string `json:"room_code"`
		}
		if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "nyet", "player_name": name, "lobby": true}, &made); code != 200 {
			t.Fatalf("create: %d", code)
		}
		return made.RoomCode
	}
	room := nyet("Bo")
	other := botRoom(t, baseURL, "Cy")
	if got := listOpenGames(t, baseURL, "?family=nyet"); len(got) != 1 || got[0].RoomCode != room {
		t.Errorf("?family=nyet lists %+v", got)
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=nyet")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the setup", `document.getElementById("setup-tab-active") !== null && !document.getElementById("game-setup").hidden`, 20*time.Second)
	if evalBool(t, ctx, `document.getElementById("setup-layout").hidden`) {
		t.Error("the setup does not open on the form")
	}
	clickSel(t, ctx, "#setup-tab-active")
	row := `document.querySelector('#setup-active-list li[data-code="` + room + `"]')`
	waitJS(t, ctx, "the table", row+` !== null`, 10*time.Second)
	if !evalBool(t, ctx, `document.getElementById("setup-layout").hidden`) {
		t.Error("the form is still showing")
	}
	if got := evalString(t, ctx, row+`.querySelector("a.button").textContent`); got != "Watch" {
		t.Errorf("the row's button says %q", got)
	}
	if evalBool(t, ctx, `document.querySelector('#setup-active-list li[data-code="`+other+`"]') !== null`) {
		t.Error("another game's table is listed")
	}
}

// Join Game lists the tables with a seat free, and its button takes one.
func TestWebJoin_ListsTheTablesWaitingForPlayers(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var lobby struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "nyet", "player_name": "Bo", "lobby": true}, &lobby); code != 200 {
		t.Fatalf("create: %d", code)
	}
	playing := botRoom(t, baseURL, "Cy")
	if got := listOpenGames(t, baseURL, "?open_seats=1"); len(got) != 1 || got[0].RoomCode != lobby.RoomCode {
		t.Errorf("?open_seats=1 lists %+v", got)
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the top bar", `document.getElementById("open-join") !== null`, 20*time.Second)
	if !evalBool(t, ctx, `document.querySelector('#open-join svg[data-icon="plus"]') !== null`) {
		t.Error("the Join Game button is not a plus")
	}
	clickSel(t, ctx, "#open-join")
	button := `document.querySelector('#join-open-games button[data-join="` + lobby.RoomCode + `"]')`
	waitJS(t, ctx, "the table", button+` !== null`, 10*time.Second)
	if evalBool(t, ctx, `document.querySelector('#join-open-games li[data-code="`+playing+`"]') !== null`) {
		t.Error("a game in play is offered")
	}
	evalBool(t, ctx, `(localStorage.setItem("bingle.name", JSON.stringify("Di")), `+button+`.click(), true)`)
	waitJS(t, ctx, "the table's page", `location.pathname === "/game/`+lobby.RoomCode+`"`, 15*time.Second)
}

// A game whose every move places its one piece says so in those words, to everyone.
func TestWebImpulse_APlacingGameJustSaysPlace(t *testing.T) {
	baseURL := newRESTTestServer(t)
	for def, want := range map[string]string{"havannah-8": "Place a stone.", "go-9x9": "Place a stone.", "reversi": "Place a disc.", "yavalath": "Place a piece.", "mycelium": "Place a node."} {
		ctx, cancel := newChromedpContext(t, 60*time.Second)
		room := newTakRoom(t, baseURL, def, "Alice")
		joinRoom(t, baseURL, room, "Bob")
		openGameAs(t, ctx, baseURL, room, def, "Alice")
		waitJS(t, ctx, def+": the card", `document.getElementById("impulse-text") !== null`, 15*time.Second)
		if got := evalString(t, ctx, `document.getElementById("impulse-text").textContent`); got != want {
			t.Errorf("%s says %q to the player, want %q", def, got, want)
		}
		openGameAs(t, ctx, baseURL, room, def, "Bob")
		waitJS(t, ctx, def+": the card", `document.getElementById("impulse-text") !== null`, 15*time.Second)
		if got := evalString(t, ctx, `document.getElementById("impulse-text").textContent`); got != want {
			t.Errorf("%s says %q to the other player, want %q", def, got, want)
		}
		cancel()
	}
}

// A game that does not narrate has only its notation, and no toggle.
func TestWebHistory_AGameWithNoStoryHasNoToggle(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "gomoku", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "gomoku", "Alice")
	waitJS(t, ctx, "the card", `document.getElementById("history-card") !== null && document.getElementById("hist-prev") !== null`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll("#history-card .history-format").length`); n != 0 {
		t.Errorf("%d toggles", n)
	}
	if got := evalString(t, ctx, `document.querySelector("#history-card h3").textContent`); got != "History" {
		t.Errorf("the card is called %q", got)
	}
}

// Submit says Submit; a placing game offers Back and not Reset.
func TestWebImpulse_SubmitAndBackInAPlacingGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "havannah-8", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "havannah-8", "Alice")
	waitJS(t, ctx, "the board", `document.querySelector("#board .cell.clickable") !== null`, 15*time.Second)
	evalBool(t, ctx, `(document.querySelector("#board .cell.clickable").dispatchEvent(new MouseEvent("click", {bubbles: true})), true)`)
	waitJS(t, ctx, "Submit", `document.getElementById("submit-move") !== null`, 5*time.Second)
	if got := evalString(t, ctx, `document.getElementById("submit-move").textContent`); got != "Submit" {
		t.Errorf("the button says %q", got)
	}
	if evalBool(t, ctx, `document.getElementById("reset-move") !== null`) {
		t.Error("a placing game offers Reset")
	}
	if !evalBool(t, ctx, `document.getElementById("back") !== null`) {
		t.Error("a placing game offers no Back")
	}
}

// Join Game takes a code or a link, and a full table is watched.
func TestWebJoin_TakesACodeOrALinkAndWatchesAFullTable(t *testing.T) {
	baseURL := newRESTTestServer(t)
	open := newTakRoom(t, baseURL, "gomoku", "Bo")
	full := botRoom(t, baseURL, "Cy")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	joinWith := func(typed string) {
		t.Helper()
		if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/")); err != nil {
			t.Fatal(err)
		}
		waitJS(t, ctx, "the top bar", `document.getElementById("open-join") !== null`, 20*time.Second)
		clickSel(t, ctx, "#open-join")
		waitJS(t, ctx, "the window", `document.getElementById("room_code") !== null`, 5*time.Second)
		if got := evalString(t, ctx, `document.getElementById("room_code").placeholder`); got != "Room Code or Link" {
			t.Errorf("the field says %q", got)
		}
		if got := evalString(t, ctx, `document.querySelector("#join-dialog").innerText`); !strings.Contains(got, "Join Game") || !strings.Contains(got, "Public Lobbies") || strings.Contains(got, "Watch") {
			t.Errorf("the window says %q", got)
		}
		evalBool(t, ctx, `(document.getElementById("room_code").value = `+strconv.Quote(typed)+`, document.getElementById("join-submit").click(), true)`)
	}
	seatsOf := func(room string) int {
		var g struct {
			Seats []struct {
				Name string `json:"name"`
			} `json:"seats"`
		}
		if code := doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &g); code != 200 {
			t.Fatalf("read: %d", code)
		}
		n := 0
		for _, s := range g.Seats {
			if s.Name != "" {
				n++
			}
		}
		return n
	}
	// A link to a table with a seat free takes the seat, under a guest's name.
	joinWith(baseURL + "/game/" + open + "#move=1")
	waitJS(t, ctx, "the table", `location.pathname === "/game/`+open+`"`, 15*time.Second)
	if n := seatsOf(open); n != 2 {
		t.Errorf("%d seats taken, want the joiner in the second", n)
	}
	// A full table, by its code: watched, and nobody's seat changes.
	joinWith(strings.ToLower(full))
	waitJS(t, ctx, "the full table", `location.pathname === "/game/`+full+`"`, 15*time.Second)
	if n := seatsOf(full); n != 2 {
		t.Errorf("%d seats at the full table", n)
	}
}

// The active games list counts another player's chat that has not been on screen.
func TestWebActiveGames_OpenCountsUnreadChat(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	room := gameOfThisBrowser(t, ctx, baseURL, `{game_def_id: "gomoku", player_name: "Ana"}`)
	joinRoom(t, baseURL, room, "Bob")
	var said struct{}
	for _, text := range []string{"hello", "are you there"} {
		if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/chat", map[string]any{"player_name": "Bob", "text": text}, &said); code/100 != 2 {
			t.Fatalf("chat: %d", code)
		}
	}
	openActiveGames(t, ctx, baseURL, "Ana", map[string]string{room: "gomoku"})
	count := `document.querySelector('#my-games li[data-code="` + room + `"] a.button .unread-count')`
	waitJS(t, ctx, "the count", count+` !== null && `+count+`.textContent === "2"`, 10*time.Second)
	// Having the chat on screen reads it.
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/game/"+room)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the chat read", `(JSON.parse(localStorage.getItem("bingle.chatRead")) || {})["`+room+`"] > 0`, 15*time.Second)
	clickSel(t, ctx, "#open-active-games")
	waitJS(t, ctx, "the row", `document.querySelector('#my-games li[data-code="`+room+`"]') !== null`, 10*time.Second)
	time.Sleep(700 * time.Millisecond)
	if evalBool(t, ctx, count+` !== null`) {
		t.Error("read messages are still counted")
	}
}

// A guest with no name of their own has a two-word one, in the profile window.
func TestWebProfile_AGuestIsGivenAName(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the top bar", `document.getElementById("open-profile") !== null`, 20*time.Second)
	clickSel(t, ctx, "#open-profile")
	name := evalString(t, ctx, `document.getElementById("profile-window-name").value`)
	if len(strings.Fields(name)) != 2 {
		t.Errorf("the name given is %q", name)
	}
	// It stays the same across pages.
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the settings", `document.getElementById("profile_name") !== null && document.getElementById("profile_name").value !== ""`, 10*time.Second)
	if got := evalString(t, ctx, `document.getElementById("profile_name").value`); got != name {
		t.Errorf("the settings say %q, not %q", got, name)
	}
}

// All Settings says nothing of email to a guest.
func TestWebProfile_AGuestSeesNoEmailSettings(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the section hidden", `document.getElementById("profile-email-section") !== null && document.getElementById("profile-email-section").hidden`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector("main").innerText`); strings.Contains(got, "Email") {
		t.Errorf("the page mentions email: %q", got)
	}
}
