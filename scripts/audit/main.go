// Command audit measures the code for the code and architecture audit (issue
// #249, docs/history/code-and-architecture-audit-findings.md): sizes, function
// lengths, comments, churn, coupling, unused names and duplication of the Go
// and JavaScript tracked in the repository.
//
//	go run ./scripts/audit [-out DIR] [-top N] [section...]
//
// Run it from the repository's root. It prints Markdown tables, and writes the
// full lists as tab-separated files into DIR (default bingle-audit in the
// system's temporary directory). The sections are sizes, funcs, comments,
// churn, deps, dead and dup; none named means all of them. It reads files and runs git and go list only.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

type section struct {
	name string
	run  func(*audit) error
}

type audit struct {
	out      string
	top      int
	files    []file
	measured []measured
}

func main() {
	out := flag.String("out", filepath.Join(os.TempDir(), "bingle-audit"), "directory for the full lists")
	top := flag.Int("top", 15, "rows in each printed table")
	flag.Parse()
	sections := []section{
		{"sizes", (*audit).sizes},
		{"funcs", (*audit).funcs},
		{"comments", (*audit).comments},
		{"churn", (*audit).churn},
		{"deps", (*audit).deps},
		{"dead", (*audit).dead},
		{"dup", (*audit).dup},
	}
	want := flag.Args()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	files, err := trackedFiles()
	if err != nil {
		fail(err)
	}
	a := &audit{out: *out, top: *top, files: files}
	for _, s := range sections {
		if len(want) > 0 && !slices.Contains(want, s.name) {
			continue
		}
		fmt.Printf("\n## %s\n", s.name)
		if err := s.run(a); err != nil {
			fail(fmt.Errorf("%s: %w", s.name, err))
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "audit:", err)
	os.Exit(1)
}
