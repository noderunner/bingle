package council

import (
	"encoding/json"
	"sort"

	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// The fixture's practice study (batch.Study), so the public tests have a game
// that brings one: a house per seat, the claims counted, a report per house,
// and one knob on its bots' guide. It is registered when the package loads,
// which only the tests do.
func init() { batch.RegisterStudy(ID, Study{}) }

// Study is the fixture's batch.Study.
type Study struct{}

var _ batch.Study = Study{}

// Table seats the houses picked (with the option "court"), as a derived game.
func (Study) Table(base string, picks []string, options []string) (string, error) {
	opts := map[string]any{}
	named := false
	for _, p := range picks {
		named = named || p != ""
	}
	if named {
		opts[optHouses] = anyList(picks)
	}
	for _, o := range options {
		if o != optCourt {
			return "", errUnknownOption(o)
		}
		opts[optCourt] = true
	}
	if len(opts) == 0 {
		return base, nil
	}
	if !named {
		opts[optHouses] = []any{"crow", "stag", "wolf", "ram"}
	}
	def := baseDefinition()
	suffix, _, err := New().Derive(def, opts)
	if err != nil {
		return "", err
	}
	return base + catalog.DerivedSep + suffix, nil
}

type errUnknownOption string

func (e errUnknownOption) Error() string { return "council has no option " + string(e) }

// Metrics are what the observer counts in a game: the claims of each seat
// (index seat-1) and of each house by key.
type Metrics struct {
	Claims []int          `json:"claims"`
	Houses map[string]int `json:"houses"`
	Winner int            `json:"winner"`
}

type observer struct{ game *catalog.CompiledGame }

// Observer counts the claims at the end of the game.
func (Study) Observer(game *catalog.CompiledGame) batch.Observer { return observer{game} }

func (o observer) Observe(before, after *state.BoardState, seat int, mv state.Selection) {}

func (o observer) Finish(final *state.BoardState, winner int, reason string) (json.RawMessage, int, int) {
	m := Metrics{Houses: map[string]int{}, Winner: winner}
	d, err := dataOf(final)
	seats, _ := table(o.game)
	for s := 1; s <= len(seats); s++ {
		n := 0
		if err == nil {
			n = len(d.Claims[s])
		}
		m.Claims = append(m.Claims, n)
		m.Houses[seats[s-1]] = n
	}
	raw, _ := json.Marshal(m)
	return raw, final.Ply(), 0
}

// HouseReport is how one house fared over a batch.
type HouseReport struct {
	Key       string  `json:"key"`
	Games     int     `json:"games"`
	Wins      int     `json:"wins"`
	AvgClaims float64 `json:"avg_claims"`
}

// Report is a report per house ("houses").
func (Study) Report(game *catalog.CompiledGame, recs []batch.Record) (map[string]json.RawMessage, error) {
	by := map[string]*HouseReport{}
	for _, r := range recs {
		var m Metrics
		if json.Unmarshal(r.Metrics, &m) != nil {
			continue
		}
		for i, key := range r.Seating {
			if key == "" {
				continue
			}
			h := by[key]
			if h == nil {
				h = &HouseReport{Key: key}
				by[key] = h
			}
			h.Games++
			if m.Winner == i+1 {
				h.Wins++
			}
			h.AvgClaims += float64(m.Houses[key])
		}
	}
	out := []HouseReport{}
	for _, h := range by {
		if h.Games > 0 {
			h.AvgClaims /= float64(h.Games)
		}
		out = append(out, *h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return map[string]json.RawMessage{"houses": raw}, nil
}

// Knobs is one: how much the guide prefers claiming to moving.
func (Study) Knobs() []batch.Knob { return []batch.Knob{{Name: "claim_first", Min: -1, Max: 1}} }

// Shipped is no shift for every house.
func (Study) Shipped(pick string) batch.Tuning { return batch.Tuning{} }

// Guide is the claims first when claim_first is above zero, the moves first
// when it is below, and every move otherwise.
func (Study) Guide(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Selection, t batch.Tuning) []state.Selection {
	w := t["claim_first"]
	if w == 0 {
		return legal
	}
	var want []state.Selection
	for _, mv := range legal {
		if (mv.Action == actClaim) == (w > 0) {
			want = append(want, mv)
		}
	}
	if len(want) == 0 {
		return legal
	}
	return want
}

// Measures are the claims a house made, out of three.
func (Study) Measures() []batch.Measure {
	return []batch.Measure{{Name: "claims", Scale: 3, Weight: 0.2}}
}

// Score is how the house pick fared in one game.
func (Study) Score(rec batch.Record, pick string) (bool, map[string]float64, bool) {
	var m Metrics
	if json.Unmarshal(rec.Metrics, &m) != nil {
		return false, nil, false
	}
	for i, key := range rec.Seating {
		if key == pick {
			return m.Winner == i+1, map[string]float64{"claims": float64(m.Houses[key])}, true
		}
	}
	return false, nil, false
}
