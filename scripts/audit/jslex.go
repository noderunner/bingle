package main

import (
	"strings"
	"unicode"
)

// A tok is one JavaScript token. The lexer knows comments, strings, template
// literals (with ${} inside them lexed as code), numbers, names and
// punctuation. A slash starts a regular expression when the token before it
// cannot end an expression; that guess is the lexer's one known weakness.
type tok struct {
	kind      byte // 'c' comment, 's' string, 'n' name or number, 'p' punctuation, 'r' regexp
	text      string
	line, end int
}

type lexer struct {
	src   string
	i     int
	line  int
	toks  []tok
	depth []int // brace depth at each open ${
	brace int
}

func lexJS(src string) []tok {
	lx := &lexer{src: src, line: 1}
	for lx.i < len(src) {
		lx.next()
	}
	return lx.toks
}

func (lx *lexer) emit(kind byte, start, startLine int) {
	lx.toks = append(lx.toks, tok{kind: kind, text: lx.src[start:lx.i], line: startLine, end: lx.line})
}

func (lx *lexer) advance() {
	if lx.src[lx.i] == '\n' {
		lx.line++
	}
	lx.i++
}

func (lx *lexer) next() {
	c := lx.src[lx.i]
	start, line := lx.i, lx.line
	switch {
	case c == '\n' || c == ' ' || c == '\t' || c == '\r':
		lx.advance()
	case strings.HasPrefix(lx.src[lx.i:], "//"):
		for lx.i < len(lx.src) && lx.src[lx.i] != '\n' {
			lx.i++
		}
		lx.emit('c', start, line)
	case strings.HasPrefix(lx.src[lx.i:], "/*"):
		lx.i += 2
		for lx.i < len(lx.src) && !strings.HasPrefix(lx.src[lx.i:], "*/") {
			lx.advance()
		}
		lx.i = min(lx.i+2, len(lx.src))
		lx.emit('c', start, line)
	case c == '"' || c == '\'':
		lx.quoted(c)
		lx.emit('s', start, line)
	case c == '`':
		lx.i++
		lx.template(start, line)
	case c == '}' && len(lx.depth) > 0 && lx.depth[len(lx.depth)-1] == lx.brace:
		lx.depth = lx.depth[:len(lx.depth)-1]
		lx.i++
		lx.template(start, line)
	case c == '/' && lx.regexAllowed():
		lx.regexp()
		lx.emit('r', start, line)
	case isNameByte(c):
		for lx.i < len(lx.src) && isNameByte(lx.src[lx.i]) {
			lx.i++
		}
		lx.emit('n', start, line)
	default:
		lx.punct(c)
		lx.emit('p', start, line)
	}
}

func (lx *lexer) punct(c byte) {
	for _, op := range []string{"=>", "...", "?."} {
		if strings.HasPrefix(lx.src[lx.i:], op) {
			lx.i += len(op)
			return
		}
	}
	switch c {
	case '{':
		lx.brace++
	case '}':
		lx.brace--
	}
	lx.i++
}

func (lx *lexer) quoted(q byte) {
	lx.i++
	for lx.i < len(lx.src) && lx.src[lx.i] != q && lx.src[lx.i] != '\n' {
		if lx.src[lx.i] == '\\' {
			lx.i++
		}
		lx.i++
	}
	lx.i = min(lx.i+1, len(lx.src))
}

// template reads a template literal's text up to its end or its next ${.
func (lx *lexer) template(start, line int) {
	for lx.i < len(lx.src) {
		switch {
		case lx.src[lx.i] == '\\':
			lx.i++
			lx.advance()
		case lx.src[lx.i] == '`':
			lx.i++
			lx.emit('s', start, line)
			return
		case strings.HasPrefix(lx.src[lx.i:], "${"):
			lx.i += 2
			lx.emit('s', start, line)
			lx.depth = append(lx.depth, lx.brace)
			return
		default:
			lx.advance()
		}
	}
	lx.emit('s', start, line)
}

func (lx *lexer) regexp() {
	lx.i++
	inClass := false
	for lx.i < len(lx.src) && lx.src[lx.i] != '\n' {
		switch c := lx.src[lx.i]; {
		case c == '\\':
			lx.i++
		case c == '[':
			inClass = true
		case c == ']':
			inClass = false
		case c == '/' && !inClass:
			lx.i++
			for lx.i < len(lx.src) && isNameByte(lx.src[lx.i]) {
				lx.i++
			}
			return
		}
		lx.i++
	}
}

var beforeRegexp = map[string]bool{
	"return": true, "typeof": true, "case": true, "in": true, "of": true, "new": true,
	"delete": true, "void": true, "throw": true, "else": true, "do": true, "await": true, "yield": true,
}

func (lx *lexer) regexAllowed() bool {
	for j := len(lx.toks) - 1; j >= 0; j-- {
		t := lx.toks[j]
		switch t.kind {
		case 'c':
			continue
		case 'n':
			return beforeRegexp[t.text]
		case 'p':
			return t.text != ")" && t.text != "]" && t.text != "}"
		default:
			return false
		}
	}
	return true
}

func isNameByte(c byte) bool {
	return c == '_' || c == '$' || c >= 0x80 || unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c))
}
