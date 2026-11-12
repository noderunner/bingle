package look_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheSharedPageKnowsNoGamesUnits holds the shared page to ADR 0124: which
// pieces hold a place, which actions a click on a unit stands for and which
// actions bring a unit on are a game's page script's to say (heldPrefix,
// tokenActions, unitAction), so no shared script spells out one game's.
func TestTheSharedPageKnowsNoGamesUnits(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "web", "static", "js")
	files, err := filepath.Glob(filepath.Join(dir, "*.js"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no shared scripts in %s: %v", dir, err)
	}
	// Cthulhu Wars' words for them, as the page once had them.
	words := []string{`"gate:"`, `"cultist"`, `"summon-"`, `"awaken-"`, `devolve:`, `"absorb"`, `"invisible"`, "ONGATE"}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range words {
			if strings.Contains(string(b), w) {
				t.Errorf("%s names %s: the game's page script says it", filepath.Base(f), w)
			}
		}
	}
}
