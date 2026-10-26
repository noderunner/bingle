package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The third playtest: the Ritual track, unit icons by class with a corner
// star, chat, the lobby with its invite link, and reaction settings.

func TestWebCthulhuWars_RitualTrackShowsWhereTheMarkerStands(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 1}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 3, 3, 3, 3}, [5]uint8{}, func(d *cthulhuwars.Data) { d.Ritual = 3 })
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the track", `document.querySelectorAll("#ritual-track .ritual-step").length === 8`, 10*time.Second)
	// Four players: 5 6 7 7 8 8 9 10, then Instant Death; the marker stands
	// on the fourth space after three Rituals.
	if got := evalString(t, ctx, `[...document.querySelectorAll("#ritual-track .ritual-step")].map(e => e.textContent + ":" + e.dataset.state).join(" ")`); got != "5:past 6:past 7:past 7:next 8:ahead 8:ahead 9:ahead 10:ahead" {
		t.Errorf("track = %q", got)
	}
	// The marker says what the next Ritual costs: no line of words besides.
	if got := evalString(t, ctx, `document.getElementById("ritual").textContent`); strings.Contains(got, "next Ritual") || !strings.Contains(got, "☠") {
		t.Errorf("ritual text = %q", got)
	}

	// Instant Death reached.
	doc = cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 1}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 3, 3, 3, 3}, [5]uint8{}, func(d *cthulhuwars.Data) { d.Ritual = 8; d.Instant = true })
	room = cwGameFrom(t, baseURL, doc)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "instant death", `document.querySelector("#ritual-track .ritual-end.reached")`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll("#ritual-track .ritual-step.past").length`); n != 8 {
		t.Errorf("%d spaces behind the marker, want all 8", n)
	}
}

func TestWebCthulhuWars_UnitsAreSizedByClassAndMarkedWithACornerStar(t *testing.T) {
	baseURL := newRESTTestServer(t)
	// Great Cthulhu has Submerge (enhances Cthulhu): only he gets a star.
	doc := cwPosition(t,
		// (two Acolytes: one stands on the Gate, the other beside it as a token)
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {1, "deep-one", "south-pacific", 1}, {1, "cthulhu", "south-pacific", 1}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 3, 3, 3, 3}, [5]uint8{0, 1 << 4, 0, 0, 0}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "tokens", `document.querySelector('#board .token[data-piece="cthulhu"] .unit-icon')`, 15*time.Second)
	scale := func(piece string) float64 {
		var v float64
		js := `(function(){ var g = document.querySelector('#board .token[data-piece="` + piece + `"][data-color="green"] .unit-icon'); var m = /scale\(([0-9.]+)\)/.exec(g.getAttribute("transform")); return parseFloat(m[1]); })()`
		if err := chromedp.Run(ctx, chromedp.Evaluate(js, &v)); err != nil {
			t.Fatal(err)
		}
		return v
	}
	c, d, g := scale("cultist"), scale("deep-one"), scale("cthulhu")
	if !(c < d && d < g) {
		t.Errorf("cultist %.3f < monster %.3f < Great Old One %.3f expected", c, d, g)
	}
	// Each unit has an icon of its own.
	if n := evalInt(t, ctx, `new Set([...document.querySelectorAll('#board .token.color-green .unit-icon')].map(e => e.closest(".token").dataset.piece)).size`); n != 3 {
		t.Errorf("%d green icons", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .token[data-piece="cthulhu"] .token-star').length`); n != 1 {
		t.Errorf("Cthulhu's star: %d", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .token[data-piece="deep-one"] .token-star, #board .token[data-piece="cultist"] .token-star').length`); n != 0 {
		t.Errorf("%d stars on units no spellbook enhances", n)
	}
	// No ring any more: just the star.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .token-ring, #board .token-ring-edge').length`); n != 0 {
		t.Errorf("%d rings", n)
	}
}

func TestWebCthulhuWars_ChatBetweenAPlayerAndASpectator(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	alice, cancelA := newChromedpContext(t, 90*time.Second)
	defer cancelA()
	watcher, cancelW := newChromedpContext(t, 90*time.Second)
	defer cancelW()
	openGameAs(t, alice, baseURL, room, "cthulhu-wars", "Alice")
	// The spectator has no name remembered for this game.
	if err := chromedp.Run(watcher,
		chromedp.Navigate(baseURL+"/game/"+room),
		chromedp.WaitReady("#board-svg", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, alice, "chat", `document.getElementById("chat-input")`, 10*time.Second)
	waitJS(t, watcher, "chat", `document.getElementById("chat-input") && document.getElementById("chat-name")`, 10*time.Second)
	waitJS(t, watcher, "live", `document.getElementById("conn-badge") && document.getElementById("conn-badge").textContent === "live"`, 10*time.Second)

	if err := chromedp.Run(alice,
		chromedp.SendKeys("#chat-input", "Who takes the Black Goat?\r", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, alice, "the sender's line", `[...document.querySelectorAll("#chat-log .chat-line")].some(e => e.textContent.indexOf("Who takes the Black Goat?") >= 0)`, 10*time.Second)
	waitJS(t, watcher, "the spectator hears it", `[...document.querySelectorAll("#chat-log .chat-line")].some(e => e.textContent.indexOf("Alice") >= 0 && e.textContent.indexOf("Who takes the Black Goat?") >= 0)`, 10*time.Second)

	// The spectator answers under a name of their own.
	if err := chromedp.Run(watcher,
		chromedp.SendKeys("#chat-name", "Watcher", chromedp.ByID),
		chromedp.SendKeys("#chat-input", "Me, if there is room\r", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, alice, "the answer", `[...document.querySelectorAll("#chat-log .chat-line.spectator")].some(e => e.textContent.indexOf("Watcher") >= 0 && e.textContent.indexOf("Me, if there is room") >= 0)`, 10*time.Second)
	// It is on the server too.
	var log []struct {
		Name string `json:"name"`
		Seat int    `json:"seat"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room+"/chat", nil, &log); code != 200 || len(log) != 2 || log[0].Seat != 1 || log[1].Seat != 0 {
		t.Fatalf("log %d %+v", code, log)
	}
}

func TestWebCthulhuWars_LobbyGathersTheTableAndStartsTheGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "lobby": true, "seat_bots": map[string]string{"2": "random"}}
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
	// The invitation, the four factions and who sits where.
	if got := evalString(t, ctx, `document.getElementById("invite-link").value`); got != baseURL+"/game/"+room {
		t.Errorf("invite link = %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll("#lobby-seats .lobby-seat").length`); n != 4 {
		t.Errorf("%d seats", n)
	}
	if got := evalString(t, ctx, `document.querySelector('#lobby-seats [data-seat="2"]').textContent`); !strings.Contains(got, "Crawling Chaos") || !strings.Contains(got, "bot") {
		t.Errorf("seat 2 = %q", got)
	}
	if !evalBool(t, ctx, `document.getElementById("start-game").disabled`) {
		t.Error("Start is disabled while seats are open")
	}
	// No board yet.
	if n := evalInt(t, ctx, `document.querySelectorAll("#board-svg").length`); n != 0 {
		t.Errorf("the board is shown in the lobby")
	}

	// A friend takes the Yellow Sign through the invite link.
	friend, cancelF := newChromedpContext(t, 60*time.Second)
	defer cancelF()
	if err := chromedp.Run(friend,
		chromedp.Navigate(baseURL+"/game/"+room),
		chromedp.WaitVisible("#lobby-join", chromedp.ByID),
		chromedp.SendKeys("#lobby-name", "Bob", chromedp.ByID),
		chromedp.Evaluate(`document.getElementById("lobby-seat").value = "4"; true`, nil),
		chromedp.Click("#lobby-take", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "Bob arrives in the host's lobby", `document.querySelector('#lobby-seats [data-seat="4"]') && document.querySelector('#lobby-seats [data-seat="4"]').textContent.indexOf("Bob") >= 0`, 10*time.Second)

	// The host fills the remaining seat with a bot and starts.
	if err := chromedp.Run(ctx,
		chromedp.Click(`#lobby-seats [data-seat="3"] [data-act="add-bot"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "Start enabled", `document.getElementById("start-game") && !document.getElementById("start-game").disabled`, 10*time.Second)
	waitJS(t, friend, "the friend waits for the host", `document.getElementById("lobby").textContent.indexOf("Waiting for the host to start") >= 0`, 10*time.Second)
	if err := chromedp.Run(ctx, chromedp.Click("#start-game", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the board", `document.getElementById("board-svg") && !document.getElementById("lobby")`, 15*time.Second)
	// Everybody's page moves on by itself.
	waitJS(t, friend, "the friend's board", `document.getElementById("board-svg") && !document.getElementById("lobby")`, 15*time.Second)
}

// reactionChoice is when a player wants to be asked about a reaction: that of
// Devolve, the one reaction of the Great Cthulhu player these tests use.
type reactionChoice struct {
	Targeted bool  `json:"targeted"`
	AskAll   bool  `json:"ask_all"`
	AskSeats []int `json:"ask_seats"`
}

// reactionSettingsDoc is the settings as the server sends them.
type reactionSettingsDoc struct {
	Choices map[string]reactionChoice `json:"choices"`
}

// devolve is the choice for Devolve in the settings.
func (d reactionSettingsDoc) devolve() reactionChoice { return d.Choices["devolve"] }

// waitReactionSettings polls Alice's choice on the server until ok accepts it:
// the page shows a choice at once and the server follows a moment later, so a
// test that looks right after a click must wait for it.
func waitReactionSettings(t *testing.T, baseURL, room string, what string, ok func(c reactionChoice) bool) {
	t.Helper()
	var doc reactionSettingsDoc
	deadline := time.Now().Add(10 * time.Second)
	for {
		code := doJSON(t, "GET", baseURL+"/api/games/"+room+"/reactions?player_name=Alice", nil, &doc)
		set := doc.devolve()
		if code == 200 && ok(set) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("waiting for %s: settings %d %+v", what, code, set)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// openOptions opens the game options window (the reaction settings live in it).
func openOptions(t *testing.T, ctx context.Context) {
	t.Helper()
	waitJS(t, ctx, "the options button", `document.getElementById("open-options") !== null`, 15*time.Second)
	if evalBool(t, ctx, `document.getElementById("game-options") && !document.getElementById("game-options").hidden`) {
		return
	}
	clickSel(t, ctx, "#open-options")
	waitJS(t, ctx, "the options window", `document.getElementById("game-options") && !document.getElementById("game-options").hidden`, 5*time.Second)
}

func TestWebCthulhuWars_ReactionSettingsOfAPlayerWhoHoldsOne(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 3, 3, 3, 3}, [5]uint8{0, 1 << 1, 0, 0, 0}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	openOptions(t, ctx)
	waitJS(t, ctx, "reaction settings", `document.getElementById("options-reactions") && document.getElementById("reactions-devolve") && document.getElementById("ask-devolve-targeted")`, 10*time.Second)
	// Reactions are passed for the player unless they ask to be asked; being
	// targeted is the one occasion that asks from the start.
	if !evalBool(t, ctx, `document.getElementById("ask-devolve-targeted").checked`) || evalBool(t, ctx, `document.getElementById("ask-devolve-all").checked`) {
		t.Error("the defaults: asked when targeted only")
	}
	got := evalString(t, ctx, `document.getElementById("options-reactions").textContent`)
	if !strings.Contains(got, "Devolve") || !strings.Contains(got, "passed for you") || !strings.Contains(got, "When I'm targeted") || !strings.Contains(got, "You are asked about Devolve when an action is aimed at you") || strings.Contains(got, "not earned") {
		t.Errorf("reaction settings = %q", got)
	}
	// Open the list, choose the Crawling Chaos player, then everyone.
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.getElementById("reactions-devolve").open = true; true`, nil),
		chromedp.Click("#ask-devolve-seat-2", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "saved", `document.getElementById("ask-devolve-state") && document.getElementById("ask-devolve-state").textContent.indexOf("Crawling Chaos") >= 0`, 10*time.Second)
	waitReactionSettings(t, baseURL, room, "the Crawling Chaos player", func(c reactionChoice) bool {
		return c.Targeted && !c.AskAll && len(c.AskSeats) == 1 && c.AskSeats[0] == 2
	})
	if err := chromedp.Run(ctx, chromedp.Click("#ask-devolve-all", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "everyone", `document.getElementById("ask-devolve-state").textContent.indexOf("after every other player's action") >= 0`, 10*time.Second)
	waitReactionSettings(t, baseURL, room, "everyone", func(c reactionChoice) bool { return c.AskAll })
	// And back: switching it off returns to the chosen player.
	if err := chromedp.Run(ctx, chromedp.Click("#ask-devolve-all", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "back", `document.getElementById("ask-devolve-state").textContent.indexOf("Crawling Chaos") >= 0`, 10*time.Second)
	// Not asked about the player either, nor when targeted: never asked.
	if err := chromedp.Run(ctx, chromedp.Click("#ask-devolve-seat-2", chromedp.ByID), chromedp.Click("#ask-devolve-targeted", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "never asked", `document.getElementById("ask-devolve-state").textContent.indexOf("always passed for you") >= 0 && !document.querySelector("#options-reactions .auto-pass-option input:checked")`, 10*time.Second)
	waitReactionSettings(t, baseURL, room, "never asked", func(c reactionChoice) bool { return !c.Targeted && !c.AskAll && len(c.AskSeats) == 0 })
	if err := chromedp.Run(ctx, chromedp.Click("#ask-devolve-targeted", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitReactionSettings(t, baseURL, room, "asked when targeted", func(c reactionChoice) bool { return c.Targeted })
	// One bar for the viewer's own cult, not one per player.
	if n := evalInt(t, ctx, `document.querySelectorAll("#reactions-devolve").length`); n != 1 {
		t.Errorf("%d reaction lists", n)
	}
}

// "When I'm targeted" has a ? button that explains it in a modal.
func TestWebCthulhuWars_WhenImTargetedIsExplainedInAModal(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 3, 3, 3, 3}, [5]uint8{0, 1 << 1, 0, 0, 0}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	openOptions(t, ctx)
	waitJS(t, ctx, "the help button", `document.getElementById("ask-devolve-help") !== null`, 10*time.Second)
	// The button sits next to the option.
	if !evalBool(t, ctx, `document.getElementById("ask-devolve-help").parentElement === document.getElementById("ask-devolve-targeted").closest(".ask-row")`) {
		t.Error("the ? is not next to “When I'm targeted”")
	}
	if got := evalString(t, ctx, `document.getElementById("ask-devolve-help").textContent`); got != "?" {
		t.Errorf("help button %q", got)
	}
	clickSel(t, ctx, "#ask-devolve-help")
	waitJS(t, ctx, "the modal", `document.getElementById("ask-targeted-dialog") && document.getElementById("ask-targeted-dialog").open`, 5*time.Second)
	text := evalString(t, ctx, `document.getElementById("ask-targeted-dialog").textContent`)
	for _, want := range []string{"When I'm targeted", "Devolve", "an area where you have a presence", "Madness", "not an action"} {
		if !strings.Contains(text, want) {
			t.Errorf("the explanation lacks %q: %s", want, text)
		}
	}
	// Got it closes it.
	clickSel(t, ctx, "#ask-targeted-dialog-close")
	waitJS(t, ctx, "closed", `document.getElementById("ask-targeted-dialog") === null`, 5*time.Second)
	// The options window is still there, the choice untouched.
	if !evalBool(t, ctx, `document.getElementById("ask-devolve-targeted").checked`) {
		t.Error("opening the explanation changed the choice")
	}
}

// The playtester went looking for Devolve's settings before Great Cthulhu had
// earned the spellbook: the control is there from the start.
func TestWebCthulhuWars_ReactionSettingsExistBeforeTheSpellbookIsEarned(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 3, 3, 3, 3}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	openOptions(t, ctx)
	waitJS(t, ctx, "reaction settings", `document.getElementById("options-reactions") && document.getElementById("ask-devolve-all")`, 10*time.Second)
	if got := evalString(t, ctx, `document.getElementById("options-reactions").textContent`); !strings.Contains(got, "Devolve (not earned yet)") || !strings.Contains(got, "Not earned yet") {
		t.Errorf("reaction settings = %q", got)
	}
	if err := chromedp.Run(ctx, chromedp.Click("#ask-devolve-all", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "saved", `document.getElementById("ask-devolve-state").textContent.indexOf("after every other player's action") >= 0`, 10*time.Second)
	waitReactionSettings(t, baseURL, room, "everyone", func(c reactionChoice) bool { return c.AskAll })
	// One bar for the viewer's own cult, not one per player.
	if n := evalInt(t, ctx, `document.querySelectorAll("#reactions-devolve").length`); n != 1 {
		t.Errorf("%d reaction lists", n)
	}
}

// reactionTable starts a game from a position where Crawling Chaos (Bob) is
// about to act and Great Cthulhu (Alice) holds Devolve; Bob has Cultists in
// Asia (nobody of Alice's) and the South Pacific (her Gate).
func reactionTable(t *testing.T, baseURL string) string {
	t.Helper()
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}, {2, "cultist", "south-pacific", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 3, 3, 0, 0}, [5]uint8{0, 1 << 1, 0, 0, 0},
		func(d *cthulhuwars.Data) { d.Turn.Seat = 2 })
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{
		"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat_bots": map[string]string{"3": "random", "4": "random"},
		"start_state": json.RawMessage(doc), "start_seat": 2,
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	joinRoom(t, baseURL, created.RoomCode, "Bob")
	return created.RoomCode
}

// The question itself offers to stop asking, for the reasons that made it
// ask: the player who acted, or being targeted. The window that is open is
// passed at once.
func TestWebCthulhuWars_StopBeingAskedFromTheQuestionItself(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := reactionTable(t, baseURL)
	// Alice wants to be asked after Bob's actions.
	if code := doJSON(t, "PUT", baseURL+"/api/games/"+room+"/reactions", map[string]any{"player_name": "Alice", "ask_seats": []int{2}}, nil); code != 200 {
		t.Fatalf("set: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "live", `document.getElementById("conn-badge") && document.getElementById("conn-badge").textContent === "live"`, 10*time.Second)
	if code, out := moveNotation(t, baseURL, room, "Bob", "recruit@asia"); code != 200 {
		t.Fatalf("recruit: %d %v", code, out)
	}
	waitJS(t, ctx, "the shortcut", `document.getElementById("ask-stop-trigger")`, 15*time.Second)
	if got := evalString(t, ctx, `document.getElementById("ask-stop-trigger").textContent`); !strings.Contains(got, "Bob") {
		t.Errorf("shortcut = %q", got)
	}
	// Nothing was aimed at Alice: no shortcut for being targeted.
	if evalBool(t, ctx, `document.getElementById("ask-stop-targeted")`) {
		t.Error("the question is not about being targeted")
	}
	if err := chromedp.Run(ctx, chromedp.Click("#ask-stop-trigger", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the question is gone", `!document.getElementById("ask-stop-trigger")`, 15*time.Second)
	var set struct {
		Choices map[string]reactionChoice `json:"choices"`
		Pending any                       `json:"pending"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room+"/reactions?player_name=Alice", nil, &set); code != 200 || set.Choices["devolve"].AskAll || len(set.Choices["devolve"].AskSeats) != 0 || set.Pending != nil {
		t.Fatalf("settings %d %+v", code, set)
	}
}

// Being targeted asks by default, and the question says so and offers to stop.
func TestWebCthulhuWars_BeingTargetedAsksAndCanBeSwitchedOffFromTheQuestion(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := reactionTable(t, baseURL)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "live", `document.getElementById("conn-badge") && document.getElementById("conn-badge").textContent === "live"`, 10*time.Second)
	// Bob recruits in Asia: nothing is aimed at Alice, she is not asked.
	if code, out := moveNotation(t, baseURL, room, "Bob", "recruit@asia"); code != 200 {
		t.Fatalf("recruit: %d %v", code, out)
	}
	time.Sleep(500 * time.Millisecond)
	if evalBool(t, ctx, `document.getElementById("ask-shortcuts")`) {
		t.Fatal("Alice was asked though nothing was aimed at her")
	}
	// A second table: Bob recruits among Alice's Cultists and her Gate.
	room = reactionTable(t, baseURL)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "live", `document.getElementById("conn-badge") && document.getElementById("conn-badge").textContent === "live"`, 10*time.Second)
	if code, out := moveNotation(t, baseURL, room, "Bob", "recruit@south-pacific"); code != 200 {
		t.Fatalf("recruit: %d %v", code, out)
	}
	waitJS(t, ctx, "the question", `document.getElementById("action-done") && document.getElementById("action-done").textContent.indexOf("Pass (do not use Devolve)") >= 0`, 15*time.Second)
	waitJS(t, ctx, "the shortcut", `document.getElementById("ask-stop-targeted")`, 10*time.Second)
	if evalBool(t, ctx, `document.getElementById("ask-stop-trigger")`) {
		t.Error("Bob's actions were not chosen: no shortcut for them")
	}
	if err := chromedp.Run(ctx, chromedp.Click("#ask-stop-targeted", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the question is gone", `!document.getElementById("ask-stop-targeted")`, 15*time.Second)
	waitReactionSettings(t, baseURL, room, "not asked when targeted", func(c reactionChoice) bool { return !c.Targeted })
}

// The choice must be changeable at any time — above all while the other
// players are acting and the page is redrawn many times a second. A click that
// takes a human's tenth of a second used to be lost whenever a move landed in
// between.
func TestWebCthulhuWars_TheReactionChoiceCanBeChangedWhileOthersPlay(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 3}, {2, "cultist", "asia", 3}, {3, "cultist", "south-america-west", 3}, {4, "cultist", "europe", 3}},
		map[string]int{"south-pacific": 1, "asia": 2, "south-america-west": 3, "europe": 4},
		[5]int{0, 1, 14, 14, 14}, [5]uint8{0, 1 << 1, 0, 0, 0}, nil)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{
		"game_def_id": "cthulhu-wars", "player_name": "Alice",
		"seat_bots":   map[string]string{"2": "random", "3": "random", "4": "random"},
		"start_state": json.RawMessage(doc), "start_seat": 1,
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	openOptions(t, ctx)
	waitJS(t, ctx, "the bar", `document.getElementById("options-reactions") && document.getElementById("ask-devolve-all")`, 10*time.Second)
	// The bar is built once: the same element for as long as the game is open.
	evalString(t, ctx, `window.__switch = document.getElementById("ask-devolve-all"); "ok"`)
	if err := chromedp.Run(ctx, chromedp.Click("#ask-devolve-all", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "on", `document.getElementById("ask-devolve-state").textContent.indexOf("after every other player's action") >= 0`, 10*time.Second)

	// Alice passes and the three bots play on: a stream of moves, and of
	// redraws of the page, while she changes her mind again and again.
	if code, _ := moveNotation(t, baseURL, room, "Alice", "pass"); code != 200 {
		t.Fatalf("pass: %d", code)
	}
	flip := func() (before, after bool) {
		t.Helper()
		var r struct{ X, Y, W, H float64 }
		if err := chromedp.Run(ctx, chromedp.Evaluate(`(function () { var r = document.getElementById("ask-devolve-all").getBoundingClientRect(); return { X: r.x, Y: r.y, W: r.width, H: r.height }; })()`, &r)); err != nil {
			t.Fatal(err)
		}
		before = evalBool(t, ctx, `document.getElementById("ask-devolve-all").checked`)
		x, y := r.X+r.W/2, r.Y+r.H/2
		// A human click: the button is down for a moment.
		if err := chromedp.Run(ctx,
			chromedp.ActionFunc(func(c context.Context) error {
				return input.DispatchMouseEvent(input.MousePressed, x, y).WithButton(input.Left).WithClickCount(1).Do(c)
			}),
			chromedp.Sleep(120*time.Millisecond),
			chromedp.ActionFunc(func(c context.Context) error {
				return input.DispatchMouseEvent(input.MouseReleased, x, y).WithButton(input.Left).WithClickCount(1).Do(c)
			}),
			chromedp.Sleep(150*time.Millisecond),
		); err != nil {
			t.Fatal(err)
		}
		return before, evalBool(t, ctx, `document.getElementById("ask-devolve-all").checked`)
	}
	for i := 1; i <= 8; i++ {
		if before, after := flip(); before == after {
			t.Errorf("click %d had no effect (the switch stayed %v)", i, before)
		}
	}
	// The page and the server agree on where it ended, and the bar is still
	// the element it was.
	final := evalBool(t, ctx, `document.getElementById("ask-devolve-all").checked`)
	deadline := time.Now().Add(10 * time.Second)
	for {
		var doc reactionSettingsDoc
		if code := doJSON(t, "GET", baseURL+"/api/games/"+room+"/reactions?player_name=Alice", nil, &doc); code != 200 {
			t.Fatalf("settings: %d", code)
		}
		set := doc.devolve()
		if set.AskAll == final {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the server holds ask_all=%v, the page shows %v", set.AskAll, final)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !evalBool(t, ctx, `document.getElementById("ask-devolve-all") === window.__switch`) {
		t.Error("the bar was rebuilt by a redraw")
	}
	if got := evalBool(t, ctx, `document.getElementById("ask-devolve-all").checked`); got != final {
		t.Errorf("the switch moved by itself: %v -> %v", final, got)
	}
}

// The bar is for a seat whose faction has a reaction, in a game that is being
// played: not for Yellow Sign (none), not in a lobby, not for a spectator.
func TestWebCthulhuWars_ReactionBarOnlyWhereThereIsAReaction(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	visible := `(function () { var sec = document.querySelector('.options-section[data-section="reactions"]'); var d = document.getElementById("options-reactions"); return !!sec && !sec.hidden && !!d && d.children.length > 0; })()`

	// Great Cthulhu: there.
	var gc struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat_bots": map[string]string{"2": "random", "3": "random", "4": "random"}}, &gc); code != 200 {
		t.Fatalf("create: %d", code)
	}
	openGameAs(t, ctx, baseURL, gc.RoomCode, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the bar for Great Cthulhu", visible, 10*time.Second)

	// Yellow Sign: none.
	var ys struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Yola", "seat": 4, "seat_bots": map[string]string{"1": "random", "2": "random", "3": "random"}}, &ys); code != 200 {
		t.Fatalf("create: %d", code)
	}
	openGameAs(t, ctx, baseURL, ys.RoomCode, "cthulhu-wars", "Yola")
	waitJS(t, ctx, "the board", `document.querySelectorAll("#seat-1, .seat-strip").length > 0`, 10*time.Second)
	time.Sleep(300 * time.Millisecond)
	if evalBool(t, ctx, visible) {
		t.Error("Yellow Sign has no reaction: no bar")
	}

	// A table that has not started: the lobby, no bar.
	var lobby struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Lena", "lobby": true, "seat_bots": map[string]string{"2": "random"}}, &lobby); code != 200 {
		t.Fatalf("create: %d", code)
	}
	games, _ := json.Marshal([]map[string]any{{"code": lobby.RoomCode, "def": "cthulhu-wars", "name": "Lena", "ts": time.Now().UnixMilli()}})
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/healthz"),
		chromedp.Evaluate(fmt.Sprintf(`localStorage.setItem("bingle.games", %q); localStorage.setItem("bingle.name", "\"Lena\""); true`, games), nil),
		chromedp.Navigate(baseURL+"/game/"+lobby.RoomCode),
		chromedp.WaitVisible("#lobby", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if evalBool(t, ctx, visible) {
		t.Error("a lobby has no reactions: no bar")
	}
}

func evalBool(t *testing.T, ctx context.Context, expr string) bool {
	t.Helper()
	var v bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(`Boolean(`+expr+`)`, &v)); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestWebCthulhuWars_AReactionIsAskedAfterAnotherPlayersAction(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := reactionTable(t, baseURL)
	// Alice asks to be asked after every action of the others.
	if code := doJSON(t, "PUT", baseURL+"/api/games/"+room+"/reactions", map[string]any{"player_name": "Alice", "ask_all": true}, nil); code != 200 {
		t.Fatalf("set: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "live", `document.getElementById("conn-badge") && document.getElementById("conn-badge").textContent === "live"`, 10*time.Second)

	// Bob acts; Alice is asked about Devolve and passes or uses it.
	if code, out := moveNotation(t, baseURL, room, "Bob", "recruit@asia"); code != 200 {
		t.Fatalf("recruit: %d %v", code, out)
	}
	waitJS(t, ctx, "the question", `document.getElementById("action-done") && document.getElementById("action-done").textContent.indexOf("Pass (do not use Devolve)") >= 0`, 15*time.Second)
	if got := evalString(t, ctx, `document.getElementById("status-message").textContent`); !strings.Contains(got, "Devolve") || !strings.Contains(got, "Crawling Chaos") {
		t.Errorf("status = %q", got)
	}
	// Clicking a glowing Cultist devolves it.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#board .token.token-target[data-piece="cultist"][data-color="green"]').dispatchEvent(new MouseEvent("click", {bubbles: true})); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "a Deep One", `document.querySelector('#board .token[data-piece="deep-one"][data-color="green"]')`, 10*time.Second)
	// The window is still open for the other Cultist: pass.
	if err := chromedp.Run(ctx, chromedp.Click("#action-done", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "back to a normal turn", `!document.getElementById("action-done") || document.getElementById("action-done").textContent.indexOf("Pass (do not use") < 0`, 10*time.Second)
}

// Choosing another layout of a game rebuilds the seat arrangement of the
// home page; it must keep who sits where. In the playtest a table of Minimax
// and MCTS bots came out all Tactical after the layout chip was clicked.
func TestWebHome_SeatChoicesSurviveChoosingAnotherLayout(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=cthulhu-wars")); err != nil {
		t.Fatal(err)
	}
	const slotsJS = `[...document.querySelectorAll("#slots-field select.slot-select")].map(s => s.value).join(",")`
	waitJS(t, ctx, "the seats", `document.querySelectorAll("#slots-field select.slot-select").length === 4 && document.querySelector('#slots-field select.slot-select option[value="bot:mcts"]')`, 20*time.Second)
	if got := evalString(t, ctx, slotsJS); got != "me,bot:tactical,bot:tactical,bot:tactical" {
		t.Fatalf("default seats = %q", got)
	}
	// Seat 2 Minimax, seat 3 MCTS, seat 4 left open for a friend.
	evalString(t, ctx, `(function () {
		var set = function (seat, v) {
			var s = document.querySelector('#slots-field select[data-seat="' + seat + '"]');
			s.value = v;
			s.dispatchEvent(new Event("change", { bubbles: true }));
		};
		set(2, "bot:minimax"); set(3, "bot:mcts"); set(4, "open");
		return "ok";
	})()`)
	const chosen = "me,bot:minimax,bot:mcts,open"
	if got := evalString(t, ctx, slotsJS); got != chosen {
		t.Fatalf("seats after choosing = %q", got)
	}
	// Another layout of the same game, the three-player one, and back.
	for _, c := range []struct{ layout, want string }{
		{"cthulhu-wars-3-5", chosen},
		{"cthulhu-wars-3p", "me,bot:minimax,bot:mcts"},
		{"cthulhu-wars-3-5", chosen},
	} {
		evalString(t, ctx, `window.__oldSlots = document.getElementById("slots-field"); "ok"`)
		clickSel(t, ctx, `#setup-variants .chip[data-id="`+c.layout+`"]`)
		waitJS(t, ctx, "the layout "+c.layout, `document.getElementById("game_def_id").value === "`+c.layout+`" && document.getElementById("slots-field") && document.getElementById("slots-field") !== window.__oldSlots`, 15*time.Second)
		if got := evalString(t, ctx, slotsJS); got != c.want {
			t.Errorf("seats on %s = %q, want %q", c.layout, got, c.want)
		}
	}

	// Create the game from the form: the kinds reach the server.
	evalString(t, ctx, `document.getElementById("create_player_name").value = "Alice"; "ok"`)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	loc := pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	room := loc[strings.LastIndex(loc, "/")+1:]
	var info struct {
		GameDefID string `json:"game_def_id"`
		Seats     []struct {
			Seat int    `json:"seat"`
			Name string `json:"name"`
			Bot  string `json:"bot"`
		} `json:"seats"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &info); code != 200 {
		t.Fatalf("get game: %d", code)
	}
	if info.GameDefID != "cthulhu-wars-3-5" {
		t.Errorf("game = %q", info.GameDefID)
	}
	kinds := map[int]string{}
	names := map[int]string{}
	for _, s := range info.Seats {
		kinds[s.Seat], names[s.Seat] = s.Bot, s.Name
	}
	if kinds[2] != "minimax" || kinds[3] != "mcts" || names[4] != "" {
		t.Errorf("bots = %v names = %v, want minimax, mcts and an open seat 4", kinds, names)
	}
	if !strings.Contains(names[2], "(Minimax)") || !strings.Contains(strings.ToLower(names[3]), "(mcts)") {
		t.Errorf("names = %q and %q must carry the kind that was chosen", names[2], names[3])
	}
}
