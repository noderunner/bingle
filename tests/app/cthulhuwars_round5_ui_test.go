package app_test

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// The fifth round: the Windwalker and Sleeper cults, five seats and the High
// Priests, chosen on the home page.

// The create form of a five-player table lists the cults of every seat, lets
// two seats trade cults, offers the High Priests and sends the choice.
func TestWebHome_ChooseTheCultsOfAFivePlayerTableAndTheHighPriests(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)

	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=cthulhu-wars-5p")); err != nil {
		t.Fatal(err)
	}
	const cultsJS = `[...document.querySelectorAll("#slots-field select.faction-select")].map(s => s.value).join(",")`
	waitJS(t, ctx, "five seats", `document.querySelectorAll("#slots-field select.faction-select").length === 5`, 20*time.Second)
	if got := evalString(t, ctx, cultsJS); got != "gc,cc,bg,ys,ww" {
		t.Fatalf("default cults = %q", got)
	}
	// The eight cults are on offer in every seat.
	if n := evalInt(t, ctx, `document.querySelectorAll('#slots-field select.faction-select[data-cult-seat="1"] option').length`); n != 8 {
		t.Errorf("%d cults on offer, want 8", n)
	}
	// The expansion is a check box, off by default.
	if got := evalString(t, ctx, `String(document.getElementById("option-high-priests") && document.getElementById("option-high-priests").checked)`); got != "false" {
		t.Errorf("High Priests box checked = %q", got)
	}
	// The swatch of a seat follows its cult.
	if got := evalString(t, ctx, `document.querySelector('#slots-field .slot-row:nth-child(5) .swatch').className`); !strings.Contains(got, "color-white") {
		t.Errorf("seat 5 swatch = %q, want white (Windwalker)", got)
	}
	// Choosing the Sleeper for the fifth seat leaves Windwalker out.
	evalString(t, ctx, `(function () {
		var s = document.querySelector('#slots-field select.faction-select[data-cult-seat="5"]');
		s.value = "sl";
		s.dispatchEvent(new Event("change", { bubbles: true }));
		return "ok";
	})()`)
	if got := evalString(t, ctx, cultsJS); got != "gc,cc,bg,ys,sl" {
		t.Errorf("cults after the Sleeper = %q", got)
	}
	if got := evalString(t, ctx, `document.querySelector('#slots-field .slot-row:nth-child(5) .swatch').className`); !strings.Contains(got, "color-orange") {
		t.Errorf("seat 5 swatch = %q, want orange (Sleeper)", got)
	}
	// Choosing the Sleeper for seat 1 trades: seat 5 takes Great Cthulhu.
	evalString(t, ctx, `(function () {
		var s = document.querySelector('#slots-field select.faction-select[data-cult-seat="1"]');
		s.value = "sl";
		s.dispatchEvent(new Event("change", { bubbles: true }));
		return "ok";
	})()`)
	if got := evalString(t, ctx, cultsJS); got != "sl,cc,bg,ys,gc" {
		t.Errorf("cults after the trade = %q", got)
	}
	// And seat 2 asks for Windwalker (not at the table): nothing to trade.
	evalString(t, ctx, `(function () {
		var s = document.querySelector('#slots-field select.faction-select[data-cult-seat="2"]');
		s.value = "ww";
		s.dispatchEvent(new Event("change", { bubbles: true }));
		return "ok";
	})()`)
	if got := evalString(t, ctx, cultsJS); got != "sl,ww,bg,ys,gc" {
		t.Errorf("cults after Windwalker = %q", got)
	}
	evalString(t, ctx, `document.getElementById("option-high-priests").click(); "ok"`)

	// The choice survives choosing another layout and coming back.
	evalString(t, ctx, `window.__oldSlots = document.getElementById("slots-field"); "ok"`)
	clickSel(t, ctx, `#setup-variants .chip[data-id="cthulhu-wars"]`)
	waitJS(t, ctx, "the four-player layout", `document.getElementById("game_def_id").value === "cthulhu-wars" && document.getElementById("slots-field") && document.getElementById("slots-field") !== window.__oldSlots`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll("#slots-field select.faction-select").length`); n != 4 {
		t.Fatalf("%d cult lists on the four-player layout", n)
	}
	// Four seats, no two the same, and the expansion box still ticked.
	got := strings.Split(evalString(t, ctx, cultsJS), ",")
	seen := map[string]bool{}
	for _, c := range got {
		if seen[c] {
			t.Errorf("cults %v list %q twice", got, c)
		}
		seen[c] = true
	}
	if got := evalString(t, ctx, `String(document.getElementById("option-high-priests").checked)`); got != "true" {
		t.Errorf("High Priests box = %q after another layout", got)
	}
	evalString(t, ctx, `window.__oldSlots = document.getElementById("slots-field"); "ok"`)
	clickSel(t, ctx, `#setup-variants .chip[data-id="cthulhu-wars-5p"]`)
	waitJS(t, ctx, "the five-player layout", `document.getElementById("game_def_id").value === "cthulhu-wars-5p" && document.getElementById("slots-field") && document.getElementById("slots-field") !== window.__oldSlots`, 15*time.Second)
	if got := evalString(t, ctx, cultsJS); got != "sl,ww,bg,ys,gc" {
		t.Errorf("cults on coming back = %q", got)
	}

	// Create the game: the choice reaches the server.
	evalString(t, ctx, `document.getElementById("create_player_name").value = "Alice"; "ok"`)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	loc := pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	room := loc[strings.LastIndex(loc, "/")+1:]
	var info struct {
		GameDefID string `json:"game_def_id"`
		Seats     []struct {
			Seat  int    `json:"seat"`
			Name  string `json:"name"`
			Label string `json:"label"`
			Color string `json:"color"`
			Bot   string `json:"bot"`
		} `json:"seats"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &info); code != 200 {
		t.Fatalf("get game: %d", code)
	}
	if want := "cthulhu-wars-5p~sl-ww-bg-ys-gc-hp"; info.GameDefID != want {
		t.Errorf("game = %q, want %q", info.GameDefID, want)
	}
	colors := map[int]string{}
	for _, s := range info.Seats {
		colors[s.Seat] = s.Color
	}
	if fmt.Sprint(colors) != "map[1:orange 2:white 3:red 4:yellow 5:green]" {
		t.Errorf("seat colors = %v", colors)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// A table left at the layout's own cults and without the expansion stays on
// the plain layout: no derived id for something that is not different.
func TestWebHome_AnUnchangedTableKeepsThePlainLayout(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=cthulhu-wars-5p")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "five seats", `document.querySelectorAll("#slots-field select.faction-select").length === 5`, 20*time.Second)
	evalString(t, ctx, `document.getElementById("create_player_name").value = "Alice"; "ok"`)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	loc := pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	room := loc[strings.LastIndex(loc, "/")+1:]
	var info struct {
		GameDefID string `json:"game_def_id"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &info); code != 200 {
		t.Fatalf("get game: %d", code)
	}
	if info.GameDefID != "cthulhu-wars-5p" {
		t.Errorf("game = %q, want the plain five-player layout", info.GameDefID)
	}
}

// The game page of a derived five-player table: every cult's panel with its
// own colour, the map of 21 areas, the Windwalker choosing where to begin and
// the High Priests recruited from the Action list.
func TestWebCthulhuWars_DerivedFivePlayerTableWithTheExpansionCults(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{
		"game_def_id": "cthulhu-wars-5p", "player_name": "Alice", "opponent": "random",
		"options": map[string]any{"factions": []string{"ww", "sl", "gc", "cc", "ys"}, "high-priests": true},
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	var info struct {
		GameDefID string `json:"game_def_id"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &info); code != 200 {
		t.Fatalf("get game: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)

	openGameAs(t, ctx, baseURL, created.RoomCode, info.GameDefID, "Alice")
	waitJS(t, ctx, "five cult panels", `document.querySelectorAll('.seat-strip').length === 5`, 20*time.Second)
	// The cults in seat order, and the colours of their panels.
	if got := evalString(t, ctx, `[...document.querySelectorAll('.seat-strip')].map(s => s.getAttribute("data-seat") + ":" + (s.querySelector(".seat-head .swatch") ? [...s.querySelector(".seat-head .swatch").classList].filter(c => c.startsWith("color-")).join("") : "")).join(",")`); !strings.Contains(got, "color-white") || !strings.Contains(got, "color-orange") {
		t.Errorf("panels %q lack the white and orange cults", got)
	}
	// The Windwalker (seat 1) begins: the areas marked with its sigil glow.
	waitJS(t, ctx, "the choice of where to begin", `document.querySelectorAll('#board .cell.area.clickable').length >= 1`, 20*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .cell.area.clickable').length`); n != 2 {
		t.Errorf("%d areas to begin in, want the 2 Windwalker sigil areas", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// With the High Priests in, the Action list has a button of its own for him.
func TestWebCthulhuWars_RecruitTheHighPriestByItsOwnButton(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{
		"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random",
		"options": map[string]any{"high-priests": true},
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	var info struct {
		GameDefID string `json:"game_def_id"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode, nil, &info)
	if info.GameDefID != "cthulhu-wars~gc-cc-bg-ys-hp" {
		t.Fatalf("game = %q", info.GameDefID)
	}
	openingDirection(t, baseURL, created.RoomCode, "Alice")
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, created.RoomCode, info.GameDefID, "Alice")
	// All six Acolytes stand on the map at the start, so the High Priest is the
	// only Cultist that can be recruited.
	waitJS(t, ctx, "the button", `document.getElementById("action-recruit-high-priest") !== null`, 20*time.Second)
	// The button says what he costs, and its card what the High Priest is.
	if got := evalString(t, ctx, `document.getElementById("action-recruit-high-priest").textContent`); got != "Recruit High Priest (3)" {
		t.Errorf("button label %q", got)
	}
	hoverOver(t, ctx, "#action-recruit-high-priest")
	waitJS(t, ctx, "the card", `document.querySelector('.hover-card') !== null`, 5*time.Second)
	card := evalString(t, ctx, `document.querySelector('.hover-card').textContent`)
	for _, want := range []string{"Recruit High Priest", "3 Power", "Unspeakable Oath", "Cultist for every purpose"} {
		if !strings.Contains(card, want) {
			t.Errorf("the High Priest's card lacks %q: %s", want, card)
		}
	}
	clickSel(t, ctx, "#action-recruit-high-priest")
	waitJS(t, ctx, "the target", `document.querySelectorAll('#board .cell.area.clickable').length >= 1`, 10*time.Second)
	clickCell(t, ctx, "south-pacific")
	waitJS(t, ctx, "the High Priest stands", `document.querySelector('#board .token[data-position="south-pacific"][data-piece="high-priest"]') !== null`, 15*time.Second)
	waitJS(t, ctx, "paid 3", `document.querySelector('#seat-1 [data-counter="power"]').textContent === "5"`, 10*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
