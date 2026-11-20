// Tests of docs/api.schema.json: the server serves it and its version, and
// what the server sends in a game played through the index's endpoints and
// its WebSocket fits it (ADR 0134).

package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/noderunner/bingle/internal/api"
	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/version"
)

// apiSchema is docs/api.schema.json, decoded.
func apiSchema(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile("../../docs/api.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestAPISchema_UpToDate fails when a message's Go type changed and the
// schema was not regenerated: then decide, too, whether version.API moves.
func TestAPISchema_UpToDate(t *testing.T) {
	want, err := api.Schema("../..")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../docs/api.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("docs/api.schema.json is out of date: run go run ./scripts/genschema, and decide whether version.API moves (CONTRIBUTING.md)")
	}
	if v := apiSchema(t)["x-api-version"]; v != version.API {
		t.Fatalf("the schema describes API %v, the server speaks %s", v, version.API)
	}
}

func TestAPISchema_IsServedWithTheVersionAndCoversTheIndex(t *testing.T) {
	base, _ := newDocumentedServer(t)
	resp, err := http.Get(base + "/api")
	if err != nil {
		t.Fatal(err)
	}
	var idx struct {
		APIVersion string `json:"api_version"`
		Schema     string `json:"schema"`
		Endpoints  []struct {
			Method string `json:"method"`
			Path   string `json:"path"`
		} `json:"endpoints"`
	}
	err = json.NewDecoder(resp.Body).Decode(&idx)
	_ = resp.Body.Close()
	if err != nil || idx.APIVersion != version.API || idx.Schema != "/api/schema" {
		t.Fatalf("GET /api: %v %+v", err, idx)
	}
	if h := resp.Header.Get(api.APIVersionHeader); h != version.API {
		t.Errorf("GET /api: %s is %q", api.APIVersionHeader, h)
	}

	// Every endpoint the index names has its messages in the schema.
	schema := apiSchema(t)
	endpoints := schema["x-endpoints"].(map[string]any)
	for _, e := range idx.Endpoints {
		if _, ok := endpoints[e.Method+" "+e.Path]; !ok {
			t.Errorf("%s %s is in the index, not in the schema's x-endpoints", e.Method, e.Path)
		}
	}

	resp, err = http.Get(base + "/api/schema")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	file, _ := os.ReadFile("../../docs/api.schema.json")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/schema+json" || !bytes.Equal(body, file) {
		t.Fatalf("GET /api/schema: %d %q, %d bytes", resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
	}

	// An error carries the header too, and a server built without the schema says so.
	var out errorResponse
	plain := newRESTTestServer(t)
	resp, err = http.Get(plain + "/api/schema")
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	_ = resp.Body.Close()
	if resp.StatusCode != 404 || out.Error == "" || resp.Header.Get(api.APIVersionHeader) != version.API {
		t.Errorf("GET /api/schema without one: %d %q %q", resp.StatusCode, out.Error, resp.Header.Get(api.APIVersionHeader))
	}
}

// schemaClient plays through the API, checking each answer against the
// schema's entry for its endpoint.
type schemaClient struct {
	t      *testing.T
	base   string
	key    string
	schema map[string]any
}

func (c *schemaClient) do(route, path string, body any, wantStatus int) map[string]any {
	c.t.Helper()
	method, _, _ := strings.Cut(route, " ")
	var raw json.RawMessage
	code := doJSONAs(c.t, c.key, method, c.base+path, body, &raw)
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		c.t.Fatalf("%s: %v (%s)", route, err, raw)
	}
	// What the client sends fits the schema too, when it is a request the server should take.
	if req, ok := c.schema["x-endpoints"].(map[string]any)[route].(map[string]any)["request"]; ok && body != nil && wantStatus < 400 {
		raw, _ := json.Marshal(body)
		var sent any
		_ = json.Unmarshal(raw, &sent)
		for _, problem := range checkSchema(c.schema, req, sent, "") {
			c.t.Errorf("%s's request: %s", route, problem)
		}
	}
	var want any
	if code >= 400 {
		want = c.schema["x-error"]
	} else {
		want = c.schema["x-endpoints"].(map[string]any)[route].(map[string]any)["response"]
	}
	if code != wantStatus {
		c.t.Fatalf("%s %s: %d %s", method, path, code, raw)
	}
	for _, problem := range checkSchema(c.schema, want, value, "") {
		c.t.Errorf("%s: %s", route, problem)
	}
	obj, _ := value.(map[string]any)
	return obj
}

func TestAPISchema_DescribesWhatTheServerSends(t *testing.T) {
	base, _ := newDocumentedServer(t)
	schema := apiSchema(t)
	ana := &schemaClient{t: t, base: base, key: newGuestKey(t, base), schema: schema}
	bo := &schemaClient{t: t, base: base, key: newGuestKey(t, base), schema: schema}

	ana.do("GET /api", "/api", nil, 200)
	ana.do("GET /api/game-defs", "/api/game-defs", nil, 200)
	for _, id := range []string{"hex-7", "tak-5x5", "blooms", "solitaire", "meridians"} {
		ana.do("GET /api/game-defs/{id}", "/api/game-defs/"+id, nil, 200)
	}
	ana.do("GET /api/bots", "/api/bots", nil, 200)

	created := ana.do("POST /api/games", "/api/games", map[string]any{"game_def_id": "hex-7", "player_name": "Ana"}, 200)
	code, _ := created["room_code"].(string)
	game := "/api/games/" + code
	ana.do("GET /api/games", "/api/games", nil, 200)
	bo.do("POST /api/games/{code}/join", game+"/join", map[string]any{"player_name": "Bo"}, 200)
	ana.do("GET /api/games/{code}", game, nil, 200)

	// A spectator's socket sees the game from here on.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, wsURL(base, wsPath(code, "Cy", true, "spectator", true)), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	if h := resp.Header.Get(api.APIVersionHeader); h != version.API {
		t.Errorf("the WebSocket handshake: %s is %q", api.APIVersionHeader, h)
	}
	event := map[string]any{"$ref": "#/$defs/WebSocketEvent"}
	readEvent := func(want string) {
		t.Helper()
		for {
			var ev map[string]any
			if err := wsjson.Read(ctx, conn, &ev); err != nil {
				t.Fatalf("waiting for a %q event: %v", want, err)
			}
			for _, problem := range checkSchema(schema, event, ev, "") {
				t.Errorf("event %v: %s", ev["type"], problem)
			}
			if ev["type"] == want {
				return
			}
		}
	}
	readEvent("joined")

	ana.do("GET /api/games/{code}/state", game+"/state", nil, 200)
	ana.do("GET /api/games/{code}/observe", game+"/observe?player_name=Ana", nil, 200)
	ana.do("GET /api/games/{code}/selections", game+"/selections", nil, 200)
	ana.do("POST /api/games/{code}/moves", game+"/moves", map[string]any{"player_name": "Ana", "notation": "d4"}, 200)
	readEvent("move")
	ana.do("POST /api/games/{code}/moves", game+"/moves", map[string]any{"player_name": "Ana", "notation": "a1"}, 403)
	ana.do("GET /api/games/{code}/moves", game+"/moves", nil, 200)
	bo.do("POST /api/games/{code}/resign", game+"/resign", map[string]any{"player_name": "Bo"}, 200)
	readEvent("state")
	ana.do("GET /api/games/{code}", game, nil, 200)
	ana.do("GET /api/games/{code}/state", game+"/state", nil, 200)

	// A game in a lobby: a bot sits and is removed, the host changes seats, a
	// player joins, leaves and comes back, and only the host starts it.
	lobby := ana.do("POST /api/games", "/api/games", map[string]any{"game_def_id": "hex-7", "player_name": "Ana", "lobby": true}, 200)
	room := "/api/games/" + lobby["room_code"].(string)
	ana.do("POST /api/games/{code}/bots", room+"/bots", map[string]any{"player_name": "Ana", "bot": "random"}, 200)
	ana.do("POST /api/games/{code}/remove", room+"/remove", map[string]any{"player_name": "Ana", "seat": 2}, 200)
	ana.do("POST /api/games/{code}/seat", room+"/seat", map[string]any{"player_name": "Ana", "seat": 2}, 200)
	bo.do("POST /api/games/{code}/join", room+"/join", map[string]any{"player_name": "Bo"}, 200)
	bo.do("POST /api/games/{code}/leave", room+"/leave", map[string]any{"player_name": "Bo"}, 200)
	bo.do("POST /api/games/{code}/join", room+"/join", map[string]any{"player_name": "Bo"}, 200)
	bo.do("POST /api/games/{code}/start", room+"/start", map[string]any{"player_name": "Bo"}, 403)
	if started := ana.do("POST /api/games/{code}/start", room+"/start", map[string]any{"player_name": "Ana"}, 200); started["status"] != "active" {
		t.Errorf("the lobby game after start: %v", started)
	}

	// Two bots play each other for a spectator, and a match without its bots is refused.
	match := ana.do("POST /api/bot-matches", "/api/bot-matches", map[string]any{"game_def_id": "hex-7", "bot_a": "random", "bot_b": "random", "delay_ms": 150}, 200)
	ana.do("GET /api/games/{code}", "/api/games/"+match["room_code"].(string), nil, 200)
	ana.do("POST /api/bot-matches", "/api/bot-matches", map[string]any{"game_def_id": "hex-7"}, 400)
}

// checkSchema lists how value breaks schema s, for the parts of JSON Schema
// docs/api.schema.json uses. The repository takes no validator as a
// dependency for one test.
func checkSchema(root map[string]any, s any, value any, at string) []string {
	sm, ok := s.(map[string]any)
	if !ok {
		return nil
	}
	if ref, ok := sm["$ref"].(string); ok {
		def := root["$defs"].(map[string]any)[strings.TrimPrefix(ref, "#/$defs/")]
		if def == nil {
			return []string{at + ": no definition " + ref}
		}
		return checkSchema(root, def, value, at)
	}
	var out []string
	if c, ok := sm["const"]; ok && c != value {
		return []string{fmt.Sprintf("%s: %v is not %v", at, value, c)}
	}
	if enum := asList(sm["enum"]); len(enum) > 0 && !slices.Contains(enum, value) {
		return []string{fmt.Sprintf("%s: %v is not one of %v", at, value, enum)}
	}
	if typ, ok := sm["type"]; ok && !hasType(typ, value) {
		return []string{fmt.Sprintf("%s: %v is not of type %v", at, value, typ)}
	}
	for _, sub := range asList(sm["allOf"]) {
		out = append(out, checkSchema(root, sub, value, at)...)
	}
	if any := asList(sm["anyOf"]); len(any) > 0 && !matchesCount(root, any, value, at, func(n int) bool { return n > 0 }) {
		out = append(out, at+": matches none of anyOf")
	}
	if one := asList(sm["oneOf"]); len(one) > 0 && !matchesCount(root, one, value, at, func(n int) bool { return n == 1 }) {
		out = append(out, at+": does not match exactly one of oneOf")
	}
	switch v := value.(type) {
	case map[string]any:
		props, _ := sm["properties"].(map[string]any)
		for _, r := range asList(sm["required"]) {
			if _, ok := v[r.(string)]; !ok {
				out = append(out, fmt.Sprintf("%s: lacks %s", at, r))
			}
		}
		for k, field := range v {
			if p, ok := props[k]; ok {
				out = append(out, checkSchema(root, p, field, at+"."+k)...)
			} else if ap, ok := sm["additionalProperties"]; ok {
				if ap == false {
					out = append(out, at+": unknown field "+k)
				} else {
					out = append(out, checkSchema(root, ap, field, at+"."+k)...)
				}
			}
		}
	case []any:
		for i, item := range v {
			out = append(out, checkSchema(root, sm["items"], item, fmt.Sprintf("%s[%d]", at, i))...)
		}
	}
	return out
}

func matchesCount(root map[string]any, schemas []any, value any, at string, ok func(int) bool) bool {
	n := 0
	for _, sub := range schemas {
		if len(checkSchema(root, sub, value, at)) == 0 {
			n++
		}
	}
	return ok(n)
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func hasType(typ any, value any) bool {
	if l, ok := typ.([]any); ok {
		for _, t := range l {
			if hasType(t, value) {
				return true
			}
		}
		return false
	}
	switch typ {
	case "null":
		return value == nil
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		f, ok := value.(float64)
		return ok && f == math.Trunc(f)
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	}
	return false
}

// nativeMember makes the account @handle and signs a native client in to it
// with a token, changing the password it was made with, as it must be first.
func nativeMember(t *testing.T, a *app.App, base, handle string, schema map[string]any) *schemaClient {
	t.Helper()
	given, err := a.Accounts().Create(context.Background(), handle, handle+"@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	c := &schemaClient{t: t, base: base, schema: schema}
	signedIn := c.do("POST /auth/login", "/auth/login", map[string]any{"handle": handle, "password": given, "token": true}, 200)
	c.key, _ = signedIn["token"].(string)
	var changed struct {
		Token string `json:"token"`
	}
	if code := doJSONAs(t, c.key, "POST", base+"/auth/password", map[string]any{"old": given, "new": handle + "'s own password"}, &changed); code != 200 || changed.Token == "" {
		t.Fatalf("@%s's new password: %d", handle, code)
	}
	c.key = changed.Token
	return c
}

// The calls of a signed-in native client, and the events of its /ws-dm, fit
// the schema: signing in, friends, who is here and direct messages (ADR 0138).
func TestAPISchema_DescribesTheAccountsCalls(t *testing.T) {
	a, base := membersServer(t)
	schema := apiSchema(t)
	ana := nativeMember(t, a, base, "ana", schema)
	bob := nativeMember(t, a, base, "bob", schema)
	ana.do("GET /auth/me", "/auth/me", nil, 200)
	ana.do("POST /auth/tour-seen", "/auth/tour-seen", nil, 200)
	if me := ana.do("GET /auth/me", "/auth/me", nil, 200); me["tour_seen"] != true {
		t.Errorf("after POST /auth/tour-seen: %v", me)
	}

	// Ana's own socket, every event checked.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL(base, "/ws-dm"), &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + ana.key}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	event := map[string]any{"$ref": "#/$defs/DirectWebSocketEvent"}
	readEvent := func(want string) map[string]any {
		t.Helper()
		for {
			var ev map[string]any
			if err := wsjson.Read(ctx, conn, &ev); err != nil {
				t.Fatalf("waiting for a %q event: %v", want, err)
			}
			for _, problem := range checkSchema(schema, event, ev, "") {
				t.Errorf("/ws-dm event %v: %s", ev["type"], problem)
			}
			if ev["type"] == want {
				data, _ := ev["data"].(map[string]any)
				return data
			}
		}
	}
	readEvent("ready")

	bob.do("POST /api/friends/request", "/api/friends/request", map[string]any{"handle": "ana"}, 200)
	readEvent("friends")
	ana.do("GET /api/friends", "/api/friends", nil, 200)
	ana.do("POST /api/friends/answer", "/api/friends/answer", map[string]any{"handle": "bob", "accept": true}, 200)
	ana.do("GET /api/friends/settings", "/api/friends/settings", nil, 200)
	ana.do("PUT /api/friends/settings", "/api/friends/settings", map[string]any{"mail_requests": false}, 200)

	anaAddr, _ := ana.do("POST /api/here", "/api/here", map[string]any{}, 200)["address"].(string)
	bobAddr, _ := bob.do("POST /api/here", "/api/here", map[string]any{}, 200)["address"].(string)
	ana.do("GET /api/here", "/api/here", nil, 200)
	bob.do("POST /api/dm", "/api/dm", map[string]any{"to": anaAddr, "text": "a game?"}, 200)
	readEvent("dm")
	ana.do("POST /api/dm/history", "/api/dm/history", map[string]any{"with": bobAddr}, 200)
	ana.do("POST /api/dm/threads", "/api/dm/threads", map[string]any{}, 200)
	bob.do("POST /api/dm/call", "/api/dm/call", map[string]any{"to": anaAddr, "join": true}, 200)
	readEvent("dm-call")
	bob.do("POST /api/dm/signal", "/api/dm/signal", map[string]any{"to": anaAddr, "kind": "offer", "data": map[string]any{"type": "offer", "sdp": "v=0\r\n"}}, 200)
	readEvent("dm-rtc")
	bob.do("POST /api/dm/call", "/api/dm/call", map[string]any{"join": false}, 200)
	readEvent("dm-call")

	// An invitation to a game, answered and dismissed.
	created := bob.do("POST /api/games", "/api/games", map[string]any{"game_def_id": "hex-7", "player_name": "Bob"}, 200)
	room, _ := created["room_code"].(string)
	bob.do("POST /api/friends/invite", "/api/friends/invite", map[string]any{"handle": "ana", "room": room}, 200)
	ana.do("POST /api/friends/invite-answer", "/api/friends/invite-answer", map[string]any{"room": room, "accept": false}, 200)
	ana.do("POST /api/friends/dismiss", "/api/friends/dismiss", map[string]any{"room": room}, 200)
	ana.do("POST /api/friends/remove", "/api/friends/remove", map[string]any{"handle": "bob"}, 200)

	// A sign-in with a second factor answers its pending step, and the code finishes it.
	browser := newBrowser(t, base)
	browser.login("ana", "ana's own password")
	var setup struct{ Secret string }
	if code := browser.do("POST", "/auth/totp/setup", map[string]any{"password": "ana's own password"}, &setup); code != 200 {
		t.Fatalf("totp setup: %d", code)
	}
	key := totpKey(t, setup.Secret)
	if code := browser.do("POST", "/auth/totp/enable", map[string]any{"code": auth.TOTPCode(key, time.Now(), 6)}, nil); code != 200 {
		t.Fatalf("totp enable: %d", code)
	}
	fresh := &schemaClient{t: t, base: base, schema: schema}
	first := fresh.do("POST /auth/login", "/auth/login", map[string]any{"handle": "ana", "password": "ana's own password", "token": true}, 200)
	fresh.do("POST /auth/second", "/auth/second", map[string]any{"pending": first["pending"], "code": "000000"}, 401)
	first = fresh.do("POST /auth/login", "/auth/login", map[string]any{"handle": "ana", "password": "ana's own password", "token": true}, 200)
	done := fresh.do("POST /auth/second", "/auth/second", map[string]any{"pending": first["pending"], "code": auth.TOTPCode(key, time.Now().Add(30*time.Second), 6)}, 200)
	fresh.key, _ = done["token"].(string)
	fresh.do("POST /auth/logout", "/auth/logout", nil, 200)
}

// A game's voice chat fits the schema: the network helpers, the call list and
// its heartbeat, the messages a browser sends (SignalData), and the call's own
// socket, which sends joined, voice and rtc and nothing else.
func TestAPISchema_DescribesTheVoiceCalls(t *testing.T) {
	base, _ := newDocumentedServer(t)
	schema := apiSchema(t)
	ana := &schemaClient{t: t, base: base, key: newGuestKey(t, base), schema: schema}
	bo := &schemaClient{t: t, base: base, key: newGuestKey(t, base), schema: schema}
	created := ana.do("POST /api/games", "/api/games", map[string]any{"game_def_id": "hex-7", "player_name": "Ana"}, 200)
	code, _ := created["room_code"].(string)
	game := "/api/games/" + code
	bo.do("POST /api/games/{code}/join", game+"/join", map[string]any{"player_name": "Bo"}, 200)
	ana.do("GET /api/rtc/config", "/api/rtc/config", nil, 200)

	// Ana's call socket first, as the page opens it, then she joins.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL(base, "/ws/"+code+"?name=Ana&role=player&only=voice"),
		&websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + ana.key}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	event := map[string]any{"$ref": "#/$defs/WebSocketEvent"}
	readEvent := func(want string) map[string]any {
		t.Helper()
		var ev map[string]any
		if err := wsjson.Read(ctx, conn, &ev); err != nil {
			t.Fatalf("waiting for a %q event: %v", want, err)
		}
		for _, problem := range checkSchema(schema, event, ev, "") {
			t.Errorf("event %v: %s", ev["type"], problem)
		}
		// The call's socket sends nothing else, so each event is the next one expected.
		if ev["type"] != want {
			t.Fatalf("the call's socket sent %v, want %q", ev, want)
		}
		data, _ := ev["data"].(map[string]any)
		return data
	}
	readEvent("joined")
	ana.do("POST /api/games/{code}/voice", game+"/voice", map[string]any{"player_name": "Ana", "active": true}, 200)
	readEvent("voice")
	bo.do("POST /api/games/{code}/voice", game+"/voice", map[string]any{"player_name": "Bo", "active": true}, 200)
	if joined := readEvent("voice"); joined["kind"] != "join" || joined["seat"] != 2.0 {
		t.Errorf("Bo joining: %v", joined)
	}
	if peers := ana.do("GET /api/games/{code}/voice", game+"/voice", nil, 200)["peers"].([]any); len(peers) != 2 {
		t.Errorf("the call: %v", peers)
	}

	// Bo, the higher seat, offers; what a browser sends of each kind reaches Ana as it was.
	for _, m := range []struct {
		kind string
		data any
	}{
		{"offer", map[string]any{"type": "offer", "sdp": "v=0\r\no=- 1 2 IN IP4 127.0.0.1\r\n"}},
		{"ice", map[string]any{"candidate": "candidate:1 1 udp 2122260223 192.0.2.1 54321 typ host", "sdpMid": "0", "sdpMLineIndex": 0, "usernameFragment": "abcd"}},
		{"ice", map[string]any{"candidate": "candidate:2 1 udp 1 192.0.2.2 1 typ host", "sdpMid": nil, "sdpMLineIndex": nil}},
		{"retry", map[string]any{}},
	} {
		bo.do("POST /api/games/{code}/voice/signal", game+"/voice/signal", map[string]any{"player_name": "Bo", "to": 1, "kind": m.kind, "data": m.data}, 200)
		if got := readEvent("rtc"); got["from"] != 2.0 || got["kind"] != m.kind {
			t.Errorf("Ana got %v for a %s", got, m.kind)
		}
	}
	ana.do("POST /api/games/{code}/voice/signal", game+"/voice/signal", map[string]any{"player_name": "Ana", "to": 2, "kind": "answer", "data": map[string]any{"type": "answer", "sdp": "v=0\r\n"}}, 200)
	ana.do("POST /api/games/{code}/voice/signal", game+"/voice/signal", map[string]any{"player_name": "Ana", "to": 2, "kind": "hello", "data": map[string]any{}}, 400)

	// The schema tells a browser's messages apart, and says what is not one.
	data := map[string]any{"$ref": "#/$defs/SignalData"}
	for _, bad := range []any{map[string]any{"sdp": "v=0"}, map[string]any{"type": "pranswer", "sdp": "v=0"}, map[string]any{"retry": true}} {
		if len(checkSchema(schema, data, bad, "")) == 0 {
			t.Errorf("SignalData takes %v", bad)
		}
	}

	bo.do("POST /api/games/{code}/voice", game+"/voice", map[string]any{"player_name": "Bo", "active": false}, 200)
	if left := readEvent("voice"); left["kind"] != "leave" {
		t.Errorf("Bo leaving: %v", left)
	}
}
