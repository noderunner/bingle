package app_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// A game's look is served to the web page apart from its info (ADR 0124):
// Blooms' web.yaml at /web, and none of it in the info any more. A game
// without a look gets an empty object, and an unknown game a 404.
func TestGameLook_ServedApartFromTheInfo(t *testing.T) {
	baseURL := newRESTTestServer(t)
	get := func(path string) (int, map[string]any) {
		t.Helper()
		resp, err := http.Get(baseURL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		var out map[string]any
		if resp.StatusCode == 200 {
			if err := json.Unmarshal(body, &out); err != nil {
				t.Fatalf("%s: %v (%s)", path, err, body)
			}
		}
		return resp.StatusCode, out
	}
	code, look := get("/api/game-defs/blooms-8/web")
	if code != 200 {
		t.Fatalf("blooms-8's look: %d", code)
	}
	for _, k := range []string{"seats", "pieces", "display", "preview"} {
		if look[k] == nil {
			t.Errorf("Blooms' look has no %s: %v", k, look)
		}
	}
	_, info := get("/api/game-defs/blooms")
	if info["preview"] != nil {
		t.Error("Blooms' info still has its preview")
	}
	for _, s := range info["seats"].([]any) {
		if s.(map[string]any)["blend"] != nil {
			t.Errorf("Blooms' info still blends a seat: %v", s)
		}
	}
	display, _ := info["display"].(map[string]any)
	if display == nil || display["no_hands"] != nil {
		t.Errorf("Blooms' info display: %v", display)
	} else if fenced := display["tints"].(map[string]any)["fenced"].(map[string]any); fenced["color"] != nil || fenced["label"] != "Fenced bloom" {
		t.Errorf("the fenced tint keeps its words and loses its colour: %v", fenced)
	}
	// Council's look names its page script (tests/testdata/council), which is
	// served with the game; a family without scripts has none to serve.
	if code, look := get("/api/game-defs/test-council/web"); code != 200 || look["script"] != "page.js" || look["style"] != "page.css" {
		t.Errorf("Council's look: %d %v", code, look)
	}
	for path, want := range map[string]int{"/static/games/test-council/page.js": 200, "/static/games/test-council/nothing.js": 404, "/static/games/hex-11/page.js": 404} {
		resp, err := http.Get(baseURL + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s: %d, want %d", path, resp.StatusCode, want)
		}
		if want == 200 && !strings.Contains(resp.Header.Get("Content-Type"), "javascript") {
			t.Errorf("%s is served as %q", path, resp.Header.Get("Content-Type"))
		}
	}
	if resp, err := http.Get(baseURL + "/static/games/test-council/page.css"); err != nil {
		t.Fatal(err)
	} else if _ = resp.Body.Close(); resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "text/css") {
		t.Errorf("Council's stylesheet: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if code, _ := get("/api/game-defs/no-such-game/web"); code != 404 {
		t.Errorf("an unknown game's look: %d", code)
	}
}
