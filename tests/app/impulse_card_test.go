// Tests of the impulse card: the name of the seat on impulse over what it is asked
// to choose, for everyone at the table, with each step of the acting player's own.

package app_test

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestWebImpulse_TheCardNamesTheSeatAndEachStepIsItsOwn(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {1, "cultist", "new-zealand", 1}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 10, 3, 3, 3}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)

	alice, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, alice, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, alice, "the buttons", `document.getElementById("action-gate") !== null`, 15*time.Second)
	says := func(want string) string {
		return `document.getElementById("impulse-title").textContent === "Alice" && document.getElementById("impulse-text").textContent === "` + want + `"`
	}
	waitJS(t, alice, "the first impulse", says("Choose an action."), 5*time.Second)

	// Somebody else sees the same two lines, under the same name, and no buttons.
	watcher, cancelWatcher := newChromedpContext(t, 90*time.Second)
	defer cancelWatcher()
	openGameAs(t, watcher, baseURL, room, "cthulhu-wars", "Bob")
	waitJS(t, watcher, "the same words for another player", says("Choose an action."), 15*time.Second)
	if evalBool(t, watcher, `document.getElementById("impulse-card").classList.contains("acting") || document.getElementById("action-gate") !== null`) {
		t.Error("another player's card offers the actions")
	}

	// Choosing an action is a step of its own, with its own question.
	clickSel(t, alice, "#action-gate")
	waitJS(t, alice, "the next impulse", says("Choose an area to place a gate."), 5*time.Second)
	// Nothing is sent yet, so the rest of the table is not told.
	time.Sleep(300 * time.Millisecond)
	if got := evalString(t, watcher, `document.getElementById("impulse-text").textContent`); got != "Choose an action." {
		t.Errorf("another player sees a step that was not sent: %q", got)
	}
}

// A generic game has no words of its own: the title is still the name, and the words
// come from its actions rather than saying "Your move".
func TestWebImpulse_OnAGameOfTheGenericEngine(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	fittedPlain(t, ctx, baseURL, "gomoku")
	waitJS(t, ctx, "the card", `document.getElementById("impulse-title") !== null && document.getElementById("impulse-title").textContent === "Alice"`, 15*time.Second)
	if got := evalString(t, ctx, `document.getElementById("impulse-card").textContent`); containsAny(got, "Your move", "Your choices") {
		t.Errorf("the card still says %q", got)
	}
	if got := evalString(t, ctx, `document.getElementById("impulse-text").textContent`); got == "" {
		t.Error("the card does not say what is being asked")
	}
}

// fittedPlain opens a game of def as its creator, Alice.
func fittedPlain(t *testing.T, ctx context.Context, base, def string) {
	t.Helper()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": def, "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	openGameAs(t, ctx, base, created.RoomCode, def, "Alice")
}

func containsAny(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
