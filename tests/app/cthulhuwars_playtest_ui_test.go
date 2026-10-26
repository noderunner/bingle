package app_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// cwPosition is cwScenario with a hook to change the position's data before
// it is written (stage, Elder Signs, the bag...). Alice holds seat 1.
func cwPosition(t *testing.T, units []cwUnit, gates map[string]int, power [5]int, books [5]uint8, edit func(d *cthulhuwars.Data)) string {
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
		Gates: gates, Bag: [3]int{18, 12, 6}, RNG: 99, Stage: "turn",
		Turn: &cthulhuwars.TurnState{Seat: 1}, Desec: map[string]bool{},
	}
	if edit != nil {
		edit(d)
	}
	// Units held by another cult are in neither the pool nor on the map.
	for by := 1; by <= 4; by++ {
		for owner := 1; owner <= 4; owner++ {
			if n := d.Captured[by][owner]; n > 0 {
				st.SetReserve(colors[owner], "cultist", st.Reserve(colors[owner], "cultist")-n)
			}
		}
	}
	st.SetExt(d)
	doc, err := st.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return string(doc)
}

func cwGameFrom(t *testing.T, baseURL, doc string) string {
	t.Helper()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random", "start_state": json.RawMessage(doc), "start_seat": 1}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create from a custom position: %d", code)
	}
	return created.RoomCode
}

func TestWebCthulhuWars_AnyUnitOpensItsCard(t *testing.T) {
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
	waitJS(t, ctx, "tokens", `document.querySelectorAll('#board .token[data-piece="cultist"]').length === 4`, 15*time.Second)
	click := func(sel string) {
		t.Helper()
		js := `(function(){ var e = document.querySelector(` + jsString(sel) + `); if (!e) return false; e.dispatchEvent(new MouseEvent("click", {bubbles: true})); return true; })()`
		if err := chromedp.Run(ctx, chromedp.Evaluate(js, nil)); err != nil {
			t.Fatal(err)
		}
	}

	// An enemy Cultist is not a legal choice: the click only opens its card.
	click(`#board .token[data-piece="cultist"][data-color="blue"]`)
	waitJS(t, ctx, "unit card", `document.getElementById("unit-info") !== null`, 5*time.Second)
	card := evalString(t, ctx, `document.getElementById("unit-info").textContent`)
	for _, want := range []string{"Acolyte Cultist", "Crawling Chaos", "Pool", "Combat"} {
		if !strings.Contains(card, want) {
			t.Errorf("the card lacks %q: %q", want, card)
		}
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .token.token-inspected').length`); n != 1 {
		t.Errorf("%d inspected tokens", n)
	}
	// No move was started by it.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .cell.area.clickable').length`); n != 0 {
		t.Errorf("%d clickable areas after looking at a foe", n)
	}

	// A second click closes it; Esc does too.
	click(`#board .token[data-piece="cultist"][data-color="blue"]`)
	waitJS(t, ctx, "card closed", `document.getElementById("unit-info") === null`, 5*time.Second)
	click(`#board .token[data-piece="cultist"][data-color="red"]`)
	waitJS(t, ctx, "another card", `document.getElementById("unit-info") !== null && document.getElementById("unit-info").textContent.indexOf("Black Goat") >= 0`, 5*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.dispatchEvent(new KeyboardEvent("keydown", {key: "Escape", bubbles: true})); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "closed by Esc", `document.getElementById("unit-info") === null`, 5*time.Second)

	// One's own glowing unit is picked up — and its card opens as well.
	click(`#board .token.token-target[data-piece="cultist"]`)
	waitJS(t, ctx, "own card", `document.getElementById("unit-info") !== null && document.getElementById("unit-info").textContent.indexOf("Great Cthulhu") >= 0`, 5*time.Second)
	// Its six neighbours: it stands beside the Gate, not on it.
	waitJS(t, ctx, "destinations", `document.querySelectorAll('#board .cell.area.clickable').length === 6`, 10*time.Second)
	// Close with the × button.
	click(`#unit-info-close`)
	waitJS(t, ctx, "closed by the button", `document.getElementById("unit-info") === null`, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestWebCthulhuWars_TheDirectionIsChosenBetweenSpelledOutOrders(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 3}, {2, "cultist", "asia", 2}, {3, "cultist", "south-america-west", 2}, {4, "cultist", "europe", 2}},
		map[string]int{"south-pacific": 1, "asia": 2, "south-america-west": 3, "europe": 4},
		[5]int{0, 9, 4, 3, 3}, [5]uint8{},
		func(d *cthulhuwars.Data) {
			d.Stage = "direction"
			d.Turn = nil
			d.First = 1
		})
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")

	// The round bar shows the phases in order and where the round stands.
	waitJS(t, ctx, "phase bar", `document.querySelectorAll('#phase-steps .phase-step').length === 4`, 15*time.Second)
	if got := evalString(t, ctx, `[...document.querySelectorAll('#phase-steps .phase-step')].map(li => li.getAttribute("data-phase") + "=" + li.getAttribute("data-state")).join(" ")`); got != "action=done gather=done first=current doom=todo" {
		t.Errorf("phases: %s", got)
	}
	if got := evalString(t, ctx, `[...document.querySelectorAll('#phase-steps .phase-step')].map(li => li.textContent.replace("✓", "").trim()).join(" > ")`); got != "Action > Gather Power > First Player > Doom" {
		t.Errorf("phase labels: %s", got)
	}

	// Two buttons, each the order of play it makes.
	waitJS(t, ctx, "direction buttons", `document.querySelectorAll('.direction-button').length === 2`, 10*time.Second)
	seats := func(id string) string {
		return evalString(t, ctx, `[...document.querySelectorAll('#action-`+id+` .order-seat')].map(s => s.textContent.trim()).join(" > ")`)
	}
	if got := seats("cw"); got != "Great Cthulhu > Crawling Chaos > Black Goat > Yellow Sign" {
		t.Errorf("clockwise: %s", got)
	}
	if got := seats("ccw"); got != "Great Cthulhu > Yellow Sign > Black Goat > Crawling Chaos" {
		t.Errorf("counter-clockwise: %s", got)
	}
	if got := evalString(t, ctx, `document.getElementById("action-cw").textContent`); !strings.Contains(got, "Clockwise") {
		t.Errorf("button text %q", got)
	}
	// The cults are listed clockwise: down the list from the viewer.
	if got := evalString(t, ctx, `[...document.querySelectorAll('#cult-stack .seat-strip')].map(s => s.getAttribute("data-seat")).join(",")`); got != "1,2,3,4" {
		t.Errorf("cult list order %s", got)
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("action-ccw").click(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "doom phase", `document.querySelector('#phase-steps .phase-step[data-phase="doom"]').getAttribute("data-state") === "current"`, 15*time.Second)
	if got := evalString(t, ctx, `document.getElementById("turn-order").textContent`); !strings.Contains(got, "Counter-clockwise") {
		t.Errorf("the order bar says %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_OwnElderSignsAreVisibleToTheirOwnerOnly(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 3}, {1, "deep-one", "south-pacific", 1}, {2, "cultist", "asia", 2}, {3, "cultist", "south-america-west", 2}, {4, "cultist", "europe", 2}},
		map[string]int{"south-pacific": 1, "asia": 2, "south-america-west": 3, "europe": 4},
		[5]int{0, 6, 6, 6, 6},
		[5]uint8{0, 1 << 1}, // Great Cthulhu has Devolve: it enhances Cultists and Deep Ones
		func(d *cthulhuwars.Data) {
			d.Signs[1] = []int{3, 1}
			d.Signs[2] = []int{2}
			d.Shown[1] = []int{2}
			d.Bag = [3]int{15, 11, 5}
		})
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")

	waitJS(t, ctx, "sign rows", `document.getElementById("signs-1") !== null && document.getElementById("signs-2") !== null`, 15*time.Second)
	chips := func(sel string) string {
		return evalString(t, ctx, `[...document.querySelectorAll('`+sel+`')].map(c => c.textContent.trim()).join(",")`)
	}
	if got := chips(`#signs-1 .sign-chip.facedown.own`); got != "3,1" {
		t.Errorf("own hidden signs: %q", got)
	}
	if got := chips(`#signs-1 .sign-chip.shown`); got != "2" {
		t.Errorf("own revealed signs: %q", got)
	}
	if got := chips(`#signs-2 .sign-chip.facedown`); got != "?" {
		t.Errorf("another cult's hidden signs must not show their value: %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#signs-2 .sign-chip.own').length`); n != 0 {
		t.Errorf("%d own chips on another cult", n)
	}
	// The bag count: 15+11+5 = 31 left.
	if got := evalString(t, ctx, `document.getElementById("elder-bag").textContent`); !strings.Contains(got, "31") {
		t.Errorf("bag: %q", got)
	}
	// The reveal button says what it is worth.
	if got := evalString(t, ctx, `document.getElementById("action-reveal") ? document.getElementById("action-reveal").textContent : ""`); !strings.Contains(got, "+4 Doom") {
		t.Errorf("reveal button: %q", got)
	}
	// Devolve's units wear the ring; nobody else's do.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .token.token-enhanced[data-color="green"]').length`); n != 2 {
		t.Errorf("%d enhanced green tokens, want the Cultists and the Deep One", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .token.token-enhanced:not([data-color="green"])').length`); n != 0 {
		t.Errorf("%d enhanced tokens of other cults", n)
	}
	// Revealing makes them public and scores them.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("action-reveal").click(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "revealed", `document.querySelectorAll('#signs-1 .sign-chip.shown').length === 3 && document.querySelectorAll('#signs-1 .sign-chip.facedown').length === 0`, 15*time.Second)
	if got := evalString(t, ctx, `document.querySelector('#seat-1 [data-counter="doom"]').textContent`); got != "4" {
		t.Errorf("doom after revealing: %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_TheLogShowsWhatOthersDidInTheirColours(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	for _, n := range []string{"cw", "go:cultist@south-pacific>new-zealand", "done"} {
		if code, out := moveNotation(t, baseURL, room, "Alice", n); code != 200 {
			t.Fatalf("%s: %d %v", n, code, out)
		}
	}
	// The three bots now play; wait until Alice is up again.
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		var st struct {
			Status   string `json:"status"`
			TurnSeat *int   `json:"turn_seat"`
		}
		doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
		if st.TurnSeat != nil && *st.TurnSeat == 1 && st.Status == "active" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "log card", `document.getElementById("history-card") !== null && document.getElementById("events") !== null`, 15*time.Second)
	// One card, two tabs: Moves and History (the second is open).
	if got := evalString(t, ctx, `document.getElementById("tab-history").textContent`); got != "History" {
		t.Errorf("history tab %q", got)
	}
	if got := evalString(t, ctx, `[...document.querySelectorAll("#history-card .card-tab")].map(b => b.id).join(",")`); got != "tab-moves,tab-history" {
		t.Errorf("tabs %q", got)
	}
	// Every other cult that acted has its own header in its colour...
	if n := evalInt(t, ctx, `document.querySelectorAll('#events .log-actor').length`); n < 3 {
		t.Errorf("%d turns in the log, want the three bots'", n)
	}
	if n := evalInt(t, ctx, `new Set([...document.querySelectorAll('#events .log-actor')].map(a => a.getAttribute("data-seat"))).size`); n < 3 {
		t.Errorf("%d distinct players in the log", n)
	}
	// ... the cult names are coloured and the verbs bold.
	if n := evalInt(t, ctx, `document.querySelectorAll('#events .log-fac').length`); n < 3 {
		t.Errorf("%d coloured names", n)
	}
	if n := evalInt(t, ctx, `new Set([...document.querySelectorAll('#events .log-fac')].map(a => [...a.classList].find(c => c.startsWith("color-")))).size`); n < 2 {
		t.Errorf("the log has %d colours", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#events .log-verb').length`); n < 3 {
		t.Errorf("%d bold verbs", n)
	}
	if got := evalString(t, ctx, `getComputedStyle(document.querySelector('#events .log-verb')).fontWeight`); got != "800" && got != "700" && got != "bold" {
		t.Errorf("verb weight %q", got)
	}
	// It is what the others did: Alice's own turn is over and not repeated.
	if got := evalString(t, ctx, `[...document.querySelectorAll('#events .log-actor')].map(a => a.getAttribute("data-seat")).join(",")`); strings.HasPrefix(got, "1,") && strings.Count(got, "1") > 1 {
		t.Errorf("seats in the log: %s", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
