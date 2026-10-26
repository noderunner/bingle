package app_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

const importedPTN = `[Site "somewhere"]
[Player1 "Ada"]
[Player2 "Grace"]
[Size "5"]
[Result "0-1"]

1. a5 e1 {opening swap} 2. c3 c4
3. c2 b4 4. d5 c5 0-1`

func TestImportPTNCreatesAReviewableRoom(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var res struct {
		RoomCode  string    `json:"room_code"`
		GameDefID string    `json:"game_def_id"`
		Status    string    `json:"status"`
		Moves     int       `json:"moves"`
		Players   [2]string `json:"players"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/import", map[string]any{"text": importedPTN}, &res); code != 200 {
		t.Fatalf("import: %d", code)
	}
	if res.GameDefID != "tak-5x5" || res.Moves != 8 || res.Players != [2]string{"Ada", "Grace"} || res.Status != "active" {
		t.Fatalf("import result = %+v", res)
	}

	// The room has the whole history and the right names.
	var hist []map[string]any
	doJSON(t, "GET", baseURL+"/api/games/"+res.RoomCode+"/moves", nil, &hist)
	if len(hist) != 8 {
		t.Fatalf("history = %d moves, want 8", len(hist))
	}
	var rec struct {
		Notation []string `json:"notation"`
		Seats    []struct {
			Name string `json:"name"`
		} `json:"seats"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+res.RoomCode+"/export", nil, &rec)
	if len(rec.Notation) != 8 || rec.Notation[0] != "a5" || rec.Seats[0].Name != "Ada" || rec.Seats[1].Name != "Grace" {
		t.Errorf("export = %+v", rec)
	}
}

func TestImportMoveListWithGameAndRejections(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var res struct {
		RoomCode string `json:"room_code"`
		Status   string `json:"status"`
	}
	// A plain move list for Tic-Tac-Toe: X wins on the diagonal.
	if code := doJSON(t, "POST", baseURL+"/api/games/import", map[string]any{
		"game_def_id": "tic-tac-toe", "text": "a3 b3 b2 c3 c1", "player1": "Xena", "player2": "Otto",
	}, &res); code != 200 || res.Status != "finished" {
		t.Fatalf("import move list: %d %+v", code, res)
	}

	cases := []struct {
		name string
		body map[string]any
		want string
	}{
		{"empty", map[string]any{"text": "  "}, "no moves"},
		{"no game", map[string]any{"text": "a1 b2"}, "which game"},
		{"unknown game", map[string]any{"game_def_id": "nope", "text": "a1"}, "not found"},
		{"illegal move", map[string]any{"game_def_id": "tic-tac-toe", "text": "a1 a1"}, "move 2"},
		{"moves after the end", map[string]any{"game_def_id": "tic-tac-toe", "text": "a3 b3 b2 c3 c1 a1"}, "game is over"},
		{"garbage", map[string]any{"game_def_id": "tak-5x5", "text": "hello world"}, "cannot import"},
	}
	for _, tc := range cases {
		var out map[string]any
		code := doJSON(t, "POST", baseURL+"/api/games/import", tc.body, &out)
		msg, _ := out["error"].(string)
		if code < 400 || !strings.Contains(strings.ToLower(msg), tc.want) {
			t.Errorf("%s: %d %q, want an error containing %q", tc.name, code, msg, tc.want)
		}
	}
}

func TestImportFromTPSStartAndBlackFirst(t *testing.T) {
	baseURL := newRESTTestServer(t)
	// Black (player 2) moves first from this TPS; the PTN's "--" placeholder
	// stands in for White's missing move.
	ptn := `[Size "5"]
[TPS "x5/x5/1,1,1,1,x/x5/x5 2 6"]

6. -- a1
7. e3`
	var res struct {
		RoomCode string `json:"room_code"`
		Moves    int    `json:"moves"`
		Status   string `json:"status"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/import", map[string]any{"text": ptn}, &res); code != 200 {
		t.Fatalf("import: %d", code)
	}
	if res.Moves != 2 || res.Status != "finished" {
		t.Errorf("result = %+v (white's e3 completes the road)", res)
	}
}

func TestMCP_ImportGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	c := newMCPClient(t, baseURL)
	res := callTool(t, c, "import_game", map[string]any{"text": importedPTN})
	if res.IsError {
		t.Fatalf("import_game: %s", toolText(res))
	}
	var out struct {
		RoomCode string `json:"room_code"`
		Moves    int    `json:"moves"`
	}
	decodeToolResult(t, res, &out)
	if out.RoomCode == "" || out.Moves != 8 {
		t.Errorf("import_game result = %+v", out)
	}
	if bad := callTool(t, c, "import_game", map[string]any{"text": "zzz", "game_def_id": "tak-5x5"}); !bad.IsError {
		t.Error("importing nonsense should be an error")
	}
}

// Importing is an option of a game's setup: a move log, pasted or uploaded,
// instead of a new game from the start.
func TestWebLobby_SetupCanImportAMoveLog(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/?game=tak-5x5"),
		chromedp.WaitVisible("#create-game-form", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the start-from choice", `!document.getElementById("start-mode-field").hidden`, 10*time.Second)
	if evalBool(t, ctx, `document.getElementById("import-text").offsetParent !== null`) {
		t.Error("the move log box shows before importing was chosen")
	}
	evalString(t, ctx, `document.querySelector('input[name="start_mode"][value="import"]').click(); "ok"`)
	waitJS(t, ctx, "the move log box", `document.getElementById("import-text").offsetParent !== null`, 5*time.Second)
	if evalBool(t, ctx, `document.getElementById("create_player_name").offsetParent !== null`) {
		t.Error("the name field stays while importing")
	}
	if err := chromedp.Run(ctx, chromedp.SetValue("#import-text", importedPTN, chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#create-submit")
	waitJS(t, ctx, "review room", `location.pathname.indexOf("/game/") === 0`, 15*time.Second)
	waitJS(t, ctx, "history", `document.querySelectorAll('#move-list button.mv').length === 8`, 10*time.Second)
}

// A game without a replayable record (several seats, chance) offers no import.
func TestWebLobby_SetupOffersNoImportForMultiSeatGames(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/?game=cthulhu-wars"),
		chromedp.WaitVisible("#create-game-form", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the seats", `document.getElementById("slots-field")`, 15*time.Second)
	if !evalBool(t, ctx, `document.getElementById("start-mode-field").hidden`) {
		t.Error("Cthulhu Wars offers to import a move log")
	}
}

func TestBotMatchPlaysItselfOutForSpectators(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var res struct {
		RoomCode string `json:"room_code"`
		Status   string `json:"status"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/bot-matches", map[string]any{
		"game_def_id": "tic-tac-toe", "bot_a": "tactical", "bot_b": "random", "delay_ms": 150,
	}, &res); code != 200 || res.RoomCode == "" || res.Status != "active" {
		t.Fatalf("start: %d %+v", code, res)
	}
	// Both seats are bots; nobody can play into it.
	var game struct {
		Seats []struct {
			Name string `json:"name"`
			Bot  string `json:"bot"`
		} `json:"seats"`
		Status string `json:"status"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+res.RoomCode, nil, &game)
	if len(game.Seats) != 2 || game.Seats[0].Bot != "tactical" || game.Seats[1].Bot != "random" {
		t.Fatalf("seats = %+v", game.Seats)
	}
	var out map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/games/"+res.RoomCode+"/moves", map[string]any{"player_name": "Mallory", "notation": "b2"}, &out); code < 400 {
		t.Errorf("a stranger moved in a bot match: %d", code)
	}

	// It finishes by itself (at most nine moves, 150 ms apart).
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		doJSON(t, "GET", baseURL+"/api/games/"+res.RoomCode, nil, &game)
		if game.Status == "finished" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if game.Status != "finished" {
		t.Fatalf("the bot match never finished (status %q)", game.Status)
	}
	var hist []map[string]any
	doJSON(t, "GET", baseURL+"/api/games/"+res.RoomCode+"/moves", nil, &hist)
	if len(hist) < 5 || len(hist) > 9 {
		t.Errorf("a finished tic-tac-toe has %d moves", len(hist))
	}

	// Bad requests.
	for _, body := range []map[string]any{
		{"game_def_id": "nope", "bot_a": "random", "bot_b": "random"},
		{"game_def_id": "tic-tac-toe", "bot_a": "ghost", "bot_b": "random"},
	} {
		if code := doJSON(t, "POST", baseURL+"/api/bot-matches", body, &out); code < 400 {
			t.Errorf("%v accepted: %d", body, code)
		}
	}
}

// Two bots can be set up as the opponents of a game, and then play it out
// while the creator watches.
func TestWebLobby_SetupCanStartTwoBotsToWatch(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/?game=tic-tac-toe"),
		chromedp.WaitVisible("#create-game-form", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "bots listed", `document.querySelectorAll('#botmatch-a option').length >= 3 && !document.getElementById("opponent-bots").hidden`, 10*time.Second)
	evalString(t, ctx, `(function(){
		document.querySelector('input[name="opponent"][value="@bots"]').click();
		document.getElementById("botmatch-a").value = "random";
		document.getElementById("botmatch-b").value = "random";
		document.getElementById("botmatch-speed").value = "250";
		return "ok";
	})()`)
	if evalBool(t, ctx, `document.getElementById("create_player_name").offsetParent !== null`) {
		t.Error("a name is asked although nobody plays")
	}
	clickSel(t, ctx, "#create-submit")
	waitJS(t, ctx, "game page", `location.pathname.indexOf("/game/") === 0`, 15*time.Second)
	// Moves arrive live over the WebSocket while we watch.
	waitJS(t, ctx, "moves appear", `document.querySelectorAll('#move-list button.mv').length >= 3`, 20*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#undo-move, #hint').length`); n != 0 {
		t.Errorf("a spectator should have no player controls, found %d", n)
	}
}

// A game exported as PTN imports back to the same moves and players.
func TestPTNExportImportRoundTrip(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Ada")
	joinRoom(t, baseURL, room, "Grace")
	for _, m := range []struct{ who, mv string }{
		{"Ada", "a5"}, {"Grace", "e1"}, {"Ada", "c3"}, {"Grace", "d3"}, {"Ada", "Sc4"}, {"Grace", "d3<"},
	} {
		if code, out := moveNotation(t, baseURL, room, m.who, m.mv); code != 200 {
			t.Fatalf("%s %s: %d %v", m.who, m.mv, code, out)
		}
	}
	resp, err := http.Get(baseURL + "/api/games/" + room + "/export?format=ptn")
	if err != nil {
		t.Fatal(err)
	}
	ptn, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	var imported struct {
		RoomCode string    `json:"room_code"`
		Moves    int       `json:"moves"`
		Players  [2]string `json:"players"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/import", map[string]any{"text": string(ptn)}, &imported); code != 200 {
		t.Fatalf("importing the exported PTN: %d\n%s", code, ptn)
	}
	if imported.Moves != 6 || imported.Players != [2]string{"Ada", "Grace"} {
		t.Errorf("import = %+v", imported)
	}
	var a, b struct {
		Notation []string `json:"notation"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/export", nil, &a)
	doJSON(t, "GET", baseURL+"/api/games/"+imported.RoomCode+"/export", nil, &b)
	if strings.Join(a.Notation, " ") != strings.Join(b.Notation, " ") {
		t.Errorf("notation differs after the round trip:\n%v\n%v", a.Notation, b.Notation)
	}
}
