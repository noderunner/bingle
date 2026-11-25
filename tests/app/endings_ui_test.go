package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// A game's own ending is worded in its definition and sent in its info
// (ADR 0150, #314), and the page says how a game ended in those words rather
// than as the reason's code: the Table fixture's "count" is "on rings".
func TestWebAGamesOwnEndingIsInItsWords(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var info struct {
		Endings map[string]string `json:"endings"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/game-defs/test-table", nil, &info); code != 200 || info.Endings["count"] != "on rings" {
		t.Fatalf("the game's info: %d, endings %v", code, info.Endings)
	}

	// Alice plays the random bot to the end, over the API.
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "test-table", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	var st struct {
		Status    string `json:"status"`
		EndReason string `json:"end_reason"`
	}
	for i := 0; ; i++ {
		if doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st); st.Status == "finished" {
			break
		}
		if i > 200 {
			t.Fatal("the game has not ended")
		}
		var sel struct {
			Options []struct {
				Notation string `json:"notation"`
			} `json:"options"`
		}
		doJSON(t, "GET", baseURL+"/api/games/"+room+"/selections", nil, &sel)
		if len(sel.Options) == 0 {
			t.Fatalf("Alice has nothing to play in a game that goes on")
		}
		pick := sel.Options[i*7%len(sel.Options)].Notation
		if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/moves", map[string]any{"player_name": "Alice", "notation": pick}, nil); code != 200 {
			t.Fatalf("move %s: %d", pick, code)
		}
	}
	if st.EndReason != "count" {
		t.Fatalf("the game ended by %q", st.EndReason)
	}

	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "test-table", "Alice")
	waitJS(t, ctx, "the result in the game's words", `document.getElementById("status-message").textContent.includes("on rings")`, 10*time.Second)
	var msg string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("status-message").textContent`, &msg)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(msg, "(count)") {
		t.Errorf("the result names the reason's code: %q", msg)
	}

	// The words for a reason and one detail of it say the detail too; a
	// detail with no words of its own is shown after the reason's words.
	script := `(async () => {
		const { gameEnding } = await import("/static/js/util.js");
		const endings = { marbles: "with a set of marbles", "marbles:white": "with four white marbles" };
		return JSON.stringify([
			gameEnding(endings, "marbles", "white"),
			gameEnding(endings, "marbles", "grey"),
			gameEnding(endings, "marbles", ""),
			gameEnding(endings, "count", ""),
			gameEnding(undefined, "marbles", "white"),
		]);
	})()`
	var got string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &got, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil {
		t.Fatal(err)
	}
	want := `[{"words":"with four white marbles","saysDetail":true},{"words":"with a set of marbles","saysDetail":false},{"words":"with a set of marbles","saysDetail":false},null,null]`
	if got != want {
		t.Errorf("gameEnding:\n got %s\nwant %s", got, want)
	}
}
