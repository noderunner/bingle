package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strings"
)

const module = "github.com/noderunner/bingle"

// deps: fan-in and fan-out of each package of the module (its own imports,
// tests left out), and the import graph of the JavaScript modules.
func (a *audit) deps() error {
	if err := a.goDeps(); err != nil {
		return err
	}
	if err := a.cycleNotes(); err != nil {
		return err
	}
	return a.jsDeps()
}

func (a *audit) goDeps() error {
	out, err := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Imports " "}}`, "./...").Output()
	if err != nil {
		return fmt.Errorf("go list: %w", err)
	}
	imports := map[string][]string{}
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(l)
		p := strings.TrimPrefix(f[0], module+"/")
		imports[p] = nil
		for _, i := range f[1:] {
			if strings.HasPrefix(i, module+"/") {
				imports[p] = append(imports[p], strings.TrimPrefix(i, module+"/"))
			}
		}
	}
	in := map[string]int{}
	for _, is := range imports {
		for _, i := range is {
			in[i]++
		}
	}
	var rows [][]string
	for p, is := range imports {
		if strings.HasPrefix(p, "tests/") || strings.HasPrefix(p, "scripts/") {
			continue
		}
		rows = append(rows, []string{p, itoa(in[p]), itoa(len(is)), itoa(len(closure(imports, p)))})
	}
	head := []string{"package", "fan-in", "fan-out", "internal deps, transitive"}
	byInt(rows, 1)
	fmt.Println("\nPackages by fan-in (packages of the module importing it, tests and scripts included):")
	table(head, rows, a.top)
	out2 := append([][]string(nil), rows...)
	byInt(out2, 2)
	fmt.Println("\nPackages by fan-out:")
	table(head, out2, a.top)
	return a.save("go-deps.tsv", head, rows)
}

func closure(g map[string][]string, p string) map[string]bool {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(q string) {
		for _, i := range g[q] {
			if !seen[i] {
				seen[i] = true
				walk(i)
			}
		}
	}
	walk(p)
	return seen
}

// cycleNotes lists the comments that say a shape was chosen to avoid an
// import cycle.
func (a *audit) cycleNotes() error {
	re := regexp.MustCompile(`(?i)(import cycle|cycl\w* import|avoid\w* (a|an) (import )?cycle|would import)`)
	var rows [][]string
	for _, f := range a.only("go") {
		src, err := os.ReadFile(f.path)
		if err != nil {
			return err
		}
		for i, l := range strings.Split(string(src), "\n") {
			if t := strings.TrimSpace(l); strings.HasPrefix(t, "//") && re.MatchString(t) {
				rows = append(rows, []string{fmt.Sprintf("%s:%d", f.path, i+1), strings.ReplaceAll(t, "|", "/")})
			}
		}
	}
	fmt.Printf("\nComments that mention an import cycle: %d\n", len(rows))
	table([]string{"where", "comment"}, rows, a.top)
	return a.save("cycle-notes.tsv", []string{"where", "comment"}, rows)
}

// jsImports resolves the modules a JavaScript file imports, statically or
// with import() of a string.
func jsImports(p string, toks []tok) []string {
	var out []string
	for i, t := range toks {
		if t.kind != 'n' || t.text != "import" {
			continue
		}
		for j := i + 1; j < len(toks) && j < i+200; j++ {
			if toks[j].kind == 's' {
				spec := strings.Trim(toks[j].text, "\"'`")
				if strings.Contains(spec, "${") || !strings.HasSuffix(spec, ".js") {
					break
				}
				out = append(out, resolveJS(p, spec))
				break
			}
			if toks[j].text == ";" || (toks[j].text == ")" && j > i+1) {
				break
			}
		}
	}
	return out
}

func resolveJS(from, spec string) string {
	if strings.HasPrefix(spec, "/static/") {
		return "internal/web" + spec
	}
	return path.Clean(path.Join(path.Dir(from), spec))
}

func (a *audit) jsDeps() error {
	imports := map[string][]string{}
	fetches := map[string]int{}
	for _, f := range a.only("js") {
		_, toks, err := measureJS(f.path)
		if err != nil {
			return err
		}
		imports[f.path] = jsImports(f.path, toks)
		for i, t := range toks {
			if t.text == "fetch" && i+1 < len(toks) && toks[i+1].text == "(" && (i == 0 || toks[i-1].text != ".") {
				fetches[f.path]++
			}
		}
	}
	in := map[string][]string{}
	for p, is := range imports {
		for _, i := range is {
			in[i] = append(in[i], path.Base(p))
		}
	}
	var rows [][]string
	for p, is := range imports {
		sort.Strings(in[p])
		rows = append(rows, []string{p, itoa(len(in[p])), itoa(len(is)), itoa(fetches[p]), strings.Join(in[p], " ")})
	}
	head := []string{"module", "imported by", "imports", "fetch( calls", "importers"}
	byInt(rows, 1)
	fmt.Println("\nJavaScript modules by how many modules import them:")
	table(head[:4], cut(rows, 4), a.top)
	byInt(rows, 3)
	fmt.Println("\nJavaScript modules calling fetch( themselves:")
	var f [][]string
	for _, r := range rows {
		if r[3] != "0" {
			f = append(f, r[:4])
		}
	}
	table(head[:4], f, a.top)
	return a.save("js-deps.tsv", head, rows)
}

func cut(rows [][]string, n int) [][]string {
	out := make([][]string, len(rows))
	for i, r := range rows {
		out[i] = r[:n]
	}
	return out
}
