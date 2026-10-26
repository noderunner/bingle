package cthulhuwars

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/state"
)

// A battle runs in the order of the rulebook: Pre-Battle abilities (Devour,
// Absorb, Abduct, Invisibility, Seek and Destroy), the dice, the Kill and
// Pain results applied to units (Regenerate, Emissary and Vengeance bend
// this), the Post-Battle abilities (Necrophagy adds Pains, the Harbinger pays
// for Great Old Ones that were hit), and last the retreats (Madness chooses
// where the pained units go).

// battleUnits counts units of type typ seat has in the battle (not hidden by
// Invisibility).
func (e *eng) battleUnits(b *Battle, seat int, typ string) int {
	a := e.areaIdx(b.Area)
	n := e.count(a, seat, typ) - b.Exempt[fmt.Sprintf("%d|%s", seat, typ)]
	if n < 0 {
		n = 0
	}
	return n
}

func (e *eng) battleTotal(b *Battle, seat int) int {
	n := 0
	for _, typ := range unitTypes {
		n += e.battleUnits(b, seat, typ)
	}
	return n
}

// battleTypes lists the unit types seat has in the battle.
func (e *eng) battleTypes(b *Battle, seat int) []string {
	var out []string
	for _, typ := range unitTypes {
		if e.battleUnits(b, seat, typ) > 0 {
			out = append(out, typ)
		}
	}
	return out
}

// foeOf is the seat opposing seat in the battle.
func (b *Battle) foeOf(seat int) int {
	if seat == b.Def {
		return b.Att
	}
	return b.Def
}

// combatOf is the number of dice seat rolls in area a against foe (b may be
// nil outside a battle: Invisibility and Absorb then do not apply).
func (e *eng) combatOf(seat, a int, b *Battle, foe int) int {
	d := e.d
	n := func(typ string) int {
		if b != nil {
			return e.battleUnits(b, seat, typ)
		}
		return e.count(a, seat, typ)
	}
	total := 0
	switch e.fac(seat) {
	case facGC:
		total += n(uDeepOne) + 2*n(uShoggoth) + 3*n(uStarspawn)
		if b != nil && b.Absorbed > 0 && n(uShoggoth) > 0 {
			total += 3 * b.Absorbed
		}
	case facCC:
		total += n(uPolyp) + 2*n(uHorror)
	case facBG:
		total += n(uFungi) + 2*n(uYoung)
		if d.hasBook(seat, bkFrenzy) {
			total += n(uCultist) + n(uHighPriest)
		}
	case facYS:
		if u := n(uUndead); u > 1 {
			total += u - 1
		}
		if by := n(uByak); by > 0 {
			total += by + 1
		}
	case facWW:
		total += n(uWendigo) + 3*n(uGnoph)
	case facSL:
		total += n(uSerpent)
		if fs := n(uFormless); fs > 0 {
			// Each rolls a die for every Formless Spawn on the map, and one
			// more while Tsathoggua is in play.
			total += fs * (e.onMap(seat, uFormless) + e.onMap(seat, uTsath))
		}
	case facOW:
		total += n(uMutant) + 2*n(uAbom) + 3*n(uSpawn)
	case facTT:
		total += n(uProto)
		// Terror: lower the enemy's Combat or raise one's own.
		if b != nil && b.Terror[seat] == 2 {
			total += n(uProto)
		}
	}
	// Great Old Ones: the cult's own and its Independents.
	for _, typ := range e.typesIn(a, seat) {
		if unitClassOf(typ) == classGOO {
			if k := n(typ); k > 0 {
				total += k * e.gooDice(seat, typ, a, b, foe)
			}
		}
	}
	// The Tcho-Tcho's Terror, chosen by the enemy of this seat.
	if b != nil && foe != 0 && b.Terror[foe] == 1 {
		total -= e.battleUnits(b, foe, uProto)
	}
	// The neutral Monsters and Terrors of the Loyalty Cards.
	total += 2*n(uShambler) + n(uStarVamp) + 2*n(uElderThing) - n(uServitor)
	total += 2*n(uGnorri) + 2*n(uShantak) + n(uVoonith) + 4*n(uHound)
	// Miri Nigri: Chaugnar Faugn adds 3 dice to a battle at a Gate its owner controls.
	if e.miriNigri(seat, a) {
		total += 3
	}
	if total < 0 {
		total = 0
	}
	return total
}

// gooDice is the number of dice one Great Old One of type typ of seat rolls in
// area a against foe (b may be nil outside a battle).
func (e *eng) gooDice(seat int, typ string, a int, b *Battle, foe int) int {
	d := e.d
	switch typ {
	case uCthulhu:
		return 6
	case uNyar:
		// Nyarlathotep: the spellbooks on his faction card and his enemy's.
		books := d.bookCount(seat)
		if foe != 0 {
			books += d.bookCount(foe)
		}
		return books
	case uShub:
		sum := len(e.gatesOf(seat)) + e.cultistsInPlay(seat)
		if d.hasBook(seat, bkRedSign) {
			sum += e.onMap(seat, uYoung)
		}
		return sum
	case uHastur:
		return e.ritualCost()
	case uRhan:
		return 3
	case uIthaqua:
		// Half of the enemy's Doom, rounded up.
		doom := 0
		if foe != 0 {
			doom = d.Doom[foe]
		}
		return (doom + 1) / 2
	case uTsath:
		// 2, or the enemy's Power when that is more.
		c := 2
		if foe != 0 && d.Power[foe] > c {
			c = d.Power[foe]
		}
		return c
	case uYog:
		return e.yogCombat(seat)
	case uUbbo:
		return d.Growth
	case uAbhoth:
		return e.filthInPlay()
	case uChaugnar:
		return 3
	case uYig:
		return 2
	case uHydra:
		// 6 minus the enemy units in the Area, at least 1.
		enemies := 0
		if foe != 0 {
			if b != nil {
				enemies = e.battleTotal(b, foe)
			} else {
				enemies = e.total(a, foe)
			}
		}
		if c := 6 - enemies; c > 1 {
			return c
		}
		return 1
	case uCthugha:
		// The Combat of an enemy Great Old One in the battle (the best one:
		// it is its owner's choice); 0 when there is none.
		if foe == 0 {
			return 0
		}
		best := 0
		for _, g := range e.typesIn(a, foe) {
			if unitClassOf(g) != classGOO || (b != nil && e.battleUnits(b, foe, g) == 0) {
				continue
			}
			if c := e.gooDice(foe, g, a, b, seat); c > best {
				best = c
			}
		}
		return best
	}
	return 0
}

// --- starting a battle -----------------------------------------------------------

func (e *eng) startBattle(att, def, a int, main bool) {
	d := e.d
	d.Battled[att] = append(d.Battled[att], e.areaID(a))
	d.Bat = &Battle{Area: e.areaID(a), Att: att, Def: def, Step: "pre", Who: att, Main: main, Exempt: map[string]int{}, Called: map[string]int{}, Retreater: 0}
	e.say("%s attacks %s in %s.", e.seatName(att), e.seatName(def), e.areaName(a))
	// Energy Nexus: just before a battle in an area with a Wizard the Sleeper
	// may take an Action that starts there.
	if sl := e.seatOf(facSL); sl != 0 && d.hasBook(sl, bkNexus) && e.count(a, sl, uWizard) > 0 && d.Power[sl] >= 1 {
		d.Prompts = append(d.Prompts, Prompt{Kind: "nexus", Seat: sl, Area: e.areaID(a)})
		e.thenAfterPrompts("battle-open")
		e.beginPrompts("battle-open")
		return
	}
	e.openBattle()
}

// openBattle is what happens when a battle is declared: the defender's Devolve
// and Great Cthulhu's Devour.
func (e *eng) openBattle() {
	d := e.d
	b := d.Bat
	a := e.areaIdx(b.Area)
	att, def := b.Att, b.Def
	// A defender with Devolve may answer the declaration: its Cultists there
	// become Deep Ones (each rolls a die) before anything else happens.
	if e.devolveAvailable(def) && e.count(a, def, uCultist) > 0 {
		d.Prompts = append(d.Prompts, Prompt{Kind: "react", Seat: def, By: att, Why: "devolve", Area: e.areaID(a)})
	}
	// Devour: Cthulhu makes the enemy eliminate a monster or cultist.
	for _, s := range []int{att, def} {
		if e.is(s, facGC) && e.count(a, s, uCthulhu) > 0 && !e.mindControlled(a, s) {
			enemy := def
			if s == def {
				enemy = att
			}
			d.Prompts = append(d.Prompts, Prompt{Kind: "lose", Seat: enemy, Area: e.areaID(a), Why: "devour", Left: 1, By: s})
		}
	}
	e.thenAfterPrompts("battle-start")
	e.beginPrompts("battle-start")
}

// beginNexus lets the Sleeper take one Action that starts in the battle area:
// a turn of its own, inside the battle (the interrupted turn waits).
func (e *eng) beginNexus(p Prompt) {
	d := e.d
	saved := *d.Turn
	d.Saved = &saved
	d.Turn = &TurnState{Seat: p.Seat, Nexus: true}
	d.Move = nil
	d.Stage = stageTurn
	e.say("Energy Nexus: %s may take an Action in %s before the battle.", e.seatName(p.Seat), e.areaName(e.areaIdx(p.Area)))
	if len(e.legalNexus()) <= 1 {
		// Nothing to do there.
		e.endNexus()
	}
}

// endNexus ends the Sleeper's Nexus turn: the battle goes on.
func (e *eng) endNexus() {
	d := e.d
	if d.Saved != nil {
		d.Turn = d.Saved
		d.Saved = nil
	}
	d.Move = nil
	d.Stage = stageTurn
	e.routed = true
	e.runThen()
}

// legalNexus lists what the Sleeper may do in its Energy Nexus turn: one Action
// that starts in the battle area, or nothing.
func (e *eng) legalNexus() []state.Move {
	d := e.d
	t := d.Turn
	seat := t.Seat
	var out []state.Move
	if !t.Main && d.Power[seat] >= 1 && d.Bat != nil {
		area := d.Bat.Area
		for _, mv := range e.mainActions(seat) {
			if e.startsIn(mv, area) {
				out = append(out, mv)
			}
		}
	}
	return append(out, state.Move{Action: actDone})
}

// startsIn says an Action starts in the area: a unit leaves it, or the Action
// is done there.
func (e *eng) startsIn(mv state.Move, area string) bool {
	switch mv.Action {
	case actGo, actLeave, actBeyond:
		return string(mv.From) == area
	case actBattle, actHibernate, actLethargy, actBoon, actBestow, actBane, actPay, actSacrifice, actClaim, actSubmerge, actForms, actGhroth, actRecrim, actUndim,
		actFilth, actSting, actZygote, actDiscard:
		return false
	}
	return mv.To != "" && string(mv.To) == area
}

// preMoves lists the pre-battle abilities seat may use now.
func (e *eng) preMoves(seat int) []state.Move {
	d := e.d
	b := d.Bat
	a := e.areaIdx(b.Area)
	var out []state.Move
	switch e.fac(seat) {
	case facGC:
		if d.hasBook(seat, bkAbsorb) && e.battleUnits(b, seat, uShoggoth) > 0 {
			for _, typ := range e.battleTypes(b, seat) {
				if !isMonsterOrCultist(typ) {
					continue
				}
				if typ == uShoggoth && e.battleUnits(b, seat, uShoggoth) < 2 {
					continue
				}
				out = append(out, state.Move{Action: actAbsorb, Piece: typ})
			}
		}
	case facWW:
		// Howl: before the battle the enemy retreats a unit of its choice.
		if d.hasBook(seat, bkHowl) && !b.Howled && e.battleUnits(b, seat, uWendigo) > 0 && e.battleTotal(b, b.foeOf(seat)) > 0 {
			out = append(out, state.Move{Action: actHowl})
		}
	case facCC:
		if d.hasBook(seat, bkAbduct) && e.battleUnits(b, seat, uNightgaunt) > 0 {
			enemy := b.foeOf(seat)
			if e.battleTotal(b, enemy)-e.classBattle(b, enemy, classGOO)-e.classBattle(b, enemy, classTerror) > 0 {
				out = append(out, state.Move{Action: actAbduct})
			}
		}
		if d.hasBook(seat, bkInvisibility) && b.Invis < e.count(a, seat, uPolyp) {
			// Any Monster or Cultist of either faction may be hidden (not a
			// Great Old One), one per Flying Polyp present.
			for _, s := range []int{b.Att, b.Def} {
				for _, typ := range e.battleTypes(b, s) {
					if !isMonsterOrCultist(typ) {
						continue
					}
					mv := state.Move{Action: actInvisible, Piece: typ}
					if s != seat {
						mv.Count = s
					}
					out = append(out, mv)
				}
			}
		}
		if d.hasBook(seat, bkSeek) {
			// Any or all Hunting Horrors: the ones standing in each area come
			// together.
			for x := range e.m.ids {
				if x != a && e.count(x, seat, uHorror) > 0 {
					out = append(out, state.Move{Action: actSeek, To: e.m.ids[x]})
				}
			}
		}
	}
	// Terror, the Tcho-Tcho's.
	out = append(out, e.terrorMoves(seat)...)
	// Shriveling, a neutral spellbook any cult may hold.
	out = append(out, e.shrivelMoves(seat)...)
	return out
}

func (e *eng) classBattle(b *Battle, seat, class int) int {
	n := 0
	for _, typ := range unitTypes {
		if unitClassOf(typ) == class {
			n += e.battleUnits(b, seat, typ)
		}
	}
	return n
}

func (e *eng) battlePreStart() {
	d := e.d
	b := d.Bat
	d.Stage = stagePre
	b.Step = "pre"
	// Demand Sacrifice: while Tsathoggua is in play the enemy of the Sleeper
	// chooses, before the battle, what it will give.
	if !b.Demand {
		for _, s := range []int{b.Att, b.Def} {
			if e.is(s, facSL) && d.hasBook(s, bkDemand) && e.onMap(s, uTsath) > 0 {
				b.Demand = true
				d.Prompts = append(d.Prompts, Prompt{Kind: "demand", Seat: b.foeOf(s), By: s})
				e.thenAfterPrompts("battle-demand")
				e.beginPrompts("battle-demand")
				return
			}
		}
	}
	e.advancePre(b.Att)
}

// answerDemand applies the enemy's answer to Demand Sacrifice.
func (e *eng) answerDemand(p Prompt, choice string) {
	b := e.d.Bat
	if choice == "signs" {
		e.say("Demand Sacrifice: %s gives %s an Elder Sign.", e.seatName(p.Seat), e.seatName(p.By))
		e.gainSign(p.By)
		return
	}
	b.Soften = true
	e.say("Demand Sacrifice: all of %s's Kills against %s's units in this battle count as Pains.", e.seatName(p.Seat), e.seatName(p.By))
}

// howlMoves lists what the enemy may do when Howl forces it to retreat a unit:
// any unit in the area, into any adjacent area.
func (e *eng) howlMoves(p Prompt) []state.Move {
	if p.Area == "" {
		return nil
	}
	a := e.areaIdx(p.Area)
	var out []state.Move
	for _, typ := range e.typesIn(a, p.Seat) {
		for _, to := range e.m.adj[a] {
			out = append(out, state.Move{Action: actRetreat, Piece: typ, To: e.m.ids[to]})
		}
	}
	return out
}

// howlRetreat moves the unit the enemy chose out of the battle area.
func (e *eng) howlRetreat(p Prompt, mv state.Move) {
	a := e.areaIdx(p.Area)
	to := e.areaIdx(string(mv.To))
	if e.take(a, p.Seat, mv.Piece) {
		e.add(to, p.Seat, mv.Piece)
		e.say("Howl: %s's %s flees from %s to %s.", e.seatName(p.Seat), unitNameOf(mv.Piece), e.areaName(a), e.areaName(to))
		e.syncGate(a)
		e.takeAbandonedGate(to, p.Seat)
	}
}

// advancePre moves to the next seat that has a pre-battle ability, or rolls.
func (e *eng) advancePre(from int) {
	d := e.d
	b := d.Bat
	if e.battleTotal(b, b.Att) == 0 || e.battleTotal(b, b.Def) == 0 {
		e.rollBattle()
		return
	}
	order := []int{b.Att, b.Def}
	start := 0
	if from == b.Def {
		start = 1
	}
	for i := start; i < 2; i++ {
		s := order[i]
		if b.Done[s] {
			continue
		}
		if len(e.preMoves(s)) > 0 {
			b.Who = s
			d.Stage = stagePre
			return
		}
		b.Done[s] = true
	}
	e.rollBattle()
}

func (e *eng) legalPre() []state.Move {
	b := e.d.Bat
	out := e.preMoves(b.Who)
	return append(out, state.Move{Action: actDone})
}

func (e *eng) doPre(seat int, mv state.Move) {
	d := e.d
	b := d.Bat
	a := e.areaIdx(b.Area)
	switch mv.Action {
	case actDone:
		b.Done[seat] = true
		e.advancePre(seat)
		return
	case actAbsorb:
		e.eliminate(a, seat, mv.Piece, 0)
		b.Absorbed++
		e.say("Absorb: %s feeds %s to its Shoggoth (+3 dice).", e.seatName(seat), article(unitNameOf(mv.Piece)))
	case actShrivel:
		e.shrivel(seat, mv.Piece)
	case actTerror:
		b.Terror[seat] = 1
		if mv.Piece == "self" {
			b.Terror[seat] = 2
		}
		e.say("Terror: %s's Proto-Shoggoths %s.", e.seatName(seat), map[string]string{"foe": "lower the enemy's Combat", "self": "add to their Combat"}[mv.Piece])
	case actAbduct:
		e.eliminate(a, seat, uNightgaunt, 0)
		enemy := b.foeOf(seat)
		e.say("Abduct: a Nightgaunt carries off one of %s's units.", e.seatName(enemy))
		d.Prompts = append(d.Prompts, Prompt{Kind: "lose", Seat: enemy, Area: b.Area, Why: "abduct", Left: 1, By: seat})
		e.thenAfterPrompts("battle-pre")
		e.beginPrompts("battle-pre")
		return
	case actHowl:
		b.Howled = true
		foe := b.foeOf(seat)
		e.say("Howl: the Wendigo's cry forces %s to retreat a unit from %s.", e.seatName(foe), e.areaName(a))
		d.Prompts = append(d.Prompts, Prompt{Kind: "howl", Seat: foe, Area: b.Area, By: seat})
		e.thenAfterPrompts("battle-pre")
		e.beginPrompts("battle-pre")
		return
	case actInvisible:
		owner := seat
		if mv.Count != 0 {
			owner = mv.Count
		}
		b.Exempt[fmt.Sprintf("%d|%s", owner, mv.Piece)]++
		b.Invis++
		e.say("Invisibility: %s of %s is hidden from the battle.", article(unitNameOf(mv.Piece)), e.seatName(owner))
	case actSeek:
		moved := 0
		for x := range e.m.ids {
			if x == a {
				continue
			}
			if mv.To != "" && e.m.ids[x] != mv.To {
				continue
			}
			n := e.count(x, seat, uHorror)
			for i := 0; i < n; i++ {
				e.take(x, seat, uHorror)
				e.add(a, seat, uHorror)
				moved++
			}
			e.syncGate(x)
		}
		e.say("Seek and Destroy: %s arrive in %s.", plural(moved, "Hunting Horror", "Hunting Horrors"), e.areaName(a))
	}
	// Keep going with this seat until it is done or has nothing left.
	if len(e.preMoves(seat)) == 0 {
		b.Done[seat] = true
		e.advancePre(seat)
	}
}

// --- rolling -----------------------------------------------------------------------

func (e *eng) rollBattle() {
	d := e.d
	b := d.Bat
	a := e.areaIdx(b.Area)
	last := &LastBattle{Area: b.Area, Att: b.Att, Def: b.Def}
	for _, s := range []int{b.Att, b.Def} {
		other := b.foeOf(s)
		n := e.combatOf(s, a, b, other)
		last.Combat[s] = n
		if e.battleTotal(b, s) == 0 {
			n = 0
		}
		dice := e.roll(n)
		if len(dice) >= 6 {
			b.SixDice[s] = true
		}
		b.Dice[s] = dice
	}
	d.Last = last
	// Channel Power: after the dice are rolled the Opener may pay to reroll
	// those that were not hits, as often as it likes.
	for _, s := range []int{b.Att, b.Def} {
		if len(e.channelMoves(s)) > 0 {
			d.Prompts = append(d.Prompts, Prompt{Kind: "channel", Seat: s})
		}
	}
	if len(d.Prompts) > 0 {
		e.thenAfterPrompts("battle-tally")
		e.beginPrompts("battle-tally")
		return
	}
	e.tallyBattle()
}

// tallyBattle turns the dice of both sides into Kills and Pains (once the
// Opener has finished rerolling) and starts assigning them.
func (e *eng) tallyBattle() {
	d := e.d
	b := d.Bat
	last := d.Last
	for _, s := range []int{b.Att, b.Def} {
		other := b.foeOf(s)
		dice := b.Dice[s]
		last.Dice[s] = append([]int(nil), dice...)
		kills, pains := 0, 0
		for _, v := range dice {
			switch {
			case v == 6:
				kills++
			case v >= 4:
				pains++
			}
		}
		e.say("%s rolls %d dice: %v — %d Kill, %d Pain.", e.seatName(s), len(dice), dice, kills, pains)
		// Vicious: each Voonith in the battle makes up a Kill the dice did not give.
		if v := e.battleUnits(b, s, uVoonith); v > kills {
			e.say("Vicious: %s's %s add %s (%d in all).", e.seatName(s), plural(v, "Voonith", "Vooniths"), plural(v-kills, "Kill", "Kills"), v)
			kills = v
		}
		b.Kills[other] = kills
		b.Pains[other] = pains
		last.Kills[other] = kills
		last.Pains[other] = pains
		e.vampirism(s, other, dice)
	}
	// Demand Sacrifice, refused: the Kills against the Sleeper are Pains.
	if b.Soften {
		for _, s := range []int{b.Att, b.Def} {
			if e.is(s, facSL) {
				b.Pains[s] += b.Kills[s]
				last.Pains[s] += b.Kills[s]
				b.Kills[s] = 0
				last.Kills[s] = 0
			}
		}
	}
	b.Rolled = true
	e.startAssign()
}

// --- assigning results -------------------------------------------------------------------

// assigner is who decides which of seat's units take its results.
func (e *eng) assigner(seat int) int {
	b := e.d.Bat
	enemy := b.foeOf(seat)
	// Hastur's Vengeance: its owner chooses which enemy units take results.
	if e.is(enemy, facYS) && e.battleUnits(b, enemy, uHastur) > 0 && !e.mindControlled(e.areaIdx(b.Area), enemy) {
		return enemy
	}
	return seat
}

// startAssign queues the Kills and Pains the dice scored — Kills first — for
// the attacker's units, then the defender's.
func (e *eng) startAssign() {
	d := e.d
	b := d.Bat
	b.Step = "assign"
	b.Queue = nil
	if !b.Assigned {
		for _, s := range []int{b.Att, b.Def} {
			for i := 0; i < b.Kills[s]; i++ {
				b.Queue = append(b.Queue, Assign{Seat: s, Who: e.assigner(s)})
			}
		}
		for _, s := range []int{b.Att, b.Def} {
			for i := 0; i < b.Pains[s]; i++ {
				b.Queue = append(b.Queue, Assign{Seat: s, Pain: true, Who: e.assigner(s)})
			}
		}
	}
	e.advanceAssign()
}

// assignOptions lists the unit types the front result may be applied to.
func (e *eng) assignOptions(as Assign) []string {
	b := e.d.Bat
	d := e.d
	var out []string
	pained := map[string]int{}
	for _, p := range b.Pained {
		if p.Seat == as.Seat {
			pained[p.Type]++
		}
	}
	for _, typ := range e.battleTypes(b, as.Seat) {
		n := e.battleUnits(b, as.Seat, typ)
		// Units that took a Kill Fire Vampires may still spare are as good as gone.
		for _, u := range b.Doomed {
			if u.Seat == as.Seat && u.Type == typ {
				n--
			}
		}
		// Angles of Time: the Hound of Tindalos cannot be assigned a Kill.
		if typ == uHound && !as.Pain {
			n = 0
		}
		if as.Pain {
			n -= pained[typ]
			// Units dragged in by Necrophagy take no part in the results.
			n -= b.Called[fmt.Sprintf("%d|%s", as.Seat, typ)]
			// A Star Spawn that took one result can take one more.
			if typ == uStarspawn && e.is(as.Seat, facGC) && d.hasBook(as.Seat, bkRegenerate) {
				n += b.Spare
			}
		}
		// Rhan-Tegoth receives only one result per battle.
		if typ == uRhan && b.Rhan {
			n = 0
		}
		if n > 0 {
			out = append(out, typ)
		}
	}
	return out
}

// finishAssign runs when no result is left to assign: the first time the
// Post-Battle abilities follow, the second time (the Pains they added are
// assigned) the pained units retreat.
func (e *eng) finishAssign() {
	d := e.d
	b := d.Bat
	// Fire Vampires: once every result is assigned, Cthugha's owner may turn
	// the Kills into Pains.
	if len(b.Doomed) > 0 {
		cth := b.foeOf(b.Doomed[0].Seat)
		if len(e.spareMoves(Prompt{Seat: cth})) > 0 {
			d.Prompts = append(d.Prompts, Prompt{Kind: "spare", Seat: cth})
			e.thenAfterPrompts("battle-spared")
			e.beginPrompts("battle-spared")
			return
		}
	}
	e.afterKills()
}

// afterKills goes on once the Kills are settled: the units not spared die. The
// first time the Post-Battle abilities follow (Snakebite, Berserkergang…); the
// second time (the results they added are assigned) the pained units retreat.
func (e *eng) afterKills() {
	d := e.d
	b := d.Bat
	e.settleDoomed()
	if b.Assigned {
		e.startRetreat()
		return
	}
	b.Assigned = true
	b.Step = "post"
	e.snakebite(b)
	// Berserkergang: for each Gnoph-Keh that took a Kill the enemy
	// eliminates a Monster or Cultist of its own — the card names no area,
	// so anywhere on the map, and never a Great Old One.
	if b.Berserk > 0 {
		if ww := e.seatOf(facWW); ww != 0 {
			foe := b.foeOf(ww)
			e.say("Berserkergang: %s must eliminate %s.", e.seatName(foe), plural(b.Berserk, "Monster or Cultist", "Monsters or Cultists"))
			d.Prompts = append(d.Prompts, Prompt{Kind: "lose", Seat: foe, Why: "berserkergang", Left: b.Berserk, By: ww, Only: e.berserkTargets(foe)})
		}
		b.Berserk = 0
		if len(d.Prompts) > 0 {
			e.thenAfterPrompts("battle-post")
			e.beginPrompts("battle-post")
			return
		}
	}
	e.advancePost(b.Att, true)
}

// berserkTargets lists the unit types of seat that Berserkergang may
// eliminate: Monsters and Cultists, not Great Old Ones.
func (e *eng) berserkTargets(seat int) []string {
	var out []string
	for _, u := range e.units(seat) {
		if u.Class == classMonster || u.Class == classCultist {
			out = append(out, u.ID)
		}
	}
	return out
}

func (e *eng) advanceAssign() {
	d := e.d
	b := d.Bat
	for len(b.Queue) > 0 {
		as := b.Queue[0]
		opts := e.assignOptions(as)
		if len(opts) == 0 {
			b.Queue = b.Queue[1:]
			continue
		}
		if len(opts) == 1 {
			if e.giveResult(as, opts[0]) {
				return // Rhan-Tegoth's owner is asked first
			}
			b.Queue = b.Queue[1:]
			continue
		}
		d.Stage = stageAssign
		return
	}
	e.finishAssign()
}

// giveResult applies one result to a unit — or, for Rhan-Tegoth, asks first
// whether his owner pays 1 Power to cancel it (Eternal). It reports whether
// that question was queued: the result then stays at the front of the queue
// until it is answered.
func (e *eng) giveResult(as Assign, typ string) bool {
	d := e.d
	b := d.Bat
	if typ == uRhan && e.is(as.Seat, facWW) && d.Power[as.Seat] >= 1 && !e.mindControlled(e.areaIdx(b.Area), as.Seat) {
		why := "Kill"
		if as.Pain {
			why = "Pain"
		}
		d.Prompts = append(d.Prompts, Prompt{Kind: "eternal", Seat: as.Seat, Why: why})
		e.thenAfterPrompts("battle-assign")
		e.beginPrompts("battle-assign")
		return true
	}
	if typ == uRhan {
		b.Rhan = true
	}
	e.applyResult(as, typ)
	return false
}

// resolveEternal settles the question of Eternal: pay 1 Power and the result is
// cancelled, or it is applied to Rhan-Tegoth.
func (e *eng) resolveEternal(pay bool) {
	d := e.d
	b := d.Bat
	p := d.Prompts[0]
	e.dropPrompt()
	if b == nil || len(b.Queue) == 0 {
		return
	}
	as := b.Queue[0]
	b.Queue = b.Queue[1:]
	b.Rhan = true
	if pay {
		d.Power[p.Seat]--
		e.say("Eternal: %s pays 1 Power and Rhan-Tegoth shrugs the %s off.", e.seatName(p.Seat), strings.ToLower(p.Why))
		return
	}
	e.applyResult(as, uRhan)
}

func (e *eng) legalAssign() []state.Move {
	b := e.d.Bat
	as := b.Queue[0]
	var out []state.Move
	for _, typ := range e.assignOptions(as) {
		mv := state.Move{Action: actAssign, Piece: typ}
		// A result for another faction's units (Hastur's Vengeance) names
		// the owner, so a click on the right unit is unambiguous.
		if as.Seat != as.Who {
			mv.Count = as.Seat
		}
		out = append(out, mv)
	}
	return out
}

func (e *eng) doAssign(seat int, mv state.Move) {
	b := e.d.Bat
	as := b.Queue[0]
	if e.giveResult(as, mv.Piece) {
		return
	}
	b.Queue = b.Queue[1:]
	e.advanceAssign()
}

// applyResult applies one Kill or Pain to a unit of the given type.
func (e *eng) applyResult(as Assign, typ string) {
	d := e.d
	b := d.Bat
	seat := as.Seat
	killer := b.foeOf(seat)
	// The Harbinger pays for enemy Great Old Ones that are Pained or Killed.
	if unitClassOf(typ) == classGOO && !e.is(seat, facCC) {
		b.Hit = append(b.Hit, PU{Seat: seat, Type: typ})
	}
	if as.Pain {
		// Regenerate: a Star Spawn that took one result takes a second Pain
		// without further effect.
		if e.is(seat, facGC) && typ == uStarspawn && d.hasBook(seat, bkRegenerate) && b.Spare > 0 {
			b.Spare--
			e.say("Regenerate: %s's Star Spawn takes a second result and is only Pained.", e.seatName(seat))
			return
		}
		b.Pained = append(b.Pained, PU{Seat: seat, Type: typ})
		if e.is(seat, facGC) && typ == uStarspawn && d.hasBook(seat, bkRegenerate) {
			b.Spare++
		}
		e.say("%s's %s takes a Pain.", e.seatName(seat), unitNameOf(typ))
		return
	}
	// Emissary of the Outer Gods turns a Kill on Nyarlathotep into a Pain.
	if e.is(seat, facCC) && typ == uNyar && d.hasBook(seat, bkEmissary) && e.classBattle(b, killer, classGOO) == 0 {
		b.Pained = append(b.Pained, PU{Seat: seat, Type: typ})
		e.say("Emissary of the Outer Gods: the Kill on Nyarlathotep is a Pain instead.")
		return
	}
	// Regenerate: the first result on a Star Spawn only Pains it; a second
	// Kill on the same one kills it. A fresh Star Spawn is hit before a
	// wounded one is finished off.
	if e.is(seat, facGC) && typ == uStarspawn && d.hasBook(seat, bkRegenerate) {
		fresh := e.battleUnits(b, seat, uStarspawn) - e.painedCount(b, seat, uStarspawn)
		if fresh > 0 {
			b.Pained = append(b.Pained, PU{Seat: seat, Type: typ})
			b.Spare++
			e.say("Regenerate: the Star Spawn shrugs the Kill off as a Pain.")
			return
		}
		if b.Spare > 0 {
			b.Spare--
			for i, p := range b.Pained {
				if p.Seat == seat && p.Type == uStarspawn {
					b.Pained = append(b.Pained[:i], b.Pained[i+1:]...)
					break
				}
			}
		}
	}
	// Fire Vampires: Cthugha's owner may still spare the unit, once all the
	// results are assigned.
	if e.sparable(killer, seat) {
		b.Doomed = append(b.Doomed, PU{Seat: seat, Type: typ})
		e.say("%s's %s takes a Kill.", e.seatName(seat), unitNameOf(typ))
		return
	}
	e.killUnit(seat, typ, killer)
}

// killUnit kills seat's unit of type typ in the battle area: it is eliminated
// and everything a kill sets off follows.
func (e *eng) killUnit(seat int, typ string, killer int) {
	d := e.d
	b := d.Bat
	a := e.areaIdx(b.Area)
	e.say("%s's %s is killed.", e.seatName(seat), unitNameOf(typ))
	e.eliminate(a, seat, typ, killer)
	b.Slain[killer]++
	if isCultist(typ) {
		b.CultistKills[seat]++
	}
	e.noteKill(killer, seat, typ)
	// Martyrdom: a Kill on the High Priest spares the others.
	if e.is(seat, facTT) && d.hasBook(seat, bkMartyr) {
		if typ == uHighPriest {
			e.martyrdom(seat)
		} else {
			b.Fallen = append(b.Fallen, PU{Seat: seat, Type: typ})
		}
	}
	// Cannibalism looks for dead enemies of the Windwalker; Berserkergang for
	// its own Gnoph-Keh that took a Kill.
	if !e.is(seat, facWW) {
		b.Killed++
	}
	if e.is(seat, facWW) && typ == uGnoph && d.hasBook(seat, bkBerserk) {
		b.Berserk++
	}
}

func (e *eng) painedCount(b *Battle, seat int, typ string) int {
	n := 0
	for _, p := range b.Pained {
		if p.Seat == seat && p.Type == typ {
			n++
		}
	}
	return n
}

// --- post-battle ----------------------------------------------------------------------

// postSeats is who may use post-battle abilities, in order: the attacker, the
// defender, then the other seats whose abilities reach into any battle.
func (e *eng) postSeats() []int {
	b := e.d.Bat
	order := []int{b.Att, b.Def}
	for _, f := range []int{facBG, facCC, facWW} {
		if s := e.seatOf(f); s != 0 && s != b.Att && s != b.Def {
			order = append(order, s)
		}
	}
	return order
}

// harbingerPending reports whether the Harbinger still has a Great Old One to
// pay for: Nyarlathotep stands in the battle and an enemy Great Old One was
// Pained or Killed.
func (e *eng) harbingerPending(seat int) (PU, bool) {
	b := e.d.Bat
	cc := e.seatOf(facCC)
	if cc == 0 || seat != cc || b.Paid >= len(b.Hit) || e.count(e.areaIdx(b.Area), cc, uNyar) == 0 || e.mindControlled(e.areaIdx(b.Area), cc) {
		return PU{}, false
	}
	if cc != b.Att && cc != b.Def {
		return PU{}, false
	}
	return b.Hit[b.Paid], true
}

func (e *eng) postMoves(seat int) []state.Move {
	d := e.d
	b := d.Bat
	a := e.areaIdx(b.Area)
	var out []state.Move
	if _, ok := e.harbingerPending(seat); ok {
		return []state.Move{{Action: actHarbinger, Piece: "power"}, {Action: actHarbinger, Piece: "signs"}}
	}
	if e.is(seat, facBG) && d.hasBook(seat, bkNecro) {
		for x := range e.m.ids {
			if x != a && e.count(x, seat, uGhoul) > 0 {
				out = append(out, state.Move{Action: actNecro})
				break
			}
		}
	}
	// The Million Favored Ones: units are replaced by the next kind up.
	out = append(out, e.upgradeMoves(seat)...)
	// Cannibalism: after enemy units died the Windwalker may place a Wendigo
	// or an Acolyte in the battle area — in any battle.
	if e.is(seat, facWW) && d.hasBook(seat, bkCannibal) && b.Killed > 0 {
		if e.pool(seat, uWendigo) > 0 {
			out = append(out, state.Move{Action: actCannibal, Piece: uWendigo})
		}
		if e.pool(seat, uCultist) > 0 {
			out = append(out, state.Move{Action: actCannibal, Piece: uCultist})
		}
	}
	return out
}

func (e *eng) advancePost(from int, fresh bool) {
	d := e.d
	b := d.Bat
	if fresh {
		b.Done = [nSeats + 1]bool{}
	}
	order := e.postSeats()
	skipping := fresh
	for _, s := range order {
		if !skipping && s == from {
			skipping = true
			if b.Done[s] {
				continue
			}
		}
		if !skipping {
			continue
		}
		if b.Done[s] {
			continue
		}
		if len(e.postMoves(s)) > 0 {
			b.Who = s
			d.Stage = stagePost
			return
		}
		b.Done[s] = true
	}
	// An old position may still hold results nobody has assigned.
	if !b.Assigned {
		e.startAssign()
		return
	}
	// The Pains Necrophagy added are assigned now, to units that took no
	// result yet.
	if b.Extra[b.Att]+b.Extra[b.Def]+b.Venom[b.Att]+b.Venom[b.Def] > 0 {
		b.Step = "assign"
		b.Queue = nil
		// Snakebite's Kills, then the Pains Necrophagy added.
		for _, s := range []int{b.Att, b.Def} {
			for i := 0; i < b.Venom[s]; i++ {
				b.Queue = append(b.Queue, Assign{Seat: s, Who: e.assigner(s)})
			}
			b.Venom[s] = 0
		}
		for _, s := range []int{b.Att, b.Def} {
			for i := 0; i < b.Extra[s]; i++ {
				b.Queue = append(b.Queue, Assign{Seat: s, Pain: true, Who: e.assigner(s)})
			}
			b.Extra[s] = 0
		}
		e.advanceAssign()
		return
	}
	e.startRetreat()
}

func (e *eng) legalPost() []state.Move {
	b := e.d.Bat
	out := e.postMoves(b.Who)
	if _, forced := e.harbingerPending(b.Who); forced {
		return out
	}
	return append(out, state.Move{Action: actDone})
}

func (e *eng) doPost(seat int, mv state.Move) {
	d := e.d
	b := d.Bat
	a := e.areaIdx(b.Area)
	switch mv.Action {
	case actDone:
		b.Done[seat] = true
		e.advancePost(seat, false)
		return
	case actHarbinger:
		hit := b.Hit[b.Paid]
		b.Paid++
		cost := e.awakenCost(hit.Seat, hit.Type)
		name := unitNameOf(hit.Type)
		if mv.Piece == "signs" {
			e.say("The Harbinger: %s takes 2 Elder Signs for %s.", e.seatName(seat), name)
			e.gainSign(seat)
			e.gainSign(seat)
		} else {
			d.Power[seat] += cost / 2
			e.say("The Harbinger: %s gains %d Power for %s (half of %d).", e.seatName(seat), cost/2, name, cost)
		}
		if _, more := e.harbingerPending(seat); !more {
			b.Done[seat] = true
			e.advancePost(seat, false)
		}
		return
	case actCannibal:
		if e.place(a, seat, mv.Piece) {
			if b.Called == nil {
				b.Called = map[string]int{}
			}
			// It arrives after the results were applied: it takes none.
			b.Called[fmt.Sprintf("%d|%s", seat, mv.Piece)]++
			e.say("Cannibalism: %s places %s in %s.", e.seatName(seat), article(unitNameOf(mv.Piece)), e.areaName(a))
			e.takeAbandonedGate(a, seat)
		}
		b.Done[seat] = true
		e.advancePost(seat, false)
	case actUpgrade:
		e.upgrade(seat, mv)
		if len(e.postMoves(seat)) == 0 {
			b.Done[seat] = true
			e.advancePost(seat, false)
		}
		return
	case actNecro:
		moved := 0
		for x := range e.m.ids {
			if x == a {
				continue
			}
			n := e.count(x, seat, uGhoul)
			for i := 0; i < n; i++ {
				e.take(x, seat, uGhoul)
				e.add(a, seat, uGhoul)
				moved++
			}
			e.syncGate(x)
		}
		if b.Called == nil {
			b.Called = map[string]int{}
		}
		b.Called[fmt.Sprintf("%d|%s", seat, uGhoul)] += moved
		if b.Assigned {
			b.Extra[b.Att] += moved
			b.Extra[b.Def] += moved
		} else {
			b.Pains[b.Att] += moved
			b.Pains[b.Def] += moved
		}
		e.say("Necrophagy: %s shamble into %s; each side suffers %s.", plural(moved, "Ghoul", "Ghouls"), e.areaName(a), plural(moved, "extra Pain", "extra Pains"))
		b.Done[seat] = true
		e.advancePost(seat, false)
	}
}

// --- retreating -------------------------------------------------------------------------------

// retreatAreas lists where a pained unit of seat may retreat: adjacent areas
// with none of the battle opponent's units.
func (e *eng) retreatAreas(seat int) []int {
	b := e.d.Bat
	a := e.areaIdx(b.Area)
	enemy := b.foeOf(seat)
	var out []int
	for _, n := range e.m.adj[a] {
		if e.total(n, enemy) == 0 {
			out = append(out, n)
		}
	}
	return out
}

func (e *eng) startRetreat() {
	d := e.d
	b := d.Bat
	b.Step = "retreat"
	b.Retreater = 0
	// A seat whose pained units cannot retreat loses one of them instead.
	for _, s := range []int{b.Att, b.Def} {
		var kinds []string
		var pained int
		for _, p := range b.Pained {
			if p.Seat == s {
				pained++
				if !containsStr(kinds, p.Type) {
					kinds = append(kinds, p.Type)
				}
			}
		}
		if pained == 0 || len(e.retreatAreas(s)) > 0 {
			continue
		}
		// Nyarlathotep, protected by Emissary, is not eliminated.
		var cand []string
		for _, k := range kinds {
			if (k == uNyar && e.is(s, facCC) && d.hasBook(s, bkEmissary)) || k == uHound {
				continue
			}
			cand = append(cand, k)
		}
		e.say("%s's pained units have nowhere to retreat.", e.seatName(s))
		// Angles of Time: a Hound that cannot be Pained away is eliminated.
		if containsStr(kinds, uHound) {
			a := e.areaIdx(b.Area)
			e.say("Angles of Time: the Hound of Tindalos of %s is eliminated.", e.seatName(s))
			e.eliminate(a, s, uHound, 0)
			var left []PU
			for _, p := range b.Pained {
				if p.Seat != s || p.Type != uHound {
					left = append(left, p)
				}
			}
			b.Pained = left
		}
		// Drop all of this seat's pained units (they stay), then eliminate one.
		var keep []PU
		for _, p := range b.Pained {
			if p.Seat != s {
				keep = append(keep, p)
			}
		}
		b.Pained = keep
		if len(cand) > 0 {
			d.Prompts = append(d.Prompts, Prompt{Kind: "lose", Seat: s, Area: b.Area, Why: "no retreat", Left: 1, By: b.foeOf(s), Only: cand})
		}
	}
	if len(d.Prompts) > 0 {
		e.thenAfterPrompts("battle-retreat")
		e.beginPrompts("battle-retreat")
		return
	}
	e.advanceRetreat()
}

// madnessWorks says whether Crawling Chaos chooses where the pained units
// go: it holds Madness, and it does so in every battle — its own or not.
func (e *eng) madnessWorks() bool {
	cc := e.seatOf(facCC)
	return cc != 0 && e.d.hasBook(cc, bkMadness)
}

func (e *eng) advanceRetreat() {
	d := e.d
	b := d.Bat
	// Madness: the retreats are Crawling Chaos' to arrange, in any order.
	if b.Retreater == 0 && !b.Declined && e.madnessWorks() && len(b.Pained) > 0 {
		b.Retreater = e.seatOf(facCC)
	}
	for len(b.Pained) > 0 {
		p := b.Pained[0]
		areas := e.retreatAreas(p.Seat)
		if len(areas) == 0 {
			b.Pained = b.Pained[1:]
			continue
		}
		if len(areas) == 1 && (len(b.Pained) == 1 || b.Retreater == 0) {
			e.retreat(p, areas[0])
			b.Pained = b.Pained[1:]
			continue
		}
		d.Stage = stageRetreat
		return
	}
	e.battleFinish()
}

func (e *eng) retreat(p PU, to int) {
	b := e.d.Bat
	a := e.areaIdx(b.Area)
	if e.take(a, p.Seat, p.Type) {
		e.add(to, p.Seat, p.Type)
		e.say("%s's %s retreats to %s.", e.seatName(p.Seat), unitNameOf(p.Type), e.areaName(to))
		e.syncGate(a)
		e.takeAbandonedGate(to, p.Seat)
	}
}

func (e *eng) legalRetreat() []state.Move {
	b := e.d.Bat
	var out []state.Move
	if b.Retreater == 0 {
		p := b.Pained[0]
		for _, to := range e.retreatAreas(p.Seat) {
			out = append(out, state.Move{Action: actRetreat, Piece: p.Type, To: e.m.ids[to]})
		}
		return out
	}
	// Crawling Chaos arranges them: any pained unit, in any order, into any
	// area its owner could have chosen.
	seen := map[PU]bool{}
	for _, p := range b.Pained {
		if seen[p] {
			continue
		}
		seen[p] = true
		for _, to := range e.retreatAreas(p.Seat) {
			mv := state.Move{Action: actRetreat, Piece: p.Type, To: e.m.ids[to]}
			if p.Seat != b.Retreater {
				mv.Count = p.Seat
			}
			out = append(out, mv)
		}
	}
	// Madness may be declined: the owners then choose.
	return append(out, state.Move{Action: actDone})
}

func (e *eng) doRetreat(seat int, mv state.Move) {
	b := e.d.Bat
	if mv.Action == actDone {
		b.Retreater = 0
		b.Declined = true
		e.say("%s lets the owners choose where their pained units go.", e.seatName(seat))
		e.advanceRetreat()
		return
	}
	idx := 0
	if mv.Count != 0 {
		idx = -1
		for i, p := range b.Pained {
			if p.Seat == mv.Count && p.Type == mv.Piece {
				idx = i
				break
			}
		}
	} else if b.Retreater != 0 {
		idx = -1
		for i, p := range b.Pained {
			if p.Seat == b.Retreater && p.Type == mv.Piece {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		return
	}
	p := b.Pained[idx]
	b.Pained = append(b.Pained[:idx], b.Pained[idx+1:]...)
	e.retreat(p, e.areaIdx(string(mv.To)))
	e.advanceRetreat()
}

// --- the end of a battle -------------------------------------------------------------------------

func (e *eng) battleFinish() {
	d := e.d
	b := d.Bat
	if b == nil {
		return
	}
	// Passion: Cultists lost to an enemy.
	for _, s := range e.playing() {
		if b.LostCultists[s] > 0 {
			e.passion(s)
		}
	}
	gc := e.seatOf(facGC)
	d.Bat = nil
	// The Sleeper's requirement: six dice or more in one battle.
	if sl := e.seatOf(facSL); sl != 0 && b.SixDice[sl] {
		e.fulfil(sl, rqSLRoll)
	}
	// Great Cthulhu's two kill requirements: one unit meets the first, two the
	// second, and three or more both in a single battle.
	if n := b.Slain[gc]; gc != 0 && n >= 1 {
		switch {
		case n >= 3:
			e.fulfil(gc, rqGCKill2)
			e.fulfil(gc, rqGCKill1)
		case n == 2:
			if !e.fulfil(gc, rqGCKill2) {
				e.fulfil(gc, rqGCKill1)
			}
		default:
			e.fulfil(gc, rqGCKill1)
		}
	}
	// A Gate left open by the battle goes to a unit that stands there: the
	// attacker's first, then the defender's, then any bystander's.
	if b.Area != "" {
		a := e.areaIdx(b.Area)
		e.syncGate(a)
		order := []int{b.Att, b.Def}
		for _, s := range e.playing() {
			if s != b.Att && s != b.Def {
				order = append(order, s)
			}
		}
		for _, s := range order {
			if s != 0 {
				e.takeAbandonedGate(a, s)
			}
		}
	}
	d.Stage = stageTurn
	if b.Main {
		e.mainTaken()
	} else {
		e.acted()
	}
	e.afterAction()
}
