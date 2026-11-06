package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// jsErrorCollector records uncaught exceptions of a page.
func collectJSErrors(ctx context.Context, mu *sync.Mutex, out *[]string) {
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*runtime.EventExceptionThrown); ok && e.ExceptionDetails != nil {
			mu.Lock()
			*out = append(*out, e.ExceptionDetails.Error())
			mu.Unlock()
		}
	})
}

func TestWebCthulhuWars_AreaMapTokensAndFactionPanels(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)

	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "map", `document.querySelectorAll('#board .cell.area').length === 17`, 15*time.Second)
	// Polygons, with the pacific drawn on both edges (two polygons).
	if n := evalInt(t, ctx, `document.querySelectorAll('#board polygon.area-bg').length`); n < 19 {
		t.Errorf("%d area polygons, want at least 19 (two oceans wrap)", n)
	}
	// Four cults start with 6 cultists in one area: one stands on the Gate, the
	// other five beside it — four cultist tokens each carrying a 5.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .token[data-piece="cultist"]').length`); n != 4 {
		t.Errorf("%d cultist tokens at the start, want 4", n)
	}
	if n := evalInt(t, ctx, `[...document.querySelectorAll('#board .token[data-piece="cultist"]')].filter(t => t.getAttribute("data-count") === "5").length`); n != 4 {
		t.Errorf("%d tokens count 5", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .gate-chip[data-holder="cultist"]').length`); n != 4 {
		t.Errorf("%d Gates with a Cultist on them, want 4", n)
	}
	// Four Gates, one per start area.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .gate-chip').length`); n != 4 {
		t.Errorf("%d Gate marks, want 4", n)
	}
	// Names and the cult panels.
	if got := evalString(t, ctx, `document.querySelector('#board .area-label[data-area="south-pacific"]').textContent`); got != "South Pacific" {
		t.Errorf("label %q", got)
	}
	waitJS(t, ctx, "four cult panels", `document.querySelectorAll('.seat-strip').length === 4 && document.querySelectorAll('.seat-strip [data-counter="power"]').length === 4`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector('#seat-1 [data-counter="power"]').textContent`); got != "8" {
		t.Errorf("power counter %q", got)
	}
	// The numbers are pictograms: an icon and a value, no words.
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-1 .seat-stats .score.stat svg.stat-icon').length`); n != 5 {
		t.Errorf("%d pictograms in the header, want power, doom, signs, spellbooks and gates", n)
	}
	// The sections: the faction ability, spellbooks, requirements, Elder Signs and units.
	if got := evalString(t, ctx, `[...document.querySelectorAll('#seat-1 .faction-card')].map(c => c.getAttribute("data-card")).join(",")`); got != "ability,books,units" {
		t.Errorf("faction cards %q", got)
	}
	// The units are icons, one each: 6 Acolytes, 4 Deep Ones, 2 Shoggoths, 2 Star
	// Spawn and Cthulhu; the 4 Deep Ones, 2 Shoggoths, 2 Star Spawn and Cthulhu are
	// in the pool, the Acolytes are on the map: greyed out.
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-1 .pool-icons .pool-unit').length`); n != 15 {
		t.Errorf("%d unit icons, want 15", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-1 .pool-icons .pool-unit[data-zone="pool"]').length`); n != 9 {
		t.Errorf("%d unit icons in the pool, want 9", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-1 .pool-icons .pool-unit.away').length`); n != 6 {
		t.Errorf("%d unit icons away, want 6", n)
	}
	// The summary counts the spellbooks, with no "of 6".
	if got := evalString(t, ctx, `document.querySelector('#seat-1 [data-counter="books"]').textContent`); strings.Contains(got, "/") {
		t.Errorf("the spellbook count says %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-1 [data-counter="books"] svg').length`); n != 1 {
		t.Errorf("%d icons on the spellbook count", n)
	}
	waitJS(t, ctx, "round bar", `document.getElementById("round-bar") && document.getElementById("round-bar").textContent.indexOf("Round 1") >= 0 && document.getElementById("ritual-track") && document.querySelectorAll("#ritual-track .ritual-step").length === 8 && document.querySelector("#ritual-track .ritual-step.next").textContent === "5"`, 5*time.Second)
	// No hand panels in this game.
	if n := evalInt(t, ctx, `document.querySelectorAll('.hand').length`); n != 0 {
		t.Errorf("%d hand panels", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_SummonByChoosingAMonsterAndAGate(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	openingDirection(t, baseURL, created.RoomCode, "Alice")
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)

	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars", "Alice")
	// Every monster has a button of its own, with what it costs just now.
	waitJS(t, ctx, "summon buttons", `document.querySelectorAll('.summon-button[data-action^="summon-"]').length === 3`, 15*time.Second)
	if got := evalString(t, ctx, `[...document.querySelectorAll('.summon-button[data-action^="summon-"]')].map(b => b.getAttribute("data-piece") + "=" + b.getAttribute("data-cost")).join(",")`); got != "deep-one=1,shoggoth=2,starspawn=3" {
		t.Errorf("summon buttons %q", got)
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("action-summon-deep-one").click(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	// Only the Gate's area is a target.
	waitJS(t, ctx, "gate area", `document.querySelectorAll('#board .cell.area.clickable').length === 1`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector('#board .cell.area.clickable').getAttribute("data-position")`); got != "south-pacific" {
		t.Fatalf("target %q", got)
	}
	clickCell(t, ctx, "south-pacific")
	waitJS(t, ctx, "summoned", `document.querySelector('#board .token[data-position="south-pacific"][data-piece="deep-one"][data-color="green"]') !== null`, 15*time.Second)
	// One Power was paid.
	waitJS(t, ctx, "power", `document.querySelector('#seat-1 [data-counter="power"]').textContent === "7"`, 10*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_MoveByClickingATokenThenItsDestination(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	openingDirection(t, baseURL, created.RoomCode, "Alice")
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars", "Alice")
	// There is no "Move" button: units are picked straight from the map.
	waitJS(t, ctx, "glowing token", `document.querySelectorAll('#board .token.token-target').length >= 1`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#action-move').length`); n != 0 {
		t.Fatalf("a Move button should not exist (%d)", n)
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#board .token.token-target[data-piece="cultist"]').dispatchEvent(new MouseEvent("click", {bubbles: true})); true`, nil)); err != nil {
		t.Fatal(err)
	}
	// Adjacent areas of the South Pacific are targets now.
	// Its six neighbours: the one that stood on the Gate is another token.
	waitJS(t, ctx, "destinations", `document.querySelectorAll('#board .cell.area.clickable').length === 6`, 10*time.Second)
	clickCell(t, ctx, "new-zealand")
	// A lone unit has moved: the move ends by itself or offers Done; either way New Zealand holds a cultist.
	waitJS(t, ctx, "arrived", `document.querySelector('#board .token[data-position="new-zealand"][data-piece="cultist"]') !== null`, 15*time.Second)
}

func TestWebCthulhuWars_RulesWindowHasTabsTablesAndImages(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "rules button", `document.getElementById("show-rules") !== null`, 15*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("show-rules").click(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	// The guide's pages, the Tutorials tab (the High Priests tutorial), Key and Controls.
	waitJS(t, ctx, "tabs", `document.querySelectorAll('#rules-dialog [role="tab"]').length === 13 && !!document.querySelector('#rules-dialog [data-tab="tutorials"]')`, 10*time.Second)
	// Exactly one panel is visible, the first tab is selected.
	if n := evalInt(t, ctx, `[...document.querySelectorAll('#rules-dialog [role="tabpanel"]')].filter(p => getComputedStyle(p).display !== "none").length`); n != 1 {
		t.Fatalf("%d visible panels", n)
	}
	if got := evalString(t, ctx, `document.querySelector('#rules-dialog [role="tab"][aria-selected="true"]').getAttribute("data-tab")`); got != "overview" {
		t.Fatalf("selected tab %q", got)
	}
	// The overview has a table and the map image loaded.
	if n := evalInt(t, ctx, `document.querySelectorAll('#rules-panel-overview table').length`); n < 1 {
		t.Errorf("overview has %d tables", n)
	}
	waitJS(t, ctx, "map image", `(function(){ var i = document.querySelector('#rules-panel-overview img.md-img'); return !!i && i.complete && i.naturalWidth > 0; })()`, 10*time.Second)
	// Piece icons render inline in table cells.
	if n := evalInt(t, ctx, `document.querySelectorAll('#rules-panel-overview td svg.md-icon').length`); n < 3 {
		t.Errorf("%d inline piece icons in the overview table", n)
	}
	// Character references are text, not markup: no "&nbsp;" shows on any page.
	if n := evalInt(t, ctx, `[...document.querySelectorAll('#rules-dialog [role="tabpanel"]')].filter(p => /&[a-z]+;|&#\d+;/i.test(p.textContent)).length`); n != 0 {
		t.Errorf("%d rules pages show a character reference as text", n)
	}
	if !evalBool(t, ctx, `document.getElementById("rules-panel-overview").textContent.indexOf("\u00a0") >= 0`) {
		t.Error("the overview lost its non-breaking spaces")
	}

	// Arrow keys move between tabs and show their panels.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#rules-dialog [data-tab="overview"]').focus(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.activeElement.dispatchEvent(new KeyboardEvent("keydown", {key: "ArrowRight", bubbles: true, cancelable: true})); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "second tab", `document.querySelector('#rules-dialog [role="tab"][aria-selected="true"]').getAttribute("data-tab") === "round" && !document.getElementById("rules-panel-round").hidden && document.getElementById("rules-panel-overview").hidden`, 5*time.Second)
	// A heading table of contents for the longer pages.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#rules-dialog [data-tab="spellbooks"]').click(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "spellbooks table", `document.querySelectorAll('#rules-panel-spellbooks table').length === 9 && document.querySelector('#rules-panel-spellbooks nav.rules-toc') !== null`, 5*time.Second)

	// Search across all pages and jump to a hit.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(function(){ var s = document.getElementById("rules-search"); s.value = "Instant Death"; s.dispatchEvent(new Event("input", {bubbles: true})); return true; })()`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "search results", `!document.getElementById("rules-results").hidden && document.querySelectorAll('#rules-results .rules-hit').length >= 1`, 5*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#rules-results .rules-hit').click(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "results gone, a panel shows", `document.getElementById("rules-results").hidden && [...document.querySelectorAll('#rules-dialog [role="tabpanel"]')].filter(p => getComputedStyle(p).display !== "none").length === 1`, 5*time.Second)

	// The Controls tab (client side) lists the keyboard.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#rules-dialog [data-tab="controls"]').click(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "keycaps", `document.querySelectorAll('#rules-panel-controls kbd').length >= 8`, 5*time.Second)
	// Escape closes the window.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("rules-dialog").close(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "closed", `document.getElementById("rules-dialog") === null`, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebRulesWindow_GeneratedGuideForAClassicGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "test-freeplay", "Alice")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "test-freeplay", "Alice")
	waitJS(t, ctx, "rules button", `document.getElementById("show-rules") !== null`, 15*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("show-rules").click(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "tabs", `document.querySelectorAll('#rules-dialog [role="tab"]').length >= 2`, 10*time.Second)
	// One page of rules (notation included, no table of pieces), then the controls of the window.
	if got := evalString(t, ctx, `[...document.querySelectorAll('#rules-dialog [role="tab"]')].map(t => t.textContent).join(",")`); got != "Rules,Controls" {
		t.Errorf("tabs %q", got)
	}
}

// The markdown renderer's building blocks, exercised in the browser.
func TestWebMarkdown_RendersRichTextSafely(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "page", `document.readyState === "complete"`, 10*time.Second)
	const script = `(async function () {
		const m = await import("/static/js/markdown.js");
		const doc = [
			"# Title",
			"",
			"Some **bold**, *em*, ~~gone~~ and ` + "`code`" + ` with a [link](https://example.com) and [tab](tab:battle#dice).",
			"",
			"| Left | Right | Mid |",
			"|:--|--:|:-:|",
			"| a | 1 | x |",
			"| b \\| c | 2 | y |",
			"",
			"- one",
			"  - nested",
			"- two",
			"",
			"1. first",
			"2. second",
			"",
			"> [!WARNING]",
			"> careful",
			"",
			"::: details More",
			"hidden *text*",
			":::",
			"",
			"![alt|64](pic.png \"Caption\")",
			"",
			"[js](javascript:alert(1)) <script>alert(1)</script> ![x](http://evil/x.png)",
			"",
			"` + "```" + `",
			"raw <b>code</b>",
			"` + "```" + `",
			"",
			"Press [[Enter]].",
		].join("\n");
		const out = m.renderMarkdown(doc, { assetBase: "/a/", onTab: () => {} });
		const host = document.createElement("div");
		host.append(out.node);
		document.body.append(host);
		const q = (s) => host.querySelectorAll(s).length;
		return JSON.stringify({
			h1: q("h1"), strong: q("strong"), em: q("em"), del: q("del"), code: q("code.md-inline-code"),
			links: [...host.querySelectorAll("a")].map(a => a.getAttribute("href") + "|" + (a.getAttribute("target") || "")),
			table: q("table.md-table"), th: q("th"), td: q("td"),
			align: [...host.querySelectorAll("tbody tr:first-child td")].map(td => td.style.textAlign).join(","),
			cell: host.querySelectorAll("tbody tr")[1].children[0].textContent,
			ul: q("ul"), nested: q("ul ul"), ol: q("ol li"),
			callout: q("aside.md-callout.warning"), details: q("details.md-details > summary"),
			img: [...host.querySelectorAll("img")].map(i => i.getAttribute("src") + "|" + i.getAttribute("width") + "|" + i.getAttribute("alt")),
			figure: q("figure figcaption"),
			scripts: q("script"), onclick: q("[onclick]"),
			badlink: q(".md-badlink"), badimage: q(".md-badimage"),
			pre: host.querySelector("pre code").textContent,
			kbd: q("kbd"),
			headings: out.headings.map(h => h.id),
		});
	})()`
	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &raw, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"h1":1`, `"strong":1`, `"em":2`, `"del":1`, `"code":1`,
		`"https://example.com|_blank"`, `"#battle|"`,
		`"table":1`, `"th":3`, `"td":6`, `"align":"left,right,center"`, `"cell":"b | c"`,
		`"nested":1`, `"ol":2`, `"callout":1`, `"details":1`,
		`"/a/pic.png|64|alt"`, `"figure":1`,
		`"scripts":0`, `"onclick":0`, `"badlink":1`, `"badimage":1`,
		`"pre":"raw <b>code</b>"`, `"kbd":1`, `"headings":["title"]`,
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("renderer output lacks %s\n%s", want, raw)
		}
	}
}

// cwScenario builds a Cthulhu Wars position document: units on the map
// (pools are what remains of each cult's box), gates, Power and the dice
// seed. Alice holds seat 1 (Great Cthulhu).
type cwUnit struct {
	Seat int
	Type string
	Area string
	N    int
}

func cwScenario(t *testing.T, units []cwUnit, gates map[string]int, power [5]int, books [5]uint8, rng uint64) string {
	t.Helper()
	colors := [5]string{"", "green", "blue", "red", "yellow"}
	box := map[int]map[string]int{
		1: {"cultist": 6, "deep-one": 4, "shoggoth": 2, "starspawn": 2, "cthulhu": 1},
		2: {"cultist": 6, "nightgaunt": 3, "flying-polyp": 3, "hunting-horror": 2, "nyarlathotep": 1},
		3: {"cultist": 6, "ghoul": 2, "fungi": 4, "dark-young": 3, "shub-niggurath": 1},
		4: {"cultist": 6, "undead": 6, "byakhee": 4, "king-in-yellow": 1, "hastur": 1},
	}
	st := state.NewBoardState()
	for _, u := range units {
		for i := 0; i < u.N; i++ {
			st.Push(board.PositionID(u.Area), state.Occupant{Piece: u.Type, Color: colors[u.Seat]})
		}
		box[u.Seat][u.Type] -= u.N
	}
	for seat := 1; seat <= 4; seat++ {
		for typ, n := range box[seat] {
			st.SetReserve(colors[seat], typ, n)
		}
	}
	var pw [6]int
	var bk [6]uint8
	copy(pw[:], power[:])
	copy(bk[:], books[:])
	d := &cthulhuwars.Data{
		Phase: "action", Round: 1, First: 1, Dir: 1, Power: pw, Books: bk,
		Gates: gates, Bag: [3]int{18, 12, 6}, RNG: rng, Stage: "turn",
		Turn: &cthulhuwars.TurnState{Seat: 1}, Desec: map[string]bool{},
	}
	st.SetExt(d)
	doc, err := st.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return string(doc)
}

func TestWebCthulhuWars_BattleByClickingThroughEveryStep(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwScenario(t,
		[]cwUnit{
			{1, "shoggoth", "asia", 2}, {1, "deep-one", "asia", 2}, {1, "cultist", "south-pacific", 2},
			{2, "flying-polyp", "asia", 2}, {2, "cultist", "asia", 1}, {2, "cultist", "europe", 1},
			{3, "cultist", "south-america-west", 2}, {4, "cultist", "europe", 2},
		},
		map[string]int{"south-pacific": 1, "europe": 4, "south-america-west": 3},
		[5]int{0, 8, 8, 8, 8},
		[5]uint8{0, 1, 0, 0, 0}, // Great Cthulhu has Absorb
		7,
	)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random", "start_state": json.RawMessage(doc), "start_seat": 1}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create from a custom position: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 1000)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "battle button", `document.getElementById("action-battle") !== null`, 15*time.Second)
	click := func(js string) {
		t.Helper()
		if err := chromedp.Run(ctx, chromedp.Evaluate(js, nil)); err != nil {
			t.Fatal(err)
		}
	}
	click(`document.getElementById("action-battle").click(); true`)
	waitJS(t, ctx, "the battle area", `document.querySelectorAll('#board .cell.area.clickable').length === 1`, 10*time.Second)
	clickCell(t, ctx, "asia")
	// Crawling Chaos is the only enemy there: that choice is taken for us.

	// Pre-battle: Absorb is available; feeding a Deep One is chosen by
	// clicking the unit itself.
	waitJS(t, ctx, "pre-battle", `document.getElementById("round-bar") && document.getElementById("battle-panel") !== null`, 10*time.Second)
	waitJS(t, ctx, "absorb glow", `document.querySelector('#board .token[data-position="asia"].token-target[data-piece="deep-one"]') !== null`, 10*time.Second)
	click(`document.querySelector('#board .token.token-target[data-piece="deep-one"]').dispatchEvent(new MouseEvent("click", {bubbles: true})); true`)
	// Absorbing one more or finishing: only deep-one + shoggoth remain; press Done.
	waitJS(t, ctx, "one deep one fewer", `document.querySelector('#board .token[data-position="asia"][data-piece="deep-one"][data-color="green"]').getAttribute("data-count") === "1"`, 10*time.Second)
	// The battle area carries the red outline while the fight lasts.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .area-battle').length`); n < 1 {
		t.Errorf("no battle outline")
	}
	waitJS(t, ctx, "the done button", `document.getElementById("action-done") !== null`, 10*time.Second)
	click(`document.getElementById("action-done").click(); true`)

	// Resolve whatever the dice ask for by clicking glowing tokens, areas or
	// buttons until the turn passes on to the next cult.
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		var step string
		if err := chromedp.Run(ctx, chromedp.Evaluate(autoStepCW, &step)); err != nil {
			t.Fatal(err)
		}
		if step == "done" {
			break
		}
		time.Sleep(120 * time.Millisecond)
	}
	waitJS(t, ctx, "battle panel shows dice", `document.getElementById("battle-panel") !== null && document.querySelectorAll('#battle-panel .die').length >= 1`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#battle-panel h3").textContent`); got == "" {
		t.Error("battle panel has no heading")
	}
	st := cfGet(t, baseURL, created.RoomCode)
	if st.State.Ext.Stage == "pre" {
		t.Errorf("the battle is still in pre-battle")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// openWithBattleShow opens a game with the battle show on: the other tests have it off, as it covers the map.
func openWithBattleShow(t *testing.T, ctx context.Context, baseURL, room string) {
	t.Helper()
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1400, 900),
		chromedp.Navigate(baseURL+"/healthz"),
		chromedp.Evaluate(`localStorage.setItem("bingle.battleAnim", "true"); true`, nil),
	); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "battle button", `document.getElementById("action-battle") !== null`, 15*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("action-battle").click(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the battle area", `document.querySelectorAll('#board .cell.area.clickable').length === 1`, 10*time.Second)
	clickCell(t, ctx, "asia")
}

// A battle zooms in on its area, the two sides stand apart, the dice fly, a red X falls on each unit
// that dies as it is chosen on the stage, the map comes back for the retreat, and the same is shown
// again from the history.
func TestWebCthulhuWars_ABattleIsPlayedOnTheMapAndAgainFromTheHistory(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwScenario(t,
		[]cwUnit{
			{1, "shoggoth", "asia", 2}, {1, "deep-one", "asia", 3}, {1, "cultist", "south-pacific", 2},
			{2, "flying-polyp", "asia", 2}, {2, "hunting-horror", "asia", 1}, {2, "cultist", "asia", 2}, {2, "cultist", "europe", 1},
			{3, "cultist", "south-america-west", 2}, {4, "cultist", "europe", 2},
		},
		map[string]int{"south-pacific": 1, "europe": 4, "south-america-west": 3},
		[5]int{0, 8, 8, 8, 8}, [5]uint8{0, 0, 0, 0, 0}, 7)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openWithBattleShow(t, ctx, baseURL, room)

	waitJS(t, ctx, "the stage", `document.getElementById("battle-stage") !== null`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#battle-stage .bt-title").textContent`); got != "Battle in Asia" {
		t.Errorf("the banner says %q", got)
	}
	// The sides fill in as the stage opens.
	waitJS(t, ctx, "five units a side", `document.querySelectorAll('#battle-stage .bt-side[data-seat="1"] .bt-unit').length === 5 && document.querySelectorAll('#battle-stage .bt-side[data-seat="2"] .bt-unit').length === 5`, 5*time.Second)
	// The map is looked at closely, around the area: its copy, not the map itself.
	waitJS(t, ctx, "the zoom", `(function () {
		var a = document.querySelector("#battle-stage .bt-map").getAttribute("viewBox").split(" ").map(Number);
		var b = document.getElementById("board-svg").getAttribute("viewBox").split(" ").map(Number);
		return a[2] < b[2] * 0.99 && a[0] !== b[0];
	})()`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#battle-stage .bt-map .token[data-color="green"], #battle-stage .bt-map .token[data-position="asia"]').length`); n != 1 {
		t.Errorf("%d of the fighters' tokens still on the copy of the map (South Pacific's Cultists are the 1 that stays)", n)
	}
	if !evalBool(t, ctx, `document.querySelector('#battle-stage .bt-dark') !== null && document.querySelector('#battle-stage mask polygon') !== null`) {
		t.Error("no dark with the shape of the area cut out of it")
	}
	waitJS(t, ctx, "the dice", `document.querySelectorAll('#battle-stage .bt-die.landed').length > 0`, 15*time.Second)
	if !evalBool(t, ctx, `(function () {
		var left = document.querySelector("#battle-stage .bt-side.left").getBoundingClientRect();
		var right = document.querySelector("#battle-stage .bt-side.right").getBoundingClientRect();
		var die = document.querySelector("#battle-stage .bt-die").getBoundingClientRect();
		return left.right <= right.left && die.left >= left.left && die.right <= right.right;
	})()`) {
		t.Error("the sides should stand across from each other with the dice between them")
	}

	// Each casualty is chosen on the stage, until the stage gives way to the map.
	dead := 0
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		var step string
		if err := chromedp.Run(ctx, chromedp.Evaluate(`(function () {
			if (!document.getElementById("battle-stage")) return "gone";
			var unit = document.querySelector("#battle-stage .bt-unit.can-choose");
			if (unit) { unit.click(); return "unit"; }
			return String(document.querySelectorAll("#battle-stage .bt-unit.dead .bt-x").length);
		})()`, &step)); err != nil {
			t.Fatal(err)
		}
		if step == "gone" {
			break
		}
		if n := 0; step != "unit" {
			if _, err := fmt.Sscan(step, &n); err == nil && n > dead {
				dead = n
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	waitJS(t, ctx, "the stage goes", `document.getElementById("battle-stage") === null`, 15*time.Second)
	if dead < 1 {
		t.Error("no unit had a red X")
	}

	// Choosing the battle in the history plays it again, with what became of each unit.
	evalString(t, ctx, `(document.getElementById("rail-tab-history").click(), "ok")`)
	waitJS(t, ctx, "the history", `document.getElementById("history-notation") !== null`, 5*time.Second)
	clickSel(t, ctx, "#history-notation")
	waitJS(t, ctx, "the moves", `document.querySelector("#move-list .mv") !== null`, 5*time.Second)
	// The move that declared the battle (a move is one impulse, ADR 0114).
	battle := evalString(t, ctx, `(function () { var b = [...document.querySelectorAll("#move-list .mv")].find(function (x) { return x.textContent.indexOf("battle") >= 0; }); if (b) b.click(); return b ? b.dataset.seq : "none"; })()`)
	var battleSeq int
	if _, err := fmt.Sscan(battle, &battleSeq); err != nil {
		t.Fatalf("no battle in the move list: %q", battle)
	}
	waitJS(t, ctx, "the battle again", `document.getElementById("battle-stage") !== null`, 15*time.Second)
	waitJS(t, ctx, "the dice again", `document.querySelectorAll('#battle-stage .bt-die.landed').length > 0`, 15*time.Second)
	waitJS(t, ctx, "the Xs again", `document.querySelectorAll("#battle-stage .bt-unit.dead .bt-x").length > 0`, 20*time.Second)
	// A replay offers no choices.
	if n := evalInt(t, ctx, `document.querySelectorAll("#battle-stage .bt-unit.can-choose").length`); n != 0 {
		t.Errorf("%d units to choose in a replay", n)
	}
	// Skip takes it off the map.
	clickSel(t, ctx, "#battle-skip")
	waitJS(t, ctx, "the skip", `document.getElementById("battle-stage") === null`, 5*time.Second)
	// Another move of the same battle stays quiet after a Skip. A move outside the battle and back
	// starts it over, and going elsewhere stops it.
	clickSel(t, ctx, fmt.Sprintf(`#move-list .mv[data-seq="%d"]`, battleSeq+1))
	time.Sleep(400 * time.Millisecond)
	if evalBool(t, ctx, `document.getElementById("battle-stage") !== null`) {
		t.Error("another move of the same battle started it over")
	}
	clickSel(t, ctx, "#hist-first")
	waitJS(t, ctx, "the first position", `document.getElementById("hist-first").disabled`, 10*time.Second)
	time.Sleep(200 * time.Millisecond)
	clickSel(t, ctx, fmt.Sprintf(`#move-list .mv[data-seq="%d"]`, battleSeq))
	waitJS(t, ctx, "the battle a third time", `document.getElementById("battle-stage") !== null`, 15*time.Second)
	// Taken down in the middle of its zoom, the show leaves nothing behind to hold up the next one.
	clickSel(t, ctx, "#hist-first")
	waitJS(t, ctx, "the show stopped", `document.getElementById("battle-stage") === null`, 10*time.Second)
	time.Sleep(200 * time.Millisecond)
	clickSel(t, ctx, fmt.Sprintf(`#move-list .mv[data-seq="%d"]`, battleSeq))
	waitJS(t, ctx, "the battle a fourth time", `document.getElementById("battle-stage") !== null`, 15*time.Second)
	waitJS(t, ctx, "its dice", `document.querySelectorAll('#battle-stage .bt-die.landed').length > 0`, 15*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// The units stand large. Each side's dice pop in before anything else, the Pre-Battle Phase lists its
// abilities and waits on the one the game waits on, the dice fly when it is done, and the Post-Battle
// Phase does the same.
func TestWebCthulhuWars_TheBattleShowListsItsPhasesAndWaitsOnChoices(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwScenario(t,
		[]cwUnit{
			{1, "shoggoth", "asia", 2}, {1, "deep-one", "asia", 2}, {1, "cultist", "south-pacific", 2},
			{2, "flying-polyp", "asia", 2}, {2, "cultist", "asia", 1}, {2, "cultist", "europe", 1},
			{3, "cultist", "south-america-west", 2}, {3, "ghoul", "africa", 1}, {4, "cultist", "europe", 2},
		},
		map[string]int{"south-pacific": 1, "europe": 4, "south-america-west": 3},
		[5]int{0, 8, 8, 8, 8},
		[5]uint8{0, 1 << 0, 0, 1 << 4, 0}, // Great Cthulhu has Absorb, the Black Goat Necrophagy
		7,
	)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openWithBattleShow(t, ctx, baseURL, room)

	waitJS(t, ctx, "the stage", `document.getElementById("battle-stage") !== null`, 10*time.Second)
	// Each pool holds its Combat: 2 Shoggoths and 2 Deep Ones roll 6.
	waitJS(t, ctx, "the pools", `document.querySelectorAll('#battle-stage .bt-dice[data-seat="1"] .bt-die.pooled').length === 6 && document.querySelectorAll('#battle-stage .bt-dice[data-seat="2"] .bt-die.pooled').length > 0`, 15*time.Second)
	if evalInt(t, ctx, `document.querySelectorAll("#battle-stage .bt-cube").length`) != 0 {
		t.Error("a die was thrown before the Pre-Battle Phase")
	}
	if px := evalInt(t, ctx, `Math.round(document.querySelector("#battle-stage .bt-unit").getBoundingClientRect().width)`); px < 80 {
		t.Errorf("the units are %dpx wide, want them large", px)
	}
	waitJS(t, ctx, "the Pre-Battle Phase", `document.querySelector("#battle-phase .bt-phase-title") && document.querySelector("#battle-phase .bt-phase-title").textContent === "Pre-Battle Phase"`, 10*time.Second)
	waitJS(t, ctx, "Absorb, lit", `document.querySelector('#battle-phase .bt-ability.current[data-ability="Absorb"][data-seat="1"] .bt-phase-icon + img') !== null`, 10*time.Second)
	// The show waits on the choice.
	time.Sleep(1500 * time.Millisecond)
	if evalInt(t, ctx, `document.querySelectorAll("#battle-stage .bt-cube").length`) != 0 {
		t.Error("the dice were thrown while a Pre-Battle choice was waiting")
	}
	waitJS(t, ctx, "the done button", `document.getElementById("action-done") !== null`, 10*time.Second)
	clickSel(t, ctx, "#action-done")
	waitJS(t, ctx, "the throw", `document.querySelectorAll("#battle-stage .bt-cube").length > 0`, 15*time.Second)
	if evalBool(t, ctx, `document.querySelector("#battle-phase .bt-phase-title") !== null`) {
		t.Error("the Pre-Battle list should go when the dice are thrown")
	}

	// The casualties, on the stage, until the Post-Battle Phase lists Necrophagy for the Black Goat.
	post := false
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) && !post {
		post = evalBool(t, ctx, `(function () {
			var unit = document.querySelector("#battle-stage .bt-unit.can-choose");
			if (unit) unit.click();
			return document.querySelector('#battle-phase .bt-ability[data-ability="Necrophagy"][data-seat="3"]') !== null
				&& document.querySelector("#battle-phase .bt-phase-title").textContent === "Post-Battle Phase";
		})()`)
		time.Sleep(100 * time.Millisecond)
	}
	if !post {
		t.Error("no Post-Battle Phase listing Necrophagy")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// An upright phone has no map on its page. The replay fills the screen on its side all the same, and
// turning the phone straightens it.
func TestWebCthulhuWars_APhoneTurnsTheBattleShowOnItsSide(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwScenario(t,
		[]cwUnit{
			{1, "deep-one", "asia", 1}, {1, "cultist", "south-pacific", 2},
			{2, "cultist", "asia", 1}, {2, "cultist", "europe", 1},
			{3, "cultist", "south-america-west", 2}, {4, "cultist", "europe", 2},
		},
		map[string]int{"south-pacific": 1, "europe": 4, "south-america-west": 3},
		[5]int{0, 8, 8, 8, 8}, [5]uint8{0, 0, 0, 0, 0}, 7)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openWithBattleShow(t, ctx, baseURL, room)
	waitJS(t, ctx, "the stage", `document.getElementById("battle-stage") !== null`, 10*time.Second)
	waitJS(t, ctx, "the stage goes", `document.getElementById("battle-stage") === null`, 20*time.Second)

	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`localStorage.setItem("bingle.layout", JSON.stringify("phone")); true`, nil),
		chromedp.EmulateViewport(390, 844),
		chromedp.Navigate("about:blank"),
		chromedp.Navigate(baseURL+"/game/"+room),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the Replay button", `document.getElementById("battle-replay") !== null`, 15*time.Second)
	if evalBool(t, ctx, `document.getElementById("board-svg") !== null`) {
		t.Fatal("this is meant to be the upright page, with no map on it")
	}
	clickSel(t, ctx, "#battle-replay")
	waitJS(t, ctx, "the stage", `document.getElementById("battle-stage") !== null`, 10*time.Second)
	covers := `(function () {
		var r = document.getElementById("battle-stage").getBoundingClientRect();
		return Math.round(r.left) === 0 && Math.round(r.top) === 0 && Math.round(r.width) === innerWidth && Math.round(r.height) === innerHeight;
	})()`
	waitJS(t, ctx, "the whole screen", covers, 5*time.Second)
	if !evalBool(t, ctx, `getComputedStyle(document.getElementById("battle-stage")).transform.startsWith("matrix(0, 1, -1, 0")`) {
		t.Errorf("upright, the stage should lie on its side: %s", evalString(t, ctx, `getComputedStyle(document.getElementById("battle-stage")).transform`))
	}
	if evalBool(t, ctx, `document.querySelector("#battle-stage .bt-map") === null`) {
		t.Error("the stage has no map")
	}
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(844, 390)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the stage straightened", `getComputedStyle(document.getElementById("battle-stage")).transform === "none"`, 5*time.Second)
	waitJS(t, ctx, "the whole screen again", covers, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// A battle with no choices in it is over within the move that declares it. It has its show all the
// same, with the units that stood there before.
func TestWebCthulhuWars_ABattleOverInOneMoveStillPlays(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwScenario(t,
		[]cwUnit{
			{1, "deep-one", "asia", 1}, {1, "cultist", "south-pacific", 2},
			{2, "cultist", "asia", 1}, {2, "cultist", "europe", 1},
			{3, "cultist", "south-america-west", 2}, {4, "cultist", "europe", 2},
		},
		map[string]int{"south-pacific": 1, "europe": 4, "south-america-west": 3},
		[5]int{0, 8, 8, 8, 8}, [5]uint8{0, 0, 0, 0, 0}, 7)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openWithBattleShow(t, ctx, baseURL, room)
	waitJS(t, ctx, "the stage", `document.getElementById("battle-stage") !== null && document.querySelector('#battle-stage .bt-side[data-seat="2"] .bt-unit') !== null`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#battle-stage .bt-side[data-seat="1"] .bt-unit').length`); n != 1 {
		t.Errorf("%d units on the attacker's side, want 1", n)
	}
	waitJS(t, ctx, "the stage goes", `document.getElementById("battle-stage") === null`, 20*time.Second)
}

// autoStepCW takes one decision of Alice's pending battle choices: a glowing
// token, then a highlighted area, then a button; "done" once nothing is left.
const autoStepCW = `(function () {
	if (!document.querySelector("#impulse-card.acting")) {
		return document.getElementById("battle-panel") && document.querySelectorAll("#battle-panel .die").length ? "done" : "wait";
	}
	var tok = document.querySelector("#board .token.token-target");
	var area = document.querySelector("#board .cell.area.clickable .cell-hit");
	var btn = document.querySelector("#move-bar .action-buttons button, #move-bar .piece-buttons button");
	function click(el) { el.dispatchEvent(new MouseEvent("click", {bubbles: true, cancelable: true})); }
	if (tok) { click(tok); return "token"; }
	if (area) { click(area); return "area"; }
	var plain = document.getElementById("action-pass");
	if (btn && btn.id !== "action-pass") { click(btn); return "button " + btn.id; }
	if (document.querySelectorAll("#battle-panel .die").length && plain) return "done";
	if (btn) { click(btn); return "button " + btn.id; }
	return "stuck";
})()`

// A cult's card is a line of numbers until it is opened, and one card is open at a time. Open, it
// has headings, and two columns of tiles: a spellbook earned takes the place of a requirement, so
// the list runs from twelve tiles at the start to six.
func TestWebCthulhuWars_TheCultCardsOpenOneAtATime(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 5, 3, 3, 3}, [5]uint8{0, 1 << 1, 63},
		func(d *cthulhuwars.Data) { d.Reqs[1] = 1<<0 | 1<<2; d.Reqs[2] = 63 })
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1600, 1000)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the cards", `document.querySelector('[data-expand="seat-1"]') !== null`, 15*time.Second)
	open := func() int { return evalInt(t, ctx, `document.querySelectorAll(".seat-more:not([hidden])").length`) }
	tiles := func(seat string) int {
		return evalInt(t, ctx, `document.querySelectorAll('#seat-`+seat+` .spell-tile').length`)
	}
	if n := open(); n != 0 {
		t.Errorf("%d cards open to begin with", n)
	}
	if !evalBool(t, ctx, `document.querySelector('#seat-1 [data-counter="power"]').offsetParent !== null`) {
		t.Error("the summary should show the numbers")
	}
	if got := evalString(t, ctx, `document.querySelector('[data-expand="seat-1"]').getAttribute("aria-expanded")`); got != "false" {
		t.Errorf("aria-expanded = %q", got)
	}

	clickSel(t, ctx, `[data-expand="seat-1"]`)
	waitJS(t, ctx, "the first open", `document.querySelector('#seat-1 .spellbook-grid').offsetParent !== null`, 5*time.Second)
	// Sections are told apart by a rule, with no titles.
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-1 .faction-heading').length`); n != 0 {
		t.Errorf("%d section titles", n)
	}
	if got := evalString(t, ctx, `[...document.querySelectorAll('#seat-1 .faction-card')].map(h => h.getAttribute("aria-label")).join()`); !strings.HasPrefix(got, "Faction Ability,Spellbooks") || !strings.HasSuffix(got, "Units") {
		t.Errorf("sections %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-1 details').length`); n != 0 {
		t.Errorf("%d lists to open inside an open card", n)
	}
	if n := tiles("1"); n != 11 { // six slots, one holding the book earned, and five books to come
		t.Errorf("%d tiles with one book earned", n)
	}
	if !evalBool(t, ctx, `getComputedStyle(document.querySelector('#seat-1 .spellbook-grid')).gridTemplateColumns.trim().split(/\s+/).length === 2`) {
		t.Error("the tiles should stand in two columns")
	}
	// A spellbook is its name, with the rule on hover; a requirement says all of it there.
	hoverOver(t, ctx, `#seat-1 .spell-tile.book.held`)
	waitJS(t, ctx, "the rule", `/Devolve/.test((document.querySelector(".hover-card") || {}).textContent || "") && document.querySelector(".hover-card .unit-text").textContent.length > 20`, 5*time.Second)
	hoverOver(t, ctx, `#seat-1 .spell-tile.req:not(.met)`)
	waitJS(t, ctx, "the requirement", `/Spellbook requirement/.test((document.querySelector(".hover-card") || {}).textContent || "")`, 5*time.Second)

	// Opening another closes the first. Six books earned: six tiles, in three rows.
	clickSel(t, ctx, `[data-expand="seat-2"]`)
	waitJS(t, ctx, "the second open", `document.querySelector('#seat-2 .spellbook-grid').offsetParent !== null`, 5*time.Second)
	if n := open(); n != 1 {
		t.Errorf("%d cards open at once", n)
	}
	if n := tiles("2"); n != 6 {
		t.Errorf("%d tiles with all six", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-2 .spell-tile.req, #seat-2 .spell-tile.locked').length`); n != 0 {
		t.Errorf("%d requirements or locked books left with all six", n)
	}
	clickSel(t, ctx, `[data-expand="seat-2"]`)
	waitJS(t, ctx, "all closed", `document.querySelectorAll(".seat-more:not([hidden])").length === 0`, 5*time.Second)

	// A monster's button is its picture and its price, and still says what it does to a screen reader.
	waitJS(t, ctx, "the summon buttons", `document.querySelector(".summon-button") !== null`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector('.summon-button[data-action^="summon-"]').innerText.replace(/\s/g, "")`); got != evalString(t, ctx, `document.querySelector('.summon-button[data-action^="summon-"]').dataset.cost`) {
		t.Errorf("the button says %q", got)
	}
	if got := evalString(t, ctx, `document.querySelector('.summon-button[data-action^="summon-"]').getAttribute("aria-label")`); !strings.HasPrefix(got, "Summon ") {
		t.Errorf("aria-label %q", got)
	}

	// The talk and the history use the height the column has left.
	column := `document.getElementById("play-rail").getBoundingClientRect().bottom`
	clickSel(t, ctx, "#rail-tab-talk")
	waitJS(t, ctx, "the chat", `document.getElementById("chat-card").offsetParent !== null`, 5*time.Second)
	if gap := evalInt(t, ctx, `Math.round(`+column+` - document.getElementById("chat-card").getBoundingClientRect().bottom)`); gap > 60 {
		t.Errorf("%dpx left under the chat", gap)
	}
	clickSel(t, ctx, "#rail-tab-history")
	waitJS(t, ctx, "the history in a tall window", `document.getElementById("history-card").offsetParent !== null`, 5*time.Second)
	if gap := evalInt(t, ctx, `Math.round(`+column+` - document.getElementById("history-card").getBoundingClientRect().bottom)`); gap > 60 {
		t.Errorf("%dpx left under the history", gap)
	}
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1280, 680)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the history tab", `document.getElementById("rail-tab-history") !== null`, 15*time.Second)
	clickSel(t, ctx, "#rail-tab-history")
	waitJS(t, ctx, "the history", `document.getElementById("history-card").offsetParent !== null`, 5*time.Second)
	if gap := evalInt(t, ctx, `Math.round(`+column+` - document.getElementById("history-card").getBoundingClientRect().bottom)`); gap > 60 {
		t.Errorf("%dpx left under the history tab", gap)
	}
}

// The names and glyphs of the map: every name lies in its own area, every glyph inside its border,
// and all of them under the pieces. No stars, no zoom buttons, and a layer of the units to hide.
func TestWebCthulhuWars_TheMapIsAFaintLayerUnderThePieces(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {1, "shoggoth", "asia", 1}, {2, "cultist", "asia", 1}, {2, "flying-polyp", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 5, 3, 3, 3}, [5]uint8{0, 63},
		func(d *cthulhuwars.Data) { d.Reqs[1] = 63 })
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1600, 1000)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the map", `document.querySelector("#board .area-label") !== null`, 15*time.Second)

	// The middle of each is in a shape of its own area, and for a sea, on no land drawn over it.
	strays := evalString(t, ctx, `(function () {
		var svg = document.querySelector("#board svg"), out = [];
		var inShape = function (poly, pt) { var m = poly.getScreenCTM().inverse(), p = svg.createSVGPoint(); p.x = pt.x; p.y = pt.y; return poly.isPointInFill(p.matrixTransform(m)); };
		for (const el of document.querySelectorAll("#board .area-label, #board .area-glyph-badge, #board .area-sigil-badge")) {
			var r = el.getBoundingClientRect(), pt = {x: r.left + r.width / 2, y: r.top + r.height / 2}, id = el.dataset.area;
			var own = [...document.querySelectorAll('#board .cell[data-position="' + id + '"] polygon.area-bg')].some(function (q) { return inShape(q, pt); });
			var drowned = document.querySelector('#board .cell.ocean[data-position="' + id + '"]') && [...document.querySelectorAll("#board .cell.land polygon.area-bg")].some(function (q) { return inShape(q, pt); });
			if (!own || drowned) out.push(el.getAttribute("class") + " of " + id);
		}
		return out.join("; ");
	})()`)
	if strays != "" {
		t.Errorf("out of their areas: %s", strays)
	}
	if !evalBool(t, ctx, `Number(getComputedStyle(document.querySelector("#board .area-label")).opacity) < 1`) {
		t.Error("the names should be faint")
	}
	if !evalBool(t, ctx, `!!(document.querySelector("#board .map-labels").compareDocumentPosition(document.querySelector("#board .map-tokens")) & Node.DOCUMENT_POSITION_FOLLOWING)`) {
		t.Error("the pieces should be drawn over the names")
	}
	if n := evalInt(t, ctx, `document.querySelectorAll(".token-star, .map-zoom").length`); n != 0 {
		t.Errorf("%d stars or zoom buttons on the map", n)
	}
	if !evalBool(t, ctx, `document.querySelector("#board .token-enhanced") !== null`) {
		t.Error("this position should have an enhanced unit")
	}

	// With the units off, only what can be picked up now is drawn. The Gates stay.
	waitJS(t, ctx, "a unit to pick up", `document.querySelector("#board .token.token-target") !== null`, 15*time.Second)
	foes := `document.querySelectorAll('#board .token[data-color="blue"]').length`
	if n := evalInt(t, ctx, foes); n == 0 {
		t.Fatal("no enemy unit to hide")
	}
	evalBool(t, ctx, `(function () { var b = document.getElementById("opt-layer-units"); b.checked = false; b.dispatchEvent(new Event("change")); return true; })()`)
	waitJS(t, ctx, "the enemy hidden", foes+` === 0`, 5*time.Second)
	if !evalBool(t, ctx, `document.querySelector("#board .token.token-target") !== null && document.querySelector("#board .gate-chip") !== null`) {
		t.Error("what can be moved, and the Gates, should stay")
	}
	evalBool(t, ctx, `(function () { var b = document.getElementById("opt-layer-units"); b.checked = true; b.dispatchEvent(new Event("change")); return true; })()`)
	waitJS(t, ctx, "the enemy back", foes+` > 0`, 5*time.Second)
}

// A phone has no hover: there, tapping a unit still opens its card, to read upright.
func TestWebCthulhuWars_APhoneStillHasTheUnitCard(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}, {2, "flying-polyp", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 5, 3, 3, 3}, [5]uint8{},
		func(d *cthulhuwars.Data) {})
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(800, 420), chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	evalBool(t, ctx, `(localStorage.setItem("bingle.layout", JSON.stringify("phone")), true)`)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the phone page", `document.body.classList.contains("m-mode") && document.querySelector('#board .token[data-color="blue"]') !== null`, 15*time.Second)
	clickSVG(t, ctx, `#board .token[data-color="blue"]`)
	// Tapped on the map, and read with the phone upright: the card is among the actions.
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(420, 800)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the card", `document.querySelector("#m-page-actions #unit-info") !== null`, 5*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll(".seat-more[hidden]").length + document.querySelectorAll(".expand-toggle").length`); n != 0 {
		t.Errorf("%d closed cards on a phone", n)
	}
}

// Cthulhu's first awakening is played on the map, by itself, and the history's badge plays it again.
func TestWebCthulhuWars_CthulhuEmerges(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwScenario(t,
		[]cwUnit{
			{1, "cultist", "south-pacific", 2},
			{2, "cultist", "asia", 2}, {3, "cultist", "south-america-west", 2}, {4, "cultist", "europe", 2},
		},
		map[string]int{"south-pacific": 1, "asia": 2, "south-america-west": 3, "europe": 4},
		[5]int{0, 12, 8, 8, 8}, [5]uint8{0, 0, 0, 0, 0}, 7)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1400, 900),
		chromedp.Navigate(baseURL+"/healthz"),
		chromedp.Evaluate(`localStorage.setItem("bingle.battleAnim", "true"); true`, nil),
	); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the awaken button", `document.querySelector('[data-action="awaken-cthulhu"]') !== null`, 15*time.Second)
	clickSel(t, ctx, `[data-action="awaken-cthulhu"]`)
	waitJS(t, ctx, "the area", `document.querySelector('#board .cell.area.clickable[data-position="south-pacific"], #emerge-stage') !== null`, 10*time.Second)
	if !evalBool(t, ctx, `document.getElementById("emerge-stage") !== null`) {
		clickCell(t, ctx, "south-pacific")
	}
	waitJS(t, ctx, "the show", `document.querySelector('#emerge-stage[data-goo="cthulhu"] .em-scene') !== null`, 15*time.Second)
	waitJS(t, ctx, "the title", `document.querySelector("#emerge-title.on") !== null`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#emerge-title .em-name").textContent`); got != "Cthulhu Emerges" {
		t.Errorf("the title says %q", got)
	}
	// His eyes have opened by then, and it takes four to eight seconds in all.
	if !evalBool(t, ctx, `Number(document.querySelector("#emerge-stage .em-figure ellipse").getAttribute("ry")) > 2`) {
		t.Error("his eyes are shut")
	}
	waitJS(t, ctx, "the end of the show", `document.getElementById("emerge-stage") === null`, 10*time.Second)

	// The badge, in the cult's colour, and it plays again and can be skipped.
	evalString(t, ctx, `(document.getElementById("rail-tab-history").click(), "ok")`)
	waitJS(t, ctx, "the badge", `document.querySelector('.log-emerges.playable[data-emerges="cthulhu"]') !== null`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector(".log-emerges.playable").textContent`); !strings.Contains(got, "Cthulhu Emerges") {
		t.Errorf("the badge says %q", got)
	}
	clickSel(t, ctx, ".log-emerges.playable")
	waitJS(t, ctx, "the show again", `document.getElementById("emerge-stage") !== null`, 5*time.Second)
	clickSel(t, ctx, "#emerge-skip")
	waitJS(t, ctx, "the skip", `document.getElementById("emerge-stage") === null`, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// The Animation Tester plays every show with no game going, and is only offered for this game.
func TestWebCthulhuWars_TheAnimationTester(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 1000), chromedp.Navigate(baseURL+"/?game=tic-tac-toe")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "another game's setup", `document.getElementById("setup-tab-tester") !== null && document.getElementById("create-submit") !== null`, 20*time.Second)
	if !evalBool(t, ctx, `document.getElementById("setup-tab-tester").hidden`) {
		t.Error("another game offers the tester")
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=cthulhu-wars")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the tab", `!document.getElementById("setup-tab-tester").hidden`, 20*time.Second)
	clickSel(t, ctx, "#setup-tab-tester")
	waitJS(t, ctx, "the board", `document.querySelector('#tester-board .cell.area[data-position="asia"]') !== null`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#tester-board .token[data-position="asia"]').length`); n == 0 {
		t.Error("no units on the board")
	}

	// Every Great Old One with a show, and each is only a click away. No game is started.
	for _, goo := range []string{"cthulhu", "nyarlathotep", "shub-niggurath", "king-in-yellow", "hastur", "tsathoggua", "ithaqua", "rhan-tegoth", "yog-sothoth", "ubbo-sathla", "bastet", "cthugha", "abhoth"} {
		clickSel(t, ctx, `[data-tester-emerge="`+goo+`"]`)
		waitJS(t, ctx, goo+"'s show", `document.querySelector('#emerge-stage[data-goo="`+goo+`"] .em-scene') !== null`, 5*time.Second)
		waitJS(t, ctx, goo+"'s zoom", `(function () {
			var map = document.querySelector("#emerge-stage .bt-map");
			var base = document.querySelector("#tester-board svg").getAttribute("viewBox");
			return map.querySelector(".bt-glow") !== null && map.getAttribute("viewBox") !== base;
		})()`, 5*time.Second)
		clickSel(t, ctx, "#emerge-skip")
		waitJS(t, ctx, goo+"'s skip", `document.getElementById("emerge-stage") === null`, 5*time.Second)
	}
	// One played right through, with its title.
	clickSel(t, ctx, `[data-tester-emerge="king-in-yellow"]`)
	waitJS(t, ctx, "the title", `document.querySelector("#emerge-title.on .em-name") !== null && document.querySelector("#emerge-title.on .em-name").textContent === "King in Yellow Emerges"`, 10*time.Second)
	waitJS(t, ctx, "its end", `document.getElementById("emerge-stage") === null`, 10*time.Second)

	// The battle, through both of its phases, to the zoom out.
	clickSel(t, ctx, "#tester-battle")
	waitJS(t, ctx, "the stage", `document.getElementById("battle-stage") !== null`, 5*time.Second)
	waitJS(t, ctx, "Pre-Battle", `document.querySelector("#battle-phase .bt-phase-title") && document.querySelector("#battle-phase .bt-phase-title").textContent === "Pre-Battle Phase"`, 15*time.Second)
	waitJS(t, ctx, "the abilities", `document.querySelectorAll('#battle-phase .bt-ability').length >= 4`, 15*time.Second)
	waitJS(t, ctx, "the throw", `document.querySelectorAll("#battle-stage .bt-cube").length > 0`, 25*time.Second)
	// Each side's dice are in its own colour, and the marks are on them.
	if !evalBool(t, ctx, `(function () {
		var root = getComputedStyle(document.documentElement);
		var die = function (seat) { return getComputedStyle(document.querySelector('#battle-stage .bt-dice[data-seat="' + seat + '"] .bt-face')).getPropertyValue("--bt-die").trim(); };
		return die(1) === root.getPropertyValue("--piece-green-fill").trim() && die(2) === root.getPropertyValue("--piece-blue-fill").trim();
	})()`) {
		t.Error("the dice are not in their cults' colours")
	}
	if !evalBool(t, ctx, `document.querySelector('#battle-stage .bt-die .bt-face.kill .bt-mark.kill .bt-skull') !== null && document.querySelector('#battle-stage .bt-die .bt-face.pain .bt-mark.pain .bt-burst') !== null && document.querySelector('#battle-stage .bt-die .bt-face.miss .bt-mark') === null`) {
		t.Error("the faces of the dice do not carry their marks")
	}
	waitJS(t, ctx, "a casualty", `document.querySelectorAll("#battle-stage .bt-unit.dead .bt-x").length > 0`, 15*time.Second)
	waitJS(t, ctx, "Post-Battle", `document.querySelector("#battle-phase .bt-phase-title") && document.querySelector("#battle-phase .bt-phase-title").textContent === "Post-Battle Phase"`, 25*time.Second)
	waitJS(t, ctx, "its end", `document.getElementById("battle-stage") === null`, 25*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// The King in Yellow's awakening earns a spellbook in the same move, so the move
// ends with the book: the show still finds where he awoke, live and again from
// the history's badge.
func TestWebCthulhuWars_TheKingEmergesWithABookInTheSameMove(t *testing.T) {
	baseURL := newRESTTestServer(t)
	raw := cwScenario(t,
		[]cwUnit{
			{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 2},
			{3, "cultist", "south-america-west", 2}, {4, "cultist", "africa", 2},
		},
		map[string]int{"south-pacific": 1, "asia": 2, "south-america-west": 3, "europe": 4},
		[5]int{0, 8, 8, 8, 12}, [5]uint8{0, 0, 0, 0, 0}, 7)
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	ext := doc["ext"].(map[string]any)
	ext["first"], ext["turn"] = 4, map[string]any{"seat": 4}
	edited, _ := json.Marshal(doc)
	body := map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat": 4, "opponent": "random", "start_state": json.RawMessage(edited), "start_seat": 4}
	var created map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d %v", code, created)
	}
	room, _ := created["room_code"].(string)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1400, 900),
		chromedp.Navigate(baseURL+"/healthz"),
		chromedp.Evaluate(`localStorage.setItem("bingle.battleAnim", "true"); true`, nil),
	); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the awaken button", `document.querySelector('[data-action^="awaken-king"]') !== null`, 15*time.Second)
	clickSel(t, ctx, `[data-action^="awaken-king"]`)
	waitJS(t, ctx, "the area", `document.querySelector('#board .cell.area.clickable[data-position="africa"], #emerge-stage') !== null`, 10*time.Second)
	if !evalBool(t, ctx, `document.getElementById("emerge-stage") !== null`) {
		clickCell(t, ctx, "africa")
	}
	waitJS(t, ctx, "the book", `document.querySelector('[data-action^="book"]') !== null || document.getElementById("emerge-stage") !== null`, 15*time.Second)
	if evalBool(t, ctx, `document.getElementById("emerge-stage") !== null`) {
		clickSel(t, ctx, "#emerge-skip")
		waitJS(t, ctx, "the skip", `document.getElementById("emerge-stage") === null`, 5*time.Second)
	}
	waitJS(t, ctx, "the book", `document.querySelector('[data-action^="book"]') !== null`, 15*time.Second)
	clickSel(t, ctx, `[data-action^="book"]`)
	if evalBool(t, ctx, `!!document.querySelector("#submit-move:not([disabled])")`) {
		clickSel(t, ctx, "#submit-move")
	}
	waitJS(t, ctx, "the move sent", `!document.querySelector('[data-action^="book"]')`, 15*time.Second)
	if evalBool(t, ctx, `document.getElementById("emerge-stage") !== null`) {
		clickSel(t, ctx, "#emerge-skip")
	}
	waitJS(t, ctx, "no show", `document.getElementById("emerge-stage") === null`, 10*time.Second)

	evalString(t, ctx, `(document.getElementById("rail-tab-history").click(), "ok")`)
	waitJS(t, ctx, "the badge", `document.querySelector('.log-emerges.playable[data-emerges="king-in-yellow"]') !== null`, 15*time.Second)
	clickSel(t, ctx, `.log-emerges.playable[data-emerges="king-in-yellow"]`)
	waitJS(t, ctx, "the show again", `document.querySelector('#emerge-stage[data-goo="king-in-yellow"]') !== null`, 5*time.Second)
}
