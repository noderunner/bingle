package i18n

import (
	"os"
	"strings"
	"testing"
)

// glossaryRows reads the first table of a glossary file (docs/glossary/<code>.md), the words of the
// interface: its rows' cells, the header and the rule under it left out. English's second table, the
// words of the code and the docs (ADR 0154), has no catalog key and is not read.
func glossaryRows(t *testing.T, code string) [][]string {
	t.Helper()
	raw, err := os.ReadFile("../../docs/glossary/" + code + ".md")
	if err != nil {
		t.Fatalf("the glossary of %s: %v", code, err)
	}
	var rows [][]string
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, "|") {
			if len(rows) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "|-") {
			continue
		}
		var cells []string
		for _, c := range strings.Split(strings.Trim(line, "|"), " | ") {
			cells = append(cells, strings.TrimSpace(c))
		}
		rows = append(rows, cells)
	}
	if len(rows) < 2 {
		t.Fatalf("the glossary of %s has no table", code)
	}
	return rows[1:]
}

// shown is the words of key in a catalog, every plural form of it together.
func shown(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case string:
		return v
	case map[string]any:
		var all []string
		for _, f := range v {
			s, _ := f.(string)
			all = append(all, s)
		}
		return strings.Join(all, " ")
	}
	return ""
}

// The glossary (ADR 0141) lists the platform's words with the key that shows each. Every
// language's file has every term, in the same order, as its own catalog shows it, so the glossary
// and the interface cannot drift apart; and two terms share a word in a language only where its
// file says so ("Same word as …"), so that two ideas are not muddled by accident.
func TestGlossary_EachLanguageHasEveryTermAsItsCatalogShowsIt(t *testing.T) {
	en, _ := Raw(Default)
	terms := glossaryRows(t, Default)
	for _, r := range terms {
		term, key := r[0], strings.Trim(r[1], "`")
		word, _, _ := strings.Cut(term, " (")
		if !strings.Contains(strings.ToLower(shown(en, key)), strings.ToLower(word)) {
			t.Errorf("en: %q is not in %s, %q", term, key, shown(en, key))
		}
	}
	for _, l := range Languages() {
		if l.Code == Default {
			continue
		}
		own, err := Raw(l.Code)
		if err != nil {
			t.Fatal(err)
		}
		rows := glossaryRows(t, l.Code)
		if len(rows) != len(terms) {
			t.Errorf("%s: %d terms, English has %d", l.Code, len(rows), len(terms))
			continue
		}
		shared := map[string][]int{}
		for i, r := range rows {
			if len(r) < 3 || r[0] != terms[i][0] {
				t.Errorf("%s: row %d is %q, English's is %q", l.Code, i+1, r, terms[i][0])
				continue
			}
			key := strings.Trim(terms[i][1], "`")
			if !strings.Contains(strings.ToLower(shown(own, key)), strings.ToLower(r[1])) {
				t.Errorf("%s: %s is %q, but %s says %q", l.Code, r[0], r[1], key, shown(own, key))
			}
			shared[strings.ToLower(r[1])] = append(shared[strings.ToLower(r[1])], i)
		}
		for word, at := range shared {
			for _, i := range at {
				for _, j := range at {
					if i != j && !strings.Contains(rows[i][2], "Same word as "+rows[j][0]) {
						t.Errorf("%s: %s and %s are both %q, and the note of %s does not say so", l.Code, rows[i][0], rows[j][0], word, rows[i][0])
					}
				}
			}
		}
	}
}
