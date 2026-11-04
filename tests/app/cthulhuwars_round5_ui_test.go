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

// The factions of a five-player table are tiles: they fill the seats in turn, one
// can be taken out and another put in its place, the High Priests are a tile too,
// and the choice survives another layout and reaches the server.
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
	waitJS(t, ctx, "five seats", seatsReady(5), 20*time.Second)
	if got := evalString(t, ctx, seatCultsJS); got != "gc,cc,bg,ys,ww" {
		t.Fatalf("default cults = %q", got)
	}
	openPickerTab(t, ctx, "factions")
	if n := evalInt(t, ctx, `document.querySelectorAll("#cw-panel .cw-tile").length`); n != 10 {
		t.Errorf("%d cults on offer, want 10", n)
	}
	// Taken out, a faction leaves its seat open; the next one sits there.
	clickSel(t, ctx, `#cw-panel [data-item="cc"]`)
	waitJS(t, ctx, "an open seat", seatCultsIs("gc,,bg,ys,ww"), 5*time.Second)
	waitJS(t, ctx, "the notice", `document.getElementById("cw-seat-notice").textContent.includes("needs a faction")`, 5*time.Second)
	clickSel(t, ctx, `#cw-panel [data-item="sl"]`)
	waitJS(t, ctx, "the Sleeper in that seat", seatCultsIs("gc,sl,bg,ys,ww"), 5*time.Second)
	// A sixth takes no seat: it is for the lobby. Taken out again, it leaves no trace.
	clickSel(t, ctx, `#cw-panel [data-item="cc"]`)
	waitJS(t, ctx, "the pool", `document.getElementById("cw-seat-notice").textContent.includes("6 factions for 5 seats")`, 5*time.Second)
	if got := evalString(t, ctx, seatCultsJS); got != "gc,sl,bg,ys,ww" {
		t.Errorf("cults after a sixth = %q", got)
	}
	clickSel(t, ctx, `#cw-panel [data-item="cc"]`)
	waitJS(t, ctx, "no pool", `document.getElementById("cw-seat-notice").hidden`, 5*time.Second)
	openPickerTab(t, ctx, "other")
	clickSel(t, ctx, `#cw-panel [data-item="high-priests"]`)

	// Four seats keep the first four; five again gets a cult for the fifth.
	openPickerTab(t, ctx, "maps")
	clickSel(t, ctx, `#cw-panel [data-item="cthulhu-wars"]`)
	waitJS(t, ctx, "the four-player layout", `document.getElementById("game_def_id").value === "cthulhu-wars" && `+seatsReady(4), 20*time.Second)
	if got := evalString(t, ctx, seatCultsJS); got != "gc,sl,bg,ys" {
		t.Errorf("cults on four seats = %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#cw-chosen [data-item="high-priests"]').length`); n != 1 {
		t.Errorf("High Priests in the selections after another layout: %d", n)
	}
	// A map is one choice: exactly one is in the selections.
	if n := evalInt(t, ctx, `document.querySelectorAll('#cw-chosen [data-kind="map"]').length`); n != 1 {
		t.Errorf("%d maps in the selections", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#cw-panel .cw-tile.on').length`); n != 1 {
		t.Errorf("%d maps marked in the Maps tab", n)
	}
	clickSel(t, ctx, `#cw-panel [data-item="cthulhu-wars-5p"]`)
	waitJS(t, ctx, "the five-player layout", `document.getElementById("game_def_id").value === "cthulhu-wars-5p" && `+seatsReady(5), 20*time.Second)

	// Create the game: the choice reaches the server.
	evalString(t, ctx, `document.getElementById("create_player_name").value = "Alice"; "ok"`)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	loc := pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	room := loc[strings.LastIndex(loc, "/")+1:]
	var info struct {
		GameDefID string `json:"game_def_id"`
		Seats     []struct {
			Seat  int    `json:"seat"`
			Color string `json:"color"`
		} `json:"seats"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &info); code != 200 {
		t.Fatalf("get game: %d", code)
	}
	if want := "cthulhu-wars-5p~gc-sl-bg-ys-cc-hp"; info.GameDefID != want {
		t.Errorf("game = %q, want %q", info.GameDefID, want)
	}
	colors := map[int]string{}
	for _, s := range info.Seats {
		colors[s.Seat] = s.Color
	}
	if fmt.Sprint(colors) != "map[1:green 2:orange 3:red 4:yellow 5:blue]" {
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
	waitJS(t, ctx, "five seats", seatsReady(5), 20*time.Second)
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
