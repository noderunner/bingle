package app_test

import (
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/tests/testdata"
)

// packStepJS clicks the first square the page offers, which builds a move
// from a man and the square it goes to, a jump at a time.
const packStepJS = `(function () {
	if (!document.querySelector("#impulse-card.acting")) return "wait";
	var hit = document.querySelector("#board .cell.clickable .cell-hit");
	if (!hit) return "stuck";
	hit.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
	return "cell";
})()`

// A game from an install pack (ADR 0147) is drawn and played in the page as
// any other: the page knows nothing of where its rules came from.
func TestWebPack_APacksGameIsPlayedInThePage(t *testing.T) {
	packs, _ := testdata.DraughtsPack(t)
	cfg := newTestConfig(t)
	cfg.Packs = packs
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	baseURL := srv.URL

	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": testdata.DraughtsPackID, "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, created.RoomCode, testdata.DraughtsPackID, "Alice")
	waitJS(t, ctx, "64 squares", `document.querySelectorAll('#board .cell').length === 64`, 15*time.Second)
	waitJS(t, ctx, "24 men", `document.querySelectorAll('#board .piece').length === 24`, 5*time.Second)
	for i := 0; i < 200; i++ {
		var what string
		if err := runSteps(ctx, stepEvalTo(packStepJS, &what)); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		time.Sleep(80 * time.Millisecond)
		if st := cfGet(t, baseURL, created.RoomCode); st.MoveSeq >= 2 {
			break
		}
	}
	if st := cfGet(t, baseURL, created.RoomCode); st.MoveSeq < 2 {
		t.Fatalf("the page played no move and the bot no reply: %d moves", st.MoveSeq)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
