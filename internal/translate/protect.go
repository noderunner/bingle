package translate

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The markers that stand for what is not language while a line is away
// (ADR 0143): ⟦1⟧, ⟦2⟧, … A line that already holds either bracket is not
// sent, since its markers could not be told from ours.
const (
	markOpen  = '⟦'
	markClose = '⟧'
)

// kept are the parts of a line that are never translated: links, handles and
// move notation (c3, a1-b2, e2e4, Nf3, exd5, O-O). Notation is matched loosely
// on purpose: a word kept as written by mistake costs little, a square
// translated costs the meaning.
var kept = regexp.MustCompile(strings.Join([]string{
	`https?://[^\s<>"]*[^\s<>".,!?;:)\]]`,
	`www\.[^\s<>"]*[^\s<>".,!?;:)\]]`,
	`@[\p{L}\p{N}_.\-]*[\p{L}\p{N}_]`,
	`\bO-O(?:-O)?\b`,
	`\b[A-Z]?[a-z]{0,2}x?[a-z]{1,2}[0-9]{1,2}(?:[-x:]?[A-Z]?[a-z]{1,2}[0-9]{1,2})*[+#]?\b`,
}, "|"))

var markerRE = regexp.MustCompile(`⟦([0-9]+)⟧`)

// protected is a line with what is not language swapped for markers.
type protected struct {
	text  string   // the line with markers
	parts []string // parts[i] is what marker i+1 stands for
}

// span is a part of a line, by byte offsets.
type span struct{ start, end int }

// protect swaps what is never translated for markers: links, handles,
// notation, emoji and the words in keep (the names of the people present). ok
// is false when the line holds a marker's bracket already.
func protect(line string, keep []string) (p protected, ok bool) {
	if strings.ContainsRune(line, markOpen) || strings.ContainsRune(line, markClose) {
		return protected{}, false
	}
	var spans []span
	for _, w := range keep {
		if w == "" {
			continue
		}
		for from := 0; ; {
			i := strings.Index(line[from:], w)
			if i < 0 {
				break
			}
			s, e := from+i, from+i+len(w)
			if wordEdge(line, s, e) {
				spans = append(spans, span{s, e})
			}
			from = e
		}
	}
	for _, m := range kept.FindAllStringIndex(line, -1) {
		spans = append(spans, span{m[0], m[1]})
	}
	spans = append(spans, emojiRuns(line)...)
	// The earliest first, and the longest of those that start together; a
	// span overlapping one already taken is left out.
	slices.SortFunc(spans, func(a, b span) int {
		if a.start != b.start {
			return a.start - b.start
		}
		return b.end - a.end
	})
	var b strings.Builder
	at := 0
	for _, s := range spans {
		if s.start < at {
			continue
		}
		b.WriteString(line[at:s.start])
		p.parts = append(p.parts, line[s.start:s.end])
		b.WriteRune(markOpen)
		b.WriteString(strconv.Itoa(len(p.parts)))
		b.WriteRune(markClose)
		at = s.end
	}
	b.WriteString(line[at:])
	p.text = b.String()
	return p, true
}

// wordEdge reports whether line[s:e] is a whole word: no letter or digit
// touches it on either side.
func wordEdge(line string, s, e int) bool {
	if s > 0 {
		if r, _ := utf8.DecodeLastRuneInString(line[:s]); isWordRune(r) {
			return false
		}
	}
	if e < len(line) {
		if r, _ := utf8.DecodeRuneInString(line[e:]); isWordRune(r) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// isEmoji reports whether r is part of an emoji or a symbol like one: symbols,
// skin tones, the joiner, the emoji presentation selector and the keycap.
func isEmoji(r rune) bool {
	return unicode.Is(unicode.So, r) || (r >= 0x1F3FB && r <= 0x1F3FF) || r == 0x200D || r == 0xFE0F || r == 0x20E3
}

// emojiRuns finds the runs of emoji in line.
func emojiRuns(line string) []span {
	var out []span
	start := -1
	for i, r := range line {
		switch {
		case isEmoji(r) && start < 0:
			start = i
		case !isEmoji(r) && start >= 0:
			out = append(out, span{start, i})
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, span{start, len(line)})
	}
	return out
}

// hasWords reports whether anything is left to translate: a letter outside
// the markers. Markers hold only digits.
func hasWords(s string) bool {
	return strings.IndexFunc(s, unicode.IsLetter) >= 0
}

// restore puts back what the markers stand for. ok is false unless every
// marker is there exactly once and no other is.
func restore(s string, p protected) (string, bool) {
	seen := make([]bool, len(p.parts))
	found := markerRE.FindAllStringSubmatch(s, -1)
	if len(found) != len(p.parts) {
		return "", false
	}
	for _, m := range found {
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 || n > len(p.parts) || seen[n-1] {
			return "", false
		}
		seen[n-1] = true
	}
	// A bracket the model wrote outside a marker of ours is not a marker.
	if rest := markerRE.ReplaceAllString(s, ""); strings.ContainsRune(rest, markOpen) || strings.ContainsRune(rest, markClose) {
		return "", false
	}
	return markerRE.ReplaceAllStringFunc(s, func(m string) string {
		n, _ := strconv.Atoi(m[len(string(markOpen)) : len(m)-len(string(markClose))])
		return p.parts[n-1]
	}), true
}
