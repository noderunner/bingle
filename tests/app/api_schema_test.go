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
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/noderunner/bingle/internal/api"
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
