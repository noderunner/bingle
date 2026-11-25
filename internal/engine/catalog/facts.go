package catalog

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/noderunner/bingle/internal/badges"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

// Facts are what a game's rules report about what a selection did (ADR 0129).
// A game lists the ones it can report; a badge may count only those.

// checkFacts checks the names and descriptions of a game's facts.
func checkFacts(def *dsl.GameDefinition) error {
	for name, f := range def.Facts {
		if !factName(name) {
			return fmt.Errorf("catalog: fact %q: a fact's name is lower case letters, digits and underscores, starting with a letter", name)
		}
		if badges.IsResult(name) {
			return fmt.Errorf("catalog: fact %q is the platform's count of results, not a game's fact", name)
		}
		if strings.TrimSpace(f.Description) == "" {
			return fmt.Errorf("catalog: fact %q has no description", name)
		}
	}
	return nil
}

// checkBadges checks the game's own badges (ADR 0130) against its facts.
func checkBadges(def *dsl.GameDefinition) error {
	facts := def.Facts
	if facts == nil {
		facts = map[string]dsl.FactDef{}
	}
	if err := badges.CheckAll(def.Badges, facts); err != nil {
		return fmt.Errorf("catalog: %w", err)
	}
	return nil
}

// MaxEndingWords is the most a game's words for one of its endings may run to.
const MaxEndingWords = 120

// checkEndings checks the words of a game's own endings: each reason, or
// reason:detail, a name of lower-case letters, digits, dashes and
// underscores, and each worded in a few words on one line.
func checkEndings(def *dsl.GameDefinition) error {
	for key, words := range def.Endings {
		reason, detail, hasDetail := strings.Cut(key, ":")
		if !endingName(reason) || hasDetail && !endingName(detail) {
			return fmt.Errorf("catalog: endings: %q is not a reason, or a reason and a detail as reason:detail", key)
		}
		if strings.TrimSpace(words) == "" || utf8.RuneCountInString(words) > MaxEndingWords || strings.ContainsAny(words, "\r\n") {
			return fmt.Errorf("catalog: endings: %q needs a few words on one line, at most %d characters", key, MaxEndingWords)
		}
	}
	return nil
}

func endingName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c != '-' && c != '_' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// factDeclared checks that name, when set, is one of the game's facts.
func factDeclared(def *dsl.GameDefinition, name string) error {
	if name == "" {
		return nil
	}
	if _, ok := def.Facts[name]; !ok {
		return fmt.Errorf("fact %q is not in the game's facts", name)
	}
	return nil
}

func factName(s string) bool {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// Fact is the declaration of one of the game's facts.
func (g *CompiledGame) Fact(name string) (dsl.FactDef, bool) {
	f, ok := g.Definition.Facts[name]
	return f, ok
}
