package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	bigFile   = 500
	smallFile = 200
	longFunc  = 50
)

type measured struct {
	file
	measure
}

// measureAll measures every file once.
func (a *audit) measureAll() ([]measured, error) {
	if a.measured != nil {
		return a.measured, nil
	}
	for _, f := range a.files {
		var m measure
		var err error
		if f.lang == "go" {
			m, _, err = measureGo(f.path)
		} else {
			m, _, err = measureJS(f.path)
		}
		if err != nil {
			return nil, err
		}
		a.measured = append(a.measured, measured{f, m})
	}
	return a.measured, nil
}

// sizes: files over 500 lines listed, under 200 counted, and totals per group.
func (a *audit) sizes() error {
	ms, err := a.measureAll()
	if err != nil {
		return err
	}
	var big, all [][]string
	type tally struct{ files, lines, big, small int }
	groups := map[string]*tally{}
	for _, m := range ms {
		row := []string{m.path, m.lang, kind(m.test), itoa(m.lines)}
		all = append(all, row)
		if m.lines > bigFile {
			big = append(big, row)
		}
		key := group(m.path) + "\t" + m.lang + "\t" + kind(m.test)
		t := groups[key]
		if t == nil {
			t = &tally{}
			groups[key] = t
		}
		t.files++
		t.lines += m.lines
		t.big += b2i(m.lines > bigFile)
		t.small += b2i(m.lines < smallFile)
	}
	byInt(big, 3)
	byInt(all, 3)
	var g [][]string
	for k, t := range groups {
		g = append(g, append(strings.Split(k, "\t"), itoa(t.files), itoa(t.lines), itoa(t.big), itoa(t.small)))
	}
	byInt(g, 4)
	ghead := []string{"group", "lang", "kind", "files", "lines", "over 500", "under 200"}
	fmt.Printf("\nFiles over %d lines: %d (code %d, tests %d)\n", bigFile, len(big), countKind(big, 2, "code"), countKind(big, 2, "test"))
	table([]string{"file", "lang", "kind", "lines"}, big, a.top)
	fmt.Println("\nTotals per group, largest first:")
	table(ghead, g, a.top)
	if err := a.save("files.tsv", []string{"file", "lang", "kind", "lines"}, all); err != nil {
		return err
	}
	return a.save("groups.tsv", ghead, g)
}

// funcs: functions over 50 lines, and how many each group has.
func (a *audit) funcs() error {
	ms, err := a.measureAll()
	if err != nil {
		return err
	}
	var long [][]string
	type tally struct{ funcs, long, longLines int }
	groups := map[string]*tally{}
	lost := 0
	for _, m := range ms {
		lost += b2i(m.unbalanced)
		key := group(m.path) + "\t" + m.lang + "\t" + kind(m.test)
		t := groups[key]
		if t == nil {
			t = &tally{}
			groups[key] = t
		}
		for _, f := range m.funcs {
			t.funcs++
			if f.lines <= longFunc {
				continue
			}
			t.long++
			t.longLines += f.lines
			long = append(long, []string{fmt.Sprintf("%s:%d", m.path, f.line), f.name, m.lang, kind(m.test), itoa(f.lines), itoa(f.depth)})
		}
	}
	byInt(long, 4)
	var g [][]string
	for k, t := range groups {
		if t.long > 0 {
			g = append(g, append(strings.Split(k, "\t"), itoa(t.funcs), itoa(t.long), itoa(t.longLines)))
		}
	}
	byInt(g, 4)
	head := []string{"where", "function", "lang", "kind", "lines", "nested in"}
	ghead := []string{"group", "lang", "kind", "functions", "over 50", "their lines"}
	for _, lang := range []string{"go", "js"} {
		var rows [][]string
		for _, r := range long {
			if r[2] == lang && r[3] == "code" {
				rows = append(rows, r)
			}
		}
		fmt.Printf("\n%s functions over %d lines, not tests: %d\n", lang, longFunc, len(rows))
		table(head, rows, a.top)
	}
	fmt.Printf("\nJavaScript files whose brackets did not balance (their functions unmeasured): %d\n", lost)
	fmt.Println("\nGroups by functions over 50 lines:")
	table(ghead, g, a.top)
	if err := a.save("funcs.tsv", head, long); err != nil {
		return err
	}
	return a.save("func-groups.tsv", ghead, g)
}

// comments: comment lines to code lines per group and per file.
func (a *audit) comments() error {
	ms, err := a.measureAll()
	if err != nil {
		return err
	}
	type tally struct{ code, comment, doc int }
	groups := map[string]*tally{}
	var files [][]string
	for _, m := range ms {
		if m.test {
			continue
		}
		key := group(m.path) + "\t" + m.lang
		t := groups[key]
		if t == nil {
			t = &tally{}
			groups[key] = t
		}
		t.code += m.code
		t.comment += m.comment
		t.doc += m.doc
		if m.code >= 50 {
			files = append(files, densityRow(m.path, m.lang, m.code, m.comment, m.doc))
		}
	}
	var g [][]string
	total := map[string]*tally{"go": {}, "js": {}}
	for k, t := range groups {
		parts := strings.Split(k, "\t")
		g = append(g, densityRow(parts[0], parts[1], t.code, t.comment, t.doc))
		tt := total[parts[1]]
		tt.code, tt.comment, tt.doc = tt.code+t.code, tt.comment+t.comment, tt.doc+t.doc
	}
	head := []string{"where", "lang", "code lines", "comment lines", "of them doc", "other per 100 code"}
	sort.SliceStable(g, func(i, j int) bool { return atof(g[i][5]) > atof(g[j][5]) })
	sort.SliceStable(files, func(i, j int) bool { return atof(files[i][5]) > atof(files[j][5]) })
	for _, lang := range []string{"go", "js"} {
		t := total[lang]
		r := densityRow("all", lang, t.code, t.comment, t.doc)
		fmt.Printf("\n%s, not tests: %s code lines, %s comment lines (%s doc), %s other comment lines per 100 code lines\n", lang, r[2], r[3], r[4], r[5])
	}
	fmt.Println("\nGroups, densest first (other = comment lines that are not doc):")
	table(head, g, a.top)
	fmt.Println("\nFiles of 50 code lines or more, densest first:")
	table(head, files, a.top)
	if err := a.save("comment-groups.tsv", head, g); err != nil {
		return err
	}
	return a.save("comment-files.tsv", head, files)
}

func densityRow(where, lang string, code, comment, doc int) []string {
	return []string{where, lang, itoa(code), itoa(comment), itoa(doc), fmt.Sprintf("%.1f", 100*float64(comment-doc)/float64(max(code, 1)))}
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func countKind(rows [][]string, col int, k string) int {
	n := 0
	for _, r := range rows {
		if r[col] == k {
			n++
		}
	}
	return n
}
