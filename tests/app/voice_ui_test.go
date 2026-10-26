package app_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// fakeMic makes Chrome answer the microphone prompt itself and feed a test tone.
var fakeMic = []chromedp.ExecAllocatorOption{
	chromedp.Flag("use-fake-ui-for-media-stream", true),
	chromedp.Flag("use-fake-device-for-media-stream", true),
}

// Two players in two browsers join the voice chat and end up connected to each
// other; muting, deafening and leaving work and are seen by the other side.
func TestVoiceUI_TwoPlayersConnect(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	alice, cancelA := newChromedpContextWith(t, 90*time.Second, fakeMic...)
	defer cancelA()
	bob, cancelB := newChromedpContextWith(t, 90*time.Second, fakeMic...)
	defer cancelB()
	openGameAs(t, alice, baseURL, room, "tic-tac-toe", "Alice")
	openGameAs(t, bob, baseURL, room, "tic-tac-toe", "Bob")

	waitJS(t, alice, "voice card", `document.getElementById("voice-join")`, 10*time.Second)
	waitJS(t, bob, "voice card", `document.getElementById("voice-join")`, 10*time.Second)

	clickSel(t, alice, "#voice-join")
	waitJS(t, alice, "Alice in the call", `document.getElementById("voice-badge").textContent === "in call"`, 10*time.Second)
	// Bob sees who is already there before joining.
	waitJS(t, bob, "the hint", `document.getElementById("voice-hint").textContent.indexOf("Alice") >= 0`, 10*time.Second)
	clickSel(t, bob, "#voice-join")

	// Both ends reach "connected" (the higher seat makes the offer).
	both := `document.querySelector('#voice-peers li[data-voice-seat="%d"]:not([data-me])') && document.querySelector('#voice-peers li[data-voice-seat="%d"]:not([data-me])').dataset.state === "connected"`
	waitJS(t, alice, "Alice connected to Bob", fmt.Sprintf(both, 2, 2), 40*time.Second)
	waitJS(t, bob, "Bob connected to Alice", fmt.Sprintf(both, 1, 1), 40*time.Second)

	// Audio really arrives: the remote element plays a live stream.
	waitJS(t, bob, "Alice's audio", `(function(){ const a = document.querySelector('audio[data-voice-audio="1"]'); return a && a.srcObject && a.srcObject.getAudioTracks().length === 1; })()`, 10*time.Second)

	// Mute: the local microphone track is switched off, and the page says so.
	clickSel(t, alice, "#voice-mute")
	waitJS(t, alice, "muted", `document.getElementById("voice-mute").textContent === "Unmute" && document.querySelector('#voice-peers li[data-me] .voice-state').textContent === "mic off"`, 5*time.Second)
	clickSel(t, alice, "#voice-mute")
	waitJS(t, alice, "unmuted", `document.querySelector('#voice-peers li[data-me] .voice-state').textContent === "mic on"`, 5*time.Second)

	// Deafen mutes the speakers (and the microphone).
	clickSel(t, bob, "#voice-deafen")
	waitJS(t, bob, "deafened", `document.querySelector('audio[data-voice-audio="1"]').muted === true && document.querySelector('#voice-peers li[data-me] .voice-state').textContent === "deafened"`, 5*time.Second)
	clickSel(t, bob, "#voice-deafen")

	// Leaving: the other side drops the peer, and the server forgets the seat.
	clickSel(t, bob, "#voice-leave")
	waitJS(t, bob, "Bob left", `document.getElementById("voice-join")`, 5*time.Second)
	waitJS(t, alice, "Bob gone", `!document.querySelector('#voice-peers li[data-voice-seat="2"]')`, 10*time.Second)
	var list voicePeers
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/voice", nil, &list)
	if len(list.Peers) != 1 || list.Peers[0].Name != "Alice" {
		t.Errorf("server's list = %+v, want only Alice", list.Peers)
	}
}

// Push to talk keeps the microphone closed until the key is held.
func TestVoiceUI_PushToTalk(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	ctx, cancel := newChromedpContextWith(t, 60*time.Second, fakeMic...)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "tic-tac-toe", "Alice")
	waitJS(t, ctx, "voice card", `document.getElementById("voice-join")`, 10*time.Second)
	clickSel(t, ctx, "#voice-join")
	waitJS(t, ctx, "in call", `document.getElementById("voice-ptt")`, 10*time.Second)
	waitJS(t, ctx, "mic on", `document.querySelector('#voice-peers li[data-me] .voice-state').textContent === "mic on"`, 5*time.Second)
	clickSel(t, ctx, "#voice-ptt")
	waitJS(t, ctx, "mic closed", `document.querySelector('#voice-peers li[data-me] .voice-state').textContent === "mic off"`, 5*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.dispatchEvent(new KeyboardEvent("keydown", {code: "KeyV", key: "v"})); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "mic open while held", `document.querySelector('#voice-peers li[data-me] .voice-state').textContent === "mic on"`, 5*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.dispatchEvent(new KeyboardEvent("keyup", {code: "KeyV", key: "v"})); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "mic closed again", `document.querySelector('#voice-peers li[data-me] .voice-state').textContent === "mic off"`, 5*time.Second)
}

// Spectators and people playing only bots get no voice card; a refused
// microphone is explained.
func TestVoiceUI_CardOnlyForPlayersAndExplainsRefusals(t *testing.T) {
	baseURL := newRESTTestServer(t)
	soloRoom := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	// A friend's seat is open: there is somebody to talk to. Against a bot there is not.
	var created struct {
		RoomCode string `json:"room_code"`
	}
	doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Zoe", "opponent": "random"}, &created)

	ctx, cancel := newChromedpContext(t, 40*time.Second) // no fake microphone: the prompt is refused
	defer cancel()
	openGameAs(t, ctx, baseURL, created.RoomCode, "tic-tac-toe", "Zoe")
	waitJS(t, ctx, "the chat", `document.getElementById("chat-card")`, 10*time.Second)
	if evalBool(t, ctx, `!!document.getElementById("voice-card")`) {
		t.Error("a voice card against a bot")
	}

	openGameAs(t, ctx, baseURL, soloRoom, "tic-tac-toe", "Alice")
	waitJS(t, ctx, "voice card", `document.getElementById("voice-join")`, 10*time.Second)
	clickSel(t, ctx, "#voice-join")
	// Headless Chrome without the flags has no microphone or denies it: either way a reason is shown.
	waitJS(t, ctx, "a reason", `document.getElementById("voice-error") && document.getElementById("voice-error").textContent.length > 10`, 15*time.Second)
	if evalString(t, ctx, `document.getElementById("voice-badge").textContent`) != "off" {
		t.Error("the badge says the call is on after a refusal")
	}
}

// Three players join in a mixed order and every pair ends up connected: the
// higher seat of each pair makes the offer, whoever came first.
func TestVoiceUI_ThreePlayerMesh(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "cthulhu-wars-3p", "player_name": "Alice", "lobby": true}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	joinRoom(t, baseURL, room, "Bob")
	joinRoom(t, baseURL, room, "Cy")
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/start", map[string]any{"player_name": "Alice"}, nil); code != 200 {
		t.Fatalf("start: %d", code)
	}

	names := []string{"Alice", "Bob", "Cy"}
	pages := map[string]context.Context{}
	for _, n := range names {
		ctx, cancel := newChromedpContextWith(t, 120*time.Second, fakeMic...)
		defer cancel()
		openGameAs(t, ctx, baseURL, room, "cthulhu-wars-3p", n)
		waitJS(t, ctx, "voice card", `document.getElementById("voice-join")`, 15*time.Second)
		pages[n] = ctx
	}
	// Seat 2 first, then 3, then 1.
	for _, n := range []string{"Bob", "Cy", "Alice"} {
		clickSel(t, pages[n], "#voice-join")
		waitJS(t, pages[n], n+" in the call", `document.getElementById("voice-badge").textContent === "in call"`, 15*time.Second)
	}
	for i, n := range names {
		for j := range names {
			if i == j {
				continue
			}
			waitJS(t, pages[n], fmt.Sprintf("%s connected to seat %d", n, j+1),
				fmt.Sprintf(`(function(){ const r = document.querySelector('#voice-peers li[data-voice-seat="%d"]:not([data-me])'); return r && r.dataset.state === "connected"; })()`, j+1), 45*time.Second)
		}
	}
}
