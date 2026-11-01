// Tests of retired games in the catalog: a retired id, and the games derived
// from it, are reported with their name; a registered game never is.

package catalog_test

import (
	"reflect"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/tests/testdata"
)

func TestRetired(t *testing.T) {
	c := catalog.NewCatalog()
	c.Retire("connect-four", "Connect Four")
	c.Retire("test-freeplay", "Freeplay")
	mod := testdata.Freeplay()
	if err := c.RegisterModule(mod); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"connect-four": "Connect Four", "connect-four~pie": "Connect Four"} {
		if name, ok := c.Retired(id); !ok || name != want {
			t.Errorf("Retired(%q) = %q, %v", id, name, ok)
		}
	}
	for _, id := range []string{"test-freeplay", "gomoku", "connect-fourx", ""} {
		if name, ok := c.Retired(id); ok {
			t.Errorf("Retired(%q) = %q, true; want false", id, name)
		}
	}
	if got := c.RetiredIDs(); !reflect.DeepEqual(got, []string{"connect-four"}) {
		t.Errorf("RetiredIDs() = %v: a registered game is not retired", got)
	}
}
