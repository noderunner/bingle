package catalog_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/tests/testdata"
)

// tableWith is the Table fixture with endings in place of its own.
func tableWith(t *testing.T, endings map[string]string) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(testdata.Table().Definition, &doc); err != nil {
		t.Fatal(err)
	}
	doc["endings"] = endings
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A game words its own endings in its definition (ADR 0150): a reason, or a
// reason and one detail of it, each in a few words on one line.
func TestEndingsAreAReasonInAFewWords(t *testing.T) {
	good := map[string]string{"count": "on rings", "marbles:white": "with four white marbles", "board-full": "by filling the board", "no_moves": "x"}
	if err := catalog.NewCatalog().Register("test-table", tableWith(t, good), nil); err != nil {
		t.Errorf("good endings refused: %v", err)
	}
	for name, endings := range map[string]map[string]string{
		"an empty reason":     {"": "on rings"},
		"a capital":           {"Count": "on rings"},
		"a space in a reason": {"by count": "on rings"},
		"an empty detail":     {"count:": "on rings"},
		"two details":         {"count:a:b": "on rings"},
		"no words":            {"count": "  "},
		"two lines":           {"count": "on rings\nand more"},
		"more than a few":     {"count": strings.Repeat("ring ", catalog.MaxEndingWords/5+1)},
		"a slash in a detail": {"count:a/b": "on rings"},
	} {
		if err := catalog.NewCatalog().Register("test-table", tableWith(t, endings), nil); err == nil || !strings.Contains(err.Error(), "endings") {
			t.Errorf("%s: %v", name, err)
		}
	}
}
