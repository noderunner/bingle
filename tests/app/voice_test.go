package app_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // coturn's credential scheme
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
)

// voicePeers is the body of the voice endpoints.
type voicePeers struct {
	Peers []struct {
		Seat int    `json:"seat"`
		Name string `json:"name"`
	} `json:"peers"`
}

func postVoice(t *testing.T, base, room, name string, active bool) (int, voicePeers) {
	t.Helper()
	var out voicePeers
	code := doJSON(t, "POST", base+"/api/games/"+room+"/voice", map[string]any{"player_name": name, "active": active}, &out)
	return code, out
}

func signal(t *testing.T, base, room, from string, to int, kind string, data any) (int, map[string]any) {
	t.Helper()
	var out map[string]any
	code := doJSON(t, "POST", base+"/api/games/"+room+"/voice/signal", map[string]any{"player_name": from, "to": to, "kind": kind, "data": data}, &out)
	return code, out
}

// roomConn is a WebSocket of a room with a goroutine reading it into a channel:
// a read that times out would close the connection, so tests wait on the channel.
type roomConn struct {
	events chan wsEvent
}

// dialRoom opens the room's WebSocket, reads the "joined" event and starts collecting the rest.
func dialRoom(t *testing.T, base, room, name, role string) *roomConn {
	t.Helper()
	return dialRoomWith(t, base, room, name, role, "&only=voice")
}

// dialRoomWith is dialRoom with extra query parameters ("" for the ordinary socket of a page).
func dialRoomWith(t *testing.T, base, room, name, role, extra string) *roomConn {
	t.Helper()
	u := "ws" + strings.TrimPrefix(base, "http") + "/ws/" + room + "?name=" + url.QueryEscape(name) + "&role=" + role + extra
	ctx, cancel := context.WithCancel(context.Background())
	dctx, dcancel := context.WithTimeout(ctx, wsDialTimeout)
	defer dcancel()
	c, _, err := websocket.Dial(dctx, u, nil)
	if err != nil {
		cancel()
		t.Fatalf("dial %s as %s: %v", room, name, err)
	}
	t.Cleanup(func() { cancel(); _ = c.CloseNow() })
	var ev wsEvent
	if err := wsjson.Read(dctx, c, &ev); err != nil || ev.Type != "joined" {
		t.Fatalf("first event = %+v, %v; want joined", ev, err)
	}
	rc := &roomConn{events: make(chan wsEvent, 256)}
	go func() {
		for {
			var ev wsEvent
			if err := wsjson.Read(ctx, c, &ev); err != nil {
				close(rc.events)
				return
			}
			rc.events <- ev
		}
	}()
	return rc
}

// nextEvent waits for an event of the wanted type (others are skipped).
func nextEvent(t *testing.T, c *roomConn, typ string, wait time.Duration) (wsEvent, bool) {
	t.Helper()
	timer := time.After(wait)
	for {
		select {
		case ev, ok := <-c.events:
			if !ok {
				return wsEvent{}, false
			}
			if ev.Type == typ {
				return ev, true
			}
		case <-timer:
			return wsEvent{}, false
		}
	}
}

func TestVoice_JoinListLeave(t *testing.T) {
	base := newRESTTestServer(t)
	room := newTakRoom(t, base, "tic-tac-toe", "Alice")
	joinRoom(t, base, room, "Bob")

	// The room hears who joins and leaves.
	watcher := dialRoom(t, base, room, "Bob", "player")

	if code, got := postVoice(t, base, room, "Alice", true); code != 200 || len(got.Peers) != 1 || got.Peers[0].Seat != 1 || got.Peers[0].Name != "Alice" {
		t.Fatalf("Alice joins: %d %+v", code, got)
	}
	ev, ok := nextEvent(t, watcher, "voice", 3*time.Second)
	if !ok || !strings.Contains(string(ev.Data), `"kind":"join"`) || !strings.Contains(string(ev.Data), `"seat":1`) {
		t.Fatalf("join event = %+v, %v", ev, ok)
	}
	// Renewing is not news.
	postVoice(t, base, room, "Alice", true)
	if code, got := postVoice(t, base, room, "bob", true); code != 200 || len(got.Peers) != 2 {
		t.Fatalf("Bob joins: %d %+v", code, got)
	}
	var list voicePeers
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/voice", nil, &list); code != 200 || len(list.Peers) != 2 || list.Peers[1].Name != "Bob" {
		t.Fatalf("list: %d %+v", code, list)
	}
	if code, got := postVoice(t, base, room, "Alice", false); code != 200 || len(got.Peers) != 1 {
		t.Fatalf("Alice leaves: %d %+v", code, got)
	}
	if ev, ok := nextEvent(t, watcher, "voice", 3*time.Second); !ok {
		t.Fatal("no event")
	} else if !strings.Contains(string(ev.Data), "join") { // Bob's own join arrives first
		t.Fatalf("event = %s", ev.Data)
	}
	if ev, ok := nextEvent(t, watcher, "voice", 3*time.Second); !ok || !strings.Contains(string(ev.Data), `"kind":"leave"`) {
		t.Fatalf("leave event = %+v, %v", ev, ok)
	}
}

func TestVoice_OnlyPlayersJoin(t *testing.T) {
	base := newRESTTestServer(t)
	room := newTakRoom(t, base, "tic-tac-toe", "Alice")
	joinRoom(t, base, room, "Bob")
	if code, _ := postVoice(t, base, room, "Mallory", true); code != 403 {
		t.Errorf("a stranger joined the call: %d, want 403", code)
	}
	if code, _ := postVoice(t, base, "ZZZZZZ", "Alice", true); code != 404 {
		t.Errorf("an unknown room: %d, want 404", code)
	}
	if code, _ := postVoice(t, base, room, "", true); code != 400 {
		t.Errorf("no name: %d, want 400", code)
	}
}

// A message reaches the seat it is for and nobody else; the sender must be in
// the call, and so must the receiver.
func TestVoice_SignalReachesOnlyItsTarget(t *testing.T) {
	base := newRESTTestServer(t)
	room := newTakRoom(t, base, "cthulhu-wars-3p", "Alice")
	joinRoom(t, base, room, "Bob")
	joinRoom(t, base, room, "Cy")
	other := newTakRoom(t, base, "tic-tac-toe", "Alice")
	joinRoom(t, base, other, "Bob")

	alice := dialRoom(t, base, room, "Alice", "player")
	bob := dialRoom(t, base, room, "Bob", "player")
	cy := dialRoom(t, base, room, "Cy", "player")
	spec := dialRoom(t, base, room, "Sam", "spectator")
	elsewhere := dialRoom(t, base, other, "Bob", "player") // Bob has seat 2 there too

	// Not in the call yet.
	if code, _ := signal(t, base, room, "Alice", 2, "offer", map[string]any{"sdp": "x"}); code != 403 {
		t.Errorf("signalling before joining: %d, want 403", code)
	}
	postVoice(t, base, room, "Alice", true)
	if code, _ := signal(t, base, room, "Alice", 2, "offer", map[string]any{"sdp": "x"}); code != 409 {
		t.Errorf("signalling a seat that is not in the call: %d, want 409", code)
	}
	postVoice(t, base, room, "Bob", true)
	postVoice(t, base, room, "Cy", true)

	if code, _ := signal(t, base, room, "Alice", 2, "offer", map[string]any{"type": "offer", "sdp": "v=0 secret"}); code != 200 {
		t.Fatalf("signal: %d", code)
	}
	ev, ok := nextEvent(t, bob, "rtc", 3*time.Second)
	if !ok {
		t.Fatal("Bob did not receive the offer")
	}
	var got struct {
		From int             `json:"from"`
		Kind string          `json:"kind"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(ev.Data, &got); err != nil || got.From != 1 || got.Kind != "offer" || !strings.Contains(string(got.Data), "v=0 secret") {
		t.Errorf("rtc event = %s (%v)", ev.Data, err)
	}
	// Nobody else hears it: not Alice, not Cy, not a spectator, not another game's Bob.
	for name, c := range map[string]*roomConn{"Alice": alice, "Cy": cy, "spectator": spec, "other game": elsewhere} {
		if ev, ok := nextEvent(t, c, "rtc", 400*time.Millisecond); ok {
			t.Errorf("%s received %s", name, ev.Data)
		}
	}
	// And it is only for the seat addressed: Cy answers Alice.
	signal(t, base, room, "Cy", 1, "ice", map[string]any{"candidate": "c"})
	if ev, ok := nextEvent(t, alice, "rtc", 3*time.Second); !ok || !strings.Contains(string(ev.Data), `"from":3`) {
		t.Errorf("Alice got %+v, %v", ev, ok)
	}
	if ev, ok := nextEvent(t, bob, "rtc", 300*time.Millisecond); ok {
		t.Errorf("Bob overheard %s", ev.Data)
	}
}

func TestVoice_SignalIsChecked(t *testing.T) {
	base := newRESTTestServer(t)
	room := newTakRoom(t, base, "tic-tac-toe", "Alice")
	joinRoom(t, base, room, "Bob")
	postVoice(t, base, room, "Alice", true)
	postVoice(t, base, room, "Bob", true)

	if code, _ := signal(t, base, room, "Alice", 2, "shout", map[string]any{"a": 1}); code != 400 {
		t.Errorf("unknown kind: %d, want 400", code)
	}
	if code, _ := signal(t, base, room, "Alice", 1, "offer", map[string]any{"a": 1}); code != 400 {
		t.Errorf("to yourself: %d, want 400", code)
	}
	if code, _ := signal(t, base, room, "Alice", 2, "offer", nil); code != 400 {
		t.Errorf("no data: %d, want 400", code)
	}
	if code, _ := signal(t, base, room, "Alice", 2, "offer", strings.Repeat("x", 20000)); code != 400 {
		t.Errorf("a message over the limit: %d, want 400", code)
	}
	if code, _ := signal(t, base, room, "Mallory", 2, "offer", map[string]any{"a": 1}); code != 403 {
		t.Errorf("a stranger signalling: %d, want 403", code)
	}
	// Flooding is limited.
	limited := false
	for i := 0; i < 400; i++ {
		if code, _ := signal(t, base, room, "Alice", 2, "ice", map[string]any{"i": i}); code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("400 messages in a burst were all relayed, want a limit (429)")
	}
	// Leaving takes the seat out of the call, so nothing is relayed to it.
	postVoice(t, base, room, "Bob", false)
	if code, _ := signal(t, base, room, "Alice", 2, "offer", map[string]any{"a": 1}); code != 409 && code != 429 {
		t.Errorf("signalling someone who left: %d, want 409", code)
	}
}

// The STUN and TURN settings reach the client; the TURN password is the
// HMAC-SHA1 of a user name that carries its own expiry (coturn's scheme).
func TestVoice_RTCConfig(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.RTCStun = []string{config.DefaultRTCStun}
	cfg.RTCTurnURLs = []string{"turn:turn.example.com:3478?transport=udp", "turns:turn.example.com:5349"}
	cfg.RTCTurnSecret = "s3cret"
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)

	var out struct {
		ICEServers []struct {
			URLs       []string `json:"urls"`
			Username   string   `json:"username"`
			Credential string   `json:"credential"`
		} `json:"ice_servers"`
	}
	if code := doJSON(t, "GET", srv.URL+"/api/rtc/config", nil, &out); code != 200 || len(out.ICEServers) != 2 {
		t.Fatalf("config: %d %+v", code, out)
	}
	if s := out.ICEServers[0]; len(s.URLs) != 1 || s.URLs[0] != config.DefaultRTCStun || s.Username != "" || s.Credential != "" {
		t.Errorf("stun entry = %+v", s)
	}
	turn := out.ICEServers[1]
	if len(turn.URLs) != 2 {
		t.Errorf("turn urls = %v", turn.URLs)
	}
	mac := hmac.New(sha1.New, []byte("s3cret"))
	mac.Write([]byte(turn.Username))
	if want := base64.StdEncoding.EncodeToString(mac.Sum(nil)); turn.Credential != want {
		t.Errorf("credential = %q, want %q", turn.Credential, want)
	}
	exp, err := strconv.ParseInt(strings.SplitN(turn.Username, ":", 2)[0], 10, 64)
	if err != nil || time.Until(time.Unix(exp, 0)) < 5*time.Hour || time.Until(time.Unix(exp, 0)) > 7*time.Hour {
		t.Errorf("username %q should carry an expiry about 6 hours ahead (%v)", turn.Username, err)
	}

	// Without the settings there is no relay and no secret in the answer.
	base := newRESTTestServer(t)
	var plain struct {
		ICEServers []map[string]any `json:"ice_servers"`
	}
	if code := doJSON(t, "GET", base+"/api/rtc/config", nil, &plain); code != 200 || plain.ICEServers == nil || len(plain.ICEServers) != 0 {
		t.Errorf("default config: %d %+v, want an empty list", code, plain)
	}
}

func TestVoice_GamePageSetsPermissionsPolicy(t *testing.T) {
	base := newRESTTestServer(t)
	resp, err := http.Get(base + "/game/ABCDEF")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got := resp.Header.Get("Permissions-Policy"); got != "microphone=(self), camera=()" {
		t.Errorf("Permissions-Policy = %q", got)
	}
}

// A call has a socket of its own (only=voice) that carries the call and nothing
// else; the socket of a page no longer carries the messages between the browsers.
func TestVoice_TheCallsSocketCarriesTheCallAndThePagesSocketDoesNot(t *testing.T) {
	base := newRESTTestServer(t)
	room := newTakRoom(t, base, "tic-tac-toe", "Alice")
	joinRoom(t, base, room, "Bob")

	call := dialRoom(t, base, room, "Bob", "player")
	page := dialRoomWith(t, base, room, "Bob", "player", "")
	if code, _ := postVoice(t, base, room, "Alice", true); code != 200 {
		t.Fatalf("alice joins: %d", code)
	}
	if code, _ := postVoice(t, base, room, "Bob", true); code != 200 {
		t.Fatalf("bob joins: %d", code)
	}
	if code, _ := signal(t, base, room, "Alice", 2, "offer", map[string]any{"sdp": "x"}); code != 200 {
		t.Fatalf("signal: %d", code)
	}
	if _, ok := nextEvent(t, call, "rtc", 3*time.Second); !ok {
		t.Fatal("the call's socket did not get the message for Bob's seat")
	}
	// The page hears who is in the call, but not the messages between the browsers.
	if _, ok := nextEvent(t, page, "voice", 3*time.Second); !ok {
		t.Error("the page's socket did not hear who joined")
	}
	if ev, ok := nextEvent(t, page, "rtc", 500*time.Millisecond); ok {
		t.Errorf("the page's socket got a call message: %+v", ev)
	}
	// A move reaches the page and not the call.
	if code, out := moveNotation(t, base, room, "Alice", "a1"); code != 200 {
		t.Fatalf("move: %d %v", code, out)
	}
	if _, ok := nextEvent(t, page, "move", 3*time.Second); !ok {
		t.Error("the page did not hear the move")
	}
	if ev, ok := nextEvent(t, call, "move", 500*time.Millisecond); ok {
		t.Errorf("the call's socket got a move: %+v", ev)
	}
}
