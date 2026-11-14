package state

import "github.com/noderunner/bingle/internal/engine/catalog"

// Fact is one thing a selection did that the rules report (ADR 0129): a name
// from the game's facts and the seat it is about, which need not be the seat
// that moved. Seat 0 is the game rather than a seat (a battle begins).
type Fact struct {
	Name string `json:"name"`
	Seat int    `json:"seat"`
}

// DeclaredFacts splits facts into those game declares and the names of those
// it does not. The rules should report only declared facts; a test plays
// every game to hold them to it, and the session drops the rest.
func DeclaredFacts(game *catalog.CompiledGame, facts []Fact) (declared []Fact, undeclared []string) {
	for _, f := range facts {
		if _, ok := game.Fact(f.Name); ok {
			declared = append(declared, f)
		} else {
			undeclared = append(undeclared, f.Name)
		}
	}
	return declared, undeclared
}
