package app_test

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

// A Primeval Map has a picture behind it and its Glacier glyphs in a layer of their
// own, and the areas above can still be clicked.
func TestWebCthulhuWars_ThePrimevalMapHasAPictureAndAGlacierLayer(t *testing.T) {
	baseURL := newRESTTestServer(t)
	const id = "cthulhu-wars-primeval-5p"
	resp, err := http.Get(baseURL + "/api/game-defs/" + id + "/assets/guide/maps/primeval-5p.webp")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/webp" {
		t.Fatalf("the picture: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": id, "player_name": "Alice", "seat_bots": map[string]string{"2": "tactical", "3": "tactical", "4": "tactical", "5": "tactical"}}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 && code != 201 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, created.RoomCode, id, "Alice")
	waitJS(t, ctx, "the picture", `document.querySelector("#board-svg.has-art image.map-art") !== null`, 30*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll("#board-svg .ice-glyph").length`); n != 12 {
		t.Errorf("%d Glacier glyphs", n)
	}
	// The areas are transparent over it, and the glyphs under what can be clicked.
	if got := evalString(t, ctx, `getComputedStyle(document.querySelector('.cell.area .area-bg')).fill`); got != "rgba(0, 0, 0, 0)" && got != "transparent" {
		t.Errorf("an area is filled with %q", got)
	}
	if got := evalString(t, ctx, `getComputedStyle(document.querySelector('.ice-glyphs')).pointerEvents`); got != "none" {
		t.Errorf("the glyphs take the pointer: %q", got)
	}
	// The layer goes and comes back, and the picture is the same element after a redraw.
	waitJS(t, ctx, "the toggle", `document.getElementById("opt-layer-ice") !== null`, 15*time.Second)
	evalString(t, ctx, `window.__art = document.querySelector("image.map-art"); document.getElementById("opt-layer-ice").click(); "ok"`)
	waitJS(t, ctx, "the glyphs hidden", `document.querySelectorAll(".ice-glyph").length === 0`, 10*time.Second)
	if !evalBool(t, ctx, `window.__art === document.querySelector("image.map-art")`) {
		t.Error("a redraw made a new picture")
	}
	evalString(t, ctx, `document.getElementById("opt-layer-ice").click(); "ok"`)
	waitJS(t, ctx, "the glyphs back", `document.querySelectorAll(".ice-glyph").length === 12`, 10*time.Second)

	// Another map has its own picture and its own number of glyphs, from a different redraw.
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars-primeval-3-5", "player_name": "Alice", "seat_bots": map[string]string{"2": "tactical", "3": "tactical", "4": "tactical"}}, &created); code != 200 && code != 201 {
		t.Fatalf("create: %d", code)
	}
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars-primeval-3-5", "Alice")
	waitJS(t, ctx, "the other map", `document.querySelector("#board-svg.has-art image.map-art") !== null && document.querySelector("image.map-art").getAttribute("href").endsWith("primeval-3-5.webp") && document.querySelectorAll(".ice-glyph").length === 11`, 30*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// The board's glyphs stand where it prints them on a map that says so, and in a round
// badge on any other; the layer hides both.
func TestWebCthulhuWars_TheGlyphsAreTheBoards(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	bots := map[string]string{"2": "tactical", "3": "tactical", "4": "tactical"}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars-3-5", "player_name": "Alice", "seat_bots": bots}, &created); code != 200 && code != 201 {
		t.Fatalf("create: %d", code)
	}
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars-3-5", "Alice")
	// Yellow Sign is at this table, so the glyphs show.
	waitJS(t, ctx, "the printed glyphs", `document.querySelectorAll("#board .area-glyph-badge.printed").length === 10`, 30*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll("#board .area-glyph-badge.printed circle").length`); n != 0 {
		t.Errorf("%d printed glyphs in a badge", n)
	}
	if got := evalString(t, ctx, `document.querySelector('#board .area-glyph-badge[data-area="north-america"] title').textContent`); got != "Dragon glyph (Yellow Sign's Desecrate requirement)" {
		t.Errorf("North America's glyph says %q", got)
	}
	if n := evalInt(t, ctx, `[...document.querySelectorAll("#board .area-glyph path")].filter(p => !p.getAttribute("fill")).length`); n != 0 {
		t.Errorf("%d glyph shapes without a colour", n)
	}
	clickSel(t, ctx, "#opt-layer-glyphs")
	waitJS(t, ctx, "the glyphs hidden", `document.querySelectorAll("#board .area-glyph-badge").length === 0`, 10*time.Second)
	clickSel(t, ctx, "#opt-layer-glyphs")
	waitJS(t, ctx, "the glyphs back", `document.querySelectorAll("#board .area-glyph-badge.printed").length === 10`, 10*time.Second)

	// A Primeval map has the same glyphs in badges, not at Earth's places.
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars-primeval-5-3", "player_name": "Alice", "seat_bots": bots}, &created); code != 200 && code != 201 {
		t.Fatalf("create: %d", code)
	}
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars-primeval-5-3", "Alice")
	waitJS(t, ctx, "the badges", `document.querySelectorAll("#board .area-glyph-badge circle").length > 0 && document.querySelectorAll("#board .area-glyph-badge.printed").length === 0 && document.querySelector("#board .area-glyph-badge .area-glyph path") !== null`, 30*time.Second)
	// The base Earth layout, whose params the others merge over, prints its own.
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat_bots": bots}, &created); code != 200 && code != 201 {
		t.Fatalf("create: %d", code)
	}
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the base map's glyphs", `document.querySelector("image.map-art") !== null && document.querySelector("image.map-art").getAttribute("href").endsWith("earth-5-3.webp") && document.querySelectorAll("#board .area-glyph-badge.printed").length === 10`, 30*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
