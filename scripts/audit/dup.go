package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

const (
	cloneLines   = 6 // a run of at least this many lines, in two files, is a clone
	minFuncLines = 8 // functions shorter than this are not compared
)

// dup: functions of the same shape in two places, and runs of lines repeated
// across files, for the areas the audit asks about.
func (a *audit) dup() error {
	if err := a.funcShapes(); err != nil {
		return err
	}
	areas := []struct{ name, pathspec, filter string }{
		{"games (Go)", "internal/games/*.go", ""},
		{"api and mcpserver", "internal/api/*.go internal/mcpserver/*.go", ""},
		{"definitions (YAML)", "internal/games/*.yaml tests/testdata/*.yaml", ""},
		{"web client (JavaScript)", "internal/web/static/js/*.js", ""},
		{"browser tests", "tests/app/*_test.go", ""},
	}
	for _, ar := range areas {
		paths, err := gitFiles(strings.Fields(ar.pathspec))
		if err != nil {
			return err
		}
		pairs, err := clones(paths)
		if err != nil {
			return err
		}
		head := []string{"file", "other file", "lines of file in clones", "longest run", "it starts at"}
		fmt.Printf("\nRepeated runs of %d or more lines, %s: %d pairs of files, %d lines in all\n", cloneLines, ar.name, len(pairs), sumCol(pairs, 2))
		table(head, pairs, a.top)
		name := strings.NewReplacer(" ", "-", "(", "", ")", "").Replace(ar.name)
		if err := a.save("clones-"+name+".tsv", head, pairs); err != nil {
			return err
		}
	}
	return nil
}

func gitFiles(specs []string) ([]string, error) {
	out, err := exec.Command("git", append([]string{"ls-files", "--"}, specs...)...).Output()
	if err != nil {
		return nil, err
	}
	var ps []string
	for _, p := range strings.Fields(string(out)) {
		if !strings.HasSuffix(p, "_test.go") || strings.Contains(specs[0], "_test") {
			ps = append(ps, p)
		}
	}
	return ps, nil
}

// importLine is a Go file's package clause or an import, which every file
// repeats.
var importLine = regexp.MustCompile(`^(package \w+|import \(|import "[^"]*"|(\w+ )?"[^"]*")$`)

type line struct {
	text string
	n    int // line number in the file
}

// significant is a file's lines without indentation, leaving out blank lines,
// comment-only lines and lines of only brackets.
func significant(p string) ([]line, error) {
	src, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var ls []line
	for i, l := range strings.Split(string(src), "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") || strings.Trim(t, "{}()[];,") == "" || importLine.MatchString(t) {
			continue
		}
		ls = append(ls, line{t, i + 1})
	}
	return ls, nil
}

// clones finds, for each pair of files, the lines of the first that lie in a
// run of cloneLines or more lines found in the second too.
func clones(paths []string) ([][]string, error) {
	type at struct{ file, i int }
	files := make([][]line, len(paths))
	windows := map[string][]at{}
	for f, p := range paths {
		ls, err := significant(p)
		if err != nil {
			return nil, err
		}
		files[f] = ls
		for i := 0; i+cloneLines <= len(ls); i++ {
			k := window(ls[i : i+cloneLines])
			windows[k] = append(windows[k], at{f, i})
		}
	}
	type pair struct{ a, b int }
	covered := map[pair]map[int]bool{}
	for _, ats := range windows {
		for _, x := range ats {
			for _, y := range ats {
				if x.file == y.file {
					continue
				}
				pr := pair{x.file, y.file}
				if covered[pr] == nil {
					covered[pr] = map[int]bool{}
				}
				for i := x.i; i < x.i+cloneLines; i++ {
					covered[pr][i] = true
				}
			}
		}
	}
	var rows [][]string
	for pr, lines := range covered {
		if pr.a > pr.b && paths[pr.a] != paths[pr.b] {
			continue // each pair once, from the first file's side
		}
		run, start := longestRun(lines)
		rows = append(rows, []string{paths[pr.a], paths[pr.b], itoa(len(lines)), itoa(run), fmt.Sprintf("%s:%d", paths[pr.a], files[pr.a][start].n)})
	}
	byInt(rows, 2)
	return rows, nil
}

func window(ls []line) string {
	var b strings.Builder
	for _, l := range ls {
		b.WriteString(l.text)
		b.WriteByte('\n')
	}
	return b.String()
}

func longestRun(set map[int]bool) (int, int) {
	idx := make([]int, 0, len(set))
	for i := range set {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	best, bestStart, run, start := 0, 0, 0, 0
	for k, i := range idx {
		if k == 0 || i != idx[k-1]+1 {
			run, start = 0, i
		}
		run++
		if run > best {
			best, bestStart = run, start
		}
	}
	return best, bestStart
}

func sumCol(rows [][]string, col int) int {
	n := 0
	for _, r := range rows {
		n += atoi(r[col])
	}
	return n
}

// funcShapes groups the Go functions (tests left out) whose bodies are the
// same token for token once names and literals are blanked out, and those
// that are identical outright.
func (a *audit) funcShapes() error {
	shapes, exact := map[string][]string{}, map[string][]string{}
	sizes := map[string]int{}
	for _, f := range a.only("go") {
		if f.test {
			continue
		}
		src, err := os.ReadFile(f.path)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, f.path, src, 0)
		if err != nil {
			return err
		}
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			lines := fset.Position(fd.End()).Line - fset.Position(fd.Pos()).Line + 1
			if lines < minFuncLines {
				continue
			}
			body := src[fset.Position(fd.Body.Pos()).Offset:fset.Position(fd.Body.End()).Offset]
			shape, text := tokens(body)
			where := fmt.Sprintf("%s:%d %s", f.path, fset.Position(fd.Pos()).Line, funcName(fd))
			shapes[shape] = append(shapes[shape], where)
			exact[text] = append(exact[text], where)
			sizes[shape], sizes[text] = lines, lines
		}
	}
	head := []string{"functions", "lines each", "where"}
	for _, g := range []struct {
		name string
		m    map[string][]string
	}{{"identical", exact}, {"same shape (names and literals aside)", shapes}} {
		var rows [][]string
		for k, ws := range g.m {
			if len(ws) > 1 {
				rows = append(rows, []string{itoa(len(ws)), itoa(sizes[k]), strings.Join(ws, "; ")})
			}
		}
		sort.SliceStable(rows, func(i, j int) bool {
			return atoi(rows[i][0])*atoi(rows[i][1]) > atoi(rows[j][0])*atoi(rows[j][1])
		})
		fmt.Printf("\nGo functions of %d lines or more, %s: %d groups\n", minFuncLines, g.name, len(rows))
		table(head, rows, a.top)
		if err := a.save("funcs-"+strings.Fields(g.name)[0]+".tsv", head, rows); err != nil {
			return err
		}
	}
	return nil
}

// tokens is a body's tokens with names and literals blanked, and as written.
func tokens(src []byte) (string, string) {
	fset := token.NewFileSet()
	var s scanner.Scanner
	s.Init(fset.AddFile("", fset.Base(), len(src)), src, nil, 0)
	var shape, text strings.Builder
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			return shape.String(), text.String()
		}
		switch {
		case tok == token.IDENT:
			shape.WriteString("I ")
		case tok.IsLiteral():
			shape.WriteString("L ")
		default:
			shape.WriteString(tok.String() + " ")
		}
		text.WriteString(tok.String() + lit + " ")
	}
}
