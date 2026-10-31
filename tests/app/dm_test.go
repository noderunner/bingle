package app_test

// Direct messages, direct calls and who is here are accounts' (ADR 0073):
// every test signs in.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
)

type dmResp struct {
	ID   int64  `json:"id"`
	From string `json:"from"`
	Name string `json:"name"`
	To   string `json:"to"`
	Text string `json:"text"`
	At   string `json:"at"`
}

// membersServer is a server whose tests make accounts of their own.
func membersServer(t *testing.T) (*app.App, string) {
	t.Helper()
	a, err := app.New(newTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return a, srv.URL
}

// member makes the account @handle and returns a browser signed in to it.
func member(t *testing.T, a *app.App, base, handle string) *browser {
	t.Helper()
	pw, err := a.Accounts().Create(context.Background(), handle, handle+"@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	b := newBrowser(t, base)
	if code := b.login(handle, pw); code != 200 {
		t.Fatalf("@%s signs in: %d", handle, code)
	}
	if code := b.do("POST", "/auth/password", map[string]any{"old": pw, "new": handle + "'s own password"}, nil); code != 200 {
		t.Fatalf("@%s sets a password: %d", handle, code)
	}
	return b
}

// here says the browser's account is about, and returns its address.
func (b *browser) here() string {
	b.t.Helper()
	var out struct {
		Address string `json:"address"`
	}
	if code := b.do("POST", "/api/here", map[string]any{}, &out); code != 200 || out.Address == "" {
		b.t.Fatalf("here: %d %+v", code, out)
	}
	return out.Address
}

// Two people who are here write to each other: the message is kept for both of
// them, the correspondence can be read back, and the list of them names the
// other person.
func TestDirectMessages_TwoPeopleWriteToEachOther(t *testing.T) {
	a, baseURL := membersServer(t)
	anaB, boB := member(t, a, baseURL, "ana"), member(t, a, baseURL, "bob")
	ana, bo := anaB.here(), boB.here()

	var msg dmResp
	if code := anaB.do("POST", "/api/dm", map[string]any{"to": bo, "text": "Fancy a game of Hex?"}, &msg); code != 200 {
		t.Fatalf("Ana writes to Bo: %d", code)
	}
	if msg.ID == 0 || msg.From != ana || msg.To != bo || msg.Name != "@ana" || msg.Text != "Fancy a game of Hex?" {
		t.Fatalf("the message = %+v", msg)
	}
	// Bo reads it, and answers.
	var read struct {
		Messages []dmResp `json:"messages"`
	}
	if code := boB.do("POST", "/api/dm/history", map[string]any{"with": ana}, &read); code != 200 {
		t.Fatalf("Bo reads: %d", code)
	}
	if len(read.Messages) != 1 || read.Messages[0].Text != "Fancy a game of Hex?" {
		t.Fatalf("Bo sees %+v", read.Messages)
	}
	if code := boB.do("POST", "/api/dm", map[string]any{"to": ana, "text": "Go on then"}, nil); code != 200 {
		t.Fatalf("Bo answers: %d", code)
	}
	// Ana sees both, in order, and only after the first when she asks for that.
	anaB.do("POST", "/api/dm/history", map[string]any{"with": bo}, &read)
	if len(read.Messages) != 2 || read.Messages[1].Name != "@bob" {
		t.Fatalf("Ana sees %+v", read.Messages)
	}
	var after struct {
		Messages []dmResp `json:"messages"`
	}
	anaB.do("POST", "/api/dm/history", map[string]any{"with": bo, "after": read.Messages[0].ID}, &after)
	if len(after.Messages) != 1 || after.Messages[0].Text != "Go on then" {
		t.Errorf("after the first message: %+v", after.Messages)
	}
	// The list of correspondences names the other person and the last thing said.
	var threads struct {
		Address string `json:"address"`
		Threads []struct {
			With   string `json:"with"`
			Name   string `json:"name"`
			Text   string `json:"text"`
			Count  int    `json:"count"`
			LastID int64  `json:"last_id"`
		} `json:"threads"`
	}
	if code := anaB.do("POST", "/api/dm/threads", map[string]any{}, &threads); code != 200 {
		t.Fatalf("Ana's threads: %d", code)
	}
	if threads.Address != ana || len(threads.Threads) != 1 {
		t.Fatalf("Ana's threads = %+v", threads)
	}
	if th := threads.Threads[0]; th.With != bo || th.Name != "@bob" || th.Text != "Go on then" || th.Count != 2 {
		t.Errorf("the correspondence = %+v", th)
	}
	// A third person sees nothing of it.
	cyB := member(t, a, baseURL, "cyd")
	cyB.here()
	var cy struct {
		Messages []dmResp `json:"messages"`
	}
	cyB.do("POST", "/api/dm/history", map[string]any{"with": ana}, &cy)
	if len(cy.Messages) != 0 {
		t.Errorf("Cy can read somebody else's correspondence: %+v", cy.Messages)
	}
}

// Writing is refused where it could not work or should not be allowed: from a
// guest, from an account not here, to somebody who is not here, to yourself,
// with nothing in it, or faster than anybody types.
func TestDirectMessages_AreRefusedWhereTheyShouldBe(t *testing.T) {
	a, baseURL := membersServer(t)
	anaB, boB := member(t, a, baseURL, "ana"), member(t, a, baseURL, "bob")
	ana, bo := anaB.here(), boB.here()
	away := member(t, a, baseURL, "cyd")
	guest := newBrowser(t, baseURL)
	for name, want := range map[string]struct {
		b    *browser
		body map[string]any
		code int
	}{
		"a guest":              {guest, map[string]any{"to": bo, "text": "hi"}, http.StatusForbidden},
		"an account not here":  {away, map[string]any{"to": bo, "text": "hi"}, 403},
		"somebody not here":    {anaB, map[string]any{"to": strings.Repeat("0", 16), "text": "hi"}, 404},
		"writing to yourself":  {anaB, map[string]any{"to": ana, "text": "hi"}, 404},
		"an empty message":     {anaB, map[string]any{"to": bo, "text": "   "}, 400},
		"a message of a novel": {anaB, map[string]any{"to": bo, "text": strings.Repeat("x", 501)}, 400},
	} {
		if code := want.b.do("POST", "/api/dm", want.body, nil); code != want.code {
			t.Errorf("%s: %d, want %d", name, code, want.code)
		}
	}
	// A guest is not listed, and sees no list.
	if code := guest.do("POST", "/api/here", map[string]any{}, nil); code != http.StatusForbidden {
		t.Errorf("a guest says it is here: %d, want 403", code)
	}
	if code := guest.do("GET", "/api/here", nil, nil); code != http.StatusForbidden {
		t.Errorf("a guest reads who is here: %d, want 403", code)
	}
	// A flood is refused rather than delivered.
	refused := 0
	for i := 0; i < 20; i++ {
		if code := anaB.do("POST", "/api/dm", map[string]any{"to": bo, "text": "hi"}, nil); code == 429 {
			refused++
		}
	}
	if refused == 0 {
		t.Error("twenty messages in a row were all delivered")
	}
}

// A person is in one voice call at a time: joining a direct call takes them out
// of a game's call, and the game is told.
func TestDirectCall_OneVoiceAtATime(t *testing.T) {
	a, baseURL := membersServer(t)
	anaB, boB := member(t, a, baseURL, "ana"), member(t, a, baseURL, "bob")
	var created struct {
		RoomCode string `json:"room_code"`
	}
	anaB.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, &created)
	room := created.RoomCode
	boB.do("POST", "/api/games/"+room+"/join", map[string]any{"player_name": "Bo"}, nil)
	ana, bo := anaB.here(), boB.here()

	// Both are in the game's call.
	for _, who := range []struct {
		name string
		b    *browser
	}{{"Ana", anaB}, {"Bo", boB}} {
		if code := who.b.do("POST", "/api/games/"+room+"/voice", map[string]any{"player_name": who.name, "active": true}, nil); code != 200 {
			t.Fatalf("%s joins the game's call: %d", who.name, code)
		}
	}
	var call struct {
		Peers []struct {
			Seat int    `json:"seat"`
			Name string `json:"name"`
		} `json:"peers"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/voice", nil, &call)
	if len(call.Peers) != 2 {
		t.Fatalf("%d in the game's call, want two", len(call.Peers))
	}

	// Ana rings Bo instead: she leaves the game's call by doing so.
	if code := anaB.do("POST", "/api/dm/call", map[string]any{"to": bo, "join": true}, nil); code != 200 {
		t.Fatalf("Ana calls Bo: %d", code)
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/voice", nil, &call)
	if len(call.Peers) != 1 || call.Peers[0].Name != "Bo" {
		t.Errorf("the game's call holds %+v, want Bo alone", call.Peers)
	}

	// A WebRTC message needs a call with that person, and is refused otherwise.
	if code := boB.do("POST", "/api/dm/signal", map[string]any{"to": ana, "kind": "offer", "data": map[string]any{"sdp": "x"}}, nil); code != 409 {
		t.Errorf("Bo signals before answering: %d, want 409", code)
	}
	if code := boB.do("POST", "/api/dm/call", map[string]any{"to": ana, "join": true}, nil); code != 200 {
		t.Fatalf("Bo answers: %d", code)
	}
	if code := boB.do("POST", "/api/dm/signal", map[string]any{"to": ana, "kind": "offer", "data": map[string]any{"sdp": "x"}}, nil); code != 200 {
		t.Errorf("Bo signals in the call: %d", code)
	}
	// Bo answering took him out of the game's call too, so nobody is left in it.
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/voice", nil, &call)
	if len(call.Peers) != 0 {
		t.Errorf("the game's call holds %+v, want nobody", call.Peers)
	}
	// Hanging up ends it; a signal afterwards is refused again.
	if code := anaB.do("POST", "/api/dm/call", map[string]any{"join": false}, nil); code != 200 {
		t.Fatalf("Ana hangs up: %d", code)
	}
	if code := anaB.do("POST", "/api/dm/signal", map[string]any{"to": bo, "kind": "ice", "data": map[string]any{"candidate": "x"}}, nil); code != 409 {
		t.Errorf("a signal after hanging up: %d, want 409", code)
	}
	// A call to somebody who is not here is refused.
	if code := anaB.do("POST", "/api/dm/call", map[string]any{"to": strings.Repeat("f", 16), "join": true}, nil); code != 404 {
		t.Errorf("calling nobody: %d, want 404", code)
	}
}

// openAs opens the home page in a browser signed in to the account @handle.
func openAs(t *testing.T, ctx context.Context, a *app.App, baseURL, handle string) {
	t.Helper()
	b := member(t, a, baseURL, handle)
	signInChrome(t, ctx, baseURL, b.cookie(auth.SessionCookie))
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/"),
		chromedp.WaitVisible("#open-messages", chromedp.ByID),
	); err != nil {
		t.Fatalf("open the page as @%s: %v", handle, err)
	}
}

// Two people in two browsers find each other in the direct messages, write to
// each other and see the badge count what has not been read.
func TestWebDirectMessages_TwoBrowsersWriteToEachOther(t *testing.T) {
	a, baseURL := membersServer(t)
	ana, cancelA := newChromedpContext(t, 90*time.Second)
	defer cancelA()
	bo, cancelB := newChromedpContext(t, 90*time.Second)
	defer cancelB()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ana, &mu, &jsErrors)
	openAs(t, ana, a, baseURL, "ana")
	openAs(t, bo, a, baseURL, "bob")

	// Each sees the other among the people who are here.
	clickSel(t, ana, "#open-messages")
	waitJS(t, ana, "Bo in the list", `[...document.querySelectorAll("#dm-people .dm-person")].some(b => b.textContent.includes("@bob"))`, 20*time.Second)
	if evalBool(t, ana, `[...document.querySelectorAll("#dm-people .dm-person")].some(b => b.textContent.includes("@ana"))`) {
		t.Error("Ana is offered as somebody to write to herself")
	}
	// Ana writes to Bo.
	clickSel(t, ana, `#dm-people .dm-person`)
	waitJS(t, ana, "the correspondence", `document.getElementById("dm-title").textContent === "With @bob" && !document.getElementById("dm-input").disabled`, 10*time.Second)
	if err := chromedp.Run(ana, chromedp.Evaluate(`(function () {
		var i = document.getElementById("dm-input");
		i.value = "Fancy a game of Hex?";
		document.getElementById("dm-send").click();
		return true;
	})()`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ana, "the message on Ana's side", `[...document.querySelectorAll("#dm-log .chat-line")].some(li => li.textContent.includes("Fancy a game of Hex?"))`, 10*time.Second)

	// Bo's badge counts it, and his window shows it when he opens it.
	waitJS(t, bo, "Bo's badge", `!document.getElementById("dm-badge").hidden && document.getElementById("dm-badge").textContent === "1"`, 15*time.Second)
	clickSel(t, bo, "#open-messages")
	waitJS(t, bo, "Ana in Bo's list", `[...document.querySelectorAll("#dm-people .dm-person")].some(b => b.textContent.includes("@ana"))`, 10*time.Second)
	clickSel(t, bo, `#dm-people .dm-person[data-address]`)
	waitJS(t, bo, "the message on Bo's side", `[...document.querySelectorAll("#dm-log .chat-line")].some(li => li.textContent.includes("Fancy a game of Hex?"))`, 10*time.Second)
	waitJS(t, bo, "Bo's badge cleared", `document.getElementById("dm-badge").hidden === true`, 10*time.Second)
	// Bo answers, and Ana sees it without doing anything.
	if err := chromedp.Run(bo, chromedp.Evaluate(`(function () {
		var i = document.getElementById("dm-input");
		i.value = "Go on then";
		document.getElementById("dm-send").click();
		return true;
	})()`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ana, "Bo's answer", `[...document.querySelectorAll("#dm-log .chat-line")].some(li => li.textContent.includes("Go on then"))`, 15*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// A voice call inside a direct message: one rings, the other answers, and the
// two browsers end up connected to each other.
func TestWebDirectCall_TwoBrowsersTalk(t *testing.T) {
	a, baseURL := membersServer(t)
	ana, cancelA := newChromedpContextWith(t, 120*time.Second, fakeMic...)
	defer cancelA()
	bo, cancelB := newChromedpContextWith(t, 120*time.Second, fakeMic...)
	defer cancelB()
	openAs(t, ana, a, baseURL, "ana")
	openAs(t, bo, a, baseURL, "bob")

	clickSel(t, ana, "#open-messages")
	waitJS(t, ana, "Bo in the list", `[...document.querySelectorAll("#dm-people .dm-person")].some(b => b.textContent.includes("@bob"))`, 20*time.Second)
	clickSel(t, ana, `#dm-people .dm-person`)
	waitJS(t, ana, "the call button", `document.getElementById("dm-call") !== null && !document.getElementById("dm-call").hidden`, 10*time.Second)
	clickSel(t, ana, "#dm-call")
	waitJS(t, ana, "Ana ringing", `document.getElementById("dm-call-state").textContent.includes("Ringing")`, 10*time.Second)

	// Bo is told, and answers.
	clickSel(t, bo, "#open-messages")
	waitJS(t, bo, "Bo's Answer button", `(function () { var b = document.getElementById("dm-call"); return b && !b.hidden && b.textContent === "Answer"; })()`, 20*time.Second)
	clickSel(t, bo, "#dm-call")
	// Both ends come up: the audio is flowing when the connection says so.
	for who, ctx := range map[string]context.Context{"Ana": ana, "Bo": bo} {
		waitJS(t, ctx, who+" in the call", `document.getElementById("dm-call-state").textContent === "In the call"`, 30*time.Second)
		waitJS(t, ctx, who+" can hang up", `document.getElementById("dm-hangup") !== null && !document.getElementById("dm-hangup").hidden`, 10*time.Second)
	}
	waitJS(t, ana, "the audio of the call", `document.querySelectorAll("#dm-audio audio[data-dm-audio]").length === 1`, 30*time.Second)
	// Ana hangs up; Bo's call ends too.
	clickSel(t, ana, "#dm-hangup")
	waitJS(t, bo, "Bo's call over", `document.getElementById("dm-hangup").hidden === true`, 20*time.Second)
}

// A person talks in one place at a time: calling somebody from the direct
// messages takes them out of the voice chat of a game, and the page says so.
func TestWebDirectCall_LeavesTheGamesVoiceChat(t *testing.T) {
	a, baseURL := membersServer(t)
	anaB, boB := member(t, a, baseURL, "ana"), member(t, a, baseURL, "bob")
	var created struct {
		RoomCode string `json:"room_code"`
	}
	anaB.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Ana"}, &created)
	room := created.RoomCode
	boB.do("POST", "/api/games/"+room+"/join", map[string]any{"player_name": "Bo"}, nil)
	// Bo is about (in another browser, as far as this one knows).
	bo := boB.here()

	ctx, cancel := newChromedpContextWith(t, 120*time.Second, fakeMic...)
	defer cancel()
	signInChrome(t, ctx, baseURL, anaB.cookie(auth.SessionCookie))
	openGameAs(t, ctx, baseURL, room, "tic-tac-toe", "Ana")
	waitJS(t, ctx, "the voice card", `document.getElementById("voice-join") !== null`, 15*time.Second)
	clickSel(t, ctx, "#voice-join")
	waitJS(t, ctx, "Ana in the game's call", `document.getElementById("voice-badge").textContent === "in call"`, 15*time.Second)

	// She rings Bo from the direct messages instead.
	clickSel(t, ctx, "#open-messages")
	waitJS(t, ctx, "Bo in the list", `document.querySelector('#dm-people .dm-person[data-address="`+bo+`"]') !== null`, 20*time.Second)
	clickSel(t, ctx, `#dm-people .dm-person[data-address="`+bo+`"]`)
	waitJS(t, ctx, "the call button", `!document.getElementById("dm-call").hidden`, 10*time.Second)
	clickSel(t, ctx, "#dm-call")
	waitJS(t, ctx, "Ana ringing Bo", `document.getElementById("dm-call-state").textContent.includes("Ringing")`, 10*time.Second)

	// The game's call has let her go, and the page says why.
	clickSel(t, ctx, "#messages-dialog .dialog-close")
	waitJS(t, ctx, "out of the game's call", `document.getElementById("voice-join") !== null && document.getElementById("voice-badge").textContent !== "in call"`, 20*time.Second)
	var call struct {
		Peers []struct {
			Name string `json:"name"`
		} `json:"peers"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/voice", nil, &call)
	for _, p := range call.Peers {
		if p.Name == "Ana" {
			t.Errorf("Ana is still in the game's call: %+v", call.Peers)
		}
	}
}
