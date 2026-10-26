package dsl

// CardsDef describes the cards of a card game: the suits (or any other
// grouping of cards), the decks built from them, what a card's back looks
// like, and the zones cards live in (hands, piles, the table). It is data
// only; internal/engine/catalog expands the decks into card types and
// internal/engine/cards deals, shuffles and moves the cards of a game in
// progress.
//
// A deck is either a template (every suit × every rank, each rank with its
// own count, text and symbols) or an explicit list of cards, or both:
//
//	"cards": {
//	  "suits": { "red": { "label": "Red", "color": "#c0392b", "symbol": "♥" } },
//	  "decks": { "main": {
//	      "ranks": [ { "id": "1", "value": 1, "count": 3 }, { "id": "2", "value": 2 } ],
//	      "extra": [ { "id": "joker", "label": "Joker", "text": "Wild", "value": 0, "count": 2 } ]
//	  } },
//	  "zones": [ { "id": "hand", "scope": "seat", "visibility": "owner", "layout": "fan" } ]
//	}
type CardsDef struct {
	// Suits are the card groupings (suits, colours, factions), by id.
	Suits map[string]SuitDef `json:"suits,omitempty"`
	// SuitOrder fixes the order suits are listed in (default: sorted by id).
	SuitOrder []string `json:"suit_order,omitempty"`
	// Decks are the decks the game uses, by name.
	Decks map[string]DeckDef `json:"decks"`
	// Back says how the back of a card looks.
	Back CardBackDef `json:"back,omitempty"`
	// Zones are the places cards can be: each seat's hand, the table, piles.
	Zones []ZoneDef `json:"zones,omitempty"`
}

// SuitDef is one suit: how it is named and drawn.
type SuitDef struct {
	Label string `json:"label,omitempty"`
	// Color is a CSS colour for the suit's pips and card faces.
	Color string `json:"color,omitempty"`
	// Symbol is the text drawn as the suit's pip (a glyph or a short word).
	Symbol string `json:"symbol,omitempty"`
	// Text is a longer description (shown in the rules and on hover).
	Text string `json:"text,omitempty"`
}

// DeckDef is a deck: a template of suits × ranks plus explicit cards.
type DeckDef struct {
	Label string `json:"label,omitempty"`
	// Suits lists the suits a template deck uses (default: all of them).
	Suits []string `json:"suits,omitempty"`
	// Ranks are the ranks of a template deck: every listed suit has each.
	Ranks []RankDef `json:"ranks,omitempty"`
	// Extra are explicit cards added to the deck (jokers, specials, or the
	// whole deck when there are no ranks).
	Extra []CardDef `json:"extra,omitempty"`
	// Remove lists card ids (suit-rank, or an extra card's id) left out of the
	// deck; a game variant can use it to strip cards.
	Remove []string `json:"remove,omitempty"`
}

// RankDef is a rank of a template deck.
type RankDef struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	// Value is the rank's number (compared by the rules).
	Value int `json:"value"`
	// Count is how many copies each suit has (default 1).
	Count int `json:"count,omitempty"`
	// Text is the card's rule text (drawn on the card).
	Text string `json:"text,omitempty"`
	// Symbols are extra marks drawn on the card.
	Symbols []string `json:"symbols,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// CardDef is an explicit card.
type CardDef struct {
	ID      string   `json:"id"`
	Label   string   `json:"label,omitempty"`
	Suit    string   `json:"suit,omitempty"`
	Value   int      `json:"value,omitempty"`
	Count   int      `json:"count,omitempty"`
	Text    string   `json:"text,omitempty"`
	Symbols []string `json:"symbols,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// CardBackDef is the back of a card.
type CardBackDef struct {
	Color string `json:"color,omitempty"`
	Text  string `json:"text,omitempty"`
}

// ZoneDef is a place cards can be.
type ZoneDef struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	// Scope is "seat" (every seat has its own copy: a hand, a trick pile) or
	// "table" (one shared by everyone: the deck, the play area).
	Scope string `json:"scope"`
	// Visibility says who sees the faces: "owner" (only the seat the zone
	// belongs to), "all" (everyone; face-down cards still show a back),
	// "flag" (a card is seen by everyone when it is face up and by no one
	// when face down), "none" (no one: a deck, discarded cards).
	Visibility string `json:"visibility,omitempty"`
	// Layout is how clients draw it: "fan" (a hand, overlapping, all faces),
	// "row" (side by side), "stack" (a pile showing its top card and count),
	// "splay" (a cascade, each card partly covering the last) or "grid".
	Layout string `json:"layout,omitempty"`
	// Hidden zones are not shown at all (only their counts, in the seat's
	// panel): useful for a deck.
	Counts bool `json:"counts,omitempty"`
}
