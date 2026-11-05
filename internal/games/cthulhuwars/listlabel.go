package cthulhuwars

import (
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

var _ state.ListLabeler = (*Ruleset)(nil)

// layoutBadges names the boards a table might not expect: the usual Earth board of each number of
// players has no badge.
var layoutBadges = map[string]string{
	"cthulhu-wars-3-5":          "3-5 Map",
	"cthulhu-wars-primeval-3p":  "Primeval Map",
	"cthulhu-wars-primeval-3-5": "Primeval 3-5 Map",
	"cthulhu-wars-primeval-5-3": "Primeval 5-3 Map",
	"cthulhu-wars-primeval-5p":  "Primeval Map",
}

// ListLabel implements state.ListLabeler. The name of a table lists every choice made for it, far too
// much for a list of games. The badges say the little a player choosing a table wants to know.
func (rs *Ruleset) ListLabel(game *catalog.CompiledGame) (string, []string) {
	def := game.Definition
	layout, _, _ := strings.Cut(def.ID, "~")
	listed := func(key string) bool {
		list, _ := def.Params[key].([]any)
		return len(list) > 0
	}
	on := func(key string) bool {
		yes, _ := def.Params[key].(bool)
		return yes
	}
	mapBadge := layoutBadges[layout]
	neutral := listed("neutral_units") || listed("independents")
	priests := on("high_priests")

	core := !strings.Contains(layout, "primeval") && !neutral && !priests && !listed("neutral_books") && !on("azathoth")
	for _, po := range packOptions {
		core = core && !on(po.key)
	}
	for seat := 1; seat <= len(def.Seats); seat++ {
		core = core && coreFaction(keyOfSeat(game, seat))
	}
	pool, _ := def.Params["faction_pool"].([]any)
	for _, f := range pool {
		key, _ := f.(string)
		core = core && coreFaction(key)
	}

	var tags []string
	if core {
		tags = append(tags, "Core only")
	}
	if mapBadge != "" {
		tags = append(tags, mapBadge)
	}
	if neutral {
		tags = append(tags, "Neutral units")
	}
	if priests {
		tags = append(tags, "High Priests")
	}
	return "Cthulhu Wars", tags
}

func coreFaction(key string) bool {
	switch key {
	case "gc", "cc", "bg", "ys":
		return true
	}
	return false
}
