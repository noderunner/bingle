package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/engine/board"
)

// setHexStyle picks a board style in the options window, as a player does.
func setHexStyle(t *testing.T, ctx context.Context, style string) {
	t.Helper()
	evalString(t, ctx, `(function () {
		var sel = document.getElementById("opt-hex-style");
		sel.value = "`+style+`";
		sel.dispatchEvent(new Event("change", { bubbles: true }));
		return "ok";
	})()`)
}

func openNewGame(t *testing.T, ctx context.Context, baseURL, def string) {
	t.Helper()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": def, "player_name": "Ana", "also_seats": []int{2}}, &created); code != 200 {
		t.Fatalf("create %s: %d", def, code)
	}
	openGameAs(t, ctx, baseURL, created.RoomCode, def, "Ana")
}

// Meridians starts on the hex grid: every point a dot, every pair of
// neighbours joined once, no hexagons; a click on a point lays a stone there.
// The player can switch to hexagons and the page remembers it for the game.
func TestWebHexGrid_MeridiansDefaultsToIt(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	openNewGame(t, ctx, baseURL, "meridians")

	g, err := board.GenerateHexHexSides(6, 7)
	if err != nil {
		t.Fatal(err)
	}
	edges := 0
	for _, id := range g.Positions() {
		edges += len(g.Neighbors(id))
	}
	edges /= 2

	waitJS(t, ctx, "the points", `document.querySelectorAll("#board .grid-point").length === 114`, 20*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll("#board polygon.cell-bg").length`); n != 0 {
		t.Errorf("%d hexagons on the hex grid", n)
	}
	if n := evalInt(t, ctx, `document.querySelector("#board .hex-lattice").getAttribute("d").split("M").length - 1`); n != edges {
		t.Errorf("%d lines on the board, want one per pair of neighbours (%d)", n, edges)
	}
	// A move is still a click on a point, and the stone stands on the point.
	evalString(t, ctx, `(function () {
		document.querySelector('#board .cell.clickable[data-position="g7"]').dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
		return "ok";
	})()`)
	waitJS(t, ctx, "the stone", `document.querySelector('#board .piece[data-position="g7"]') !== null`, 15*time.Second)
	if got := evalString(t, ctx, `(function () {
		var dot = document.querySelector('#board .cell[data-position="g7"] .grid-point');
		return document.querySelector('#board .piece[data-position="g7"]').getAttribute("transform").includes(dot.getAttribute("cx")) ? "on" : "off";
	})()`); got != "on" {
		t.Errorf("the stone is %s its point", got)
	}

	// The option, and the switch: a whole board of hexagons, and no lattice.
	clickSel(t, ctx, "#open-options")
	waitJS(t, ctx, "the option", `document.getElementById("opt-hex-style") !== null && document.getElementById("opt-hex-style").value === "grid"`, 10*time.Second)
	setHexStyle(t, ctx, "cells")
	waitJS(t, ctx, "the hexagons", `document.querySelectorAll("#board polygon.cell-bg").length === 114 && document.querySelector("#board .hex-lattice") === null`, 10*time.Second)
	if got := evalString(t, ctx, `localStorage.getItem("bingle.hexStyle.meridians")`); !strings.Contains(got, "cells") {
		t.Errorf("stored style %q", got)
	}

	// Remembered, and shared with the game's variants.
	openNewGame(t, ctx, baseURL, "meridians-5")
	waitJS(t, ctx, "hexagons in the variant", `document.querySelectorAll("#board polygon.cell-bg").length > 0 && document.querySelector("#board .hex-lattice") === null`, 20*time.Second)
}

// Every other hex game starts on hexagons and can be switched, whatever was
// chosen for Meridians, and a game without a hex board to draw offers nothing.
func TestWebHexGrid_AnyHexGameCanUseIt(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	openNewGame(t, ctx, baseURL, "yavalath")
	if err := chromedp.Run(ctx, chromedp.Evaluate(`localStorage.setItem("bingle.hexStyle.meridians", '"grid"'); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "hexagons", `document.querySelectorAll("#board polygon.cell-bg").length === 61 && document.querySelector("#board .hex-lattice") === null`, 20*time.Second)
	clickSel(t, ctx, "#open-options")
	waitJS(t, ctx, "the option", `document.getElementById("opt-hex-style") !== null && document.getElementById("opt-hex-style").value === "cells"`, 10*time.Second)
	setHexStyle(t, ctx, "grid")
	waitJS(t, ctx, "the grid", `document.querySelectorAll("#board .grid-point").length === 61 && document.querySelectorAll("#board polygon.cell-bg").length === 0`, 10*time.Second)

	// The players' edges of Hex stay on the board.
	openNewGame(t, ctx, baseURL, "hex-7")
	clickSel(t, ctx, "#open-options")
	waitJS(t, ctx, "the option in Hex", `document.getElementById("opt-hex-style") !== null`, 10*time.Second)
	setHexStyle(t, ctx, "grid")
	waitJS(t, ctx, "the edges", `document.querySelectorAll("#board circle.cell-bg.edge-red").length > 0 && document.querySelectorAll("#board circle.cell-bg.edge-blue").length > 0`, 10*time.Second)

	// A free board (the Table fixture's) is no board to draw either way.
	for _, def := range []string{"test-table", "go-9x9"} {
		openNewGame(t, ctx, baseURL, def)
		clickSel(t, ctx, "#open-options")
		waitJS(t, ctx, def+": the options", `document.getElementById("opt-board") !== null`, 10*time.Second)
		if n := evalInt(t, ctx, `document.querySelectorAll("#opt-hex-style").length`); n != 0 {
			t.Errorf("%s offers a board style", def)
		}
	}
}
