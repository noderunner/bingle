// Tests that closing one WebSocket does not disturb the others or the game, even with connections coming and going during moves.

package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// readEventWithTimeout reads exactly one JSON message off conn within
// wsReadTimeout, returning the decoded wsEvent or an error. Unlike
// readFirstEvent (ws_connect_test.go), it does NOT call any *testing.T
// method — safe to call from a goroutine other than the test's own, where
// calling t.Fatal/t.FailNow is documented as unsafe.
func readEventWithTimeout(conn *websocket.Conn) (wsEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wsReadTimeout)
	defer cancel()

	var ev wsEvent
	err := wsjson.Read(ctx, conn, &ev)
	return ev, err
}

// doMove POSTs a move request directly (mirroring postMove in
// rest_make_move_test.go) without touching *testing.T at all, so it's safe
// to call from a non-test goroutine.
func doMove(baseURL, code, playerName, from, to string) (*http.Response, []byte, error) {
	payload, err := json.Marshal(map[string]string{
		"player_name": playerName,
		"from":        from,
		"to":          to,
	})
	if err != nil {
		return nil, nil, err
	}

	resp, err := http.Post(baseURL+"/api/games/"+code+"/moves", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return resp, nil, err
	}
	return resp, buf.Bytes(), nil
}

// TestWSDisconnect_ClosingOneConnection_OtherStillReceivesMoveEvent covers
// this backlog item's AC core behavioral proof: with TWO spectator
// WebSocket connections open on the same active game's room (both past
// their "joined" event), closing ONE of them and then making a legal REST
// move must still deliver a correct "move" event to the OTHER,
// still-open connection.
func TestWSDisconnect_ClosingOneConnection_OtherStillReceivesMoveEvent(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	connA := dialSpectator(t, baseURL, roomCode, "WatcherA")
	connB := dialSpectator(t, baseURL, roomCode, "WatcherB")

	if err := connA.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatalf("connA.Close(StatusNormalClosure): %v, want nil", err)
	}

	resp, raw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/games/%s/moves status = %d, want 200 (body: %s)", roomCode, resp.StatusCode, raw)
	}

	ev := readFirstEvent(t, connB)
	if ev.Type != "move" {
		t.Fatalf("connB event type = %q, want %q (raw data: %s)", ev.Type, "move", ev.Data)
	}

	var data wsMoveData
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		t.Fatalf("decode connB move event data: %v (raw: %s)", err, ev.Data)
	}
	if data.Seq != 1 {
		t.Errorf("connB data.seq = %d, want 1", data.Seq)
	}
	if data.Seat != 1 {
		t.Errorf("connB data.seat = %d, want 1", data.Seat)
	}
	if data.Color != "black" {
		t.Errorf("connB data.color = %q, want %q", data.Color, "black")
	}
	if data.From != "2,0" || data.To != "1,0" {
		t.Errorf("connB data.from/to = %q/%q, want 2,0/1,0", data.From, data.To)
	}
	if data.TurnSeat == nil || *data.TurnSeat != 2 {
		t.Errorf("connB data.turn_seat = %v, want 2 (Bob/white's turn next)", data.TurnSeat)
	}
	if data.Status != "active" {
		t.Errorf("connB data.status = %q, want %q", data.Status, "active")
	}
}

// TestWSDisconnect_ClosingOneConnection_StateUnaffected covers this backlog
// item's "without affecting ... persisted state" clause directly: closing
// one of two open WebSocket connections and then making a legal REST move
// must still be correctly persisted — closing a WS connection is purely a
// transport-layer event and must never influence game state.
func TestWSDisconnect_ClosingOneConnection_StateUnaffected(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	connA := dialSpectator(t, baseURL, roomCode, "WatcherA")
	_ = dialSpectator(t, baseURL, roomCode, "WatcherB")

	if err := connA.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatalf("connA.Close(StatusNormalClosure): %v, want nil", err)
	}

	resp, raw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/games/%s/moves status = %d, want 200 (body: %s)", roomCode, resp.StatusCode, raw)
	}

	state := getState(t, baseURL, roomCode)
	if state.MoveSeq != 1 {
		t.Errorf("state.move_seq = %d, want 1", state.MoveSeq)
	}
	if state.TurnSeat == nil || *state.TurnSeat != 2 {
		t.Errorf("state.turn_seat = %v, want 2", state.TurnSeat)
	}
	if state.Status != "active" {
		t.Errorf("state.status = %q, want %q", state.Status, "active")
	}

	occupants := decodeOccupants(t, state.BoardState)
	if _, stillAtOld := occupants["2,0"]; stillAtOld {
		t.Errorf("state.board_state still has an occupant at \"2,0\" after moving away from it: %+v", occupants)
	}
	got, ok := occupants["1,0"]
	if !ok {
		t.Fatalf("state.board_state has no occupant at \"1,0\" after moving there: %+v", occupants)
	}
	if got != (occupant{Piece: "stone", Color: "black"}) {
		t.Errorf("state.board_state[\"1,0\"] = %+v, want {Piece: stone, Color: black}", got)
	}
}

// TestWSDisconnect_SecondMoveAfterClose_SucceedsAndStillOpenConnReceivesIt
// covers "no error/panic from publishing after a disconnect": after closing
// one of two spectator connections and making one legal move (consumed
// above), a SECOND legal move (now the other seat's turn) must ALSO succeed
// via REST (200, not 500) and ALSO be observed correctly on the
// still-open connection. This is what would actually exercise a bug if
// hub.publish ever errored/panicked while iterating a subscriber set that
// recently had an entry removed via unsubscribe.
func TestWSDisconnect_SecondMoveAfterClose_SucceedsAndStillOpenConnReceivesIt(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	connA := dialSpectator(t, baseURL, roomCode, "WatcherA")
	connB := dialSpectator(t, baseURL, roomCode, "WatcherB")

	if err := connA.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatalf("connA.Close(StatusNormalClosure): %v, want nil", err)
	}

	// Move 1: Alice, seat 1 (black). This also gives the (already-closed)
	// connA's server-side handler goroutine its first opportunity to notice
	// the close (its next wsjson.Write fails) and run its deferred
	// unsubscribe — racing, under the hub's mutex, against this same
	// publish call.
	resp1, raw1 := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	}, "")
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("move 1 status = %d, want 200 (body: %s)", resp1.StatusCode, raw1)
	}

	ev1 := readFirstEvent(t, connB)
	if ev1.Type != "move" {
		t.Fatalf("connB event 1 type = %q, want %q (raw: %s)", ev1.Type, "move", ev1.Data)
	}
	var data1 wsMoveData
	if err := json.Unmarshal(ev1.Data, &data1); err != nil {
		t.Fatalf("decode connB move 1: %v (raw: %s)", err, ev1.Data)
	}
	if data1.Seq != 1 {
		t.Errorf("connB move 1 seq = %d, want 1", data1.Seq)
	}

	// Move 2: Bob, seat 2 (white) — it's now his turn.
	resp2, raw2 := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Bob",
		"from":        "2,8",
		"to":          "1,8",
	}, "")
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("move 2 status = %d, want 200 (body: %s) — publish must not error/panic after a subscriber disconnected", resp2.StatusCode, raw2)
	}

	ev2 := readFirstEvent(t, connB)
	if ev2.Type != "move" {
		t.Fatalf("connB event 2 type = %q, want %q (raw: %s)", ev2.Type, "move", ev2.Data)
	}
	var data2 wsMoveData
	if err := json.Unmarshal(ev2.Data, &data2); err != nil {
		t.Fatalf("decode connB move 2: %v (raw: %s)", err, ev2.Data)
	}
	if data2.Seq != 2 {
		t.Errorf("connB move 2 seq = %d, want 2", data2.Seq)
	}
	if data2.Seat != 2 {
		t.Errorf("connB move 2 seat = %d, want 2", data2.Seat)
	}
	if data2.Color != "white" {
		t.Errorf("connB move 2 color = %q, want %q", data2.Color, "white")
	}
	if data2.From != "2,8" || data2.To != "1,8" {
		t.Errorf("connB move 2 from/to = %q/%q, want 2,8/1,8", data2.From, data2.To)
	}
}

// TestWSDisconnect_ConcurrentChurnDuringMoves_NoRaceOrPanic is the stress/
// race angle: while a long-lived spectator connection stays open and a
// separate goroutine repeatedly makes legal REST moves (alternating turns
// between the room's two seated players), ANOTHER goroutine concurrently
// opens brand-new WebSocket connections to the same room and closes each
// one immediately after reading its "joined" event. This drives real
// concurrent hub.subscribe/unsubscribe calls (from the churn goroutine)
// against real concurrent hub.publish calls (from the moves), which is
// exactly the scenario this backlog item's AC is about. Run with `go test
// -race` to have any actual data race caught by the race detector, not just
// a logical assertion failure.
//
// Assertions: neither goroutine reports an error (no REST 500s, no dial/
// read failures), the long-lived connection receives every move event, in
// order, with the right seq numbers, and a final GET .../state shows
// exactly that many recorded moves.
func TestWSDisconnect_ConcurrentChurnDuringMoves_NoRaceOrPanic(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	longLived := dialSpectator(t, baseURL, roomCode, "LongLivedWatcher")

	const iterations = 20

	// Reader goroutine: drains `iterations` "move" events off the
	// long-lived connection as they arrive (concurrently with both other
	// goroutines below), so it never falls behind hubSubscriberBuffer.
	type readerResult struct {
		seqs []int
		err  error
	}
	readerDone := make(chan readerResult, 1)
	go func() {
		seqs := make([]int, 0, iterations)
		for i := 0; i < iterations; i++ {
			ev, err := readEventWithTimeout(longLived)
			if err != nil {
				readerDone <- readerResult{seqs, fmt.Errorf("reading move event %d: %w", i, err)}
				return
			}
			if ev.Type != "move" {
				readerDone <- readerResult{seqs, fmt.Errorf("event %d type = %q, want %q (raw: %s)", i, ev.Type, "move", ev.Data)}
				return
			}
			var data wsMoveData
			if err := json.Unmarshal(ev.Data, &data); err != nil {
				readerDone <- readerResult{seqs, fmt.Errorf("decoding move event %d: %w (raw: %s)", i, err, ev.Data)}
				return
			}
			seqs = append(seqs, data.Seq)
		}
		readerDone <- readerResult{seqs, nil}
	}()

	// Mover goroutine: makes `iterations` legal REST moves, alternating
	// turns between Alice (seat 1) and Bob (seat 2) by toggling each
	// player's stone back and forth between two adjacent, otherwise-empty
	// cells, so every move stays legal indefinitely.
	moveDone := make(chan error, 1)
	go func() {
		aliceCur, aliceOther := "2,0", "1,0"
		bobCur, bobOther := "2,8", "1,8"
		for i := 0; i < iterations; i++ {
			var name, from, to string
			if i%2 == 0 {
				name, from, to = "Alice", aliceCur, aliceOther
				aliceCur, aliceOther = aliceOther, aliceCur
			} else {
				name, from, to = "Bob", bobCur, bobOther
				bobCur, bobOther = bobOther, bobCur
			}
			resp, raw, err := doMove(baseURL, roomCode, name, from, to)
			if err != nil {
				moveDone <- fmt.Errorf("move %d (%s %s->%s) request error: %w", i, name, from, to, err)
				return
			}
			if resp.StatusCode != http.StatusOK {
				moveDone <- fmt.Errorf("move %d (%s %s->%s) status = %d, want 200 (body: %s)", i, name, from, to, resp.StatusCode, raw)
				return
			}
		}
		moveDone <- nil
	}()

	// Churn goroutine: repeatedly dials a brand-new spectator connection,
	// reads its "joined" event, and immediately closes it — concurrently
	// with the moves (and their publishes) above.
	churnDone := make(chan error, 1)
	go func() {
		for i := 0; i < iterations; i++ {
			name := fmt.Sprintf("Churn%d", i)
			conn, err := dialWS(t, baseURL, wsPath(roomCode, name, true, "spectator", true))
			if err != nil {
				churnDone <- fmt.Errorf("churn %d: dial: %w", i, err)
				return
			}
			ev, err := readEventWithTimeout(conn)
			if err != nil {
				_ = conn.CloseNow()
				churnDone <- fmt.Errorf("churn %d: read joined event: %w", i, err)
				return
			}
			if ev.Type != "joined" {
				_ = conn.CloseNow()
				churnDone <- fmt.Errorf("churn %d: first event type = %q, want %q (raw: %s)", i, ev.Type, "joined", ev.Data)
				return
			}
			if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
				churnDone <- fmt.Errorf("churn %d: Close: %w", i, err)
				return
			}
		}
		churnDone <- nil
	}()

	moveErr := <-moveDone
	churnErr := <-churnDone
	reader := <-readerDone

	if moveErr != nil {
		t.Errorf("mover goroutine: %v", moveErr)
	}
	if churnErr != nil {
		t.Errorf("churn goroutine: %v", churnErr)
	}
	if reader.err != nil {
		t.Errorf("reader goroutine: %v", reader.err)
	}

	if len(reader.seqs) != iterations {
		t.Fatalf("long-lived connection received %d move events, want %d (seqs: %v)", len(reader.seqs), iterations, reader.seqs)
	}
	for i, seq := range reader.seqs {
		if seq != i+1 {
			t.Errorf("reader.seqs[%d] = %d, want %d (full: %v)", i, seq, i+1, reader.seqs)
		}
	}

	state := getState(t, baseURL, roomCode)
	if state.MoveSeq != iterations {
		t.Errorf("final state.move_seq = %d, want %d", state.MoveSeq, iterations)
	}
}
