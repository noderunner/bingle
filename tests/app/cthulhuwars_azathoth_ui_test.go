package app_test

import (
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// The setup page offers the Azathoth faction and the replacement of a timer
// spellbook (Ghroth, The Thousand Forms) by a neutral one, and the game that
// comes of it shows the neutral cards.
func TestWebHome_NeutralSpellbooksAndAzathothAreChosenOnTheSetupPage(t *testing.T) {
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
	// Both cults with a timer book are at the table: both rows show.
	if n := evalInt(t, ctx, `document.querySelectorAll("#book-swaps .swap-row:not([hidden])").length`); n != 2 {
		t.Fatalf("%d swap rows, want 2 (Black Goat and Crawling Chaos)", n)
	}
	if got := evalString(t, ctx, `[...document.querySelectorAll("#book-swaps .swap-row")].map(r => r.textContent.split(":")[0]).join("|")`); !strings.Contains(got, "Ghroth") || !strings.Contains(got, "The Thousand Forms") {
		t.Errorf("swap rows %q", got)
	}
	if got := evalString(t, ctx, `String(document.getElementById("option-azathoth") && document.getElementById("option-azathoth").checked)`); got != "false" {
		t.Errorf("Azathoth box checked = %q", got)
	}
	set := func(cult, value string) {
		evalString(t, ctx, `(function () {
			var s = document.querySelector('select.swap-select[data-swap="`+cult+`"]');
			s.value = "`+value+`";
			s.dispatchEvent(new Event("change", { bubbles: true }));
			return "ok";
		})()`)
	}
	set("bg", "shrivel")
	// A neutral spellbook replaces one book only: it is taken from the other list.
	if got := evalString(t, ctx, `String(document.querySelector('select.swap-select[data-swap="cc"] option[value="shrivel"]').disabled)`); got != "true" {
		t.Errorf("Shriveling still offered to Crawling Chaos: %q", got)
	}
	set("cc", "mao")
	// A cult that leaves the table takes its row with it.
	evalString(t, ctx, `(function () {
		var s = document.querySelector('#slots-field select.faction-select[data-cult-seat="2"]');
		s.value = "ww";
		s.dispatchEvent(new Event("change", { bubbles: true }));
		return "ok";
	})()`)
	if n := evalInt(t, ctx, `document.querySelectorAll("#book-swaps .swap-row:not([hidden])").length`); n != 1 {
		t.Errorf("%d swap rows after Crawling Chaos left, want 1", n)
	}
	evalString(t, ctx, `(function () {
		var s = document.querySelector('#slots-field select.faction-select[data-cult-seat="2"]');
		s.value = "cc";
		s.dispatchEvent(new Event("change", { bubbles: true }));
		return "ok";
	})()`)
	evalString(t, ctx, `document.getElementById("option-azathoth").click(); "ok"`)
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
	if want := "cthulhu-wars~gc-cc-bg-ys-az-bg.shrivel-cc.mao"; info.GameDefID != want {
		t.Fatalf("game = %q, want %q", info.GameDefID, want)
	}
	// The game page: the neutral card of every cult lists the spellbooks and
	// the Loyalty Cards; Black Goat's book card shows Shriveling in Ghroth's place.
	// Play it: the same table with bots in the other seats.
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat": 1,
		"seat_bots": map[string]string{"2": "tactical", "3": "tactical", "4": "tactical"},
		"options":   map[string]any{"factions": []any{"gc", "cc", "bg", "ys"}, "azathoth": true, "book-swaps": map[string]any{"bg": "shrivel", "cc": "mao"}}}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 && code != 201 {
		t.Fatalf("create: %d", code)
	}
	openGameAs(t, ctx, baseURL, created.RoomCode, info.GameDefID, "Alice")
	waitJS(t, ctx, "the neutral cards", `document.querySelectorAll('[data-card="neutral"]').length >= 1`, 30*time.Second)
	if got := evalString(t, ctx, `document.querySelector('[data-card="neutral"]').textContent`); !strings.Contains(got, "Undimensioned") || !strings.Contains(got, "Dimensional Shambler") {
		t.Errorf("neutral card: %q", got)
	}
	if got := evalString(t, ctx, `[...document.querySelectorAll('[data-card="books"]')].map(c => c.textContent).join("|")`); !strings.Contains(got, "in place of Ghroth") || !strings.Contains(got, "in place of The Thousand Forms") {
		t.Errorf("book cards: %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
