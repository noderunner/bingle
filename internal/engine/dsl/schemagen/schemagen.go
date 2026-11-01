// Package schemagen writes the JSON Schema of a game definition from the Go
// types of package dsl, with each field's doc comment as its description, so
// an editor can complete and check a definition as it is typed (in YAML too,
// through a `# yaml-language-server: $schema=...` comment).
//
// The schema is generated, never edited: `go run ./scripts/genschema`
// rewrites docs/game-definition.schema.json, and a test fails when the file
// is out of date.
package schemagen

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/noderunner/bingle/internal/engine/dsl"
)

// ID is the schema's $id.
const ID = "https://github.com/noderunner/bingle/docs/game-definition.schema.json"

// required are the top-level fields every complete definition has (a
// variant's patch is checked against the same types without them).
var required = []string{"id", "name", "board", "seats", "pieces", "turn_order", "win_condition"}

// Generate returns the schema, reading doc comments from the Go source of
// package dsl in dslDir.
func Generate(dslDir string) ([]byte, error) {
	docs, err := readDocs(dslDir)
	if err != nil {
		return nil, err
	}
	g := &gen{docs: docs, defs: map[string]any{}}
	root := g.schema(reflect.TypeFor[dsl.GameDefinition]())
	out := map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"$id":         ID,
		"title":       "Bingle game definition",
		"description": "A game definition of the bingle engine, in YAML or JSON. Generated from internal/engine/dsl: do not edit.",
		"$ref":        root["$ref"],
		"required":    required,
		"$defs":       g.defs,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// typeDocs holds the doc comments of one struct type and of its fields.
type typeDocs struct {
	doc    string
	fields map[string]string
}

// readDocs collects the doc comments of every struct type in dir.
func readDocs(dir string) (map[string]typeDocs, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	out := map[string]typeDocs{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		file, err := parser.ParseFile(fset, f, src, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		for _, d := range file.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				td := typeDocs{doc: text(ts.Doc, gd.Doc), fields: map[string]string{}}
				if st, ok := ts.Type.(*ast.StructType); ok {
					for _, fl := range st.Fields.List {
						for _, n := range fl.Names {
							td.fields[n.Name] = text(fl.Doc, fl.Comment)
						}
					}
				}
				out[ts.Name.Name] = td
			}
		}
	}
	return out, nil
}

// text is the first non-empty comment group, flattened to one line.
func text(groups ...*ast.CommentGroup) string {
	for _, g := range groups {
		if g == nil {
			continue
		}
		if s := strings.Join(strings.Fields(g.Text()), " "); s != "" {
			return s
		}
	}
	return ""
}

type gen struct {
	docs map[string]typeDocs
	defs map[string]any
}

var (
	rawMessage  = reflect.TypeFor[json.RawMessage]()
	quantity    = reflect.TypeFor[dsl.Quantity]()
	variantType = reflect.TypeFor[dsl.VariantDef]()
)

// schema is the schema of a value of type t.
func (g *gen) schema(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t {
	case quantity:
		return map[string]any{"type": []string{"integer", "string"}}
	case rawMessage:
		return map[string]any{}
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": g.schema(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": g.schema(t.Elem())}
	case reflect.Struct:
		return g.object(t)
	}
	panic(fmt.Sprintf("schemagen: no schema for %s", t))
}

// object is a reference to the definition of struct type t, written once.
func (g *gen) object(t reflect.Type) map[string]any {
	ref := map[string]any{"$ref": "#/$defs/" + t.Name()}
	if _, done := g.defs[t.Name()]; done {
		return ref
	}
	def := map[string]any{"type": "object", "additionalProperties": false}
	g.defs[t.Name()] = def // before the fields, for types that contain themselves
	if d := g.docs[t.Name()].doc; d != "" {
		def["description"] = d
	}
	props := map[string]any{}
	g.fields(t, props)
	def["properties"] = props
	return ref
}

// fields adds the properties of struct t, promoting embedded structs' fields.
func (g *gen) fields(t reflect.Type, props map[string]any) {
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" || (!f.IsExported() && !f.Anonymous) {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				g.fields(ft, props)
				continue
			}
		}
		if name == "" {
			name = f.Name
		}
		var s map[string]any
		if t == variantType && name == "patch" {
			s = map[string]any{"$ref": "#/$defs/GameDefinition"}
		} else {
			s = g.schema(f.Type)
		}
		if d := g.docs[t.Name()].fields[f.Name]; d != "" {
			if _, isRef := s["$ref"]; isRef {
				s = map[string]any{"allOf": []any{s}, "description": d}
			} else {
				s["description"] = d
			}
		}
		props[name] = s
	}
}
