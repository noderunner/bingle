package app_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// The illustrated board themes: offered by the games played with stones (Blooms
// first), drawn into the board, and left out of the other games.

func TestWebBlooms_IllustratedBoardThemesAreSelectableAndDrawn(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "blooms", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	// A stone on the board, so that the marks can be seen.
	var legal struct {
		Moves []struct {
			Notation string `json:"notation"`
		} `json:"moves"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode+"/moves/legal?seat=1", nil, &legal); code != 200 || len(legal.Moves) == 0 {
		t.Fatalf("legal moves: %d", code)
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/moves", map[string]any{"player_name": "Alice", "notation": legal.Moves[0].Notation}, nil); code != 200 {
		t.Fatalf("move: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, created.RoomCode, "blooms", "Alice")
	waitJS(t, ctx, "stones", `document.querySelectorAll('#board .piece').length >= 1`, 20*time.Second)
	// The options window lists the plain themes and, in a group of their own, the illustrated ones.
	clickSel(t, ctx, "#open-options")
	waitJS(t, ctx, "the theme list", `document.getElementById("opt-board") !== null`, 10*time.Second)
	if got := evalString(t, ctx, `[...document.querySelectorAll('#opt-board optgroup option')].map(o => o.textContent).join("|")`); got != "Hot Synthwave Vibes|Celtic Rune Fantasy|Southwestern Solstice|Moroccan Bazaar|Coral Abyss|Precious Gemstones" {
		t.Errorf("illustrated themes = %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#opt-board > option').length`); n != 4 {
		t.Errorf("%d plain themes, want 4", n)
	}
	for _, th := range []string{"synthwave", "celtic", "solstice", "bazaar", "abyss", "gems"} {
		evalString(t, ctx, `(function(){ var s = document.getElementById("opt-board"); s.value = "`+th+`"; s.dispatchEvent(new Event("change", {bubbles: true})); return "ok"; })()`)
		waitJS(t, ctx, th+" applied", `document.documentElement.dataset.board === "`+th+`" && document.querySelector('#board .theme-decor.th-`+th+`') !== null`, 10*time.Second)
		// Gradients and the stylesheet of the themes are in the page, and a stone is painted with one.
		if got := evalString(t, ctx, `getComputedStyle(document.querySelector('#board .piece .piece-body')).fill`); !strings.Contains(got, "url(") {
			t.Errorf("%s: a stone is painted %q, want a gradient", th, got)
		}
		if n := evalInt(t, ctx, `document.querySelectorAll('#board .piece .theme-mark').length`); n < 1 {
			t.Errorf("%s: no mark on the stone", th)
		}
		// Nothing of the decoration takes a click.
		if got := evalString(t, ctx, `getComputedStyle(document.querySelector('#board .theme-decor')).pointerEvents`); got != "none" {
			t.Errorf("%s: the decoration takes clicks (%s)", th, got)
		}
	}
	// The choice is remembered, and a plain theme takes the decoration away again.
	if got := evalString(t, ctx, `localStorage.getItem("bingle.board")`); !strings.Contains(got, "gems") {
		t.Errorf("stored theme %q", got)
	}
	evalString(t, ctx, `(function(){ var s = document.getElementById("opt-board"); s.value = "wood"; s.dispatchEvent(new Event("change", {bubbles: true})); return "ok"; })()`)
	waitJS(t, ctx, "wood again", `document.documentElement.dataset.board === "wood" && document.querySelector('#board .theme-decor') === null && document.querySelector('#board .theme-mark') === null`, 10*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebBoardThemes_OnlyStoneGamesOfferThem(t *testing.T) {
	baseURL := newRESTTestServer(t)
	for _, c := range []struct {
		def   string
		offer bool
	}{
		{"hex-11", true}, {"yavalath", true}, {"yinsh", true}, {"go-9x9", true}, {"tak-5x5", true}, {"cthulhu-wars", false}, {"hive", false},
	} {
		var created struct {
			RoomCode string `json:"room_code"`
		}
		if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": c.def, "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
			if c.def == "hex-11" {
				continue // the id of the Hex board varies
			}
			t.Fatalf("create %s: %d", c.def, code)
		}
		ctx, cancel := newChromedpContext(t, 60*time.Second)
		openGameAs(t, ctx, baseURL, created.RoomCode, c.def, "Alice")
		// A theme chosen in another game: the page falls back to wood where it is not offered.
		if err := chromedp.Run(ctx, chromedp.Evaluate(`localStorage.setItem("bingle.board", '"celtic"'); true`, nil), chromedp.Navigate(baseURL+"/game/"+created.RoomCode), chromedp.WaitReady("#board-svg, #card-table")); err != nil {
			cancel()
			t.Fatal(err)
		}
		waitJS(t, ctx, c.def+": the page", `document.documentElement.dataset.board !== undefined`, 15*time.Second)
		want := "wood"
		if c.offer {
			want = "celtic"
		}
		if got := evalString(t, ctx, `document.documentElement.dataset.board`); got != want {
			t.Errorf("%s: theme %q, want %q", c.def, got, want)
		}
		clickSel(t, ctx, "#open-options")
		waitJS(t, ctx, c.def+": the theme list", `document.getElementById("opt-board") !== null`, 10*time.Second)
		if n := evalInt(t, ctx, `document.querySelectorAll('#opt-board optgroup option').length`); (n == 6) != c.offer {
			t.Errorf("%s: %d illustrated themes offered", c.def, n)
		}
		cancel()
	}
}

// With a decorated theme the coordinates would sit on the illustrated frame
// and be hard to read: they stand in a plain strip outside it instead. A plain
// theme keeps them on the frame.
func TestWebBoardThemes_CoordinatesStandOutsideTheDecoratedFrame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	for _, def := range []string{"tak-5x5", "hex-7", "havannah-4"} {
		var created struct {
			RoomCode string `json:"room_code"`
		}
		if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": def, "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
			t.Fatalf("create %s: %d", def, code)
		}
		ctx, cancel := newChromedpContext(t, 60*time.Second)
		openGameAs(t, ctx, baseURL, created.RoomCode, def, "Alice")
		waitJS(t, ctx, def+": the board", `document.querySelector('#board-svg .board-frame') !== null`, 20*time.Second)
		clickSel(t, ctx, "#open-options")
		waitJS(t, ctx, def+": the options", `document.getElementById("opt-board") !== null && document.getElementById("opt-coords") !== null`, 10*time.Second)
		evalString(t, ctx, `(function(){ var c = document.getElementById("opt-coords"); if (!c.checked) c.click(); return "ok"; })()`)
		// Where the labels are, against the frame the picture draws.
		const outside = `(function(){
			var svg = document.getElementById("board-svg"), f = svg.querySelector(".board-frame");
			var W = +f.getAttribute("width"), H = +f.getAttribute("height");
			var labels = [...svg.querySelectorAll("text.coord")];
			var out = labels.filter(function(t){ var x = +t.getAttribute("x"), y = +t.getAttribute("y"); return x < 0 || y < 0 || x > W || y > H; }).length;
			var vb = svg.viewBox.baseVal;
			return JSON.stringify({labels: labels.length, outside: out, marked: svg.querySelectorAll("text.coord.coord-out").length, strip: vb.x < 0 && vb.y < 0 && vb.x + vb.width > W && vb.y + vb.height > H});
		})()`
		for _, c := range []struct {
			theme   string
			outside bool
		}{{"gems", true}, {"bazaar", true}, {"synthwave", true}, {"wood", false}} {
			evalString(t, ctx, `(function(){ var s = document.getElementById("opt-board"); s.value = "`+c.theme+`"; s.dispatchEvent(new Event("change", {bubbles: true})); return "ok"; })()`)
			waitJS(t, ctx, def+"/"+c.theme+": applied", `document.documentElement.dataset.board === "`+c.theme+`"`, 10*time.Second)
			var got struct {
				Labels, Outside, Marked int
				Strip                   bool
			}
			waitJS(t, ctx, def+"/"+c.theme+": labels", `JSON.parse(`+outside+`).labels > 0`, 10*time.Second)
			if err := json.Unmarshal([]byte(evalString(t, ctx, outside)), &got); err != nil {
				t.Fatal(err)
			}
			switch {
			case c.outside && (got.Outside != got.Labels || got.Marked != got.Labels || !got.Strip):
				t.Errorf("%s/%s: %+v, want every coordinate outside the frame, in a strip", def, c.theme, got)
			case !c.outside && (got.Outside != 0 || got.Marked != 0 || got.Strip):
				t.Errorf("%s/%s: %+v, want the coordinates on the frame", def, c.theme, got)
			}
		}
		cancel()
	}
}
