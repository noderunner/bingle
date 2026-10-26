package nyet

import "strconv"

// The deck has four colours and the players see four classic suits. The
// engine, the saved games and the notation keep the colours' own names (red-9,
// nyet:trump:blue), so that nothing stored changes; everything a player reads
// says the suit instead. The pairing is fixed, and cards.suits of nyet.json
// gives each suit its symbol and colour to match.
var suitOfColour = map[string]string{"red": "hearts", "blue": "spades", "yellow": "diamonds", "green": "clubs"}

// suitName is the suit a colour of the deck is shown as ("red" → "hearts"); a
// name that is no colour (such as "none") comes back as it is.
func suitName(colour string) string {
	if s, ok := suitOfColour[colour]; ok {
		return s
	}
	return colour
}

// characters are the five players of Nyet!, by seat: the animal each plays and
// the colour of its seat. The boxes of the first-player line show all five,
// whoever is at the table.
var characters = [...]struct{ name, colour string }{
	1: {"wolf", "red"}, 2: {"crane", "blue"}, 3: {"tiger", "green"}, 4: {"bear", "yellow"}, 5: {"eagle", "orange"},
}

// characterIcon is the icon descriptor of a seat's animal ("animal:wolf:red"),
// or "" for a seat that is not one of the five.
func characterIcon(seat int) string {
	if seat < 1 || seat >= len(characters) {
		return ""
	}
	c := characters[seat]
	return "animal:" + c.name + ":" + c.colour
}

// boxIcon is the icon descriptor of a box of the board; the page draws it.
func boxIcon(line, option string) string {
	switch line {
	case lineFirst:
		s, _ := strconv.Atoi(option)
		return characterIcon(s)
	case lineDiscard:
		return "discard:" + option
	case lineTrump:
		return "suit:" + option
	case lineSuper:
		return "super:" + option
	case linePoints:
		return "points:" + option
	}
	return ""
}
