// Tests of prototype games: the rules versions a table can choose, and a game
// keeping the rules it started with when a newer version arrives.

package app_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/catalog"
	courtfallv1 "github.com/noderunner/bingle/internal/games/courtfall/v1.0.0"
	"github.com/noderunner/bingle/internal/session"
)

// newPrototypeServer serves the shipped games plus a Courtfall 1.1.0, so that
// there is a choice, and returns the app with the URL.
func newPrototypeServer(t *testing.T) (*app.App, string) {
	t.Helper()
	a, err := app.New(newTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	addCourtfallVersion(t, a, "1.1.0")
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return a, srv.URL
}

// addCourtfallVersion registers version's copy of Courtfall 1.0.0.
func addCourtfallVersion(t *testing.T, a *app.App, version string) {
	t.Helper()
	mod := courtfallv1.Module()
	var doc map[string]any
	if err := json.Unmarshal(mod.Definition, &doc); err != nil {
		t.Fatal(err)
	}
	mod.ID = "courtfall-" + version
	doc["id"], doc["version"] = mod.ID, version
	var err error
	if mod.Definition, err = json.Marshal(doc); err != nil {
		t.Fatal(err)
	}
	if err := a.Catalog().RegisterModule(mod); err != nil {
		t.Fatal(err)
	}
}

// Every prototype that ships keeps its version in a folder named for it, and
// its first versions start at 1.0.0.
func TestPrototypes_LiveInAFolderOfTheirVersion(t *testing.T) {
	found := map[string]bool{}
	for _, mod := range app.BuiltinModules() {
		cat := catalog.NewCatalog()
		if err := cat.RegisterModule(mod); err != nil {
			t.Fatal(err)
		}
		cg, ok := cat.Get(mod.ID)
		if !ok || !cg.Definition.Prototype {
			continue
		}
		def := cg.Definition
		found[def.ID] = true
		dir := filepath.Join("..", "..", "internal", "games", def.Family, "v"+def.Version)
		if _, err := os.Stat(filepath.Join(dir, def.Family+".yaml")); err != nil {
			t.Errorf("%s: %v", def.ID, err)
		}
	}
	for _, id := range []string{"courtfall-1.0.0", "mycelium-1.0.0"} {
		if !found[id] {
			t.Errorf("%s is not a prototype that ships (%v)", id, found)
		}
	}
}

// A game keeps the version it was created under, whatever comes later; the
// family's name creates the newest, and a table may choose an older one.
func TestPrototypes_AGameKeepsItsVersion(t *testing.T) {
	a, base := newPrototypeServer(t)
	ctx := context.Background()
	mgr := a.Manager()

	storedAs := func(room string) string {
		t.Helper()
		_, body := getJSON(t, base, "/api/games/"+room)
		var g struct {
			GameDefID string `json:"game_def_id"`
		}
		if err := json.Unmarshal(body, &g); err != nil {
			t.Fatal(err)
		}
		return g.GameDefID
	}
	create := func(id string, options map[string]any) string {
		t.Helper()
		res, err := mgr.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: id, CreatorName: "Alice", Options: options})
		if err != nil {
			t.Fatalf("creating %s: %v", id, err)
		}
		return storedAs(res.RoomCode)
	}
	if got := create("courtfall", nil); got != "courtfall-1.1.0" {
		t.Errorf("the family's name made %q", got)
	}
	if got := create("courtfall-1.0.0", nil); got != "courtfall-1.0.0" {
		t.Errorf("choosing the old version made %q", got)
	}
	if got := create("courtfall-1.0.0", map[string]any{"standard-arena": true}); got != "courtfall-1.0.0~arena" {
		t.Errorf("the old version with an option made %q", got)
	}
	if got := create("courtfall", map[string]any{"standard-arena": true}); got != "courtfall-1.1.0~arena" {
		t.Errorf("the family's name with an option made %q", got)
	}

	res, err := mgr.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "courtfall", CreatorName: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	addCourtfallVersion(t, a, "2.0.0")
	if got := storedAs(res.RoomCode); got != "courtfall-1.1.0" {
		t.Errorf("a newer version moved a game to %q", got)
	}
	if got := create("courtfall", nil); got != "courtfall-2.0.0" {
		t.Errorf("after the new version, the family's name made %q", got)
	}

	// The API says what a client needs to offer the choice.
	var list []struct {
		ID        string `json:"id"`
		Family    string `json:"family"`
		Version   string `json:"version"`
		Prototype bool   `json:"prototype"`
	}
	_, body := getJSON(t, base, "/api/game-defs")
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	versions := map[string]string{}
	for _, d := range list {
		if d.Family == "courtfall" {
			if !d.Prototype {
				t.Errorf("%s is not marked a prototype", d.ID)
			}
			versions[d.ID] = d.Version
		}
	}
	if len(versions) != 3 || versions["courtfall-1.0.0"] != "1.0.0" || versions["courtfall-2.0.0"] != "2.0.0" {
		t.Errorf("the versions listed: %v", versions)
	}
	var info struct {
		ID        string `json:"id"`
		Prototype bool   `json:"prototype"`
	}
	_, body = getJSON(t, base, "/api/game-defs/courtfall")
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatal(err)
	}
	if info.ID != "courtfall-2.0.0" || !info.Prototype {
		t.Errorf("GET /game-defs/courtfall = %+v", info)
	}
}

// The setup offers the versions, newest first and chosen; the choice is what is
// played, with the rules page of that version, and the game says which it is.
func TestWebPrototypes_ChoosingARulesVersion(t *testing.T) {
	_, base := newPrototypeServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/"), chromedp.WaitVisible(`.game-card[data-family="courtfall"]`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, `document.querySelector('.game-card[data-family="courtfall"] .badge.prototype').textContent`); got != "Prototype" {
		t.Errorf("the card's mark is %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('.game-card[data-family="courtfall"]').length`); n != 1 {
		t.Errorf("Courtfall has %d cards, one per version?", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('.game-card[data-family="courtfall"] .badge.beta, .game-card[data-family="mycelium"] .badge.beta').length`); n != 0 {
		t.Errorf("%d prototypes are also marked Beta", n)
	}

	clickSel(t, ctx, `.game-card[data-family="courtfall"]`)
	waitJS(t, ctx, "the choice of rules", `!!document.getElementById("setup-rules-version") && !document.getElementById("game-setup").hidden`, 10*time.Second)
	if got := evalString(t, ctx, `[...document.getElementById("setup-rules-version").options].map(o => o.textContent).join("|")`); got != "1.1.0 (newest)|1.0.0" {
		t.Errorf("the versions offered: %q", got)
	}
	if got := evalString(t, ctx, `document.getElementById("game_def_id").value`); got != "courtfall-1.1.0" {
		t.Errorf("the game chosen at first is %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll("#setup-variant").length`); n != 0 {
		t.Errorf("the versions are offered as variants too (%d)", n)
	}

	// Another version.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => { const s = document.getElementById("setup-rules-version"); s.value = "1.0.0"; s.dispatchEvent(new Event("change")); })()`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the older version", `document.getElementById("game_def_id").value === "courtfall-1.0.0" && document.getElementById("setup-rules-version").value === "1.0.0"`, 5*time.Second)

	// A lone version is still said.
	clickSel(t, ctx, `#setup-back`)
	waitJS(t, ctx, "the list", `!document.getElementById("gallery-view").hidden`, 5*time.Second)
	clickSel(t, ctx, `.game-card[data-family="mycelium"]`)
	waitJS(t, ctx, "Mycelium's rules", `!!document.getElementById("setup-rules-version") && document.getElementById("setup-rules-version").value === "1.0.0"`, 10*time.Second)
	clickSel(t, ctx, `#setup-back`)
	waitJS(t, ctx, "the list again", `!document.getElementById("gallery-view").hidden`, 5*time.Second)

	// A game that is no prototype has no such choice.
	clickSel(t, ctx, `.game-card[data-family="tak-5x5"]`)
	waitJS(t, ctx, "Tak's setup", `document.getElementById("game_def_id").value.indexOf("tak") === 0 && !document.getElementById("setup-rules-version")`, 10*time.Second)

	// A link by the family's name opens the newest.
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/?game=courtfall")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the link opens the newest", `!!document.getElementById("game_def_id") && document.getElementById("game_def_id").value === "courtfall-1.1.0" && !document.getElementById("game-setup").hidden`, 10*time.Second)
}
