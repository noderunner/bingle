package app_test

import (
	"encoding/json"
	"io"
	"net/http"
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
	if code, look := get("/api/game-defs/solitaire/web"); code != 200 || len(look) != 0 {
		t.Errorf("a game with no look: %d %v", code, look)
	}
	if code, _ := get("/api/game-defs/no-such-game/web"); code != 404 {
		t.Errorf("an unknown game's look: %d", code)
	}
}
