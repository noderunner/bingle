// Tests of what a client finds without the source: GET /api, GET /api/docs, JSON for a wrong turn,
// and the curl walkthrough at the top of docs/api.md.

package app_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/app"
)

func newDocumentedServer(t *testing.T) (string, string) {
	t.Helper()
	docs, err := os.ReadFile("../../docs/api.md")
	if err != nil {
		t.Fatal(err)
	}
	cfg := newTestConfig(t)
	cfg.APIDocs = string(docs)
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return srv.URL, string(docs)
}

type apiIndex struct {
	Version   string   `json:"version"`
	Docs      string   `json:"docs"`
	MCP       string   `json:"mcp"`
	Start     []string `json:"start"`
	Endpoints []struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		About  string `json:"about"`
	} `json:"endpoints"`
}

func TestAPIIndex_SaysWhereToStartAndEveryEndpointItNamesIsThere(t *testing.T) {
	base, docs := newDocumentedServer(t)
	for _, path := range []string{"/api", "/api/"} {
		var idx apiIndex
		if code := doJSON(t, "GET", base+path, nil, &idx); code != 200 {
			t.Fatalf("GET %s: %d", path, code)
		}
		if idx.Docs != "/api/docs" || idx.MCP != "/mcp" || len(idx.Start) == 0 || len(idx.Endpoints) < 10 {
			t.Fatalf("GET %s: %+v", path, idx)
		}
		if path != "/api" {
			continue
		}
		for _, e := range idx.Endpoints {
			// Each is a route of the API, not the answer to a path it lacks, and the docs describe it.
			url := base + strings.NewReplacer("{id}", "hex-7", "{code}", "NOSUCH").Replace(e.Path)
			var raw json.RawMessage
			code := doJSON(t, e.Method, url, map[string]any{}, &raw)
			if code == http.StatusMethodNotAllowed || strings.Contains(string(raw), "no such endpoint") {
				t.Errorf("%s %s is not a route: %d %s", e.Method, e.Path, code, raw)
			}
			if !strings.Contains(docs, strings.TrimPrefix(e.Path, "/api")) {
				t.Errorf("%s %s is not in docs/api.md", e.Method, e.Path)
			}
		}
	}
}

func TestAPIDocs_AreServedAsTextAWrongTurnAsJSON(t *testing.T) {
	base, docs := newDocumentedServer(t)
	resp, err := http.Get(base + "/api/docs")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") || string(body) != docs {
		t.Fatalf("GET /api/docs: %d %q, %d bytes", resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
	}

	var out errorResponse
	if code := doJSON(t, "GET", base+"/api/nope", nil, &out); code != 404 || !strings.Contains(out.Error, "GET /api") {
		t.Errorf("an unknown path: %d %q", code, out.Error)
	}
	if code := doJSON(t, "DELETE", base+"/api/game-defs", nil, &out); code != 405 || !strings.Contains(out.Error, "GET /api") {
		t.Errorf("a wrong method: %d %q", code, out.Error)
	}

	// A server built without them says so, rather than pointing at nothing.
	plain := newRESTTestServer(t)
	var idx apiIndex
	if code := doJSON(t, "GET", plain+"/api", nil, &idx); code != 200 || idx.Docs != "" {
		t.Errorf("index without docs: %d %+v", code, idx)
	}
	if code := doJSON(t, "GET", plain+"/api/docs", nil, &out); code != 404 {
		t.Errorf("docs without docs: %d", code)
	}
}

// The walkthrough in docs/api.md, step by step, so that it stays true.
func TestAPIDocs_TheHexWalkthroughWorks(t *testing.T) {
	base, _ := newDocumentedServer(t)
	key := newGuestKey(t, base)

	var created createGameResponse
	if code := doJSONAs(t, key, "POST", base+"/api/games", map[string]any{"game_def_id": "hex-7", "player_name": "Ana", "opponent": "random"}, &created); code != 200 || created.Status != "active" || created.Seat != 1 {
		t.Fatalf("create: %d %+v", code, created)
	}
	game := base + "/api/games/" + created.RoomCode
	var st struct {
		Impulse *int   `json:"impulse_seat"`
		Status  string `json:"status"`
	}
	if code := doJSONAs(t, key, "GET", game+"/state", nil, &st); code != 200 || st.Impulse == nil || *st.Impulse != 1 {
		t.Fatalf("state: %d %+v", code, st)
	}
	var legal struct {
		Seat  int `json:"seat"`
		Moves []struct {
			Notation string `json:"notation"`
		} `json:"options"`
	}
	if code := doJSONAs(t, key, "GET", game+"/selections", nil, &legal); code != 200 || legal.Seat != 1 || len(legal.Moves) != 49 {
		t.Fatalf("legal: %d seat %d, %d moves", code, legal.Seat, len(legal.Moves))
	}
	var played struct {
		BotMoves []struct {
			Seat int `json:"seat"`
		} `json:"bot_moves"`
		State struct {
			Impulse *int `json:"impulse_seat"`
			Seq     int  `json:"move_seq"`
		} `json:"state"`
	}
	if code := doJSONAs(t, key, "POST", game+"/moves", map[string]any{"player_name": "Ana", "notation": "d4"}, &played); code != 200 || len(played.BotMoves) != 1 || played.State.Seq != 2 {
		t.Fatalf("move: %d %+v", code, played)
	}
	var obs struct {
		BoardText string `json:"board_text"`
	}
	if code := doJSONAs(t, key, "GET", game+"/observe?player_name=Ana", nil, &obs); code != 200 || !strings.Contains(obs.BoardText, "rS") {
		t.Fatalf("observe: %d %q", code, obs.BoardText)
	}
	var out errorResponse
	if code := doJSONAs(t, key, "POST", game+"/moves", map[string]any{"player_name": "Ana", "notation": "z9"}, &out); code != 400 {
		t.Errorf("a move that is not one: %d %q", code, out.Error)
	}
	if code := doJSON(t, "POST", game+"/moves", map[string]any{"player_name": "Ana", "notation": "a1"}, &out); code != 403 {
		t.Errorf("without the key: %d %q", code, out.Error)
	}
	if code := doJSONAs(t, key, "POST", game+"/resign", map[string]any{"player_name": "Ana"}, &st); code != 200 || st.Status != "finished" {
		t.Errorf("resign: %d %+v", code, st)
	}
}
