package main

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
)

// A measure is what one file holds, line by line. A comment line holds a
// comment and no code; a doc line is a comment line of the package's doc, of
// the doc comment of an exported top-level name or of a struct's exported field
// (in JavaScript, a /** */ block).
type measure struct {
	lines, code, comment, doc int
	funcs                     []fn
	unbalanced                bool // JavaScript only: the lexer lost its place
}

// A fn is one function: in Go a declaration (its closures are part of it), in
// JavaScript any named function, method or arrow function, nested or not.
type fn struct {
	name  string
	line  int
	lines int
	depth int // JavaScript only: functions around it
}

func measureGo(path string) (measure, *ast.File, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return measure{}, nil, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return measure{}, nil, err
	}
	code, comment := goLines(src)
	m := measure{lines: lineCount(src)}
	doc := map[int]bool{}
	for _, g := range exportedDocs(f) {
		for l := fset.Position(g.Pos()).Line; l <= fset.Position(g.End()).Line; l++ {
			doc[l] = true
		}
	}
	for l := range comment {
		if code[l] {
			continue
		}
		m.comment++
		if doc[l] {
			m.doc++
		}
	}
	m.code = len(code)
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			start, end := fset.Position(fd.Pos()).Line, fset.Position(fd.End()).Line
			m.funcs = append(m.funcs, fn{name: funcName(fd), line: start, lines: end - start + 1})
		}
	}
	return m, f, nil
}

// goLines returns the lines that hold code and the lines that hold a comment.
func goLines(src []byte) (code, comment map[int]bool) {
	code, comment = map[int]bool{}, map[int]bool{}
	fset := token.NewFileSet()
	tf := fset.AddFile("", fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(tf, src, nil, scanner.ScanComments)
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			return code, comment
		}
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}
		start := fset.Position(pos).Line
		end := fset.Position(pos + token.Pos(max(len(lit), 1)) - 1).Line
		into := code
		if tok == token.COMMENT {
			into = comment
		}
		for l := start; l <= end; l++ {
			into[l] = true
		}
	}
}

// exportedDocs is the package doc and the doc comments of exported top-level
// names (an exported method counts; a method of an unexported type does not).
func exportedDocs(f *ast.File) []*ast.CommentGroup {
	var gs []*ast.CommentGroup
	add := func(g *ast.CommentGroup, exported bool) {
		if g != nil && exported {
			gs = append(gs, g)
		}
	}
	add(f.Doc, true)
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			add(d.Doc, d.Name.IsExported() && (d.Recv == nil || ast.IsExported(recvName(d))))
		case *ast.GenDecl:
			some := false
			for _, s := range d.Specs {
				e := specExported(s)
				some = some || e
				switch s := s.(type) {
				case *ast.TypeSpec:
					add(s.Doc, e)
					gs = append(gs, fieldDocs(s)...)
				case *ast.ValueSpec:
					add(s.Doc, e)
				}
			}
			add(d.Doc, some)
		}
	}
	return gs
}

// fieldDocs is the doc comments of a struct's exported fields, which
// schemagen turns into the descriptions of the JSON schemas (of unexported
// request types too).
func fieldDocs(s *ast.TypeSpec) []*ast.CommentGroup {
	st, ok := s.Type.(*ast.StructType)
	if !ok {
		return nil
	}
	var gs []*ast.CommentGroup
	for _, f := range st.Fields.List {
		if f.Doc != nil && (len(f.Names) == 0 || f.Names[0].IsExported()) {
			gs = append(gs, f.Doc)
		}
	}
	return gs
}

func specExported(s ast.Spec) bool {
	switch s := s.(type) {
	case *ast.TypeSpec:
		return s.Name.IsExported()
	case *ast.ValueSpec:
		for _, n := range s.Names {
			if n.IsExported() {
				return true
			}
		}
	}
	return false
}

func recvName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	t := fd.Recv.List[0].Type
	for {
		switch x := t.(type) {
		case *ast.StarExpr:
			t = x.X
		case *ast.IndexExpr:
			t = x.X
		case *ast.IndexListExpr:
			t = x.X
		case *ast.Ident:
			return x.Name
		default:
			return "?"
		}
	}
}

func funcName(fd *ast.FuncDecl) string {
	if r := recvName(fd); r != "" {
		return r + "." + fd.Name.Name
	}
	return fd.Name.Name
}

func lineCount(src []byte) int {
	n := 0
	for _, b := range src {
		if b == '\n' {
			n++
		}
	}
	if len(src) > 0 && src[len(src)-1] != '\n' {
		n++
	}
	return n
}
