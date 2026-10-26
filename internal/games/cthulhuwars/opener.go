package cthulhuwars

import (
	"sort"

	"github.com/noderunner/bingle/internal/engine/state"
)

// The Opener of the Way (Onslaught One): Gate manipulation and upgrading
// units. Its Gates are moved by The Beyond One, Yog-Sothoth is a Gate himself,
// and its spellbooks reroll dice, reset Power, double Doom, curse an area,
// promote units after a battle and summon at the enemy's Gates.

// --- Yog-Sothoth: The Key and the Gate ---------------------------------------------------

// yogIn says the Opener's Yog-Sothoth stands in area a: he counts as a Gate.
func (e *eng) yogIn(a int) bool {
	ow := e.seatOf(facOW)
	return ow != 0 && e.count(a, ow, uYog) > 0
}

// gatesOnMap counts the Gates on the map for the Opener's requirements:
// every Gate token and Yog-Sothoth, who is one.
func (e *eng) gatesOnMap() int {
	n := len(e.d.Gates)
	ow := e.seatOf(facOW)
	if ow != 0 {
		for a := range e.m.ids {
			if e.count(a, ow, uYog) > 0 {
				n++
			}
		}
	}
	return n
}

// yogCombat is Yog-Sothoth's Combat: twice the number of enemy-controlled
// Great Old Ones in play.
func (e *eng) yogCombat(seat int) int {
	n := 0
	for _, o := range e.others(seat) {
		n += e.gooTotal(o)
	}
	return 2 * n
}

// --- setup ---------------------------------------------------------------------------------

// emptyAreas lists the areas with no unit and no Gate: where the Opener and
// the Tcho-Tcho may begin.
func (e *eng) emptyAreas() []int {
	var out []int
	for a := range e.m.ids {
		if e.hasGate(a) {
			continue
		}
		empty := true
		for _, s := range e.playing() {
			if e.total(a, s) > 0 {
				empty = false
				break
			}
		}
		if empty {
			out = append(out, a)
		}
	}
	return out
}

// --- The Beyond One ---------------------------------------------------------------------------

// beyondMoves lists the ways seat may use The Beyond One: one of its units
// with a cost of 3 or more, in an area with a Gate and no enemy Great Old One,
// goes with that Gate (and whoever controls it) to an area without a Gate.
// Yog-Sothoth is a Gate for everything but this.
func (e *eng) beyondMoves(seat int) []state.Move {
	d := e.d
	var out []state.Move
	var free []int
	for a := range e.m.ids {
		if _, ok := d.Gates[e.areaID(a)]; !ok {
			free = append(free, a)
		}
	}
	for a := range e.m.ids {
		if _, ok := d.Gates[e.areaID(a)]; !ok {
			continue
		}
		guarded := false
		for _, o := range e.others(seat) {
			if e.classCount(a, o, classGOO) > 0 {
				guarded = true
				break
			}
		}
		if guarded {
			continue
		}
		for _, typ := range e.typesIn(a, seat) {
			if e.unitCost(seat, typ) < 3 {
				continue
			}
			for _, to := range free {
				out = append(out, state.Move{Action: actBeyond, Piece: typ, From: e.m.ids[a], To: e.m.ids[to]})
			}
		}
	}
	return out
}

// beyond moves the unit, the Gate and the unit that controls it.
func (e *eng) beyond(seat int, mv state.Move) {
	d := e.d
	from, to := e.areaIdx(string(mv.From)), e.areaIdx(string(mv.To))
	fid, tid := e.areaID(from), e.areaID(to)
	ctl := d.Gates[fid]
	holder := e.holderAt(from)
	selectedHolds := ctl == seat && holder == mv.Piece
	e.take(from, seat, mv.Piece)
	e.add(to, seat, mv.Piece)
	moved := article(unitNameOf(mv.Piece))
	if ctl != 0 && holder != "" && !selectedHolds {
		if e.take(from, ctl, holder) {
			e.add(to, ctl, holder)
			moved += " and " + article(unitNameOf(holder))
		}
	}
	delete(d.Gates, fid)
	delete(d.Holders, fid)
	d.Gates[tid] = ctl
	if ctl != 0 && holder != "" {
		d.Holders[tid] = holder
	}
	e.say("The Beyond One: %s takes %s and the Gate from %s to %s.", e.seatName(seat), moved, e.areaName(from), e.areaName(to))
	// What the move left behind may have lost the Gate it guarded.
	e.syncGate(to)
}

// --- Dragon Ascending and Dragon Descending ------------------------------------------------------

func (d *Data) spent(seat, bit int) bool { return has(d.Used[seat], bit) }

// ascendMoves: once only, set Power to an enemy's (only when it is more).
func (e *eng) ascendMoves(seat int) []state.Move {
	d := e.d
	if !d.hasBook(seat, bkAscend) || !e.is(seat, facOW) || d.spent(seat, 0) {
		return nil
	}
	var out []state.Move
	for _, o := range e.others(seat) {
		if d.Power[o] > d.Power[seat] {
			out = append(out, state.Move{Action: actAscend, Count: o})
		}
	}
	return out
}

func (e *eng) ascend(seat, enemy int) {
	d := e.d
	d.Used[seat] |= 1
	e.say("Dragon Ascending: %s sets their Power to %s's: %d → %d.", e.seatName(seat), e.seatName(enemy), d.Power[seat], d.Power[enemy])
	d.Power[seat] = d.Power[enemy]
}

// descendAvailable says the Opener may double the Doom of its Ritual now.
func (e *eng) descendAvailable(seat int) bool {
	d := e.d
	return e.is(seat, facOW) && d.hasBook(seat, bkDescend) && !d.spent(seat, 1)
}

// --- Channel Power ---------------------------------------------------------------------------------

// channelMoves: pay 1 Power to reroll every die that was not a Kill or a Pain.
func (e *eng) channelMoves(seat int) []state.Move {
	d := e.d
	b := d.Bat
	if b == nil || !e.is(seat, facOW) || !d.hasBook(seat, bkChannel) || d.Power[seat] < 1 {
		return nil
	}
	for _, v := range b.Dice[seat] {
		if v < 4 {
			return []state.Move{{Action: actChannel}, {Action: actDone}}
		}
	}
	return nil
}

// channel rerolls the dice of seat that were not Kills or Pains.
func (e *eng) channel(seat int) {
	d := e.d
	b := d.Bat
	d.Power[seat]--
	var idx []int
	for i, v := range b.Dice[seat] {
		if v < 4 {
			idx = append(idx, i)
		}
	}
	fresh := e.roll(len(idx))
	for k, i := range idx {
		b.Dice[seat][i] = fresh[k]
	}
	e.say("Channel Power: %s pays 1 Power and rerolls %s: %v.", e.seatName(seat), plural(len(idx), "die", "dice"), fresh)
}

// --- The Million Favored Ones ------------------------------------------------------------------------

// upgradeOf is what a unit becomes with The Million Favored Ones.
var upgradeOf = map[string]string{uCultist: uMutant, uMutant: uAbom, uAbom: uSpawn}

// upgradeMoves lists the replacements the Opener may make in the battle: an
// Acolyte, Mutant or Abomination for the next kind up (when one is in the
// pool), a Spawn for as many Mutants as it likes from the pool.
func (e *eng) upgradeMoves(seat int) []state.Move {
	d := e.d
	b := d.Bat
	if b == nil || !e.is(seat, facOW) || !d.hasBook(seat, bkMillion) {
		return nil
	}
	var out []state.Move
	for from, to := range upgradeOf {
		if e.battleUnits(b, seat, from) > 0 && e.pool(seat, to) > 0 {
			out = append(out, state.Move{Action: actUpgrade, Piece: from})
		}
	}
	if e.battleUnits(b, seat, uSpawn) > 0 {
		for n := 1; n <= e.pool(seat, uMutant); n++ {
			out = append(out, state.Move{Action: actUpgrade, Piece: uSpawn, Count: n})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Piece != out[j].Piece {
			return out[i].Piece < out[j].Piece
		}
		return out[i].Count < out[j].Count
	})
	return out
}

// upgrade replaces one unit; a Spawn is replaced by mv.Count Mutants.
func (e *eng) upgrade(seat int, mv state.Move) {
	b := e.d.Bat
	a := e.areaIdx(b.Area)
	from := mv.Piece
	// A unit that took a Pain still retreats: the replacement takes its place
	// in the list of the Pained, but a unit that took none goes first.
	pained := e.painedCount(b, seat, from)
	fresh := e.battleUnits(b, seat, from) - pained
	e.eliminateQuiet(a, seat, from)
	if from == uSpawn {
		for i := 0; i < mv.Count; i++ {
			e.place(a, seat, uMutant)
		}
		e.say("The Million Favored Ones: a Spawn of Yog-Sothoth in %s becomes %s.", e.areaName(a), plural(mv.Count, "Mutant", "Mutants"))
		if fresh <= 0 && pained > 0 {
			e.dropPained(b, seat, from, mv.Count, uMutant)
		}
		return
	}
	to := upgradeOf[from]
	e.place(a, seat, to)
	e.say("The Million Favored Ones: %s in %s becomes %s.", article(unitNameOf(from)), e.areaName(a), article(unitNameOf(to)))
	if fresh <= 0 && pained > 0 {
		e.dropPained(b, seat, from, 1, to)
	}
}

// eliminateQuiet returns a unit to its pool without any consequence of a
// loss (it is replaced, not lost).
func (e *eng) eliminateQuiet(a, seat int, typ string) {
	if e.take(a, seat, typ) {
		e.setPool(seat, typ, e.pool(seat, typ)+1)
	}
}

// dropPained changes the entry of a Pained unit that was replaced: the
// replacements retreat in its stead.
func (e *eng) dropPained(b *Battle, seat int, from string, n int, to string) {
	for i, p := range b.Pained {
		if p.Seat == seat && p.Type == from {
			b.Pained = append(b.Pained[:i], b.Pained[i+1:]...)
			for k := 0; k < n; k++ {
				b.Pained = append(b.Pained, PU{Seat: seat, Type: to})
			}
			return
		}
	}
}

// --- They Break Through ------------------------------------------------------------------------------

// summonAreas lists where seat may summon Monsters: the areas of its own
// Gates and, with They Break Through, every enemy-controlled or abandoned Gate.
func (e *eng) summonAreas(seat int) []int {
	gates := e.gatesOf(seat)
	if !e.is(seat, facOW) || !e.d.hasBook(seat, bkBreak) {
		return gates
	}
	seen := map[int]bool{}
	for _, a := range gates {
		seen[a] = true
	}
	for id := range e.d.Gates {
		if a := e.areaIdx(id); !seen[a] {
			seen[a] = true
			gates = append(gates, a)
		}
	}
	sort.Ints(gates)
	return gates
}

// --- Dread Curse of Azathoth -------------------------------------------------------------------------

// CurseState is a Dread Curse of Azathoth being settled: the results rolled
// that are still to be given to an enemy, the one being placed, and the units
// that were Pained and wait for the Opener to say where they go.
type CurseState struct {
	Seat int    `json:"seat"`
	Area string `json:"area"`
	// Left lists the results still to give, Kills first: each entry says
	// whether it is a Pain.
	Left []bool `json:"left,omitempty"`
	// Pain says the result being placed is a Pain; Target the enemy that takes it.
	Pain   bool `json:"pain,omitempty"`
	Target int  `json:"target,omitempty"`
	// Pained are the units to move.
	Pained []PU `json:"pained,omitempty"`
}

func (c *CurseState) clone() *CurseState {
	if c == nil {
		return nil
	}
	o := *c
	o.Left = append([]bool(nil), c.Left...)
	o.Pained = append([]PU(nil), c.Pained...)
	return &o
}

// curseCount is how many dice the curse rolls: one per Abomination and Spawn
// of Yog-Sothoth in play.
func (e *eng) curseCount(seat int) int { return e.onMap(seat, uAbom) + e.onMap(seat, uSpawn) }

// curseMoves lists the areas the Opener may curse: any that holds an enemy unit.
func (e *eng) curseMoves(seat int) []state.Move {
	if !e.d.hasBook(seat, bkCurse) || !e.is(seat, facOW) || e.curseCount(seat) == 0 {
		return nil
	}
	var out []state.Move
	for a := range e.m.ids {
		for _, o := range e.others(seat) {
			if e.total(a, o) > 0 {
				out = append(out, state.Move{Action: actCurse, To: e.m.ids[a]})
				break
			}
		}
	}
	return out
}

// beginCurse pays for the curse, rolls the dice and starts handing out the results.
func (e *eng) beginCurse(seat int, area string) {
	d := e.d
	d.Power[seat] -= 2
	n := e.curseCount(seat)
	dice := e.roll(n)
	kills, pains := 0, 0
	for _, v := range dice {
		switch {
		case v == 6:
			kills++
		case v >= 4:
			pains++
		}
	}
	a := e.areaIdx(area)
	e.say("Dread Curse of Azathoth: %s curses %s with %s: %v — %s, %s.", e.seatName(seat), e.areaName(a), plural(n, "die", "dice"), dice, plural(kills, "Kill", "Kills"), plural(pains, "Pain", "Pains"))
	c := &CurseState{Seat: seat, Area: area}
	for i := 0; i < kills; i++ {
		c.Left = append(c.Left, false)
	}
	for i := 0; i < pains; i++ {
		c.Left = append(c.Left, true)
	}
	d.Curse = c
	e.mainTaken()
	e.curseNext()
	e.thenAfterPrompts("after")
	e.beginPrompts("after")
}

// curseNext queues the next decision of the curse: which enemy takes the next
// result, or — when all are given — where each Pained unit goes.
func (e *eng) curseNext() {
	d := e.d
	c := d.Curse
	if c == nil {
		return
	}
	if len(c.Left) > 0 {
		c.Pain, c.Left = c.Left[0], c.Left[1:]
		d.Prompts = append(d.Prompts, Prompt{Kind: "curse-target", Seat: c.Seat, Area: c.Area})
		return
	}
	if len(c.Pained) > 0 {
		d.Prompts = append(d.Prompts, Prompt{Kind: "curse-move", Seat: c.Seat, Area: c.Area})
		return
	}
	d.Curse = nil
}

// curseTargets lists the enemies with units in the cursed area.
func (e *eng) curseTargets() []int {
	c := e.d.Curse
	if c == nil {
		return nil
	}
	a := e.areaIdx(c.Area)
	var out []int
	for _, o := range e.others(c.Seat) {
		if e.total(a, o) > 0 {
			out = append(out, o)
		}
	}
	return out
}

// curseVictims lists the unit types of the target that may take the result:
// any unit in the area that is not already waiting to move.
func (e *eng) curseVictims() []string {
	c := e.d.Curse
	if c == nil {
		return nil
	}
	a := e.areaIdx(c.Area)
	var out []string
	for _, typ := range e.typesIn(a, c.Target) {
		n := e.count(a, c.Target, typ)
		for _, p := range c.Pained {
			if p.Seat == c.Target && p.Type == typ {
				n--
			}
		}
		if n > 0 {
			out = append(out, typ)
		}
	}
	return out
}

// curseGive hands the current result to the target and asks it which unit
// takes it.
func (e *eng) curseGive(target int) {
	c := e.d.Curse
	c.Target = target
	e.d.Prompts = append(e.d.Prompts, Prompt{Kind: "curse-hit", Seat: target, Area: c.Area, By: c.Seat})
}

// curseHit applies the result to a unit of the target.
func (e *eng) curseHit(typ string) {
	c := e.d.Curse
	a := e.areaIdx(c.Area)
	if c.Pain {
		c.Pained = append(c.Pained, PU{Seat: c.Target, Type: typ})
		e.say("Dread Curse: %s's %s takes a Pain.", e.seatName(c.Target), unitNameOf(typ))
	} else {
		e.say("Dread Curse: %s's %s is killed.", e.seatName(c.Target), unitNameOf(typ))
		e.eliminate(a, c.Target, typ, c.Seat)
		e.noteKill(c.Seat, c.Target, typ)
	}
	e.curseNext()
}

// curseMoveMoves lists where the first Pained unit may go: any adjacent area
// (the Opener chooses, whatever the usual rules of retreating say).
func (e *eng) curseMoveMoves() []state.Move {
	c := e.d.Curse
	if c == nil || len(c.Pained) == 0 {
		return nil
	}
	p := c.Pained[0]
	a := e.areaIdx(c.Area)
	var out []state.Move
	for _, to := range e.m.adj[a] {
		out = append(out, state.Move{Action: actRetreat, Piece: p.Type, To: e.m.ids[to], Count: p.Seat})
	}
	return out
}

// curseMove moves the first Pained unit.
func (e *eng) curseMove(to string) {
	c := e.d.Curse
	p := c.Pained[0]
	c.Pained = c.Pained[1:]
	a := e.areaIdx(c.Area)
	dest := e.areaIdx(to)
	if e.take(a, p.Seat, p.Type) {
		e.add(dest, p.Seat, p.Type)
		e.say("Dread Curse: %s's %s is driven from %s to %s.", e.seatName(p.Seat), unitNameOf(p.Type), e.areaName(a), e.areaName(dest))
		e.syncGate(a)
		e.takeAbandonedGate(dest, p.Seat)
	}
	e.curseNext()
}
