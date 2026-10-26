package app_test

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The thirteenth round: the packs of Loyalty Cards on the setup page, the real
// glyph of every faction on the map and in the rules, the icons of the new
// units and the prompts that come with them.

func TestWebHome_LoyaltyCardPacksAreChosenOnTheSetupPage(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=cthulhu-wars")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "four seats", `document.querySelectorAll("#slots-field select.faction-select").length === 4`, 20*time.Second)
	for _, id := range []string{"azathoth", "dreamlands", "beyond", "igoo-abhoth", "igoo-chaugnar", "igoo-cthugha", "igoo-hydra", "igoo-yig"} {
		if n := evalInt(t, ctx, `document.querySelectorAll("#option-`+id+`").length`); n != 1 {
			t.Errorf("%d check boxes #option-%s", n, id)
		}
		if got := evalString(t, ctx, `String(document.getElementById("option-`+id+`").checked)`); got != "false" {
			t.Errorf("#option-%s is checked from the start", id)
		}
	}
	for _, id := range []string{"dreamlands", "beyond", "igoo-yig", "igoo-abhoth"} {
		evalString(t, ctx, `document.getElementById("option-`+id+`").click(); "ok"`)
	}
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
	if want := "cthulhu-wars~gc-cc-bg-ys-dl-bt-g1ay"; info.GameDefID != want {
		t.Fatalf("game = %q, want %q", info.GameDefID, want)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// cardGame opens a game on a table with every pack, from a position the edit
// function shapes; Alice holds seat 1 (Great Cthulhu).
func cardGame(t *testing.T, baseURL string, edit func(d *cthulhuwars.Data, st *state.BoardState)) (string, string) {
	t.Helper()
	colors := [5]string{"", "green", "blue", "red", "yellow"}
	st := state.NewBoardState()
	box := map[int]map[string]int{
		1: {"cultist": 6, "deep-one": 4, "shoggoth": 2, "starspawn": 2, "cthulhu": 1},
		2: {"cultist": 6, "nightgaunt": 3, "flying-polyp": 3, "hunting-horror": 2, "nyarlathotep": 1},
		3: {"cultist": 6, "ghoul": 2, "fungi": 4, "dark-young": 3, "shub-niggurath": 1},
		4: {"cultist": 6, "undead": 6, "byakhee": 4, "king-in-yellow": 1, "hastur": 1},
	}
	place := func(seat int, typ, area string, n int) {
		for i := 0; i < n; i++ {
			st.Push(board.PositionID(area), state.Occupant{Piece: typ, Color: colors[seat]})
		}
		box[seat][typ] -= n
	}
	d := &cthulhuwars.Data{
		Phase: "action", Round: 1, First: 1, Dir: 1, Bag: [3]int{18, 12, 6}, RNG: 99, Stage: "turn",
		Turn: &cthulhuwars.TurnState{Seat: 1}, Desec: map[string]bool{}, Gates: map[string]int{"south-pacific": 1, "asia": 2},
		Players: 4,
	}
	d.Power = [6]int{0, 8, 8, 8, 8}
	place(1, "cthulhu", "south-pacific", 1)
	place(1, "cultist", "south-pacific", 1)
	place(2, "cultist", "asia", 2)
	place(2, "nyarlathotep", "asia", 1)
	place(3, "cultist", "africa", 1)
	pool := map[string]map[string]int{}
	edit(d, st)
	_ = pool
	for seat := 1; seat <= 4; seat++ {
		for typ, n := range box[seat] {
			st.SetReserve(colors[seat], typ, n)
		}
	}
	st.SetExt(d)
	doc, _ := st.MarshalJSON()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	id := "cthulhu-wars~gc-cc-bg-ys-az-dl-bt-g1acthy"
	body := map[string]any{"game_def_id": id, "player_name": "Alice", "opponent": "random", "start_state": json.RawMessage(doc), "start_seat": 1}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	return created.RoomCode, id
}

func TestWebCthulhuWars_TheGlyphsOfTheCultsAreOnTheMapAndTheNewUnitsHaveIcons(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room, id := cardGame(t, baseURL, func(d *cthulhuwars.Data, st *state.BoardState) {
		for i, typ := range []string{"moonbeast", "gnorri", "shantak", "wamp", "voonith", "hound-of-tindalos", "abhoth", "chaugnar-faugn", "cthugha", "mother-hydra", "yig", "filth"} {
			area := []string{"africa", "europe", "australia", "new-zealand", "antarctica", "south-pacific", "north-america-west", "north-america-east", "central-america", "south-america-east", "indian-ocean", "arctic-ocean"}[i]
			st.Push(board.PositionID(area), state.Occupant{Piece: typ, Color: "green"})
		}
		for i := 4; i <= 14; i++ {
			d.Loyal[1] |= 1 << uint(i)
		}
		st.SetReserve("green", "moonbeast", 3)
		st.SetReserve("green", "gnorri", 2)
		st.SetReserve("green", "shantak", 1)
		st.SetReserve("green", "wamp", 3)
		st.SetReserve("green", "voonith", 1)
		st.SetReserve("green", "filth", 11)
	})
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, id, "Alice")
	waitJS(t, ctx, "the new units", `document.querySelectorAll('#board .token[data-piece="abhoth"]').length === 1`, 20*time.Second)
	// Every new unit is drawn with an icon of its own, not a letter in a disc.
	for _, typ := range []string{"moonbeast", "gnorri", "shantak", "wamp", "voonith", "hound-of-tindalos", "abhoth", "chaugnar-faugn", "cthugha", "mother-hydra", "yig", "filth"} {
		if n := evalInt(t, ctx, `document.querySelectorAll('#board .token[data-piece="`+typ+`"] .unit-icon path.piece-body').length`); n < 1 {
			t.Errorf("%s has no icon on the map", typ)
		}
	}
	// A glyph where each cult begins, in its own colours.
	for _, key := range []string{"gc", "cc", "bg", "ys"} {
		if n := evalInt(t, ctx, `document.querySelectorAll('#board .area-sigil-badge[data-sigil="`+key+`"] path.area-sigil').length`); n != 1 {
			t.Errorf("%d glyphs of %s on the map, want 1", n, key)
		}
	}
	if got := evalString(t, ctx, `document.querySelector('#board .area-sigil-badge[data-sigil="gc"]').dataset.area`); got != "south-pacific" {
		t.Errorf("Great Cthulhu's glyph is in %q", got)
	}
	if got := evalString(t, ctx, `document.querySelector('#board .area-sigil-badge[data-sigil="ys"] path.area-sigil').getAttribute("fill")`); got == "" {
		t.Error("the glyph has no colour")
	}
	// The cards: Great Cthulhu holds eleven Loyalty Cards.
	waitJS(t, ctx, "the loyalty card", `document.querySelector('[data-card="loyalty"]') !== null`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector('[data-card="loyalty"]').textContent`); !strings.Contains(got, "Abhoth") || !strings.Contains(got, "Hound of Tindalos") {
		t.Errorf("the Loyalty Cards panel: %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_TheRulesShowTheGlyphsAndExplainTheLoyaltyCards(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room, id := cardGame(t, baseURL, func(d *cthulhuwars.Data, st *state.BoardState) {})
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, id, "Alice")
	waitJS(t, ctx, "rules button", `document.getElementById("show-rules") !== null`, 15*time.Second)
	clickSel(t, ctx, "#show-rules")
	waitJS(t, ctx, "tabs", `document.querySelectorAll('#rules-dialog [role="tab"]').length >= 12`, 10*time.Second)
	// The Factions page: a glyph for each of the eight cults, and the placeholders.
	clickSel(t, ctx, `#rules-dialog [role="tab"][data-tab="factions"]`)
	waitJS(t, ctx, "the glyph images", `[...document.querySelectorAll('#rules-panel-factions img.md-img')].filter(i => /glyphs\/[a-z]+\.svg/.test(i.src)).length === 13`, 15*time.Second)
	if n := evalInt(t, ctx, `[...document.querySelectorAll('#rules-panel-factions img.md-img')].filter(i => /glyphs\/(gc|cc|bg|ys|ww|sl|ow|tt)\.svg/.test(i.src)).length`); n != 8 {
		t.Errorf("%d glyphs of the cults, want 8", n)
	}
	// Each file is served, as an SVG that draws something.
	for _, key := range []string{"gc", "cc", "bg", "ys", "ww", "sl", "ow", "tt", "az", "an", "bu", "ds", "in"} {
		resp, err := http.Get(baseURL + "/api/game-defs/cthulhu-wars/assets/guide/glyphs/" + key + ".svg")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "svg") || !strings.Contains(string(body), "<path") {
			t.Errorf("glyph %s: status %d, type %q", key, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
	// The Neutral and Independents pages carry the official rules and the strategies.
	clickSel(t, ctx, `#rules-dialog [role="tab"][data-tab="neutral"]`)
	waitJS(t, ctx, "the neutral page", `document.getElementById("rules-panel-neutral") && !document.getElementById("rules-panel-neutral").hidden`, 5*time.Second)
	page := evalString(t, ctx, `document.getElementById("rules-panel-neutral").textContent`)
	for _, want := range []string{
		"Moonbeast", "Gnorri", "Shantak", "Wamp", "Voonith", "Hound of Tindalos", "Blasphemous Obeisance", "Crypt Dweller",
		"Angles of Time", "pay 2 Power and 2 Doom", "never purchase more than one", "Nuclear Chaos", "Moonbeasts' ability",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the Neutral page lacks %q", want)
		}
	}
	clickSel(t, ctx, `#rules-dialog [role="tab"][data-tab="independents"]`)
	waitJS(t, ctx, "the independents page", `document.getElementById("rules-panel-independents") && !document.getElementById("rules-panel-independents").hidden`, 5*time.Second)
	page = evalString(t, ctx, `document.getElementById("rules-panel-independents").textContent`)
	for _, want := range []string{
		"Abhoth", "Chaugnar Faugn", "Cthugha", "Mother Hydra", "Yig", "falls off", "do NOT gain an Elder Sign", "one fewer Independent than the number of players",
		"Strategy (from the wiki)", "Neutral spellbook combo with The Mao Ceremony", "Fire Vampires", "Miri Nigri", "Snakebite",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the Independents page lacks %q", want)
		}
	}
	// The Key shows a faction glyph.
	clickSel(t, ctx, `#rules-dialog [role="tab"][data-tab="key"]`)
	waitJS(t, ctx, "the key", `document.querySelector('#rules-panel-key .area-sigil-badge path.area-sigil') !== null`, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_TheMoonbeastAsksWhichSpellbookItLiesOn(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room, id := cardGame(t, baseURL, func(d *cthulhuwars.Data, st *state.BoardState) {
		d.Loyal[1] |= 1 << 4
		d.Books[2], d.Books[3] = 0b101, 0b11
		d.Stage, d.Resume = "prompt", "turn"
		d.Prompts = []cthulhuwars.Prompt{{Kind: "moon", Seat: 1}}
		st.SetReserve("green", "moonbeast", 4)
	})
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, id, "Alice")
	waitJS(t, ctx, "the choices", `[...document.querySelectorAll('#move-bar button, #moves button, button')].some(b => /Lay it on Black Goat's Frenzy/.test(b.textContent))`, 20*time.Second)
	if got := evalString(t, ctx, `document.body.innerText`); !strings.Contains(got, "Blasphemous Obeisance") {
		t.Errorf("the prompt does not say what happens: %.300s", got)
	}
	clickButton := `[...document.querySelectorAll('button')].find(b => /Lay it on Black Goat's Frenzy/.test(b.textContent)).click(); "ok"`
	evalString(t, ctx, clickButton)
	waitJS(t, ctx, "the book is marked", `[...document.querySelectorAll('[data-card="books"]')].some(c => /a Moonbeast lies on it/.test(c.textContent))`, 20*time.Second)
}

func TestWebCthulhuWars_CthughaIsAwakenedByChoosingTheGreatOldOneHeReplaces(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room, id := cardGame(t, baseURL, func(d *cthulhuwars.Data, st *state.BoardState) {
		d.Awoken = map[string]bool{"1|0": true}
		st.SetReserve("green", "cthugha", 1)
	})
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, id, "Alice")
	waitJS(t, ctx, "the button", `document.getElementById("action-awaken-cthugha") !== null`, 20*time.Second)
	if got := evalString(t, ctx, `document.getElementById("action-awaken-cthugha").textContent`); !strings.Contains(got, "(2)") {
		t.Errorf("Cthugha costs 6 minus Cthulhu's 4: %q", got)
	}
	clickSel(t, ctx, "#action-awaken-cthugha")
	waitJS(t, ctx, "the area", `document.querySelector('#board .cell.area.clickable[data-position="south-pacific"]') !== null`, 10*time.Second)
	clickSVG(t, ctx, `#board .cell.area.clickable[data-position="south-pacific"]`)
	waitJS(t, ctx, "the unit to replace, or done", `document.querySelector('#board .token[data-piece="cthugha"]') !== null || document.querySelector('#board .token.token-target[data-piece="cthulhu"]') !== null`, 10*time.Second)
	if evalInt(t, ctx, `document.querySelectorAll('#board .token[data-piece="cthugha"]').length`) == 0 {
		clickSVG(t, ctx, `#board .token.token-target[data-piece="cthulhu"]`)
	}
	waitJS(t, ctx, "Cthugha stands where Cthulhu stood", `document.querySelector('#board .token[data-position="south-pacific"][data-piece="cthugha"]') !== null && document.querySelector('#board .token[data-piece="cthulhu"]') === null`, 15*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
