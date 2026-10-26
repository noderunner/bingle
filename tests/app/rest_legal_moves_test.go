// Tests of the legal-moves endpoint: the destinations from a square for corner, edge and empty squares.

package app_test

import (
	"encoding/json"
	"net/http"
	"sort"
	"testing"
)

// getLegalMoves GETs baseURL+"/api/games/"+code+"/legal-moves", optionally
// appending "?from="+from (skipped entirely when from == "" so callers can
// exercise the missing-from-parameter case), and returns the raw HTTP
// response plus its body bytes.
func getLegalMoves(t *testing.T, baseURL, code, from string) (*http.Response, []byte) {
	t.Helper()

	path := "/api/games/" + code + "/legal-moves"
	if from != "" {
		path += "?from=" + from
	}
	return getJSON(t, baseURL, path)
}

// sortedStrings returns a sorted copy of ss, for order-independent slice
// comparison.
func sortedStrings(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	sort.Strings(out)
	return out
}

// assertLegalMoves decodes raw as a JSON array of position strings and
// fails the test unless it's set-equal (order-independent, no
// duplicates-tolerant beyond exact count) to want.
func assertLegalMoves(t *testing.T, raw []byte, want []string) {
	t.Helper()

	var got []string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode response body as []string: %v (body: %s)", err, raw)
	}

	gotSorted := sortedStrings(got)
	wantSorted := sortedStrings(want)

	if len(gotSorted) != len(wantSorted) {
		t.Fatalf("legal-moves = %v, want exactly %v", got, want)
	}
	for i := range gotSorted {
		if gotSorted[i] != wantSorted[i] {
			t.Fatalf("legal-moves = %v, want exactly %v", got, want)
		}
	}
}

// TestGetLegalMoves_CornerPiece_ReturnsExactlyTwoDestinations covers this
// backlog item's AC itself: a black stone maneuvered to corner "0,0" (which
// has exactly 2 graph neighbors, "1,0" and "0,1", both guaranteed empty)
// returns exactly those 2 positions and no others.
//
// Maneuver sequence (must alternate turns; legal-moves itself doesn't care
// whose turn it currently is once queried, but getting a piece to "0,0"
// requires legal alternating moves to get there):
//
//	seq 1: black "2,0" -> "1,0"
//	seq 2: white "2,8" -> "1,8" (any legal white move; unrelated to the corner)
//	seq 3: black "1,0" -> "0,0"
func TestGetLegalMoves_CornerPiece_ReturnsExactlyTwoDestinations(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	move1Resp, move1Raw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "2,0",
		"to":          "1,0",
	}, "")
	if move1Resp.StatusCode != http.StatusOK {
		t.Fatalf("setup move 1 (black 2,0->1,0) status = %d, want %d (body: %s)", move1Resp.StatusCode, http.StatusOK, move1Raw)
	}

	move2Resp, move2Raw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Bob",
		"from":        "2,8",
		"to":          "1,8",
	}, "")
	if move2Resp.StatusCode != http.StatusOK {
		t.Fatalf("setup move 2 (white 2,8->1,8) status = %d, want %d (body: %s)", move2Resp.StatusCode, http.StatusOK, move2Raw)
	}

	move3Resp, move3Raw := postMove(t, baseURL, roomCode, map[string]string{
		"player_name": "Alice",
		"from":        "1,0",
		"to":          "0,0",
	}, "")
	if move3Resp.StatusCode != http.StatusOK {
		t.Fatalf("setup move 3 (black 1,0->0,0) status = %d, want %d (body: %s)", move3Resp.StatusCode, http.StatusOK, move3Raw)
	}

	resp, raw := getLegalMoves(t, baseURL, roomCode, "0,0")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	assertLegalMoves(t, raw, []string{"1,0", "0,1"})
}

// TestGetLegalMoves_UnmovedStartingStone_ReturnsExactlyThreeDestinations
// guards against an implementation that hardcodes "at most 2" (or similar):
// right after game creation, before any moves, black's stone at "4,0" still
// has 3 empty graph neighbors on the y=0 edge ("3,0", "5,0", "4,1") — the
// endpoint must return exactly those 3, not fewer.
func TestGetLegalMoves_UnmovedStartingStone_ReturnsExactlyThreeDestinations(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	resp, raw := getLegalMoves(t, baseURL, roomCode, "4,0")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	assertLegalMoves(t, raw, []string{"3,0", "5,0", "4,1"})
}

// TestGetLegalMoves_EmptyPosition_ReturnsEmptyArray covers a from position
// that's a valid board position but has no occupant: 200 with an empty
// array, not an error — no piece there means no legal moves, not a bad
// request.
func TestGetLegalMoves_EmptyPosition_ReturnsEmptyArray(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	resp, raw := getLegalMoves(t, baseURL, roomCode, "5,5")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	assertLegalMoves(t, raw, []string{})

	// Also assert the literal wire shape is "[]", not "null".
	var asMap any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}
	arr, ok := asMap.([]any)
	if !ok {
		t.Fatalf("response body = %s, want a JSON array", raw)
	}
	if len(arr) != 0 {
		t.Errorf("response body = %s, want an empty JSON array", raw)
	}
}

// TestGetLegalMoves_OffBoardPosition_ReturnsEmptyArray covers a from value
// that isn't even a valid position on the 9x9 grid (e.g. "99,99", off the
// board entirely): 200 with an empty array, same reasoning as the
// empty-position case — no occupant there either, since it's not even a
// real position on the board — not a 400.
func TestGetLegalMoves_OffBoardPosition_ReturnsEmptyArray(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	resp, raw := getLegalMoves(t, baseURL, roomCode, "99,99")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	assertLegalMoves(t, raw, []string{})
}

// TestGetLegalMoves_MissingFromParam_Returns400 covers the REQUIRED ?from=
// query parameter: omitting it entirely returns 400 with a non-empty
// {"error"} body, not a 200 with an empty array and not treated the same as
// an occupant-less position.
func TestGetLegalMoves_MissingFromParam_Returns400(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := setupActiveGame(t, baseURL)

	resp, raw := getLegalMoves(t, baseURL, roomCode, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusBadRequest, raw)
	}

	var body errorResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}
	if body.Error == "" {
		t.Errorf("error field is empty, want a non-empty error message (body: %s)", raw)
	}
}

// TestGetLegalMoves_UnknownRoomCode_Returns404 covers an unknown room code
// -> 404 with a non-empty {"error"} body, mirroring the other endpoints'
// same rule (docs/history/plan.md section 6.8's statusForError mapping via
// store.ErrNotFound).
func TestGetLegalMoves_UnknownRoomCode_Returns404(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp, raw := getLegalMoves(t, baseURL, "ZZZZZZ", "0,0")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusNotFound, raw)
	}

	var body errorResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}
	if body.Error == "" {
		t.Errorf("error field is empty, want a non-empty error message (body: %s)", raw)
	}
}
