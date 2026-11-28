// The private overlay's games are marked in the list of games and in a
// game's information, so a client can leave them out (ADR 0140).

package app_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/games/fendo"
)

// newPrivateGameServer serves the shipped games plus secret-fendo, a copy of
// Fendo and its variants marked as if built from the overlay.
func newPrivateGameServer(t *testing.T) string {
	t.Helper()
	a, err := app.New(newTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	mod := fendo.Module()
	def := strings.ReplaceAll(string(mod.Definition), `"fendo`, `"secret-fendo`)
	mod.ID, mod.Definition, mod.Private = "secret-fendo", []byte(def), true
	if err := a.Catalog().RegisterModule(mod); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestPrivateGames_AreMarked(t *testing.T) {
	base := newPrivateGameServer(t)
	_, raw := getJSON(t, base, "/api/game-defs")
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
		_, raw := getJSON(t, base, "/api/game-defs/"+id)
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

// The list of games gives a private game a Private badge, and no other game one.
func TestPrivateGames_HaveABadgeInTheList(t *testing.T) {
	base := newPrivateGameServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := runSteps(ctx,
		stepNavigate(base+"/"),
		stepWaitVisible(`.game-card[data-family="secret-fendo"]`),
	); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, `document.querySelector('.game-card[data-family="secret-fendo"] .badge.private').textContent`); got != "Private" {
		t.Errorf("the private game's badge says %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll(".game-card .badge.private").length`); n != 1 {
		t.Errorf("%d Private badges, want 1", n)
	}
}
