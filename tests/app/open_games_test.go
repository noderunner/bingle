package app_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

type openGameResp struct {
	RoomCode  string `json:"room_code"`
	GameDefID string `json:"game_def_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Lobby     bool   `json:"lobby"`
	Seats     int    `json:"seats"`
	OpenSeats int    `json:"open_seats"`
	MoveSeq   int    `json:"move_seq"`
	Players   []struct {
		Seat int    `json:"seat"`
		Name string `json:"name"`
		Bot  string `json:"bot"`
	} `json:"players"`
	UpdatedAt string `json:"updated_at"`
}

func listOpenGames(t *testing.T, baseURL, query string) []openGameResp {
	t.Helper()
	var out struct {
		Games []openGameResp `json:"games"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games"+query, nil, &out); code != 200 {
		t.Fatalf("list games%s: %d", query, code)
	}
	return out.Games
}

// Every game on the server that has not finished is listed, with who is at the
// table and how many seats nobody holds — a game waiting for an opponent, one
// sitting in a lobby and one being played. A finished game drops out.
func TestOpenGames_ListsEveryUnfinishedGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	if got := listOpenGames(t, baseURL, ""); len(got) != 0 {
		t.Fatalf("a fresh server lists %d games", len(got))
	}
	create := func(body map[string]any) string {
		var created struct {
			RoomCode string `json:"room_code"`
		}
		if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
			t.Fatalf("create %v: %d", body, code)
		}
		return created.RoomCode
	}
	// Waiting for an opponent.
	waiting := create(map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"})
	// A table of four waiting in a lobby, two seats taken.
	lobby := create(map[string]any{"game_def_id": "nyet", "player_name": "Bo", "lobby": true, "also_seats": []int{2}})
	// A game under way against a bot.
	playing := create(map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Cy", "opponent": "random"})
	// And one that is over.
	done := create(map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Di", "opponent": "random"})
	if code := doJSON(t, "POST", baseURL+"/api/games/"+done+"/resign", map[string]any{"player_name": "Di"}, nil); code != 200 {
		t.Fatalf("resign: %d", code)
	}

	byCode := map[string]openGameResp{}
	for _, g := range listOpenGames(t, baseURL, "") {
		byCode[g.RoomCode] = g
	}
	if len(byCode) != 3 {
		t.Fatalf("%d games listed, want the three that are still going: %v", len(byCode), byCode)
	}
	if _, listed := byCode[done]; listed {
		t.Error("a finished game is in the list")
	}
	if g := byCode[waiting]; g.Status != "waiting" || g.Seats != 2 || g.OpenSeats != 1 || len(g.Players) != 1 || g.Players[0].Name != "Ana" {
		t.Errorf("the game waiting for an opponent = %+v", g)
	}
	if g := byCode[lobby]; !g.Lobby || g.Seats != 4 || g.OpenSeats != 2 || g.Name == "" {
		t.Errorf("the game in the lobby = %+v", g)
	}
	if g := byCode[playing]; g.Status != "active" || g.OpenSeats != 0 || len(g.Players) != 2 {
		t.Errorf("the game being played = %+v", g)
	} else if g.Players[1].Bot == "" {
		t.Errorf("the bot at the table is not marked: %+v", g.Players)
	}
	// The list can be cut short.
	if got := listOpenGames(t, baseURL, "?limit=1"); len(got) != 1 {
		t.Errorf("limit=1 gave %d games", len(got))
	}
}

// Who is here (ADR 0073): a signed-in account says so and is listed under its
// handle and an address that is a hash of nothing it can be found by; saying
// it is leaving takes it off. Guests are not listed and see no list.
func TestWhoIsHere_AccountsSayTheyAreAbout(t *testing.T) {
	a, baseURL := membersServer(t)
	type hereResp struct {
		Address string `json:"address"`
		People  []struct {
			Address string `json:"address"`
			Name    string `json:"name"`
			Since   string `json:"since"`
			Seen    string `json:"seen"`
		} `json:"people"`
	}
	anaB, boB := member(t, a, baseURL, "ana"), member(t, a, baseURL, "bob")
	say := func(b *browser, here bool) (hereResp, int) {
		var out hereResp
		code := b.do("POST", "/api/here", map[string]any{"here": here}, &out)
		return out, code
	}
	var empty hereResp
	if code := anaB.do("GET", "/api/here", nil, &empty); code != 200 || len(empty.People) != 0 {
		t.Fatalf("a fresh server has %d people here (%d)", len(empty.People), code)
	}
	ana, code := say(anaB, true)
	if code != 200 || len(ana.Address) != 16 {
		t.Fatalf("Ana says she is here: %d %+v", code, ana)
	}
	bo, _ := say(boB, true)
	if len(bo.People) != 2 {
		t.Fatalf("%d people are here, want two: %+v", len(bo.People), bo.People)
	}
	names := []string{}
	for _, p := range bo.People {
		names = append(names, p.Name)
		if p.Address == "" || p.Since == "" || p.Seen == "" {
			t.Errorf("%+v is missing its address or its times", p)
		}
	}
	if got := strings.Join(names, ","); got != "@ana,@bob" {
		t.Errorf("the list reads %q, want the longest-standing first, by handle", got)
	}
	// A rename keeps the address.
	anaB.do("POST", "/auth/handle", map[string]any{"handle": "anabel"}, nil)
	if again, _ := say(anaB, true); again.Address != ana.Address {
		t.Errorf("a rename changed the address: %q, then %q", ana.Address, again.Address)
	}
	// Leaving takes her off.
	gone, _ := say(anaB, false)
	if len(gone.People) != 1 || gone.People[0].Name != "@bob" {
		t.Errorf("after Ana left: %+v", gone.People)
	}
	// A guest is not listed, and sees nothing.
	guest := newBrowser(t, baseURL)
	if code := guest.do("POST", "/api/here", map[string]any{}, nil); code != 403 {
		t.Errorf("a guest says it is here: %d, want 403", code)
	}
	if code := doJSON(t, "GET", baseURL+"/api/here", nil, nil); code != 403 {
		t.Errorf("an anonymous read of who is here: %d, want 403", code)
	}
}

// The Active games window has two tabs: the games of this browser, and every
// game on the server — where a game waiting in a lobby says so and how many
// seats are still open, and a game of somebody else's can be opened to watch.
func TestWebOpenGames_TheActiveGamesWindowHasTwoTabs(t *testing.T) {
	baseURL := newRESTTestServer(t)
	// Ana's own game against a bot, and a table of four of Bo's waiting in a lobby.
	mine := botRoom(t, baseURL, "Ana")
	var theirs struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "nyet", "player_name": "Bo", "lobby": true}, &theirs); code != 200 {
		t.Fatalf("Bo's table: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	rememberGames(t, ctx, baseURL, "Ana", map[string]string{mine: "tic-tac-toe"})
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the top bar", `document.getElementById("open-active-games") !== null`, 20*time.Second)
	clickSel(t, ctx, "#open-active-games")
	waitJS(t, ctx, "my games", `document.querySelector('#my-games li[data-code="`+mine+`"]') !== null`, 10*time.Second)
	// My games holds only this browser's game; the tabs are named.
	if n := evalInt(t, ctx, `document.querySelectorAll("#my-games li").length`); n != 1 {
		t.Errorf("%d games in My games, want 1", n)
	}
	if got := evalString(t, ctx, `[...document.querySelectorAll("#active-games-dialog .tab")].map(b => b.textContent.trim()).join(",")`); got != "My games,All games" {
		t.Errorf("the tabs are %q", got)
	}
	// All games holds both, with Bo's table marked as waiting in its lobby and
	// its three open seats counted.
	clickSel(t, ctx, "#tab-all-games")
	waitJS(t, ctx, "all games", `document.querySelectorAll("#all-games li").length === 2`, 10*time.Second)
	row := `document.querySelector('#all-games li[data-code="` + theirs.RoomCode + `"]')`
	if !evalBool(t, ctx, row+` !== null`) {
		t.Fatal("Bo's table is not in All games")
	}
	if got := evalString(t, ctx, row+`.querySelector(".badge.waiting").textContent`); got != "In the lobby" {
		t.Errorf("Bo's table is marked %q", got)
	}
	if got := evalString(t, ctx, row+`.querySelector("[data-open-seats]").textContent`); got != "3 seats open" {
		t.Errorf("the open seats read %q, want three", got)
	}
	if got := evalString(t, ctx, row+`.querySelector("a.button").getAttribute("href")`); got != "/game/"+theirs.RoomCode {
		t.Errorf("the way into Bo's table is %q", got)
	}
	// This browser's own game is in the list too, marked as its own.
	if !evalBool(t, ctx, `document.querySelector('#all-games li[data-code="`+mine+`"]').textContent.includes("yours")`) {
		t.Error("the browser's own game is not marked in All games")
	}
	// The tab that was open is the one that opens next time.
	clickSel(t, ctx, "#active-games-dialog .dialog-close")
	clickSel(t, ctx, "#open-active-games")
	waitJS(t, ctx, "All games again", `document.getElementById("panel-all-games").hidden === false`, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// The Global Chat tab shows how many messages the person has not read — those
// said while another tab was showing, and those said while this browser was
// away, which it knows from the last message it showed.
func TestWebGlobalChat_TheTabSaysHowManyAreUnread(t *testing.T) {
	a, baseURL := membersServer(t)
	bo := member(t, a, baseURL, "bob") // the global chat is written by accounts (ADR 0073)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the games tab", `document.getElementById("tab-chat") !== null`, 20*time.Second)
	// Somebody says something while the Games tab is showing.
	if code := bo.do("POST", "/api/chat", map[string]any{"player_name": "Bo", "text": "Anybody for Hex?"}, nil); code != 200 {
		t.Fatalf("Bo says something: %d", code)
	}
	waitJS(t, ctx, "the badge", `document.getElementById("chat-tab-badge").hidden === false && document.getElementById("chat-tab-badge").textContent === "1"`, 10*time.Second)
	// Reading the chat clears it.
	clickSel(t, ctx, "#tab-chat")
	waitJS(t, ctx, "the badge gone", `document.getElementById("chat-tab-badge").hidden === true`, 10*time.Second)
	// Two more are said while this browser is away from the page (so it does
	// not see them arrive), and are counted when it comes back: the page
	// remembers the last message it showed.
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the profile page", `document.getElementById("profile_name") !== null`, 20*time.Second)
	for _, line := range []string{"I am about", "Still here"} {
		if code := bo.do("POST", "/api/chat", map[string]any{"player_name": "Bo", "text": line}, nil); code != 200 {
			t.Fatalf("Bo says %q: %d", line, code)
		}
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the badge after coming back", `document.getElementById("chat-tab-badge") && document.getElementById("chat-tab-badge").hidden === false && document.getElementById("chat-tab-badge").textContent === "2"`, 15*time.Second)
}
