package look_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The page's paint, held to ADR 0154: one module, paint.js, turns a color
// into paint and holds the color words the page knows. The stylesheet and the
// board themes paint exactly those words, and no other shared script carries
// a piece's paint or a list of color words of its own.

var static = filepath.Join("..", "..", "internal", "web", "static")

func readStatic(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(static, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// colorWords reads paint.js's COLOR_WORDS.
func colorWords(t *testing.T) []string {
	t.Helper()
	m := regexp.MustCompile(`export const COLOR_WORDS = Object\.freeze\(\[([^\]]*)\]\)`).FindStringSubmatch(readStatic(t, "js/paint.js"))
	if m == nil {
		t.Fatal("paint.js has no COLOR_WORDS")
	}
	var words []string
	for _, q := range regexp.MustCompile(`"([a-z]+)"`).FindAllStringSubmatch(m[1], -1) {
		words = append(words, q[1])
	}
	if len(words) < 2 {
		t.Fatalf("COLOR_WORDS = %v", words)
	}
	slices.Sort(words)
	return words
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func TestOneModulePaintsAColor(t *testing.T) {
	words := colorWords(t)
	known := map[string]bool{}
	for _, w := range words {
		known[w] = true
	}
	files, err := filepath.Glob(filepath.Join(static, "js", "*.js"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no shared scripts: %v", err)
	}
	pieceVar := regexp.MustCompile(`--piece-(\$\{|[a-z]+-(fill|stroke))`)
	quoted := regexp.MustCompile(`"([a-z]+)"`)
	for _, f := range files {
		name := filepath.Base(f)
		if name == "paint.js" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if strings.Contains(src, "cssColor(") {
			t.Errorf("%s calls cssColor: ask paint.js (colorClass, colorPaint, classWord)", name)
		}
		// The board themes paint the words; every other script asks paint.js.
		if name == "themes.js" {
			continue
		}
		if m := pieceVar.FindString(src); m != "" {
			t.Errorf("%s writes a piece's paint (%s…): ask paint.js's colorPaint", name, m)
		}
		named := map[string]bool{}
		for _, q := range quoted.FindAllStringSubmatch(src, -1) {
			if known[q[1]] {
				named[q[1]] = true
			}
		}
		if len(named) >= 3 {
			t.Errorf("%s lists color words of its own %v: they are paint.js's COLOR_WORDS", name, sortedKeys(named))
		}
	}
}

// The stylesheet paints every color word and no other, pieces and swatches.
func TestTheStylesheetPaintsTheColorWords(t *testing.T) {
	words := colorWords(t)
	css := readStatic(t, "style.css")
	for _, part := range []string{"fill", "stroke"} {
		got := map[string]bool{}
		for _, m := range regexp.MustCompile(`--piece-([a-z]+)-`+part+`:`).FindAllStringSubmatch(css, -1) {
			got[m[1]] = true
		}
		if have := sortedKeys(got); !slices.Equal(have, words) {
			t.Errorf("style.css's --piece-<word>-%s are for %v, want paint.js's %v", part, have, words)
		}
	}
	for _, w := range words {
		for _, rule := range []string{".piece.color-" + w + " .piece-body", ".swatch.color-" + w + " "} {
			if !strings.Contains(css, rule) {
				t.Errorf("style.css has no rule %q", rule)
			}
		}
	}
}

// Every board theme has a stone for every color word and for no other.
func TestEveryBoardThemePaintsTheColorWords(t *testing.T) {
	words := colorWords(t)
	src := readStatic(t, "js/themes.js")
	if !strings.Contains(src, `import { COLOR_WORDS } from "./paint.js";`) {
		t.Error("themes.js does not take its color words from paint.js")
	}
	theme := regexp.MustCompile(`(?s)id: "([a-z]+)",.*?stones: \{(.*?)\n\t\},`)
	key := regexp.MustCompile(`(?m)^\t\t([a-z]+): stone\(`)
	themes := theme.FindAllStringSubmatch(src, -1)
	if len(themes) < 6 {
		t.Fatalf("%d board themes found in themes.js", len(themes))
	}
	for _, th := range themes {
		got := map[string]bool{}
		for _, k := range key.FindAllStringSubmatch(th[2], -1) {
			got[k[1]] = true
		}
		if have := sortedKeys(got); !slices.Equal(have, words) {
			t.Errorf("the %s theme paints %v, want paint.js's %v", th[1], have, words)
		}
	}
}
