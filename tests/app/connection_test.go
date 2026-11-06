package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/coder/websocket"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
)

// newConnectionServer is a server that watches its sockets at the given times.
func newConnectionServer(t *testing.T, times config.Connection) string {
	t.Helper()
	cfg := newTestConfig(t)
	cfg.Connection = times
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	registerFixtures(t, a)
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return srv.URL
}

func connectedSeats(t *testing.T, base, room string) []int {
	t.Helper()
	var got struct {
		Seats []int `json:"seats"`
	}
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/connections", nil, &got); code != 200 {
		t.Fatalf("connections: %d", code)
	}
	return got.Seats
}

func waitConnected(t *testing.T, base, room string, want ...int) {
	t.Helper()
	if want == nil {
		want = []int{}
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := connectedSeats(t, base, room)
		if slices.Equal(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("connected seats = %v, want %v", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// dialPlaying opens a socket for name and keeps reading it, as a page does, so
// that pings are answered.
func dialPlaying(t *testing.T, base, room, name, role string) *websocket.Conn {
	t.Helper()
	conn, err := dialWS(t, base, wsPath(room, name, true, role, true))
	if err != nil {
		t.Fatalf("dial %s: %v", name, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	go func() {
		for {
			if _, _, err := conn.Read(context.Background()); err != nil {
				return
			}
		}
	}()
	return conn
}

// readConnectionEvents collects the "connection" events of a socket on a channel.
func readConnectionEvents(t *testing.T, base, room, name string) <-chan string {
	t.Helper()
	conn, err := dialWS(t, base, wsPath(room, name, true, "spectator", true))
	if err != nil {
		t.Fatalf("dial %s: %v", name, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	out := make(chan string, 64)
	go func() {
		for {
			_, b, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			var ev struct {
				Type string `json:"type"`
				Data struct {
					Seat      int  `json:"seat"`
					Connected bool `json:"connected"`
				} `json:"data"`
			}
			if json.Unmarshal(b, &ev) == nil && ev.Type == "connection" {
				out <- fmt.Sprintf("%d:%v", ev.Data.Seat, ev.Data.Connected)
			}
		}
	}()
	return out
}

func wantConnectionEvent(t *testing.T, events <-chan string, want string) {
	t.Helper()
	select {
	case got := <-events:
		if got != want {
			t.Fatalf("connection event %s, want %s", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no connection event, want %s", want)
	}
}

// A closed socket is noticed at once, with nothing happening in the room to
// show it up, and the seat goes after the grace period. A socket used to linger
// until the room's next event failed to write to it.
func TestConnection_ClosingThePageDisconnectsTheSeat(t *testing.T) {
	base := newConnectionServer(t, config.Connection{Grace: 50 * time.Millisecond})
	room := newTakRoom(t, base, "gomoku", "Alice")
	joinRoom(t, base, room, "Bob")
	if got := connectedSeats(t, base, room); len(got) != 0 {
		t.Fatalf("a fresh room has %v connected", got)
	}
	events := readConnectionEvents(t, base, room, "watcher")

	alice := dialPlaying(t, base, room, "Alice", "player")
	wantConnectionEvent(t, events, "1:true")
	waitConnected(t, base, room, 1)

	_ = alice.Close(websocket.StatusNormalClosure, "")
	wantConnectionEvent(t, events, "1:false")
	waitConnected(t, base, room)
}

// A spectator's socket connects nobody, and a second tab is harmless.
func TestConnection_CountsSocketsOfTheSeatOnly(t *testing.T) {
	base := newConnectionServer(t, config.Connection{Grace: 20 * time.Millisecond})
	room := newTakRoom(t, base, "gomoku", "Alice")
	joinRoom(t, base, room, "Bob")

	dialPlaying(t, base, room, "Bob", "spectator")
	if got := connectedSeats(t, base, room); len(got) != 0 {
		t.Errorf("a spectator connected seats %v", got)
	}

	first := dialPlaying(t, base, room, "Alice", "player")
	second := dialPlaying(t, base, room, "Alice", "player")
	waitConnected(t, base, room, 1)
	_ = first.Close(websocket.StatusNormalClosure, "")
	time.Sleep(150 * time.Millisecond)
	waitConnected(t, base, room, 1)
	_ = second.Close(websocket.StatusNormalClosure, "")
	waitConnected(t, base, room)
}

// A reload within the grace period flags nobody: nobody hears of it at all.
func TestConnection_AReloadWithinTheGraceIsNotADisconnection(t *testing.T) {
	base := newConnectionServer(t, config.Connection{Grace: time.Hour})
	room := newTakRoom(t, base, "gomoku", "Alice")
	joinRoom(t, base, room, "Bob")
	events := readConnectionEvents(t, base, room, "watcher")

	alice := dialPlaying(t, base, room, "Alice", "player")
	wantConnectionEvent(t, events, "1:true")
	_ = alice.Close(websocket.StatusNormalClosure, "")
	time.Sleep(100 * time.Millisecond)
	dialPlaying(t, base, room, "Alice", "player")
	time.Sleep(100 * time.Millisecond)

	waitConnected(t, base, room, 1)
	select {
	case got := <-events:
		t.Errorf("the room heard %s about a reload", got)
	default:
	}
}

// The "joined" event says who is connected, the joiner included.
func TestConnection_JoinedSaysWhoIsConnected(t *testing.T) {
	base := newConnectionServer(t, config.Connection{})
	room := newTakRoom(t, base, "gomoku", "Alice")
	joinRoom(t, base, room, "Bob")
	dialPlaying(t, base, room, "Alice", "player")
	waitConnected(t, base, room, 1)

	bob, err := dialWS(t, base, wsPath(room, "Bob", true, "player", true))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bob.CloseNow() }()
	ev := readFirstEvent(t, bob)
	var data struct {
		Seat      int   `json:"seat"`
		Connected []int `json:"connected"`
	}
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		t.Fatal(err)
	}
	if ev.Type != "joined" || data.Seat != 2 || !slices.Equal(data.Connected, []int{1, 2}) {
		t.Errorf("first event %s %+v, want joined at seat 2 with seats 1 and 2 connected", ev.Type, data)
	}
}

// A connection that dies silently is found by the ping: nothing closes this
// socket, it only stops answering, because a client answers pings while reading.
func TestConnection_ASocketThatStopsAnsweringIsClosed(t *testing.T) {
	times := config.Connection{PingEvery: 30 * time.Millisecond, PingTimeout: 30 * time.Millisecond, Grace: 20 * time.Millisecond}
	base := newConnectionServer(t, times)
	room := newTakRoom(t, base, "gomoku", "Alice")
	joinRoom(t, base, room, "Bob")

	silent, err := dialWS(t, base, wsPath(room, "Alice", true, "player", true))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = silent.CloseNow() }()
	dialPlaying(t, base, room, "Bob", "player")

	waitConnected(t, base, room, 2)
	time.Sleep(300 * time.Millisecond)
	// Well past many pings, the one that answers them is still there.
	waitConnected(t, base, room, 2)
}

// Changing seats before the game takes the connection along, and leaving the
// table ends it, with the socket still open in both cases.
func TestConnection_FollowsThePlayerAroundTheLobby(t *testing.T) {
	base := newConnectionServer(t, config.Connection{Grace: time.Hour})
	room := newTakRoom(t, base, "cthulhu-wars", "Alice")
	dialPlaying(t, base, room, "Alice", "player")
	waitConnected(t, base, room, 1)

	if code := doJSON(t, "POST", base+"/api/games/"+room+"/seat", map[string]any{"player_name": "Alice", "seat": 3}, nil); code != 200 {
		t.Fatalf("choose seat: %d", code)
	}
	waitConnected(t, base, room, 3)

	joinRoom(t, base, room, "Bob")
	dialPlaying(t, base, room, "Bob", "player")
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/leave", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatalf("leave: %d", code)
	}
	waitConnected(t, base, room, 3)
}

// A pie-rule swap changes who sits where with both sockets still open: the
// connection stays with the person, not the seat number.
func TestConnection_FollowsThePlayerThroughAPieSwap(t *testing.T) {
	base := newConnectionServer(t, config.Connection{Grace: time.Hour})
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "yavalath", "player_name": "Ana", "options": map[string]any{"pie-rule": true}}
	if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	joinRoom(t, base, room, "Bob")
	dialPlaying(t, base, room, "Ana", "player")
	waitConnected(t, base, room, 1)

	var legal struct {
		Moves []struct {
			Notation string `json:"notation"`
		} `json:"options"`
	}
	doJSON(t, "GET", base+"/api/games/"+room+"/selections", nil, &legal)
	if code, out := moveNotation(t, base, room, "Ana", legal.Moves[0].Notation); code != 200 {
		t.Fatalf("first move: %d %v", code, out)
	}
	if code, out := moveNotation(t, base, room, "Bob", "swap"); code != 200 {
		t.Fatalf("swap: %d %v", code, out)
	}
	waitConnected(t, base, room, 2)
}

// The page shows a player as disconnected, in place, and shows them back again.
func TestWebConnection_OthersSeeAPlayerGoAndComeBack(t *testing.T) {
	base := newConnectionServer(t, config.Connection{Grace: 50 * time.Millisecond})
	room := newTakRoom(t, base, "gomoku", "Alice")
	joinRoom(t, base, room, "Bob")

	bobCtx, cancelB := newChromedpContext(t, 60*time.Second)
	defer cancelB()
	openGameAs(t, bobCtx, base, room, "gomoku", "Bob")

	seat1 := `document.querySelector('#seat-1 .seat-conn').textContent`
	seat2 := `document.querySelector('#seat-2 .seat-conn').textContent`
	waitJS(t, bobCtx, "Alice shows as disconnected", seat1+` === "disconnected"`, 10*time.Second)
	if got := evalString(t, bobCtx, seat2); got != "" {
		t.Errorf("Bob's own seat says %q", got)
	}
	if got := evalInt(t, bobCtx, `document.querySelectorAll('.presence-toggle, .eye').length`); got != 0 {
		t.Errorf("%d pieces of the old present/away switch are left", got)
	}

	// A connection coming and going leaves the page alone, so a card being read stays open.
	evalString(t, bobCtx, `document.querySelector('#seat-1').dataset.same = "yes"; "ok"`)

	aliceCtx, cancelA := newChromedpContext(t, 60*time.Second)
	openGameAs(t, aliceCtx, base, room, "gomoku", "Alice")
	waitJS(t, bobCtx, "Alice shows as back", seat1+` === "" && !document.querySelector('#seat-1').classList.contains("disconnected")`, 10*time.Second)
	if got := evalString(t, bobCtx, `document.querySelector('#seat-1').dataset.same || ""`); got != "yes" {
		t.Error("the page was re-rendered for a connection")
	}

	if err := chromedp.Cancel(aliceCtx); err != nil {
		t.Logf("closing Alice's browser: %v", err)
	}
	cancelA()
	waitJS(t, bobCtx, "Alice shows as gone again", seat1+` === "disconnected" && document.querySelector('#seat-1').classList.contains("disconnected")`, 10*time.Second)
}
