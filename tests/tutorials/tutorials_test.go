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

// The page compares the learner's move with the script by its notation, so
// the script must spell each move as the engine writes it.
func TestVerify_CanonicalNotation(t *testing.T) {
	cat := shippedCatalog(t)
	doc := "id: t\nkind: rules\ntitle: T\ngame: gomoku\nsteps:\n  - text: Go.\n    play: [stone@h8]\n"
	tuts, err := load(t, map[string]string{"t.yaml": doc})
	if err != nil {
		t.Fatal(err)
	}
	wantErr(t, tutorial.Verify(cat, "gomoku", tuts[0]), `write it as the game does, "h8"`)
	tuts, _ = load(t, map[string]string{"t.yaml": strings.Replace(doc, "stone@h8", "h8", 1)})
	if err := tutorial.Verify(cat, "gomoku", tuts[0]); err != nil {
		t.Fatal(err)
	}
}

// A step can show another position (Step.Show), reached by legal moves from
// the start; it asks for no moves of its own.
func TestShow(t *testing.T) {
	cat := shippedCatalog(t)
	withShow := strings.Replace(good, "  - text: Then.\n", "  - text: A picture.\n    show: [red@e5, blue@d4, black@d3]\n  - text: Then.\n", 1)
	tuts, err := load(t, map[string]string{"t.yaml": withShow})
	if err != nil {
		t.Fatal(err)
	}
	if err := tutorial.Verify(cat, "blooms", tuts[0]); err != nil {
		t.Fatalf("a legal picture: %v", err)
	}
	tuts, _ = load(t, map[string]string{"t.yaml": strings.Replace(withShow, "black@d3", "black@e5", 1)})
	wantErr(t, tutorial.Verify(cat, "blooms", tuts[0]), "shown move 3")
	_, err = load(t, map[string]string{"t.yaml": strings.Replace(good, "    play: [red@d4]\n", "    play: [red@d4]\n    show: [red@d4]\n", 1)})
	wantErr(t, err, "cannot ask for moves too")
}

// The guide tells a client what to point the learner at: each expected
// move's selections, and the position a step shows.
func TestGuide(t *testing.T) {
	cat := shippedCatalog(t)
	game, _ := cat.Get("blooms-4")
	withShow := strings.Replace(good, "  - text: Then.\n", "  - text: A picture.\n    show: [red@e5]\n  - text: Then.\n", 1)
	tuts, _ := load(t, map[string]string{"t.yaml": withShow})
	g, err := tutorial.Guide(game, tuts[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(g) != 3 {
		t.Fatalf("%d steps guided", len(g))
	}
	first := g[0].PlaySteps
	if len(first) != 1 || len(first[0]) != 2 || first[0][0].Kind != "piece" || first[0][0].Value != "red" || first[0][1].Kind != "pos" || first[0][1].Value != "d4" {
		t.Fatalf("the first move's selections: %+v", first)
	}
	if g[1].Shown == nil || len(g[1].PlaySteps) != 0 {
		t.Fatalf("the picture: %+v", g[1])
	}
	if top, ok := g[1].Shown.Get("e5"); !ok || top.Piece != "red" {
		t.Fatalf("the picture lacks its stone: %v %v", top, ok)
	}
	if len(g[2].PlaySteps) != 2 || g[2].PlaySteps[1][0].Value != "yellow" {
		t.Fatalf("the second step's selections: %+v", g[2].PlaySteps)
	}
}

// A tutorial can begin from a position written out (Start) instead of the
// game's start, and a step can picture one (Position); both are checked as
// a custom start is, and the steps are replayed from the written start.
func TestStartAndPosition(t *testing.T) {
	cat := shippedCatalog(t)
	doc := `id: t
kind: rules
title: T
game: gomoku
start: {v: 2, cells: {h8: [{piece: stone, color: black}], h9: [{piece: stone, color: white}]}, ply: 2}
start_seat: 1
steps:
  - text: A picture.
    position: {v: 2, cells: {a1: [{piece: stone, color: black}]}, ply: 1}
  - text: Play on from the written start.
    play: [h10]
`
	tuts, err := load(t, map[string]string{"t.yaml": doc})
	if err != nil {
		t.Fatal(err)
	}
	if err := tutorial.Verify(cat, "gomoku", tuts[0]); err != nil {
		t.Fatalf("a written start: %v", err)
	}
	game, _ := cat.Get("gomoku")
	st, seat, err := tutorial.Start(game, tuts[0])
	if err != nil || seat != 1 {
		t.Fatalf("Start: seat %d, %v", seat, err)
	}
	if top, ok := st.Get("h9"); !ok || top.Color != "white" {
		t.Fatalf("the room does not begin from the written start: %v", top)
	}
	g, err := tutorial.Guide(game, tuts[0])
	if err != nil || g[0].Shown == nil {
		t.Fatalf("the pictured position: %+v %v", g, err)
	}
	if top, ok := g[0].Shown.Get("a1"); !ok || top.Color != "black" {
		t.Fatal("the picture is not the written position")
	}

	// A move the written start already holds is not legal.
	bad, _ := load(t, map[string]string{"t.yaml": strings.Replace(doc, "play: [h10]", "play: [h9]", 1)})
	wantErr(t, tutorial.Verify(cat, "gomoku", bad[0]), "not legal")
	// A written position the game refuses is caught.
	bad, _ = load(t, map[string]string{"t.yaml": strings.Replace(doc, "color: white}]}, ply: 2}", "color: purple}]}, ply: 2}", 1)})
	wantErr(t, tutorial.Verify(cat, "gomoku", bad[0]), "start")
	_, err = load(t, map[string]string{"t.yaml": strings.Replace(doc, "    position: {v: 2", "    show: [h1]\n    position: {v: 2", 1)})
	wantErr(t, err, "only one of them")
}

func pictureStep(extra string) string {
	return "id: pic\nkind: rules\ntitle: Pictures\ngame: blooms-4\nsteps:\n  - text: A picture.\n" + extra
}

func verifyPicture(t *testing.T, extra string) error {
	t.Helper()
	tuts, err := load(t, map[string]string{"pic.yaml": pictureStep(extra)})
	if err != nil {
		return err
	}
	return tutorial.Verify(shippedCatalog(t), "blooms", tuts[0])
}

// A picture need not be a position play could reach, but it must be one the game
// can draw, and a step is only one kind of thing.
func TestPictures(t *testing.T) {
	if err := verifyPicture(t, "    pieces: [blue@g1, red@g2, red@g3, yellow@g4]\n"); err != nil {
		t.Errorf("a board no legal game has: %v", err)
	}
	if err := verifyPicture(t, "    pieces: [red@g2]\n    arrow: [g2, g3]\n    illegal: true\n"); err != nil {
		t.Errorf("an illegal arrow: %v", err)
	}
	if err := verifyPicture(t, "    highlight: [d4]\n    illegal: true\n"); err != nil {
		t.Errorf("an illegal highlight on the room's own board: %v", err)
	}
	for name, c := range map[string]struct{ extra, want string }{
		"no such square":   {"    pieces: [red@z9]\n", "not a square"},
		"not a piece":      {"    pieces: [red]\n", "piece@square"},
		"unknown piece":    {"    pieces: [dragon@d4]\n", "dragon"},
		"unknown colour":   {"    pieces: [\"red:plaid@d4\"]\n", "plaid"},
		"two on one cell":  {"    pieces: [red@d4, blue@d4]\n", "two stones"},
		"two kinds":        {"    pieces: [red@d4]\n    show: [red@d4]\n", "only one"},
		"and moves":        {"    pieces: [red@d4]\n    play: [red@d4]\n", "cannot ask for moves"},
		"illegal, nothing": {"    illegal: true\n", "no arrow or highlight"},
		"illegal, moves":   {"    highlight: [d4]\n    play: [red@d4]\n    illegal: true\n", "cannot ask for moves"},
		"arrow off board":  {"    pieces: [red@d4]\n    arrow: [d4, z9]\n    illegal: true\n", "not a square"},
	} {
		t.Run(name, func(t *testing.T) { wantErr(t, verifyPicture(t, c.extra), c.want) })
	}
}
