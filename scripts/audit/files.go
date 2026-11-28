package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// A file is one tracked Go or JavaScript file. Tests are _test.go files,
// everything under tests/ (the fixture games' code included); the audit's own
// scripts are left out.
type file struct {
	path string
	lang string // "go" or "js"
	dir  string
	test bool
}

func trackedFiles() ([]file, error) {
	out, err := exec.Command("git", "ls-files", "--", "*.go", "*.js").Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var files []file
	for _, p := range strings.Fields(string(out)) {
		if strings.HasPrefix(p, "scripts/audit/") {
			continue
		}
		f := file{path: p, lang: strings.TrimPrefix(filepath.Ext(p), "."), dir: filepath.Dir(p)}
		f.test = strings.HasSuffix(p, "_test.go") || strings.HasPrefix(p, "tests/")
		files = append(files, f)
	}
	return files, nil
}

func (a *audit) only(lang string) []file {
	var fs []file
	for _, f := range a.files {
		if f.lang == lang {
			fs = append(fs, f)
		}
	}
	return fs
}

func kind(test bool) string {
	if test {
		return "test"
	}
	return "code"
}

// group is where a file's numbers are summed: its directory, or for the games
// and the tests the directory one level down, so each game is one row.
func group(path string) string {
	parts := strings.Split(filepath.Dir(path), "/")
	for _, root := range [][]string{{"internal", "games"}, {"internal", "engine"}, {"tests"}} {
		if len(parts) > len(root) && slices.Equal(parts[:len(root)], root) {
			return strings.Join(parts[:len(root)+1], "/")
		}
	}
	return filepath.Dir(path)
}

// table prints a Markdown table of at most top rows.
func table(head []string, rows [][]string, top int) {
	fmt.Println()
	fmt.Println("| " + strings.Join(head, " | ") + " |")
	fmt.Println("|" + strings.Repeat("---|", len(head)))
	for i, r := range rows {
		if top > 0 && i == top {
			fmt.Printf("| … %d more |%s\n", len(rows)-top, strings.Repeat(" |", len(head)-1))
			break
		}
		fmt.Println("| " + strings.Join(r, " | ") + " |")
	}
}

// save writes rows as a tab-separated file in the output directory.
func (a *audit) save(name string, head []string, rows [][]string) error {
	var b strings.Builder
	b.WriteString(strings.Join(head, "\t") + "\n")
	for _, r := range rows {
		b.WriteString(strings.Join(r, "\t") + "\n")
	}
	return os.WriteFile(filepath.Join(a.out, name), []byte(b.String()), 0o644)
}

// byInt sorts rows by the integer in column col, largest first, then by the
// first column.
func byInt(rows [][]string, col int) {
	sort.SliceStable(rows, func(i, j int) bool {
		x, y := atoi(rows[i][col]), atoi(rows[j][col])
		if x != y {
			return x > y
		}
		return rows[i][0] < rows[j][0]
	})
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func itoa(n int) string { return fmt.Sprint(n) }
