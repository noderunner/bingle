package onitama

import (
	"encoding/json"
	"slices"
)

// Upgrade converts a position stored by the Go ruleset Onitama had until 0.2.0
// to the declared game's: the five cards in play move from the ruleset's data
// into the card table (each hand, the side), the other eleven go in the box,
// and the data goes. The pieces and the ply read the same. A position already
// in the new form, with no ruleset data, is left alone.
func Upgrade(doc []byte) ([]byte, bool) {
	var d map[string]json.RawMessage
	if err := json.Unmarshal(doc, &d); err != nil {
		return doc, false
	}
	raw, ok := d["ext"]
	if !ok {
		return doc, false
	}
	var old struct {
		Hands [2][2]string `json:"hands"`
		Side  string       `json:"side"`
	}
	if err := json.Unmarshal(raw, &old); err != nil || old.Side == "" {
		return doc, false
	}
	type card struct {
		ID string `json:"id"`
		Up bool   `json:"up,omitempty"`
	}
	type zone struct {
		Cards []card `json:"cards,omitempty"`
	}
	up := func(ids ...string) zone {
		z := zone{}
		for _, id := range ids {
			z.Cards = append(z.Cards, card{ID: id, Up: true})
		}
		return z
	}
	inPlay := []string{old.Hands[0][0], old.Hands[0][1], old.Hands[1][0], old.Hands[1][1], old.Side}
	box := zone{}
	for _, c := range moveCards {
		if !slices.Contains(inPlay, c.ID) {
			box.Cards = append(box.Cards, card{ID: c.ID})
		}
	}
	table := map[string]any{"zones": map[string]zone{
		"hand:1": up(old.Hands[0][0], old.Hands[0][1]),
		"hand:2": up(old.Hands[1][0], old.Hands[1][1]),
		"side":   up(old.Side),
		"box":    box,
	}}
	b, err := json.Marshal(table)
	if err != nil {
		return doc, false
	}
	delete(d, "ext")
	d["cards"] = b
	out, err := json.Marshal(d)
	if err != nil {
		return doc, false
	}
	return out, true
}
