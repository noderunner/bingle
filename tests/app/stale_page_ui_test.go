package app_test

import (
	"fmt"
	"testing"
	"time"
)

// A page waiting on another seat catches up when it has missed the move that
// ended the wait: after a silence it asks the server whose turn it is (a burst
// of bot moves once left a page waiting on a seat that had long since played).
func TestWebGame_AStalePageCatchesUp(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	// The page's socket goes deaf to moves once window.deaf is set.
	deaf := `(function () {
		const Real = window.WebSocket;
		window.WebSocket = function (url, protocols) {
			const ws = new Real(url, protocols);
			const add = ws.addEventListener.bind(ws);
			ws.addEventListener = (type, fn, opts) => add(type, (ev) => { if (type === "message" && window.deaf) return; fn(ev); }, opts);
			return ws;
		};
		window.WebSocket.prototype = Real.prototype;
		Object.assign(window.WebSocket, Real);
	})()`
	if err := runSteps(ctx, stepAddScript(deaf)); err != nil {
		t.Fatal(err)
	}
	var created struct {
		RoomCode string `json:"room_code"`
	}
	alice := newGuestKey(t, base)
	if code := doJSONAs(t, alice, "POST", base+"/api/games", map[string]any{"game_def_id": "gomoku", "player_name": "Alice"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	if err := runSteps(ctx, stepNavigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	var joined bool
	join := fmt.Sprintf(`import("/static/js/api.js").then(m => m.api.join("%s", "Bob")).then(() => true)`, room)
	if err := runSteps(ctx, stepEvalTo(join, &joined, awaitPromise)); err != nil || !joined {
		t.Fatalf("join: %v %v", joined, err)
	}
	openGameAs(t, ctx, base, room, "gomoku", "Bob")
	waitJS(t, ctx, "Alice's turn", `document.getElementById("impulse-title") && document.getElementById("impulse-title").textContent === "Alice"`, 15*time.Second)
	evalBool(t, ctx, `(window.deaf = true, true)`)
	if code := moveAs(t, alice, base, room, "Alice", "h8"); code != 200 {
		t.Fatalf("move: %d", code)
	}
	// The event is lost; the check after the silence finds Bob's turn.
	waitJS(t, ctx, "Bob's turn", `document.querySelector("#impulse-card.acting") !== null`, 50*time.Second)
}
