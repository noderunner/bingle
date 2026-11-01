package dsl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path"
	"reflect"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Limits on what Load reads and produces. A definition is authored by hand,
// but a data pack is untrusted: these keep an include chain or an alias bomb
// from exhausting the server.
const (
	// MaxIncludeDepth is how deeply !include may nest.
	MaxIncludeDepth = 16
	// MaxSourceBytes caps the total size of the files one definition reads.
	MaxSourceBytes = 16 << 20
	// MaxOutputBytes caps the JSON a definition expands to, aliases included.
	MaxOutputBytes = 64 << 20
	// maxNesting caps how deeply nodes (and aliases of them) may nest.
	maxNesting = 256
)

// IncludeTag is the YAML tag that replaces a node with the contents of
// another file: `board: !include board.yaml`.
const IncludeTag = "!include"

// Load reads the game definition at name in fsys and returns it as JSON,
// the form the rest of the engine reads.
//
// A ".json" file is returned unchanged. A ".yaml" or ".yml" file is
// converted: its !include tags are replaced by the files they name (relative
// to the including file, never outside fsys, never a symbolic link), anchors,
// aliases and merge keys are expanded, and the keys keep the order they were
// written in.
//
// Either way every field name is checked against GameDefinition, including
// the fields of each variant's patch, and an unknown one is an error that
// names the file and line it is on.
func Load(fsys fs.FS, name string) ([]byte, error) {
	l := &loader{fsys: fsys, files: map[*yaml.Node]string{}}
	root, err := l.parse(name)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := l.emit(&buf, root, reflect.TypeFor[GameDefinition](), 0); err != nil {
		return nil, err
	}
	if isJSON(name) {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("dsl: %w", err)
		}
		return data, nil
	}
	return buf.Bytes(), nil
}

// MustLoad is Load for a definition embedded in the binary, where an error
// is a build mistake: it panics.
func MustLoad(fsys fs.FS, name string) []byte {
	data, err := Load(fsys, name)
	if err != nil {
		panic(err)
	}
	return data
}

func isJSON(name string) bool { return strings.EqualFold(path.Ext(name), ".json") }

type loader struct {
	fsys  fs.FS
	files map[*yaml.Node]string // the file every node was read from
	stack []string              // the files being read, outermost first
	read  int64                 // bytes read so far
}

// errorf is an error at n's place in the definition's source.
func (l *loader) errorf(n *yaml.Node, format string, args ...any) error {
	return fmt.Errorf("dsl: %s:%d:%d: %s", l.files[n], n.Line, n.Column, fmt.Sprintf(format, args...))
}

// parse reads one file, marks its nodes as its own and resolves its includes.
func (l *loader) parse(name string) (*yaml.Node, error) {
	for _, f := range l.stack {
		if f == name {
			return nil, fmt.Errorf("dsl: %s includes itself (%s)", name, strings.Join(append(l.stack, name), " -> "))
		}
	}
	if len(l.stack) >= MaxIncludeDepth {
		return nil, fmt.Errorf("dsl: %s: includes nest more than %d deep", name, MaxIncludeDepth)
	}
	info, err := fs.Lstat(l.fsys, name)
	if err != nil {
		return nil, fmt.Errorf("dsl: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("dsl: %s is not a regular file", name)
	}
	if l.read += info.Size(); l.read > MaxSourceBytes {
		return nil, fmt.Errorf("dsl: %s: the definition's files are larger than %d bytes", name, MaxSourceBytes)
	}
	data, err := fs.ReadFile(l.fsys, name)
	if err != nil {
		return nil, fmt.Errorf("dsl: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("dsl: %s: %w", name, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, fmt.Errorf("dsl: %s is empty", name)
	}
	root := doc.Content[0]
	l.mark(root, name)
	l.stack = append(l.stack, name)
	defer func() { l.stack = l.stack[:len(l.stack)-1] }()
	if err := l.resolve(root, name, 0); err != nil {
		return nil, err
	}
	return root, nil
}

// mark records that n and everything under it came from file.
func (l *loader) mark(n *yaml.Node, file string) {
	if _, done := l.files[n]; done {
		return
	}
	l.files[n] = file
	for _, c := range n.Content {
		l.mark(c, file)
	}
}

// resolve replaces every !include under n with the file it names.
func (l *loader) resolve(n *yaml.Node, file string, depth int) error {
	if depth > maxNesting {
		return l.errorf(n, "nested more than %d deep", maxNesting)
	}
	if n.Tag == IncludeTag {
		if n.Kind != yaml.ScalarNode {
			return l.errorf(n, "%s takes a file name", IncludeTag)
		}
		if isJSON(file) {
			return l.errorf(n, "a JSON definition cannot include other files")
		}
		target, err := includePath(file, n.Value)
		if err != nil {
			return l.errorf(n, "%v", err)
		}
		inc, err := l.parse(target)
		if err != nil {
			return err
		}
		*n = *inc
		l.files[n] = target
		return nil
	}
	for _, c := range n.Content {
		if err := l.resolve(c, file, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// includePath resolves an include's file name against the including file.
func includePath(from, name string) (string, error) {
	if name == "" {
		return "", errors.New("an empty file name")
	}
	if path.IsAbs(name) || strings.Contains(name, `\`) {
		return "", fmt.Errorf("%q: a file name is relative, with forward slashes", name)
	}
	p := path.Join(path.Dir(from), name)
	if !fs.ValidPath(p) {
		return "", fmt.Errorf("%q is outside the game's own files", name)
	}
	return p, nil
}

// emit writes n as JSON, checking its field names against t (nil: anything
// goes, as in params).
func (l *loader) emit(buf *bytes.Buffer, n *yaml.Node, t reflect.Type, depth int) error {
	if depth > maxNesting {
		return l.errorf(n, "nested more than %d deep", maxNesting)
	}
	if buf.Len() > MaxOutputBytes {
		return l.errorf(n, "the definition expands to more than %d bytes", MaxOutputBytes)
	}
	t = checkedType(t)
	switch n.Kind {
	case yaml.DocumentNode:
		return l.emit(buf, n.Content[0], t, depth+1)
	case yaml.AliasNode:
		return l.emit(buf, n.Alias, t, depth+1)
	case yaml.ScalarNode:
		return l.emitScalar(buf, n)
	case yaml.SequenceNode:
		var elem reflect.Type
		if t != nil && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array) {
			elem = t.Elem()
		}
		buf.WriteByte('[')
		for i, c := range n.Content {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := l.emit(buf, c, elem, depth+1); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	case yaml.MappingNode:
		return l.emitMapping(buf, n, t, depth)
	}
	return l.errorf(n, "an unexpected YAML node")
}

// emitScalar writes a plain value. The common cases are written directly;
// anything else (0x1F, 1_000, .inf, !!binary) goes through yaml.v3's own
// decoding, which is much slower.
func (l *loader) emitScalar(buf *bytes.Buffer, n *yaml.Node) error {
	var v any
	switch n.ShortTag() {
	case "!!str", "!!timestamp": // a date stays the text it was written as, not a time
		v = n.Value
	case "!!null":
		buf.WriteString("null")
		return nil
	case "!!bool":
		switch n.Value {
		case "true", "True", "TRUE":
			buf.WriteString("true")
			return nil
		case "false", "False", "FALSE":
			buf.WriteString("false")
			return nil
		}
	case "!!int":
		if i, err := strconv.ParseInt(n.Value, 10, 64); err == nil {
			buf.WriteString(strconv.FormatInt(i, 10))
			return nil
		}
	case "!!float":
		if f, err := strconv.ParseFloat(n.Value, 64); err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) {
			v = f
		}
	}
	if v == nil {
		if err := n.Decode(&v); err != nil {
			return l.errorf(n, "%v", err)
		}
	}
	data, err := json.Marshal(v)
	if err != nil {
		return l.errorf(n, "%q is not a JSON value", n.Value)
	}
	buf.Write(data)
	return nil
}

// pair is one key of a mapping and its value.
type pair struct {
	key   *yaml.Node
	value *yaml.Node
}

// emitMapping writes a mapping as a JSON object: keys in the order written,
// merge keys (<<) expanded in place, an explicit key winning over a merged
// one and an earlier merged key over a later one.
func (l *loader) emitMapping(buf *bytes.Buffer, n *yaml.Node, t reflect.Type, depth int) error {
	explicit := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		if isMerge(k) {
			continue
		}
		if k.Kind != yaml.ScalarNode {
			return l.errorf(k, "a key must be a plain value")
		}
		if explicit[k.Value] {
			return l.errorf(k, "%q appears twice", k.Value)
		}
		explicit[k.Value] = true
	}
	var pairs []pair
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if !isMerge(k) {
			pairs = append(pairs, pair{k, v})
			seen[k.Value] = true
			continue
		}
		merged, err := l.mergeSources(v, depth)
		if err != nil {
			return err
		}
		for _, p := range merged {
			if explicit[p.key.Value] || seen[p.key.Value] {
				continue
			}
			pairs = append(pairs, p)
			seen[p.key.Value] = true
		}
	}

	fields := jsonFields(t)
	buf.WriteByte('{')
	for i, p := range pairs {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, _ := json.Marshal(p.key.Value)
		buf.Write(key)
		buf.WriteByte(':')
		var vt reflect.Type
		switch {
		case t == nil:
		case t.Kind() == reflect.Map:
			vt = t.Elem()
		case t.Kind() == reflect.Struct:
			f, ok := fields[p.key.Value]
			if !ok {
				return l.errorf(p.key, "unknown field %q in %s", p.key.Value, describe(t))
			}
			vt = f
		}
		if p.value.Kind == yaml.ScalarNode && p.value.ShortTag() == "!!null" {
			vt = nil // a null removes a field in a merge patch
		}
		if err := l.emit(buf, p.value, vt, depth+1); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

func isMerge(k *yaml.Node) bool {
	return k.Kind == yaml.ScalarNode && k.Value == "<<" && k.ShortTag() == "!!merge"
}

// mergeSources returns the pairs a merge key brings in: one mapping, or a
// list of them, each possibly an alias.
func (l *loader) mergeSources(v *yaml.Node, depth int) ([]pair, error) {
	if depth > maxNesting {
		return nil, l.errorf(v, "nested more than %d deep", maxNesting)
	}
	for v.Kind == yaml.AliasNode {
		v = v.Alias
	}
	switch v.Kind {
	case yaml.MappingNode:
		var out []pair
		for i := 0; i+1 < len(v.Content); i += 2 {
			k := v.Content[i]
			if isMerge(k) {
				inner, err := l.mergeSources(v.Content[i+1], depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, inner...)
				continue
			}
			out = append(out, pair{k, v.Content[i+1]})
		}
		return out, nil
	case yaml.SequenceNode:
		var out []pair
		for _, c := range v.Content {
			inner, err := l.mergeSources(c, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, inner...)
		}
		return out, nil
	}
	return nil, l.errorf(v, "a merge key (<<) takes a mapping or a list of them")
}

var (
	unmarshalerType = reflect.TypeFor[json.Unmarshaler]()
	rawMessageType  = reflect.TypeFor[json.RawMessage]()
	variantType     = reflect.TypeFor[VariantDef]()
	definitionType  = reflect.TypeFor[GameDefinition]()
)

// checkedType is the type whose field names a node is checked against: t
// without pointers, or nil when the node is free-form (an interface, a type
// that decodes itself, raw JSON).
func checkedType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() == reflect.Interface || t == rawMessageType ||
		t.Implements(unmarshalerType) || reflect.PointerTo(t).Implements(unmarshalerType) {
		return nil
	}
	return t
}

// jsonFields maps the JSON names of a struct's fields to their types, with
// embedded structs' fields promoted as encoding/json promotes them. A
// variant's patch is checked as a (partial) game definition.
func jsonFields(t reflect.Type) map[string]reflect.Type {
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	out := map[string]reflect.Type{}
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
				for k, v := range jsonFields(ft) {
					if _, set := out[k]; !set {
						out[k] = v
					}
				}
				continue
			}
		}
		if name == "" {
			name = f.Name
		}
		ft := f.Type
		if t == variantType && name == "patch" {
			ft = definitionType
		}
		out[name] = ft
	}
	return out
}

// describe names a definition type for an error message.
func describe(t reflect.Type) string {
	if t == definitionType {
		return "the game definition"
	}
	return t.Name()
}
