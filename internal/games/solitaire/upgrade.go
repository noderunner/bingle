package solitaire

import "encoding/json"

// Upgrade converts a position stored by the Go ruleset that played Klondike,
// Spider and FreeCell until Solitaire 0.3.0 to the declared game's: its table
// of cards becomes the position's own, and the ruleset's data goes. The ply
// reads the same; whether the game is over is worked out from the table. A
// position already in the new form, with no ruleset data, is left alone.
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
		Mode  string          `json:"mode"`
		Table json.RawMessage `json:"table"`
	}
	if err := json.Unmarshal(raw, &old); err != nil || len(old.Table) == 0 || old.Mode == modeBowling {
		return doc, false
	}
	delete(d, "ext")
	d["cards"] = old.Table
	out, err := json.Marshal(d)
	if err != nil {
		return doc, false
	}
	return out, true
}
