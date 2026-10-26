package courtfall

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
)

// --- dice -------------------------------------------------------------------

// nextRandom advances a splitmix64 generator state and returns its output.
func nextRandom(state *uint64) uint64 {
	*state += 0x9e3779b97f4a7c15
	z := *state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// RollDice returns the next n dice (1-6) a generator state produces, and the
// state after them. Courtfall rolls every die this way, so the result of a
// position's dice is a function of its RNG field.
func RollDice(state uint64, n int) (dice []int, after uint64) {
	dice = make([]int, n)
	for i := range dice {
		dice[i] = int((uint64(uint32(nextRandom(&state)>>32))*6)>>32) + 1
	}
	return dice, state
}

// die rolls one six-sided die.
func (e *eng) die() int {
	if e.d.RNG == 0 {
		// An unseeded position (a public document used as a start) gets a
		// fresh generator on its first roll.
		e.d.RNG = freshSeed()
	}
	dice, after := RollDice(e.d.RNG, 1)
	e.d.RNG = after
	return dice[0]
}

func (e *eng) roll(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = e.die()
	}
	return out
}

// --- values -----------------------------------------------------------------

// cv is a unit's current dice pool: its Combat Value less condition and
// Mire, between 1 and 4. An Exhausted unit has a single (defense) die.
func (e *eng) cv(u *unit) int {
	n := unitCV[u.piece]
	switch e.cond(u) {
	case condFatigued:
		n--
	case condExhausted:
		n = 1
	}
	if e.tile(u.cell) == 2 {
		n--
	}
	if n < 1 {
		n = 1
	}
	if n > 4 {
		n = 4
	}
	return n
}

// friendBeside reports whether a non-Exhausted unit of x's side other than
// x stands next to it.
func (e *eng) friendBeside(x *unit) bool {
	return e.sideBeside(x.cell, x.seat, x)
}

// sideBeside reports whether a non-Exhausted unit of seat, other than skip,
// stands next to cell.
func (e *eng) sideBeside(cell, seat int, skip *unit) bool {
	for d := 0; d < 6; d++ {
		nb := e.g.nbr[cell][d]
		if nb < 0 {
			continue
		}
		if o := e.unitAt(nb); o != nil && o != skip && o.seat == seat && e.cond(o) != condExhausted {
			return true
		}
	}
	return false
}

// attackEdges counts the attack Edges a (attacking t) holds, at most two.
// exchange is true for a Strike (Isolation is an Edge "in Exchanges"); a
// shot or a shove only gets Flanking and Worn Down.
func (e *eng) attackEdges(a, t *unit, charge, exchange bool) int {
	n := 0
	if e.sideBeside(t.cell, a.seat, a) { // Flanking
		n++
	}
	if e.cond(t) != condFresh { // Worn Down
		n++
	}
	if charge { // Charge
		n++
	}
	if exchange && e.court(a.seat) == courtVeiled && !e.sideBeside(t.cell, t.seat, t) { // Isolation
		n++
	}
	if n > 2 {
		n = 2
	}
	return n
}

// shieldwall reports whether x holds the Shieldwall defensive Edge.
func (e *eng) shieldwall(x *unit) bool {
	if x.piece != "pawn" && e.court(x.seat) != courtBastion {
		return false
	}
	return e.friendBeside(x)
}

// defenseEdges counts the defensive Edges x holds against a melee or ranged
// attack, at most two.
func (e *eng) defenseEdges(x *unit, ranged bool) int {
	n := 0
	if ranged && e.tile(x.cell) == 1 {
		n++
	}
	if e.shieldwall(x) {
		n++
	}
	if n > 2 {
		n = 2
	}
	return n
}

// pushable reports whether x can be pushed at all: a Warden beside a
// non-Exhausted friend holds its ground.
func (e *eng) pushable(x *unit) bool {
	return x.piece != "pawn" || !e.friendBeside(x)
}

// need turns a base success number and Edge count into the number a die
// must reach (never below 4).
func need(base, edges int) int {
	n := base - edges
	if n < 4 {
		n = 4
	}
	return n
}

func successes(dice []int, n int) int {
	c := 0
	for _, v := range dice {
		if v >= n {
			c++
		}
	}
	return c
}

// --- declaring contests -----------------------------------------------------

func (e *eng) doStrike(mv state.Move) {
	d := e.d
	a := e.activeUnit()
	t := e.unitAt(e.g.index[mv.To])
	charge := a.piece == "rook" && d.Act.Charge != "" &&
		int(e.g.dist[e.g.index[board.PositionID(d.Act.Charge)]][t.cell]) > 1
	d.Act.Left--
	d.Act.Charge = ""
	d.Con = &Contest{
		Kind: "strike", A: a.key, T: t.key, ASeat: a.seat, TSeat: t.seat,
		AAtk: mv.Count, TAtk: -1, Charge: charge,
	}
	d.Last = nil
	d.Stage = stageRespond
	e.say("%s strikes %s and commits their dice in secret.", e.label(a), e.label(t))
}

func (e *eng) doShoot(mv state.Move) {
	d := e.d
	a := e.activeUnit()
	t := e.unitAt(e.g.index[mv.To])
	d.Act.Left--
	d.Act.Charge = ""
	n := 3
	if e.cond(a) == condFatigued {
		n--
	}
	if e.tile(a.cell) == 2 {
		n--
	}
	if n < 1 {
		n = 1
	}
	tn := 2
	if e.cond(t) != condFresh {
		tn = 1
	}
	if e.tile(t.cell) == 2 {
		tn--
	}
	if tn < 1 {
		tn = 1
	}
	c := &Contest{
		Kind: "shoot", A: a.key, T: t.key, ASeat: a.seat, TSeat: t.seat,
		AAtk: n, TAtk: -1, Rolled: true,
		ADN: need(6, e.attackEdges(a, t, false, false)),
		TFN: need(6, e.defenseEdges(t, true)),
	}
	c.AD = e.roll(n)
	c.TF = e.roll(tn)
	d.Con = c
	d.Last = nil
	e.say("%s shoots at %s: %d dice need %d+ against %d dice needing %d+.", e.label(a), e.label(t), n, c.ADN, tn, c.TFN)
	e.nextReroll()
}

func (e *eng) doShove(mv state.Move) {
	d := e.d
	a := e.activeUnit()
	t := e.unitAt(e.g.index[mv.To])
	d.Act.Left--
	d.Act.Charge = ""
	c := &Contest{
		Kind: "shove", A: a.key, T: t.key, ASeat: a.seat, TSeat: t.seat,
		AAtk: -1, TAtk: -1, Rolled: true,
	}
	d.Last = nil
	if e.cond(t) == condExhausted {
		c.Auto = true
		d.Con = c
		e.say("%s shoves %s, who is Exhausted and cannot resist.", e.label(a), e.label(t))
		e.resolve()
		return
	}
	c.ADN = need(5, e.attackEdges(a, t, false, false))
	c.TFN = need(6, e.defenseEdges(t, false))
	c.AD = e.roll(e.cv(a))
	c.TF = e.roll(e.cv(t))
	d.Con = c
	e.say("%s shoves %s: %d dice need %d+ against %d dice needing %d+.", e.label(a), e.label(t), len(c.AD), c.ADN, len(c.TF), c.TFN)
	e.nextReroll()
}

func (e *eng) legalRespond() []state.Move {
	c := e.d.Con
	t := e.unitByKey(c.T)
	if t == nil {
		return nil
	}
	max := e.cv(t)
	if e.cond(t) == condExhausted {
		max = 0 // an Exhausted unit has one defense die
	}
	var out []state.Move
	for n := 0; n <= max; n++ {
		out = append(out, state.Move{Action: actRespond, Count: n})
	}
	return out
}

func (e *eng) doRespond(mv state.Move) {
	d := e.d
	c := d.Con
	a, t := e.unitByKey(c.A), e.unitByKey(c.T)
	c.TAtk = mv.Count
	aCV, tCV := e.cv(a), e.cv(t)
	if e.cond(t) == condExhausted {
		tCV = 1
	}
	c.ADN = need(6, e.attackEdges(a, t, c.Charge, true))
	c.AFN = need(6, e.defenseEdges(a, false))
	c.TDN = need(6, e.attackEdges(t, a, false, true))
	c.TFN = need(6, e.defenseEdges(t, false))
	c.AD = e.roll(c.AAtk)
	c.AF = e.roll(aCV - c.AAtk)
	c.TD = e.roll(c.TAtk)
	c.TF = e.roll(tCV - c.TAtk)
	c.Rolled = true
	c.Step = 0
	e.say("%s answers with %d attack and %d defense dice; %s had put %d on attack and %d on defense.",
		e.label(t), c.TAtk, tCV-c.TAtk, e.label(a), c.AAtk, aCV-c.AAtk)
	e.nextReroll()
}

// --- rerolls ----------------------------------------------------------------

type diceRef struct {
	p    *int
	need int
}

// diceOf lists a side's dice (attack first, then defense) for rerolling.
func (e *eng) diceOf(side int) []diceRef {
	c := e.d.Con
	var out []diceRef
	add := func(vals []int, need int) {
		for i := range vals {
			out = append(out, diceRef{&vals[i], need})
		}
	}
	if side == 0 {
		add(c.AD, c.ADN)
		if c.Kind == "strike" {
			add(c.AF, c.AFN)
		}
	} else {
		if c.Kind == "strike" {
			add(c.TD, c.TDN)
		}
		add(c.TF, c.TFN)
	}
	return out
}

func (e *eng) rerollSeat(side int) int {
	if side == 0 {
		return e.d.Con.ASeat
	}
	return e.d.Con.TSeat
}

func (e *eng) failedDice(side int) []int {
	var out []int
	for i, r := range e.diceOf(side) {
		if *r.p < r.need {
			out = append(out, i+1)
		}
	}
	return out
}

func (e *eng) canReroll(side int) bool {
	c := e.d.Con
	if c.Used[side] || c.Auto || e.mom(e.rerollSeat(side)) < 1 {
		return false
	}
	return len(e.failedDice(side)) > 0
}

// nextReroll offers the next side that can reroll, or resolves the contest.
func (e *eng) nextReroll() {
	d := e.d
	for d.Con.Step < 2 {
		if e.canReroll(d.Con.Step) {
			d.Stage = stageReroll
			return
		}
		d.Con.Step++
	}
	e.resolve()
}

func (e *eng) legalReroll() []state.Move {
	c := e.d.Con
	out := []state.Move{{Action: actKeep}}
	if c.Step >= 2 || !e.canReroll(c.Step) {
		return out
	}
	for _, k := range e.failedDice(c.Step) {
		out = append(out, state.Move{Action: actReroll, Count: k})
	}
	return out
}

func (e *eng) doReroll(k int) {
	c := e.d.Con
	side := c.Step
	seat := e.rerollSeat(side)
	refs := e.diceOf(side)
	old := *refs[k-1].p
	e.d.Mom[seat-1]--
	*refs[k-1].p = e.die()
	c.Used[side] = true
	e.say("%s spends 1 Momentum to reroll die %d: %d becomes %d.", e.seatLabel(seat), k, old, *refs[k-1].p)
	c.Step++
	e.nextReroll()
}

// --- resolution -------------------------------------------------------------

func (e *eng) resolve() {
	d := e.d
	c := d.Con
	a, t := e.unitByKey(c.A), e.unitByKey(c.T)
	d.Last = c.clone()
	d.Con = nil
	switch c.Kind {
	case "strike":
		sa, sad := successes(c.AD, c.ADN), successes(c.AF, c.AFN)
		st, std := successes(c.TD, c.TDN), successes(c.TF, c.TFN)
		e.say("Result — %s: %d attack vs %d defense successes; %s: %d attack vs %d.", e.label(a), sa, sad, e.label(t), st, std)
		hitT := sa > std
		hitA := st > sad
		if !hitT && !hitA {
			e.say("Neither attack lands.")
		}
		if hitT {
			e.combatHit(t, a.seat)
		}
		if hitA {
			e.combatHit(a, t.seat)
		}
		if hitT && sa-std >= 2 {
			e.say("%s wins the exchange by %d and pushes %s.", e.label(a), sa-std, e.label(t))
			e.d.Pushes = append(e.d.Pushes, Push{Unit: t.key, By: a.seat})
		}
		if hitA && st-sad >= 2 {
			e.say("%s wins the counter by %d and pushes %s.", e.label(t), st-sad, e.label(a))
			e.d.Pushes = append(e.d.Pushes, Push{Unit: a.key, By: t.seat})
		}
		e.runPushes()
	case "shoot":
		sa, std := successes(c.AD, c.ADN), successes(c.TF, c.TFN)
		e.say("Result — %d successes against %d.", sa, std)
		if sa > std {
			e.combatHit(t, a.seat)
			if sa-std >= 2 {
				e.say("The shot wins by %d and pushes %s.", sa-std, e.label(t))
				e.d.Pushes = append(e.d.Pushes, Push{Unit: t.key, By: a.seat})
			}
		} else {
			e.say("The shot does not land.")
		}
		e.runPushes()
	case "shove":
		win := c.Auto
		if !win {
			sa, st := successes(c.AD, c.ADN), successes(c.TF, c.TFN)
			e.say("Result — %d successes against %d.", sa, st)
			win = sa > st
		}
		if !win {
			e.say("%s holds its ground.", e.label(t))
			e.runPushes()
			return
		}
		hops := 1
		if a.piece == "rook" {
			hops = 2
		}
		if e.court(a.seat) == courtCinder {
			hops++
		}
		dir := e.g.dirBetween(a.cell, t.cell)
		e.shovePush(t, dir, hops, a.seat)
		e.runPushes()
	}
}

// combatHit advances t's condition for a Hit by seat by.
func (e *eng) combatHit(t *unit, by int) {
	switch e.cond(t) {
	case condFresh:
		e.setCond(t, condFatigued)
		e.addMom(by, 1)
		e.say("Hit! %s is Fatigued; %s gains 1 Momentum.", e.label(t), e.seatLabel(by))
	case condFatigued:
		e.setCond(t, condExhausted)
		e.addMom(by, 1)
		extra := ""
		if t.piece == "queen" {
			e.addMom(by, 1)
			extra = " (+1 for the fallen Champion)"
		}
		e.say("Hit! %s is Exhausted; %s gains 1 Momentum%s.", e.label(t), e.seatLabel(by), extra)
	default:
		e.say("Hit! %s is already Exhausted and is pushed instead.", e.label(t))
		e.d.Pushes = append(e.d.Pushes, Push{Unit: t.key, By: by})
	}
}

// terrainHit is a Hit dealt by the arena itself (a Hazard or the edge): it
// earns nobody Momentum, but an Exhausted unit is pushed on by `by`.
func (e *eng) terrainHit(u *unit, by int, why string) {
	switch e.cond(u) {
	case condFresh:
		e.setCond(u, condFatigued)
		e.say("%s %s and suffers a Hit: Fatigued.", e.label(u), why)
	case condFatigued:
		e.setCond(u, condExhausted)
		e.say("%s %s and suffers a Hit: Exhausted.", e.label(u), why)
	default:
		e.say("%s %s while Exhausted and is pushed on.", e.label(u), why)
		if by > 0 {
			e.d.Pushes = append([]Push{{Unit: u.key, By: by, Chain: true}}, e.d.Pushes...)
		}
	}
}

// pushBonus pays the pusher for driving an enemy into a Hazard or off the
// edge.
func (e *eng) pushBonus(by int) {
	if by < 1 {
		return
	}
	n := 1
	if e.court(by) == courtCinder {
		n = 2
	}
	before := e.mom(by)
	e.addMom(by, n)
	if gained := e.mom(by) - before; gained > 0 {
		e.say("%s gains %d Momentum for the push.", e.seatLabel(by), gained)
	}
}

// pushOne moves u one hex in direction dir (the caller has checked it).
func (e *eng) pushOne(u *unit, dir, by int) {
	dest := e.g.nbr[u.cell][dir]
	if dest < 0 {
		e.say("%s is pushed off the edge of the arena and stays where it stood.", e.label(u))
		e.terrainHit(u, by, "is pushed off the edge")
		e.pushBonus(by)
		return
	}
	e.moveUnit(u, dest)
	e.say("%s is pushed to %s.", e.label(u), e.cellName(dest))
	if e.tile(dest) == 3 {
		e.terrainHit(u, by, "is pushed into a Hazard")
		e.pushBonus(by)
	}
}

// shovePush pushes t up to hops hexes in direction dir.
func (e *eng) shovePush(t *unit, dir, hops, by int) {
	if !e.pushable(t) {
		e.say("%s cannot be pushed.", e.label(t))
		return
	}
	for i := 0; i < hops; i++ {
		dest := e.g.nbr[t.cell][dir]
		if dest >= 0 && e.at[dest] >= 0 {
			break // blocked by another unit
		}
		e.pushOne(t, dir, by)
		if dest < 0 || e.tile(dest) == 3 {
			break // the edge and Hazards stop the push
		}
	}
}

// pushDirs lists the directions job's unit may be pushed in.
func (e *eng) pushDirs(u *unit, chain bool) []int {
	var out []int
	for d := 0; d < 6; d++ {
		nb := e.g.nbr[u.cell][d]
		switch {
		case nb < 0:
			if !chain {
				out = append(out, d)
			}
		case e.at[nb] < 0:
			out = append(out, d)
		}
	}
	return out
}

// runPushes resolves queued pushes, stopping at the first that needs a
// choice. Once the queue is empty the contest is over.
func (e *eng) runPushes() {
	d := e.d
	for len(d.Pushes) > 0 {
		job := d.Pushes[0]
		u := e.unitByKey(job.Unit)
		if u == nil || !e.pushable(u) {
			if u != nil {
				e.say("%s cannot be pushed.", e.label(u))
			}
			d.Pushes = d.Pushes[1:]
			continue
		}
		dirs := e.pushDirs(u, job.Chain)
		switch len(dirs) {
		case 0:
			d.Pushes = d.Pushes[1:]
			continue
		case 1:
			d.Pushes = d.Pushes[1:]
			e.pushOne(u, dirs[0], job.By)
			continue
		}
		d.Stage = stagePush
		return
	}
	e.afterContest()
}

func (e *eng) legalPush() []state.Move {
	d := e.d
	if len(d.Pushes) == 0 {
		return nil
	}
	job := d.Pushes[0]
	u := e.unitByKey(job.Unit)
	if u == nil {
		return nil
	}
	var out []state.Move
	for _, dir := range e.pushDirs(u, job.Chain) {
		mv := state.Move{Action: actPush, From: e.g.ids[u.cell], Count: dir + 1}
		if nb := e.g.nbr[u.cell][dir]; nb >= 0 {
			mv.To = e.g.ids[nb]
		}
		out = append(out, mv)
	}
	return out
}

func (e *eng) doPush(mv state.Move) {
	d := e.d
	job := d.Pushes[0]
	d.Pushes = d.Pushes[1:]
	u := e.unitByKey(job.Unit)
	e.pushOne(u, mv.Count-1, job.By)
	e.runPushes()
}

// afterContest runs Hit and Run, then carries on with the activation.
func (e *eng) afterContest() {
	d := e.d
	if last := d.Last; last != nil && last.Kind == "strike" && d.Act != nil && d.Act.Unit == "knight" {
		if u := e.activeUnit(); u != nil && u.key == last.A && e.cond(u) != condExhausted && len(e.shiftTargets(u)) > 0 {
			d.Stage = stageShift
			return
		}
	}
	e.afterAction()
}

func (e *eng) shiftTargets(u *unit) []int {
	var out []int
	for d := 0; d < 6; d++ {
		if nb := e.g.nbr[u.cell][d]; nb >= 0 && e.at[nb] < 0 {
			out = append(out, nb)
		}
	}
	return out
}

func (e *eng) legalShift() []state.Move {
	u := e.activeUnit()
	if u == nil {
		return []state.Move{{Action: actStay}}
	}
	var out []state.Move
	for _, c := range e.shiftTargets(u) {
		out = append(out, state.Move{Action: actShift, Piece: u.piece, From: e.g.ids[u.cell], To: e.g.ids[c]})
	}
	return append(out, state.Move{Action: actStay})
}

func (e *eng) doShift(mv state.Move) {
	u := e.activeUnit()
	to := e.g.index[mv.To]
	e.say("Hit and Run: %s shifts to %s.", e.label(u), mv.To)
	e.moveUnit(u, to)
	if e.tile(to) == 3 {
		e.terrainHit(u, 0, "enters a Hazard")
	}
	e.afterAction()
}

// --- prompt -----------------------------------------------------------------

// prompt says what the position is waiting for, in a sentence.
func (e *eng) prompt() string {
	d := e.d
	seat := toMove(d)
	who := e.seatLabel(seat)
	switch d.Phase {
	case phaseTerrain:
		return fmt.Sprintf("%s lays a tile (Overgrowth %d, Mire %d, Hazard %d left).", who, d.Left[0], d.Left[1], d.Left[2])
	case phaseCourt:
		return fmt.Sprintf("%s chooses a Court.", who)
	case phaseDeploy:
		return fmt.Sprintf("%s deploys a unit in their back two rows.", who)
	case phaseOver:
		if d.Winner == 0 {
			return "The game is drawn."
		}
		return fmt.Sprintf("%s wins.", e.seatLabel(d.Winner))
	}
	switch d.Stage {
	case stageRecover:
		return fmt.Sprintf("%s: choose which unit takes the free recovery.", who)
	case stageMain:
		extras := ""
		var opts []string
		if e.mom(seat) >= 1 {
			opts = append(opts, "Surge")
		}
		if e.mom(seat) >= 2 {
			opts = append(opts, "Rouse")
		}
		if e.court(seat) == courtStone && !d.StoneUsed[seat-1] {
			opts = append(opts, "swap two tiles")
		}
		if len(opts) > 0 {
			extras = " (first, if you like: " + strings.Join(opts, ", ") + ")"
		}
		return fmt.Sprintf("%s: activate a Fresh unit, exert a Fatigued one or take a Clear Turn%s.", who, extras)
	case stageAct:
		name := ""
		left := 0
		if d.Act != nil {
			name, left = unitName[d.Act.Unit], d.Act.Left
		}
		return fmt.Sprintf("%s's %s has %d action%s left: move, strike, shoot, shove — or end the activation.", who, name, left, plural(left))
	case stageRespond:
		return fmt.Sprintf("%s: your unit is being struck — put some of its dice on attack (the rest defend). The attacker has already committed in secret.", who)
	case stageReroll:
		return fmt.Sprintf("%s: spend 1 Momentum to reroll a failed die, or keep the roll.", who)
	case stagePush:
		name := ""
		if len(d.Pushes) > 0 {
			if u := e.unitByKey(d.Pushes[0].Unit); u != nil {
				name = e.label(u)
			}
		}
		return fmt.Sprintf("%s: choose the direction to push %s.", who, name)
	case stageShift:
		return fmt.Sprintf("%s: Hit and Run — shift the Outrider one hex for free, or stay.", who)
	case stageSeize:
		return fmt.Sprintf("%s: Seize the Tempo (2 Momentum) for another turn, or end the turn.", who)
	}
	return ""
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// describe lists a unit as text for tooltips.
func (e *eng) describeUnit(u *unit) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("%s %s", e.seatLabel(u.seat), unitName[u.piece]))
	switch e.cond(u) {
	case condFatigued:
		parts = append(parts, "Fatigued")
	case condExhausted:
		parts = append(parts, "Exhausted")
	default:
		parts = append(parts, "Fresh")
	}
	parts = append(parts, fmt.Sprintf("CV %d", e.cv(u)))
	return strings.Join(parts, " · ")
}
