package batch

import (
	"math"
	"sort"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
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
	// Factions are the cults that played, in line-up order (Cthulhu Wars).
	Factions []FactionReport `json:"factions"`
	// Seats is what every game has: how each seat of the table fared, which
	// is where the advantage of moving first shows.
	Seats []SeatReport `json:"seats,omitempty"`
	// Bots is the same by bot, whichever seat it played: which bot is
	// stronger, at the settings this batch gave them.
	Bots []BotReport `json:"bots,omitempty"`
	// AreaNames maps an area id to its name.
	AreaNames map[string]string `json:"area_names,omitempty"`
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

// Tally counts games and how many of them a cult won.
type Tally struct {
	N    int `json:"n"`
	Wins int `json:"wins"`
}

// Lift compares the games in which a cult did more than its average of
// something (built it, used it, held an area) with those in which it did not:
// the win rates tell whether doing it goes with winning. Threshold is the
// average the split was made at.
type Lift struct {
	With      Tally   `json:"with"`
	Without   Tally   `json:"without"`
	Threshold float64 `json:"threshold"`
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
	BySeat []Tally `json:"by_seat"`

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
	Book  string  `json:"book"`
	Share float64 `json:"share"` // of games in which it was earned
	Round float64 `json:"round"` // average round it was earned in
	Lift  Lift    `json:"lift"`
}

// ReqAvg is when (and whether) a spellbook requirement is met.
type ReqAvg struct {
	Req   string  `json:"req"`
	Text  string  `json:"text"`
	Share float64 `json:"share"` // of games in which it was met
	Round float64 `json:"round"` // average round it was met in
	Lift  Lift    `json:"lift"`
}

// UnitAvg is how many units of a type the cult built.
type UnitAvg struct {
	Type  string  `json:"type"`
	Built float64 `json:"built"`
	Lost  float64 `json:"lost"`
	// WonBuilt and LostBuilt are the averages in the games it won and lost.
	WonBuilt  float64 `json:"won_built"`
	LostBuilt float64 `json:"lost_built"`
	Lift      Lift    `json:"lift"`
}

// ActionAvg is how often the cult took an action.
type ActionAvg struct {
	Action string  `json:"action"`
	Uses   float64 `json:"uses"`
	Share  float64 `json:"share"` // of games in which it was used at least once
	Lift   Lift    `json:"lift"`
}

// AreaAvg is how much of the game the cult held an area.
type AreaAvg struct {
	Area     string  `json:"area"`
	Gate     float64 `json:"gate"`     // share of samples with the cult's Gate there
	Presence float64 `json:"presence"` // share with a unit there
	Control  float64 `json:"control"`  // share holding it
	Lift     Lift    `json:"lift"`     // on the control share
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

// seatGame is one cult's part of one game.
type seatGame struct {
	rec   *Record
	seat  cthulhuwars.SeatMetrics
	won   bool
	games *cthulhuwars.GameMetrics
}

// BuildReport aggregates the records of a batch. names maps area ids to names.
func BuildReport(recs []Record, names map[string]string) Report {
	rep := Report{Games: len(recs), EndReasons: map[string]int{}, AreaNames: names}
	var plies, rounds []float64
	order := []string{}
	byFac := map[string][]seatGame{}
	for i := range recs {
		r := &recs[i]
		m := &r.Metrics
		rep.EndReasons[r.EndReason]++
		if r.Winner != 0 {
			rep.Decided++
		}
		plies = append(plies, float64(r.Plies))
		if r.Rounds > 0 {
			rounds = append(rounds, float64(r.Rounds))
		}
		for _, sm := range m.Seats {
			if _, ok := byFac[sm.Faction]; !ok {
				order = append(order, sm.Faction)
			}
			byFac[sm.Faction] = append(byFac[sm.Faction], seatGame{rec: r, seat: sm, won: sm.Won, games: m})
		}
	}
	rep.Plies, rep.Rounds = distOf(plies), distOf(rounds)
	rep.PlyBuckets = histogram(plies, 12)
	sort.Strings(order)
	for _, key := range order {
		rep.Factions = append(rep.Factions, factionReport(key, byFac[key]))
	}
	return rep
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
	fr.BySeat = make([]Tally, maxSeat)
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
	fr.Lo, fr.Hi = wilson(fr.Wins, len(gs))

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
		ba.Lift = liftOf(vals, wins)
		fr.Books = append(fr.Books, ba)
	}

	// Spellbook requirements, every one of the cult's, met or not.
	for _, info := range cthulhuwars.Requirements(fr.Key) {
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
		ra.Lift = liftOf(vals, wins)
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
		ua.Lift = liftOf(vals, wins)
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
		aa.Lift = liftOf(vals, wins)
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
		aa.Lift = liftOf(vals, wins)
		fr.Areas = append(fr.Areas, aa)
	}
	return fr
}

// liftOf splits the games at the mean of vals: the games above it against the
// rest. When every game is the same there is nothing to compare.
func liftOf(vals []float64, wins []bool) Lift {
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

// wilson is the 95% Wilson score interval of k successes in n trials.
func wilson(k, n int) (lo, hi float64) {
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

func distOf(v []float64) Dist {
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

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
