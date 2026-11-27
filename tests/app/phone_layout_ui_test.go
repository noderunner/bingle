package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

// councilWithHistory makes a game of the four-seat fixture with a map of areas,
// four people at it, and plays first legal moves until it has played plies.
func councilWithHistory(t *testing.T, base string, plies int) string {
	t.Helper()
	var created createGameResponse
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "test-council", "player_name": "Alice"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	names := []string{"Alice", "Bob", "Cara", "Dan"}
	for _, n := range names[1:] {
		joinRoom(t, base, created.RoomCode, n)
	}
	played := 0
	for i := 0; i < plies*8 && played < plies; i++ {
		n := names[i%len(names)]
		var legal struct {
			Options []struct {
				Notation string `json:"notation"`
			} `json:"options"`
		}
		doJSON(t, "GET", base+"/api/games/"+created.RoomCode+"/selections?player_name="+n, nil, &legal)
		if len(legal.Options) == 0 {
			continue
		}
		if code, _ := moveNotation(t, base, created.RoomCode, n, legal.Options[0].Notation); code == 200 {
			played++
		}
	}
	if played < plies {
		t.Fatalf("played %d plies, want %d", played, plies)
	}
	return created.RoomCode
}

// openOnPhone opens a game held upright on a phone-sized touch screen, in the
// phone layout, with the moves written in notation. games are the room codes
// for the browser's list of games, the first being the one opened.
func openOnPhone(t *testing.T, ctx context.Context, base, name string, games map[string]string, first string) {
	t.Helper()
	list := []map[string]any{}
	for code, def := range games {
		list = append(list, map[string]any{"code": code, "def": def, "name": name, "ts": time.Now().UnixMilli()})
	}
	gamesJSON, _ := json.Marshal(list)
	nameJSON, _ := json.Marshal(name)
	if err := chromedp.Run(ctx,
		emulation.SetDeviceMetricsOverride(390, 760, 2, true).WithScreenOrientation(&emulation.ScreenOrientation{Type: emulation.OrientationTypePortraitPrimary}),
		emulation.SetTouchEmulationEnabled(true),
		chromedp.Navigate(base+"/healthz"),
		chromedp.Evaluate(fmt.Sprintf(`localStorage.setItem("bingle.games", %q); localStorage.setItem("bingle.name", %q);
			localStorage.setItem("bingle.layout", '"phone"'); localStorage.setItem("bingle.historyFormat", '"notation"');
			localStorage.setItem("bingle.mobileHint2", "true"); true`, gamesJSON, nameJSON), nil),
		chromedp.Navigate(base+"/game/"+first),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the phone layout", `document.getElementById("m-shell") !== null`, 15*time.Second)
}

// On a phone the history opens on its newest move, as on a computer, and the
// buttons to the first and the latest position bring the list there with them.
func TestPhoneUI_TheHistoryOpensAtItsEndAndItsButtonsFollow(t *testing.T) {
	base := newRESTTestServer(t)
	room := councilWithHistory(t, base, 30)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openOnPhone(t, ctx, base, "Alice", map[string]string{room: "test-council"}, room)
	clickSel(t, ctx, `#m-nav [data-group="log"]`)
	list := `document.querySelector("#m-page-log .move-list")`
	atEnd := list + `.scrollHeight > ` + list + `.clientHeight && ` + list + `.scrollTop + ` + list + `.clientHeight >= ` + list + `.scrollHeight - 4`
	waitJS(t, ctx, "the history on its newest move", atEnd, 5*time.Second)

	clickSel(t, ctx, "#hist-first")
	waitJS(t, ctx, "the first position, the list at its top", `location.hash === "#move=0" && `+list+`.scrollTop === 0 && document.getElementById("hist-first").disabled`, 5*time.Second)
	clickSel(t, ctx, "#hist-last")
	waitJS(t, ctx, "the latest position, the list at its end", `location.hash === "" && document.getElementById("hist-last").disabled && `+atEnd, 5*time.Second)

	// Told in words, the story scrolls by itself too, not the pane around it: the
	// page opens it on its end by scrolling it, as on a computer. The Council tells
	// nothing in words, and the trick game tells its deal.
	var told createGameResponse
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "test-tricks-3", "player_name": "Alice", "seat_bots": map[string]string{"2": "random", "3": "random"}}, &told); code != 200 {
		t.Fatalf("create: %d", code)
	}
	openOnPhone(t, ctx, base, "Alice", map[string]string{told.RoomCode: "test-tricks-3"}, told.RoomCode)
	clickSel(t, ctx, `#m-nav [data-group="log"]`)
	waitJS(t, ctx, "the story told so far", `document.getElementById("history-friendly") !== null`, 5*time.Second)
	clickSel(t, ctx, "#history-friendly")
	waitJS(t, ctx, "the story", `document.querySelector("#m-page-log #events") !== null`, 5*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(function () { const ev = document.querySelector("#m-page-log #events"); for (let i = 0; i < 200; i++) { const p = document.createElement("p"); p.textContent = "line " + i; ev.append(p); } return true; })()`, nil)); err != nil {
		t.Fatal(err)
	}
	if !evalBool(t, ctx, `(function () { const ev = document.querySelector("#m-page-log #events"), pane = ev.parentElement; return ev.scrollHeight > ev.clientHeight && pane.scrollHeight <= pane.clientHeight + 1; })()`) {
		t.Error("the pane around the story scrolls, not the story")
	}
}

// On a phone the panel of the call's button lies inside the screen, and the
// games of the Active Games and Join Game windows take two lines, none of
// them wider than the window.
func TestPhoneUI_TheCallsPanelAndTheGameListsFitTheScreen(t *testing.T) {
	base := newRESTTestServer(t)
	room := councilWithHistory(t, base, 2)
	var waiting, lobby createGameResponse
	doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Yan", "public": true}, &waiting)
	doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "test-council", "player_name": "Zed", "lobby": true, "public": true}, &lobby)
	ctx, cancel := newChromedpContextWith(t, 60*time.Second, fakeMic...)
	defer cancel()
	openOnPhone(t, ctx, base, "Alice", map[string]string{room: "test-council", waiting.RoomCode: "tic-tac-toe"}, room)

	clickSel(t, ctx, `#m-nav [data-group="chat"]`)
	clickSel(t, ctx, "#voice-join")
	waitJS(t, ctx, "in the call", `document.getElementById("voice-join").getAttribute("aria-pressed") === "true"`, 10*time.Second)
	clickSel(t, ctx, "#voice-join")
	waitJS(t, ctx, "the call's panel", `!document.getElementById("voice-pop").hidden`, 5*time.Second)
	inside := `(function (el) { const r = el.getBoundingClientRect(); return r.left >= 0 && r.right <= document.documentElement.clientWidth && r.bottom <= innerHeight; })(%s)`
	if !evalBool(t, ctx, fmt.Sprintf(inside, `document.getElementById("voice-pop")`)) {
		t.Errorf("the call's panel is not inside the screen: %s", evalString(t, ctx, `JSON.stringify(document.getElementById("voice-pop").getBoundingClientRect())`))
	}
	clickSel(t, ctx, "#voice-chip")

	// A row of a list: its badges under its name, and nothing wider than the window.
	fits := `(function (sel) {
		const rows = [...document.querySelectorAll(sel)];
		return rows.length > 0 && rows.every((li) => {
			const box = li.closest("dialog").getBoundingClientRect();
			const name = li.querySelector(".grow").getBoundingClientRect();
			const badges = li.querySelector(".game-badges");
			const under = !badges || badges.getBoundingClientRect().top >= name.bottom - 1;
			return under && [...li.querySelectorAll("*")].every((el) => el.getBoundingClientRect().right <= box.right + 0.5);
		});
	})(%q)`
	clickSel(t, ctx, "#open-active-games")
	waitJS(t, ctx, "the two games, each on two lines", `document.querySelectorAll("#my-games li").length === 2 && `+fmt.Sprintf(fits, "#my-games li"), 10*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("active-games-dialog").close(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#open-join")
	waitJS(t, ctx, "the public lobby, on two lines", `document.querySelector('#join-open-games li[data-code="`+lobby.RoomCode+`"]') !== null && `+fmt.Sprintf(fits, "#join-open-games li"), 10*time.Second)
}

// On a computer a game with a map has the chat in a tab of its column, which
// hides the card's own heading: the voice chat's microphone stays there.
func TestVoiceUI_TheMicIsInTheChatTabOfAMapGame(t *testing.T) {
	base := newRESTTestServer(t)
	room := councilWithHistory(t, base, 2)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 900)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, base, room, "test-council", "Alice")
	waitJS(t, ctx, "the chat's tab", `document.getElementById("rail-tab-talk") !== null`, 10*time.Second)
	clickSel(t, ctx, "#rail-tab-talk")
	waitJS(t, ctx, "the microphone on screen", `(function () { const b = document.getElementById("voice-join"); return !!b && b.offsetParent !== null && b.getBoundingClientRect().width > 0; })()`, 5*time.Second)
	if evalBool(t, ctx, `(function () { const t = document.querySelector("#rail-pane-talk .chat-title"); return !!t && t.offsetParent !== null; })()`) {
		t.Error("the tab and the card both say Chat")
	}
}
