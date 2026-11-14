package catalog

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/dsl"
)

// Facts are what a game's rules report about what a selection did (ADR 0129).
// A game lists the ones it can report; a badge may count only those.

// ReservedFacts are the platform's counts of results, which no game may
// declare as a fact of its own (ADR 0130).
var ReservedFacts = []string{"played", "won", "drawn", "lost"}

// checkFacts checks the names and descriptions of a game's facts.
func checkFacts(def *dsl.GameDefinition) error {
	for name, f := range def.Facts {
		if !factName(name) {
			return fmt.Errorf("catalog: fact %q: a fact's name is lower case letters, digits and underscores, starting with a letter", name)
		}
		for _, r := range ReservedFacts {
			if name == r {
				return fmt.Errorf("catalog: fact %q is the platform's count of results, not a game's fact", name)
			}
		}
		if strings.TrimSpace(f.Description) == "" {
			return fmt.Errorf("catalog: fact %q has no description", name)
		}
	}
	return nil
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
