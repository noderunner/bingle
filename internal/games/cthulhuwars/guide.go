package cthulhuwars

import (
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// This file is what the computer players know about playing Cthulhu Wars
// sensibly. The built-in bots are generic — random, one-move look-ahead,
// search — so left alone they spend Power at random, give Gates up and leave
// Cultists to be captured. GuideMoves narrows a decision to the moves worth
// considering; every bot then chooses among those only.
//
// The guidance is a handful of strategy tips, as a player would give them:
//
//  1. Income is the objective of the early game: Gates (2 Power each) and
//     Cultists (1 each) are what the round's Power comes from.
//  2. Spend as little as possible at a time, so Power is left to react to
//     what the others do: one action, then end the turn; one unit per Move.
//  3. The usual opening: move a Cultist to a neighbouring area free of
//     enemies, then build a Gate there.
//  4. Do not leave Gates or Cultists in an area with no monster to protect
//     them unless a bigger goal needs it: they are easily captured.
//  5. There is no need to summon a protector at once while enough Power is
//     left to summon one in reply when an enemy monster walks in — but when
//     the turn would otherwise leave too little, summon it as the last action.
//
// plus the one rule of thumb the bots broke most often: never give up a Gate.

var _ state.BotGuide = (*Ruleset)(nil)

// Scores of the decisions a guide weighs, best first. They only need to be
// ordered sensibly: the bot picks among the moves within guideSlack of the
// best.
const (
	guideSlack = 4.0

	scoreDoneTurn   = 50.0 // ending the turn once the main action is taken
	scoreDoneMoving = 50.0 // finishing a Move after the first unit has gone
)

type guide struct {
	e    *eng
	seat int
	// t shifts the scores (see GuideTuning).
	t GuideTuning
	// need holds the areas where seat's Gate or Cultists stand with no
	// monster, and an enemy that has — or could next turn bring — one.
	need map[int]int
	// reserve is the Power seat should keep to summon a protector in reply
	// (0 when nothing needs protecting).
	reserve int
}

type scoredMove struct {
	mv    state.Move
	score float64
}

// GuideMoves implements state.BotGuide.
func (rs *Ruleset) GuideMoves(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) []state.Move {
	return rs.guideMoves(game, st, seat, legal, nil)
}

// guideMoves is GuideMoves with a tuning (nil: the shipped one of the seat's cult).
func (rs *Ruleset) guideMoves(game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move, t *GuideTuning) []state.Move {
	d, err := dataOf(st)
	if err != nil || d.Phase == phaseOver || seat < 1 || seat > nSeats {
		return legal
	}
	e := rs.newEng(game, st, d)
	g := &guide{e: e, seat: seat}
	if t != nil {
		g.t = *t
	} else if f := e.fac(seat); f >= 0 {
		g.t = ShippedTuning(factions[f].Key)
	}
	// A unit stepping off its Gate abandons it: never worth considering.
	legal = withoutStepOffs(legal, d, seat)
	var out []state.Move
	switch d.Stage {
	case stageSetup:
		out = g.setup(legal)
	case stageTurn:
		g.survey()
		out = g.turn(legal)
	case stageMoving:
		g.survey()
		out = g.moving(legal)
	case stagePre:
		out = g.pre(legal)
	case stagePost:
		out = g.post(legal)
	case stageAssign:
		out = g.assign(legal)
	case stageRetreat:
		out = g.retreat(legal)
	case stagePrompt:
		out = g.prompt(legal)
	case stageDoom:
		out = g.doom(legal)
	default:
		return legal
	}
	if len(out) == 0 {
		return legal
	}
	return out
}

// withoutStepOffs drops the moves that give a Gate up without a reason — the
// unit on it "moving" to the area it stands in, or a unit swapped onto a Gate
// that is already its own — unless nothing else is left.
func withoutStepOffs(legal []state.Move, d *Data, seat int) []state.Move {
	out := make([]state.Move, 0, len(legal))
	for _, mv := range legal {
		if (mv.Action == actGo || mv.Action == actLeave) && mv.From == mv.To {
			continue
		}
		if mv.Action == actControl && d.Gates[string(mv.To)] == seat {
			continue
		}
		out = append(out, mv)
	}
	if len(out) == 0 {
		return legal
	}
	return out
}

// top keeps the moves within slack of the best score, in their given order.
func top(list []scoredMove, slack float64) []state.Move {
	if len(list) == 0 {
		return nil
	}
	best := list[0].score
	for _, s := range list[1:] {
		if s.score > best {
			best = s.score
		}
	}
	var out []state.Move
	for _, s := range list {
		if s.score >= best-slack {
			out = append(out, s.mv)
		}
	}
	return out
}

// --- the picture of the table ------------------------------------------------

// survey finds what needs protecting and the Power to keep for it.
func (g *guide) survey() {
	e, seat := g.e, g.seat
	g.need = map[int]int{}
	gates := map[int]bool{}
	for _, a := range e.gatesOf(seat) {
		gates[a] = true
	}
	for a := range e.m.ids {
		if e.protectionRank(a, seat) > 0 {
			continue
		}
		if e.cultists(a, seat) == 0 && !gates[a] {
			continue
		}
		if lvl := g.threatAt(a); lvl > 0 {
			g.need[a] = lvl
		}
	}
	// A protector can only be summoned at a Gate of ours: that is where the
	// Power must be kept for.
	cheapest := 0
	for _, u := range e.units(seat) {
		if (u.Class != classMonster && u.Class != classTerror) || u.Token || e.pool(seat, u.ID) <= 0 {
			continue
		}
		if c := e.summonCost(seat, u.ID); cheapest == 0 || c < cheapest {
			cheapest = c
		}
	}
	for a := range g.need {
		if gates[a] {
			g.reserve = cheapest
			break
		}
	}
}

// threatAt says how dangerous area a is for the Cultists standing there: 2
// when an enemy monster or Great Old One that outranks ours is there
// already, 1 when one could walk in on its owner's next turn (and its owner
// has the Power for the move and the capture), 0 otherwise.
func (g *guide) threatAt(a int) int {
	e, d, seat := g.e, g.e.d, g.seat
	mine := e.protectionRank(a, seat)
	level := 0
	for _, en := range e.others(seat) {
		if e.protectionRank(a, en) > mine {
			return 2
		}
		if d.Power[en] < 2 {
			continue
		}
		for _, n := range e.reach(en, a) {
			if e.protectionRank(n, en) > mine {
				level = 1
				break
			}
		}
	}
	return level
}

// enemyIn reports whether any enemy has a unit in area a.
func (g *guide) enemyIn(a int) bool {
	for _, en := range g.e.others(g.seat) {
		if g.e.total(a, en) > 0 {
			return true
		}
	}
	return false
}

// enemyRank is the highest rank of the enemy units standing in a.
func (g *guide) enemyRank(a int) int {
	r := 0
	for _, en := range g.e.others(g.seat) {
		if x := g.e.protectionRank(a, en); x > r {
			r = x
		}
	}
	return r
}

// holders counts the units in a that can hold a Gate for seat.
func (g *guide) holders(a int) int {
	e, seat := g.e, g.seat
	n := e.cultists(a, seat)
	if e.is(seat, facBG) && e.d.hasBook(seat, bkRedSign) {
		n += e.count(a, seat, uYoung)
	}
	return n
}

// leavesGateEmpty reports whether taking one unit of type typ out of a would
// leave seat's Gate there with nobody able to hold it.
func (g *guide) leavesGateEmpty(a int, typ string) bool {
	e, seat := g.e, g.seat
	if e.d.Gates[e.areaID(a)] != seat {
		return false
	}
	holds := isCultist(typ) || (typ == uYoung && e.is(seat, facBG) && e.d.hasBook(seat, bkRedSign))
	return holds && g.holders(a) <= 1
}

// rankWithout is the rank seat's units in a would have after one unit of type
// typ left.
func (g *guide) rankWithout(a int, typ string) int {
	e, seat := g.e, g.seat
	rank := 0
	for _, t := range e.typesIn(a, seat) {
		n := e.count(a, seat, t)
		if t == typ {
			n--
		}
		if n <= 0 {
			continue
		}
		switch unitClassOf(t) {
		case classGOO:
			return 2
		case classMonster:
			rank = 1
		}
	}
	return rank
}

// adjacentThreats counts the enemy monsters and Great Old Ones next to a.
func (g *guide) adjacentThreats(a int) int {
	e := g.e
	n := 0
	for _, nb := range e.m.adj[a] {
		for _, en := range e.others(g.seat) {
			n += e.classCount(nb, en, classMonster) + e.classCount(nb, en, classGOO)
		}
	}
	return n
}

// exposedAreas counts the areas where seat's Cultists stand with no monster
// or Great Old One of its own beside an enemy that has one: they can be
// captured at once.
func (e *eng) exposedAreas(seat int) int {
	n := 0
	for a := range e.m.ids {
		if e.protectionRank(a, seat) > 0 || e.cultists(a, seat) == 0 {
			continue
		}
		for _, en := range e.others(seat) {
			if e.protectionRank(a, en) > 0 {
				n++
				break
			}
		}
	}
	return n
}

// cost is the Power a decision costs the seat taking it.
func (g *guide) cost(mv state.Move) int {
	e, seat := g.e, g.seat
	switch mv.Action {
	case actRecruit, actBattle, actCapture, actGo, actLeave, actSubmerge, actScream, actShriek, actZingaya, actHWINN, actAvatar:
		return 1
	case actGate:
		return e.game.ParamInt("gate_cost", 3)
	case actSummon:
		return e.summonCost(seat, mv.Piece)
	case actAwaken:
		return e.moveCost(seat, mv)
	case actPay:
		return mv.Count
	case actDreams, actGhroth:
		return 2
	case actDesecrate:
		return e.desecrateCost(seat)
	}
	return 0
}

// --- the turn ----------------------------------------------------------------

func (g *guide) turn(legal []state.Move) []state.Move {
	d, seat := g.e.d, g.seat
	t := d.Turn
	extra := t != nil && t.Main && (!t.Second || t.SecondTaken)
	var list []scoredMove
	for _, mv := range legal {
		var s float64
		switch mv.Action {
		case actAbandon:
			continue // giving a Gate up is almost never right
		case actUndim, actRecrim:
			continue // the bots leave these two neutral spellbooks to people
		case actPass:
			s = 0
		case actDone:
			s = scoreDoneTurn + g.t.EndTurn
			if t != nil && !t.Main {
				// Only after a Fertility Cult summon: no main action is
				// taken yet, so the bot goes on to one, as it did when
				// only Pass could follow.
				s = 0
			}
		case actReveal:
			if !g.revealWins() {
				continue
			}
			s = 99
		case actOath:
			// Only when the High Priest is about to be captured.
			if !g.e.oathThreatened(seat) {
				continue
			}
			s = 80
		case actClaim:
			if !g.claimNow() {
				continue
			}
			s = 90
		case actRansom:
			if d.Doom[seat] < 5 {
				continue
			}
			s = 62
		default:
			s = g.score(mv)
			// Tip 2 and 5: do not spend what is needed to answer a threat,
			// unless this is the answer.
			if g.reserve > 0 && (mv.Action != actSummon || !g.defends(mv)) && d.Power[seat]-g.cost(mv) < g.reserve {
				s -= 35 + g.t.Reserve
			}
			// Tip 2: after the main action only what is worth more than
			// ending the turn.
			if extra && s <= scoreDoneTurn+g.t.EndTurn && mv.Action != actControl {
				continue
			}
		}
		list = append(list, scoredMove{mv, s})
	}
	return top(list, guideSlack+g.t.Slack)
}

// defends reports whether a summon puts a protector where one is needed.
func (g *guide) defends(mv state.Move) bool {
	if mv.Action != actSummon {
		return false
	}
	_, need := g.need[g.e.areaIdx(string(mv.To))]
	return need
}

// score values one decision of the turn: its own worth, plus what it does for
// a spellbook requirement.
func (g *guide) score(mv state.Move) float64 {
	s := g.scoreBase(mv)
	return s + g.reqBonus(mv, s)
}

// scoreBase values one decision of the turn.
func (g *guide) scoreBase(mv state.Move) float64 {
	e, d, seat := g.e, g.e.d, g.seat
	p := d.Power[seat]
	area := -1
	if mv.To != "" {
		area = e.areaIdx(string(mv.To))
	}
	switch mv.Action {
	case actControl:
		return 95
	case actGate:
		s := 90.0 + g.t.Gate
		if g.threatAt(area) == 2 {
			s -= 40 // it would be captured at once
		}
		if e.is(seat, facGC) && e.m.ocean[area] {
			s += 4 // Cthulhu wakes in the sea
		}
		return s
	case actAwaken:
		if isIndependent(mv.Piece) {
			return g.scoreIndependent(mv, area)
		}
		switch mv.Piece {
		case uShub:
			if e.cultistsInPlay(seat) >= 4 {
				return 70 + g.t.Awaken
			}
			return 25 + g.t.Awaken
		case uKing:
			return 80 + g.t.Awaken
		}
		return 85 + g.t.Awaken
	case actCapture:
		switch mv.Piece {
		case onGate + uCultist:
			// The Cultist that holds the Gate leaves it open to the captor's own.
			return 89 + g.t.Capture
		case uCultist:
			// Offered only beside the Cultist on the Gate, which is worth more:
			// the Gate stays open for the captor's own.
			return 60 + g.t.Capture
		case "", uHighPriest:
			return 88 + g.t.Capture
		}
		return 82 + g.t.Capture // a Monster, for a Power in Gather Power
	case actRecruit:
		if mv.Piece == uHighPriest {
			// A High Priest is worth his Power in the long run, not at once.
			if g.e.d.Power[seat] >= 7 && g.e.cultistsInPlay(seat) >= 3 {
				return 42
			}
			return 12
		}
		s := 70.0 + g.t.Recruit
		if e.d.Gates[e.areaID(area)] == seat {
			s += 8
		}
		if e.protectionRank(area, seat) > 0 {
			s += 4
		}
		if g.threatAt(area) == 2 {
			s -= 30
		}
		return s
	case actSummon:
		return g.scoreSummon(mv, area, p)
	case actGo, actLeave:
		return g.scoreGo(mv)
	case actBattle:
		return g.scoreBattle(area, mv.Count)
	case actPay:
		if len(e.gatesOf(seat)) < 2 {
			return 30 // income first
		}
		if mv.Count >= 10 {
			return 40 // both books, at a price
		}
		return 55
	case actGrant:
		// Someone must gain 3 Doom: the one furthest behind hurts least.
		for _, en := range e.others(seat) {
			if d.Doom[en] < d.Doom[mv.Count] {
				return 14
			}
		}
		return 42
	case actAvatar:
		// Shub-Niggurath steps into another area: worth it to reach a lone
		// Cultist on a Gate, or to rescue one of ours.
		if _, ok := d.Gates[e.areaID(area)]; ok && mv.Count != seat && e.cultists(area, mv.Count) == 1 {
			return 52
		}
		return 18
	case actSacrifice:
		if e.cultistsInPlay(seat) >= 5 {
			return 45
		}
		return 15
	case actDreams:
		s := 55.0
		if _, ok := d.Gates[e.areaID(area)]; ok && e.cultists(area, mv.Count) == 1 {
			s = 78 // the Gate falls to us
		}
		return s
	case actZingaya:
		return 75
	case actDevolve:
		// A free monster where Cultists need one, but never the last Cultist
		// of a Gate.
		if e.count(area, seat, uCultist) >= 2 && e.protectionRank(area, seat) == 0 && g.threatAt(area) > 0 {
			return 72
		}
		return 5
	case actForms:
		for _, en := range e.others(seat) {
			if d.Power[en] > 0 {
				return 48
			}
		}
		return 0
	case actGhroth:
		areas := 0
		for a := range e.m.ids {
			if e.count(a, seat, uFungi) > 0 {
				areas++
			}
		}
		return 20 + 12*float64(areas)
	case actSubmerge:
		if a, ok := e.gooArea(seat, uCthulhu); ok && g.threatAt(a) > 0 {
			return 60
		}
		return 5
	case actEmerge:
		s := 20.0
		if g.need[area] > 0 {
			s = 62
		}
		if e.d.Gates[e.areaID(area)] == seat {
			s += 15
		}
		return s
	case actDesecrate:
		if a, ok := e.gooArea(seat, uKing); ok {
			return 20 + 8*float64(e.total(a, seat))
		}
		return 20
	case actShriek:
		if g.need[area] > 0 {
			return 66
		}
		return 10
	case actScream, actHWINN:
		return 12
	}
	if s, ok := g.scoreOT(mv, area); ok {
		return s
	}
	if s, ok := g.scoreExpansion(mv, area); ok {
		return s
	}
	if s, ok := g.scoreCards(mv, area); ok {
		return s
	}
	return 15
}

// scoreSummon values putting a monster at a Gate (tips 4 and 5).
func (g *guide) scoreSummon(mv state.Move, a, p int) float64 {
	e, seat := g.e, g.seat
	c := e.summonCost(seat, mv.Piece)
	s := 40.0 + g.t.SummonIdle
	switch {
	case g.need[a] == 2:
		s = 88 + g.t.SummonUrgent // an enemy is already there: it captures next turn
	case g.need[a] == 1:
		// An enemy could walk in, but the answer can wait for it as long as
		// the Power for it is kept (see the reserve in turn): this is worth
		// less than building income, yet more than any move that would
		// leave too little Power to answer — those lose 35 in turn.
		s = 56 + g.t.SummonWatch
	case e.protectionRank(a, seat) > 0:
		s = 28 + g.t.SummonIdle // already guarded
		if p >= c+6 {
			s = 42 + g.t.SummonIdle
		}
	}
	// Cheap protectors first: spend as little as possible.
	s -= 3 * float64(c)
	if u, ok := e.f(seat).unitSpec(mv.Piece); ok {
		s += 1.5 * float64(u.Combat)
	}
	return s
}

// scoreGo values moving one unit (tips 3 and 4).
func (g *guide) scoreGo(mv state.Move) float64 {
	e, d, seat := g.e, g.e.d, g.seat
	from, to := e.areaIdx(string(mv.From)), e.areaIdx(string(mv.To))
	typ := mv.Piece
	// The unit on a Gate that leaves abandons it, whoever else stands there.
	if mv.Action == actLeave || g.leavesGateEmpty(from, typ) {
		return -50
	}
	pen := 0.0
	// A protector that leaves exposes what it guarded.
	if unitClassOf(typ) != classCultist && g.rankWithout(from, typ) < e.protectionRank(from, seat) &&
		(e.cultists(from, seat) > 0 || d.Gates[e.areaID(from)] == seat) {
		pen = 30
	}
	if unitClassOf(typ) == classCultist {
		hasGate, abandoned := false, false
		if ctl, ok := d.Gates[e.areaID(to)]; ok {
			hasGate, abandoned = true, ctl == 0
		}
		switch {
		case abandoned && g.enemyRank(to) <= e.protectionRank(to, seat):
			return 80 - pen
		case hasGate:
			return 22 - pen
		case g.enemyIn(to):
			if g.enemyRank(to) > 0 {
				return -10 - pen
			}
			return 30 - pen // enemy Cultists only: no danger, no free area
		}
		// A free area to build a Gate in: the opening.
		s := 60.0 + g.t.OpeningMove - 8*float64(g.adjacentThreats(to))
		if e.is(seat, facGC) && e.m.ocean[to] {
			s += 3
		}
		return s - pen
	}
	// Monsters and Great Old Ones: guard what is exposed, hunt what can be
	// captured, otherwise stay.
	rank := 1
	if unitClassOf(typ) == classGOO {
		rank = 2
	}
	if cur := e.protectionRank(to, seat); cur > rank {
		rank = cur
	}
	if _, need := g.need[to]; need && e.protectionRank(to, seat) == 0 {
		return 65 + g.t.GuardMove - pen
	}
	for _, en := range e.others(seat) {
		if n := e.cultists(to, en); n > 0 && e.protectionRank(to, en) < rank {
			return 55 + g.t.HuntMove + 3*float64(n) - pen
		}
	}
	return 20 - pen
}

// scoreBattle values attacking enemy en in area a: fight only when the dice
// favour us.
func (g *guide) scoreBattle(a, en int) float64 {
	e, seat := g.e, g.seat
	mine := e.combatOf(seat, a, nil, en)
	theirs := e.combatOf(en, a, nil, seat)
	switch {
	case mine < 1:
		return 0
	case theirs == 0:
		return 70 + g.t.Battle
	}
	s := 40 + 7*float64(mine-theirs)
	if s > 80 {
		s = 80
	}
	if s < 5 {
		s = 5
	}
	return s + g.t.Battle
}

// revealWins reports whether turning every Elder Sign over wins the game for
// seat: it holds all six spellbooks and the total beats what the others could
// have.
func (g *guide) revealWins() bool {
	e, d, seat := g.e, g.e.d, g.seat
	if d.bookCount(seat) < 6 || len(d.Signs[seat]) == 0 {
		return false
	}
	total := d.Doom[seat]
	for _, v := range d.Signs[seat] {
		total += v
	}
	if total < e.game.ParamInt("doom_to_win", 30) {
		return false
	}
	for _, en := range e.others(seat) {
		if d.bookCount(en) >= 6 && d.Doom[en]+3*len(d.Signs[en]) >= total {
			return false
		}
	}
	return true
}

// --- moving ------------------------------------------------------------------

// moving guides the units that follow the first in a Move action (tip 2: one
// unit at a time): only a move that a plan needs — taking an abandoned Gate,
// guarding what the move exposed — is worth the extra Power.
func (g *guide) moving(legal []state.Move) []state.Move {
	var list []scoredMove
	for _, mv := range legal {
		if mv.Action == actDone {
			list = append(list, scoredMove{mv, scoreDoneMoving})
			continue
		}
		list = append(list, scoredMove{mv, g.followUp(mv)})
	}
	return top(list, guideSlack+g.t.Slack)
}

// followUp values a further move of the same Move action: only the moves a
// plan cannot do without count for anything.
func (g *guide) followUp(mv state.Move) float64 {
	e, d, seat := g.e, g.e.d, g.seat
	if mv.Action == actControl {
		return 95 // a unit on an abandoned Gate: always
	}
	from, to := e.areaIdx(string(mv.From)), e.areaIdx(string(mv.To))
	if mv.Action == actLeave || g.leavesGateEmpty(from, mv.Piece) {
		return -50
	}
	if mv.Piece == uHound {
		// Chronophage: a free hop, to a Gate where it guards.
		if g.need[to] > 0 || e.total(to, seat) > 0 {
			return 55
		}
		return 0
	}
	if unitClassOf(mv.Piece) == classCultist {
		if ctl, ok := d.Gates[e.areaID(to)]; ok && ctl == 0 && g.enemyRank(to) <= e.protectionRank(to, seat) {
			return 80 // an abandoned Gate to take
		}
		return 0
	}
	if _, need := g.need[to]; need && e.protectionRank(to, seat) == 0 && g.rankWithout(from, mv.Piece) >= e.protectionRank(from, seat) {
		return 65 // a guard for what the first move exposed
	}
	return 0
}

// --- results, retreats, choices --------------------------------------------------

// unitValue is how much seat minds losing a unit: what it costs to get one
// back, and a Cultist is worth a little more (its Power every round), the
// sole holder of a Gate much more.
func (g *guide) unitValue(owner int, typ string, a int) float64 {
	e := g.e
	switch unitClassOf(typ) {
	case classGOO:
		return 100
	case classMonster, classTerror:
		if u, ok := e.f(owner).unitSpec(typ); ok {
			return float64(u.Cost)
		}
		return 2
	}
	v := 1.5
	if a >= 0 && e.d.Gates[e.areaID(a)] == owner {
		hold := e.cultists(a, owner)
		if e.is(owner, facBG) && e.d.hasBook(owner, bkRedSign) {
			hold += e.count(a, owner, uYoung)
		}
		if hold <= 1 {
			v += 6
		}
	}
	return v
}

// assign picks the unit that takes a Kill or Pain: the one least missed — or,
// when the seat decides for an enemy (Hastur's Vengeance), the one most
// missed.
func (g *guide) assign(legal []state.Move) []state.Move {
	b := g.e.d.Bat
	if b == nil || len(b.Queue) == 0 {
		return legal
	}
	owner := b.Queue[0].Seat
	a := g.e.areaIdx(b.Area)
	var list []scoredMove
	for _, mv := range legal {
		if mv.Action != actAssign {
			return legal
		}
		v := g.unitValue(owner, mv.Piece, a)
		if owner == g.seat {
			v = -v
		}
		list = append(list, scoredMove{mv, v})
	}
	return top(list, 0.01)
}

// retreat sends a pained unit towards its friends, away from enemies.
func (g *guide) retreat(legal []state.Move) []state.Move {
	e, seat := g.e, g.seat
	var list []scoredMove
	for _, mv := range legal {
		if mv.Action == actDone {
			// Declining Madness: the owners choose. Only when nothing better.
			list = append(list, scoredMove{mv, -100})
			continue
		}
		if mv.Action != actRetreat {
			return legal
		}
		a := e.areaIdx(string(mv.To))
		s := 0.0
		if mv.Count != 0 && mv.Count != seat {
			// Madness: an enemy's pained unit goes where it is least safe — and
			// after our own units have been seen to.
			s = 10 - 2*float64(e.total(a, mv.Count)) + 2*float64(e.total(a, seat)) - 50
		} else {
			s += 2 * float64(e.total(a, seat))
			if e.d.Gates[e.areaID(a)] == seat {
				s += 5
			}
			s -= 4 * float64(g.enemyRank(a))
		}
		list = append(list, scoredMove{mv, s})
	}
	return top(list, 0.01)
}

// bookOrder is the order a cult prefers its spellbooks: Power and Gates first.
var bookOrder = [nFactions][]int{
	facGC: {bkYha, bkDreams, bkDevolve, bkAbsorb, bkRegenerate, bkSubmerge},
	facCC: {bkForms, bkAbduct, bkSeek, bkInvisibility, bkEmissary, bkMadness},
	facBG: {bkThousand, bkRedSign, bkGhroth, bkFrenzy, bkNecro, bkBlood},
	facYS: {bkPassion, bkZingaya, bkScreaming, bkShriek, bkHWINN, bkThirdEye},
	facWW: {bkHerald, bkCannibal, bkHowl, bkArctic, bkIceAge, bkBerserk},
	facSL: {bkBurrow, bkDemand, bkCaptureMonster, bkSlumber, bkNexus, bkSorcery},
	facOW: {bkMillion, bkBreak, bkChannel, bkDescend, bkAscend, bkCurse},
	facTT: {bkHiero, bkTerror, bkMartyr, bkTablets, bkSoulless, bkIdolatry},
}

func (g *guide) prompt(legal []state.Move) []state.Move {
	d := g.e.d
	if len(d.Prompts) == 0 {
		return legal
	}
	p := d.Prompts[0]
	var list []scoredMove
	switch p.Kind {
	case "book":
		for _, mv := range legal {
			rank := 0.0
			order := bookOrder[g.e.fac(g.seat)]
			for i, b := range order {
				if b == mv.Count-1 {
					rank = float64(len(order) - i)
				}
			}
			// A neutral spellbook ranks below every book of the cult's own, the
			// ones a bot can use best first.
			if mv.Count-1 >= bookSlots {
				rank = []float64{0.5, 0.3, 0, 0.2, 0.6, 0}[mv.Count-1-bookSlots]
			}
			list = append(list, scoredMove{mv, rank})
		}
		return top(list, 0.01)
	case "desecrate":
		// The most Power's worth of unit comes free: a Byakhee, then an
		// Undead (an Acolyte is the likeliest to be captured there).
		rank := map[string]float64{uByak: 3, uUndead: 2, uCultist: 1}
		for _, mv := range legal {
			if mv.Action != actDesecrate {
				return legal
			}
			list = append(list, scoredMove{mv, rank[mv.Piece]})
		}
		return top(list, 0.01)
	case "lose":
		for _, mv := range legal {
			if mv.Action != actLose {
				return legal
			}
			a := g.e.areaIdx(string(mv.To))
			v := g.unitValue(g.seat, mv.Piece, a)
			// Among equals lose the one from the biggest crowd.
			v += 0.1 * float64(g.e.total(a, g.seat)) * -1
			list = append(list, scoredMove{mv, -v})
		}
		return top(list, 0.01)
	case "submerge":
		// Monsters sink with Cthulhu (safe, and ready to rise with him);
		// Cultists stay to hold what they hold.
		var monsters, done []state.Move
		for _, mv := range legal {
			switch {
			case mv.Action == actDone:
				done = append(done, mv)
			case mv.Action == actBring && mv.Piece != bringAll && unitClassOf(mv.Piece) == classMonster:
				monsters = append(monsters, mv)
			}
		}
		if len(monsters) > 0 {
			return monsters
		}
		return done
	case "scream", "shriek":
		// Everything that can come along comes.
		for _, mv := range legal {
			if mv.Action == actBring && mv.Piece == bringAll {
				return []state.Move{mv}
			}
		}
		var bring []state.Move
		for _, mv := range legal {
			if mv.Action == actBring {
				bring = append(bring, mv)
			}
		}
		if len(bring) > 0 {
			return bring
		}
		return legal
	case "avatar":
		// The unit given up is the one least missed.
		a := g.e.areaIdx(p.Area)
		for _, mv := range legal {
			if mv.Action != actSwap {
				return legal
			}
			list = append(list, scoredMove{mv, -g.unitValue(p.Seat, mv.Piece, a)})
		}
		return top(list, 0.01)
	case "place":
		return g.place(legal)
	case "react":
		return g.react(p, legal)
	case "mao", "undim":
		// Not a bot's kind of decision: it declines.
		for _, mv := range legal {
			if mv.Action == actDone {
				return []state.Move{mv}
			}
		}
	}
	if moves, ok := g.promptOT(p, legal); ok {
		return moves
	}
	if moves, ok := g.guideExpansionPrompt(p, legal); ok {
		return moves
	}
	if moves, ok := g.guideCardsPrompt(p, legal); ok {
		return moves
	}
	return legal
}

// --- the Doom phase --------------------------------------------------------------

func (g *guide) doom(legal []state.Move) []state.Move {
	e, d, seat := g.e, g.e.d, g.seat
	var list []scoredMove
	for _, mv := range legal {
		s := 0.0
		switch mv.Action {
		case actSkip:
			s = 40
		case actReveal:
			if !g.revealWins() {
				continue
			}
			s = 99
		case actRitual:
			cost := e.ritualCost()
			goal := e.game.ParamInt("doom_to_win", 30)
			gates := len(e.gatesOf(seat))
			wins := d.bookCount(seat) >= 6 && d.Doom[seat]+gates >= goal
			// What a Ritual buys: a Doom point per Gate and an Elder Sign (about
			// 1.7 Doom) per Great Old One in play. Early on Power buys more
			// income elsewhere, so only a good price is paid.
			gain := float64(gates) + 1.7*float64(e.gooInPlay(seat))
			switch {
			case d.Ritual >= len(e.ritualTrack())-1 && !wins:
				continue // the last space is followed by Instant Death: nobody wins
			case wins:
				s = 60
			case gain >= 0.75*float64(cost) && d.Power[seat] >= cost+3:
				s = 60
			default:
				continue // keep the Power for the next round's actions
			}
			// Dragon Descending doubles the Gates' Doom: worth it with many Gates.
			if mv.Piece == "descend" {
				if gates < 3 && !wins {
					continue
				}
				s += 8
			}
		case actDeath:
			a := e.areaIdx(string(mv.To))
			// A free Monster where it protects the most.
			s = 40 + 4*float64(e.total(a, seat))
			if e.d.Gates[e.areaID(a)] == seat {
				s += 10
			}
			if g.need[a] > 0 {
				s += 15
			}
		case actOath:
			// Worth it when it pays for a Ritual that is worth its price.
			gates := float64(e.gateCount(seat))
			gain := gates + 1.7*float64(e.gooInPlay(seat))
			if gain >= 0.75*float64(e.ritualCostFor(seat)) && d.Ritual < len(e.ritualTrack())-1 {
				s = 58
			} else {
				continue
			}
		case actClaim:
			s = 85
		case actAwaken:
			s = 75 // Ubbo-Sathla, for nothing
		case actAscend:
			s = 2
			if d.Power[seat] <= 2 && d.Power[mv.Count] >= d.Power[seat]+4 {
				s = 80
			}
		case actBlood:
			a := e.areaIdx(string(mv.To))
			if e.cultistsInPlay(seat) >= 4 && e.cultists(a, seat) >= 2 && e.d.Gates[e.areaID(a)] != seat {
				s = 45
			} else {
				continue
			}
		case actBuy:
			// A Loyalty Card is bought when it is worth 2 Doom: the best one,
			// at a Gate where it guards.
			s = g.cardBuyScore(mv)
			if s <= 0 {
				continue
			}
			if mv.To != "" {
				a := e.areaIdx(string(mv.To))
				s += 0.3 * float64(e.cultists(a, seat))
				if g.need[a] > 0 {
					s += 1
				}
			}
		case actRansom:
			if d.Doom[seat] < 5 {
				continue
			}
			s = 62
		case actCrypt:
			// The Wamps of the pool put where an enemy must: only when they are many.
			if e.pool(seat, uWamp) >= 3 {
				s = 41
			} else {
				continue
			}
		}
		list = append(list, scoredMove{mv, s})
	}
	return top(list, 0.01)
}

// --- reactions and the places Ghroth puts a Cultist ----------------------------------

// react decides a Devolve window: a Cultist becomes a Deep One when the area
// is threatened and another Cultist stays to hold the Gate (or no Gate is
// there), and in a battle where the extra die is worth more than the Cultist.
func (g *guide) react(p Prompt, legal []state.Move) []state.Move {
	e, seat := g.e, g.seat
	var list []scoredMove
	for _, mv := range legal {
		if mv.Action == actOath {
			// Sacrifice the High Priest when it buys a way back into the game
			// or saves him; before the first player is chosen when it matters.
			s := 5.0
			if p.By == 0 {
				// Gather Power: when it puts the cult in front.
				if e.oathAtGather(seat) {
					s = 75
				}
			} else if e.d.Power[seat] <= 0 || e.oathThreatened(seat) {
				s = 75
			}
			list = append(list, scoredMove{mv, s})
			continue
		}
		if mv.Action == actWalk {
			// A Shambler is worth most where our units are, or a Gate to guard.
			a := e.areaIdx(string(mv.To))
			s := 55 + 4*float64(e.total(a, seat))
			if e.d.Gates[e.areaID(a)] == seat {
				s += 15
			}
			if g.need[a] > 0 {
				s += 10
			}
			list = append(list, scoredMove{mv, s})
			continue
		}
		if mv.Action != actDevolve {
			list = append(list, scoredMove{mv, scoreDoneTurn})
			continue
		}
		a := e.areaIdx(string(mv.To))
		s := 5.0
		mayLeave := e.count(a, seat, uCultist) >= 2 || e.d.Gates[e.areaID(a)] != seat
		switch {
		case !mayLeave:
			s = -20 // the Gate would be abandoned
		case p.Area != "":
			s = 70 // a die for the battle that was just declared
		case e.protectionRank(a, seat) == 0 && g.threatAt(a) > 0:
			s = 72 // a guard where Cultists were bare
		}
		list = append(list, scoredMove{mv, s})
	}
	return top(list, 0.01)
}

// place picks where Ghroth's Acolyte goes: one of ours onto an abandoned Gate
// or into a free area, an enemy's under our monsters where it cannot be
// protected — and nothing that helps an enemy otherwise.
func (g *guide) place(legal []state.Move) []state.Move {
	e, d, seat := g.e, g.e.d, g.seat
	var list []scoredMove
	for _, mv := range legal {
		if mv.Action != actPlace {
			return legal
		}
		a := e.areaIdx(string(mv.To))
		s := 0.0
		if mv.Count == seat {
			s = 30
			if ctl, ok := d.Gates[e.areaID(a)]; ok && ctl == 0 {
				s += 40
			}
			if !e.hasGate(a) && !g.enemyIn(a) {
				s += 15
			}
			s += 3 * float64(e.total(a, seat))
			s -= 6 * float64(g.enemyRank(a))
		} else {
			s = -10
			if e.protectionRank(a, seat) > e.protectionRank(a, mv.Count) {
				s = 45 + 5*float64(e.protectionRank(a, seat))
				if _, ok := d.Gates[e.areaID(a)]; ok {
					s += 5
				}
			}
		}
		list = append(list, scoredMove{mv, s})
	}
	return top(list, 0.01)
}

// --- battle abilities -------------------------------------------------------------------

// pre chooses among the Pre-Battle abilities: Abduct and Seek and Destroy
// when they are there, Invisibility on the strongest enemy Monster, Absorb on
// units that fight for nothing.
func (g *guide) pre(legal []state.Move) []state.Move {
	e, seat := g.e, g.seat
	var list []scoredMove
	for _, mv := range legal {
		s := 0.0
		switch mv.Action {
		case actDone:
			s = scoreDoneTurn
		case actAbduct:
			s = 66
		case actSeek:
			s = 60
		case actAbsorb:
			switch unitClassOf(mv.Piece) {
			case classCultist:
				s = 64
				// Not the Cultist that holds a Gate.
				if b := e.d.Bat; b != nil && g.leavesGateEmpty(e.areaIdx(b.Area), uCultist) {
					s = 20
				}
			default:
				s = 40
			}
		case actHowl:
			s = 66
		case actTerror:
			s = 60
			if mv.Piece == "foe" {
				s = 61
			}
		case actShrivel:
			// Take the strongest Monster out (or the Cultist, if that is all).
			s = 52
			if u, ok := e.f(e.d.Bat.foeOf(seat)).unitSpec(mv.Piece); ok {
				s += 3 * float64(u.Combat+u.Cost)
			}
		case actInvisible:
			if mv.Count == 0 || mv.Count == seat {
				s = 25 // our own unit: rarely worth a Polyp
			} else {
				s = 55
				if u, ok := e.f(mv.Count).unitSpec(mv.Piece); ok {
					s += 3 * float64(u.Combat)
				}
			}
		}
		list = append(list, scoredMove{mv, s})
	}
	return top(list, guideSlack+g.t.Slack)
}

// post chooses the Post-Battle abilities: the Harbinger takes the Elder Signs;
// Necrophagy is used when it costs the foe at least as much as it costs us.
func (g *guide) post(legal []state.Move) []state.Move {
	d, seat := g.e.d, g.seat
	var list []scoredMove
	for _, mv := range legal {
		s := 0.0
		switch mv.Action {
		case actHarbinger:
			s = 40
			if mv.Piece == "signs" {
				s = 60
			}
		case actCannibal:
			s = 55
			if mv.Piece == uWendigo {
				s = 70
			}
		case actUpgrade:
			// Promote what is cheap to promote; a Spawn is not broken up.
			if mv.Piece == uSpawn {
				s = 0
			} else {
				s = 66
			}
		case actNecro:
			s = 30
			if b := d.Bat; b != nil {
				if seat != b.Att && seat != b.Def {
					s = 45 // not our battle: a Pain for each of them
				} else if b.Pains[b.foeOf(seat)] >= b.Pains[seat] {
					s = 60
				}
			}
		case actDone:
			s = scoreDoneTurn
		}
		list = append(list, scoredMove{mv, s})
	}
	return top(list, guideSlack+g.t.Slack)
}
