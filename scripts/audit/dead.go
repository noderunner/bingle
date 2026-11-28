package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// A goPkg is one directory's Go files, every build tag included.
type goPkg struct {
	dir, name string
	files     map[string]*ast.File // path to file
}

// A decl is one top-level name: a function, a method, a type, a var or a const.
type decl struct {
	pkg, name, kind, where string
	method, test           bool
}

func parseGo(files []file) (map[string]*goPkg, *token.FileSet, error) {
	fset := token.NewFileSet()
	pkgs := map[string]*goPkg{}
	for _, f := range files {
		af, err := parser.ParseFile(fset, f.path, nil, parser.SkipObjectResolution|parser.ParseComments)
		if err != nil {
			return nil, nil, err
		}
		p := pkgs[f.dir]
		if p == nil {
			p = &goPkg{dir: f.dir, files: map[string]*ast.File{}}
			pkgs[f.dir] = p
		}
		if !strings.HasSuffix(af.Name.Name, "_test") && !strings.HasSuffix(f.path, "_test.go") {
			p.name = af.Name.Name
		}
		p.files[f.path] = af
	}
	return pkgs, fset, nil
}

func declsOf(p *goPkg, fset *token.FileSet) []decl {
	var ds []decl
	for fp, f := range p.files {
		test := strings.HasSuffix(fp, "_test.go")
		add := func(n *ast.Ident, kind string, method bool) {
			if n.Name != "_" && n.Name != "init" && n.Name != "main" {
				ds = append(ds, decl{p.dir, n.Name, kind, fmt.Sprintf("%s:%d", fp, fset.Position(n.Pos()).Line), method, test})
			}
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if !directive(d.Doc) {
					add(d.Name, "func", d.Recv != nil)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						add(s.Name, "type", false)
					case *ast.ValueSpec:
						for _, n := range s.Names {
							add(n, strings.ToLower(d.Tok.String()), false)
						}
					}
				}
			}
		}
	}
	return ds
}

// directive says whether a doc holds //go:wasmexport, //go:linkname or
// //export, which call a function from outside Go.
func directive(g *ast.CommentGroup) bool {
	if g == nil {
		return false
	}
	for _, c := range g.List {
		for _, d := range []string{"//go:wasmexport", "//go:linkname", "//export "} {
			if strings.HasPrefix(c.Text, d) {
				return true
			}
		}
	}
	return false
}

// refs counts, per directory, the identifiers and selectors each file uses,
// apart from the names it declares at top level.
type refs struct {
	idents    map[string]map[string]int // dir to name to uses in code files
	testUses  map[string]map[string]int // the same, in test files
	selectors map[string]map[string]int // dir to selector name (x.Name) to uses
	qualified map[string]map[string]int // import path to Name to uses as pkg.Name from other dirs
	qualTest  map[string]map[string]int
}

func collectRefs(pkgs map[string]*goPkg) refs {
	r := refs{map[string]map[string]int{}, map[string]map[string]int{}, map[string]map[string]int{}, map[string]map[string]int{}, map[string]map[string]int{}}
	inc := func(m map[string]map[string]int, k, n string) {
		if m[k] == nil {
			m[k] = map[string]int{}
		}
		m[k][n]++
	}
	pkgName := map[string]string{}
	for dir, p := range pkgs {
		pkgName[module+"/"+dir] = p.name
	}
	for dir, p := range pkgs {
		for fp, f := range p.files {
			test := strings.HasSuffix(fp, "_test.go") || strings.HasPrefix(fp, "tests/")
			alias := map[string]string{}
			for _, im := range f.Imports {
				ip, _ := strconv.Unquote(im.Path.Value)
				name := pkgName[ip]
				if im.Name != nil {
					name = im.Name.Name
				}
				if name != "" {
					alias[name] = strings.TrimPrefix(ip, module+"/")
				}
			}
			declared := topLevelIdents(f)
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.SelectorExpr:
					inc(r.selectors, dir, n.Sel.Name)
					if x, ok := n.X.(*ast.Ident); ok && alias[x.Name] != "" {
						if test {
							inc(r.qualTest, alias[x.Name], n.Sel.Name)
						} else {
							inc(r.qualified, alias[x.Name], n.Sel.Name)
						}
					}
				case *ast.Ident:
					if declared[n] {
						return true
					}
					if test {
						inc(r.testUses, dir, n.Name)
					} else {
						inc(r.idents, dir, n.Name)
					}
				}
				return true
			})
		}
	}
	return r
}

func topLevelIdents(f *ast.File) map[*ast.Ident]bool {
	m := map[*ast.Ident]bool{}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			m[d.Name] = true
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					m[s.Name] = true
				case *ast.ValueSpec:
					for _, n := range s.Names {
						m[n] = true
					}
				}
			}
		}
	}
	return m
}

// Methods that satisfy the standard library's interfaces, called from it.
var stdMethods = map[string]bool{"String": true, "Error": true, "Unwrap": true, "ServeHTTP": true,
	"MarshalJSON": true, "UnmarshalJSON": true, "MarshalText": true, "UnmarshalText": true,
	"Len": true, "Less": true, "Swap": true, "Push": true, "Pop": true, "Read": true, "Write": true,
	"Close": true, "Format": true, "Is": true, "Scan": true, "Value": true, "Describe": true, "Collect": true,
	"MarshalYAML": true, "UnmarshalYAML": true, "Header": true, "WriteHeader": true, "Flush": true, "Hijack": true,
	"Name": true, "Size": true, "Mode": true, "ModTime": true, "IsDir": true, "Sys": true, "Stat": true,
	"Open": true, "ReadDir": true, "Seek": true, "Info": true, "Type": true, "ReadAt": true}

func (a *audit) dead() error {
	pkgs, fset, err := parseGo(a.only("go"))
	if err != nil {
		return err
	}
	r := collectRefs(pkgs)
	tmpl, err := templateWords()
	if err != nil {
		return err
	}
	var unexp, exp [][]string
	for _, p := range pkgs {
		if strings.HasPrefix(p.dir, "tests/") {
			continue
		}
		for _, d := range declsOf(p, fset) {
			if d.test {
				continue
			}
			if row := judge(d, p, r, tmpl); row != nil {
				if ast.IsExported(d.name) {
					exp = append(exp, row)
				} else {
					unexp = append(unexp, row)
				}
			}
		}
	}
	head := []string{"where", "name", "kind", "finding"}
	sortRows(unexp)
	sortRows(exp)
	fmt.Printf("\nUnexported names used nowhere, or only by tests: %d\n", len(unexp))
	table(head, unexp, a.top)
	fmt.Printf("\nExported names no other package uses (candidates: the private overlay is not here): %d\n", len(exp))
	summarise(exp)
	if err := a.save("dead-unexported.tsv", head, unexp); err != nil {
		return err
	}
	if err := a.save("dead-exported.tsv", head, exp); err != nil {
		return err
	}
	return a.deadJS()
}

func judge(d decl, p *goPkg, r refs, tmpl map[string]bool) []string {
	inside := r.idents[p.dir][d.name]
	if d.method {
		inside = r.selectors[p.dir][d.name] + r.idents[p.dir][d.name]
	}
	inTests := r.testUses[p.dir][d.name]
	row := func(finding string) []string { return []string{d.where, d.name, d.kind, finding} }
	if !ast.IsExported(d.name) {
		switch {
		case inside == 0 && inTests == 0:
			return row("unused")
		case inside == 0:
			return row("used only by tests")
		}
		return nil
	}
	if p.name == "main" || tmpl[d.name] || (d.method && stdMethods[d.name]) {
		return nil
	}
	var outside, outTests int
	if d.method {
		for dir, m := range r.selectors {
			if dir != p.dir {
				outside += m[d.name]
			}
		}
	} else {
		outside, outTests = r.qualified[p.dir][d.name], r.qualTest[p.dir][d.name]
	}
	switch {
	case outside > 0:
		return nil
	case outTests > 0:
		return row("outside its package, used only by tests")
	case inside > 0 || inTests > 0:
		return row("used only inside its package")
	}
	return row("used nowhere")
}

func sortRows(rows [][]string) {
	sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
}

func summarise(rows [][]string) {
	n := map[string]map[string]int{}
	for _, r := range rows {
		dir := filepath.Dir(strings.SplitN(r[0], ":", 2)[0])
		if n[dir] == nil {
			n[dir] = map[string]int{}
		}
		n[dir][r[3]]++
	}
	var t [][]string
	for dir, m := range n {
		t = append(t, []string{dir, itoa(m["used nowhere"]), itoa(m["used only inside its package"]), itoa(m["outside its package, used only by tests"])})
	}
	byInt(t, 1)
	table([]string{"package", "used nowhere", "only inside", "only tests outside"}, t, 20)
}

// templateWords is every .Name a template refers to, since a template may call
// an exported method or read a field no Go file names.
func templateWords() (map[string]bool, error) {
	ms, err := filepath.Glob("internal/web/templates/*.tmpl")
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`\.([A-Z]\w*)`)
	words := map[string]bool{}
	for _, m := range ms {
		src, err := os.ReadFile(m)
		if err != nil {
			return nil, err
		}
		for _, w := range re.FindAllStringSubmatch(string(src), -1) {
			words[w[1]] = true
		}
	}
	return words, nil
}
