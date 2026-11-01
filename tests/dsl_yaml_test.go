// Tests of YAML game definitions: conversion to JSON, includes, anchors and
// merge keys, the check of field names, the limits that guard against hostile
// files, the JSON-to-YAML direction and the generated JSON Schema.

package dsl_test

import (
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/dsl/schemagen"
)

// minimalYAML is a complete definition with nothing to spare.
const minimalYAML = `id: mini
name: Mini
board:
  generator: {type: grid, width: 3, height: 3, adjacency: orthogonal4}
seats:
  - {seat: 1, color: white}
  - {seat: 2, color: black}
pieces:
  stone: {movement: {primitive: step}}
turn_order: {type: round-robin, seats: [1, 2]}
win_condition: {type: none}
`

func load(t *testing.T, files map[string]string, name string) ([]byte, error) {
	t.Helper()
	fsys := fstest.MapFS{}
	for k, v := range files {
		fsys[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return dsl.Load(fsys, name)
}

func mustLoad(t *testing.T, files map[string]string, name string) []byte {
	t.Helper()
	data, err := load(t, files, name)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return data
}

// wantErr checks that err mentions every one of parts.
func wantErr(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error; want one mentioning %q", parts)
	}
	for _, p := range parts {
		if !strings.Contains(err.Error(), p) {
			t.Fatalf("error %q does not mention %q", err, p)
		}
	}
}

// tokenStream is a JSON document as its tokens: two documents with the same
// stream are equal, keys in the same order.
func tokenStream(t *testing.T, data []byte) []any {
	t.Helper()
	d := json.NewDecoder(strings.NewReader(string(data)))
	var out []any
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("not JSON: %v\n%s", err, data)
		}
		out = append(out, tok)
	}
}

func TestLoadYAML_ConvertsAndParses(t *testing.T) {
	data := mustLoad(t, map[string]string{"g.yaml": minimalYAML}, "g.yaml")
	def, err := dsl.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if def.ID != "mini" || def.Board.Generator.Width != 3 || len(def.Seats) != 2 {
		t.Fatalf("parsed %+v", def)
	}
}

func TestLoadYAML_KeepsKeyOrderAndTypes(t *testing.T) {
	src := "id: x\nname: '007'\nparams:\n  zeta: 1\n  alpha: 2.5\n  on: yes\n  flag: true\n  none: null\n  day: 2026-10-31\n  text: |\n    two\n    lines\n"
	// "params" is free-form, so the check lets anything through it.
	got := mustLoad(t, map[string]string{"g.yaml": src}, "g.yaml")
	want := `{"id":"x","name":"007","params":{"zeta":1,"alpha":2.5,"on":"yes","flag":true,"none":null,"day":"2026-10-31","text":"two\nlines\n"}}`
	if !reflect.DeepEqual(tokenStream(t, got), tokenStream(t, []byte(want))) {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestLoadJSON_UnchangedButChecked(t *testing.T) {
	src := `{"id": "x", "name": "y"}`
	got := mustLoad(t, map[string]string{"g.json": src}, "g.json")
	if string(got) != src {
		t.Fatalf("a JSON definition changed: %s", got)
	}
	_, err := load(t, map[string]string{"g.json": "{\n  \"id\": \"x\",\n  \"nmae\": \"y\"\n}"}, "g.json")
	wantErr(t, err, "g.json:3:", `unknown field "nmae"`)
}

func TestLoadYAML_UnknownFields(t *testing.T) {
	cases := map[string]struct {
		src  string
		want []string
	}{
		"top level":   {"id: x\nnmae: y\n", []string{"g.yaml:2:1", `unknown field "nmae"`, "the game definition"}},
		"nested":      {"id: x\nboard:\n  stlye: squares\n", []string{"g.yaml:3:3", `"stlye"`, "Board"}},
		"in a list":   {"id: x\nseats:\n  - {seat: 1, colour: red}\n", []string{"g.yaml:3:15", `"colour"`, "SeatDef"}},
		"map values":  {"id: x\npieces:\n  stone: {lable: S}\n", []string{"g.yaml:3:11", `"lable"`, "PieceDef"}},
		"in a patch":  {"id: x\nvariants:\n  - id: v\n    patch: {board: {regionz: {}}}\n", []string{"g.yaml:4:21", `"regionz"`}},
		"wrong case":  {"id: x\nName: y\n", []string{"g.yaml:2:1", `"Name"`}},
		"an anchor":   {"id: x\nbase: &b {label: B}\n", []string{"g.yaml:2:1", `"base"`}},
		"via a merge": {"id: x\npieces:\n  a: &a {label: A}\n  b: {<<: *a, lable: B}\n", []string{"g.yaml:4:15", `"lable"`}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := load(t, map[string]string{"g.yaml": c.src}, "g.yaml")
			wantErr(t, err, c.want...)
		})
	}
}

func TestLoadYAML_PatchMayDeleteWithNull(t *testing.T) {
	src := "id: x\nvariants:\n  - id: v\n    patch: {preview: null, board: {style: null}}\n"
	mustLoad(t, map[string]string{"g.yaml": src}, "g.yaml")
}

func TestLoadYAML_Includes(t *testing.T) {
	files := map[string]string{
		"def/game.yaml":         "id: x\nboard: !include parts/board.yaml\nvariants:\n  - !include variants/big.yaml\n",
		"def/parts/board.yaml":  "generator: !include gen.yaml\nstyle: squares\n",
		"def/parts/gen.yaml":    "{type: grid, width: 3, height: 3, adjacency: orthogonal4}\n",
		"def/variants/big.yaml": "id: big\npatch:\n  board: {generator: {width: 9}}\n",
	}
	got := mustLoad(t, files, "def/game.yaml")
	want := `{"id":"x","board":{"generator":{"type":"grid","width":3,"height":3,"adjacency":"orthogonal4"},"style":"squares"},` +
		`"variants":[{"id":"big","patch":{"board":{"generator":{"width":9}}}}]}`
	if !reflect.DeepEqual(tokenStream(t, got), tokenStream(t, []byte(want))) {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestLoadYAML_ErrorsNameTheIncludedFile(t *testing.T) {
	files := map[string]string{
		"g.yaml": "id: x\nboard: !include b.yaml\n",
		"b.yaml": "# the board\nstyle: squares\nsytle: hexes\n",
	}
	_, err := load(t, files, "g.yaml")
	wantErr(t, err, "b.yaml:3:1", `"sytle"`)
}

func TestLoadYAML_IncludeRefusals(t *testing.T) {
	cases := map[string]struct {
		files map[string]string
		want  string
	}{
		"outside":    {map[string]string{"d/g.yaml": "board: !include ../../b.yaml\n", "b.yaml": "{}"}, "outside the game's own files"},
		"deep out":   {map[string]string{"g.yaml": "board: !include a/../../b.yaml\n"}, "outside the game's own files"},
		"absolute":   {map[string]string{"g.yaml": "board: !include /etc/passwd\n"}, "relative"},
		"backslash":  {map[string]string{"g.yaml": "board: !include a\\b.yaml\n"}, "forward slashes"},
		"empty":      {map[string]string{"g.yaml": "board: !include ''\n"}, "empty file name"},
		"missing":    {map[string]string{"g.yaml": "board: !include b.yaml\n"}, "b.yaml"},
		"cycle":      {map[string]string{"g.yaml": "board: !include b.yaml\n", "b.yaml": "x: !include g.yaml\n"}, "includes itself"},
		"self":       {map[string]string{"g.yaml": "board: !include g.yaml\n"}, "includes itself"},
		"not a name": {map[string]string{"g.yaml": "board: !include {a: 1}\n"}, "takes a file name"},
		"from JSON":  {map[string]string{"g.json": `{"board": !include "b.yaml"}`}, "JSON definition cannot include"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			main := "g.yaml"
			for k := range c.files {
				if strings.HasPrefix(k, "d/") || strings.HasSuffix(k, ".json") {
					main = k
				}
			}
			_, err := load(t, c.files, main)
			wantErr(t, err, c.want)
		})
	}
}

func TestLoadYAML_IncludeDepthLimit(t *testing.T) {
	files := map[string]string{}
	for i := 0; i <= dsl.MaxIncludeDepth; i++ {
		files[name(i)] = "x: !include " + name(i+1) + "\n"
	}
	files[name(dsl.MaxIncludeDepth+1)] = "{}"
	_, err := load(t, files, name(0))
	wantErr(t, err, "nest more than")
}

func name(i int) string { return "f" + strings.Repeat("i", i) + ".yaml" }

func TestLoadYAML_RefusesSymlinks(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.yaml")
	if err := os.WriteFile(secret, []byte("style: squares\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "b.yaml")); err != nil {
		t.Skip("no symbolic links here:", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "g.yaml"), []byte("board: !include b.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	for _, fsys := range []fs.FS{os.DirFS(dir), root.FS()} {
		_, err := dsl.Load(fsys, "g.yaml")
		wantErr(t, err, "not a regular file")
	}
}

func TestLoadYAML_AnchorsAndMergeKeys(t *testing.T) {
	src := `id: x
pieces:
  base: &base {label: Base, code: B, shape: square}
  other: {<<: *base, label: Other}
  both:
    <<: [{label: First, code: F}, *base]
    shape: round
`
	got := mustLoad(t, map[string]string{"g.yaml": src}, "g.yaml")
	want := `{"id":"x","pieces":{` +
		`"base":{"label":"Base","code":"B","shape":"square"},` +
		`"other":{"code":"B","shape":"square","label":"Other"},` +
		`"both":{"label":"First","code":"F","shape":"round"}}}`
	if !reflect.DeepEqual(tokenStream(t, got), tokenStream(t, []byte(want))) {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestLoadYAML_RefusesDuplicateKeys(t *testing.T) {
	_, err := load(t, map[string]string{"g.yaml": "id: x\nname: a\nname: b\n"}, "g.yaml")
	wantErr(t, err, "g.yaml:3:1", `"name" appears twice`)
}

func TestLoadYAML_AliasBombIsCapped(t *testing.T) {
	var b strings.Builder
	b.WriteString("id: x\nparams:\n  a0: &a0 [" + strings.Repeat(`"xxxxxxxxxxxxxxxx",`, 64) + "]\n")
	for i := 1; i < 12; i++ {
		b.WriteString("  a" + strconv.Itoa(i) + ": &a" + strconv.Itoa(i) + " [" + strings.Repeat("*a"+strconv.Itoa(i-1)+",", 16) + "]\n")
	}
	_, err := load(t, map[string]string{"g.yaml": b.String()}, "g.yaml")
	wantErr(t, err, "expands to more than")
}

func TestLoadYAML_NotJSONValues(t *testing.T) {
	_, err := load(t, map[string]string{"g.yaml": "id: x\nparams: {a: .inf}\n"}, "g.yaml")
	wantErr(t, err, "g.yaml:2:", "not a JSON value")
}

func TestParse_RefusesUnknownFieldsAndTrailingText(t *testing.T) {
	base := mustLoad(t, map[string]string{"g.yaml": minimalYAML}, "g.yaml")
	var doc map[string]any
	if err := json.Unmarshal(base, &doc); err != nil {
		t.Fatal(err)
	}
	doc["seats"].([]any)[0].(map[string]any)["colour"] = "red"
	bad, _ := json.Marshal(doc)
	_, err := dsl.Parse(bad)
	wantErr(t, err, `unknown field "colour"`)

	_, err = dsl.Parse(append(append([]byte{}, base...), []byte(` {"id": "again"}`)...))
	wantErr(t, err, "after")
}

// TestToYAML_RoundTripsEveryShippedDefinition converts every definition the
// binary ships back to YAML and loads it again: nothing may change, key
// order included.
func TestToYAML_RoundTripsEveryShippedDefinition(t *testing.T) {
	roots := shippedDefinitions(t)
	if len(roots) < 20 {
		t.Fatalf("found only %d definitions", len(roots))
	}
	for _, path := range roots {
		root, err := os.OpenRoot(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		data, err := dsl.Load(root.FS(), filepath.Base(path))
		_ = root.Close()
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		y, err := dsl.ToYAML(data)
		if err != nil {
			t.Fatalf("%s: ToYAML: %v", path, err)
		}
		back := mustLoad(t, map[string]string{"g.yaml": string(y)}, "g.yaml")
		if !reflect.DeepEqual(tokenStream(t, data), tokenStream(t, back)) {
			t.Errorf("%s changed in a round trip through ToYAML", path)
		}
	}
}

// shippedDefinitions lists the top-level definition files of the games: the
// files that carry the schema comment (included parts do not).
func shippedDefinitions(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir("../internal/games", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".yaml" {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.HasPrefix(string(data), "# yaml-language-server: $schema=") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestShippedDefinitions_NameTheSchema checks the schema comment of every
// top-level definition points at the schema file.
func TestShippedDefinitions_NameTheSchema(t *testing.T) {
	for _, p := range shippedDefinitions(t) {
		data, _ := os.ReadFile(p)
		line, _, _ := strings.Cut(string(data), "\n")
		rel := strings.TrimPrefix(line, "# yaml-language-server: $schema=")
		if _, err := os.Stat(filepath.Join(filepath.Dir(p), rel)); err != nil {
			t.Errorf("%s: the schema comment points at %s, which does not exist", p, rel)
		}
	}
}

func TestToYAML_Style(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("long ", 20))
	y, err := dsl.ToYAML([]byte(`{"id":"x","rules":["It's a player's turn: move.","` + long + `"],"seats":[1,2],"text":"a\nb"}`))
	if err != nil {
		t.Fatal(err)
	}
	want := "id: x\nrules:\n  - \"It's a player's turn: move.\"\n  - " + strings.TrimSpace(long) + "\nseats: [1, 2]\ntext: |-\n  a\n  b\n"
	if string(y) != want {
		t.Fatalf("got\n%s\nwant\n%s", y, want)
	}
}

// TestSchema_UpToDate fails when the Go types changed and
// docs/game-definition.schema.json was not regenerated.
func TestSchema_UpToDate(t *testing.T) {
	want, err := schemagen.Generate("../internal/engine/dsl")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../docs/game-definition.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("docs/game-definition.schema.json is out of date: run go run ./scripts/genschema")
	}
	var s map[string]any
	if err := json.Unmarshal(got, &s); err != nil {
		t.Fatal(err)
	}
	defs := s["$defs"].(map[string]any)
	gd := defs["GameDefinition"].(map[string]any)
	if gd["additionalProperties"] != false {
		t.Fatal("the schema lets unknown fields through")
	}
	if _, ok := gd["properties"].(map[string]any)["win_condition"]; !ok {
		t.Fatal("the schema lacks win_condition")
	}
}
