// Tests of default_clock_minutes (ADR 0081): the catalog takes a time from 1 to
// 60 minutes, and a variant's patch can set its own.

package catalog_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

func TestDefaultClockMinutesRange(t *testing.T) {
	for minutes, ok := range map[int]bool{-1: false, 1: true, 15: true, 60: true, 61: false} {
		doc := pieDefJSON("", fmt.Sprintf(`"default_clock_minutes": %d,`, minutes))
		err := catalog.NewCatalog().RegisterModule(catalog.Module{ID: "pietest", Definition: []byte(doc)})
		if ok && err != nil {
			t.Errorf("%d minutes: %v", minutes, err)
		}
		if !ok && (err == nil || !strings.Contains(err.Error(), "default_clock_minutes")) {
			t.Errorf("%d minutes: got %v, want it refused", minutes, err)
		}
	}
}
