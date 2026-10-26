// Tests of POST /games and GET /game-defs: a known game id gives a room code, seat and colour; an unknown one is a 404.

package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/session/roomcode"
)

// newRESTTestServer builds a fresh *app.App (against its own temp DB, via
// newTestConfig from tests/app/app_test.go) and wraps its router in an
// httptest.Server, registering cleanup for both. It returns the server's
// base URL.
func newRESTTestServer(t *testing.T) string {
	t.Helper()
	cfg := newTestConfig(t)

	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New(cfg) returned error: %v, want nil", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	registerFixtures(t, a)

	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return srv.URL
}

// createGameResponse mirrors the POST /api/games 200 response shape from
// docs/history/plan.md section 6.8: {"room_code", "seat", "color", "status"}.
type createGameResponse struct {
	RoomCode string `json:"room_code"`
	Seat     int    `json:"seat"`
	Color    string `json:"color"`
	Status   string `json:"status"`
}

// errorResponse mirrors the REST error shape from docs/history/plan.md section 6.8:
// every error response is {"error": "<message>"}.
type errorResponse struct {
	Error string `json:"error"`
}

// gameDefSummary mirrors one entry of GET /api/game-defs's response array:
// [{"id", "name", "seats_required"}].
type gameDefSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	SeatsRequired int    `json:"seats_required"`
}

// postGame POSTs raw JSON (or, if rawBody != "", that literal string instead
// of marshaling anything) to baseURL+"/api/games" and returns the raw HTTP
// response along with its body bytes (already read and the response body
// closed), so callers can both inspect the status code and decode/inspect
// the body without juggling readers.
func postGame(t *testing.T, baseURL string, body any, rawBody string) (*http.Response, []byte) {
	t.Helper()

	var payload []byte
	if rawBody != "" {
		payload = []byte(rawBody)
	} else {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal(body): %v", err)
		}
	}

	resp, err := http.Post(baseURL+"/api/games", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/games: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp, buf.Bytes()
}

// isRoomCode reports whether s is exactly 6 characters, every one drawn
// from roomcode.Alphabet.
func isRoomCode(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune(roomcode.Alphabet, r) {
			return false
		}
	}
	return true
}

// TestPostGames_KnownGameDefID_Returns200WithRoomCode covers the backlog
// item's AC itself: POST /api/games with a known game_def_id returns 200
// and a JSON body with a room_code field that's exactly 6 characters, every
// character drawn from roomcode.Alphabet.
func TestPostGames_KnownGameDefID_Returns200WithRoomCode(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp, raw := postGame(t, baseURL, map[string]string{
		"game_def_id": "test-freeplay",
		"player_name": "Alice",
	}, "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	var body createGameResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}

	if !isRoomCode(body.RoomCode) {
		t.Errorf("room_code = %q, want a 6-character code drawn from roomcode.Alphabet (%q)", body.RoomCode, roomcode.Alphabet)
	}
}

// TestPostGames_KnownGameDefID_ReturnsSeatColorStatus covers the response
// shape beyond room_code: seat == 1, color == "black" (freeplay.json's seat 1
// color), status == "waiting" (a freshly-created game with only the
// creator seated).
func TestPostGames_KnownGameDefID_ReturnsSeatColorStatus(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp, raw := postGame(t, baseURL, map[string]string{
		"game_def_id": "test-freeplay",
		"player_name": "Alice",
	}, "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, raw)
	}

	var body createGameResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, raw)
	}

	if body.Seat != 1 {
		t.Errorf("seat = %d, want 1", body.Seat)
	}
	if body.Color != "black" {
		t.Errorf("color = %q, want %q", body.Color, "black")
	}
	if body.Status != "waiting" {
		t.Errorf("status = %q, want %q", body.Status, "waiting")
	}
}

// TestGetGameDefs_ReturnsTestFreeplay covers GET /api/game-defs: 200 and a
// JSON array containing at least one entry with id == "test-freeplay",
// name == "Test: Free Movement & Capture", seats_required == 2 — matching
// tests/testdata/freeplay.json, which is always registered into a real
// app.New()'s catalog.
func TestGetGameDefs_ReturnsTestFreeplay(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp, err := http.Get(baseURL + "/api/game-defs")
	if err != nil {
		t.Fatalf("GET /api/game-defs: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var defs []gameDefSummary
	if err := json.NewDecoder(resp.Body).Decode(&defs); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	found := false
	for _, d := range defs {
		if d.ID == "test-freeplay" {
			found = true
			if d.Name != "Test: Free Movement & Capture" {
				t.Errorf("test-freeplay name = %q, want %q", d.Name, "Test: Free Movement & Capture")
			}
			if d.SeatsRequired != 2 {
				t.Errorf("test-freeplay seats_required = %d, want 2", d.SeatsRequired)
			}
		}
	}
	if !found {
		t.Errorf("GET /api/game-defs response %+v does not contain an entry with id == %q", defs, "test-freeplay")
	}
}

// TestPostGames_UnknownGameDefID_Returns404 covers session.ErrGameDefNotFound
// surfacing as an HTTP 404 with a non-empty {"error": "..."} body, per
// docs/history/plan.md section 6.8's statusForError mapping.
func TestPostGames_UnknownGameDefID_Returns404(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp, raw := postGame(t, baseURL, map[string]string{
		"game_def_id": "nonexistent-game",
		"player_name": "Alice",
	}, "")

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

// TestPostGames_BlankPlayerName_Returns400 covers docs/history/plan.md section 6.8's
// "blank/whitespace-only player_name ... is rejected with 400 before even
// calling Manager" rule, for both a totally empty name and a
// whitespace-only one.
func TestPostGames_BlankPlayerName_Returns400(t *testing.T) {
	cases := []struct {
		name       string
		playerName string
	}{
		{name: "empty string", playerName: ""},
		{name: "whitespace only", playerName: "   "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL := newRESTTestServer(t)

			resp, raw := postGame(t, baseURL, map[string]string{
				"game_def_id": "test-freeplay",
				"player_name": tc.playerName,
			}, "")

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusBadRequest, raw)
			}
		})
	}
}

// TestPostGames_BlankGameDefID_Returns400 covers a blank game_def_id being
// rejected with 400 (it is not a "known" game_def_id, and per plan section
// 6.8's error mapping only store.ErrNotFound/session.ErrGameDefNotFound
// produce a 404 — a request-validation failure like a missing field is a
// 400, mirroring the blank player_name rule).
func TestPostGames_BlankGameDefID_Returns400(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp, raw := postGame(t, baseURL, map[string]string{
		"game_def_id": "",
		"player_name": "Alice",
	}, "")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusBadRequest, raw)
	}
}

// TestPostGames_MalformedJSON_Returns400 covers robustness: malformed JSON
// in the request body must not panic or 500 the server — it's a 400.
func TestPostGames_MalformedJSON_Returns400(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp, raw := postGame(t, baseURL, nil, `{"game_def_id": "test-freeplay", "player_name": `)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusBadRequest, raw)
	}
}

// TestPostGames_MalformedJSON_NotEmptyBody guards specifically against a
// non-JSON body at all (e.g. plain text), which must also be a 400, not a
// 500 or a hang/panic.
func TestPostGames_MalformedJSON_NotEmptyBody(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp, raw := postGame(t, baseURL, nil, `this is not json at all`)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusBadRequest, raw)
	}
}

// TestPostGames_TwoCalls_ReturnDifferentRoomCodes covers that two separate
// POST /api/games calls (different player names) produce two distinct room
// codes — each call creates its own new room.
func TestPostGames_TwoCalls_ReturnDifferentRoomCodes(t *testing.T) {
	baseURL := newRESTTestServer(t)

	resp1, raw1 := postGame(t, baseURL, map[string]string{
		"game_def_id": "test-freeplay",
		"player_name": "Alice",
	}, "")
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first POST /api/games status = %d, want %d (body: %s)", resp1.StatusCode, http.StatusOK, raw1)
	}
	var body1 createGameResponse
	if err := json.Unmarshal(raw1, &body1); err != nil {
		t.Fatalf("decode first response body: %v (body: %s)", err, raw1)
	}

	resp2, raw2 := postGame(t, baseURL, map[string]string{
		"game_def_id": "test-freeplay",
		"player_name": "Bob",
	}, "")
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second POST /api/games status = %d, want %d (body: %s)", resp2.StatusCode, http.StatusOK, raw2)
	}
	var body2 createGameResponse
	if err := json.Unmarshal(raw2, &body2); err != nil {
		t.Fatalf("decode second response body: %v (body: %s)", err, raw2)
	}

	if body1.RoomCode == "" || body2.RoomCode == "" {
		t.Fatalf("expected non-empty room codes, got %q and %q", body1.RoomCode, body2.RoomCode)
	}
	if body1.RoomCode == body2.RoomCode {
		t.Errorf("two separate POST /api/games calls returned the same room_code %q, want two distinct codes", body1.RoomCode)
	}
}
