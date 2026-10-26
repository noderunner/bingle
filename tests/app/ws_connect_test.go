// Tests of the room WebSocket: the joined event comes first for players, spectators and reconnects, and bad requests are refused before the upgrade.

package app_test

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// wsDialTimeout bounds how long a single Dial attempt (success or expected
// failure) may take, so a hung handshake fails the test instead of hanging
// the whole suite.
const wsDialTimeout = 5 * time.Second

// wsReadTimeout bounds how long reading the first message off an
// established connection may take.
const wsReadTimeout = 5 * time.Second

// wsEvent mirrors the server->client WebSocket message envelope from
// docs/history/plan.md's "WebSocket message envelope" section:
//
//	type Event struct {
//	    Type string `json:"type"`
//	    Data any    `json:"data"`
//	}
//
// Data is decoded lazily (as json.RawMessage) so each test can unmarshal it
// into whatever shape it expects (session.JoinResult's fields, which carry
// no json struct tags of their own, so their wire keys are "Role"/"Seat"/
// "Color"/"Status" unless the handler wraps them in a tagged DTO — decoding
// into a struct tagged "role"/"seat"/"color"/"status" below still works
// either way, since encoding/json's Unmarshal falls back to a
// case-insensitive field/tag match).
type wsEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// wsJoinedData mirrors session.JoinResult's fields for decoding a "joined"
// event's Data payload. Tagged with lowercase keys; encoding/json's
// case-insensitive fallback matching means this decodes correctly whether
// the handler sends session.JoinResult verbatim (wire keys "Role"/"Seat"/
// "Color"/"Status") or a lowercase-tagged DTO mirroring the REST join
// response shape.
type wsJoinedData struct {
	Role   string `json:"role"`
	Seat   int    `json:"seat"`
	Color  string `json:"color"`
	Status string `json:"status"`
}

// wsURL rewrites baseURL's scheme from http(s) to ws(s) and appends path
// (which should start with "/" and may include a query string), returning
// the full dial target.
func wsURL(baseURL, path string) string {
	u := baseURL
	u = strings.Replace(u, "http://", "ws://", 1)
	u = strings.Replace(u, "https://", "wss://", 1)
	return u + path
}

// wsPath builds "/ws/{code}?name=...&role=..." from the given room code and
// query values, properly URL-encoding name/role. Passing role == "" omits
// the "role" query key entirely (as opposed to sending role=""), and
// passing includeName == false omits "name" entirely rather than sending
// name="" — both are distinct edge cases callers may want to exercise
// separately.
func wsPath(code string, name string, includeName bool, role string, includeRole bool) string {
	q := url.Values{}
	if includeName {
		q.Set("name", name)
	}
	if includeRole {
		q.Set("role", role)
	}
	return "/ws/" + code + "?" + q.Encode()
}

// dialWS attempts a WebSocket dial against baseURL+path and returns the
// resulting *websocket.Conn (nil on failure) and error. Callers that expect
// success must close the returned conn (e.g. via t.Cleanup(func() {
// _ = conn.CloseNow() })).
func dialWS(t *testing.T, baseURL, path string) (*websocket.Conn, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), wsDialTimeout)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(baseURL, path), nil)
	return conn, err
}

// readFirstEvent reads exactly one JSON message off conn, decoded as a
// wsEvent, failing the test if the read errors or times out.
func readFirstEvent(t *testing.T, conn *websocket.Conn) wsEvent {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), wsReadTimeout)
	defer cancel()

	var ev wsEvent
	if err := wsjson.Read(ctx, conn, &ev); err != nil {
		t.Fatalf("wsjson.Read (first message): %v", err)
	}
	return ev
}

// TestWSConnect_PlayerFillsLastSeat_ReceivesJoinedEventFirst covers this
// backlog item's AC itself: create a room (creator "Alice", seat 1), then
// dial /ws/{code}?name=Bob&role=player. The very first message read off the
// connection must be {"type": "joined", "data": {"role": "player", "seat":
// 2, "color": "white", "status": "active"}} — Bob fills the last seat
// (demo.json's seat 2, color "white"), which also activates the game.
func TestWSConnect_PlayerFillsLastSeat_ReceivesJoinedEventFirst(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	conn, err := dialWS(t, baseURL, wsPath(roomCode, "Bob", true, "player", true))
	if err != nil {
		t.Fatalf("dialWS(/ws/%s?name=Bob&role=player) returned error: %v, want nil", roomCode, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	ev := readFirstEvent(t, conn)

	if ev.Type != "joined" {
		t.Fatalf("first event type = %q, want %q (raw data: %s)", ev.Type, "joined", ev.Data)
	}

	var data wsJoinedData
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		t.Fatalf("decode joined event data: %v (raw: %s)", err, ev.Data)
	}

	if data.Role != "player" {
		t.Errorf("data.role = %q, want %q", data.Role, "player")
	}
	if data.Seat != 2 {
		t.Errorf("data.seat = %d, want 2", data.Seat)
	}
	if data.Color != "white" {
		t.Errorf("data.color = %q, want %q", data.Color, "white")
	}
	if data.Status != "active" {
		t.Errorf("data.status = %q, want %q", data.Status, "active")
	}
}

// TestWSConnect_Spectator_ReceivesJoinedEventFirst covers the spectator
// path: dialing /ws/{code}?name=Dana&role=spectator on a fresh (still
// "waiting") room must also receive a "joined" event first, with
// role == "spectator" and seat/color zero-valued (0 / ""), matching
// JoinGame's spectator semantics (no seat assignment, no DB write).
func TestWSConnect_Spectator_ReceivesJoinedEventFirst(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	conn, err := dialWS(t, baseURL, wsPath(roomCode, "Dana", true, "spectator", true))
	if err != nil {
		t.Fatalf("dialWS(/ws/%s?name=Dana&role=spectator) returned error: %v, want nil", roomCode, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	ev := readFirstEvent(t, conn)

	if ev.Type != "joined" {
		t.Fatalf("first event type = %q, want %q (raw data: %s)", ev.Type, "joined", ev.Data)
	}

	var data wsJoinedData
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		t.Fatalf("decode joined event data: %v (raw: %s)", err, ev.Data)
	}

	if data.Role != "spectator" {
		t.Errorf("data.role = %q, want %q", data.Role, "spectator")
	}
	if data.Status != "waiting" {
		t.Errorf("data.status = %q, want %q (game had only the creator seated)", data.Status, "waiting")
	}
	if data.Seat != 0 {
		t.Errorf("data.seat = %d, want 0 (spectator has no seat)", data.Seat)
	}
	if data.Color != "" {
		t.Errorf("data.color = %q, want empty (spectator has no color)", data.Color)
	}
}

// TestWSConnect_SpectatorOnActiveGame_ReceivesJoinedEventFirst is a second
// data point for the spectator case, joining after the game has already
// activated (both seats filled), confirming the joined event's status
// echoes the game's real current status ("active", not "waiting").
func TestWSConnect_SpectatorOnActiveGame_ReceivesJoinedEventFirst(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	joinResp, joinRaw := postJoin(t, baseURL, roomCode, map[string]string{
		"player_name": "Bob",
		"role":        "player",
	}, "")
	if joinResp.StatusCode != 200 {
		t.Fatalf("setup join status = %d, want 200 (body: %s)", joinResp.StatusCode, joinRaw)
	}

	conn, err := dialWS(t, baseURL, wsPath(roomCode, "Eve", true, "spectator", true))
	if err != nil {
		t.Fatalf("dialWS(/ws/%s?name=Eve&role=spectator) returned error: %v, want nil", roomCode, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	ev := readFirstEvent(t, conn)

	if ev.Type != "joined" {
		t.Fatalf("first event type = %q, want %q (raw data: %s)", ev.Type, "joined", ev.Data)
	}

	var data wsJoinedData
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		t.Fatalf("decode joined event data: %v (raw: %s)", err, ev.Data)
	}

	if data.Role != "spectator" {
		t.Errorf("data.role = %q, want %q", data.Role, "spectator")
	}
	if data.Status != "active" {
		t.Errorf("data.status = %q, want %q", data.Status, "active")
	}
}

// TestWSConnect_ReconnectExistingName_ReceivesJoinedEventForExistingSeat
// covers the reconnect-by-name path (mirroring JoinGame's REST coverage):
// dialing /ws/{code}?name=Alice&role=player on a room Alice already created
// (she holds seat 1) must reattach her to seat 1, not treat her as a new
// player — the joined event must show seat 1 / color "black" (demo.json's
// seat 1 color) and must NOT change the game's status (still "waiting",
// since only seat 1 was ever filled).
func TestWSConnect_ReconnectExistingName_ReceivesJoinedEventForExistingSeat(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	conn, err := dialWS(t, baseURL, wsPath(roomCode, "Alice", true, "player", true))
	if err != nil {
		t.Fatalf("dialWS(/ws/%s?name=Alice&role=player) returned error: %v, want nil", roomCode, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	ev := readFirstEvent(t, conn)

	if ev.Type != "joined" {
		t.Fatalf("first event type = %q, want %q (raw data: %s)", ev.Type, "joined", ev.Data)
	}

	var data wsJoinedData
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		t.Fatalf("decode joined event data: %v (raw: %s)", err, ev.Data)
	}

	if data.Role != "player" {
		t.Errorf("data.role = %q, want %q", data.Role, "player")
	}
	if data.Seat != 1 {
		t.Errorf("data.seat = %d, want 1 (reconnecting to the same name's existing seat)", data.Seat)
	}
	if data.Color != "black" {
		t.Errorf("data.color = %q, want %q", data.Color, "black")
	}
	if data.Status != "waiting" {
		t.Errorf("data.status = %q, want %q (reconnect must not change game status)", data.Status, "waiting")
	}
}

// TestWSConnect_UnknownRoomCode_FailsHandshake covers docs/history/plan.md's
// "Room code not found" edge case surfacing over WS: dialing a room code
// that was never created must fail the HTTP upgrade itself (a pre-upgrade
// http.Error, mapped via statusForError from store.ErrNotFound to 404) —
// websocket.Dial must return a non-nil error, and no *websocket.Conn should
// ever be usable.
func TestWSConnect_UnknownRoomCode_FailsHandshake(t *testing.T) {
	baseURL := newRESTTestServer(t)

	conn, err := dialWS(t, baseURL, wsPath("ZZZZZZ", "Eve", true, "player", true))
	if conn != nil {
		_ = conn.CloseNow()
	}
	if err == nil {
		t.Fatal("dialWS(/ws/ZZZZZZ?name=Eve&role=player) returned nil error, want a non-nil dial/handshake error for an unknown room code")
	}
}

// TestWSConnect_BlankName_FailsHandshake covers docs/history/plan.md section 6.9's
// "same parsing/validation as handleJoinGame" rule: a blank (or entirely
// missing) name query param must fail the upgrade with a pre-upgrade 400,
// exactly like handleJoinGame's blank-player_name rule — websocket.Dial
// must return a non-nil error.
func TestWSConnect_BlankName_FailsHandshake(t *testing.T) {
	cases := []struct {
		name        string
		includeName bool
		nameValue   string
	}{
		{name: "empty string name", includeName: true, nameValue: ""},
		{name: "whitespace-only name", includeName: true, nameValue: "   "},
		{name: "name query key entirely absent", includeName: false, nameValue: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL := newRESTTestServer(t)
			roomCode := createRoom(t, baseURL, "Alice")

			conn, err := dialWS(t, baseURL, wsPath(roomCode, tc.nameValue, tc.includeName, "player", true))
			if conn != nil {
				_ = conn.CloseNow()
			}
			if err == nil {
				t.Fatalf("dialWS(/ws/%s?...) with blank/absent name returned nil error, want a non-nil dial/handshake error", roomCode)
			}
		})
	}
}

// TestWSConnect_InvalidRole_FailsHandshake covers an invalid "role" query
// value (neither "player" nor "spectator") failing the same way
// handleJoinGame rejects an invalid "role" body field: a pre-upgrade 400,
// so websocket.Dial must return a non-nil error.
func TestWSConnect_InvalidRole_FailsHandshake(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	conn, err := dialWS(t, baseURL, wsPath(roomCode, "Frank", true, "referee", true))
	if conn != nil {
		_ = conn.CloseNow()
	}
	if err == nil {
		t.Fatalf("dialWS(/ws/%s?name=Frank&role=referee) returned nil error, want a non-nil dial/handshake error for an invalid role", roomCode)
	}
}

// TestWSConnect_SeatsFull_FailsHandshake covers docs/history/plan.md's ErrSeatsFull
// edge case surfacing over WS: once both seats are filled (Alice + Bob), a
// THIRD distinct player name dialing with role=player must fail the
// handshake (pre-upgrade error mapped from session.ErrSeatsFull to 409) —
// websocket.Dial must return a non-nil error, mirroring the REST 409.
func TestWSConnect_SeatsFull_FailsHandshake(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	fillResp, fillRaw := postJoin(t, baseURL, roomCode, map[string]string{
		"player_name": "Bob",
		"role":        "player",
	}, "")
	if fillResp.StatusCode != 200 {
		t.Fatalf("setup join status = %d, want 200 (body: %s)", fillResp.StatusCode, fillRaw)
	}

	conn, err := dialWS(t, baseURL, wsPath(roomCode, "Carol", true, "player", true))
	if conn != nil {
		_ = conn.CloseNow()
	}
	if err == nil {
		t.Fatalf("dialWS(/ws/%s?name=Carol&role=player) on a seats-full room returned nil error, want a non-nil dial/handshake error", roomCode)
	}
}
