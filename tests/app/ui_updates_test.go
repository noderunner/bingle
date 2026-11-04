// Tests of the list of active games and the ways to find a game: the ✕ that
// resigns, a game's own Active Games tab, the tables in the Join Game window.

package app_test

import (
	"context"
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
	evalBool(t, ctx, `(document.getElementById("join_player_name").value = "Di", `+button+`.click(), true)`)
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

// The arrow under "Since your last move" goes back a move at each click.
func TestWebCthulhuWars_TheHistoryStepsBackFromTheMark(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 2, 1, 1, 1}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the buttons", `document.getElementById("action-recruit") !== null`, 15*time.Second)
	clickSel(t, ctx, "#action-recruit")
	waitJS(t, ctx, "an area", `document.querySelector('#board .cell.area.clickable[data-position="south-pacific"]') !== null`, 10*time.Second)
	clickCell(t, ctx, "south-pacific")
	// The bots play round to Alice again, and the mark is there.
	waitJS(t, ctx, "the arrow", `document.getElementById("history-rewind") !== null`, 60*time.Second)
	if got := evalString(t, ctx, `document.getElementById("history-rewind").innerText.trim()`); got != "" {
		t.Errorf("the arrow has words: %q", got)
	}
	seq := func() int { return evalInt(t, ctx, `Number(location.hash.replace("#move=", ""))`) }
	evalBool(t, ctx, `(document.getElementById("history-rewind").click(), true)`)
	waitJS(t, ctx, "one move back", `location.hash.indexOf("#move=") === 0`, 10*time.Second)
	first := seq()
	if first <= 0 {
		t.Fatalf("the first click went to move %d", first)
	}
	if !evalBool(t, ctx, `document.getElementById("history-rewind") !== null`) {
		t.Fatal("the arrow is gone after one click")
	}
	evalBool(t, ctx, `(document.getElementById("history-rewind").click(), true)`)
	waitJS(t, ctx, "two moves back", `location.hash === "#move=`+itoa(first-1)+`"`, 10*time.Second)
}
