package i18n

import "strings"

// accented is each ASCII letter's look-alike in the pseudo-locale.
var accented = map[rune]rune{
	'a': 'á', 'b': 'ƀ', 'c': 'ç', 'd': 'ď', 'e': 'é', 'f': 'ƒ', 'g': 'ĝ', 'h': 'ĥ', 'i': 'í', 'j': 'ĵ',
	'k': 'ķ', 'l': 'ĺ', 'm': 'ɱ', 'n': 'ñ', 'o': 'ö', 'p': 'þ', 'q': 'ǫ', 'r': 'ŕ', 's': 'š', 't': 'ţ',
	'u': 'ü', 'v': 'ṽ', 'w': 'ŵ', 'x': 'ẋ', 'y': 'ý', 'z': 'ž',
	'A': 'Á', 'B': 'Ɓ', 'C': 'Ç', 'D': 'Ď', 'E': 'É', 'F': 'Ƒ', 'G': 'Ĝ', 'H': 'Ĥ', 'I': 'Í', 'J': 'Ĵ',
	'K': 'Ķ', 'L': 'Ĺ', 'M': 'Ṁ', 'N': 'Ñ', 'O': 'Ö', 'P': 'Þ', 'Q': 'Ǫ', 'R': 'Ŕ', 'S': 'Š', 'T': 'Ţ',
	'U': 'Ü', 'V': 'Ṽ', 'W': 'Ŵ', 'X': 'Ẋ', 'Y': 'Ý', 'Z': 'Ž',
}

// pseudo is the pseudo-locale of a catalog: every value accented, about 40%
// longer (German runs about 30% longer than English) and in brackets, so that
// a word left out of the catalog stands out by being plain.
func pseudo(en map[string]any) map[string]any {
	out := make(map[string]any, len(en))
	for k, v := range en {
		switch v := v.(type) {
		case string:
			out[k] = pseudoText(v, strings.HasSuffix(k, "_html"))
		case map[string]any:
			forms := make(map[string]any, len(v))
			for f, s := range v {
				if s, ok := s.(string); ok {
					forms[f] = pseudoText(s, false)
				}
			}
			out[k] = forms
		}
	}
	return out
}

// pseudoText accents s, leaving what is between braces (a value's name) and,
// for markup, between angle brackets and in entities, as it is.
func pseudoText(s string, markup bool) string {
	var b strings.Builder
	letters := 0
	var until rune // the end of a part left as it is
	for _, r := range s {
		switch {
		case until != 0:
			if r == until {
				until = 0
			}
		case r == '{':
			until = '}'
		case markup && r == '<':
			until = '>'
		case markup && r == '&':
			until = ';'
		default:
			if a, ok := accented[r]; ok {
				r = a
				letters++
			}
		}
		b.WriteRune(r)
	}
	return "[" + b.String() + " " + strings.Repeat("·", (letters*2+4)/5) + "]"
}
