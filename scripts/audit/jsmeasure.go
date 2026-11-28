package main

import (
	"os"
	"strings"
)

var notFunc = map[string]bool{"if": true, "for": true, "while": true, "switch": true, "catch": true, "with": true}

func measureJS(path string) (measure, []tok, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return measure{}, nil, err
	}
	toks := lexJS(string(src))
	m := measure{lines: lineCount(src)}
	code, comment, doc := map[int]bool{}, map[int]bool{}, map[int]bool{}
	for _, t := range toks {
		into := code
		if t.kind == 'c' {
			into = comment
		}
		for l := t.line; l <= t.end; l++ {
			into[l] = true
			if t.kind == 'c' && strings.HasPrefix(t.text, "/**") {
				doc[l] = true
			}
		}
	}
	for l := range comment {
		if !code[l] {
			m.comment++
			if doc[l] {
				m.doc++
			}
		}
	}
	m.code = len(code)
	m.funcs, m.unbalanced = jsFuncs(toks)
	return m, toks, nil
}

// jsFuncs finds each { that opens a function's body and its matching }, and
// says whether the brackets failed to balance (a sign the lexer went wrong).
func jsFuncs(all []tok) ([]fn, bool) {
	var toks []tok
	for _, t := range all {
		if t.kind != 'c' {
			toks = append(toks, t)
		}
	}
	open := map[int]int{} // index of a ) or } to the index of its ( or {
	var stack []int
	for i, t := range toks {
		if t.kind != 'p' {
			continue
		}
		switch t.text {
		case "(", "{", "[":
			stack = append(stack, i)
		case ")", "}", "]":
			if len(stack) == 0 {
				return nil, true
			}
			open[i] = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		}
	}
	unbalanced := len(stack) > 0
	type frame struct {
		name string
		line int
	}
	var fns []fn
	var braces []*frame
	depth := 0
	for i, t := range toks {
		if t.kind != 'p' {
			continue
		}
		switch t.text {
		case "{":
			var f *frame
			if name, line, ok := funcAt(toks, open, i); ok {
				f = &frame{name, line}
			}
			braces = append(braces, f)
			if f != nil {
				depth++
			}
		case "}":
			if len(braces) == 0 {
				continue
			}
			f := braces[len(braces)-1]
			braces = braces[:len(braces)-1]
			if f != nil {
				depth--
				fns = append(fns, fn{name: f.name, line: f.line, lines: t.line - f.line + 1, depth: depth})
			}
		}
	}
	return fns, unbalanced
}

// funcAt says whether the { at i opens a function, with its name and the line
// its head starts on.
func funcAt(toks []tok, open map[int]int, i int) (string, int, bool) {
	if i == 0 {
		return "", 0, false
	}
	prev := toks[i-1]
	if prev.text == "=>" {
		head := i - 2
		if head >= 0 && toks[head].text == ")" {
			head = open[head]
		}
		return nameBefore(toks, head), toks[max(head, 0)].line, true
	}
	if prev.text != ")" {
		return "", 0, false
	}
	p := open[i-1]
	if p == 0 {
		return "", 0, false
	}
	before := toks[p-1]
	switch {
	case before.text == "function":
		return nameBefore(toks, p-1), before.line, true
	case before.kind == 'n' && notFunc[before.text]:
		return "", 0, false
	case before.kind == 'n' && p >= 2 && (toks[p-2].text == "function" || toks[p-2].text == "*"):
		return before.text, toks[p-2].line, true
	case before.kind == 'n':
		return before.text, before.line, true // a method
	}
	return "", 0, false
}

// nameBefore names an anonymous function from what it is assigned to:
// name = …, name: … or async in between.
func nameBefore(toks []tok, i int) string {
	j := i - 1
	if j >= 0 && toks[j].text == "async" {
		j--
	}
	if j >= 1 && (toks[j].text == "=" || toks[j].text == ":") && toks[j-1].kind == 'n' {
		return toks[j-1].text
	}
	return "(anonymous)"
}
