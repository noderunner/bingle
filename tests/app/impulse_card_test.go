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
	var created createGameResponse
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "test-council", "player_name": "Alice", "seat_bots": map[string]string{"2": "random", "3": "random", "4": "random"}}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode

	alice, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, alice, baseURL, room, "test-council", "Alice")
	waitJS(t, alice, "the buttons", `document.getElementById("action-claim") !== null`, 15*time.Second)
	says := func(want string) string {
		return `document.getElementById("impulse-title").textContent === "Alice" && document.getElementById("impulse-text").textContent === "` + want + `"`
	}
	const first = "Move your envoy, or claim the area it stands in."
	waitJS(t, alice, "the first impulse", says(first), 5*time.Second)

	// Somebody else sees the same two lines, under the same name, and no buttons.
	watcher, cancelWatcher := newChromedpContext(t, 90*time.Second)
	defer cancelWatcher()
	openGameAs(t, watcher, baseURL, room, "test-council", "Bob")
	waitJS(t, watcher, "the same words for another player", says(first), 15*time.Second)
	if evalBool(t, watcher, `document.getElementById("impulse-card").classList.contains("acting") || document.getElementById("action-claim") !== null`) {
		t.Error("another player's card offers the actions")
	}

	// Choosing an action is a step of its own, with its own question.
	clickSel(t, alice, "#action-claim")
	waitJS(t, alice, "the next impulse", says("Claim: choose an area."), 5*time.Second)
	// Nothing is sent yet, so the rest of the table is not told.
	time.Sleep(300 * time.Millisecond)
	if got := evalString(t, watcher, `document.getElementById("impulse-text").textContent`); got != first {
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
