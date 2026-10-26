// Tests that a move made over REST is pushed to every WebSocket subscribed to the room.

package app_test

import (
	"encoding/json"
	"testing"

	"github.com/coder/websocket"
)

// wsMoveData mirrors the wire DTO docs/history/plan.md's item-31 addendum requires
// Manager.MakeMove to publish as a "move" event's Data payload — NOT the raw
// (untagged) session.MoveOutcome, and NOT its BoardState field verbatim (a
// JSON-text string, which would arrive double-encoded): "seq", "seat",
// "color", "from", "to", "captured", "turn_seat" (nullable — nil when the
// move ended the game), "game_ended", "winner_seat", "status", and
// "board_state" decoded as json.RawMessage so the test can assert it's a
// nested JSON object, not an escaped string.
type wsMoveData struct {
	Seq        int             `json:"seq"`
	Seat       int             `json:"seat"`
	Color      string          `json:"color"`
	From       string          `json:"from"`
	To         string          `json:"to"`
	Captured   *string         `json:"captured"`
	TurnSeat   *int            `json:"turn_seat"`
	GameEnded  bool            `json:"game_ended"`
	WinnerSeat *int            `json:"winner_seat"`
	Status     string          `json:"status"`
	BoardState json.RawMessage `json:"board_state"`
}

// dialSpectator dials /ws/{code} as a spectator named name, fails the test
// on a dial error, registers cleanup to close the connection, reads and
// discards the initial "joined" event (asserting its type, so a broken dial
// path fails loudly here rather than silently later), and returns the
// still-open *websocket.Conn.
func dialSpectator(t *testing.T, baseURL, roomCode, name string) *websocket.Conn {
	t.Helper()

	conn, err := dialWS(t, baseURL, wsPath(roomCode, name, true, "spectator", true))
	if err != nil {
		t.Fatalf("dialWS(/ws/%s?name=%s&role=spectator) returned error: %v, want nil", roomCode, name, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	joined := readFirstEvent(t, conn)
	if joined.Type != "joined" {
		t.Fatalf("first event type = %q, want %q (raw data: %s)", joined.Type, "joined", joined.Data)
	}

	return conn
}

// TestWSMoveBroadcast_RESTMoveObservedOnOpenWebSocket covers this backlog
// item's AC itself: with an active game and an already-open spectator
// WebSocket connection to its room (initial "joined" event already
// consumed), a separate REST call — POST /api/games/{code}/moves, a legal,
// non-capturing move — must be observed as a "move" event on that SAME,
// STILL-OPEN connection, without the test ever re-polling REST. The event's
// data must carry seq==1, the submitted seat/color/from/to, captured==null
// (this move captures nothing), turn_seat reflecting the new turn (seat 2,
// Bob/white, since Alice/black just moved), status=="active",
// game_ended==false, and board_state decoded as a nested JSON object (not
// an escaped string) reflecting the move (occupant moved from "2,0" to
// "1,0").
func TestWSMoveBroadcast_RESTMoveObservedOnOpenWebSocket(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	conn := dialSpectator(t, baseURL, roomCode, "Watcher")

	resp, raw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	}, "")
	if resp.StatusCode != 200 {
		t.Fatalf("POST /api/games/%s/moves status = %d, want 200 (body: %s)", roomCode, resp.StatusCode, raw)
	}

	// Block-read the NEXT message off the still-open connection — this is
	// what proves "no polling required": no REST re-query happens between
	// the move above and this read.
	ev := readFirstEvent(t, conn)

	if ev.Type != "move" {
		t.Fatalf("event type = %q, want %q (raw data: %s)", ev.Type, "move", ev.Data)
	}

	var data wsMoveData
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		t.Fatalf("decode move event data: %v (raw: %s)", err, ev.Data)
	}

	if data.Seq != 1 {
		t.Errorf("data.seq = %d, want 1", data.Seq)
	}
	if data.Seat != 1 {
		t.Errorf("data.seat = %d, want 1", data.Seat)
	}
	if data.Color != "black" {
		t.Errorf("data.color = %q, want %q", data.Color, "black")
	}
	if data.From != "2,0" {
		t.Errorf("data.from = %q, want %q", data.From, "2,0")
	}
	if data.To != "1,0" {
		t.Errorf("data.to = %q, want %q", data.To, "1,0")
	}
	if data.Captured != nil {
		t.Errorf("data.captured = %v, want nil (JSON null; nothing was at 1,0)", *data.Captured)
	}
	if data.TurnSeat == nil || *data.TurnSeat != 2 {
		t.Errorf("data.turn_seat = %v, want 2 (Bob/white's turn next)", data.TurnSeat)
	}
	if data.GameEnded {
		t.Errorf("data.game_ended = %v, want false", data.GameEnded)
	}
	if data.WinnerSeat != nil {
		t.Errorf("data.winner_seat = %v, want nil", *data.WinnerSeat)
	}
	if data.Status != "active" {
		t.Errorf("data.status = %q, want %q", data.Status, "active")
	}

	if len(data.BoardState) == 0 {
		t.Fatalf("data.board_state is empty (raw event data: %s)", ev.Data)
	}
	trimmed := trimLeadingWhitespace(data.BoardState)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		t.Fatalf("data.board_state does not start with '{' (looks like a JSON string, not a nested object): %s", data.BoardState)
	}

	occupants := decodeOccupants(t, data.BoardState)
	if _, stillAtOld := occupants["2,0"]; stillAtOld {
		t.Errorf("data.board_state still has an occupant at \"2,0\" after moving away from it: %+v", occupants)
	}
	got, ok := occupants["1,0"]
	if !ok {
		t.Fatalf("data.board_state has no occupant at \"1,0\" after moving there: %+v", occupants)
	}
	if got != (occupant{Piece: "stone", Color: "black"}) {
		t.Errorf("data.board_state[\"1,0\"] = %+v, want {Piece: stone, Color: black}", got)
	}
}

// TestWSMoveBroadcast_AllSubscribersReceiveEvent covers the bonus
// broadcast-fanout case: two independent spectator WebSocket connections to
// the SAME room (each past its own "joined" event) must BOTH receive the
// "move" event resulting from a single REST move call — proving this is a
// genuine broadcast to every subscriber, not merely a reply routed back to
// whichever connection happened to make (or accompany) the REST call.
func TestWSMoveBroadcast_AllSubscribersReceiveEvent(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	connA := dialSpectator(t, baseURL, roomCode, "WatcherA")
	connB := dialSpectator(t, baseURL, roomCode, "WatcherB")

	resp, raw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	}, "")
	if resp.StatusCode != 200 {
		t.Fatalf("POST /api/games/%s/moves status = %d, want 200 (body: %s)", roomCode, resp.StatusCode, raw)
	}

	evA := readFirstEvent(t, connA)
	if evA.Type != "move" {
		t.Fatalf("connA event type = %q, want %q (raw data: %s)", evA.Type, "move", evA.Data)
	}
	var dataA wsMoveData
	if err := json.Unmarshal(evA.Data, &dataA); err != nil {
		t.Fatalf("decode connA move event data: %v (raw: %s)", err, evA.Data)
	}
	if dataA.Seq != 1 || dataA.From != "2,0" || dataA.To != "1,0" {
		t.Errorf("connA move data = %+v, want seq=1 from=2,0 to=1,0", dataA)
	}

	evB := readFirstEvent(t, connB)
	if evB.Type != "move" {
		t.Fatalf("connB event type = %q, want %q (raw data: %s)", evB.Type, "move", evB.Data)
	}
	var dataB wsMoveData
	if err := json.Unmarshal(evB.Data, &dataB); err != nil {
		t.Fatalf("decode connB move event data: %v (raw: %s)", err, evB.Data)
	}
	if dataB.Seq != 1 || dataB.From != "2,0" || dataB.To != "1,0" {
		t.Errorf("connB move data = %+v, want seq=1 from=2,0 to=1,0", dataB)
	}
}
