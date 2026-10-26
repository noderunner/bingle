// Tests of the move history endpoint: ordering, and the since_seq filter.

package app_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// getMoves GETs baseURL+"/api/games/"+code+"/moves", optionally with a raw
// query string (e.g. "since_seq=1", no leading "?"; pass "" for none), and
// returns the raw HTTP response along with its body bytes.
func getMoves(t *testing.T, baseURL, code, rawQuery string) (*http.Response, []byte) {
	t.Helper()
	path := "/api/games/" + code + "/moves"
	if rawQuery != "" {
		path += "?" + rawQuery
	}
	return getJSON(t, baseURL, path)
}

// submitThreeMoves drives exactly three sequential, legal, non-capturing
// moves against roomCode (created via setupActiveGame, so Alice holds seat
// 1/black and Bob holds seat 2/white), alternating black/white/black per
// demo.json's round-robin turn order, and returns the three moveResult
// values from each POST's response ("move" object), in submission order
// (seq 1, 2, 3). Fails the test immediately if any of the three moves
// doesn't come back 200.
func submitThreeMoves(t *testing.T, baseURL, roomCode string) []moveResult {
	t.Helper()

	type step struct {
		playerName string
		from       string
		to         string
	}
	steps := []step{
		{playerName: "Alice", from: "2,0", to: "1,0"}, // seq 1, seat 1/black
		{playerName: "Bob", from: "2,8", to: "1,8"},   // seq 2, seat 2/white
		{playerName: "Alice", from: "1,0", to: "2,0"}, // seq 3, seat 1/black
	}

	results := make([]moveResult, 0, len(steps))
	for i, s := range steps {
		resp, raw := postMove(t, baseURL, roomCode, map[string]string{
			"player_name": s.playerName,
			"from":        s.from,
			"to":          s.to,
		}, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("setup move #%d (%s %s->%s) status = %d, want %d (body: %s)",
				i+1, s.playerName, s.from, s.to, resp.StatusCode, http.StatusOK, raw)
		}
		var body makeMoveResponse
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode setup move #%d response: %v (body: %s)", i+1, err, raw)
		}
		results = append(results, body.Move)
	}
	return results
}

// TestGetMoves_AfterThreeMoves_ReturnsExactlyThreeInAscendingSeq covers this
// backlog item's AC itself: after exactly 3 submitted moves, GET
// /api/games/{code}/moves returns 200 with a JSON array of EXACTLY 3
// entries, seq values 1, 2, 3 in that order, and each entry's
// seat/color/from/to matching what was actually submitted for that move (in
// order). It also confirms every entry's "captured" is JSON null, since all
// three moves here are non-capturing (covers this backlog item's
// captured-is-null requirement).
func TestGetMoves_AfterThreeMoves_ReturnsExactlyThreeInAscendingSeq(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)
	submitted := submitThreeMoves(t, baseURL, roomCode)

	resp, raw := getMoves(t, baseURL, roomCode, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	var history []moveResult
	if err := json.Unmarshal(raw, &history); err != nil {
		t.Fatalf("decode response body as []moveResult: %v (body: %s)", err, raw)
	}

	if len(history) != 3 {
		t.Fatalf("len(history) = %d, want exactly 3 (body: %s)", len(history), raw)
	}

	wantSeqs := []int{1, 2, 3}
	for i, entry := range history {
		if entry.Seq != wantSeqs[i] {
			t.Errorf("history[%d].seq = %d, want %d (ascending order; body: %s)", i, entry.Seq, wantSeqs[i], raw)
		}
		want := submitted[i]
		if entry.Seat != want.Seat {
			t.Errorf("history[%d].seat = %d, want %d (matching submitted move #%d)", i, entry.Seat, want.Seat, i+1)
		}
		if entry.Color != want.Color {
			t.Errorf("history[%d].color = %q, want %q (matching submitted move #%d)", i, entry.Color, want.Color, i+1)
		}
		if entry.From != want.From {
			t.Errorf("history[%d].from = %q, want %q (matching submitted move #%d)", i, entry.From, want.From, i+1)
		}
		if entry.To != want.To {
			t.Errorf("history[%d].to = %q, want %q (matching submitted move #%d)", i, entry.To, want.To, i+1)
		}
		if entry.Captured != nil {
			t.Errorf("history[%d].captured = %v, want nil (all three moves in this test are non-capturing)", i, *entry.Captured)
		}
	}

	// Confirm "captured" is literally JSON null in the wire payload for at
	// least the first entry, not merely a coincidentally-nil pointer from a
	// missing key (mirroring TestPostMove_LegalMove_Returns200WithMoveAndState's
	// style in rest_make_move_test.go).
	var asSlice []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asSlice); err != nil {
		t.Fatalf("decode response body as []map[string]json.RawMessage: %v (body: %s)", err, raw)
	}
	if len(asSlice) != 3 {
		t.Fatalf("len(asSlice) = %d, want 3 (body: %s)", len(asSlice), raw)
	}
	for i, entry := range asSlice {
		capturedRaw, ok := entry["captured"]
		if !ok {
			t.Errorf("history[%d] missing \"captured\" key entirely (entry: %v)", i, entry)
			continue
		}
		if string(capturedRaw) != "null" {
			t.Errorf("history[%d].captured raw JSON = %s, want literal null", i, capturedRaw)
		}
	}
}

// TestGetMoves_SinceSeq1_ReturnsOnlyLaterMoves covers ?since_seq=1 after the
// same 3 moves: only entries with seq > 1 (seq 2 and seq 3) come back, in
// that order.
func TestGetMoves_SinceSeq1_ReturnsOnlyLaterMoves(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)
	submitted := submitThreeMoves(t, baseURL, roomCode)

	resp, raw := getMoves(t, baseURL, roomCode, "since_seq=1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	var history []moveResult
	if err := json.Unmarshal(raw, &history); err != nil {
		t.Fatalf("decode response body as []moveResult: %v (body: %s)", err, raw)
	}

	if len(history) != 2 {
		t.Fatalf("len(history) = %d, want exactly 2 (seq 2 and 3 only; body: %s)", len(history), raw)
	}
	if history[0].Seq != 2 {
		t.Errorf("history[0].seq = %d, want 2", history[0].Seq)
	}
	if history[1].Seq != 3 {
		t.Errorf("history[1].seq = %d, want 3", history[1].Seq)
	}
	if history[0].From != submitted[1].From || history[0].To != submitted[1].To {
		t.Errorf("history[0] = %+v, want to match submitted move #2 (%+v)", history[0], submitted[1])
	}
	if history[1].From != submitted[2].From || history[1].To != submitted[2].To {
		t.Errorf("history[1] = %+v, want to match submitted move #3 (%+v)", history[1], submitted[2])
	}
}

// TestGetMoves_SinceSeq0_ReturnsAll covers ?since_seq=0 explicitly returning
// all 3 moves (seq 0 is never a real move, so nothing is excluded).
func TestGetMoves_SinceSeq0_ReturnsAll(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)
	submitThreeMoves(t, baseURL, roomCode)

	resp, raw := getMoves(t, baseURL, roomCode, "since_seq=0")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	var history []moveResult
	if err := json.Unmarshal(raw, &history); err != nil {
		t.Fatalf("decode response body as []moveResult: %v (body: %s)", err, raw)
	}
	if len(history) != 3 {
		t.Fatalf("len(history) = %d, want exactly 3 (body: %s)", len(history), raw)
	}
	for i, wantSeq := range []int{1, 2, 3} {
		if history[i].Seq != wantSeq {
			t.Errorf("history[%d].seq = %d, want %d", i, history[i].Seq, wantSeq)
		}
	}
}

// TestGetMoves_SinceSeqOmitted_ReturnsAll covers omitting since_seq entirely
// (no query string at all) behaving the same as ?since_seq=0: all 3 moves
// come back.
func TestGetMoves_SinceSeqOmitted_ReturnsAll(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)
	submitThreeMoves(t, baseURL, roomCode)

	resp, raw := getMoves(t, baseURL, roomCode, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	var history []moveResult
	if err := json.Unmarshal(raw, &history); err != nil {
		t.Fatalf("decode response body as []moveResult: %v (body: %s)", err, raw)
	}
	if len(history) != 3 {
		t.Fatalf("len(history) = %d, want exactly 3 (body: %s)", len(history), raw)
	}
}

// TestGetMoves_SinceSeqBeyondLast_ReturnsEmptyArray covers ?since_seq=99
// (beyond the last move) returning an EMPTY array ("[]", not "null", not an
// error) — no moves exist past that point, but that is not a "not found"
// condition, so this must still be 200.
func TestGetMoves_SinceSeqBeyondLast_ReturnsEmptyArray(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)
	submitThreeMoves(t, baseURL, roomCode)

	resp, raw := getMoves(t, baseURL, roomCode, "since_seq=99")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	trimmed := strings.TrimSpace(string(raw))
	if trimmed != "[]" {
		t.Errorf("body = %q, want literal \"[]\" (empty JSON array, not null and not an error)", trimmed)
	}

	var history []moveResult
	if err := json.Unmarshal(raw, &history); err != nil {
		t.Fatalf("decode response body as []moveResult: %v (body: %s)", err, raw)
	}
	if len(history) != 0 {
		t.Errorf("len(history) = %d, want 0", len(history))
	}
}

// TestGetMoves_NewGameNoMoves_ReturnsEmptyArray covers a brand-new game with
// ZERO moves made yet (not even activated — only the creator has joined):
// GET .../moves returns 200 with an empty array, not an error.
func TestGetMoves_NewGameNoMoves_ReturnsEmptyArray(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	resp, raw := getMoves(t, baseURL, roomCode, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	trimmed := strings.TrimSpace(string(raw))
	if trimmed != "[]" {
		t.Errorf("body = %q, want literal \"[]\" (empty JSON array, not null and not an error)", trimmed)
	}

	var history []moveResult
	if err := json.Unmarshal(raw, &history); err != nil {
		t.Fatalf("decode response body as []moveResult: %v (body: %s)", err, raw)
	}
	if len(history) != 0 {
		t.Errorf("len(history) = %d, want 0", len(history))
	}
}

// TestGetMoves_UnknownRoomCode_Returns404 covers an unknown room code -> 404
// with a non-empty {"error"} body, mirroring the other endpoints' same rule
// (docs/history/plan.md section 6.8's statusForError mapping via store.ErrNotFound).
func TestGetMoves_UnknownRoomCode_Returns404(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp, raw := getMoves(t, baseURL, "ZZZZZZ", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusNotFound, raw)
	}

	var body errorResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}
	if strings.TrimSpace(body.Error) == "" {
		t.Errorf("error field = %q, want a non-empty error message (body: %s)", body.Error, raw)
	}
}
