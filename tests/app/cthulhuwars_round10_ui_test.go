package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The tenth playtest (game TQQJ96): the first player of the game, the History
// tab, the Black Goat's Done, the Auto-Play Timeout, the lobby's cults and the
// chat's emoji.

// appServer is a server whose app the test can reach (its manager, its DB).
func appServer(t *testing.T) (base string, a *app.App, cfg config.Config) {
	t.Helper()
	cfg = newTestConfig(t)
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return srv.URL, a, cfg
}

// The table of TQQJ96 (Black Goat, Crawling Chaos, Great Cthulhu, Yellow Sign,
// Windwalker): the person who plays Great Cthulhu, in seat 3, chooses the
// direction of play first — not the person in seat 1.
func TestWebCthulhuWars_GreatCthulhuInSeatThreeChoosesTheDirectionFirst(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{
		"game_def_id": "cthulhu-wars-5p", "player_name": "Titan", "seat": 3,
		"seat_bots": map[string]string{"1": "tactical", "2": "tactical", "4": "tactical", "5": "tactical"},
		"options":   map[string]any{"factions": []string{"bg", "cc", "gc", "ys", "ww"}},
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	var g struct {
		GameDefID string `json:"game_def_id"`
		TurnSeat  int    `json:"turn_seat"`
	}
	// The Windwalker's bot sets up by itself; then it is Great Cthulhu's turn to choose.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &g)
		if g.TurnSeat == 3 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if g.TurnSeat != 3 {
		t.Fatalf("seat %d is to move, want Great Cthulhu's seat 3", g.TurnSeat)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, created.RoomCode, g.GameDefID, "Titan")
	waitJS(t, ctx, "the choice of direction", `document.getElementById("action-cw") && document.getElementById("action-ccw")`, 20*time.Second)
	// The phase bar is at "First Player", the round has not begun.
	if got := evalString(t, ctx, `[...document.querySelectorAll("#phase-steps .phase-step")].map(p => p.classList[1] + ":" + p.textContent.replace("✓", "").trim()).join("|")`); got != "todo:Action|done:Gather Power|current:First Player|todo:Doom" {
		t.Errorf("phase bar %q", got)
	}
	if got := evalString(t, ctx, `document.getElementById("status-message").textContent`); !strings.Contains(got, "first player") {
		t.Errorf("status %q", got)
	}
	// Clockwise plays down the list from Great Cthulhu: Yellow Sign, Windwalker, Black Goat, Crawling Chaos.
	if got := evalString(t, ctx, `document.getElementById("action-cw").textContent`); !strings.Contains(got, "Great Cthulhu") || !strings.Contains(got, "Yellow Sign") {
		t.Errorf("clockwise button %q", got)
	}
	clickSel(t, ctx, "#action-cw")
	waitJS(t, ctx, "the first Action", `!document.getElementById("action-cw") && document.getElementById("action-pass")`, 20*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// The History tab is always there, for the whole game: at the start, and
// after the viewer's own Done when the others' answers (passes) say nothing.
func TestWebCthulhuWars_TheHistoryTabIsAlwaysThere(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars", "Alice")
	tabs := `[...document.querySelectorAll("#history-card .card-tab")].map(b => b.id).join(",")`
	// At the very start nothing has been told: both tabs, and the History says so.
	waitJS(t, ctx, "the tabs", tabs+` === "tab-moves,tab-history"`, 15*time.Second)
	if got := evalString(t, ctx, `document.getElementById("history-pane") ? document.getElementById("history-pane").textContent : ""`); !strings.Contains(got, "Nothing to tell yet") {
		t.Errorf("history at the start: %q", got)
	}
	clickSel(t, ctx, "#tab-moves")
	waitJS(t, ctx, "the moves", `document.getElementById("moves-pane") !== null`, 5*time.Second)
	if got := evalString(t, ctx, tabs); got != "tab-moves,tab-history" {
		t.Errorf("tabs after choosing Moves: %q", got)
	}
	clickSel(t, ctx, "#tab-history")
	waitJS(t, ctx, "the history", `document.getElementById("history-pane") !== null`, 5*time.Second)
}

func TestWebCthulhuWars_TheHistoryTabSurvivesQuietStretches(t *testing.T) {
	baseURL := newRESTTestServer(t)
	// Bob (Crawling Chaos) moves a Cultist and says Done; Alice (Great Cthulhu,
	// holding Devolve) has her reaction passed for her, which says nothing.
	// Then it is Bob's turn to watch: nothing new was narrated since his Done.
	room := reactionTable(t, baseURL)
	for _, n := range []string{"go:cultist@south-pacific>antarctica", "done"} {
		if code, out := moveNotation(t, baseURL, room, "Bob", n); code != 200 {
			t.Fatalf("%s: %d %v", n, code, out)
		}
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Bob")
	waitJS(t, ctx, "the tabs", `[...document.querySelectorAll("#history-card .card-tab")].map(b => b.id).join(",") === "tab-moves,tab-history"`, 15*time.Second)
	got := evalString(t, ctx, `document.getElementById("history-pane").textContent`)
	if !strings.Contains(got, "Nothing new from the others since your last move") || !strings.Contains(got, "moves an Acolyte Cultist") {
		t.Errorf("the quiet stretch is told, with Bob's own turn: %q", got)
	}
	// Looking back at an earlier move and returning does not lose the tab.
	clickSel(t, ctx, "#tab-moves")
	waitJS(t, ctx, "the moves", `document.querySelector("#move-list .mv") !== null`, 5*time.Second)
	clickSel(t, ctx, `#move-list .mv[data-seq="1"]`)
	waitJS(t, ctx, "the earlier position", `document.querySelector("#move-list .mv.current") && document.querySelector("#move-list .mv.current").getAttribute("data-seq") === "1"`, 10*time.Second)
	tabs := `[...document.querySelectorAll("#history-card .card-tab")].map(b => b.id).join(",")`
	if got := evalString(t, ctx, tabs); got != "tab-moves,tab-history" {
		t.Errorf("tabs at an earlier move: %q", got)
	}
	clickSel(t, ctx, "#hist-last")
	waitJS(t, ctx, "back to the present", `!document.querySelector("#move-list .mv.current") || document.querySelector("#move-list .mv.current").getAttribute("data-seq") !== "1"`, 10*time.Second)
	if got := evalString(t, ctx, tabs); got != "tab-moves,tab-history" {
		t.Errorf("tabs back at the present: %q", got)
	}
}

// Black Goat's Fertility Cult makes summoning an unlimited action, and one
// Ghoul is a turn if the player wants it so: Done is offered right after it.
func TestWebCthulhuWars_BlackGoatCanPressDoneAfterASingleSummon(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{3, "cultist", "south-america-west", 2}, {1, "cultist", "south-pacific", 1}},
		map[string]int{"south-america-west": 3, "south-pacific": 1},
		[5]int{0, 3, 3, 8, 3}, [5]uint8{}, func(d *cthulhuwars.Data) { d.Turn.Seat = 3 })
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{
		"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat": 3,
		"seat_bots":   map[string]string{"1": "random", "2": "random", "4": "random"},
		"start_state": json.RawMessage(doc), "start_seat": 3,
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the summon buttons", `document.getElementById("action-summon-ghoul") !== null`, 15*time.Second)
	// Before any action there is Pass (it drops the Power), not Done.
	if !evalBool(t, ctx, `document.getElementById("action-pass") && !document.getElementById("action-done")`) {
		t.Error("at the start of the turn: Pass, not Done")
	}
	clickSel(t, ctx, "#action-summon-ghoul")
	waitJS(t, ctx, "the Gate", `document.querySelectorAll('#board .cell.area.clickable').length === 1`, 10*time.Second)
	clickCell(t, ctx, "south-america-west")
	waitJS(t, ctx, "the Ghoul", `document.querySelector('#board .token[data-position="south-america-west"][data-piece="ghoul"]') !== null`, 15*time.Second)
	// A single Ghoul summoned: Done is there (and more summons, and the main actions).
	waitJS(t, ctx, "Done", `document.getElementById("action-done") !== null && !document.getElementById("action-pass")`, 10*time.Second)
	if !evalBool(t, ctx, `document.getElementById("action-summon-ghoul") !== null`) {
		t.Error("more summons stay open")
	}
	clickSel(t, ctx, "#action-done")
	waitJS(t, ctx, "the turn passed", `document.getElementById("action-done") === null`, 15*time.Second)
	var st struct {
		Seats []struct{ Seat int } `json:"seats"`
	}
	_ = st
	// The Power was kept: Done does not drop it.
	var after struct {
		State struct {
			Ext struct {
				Power []int `json:"power"`
			} `json:"ext"`
		} `json:"state"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode+"/state", nil, &after)
	if p := after.State.Ext.Power; len(p) < 4 || p[3] != 7 {
		t.Errorf("Black Goat's Power after one Ghoul and Done: %v", p)
	}
}

// The Auto-Play Timeout in the options window: off by default, a day when on,
// days, hours and minutes, a ? with an explanation; the bot's moves are marked
// in the Moves and History tabs.
func TestWebCthulhuWars_AutoPlayTimeoutIsSetInTheOptions(t *testing.T) {
	baseURL, a, cfg := appServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	openingDirection(t, baseURL, room, "Alice")
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	openOptions(t, ctx)
	waitJS(t, ctx, "the section", `document.getElementById("autoplay-on") !== null`, 15*time.Second)
	// Off by default, with a day.
	if evalBool(t, ctx, `document.getElementById("autoplay-on").checked`) {
		t.Error("the option should start off")
	}
	if got := evalString(t, ctx, `["days", "hours", "minutes"].map(k => document.getElementById("autoplay-" + k).value).join(",")`); got != "1,0,0" {
		t.Errorf("default timeout (days,hours,minutes) = %q", got)
	}
	if got := evalString(t, ctx, `document.querySelector('.options-section[data-section="autoplay"] h3').textContent`); got != "Auto-Play Timeout" {
		t.Errorf("heading %q", got)
	}
	// The ? explains it.
	clickSel(t, ctx, "#autoplay-help")
	waitJS(t, ctx, "the modal", `document.getElementById("autoplay-help-dialog") && document.getElementById("autoplay-help-dialog").open`, 5*time.Second)
	text := evalString(t, ctx, `document.getElementById("autoplay-help-dialog").textContent`)
	for _, want := range []string{"tactical bot", "24 hours", "auto-play", "starts the count again"} {
		if !strings.Contains(text, want) {
			t.Errorf("the explanation lacks %q: %s", want, text)
		}
	}
	clickSel(t, ctx, "#autoplay-help-dialog-close")
	waitJS(t, ctx, "closed", `document.getElementById("autoplay-help-dialog") === null`, 5*time.Second)

	// Switch it on, set 0 days 0 hours 2 minutes.
	clickSel(t, ctx, "#autoplay-on")
	waitJS(t, ctx, "on", `document.getElementById("autoplay-state").textContent.indexOf("On.") === 0`, 10*time.Second)
	evalString(t, ctx, `(function () {
		var set = function (id, v) { var e = document.getElementById("autoplay-" + id); e.value = String(v); e.dispatchEvent(new Event("input", { bubbles: true })); };
		set("days", 0); set("hours", 0); set("minutes", 2);
		return "ok";
	})()`)
	// Nothing is saved until the player says so: the fields are only edited.
	if evalBool(t, ctx, `document.getElementById("autoplay-save").disabled`) {
		t.Error("the Set button should be enabled after an edit")
	}
	clickSel(t, ctx, "#autoplay-save")
	waitJS(t, ctx, "saved", `document.getElementById("autoplay-save").disabled && document.getElementById("autoplay-minutes").value === "2" && document.getElementById("autoplay-days").value === "0"`, 10*time.Second)
	var s struct {
		Enabled        bool   `json:"enabled"`
		TimeoutSeconds int    `json:"timeout_seconds"`
		Deadline       string `json:"deadline"`
	}
	deadline := time.Now().Add(10 * time.Second)
	alice := browserGuestKey(t, ctx) // the seat is the browser's
	for {
		doJSONAs(t, alice, "GET", baseURL+"/api/games/"+room+"/autoplay?player_name=Alice", nil, &s)
		if s.Enabled && s.TimeoutSeconds == 120 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the server holds %+v", s)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Less than a minute is refused, and the page keeps the saved value.
	evalString(t, ctx, `(function () { var e = document.getElementById("autoplay-minutes"); e.value = "0"; e.dispatchEvent(new Event("input", { bubbles: true })); return "ok"; })()`)
	clickSel(t, ctx, "#autoplay-save")
	time.Sleep(300 * time.Millisecond)
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/autoplay?player_name=Alice", nil, &s)
	if s.TimeoutSeconds != 120 {
		t.Errorf("a timeout of no minutes was saved: %+v", s)
	}

	// Two minutes pass without a move: the bot plays Alice's turn, and the
	// page marks its moves as auto-play.
	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`UPDATE games SET updated_at = datetime('now', '-300 seconds') WHERE room_code = ?`, room); err != nil {
		t.Fatal(err)
	}
	if got := a.Manager().AutoPlayDue(context.Background()); len(got) != 1 {
		t.Fatalf("due: %v", got)
	}
	clickSel(t, ctx, "#tab-moves")
	waitJS(t, ctx, "auto-play in the Moves tab", `document.querySelectorAll("#move-list .mv").length > 1 && [...document.querySelectorAll("#move-list .mv")].some(b => b.textContent.indexOf("(auto-play)") >= 0)`, 20*time.Second)
	if n := evalInt(t, ctx, `[...document.querySelectorAll("#move-list .mv")].filter(b => b.textContent.indexOf("(auto-play)") >= 0 && b.parentElement.className.indexOf("by-seat-1") >= 0).length`); n == 0 {
		t.Error("the bot's moves are Alice's, marked auto-play")
	}
	if got := evalString(t, ctx, `[...document.querySelectorAll("#move-list .mv")].find(b => b.textContent.indexOf("(auto-play)") >= 0).title`); !strings.Contains(got, "tactical bot") {
		t.Errorf("title %q", got)
	}
	clickSel(t, ctx, "#tab-history")
	waitJS(t, ctx, "the history", `document.getElementById("history-pane") !== null`, 5*time.Second)
	// The History shows the turn the bot took for Alice, marked.
	got := evalString(t, ctx, `document.getElementById("history-pane").textContent`)
	if !strings.Contains(got, "auto-play") {
		t.Errorf("the history does not mark the bot's turn: %q", got)
	}
	// ... and the count began again: the option says when the next one is due.
	waitJS(t, ctx, "a new deadline", `document.getElementById("autoplay-state").textContent.indexOf("2 minutes") >= 0 || document.getElementById("autoplay-state").textContent.indexOf("plays for you at") >= 0`, 10*time.Second)
}

// In the lobby anyone at the table can change the cult of an open seat.
func TestWebLobby_TheCultOfAnOpenSeatIsChosenInTheLobby(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "lobby": true, "seat_bots": map[string]string{"2": "tactical"}}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	games, _ := json.Marshal([]map[string]any{{"code": room, "def": "cthulhu-wars", "name": "Alice", "ts": time.Now().UnixMilli()}})
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/healthz"),
		chromedp.Evaluate(fmt.Sprintf(`localStorage.setItem("bingle.games", %q); localStorage.setItem("bingle.name", "\"Alice\""); true`, games), nil),
		chromedp.Navigate(baseURL+"/game/"+room),
		chromedp.WaitVisible("#lobby", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	// An open seat has a list of cults; a seat that is taken (Alice's, the bot's) has not.
	waitJS(t, ctx, "the cult lists", `document.querySelectorAll("#lobby-seats select.lobby-cult-select").length === 2`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#lobby-seats [data-seat="1"] select, #lobby-seats [data-seat="2"] select.lobby-cult-select').length`); n != 0 {
		t.Errorf("the seats Alice and the bot hold have a cult list (%d)", n)
	}
	// The cults played by the taken seats cannot be chosen; the others can.
	if got := evalString(t, ctx, `[...document.querySelectorAll('#cult-3 option')].map(o => o.value + (o.disabled ? "!" : "")).join(",")`); got != "gc!,cc!,bg,ys,ww,sl,ow,tt" {
		t.Errorf("options of seat 3 = %q", got)
	}
	// Seat 3 becomes the Sleeper.
	evalString(t, ctx, `(function () { var s = document.getElementById("cult-3"); s.value = "sl"; s.dispatchEvent(new Event("change", { bubbles: true })); return "ok"; })()`)
	waitJS(t, ctx, "the Sleeper", `document.querySelector('#lobby-seats [data-seat="3"] .swatch.color-orange') !== null && document.getElementById("cult-3").value === "sl"`, 15*time.Second)
	var g struct {
		GameDefID string `json:"game_def_id"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &g)
	if g.GameDefID != "cthulhu-wars~gc-cc-sl-ys" {
		t.Errorf("the game is %q", g.GameDefID)
	}
	// The join row of a visitor names the new cult.
	if got := evalString(t, ctx, `[...document.querySelectorAll('#cult-4 option')].map(o => o.value + (o.disabled ? "!" : "")).join(",")`); got != "gc!,cc!,bg,ys,ww,sl,ow,tt" {
		t.Errorf("options of seat 4 after the change = %q", got)
	}
	// A friend arrives through the link and sees the Sleeper in seat 3.
	friend, cancelF := newChromedpContext(t, 60*time.Second)
	defer cancelF()
	if err := chromedp.Run(friend, chromedp.Navigate(baseURL+"/game/"+room), chromedp.WaitVisible("#lobby-join", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, friend, `[...document.querySelectorAll('#lobby-seat option')].map(o => o.textContent).join(",")`); got != "Sleeper,Yellow Sign" {
		t.Errorf("the visitor's seats = %q", got)
	}
}

// The chat has emoji: a button opens a panel, a click puts one in the message.
func TestWebCthulhuWars_TheChatHasEmoji(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the chat", `document.getElementById("chat-input") && document.getElementById("emoji-button")`, 15*time.Second)
	if evalBool(t, ctx, `!document.getElementById("emoji-panel").hidden`) {
		t.Error("the panel starts closed")
	}
	clickSel(t, ctx, "#emoji-button")
	waitJS(t, ctx, "the panel", `!document.getElementById("emoji-panel").hidden && document.getElementById("emoji-button").getAttribute("aria-expanded") === "true"`, 5*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll("#emoji-panel .emoji-cell").length`); n < 60 {
		t.Errorf("%d emoji in the panel", n)
	}
	// Type, then pick two emoji: they land at the cursor, the panel stays open.
	evalString(t, ctx, `(function () {
		var i = document.getElementById("chat-input"); i.focus(); i.value = "hello "; i.setSelectionRange(6, 6);
		i.dispatchEvent(new Event("input", { bubbles: true })); return "ok";
	})()`)
	clickSel(t, ctx, `#emoji-panel .emoji-cell[data-emoji="🐙"]`)
	clickSel(t, ctx, `#emoji-panel .emoji-cell[data-emoji="👍"]`)
	if got := evalString(t, ctx, `document.getElementById("chat-input").value`); got != "hello 🐙👍" {
		t.Errorf("the message = %q", got)
	}
	if evalBool(t, ctx, `document.getElementById("emoji-panel").hidden`) {
		t.Error("the panel closed after a pick")
	}
	// Send it: the line carries the emoji.
	clickSel(t, ctx, "#chat-send")
	waitJS(t, ctx, "the line", `[...document.querySelectorAll("#chat-log .chat-text")].some(e => e.textContent === "hello 🐙👍")`, 10*time.Second)
	var chat []struct {
		Text string `json:"text"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode+"/chat", nil, &chat)
	if len(chat) != 1 || chat[0].Text != "hello 🐙👍" {
		t.Errorf("the server holds %+v", chat)
	}
	// The recent emoji lead the panel; Escape closes it.
	if got := evalString(t, ctx, `document.querySelector("#emoji-panel .emoji-grid").firstChild.getAttribute("data-emoji")`); got != "👍" {
		t.Errorf("first of the recent emoji = %q", got)
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector("#emoji-panel .emoji-cell").dispatchEvent(new KeyboardEvent("keydown", {key: "Escape", bubbles: true})); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "closed", `document.getElementById("emoji-panel").hidden`, 5*time.Second)
}
