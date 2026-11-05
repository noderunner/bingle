package cthulhuwars

import (
	"regexp"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// The setup page's catalog: every cult, neutral unit, Independent and rule a
// table can be set up with, described for the tiles of the picker and the cards
// that hover over them. The page turns what is chosen into the same options any
// other client sends (see Derive), so this is description only.

// SetupCatalog is what GET /game-defs/{id}/setup-catalog returns.
type SetupCatalog struct {
	Tabs    []SetupTab    `json:"tabs"`
	Presets []SetupPreset `json:"presets"`
	// Swaps is the option that swaps a timer spellbook for a neutral one.
	Swaps any `json:"swaps,omitempty"`
}

// SetupTab is one tab of the picker. The page adds the maps, which are the
// layouts of the game, itself.
type SetupTab struct {
	ID    string      `json:"id"`
	Label string      `json:"label"`
	Items []SetupItem `json:"items"`
}

// SetupItem is one tile. Kind (in NeutralItem) is "faction", "rule", "monster",
// "terror", "goo" or "book". Option is the setup option that chooses it: a list
// option when Value is set, otherwise a bool.
type SetupItem struct {
	NeutralItem
	Option string `json:"option"`
	Value  string `json:"value,omitempty"`
	// Group is the heading it stands under: the box it comes from.
	Group string `json:"group,omitempty"`
	// Budget says it counts towards the recommended number of neutral units and
	// Independents, and that the shuffle may pick it.
	Budget bool `json:"budget,omitempty"`
	// Factions: its colour, ability, units and spellbooks.
	Color   string      `json:"color,omitempty"`
	Ability string      `json:"ability,omitempty"`
	Units   []SetupUnit `json:"units,omitempty"`
	Books   []string    `json:"books,omitempty"`
	// Shape names the piece to draw for a rule that is a unit.
	Shape string `json:"shape,omitempty"`
}

// SetupUnit is a line of a cult's summary.
type SetupUnit struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
	Cost   int    `json:"cost"`
	Combat string `json:"combat"`
}

// SetupPreset is a ready-made table anybody may start from.
type SetupPreset struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	GameDefID string         `json:"game_def_id"`
	Options   map[string]any `json:"options"`
}

// setupPresets are the tables the game ships with: they show what the picker
// can do and leave a guest something to choose from.
var setupPresets = []SetupPreset{
	{ID: "core", Name: "The core box", GameDefID: "cthulhu-wars", Options: map[string]any{
		optFactions: []any{"gc", "cc", "bg", "ys"},
	}},
	{ID: "first-neutrals", Name: "A first taste of neutrals", GameDefID: "cthulhu-wars", Options: map[string]any{
		optFactions: []any{"gc", "cc", "bg", "ys"},
		"igoo-yig":  true,
		optUnits:    []any{uGnorri, uVoonith},
		optBooks:    []any{"shrivel", "recrim"},
	}},
	{ID: "five-cults", Name: "Five cults and their High Priests", GameDefID: "cthulhu-wars-5p", Options: map[string]any{
		optFactions:    []any{"gc", "cc", "bg", "ys", "ww"},
		optHighPriests: true,
		"igoo-abhoth":  true,
		"igoo-cthugha": true,
		optUnits:       []any{uShantak, uHound},
	}},
	{ID: "primeval", Name: "Primeval, three cults", GameDefID: "cthulhu-wars-primeval-3p", Options: map[string]any{
		optFactions:  []any{"sl", "ow", "tt"},
		"igoo-dagon": true,
		optUnits:     []any{uMoonbeast},
	}},
}

// packGroups name the boxes the Loyalty Cards come from.
var packGroups = map[int]string{
	packAzathoth:   "Azathoth",
	packDreamlands: "Dreamlands Surface Monsters",
	packBeyond:     "Beyond Time and Space",
	packUnderworld: "Dreamlands Underworld Monsters",
	packCosmic:     "Cosmic Terrors",
	packCats:       "Something About Cats",
	packCampbell:   "Ramsey Campbell Horrors 2",
}

var igooBox = regexp.MustCompile(`\(([^)]*)\)\s*$`)

var (
	setupCatalogMu    sync.Mutex
	setupCatalogValue *SetupCatalog
)

var _ state.SetupCatalogs = (*Ruleset)(nil)

// SetupCatalog implements state.SetupCatalogs. What it says does not depend on
// the layout, so it is worked out once.
func (rs *Ruleset) SetupCatalog(game *catalog.CompiledGame) (any, error) {
	setupCatalogMu.Lock()
	defer setupCatalogMu.Unlock()
	if setupCatalogValue == nil {
		built, err := rs.buildSetupCatalog(game)
		if err != nil {
			return nil, err
		}
		setupCatalogValue = built
	}
	return setupCatalogValue, nil
}

func (rs *Ruleset) buildSetupCatalog(game *catalog.CompiledGame) (*SetupCatalog, error) {
	// Combat is worded by the engine, which wants a position to look at: a new
	// game's says what a unit's dice depend on without counting anything yet.
	st, err := rs.Start(game, 1)
	if err != nil {
		return nil, err
	}
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	e := rs.newEng(game, st, d)

	optionText := map[string]string{}
	optionLabel := map[string]string{}
	factionText := map[string]string{}
	out := &SetupCatalog{Presets: setupPresets}
	for _, o := range game.Definition.Options {
		optionText[o.ID], optionLabel[o.ID] = o.Text, o.Label
		switch o.ID {
		case optFactions:
			for _, c := range o.Choices {
				factionText[c.ID] = c.Text
			}
		case optBookSwaps:
			out.Swaps = o
		}
	}

	var cults []SetupItem
	for _, f := range factions {
		item := SetupItem{
			NeutralItem: NeutralItem{Kind: "faction", ID: f.Key, Name: f.Name, Text: factionText[f.Key]},
			Option:      optFactions, Value: f.Key, Color: f.Color, Ability: f.Ability,
		}
		for _, u := range f.Units {
			item.Units = append(item.Units, SetupUnit{ID: u.ID, Name: u.Name, Count: u.Count, Cost: u.Cost, Combat: e.combatText(0, u)})
		}
		for _, b := range f.Books {
			item.Books = append(item.Books, b.Name)
		}
		cults = append(cults, item)
	}

	var neutrals, indies []SetupItem
	byIgoo := map[string]string{}
	for _, x := range igooKeys {
		byIgoo[x.unit] = x.option
	}
	for _, c := range cards {
		item := SetupItem{NeutralItem: cardItem(c, e.combatText(0, c.Unit)), Budget: true}
		if c.Kind == cardIndependent {
			item.Option = byIgoo[c.Unit.ID]
			if m := igooBox.FindStringSubmatch(optionLabel[item.Option]); m != nil {
				item.Group = m[1]
			}
			indies = append(indies, item)
			continue
		}
		item.Option, item.Value, item.Group = optUnits, c.Unit.ID, packGroups[c.Pack]
		neutrals = append(neutrals, item)
	}

	other := []SetupItem{{
		NeutralItem: NeutralItem{Kind: "rule", ID: optHighPriests, Name: "High Priests", Text: optionText[optHighPriests]},
		Option:      optHighPriests, Group: "High Priests", Shape: uHighPriest,
	}}
	for n, b := range neutralBooks {
		item := SetupItem{NeutralItem: neutralBookItem(b), Option: optBooks, Value: neutralKeys[n], Group: "Azathoth - Neutral Spellbooks"}
		other = append(other, item)
	}

	out.Tabs = []SetupTab{
		{ID: "factions", Label: "Factions", Items: cults},
		{ID: "neutrals", Label: "Monsters & Terrors", Items: neutrals},
		{ID: "independents", Label: "IGOOs", Items: indies},
		{ID: "other", Label: "Other", Items: other},
	}
	return out, nil
}
