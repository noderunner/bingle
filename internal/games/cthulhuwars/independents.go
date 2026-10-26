package cthulhuwars

import (
	"strings"

	"github.com/noderunner/bingle/internal/engine/state"
)

// The five Independent Great Old Ones of Great Old One Pack 1. An Independent
// belongs to no cult: whoever has one of its own Great Old Ones standing with a
// Gate it controls may awaken it. It has a spellbook of its own that its
// controller earns when the card's requirement is met; that spellbook counts as
// none of the cult's six, and falls off the card when the Independent dies.

// independent is the card data of one Independent Great Old One.
type independent struct {
	Unit   unitSpec
	Book   bookSpec
	Req    string
	Awaken []string
}

// awakenIndependent are the steps shared by four of the five.
func awakenIndependent(name string) []string {
	return []string{
		"Your Controlled Gate is in an Area with your Great Old One.",
		"Pay 4 Power, and place " + name + " in the Area containing the Gate.",
	}
}

var independents = []independent{
	{
		Unit: unitSpec{ID: uAbhoth, Name: "Abhoth", Class: classGOO, Count: 1, Cost: 4, Combat: 0,
			Text: "Combat equals the number of Filth Tokens in play (0–12).",
			Abilities: []abilitySpec{{Name: "Filth", Timing: "Action: Cost 1",
				Text: "Place a Filth Token in any Area."}}},
		Book: bookSpec{ID: "the-brood", Name: "The Brood", Timing: "Ongoing",
			Text:  "Gates in Areas containing Filth Tokens do not count during the Doom phase. Does not apply to Abhoth's Faction.",
			Units: []string{uAbhoth, uFilth}},
		Req:    "Choose One: EITHER your Faction has 4+ different Monster types in play, including Filth Tokens, OR Your faction has 8+ total Monsters in play, including Filth Tokens.",
		Awaken: awakenIndependent("Abhoth"),
	},
	{
		Unit: unitSpec{ID: uChaugnar, Name: "Chaugnar Faugn", Class: classGOO, Count: 1, Cost: 4, Combat: 3,
			Abilities: []abilitySpec{{Name: "Miri Nigri", Timing: "Ongoing",
				Text: "In a Battle taking place in an Area with your controlled Gate, add +3 Combat dice."}}},
		Book: bookSpec{ID: "curse-of-chaugnar-faugn", Name: "Curse of Chaugnar Faugn", Timing: "Ongoing",
			Text:  "Turn all Elder Signs in the Pool face-up. When any player receives an Elder Sign, you pick which one they receive. If you lose this Spellbook, turn the Elder Signs face-down again, and mix them up.",
			Units: []string{uChaugnar}},
		Req:    "As your Action for a Round, discard 2 Elder Signs (you receive no Doom for them).",
		Awaken: awakenIndependent("Chaugnar Faugn"),
	},
	{
		Unit: unitSpec{ID: uCthugha, Name: "Cthugha", Class: classGOO, Count: 1, Cost: 6, Combat: 0,
			Text: "Combat equals the Combat of an enemy Great Old One in the Battle (your choice); 0 if there is none.",
			Abilities: []abilitySpec{{Name: "Fire Vampires", Timing: "Post-Battle",
				Text: "If Cthugha is involved in a Battle, after all Kill results are assigned but before they are applied, you may choose to spare one or more Killed enemy Units by reducing their Kill to a Pain instead. For each enemy Unit you spare in this way, you gain 1 Power. You cannot use Fire Vampires (or Firestorm) against an enemy with zero Power (or a Hibernating Windwalker)."}}},
		Book: bookSpec{ID: "firestorm", Name: "Firestorm", Timing: "Post-Battle",
			Text:  "If Cthugha is involved in a Battle, for each Killed enemy unit you \"spare\", you also gain 1 Elder Sign.",
			Units: []string{uCthugha}},
		Req: "Kill an enemy Great Old One in Battle.",
		Awaken: []string{
			"Your Controlled Gate is in an Area with your Great Old One.",
			"Pay Power equal to 6 minus your Great Old One's Awakening Power Cost. If the result is negative, gain the result in Power.",
			"Replace your Great Old One with Cthugha.",
		},
	},
	{
		Unit: unitSpec{ID: uHydra, Name: "Mother Hydra", Class: classGOO, Count: 1, Cost: 4, Combat: 0,
			Text: "Combat equals 6 minus the number of enemy units in the Area (minimum 1).",
			Abilities: []abilitySpec{{Name: "The Agony Sting", Timing: "Action: Cost 1",
				Text: "Choose any Ocean Area. All enemy Cultists in that Area must be moved into Adjacent Land Areas by their owners (your Cultists are immune). In a dispute over who moves first, you decide."}}},
		Book: bookSpec{ID: "the-zygote", Name: "The Zygote", Timing: "Action: Cost 1",
			Text:  "Take all Acolyte Cultists in your Pool and place them on the map in any Areas in which you have a Unit.",
			Units: []string{uHydra, uCultist}},
		Req:    "Choose one: EITHER control no Great Old Ones in Ocean Areas OR enemy factions control no Great Old Ones in Ocean Areas.",
		Awaken: awakenIndependent("Mother Hydra"),
	},
	{
		Unit: unitSpec{ID: uYig, Name: "Yig", Class: classGOO, Count: 1, Cost: 4, Combat: 2,
			Abilities: []abilitySpec{{Name: "Snakebite", Timing: "Post-Battle",
				Text: "Your Cultists are now poisonous. If any of your Cultists are Killed in Battle, the enemy receives 1 extra Kill result. (He only takes 1 extra Kill regardless of how many Cultists die.)"}}},
		Book: bookSpec{ID: "messenger-of-yig", Name: "Messenger of Yig", Timing: "Ongoing",
			Text:  "In the Doom phase, each other player must decide if they will donate 1 Power from their total to you. For each player that refuses, gain 1 Doom Point.",
			Units: []string{uYig}},
		Req:    "As your Action for a round, remove one of your controlled Gates from the map.",
		Awaken: awakenIndependent("Yig"),
	},
}

// --- awakening ----------------------------------------------------------------------------

// igooIndex is the number of an Independent among the five (the bit of
// Data.IBooks), -1 for any other unit.
func igooIndex(typ string) int {
	if i := cardIndex(typ); i >= firstIndependent {
		return i - firstIndependent
	}
	return -1
}

// controlsGateAt says seat controls a Gate in area a: its Gate token, or — for
// the Opener — Yog-Sothoth, who is one.
func (e *eng) controlsGateAt(a, seat int) bool {
	return e.d.Gates[e.areaID(a)] == seat || (e.is(seat, facOW) && e.count(a, seat, uYog) > 0)
}

// independentAwakenMoves lists the Independent Great Old Ones seat may awaken:
// each stands in the general pool, and is awakened where seat controls a Gate
// in an area with a Great Old One of its own. Cthugha replaces that Great Old
// One, one move for each it may replace.
func (e *eng) independentAwakenMoves(seat int) []state.Move {
	if !e.independentsInGame() {
		return nil
	}
	var out []state.Move
	for i := firstIndependent; i < len(cards); i++ {
		typ := cards[i].Unit.ID
		if e.cardOwner(i) != 0 || !e.cardInGame(i) {
			continue
		}
		for _, a := range e.gatesOf(seat) {
			goos := e.seatGOOAt(a, seat)
			if len(goos) == 0 {
				continue
			}
			if typ != uCthugha {
				out = append(out, state.Move{Action: actAwaken, Piece: typ, To: e.m.ids[a]})
				continue
			}
			for _, g := range goos {
				out = append(out, state.Move{Action: actAwaken, Piece: typ, To: e.m.ids[a], Count: unitTypeIndex[g] + 1})
			}
		}
	}
	return out
}

// replacedBy is the Great Old One an awakening of Cthugha replaces ("" for any
// other awakening).
func replacedBy(mv state.Move) string {
	if mv.Piece == uCthugha && mv.Count >= 1 && mv.Count <= len(unitTypes) {
		return unitTypes[mv.Count-1]
	}
	return ""
}

// awakenIndependent settles the awakening of an Independent: the Power is paid,
// Cthugha's victim leaves, the card, the spellbook and the tokens go to seat and
// the figure appears at the Gate.
func (e *eng) awakenIndependent(seat int, mv state.Move) {
	d := e.d
	typ := mv.Piece
	a := e.areaIdx(string(mv.To))
	i := cardIndex(typ)
	cost := e.pay(seat, mv)
	if g := replacedBy(mv); g != "" {
		// "Replace your Great Old One with Cthugha."
		if !e.take(a, seat, g) {
			return
		}
		e.setPool(seat, g, e.pool(seat, g)+1)
		if isIndependent(g) {
			e.igooDies(seat, g)
		}
		e.say("Cthugha replaces %s's %s in %s.", e.seatName(seat), unitNameOf(g), e.areaName(a))
	}
	d.Loyal[seat] |= 1 << uint(i)
	d.IBooks &^= 1 << uint(i-firstIndependent)
	e.setPool(seat, typ, 1)
	if typ == uAbhoth {
		// The twelve Filth Tokens that are not on the map already.
		e.setPool(seat, uFilth, filthSpec.Count-e.filthInPlay())
	}
	e.place(a, seat, typ)
	if cost >= 0 {
		e.say("%s awakens the Independent %s in %s (%d Power).", e.seatName(seat), unitNameOf(typ), e.areaName(a), cost)
	} else {
		e.say("%s awakens the Independent %s in %s and gains %s.", e.seatName(seat), unitNameOf(typ), e.areaName(a), plural(-cost, "Power", "Power"))
	}
}

// --- ongoing effects ----------------------------------------------------------------------------

// igooBook says seat controls the Independent typ and has earned its spellbook.
func (e *eng) igooBook(seat int, typ string) bool {
	i := igooIndex(typ)
	return i >= 0 && e.holdsCard(seat, typ) && e.d.IBooks&(1<<uint(i)) != 0
}

// earnIGOOBook puts the spellbook on the Independent's card (once).
func (e *eng) earnIGOOBook(seat int, typ string) {
	i := igooIndex(typ)
	if i < 0 || !e.holdsCard(seat, typ) || e.d.IBooks&(1<<uint(i)) != 0 {
		return
	}
	e.d.IBooks |= 1 << uint(i)
	e.say("%s earns the spellbook %s of %s.", e.seatName(seat), cards[firstIndependent+i].Book.Name, unitNameOf(typ))
}

// abilityWorks says seat can use the special ability of its Independent typ:
// it is on the map and no Elder Thing shares its area.
func (e *eng) abilityWorks(seat int, typ string) (int, bool) {
	if !e.holdsCard(seat, typ) {
		return 0, false
	}
	a, ok := e.gooArea(seat, typ)
	if !ok || e.mindControlled(a, seat) {
		return a, false
	}
	return a, true
}

// filthInPlay counts the Filth Tokens on the map, whoever's they are.
func (e *eng) filthInPlay() int {
	n := 0
	for _, s := range e.playing() {
		n += e.onMap(s, uFilth)
	}
	return n
}

// miriNigri says Chaugnar Faugn adds 3 dice for seat in a battle in area a: it
// controls him and a Gate there.
func (e *eng) miriNigri(seat, a int) bool {
	if _, ok := e.abilityWorks(seat, uChaugnar); !ok {
		return false
	}
	return e.controlsGateAt(a, seat)
}

// broodHolder is the seat whose Abhoth has earned The Brood and is on the map
// (0 when no Gates are being spoiled).
func (e *eng) broodHolder() int {
	for _, s := range e.playing() {
		if e.igooBook(s, uAbhoth) && e.onMap(s, uAbhoth) > 0 {
			return s
		}
	}
	return 0
}

// doomGateCount is how many Gates seat counts in the Doom phase: The Brood
// spoils the Gates standing in areas with Filth, except the Gates of Abhoth's
// own faction.
func (e *eng) doomGateCount(seat int) int {
	n := e.gateCount(seat)
	owner := e.broodHolder()
	if owner == 0 || owner == seat {
		return n
	}
	for _, a := range e.gatesOf(seat) {
		for _, s := range e.playing() {
			if e.count(a, s, uFilth) > 0 {
				n--
				break
			}
		}
	}
	return n
}

// --- Abhoth: Filth ------------------------------------------------------------------------------

// filthMoves lists the areas seat may put a Filth Token in: any, as an Action of
// 1 Power, while Abhoth is on the map and Filth is left.
func (e *eng) filthMoves(seat int) []state.Move {
	if _, ok := e.abilityWorks(seat, uAbhoth); !ok || e.pool(seat, uFilth) <= 0 {
		return nil
	}
	out := make([]state.Move, 0, len(e.m.ids))
	for a := range e.m.ids {
		out = append(out, state.Move{Action: actFilth, To: e.m.ids[a]})
	}
	return out
}

// filth places the Token.
func (e *eng) filth(seat, a int) {
	e.pay(seat, state.Move{Action: actFilth, To: e.m.ids[a]})
	if e.place(a, seat, uFilth) {
		e.say("Filth: %s puts a Filth Token in %s (%d in play).", e.seatName(seat), e.areaName(a), e.filthInPlay())
	}
	e.mainTaken()
}

// --- Mother Hydra: The Agony Sting, The Zygote ----------------------------------------------

// stingMoves lists the oceans seat may sting: those that hold an enemy Cultist.
func (e *eng) stingMoves(seat int) []state.Move {
	if _, ok := e.abilityWorks(seat, uHydra); !ok {
		return nil
	}
	var out []state.Move
	for a := range e.m.ids {
		if !e.m.ocean[a] {
			continue
		}
		for _, o := range e.others(seat) {
			if e.cultists(a, o) > 0 {
				out = append(out, state.Move{Action: actSting, To: e.m.ids[a]})
				break
			}
		}
	}
	return out
}

// sting empties the ocean of enemy Cultists: each of their owners moves them to
// adjacent land (the enemy first from the left of the stinger, which decides a
// dispute).
func (e *eng) sting(seat, a int) {
	e.pay(seat, state.Move{Action: actSting, To: e.m.ids[a]})
	e.say("The Agony Sting: %s drives the enemy Cultists out of %s.", e.seatName(seat), e.areaName(a))
	for _, o := range e.others(seat) {
		if e.cultists(a, o) > 0 {
			e.d.Prompts = append(e.d.Prompts, Prompt{Kind: "sting", Seat: o, By: seat, Area: e.areaID(a)})
		}
	}
	e.mainTaken()
	e.thenAfterPrompts("after")
	e.beginPrompts("after")
}

// stingTargets are the land areas next to the ocean of a "sting" prompt.
func (e *eng) stingTargets(p Prompt) []int {
	var out []int
	for _, n := range e.m.adj[e.areaIdx(p.Area)] {
		if !e.m.ocean[n] {
			out = append(out, n)
		}
	}
	return out
}

// stingMovesFor lists the moves of a "sting" prompt: a Cultist of the seat in
// the ocean, to any adjacent land area.
func (e *eng) stingMovesFor(p Prompt) []state.Move {
	a := e.areaIdx(p.Area)
	var out []state.Move
	for _, typ := range []string{uCultist, uHighPriest} {
		if e.count(a, p.Seat, typ) == 0 {
			continue
		}
		for _, to := range e.stingTargets(p) {
			out = append(out, state.Move{Action: actSting, Piece: typ, To: e.m.ids[to]})
		}
	}
	return out
}

// stingMove moves one Cultist out of the ocean.
func (e *eng) stingMove(p Prompt, mv state.Move) {
	a := e.areaIdx(p.Area)
	to := e.areaIdx(string(mv.To))
	if !e.take(a, p.Seat, mv.Piece) {
		return
	}
	e.add(to, p.Seat, mv.Piece)
	e.say("%s's %s flees from %s to %s.", e.seatName(p.Seat), unitNameOf(mv.Piece), e.areaName(a), e.areaName(to))
	e.syncGate(a)
	e.takeAbandonedGate(to, p.Seat)
}

// zygoteAvailable says seat may use The Zygote: it controls Mother Hydra and
// her spellbook, has an Acolyte in its pool and a unit on the map.
func (e *eng) zygoteAvailable(seat int) bool {
	return e.igooBook(seat, uHydra) && e.pool(seat, uCultist) > 0 && e.hasAnyUnits(seat)
}

// zygoteMoves lists the areas the next Acolyte may go to: any where seat has a
// unit.
func (e *eng) zygoteMoves(seat int) []state.Move {
	var out []state.Move
	for a := range e.m.ids {
		if e.total(a, seat) > 0 {
			out = append(out, state.Move{Action: actZygote, To: e.m.ids[a]})
		}
	}
	return out
}

// --- Chaugnar Faugn: Curse of Chaugnar Faugn --------------------------------------------------

// discardMoves lists the pairs of Elder Signs seat may discard for Chaugnar
// Faugn's requirement; Count is the two values, as 10a+b.
func (e *eng) discardMoves(seat int) []state.Move {
	d := e.d
	if !e.holdsCard(seat, uChaugnar) || e.igooBook(seat, uChaugnar) || len(d.Signs[seat]) < 2 {
		return nil
	}
	have := map[int]int{}
	for _, v := range d.Signs[seat] {
		have[v]++
	}
	var out []state.Move
	for a := 1; a <= 3; a++ {
		for b := a; b <= 3; b++ {
			if (a == b && have[a] >= 2) || (a != b && have[a] >= 1 && have[b] >= 1) {
				out = append(out, state.Move{Action: actDiscard, Count: 10*a + b})
			}
		}
	}
	return out
}

// discard puts two Elder Signs back in the pool and earns the spellbook.
func (e *eng) discard(seat int, mv state.Move) {
	d := e.d
	for _, v := range []int{mv.Count / 10, mv.Count % 10} {
		for i, x := range d.Signs[seat] {
			if x == v {
				d.Signs[seat] = append(d.Signs[seat][:i], d.Signs[seat][i+1:]...)
				d.Bag[v-1]++
				break
			}
		}
	}
	e.say("%s discards two Elder Signs (%d and %d) for no Doom.", e.seatName(seat), mv.Count/10, mv.Count%10)
	e.earnIGOOBook(seat, uChaugnar)
	e.mainTaken()
}

// curseOwner is the seat whose Chaugnar Faugn has earned the Curse (0: none).
func (e *eng) curseOwner() int {
	for _, s := range e.playing() {
		if e.igooBook(s, uChaugnar) {
			return s
		}
	}
	return 0
}

// pickValues lists the values of Elder Sign left in the pool: what the Curse's
// holder may give.
func (e *eng) pickValues() []int {
	var out []int
	for v := 1; v <= 3; v++ {
		if e.d.Bag[v-1] > 0 {
			out = append(out, v)
		}
	}
	return out
}

// openPicks asks the holder of the Curse for the value of every Elder Sign that
// was just given out. The question goes first in the queue, whatever the game
// was doing, and play resumes where it was.
func (e *eng) openPicks() {
	d := e.d
	if len(d.Picks) == 0 {
		return
	}
	owner := e.curseOwner()
	picks := d.Picks
	d.Picks = nil
	if owner == 0 || d.Phase == phaseOver {
		for _, s := range picks {
			e.settlePick(s, 0)
		}
		return
	}
	var ps []Prompt
	for _, s := range picks {
		ps = append(ps, Prompt{Kind: "pick", Seat: owner, By: s})
	}
	d.Prompts = append(ps, d.Prompts...)
	if d.Stage != stagePrompt {
		d.Resume = d.Stage
		d.Stage = stagePrompt
		d.Then = "resume"
	}
	e.routed = true
	e.normalizePrompts()
	if len(d.Prompts) == 0 {
		d.Stage = d.Resume
		e.runThen()
	}
}

// settlePick gives the pending Elder Sign of seat the value v (0: a random one
// from the pool, when the Curse is gone).
func (e *eng) settlePick(seat, v int) {
	d := e.d
	idx := -1
	for i, x := range d.Signs[seat] {
		if x == 0 {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	if v < 1 || v > 3 || d.Bag[v-1] == 0 {
		total := d.Bag[0] + d.Bag[1] + d.Bag[2]
		if total == 0 {
			d.Signs[seat] = append(d.Signs[seat][:idx], d.Signs[seat][idx+1:]...)
			e.addDoom(seat, 1, doomSigns)
			return
		}
		r := int(uint64(uint32(nextRandomFrom(e))) % uint64(total))
		for i := 0; i < 3; i++ {
			if r < d.Bag[i] {
				v = i + 1
				break
			}
			r -= d.Bag[i]
		}
	}
	d.Bag[v-1]--
	d.Signs[seat][idx] = v
}

// --- Yig: Snakebite, Messenger of Yig ----------------------------------------------------------

// gateOffMoves lists the Gates seat may remove for Yig's requirement.
func (e *eng) gateOffMoves(seat int) []state.Move {
	if !e.holdsCard(seat, uYig) || e.igooBook(seat, uYig) {
		return nil
	}
	var out []state.Move
	for _, a := range e.gatesOf(seat) {
		if e.d.Gates[e.areaID(a)] == seat {
			out = append(out, state.Move{Action: actGateOff, To: e.m.ids[a]})
		}
	}
	return out
}

// gateOff removes one of seat's controlled Gates from the map.
func (e *eng) gateOff(seat, a int) {
	id := e.areaID(a)
	delete(e.d.Gates, id)
	delete(e.d.Holders, id)
	e.say("%s removes the Gate in %s from the map.", e.seatName(seat), e.areaName(a))
	e.earnIGOOBook(seat, uYig)
	e.mainTaken()
}

// snakebite is the extra Kill Yig's poison gives the enemy of seat when some of
// its Cultists were Killed in the battle: one, however many died.
func (e *eng) snakebite(b *Battle) {
	for _, s := range []int{b.Att, b.Def} {
		if b.CultistKills[s] == 0 {
			continue
		}
		if _, ok := e.abilityWorks(s, uYig); !ok {
			continue
		}
		foe := b.foeOf(s)
		b.Venom[foe]++
		e.say("Snakebite: the poison of %s's dead Cultists gives %s one more Kill.", e.seatName(s), e.seatName(foe))
	}
}

// messengerOwner is the seat whose Yig has earned Messenger of Yig.
func (e *eng) messengerOwner() int {
	for _, s := range e.playing() {
		if e.igooBook(s, uYig) {
			return s
		}
	}
	return 0
}

// donateMoves are the answers to Messenger of Yig: give 1 Power, or refuse.
func (e *eng) donateMoves(p Prompt) []state.Move {
	out := []state.Move{{Action: actDonate, Piece: "refuse"}}
	if e.d.Power[p.Seat] >= 1 {
		out = append([]state.Move{{Action: actDonate, Piece: "give"}}, out...)
	}
	return out
}

// donate settles the answer of seat to Yig's owner.
func (e *eng) donate(p Prompt, give bool) {
	d := e.d
	if give && d.Power[p.Seat] >= 1 {
		d.Power[p.Seat]--
		d.Power[p.By]++
		e.say("Messenger of Yig: %s gives 1 Power to %s.", e.seatName(p.Seat), e.seatName(p.By))
		return
	}
	e.addDoom(p.By, 1, doomCards)
	e.say("Messenger of Yig: %s refuses, and %s gains 1 Doom (now %d).", e.seatName(p.Seat), e.seatName(p.By), d.Doom[p.By])
	if d.Doom[p.By] >= e.game.ParamInt("doom_to_win", 30) {
		d.EndPending = true
	}
}

// --- Cthugha: Fire Vampires, Firestorm ------------------------------------------------------------

// sparable says a Kill on seat's unit may be turned into a Pain by Fire
// Vampires: its enemy controls Cthugha, who stands in the battle, and seat has
// Power and is not hibernating.
func (e *eng) sparable(killer, seat int) bool {
	b := e.d.Bat
	if b == nil || !e.holdsCard(killer, uCthugha) || e.d.Power[seat] <= 0 || e.d.Hib[seat] {
		return false
	}
	if e.battleUnits(b, killer, uCthugha) == 0 {
		return false
	}
	a, ok := e.gooArea(killer, uCthugha)
	return ok && !e.mindControlled(a, killer)
}

// spareMoves lists the Killed units the holder of Cthugha may spare.
func (e *eng) spareMoves(p Prompt) []state.Move {
	b := e.d.Bat
	if b == nil {
		return nil
	}
	var out []state.Move
	seen := map[PU]bool{}
	for _, u := range b.Doomed {
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, state.Move{Action: actSpare, Piece: u.Type, Count: u.Seat})
	}
	if len(out) == 0 {
		return nil
	}
	return append(out, state.Move{Action: actDone})
}

// spare turns one Kill into a Pain and pays what Fire Vampires and Firestorm
// give.
func (e *eng) spare(p Prompt, mv state.Move) {
	b := e.d.Bat
	victim := mv.Count
	if victim == 0 {
		victim = b.foeOf(p.Seat)
	}
	for i, u := range b.Doomed {
		if u.Seat != victim || u.Type != mv.Piece {
			continue
		}
		b.Doomed = append(b.Doomed[:i], b.Doomed[i+1:]...)
		b.Pained = append(b.Pained, u)
		e.d.Power[p.Seat]++
		e.say("Fire Vampires: %s spares %s's %s — a Pain, not a Kill — and gains 1 Power.", e.seatName(p.Seat), e.seatName(victim), unitNameOf(u.Type))
		if e.igooBook(p.Seat, uCthugha) {
			e.say("Firestorm: %s also gains an Elder Sign.", e.seatName(p.Seat))
			e.gainSign(p.Seat)
		}
		return
	}
}

// settleDoomed lets the Killed units that were not spared die.
func (e *eng) settleDoomed() {
	b := e.d.Bat
	doomed := b.Doomed
	b.Doomed = nil
	for _, u := range doomed {
		e.killUnit(u.Seat, u.Type, b.foeOf(u.Seat))
	}
}

// independentActions lists the main actions of seat's Independents: Abhoth's
// Filth, Mother Hydra's Sting and Zygote, and the two requirements that are
// Actions (Chaugnar Faugn's, Yig's).
func (e *eng) independentActions(seat int) []state.Move {
	p := e.d.Power[seat]
	var out []state.Move
	add := func(mv state.Move) {
		if p >= e.moveCost(seat, mv) {
			out = append(out, mv)
		}
	}
	for _, mv := range e.filthMoves(seat) {
		add(mv)
	}
	for _, mv := range e.stingMoves(seat) {
		add(mv)
	}
	if e.zygoteAvailable(seat) {
		add(state.Move{Action: actZygote})
	}
	out = append(out, e.discardMoves(seat)...)
	out = append(out, e.gateOffMoves(seat)...)
	return out
}

// carryMoves lists the free moves of a Cultist a Shantak carries: from the
// area the Shantak left to the one it went to, wherever that is.
func (e *eng) carryMoves(seat int) []state.Move {
	d := e.d
	if d.Move == nil || len(d.Move.Carry) == 0 {
		return nil
	}
	var out []state.Move
	seen := map[string]bool{}
	for _, c := range d.Move.Carry {
		if seen[c] {
			continue
		}
		seen[c] = true
		from, to, _ := strings.Cut(c, ">")
		a := e.areaIdx(from)
		occupant := ""
		if d.Gates[from] == seat {
			occupant = e.holderAt(a)
		}
		for _, typ := range []string{uCultist, uHighPriest} {
			n := e.count(a, seat, typ)
			for _, k := range d.Move.Moved {
				if k == from+"|"+typ {
					n--
				}
			}
			if typ == occupant {
				if n-1 > 0 {
					out = append(out, state.Move{Action: actGo, Piece: typ, From: boardID(from), To: boardID(to)})
				}
				if n > 0 {
					out = append(out, state.Move{Action: actLeave, Piece: typ, From: boardID(from), To: boardID(to)})
				}
				continue
			}
			if n > 0 {
				out = append(out, state.Move{Action: actGo, Piece: typ, From: boardID(from), To: boardID(to)})
			}
		}
	}
	return out
}

// gooInOceans says seat has a Great Old One — its own or an Independent — in an
// Ocean area (Cthulhu submerged is in one).
func (e *eng) gooInOceans(seat int) bool {
	if e.is(seat, facGC) && e.d.Away[uCthulhu] > 0 {
		return true
	}
	for a := range e.m.ids {
		if e.m.ocean[a] && e.classCount(a, seat, classGOO) > 0 {
			return true
		}
	}
	return false
}

// zygotePlace puts one Acolyte of the pool in area a.
func (e *eng) zygotePlace(seat, a int) {
	if e.place(a, seat, uCultist) {
		e.say("The Zygote: %s places an Acolyte in %s.", e.seatName(seat), e.areaName(a))
		e.takeAbandonedGate(a, seat)
	}
}
