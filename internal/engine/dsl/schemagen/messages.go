package schemagen

import (
	"path/filepath"
	"reflect"
	"strings"
)

// module is the import path of this repository's Go module, which Messages
// strips from a type's package path to find its source.
const module = "github.com/noderunner/bingle/"

// Messages describes the API's messages (ADR 0134): request bodies,
// responses and WebSocket events, from the Go types the server sends and
// reads, wherever in the repository they are declared. Unlike a game
// definition, a message's object lets unknown fields through, since a newer
// server may add one and a client must ignore it, and the fields a response
// always carries are required (null allowed where Go may send it).
type Messages struct {
	g    *gen
	docs map[string]map[string]typeDocs // package path -> type name -> docs
	root string
	err  error
}

// MessageOptions are what a description of messages needs to know beyond
// the Go types.
type MessageOptions struct {
	// Special gives the schemas of types with a JSON form of their own (a
	// custom MarshalJSON, a number or a list).
	Special map[reflect.Type]map[string]any
	// Required lists the fields a request body must have, which its Go type
	// cannot say.
	Required map[reflect.Type][]string
	// Names renames the definitions of types whose Go name says too little
	// outside their package.
	Names map[reflect.Type]string
}

// NewMessages starts a description of messages, reading doc comments from
// the repository at root.
func NewMessages(root string, o MessageOptions) *Messages {
	m := &Messages{docs: map[string]map[string]typeDocs{}, root: root}
	m.g = newGen(m.docsOf)
	m.g.open = true
	m.g.special = o.Special
	m.g.required = o.Required
	m.g.rename = o.Names
	return m
}

// docsOf reads the doc comments of t's package the first time one is asked for.
func (m *Messages) docsOf(t reflect.Type) typeDocs {
	pkg := t.PkgPath()
	if !strings.HasPrefix(pkg, module) {
		return typeDocs{}
	}
	docs, ok := m.docs[pkg]
	if !ok {
		var err error
		docs, err = readDocs(filepath.Join(m.root, filepath.FromSlash(strings.TrimPrefix(pkg, module))))
		if err != nil && m.err == nil {
			m.err = err
		}
		m.docs[pkg] = docs
	}
	return docs[t.Name()]
}

// Response is the schema of a message the server sends: a $ref to its
// definition for a struct.
func (m *Messages) Response(t reflect.Type) map[string]any {
	m.g.request = false
	return m.g.schema(t)
}

// Request is the schema of a request body.
func (m *Messages) Request(t reflect.Type) map[string]any {
	m.g.request = true
	defer func() { m.g.request = false }()
	return m.g.schema(t)
}

// Defs are the definitions written so far, and the first error met reading
// doc comments.
func (m *Messages) Defs() (map[string]any, error) {
	return m.g.defs, m.err
}
