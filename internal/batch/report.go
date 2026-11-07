package batch

import (
	"encoding/json"
	"math"
	"sort"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Report is what a batch's charts are drawn from: every number the report page
// shows, aggregated over the batch's games.
type Report struct {
	Games   int `json:"games"`
	Decided int `json:"decided"`
	// EndReasons counts games by how they ended.
	EndReasons map[string]int `json:"end_reasons"`
	Plies      Dist           `json:"plies"`
	Rounds     Dist           `json:"rounds"`
	// PlyBuckets is a histogram of game length: Counts[i] games had
	// Lo + i*Width <= plies < Lo + (i+1)*Width.
	PlyBuckets Histogram `json:"ply_buckets"`
	// Seats is what every game has: how each seat of the table fared, which
	// is where the advantage of moving first shows.
	Seats []SeatReport `json:"seats,omitempty"`
	// Bots is the same by bot, whichever seat it played: which bot is
	// stronger, at the settings this batch gave them.
	Bots []BotReport `json:"bots,omitempty"`
	// Game is the game's own part of the report, from its study: top-level
	// fields beside these (Cthulhu Wars' factions and area names).
	Game map[string]json.RawMessage `json:"-"`
}

// MarshalJSON writes the common fields and the game's own beside them.
func (r Report) MarshalJSON() ([]byte, error) {
	type plain Report
	raw, err := json.Marshal(plain(r))
	if err != nil || len(r.Game) == 0 {
		return raw, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, err
	}
	for k, v := range r.Game {
		if _, taken := all[k]; !taken {
			all[k] = v
		}
	}
	return json.Marshal(all)
}

// SeatReport is one seat of the table over a whole batch.
type SeatReport struct {
	Seat  int    `json:"seat"`
	Label string `json:"label,omitempty"`
	Color string `json:"color,omitempty"`
	// Bots lists the bots that sat here (more than one when the batch moved
	// them round the seats).
	Bots  []string `json:"bots,omitempty"`
	Games int      `json:"games"`
	Wins  int      `json:"wins"`
	// Draws counts the games that ended with nobody winning, which belong to
	// every seat at once; Unfinished those that ran out of plies.
	Draws      int `json:"draws"`
	Unfinished int `json:"unfinished"`
}

// BotReport is one bot over a whole batch, whichever seat it played.
type BotReport struct {
	Bot   string `json:"bot"`
	Games int    `json:"games"`
	Wins  int    `json:"wins"`
	Draws int    `json:"draws"`
}

// seatReports counts how each seat of the game fared. The seats come from the
// definition, so they carry their label and colour even where no game was
// played in one.
func seatReports(cat *catalog.Catalog, gameDefID string, recs []Record) []SeatReport {
	game, ok := cat.Get(gameDefID)
	if !ok {
		return nil
	}
	defs := game.Definition.Seats
	out := make([]SeatReport, 0, len(defs))
	at := map[int]int{}
	for i, s := range defs {
		label := s.Label
		if label == "" {
			label = s.Color
		}
		out = append(out, SeatReport{Seat: s.Seat, Label: label, Color: s.Color})
		at[s.Seat] = i
	}
	seen := make([]map[string]bool, len(out))
	for _, r := range recs {
		for seat := 1; seat <= len(out); seat++ {
			i, ok := at[seat]
			if !ok {
				continue
			}
			out[i].Games++
			switch {
			case r.Winner == seat:
				out[i].Wins++
			case r.Winner == 0 && isUnfinished(r.EndReason):
				out[i].Unfinished++
			case r.Winner == 0:
				out[i].Draws++
			}
			if seat <= len(r.Bots) && r.Bots[seat-1] != "" {
				if seen[i] == nil {
					seen[i] = map[string]bool{}
				}
				if !seen[i][r.Bots[seat-1]] {
					seen[i][r.Bots[seat-1]] = true
					out[i].Bots = append(out[i].Bots, r.Bots[seat-1])
				}
			}
		}
	}
	for i := range out {
		sort.Strings(out[i].Bots)
	}
	return out
}

// botReports counts how each bot fared, wherever it sat.
func botReports(recs []Record) []BotReport {
	byBot := map[string]*BotReport{}
	order := []string{}
	for _, r := range recs {
		for seat, bot := range r.Bots {
			if bot == "" {
				continue
			}
			b := byBot[bot]
			if b == nil {
				b = &BotReport{Bot: bot}
				byBot[bot] = b
				order = append(order, bot)
			}
			b.Games++
			if r.Winner == seat+1 {
				b.Wins++
			} else if r.Winner == 0 && !isUnfinished(r.EndReason) {
				b.Draws++
			}
		}
	}
	sort.Strings(order)
	out := make([]BotReport, 0, len(order))
	for _, k := range order {
		out = append(out, *byBot[k])
	}
	return out
}

// isUnfinished reports whether a game ended without being played out.
func isUnfinished(reason string) bool {
	switch reason {
	case "unfinished", "illegal-move", "no-legal-moves":
		return true
	}
	return false
}

// Dist summarises a list of numbers.
type Dist struct {
	Min  float64 `json:"min"`
	Mean float64 `json:"mean"`
	P50  float64 `json:"p50"`
	P90  float64 `json:"p90"`
	Max  float64 `json:"max"`
}

// Histogram is a count per bucket of equal width.
type Histogram struct {
	Lo     int   `json:"lo"`
	Width  int   `json:"width"`
	Counts []int `json:"counts"`
}

// Tally counts games and how many of them were won.
type Tally struct {
	N    int `json:"n"`
	Wins int `json:"wins"`
}

// Lift compares the games in which a seat did more than its average of
// something (built it, used it, held an area) with those in which it did not:
// the win rates tell whether doing it goes with winning. Threshold is the
// average the split was made at.
type Lift struct {
	With      Tally   `json:"with"`
	Without   Tally   `json:"without"`
	Threshold float64 `json:"threshold"`
}

// BuildReport aggregates the records of a batch of game (the batch's base
// game): what every game has, and the game's own part when its family has a
// study.
func BuildReport(game *catalog.CompiledGame, recs []Record) (Report, error) {
	rep := Report{Games: len(recs), EndReasons: map[string]int{}}
	var plies, rounds []float64
	for i := range recs {
		r := &recs[i]
		rep.EndReasons[r.EndReason]++
		if r.Winner != 0 {
			rep.Decided++
		}
		plies = append(plies, float64(r.Plies))
		if r.Rounds > 0 {
			rounds = append(rounds, float64(r.Rounds))
		}
	}
	rep.Plies, rep.Rounds = DistOf(plies), DistOf(rounds)
	rep.PlyBuckets = histogram(plies, 12)
	if study := StudyOf(game); study != nil {
		own, err := study.Report(game, recs)
		if err != nil {
			return rep, err
		}
		rep.Game = own
	}
	return rep, nil
}

// LiftOf splits the games at the mean of vals: the games above it against the
// rest. When every game is the same there is nothing to compare.
func LiftOf(vals []float64, wins []bool) Lift {
	var l Lift
	if len(vals) == 0 {
		return l
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	l.Threshold = sum / float64(len(vals))
	const eps = 1e-9
	for i, v := range vals {
		t := &l.Without
		if v > l.Threshold+eps {
			t = &l.With
		}
		t.N++
		if wins[i] {
			t.Wins++
		}
	}
	return l
}

// Wilson is the 95% Wilson score interval of k successes in n trials.
func Wilson(k, n int) (lo, hi float64) {
	if n == 0 {
		return 0, 0
	}
	const z = 1.96
	p := float64(k) / float64(n)
	nf := float64(n)
	den := 1 + z*z/nf
	centre := (p + z*z/(2*nf)) / den
	half := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf)) / den
	return math.Max(0, centre-half), math.Min(1, centre+half)
}

// DistOf summarises v.
func DistOf(v []float64) Dist {
	if len(v) == 0 {
		return Dist{}
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	sum := 0.0
	for _, x := range s {
		sum += x
	}
	at := func(q float64) float64 { return s[int(q*float64(len(s)-1)+0.5)] }
	return Dist{Min: s[0], Mean: sum / float64(len(s)), P50: at(0.5), P90: at(0.9), Max: s[len(s)-1]}
}

func histogram(v []float64, buckets int) Histogram {
	if len(v) == 0 {
		return Histogram{}
	}
	lo, hi := v[0], v[0]
	for _, x := range v {
		lo, hi = math.Min(lo, x), math.Max(hi, x)
	}
	width := int(math.Ceil((hi - lo + 1) / float64(buckets)))
	if width < 1 {
		width = 1
	}
	h := Histogram{Lo: int(lo), Width: width, Counts: make([]int, buckets)}
	for _, x := range v {
		i := (int(x) - h.Lo) / width
		if i >= buckets {
			i = buckets - 1
		}
		h.Counts[i]++
	}
	return h
}
