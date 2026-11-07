package cthulhuwars

// The study of Cthulhu Wars' practice batches (batch.Study): a cult per seat,
// the Observer's measurements (metrics.go), a report per cult, and the guide's
// knobs (tuning.go) for internal/tune. It is registered from
// internal/games/all, with the game.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Study is Cthulhu Wars' batch.Study.
type Study struct{}

var _ batch.Study = Study{}

// Table seats the cults: a table that names one for every seat is a game of
// its own, derived from base (base~gc-cc-bg-ys, with -hp for the High
// Priests); one that names none plays base as it is.
func (Study) Table(base string, picks []string, options []string) (string, error) {
	hp := false
	for _, o := range options {
		if o != "hp" {
			return "", fmt.Errorf("cthulhu wars has no option %q (only hp, the High Priests)", o)
		}
		hp = true
	}
	named := 0
	seen := map[string]bool{}
	for _, p := range picks {
		if p == "" {
			continue
		}
		named++
		if factionIndex(p) < 0 {
			return "", fmt.Errorf("unknown cult %q", p)
		}
		if seen[p] {
			return "", fmt.Errorf("cult %q sits twice", p)
		}
		seen[p] = true
	}
	if named == 0 {
		if hp {
			return "", fmt.Errorf("the High Priests need a cult named for every seat")
		}
		return base, nil
	}
	suffix := strings.Join(picks, "-")
	if hp {
		suffix += "-hp"
	}
	return base + catalog.DerivedSep + suffix, nil
}

// Observer is the batch.Observer of one game.
func (Study) Observer(game *catalog.CompiledGame) batch.Observer {
	return studyObserver{NewObserver(game)}
}

type studyObserver struct{ o *Observer }

func (s studyObserver) Observe(before, after *state.BoardState, seat int, mv state.Selection) {
	s.o.Observe(before, after, seat, mv)
}

func (s studyObserver) Finish(final *state.BoardState, winner int, reason string) (json.RawMessage, int, int) {
	m := s.o.Finish(final, winner, reason)
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, m.Plies, m.Rounds
	}
	return raw, m.Plies, m.Rounds
}

// metricsOf reads a record's measurements; a record without any has none.
func metricsOf(rec batch.Record) (GameMetrics, bool) {
	var m GameMetrics
	if len(rec.Metrics) == 0 || json.Unmarshal(rec.Metrics, &m) != nil {
		return m, false
	}
	return m, true
}

// Report is the batch's report per cult ("factions", in line-up order) and the
// names of the map's areas ("area_names").
func (Study) Report(game *catalog.CompiledGame, recs []batch.Record) (map[string]json.RawMessage, error) {
	order := []string{}
	byFac := map[string][]seatGame{}
	for i := range recs {
		m, ok := metricsOf(recs[i])
		if !ok {
			continue
		}
		mp := &m
		for _, sm := range m.Seats {
			if _, ok := byFac[sm.Faction]; !ok {
				order = append(order, sm.Faction)
			}
			byFac[sm.Faction] = append(byFac[sm.Faction], seatGame{seat: sm, won: sm.Won, games: mp})
		}
	}
	sort.Strings(order)
	factions := []FactionReport{}
	for _, key := range order {
		factions = append(factions, factionReport(key, byFac[key]))
	}
	names := map[string]string{}
	if game != nil {
		for _, p := range game.Definition.Board.Positions {
			names[p.ID] = p.Name
		}
	}
	out := map[string]json.RawMessage{}
	for k, v := range map[string]any{"factions": factions, "area_names": names} {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		out[k] = raw
	}
	return out, nil
}

// Knobs are GuideTuning's fields, by their JSON names, with their search
// ranges.
func (Study) Knobs() []batch.Knob {
	out := make([]batch.Knob, len(GuideKnobs))
	for i, k := range GuideKnobs {
		out[i] = batch.Knob{Name: k.Name, Min: k.Min, Max: k.Max}
	}
	return out
}

// Shipped is a cult's tuning from tuned.json.
func (Study) Shipped(pick string) batch.Tuning { return tuningMap(ShippedTuning(pick)) }

// Guide is the guide's moves under tuning t.
func (Study) Guide(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Selection, t batch.Tuning) []state.Selection {
	rules, _ := state.RulesetOf(game)
	rs, ok := rules.(*Ruleset)
	if !ok {
		return nil
	}
	g := guideTuning(t)
	return rs.GuideMovesTuned(game, st, seat, legal, &g)
}

// Measures are the Doom a cult reached (out of 30) and the spellbooks it held
// (out of 6): a win is a noisy signal, and a cult that cannot win yet needs
// something else to tell a search which way to go.
func (Study) Measures() []batch.Measure {
	return []batch.Measure{{Name: "doom", Scale: 30, Weight: 0.25}, {Name: "books", Scale: 6, Weight: 0}}
}

// Score is how the cult pick fared in one game.
func (Study) Score(rec batch.Record, pick string) (bool, map[string]float64, bool) {
	m, ok := metricsOf(rec)
	if !ok {
		return false, nil, false
	}
	for _, s := range m.Seats {
		if s.Faction == pick {
			return s.Won, map[string]float64{"doom": float64(s.Doom), "books": float64(s.BooksTotal)}, true
		}
	}
	return false, nil, false
}

// tuningMap is t by knob name, leaving out the knobs it does not shift.
func tuningMap(t GuideTuning) batch.Tuning {
	out := batch.Tuning{}
	for _, k := range GuideKnobs {
		if v := *k.Ptr(&t); v != 0 {
			out[k.Name] = v
		}
	}
	return out
}

// guideTuning is the GuideTuning that m names.
func guideTuning(m batch.Tuning) GuideTuning {
	var t GuideTuning
	for _, k := range GuideKnobs {
		*k.Ptr(&t) = m[k.Name]
	}
	return t
}

// seatGame is one cult's part of one game.
type seatGame struct {
	seat  SeatMetrics
	won   bool
	games *GameMetrics
}

// FactionReport is everything about one cult.
type FactionReport struct {
	Key   string  `json:"key"`
	Games int     `json:"games"`
	Wins  int     `json:"wins"`
	Rate  float64 `json:"rate"`
	// Lo and Hi are the 95% Wilson interval of the win rate.
	Lo float64 `json:"lo"`
	Hi float64 `json:"hi"`
	// BySeat is the cult's record by the seat it sat in (index 0 = seat 1).
	BySeat []batch.Tally `json:"by_seat"`

	AvgDoom  float64 `json:"avg_doom"`
	AvgPower float64 `json:"avg_power"`
	AvgGates float64 `json:"avg_gates"`
	AvgSigns float64 `json:"avg_signs"`
	AvgBooks float64 `json:"avg_books"`
	// DoomFrom is the average final Doom by source.
	DoomFrom map[string]float64 `json:"doom_from"`

	// Rounds is the average standing at the end of round 1, 2, 3 …; N the games
	// that reached it.
	Rounds []RoundAvg `json:"rounds"`
	Books  []BookAvg  `json:"books"`
	// Reqs are the spellbook requirements, met or not: the bottleneck view.
	Reqs  []ReqAvg  `json:"reqs"`
	Units []UnitAvg `json:"units"`
	// Actions are the average uses of each action per game, with the lift.
	Actions []ActionAvg `json:"actions"`
	Areas   []AreaAvg   `json:"areas"`

	Battles BattleAvg `json:"battles"`
}

// RoundAvg is an average standing at the end of a round.
type RoundAvg struct {
	Round int     `json:"round"`
	N     int     `json:"n"`
	Doom  float64 `json:"doom"`
	Power float64 `json:"power"`
	Gates float64 `json:"gates"`
	Units float64 `json:"units"`
}

// BookAvg is when (and whether) a spellbook is earned.
type BookAvg struct {
	Book  string     `json:"book"`
	Share float64    `json:"share"` // of games in which it was earned
	Round float64    `json:"round"` // average round it was earned in
	Lift  batch.Lift `json:"lift"`
}

// ReqAvg is when (and whether) a spellbook requirement is met.
type ReqAvg struct {
	Req   string     `json:"req"`
	Text  string     `json:"text"`
	Share float64    `json:"share"` // of games in which it was met
	Round float64    `json:"round"` // average round it was met in
	Lift  batch.Lift `json:"lift"`
}

// UnitAvg is how many units of a type the cult built.
type UnitAvg struct {
	Type  string  `json:"type"`
	Built float64 `json:"built"`
	Lost  float64 `json:"lost"`
	// WonBuilt and LostBuilt are the averages in the games it won and lost.
	WonBuilt  float64    `json:"won_built"`
	LostBuilt float64    `json:"lost_built"`
	Lift      batch.Lift `json:"lift"`
}

// ActionAvg is how often the cult took an action.
type ActionAvg struct {
	Action string     `json:"action"`
	Uses   float64    `json:"uses"`
	Share  float64    `json:"share"` // of games in which it was used at least once
	Lift   batch.Lift `json:"lift"`
}

// AreaAvg is how much of the game the cult held an area.
type AreaAvg struct {
	Area     string     `json:"area"`
	Gate     float64    `json:"gate"`     // share of samples with the cult's Gate there
	Presence float64    `json:"presence"` // share with a unit there
	Control  float64    `json:"control"`  // share holding it
	Lift     batch.Lift `json:"lift"`     // on the control share
}

// BattleAvg are the average battles per game.
type BattleAvg struct {
	Attacked float64 `json:"attacked"`
	Defended float64 `json:"defended"`
	Won      float64 `json:"won"`
	Lost     float64 `json:"lost"`
	Kills    float64 `json:"kills_dealt"`
	Taken    float64 `json:"kills_taken"`
}

func factionReport(key string, gs []seatGame) FactionReport {
	fr := FactionReport{Key: key, Games: len(gs), DoomFrom: map[string]float64{}}
	n := float64(len(gs))
	maxSeat := 0
	for _, g := range gs {
		if g.seat.Seat > maxSeat {
			maxSeat = g.seat.Seat
		}
	}
	fr.BySeat = make([]batch.Tally, maxSeat)
	wins := make([]bool, len(gs))
	for i, g := range gs {
		wins[i] = g.won
		if g.won {
			fr.Wins++
		}
		fr.BySeat[g.seat.Seat-1].N++
		if g.won {
			fr.BySeat[g.seat.Seat-1].Wins++
		}
		s := g.seat
		fr.AvgDoom += float64(s.Doom) / n
		fr.AvgPower += float64(s.Power) / n
		fr.AvgGates += float64(s.Gates) / n
		fr.AvgSigns += float64(s.Signs) / n
		fr.AvgBooks += float64(s.BooksTotal) / n
		fr.DoomFrom["gates"] += float64(s.DoomFrom.Gates) / n
		fr.DoomFrom["rituals"] += float64(s.DoomFrom.Rituals) / n
		fr.DoomFrom["gifts"] += float64(s.DoomFrom.Gifts) / n
		fr.DoomFrom["signs"] += float64(s.DoomFrom.Signs) / n
		b := s.Battles
		fr.Battles.Attacked += float64(b.Attacked) / n
		fr.Battles.Defended += float64(b.Defended) / n
		fr.Battles.Won += float64(b.Won) / n
		fr.Battles.Lost += float64(b.Lost) / n
		fr.Battles.Kills += float64(b.KillsDealt) / n
		fr.Battles.Taken += float64(b.KillsTaken) / n
	}
	fr.Rate = float64(fr.Wins) / n
	fr.Lo, fr.Hi = batch.Wilson(fr.Wins, len(gs))

	// Standing by round.
	maxRound := 0
	for _, g := range gs {
		for _, r := range g.seat.Rounds {
			if r.Round > maxRound {
				maxRound = r.Round
			}
		}
	}
	for rd := 1; rd <= maxRound; rd++ {
		ra := RoundAvg{Round: rd}
		for _, g := range gs {
			for _, r := range g.seat.Rounds {
				if r.Round == rd {
					ra.N++
					ra.Doom += float64(r.Doom)
					ra.Power += float64(r.Power)
					ra.Gates += float64(r.Gates)
					ra.Units += float64(r.Units)
					break
				}
			}
		}
		if ra.N == 0 {
			continue
		}
		f := float64(ra.N)
		ra.Doom, ra.Power, ra.Gates, ra.Units = ra.Doom/f, ra.Power/f, ra.Gates/f, ra.Units/f
		fr.Rounds = append(fr.Rounds, ra)
	}

	// Spellbooks.
	bookSet := map[string]bool{}
	for _, g := range gs {
		for _, b := range g.seat.Books {
			bookSet[b.Book] = true
		}
	}
	for _, id := range sortedKeys(bookSet) {
		ba := BookAvg{Book: id}
		var vals []float64
		cnt := 0
		for _, g := range gs {
			earned := 0.0
			for _, b := range g.seat.Books {
				if b.Book == id {
					earned = 1
					ba.Round += float64(b.Round)
					cnt++
				}
			}
			vals = append(vals, earned)
		}
		if cnt > 0 {
			ba.Round /= float64(cnt)
		}
		ba.Share = float64(cnt) / n
		ba.Lift = batch.LiftOf(vals, wins)
		fr.Books = append(fr.Books, ba)
	}

	// Spellbook requirements, every one of the cult's, met or not.
	for _, info := range Requirements(fr.Key) {
		ra := ReqAvg{Req: info.ID, Text: info.Text}
		var vals []float64
		cnt := 0
		for _, g := range gs {
			met := 0.0
			for _, r := range g.seat.Reqs {
				if r.Req == info.ID {
					met = 1
					ra.Round += float64(r.Round)
					cnt++
				}
			}
			vals = append(vals, met)
		}
		if cnt > 0 {
			ra.Round /= float64(cnt)
		}
		ra.Share = float64(cnt) / n
		ra.Lift = batch.LiftOf(vals, wins)
		fr.Reqs = append(fr.Reqs, ra)
	}

	// Units.
	unitSet := map[string]bool{}
	for _, g := range gs {
		for t := range g.seat.Built {
			unitSet[t] = true
		}
	}
	for _, t := range sortedKeys(unitSet) {
		ua := UnitAvg{Type: t}
		var vals []float64
		var wonN, lostN float64
		for _, g := range gs {
			built := float64(g.seat.Built[t])
			vals = append(vals, built)
			ua.Built += built / n
			ua.Lost += float64(g.seat.Lost[t]) / n
			if g.won {
				ua.WonBuilt += built
				wonN++
			} else {
				ua.LostBuilt += built
				lostN++
			}
		}
		if wonN > 0 {
			ua.WonBuilt /= wonN
		}
		if lostN > 0 {
			ua.LostBuilt /= lostN
		}
		ua.Lift = batch.LiftOf(vals, wins)
		fr.Units = append(fr.Units, ua)
	}

	// Actions.
	actSet := map[string]bool{}
	for _, g := range gs {
		for a := range g.seat.Actions {
			actSet[a] = true
		}
	}
	for _, a := range sortedKeys(actSet) {
		aa := ActionAvg{Action: a}
		var vals []float64
		used := 0
		for _, g := range gs {
			c := float64(g.seat.Actions[a])
			vals = append(vals, c)
			aa.Uses += c / n
			if c > 0 {
				used++
			}
		}
		aa.Share = float64(used) / n
		aa.Lift = batch.LiftOf(vals, wins)
		fr.Actions = append(fr.Actions, aa)
	}

	// Areas.
	areaSet := map[string]bool{}
	for _, g := range gs {
		for a := range g.seat.Areas {
			areaSet[a] = true
		}
	}
	for _, a := range sortedKeys(areaSet) {
		aa := AreaAvg{Area: a}
		var vals []float64
		for _, g := range gs {
			samples := float64(g.games.Samples)
			if samples == 0 {
				samples = 1
			}
			st := g.seat.Areas[a]
			share := 0.0
			if st != nil {
				share = float64(st.Control) / samples
				aa.Gate += float64(st.Gate) / samples / n
				aa.Presence += float64(st.Presence) / samples / n
			}
			vals = append(vals, share)
			aa.Control += share / n
		}
		aa.Lift = batch.LiftOf(vals, wins)
		fr.Areas = append(fr.Areas, aa)
	}
	return fr
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
