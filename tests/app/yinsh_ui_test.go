package app_test

import (
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// yinshStepJS clicks the first highlighted point (a ring, then where it goes).
const yinshStepJS = `(function () {
	var bar = document.querySelector(".status-bar");
	if (!bar || !bar.classList.contains("your-turn")) return "wait";
	var cell = document.querySelector("#board .cell.clickable");
	if (!cell) return "stuck";
	cell.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
	return "cell";
})()`

// The UI can play Yinsh: the 85-point board, rings and markers, the scoreboards,
// the ring setup and then a ring and a destination for every turn.
func TestWebYinsh_PlayTurnsThroughTheUI(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "yinsh", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*runtime.EventExceptionThrown); ok && e.ExceptionDetails != nil {
			mu.Lock()
			jsErrors = append(jsErrors, e.ExceptionDetails.Error())
			mu.Unlock()
		}
	})
	openGameAs(t, ctx, baseURL, created.RoomCode, "yinsh", "Alice")
	waitJS(t, ctx, "85 points", `document.querySelectorAll('#board .cell').length === 85`, 15*time.Second)
	waitJS(t, ctx, "scoreboards", `document.querySelectorAll('.seat-strip [data-counter="rings"]').length === 2`, 5*time.Second)
	stuck := 0
	for i := 0; i < 400 && stuck < 40; i++ {
		var what string
		if err := chromedp.Run(ctx, chromedp.Evaluate(yinshStepJS, &what)); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		switch what {
		case "wait", "stuck":
			stuck++
			time.Sleep(80 * time.Millisecond)
		default:
			stuck = 0
			time.Sleep(50 * time.Millisecond)
		}
		if st := cfGet(t, baseURL, created.RoomCode); st.MoveSeq >= 16 || st.Status == "finished" {
			break
		}
	}
	if st := cfGet(t, baseURL, created.RoomCode); st.MoveSeq < 12 {
		t.Fatalf("the UI should have placed rings and moved: %d moves", st.MoveSeq)
	}
	waitJS(t, ctx, "a marker on the board", `document.querySelectorAll('#board [data-piece="marker"]').length > 0`, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
