package dsl

import (
	"bytes"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// flowWidth is how long a list or mapping of plain values may be and still
// be written on one line, `[a1, b2]` or `{x: 1, y: 2}`.
const flowWidth = 80

// ToYAML rewrites a JSON game definition as YAML, keeping the order of its
// keys: the starting point for authoring an existing definition by hand.
// Short lists and mappings of plain values are written on one line, text
// with line breaks as a literal block, everything else in block style.
func ToYAML(jsonDoc []byte) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(jsonDoc, &doc); err != nil {
		return nil, fmt.Errorf("dsl: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, fmt.Errorf("dsl: the document is empty")
	}
	restyle(doc.Content[0])
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc.Content[0]); err != nil {
		return nil, fmt.Errorf("dsl: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("dsl: %w", err)
	}
	return buf.Bytes(), nil
}

// restyle drops the JSON look of n (quotes, braces) and picks a YAML one.
func restyle(n *yaml.Node) {
	n.Style = 0
	for _, c := range n.Content {
		restyle(c)
	}
	switch n.Kind {
	case yaml.ScalarNode:
		switch {
		case n.ShortTag() != "!!str":
		case strings.Contains(n.Value, "\n"):
			n.Style = yaml.LiteralStyle
		case strings.Contains(n.Value, "'") && !strings.Contains(n.Value, `"`) && needsQuotes(n.Value):
			// "a player's turn" reads better than 'a player''s turn'.
			n.Style = yaml.DoubleQuotedStyle
		}
	case yaml.SequenceNode, yaml.MappingNode:
		if w, ok := inlineWidth(n); ok && w <= flowWidth && len(n.Content) > 0 {
			n.Style = yaml.FlowStyle
		}
	}
}

// inlineWidth estimates the width of n written on one line, and whether it
// can be: only plain values without line breaks can.
func inlineWidth(n *yaml.Node) (int, bool) {
	w := 2
	for _, c := range n.Content {
		if c.Kind != yaml.ScalarNode || strings.Contains(c.Value, "\n") {
			return 0, false
		}
		w += len(c.Value) + 2
		if c.ShortTag() == "!!str" && needsQuotes(c.Value) {
			w += 2
		}
	}
	return w, true
}

// needsQuotes is a cautious guess at whether a string is quoted in YAML; it
// only affects whether a line is judged short enough.
func needsQuotes(s string) bool {
	return s == "" || strings.TrimSpace(s) != s || strings.ContainsAny(s[:1], "-?:,[]{}#&*!|>'\"%@`") ||
		strings.Contains(s, ": ") || strings.Contains(s, " #") || strings.HasSuffix(s, ":")
}
