// Package pack_test holds install packs to ADR 0147: the fixture pack loads
// with its games named for its version, and a pack made bad in any one way
// is refused with a reason, without stopping the others.
package pack_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/wasmrules"
	"github.com/noderunner/bingle/internal/pack"
	"github.com/noderunner/bingle/tests/testdata"
)

var shared = sync.OnceValues(func() (*wasmrules.Runtime, error) {
	return wasmrules.New(context.Background(), wasmrules.Options{Log: slog.New(slog.DiscardHandler)})
})

func runtime(t *testing.T) *wasmrules.Runtime {
	t.Helper()
	rt, err := shared()
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func read(t *testing.T, dir string) (*pack.Pack, error) {
	t.Helper()
	p, err := pack.Read(context.Background(), runtime(t), dir)
	if err == nil {
		t.Cleanup(func() { _ = p.Close(context.Background()) })
	}
	return p, err
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func edit(t *testing.T, dir, name, from, to string) {
	t.Helper()
	path := filepath.Join(dir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(from)) {
		t.Fatalf("%s has no %q", name, from)
	}
	if err := os.WriteFile(path, bytes.Replace(raw, []byte(from), []byte(to), 1), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTheFixturePackLoads(t *testing.T) {
	_, dir := testdata.DraughtsPack(t)
	p, err := read(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Family != "pack-draughts" || p.Home != testdata.DraughtsPackID || p.Title != "Draughts" {
		t.Errorf("family %q, home %q, title %q", p.Family, p.Home, p.Title)
	}
	if want := []string{testdata.DraughtsPackID, testdata.DraughtsPackSmallID}; !slices.Equal(p.Games, want) {
		t.Errorf("games %v, want %v", p.Games, want)
	}
	if p.Module.Private {
		t.Error("a pack without private: true is private")
	}
	if p.Module.Scripts != nil {
		t.Error("a pack brings page scripts")
	}

	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(p.Module); err != nil {
		t.Fatal(err)
	}
	for _, id := range p.Games {
		cg, ok := cat.Get(id)
		if !ok {
			t.Fatalf("%s is not registered", id)
		}
		if d := cg.Definition; !d.Prototype || d.Family != p.Family || d.Version != "0.1.0" {
			t.Errorf("%s: prototype %v, family %q, version %q", id, d.Prototype, d.Family, d.Version)
		}
	}
	// The family's name is the newest version (ADR 0098).
	if got := cat.Pin("pack-draughts"); got != testdata.DraughtsPackID {
		t.Errorf("pack-draughts means %q", got)
	}
	// The guide is served, and nothing else of the pack.
	cg, _ := cat.Get(testdata.DraughtsPackID)
	for name, want := range map[string]bool{"rules.md": true, "board.svg": true, "rules.wasm": false, "manifest.yaml": false, "game.yaml": false, "guide/rules.md": false} {
		_, err := cg.Assets.Open(name)
		if got := err == nil; got != want {
			t.Errorf("the game's files have %s: %v, want %v", name, got, want)
		}
	}
}

func TestAPrivatePackIsMarked(t *testing.T) {
	_, dir := testdata.DraughtsPack(t)
	edit(t, dir, "manifest.yaml", "abi: 0", "abi: 0\nprivate: true")
	p, err := read(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Module.Private {
		t.Error("private: true is not marked")
	}
}

// Each pack here is the fixture made bad in one way, and is refused with a
// reason that says so.
func TestABadPackIsRefused(t *testing.T) {
	cases := []struct {
		name string
		bad  func(t *testing.T, dir string)
		says string
	}{
		{"no manifest", func(t *testing.T, dir string) { _ = os.Remove(filepath.Join(dir, "manifest.yaml")) }, "no manifest.yaml"},
		{"another ABI", func(t *testing.T, dir string) { edit(t, dir, "manifest.yaml", "abi: 0", "abi: 7") }, "ABI 7"},
		{"no ABI", func(t *testing.T, dir string) { edit(t, dir, "manifest.yaml", "abi: 0", "") }, "no abi"},
		{"a name that is not one", func(t *testing.T, dir string) { edit(t, dir, "manifest.yaml", "name: draughts", "name: Draughts!") }, "lower-case"},
		{"a version that is not one", func(t *testing.T, dir string) { edit(t, dir, "manifest.yaml", "version: 0.1.0", "version: one") }, "major.minor.patch"},
		{"an unknown manifest field", func(t *testing.T, dir string) { edit(t, dir, "manifest.yaml", "abi: 0", "abi: 0\nauthor: me") }, "author"},
		{"no rules", func(t *testing.T, dir string) { _ = os.Remove(filepath.Join(dir, "rules.wasm")) }, "no rules.wasm"},
		{"rules that are not a module", func(t *testing.T, dir string) { write(t, dir, "rules.wasm", "not a module") }, "not a module"},
		{"a definition that sets its id", func(t *testing.T, dir string) {
			edit(t, dir, "game.yaml", "name: Draughts", "id: draughts\nname: Draughts")
		}, `sets "id"`},
		{"a definition that sets its version", func(t *testing.T, dir string) {
			edit(t, dir, "game.yaml", "name: Draughts", "version: 9.9.9\nname: Draughts")
		}, `sets "version"`},
		{"a definition that is not a ruleset's", func(t *testing.T, dir string) { edit(t, dir, "game.yaml", "engine: ruleset\n", "") }, "engine: ruleset"},
		{"a variant patched out of its family", func(t *testing.T, dir string) {
			edit(t, dir, "game.yaml", "      params: {rows: 2, quiet: 40}", "      family: other\n      params: {rows: 2, quiet: 40}")
		}, "changes the family"},
		{"a variant named after another game", func(t *testing.T, dir string) { edit(t, dir, "game.yaml", "id: draughts-6", "id: checkers-6") }, "has to be named"},
		{"a colour that is not a colour", func(t *testing.T, dir string) {
			edit(t, dir, "web.yaml", "  no_hands: true", "  no_hands: true\n  tints:\n    last: {color: 'url(https://example.com/x)'}")
		}, "not a colour"},
		{"a look with a script", func(t *testing.T, dir string) { edit(t, dir, "web.yaml", "board:", "script: draughts.js\nboard:") }, "names a script"},
		{"a look with a style", func(t *testing.T, dir string) { edit(t, dir, "web.yaml", "board:", "style: draughts.css\nboard:") }, "names a style"},
		{"a look that does not fit the game", func(t *testing.T, dir string) { edit(t, dir, "web.yaml", "man: {shape: disc}", "pawn: {shape: disc}") }, "pawn"},
		{"a picture that is not one", func(t *testing.T, dir string) { write(t, dir, "guide/board.png", "<script>alert(1)</script>") }, "not image/png"},
		{"an SVG that is not one", func(t *testing.T, dir string) { write(t, dir, "guide/board.svg", "hello") }, "not an SVG"},
		{"text that is not text", func(t *testing.T, dir string) { write(t, dir, "guide/rules.md", "rules\x00\xff") }, "not text"},
		{"a kind of file a pack does not hold", func(t *testing.T, dir string) { write(t, dir, "guide/page.html", "<p>hi</p>") }, "no other kind of file"},
		{"a file beside the others", func(t *testing.T, dir string) { write(t, dir, "notes.txt", "hello") }, "no other file"},
		{"a folder beside the others", func(t *testing.T, dir string) { write(t, dir, "extra/rules.md", "hello") }, "no other folder"},
		{"a link out of the pack", func(t *testing.T, dir string) {
			outside := filepath.Join(t.TempDir(), "secret.md")
			write(t, filepath.Dir(outside), "secret.md", "a secret")
			if err := os.Symlink(outside, filepath.Join(dir, "guide", "secret.md")); err != nil {
				t.Skip("no links here:", err)
			}
		}, "is a link"},
		{"a link inside the pack", func(t *testing.T, dir string) {
			if err := os.Symlink("rules.md", filepath.Join(dir, "guide", "again.md")); err != nil {
				t.Skip("no links here:", err)
			}
		}, "is a link"},
		{"a file over the limit", func(t *testing.T, dir string) {
			write(t, dir, "guide/rules.md", strings.Repeat("x", pack.MaxFileBytes+1))
		}, "over"},
		{"a guide tab with no file", func(t *testing.T, dir string) { _ = os.Remove(filepath.Join(dir, "guide", "rules.md")) }, "rules.md"},
		{"rules that panic", func(t *testing.T, dir string) {
			testdata.WriteRules(t, dir, testdata.FaultyWasm)
			edit(t, dir, "game.yaml", "params: {rows: 3, quiet: 80}", "params: {rows: 3, quiet: 80, fault: 1}")
		}, "failed while it was played"},
		{"rules that think for ever", func(t *testing.T, dir string) {
			testdata.WriteRules(t, dir, testdata.FaultyWasm)
			edit(t, dir, "game.yaml", "params: {rows: 3, quiet: 80}", "params: {rows: 3, quiet: 80, fault: 2}")
		}, "failed while it was played"},
		{"rules that do not play the game", func(t *testing.T, dir string) {
			edit(t, dir, "game.yaml", "params: {rows: 3, quiet: 80}", "params: {rows: 5, quiet: 80}")
		}, "could not start it"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if testing.Short() && strings.Contains(tc.name, "for ever") {
				t.Skip("waits out the deadline of two calls")
			}
			_, dir := testdata.DraughtsPack(t)
			tc.bad(t, dir)
			_, err := read(t, dir)
			if err == nil {
				t.Fatal("the pack was accepted")
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Fatalf("refused with %q, which does not say %q", err, tc.says)
			}
		})
	}
}

// A pack that fails is left out, and the packs beside it load.
func TestLoadLeavesOutABadPackAndKeepsTheRest(t *testing.T) {
	packs, dir := testdata.DraughtsPack(t)
	bad := filepath.Join(packs, "broken")
	if err := os.CopyFS(bad, os.DirFS(dir)); err != nil {
		t.Fatal(err)
	}
	edit(t, bad, "manifest.yaml", "version: 0.1.0", "version: 0.2.0")
	edit(t, bad, "web.yaml", "board:", "script: x.js\nboard:")
	// The same pack twice is loaded once.
	if err := os.CopyFS(filepath.Join(packs, "again"), os.DirFS(dir)); err != nil {
		t.Fatal(err)
	}
	write(t, packs, "README", "not a pack")

	var logged bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logged, nil))
	loaded, err := pack.Load(context.Background(), runtime(t), packs, log)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range loaded {
		t.Cleanup(func() { _ = p.Close(context.Background()) })
	}
	if len(loaded) != 1 || loaded[0].Home != testdata.DraughtsPackID {
		t.Fatalf("loaded %d packs, want only %s", len(loaded), testdata.DraughtsPackID)
	}
	for _, says := range []string{"names a script", "was loaded already", "README"} {
		if !strings.Contains(logged.String(), says) {
			t.Errorf("the log does not say %q:\n%s", says, logged.String())
		}
	}
}
