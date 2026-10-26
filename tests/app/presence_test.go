package app_test

import (
	"testing"
	"time"
)

func TestRESTPresenceIsSharedAndOnlyForSeatedPlayers(t *testing.T) {
	base := newRESTTestServer(t)
	room := newTakRoom(t, base, "tic-tac-toe", "Alice")
	joinRoom(t, base, room, "Bob")

	var got struct {
		Seats []int `json:"seats"`
	}
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/presence", nil, &got); code != 200 || len(got.Seats) != 0 {
		t.Fatalf("fresh room: %d %+v", code, got)
	}
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/presence", map[string]any{"player_name": "Alice", "present": true}, &got); code != 200 || len(got.Seats) != 1 || got.Seats[0] != 1 {
		t.Fatalf("Alice present: %d %+v", code, got)
	}
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/presence", map[string]any{"player_name": "Nobody", "present": true}, nil); code != 403 {
		t.Errorf("a stranger marking presence: %d, want 403", code)
	}
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/presence", map[string]any{"player_name": "Alice", "present": false}, &got); code != 200 || len(got.Seats) != 0 {
		t.Fatalf("Alice away: %d %+v", code, got)
	}
}

func TestWebPresenceIconTogglesAndOthersSeeIt(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	aliceCtx, cancelA := newChromedpContext(t, 60*time.Second)
	defer cancelA()
	openGameAs(t, aliceCtx, baseURL, room, "tic-tac-toe", "Alice")
	bobCtx, cancelB := newChromedpContext(t, 60*time.Second)
	defer cancelB()
	openGameAs(t, bobCtx, baseURL, room, "tic-tac-toe", "Bob")

	aliceBox := `document.querySelector('#seat-1 .swatch')`
	waitJS(t, aliceCtx, "Alice's own box is a button", aliceBox+`.tagName === "BUTTON" && `+aliceBox+`.dataset.presence === "away"`, 10*time.Second)
	if got := evalString(t, bobCtx, `document.querySelector('#seat-1 .swatch').tagName`); got != "SPAN" {
		t.Errorf("another player's box should not be clickable, got %s", got)
	}
	waitJS(t, bobCtx, "Bob sees his own as a button", `document.querySelector('#seat-2 .swatch').tagName === "BUTTON"`, 10*time.Second)

	evalString(t, aliceCtx, aliceBox+`.click(); "ok"`)
	waitJS(t, aliceCtx, "Alice shows present", `document.querySelector('#seat-1 .swatch').dataset.presence === "present"`, 10*time.Second)
	waitJS(t, bobCtx, "Bob sees Alice present", `document.querySelector('#seat-1 .swatch').dataset.presence === "present" && document.querySelector('#seat-1 .swatch .eye')`, 10*time.Second)
	if got := evalString(t, bobCtx, `document.querySelector('#seat-2 .swatch').dataset.presence`); got != "away" {
		t.Errorf("Bob should still be away, got %s", got)
	}

	evalString(t, aliceCtx, `document.querySelector('#seat-1 .swatch').click(); "ok"`)
	waitJS(t, bobCtx, "Bob sees Alice away again", `document.querySelector('#seat-1 .swatch').dataset.presence === "away"`, 10*time.Second)
}
