package blooms

import "encoding/json"

// Upgrade converts a position stored by the Go ruleset Blooms had until 0.6.0
// to the declared game's: the captures move from the ruleset's data to each
// seat's "captured" pool, a turn halfway through becomes the turn in progress,
// and the data goes. Everything else (the stones, the ply) reads the same. A
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
		Stage    string `json:"stage"`
		First    string `json:"first"`
		Captured [2]int `json:"captured"`
	}
	if err := json.Unmarshal(raw, &old); err != nil {
		return doc, false
	}
	delete(d, "ext")
	reserves := map[string]map[string]int{"red": {"captured": old.Captured[0]}, "blue": {"captured": old.Captured[1]}}
	if d["reserves"], ok = marshal(reserves); !ok {
		return doc, false
	}
	if old.Stage == "second" && old.First != "" {
		if d["turn"], ok = marshal(map[string]any{"part": 0, "made": 1, "pieces": []string{old.First}}); !ok {
			return doc, false
		}
	}
	out, err := json.Marshal(d)
	if err != nil {
		return doc, false
	}
	return out, true
}

func marshal(v any) (json.RawMessage, bool) {
	b, err := json.Marshal(v)
	return b, err == nil
}
