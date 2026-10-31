package app_test

// Guests and their keys (ADR 0067): a seat belongs to the principal that took
// it, and a name says which seat a request means, never that it is its own.

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/noderunner/bingle/internal/api"
	"github.com/noderunner/bingle/internal/app"
)

// guestGame is a tic-tac-toe game between two guests, Alice (seat 1, to
// move) and Bob.
func guestGame(t *testing.T, base string) (room, alice, bob string) {
	t.Helper()
	alice, bob = newGuestKey(t, base), newGuestKey(t, base)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSONAs(t, alice, "POST", base+"/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Alice"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if code := doJSONAs(t, bob, "POST", base+"/api/games/"+created.RoomCode+"/join", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatalf("join: %d", code)
	}
	return created.RoomCode, alice, bob
}

func moveAs(t *testing.T, key, base, room, name, notation string) int {
	t.Helper()
	return doJSONAs(t, key, "POST", base+"/api/games/"+room+"/moves", map[string]any{"player_name": name, "notation": notation}, nil)
}

func TestGuest_AMoveIsTheSeatOwnersAlone(t *testing.T) {
	base := newRESTTestServer(t)
	room, alice, bob := guestGame(t, base)
	if code := moveAs(t, bob, base, room, "Alice", "a1"); code != http.StatusForbidden {
		t.Errorf("Bob plays as Alice: %d, want 403", code)
	}
	if code := moveAs(t, "", base, room, "Alice", "a1"); code != http.StatusForbidden {
		t.Errorf("an anonymous request plays as Alice: %d, want 403", code)
	}
	if code := moveAs(t, alice, base, room, "Alice", "a1"); code != 200 {
		t.Errorf("Alice plays: %d, want 200", code)
	}
	// Typing a seated player's name no longer takes the seat either.
	if code := doJSONAs(t, bob, "POST", base+"/api/games/"+room+"/join", map[string]any{"player_name": "alice"}, nil); code != http.StatusForbidden {
		t.Errorf("Bob joins as Alice: %d, want 403", code)
	}
}

// A seat nobody owns — taken by an anonymous client, or before there were
// guests — is acted for by name, and the first guest to do so owns it.
func TestGuest_AnUnownedSeatIsBoundOnce(t *testing.T) {
	base := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Alice"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	if code := doJSON(t, "POST", base+"/api/games/"+room+"/join", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatalf("join: %d", code)
	}
	// Anonymous clients keep playing as they always have.
	if code := moveAs(t, "", base, room, "Alice", "a1"); code != 200 {
		t.Fatalf("anonymous move on an unowned seat: %d", code)
	}
	first, second := newGuestKey(t, base), newGuestKey(t, base)
	if code := moveAs(t, first, base, room, "Bob", "b1"); code != 200 {
		t.Fatalf("the first guest to play Bob's seat: %d", code)
	}
	if code := moveAs(t, "", base, room, "Alice", "a2"); code != 200 {
		t.Fatalf("Alice's seat is still nobody's: %d", code)
	}
	if code := moveAs(t, second, base, room, "Bob", "b2"); code != http.StatusForbidden {
		t.Errorf("a second guest plays Bob's bound seat: %d, want 403", code)
	}
	if code := moveAs(t, "", base, room, "Bob", "b2"); code != http.StatusForbidden {
		t.Errorf("an anonymous request plays Bob's bound seat: %d, want 403", code)
	}
	if code := moveAs(t, first, base, room, "Bob", "b2"); code != 200 {
		t.Errorf("Bob's owner plays: %d, want 200", code)
	}
}

// A browser's first state-changing request from the page makes its guest
// and sets the cookie; the cookie and the same key as a bearer token are the
// same guest.
func TestGuest_TheCookieAndTheBearerKeyAreTheSameGuest(t *testing.T) {
	base := newRESTTestServer(t)
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	page := func(method, path string, body any) *http.Response {
		t.Helper()
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, base+path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Bingle-Session", "1")
		resp, err := browser.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp
	}
	resp := page("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Alice"})
	var key string
	for _, c := range resp.Cookies() {
		if c.Name == "bingle_guest" {
			key = c.Value
			if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
				t.Errorf("the cookie is %+v, want HttpOnly, SameSite=Lax, Path=/", c)
			}
		}
	}
	if !strings.HasPrefix(key, "bgg_") {
		t.Fatalf("no guest cookie on the first request from the page: %v", resp.Cookies())
	}
	// No guest key in the answer itself: the page never holds it.
	var room string
	{
		var list struct {
			Games []struct {
				RoomCode string `json:"room_code"`
			} `json:"games"`
		}
		doJSON(t, "GET", base+"/api/games", nil, &list)
		if len(list.Games) != 1 {
			t.Fatalf("games: %+v", list)
		}
		room = list.Games[0].RoomCode
	}
	if code := doJSONAs(t, newGuestKey(t, base), "POST", base+"/api/games/"+room+"/join", map[string]any{"player_name": "Bob"}, nil); code != 200 {
		t.Fatal(code)
	}
	if code := moveAs(t, key, base, room, "Alice", "a1"); code != 200 {
		t.Errorf("the cookie's key as a bearer token plays Alice's seat: %d, want 200", code)
	}
	if resp := page("POST", "/api/games/"+room+"/chat", map[string]any{"player_name": "Alice", "text": "hi"}); resp.StatusCode != 200 {
		t.Errorf("the browser speaks as Alice: %d, want 200", resp.StatusCode)
	}
}

// A request proved by the cookie must come from the page: a cross-site form
// can send the cookie but not the header.
func TestGuest_ACookieRequestWithoutThePageHeaderIsRefused(t *testing.T) {
	base := newRESTTestServer(t)
	key := newGuestKey(t, base)
	body, _ := json.Marshal(map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Alice"})
	req, _ := http.NewRequest("POST", base+"/api/games", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "bingle_guest", Value: key})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a cookie POST without X-Bingle-Session: %d, want 403", resp.StatusCode)
	}
	// Reading is fine: a GET changes nothing.
	req, _ = http.NewRequest("GET", base+"/api/games", nil)
	req.AddCookie(&http.Cookie{Name: "bingle_guest", Value: key})
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 200 {
		t.Errorf("a cookie GET: %v %v", resp, err)
	}
}

func TestGuest_AnUnknownKey(t *testing.T) {
	base := newRESTTestServer(t)
	// A bearer key that does not work is refused outright.
	if code := doJSONAs(t, "bgg_made-up", "POST", base+"/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Alice"}, nil); code != http.StatusUnauthorized {
		t.Errorf("an unknown bearer key: %d, want 401", code)
	}
	// A cookie that does not work is cleared, and the browser is a newcomer.
	body, _ := json.Marshal(map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Alice"})
	req, _ := http.NewRequest("POST", base+"/api/games", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bingle-Session", "1")
	req.AddCookie(&http.Cookie{Name: "bingle_guest", Value: "bgg_from-another-server"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("an unknown cookie: %d, want 200", resp.StatusCode)
	}
	fresh := ""
	for _, c := range resp.Cookies() {
		if c.Name == "bingle_guest" && c.Value != "" {
			fresh = c.Value
		}
	}
	if fresh == "" || fresh == "bgg_from-another-server" {
		t.Errorf("the browser was not given a guest of its own: %v", resp.Cookies())
	}
}

// A guest key is a credential: it is in no log line, whatever the request.
func TestGuest_AKeyIsNeverLogged(t *testing.T) {
	var logs bytes.Buffer
	cfg := newTestConfig(t)
	a, err := app.New(cfg, app.WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	room, alice, bob := guestGame(t, srv.URL)
	moveAs(t, bob, srv.URL, room, "Alice", "a1") // refused, and logged as refused
	moveAs(t, alice, srv.URL, room, "Alice", "a1")
	doJSONAs(t, alice, "POST", srv.URL+"/api/games/"+room+"/moves", map[string]any{"player_name": "Alice", "notation": "nonsense"}, nil)
	doJSONAs(t, "bgg_not-a-real-key", "POST", srv.URL+"/api/games", map[string]any{}, nil)
	out := logs.String()
	if out == "" {
		t.Fatal("nothing was logged, so this test proves nothing")
	}
	for _, k := range []string{alice, bob, "bgg_not-a-real-key", strings.TrimPrefix(alice, "bgg_")} {
		if strings.Contains(out, k) {
			t.Errorf("a guest key is in the log:\n%s", out)
		}
	}
}

// Every API route that acts for a seat refuses a request from somebody other
// than the seat's owner. A new route that reads player_name and forgets to
// check fails here, unless it is listed as not acting for a seat.
func TestGuest_EveryRouteThatActsForASeatChecksItsOwner(t *testing.T) {
	base := newRESTTestServer(t)
	// Routes that take a game but act for no seat in it.
	notASeat := map[string]string{
		"POST /games/{code}/simulate": "plays moves on a copy, for anybody",
	}
	var routes []string
	err := chi.Walk(api.NewRouter(nil), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method != "GET" && strings.Contains(route, "{code}") {
			routes = append(routes, method+" "+route)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) < 15 {
		t.Fatalf("only %d routes found: %v", len(routes), routes)
	}
	for _, rt := range routes {
		if _, ok := notASeat[rt]; ok {
			continue
		}
		room, _, bob := guestGame(t, base)
		method, path, _ := strings.Cut(rt, " ")
		path = strings.Replace(path, "{code}", url.PathEscape(room), 1)
		body := map[string]any{"player_name": "Alice", "notation": "a1", "text": "hello", "seat": 2, "active": true, "present": true, "action": "offer", "enabled": true, "bot": "random", "cult": "gc", "to": 2, "kind": "offer", "data": map[string]any{}}
		code := doJSONAs(t, bob, method, base+"/api"+path, body, nil)
		if code < 400 {
			t.Errorf("%s as Bob for Alice's seat: %d, want a refusal", rt, code)
		}
	}
}

// What a seat sees privately (its hand, its secrets) is for its owner: anybody
// else naming the seat is shown what a spectator sees.
func TestGuest_ASeatsOwnViewIsItsOwners(t *testing.T) {
	base := newRESTTestServer(t)
	room, alice, bob := guestGame(t, base)
	seatOf := func(key string) int {
		t.Helper()
		var obs struct {
			Seat int `json:"seat"`
		}
		if code := doJSONAs(t, key, "GET", base+"/api/games/"+room+"/observe?player_name=Alice", nil, &obs); code != 200 {
			t.Fatalf("observe: %d", code)
		}
		return obs.Seat
	}
	if s := seatOf(alice); s != 1 {
		t.Errorf("Alice observes as seat %d, want 1", s)
	}
	if s := seatOf(bob); s != 0 {
		t.Errorf("Bob naming Alice observes as seat %d, want a spectator's 0", s)
	}
	if s := seatOf(""); s != 0 {
		t.Errorf("an anonymous request naming Alice observes as seat %d, want 0", s)
	}
}

// An agent that connects to MCP with a guest key owns the seats it takes,
// exactly as a browser does: the key reaches the tools through the request.
func TestGuest_AnAgentWithAKeyOwnsItsSeats(t *testing.T) {
	base := newRESTTestServer(t)
	key := newGuestKey(t, base)
	c, err := client.NewStreamableHttpClient(base+"/mcp", transport.WithHTTPHeaders(map[string]string{"Authorization": "Bearer " + key}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	var initReq mcp.InitializeRequest
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "bingle-test-agent", Version: "0.0.1"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatal(err)
	}
	res := callTool(t, c, "create_game", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Agent"})
	if res.IsError {
		t.Fatalf("create_game: %s", toolErrorText(res))
	}
	var created createGameResponse
	decodeToolResult(t, res, &created)
	if code := doJSON(t, "POST", base+"/api/games/"+created.RoomCode+"/join", map[string]any{"player_name": "Agent"}, nil); code != http.StatusForbidden {
		t.Errorf("an anonymous request takes the agent's seat: %d, want 403", code)
	}
	if code := doJSONAs(t, key, "POST", base+"/api/games/"+created.RoomCode+"/join", map[string]any{"player_name": "Agent"}, nil); code != 200 {
		t.Errorf("the agent's own key over REST: %d, want 200", code)
	}
}
