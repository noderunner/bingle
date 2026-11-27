// Package spelling_test keeps one word spelled one way. The repository's word for a set of pieces is
// color, in the definition, the API, the code and the prose (CLAUDE.md, ADR 0154); the British
// spelling had crept into comments, docs, class names and an ADR's file name, and this test fails
// when a tracked file has it again.
package spelling_test

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The word is built from two halves so that this file is not a use of it.
var british = "col" + "our"

// exceptions are the tracked files that may use the British spelling, each for the reason beside it.
// Every other file, its name included, may not.
var exceptions = map[string]string{
	// An unknown field and an unknown value on purpose: that the other spelling is refused is worth a test.
	"tests/dsl_yaml_test.go":    "a seat's \"" + british + "\" is an unknown field",
	"tests/turns/turns_test.go": "a turn's distinct \"" + british + "\" is an unknown value",
}

func TestNoTrackedFileSpellsColorTheBritishWay(t *testing.T) {
	out, err := exec.Command("git", "-C", "../..", "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("the tracked files cannot be listed (no git, or not a checkout): %v", err)
	}
	used := map[string]bool{}
	for _, path := range strings.Split(string(out), "\x00") {
		if path == "" {
			continue
		}
		raw, err := os.ReadFile("../../" + path)
		if err != nil {
			continue // listed but gone from the working tree, or a link to nowhere
		}
		if bytes.IndexByte(raw, 0) >= 0 {
			continue // a binary file
		}
		has := strings.Contains(strings.ToLower(path), british) || strings.Contains(strings.ToLower(string(raw)), british)
		if !has {
			continue
		}
		if _, ok := exceptions[path]; ok {
			used[path] = true
			continue
		}
		line := 0
		for i, l := range strings.Split(string(raw), "\n") {
			if strings.Contains(strings.ToLower(l), british) {
				line = i + 1
				break
			}
		}
		t.Errorf("%s:%d spells color the British way: spell it color (an exception goes in this test's list, with its reason)", path, line)
	}
	for path := range exceptions {
		if !used[path] {
			t.Errorf("%s is listed as an exception but no longer has the British spelling: take it off the list", path)
		}
	}
}
