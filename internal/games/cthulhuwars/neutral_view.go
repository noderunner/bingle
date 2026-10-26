package cthulhuwars

// NeutralView is the window of what is still there for the taking: the neutral
// spellbooks nobody holds and the Loyalty Cards nobody has bought or awakened.
// It is only drawn for a table that plays with neutral content.
type NeutralView struct {
	Items []NeutralItem `json:"items"`
}

// NeutralItem is one neutral spellbook or unit that can still be taken.
type NeutralItem struct {
	// Kind is "book", "monster", "terror" or "goo".
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
	// Obtain is the price of taking it ("2 Doom", "2 Doom and 2 Power", "4
	// Power"); How is the card's text on what happens when it is taken.
	Obtain string `json:"obtain"`
	How    string `json:"how,omitempty"`
	// Books: when it can be used.
	Timing string `json:"timing,omitempty"`
	Text   string `json:"text,omitempty"`
	// Units: the Power one costs to summon or awaken, its Combat, how many
	// figures come with the card and what it can do.
	Cost      int           `json:"cost,omitempty"`
	Combat    string        `json:"combat,omitempty"`
	Total     int           `json:"total,omitempty"`
	Enter     string        `json:"enter,omitempty"`
	Abilities []AbilityView `json:"abilities,omitempty"`
	// Independent Great Old Ones: the steps of awakening, the spellbook on the
	// card and the requirement that earns it.
	Awaken []string      `json:"awaken,omitempty"`
	Book   *UnitBookView `json:"book,omitempty"`
	Req    string        `json:"req,omitempty"`
}

// neutralInPlay says the table plays with neutral spellbooks or Loyalty Cards.
func (e *eng) neutralInPlay() bool { return e.m.cfg.az || e.m.cfg.packs != 0 }

// neutralView lists what is left to take, nil when no neutral content is in
// play at this table.
func (e *eng) neutralView() *NeutralView {
	if !e.neutralInPlay() {
		return nil
	}
	nv := &NeutralView{Items: []NeutralItem{}}
	if e.m.cfg.az {
		for n, b := range neutralBooks {
			if e.neutralHeld(n) || e.setAside(n) {
				continue
			}
			nv.Items = append(nv.Items, NeutralItem{
				Kind: "book", ID: b.ID, Name: b.Name, Timing: b.Timing, Text: b.Text,
				Obtain: "A spellbook slot",
				How:    "Take it in place of one of your own spellbooks when a requirement of your faction earns you one.",
			})
		}
	}
	for i, c := range cards {
		if !e.cardInGame(i) || e.cardOwner(i) != 0 {
			continue
		}
		u := c.Unit
		item := NeutralItem{
			ID: u.ID, Name: u.Name, How: c.Buy, Obtain: cardCostText(c), Cost: u.Cost,
			Combat: e.combatText(0, u), Total: u.Count, Enter: enterText(u.ID), Text: u.Text,
		}
		for _, a := range u.Abilities {
			item.Abilities = append(item.Abilities, AbilityView(a))
		}
		switch c.Kind {
		case cardTerror:
			item.Kind = "terror"
		case cardIndependent:
			item.Kind = "goo"
			item.Awaken = c.Awaken
			item.Enter = "An Independent Great Old One: awakened in the Action phase, never summoned."
			item.Book = &UnitBookView{ID: c.Book.ID, Name: c.Book.Name, Timing: c.Book.Timing, Text: c.Book.Text}
			item.Req = c.Req
		default:
			item.Kind = "monster"
		}
		nv.Items = append(nv.Items, item)
	}
	return nv
}

// setAside says the neutral spellbook n was put aside at setup for some cult.
func (e *eng) setAside(n int) bool { return e.reservedElsewhere(0, n) }
