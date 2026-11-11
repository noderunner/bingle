package session

import (
	"context"
)

// GameDefSummary describes one registered game definition, matching the REST
// table's GET /api/game-defs response shape.
type GameDefSummary struct {
	ID            string
	Name          string
	SeatsRequired int
	Description   string
	BoardWidth    int
	BoardHeight   int
	// Family groups variants of one game (equal to ID for stand-alone games).
	Family string
	// Version, Designers and Publisher describe the game implementation (see dsl.GameDefinition).
	Version   string
	Designers []string
	Publisher string
	// Prototype marks a game whose rules are not final: each version of them is
	// a game of the family, and Version says which.
	Prototype bool
	// Tutorials is how many tutorials a player of this game is offered.
	Tutorials int
}

// ListGameDefs returns a summary of every game definition registered in m's
// catalog, sorted (via catalog.List) by ID for determinism. No error is
// possible.
func (m *Manager) ListGameDefs(ctx context.Context) []GameDefSummary {
	compiled := m.catalog.List()
	summaries := make([]GameDefSummary, len(compiled))
	for i, cg := range compiled {
		summaries[i] = GameDefSummary{
			ID:            cg.Definition.ID,
			Name:          cg.Definition.Name,
			SeatsRequired: len(cg.Definition.Seats),
			Description:   cg.Definition.Description,
			BoardWidth:    cg.Board.Width(),
			BoardHeight:   cg.Board.Height(),
			Family:        familyOf(cg.Definition.Family, cg.Definition.ID),
			Version:       cg.Definition.Version,
			Designers:     cg.Definition.Designers,
			Publisher:     cg.Definition.Publisher,
			Prototype:     cg.Definition.Prototype,
		}
		if _, list, err := m.TutorialsFor(cg.Definition.ID); err == nil {
			summaries[i].Tutorials = len(list)
		}
	}
	return summaries
}

func familyOf(family, id string) string {
	if family != "" {
		return family
	}
	return id
}
