package app_test

import (
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// The UI can play Onitama: ten pieces, five drawn cards, the arches tinted,
// and a move built from a card, a piece and a square.
func TestWebOnitama_PlayTurnsThroughTheUI(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "onitama", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
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
	openGameAs(t, ctx, baseURL, created.RoomCode, "onitama", "Alice")
	waitJS(t, ctx, "board", `document.querySelectorAll('#board .cell').length === 25`, 15*time.Second)
	waitJS(t, ctx, "ten pieces", `document.querySelectorAll('#board .piece').length === 10`, 10*time.Second)
	waitJS(t, ctx, "five cards", `document.querySelectorAll('#cards-panel .move-card').length === 5`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .cell-bg.tint').length`); n < 2 {
		t.Errorf("the arches should be tinted: %d", n)
	}
	stuck := 0
	for i := 0; i < 200 && stuck < 40; i++ {
		var what string
		if err := chromedp.Run(ctx, chromedp.Evaluate(autoStepJS, &what)); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		switch what {
		case "wait", "stuck":
			stuck++
			time.Sleep(80 * time.Millisecond)
		default:
			stuck = 0
			time.Sleep(60 * time.Millisecond)
		}
		if st := cfGet(t, baseURL, created.RoomCode); st.MoveSeq >= 8 || st.Status == "finished" {
			break
		}
	}
	if st := cfGet(t, baseURL, created.RoomCode); st.MoveSeq < 4 {
		t.Fatalf("the UI should have played turns: %d moves", st.MoveSeq)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
