package cthulhuwars

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
)

func boardID(s string) board.PositionID { return board.PositionID(s) }

// reach lists the areas a unit of seat may move to from a with one Move (the
// slice is shared: callers must not change it).
func (e *eng) reach(seat, a int) []int {
	if e.ability(seat, abFlight) { // Flight
		return e.m.reach2[a]
	}
	return e.m.adj[a]
}

// summonCost is the Power cost to summon a monster.
func (e *eng) summonCost(seat int, typ string) int {
	u, ok := e.f(seat).unitSpec(typ)
	if !ok {
		return 99
	}
	cost := u.Cost
	if typ == uGnoph {
		// As many Power as there are Gnoph-Keh in the pool.
		cost = e.pool(seat, uGnoph)
	}
	if e.is(seat, facBG) && e.d.hasBook(seat, bkThousand) && e.gooInPlay(seat) > 0 {
		switch typ {
		case uGhoul, uFungi, uYoung:
			cost--
		}
	}
	if cost < 0 {
		cost = 0
	}
	return cost
}

// awakenCost is the Power cost to awaken a Great Old One (0 if it cannot be
// awakened now) and the area it would appear in is checked by awakenAreas.
func (e *eng) awakenCost(seat int, typ string) int {
	switch typ {
	case uCthulhu:
		if e.d.Awoke(seat, 0) {
			return 4
		}
		return 10
	case uNyar:
		return 10
	case uShub:
		return 8
	case uKing:
		return 4
	case uHastur:
		return 10
	case uRhan, uIthaqua:
		return 6
	case uTsath:
		// Immortal, borrowed with Ancient Sorcery, makes him cheaper after the
		// first time.
		if e.ability(seat, abImmortal) && e.d.Awoke(seat, 0) {
			return 4
		}
		return 8
	case uYog:
		return 6
	case uAbhoth, uChaugnar, uHydra, uYig:
		return 4
	case uCthugha:
		return 6
	case uUbbo:
		// Free in the Doom phase, 6 Power in the Action phase.
		if e.d.Phase == phaseDoom {
			return 0
		}
		return 6
	}
	return 0
}

// awakenAreas lists where seat could awaken the Great Old One typ.
func (e *eng) awakenAreas(seat int, typ string) []int {
	if e.pool(seat, typ) <= 0 {
		return nil
	}
	var out []int
	switch typ {
	case uCthulhu:
		// Only in the faction's starting area, which must hold a Gate — its
		// own, an abandoned one or an enemy's.
		if a := e.areaIdx(startArea(e.game, *e.f(seat))); e.hasGate(a) {
			out = append(out, a)
		}
	case uNyar:
		out = e.gatesOf(seat)
	case uShub:
		if e.cultistsInPlay(seat) >= 2 {
			out = e.gatesOf(seat)
		}
	case uKing:
		for a := range e.m.ids {
			if e.total(a, seat) > 0 && !e.hasGate(a) {
				out = append(out, a)
			}
		}
	case uHastur:
		for _, a := range e.gatesOf(seat) {
			if e.count(a, seat, uKing) > 0 {
				out = append(out, a)
			}
		}
	case uRhan:
		// In any area marked with the Windwalker sigil.
		out = append(out, e.sigilAreas(facWW)...)
	case uIthaqua:
		// Once Rhan-Tegoth has been awakened, in place of a Gate (anyone's) in an
		// area marked with the sigil.
		if e.d.Awoke(seat, 0) {
			for _, a := range e.sigilAreas(facWW) {
				if e.hasGate(a) {
					out = append(out, a)
				}
			}
		}
	case uTsath:
		// Where a Formless Spawn stands.
		for a := range e.m.ids {
			if e.count(a, seat, uFormless) > 0 {
				out = append(out, a)
			}
		}
	case uYog:
		// Where a Spawn of Yog-Sothoth stands: it is replaced.
		for a := range e.m.ids {
			if e.count(a, seat, uSpawn) > 0 {
				out = append(out, a)
			}
		}
	case uUbbo:
		// At a Gate, while a High Priest is in play.
		out = e.ubboAreas(seat)
	}
	return out
}

// protectionRank is the strongest non-cultist unit seat has in a: 2 for a
// Great Old One, 1 for a monster, 0 for none.
func (e *eng) protectionRank(a, seat int) int {
	if e.classCount(a, seat, classGOO) > 0 {
		return 2
	}
	// A Terror guards as well as a Monster does.
	if e.classCount(a, seat, classMonster)+e.classCount(a, seat, classTerror) > 0 {
		return 1
	}
	return 0
}

// canBattle reports whether seat could start a battle against enemy in a.
func (e *eng) canBattle(seat, enemy, a int) bool {
	if e.total(a, enemy) == 0 || e.total(a, seat) == 0 {
		return false
	}
	return e.combatOf(seat, a, nil, enemy) >= 1
}

func (e *eng) hasBattled(seat int, a int) bool {
	id := e.areaID(a)
	for _, x := range e.d.Battled[seat] {
		if x == id {
			return true
		}
	}
	return false
}

func (e *eng) unlimitedBattle(seat int) bool { return e.d.bookCount(seat) >= 6 }

// --- what an action costs --------------------------------------------------------------

// gateCost is the Power a Gate costs.
func (e *eng) gateCost() int { return e.game.ParamInt("gate_cost", 3) }

// gateCostFor is what seat pays for a Gate: Umr at-Tawil makes it 2.
func (e *eng) gateCostFor(seat int) int {
	c := e.gateCost()
	if e.d.hasNeutral(seat, nbUmr) && c > 2 {
		c = 2
	}
	return c
}

// moveCost is the Power seat pays for mv: its price plus the Ice Age
// surcharge when it ends in the area of Windwalker's token.
func (e *eng) moveCost(seat int, mv state.Move) int {
	to := -1
	if mv.To != "" {
		to = e.areaIdx(string(mv.To))
	}
	c := 0
	tax := true
	switch mv.Action {
	case actRecruit:
		c = 1
		if mv.Piece == uHighPriest {
			c = hpSpec.Cost
		}
	case actSummon:
		c = e.summonCost(seat, mv.Piece)
	case actAwaken:
		c = e.awakenCost(seat, mv.Piece)
		if g := replacedBy(mv); g != "" {
			// Cthugha: 6 minus what the Great Old One he replaces costs.
			c = e.awakenCost(seat, uCthugha) - e.awakenCost(seat, g)
		}
	case actGate:
		c = e.gateCostFor(seat)
	case actRecrim:
		c = 1
		tax = false
	case actUndim:
		c = 2
		tax = false
	case actGo, actLeave, actBattle, actCapture, actSubmerge, actScream, actShriek, actZingaya, actHWINN, actAvatar, actIceAge, actSorcery, actSlumber, actWake:
		c = 1
	case actFilth, actSting, actZygote:
		c = 1
	case actDreams, actGhroth:
		c = 2
	case actBeyond, actIdolatry:
		c = 1
	case actCurse:
		c = 2
		tax = false
	case actDesecrate:
		c = e.desecrateCost(seat)
	case actPay:
		c = mv.Count
		tax = false
	case actBoon, actBestow, actBane:
		c = 3
		tax = false
	case actEmerge:
		c = 0
	default:
		tax = false
	}
	if tax && to >= 0 {
		c += e.iceTax(seat, to)
	}
	return c
}

// --- legal moves: the turn ----------------------------------------------------

func (e *eng) legalTurn() []state.Move {
	d := e.d
	t := d.Turn
	seat := t.Seat
	p := d.Power[seat]
	var out []state.Move
	if t.Nexus {
		return e.legalNexus()
	}
	if p >= 1 {
		out = append(out, e.unlimitedActions(seat)...)
	} else {
		// Dragon Ascending may be played at any time, with no Power at all.
		out = append(out, e.ascendMoves(seat)...)
	}
	// Paying a Moonbeast off costs Doom, not Power, and is no Action.
	out = append(out, e.ransomMoves(seat)...)
	mainAvail := !t.Main || (t.Second && !t.SecondTaken)
	if mainAvail && p >= 1 {
		out = append(out, e.mainActions(seat)...)
	}
	if t.Main || e.summonedThisTurn(seat) {
		out = append(out, state.Move{Action: actDone})
	} else {
		out = append(out, state.Move{Action: actPass})
	}
	return out
}

// summonedThisTurn says the seat's turn already holds a Fertility Cult summon,
// which is an action all the same: the turn may end with Done. Besides the
// flag it reads the log, for a position saved before the flag existed: the
// seat's latest decision summoned a Monster and nothing but passes (decisions
// that say nothing) have been taken since.
func (e *eng) summonedThisTurn(seat int) bool {
	t := e.d.Turn
	if t == nil {
		return false
	}
	if t.Summoned {
		return true
	}
	if t.Main || !e.ability(seat, abFertility) {
		return false
	}
	log := e.d.Log
	for i := len(log) - 1; i >= 0; i-- {
		le := log[i]
		if le.Seat == seat {
			for _, line := range le.Lines {
				if strings.Contains(line, " summons ") {
					return true
				}
			}
			return false
		}
		if len(le.Lines) > 0 {
			return false // somebody did something since: a later turn
		}
	}
	return false
}

func (e *eng) unlimitedActions(seat int) []state.Move {
	d := e.d
	var out []state.Move
	out = append(out, e.controlMoves(seat)...)
	out = append(out, e.stepOffMoves(seat)...)
	if e.is(seat, facGC) && d.hasBook(seat, bkDevolve) && e.pool(seat, uDeepOne) > 0 {
		for a := range e.m.ids {
			if e.count(a, seat, uCultist) > 0 {
				out = append(out, state.Move{Action: actDevolve, To: e.m.ids[a]})
			}
		}
	}
	if len(d.Signs[seat]) > 0 {
		out = append(out, state.Move{Action: actReveal})
	}
	out = append(out, e.ascendMoves(seat)...)
	if e.ability(seat, abFertility) {
		out = append(out, e.summonMoves(seat)...)
	}
	if e.unlimitedBattle(seat) {
		out = append(out, e.battleMoves(seat)...)
	}
	// Windwalker's fifth spellbook is taken whenever its owner likes.
	if e.is(seat, facWW) && !d.reqDone(seat, rqWWTake) && d.bookCount(seat) < 6 {
		out = append(out, state.Move{Action: actClaim})
	}
	// Unspeakable Oath: at the end of any Action, so also after one's own.
	if e.oathAvailable(seat) {
		out = append(out, state.Move{Action: actOath})
	}
	return out
}

// worthwhileFreeAction reports whether seat has an unlimited action that
// should keep its turn open after the main action. Revealing Elder Signs does
// not: it can wait for the next turn or the Doom phase, and does not deserve a
// "Done" click after every action. The Oath and the Windwalker's spellbook can
// be taken at the start of a turn, so they do not hold it open either.
func (e *eng) worthwhileFreeAction(seat int) bool {
	for _, mv := range e.unlimitedActions(seat) {
		switch mv.Action {
		case actReveal, actOath, actClaim, actLeave, actAscend:
			continue
		case actControl:
			// Putting another unit on a Gate that is already one's own is a
			// matter of taste: only an abandoned Gate keeps the turn open.
			if ctl := e.d.Gates[string(mv.To)]; ctl != 0 {
				continue
			}
		}
		return true
	}
	return false
}

func (e *eng) summonMoves(seat int) []state.Move {
	var out []state.Move
	gates := e.summonAreas(seat)
	adulation := e.adulation(seat)
	for _, u := range e.units(seat) {
		if (u.Class != classMonster && u.Class != classTerror) || u.Token || e.pool(seat, u.ID) <= 0 {
			continue
		}
		// Adulation: while Servitors are in the pool, nothing else is summoned.
		if adulation && u.Class == classMonster && u.ID != uServitor {
			continue
		}
		// A Moonbeast needs an enemy spellbook to lie on.
		if u.ID == uMoonbeast && len(e.moonOptions(seat)) == 0 {
			continue
		}
		for _, a := range gates {
			mv := state.Move{Action: actSummon, Piece: u.ID, To: e.m.ids[a]}
			if e.d.Power[seat] < e.moveCost(seat, mv) {
				continue
			}
			out = append(out, mv)
		}
	}
	return out
}

func (e *eng) battleMoves(seat int) []state.Move {
	var out []state.Move
	for a := range e.m.ids {
		if e.hasBattled(seat, a) {
			continue
		}
		for _, enemy := range e.others(seat) {
			if e.canBattle(seat, enemy, a) {
				mv := state.Move{Action: actBattle, To: e.m.ids[a], Count: enemy}
				if e.d.Power[seat] < e.moveCost(seat, mv) {
					continue
				}
				out = append(out, mv)
			}
		}
	}
	return out
}

// captureMoves lists what seat may capture: a Cultist (or a High Priest) of
// an enemy that no unit of equal or higher rank guards, and — for the Sleeper
// with Capture Monster and Tsathoggua — a Monster.
func (e *eng) captureMoves(seat int) []state.Move {
	var out []state.Move
	for a := range e.m.ids {
		mine := e.protectionRank(a, seat)
		if mine == 0 {
			continue
		}
		for _, enemy := range e.others(seat) {
			if e.protectionRank(a, enemy) >= mine {
				continue
			}
			base := state.Move{Action: actCapture, To: e.m.ids[a], Count: enemy}
			if e.d.Power[seat] < e.moveCost(seat, base) {
				continue
			}
			// Ferox: while Ithaqua is in play only a Great Old One can capture
			// the Windwalker's Cultists.
			safe := e.is(enemy, facWW) && e.onMap(enemy, uIthaqua) > 0 && mine < 2
			if !safe {
				if n := e.count(a, enemy, uCultist); n > 0 {
					if n > 1 && e.gateHolderOf(a, enemy) == uCultist {
						// The Cultist on the Gate or one of those beside it:
						// the captor chooses.
						beside, onIt := base, base
						beside.Piece = uCultist
						onIt.Piece = onGate + uCultist
						out = append(out, beside, onIt)
					} else {
						out = append(out, base)
					}
				}
				if e.count(a, enemy, uHighPriest) > 0 {
					mv := base
					mv.Piece = uHighPriest
					out = append(out, mv)
				}
			}
			if e.is(seat, facSL) && e.d.hasBook(seat, bkCaptureMonster) && e.count(a, seat, uTsath) > 0 {
				for _, typ := range e.typesIn(a, enemy) {
					if unitClassOf(typ) == classMonster {
						mv := base
						mv.Piece = typ
						out = append(out, mv)
					}
				}
			}
		}
	}
	return out
}

func (e *eng) mainActions(seat int) []state.Move {
	d := e.d
	p := d.Power[seat]
	var out []state.Move
	second := d.Turn.Main // a second action after He Who is Not to be Named
	f := e.f(seat)
	add := func(mv state.Move) {
		if p >= e.moveCost(seat, mv) {
			out = append(out, mv)
		}
	}

	// Recruit (an Acolyte, or the High Priest).
	recruit := func(piece string) {
		if e.pool(seat, piece) <= 0 {
			return
		}
		mv := state.Move{Action: actRecruit}
		if piece != uCultist {
			mv.Piece = piece
		}
		if e.hasAnyUnits(seat) {
			for a := range e.m.ids {
				if e.total(a, seat) > 0 {
					mv.To = e.m.ids[a]
					add(mv)
				}
			}
		} else {
			for a := range e.m.ids {
				mv.To = e.m.ids[a]
				add(mv)
			}
		}
	}
	recruit(uCultist)
	if e.hpFor(seat) {
		recruit(uHighPriest)
	}
	// Summon (the Black Goat does it as an unlimited action).
	if !e.ability(seat, abFertility) {
		out = append(out, e.summonMoves(seat)...)
	}
	// Awaken.
	for _, u := range f.Units {
		if u.Class != classGOO {
			continue
		}
		for _, a := range e.awakenAreas(seat, u.ID) {
			add(state.Move{Action: actAwaken, Piece: u.ID, To: e.m.ids[a]})
		}
	}
	// Awaken an Independent Great Old One.
	for _, mv := range e.independentAwakenMoves(seat) {
		add(mv)
	}
	// Create a Gate.
	for a := range e.m.ids {
		if e.hasGate(a) {
			continue
		}
		if e.cultists(a, seat) > 0 || (e.is(seat, facBG) && d.hasBook(seat, bkRedSign) && e.count(a, seat, uYoung) > 0) {
			add(state.Move{Action: actGate, To: e.m.ids[a]})
		}
	}
	// Move: the first unit moved begins the Move action (and every other
	// unit may follow it); there is no separate "start moving" decision to
	// choose and then abandon, so a turn cannot idle in a no-op loop.
	for _, mv := range e.unitMoves(seat, nil) {
		add(mv)
	}
	// Battle (unlimited for a faction with six spellbooks).
	if !e.unlimitedBattle(seat) {
		out = append(out, e.battleMoves(seat)...)
	}
	out = append(out, e.captureMoves(seat)...)
	out = append(out, e.factionActions(seat, second)...)
	out = append(out, e.recriminateMoves(seat)...)
	out = append(out, e.independentActions(seat)...)
	if e.undimAvailable(seat) {
		out = append(out, state.Move{Action: actUndim})
	}
	return out
}

// unitMoves lists single-unit moves for seat. The unit that stands on a Gate
// of the seat is one of its own, apart from the rest of its kind there: it
// moves with leave (to any area in reach, or to the area it stands in), which
// abandons the Gate; the others move with go and leave the Gate alone.
func (e *eng) unitMoves(seat int, moved []string) []state.Move {
	var out []state.Move
	for a := range e.m.ids {
		occupant := ""
		if e.d.Gates[e.areaID(a)] == seat {
			occupant = e.holderAt(a)
		}
		for t, typ := range unitTypes {
			n := e.cnt[e.idx(a, seat, t)]
			// The Hound of Tindalos moves only with the others (houndHops) and the
			// Filth never moves at all.
			if n <= 0 || typ == uHound || typ == uFilth {
				continue
			}
			// Units that arrived during this action may not move again.
			arrived := 0
			if len(moved) > 0 {
				key := e.areaID(a) + "|" + typ
				for _, k := range moved {
					if k == key {
						arrived++
					}
				}
			}
			free := n - arrived
			if free <= 0 {
				continue
			}
			stack := free
			if typ == occupant {
				// One of them stands on the Gate; the rest stand beside it.
				stack = n - 1 - arrived
			}
			if stack > 0 {
				for _, to := range e.reachFor(seat, a, typ) {
					out = append(out, state.Move{Action: actGo, Piece: typ, From: e.m.ids[a], To: e.m.ids[to]})
				}
			}
			if typ == occupant {
				// (Stepping off the Gate in place is not a move: stepOffMoves.)
				for _, to := range e.reachFor(seat, a, typ) {
					out = append(out, state.Move{Action: actLeave, Piece: typ, From: e.m.ids[a], To: e.m.ids[to]})
				}
			}
		}
	}
	return out
}

// stepOffMoves lists the free actions that give a Gate up without moving: the
// unit that stands on a Gate of the seat steps off it, where it is. A Gate
// changes hands once in a turn (Turn.Toggled), so the free actions cannot go
// round for ever; moving the unit away is a Move step and costs Power.
func (e *eng) stepOffMoves(seat int) []state.Move {
	var out []state.Move
	for a, aid := range e.m.ids {
		id := string(aid)
		if e.d.Gates[id] != seat {
			continue
		}
		if e.d.Turn != nil && containsStr(e.d.Turn.Toggled, id) {
			continue
		}
		if occupant := e.holderAt(a); occupant != "" {
			out = append(out, state.Move{Action: actLeave, Piece: occupant, From: aid, To: aid})
		}
	}
	return out
}

// stepOff gives the Gate of the area up: its occupant stays where it is. It
// is free, it is no action and it ends nothing.
func (e *eng) stepOff(seat int, mv state.Move) {
	a := e.areaIdx(string(mv.From))
	e.vacate(a)
	if e.d.Turn != nil {
		e.d.Turn.Toggled = append(e.d.Turn.Toggled, e.areaID(a))
	}
	e.say("%s steps %s off the Gate in %s: the Gate is abandoned.", e.seatName(seat), article(unitNameOf(mv.Piece)), e.areaName(a))
}

// controlMoves lists the free actions that put a unit on a Gate: a unit of
// seat that stands in the area of an abandoned Gate takes control of it, and
// a unit of another kind may take the place of the one on a Gate of its own.
func (e *eng) controlMoves(seat int) []state.Move {
	var out []state.Move
	// Areas in map order: the list of moves is deterministic.
	for a, aid := range e.m.ids {
		id := string(aid)
		ctl, ok := e.d.Gates[id]
		if !ok || (ctl != 0 && ctl != seat) {
			continue
		}
		occupant := ""
		if ctl == seat {
			occupant = e.holderAt(a)
		}
		// Taking an abandoned Gate is always on offer; replacing the unit on
		// one's own is a change of hands too: once in a turn.
		if ctl == seat && e.d.Turn != nil && containsStr(e.d.Turn.Toggled, id) {
			continue
		}
		for _, typ := range e.holderTypes(seat) {
			if typ != occupant && e.count(a, seat, typ) > 0 {
				out = append(out, state.Move{Action: actControl, Piece: typ, To: aid})
			}
		}
	}
	return out
}

// stepCost is the Power one more unit of a Move action costs: 1 (plus the Ice
// Age surcharge once per Move), nothing for a unit that follows Ithaqua with
// Arctic Wind.
func (e *eng) stepCost(seat int, mv state.Move) int {
	d := e.d
	// Chronophage: the Hound hops for nothing; a Shantak carries a Cultist along.
	if mv.Piece == uHound || e.carried(mv) >= 0 {
		return 0
	}
	if m := d.Move; m != nil {
		if m.WindTo != "" && string(mv.From) == m.WindFrom && string(mv.To) == m.WindTo {
			return 0
		}
		if m.Taxed {
			return 1
		}
	}
	return 1 + e.iceTax(seat, e.areaIdx(string(mv.To)))
}

// moveOver says the Move action has nothing left to pay for: no unit can move
// any more. (Free actions — a unit put on a Gate, a step off one — are no
// reason to keep it open: they are there after it, too.)
func (e *eng) moveOver() bool {
	for _, mv := range e.legalMoving() {
		switch {
		case mv.Action == actDone, mv.Action == actControl:
			continue
		case mv.Action == actLeave && mv.From == mv.To:
			continue
		}
		return false
	}
	return true
}

func (e *eng) legalMoving() []state.Move {
	d := e.d
	seat := d.Turn.Seat
	var out []state.Move
	keyOf := func(mv state.Move) string {
		return mv.Action + "|" + mv.Piece + "|" + string(mv.From) + ">" + string(mv.To)
	}
	var seen map[string]bool
	moves := e.unitMoves(seat, d.Move.Moved)
	if carry := e.carryMoves(seat); len(carry) > 0 {
		// A step that is both an ordinary move and a free ride is listed once.
		seen = map[string]bool{}
		for _, mv := range moves {
			seen[keyOf(mv)] = true
		}
		for _, mv := range carry {
			if !seen[keyOf(mv)] {
				moves = append(moves, mv)
			}
		}
	}
	for _, mv := range moves {
		if d.Move.Origin != "" && string(mv.From) != d.Move.Origin {
			continue
		}
		if d.Power[seat] >= e.stepCost(seat, mv) {
			out = append(out, mv)
		}
	}
	// Chronophage: the Hound hops from Gate to Gate for nothing.
	out = append(out, e.houndHops(seat)...)
	// Putting a unit on an abandoned Gate, or stepping off one, is free: it
	// goes with the Move, not instead. (Replacing the unit on a Gate of
	// one's own waits for the turn.)
	if d.Move.Origin == "" {
		for _, mv := range e.controlMoves(seat) {
			if d.Gates[string(mv.To)] == 0 {
				out = append(out, mv)
			}
		}
		out = append(out, e.stepOffMoves(seat)...)
	}
	return append(out, state.Move{Action: actDone})
}

// --- executing ------------------------------------------------------------------

// mainTaken records that the turn's main (or second) action was taken.
func (e *eng) mainTaken() {
	t := e.d.Turn
	e.acted()
	if t.Main && t.Second {
		t.SecondTaken = true
		return
	}
	t.Main = true
}

// pay takes the Power an action costs from seat.
func (e *eng) pay(seat int, mv state.Move) int {
	c := e.moveCost(seat, mv)
	e.d.Power[seat] -= c
	if e.d.Power[seat] < 0 {
		e.d.Power[seat] = 0
	}
	return c
}

func (e *eng) doTurn(seat int, mv state.Move) {
	d := e.d
	to := -1
	if mv.To != "" {
		to = e.areaIdx(string(mv.To))
	}
	switch mv.Action {
	case actPass:
		e.say("%s passes, dropping to 0 Power.", e.seatName(seat))
		d.Power[seat] = 0
		e.endTurn()
		return
	case actDone:
		e.endTurn()
		return
	case actRecruit:
		typ := uCultist
		if mv.Piece != "" {
			typ = mv.Piece
		}
		e.pay(seat, mv)
		e.place(to, seat, typ)
		if typ == uCultist {
			e.say("%s recruits a Cultist in %s.", e.seatName(seat), e.areaName(to))
		} else {
			e.say("%s recruits its %s in %s.", e.seatName(seat), unitNameOf(typ), e.areaName(to))
		}
		e.takeAbandonedGate(to, seat)
		e.mainTaken()
	case actSummon:
		cost := e.pay(seat, mv)
		switch mv.Piece {
		case uShambler:
			// Walk Between Worlds: it goes on the faction card and is put on
			// the map after an Action.
			e.setPool(seat, uShambler, e.pool(seat, uShambler)-1)
			d.Card[seat]++
			e.say("%s summons a Dimensional Shambler at the Gate in %s (%d Power): it waits on their faction card.", e.seatName(seat), e.areaName(to), cost)
		case uMoonbeast:
			// Blasphemous Obeisance: it goes on an enemy's spellbook, not on the map.
			e.say("%s summons a Moonbeast at the Gate in %s (%d Power).", e.seatName(seat), e.areaName(to), cost)
			e.moonBegin(seat)
		default:
			e.place(to, seat, mv.Piece)
			e.say("%s summons %s in %s (%d Power).", e.seatName(seat), article(unitNameOf(mv.Piece)), e.areaName(to), cost)
		}
		if !e.ability(seat, abFertility) {
			e.mainTaken()
		} else {
			// An unlimited action, but an action all the same: the turn may
			// end here (Done) with this one Monster.
			d.Turn.Summoned = true
			e.acted()
		}
	case actAwaken:
		if isIndependent(mv.Piece) {
			e.awakenIndependent(seat, mv)
			// Immortal: any Great Old One awakened gives Great Cthulhu an Elder Sign.
			if e.ability(seat, abImmortal) {
				e.say("Immortal: %s gains an Elder Sign for awakening %s.", e.seatName(seat), unitNameOf(mv.Piece))
				e.gainSign(seat)
			}
			e.mainTaken()
			break
		}
		e.awaken(seat, mv.Piece, to)
	case actGate:
		e.pay(seat, mv)
		e.occupy(to, seat, "")
		e.say("%s builds a Gate in %s.", e.seatName(seat), e.areaName(to))
		e.mainTaken()
	case actLeave:
		if mv.From != mv.To {
			e.beginMove(seat, mv)
			return
		}
		// Stepping off the Gate where the unit stands: free, no Move.
		e.stepOff(seat, mv)
	case actGo:
		e.beginMove(seat, mv)
		return
	case actBattle:
		e.pay(seat, mv)
		e.startBattle(seat, mv.Count, to, !e.unlimitedBattle(seat))
		return
	case actCapture:
		e.pay(seat, mv)
		e.capture(seat, mv.Count, to, mv.Piece)
		e.mainTaken()
	case actControl:
		e.putOnGate(seat, mv)
	case actDevolve:
		e.devolve(seat, to)
	case actReveal:
		e.reveal(seat)
	case actOath:
		e.sacrificePriest(seat)
	case actAscend:
		e.ascend(seat, mv.Count)
	case actRecrim:
		e.recriminate(seat, mv)
		return
	case actUndim:
		e.beginUndim(seat)
		return
	case actRansom:
		e.ransom(seat, mv)
	case actFilth:
		e.filth(seat, to)
	case actSting:
		e.sting(seat, to)
		return
	case actZygote:
		e.pay(seat, mv)
		n := e.pool(seat, uCultist)
		e.say("The Zygote: %s places %s.", e.seatName(seat), plural(n, "Acolyte", "Acolytes"))
		d.Prompts = append(d.Prompts, Prompt{Kind: "zygote", Seat: seat, Left: n})
		e.mainTaken()
		e.thenAfterPrompts("after")
		e.beginPrompts("after")
		return
	case actDiscard:
		e.discard(seat, mv)
	case actGateOff:
		e.gateOff(seat, to)
	case actPay:
		d.Power[seat] -= mv.Count
		e.say("%s pays %d Power to the Outer Gods.", e.seatName(seat), mv.Count)
		switch mv.Count {
		case 4:
			e.fulfil(seat, rqCCPay4)
		case 6:
			e.fulfil(seat, rqCCPay6)
		default:
			// Both requirements at once.
			e.fulfil(seat, rqCCPay4)
			e.fulfil(seat, rqCCPay6)
		}
		e.mainTaken()
	case actSacrifice:
		e.say("%s offers two Cultists in sacrifice.", e.seatName(seat))
		d.Prompts = append(d.Prompts, Prompt{Kind: "lose", Seat: seat, Why: "sacrifice", Left: 2, CultistsOnly: true})
		e.fulfil(seat, rqBGSac)
		e.mainTaken()
		e.thenAfterPrompts("after")
		e.beginPrompts("sacrifice")
		return
	default:
		e.factionAction(seat, mv, to)
	}
	if e.routed || d.Stage == stagePrompt || d.Phase == phaseOver {
		return
	}
	e.afterAction()
}

// doSetup places Windwalker's Gate and Acolytes in the sigil area it chose.
func (e *eng) doSetup(seat int, mv state.Move) {
	d := e.d
	a := e.areaIdx(string(mv.To))
	id := e.areaID(a)
	cultists := 6
	switch {
	case e.is(seat, facWW):
		d.WWStart = id
	case e.is(seat, facTT):
		d.TTStart = id
		// With the High Priests option one of the six Acolytes is a High Priest.
		if e.hasHP() {
			cultists = 5
			e.place(a, seat, uHighPriest)
		}
	}
	for i := 0; i < cultists; i++ {
		e.place(a, seat, uCultist)
	}
	e.occupy(a, seat, uCultist)
	e.say("%s begins in %s.", e.seatName(seat), e.areaName(a))
	d.Setup = 0
	if len(d.SetupLater) > 0 {
		d.Setup, d.SetupLater = d.SetupLater[0], d.SetupLater[1:]
		return
	}
	d.Stage = stageDirection
}

// beginFirstAction starts the first Action phase, once the first player has
// chosen the direction of play.
func (e *eng) beginFirstAction() {
	d := e.d
	d.Opening = false
	d.Phase = phaseAction
	d.Stage = stageTurn
	d.Turn = &TurnState{Seat: d.First}
	e.say("— Round %d —", d.Round)
	e.continueTurn()
}

// beginMove starts a Move action with its first step.
func (e *eng) beginMove(seat int, mv state.Move) {
	d := e.d
	d.Stage = stageMoving
	d.Move = &MoveState{}
	if d.Turn.Nexus && d.Bat != nil {
		// An Energy Nexus Move starts in the area of the battle.
		d.Move.Origin = d.Bat.Area
	}
	e.doMoving(seat, mv)
}

// putOnGate is the free action that puts a unit of seat that stands in an area
// onto its Gate: the Gate was abandoned, or the unit takes the place of the
// one on it. It does not end the turn, nor a Move action in progress.
func (e *eng) putOnGate(seat int, mv state.Move) {
	d := e.d
	to := e.areaIdx(string(mv.To))
	id := e.areaID(to)
	abandoned := d.Gates[id] == 0
	e.occupy(to, seat, mv.Piece)
	// The Gate has changed hands this turn: it may not again, so that the free
	// actions cannot go round for ever.
	if d.Turn != nil && !containsStr(d.Turn.Toggled, id) {
		d.Turn.Toggled = append(d.Turn.Toggled, id)
	}
	name := unitNameOf(e.holderAt(to))
	if abandoned {
		e.say("%s takes control of the Gate in %s with %s.", e.seatName(seat), e.areaName(to), article(name))
	} else {
		e.say("%s puts %s on the Gate in %s.", e.seatName(seat), article(name), e.areaName(to))
	}
}

// doMoving applies one step of a Move action.
func (e *eng) doMoving(seat int, mv state.Move) {
	d := e.d
	if mv.Action == actDone {
		e.finishMove()
		return
	}
	if mv.Action == actControl {
		// Free, and in the middle of a Move: a unit goes onto a Gate.
		e.putOnGate(seat, mv)
		if e.moveOver() {
			e.finishMove()
		}
		return
	}
	if mv.Action == actLeave && mv.From == mv.To {
		// Free, and in the middle of a Move: the unit steps off its Gate.
		e.stepOff(seat, mv)
		if e.moveOver() {
			e.finishMove()
		}
		return
	}
	from, to := e.areaIdx(string(mv.From)), e.areaIdx(string(mv.To))
	cost := e.stepCost(seat, mv)
	if cost > 0 && e.iceTax(seat, to) > 0 {
		d.Move.Taxed = true
	}
	// A Cultist that rides a Shantak uses up the seat.
	if i := e.carried(mv); i >= 0 {
		d.Move.Carry = append(d.Move.Carry[:i], d.Move.Carry[i+1:]...)
	}
	d.Power[seat] -= cost
	d.Move.Spent += cost
	d.Move.Moved = append(d.Move.Moved, e.areaID(to)+"|"+mv.Piece)
	// Horror Steed: the Shantak may carry a Cultist of the area it left, for free.
	if mv.Piece == uShantak {
		d.Move.Carry = append(d.Move.Carry, string(mv.From)+">"+string(mv.To))
	}
	if mv.Action == actLeave {
		// The unit that stands on the Gate moves away, and the Gate is
		// abandoned, whoever else stands there. It is not taken again by
		// itself this turn.
		e.take(from, seat, mv.Piece)
		e.add(to, seat, mv.Piece)
		e.vacate(from)
		d.Turn.Toggled = append(d.Turn.Toggled, e.areaID(from))
		e.say("%s moves %s off the Gate in %s to %s: the Gate is abandoned.", e.seatName(seat), article(unitNameOf(mv.Piece)), e.areaName(from), e.areaName(to))
		e.takeAbandonedGate(to, seat)
		if e.moveOver() {
			e.finishMove()
		}
		return
	}
	e.take(from, seat, mv.Piece)
	e.add(to, seat, mv.Piece)
	// Arctic Wind: the units that stand with Ithaqua may follow him.
	if mv.Piece == uIthaqua && e.is(seat, facWW) && d.hasBook(seat, bkArctic) {
		d.Move.WindFrom, d.Move.WindTo = e.areaID(from), e.areaID(to)
	}
	e.say("%s moves %s from %s to %s.", e.seatName(seat), article(unitNameOf(mv.Piece)), e.areaName(from), e.areaName(to))
	e.syncGate(from)
	e.takeAbandonedGate(to, seat)
	if e.moveOver() {
		e.finishMove()
	}
}

func (e *eng) finishMove() {
	d := e.d
	seat := d.Turn.Seat
	spent := d.Move != nil && (d.Move.Spent > 0 || len(d.Move.Moved) > 0)
	paid := 0
	if d.Move != nil {
		paid = d.Move.Spent
	}
	d.Move = nil
	d.Stage = stageTurn
	if spent {
		// Burrow: after a Move that cost 2 or more Power the Sleeper regains 1.
		if e.is(seat, facSL) && d.hasBook(seat, bkBurrow) && paid >= 2 {
			d.Power[seat]++
			e.say("Burrow: %s regains 1 Power.", e.seatName(seat))
		}
		e.mainTaken()
		e.afterAction()
	}
}

// afterAction continues the turn once an action has fully resolved.
func (e *eng) afterAction() {
	d := e.d
	e.syncAllGates()
	e.angles()
	e.checkRequirements()
	if len(d.Prompts) > 0 {
		e.thenAfterPrompts("turn")
		e.beginPrompts("turn")
		return
	}
	e.continueTurn()
}

// continueTurn decides what the active player may do next.
func (e *eng) continueTurn() {
	d := e.d
	seat := d.Turn.Seat
	d.Stage = stageTurn
	if d.Turn.Nexus {
		// An Energy Nexus Action is over once it is taken.
		if d.Turn.Main {
			e.endTurn()
		}
		return
	}
	if (d.Power[seat] <= 0 && len(e.ascendMoves(seat)) == 0) || d.Hib[seat] {
		e.endTurn()
		return
	}
	if d.Turn.Main && (!d.Turn.Second || d.Turn.SecondTaken) {
		// Only unlimited actions remain: end the turn if none is worth
		// waiting for.
		if !e.worthwhileFreeAction(seat) {
			e.endTurn()
			return
		}
	}
	// Nothing at all to do: the player's remaining Power is lost.
	moves := e.legalTurn()
	if len(moves) == 1 && moves[0].Action == actPass {
		e.say("%s has no legal action and drops to 0 Power.", e.seatName(seat))
		d.Power[seat] = 0
		e.endTurn()
		return
	}
	// A Black Goat that summoned a Monster and has nothing left to do: Done.
	if len(moves) == 1 && moves[0].Action == actDone && !d.Turn.Main {
		e.endTurn()
	}
}

// endTurn passes play to the next player with Power, or ends the phase.
func (e *eng) endTurn() {
	d := e.d
	if d.Turn != nil && d.Turn.Nexus {
		e.endNexus()
		return
	}
	if d.EndPending && d.Phase == phaseAction {
		e.finishGame("doom")
		return
	}
	seat := d.Turn.Seat
	d.Move = nil
	for k := 1; k <= e.n; k++ {
		s := ((seat-1)+k*d.Dir)%e.n + 1
		if s < 1 {
			s += e.n
		}
		if d.Power[s] > 0 && !d.Hib[s] {
			d.Turn = &TurnState{Seat: s}
			d.Stage = stageTurn
			e.continueTurn()
			return
		}
	}
	// Nobody has Power: before the phase ends a player with no Power left may
	// still sacrifice a High Priest after the last Action, and go on acting.
	if d.ReactOwed != 0 && d.Phase == phaseAction {
		e.openReactions("endturn")
		if len(d.Prompts) > 0 {
			return
		}
	}
	e.endActionPhase()
}

// --- the individual actions ---------------------------------------------------

func (e *eng) awaken(seat int, typ string, a int) {
	d := e.d
	cost := e.pay(seat, state.Move{Action: actAwaken, Piece: typ, To: e.m.ids[a]})
	if typ == uIthaqua {
		// He takes the place of the Gate.
		delete(d.Gates, e.areaID(a))
		delete(d.Holders, e.areaID(a))
	}
	switch typ {
	case uYog:
		e.eliminateQuiet(a, seat, uSpawn) // the Spawn is replaced
	case uUbbo:
		e.eliminatePriest(seat)
	}
	if !e.place(a, seat, typ) {
		return
	}
	e.say("%s awakens %s in %s (%d Power).", e.seatName(seat), unitNameOf(typ), e.areaName(a), cost)
	d.markAwoke(seat, typ)
	switch typ {
	case uShub:
		d.Prompts = append(d.Prompts, Prompt{Kind: "lose", Seat: seat, Why: "awaken Shub-Niggurath", Left: 2, CultistsOnly: true})
		e.fulfil(seat, rqBGAwaken)
		e.mainTaken()
		e.thenAfterPrompts("turn")
		e.beginPrompts("turn")
		return
	case uCthulhu:
		e.fulfil(seat, rqGCAwaken)
	case uNyar:
		e.fulfil(seat, rqCCAwaken)
	case uKing:
		e.fulfil(seat, rqYSKing)
	case uHastur:
		e.fulfil(seat, rqYSHastur)
	case uRhan:
		e.fulfil(seat, rqWWRhan)
	case uIthaqua:
		e.fulfil(seat, rqWWIthaqua)
	case uTsath:
		e.fulfil(seat, rqSLAwaken)
	case uYog:
		e.fulfil(seat, rqOWAwaken)
	case uUbbo:
		e.fulfil(seat, rqTTAwaken)
	}
	// Immortal: whenever the cult awakens any Great Old One it gains an
	// Elder Sign.
	if e.ability(seat, abImmortal) {
		e.say("Immortal: %s gains an Elder Sign for awakening %s.", e.seatName(seat), unitNameOf(typ))
		e.gainSign(seat)
	}
	e.mainTaken()
}

// gateHolderOf is the type of the unit of seat that occupies the Gate of area
// a ("" when the Gate is abandoned, is another cult's or there is none).
func (e *eng) gateHolderOf(a, seat int) string {
	if ctl, ok := e.d.Gates[e.areaID(a)]; !ok || ctl != seat {
		return ""
	}
	return e.holderAt(a)
}

// capture takes a Cultist (piece "" or the High Priest) or, for the Sleeper's
// Capture Monster, a Monster from an enemy in a. A Cultist of the stack is
// taken before the one on the Gate unless piece names the latter
// (onGate+Cultist); a Gate whose occupant is captured stays abandoned, and the
// captor's own Cultist (or High Priest) steps onto it at once.
func (e *eng) capture(seat, enemy, a int, piece string) {
	occ := e.gateHolderOf(a, enemy)
	fromGate := false
	switch piece {
	case "", uCultist, onGate + uCultist:
		fromGate = piece == onGate+uCultist || (occ == uCultist && e.count(a, enemy, uCultist) == 1)
		e.take(a, enemy, uCultist)
		e.d.Captured[seat][enemy]++
		if fromGate {
			e.say("%s captures the %s Cultist that held the Gate in %s.", e.seatName(seat), e.seatName(enemy), e.areaName(a))
		} else {
			e.say("%s captures a %s Cultist in %s.", e.seatName(seat), e.seatName(enemy), e.areaName(a))
		}
	case uHighPriest:
		fromGate = occ == uHighPriest
		e.take(a, enemy, uHighPriest)
		e.d.CapHP[seat][enemy]++
		e.say("%s captures the %s High Priest in %s.", e.seatName(seat), e.seatName(enemy), e.areaName(a))
	default:
		e.take(a, enemy, piece)
		e.d.Held = append(e.d.Held, PU{Seat: enemy, Type: piece})
		e.say("Capture Monster: %s captures %s of %s in %s.", e.seatName(seat), article(unitNameOf(piece)), e.seatName(enemy), e.areaName(a))
		e.syncGate(a)
		return
	}
	if fromGate {
		e.vacate(a)
		e.say("The Gate in %s is abandoned.", e.areaName(a))
		if e.canControlGate(seat, a) {
			e.occupy(a, seat, "")
			e.say("%s takes control of the Gate in %s.", e.seatName(seat), e.areaName(a))
		}
	} else {
		e.syncGate(a)
	}
	e.passion(enemy)
	if e.is(seat, facCC) {
		e.fulfil(seat, rqCCCapture)
	}
}

// reveal turns every Elder Sign of seat face up: they become public, their
// values are added to its Doom and its score is known to all.
func (e *eng) reveal(seat int) {
	d := e.d
	signs := d.Signs[seat]
	if len(signs) == 0 {
		return
	}
	total := 0
	for _, v := range signs {
		total += v
	}
	d.Signs[seat] = nil
	d.Shown[seat] = append(d.Shown[seat], signs...)
	before := d.Doom[seat]
	e.addDoom(seat, total, doomSigns)
	vals := make([]string, len(signs))
	for i, v := range signs {
		vals[i] = fmt.Sprint(v)
	}
	e.say("%s reveals %s: %s — %d Doom (%d → %d on the Doom track).", e.seatName(seat), plural(len(signs), "Elder Sign", "Elder Signs"), strings.Join(vals, ", "), total, before, d.Doom[seat])
	// Stars Are Right: the same number again as Power, in the Doom phase.
	if d.Phase == phaseDoom && d.hasNeutral(seat, nbStars) {
		d.Power[seat] += total
		e.say("Stars Are Right: %s also receives %s (now %d).", e.seatName(seat), plural(total, "Power", "Power"), d.Power[seat])
	}
	if d.Doom[seat] >= e.game.ParamInt("doom_to_win", 30) {
		d.EndPending = true
	}
}

// plural renders "1 Elder Sign" or "3 Elder Signs".
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// gainSign draws an Elder Sign for seat (or a single Doom point when the
// bag is empty).
func (e *eng) gainSign(seat int) {
	d := e.d
	total := d.Bag[0] + d.Bag[1] + d.Bag[2]
	if total == 0 {
		e.addDoom(seat, 1, doomSigns)
		e.say("%s gains 1 Doom instead of an Elder Sign (none are left): %d on the Doom track.", e.seatName(seat), d.Doom[seat])
		return
	}
	// Curse of Chaugnar Faugn: the signs in the pool are face up, and its holder
	// picks the one that is given (see openPicks), unless there is no choice.
	if owner := e.curseOwner(); owner != 0 && len(e.pickValues()) > 1 {
		d.Signs[seat] = append(d.Signs[seat], 0)
		d.Picks = append(d.Picks, seat)
		e.say("%s receives an Elder Sign: %s chooses which (Curse of Chaugnar Faugn).", e.seatName(seat), e.seatName(owner))
		if e.is(seat, facTT) {
			e.fulfil(seat, rqTTSign)
		}
		return
	}
	r := int(uint64(uint32(nextRandomFrom(e))) % uint64(total))
	v := 1
	for i := 0; i < 3; i++ {
		if r < d.Bag[i] {
			v = i + 1
			d.Bag[i]--
			break
		}
		r -= d.Bag[i]
	}
	d.Signs[seat] = append(d.Signs[seat], v)
	e.say("%s receives an Elder Sign.", e.seatName(seat))
	if e.is(seat, facTT) {
		e.fulfil(seat, rqTTSign)
	}
}

func nextRandomFrom(e *eng) uint32 {
	if e.d.RNG == 0 {
		e.d.RNG = freshSeed()
	}
	return uint32(nextRandom(&e.d.RNG) >> 32)
}

func (d *Data) markAwoke(seat int, typ string) {
	idx := 0
	if typ == uHastur || typ == uIthaqua {
		idx = 1
	}
	if d.Awoken == nil {
		d.Awoken = map[string]bool{}
	}
	d.Awoken[fmt.Sprintf("%d|%d", seat, idx)] = true
}

// Awoke reports whether seat's n-th Great Old One has been awakened before.
func (d *Data) Awoke(seat, n int) bool { return d.Awoken[fmt.Sprintf("%d|%d", seat, n)] }
