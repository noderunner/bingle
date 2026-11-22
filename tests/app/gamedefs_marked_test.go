// The private overlay's games are marked in the list of games and in a
// game's information, so a client can leave them out (ADR 0140).

package app_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/games/fendo"
)

func TestPrivateGames_AreMarked(t *testing.T) {
	a, err := app.New(newTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	// A copy of Fendo, with its variants, as if built from the overlay.
	mod := fendo.Module()
	def := strings.ReplaceAll(string(mod.Definition), `"fendo`, `"secret-fendo`)
	mod.ID, mod.Definition, mod.Private = "secret-fendo", []byte(def), true
	if err := a.Catalog().RegisterModule(mod); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)

	_, raw := getJSON(t, srv.URL, "/api/game-defs")
	var defs []map[string]any
	if err := json.Unmarshal(raw, &defs); err != nil {
		t.Fatal(err)
	}
	marked := map[string]bool{}
	for _, d := range defs {
		id := d["id"].(string)
		_, has := d["private"]
		if want := strings.HasPrefix(id, "secret-"); has != want || (has && d["private"] != true) {
			t.Errorf("%s: private %v, want %v", id, d["private"], want)
		}
		marked[id] = has
	}
	for _, id := range []string{"secret-fendo", "secret-fendo-9"} {
		if !marked[id] {
			t.Errorf("%s is not listed as private (%v)", id, marked)
		}
	}

	for id, want := range map[string]bool{"secret-fendo-9": true, "fendo-9": false} {
		_, raw := getJSON(t, srv.URL, "/api/game-defs/"+id)
		var info struct {
			Private *bool `json:"private"`
		}
		if err := json.Unmarshal(raw, &info); err != nil {
			t.Fatal(err)
		}
		if got := info.Private != nil && *info.Private; got != want || (!want && info.Private != nil) {
			t.Errorf("GET /api/game-defs/%s: private %v, want %v", id, info.Private, want)
		}
	}
}
