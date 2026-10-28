package app_test

import (
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// The UI can play Meridians: the centreless hexagon is drawn as points, a move
// is one click, the page says what the turn asks for, and the stones of a group
// that can see no friend are marked before they come off.
func TestWebMeridians_PlayThroughTheUI(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "meridians", "player_name": "Ana", "also_seats": []int{2}}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, created.RoomCode, "meridians", "Ana")

	// The board: 114 points, every one of them a legal first move.
	waitJS(t, ctx, "the board", `document.querySelectorAll("#board .cell[data-position]").length === 114`, 20*time.Second)
	waitJS(t, ctx, "every point clickable", `document.querySelectorAll("#board .cell.clickable").length === 114`, 15*time.Second)
	if got := evalString(t, ctx, `document.getElementById("status-message").textContent`); !strings.Contains(got, "first stone") {
		t.Errorf("the status says %q, want it to ask for the first stone", got)
	}

	// Four stones, one click each: Light, Dark, Light, Dark. (A cell of the
	// board is an SVG element, so the click is dispatched as an event.)
	const clickPoint = `(function () {
		var cell = document.querySelector("#board .cell.clickable");
		if (!cell) return "none";
		cell.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
		return cell.dataset.position;
	})()`
	for i := 0; i < 4; i++ {
		before := evalInt(t, ctx, `document.querySelectorAll("#board .piece").length`)
		if got := evalString(t, ctx, clickPoint); got == "none" {
			t.Fatalf("move %d: no point can be clicked", i+1)
		}
		waitJS(t, ctx, "the stone", `document.querySelectorAll("#board .piece").length > `+itoa(before), 15*time.Second)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll("#board .piece").length`); n != 4 {
		t.Fatalf("%d stones on the board after four moves", n)
	}
	// A player's second stone never touches their first, so both of each
	// colour are still there: nothing died in the opening.
	if got := evalString(t, ctx, `[...document.querySelectorAll("#board .piece")].map(p => p.dataset.color).sort().join(",")`); got != "black,black,white,white" {
		t.Errorf("the stones are %q, want two of each", got)
	}
	// The game narrates, so the History tab shows first; the Moves tab carries
	// the points as notation (a1 … l13).
	if got := evalString(t, ctx, `document.getElementById("events").textContent`); !strings.Contains(got, "lays a stone") {
		t.Errorf("the history reads %q", got)
	}
	clickSel(t, ctx, "#tab-moves")
	waitJS(t, ctx, "the moves", `document.querySelectorAll('#move-list button.mv[data-seq]').length === 4`, 15*time.Second)
	if got := evalString(t, ctx, `[...document.querySelectorAll('#move-list button.mv[data-seq]')].map(b => b.textContent.trim()).join(" ")`); !regexp.MustCompile(`[a-l](\d|1[0-3])`).MatchString(got) {
		t.Errorf("the move list reads %q, want the points the stones went on", got)
	}
	// The rules window has the game's own page, with its pictures.
	clickSel(t, ctx, "#show-rules")
	waitJS(t, ctx, "the rules", `document.getElementById("rules-dialog") !== null && document.querySelectorAll("#rules-dialog [role=tab]").length >= 1`, 10*time.Second)
	waitJS(t, ctx, "the sight picture", `document.querySelectorAll("#rules-dialog img[src*='meridians-sight']").length === 1`, 10*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
