package app_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/auth"
)

// The setup page of Cthulhu Wars: tabs of tiles, the current selections, the
// notice about too many extras, the shuffle and setups to start from.

const seatCultsJS = `[...document.querySelectorAll("#slots-field .slot-cult")].map(c => c.dataset.value).join(",")`

// seatsReady says the seats of a table of n are drawn with their factions.
func seatsReady(n int) string {
	return fmt.Sprintf(`document.querySelectorAll("#slots-field .slot-cult").length === %d && document.querySelector("#cw-setup") !== null && document.querySelector("#slots-field .slot-cult").dataset.value !== undefined`, n)
}

func seatCultsIs(want string) string { return seatCultsJS + ` === "` + want + `"` }

func openPickerTab(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	clickSel(t, ctx, `.cw-tab[data-tab="`+id+`"]`)
	waitJS(t, ctx, "the tab "+id, `document.querySelector('.cw-tab[data-tab="`+id+`"]').classList.contains("on")`, 5*time.Second)
}

const extrasJS = `document.querySelectorAll('#cw-chosen .cw-tile:is([data-kind="monster"],[data-kind="terror"],[data-kind="goo"])').length`

func TestWebCthulhuWarsSetup_PickingUnitsOneByOne(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1280, 900), chromedp.Navigate(baseURL+"/?game=cthulhu-wars")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "four seats", seatsReady(4), 20*time.Second)

	// The old wall of check boxes is gone, and so is the list of layouts.
	if n := evalInt(t, ctx, `document.querySelectorAll("#create-game-form input[data-option]").length`); n != 0 {
		t.Errorf("%d option check boxes are left", n)
	}
	if !evalBool(t, ctx, `document.getElementById("setup-variants").hidden`) {
		t.Error("the list of layouts still shows beside the Maps tab")
	}
	// Five tabs, and a picture on every map.
	if got := evalString(t, ctx, `[...document.querySelectorAll(".cw-tab")].map(b => b.dataset.tab).join(",")`); got != "maps,factions,neutrals,independents,other" {
		t.Errorf("tabs = %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#cw-panel .cw-tile[data-kind="map"] img').length`); n != 8 {
		t.Errorf("%d map pictures, want 8", n)
	}

	// Every tile has an icon that drew, its name beneath it.
	for _, tab := range []string{"factions", "neutrals", "independents", "other"} {
		openPickerTab(t, ctx, tab)
		if n := evalInt(t, ctx, `[...document.querySelectorAll("#cw-panel .cw-tile")].filter(b => !b.querySelector(".cw-tile-icon svg, .cw-tile-icon img") || !b.querySelector(".cw-tile-name").textContent).length`); n != 0 {
			t.Errorf("%s: %d tiles without an icon or a name", tab, n)
		}
	}
	openPickerTab(t, ctx, "factions")
	waitJS(t, ctx, "the glyphs to load", `[...document.querySelectorAll("#cw-panel .cw-glyph")].every(i => i.complete && i.naturalWidth > 0)`, 10*time.Second)

	// Hovering shows a summary.
	openPickerTab(t, ctx, "neutrals")
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		var p []float64
		if err := chromedp.Evaluate(`(function(){const el=document.querySelector('#cw-panel [data-item="gnorri"]');el.scrollIntoView({block:"center"});const b=el.getBoundingClientRect();return [b.x+b.width/2,b.y+b.height/2]})()`, &p).Do(c); err != nil {
			return err
		}
		return chromedp.MouseEvent("mouseMoved", p[0], p[1]).Do(c)
	})); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the hover card", `document.querySelector(".hover-card") && document.querySelector(".hover-card").textContent.includes("Grottos")`, 5*time.Second)

	// A unit goes into the selections and comes out again from there.
	clickSel(t, ctx, `#cw-panel [data-item="gnorri"]`)
	waitJS(t, ctx, "the Gnorri chosen", `document.querySelector('#cw-chosen [data-item="gnorri"]') && document.querySelector('#cw-panel [data-item="gnorri"]').classList.contains("on")`, 5*time.Second)
	clickSel(t, ctx, `#cw-chosen [data-item="gnorri"]`)
	waitJS(t, ctx, "the Gnorri taken out", `!document.querySelector('#cw-chosen [data-item="gnorri"]') && !document.querySelector('#cw-panel [data-item="gnorri"]').classList.contains("on")`, 5*time.Second)

	// Three extras at a table of four are what is recommended; a fourth is not an error.
	for _, id := range []string{"gnorri", "hound-of-tindalos", "satyr"} {
		clickSel(t, ctx, `#cw-panel [data-item="`+id+`"]`)
	}
	if !evalBool(t, ctx, `document.getElementById("cw-notice").hidden`) {
		t.Errorf("a notice at the recommended number: %q", evalString(t, ctx, `document.getElementById("cw-notice").textContent`))
	}
	openPickerTab(t, ctx, "independents")
	clickSel(t, ctx, `#cw-panel [data-item="yig"]`)
	waitJS(t, ctx, "the notice", `!document.getElementById("cw-notice").hidden && document.getElementById("cw-notice").textContent.includes("3 recommended for 4 players")`, 5*time.Second)
	// Spellbooks and rules do not count.
	clickSel(t, ctx, `#cw-chosen [data-item="yig"]`)
	openPickerTab(t, ctx, "other")
	clickSel(t, ctx, `#cw-panel [data-item="high-priests"]`)
	waitJS(t, ctx, "no notice", `document.getElementById("cw-notice").hidden`, 5*time.Second)

	// What is chosen is what the game has.
	evalString(t, ctx, `document.getElementById("create_player_name").value = "Alice"; "ok"`)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	loc := pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	var info struct {
		GameDefID string `json:"game_def_id"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+loc[strings.LastIndex(loc, "/")+1:], nil, &info); code != 200 {
		t.Fatalf("get game: %d", code)
	}
	// The Satyr is a whole pack by itself; the Gnorri and the Hound are single cards.
	if want := "cthulhu-wars~gc-cc-bg-ys-hp-sa-nu220"; info.GameDefID != want {
		t.Errorf("game = %q, want %q", info.GameDefID, want)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// A game cannot start with an empty seat.
func TestWebCthulhuWarsSetup_EverySeatNeedsAFaction(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=cthulhu-wars")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "four seats", seatsReady(4), 20*time.Second)
	clickSel(t, ctx, `#cw-chosen [data-item="bg"]`)
	waitJS(t, ctx, "an open seat", seatCultsIs("gc,cc,,ys"), 5*time.Second)
	evalString(t, ctx, `document.getElementById("create_player_name").value = "Alice"; "ok"`)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	waitJS(t, ctx, "the refusal", `document.body.textContent.includes("Choose a faction for every seat.")`, 5*time.Second)
	if strings.Contains(evalString(t, ctx, `location.pathname`), "/game/") {
		t.Error("a game was made with an empty seat")
	}
}

// The shuffle fills open seats, adds up to the recommended number and leaves what
// the player chose alone; again, it deals anew instead of adding more.
func TestWebCthulhuWarsSetup_Shuffle(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=cthulhu-wars-5p")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "five seats", seatsReady(5), 20*time.Second)
	openPickerTab(t, ctx, "independents")
	clickSel(t, ctx, `#cw-panel [data-item="yig"]`)
	clickSel(t, ctx, `#cw-chosen [data-item="bg"]`)
	waitJS(t, ctx, "an open seat", `(`+seatCultsJS+`).split(",").filter(Boolean).length === 4`, 5*time.Second)
	for i := 0; i < 5; i++ {
		clickSel(t, ctx, "#cw-shuffle")
		waitJS(t, ctx, "four extras", extrasJS+` === 4`, 5*time.Second)
		waitJS(t, ctx, "every seat filled", `(`+seatCultsJS+`).split(",").filter(Boolean).length === 5`, 5*time.Second)
		if n := evalInt(t, ctx, `document.querySelectorAll('#cw-chosen [data-item="yig"]').length`); n != 1 {
			t.Fatalf("shuffle %d took away what was chosen by hand", i)
		}
		if !evalBool(t, ctx, `document.getElementById("cw-notice").hidden`) {
			t.Error("a shuffle went past the recommended number")
		}
	}
	// Faction and extras together still make a game.
	evalString(t, ctx, `document.getElementById("create_player_name").value = "Alice"; "ok"`)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
}

// An account saves a setup and shares it; a guest cannot save but may start from
// it, or from a ready-made one.
func TestWebCthulhuWarsSetup_SavedSetups(t *testing.T) {
	a, base := membersServer(t)
	ana := member(t, a, base, "ana")
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	signInChrome(t, ctx, base, ana.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/?game=cthulhu-wars")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "four seats", seatsReady(4), 20*time.Second)
	waitJS(t, ctx, "the save button", `!document.getElementById("cw-profile-save").hidden`, 5*time.Second)
	openPickerTab(t, ctx, "neutrals")
	clickSel(t, ctx, `#cw-panel [data-item="gug"]`)
	clickSel(t, ctx, "#cw-profile-save")
	evalString(t, ctx, `document.getElementById("cw-profile-name").value = "Friday night"; document.getElementById("cw-profile-share").click(); "ok"`)
	clickSel(t, ctx, "#cw-profile-confirm")
	waitJS(t, ctx, "the setup saved", `document.getElementById("cw-profile-msg").textContent.includes("Friday night") && document.getElementById("cw-profile").selectedOptions[0].textContent.includes("Friday night")`, 10*time.Second)
	if evalBool(t, ctx, `document.getElementById("cw-profile-delete").hidden`) {
		t.Error("no way to delete one's own setup")
	}
	mu.Lock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
	mu.Unlock()

	// A setup with more factions than seats keeps them.
	if code := ana.do("POST", "/api/setup-profiles", map[string]any{"name": "Choose in the lobby", "game_def_id": "cthulhu-wars", "shared": true,
		"options": map[string]any{"factions": []any{"gc", "cc", "bg", "ys"}, "faction-pool": []any{"gc", "cc", "bg", "ys", "tt"}}}, nil); code != 200 {
		t.Fatalf("save a pool: %d", code)
	}

	guest, cancel2 := newChromedpContext(t, 120*time.Second)
	defer cancel2()
	if err := chromedp.Run(guest, chromedp.Navigate(base+"/?game=cthulhu-wars-5p")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, guest, "five seats", seatsReady(5), 20*time.Second)
	if !evalBool(t, guest, `document.getElementById("cw-profile-save").hidden`) {
		t.Error("a guest is offered saving")
	}
	waitJS(t, guest, "the shared setup", `[...document.querySelectorAll("#cw-profile option")].some(o => o.textContent === "Friday night (@ana)")`, 10*time.Second)
	// Starting from it brings its map, its seats and its unit.
	evalString(t, guest, `(function(){const s=document.getElementById("cw-profile");s.value=[...s.options].find(o=>o.textContent==="Friday night (@ana)").value;s.dispatchEvent(new Event("change",{bubbles:true}));return "ok"})()`)
	waitJS(t, guest, "the setup applied", `document.getElementById("game_def_id").value === "cthulhu-wars" && `+seatCultsIs("gc,cc,bg,ys")+` && document.querySelector('#cw-chosen [data-item="gug"]') !== null`, 20*time.Second)
	if evalBool(t, guest, `!document.getElementById("cw-profile-delete").hidden`) {
		t.Error("a guest is offered deleting another's setup")
	}
	evalString(t, guest, `(function(){const s=document.getElementById("cw-profile");s.value=[...s.options].find(o=>o.textContent.startsWith("Choose in the lobby")).value;s.dispatchEvent(new Event("change",{bubbles:true}));return "ok"})()`)
	waitJS(t, guest, "the pool applied", `document.getElementById("cw-seat-notice").textContent.includes("5 factions for 4 seats") && document.querySelector('#cw-chosen [data-item="tt"]') !== null && document.getElementById("lobby").disabled`, 20*time.Second)
	// A ready-made one replaces it.
	evalString(t, guest, `(function(){const s=document.getElementById("cw-profile");s.value="preset:primeval";s.dispatchEvent(new Event("change",{bubbles:true}));return "ok"})()`)
	waitJS(t, guest, "the ready-made setup", `document.getElementById("game_def_id").value === "cthulhu-wars-primeval-3p" && `+seatCultsIs("sl,ow,tt")+` && !document.querySelector('#cw-chosen [data-item="gug"]') && !document.getElementById("lobby").disabled`, 20*time.Second)
	// Changing anything leaves the setup behind: the list no longer claims it.
	clickSel(t, guest, `#cw-chosen [data-item="moonbeast"]`)
	waitJS(t, guest, "the list reset", `document.getElementById("cw-profile").value === ""`, 5*time.Second)
}

// More factions than seats: the lobby must be on, and there the table chooses from
// those factions alone, a player's own seat included.
func TestWebCthulhuWarsSetup_MoreFactionsThanSeats(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=cthulhu-wars")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "four seats", seatsReady(4), 20*time.Second)
	// The lobby is a choice until there is a pool.
	evalString(t, ctx, `document.getElementById("lobby").click(); "ok"`)
	openPickerTab(t, ctx, "factions")
	clickSel(t, ctx, `#cw-panel [data-item="sl"]`)
	waitJS(t, ctx, "the lobby forced on", `document.getElementById("lobby").checked && document.getElementById("lobby").disabled`, 5*time.Second)
	if got := evalString(t, ctx, seatCultsJS); got != "gc,cc,bg,ys" {
		t.Errorf("seats = %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#cw-chosen [data-kind="faction"]').length`); n != 5 {
		t.Errorf("%d factions in the selections", n)
	}
	// Taking a seated faction out seats the spare one, and the lobby is free again.
	clickSel(t, ctx, `#cw-chosen [data-item="bg"]`)
	waitJS(t, ctx, "the spare seated", seatCultsIs("gc,cc,sl,ys")+` && !document.getElementById("lobby").disabled && !document.getElementById("lobby").checked`, 5*time.Second)
	clickSel(t, ctx, `#cw-panel [data-item="bg"]`)
	waitJS(t, ctx, "a pool again", `document.getElementById("lobby").disabled`, 5*time.Second)
	// Open seats keep a friend's place: the lobby is where the choice is made.
	evalString(t, ctx, `document.querySelectorAll("#slots-field .slot-select").forEach((s, i) => { if (i) { s.value = "open"; s.dispatchEvent(new Event("change", {bubbles: true})); } }); "ok"`)
	evalString(t, ctx, `document.getElementById("create_player_name").value = "Alice"; "ok"`)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	waitJS(t, ctx, "the lobby", `document.getElementById("lobby-pool") !== null && document.getElementById("cult-1") !== null`, 20*time.Second)
	if got := evalString(t, ctx, `[...document.querySelectorAll('#cult-1 option')].map(o => o.value).join(",")`); got != "gc,cc,bg,ys,sl" {
		t.Errorf("Alice's choices = %q", got)
	}
	evalString(t, ctx, `(function () { var s = document.getElementById("cult-1"); s.value = "bg"; s.dispatchEvent(new Event("change", { bubbles: true })); return "ok"; })()`)
	waitJS(t, ctx, "her choice", `document.getElementById("cult-1") && document.getElementById("cult-1").value === "bg" && document.querySelector('#lobby-seats [data-seat="1"] .swatch.color-red') !== null`, 15*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
