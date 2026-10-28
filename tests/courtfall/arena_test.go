package courtfall_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/courtfall"
)

// arenaGame returns the game a table gets when it asks for the standard arena.
func arenaGame(t *testing.T) (*catalog.Catalog, *catalog.CompiledGame, string) {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(courtfall.Module()); err != nil {
		t.Fatal(err)
	}
	id, err := cat.Derive("courtfall", map[string]any{"standard-arena": true})
	if err != nil {
		t.Fatalf("derive the standard arena: %v", err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("%s is not in the catalog", id)
	}
	return cat, g, id
}

// The standard arena is laid before the first move, and the game starts at the
// choice of Courts instead of the ten-tile draft.
func TestStandardArena_IsLaidAndSkipsTheDraft(t *testing.T) {
	cat, g, id := arenaGame(t)
	if id != "courtfall~arena" {
		t.Errorf("the derived game is %q", id)
	}
	st, err := state.InitialStateSeeded(g, 7)
	if err != nil {
		t.Fatal(err)
	}
	d := data(t, st)
	if d.Phase != "court" {
		t.Fatalf("the game starts in the %q phase, want the choice of Courts", d.Phase)
	}
	if n := len(d.Tiles); n != 10 {
		t.Fatalf("%d special tiles are down, want ten", n)
	}
	// Dark chooses a Court first, as after a draft.
	if len(state.LegalMoves(g, st, 1)) != 0 {
		t.Error("Light may choose a Court first, want Dark to choose")
	}
	if len(state.LegalMoves(g, st, 2)) != 5 {
		t.Errorf("Dark is offered %d Courts, want five", len(state.LegalMoves(g, st, 2)))
	}
	for _, mv := range state.LegalMoves(g, st, 2) {
		if mv.Action == "tile" {
			t.Fatalf("a tile can still be laid: %+v", mv)
		}
	}
	// The plain game still drafts its arena.
	plain, _ := cat.Get("courtfall")
	pst, err := state.InitialStateSeeded(plain, 7)
	if err != nil {
		t.Fatal(err)
	}
	if pd := data(t, pst); pd.Phase != "terrain" || len(pd.Tiles) != 0 {
		t.Errorf("the plain game starts as %q with %d tiles", pd.Phase, len(pd.Tiles))
	}
	// Choosing to draft is what the game does anyway: no derived game for it.
	if again, err := cat.Derive("courtfall", map[string]any{"standard-arena": false}); err != nil || again != "courtfall" {
		t.Errorf("drafting the arena derived %q (%v)", again, err)
	}
	if _, err := cat.Derive("courtfall", map[string]any{"terrain": true}); err == nil {
		t.Error("an option Courtfall does not have was accepted")
	}
}

// The standard arena obeys the draft's own rules, and is the same for both
// players: a half turn of the board about the Eye maps it onto itself.
func TestStandardArena_IsLegalAndEvenHanded(t *testing.T) {
	_, g, _ := arenaGame(t)
	st, err := state.InitialStateSeeded(g, 1)
	if err != nil {
		t.Fatal(err)
	}
	tiles := data(t, st).Tiles
	counts := map[string]int{}
	for _, tile := range tiles {
		counts[tile]++
	}
	if counts["green"] != 4 || counts["blue"] != 3 || counts["red"] != 3 {
		t.Errorf("the arena holds %v, want 4 Overgrowth, 3 Mire and 3 Hazard", counts)
	}
	// Nothing in a deployment zone, and nothing on the Eye.
	for _, name := range []string{"zone-1", "zone-2", "eye"} {
		ids, ok := g.Board.Region(name)
		if !ok {
			t.Fatalf("the board has no %q region", name)
		}
		for _, id := range ids {
			if tile, ok := tiles[string(id)]; ok {
				t.Errorf("%s lies on %s, which is %s", tile, id, name)
			}
		}
	}
	// No two Hazards adjacent, and no tile touching more than two others.
	for hex, tile := range tiles {
		neighbours := 0
		for _, n := range g.Board.Neighbors(board.PositionID(hex)) {
			other, ok := tiles[string(n)]
			if !ok {
				continue
			}
			neighbours++
			if tile == "red" && other == "red" {
				t.Errorf("Hazards on %s and %s touch", hex, n)
			}
		}
		if neighbours > 2 {
			t.Errorf("%s touches %d special tiles, which the draft forbids", hex, neighbours)
		}
	}
	// A half turn about the Eye: every tile has a partner of some colour, and
	// only the two the odd counts cannot pair sit anywhere else.
	odd := 0
	for hex := range tiles {
		p, ok := halfTurn(t, g, hex)
		if !ok {
			t.Fatalf("%s has no partner a half turn away", hex)
		}
		if _, ok := tiles[p]; !ok {
			t.Errorf("%s has no tile opposite it (%s is bare): the arena is lopsided", hex, p)
			continue
		}
		if tiles[p] != tiles[hex] {
			odd++
		}
	}
	if odd != 2 {
		t.Errorf("%d tiles differ from the tile opposite them, want the two the odd counts leave over", odd)
	}
}

// halfTurn is the hex a half turn of the board about the Eye maps hex onto.
func halfTurn(t *testing.T, g *catalog.CompiledGame, hex string) (string, bool) {
	t.Helper()
	q, r, ok := axial(hex)
	if !ok {
		return "", false
	}
	out := label(6-q, 6-r)
	for _, id := range g.Board.Positions() {
		if string(id) == out {
			return out, true
		}
	}
	return out, false
}

// axial reads a hex label ("c5") as the arena's column and row indices.
func axial(hex string) (int, int, bool) {
	if len(hex) != 2 {
		return 0, 0, false
	}
	q := int(hex[0] - 'a')
	r := int(hex[1] - '1')
	if q < 0 || q > 6 || r < 0 || r > 6 {
		return 0, 0, false
	}
	return q, r, true
}

func label(q, r int) string {
	return string(rune('a'+q)) + string(rune('1'+r))
}
