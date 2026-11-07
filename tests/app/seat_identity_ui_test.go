package app_test

import (
	"strings"
	"testing"
	"time"
)

// A browser that remembers a seat's name is not that seat: the seat a guest or
// an account holds is the page's own only when the server says it is. Another
// browser with the same name watches, told why, with nothing to play.
func TestWebGame_ANameIsNotASeat(t *testing.T) {
	base := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	alice := newGuestKey(t, base)
	if code := doJSONAs(t, alice, "POST", base+"/api/games", map[string]any{"game_def_id": "gomoku", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	// This browser has a guest of its own, and remembers the name "Alice" for the room.
	openGameAs(t, ctx, base, created.RoomCode, "gomoku", "Alice")
	waitJS(t, ctx, "the board", `document.querySelector("#board-svg") !== null && document.getElementById("impulse-card") !== null`, 15*time.Second)
	time.Sleep(time.Second)
	if evalBool(t, ctx, `document.querySelector("#impulse-card.acting") !== null || document.querySelector("#board .cell.clickable, #board .cell.quick") !== null`) {
		t.Fatal("a browser that only knows the name plays the seat")
	}
	if got := evalString(t, ctx, `document.body.innerText`); !strings.Contains(got, "Alice plays from another browser or account") {
		t.Errorf("the page does not say why it only watches")
	}
}
