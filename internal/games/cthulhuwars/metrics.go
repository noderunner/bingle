package cthulhuwars

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// This file is the metrics observer: it watches a game from the outside, one
// decision at a time, and records what each seat did — the actions taken, the
// units built and lost, the Gates held, the spellbooks earned, the areas
// controlled — so that hundreds of simulated games can be compared (see
// internal/batch). It never changes a position and the rules never call it:
// everything is read from the positions before and after a decision.

// AreaStat counts, in the sampled moments of a game (the end of every round,
// and the end of the game), how often a seat held an area.
type AreaStat struct {
	// Gate: the seat controlled a Gate there. Presence: it had a unit there.
	// Control: it held the area — the Gate, or else the only units.
	Gate     int `json:"gate,omitempty"`
	Presence int `json:"presence,omitempty"`
	Control  int `json:"control,omitempty"`
}

// BookEvent is a spellbook earned.
type BookEvent struct {
	Book  string `json:"book"`
	Ply   int    `json:"ply"`
	Round int    `json:"round"`
}

// ReqEvent is a spellbook requirement met (the book it earns comes later, with
// the player's choice).
type ReqEvent struct {
	Req   string `json:"req"`
	Ply   int    `json:"ply"`
	Round int    `json:"round"`
}

// RequirementInfo names a spellbook requirement for reports.
type RequirementInfo struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Requirements lists a faction's spellbook requirements by faction key.
func Requirements(key string) []RequirementInfo {
	for i := range factions {
		if factions[i].Key == key {
			var out []RequirementInfo
			for _, r := range factions[i].Reqs {
				out = append(out, RequirementInfo(r))
			}
			return out
		}
	}
	return nil
}

// RoundSample is a seat's standing at the end of a round.
type RoundSample struct {
	Round int `json:"round"`
	Doom  int `json:"doom"`
	Power int `json:"power"`
	Gates int `json:"gates"`
	Units int `json:"units"`
	Books int `json:"books"`
	Signs int `json:"signs"`
}

// BattleStats are the battles a seat fought.
type BattleStats struct {
	Attacked int `json:"attacked,omitempty"`
	Defended int `json:"defended,omitempty"`
	// Won: the seat dealt more Kills than it took; Lost: fewer; the rest are
	// ties.
	Won  int `json:"won,omitempty"`
	Lost int `json:"lost,omitempty"`
	// KillsDealt and KillsTaken sum the Kill results; PainsDealt and
	// PainsTaken the Pain results.
	KillsDealt int `json:"kills_dealt,omitempty"`
	KillsTaken int `json:"kills_taken,omitempty"`
	PainsDealt int `json:"pains_dealt,omitempty"`
	PainsTaken int `json:"pains_taken,omitempty"`
}

// SeatMetrics is what one seat did in a game.
type SeatMetrics struct {
	Seat    int    `json:"seat"`
	Faction string `json:"faction"`
	// Won says the seat won the game.
	Won bool `json:"won"`
	// Actions counts the decisions by action id (ritual, battle, awaken, …);
	// Built counts units put on the map from the pool by unit type (recruited,
	// summoned, awakened) and Lost units that went back to the pool (killed or
	// eliminated).
	Actions map[string]int `json:"actions,omitempty"`
	Built   map[string]int `json:"built,omitempty"`
	Lost    map[string]int `json:"lost,omitempty"`
	// GatesBuilt and GatesLost count Gates the seat built and Gates it ceased
	// to control (abandoned, taken, destroyed).
	GatesBuilt int `json:"gates_built,omitempty"`
	GatesLost  int `json:"gates_lost,omitempty"`
	// Books lists the spellbooks in the order they were earned.
	Books []BookEvent `json:"books,omitempty"`
	// Reqs lists the spellbook requirements in the order they were met.
	Reqs []ReqEvent `json:"reqs,omitempty"`
	// Areas maps an area id to how often the seat held it (see AreaStat).
	Areas   map[string]*AreaStat `json:"areas,omitempty"`
	Rounds  []RoundSample        `json:"rounds,omitempty"`
	Battles BattleStats          `json:"battles"`
	// Final standing.
	Doom       int `json:"doom"`
	Power      int `json:"power"`
	Signs      int `json:"signs"`
	Gates      int `json:"gates"`
	BooksTotal int `json:"books_total"`
	// DoomFrom says where the final Doom came from.
	DoomFrom DoomLedger `json:"doom_from"`
}

// GameMetrics is the record of one finished game.
type GameMetrics struct {
	Players int `json:"players"`
	Plies   int `json:"plies"`
	Rounds  int `json:"rounds"`
	// Winner is the winning seat, 0 when nobody won; EndReason says how the
	// game ended.
	Winner    int    `json:"winner"`
	EndReason string `json:"end_reason,omitempty"`
	// Samples is how many times the areas were sampled.
	Samples int           `json:"samples"`
	Seats   []SeatMetrics `json:"seats"`
}

// Observer records the metrics of one game.
type Observer struct {
	game  *catalog.CompiledGame
	rs    *Ruleset
	m     *mapData
	n     int
	seats []*SeatMetrics
	// samples counts area samples; lastRound is the round last sampled.
	samples   int
	lastRound int
	plies     int
}

// NewObserver starts recording a game of the given definition.
func NewObserver(game *catalog.CompiledGame) *Observer {
	m := mapOf(game)
	o := &Observer{game: game, rs: New(), m: m, n: m.cfg.n}
	for seat := 1; seat <= o.n; seat++ {
		sm := &SeatMetrics{
			Seat: seat, Faction: keyOfSeat(game, seat),
			Actions: map[string]int{}, Built: map[string]int{}, Lost: map[string]int{},
			Areas: map[string]*AreaStat{},
		}
		o.seats = append(o.seats, sm)
	}
	return o
}

func (o *Observer) seat(s int) *SeatMetrics {
	if s < 1 || s > len(o.seats) {
		return nil
	}
	return o.seats[s-1]
}

// Observe records one decision: seat played mv, taking the position from
// before to after. It reads both and keeps neither.
func (o *Observer) Observe(before, after *state.BoardState, seat int, mv state.Move) {
	bd, err1 := dataOf(before)
	ad, err2 := dataOf(after)
	if err1 != nil || err2 != nil {
		return
	}
	o.plies++
	if sm := o.seat(seat); sm != nil {
		sm.Actions[mv.Action]++
	}
	be := o.rs.newEng(o.game, before, bd)
	ae := o.rs.newEng(o.game, after, ad)

	// Units: the pools tell what was put on the map and what came back.
	for s := 1; s <= o.n; s++ {
		sm := o.seat(s)
		for _, typ := range unitTypes {
			delta := ae.pool(s, typ) - be.pool(s, typ)
			if delta < 0 {
				sm.Built[typ] -= delta
			} else if delta > 0 {
				sm.Lost[typ] += delta
			}
		}
	}

	// Gates.
	for area, ctl := range ad.Gates {
		prev, had := bd.Gates[area]
		if !had {
			if sm := o.seat(ctl); sm != nil {
				sm.GatesBuilt++
			}
			continue
		}
		if prev != ctl && prev != 0 {
			if sm := o.seat(prev); sm != nil {
				sm.GatesLost++
			}
		}
	}
	for area, prev := range bd.Gates {
		if _, still := ad.Gates[area]; !still && prev != 0 {
			if sm := o.seat(prev); sm != nil {
				sm.GatesLost++
			}
		}
	}

	// Spellbooks earned.
	for s := 1; s <= o.n; s++ {
		f := ae.fac(s)
		if f < 0 {
			continue
		}
		for i, bk := range factions[f].Books {
			if has(ad.Books[s], i) && !has(bd.Books[s], i) {
				sm := o.seat(s)
				sm.Books = append(sm.Books, BookEvent{Book: bk.ID, Ply: o.plies, Round: ad.Round})
			}
		}
	}

	// Spellbook requirements met.
	for s := 1; s <= o.n; s++ {
		f := ae.fac(s)
		if f < 0 {
			continue
		}
		for i, r := range factions[f].Reqs {
			if has(ad.Reqs[s], i) && !has(bd.Reqs[s], i) {
				sm := o.seat(s)
				sm.Reqs = append(sm.Reqs, ReqEvent{Req: r.ID, Ply: o.plies, Round: ad.Round})
			}
		}
	}

	// A battle whose dice were just rolled.
	if ab := ad.Bat; ab != nil && ab.Rolled && (bd.Bat == nil || !bd.Bat.Rolled) {
		o.battle(ab)
	}

	// The end of a round: how everyone stands, and who holds which area.
	if ad.Round > bd.Round {
		o.sample(ae, bd.Round)
	}
}

// battle records the dice of a battle: Kills[x] and Pains[x] are the results
// x's units take.
func (o *Observer) battle(b *Battle) {
	att, def := o.seat(b.Att), o.seat(b.Def)
	if att == nil || def == nil {
		return
	}
	att.Battles.Attacked++
	def.Battles.Defended++
	for _, p := range [][2]*SeatMetrics{{att, def}, {def, att}} {
		me, foe := p[0], p[1]
		dealt, taken := b.Kills[foe.Seat], b.Kills[me.Seat]
		me.Battles.KillsDealt += dealt
		me.Battles.KillsTaken += taken
		me.Battles.PainsDealt += b.Pains[foe.Seat]
		me.Battles.PainsTaken += b.Pains[me.Seat]
		switch {
		case dealt > taken:
			me.Battles.Won++
		case dealt < taken:
			me.Battles.Lost++
		}
	}
}

// sample records every seat's standing and the areas it holds. round is the
// round that has just ended.
func (o *Observer) sample(e *eng, round int) {
	d := e.d
	o.samples++
	o.lastRound = round
	for s := 1; s <= o.n; s++ {
		sm := o.seat(s)
		gates, units := 0, 0
		for _, ctl := range d.Gates {
			if ctl == s {
				gates++
			}
		}
		for a := range e.m.ids {
			units += e.total(a, s)
		}
		sm.Rounds = append(sm.Rounds, RoundSample{
			Round: round, Doom: d.Doom[s], Power: d.Power[s], Gates: gates, Units: units,
			Books: d.bookCount(s), Signs: len(d.Signs[s]) + len(d.Shown[s]),
		})
	}
	for a, id := range e.m.ids {
		area := string(id)
		owners := 0 // seats with units here
		sole := 0
		for s := 1; s <= o.n; s++ {
			if e.total(a, s) > 0 {
				owners++
				sole = s
			}
		}
		gate := d.Gates[area]
		for s := 1; s <= o.n; s++ {
			present := e.total(a, s) > 0
			var st *AreaStat
			get := func() *AreaStat {
				if st == nil {
					if st = o.seat(s).Areas[area]; st == nil {
						st = &AreaStat{}
						o.seat(s).Areas[area] = st
					}
				}
				return st
			}
			if gate == s {
				get().Gate++
			}
			if present {
				get().Presence++
			}
			if gate == s || (gate == 0 && owners == 1 && sole == s) {
				get().Control++
			}
		}
	}
}

// Finish closes the record with the final position and returns it. winner is
// the winning seat (0 for none).
func (o *Observer) Finish(final *state.BoardState, winner int, reason string) GameMetrics {
	gm := GameMetrics{Players: o.n, Plies: o.plies, Winner: winner, EndReason: reason, Samples: o.samples}
	if d, err := dataOf(final); err == nil {
		e := o.rs.newEng(o.game, final, d)
		gm.Rounds = d.Round
		// The last, unfinished round counts as a sample of its own.
		o.sample(e, d.Round)
		gm.Samples = o.samples
		for s := 1; s <= o.n; s++ {
			sm := o.seat(s)
			sm.Doom, sm.Power = d.Doom[s], d.Power[s]
			sm.Signs = len(d.Signs[s]) + len(d.Shown[s])
			sm.BooksTotal = d.bookCount(s)
			sm.DoomFrom = d.DoomBy[s]
			for _, ctl := range d.Gates {
				if ctl == s {
					sm.Gates++
				}
			}
		}
	}
	for _, sm := range o.seats {
		sm.Won = winner == sm.Seat
		gm.Seats = append(gm.Seats, *sm)
	}
	return gm
}
