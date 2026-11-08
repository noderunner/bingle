// Package pack_test holds a data pack to ADR 0128: what a good one becomes, and
// everything a hostile one might try.
package pack_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/pack"
)

const goodDefinition = `id: lines
name: Lines
description: Three in a row on a small board.
version: 1.2.0
rules:
  - Place a stone. Three in a row wins.
guide:
  tabs:
    - {id: rules, title: Rules, file: guide/rules.md}
board:
  generator: {type: grid, width: 3, height: 3, adjacency: orthogonal8, labels: algebraic}
seats:
  - {seat: 1, color: red, label: Red}
  - {seat: 2, color: blue, label: Blue}
pieces:
  stone: {label: Stone}
turn_order:
  type: round-robin
  seats: [1, 2]
actions:
  - {id: place, type: place, label: Place a stone, pieces: [stone]}
win_condition:
  type: rules
  rules:
    - {id: full, type: end, when: [no-legal-moves], decide: {type: most, piece: stone}, reason: full}
variants:
  - id: lines-big
    name: Lines (4×4)
    patch:
      board: {generator: {width: 4, height: 4}}
`

const goodLook = `pieces:
  stone: {shape: disc}
variants:
  lines-big: {board: {style: points}}
`

var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00")

func good() map[string][]byte {
	return map[string][]byte{
		"game.yaml":      []byte(goodDefinition),
		"web.yaml":       []byte(goodLook),
		"guide/rules.md": []byte("# Lines\n\n![a line](line.png)\n"),
		"guide/line.png": pngHeader,
		"guide/line.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
	}
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A good pack becomes a game named for its version, with its variants, its guide
// and its look, that registers in a catalog beside another version of itself.
func TestRead_AGoodPack(t *testing.T) {
	p, err := pack.Read(zipOf(t, good()))
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "lines" || p.Version != "1.2.0" || p.Family != "pack-lines" || p.Home != "pack-lines-1.2.0" || p.Title != "Lines" {
		t.Errorf("the pack reads as %+v", p)
	}
	if strings.Join(p.Games, ",") != "pack-lines-1.2.0,pack-lines-1.2.0-big" {
		t.Errorf("games: %v", p.Games)
	}
	if len(p.SHA256) != 64 {
		t.Errorf("hash: %q", p.SHA256)
	}
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(p.Module); err != nil {
		t.Fatal(err)
	}
	newer := good()
	newer["game.yaml"] = []byte(strings.Replace(goodDefinition, "version: 1.2.0", "version: 1.10.0", 1))
	q, err := pack.Read(zipOf(t, newer))
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterModule(q.Module); err != nil {
		t.Fatal(err)
	}
	if got := cat.Pin("pack-lines"); got != "pack-lines-1.10.0" {
		t.Errorf("the family's name means %q, want the newest version", got)
	}
	if _, ok := cat.Get("pack-lines-1.2.0"); !ok {
		t.Error("the older version is gone")
	}
	if cg, ok := cat.Get("pack-lines-1.2.0-big"); !ok || cg.Web == nil {
		t.Error("the older version's variant lost its look")
	}
}

func refused(t *testing.T, name string, files map[string][]byte, want string) {
	t.Helper()
	_, err := pack.Read(zipOf(t, files))
	if err == nil {
		t.Errorf("%s: accepted", name)
	} else if !strings.Contains(err.Error(), want) {
		t.Errorf("%s: %v, want it to say %q", name, err, want)
	}
}

func with(name, content string) map[string][]byte {
	files := good()
	files[name] = []byte(content)
	return files
}

func definitionWith(old, new string) map[string][]byte {
	if !strings.Contains(goodDefinition, old) {
		panic(old)
	}
	return with("game.yaml", strings.Replace(goodDefinition, old, new, 1))
}

// What needs Go, or would decide what the server decides, is refused, however
// it is asked for, a variant's patch included.
func TestRead_ADefinitionIsDataOnly(t *testing.T) {
	for name, c := range map[string]struct {
		files map[string][]byte
		want  string
	}{
		"rules in Go":            {definitionWith("rules:\n", "engine: ruleset\nrules:\n"), "engine"},
		"rules in Go by a patch": {definitionWith("name: Lines (4×4)\n    patch:\n", "name: Lines (4×4)\n    patch:\n      engine: ruleset\n"), "engine"},
		"a callback win":         {definitionWith("win_condition:\n  type: rules\n  rules:\n    - {id: full, type: end, when: [no-legal-moves], decide: {type: most, piece: stone}, reason: full}\n", "win_condition: {type: callback}\n"), "callback"},
		"its own family":         {definitionWith("rules:\n", "family: tak\nrules:\n"), "family"},
		"prototype":              {definitionWith("rules:\n", "prototype: false\nrules:\n"), "prototype"},
		"a family by a patch":    {definitionWith("name: Lines (4×4)\n    patch:\n", "name: Lines (4×4)\n    patch:\n      family: tak\n"), "family"},
		"a version by a patch":   {definitionWith("name: Lines (4×4)\n    patch:\n", "name: Lines (4×4)\n    patch:\n      version: 9.9.9\n"), "version"},
		"no version":             {definitionWith("version: 1.2.0\n", ""), "no version"},
		"a bad version":          {definitionWith("version: 1.2.0\n", "version: soon\n"), "version"},
		"an id of a path":        {definitionWith("id: lines\n", "id: ../lines\n"), "id"},
		"a derived id":           {definitionWith("id: lines\n", "id: tak~x\n"), "id"},
		"an id with the prefix":  {definitionWith("id: lines\n", "id: pack-tak\n"), "own name"},
		"an upper-case id":       {definitionWith("id: lines\n", "id: Lines\n"), "id"},
		"a stray variant":        {definitionWith("  - id: lines-big\n", "  - id: tak-5x5\n"), "variant"},
		"a colour with a call":   {definitionWith("color: red, label: Red", "color: 'url(https://example.org/x)', label: Red"), "colour"},
		"no definition":          {func() map[string][]byte { f := good(); delete(f, "game.yaml"); return f }(), "game.yaml"},
		"an unknown key":         {definitionWith("rules:\n", "bogus: 1\nrules:\n"), "bogus"},
	} {
		refused(t, name, c.files, c.want)
	}
}

// Only the prefix itself is reserved: a game may be called Packrat.
func TestRead_ANameMayStartWithPack(t *testing.T) {
	files := definitionWith("id: lines\n", "id: packrat\n")
	files["game.yaml"] = bytes.Replace(files["game.yaml"], []byte("lines-big"), []byte("packrat-big"), 1)
	files["web.yaml"] = []byte("pieces:\n  stone: {shape: disc}\n")
	p, err := pack.Read(zipOf(t, files))
	if err != nil {
		t.Fatal(err)
	}
	if p.Home != "pack-packrat-1.2.0" {
		t.Errorf("home %q", p.Home)
	}
}

// A look is data: nothing in it runs in a player's browser or calls another server.
func TestRead_ALookRunsNothing(t *testing.T) {
	for name, c := range map[string]struct {
		look, want string
	}{
		"a script":              {"script: page.js\n", "script"},
		"a stylesheet":          {"style: page.css\n", "style"},
		"a script by a variant": {"variants:\n  lines-big: {script: page.js}\n", "script"},
		"a style by a variant":  {"variants:\n  lines-big: {style: page.css}\n", "style"},
		"a url for a colour":    {"display:\n  tints:\n    last: {color: 'url(https://example.org/x)'}\n", "colour"},
		"a rule for a colour":   {"cards:\n  back: {color: 'red; background: url(x)'}\n", "colour"},
		"a stray variant":       {"variants:\n  tak-5x5: {board: {style: points}}\n", "variant"},
		"an unknown key":        {"bogus: 1\n", "bogus"},
	} {
		refused(t, name, with("web.yaml", c.look), c.want)
	}
}

func TestRead_AColourMayBeAColour(t *testing.T) {
	def := strings.Replace(goodDefinition, "rules:\n", "display:\n  tints:\n    last: {label: Last}\n    two: {label: Two}\n    three: {label: Three}\nrules:\n", 1)
	files := with("game.yaml", def)
	files["web.yaml"] = []byte("display:\n  tints:\n    last: {color: '#c8dfa8'}\n    two: {color: 'rgb(10, 20, 30)'}\n    three: {color: teal}\n")
	if _, err := pack.Read(zipOf(t, files)); err != nil {
		t.Error(err)
	}
}

// Only text and real pictures, by what they are and not what they are called.
func TestRead_FilesAreWhatTheySay(t *testing.T) {
	for name, c := range map[string]struct {
		file, content, want string
	}{
		"a script":              {"page.js", "alert(1)", "no other kind of file"},
		"a stylesheet":          {"page.css", "body{}", "no other kind of file"},
		"a web page":            {"guide/x.html", "<script>alert(1)</script>", "no other kind of file"},
		"no extension":          {"run", "#!/bin/sh", "no other kind of file"},
		"a program as a png":    {"guide/x.png", "MZ\x90\x00", "not image/png"},
		"html as a jpeg":        {"guide/x.jpg", "<html><script>alert(1)</script>", "not image/jpeg"},
		"markdown that is not":  {"guide/x.md", "a\x00b", "not text"},
		"an svg that is not":    {"guide/x.svg", "hello", "not an SVG"},
		"a path out":            {"../evil.md", "x", "not a path"},
		"an absolute path":      {"/etc/evil.md", "x", "not a path"},
		"a backslash path":      {`guide\..\evil.md`, "x", "not a path"},
		"a dotted path":         {"guide/./x.md", "x", "not a path"},
		"the same file by case": {"GUIDE/RULES.MD", "x", "twice"},
	} {
		refused(t, name, with(c.file, c.content), c.want)
	}
}

func TestRead_ALinkIsRefused(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range good() {
		f, _ := w.Create(name)
		_, _ = f.Write(content)
	}
	h := &zip.FileHeader{Name: "guide/link.md"}
	h.SetMode(0o777 | 1<<27) // fs.ModeSymlink
	f, err := w.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("/etc/passwd"))
	_ = w.Close()
	if _, err := pack.Read(buf.Bytes()); err == nil || !strings.Contains(err.Error(), "plain file") {
		t.Errorf("a link: %v", err)
	}
}

// A zip bomb stops at the limit, not at the end of the memory.
func TestRead_TheLimits(t *testing.T) {
	if _, err := pack.Read(make([]byte, pack.MaxPackBytes+1)); err == nil || !strings.Contains(err.Error(), "over the limit") {
		t.Errorf("a big zip: %v", err)
	}
	if _, err := pack.Read([]byte("not a zip")); err == nil || !strings.Contains(err.Error(), "not a zip") {
		t.Errorf("not a zip: %v", err)
	}
	refused(t, "a bomb", with("guide/bomb.md", strings.Repeat("a", pack.MaxFileBytes+1)), "unpacks to over")

	total := good()
	for i := range pack.MaxTotalBytes/pack.MaxFileBytes + 1 {
		total[fmt.Sprintf("guide/big%d.md", i)] = bytes.Repeat([]byte("a"), pack.MaxFileBytes)
	}
	refused(t, "many big files", total, "unpacks to over")

	many := good()
	for i := range pack.MaxFiles {
		many[fmt.Sprintf("guide/%d.md", i)] = []byte("x")
	}
	refused(t, "too many files", many, "entries")
}

// Each variant is a whole copy of the definition, so their number is limited too.
func TestRead_TooManyVariants(t *testing.T) {
	var b strings.Builder
	b.WriteString(goodDefinition)
	for i := range dsl.MaxVariants + 1 {
		fmt.Fprintf(&b, "  - {id: lines-v%d}\n", i)
	}
	files := with("game.yaml", b.String())
	files["web.yaml"] = []byte("pieces:\n  stone: {shape: disc}\n")
	refused(t, "too many variants", files, "over the limit")
}

func TestRead_ADefinitionTooLong(t *testing.T) {
	long := strings.Replace(goodDefinition, "rules:\n", "rules:\n  - "+strings.Repeat("a", pack.MaxDefinitionBytes)+"\n", 1)
	refused(t, "a long definition", with("game.yaml", long), "over the limit")
}

// YAML that expands to a great deal, or includes itself, is the loader's limit.
func TestRead_YAMLCannotExplode(t *testing.T) {
	var b strings.Builder
	b.WriteString("a0: &a0 [x,x,x,x,x,x,x,x,x,x]\n")
	for i := 1; i < 12; i++ {
		fmt.Fprintf(&b, "a%d: &a%d [*a%d,*a%d,*a%d,*a%d,*a%d,*a%d,*a%d,*a%d,*a%d,*a%d]\n", i, i, i-1, i-1, i-1, i-1, i-1, i-1, i-1, i-1, i-1, i-1)
	}
	b.WriteString(goodDefinition)
	refused(t, "a billion laughs", with("game.yaml", b.String()), "")
	refused(t, "an include of itself", with("game.yaml", strings.Replace(goodDefinition, "rules:\n", "rules: !include game.yaml\n#", 1)), "")
}

// A definition that compiles but cannot be played stops here.
func TestRead_ADefinitionThatDoesNotCompile(t *testing.T) {
	refused(t, "a guide with no page", definitionWith("file: guide/rules.md", "file: guide/none.md"), "none.md")
	refused(t, "an action on no piece", definitionWith("pieces: [stone]", "pieces: [ghost]"), "ghost")
	refused(t, "a board too big", definitionWith("width: 3, height: 3", "width: 5000, height: 5000"), "board")
}
