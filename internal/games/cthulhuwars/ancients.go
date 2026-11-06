package cthulhuwars

import (
	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
)

// The Ancients: a cult with no Great Old One. It builds Cathedrals, which earn
// most of its spellbooks, and it leans on its enemies with gifts of Monsters and
// Power.

const (
	uUnMen      = "un-men"
	uReanimated = "reanimated"
	uYothan     = "yothan"
)

// The spellbooks of the Ancients, in the order of the card.
const (
	bkBrainless = iota
	bkConsecration
	bkExtinction
	bkFestival
	bkUnholy
	bkWorship
)

// The requirements of the Ancients.
const (
	rqANDragon = iota
	rqANThorns
	rqANChevron
	rqANPlain
	rqANLowest
	rqANHighest
)

const maxCathedrals = 4

var ancientsFaction = faction{
	Key: "an", Name: "The Ancients", Color: "teal", Seat: 9, Start: "",
	Ability: "Dematerialization (Doom phase): relocate your units from one area to a single other area, anywhere on the map.",
	Units: []unitSpec{
		{ID: uCultist, Name: "Acolyte Cultist", Class: classCultist, Count: 6, Cost: 1, Combat: 0},
		{ID: uUnMen, Name: "Un-Men", Class: classMonster, Count: 3, Cost: 3, Combat: 0, Text: "Festival (spellbook): cost 0, and each one summoned gives an enemy of your choice 1 Power."},
		{ID: uReanimated, Name: "Reanimated", Class: classMonster, Count: 3, Cost: 4, Combat: 2, Text: "Brainless (spellbook): cost 1, but they only Move, Capture or declare Battle with another unit of yours beside them."},
		{ID: uYothan, Name: "Yothan", Class: classTerror, Count: 3, Cost: 6, Combat: 7, Text: "A Terror. Extinction (spellbook): cost 3, but one that is Killed or Eliminated leaves the game for good."},
	},
	Books: []bookSpec{
		{ID: "brainless", Name: "Brainless", Timing: "Ongoing", Text: "Reanimated now cost 1 Power to Summon. They may only Move, Capture, or declare Battle if they share an Area with one or more of your non-Reanimated Units.", Units: []string{uReanimated}},
		{ID: "consecration", Name: "Consecration", Timing: "Doom Phase", Text: "When you perform a Ritual of Annihilation, gain 1 Elder Sign if at least one Cathedral is in play. If all four Cathedrals are in play, gain 2 Elder Signs instead."},
		{ID: "extinction", Name: "Extinction", Timing: "Ongoing", Text: "Yothans now cost 3 Power to Summon. When a Yothan is Killed or Eliminated, remove it permanently from the game.", Units: []string{uYothan}},
		{ID: "festival", Name: "Festival", Timing: "Ongoing", Text: "Un-Men now cost 0 Power to Summon. When you Summon an Un-Man, also select an enemy to gain 1 Power.", Units: []string{uUnMen}},
		{ID: "unholy-ground", Name: "Unholy Ground", Timing: "Post-Battle", Text: "If there is a Cathedral in the Battle Area, you may choose to remove a Cathedral from anywhere. If you do, an enemy Great Old One in the Battle must be Eliminated by its owner."},
		{ID: "worship-services", Name: "Worship Services", Timing: "Gather Power Phase", Text: "Gain 1 Power for each Cathedral that shares an Area with an enemy Gate. Those enemies each gain 1 Power."},
	},
	Reqs: []reqSpec{
		{ID: "dragon", Text: "A Cathedral is in an Area marked with the Dragon glyph."},
		{ID: "thorns", Text: "A Cathedral is in an Area marked with the Thorns glyph."},
		{ID: "chevron", Text: "A Cathedral is in an Area marked with the Chevron glyph."},
		{ID: "plain", Text: "A Cathedral is in an Area without any of these glyphs."},
		{ID: "lowest", Text: "As your Action, each enemy Summons their lowest cost Monster at their Controlled Gate for free."},
		{ID: "highest", Text: "As your Action, each enemy Summons their highest cost Monster at their Controlled Gate for free."},
	},
}

// hasCathedral says a Cathedral stands in a.
func (e *eng) hasCathedral(a int) bool {
	id := e.areaID(a)
	for _, c := range e.d.Cathedrals {
		if c == id {
			return true
		}
	}
	return false
}

// cathedralCost is 3 Power, or 1 when the area is not adjacent to a Cathedral.
func (e *eng) cathedralCost(a int) int {
	for _, n := range e.m.adj[a] {
		if e.hasCathedral(n) {
			return 3
		}
	}
	return 1
}

// ancientsTurnMoves lists the Actions only the Ancients have.
func (e *eng) ancientsTurnMoves(seat int) []state.Selection {
	d := e.d
	if !e.is(seat, facAN) {
		return nil
	}
	var out []state.Selection
	add := func(mv state.Selection) {
		if d.Power[seat] >= e.cardActCost(mv) {
			out = append(out, mv)
		}
	}
	if len(d.Cathedrals) < maxCathedrals {
		for a := range e.m.ids {
			if e.cultists(a, seat) > 0 && !e.hasCathedral(a) {
				add(state.Selection{Action: actCard, Piece: "cathedral", To: e.m.ids[a]})
			}
		}
	}
	if !d.reqDone(seat, rqANLowest) {
		add(state.Selection{Action: actCard, Piece: "an-lowest"})
	}
	if !d.reqDone(seat, rqANHighest) {
		add(state.Selection{Action: actCard, Piece: "an-highest"})
	}
	return out
}

// buildCathedral settles the Action and meets the requirement of the area's glyph.
func (e *eng) buildCathedral(seat, a int) {
	d := e.d
	d.Cathedrals = append(d.Cathedrals, e.areaID(a))
	e.say("%s builds a Cathedral in %s (%d of %d).", e.seatName(seat), e.areaName(a), len(d.Cathedrals), maxCathedrals)
	switch e.m.glyph[a] {
	case "dragon":
		e.fulfil(seat, rqANDragon)
	case "thorns":
		e.fulfil(seat, rqANThorns)
	case "chevron":
		e.fulfil(seat, rqANChevron)
	default:
		e.fulfil(seat, rqANPlain)
	}
}

// giftMonsters has each enemy summon, free, its cheapest or dearest Monster at a
// Gate it controls.
func (e *eng) giftMonsters(seat int, dearest bool) {
	for _, o := range e.others(seat) {
		gates := e.gatesOf(o)
		if len(gates) == 0 {
			continue
		}
		best, cost := "", -1
		for _, u := range e.units(o) {
			if u.Class != classMonster || u.Token || e.pool(o, u.ID) <= 0 || u.ID == uShambler || u.ID == uMoonbeast {
				continue
			}
			c := e.summonCost(o, u.ID)
			if best == "" || (dearest && c > cost) || (!dearest && c < cost) {
				best, cost = u.ID, c
			}
		}
		if best != "" && e.place(gates[0], o, best) {
			e.say("%s summons %s at its Gate in %s for free.", e.seatName(o), article(unitNameOf(best)), e.areaName(gates[0]))
		}
	}
}

// brainless says the Reanimated of seat in a are held back by Brainless: they
// have no other unit of the cult with them.
func (e *eng) brainless(a, seat int) bool {
	return e.is(seat, facAN) && e.d.hasBook(seat, bkBrainless) && e.count(a, seat, uReanimated) > 0 && e.total(a, seat) == e.count(a, seat, uReanimated)
}

// consecration is the Elder Signs a Ritual of the Ancients earns besides.
func (e *eng) consecration(seat int) {
	if !e.is(seat, facAN) || !e.d.hasBook(seat, bkConsecration) {
		return
	}
	n := 0
	switch {
	case len(e.d.Cathedrals) >= maxCathedrals:
		n = 2
	case len(e.d.Cathedrals) > 0:
		n = 1
	}
	if n > 0 {
		e.say("Consecration: %s gains %s.", e.seatName(seat), plural(n, "Elder Sign", "Elder Signs"))
	}
	for i := 0; i < n; i++ {
		e.gainSign(seat)
	}
}

// worshipServices pays the Cathedrals that share an area with an enemy Gate.
func (e *eng) worshipServices() {
	d := e.d
	s := e.seatOf(facAN)
	if s == 0 || !d.hasBook(s, bkWorship) {
		return
	}
	for _, id := range d.Cathedrals {
		if ctl := d.Gates[id]; ctl != 0 && ctl != s {
			d.Power[s]++
			d.Power[ctl]++
			e.say("Worship Services: the Cathedral in %s pays %s and %s 1 Power each.", e.areaName(e.areaIdx(id)), e.seatName(s), e.seatName(ctl))
		}
	}
}

// unholyMoves lists the Cathedrals the Ancients may give up after a battle in an
// area with one, to have an enemy Great Old One in it Eliminated.
func (e *eng) unholyMoves(seat int) []state.Selection {
	d := e.d
	b := d.Bat
	if !e.is(seat, facAN) || !d.hasBook(seat, bkUnholy) || b == nil {
		return nil
	}
	a := e.areaIdx(b.Area)
	if !e.hasCathedral(a) || e.classCount(a, b.foeOf(seat), classGOO) == 0 {
		return nil
	}
	var out []state.Selection
	for _, id := range d.Cathedrals {
		out = append(out, state.Selection{Action: actCard, Piece: "unholy", To: board.PositionID(id)})
	}
	return out
}

// unholy gives up a Cathedral and Eliminates the enemy's Great Old One.
func (e *eng) unholy(seat int, mv state.Selection) {
	d := e.d
	b := d.Bat
	a := e.areaIdx(b.Area)
	foe := b.foeOf(seat)
	for i, id := range d.Cathedrals {
		if id == string(mv.To) {
			d.Cathedrals = append(d.Cathedrals[:i], d.Cathedrals[i+1:]...)
			break
		}
	}
	for _, g := range e.typesIn(a, foe) {
		if unitClassOf(g) == classGOO {
			e.say("Unholy Ground: %s gives up the Cathedral in %s, and %s's %s is Eliminated.", e.seatName(seat), e.areaName(e.areaIdx(string(mv.To))), e.seatName(foe), unitNameOf(g))
			e.eliminate(a, foe, g, seat)
			for i, u := range b.Pained {
				if u.Seat == foe && u.Type == g {
					b.Pained = append(b.Pained[:i], b.Pained[i+1:]...)
					break
				}
			}
			return
		}
	}
}

// dematMoves lists the areas the Ancients may relocate units from in their Doom turn.
func (e *eng) dematMoves(seat int) []state.Selection {
	if !e.is(seat, facAN) || e.d.Doomx == nil || e.d.Doomx.Demat {
		return nil
	}
	var out []state.Selection
	for a := range e.m.ids {
		if e.dematUnits(a, seat) > 0 {
			out = append(out, state.Selection{Action: actCard, Piece: "demat", To: e.m.ids[a]})
		}
	}
	return out
}

// dematUnits counts the units of seat that can leave a: all but the one on a Gate.
func (e *eng) dematUnits(a, seat int) int {
	n := e.total(a, seat)
	if e.d.Gates[e.areaID(a)] == seat {
		n--
	}
	return n
}

// dematerialize moves all of seat's units but the Gate's holder from one area to another.
func (e *eng) dematerialize(seat, from, to int) {
	moved := 0
	holder := ""
	if e.d.Gates[e.areaID(from)] == seat {
		holder = e.holderAt(from)
	}
	for _, typ := range e.typesIn(from, seat) {
		n := e.count(from, seat, typ)
		if typ == holder {
			n--
		}
		for ; n > 0; n-- {
			if e.take(from, seat, typ) {
				e.add(to, seat, typ)
				moved++
			}
		}
	}
	e.say("Dematerialization: %s moves %s from %s to %s.", e.seatName(seat), plural(moved, "unit", "units"), e.areaName(from), e.areaName(to))
	e.takeAbandonedGate(to, seat)
}
