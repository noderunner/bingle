package app_test

import (
	"context"
	"encoding/json"
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
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-1 .seat-stats .score.stat svg.stat-icon').length`); n != 4 {
		t.Errorf("%d pictograms in the header, want power, doom, signs and gates", n)
	}
	// The sections: the faction ability, spellbooks, requirements, Elder Signs and units.
	if got := evalString(t, ctx, `[...document.querySelectorAll('#seat-1 .faction-card')].map(c => c.getAttribute("data-card")).join(",")`); got != "ability,books,reqs,signs,units" {
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
	// The Spellbooks counter is gone from the header: the section has it.
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-1 [data-counter="books"]').length`); n != 0 {
		t.Errorf("the spellbook summary is still in the header")
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
	waitJS(t, ctx, "tabs", `document.querySelectorAll('#rules-dialog [role="tab"]').length === 12`, 10*time.Second)
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
	room := newTakRoom(t, baseURL, "demo-freeplay", "Alice")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "demo-freeplay", "Alice")
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

// autoStepCW takes one decision of Alice's pending battle choices: a glowing
// token, then a highlighted area, then a button; "done" once nothing is left.
const autoStepCW = `(function () {
	var bar = document.querySelector(".status-bar");
	if (!bar || !bar.classList.contains("your-turn")) {
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
