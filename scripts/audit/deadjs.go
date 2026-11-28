package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// deadJS lists the exports of each JavaScript module whose name no other
// module, page script or template mentions, and whether the module itself or
// a Go test (the browser tests run JavaScript) does. It goes by name, not by
// import, so a name another file uses for something else hides an unused
// export.
func (a *audit) deadJS() error {
	words := map[string]map[string]bool{} // file to the names it uses
	exports := map[string][]tok{}
	toksOf := map[string][]tok{}
	for _, f := range a.only("js") {
		_, toks, err := measureJS(f.path)
		if err != nil {
			return err
		}
		toksOf[f.path] = toks
		words[f.path] = map[string]bool{}
		for i, t := range toks {
			if t.kind == 'n' {
				words[f.path][t.text] = true
			}
			if t.kind == 'n' && t.text == "export" {
				if n := exportedName(toks, i); n.text != "" {
					exports[f.path] = append(exports[f.path], n)
				}
			}
		}
	}
	tmpl, err := wordsOf("internal/web/templates/*.tmpl")
	if err != nil {
		return err
	}
	tests, err := wordsOf("tests/app/*.go")
	if err != nil {
		return err
	}
	var rows [][]string
	for p, es := range exports {
		for _, e := range es {
			used := tmpl[e.text]
			for q, w := range words {
				used = used || (q != p && w[e.text])
			}
			if used {
				continue
			}
			finding := "used nowhere"
			switch {
			case tests[e.text]:
				finding = "used only by browser tests"
			case uses(toksOf[p], e.text):
				finding = "used only inside its module"
			}
			rows = append(rows, []string{fmt.Sprintf("%s:%d", p, e.line), e.text, finding})
		}
	}
	sortRows(rows)
	head := []string{"where", "export no other module names", "finding"}
	fmt.Printf("\nJavaScript exports no other file names: %d\n", len(rows))
	table(head, rows, a.top)
	return a.save("dead-js.tsv", head, rows)
}

// exportedName is the name an export statement at i declares, for the
// forms export function|class|const|let|var|async function name.
func exportedName(toks []tok, i int) tok {
	for j := i + 1; j < len(toks) && j < i+4; j++ {
		switch toks[j].text {
		case "function", "class", "const", "let", "var", "async", "*":
			continue
		case "default", "{":
			return tok{}
		}
		if toks[j].kind == 'n' {
			return toks[j]
		}
		return tok{}
	}
	return tok{}
}

// uses says whether a module names an export of its own other than where it
// declares it.
func uses(toks []tok, name string) bool {
	n := 0
	for _, t := range toks {
		if t.kind == 'n' && t.text == name {
			n++
		}
	}
	return n > 1
}

// wordsOf is every word of the files a glob matches: the page templates,
// which import modules in inline scripts, or the browser tests.
func wordsOf(glob string) (map[string]bool, error) {
	ms, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`[A-Za-z_$][\w$]*`)
	words := map[string]bool{}
	for _, m := range ms {
		src, err := os.ReadFile(m)
		if err != nil {
			return nil, err
		}
		for _, w := range re.FindAllString(string(src), -1) {
			words[w] = true
		}
	}
	return words, nil
}
