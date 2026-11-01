package session_test

import (
	"context"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/courtfall"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// A tutorial of a game with chance opens every room from the same seeded
// position, so its scripted dice always roll the same; another seed gives
// another position.
func TestStartTutorial_SeedsTheStart(t *testing.T) {
	mod := courtfall.Module()
	doc := func(id string, seed string) string {
		return "id: " + id + "\nkind: rules\ntitle: T\ngame: courtfall\nseed: " + seed + "\nsteps:\n  - text: Look.\n"
	}
	mod.Tutorials = fstest.MapFS{
		"seven.yaml": {Data: []byte(doc("seven", "7"))},
		"eight.yaml": {Data: []byte(doc("eight", "8"))},
	}
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(mod); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mgr := session.NewManager(st, cat)
	ctx := context.Background()

	start := func(id, name string) store.Game {
		res, err := mgr.StartTutorial(ctx, "courtfall", id, name)
		if err != nil {
			t.Fatalf("StartTutorial(%s): %v", id, err)
		}
		g, err := st.GetGame(ctx, res.RoomCode)
		if err != nil {
			t.Fatal(err)
		}
		if g.Tutorial != "courtfall/"+id || g.Status != "active" {
			t.Fatalf("the room: tutorial %q, status %q", g.Tutorial, g.Status)
		}
		return g
	}
	a, b, c := start("seven", "Ann"), start("seven", "Bea"), start("eight", "Cy")
	if a.InitialState == "" || a.InitialState != b.InitialState {
		t.Fatal("two rooms of one tutorial began from different positions")
	}
	if a.InitialState == c.InitialState {
		t.Fatal("two seeds gave the same position")
	}
}
