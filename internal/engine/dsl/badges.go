package dsl

import (
	"encoding/json"
	"fmt"
)

// BadgeDef is one badge (ADR 0130): something a player would mention,
// awarded once, and built from a fixed set of parts that must all hold. A
// game's badges are in its definition; the platform's in one file of their
// own. Its icon is each client's.
type BadgeDef struct {
	// ID names the badge: lower case letters, digits and dashes. A game's
	// badge is known as family.id.
	ID string `json:"id"`
	// Name and Description are what a player reads.
	Name        string `json:"name"`
	Description string `json:"description"`
	// When is "during" (awarded as soon as it holds, from what happens in
	// play) or "end" (checked once the game has finished).
	When string `json:"when"`
	// Count counts a fact about the earning seat.
	Count *BadgeCount `json:"count,omitempty"`
	// Distinct counts different opponents or families played, across games.
	Distinct *BadgeDistinct `json:"distinct,omitempty"`
	// Won: the earning seat won the game.
	Won bool `json:"won,omitempty"`
	// Rated: only rated games whose result was written.
	Rated bool `json:"rated,omitempty"`
	// Bot makes a bot badge: a game of one account against bots.
	Bot *BadgeBot `json:"bot,omitempty"`
	// Seat limits the badge to a seat's faction or side.
	Seat *BadgeSeat `json:"seat,omitempty"`
	// Game limits the badge to a family (a platform badge) or a variant.
	Game *BadgeGame `json:"game,omitempty"`
}

// BadgeCount is a fact about the earning seat, at least AtLeast times, Within
// the game, a window between two facts, or across games.
type BadgeCount struct {
	// Fact is one of the game's facts, or played, won, drawn or lost (the
	// platform's results, counted across games).
	Fact    string `json:"fact"`
	AtLeast int    `json:"at_least"`
	// Within is "game" (the default), "games", or [open, close]: two facts
	// between which the count starts afresh each time.
	Within Within `json:"within,omitempty"`
}

// BadgeDistinct is how many different opponents or families an account has
// played, in games that count.
type BadgeDistinct struct {
	// Of is "opponents" or "families".
	Of      string `json:"of"`
	AtLeast int    `json:"at_least"`
}

// BadgeBot makes a badge one for beating bots: AtLeast names the weakest bot
// that will do, in the server's list of bots (weakest first).
type BadgeBot struct {
	AtLeast string `json:"at_least"`
}

// BadgeSeat limits a badge to the earning seat's faction, or its side (the
// seat as the definition numbers it).
type BadgeSeat struct {
	Faction string `json:"faction,omitempty"`
	Side    int    `json:"side,omitempty"`
}

// BadgeGame limits a badge to one family (a platform badge) or one game of a
// family (a variant, by its id).
type BadgeGame struct {
	Family  string `json:"family,omitempty"`
	Variant string `json:"variant,omitempty"`
}

// Within is where a badge's count is taken: "game", "games", or two facts
// that open and close a window.
type Within []string

// The places a count is taken.
const (
	WithinGame  = "game"
	WithinGames = "games"
)

// UnmarshalJSON accepts "game", "games" or a list of two fact names.
func (w *Within) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*w = Within{s}
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("dsl: within is game, games or a list of two facts")
	}
	*w = list
	return nil
}

// MarshalJSON mirrors UnmarshalJSON.
func (w Within) MarshalJSON() ([]byte, error) {
	if len(w) == 1 {
		return json.Marshal(w[0])
	}
	return json.Marshal([]string(w))
}

// Window is the facts that open and close a window, if Within is one.
func (w Within) Window() (open, close string, ok bool) {
	if len(w) == 2 {
		return w[0], w[1], true
	}
	return "", "", false
}

// Place is "game", "games" or "window".
func (w Within) Place() string {
	switch {
	case len(w) == 0:
		return WithinGame
	case len(w) == 2:
		return "window"
	}
	return w[0]
}

// PlatformBadges is the file of the platform's badges.
type PlatformBadges struct {
	Badges []BadgeDef `json:"badges"`
}
