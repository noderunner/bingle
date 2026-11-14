// A selection's facts (ADR 0129) are stored with its row in moves, and a
// selection that reports none stores nothing.

package session_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

const factsFixtureDefJSON = `{
  "id": "fixture-facts",
  "name": "Fixture facts",
  "board": { "generator": { "type": "grid", "width": 3, "height": 3, "adjacency": "orthogonal4", "labels": "algebraic" } },
  "seats": [ { "seat": 1, "color": "black" }, { "seat": 2, "color": "white" } ],
  "pieces": { "black": { "color": "black" }, "white": { "color": "white" } },
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "actions": [
    { "id": "place", "type": "place", "pieces": ["black", "white"], "source": "supply", "fact": "placed" },
    { "id": "pass", "type": "pass" }
  ],
  "facts": { "placed": { "description": "A stone is placed." } },
  "win_condition": { "type": "none" }
}`

func TestASelectionsFactsAreStoredWithIt(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.Register("fixture-facts", []byte(factsFixtureDefJSON), nil); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mgr := session.NewManager(st, cat)
	ctx := context.Background()

	created, err := mgr.CreateGame(ctx, "fixture-facts", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Play(ctx, created.RoomCode, "Alice", state.Selection{Action: "place", Piece: "black", To: "b2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Play(ctx, created.RoomCode, "Bob", state.Selection{Action: "pass"}); err != nil {
		t.Fatal(err)
	}

	game, err := st.GetGame(ctx, created.RoomCode)
	if err != nil {
		t.Fatal(err)
	}
	moves, err := st.ListMoves(ctx, game.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(moves) != 2 {
		t.Fatalf("%d moves recorded, want 2", len(moves))
	}
	if want := `[{"name":"placed","seat":1}]`; moves[0].Facts != want {
		t.Errorf("the placement's facts = %q, want %q", moves[0].Facts, want)
	}
	if moves[1].Facts != "" {
		t.Errorf("the pass's facts = %q, want none", moves[1].Facts)
	}
}
