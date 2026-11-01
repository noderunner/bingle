// Tests of tutorials: every shipped tutorial plays against the engine as the
// tutorial page will play it, and each kind of mistake in one is caught.

package tutorials_test

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/tutorial"
)

func shippedCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatal(err)
		}
	}
	return cat
}

// TestEveryShippedTutorialPlays is the guarantee that a tutorial never
// teaches rules the engine no longer has: a rule change that makes any
// scripted move illegal fails here.
func TestEveryShippedTutorialPlays(t *testing.T) {
	cat := shippedCatalog(t)
	count := 0
	for _, m := range app.BuiltinModules() {
		tuts, err := tutorial.Load(m.Tutorials)
		if err != nil {
			t.Errorf("%s: %v", m.ID, err)
			continue
		}
		for _, tu := range tuts {
			count++
			if err := tutorial.Verify(cat, m.ID, tu); err != nil {
				t.Errorf("%s: %v", m.ID, err)
			}
		}
	}
	if count == 0 {
		t.Fatal("no tutorials found")
	}
	t.Logf("%d tutorials play", count)
}

const good = `id: t
kind: rules
title: T
game: blooms-4
steps:
  - text: First.
    play: [red@d4]
    reply: [blue@a4, black@g1]
  - text: Then.
    play: [red@b3, yellow@b4]
`

func load(t *testing.T, files map[string]string) ([]tutorial.Tutorial, error) {
	t.Helper()
	fsys := fstest.MapFS{}
	for k, v := range files {
		fsys[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return tutorial.Load(fsys)
}

func wantErr(t *testing.T, err error, part string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), part) {
		t.Fatalf("error %v, want one mentioning %q", err, part)
	}
}

func TestVerify_CatchesMistakes(t *testing.T) {
	cat := shippedCatalog(t)
	cases := map[string]struct {
		from, to, family string
		want             string
	}{
		"an illegal move":        {"play: [red@b3, yellow@b4]", "play: [red@b3, red@b4]", "blooms", "not legal"},
		"a reply on your turn":   {"play: [red@d4]\n    reply: [blue@a4, black@g1]", "reply: [blue@a4]", "blooms", "not a reply"},
		"your move on theirs":    {"    reply: [blue@a4, black@g1]\n", "", "blooms", "not the learner's"},
		"an unknown notation":    {"play: [red@d4]", "play: [purple@d4]", "blooms", "purple"},
		"a square off the board": {"  - text: Then.", "  - text: Then.\n    highlight: [z9]", "blooms", `"z9" is not a square`},
		"another game's file":    {"game: blooms-4", "game: blooms-4", "meridians", "not of \"meridians\""},
		"an unknown game":        {"game: blooms-4", "game: blooms-9", "blooms", `no game "blooms-9"`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			tuts, err := load(t, map[string]string{"t.yaml": strings.Replace(good, c.from, c.to, 1)})
			if err != nil {
				t.Fatal(err)
			}
			wantErr(t, tutorial.Verify(cat, c.family, tuts[0]), c.want)
		})
	}
	tuts, err := load(t, map[string]string{"t.yaml": good})
	if err != nil {
		t.Fatal(err)
	}
	if err := tutorial.Verify(cat, "blooms", tuts[0]); err != nil {
		t.Fatalf("the good tutorial: %v", err)
	}
}

// A game with chance must fix its seed; with one, the dice repeat.
func TestVerify_ChanceNeedsASeed(t *testing.T) {
	cat := shippedCatalog(t)
	doc := "id: t\nkind: rules\ntitle: T\ngame: courtfall\nsteps:\n  - text: Look.\n"
	tuts, err := load(t, map[string]string{"t.yaml": doc})
	if err != nil {
		t.Fatal(err)
	}
	wantErr(t, tutorial.Verify(cat, "courtfall", tuts[0]), "name a seed")
	tuts, _ = load(t, map[string]string{"t.yaml": strings.Replace(doc, "game: courtfall", "game: courtfall\nseed: 7", 1)})
	if err := tutorial.Verify(cat, "courtfall", tuts[0]); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_Mistakes(t *testing.T) {
	cases := map[string]struct {
		files map[string]string
		want  string
	}{
		"a misspelled field":  {map[string]string{"t.yaml": strings.Replace(good, "title: T", "titel: T", 1)}, `t.yaml:3:1: unknown field "titel"`},
		"a field in a step":   {map[string]string{"t.yaml": strings.Replace(good, "    play: [red@d4]", "    plya: [red@d4]", 1)}, `unknown field "plya" in Step`},
		"a wrong kind":        {map[string]string{"t.yaml": strings.Replace(good, "kind: rules", "kind: lore", 1)}, "not rules, strategy or variant"},
		"no steps":            {map[string]string{"t.yaml": "id: t\nkind: rules\ntitle: T\ngame: g\nsteps: []\n"}, "needs steps"},
		"a step without text": {map[string]string{"t.yaml": strings.Replace(good, "text: Then.", "text: ' '", 1)}, "step 2 has no text"},
		"a hint, no move":     {map[string]string{"t.yaml": good + "  - text: More.\n    hint: Nothing to do.\n"}, "hint but asks for no move"},
		"a one-square arrow":  {map[string]string{"t.yaml": good + "  - text: More.\n    arrow: [d4]\n"}, "two squares"},
		"misnamed file":       {map[string]string{"other.yaml": good}, "name the file after the tutorial"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := load(t, c.files)
			wantErr(t, err, c.want)
		})
	}
}

func TestLoad_SortsByKindThenOrder(t *testing.T) {
	doc := func(id, kind string, order int) string {
		return strings.NewReplacer("id: t", "id: "+id, "kind: rules", "kind: "+kind, "title: T", "title: T\norder: "+string(rune('0'+order))).Replace(good)
	}
	tuts, err := load(t, map[string]string{
		"v.yaml": doc("v", "variant", 1), "s.yaml": doc("s", "strategy", 1),
		"r2.yaml": doc("r2", "rules", 2), "r1.yaml": doc("r1", "rules", 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tu := range tuts {
		got = append(got, tu.ID)
	}
	if strings.Join(got, ",") != "r1,r2,s,v" {
		t.Fatalf("order %v", got)
	}
}

// TestSchema_UpToDate fails when the tutorial types changed and
// docs/tutorial.schema.json was not regenerated.
func TestSchema_UpToDate(t *testing.T) {
	want, err := tutorial.Schema("../../internal/tutorial")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../docs/tutorial.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("docs/tutorial.schema.json is out of date: run go run ./scripts/genschema")
	}
}
