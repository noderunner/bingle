// Tests of prototypes: each version of a game's rules is a game of its own,
// and the family's name means the newest.

package catalog_test

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

func protoDef(id, family, version, extra string) string {
	return fmt.Sprintf(`{
  "id": %q, "family": %q, "version": %q, "prototype": true, "name": "Proto", %s
  "board": { "generator": { "type": "grid", "width": 3, "height": 3, "adjacency": "orthogonal4" } },
  "seats": [ { "seat": 1, "color": "black" }, { "seat": 2, "color": "white" } ],
  "pieces": { "stone": { "shape": "disc" } },
  "actions": [ { "id": "place", "type": "place", "pieces": ["stone"], "source": "supply" } ],
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "win_condition": { "type": "none" }
}`, id, family, version, extra)
}

func protoModule(version string) catalog.Module {
	id := "proto-" + version
	return catalog.Module{
		ID:         id,
		Definition: []byte(protoDef(id, "proto", version, `"pie_rule": "optional",`)),
		Tutorials:  fstest.MapFS{version + ".yaml": &fstest.MapFile{Data: []byte("x")}},
	}
}

func protoCatalog(t *testing.T, versions ...string) *catalog.Catalog {
	t.Helper()
	cat := catalog.NewCatalog()
	for _, v := range versions {
		if err := cat.RegisterModule(protoModule(v)); err != nil {
			t.Fatalf("registering %s: %v", v, err)
		}
	}
	return cat
}

// The family's name means the newest version, by number and whatever the order
// they were registered in: 1.10.0 follows 1.9.0.
func TestPrototype_TheFamilyMeansTheNewestVersion(t *testing.T) {
	cat := protoCatalog(t, "1.9.0", "1.10.0", "1.0.0")
	cg, ok := cat.Get("proto")
	if !ok || cg.Definition.ID != "proto-1.10.0" {
		t.Fatalf("Get(proto) = %v, %v, want proto-1.10.0", cg, ok)
	}
	if got := cat.Pin("proto"); got != "proto-1.10.0" {
		t.Errorf("Pin(proto) = %q", got)
	}
	if got := cat.Pin("proto~pie"); got != "proto-1.10.0~pie" {
		t.Errorf("Pin(proto~pie) = %q", got)
	}
	// A version already named stays itself, and so does any other game.
	if got := cat.Pin("proto-1.0.0~pie"); got != "proto-1.0.0~pie" {
		t.Errorf("Pin(proto-1.0.0~pie) = %q", got)
	}
	if got := cat.Pin("tak-5x5"); got != "tak-5x5" {
		t.Errorf("Pin(tak-5x5) = %q", got)
	}
	if old, ok := cat.Get("proto-1.0.0"); !ok || old.Definition.Version != "1.0.0" {
		t.Errorf("the old version: %v, %v", old, ok)
	}
}

// Every version is listed, and the family's name itself is not a game to list.
func TestPrototype_ListsEachVersion(t *testing.T) {
	var got []string
	for _, cg := range protoCatalog(t, "1.0.0", "1.1.0").List() {
		got = append(got, cg.Definition.ID)
	}
	if strings.Join(got, " ") != "proto-1.0.0 proto-1.1.0" {
		t.Errorf("List = %v", got)
	}
}

// A setup option builds on the version it was asked of, and without a version
// on the newest.
func TestPrototype_OptionsKeepTheVersion(t *testing.T) {
	cat := protoCatalog(t, "1.0.0", "1.1.0")
	opts := map[string]any{catalog.PieOption: true}
	for from, want := range map[string]string{"proto-1.0.0": "proto-1.0.0~", "proto": "proto-1.1.0~"} {
		id, err := cat.Derive(from, opts)
		if err != nil || !strings.HasPrefix(id, want) {
			t.Errorf("Derive(%s) = %q, %v, want %s…", from, id, err, want)
		}
		cg, ok := cat.Get(id)
		if !ok || !cg.Definition.Prototype || cg.Definition.Family != "proto" {
			t.Errorf("%s: %v, %v", id, cg, ok)
		}
	}
	// A game stored under a derived id of the family's name is still found.
	if cg, ok := cat.Get("proto~" + strings.TrimPrefix(mustDerive(t, cat, opts), "proto-1.1.0~")); !ok || cg.Definition.Version != "1.1.0" {
		t.Errorf("derived from the family's name: %v, %v", cg, ok)
	}
}

func mustDerive(t *testing.T, cat *catalog.Catalog, opts map[string]any) string {
	t.Helper()
	id, err := cat.Derive("proto", opts)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Each version brings its own tutorials, under its own id.
func TestPrototype_TutorialsBelongToTheVersion(t *testing.T) {
	cat := protoCatalog(t, "1.0.0", "1.1.0")
	for id, want := range map[string]string{"proto-1.0.0": "proto-1.0.0", "proto-1.1.0": "proto-1.1.0", "proto": "proto-1.1.0"} {
		family, fsys, ok := cat.Tutorials(id)
		if !ok || family != want {
			t.Errorf("Tutorials(%s) names %q (%v), want %q", id, family, ok, want)
			continue
		}
		version := strings.TrimPrefix(want, "proto-")
		if _, ok := fsys.(fstest.MapFS)[version+".yaml"]; !ok {
			t.Errorf("Tutorials(%s) are not those of %s: %v", id, version, fsys)
		}
	}
}

// The id has to say the version, because it is the only record of the rules.
func TestPrototype_TheIDHasToSayTheVersion(t *testing.T) {
	for name, tc := range map[string]struct{ id, family, version, want string }{
		"another version":  {"proto-1.0.0", "proto", "1.1.0", "has to be named"},
		"another family":   {"proto-1.0.0", "other", "1.0.0", "has to be named"},
		"no version in id": {"proto", "proto", "1.0.0", "has to be named"},
		"no family":        {"proto-1.0.0", "", "1.0.0", "needs a family"},
		"no version":       {"proto-", "proto", "", "needs a family"},
		"not a semver":     {"proto-1.0", "proto", "1.0", "major.minor.patch"},
	} {
		err := catalog.NewCatalog().RegisterModule(catalog.Module{ID: tc.id, Definition: []byte(protoDef(tc.id, tc.family, tc.version, ""))})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want one saying %q", name, err, tc.want)
		}
	}
}

// A variant of a version is the version's, not a newer game: only the module's
// own game can be what the family's name means.
func TestPrototype_AModuleIsNamedForItsVersion(t *testing.T) {
	err := catalog.NewCatalog().RegisterModule(catalog.Module{ID: "proto-1.0.0-small", Definition: []byte(protoDef("proto-1.0.0-small", "proto", "1.0.0", ""))})
	if err == nil || !strings.Contains(err.Error(), "has to be named") {
		t.Errorf("error %v", err)
	}
}
