package app_test

import (
	"context"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/app"
)

func TestRESTRealTime_TheSettingAndTheOffer(t *testing.T) {
	base := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "gomoku", "player_name": "Alice", "real_time": map[string]any{"first_warning_seconds": 60}}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	joinRoom(t, base, room, "Bob")

	var game struct {
		RealTime *struct {
			First  int `json:"first_warning_seconds"`
			Second int `json:"second_warning_seconds"`
			Wait   *struct {
				Seat  int `json:"seat"`
				Stage int `json:"stage"`
			} `json:"wait"`
		} `json:"real_time"`
	}
	if code := doJSON(t, "GET", base+"/api/games/"+room, nil, &game); code != 200 {
		t.Fatalf("game: %d", code)
	}
	if game.RealTime == nil || game.RealTime.First != 60 || game.RealTime.Second != 300 || game.RealTime.Wait == nil || game.RealTime.Wait.Seat != 1 || game.RealTime.Wait.Stage != 0 {
		t.Fatalf("real_time = %+v", game.RealTime)
	}
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/auto-play-impulse", map[string]any{"player_name": "Bob"}, nil); code != 409 {
		t.Errorf("no turn on offer: %d, want 409", code)
	}
	bad := map[string]any{"game_def_id": "gomoku", "player_name": "Alice", "real_time": map[string]any{"first_warning_seconds": 5}}
	if code := doJSON(t, "POST", base+"/api/games", bad, nil); code != 400 {
		t.Errorf("a warning out of range: %d, want 400", code)
	}
}

func TestMCPRealTime_CreateAndRead(t *testing.T) {
	c := newMCPClient(t, newRESTTestServer(t))
	var created struct {
		RoomCode string `json:"room_code"`
	}
	decodeToolResult(t, callTool(t, c, "create_game", map[string]any{"game_def_id": "gomoku", "player_name": "Agent", "real_time": true, "first_warning_seconds": 90}), &created)
	var got struct {
		RealTime *struct {
			First int `json:"first_warning_seconds"`
		} `json:"real_time"`
	}
	decodeToolResult(t, callTool(t, c, "get_state", map[string]any{"room_code": created.RoomCode}), &got)
	if got.RealTime == nil || got.RealTime.First != 90 {
		t.Errorf("real_time = %+v, want a first warning of 90 seconds", got.RealTime)
	}
}

// The setup offers it, shows the warnings only when they would apply, and sends them.
func TestWebRealTime_TheSetup(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/?game=gomoku"), chromedp.WaitVisible(`#create-submit`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	warnings := `document.getElementById("real-time-warnings").hidden`
	// The field is in the page from the start: wait for the setup to have drawn the game's, or a click
	// lands before its listener and the setup then draws the box as it found it.
	waitJS(t, ctx, "the field", `document.getElementById("real-time-field").dataset.wired === "1" && document.getElementById("real-time-field").hidden === false`, 10*time.Second)
	if !evalBool(t, ctx, warnings) {
		t.Error("the warnings show before real-time is chosen")
	}
	evalString(t, ctx, `document.getElementById("real-time").click(); "ok"`)
	if evalBool(t, ctx, warnings) {
		t.Error("the warnings are hidden in a real-time game with no timer")
	}
	evalString(t, ctx, `(() => { const c = document.getElementById("clock_seconds"); c.value = "minutes"; c.dispatchEvent(new Event("change")); return "ok"; })()`)
	if !evalBool(t, ctx, warnings) {
		t.Error("the warnings show in a game with a clock")
	}
	evalString(t, ctx, `(() => { const c = document.getElementById("clock_seconds"); c.value = "0"; c.dispatchEvent(new Event("change")); document.getElementById("real-time-first").value = "3"; return "ok"; })()`)

	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`localStorage.setItem("bingle.name", JSON.stringify("Alice"))`, nil),
		submitClosestForm(ctx, `select[name="game_def_id"]`),
	); err != nil {
		t.Fatal(err)
	}
	url := pollLocation(t, ctx, gamePageURLPattern)
	room := gamePageURLPattern.FindStringSubmatch(url)[1]
	waitJS(t, ctx, "the badge", `document.getElementById("real-time-badge") !== null`, 10*time.Second)
	var game struct {
		RealTime *struct {
			First int `json:"first_warning_seconds"`
		} `json:"real_time"`
	}
	doJSON(t, "GET", base+"/api/games/"+room, nil, &game)
	if game.RealTime == nil || game.RealTime.First != 180 {
		t.Errorf("real_time = %+v, want a first warning of 3 minutes", game.RealTime)
	}

	// The page's reconnection stops sooner in a real-time game.
	var delays []int
	if err := chromedp.Run(ctx, chromedp.Evaluate(`import("/static/js/util.js").then((u) => [u.reconnectDelay(6, false), u.reconnectDelay(6, true), u.reconnectDelay(1, true)])`, &delays, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil {
		t.Fatal(err)
	}
	if len(delays) != 3 || delays[0] != 44800 || delays[1] != 3000 || delays[2] != 1400 {
		t.Errorf("reconnect delays = %v", delays)
	}
}

// The warnings reach the right people in place, and the table can ask for the bot.
func TestWebRealTime_TheWarningsAndTheOffer(t *testing.T) {
	a, err := app.New(newTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	registerFixtures(t, a)
	var offset atomic.Int64
	a.Manager().SetClock(func() time.Time { return time.Now().Add(time.Duration(offset.Load())) })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	base := srv.URL

	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "gomoku", "player_name": "Alice", "real_time": map[string]any{}}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	joinRoom(t, base, room, "Bob")

	aliceCtx, cancelA := newChromedpContext(t, 90*time.Second)
	defer cancelA()
	openGameAs(t, aliceCtx, base, room, "gomoku", "Alice")
	bobCtx, cancelB := newChromedpContext(t, 90*time.Second)
	defer cancelB()
	openGameAs(t, bobCtx, base, room, "gomoku", "Bob")
	waitJS(t, bobCtx, "Alice connected", `document.querySelector('#seat-1 .seat-conn').textContent === ""`, 10*time.Second)
	evalString(t, bobCtx, `document.querySelector('#seat-1').dataset.same = "yes"; "ok"`)

	banner := `document.getElementById("slow-turn")`
	offset.Store(int64(3 * time.Minute))
	a.Manager().RealTimeDue(context.Background())
	waitJS(t, aliceCtx, "Alice is told first", banner+`.textContent.includes("waiting for your move")`, 10*time.Second)
	if !evalBool(t, bobCtx, banner+`.hidden`) {
		t.Error("Bob was told of the first warning")
	}

	offset.Store(int64(8 * time.Minute))
	a.Manager().RealTimeDue(context.Background())
	waitJS(t, bobCtx, "Bob is offered the bot", `document.getElementById("slow-turn-bot") !== null`, 10*time.Second)
	if evalBool(t, aliceCtx, `document.getElementById("slow-turn-bot") !== null`) {
		t.Error("the slow player is offered a bot for their own turn")
	}
	if got := evalString(t, bobCtx, `document.querySelector('#seat-1').dataset.same || ""`); got != "yes" {
		t.Error("the page was re-rendered for a warning")
	}

	evalString(t, bobCtx, `document.getElementById("slow-turn-bot").click(); "ok"`)
	for _, ctx := range []context.Context{aliceCtx, bobCtx} {
		waitJS(t, ctx, "the banner goes with the move", banner+`.hidden`, 15*time.Second)
	}
	var moves []struct {
		Seat     int  `json:"seat"`
		AutoPlay bool `json:"auto_play"`
	}
	doJSON(t, "GET", base+"/api/games/"+room+"/moves", nil, &moves)
	if len(moves) != 1 || moves[0].Seat != 1 || !moves[0].AutoPlay {
		t.Errorf("moves = %+v, want one of Alice's, marked auto-play", moves)
	}
}
