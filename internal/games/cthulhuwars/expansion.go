package cthulhuwars

import (
	"github.com/noderunner/bingle/internal/engine/state"
)

// This file holds what the expansion cults and the High Priests share with the
// rest of the rules: the unique abilities a cult may borrow (Ancient Sorcery),
// the Sleeper's sleeping Gate, the Ice Age surcharge and the Unspeakable Oath.

// Unique abilities (the faction ability on a faction card).
const (
	abImmortal   = iota // Great Cthulhu: Elder Signs for awakenings, cheaper second awakening
	abFlight            // Crawling Chaos: every unit moves up to two areas
	abFertility         // Black Goat: summoning is an unlimited action
	abFeast             // Yellow Sign: Power for Desecrated areas
	abHibernate         // Windwalker: bank the Power
	abDeath             // Sleeper: a free Monster in the Doom phase
	abBeyond            // Opener of the Way: move a unit with a Gate
	abSycophancy        // Tcho-Tcho: a Ritual of an enemy gives it Doom or costs the enemy one
)

var factionAbility = [nFactions]int{
	facGC: abImmortal, facCC: abFlight, facBG: abFertility, facYS: abFeast, facWW: abHibernate, facSL: abDeath,
	facOW: abBeyond, facTT: abSycophancy,
}

// ability reports whether seat has the unique ability ab: its own, or — for
// the Sleeper — one it borrowed with Ancient Sorcery from the cult whose
// faction card holds one of its Serpent Men.
func (e *eng) ability(seat, ab int) bool {
	f := e.fac(seat)
	if f < 0 {
		return false
	}
	if factionAbility[f] == ab {
		return true
	}
	if f == facSL {
		for _, s := range e.d.Sorcery {
			if s.Ending {
				continue
			}
			if of := e.fac(s.Seat); of >= 0 && of != facSL && factionAbility[of] == ab {
				return true
			}
		}
	}
	return false
}

// --- the sleeping Gate of the Sleeper ----------------------------------------------

// slumberCultists is 1 when seat is the Sleeper with a Cultist asleep on its
// faction card (it still counts as one of its Cultists in play).
func (e *eng) slumberCultists(seat int) int {
	if e.d.Slumber != nil && e.is(seat, facSL) {
		return 1
	}
	return 0
}

// slumberGates is 1 when seat is the Sleeper with a Gate asleep.
func (e *eng) slumberGates(seat int) int { return e.slumberCultists(seat) }

// gateCount is how many Gates seat controls for Doom and Power: those on the
// map and the one asleep on the Sleeper's faction card.
func (e *eng) gateCount(seat int) int { return len(e.gatesOf(seat)) + e.slumberGates(seat) }

// --- the Ice Age surcharge --------------------------------------------------------------

// iceTax is the extra Power an Action of seat that ends in area a costs: 1 in
// the area of Windwalker's Ice Age token, for every cult but Windwalker.
func (e *eng) iceTax(seat, a int) int {
	if e.d.Ice == "" || a < 0 || e.areaID(a) != e.d.Ice || e.is(seat, facWW) {
		return 0
	}
	return 1
}

// --- High Priests and Unspeakable Oath ----------------------------------------------------

// hpArea is where seat's High Priest stands, if it is on the map.
func (e *eng) hpArea(seat int) (int, bool) {
	if !e.hpFor(seat) {
		return 0, false
	}
	for a := range e.m.ids {
		if e.count(a, seat, uHighPriest) > 0 {
			return a, true
		}
	}
	return 0, false
}

// oathAvailable says seat can sacrifice its High Priest now.
func (e *eng) oathAvailable(seat int) bool {
	_, ok := e.hpArea(seat)
	return ok
}

// oathThreatened says an enemy that outranks seat's High Priest stands beside
// him, so that he may be captured: the moment to think of Unspeakable Oath.
func (e *eng) oathThreatened(seat int) bool {
	a, ok := e.hpArea(seat)
	if !ok {
		return false
	}
	mine := e.protectionRank(a, seat)
	for _, en := range e.others(seat) {
		if e.protectionRank(a, en) > mine {
			return true
		}
	}
	return false
}

// sacrificePriest returns seat's High Priest to its pool for 2 Power.
func (e *eng) sacrificePriest(seat int) {
	a, ok := e.hpArea(seat)
	if !ok {
		return
	}
	e.take(a, seat, uHighPriest)
	e.setPool(seat, uHighPriest, e.pool(seat, uHighPriest)+1)
	e.d.Power[seat] += 2
	e.say("Unspeakable Oath: %s sacrifices its High Priest in %s and gains 2 Power (now %d).", e.seatName(seat), e.areaName(a), e.d.Power[seat])
	e.syncGate(a)
}

// --- helpers for the expansion actions --------------------------------------------------

// enemiesWithSixBooks counts the other cults with all six spellbooks.
func (e *eng) enemiesWithSixBooks(seat int) int {
	n := 0
	for _, o := range e.others(seat) {
		if e.d.bookCount(o) >= 6 {
			n++
		}
	}
	return n
}

// enemyGreatOldOnes counts the Great Old Ones the other cults have in play.
func (e *eng) enemyGreatOldOnes(seat int) int {
	n := 0
	for _, o := range e.others(seat) {
		n += e.gooTotal(o)
	}
	return n
}

// anotherHasPower says some other cult can still act this Action phase: it has
// Power and has not hibernated.
func (e *eng) anotherHasPower(seat int) bool {
	for _, o := range e.others(seat) {
		if e.d.Power[o] > 0 && !e.d.Hib[o] {
			return true
		}
	}
	return false
}

// lowestCostMonster is the type of the cheapest Monster in seat's pool ("" if
// none): Death from Below places it.
func (e *eng) lowestCostMonster(seat int) string {
	best, bestCost := "", 1<<30
	for _, u := range e.units(seat) {
		if u.Class != classMonster || u.Token || e.pool(seat, u.ID) <= 0 {
			continue
		}
		c := e.summonCost(seat, u.ID)
		if c < bestCost {
			best, bestCost = u.ID, c
		}
	}
	return best
}

// sigilAreas lists the areas marked with faction f's sigil.
func (e *eng) sigilAreas(f int) []int {
	return e.m.sigil[f]
}

// setsUpLater says seat's cult begins after the others, in an area it chooses:
// the Tcho-Tcho, the Windwalker (when the map has sigil areas) and the Opener
// of the Way.
func (e *eng) setsUpLater(seat int) bool {
	switch e.fac(seat) {
	case facTT, facOW:
		return true
	case facWW:
		return len(e.sigilAreas(facWW)) > 0
	}
	return false
}

// setupRank orders the cults that set up late: the Tcho-Tcho, then the
// Windwalker, the Opener of the Way last (the cards say "after all other
// players have set up").
func setupRank(f int) int {
	switch f {
	case facTT:
		return 0
	case facWW:
		return 1
	}
	return 2
}

// legalSetup lists where the cult that is setting up may begin: the sigil
// areas of the Windwalker, an empty area with a glyph for the Tcho-Tcho, any
// empty area for the Opener of the Way.
func (e *eng) legalSetup() []state.Move {
	var out []state.Move
	var areas []int
	switch e.fac(e.d.Setup) {
	case facWW:
		areas = e.sigilAreas(facWW)
	case facTT:
		areas = e.glyphAreas()
		if len(areas) == 0 {
			areas = e.emptyAreas()
		}
	default:
		areas = e.emptyAreas()
	}
	for _, a := range areas {
		out = append(out, state.Move{Action: actStart, To: e.m.ids[a]})
	}
	return out
}
