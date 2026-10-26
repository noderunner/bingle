package cthulhuwars

import (
	"github.com/noderunner/bingle/internal/engine/state"
)

// The neutral content of the Azathoth faction (Onslaught One): six neutral
// spellbooks that any cult may take in place of one of its own, and four
// neutral Monsters whose Loyalty Cards any cult may buy with Doom. Azathoth
// himself is an Independent Great Old One and is not part of this.
//
// A neutral spellbook has the global book number bookSlots+n (n below
// nNeutralBooks): the bit of that number in Data.Books says the seat holds it.
// A cult's own books keep the numbers 0..5.

const bookSlots = 6

// Neutral spellbook numbers.
const (
	nbShrivel = iota
	nbStars
	nbRecrim
	nbMao
	nbUmr
	nbUndim
	nNeutralBooks
)

// neutralBooks are the six neutral spellbooks, in the order of the faction
// card. Key is the short name used in setup options and game ids.
var neutralBooks = [nNeutralBooks]bookSpec{
	{ID: "shriveling", Name: "Shriveling", Timing: "Pre-Battle", Text: "Select an enemy Monster or Cultist in the battle. That unit is eliminated, and its owner receives Power equal to the unit's cost."},
	{ID: "stars-are-right", Name: "Stars Are Right", Timing: "Ongoing", Text: "During the Doom phase, if you turn in Elder Signs for Doom points, you also immediately receive Power equal to their face value."},
	{ID: "recriminations", Name: "Recriminations", Timing: "Action: Cost 1", Text: "Remove any spellbook (including this one) from your faction card and replace it with another available spellbook."},
	{ID: "mao-ceremony", Name: "The Mao Ceremony", Timing: "Ongoing", Text: "At the end of Gather Power, after Power has been added (before the first player is determined), you may sacrifice 1 or more of your own Cultists, adding 1 Power apiece to your total."},
	{ID: "umr-at-tawil", Name: "Umr at-Tawil", Timing: "Ongoing", Text: "Gates now cost you 2 Power to build."},
	{ID: "undimensioned", Name: "Undimensioned", Timing: "Action: Cost 2", Text: "Rearrange your units among their areas as you see fit. You may completely empty an area, but you may not move to any new areas."},
}

// neutralKeys are the short names of the neutral spellbooks (no dashes: they
// go into the id of a game).
var neutralKeys = [nNeutralBooks]string{"shrivel", "stars", "recrim", "mao", "umr", "undim"}

// neutralBookByKey finds a neutral spellbook by its short name, -1 if none.
func neutralBookByKey(k string) int {
	for i, key := range neutralKeys {
		if key == k {
			return i
		}
	}
	return -1
}

// timerBooks are the spellbooks that need a clock — Ghroth gives the enemies a
// minute to decide, The Thousand Forms a negotiation — which a cult may leave
// out of its card in favour of a neutral one: they are awkward to play when the
// players take their turns days apart.
var timerBooks = map[int]int{facBG: bkGhroth, facCC: bkForms}

// Neutral monsters: the Loyalty Cards of the Azathoth faction.
const (
	uShambler   = "dimensional-shambler"
	uStarVamp   = "star-vampire"
	uElderThing = "elder-thing"
	uServitor   = "servitor"
)

// neutralUnits are the four neutral Monsters, in the order of the faction card.
var neutralUnits = []unitSpec{
	{ID: uShambler, Name: "Dimensional Shambler", Class: classMonster, Count: 3, Cost: 2, Combat: 2,
		Text:      "Walk Between Worlds: when summoned, it goes on your faction card. After any Action (by any player) you may place one or more Shamblers from your card in any area; once placed they stay on the map.",
		Abilities: []abilitySpec{{Name: "Walk Between Worlds", Timing: "Ongoing", Text: "When summoned, place on your faction card. After any Action (by any player) place one or more Dimensional Shamblers from your faction card in any area. Once placed, Dimensional Shamblers remain on the map (until killed or otherwise eliminated)."}}},
	{ID: uStarVamp, Name: "Star Vampire", Class: classMonster, Count: 3, Cost: 2, Combat: 1,
		Text:      "Vampirism: its dice are rolled separately; each Pain drains 1 Power from the enemy and each Kill drains 1 Doom, which you receive.",
		Abilities: []abilitySpec{{Name: "Vampirism", Timing: "Battle", Text: "Roll the Star Vampire's combat dice separately. Each Pain they roll drains 1 Power from the enemy faction. Each Kill they roll drains 1 Doom point from the enemy faction. The drained points are transferred to you immediately. If the enemy lacks Power or Doom points, you get nothing. The Pains and Kills rolled still count towards your Combat results."}}},
	{ID: uElderThing, Name: "Elder Thing", Class: classMonster, Count: 3, Cost: 2, Combat: 2,
		Text:      "Mind Control: while it shares an area with an enemy Great Old One, that Great Old One cannot use its special ability.",
		Abilities: []abilitySpec{{Name: "Mind Control", Timing: "Ongoing", Text: "If an Elder Thing shares an area with an enemy Great Old One, the latter may not use its special ability."}}},
	{ID: uServitor, Name: "Servitor of the Outer Gods", Class: classMonster, Count: 3, Cost: 1, Combat: -1,
		Text:      "Adulation: the owner of this card may not summon any Monster except Servitors while any Servitor is still in the pool. Each Servitor in a battle takes 1 die away from its owner.",
		Abilities: []abilitySpec{{Name: "Adulation", Timing: "Ongoing", Text: "The owner of this Loyalty Card may not summon any Monsters except Servitors if any Servitors are still in his pool."}}},
}

// loyaltyText is how a Loyalty Card is bought, for the guide and the buttons.
var loyaltyText = map[string]string{
	uShambler:   "Pay 2 Doom to obtain this Loyalty Card, and place 1 Dimensional Shambler on your faction card.",
	uStarVamp:   "Pay 2 Doom to obtain this Loyalty Card, and place 1 Star Vampire at a Gate you control.",
	uElderThing: "Pay 2 Doom to obtain this Loyalty Card, and place 1 Elder Thing at a Gate you control.",
	uServitor:   "Pay 2 Doom for this Loyalty Card, then hand it to another player (do not place a Servitor): the Adulation handicap is theirs.",
}

// lateUnit says typ belongs to a cult or the neutral set that was added after
// the first six cults: its moves are listed at the end of the action space.
func lateUnit(typ string) bool {
	if isNeutralUnit(typ) || lateCard(typ) {
		return true
	}
	for _, f := range factions[facOW:] {
		for _, u := range f.Units {
			if u.ID == typ && u.ID != uCultist {
				return true
			}
		}
	}
	return false
}

// isNeutralUnit says typ is one of the neutral Monsters.
func isNeutralUnit(typ string) bool {
	for _, u := range neutralUnits {
		if u.ID == typ {
			return true
		}
	}
	return false
}

// neutralUnitIndex is the number of a neutral Monster (its bit in Data.Loyal),
// -1 for any other unit.
func neutralUnitIndex(typ string) int {
	for i, u := range neutralUnits {
		if u.ID == typ {
			return i
		}
	}
	return -1
}

// --- what a cult may choose -------------------------------------------------------------

// bookSpecOf is the spellbook with global number b (own or neutral) as seat
// sees it.
func (e *eng) bookSpecOf(seat, b int) bookSpec {
	if b >= bookSlots {
		return neutralBooks[b-bookSlots]
	}
	return e.f(seat).Books[b]
}

// bookByID finds a spellbook by id in any faction or among the neutral ones;
// it returns its global number.
func bookByID(id string) (int, bool) {
	for _, f := range factions {
		for i, b := range f.Books {
			if b.ID == id {
				return i, true
			}
		}
	}
	for i, b := range neutralBooks {
		if b.ID == id {
			return bookSlots + i, true
		}
	}
	return 0, false
}

// hasNeutral says seat can use the neutral spellbook n: it holds it and no
// Moonbeast lies on it.
func (d *Data) hasNeutral(seat, n int) bool {
	return has(d.Neutral[seat], n) && !d.mooned(seat, bookSlots+n)
}

// heldNeutral says seat holds the neutral spellbook n (a Moonbeast may lie on it).
func (d *Data) heldNeutral(seat, n int) bool { return has(d.Neutral[seat], n) }

// holds says seat holds the spellbook with global number b (own or neutral).
func (d *Data) holds(seat, b int) bool {
	if b >= bookSlots {
		return d.heldNeutral(seat, b-bookSlots)
	}
	return has(d.Books[seat], b)
}

// setBook gives seat the spellbook with global number b, or takes it away.
func (d *Data) setBook(seat, b int, on bool) {
	mask, bit := &d.Books[seat], b
	if b >= bookSlots {
		mask, bit = &d.Neutral[seat], b-bookSlots
	}
	if on {
		*mask |= 1 << uint(bit)
	} else {
		*mask &^= 1 << uint(bit)
	}
}

// swapOf is the neutral spellbook that replaces the timer book of the cult in
// seat (set up with the game), -1 when it keeps its own.
func (e *eng) swapOf(seat int) int {
	f := e.fac(seat)
	if f < 0 {
		return -1
	}
	if _, ok := timerBooks[f]; !ok {
		return -1
	}
	return e.m.cfg.swap[f]
}

// swappedOut is the own spellbook of seat that was replaced at setup, -1 if
// none.
func (e *eng) swappedOut(seat int) int {
	if e.swapOf(seat) < 0 {
		return -1
	}
	return timerBooks[e.fac(seat)]
}

// reservedFor says the neutral spellbook n was set aside at setup for the cult
// in another seat than seat.
func (e *eng) reservedElsewhere(seat, n int) bool {
	for _, s := range e.playing() {
		if s != seat && e.swapOf(s) == n {
			return true
		}
	}
	return false
}

// neutralHeld says some seat holds the neutral spellbook n.
func (e *eng) neutralHeld(n int) bool { return e.neutralHolder(n) != 0 }

// bookChoices lists the spellbooks seat could take now, as global numbers: its
// own it does not hold (less the one replaced at setup), the neutral one set
// aside for it, and — with the Azathoth faction in the game — any neutral
// spellbook nobody holds or has set aside. skip is a book that must not be
// chosen (the one Recriminations has just removed), -1 for none.
func (e *eng) bookChoices(seat, skip int) []int {
	d := e.d
	var out []int
	out2 := func(b int) {
		if b != skip && !d.holds(seat, b) {
			out = append(out, b)
		}
	}
	gone := e.swappedOut(seat)
	for i := 0; i < bookSlots; i++ {
		if i != gone {
			out2(i)
		}
	}
	mine := e.swapOf(seat)
	for n := 0; n < nNeutralBooks; n++ {
		switch {
		case n == mine:
			out2(bookSlots + n)
		case e.m.cfg.az && !e.neutralHeld(n) && !e.reservedElsewhere(seat, n):
			out2(bookSlots + n)
		}
	}
	return out
}

// --- the neutral monsters ----------------------------------------------------------------

// loyal says seat bought the Loyalty Card of neutral Monster index i.
func (d *Data) loyal(seat, i int) bool { return d.Loyal[seat]&(1<<uint(i)) != 0 }

// cardOwner is the seat holding the Loyalty Card of neutral Monster i, 0 if
// nobody does.
func (e *eng) cardOwner(i int) int {
	for _, s := range e.playing() {
		if e.d.loyal(s, i) {
			return s
		}
	}
	return 0
}

// adulation says seat may only summon Servitors: it holds their card and some
// are still in its pool.
func (e *eng) adulation(seat int) bool {
	return e.d.loyal(seat, neutralUnitIndex(uServitor)) && e.pool(seat, uServitor) > 0
}

// walkAvailable says seat has Shamblers on its faction card to put on the map.
func (e *eng) walkAvailable(seat int) bool { return e.d.Card[seat] > 0 }

// walkMoves are the areas a Shambler on seat's card may be placed in (any).
func (e *eng) walkMoves(seat int) []state.Move {
	if e.d.Card[seat] <= 0 {
		return nil
	}
	out := make([]state.Move, 0, len(e.m.ids)+1)
	for a := range e.m.ids {
		out = append(out, state.Move{Action: actWalk, To: e.m.ids[a]})
	}
	return append(out, state.Move{Action: actDone})
}

// walk places a Shambler from the faction card.
func (e *eng) walk(seat, a int) {
	if e.d.Card[seat] <= 0 {
		return
	}
	e.d.Card[seat]--
	e.add(a, seat, uShambler)
	e.say("Walk Between Worlds: a Dimensional Shambler of %s steps into %s.", e.seatName(seat), e.areaName(a))
	e.takeAbandonedGate(a, seat)
}

// mindControlled says the Great Old One of owner in area a cannot use its
// special ability: an enemy Elder Thing stands there.
func (e *eng) mindControlled(a, owner int) bool {
	if !e.m.cfg.az {
		return false
	}
	for _, s := range e.others(owner) {
		if e.count(a, s, uElderThing) > 0 {
			return true
		}
	}
	return false
}

// --- the neutral spellbooks ---------------------------------------------------------------

// unitCost is the Power a unit of owner's costs (a Cultist 1, a High Priest 3,
// otherwise its price now).
func (e *eng) unitCost(owner int, typ string) int {
	switch typ {
	case uCultist:
		return 1
	case uHighPriest:
		return hpSpec.Cost
	}
	if u, ok := e.f(owner).unitSpec(typ); ok {
		if typ == uGnoph {
			return e.summonCost(owner, typ)
		}
		return u.Cost
	}
	return 0
}

// shrivelMoves lists the units Shriveling may remove in the battle: any enemy
// Monster or Cultist (not a Great Old One).
func (e *eng) shrivelMoves(seat int) []state.Move {
	b := e.d.Bat
	if !e.d.hasNeutral(seat, nbShrivel) || b.Shrivelled[seat] {
		return nil
	}
	var out []state.Move
	foe := b.foeOf(seat)
	for _, typ := range e.battleTypes(b, foe) {
		if !isMonsterOrCultist(typ) {
			continue
		}
		out = append(out, state.Move{Action: actShrivel, Piece: typ})
	}
	return out
}

// shrivel eliminates the chosen unit and pays its owner its cost in Power.
func (e *eng) shrivel(seat int, typ string) {
	b := e.d.Bat
	a := e.areaIdx(b.Area)
	foe := b.foeOf(seat)
	b.Shrivelled[seat] = true
	cost := e.unitCost(foe, typ)
	e.eliminate(a, foe, typ, seat)
	e.d.Power[foe] += cost
	e.say("Shriveling: %s's %s withers away; %s receives %s.", e.seatName(foe), unitNameOf(typ), e.seatName(foe), plural(cost, "Power", "Power"))
}

// recriminateMoves lists the books seat may swap out: every spellbook it holds
// for which another one is available.
func (e *eng) recriminateMoves(seat int) []state.Move {
	d := e.d
	if !d.hasNeutral(seat, nbRecrim) || d.Power[seat] < 1 {
		return nil
	}
	var out []state.Move
	for b := 0; b < bookSlots+nNeutralBooks; b++ {
		if !d.holds(seat, b) || len(e.bookChoices(seat, b)) == 0 {
			continue
		}
		out = append(out, state.Move{Action: actRecrim, Piece: e.bookSpecOf(seat, b).ID, Count: b + 1})
	}
	return out
}

// recriminate removes a spellbook and queues the choice of its replacement.
func (e *eng) recriminate(seat int, mv state.Move) {
	d := e.d
	b := mv.Count - 1
	d.Power[seat]--
	d.setBook(seat, b, false)
	e.moonFree(seat, b)
	e.say("Recriminations: %s sets aside %s and chooses another spellbook.", e.seatName(seat), e.bookSpecOf(seat, b).Name)
	d.Prompts = append(d.Prompts, Prompt{Kind: "book", Seat: seat, Skip: b + 1})
	e.mainTaken()
	e.thenAfterPrompts("after")
	e.beginPrompts("after")
}

// undimMoves: Undimensioned costs 2 Power and is a main action.
func (e *eng) undimAvailable(seat int) bool {
	return e.d.hasNeutral(seat, nbUndim) && e.d.Power[seat] >= 2 && e.areasWithUnits(seat) > 0
}

// beginUndim pays for Undimensioned and starts the rearrangement.
func (e *eng) beginUndim(seat int) {
	d := e.d
	d.Power[seat] -= 2
	var areas []string
	for a := range e.m.ids {
		if e.total(a, seat) > 0 {
			areas = append(areas, e.areaID(a))
		}
	}
	e.say("Undimensioned: %s rearranges their units among the areas where they have some.", e.seatName(seat))
	d.Prompts = append(d.Prompts, Prompt{Kind: "undim", Seat: seat, Only: areas})
	e.mainTaken()
	e.thenAfterPrompts("after")
	e.beginPrompts("after")
}

// undimMoves lists what the rearranging seat may do: move one unit that does
// not hold a Gate from an area into another area of the original set.
func (e *eng) undimMoves(p Prompt) []state.Move {
	var out []state.Move
	for _, from := range p.Only {
		fa := e.areaIdx(from)
		for _, typ := range e.typesIn(fa, p.Seat) {
			if typ == uFilth {
				continue // Filth never moves
			}
			n := e.count(fa, p.Seat, typ)
			if e.d.Gates[from] == p.Seat && e.holderAt(fa) == typ {
				n-- // the unit on the Gate stays on it
			}
			if n <= 0 {
				continue
			}
			for _, to := range p.Only {
				if to != from {
					out = append(out, state.Move{Action: actRearrange, Piece: typ, From: e.m.ids[fa], To: boardID(to)})
				}
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return append(out, state.Move{Action: actDone})
}

// rearrange moves one unit for Undimensioned.
func (e *eng) rearrange(seat int, mv state.Move) {
	from, to := e.areaIdx(string(mv.From)), e.areaIdx(string(mv.To))
	if e.take(from, seat, mv.Piece) {
		e.add(to, seat, mv.Piece)
		e.say("Undimensioned: %s's %s goes from %s to %s.", e.seatName(seat), unitNameOf(mv.Piece), e.areaName(from), e.areaName(to))
		e.syncGate(from)
		e.takeAbandonedGate(to, seat)
	}
}

// maoMoves lists the Cultists seat may sacrifice to The Mao Ceremony.
func (e *eng) maoMoves(p Prompt) []state.Move {
	var out []state.Move
	for a := range e.m.ids {
		for _, typ := range []string{uCultist, uHighPriest} {
			if e.count(a, p.Seat, typ) > 0 {
				out = append(out, state.Move{Action: actMao, Piece: typ, To: e.m.ids[a]})
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return append(out, state.Move{Action: actDone})
}

// mao sacrifices one Cultist for 1 Power.
func (e *eng) mao(seat int, mv state.Move) {
	a := e.areaIdx(string(mv.To))
	e.eliminate(a, seat, mv.Piece, 0)
	e.d.Power[seat]++
	e.say("The Mao Ceremony: %s sacrifices a Cultist in %s for 1 Power (now %d).", e.seatName(seat), e.areaName(a), e.d.Power[seat])
}

// vampirism settles the Star Vampires' share of seat's dice (the first ones
// rolled, one per Star Vampire in the battle): each Pain they rolled drains 1
// Power from foe and each Kill 1 Doom, which seat receives. The results still
// count in the battle.
func (e *eng) vampirism(seat, foe int, dice []int) {
	b := e.d.Bat
	sv := e.battleUnits(b, seat, uStarVamp)
	if sv == 0 {
		return
	}
	if sv > len(dice) {
		sv = len(dice)
	}
	pains, kills := 0, 0
	for _, v := range dice[:sv] {
		switch {
		case v == 6:
			kills++
		case v >= 4:
			pains++
		}
	}
	d := e.d
	power, doom := minInt(pains, d.Power[foe]), minInt(kills, d.Doom[foe])
	if pains+kills == 0 {
		return
	}
	d.Power[foe] -= power
	d.Power[seat] += power
	d.Doom[foe] -= doom
	e.addDoom(seat, doom, doomGifts)
	e.say("Vampirism: %s's Star Vampires drain %s and %s from %s.", e.seatName(seat), plural(power, "Power", "Power"), plural(doom, "Doom", "Doom"), e.seatName(foe))
	if d.Doom[seat] >= e.game.ParamInt("doom_to_win", 30) {
		d.EndPending = true
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// neutralCard is the card of what the Azathoth faction offers seat: its
// neutral spellbooks and Loyalty Cards, and who holds each. It is nil when the
// faction is not in the game and nothing was swapped in.
func (e *eng) neutralCard(seat int) *CardView {
	if !e.m.cfg.az && e.swapOf(seat) < 0 {
		return nil
	}
	card := &CardView{ID: "neutral", Title: "Neutral (Azathoth)"}
	for n, b := range neutralBooks {
		row := RowView{Text: b.Name, Detail: b.Timing + ": " + b.Text}
		switch holder := e.neutralHolder(n); {
		case holder == seat:
			row.State = "done"
		case holder != 0:
			row.Text += " — held by " + e.seatName(holder)
		case e.swapOf(seat) == n:
			row.State = "todo"
			row.Text += " — set aside for you"
		case e.reservedElsewhere(seat, n):
			row.Text += " — set aside for another cult"
		case e.m.cfg.az:
			row.State = "todo"
			row.Text += " — available"
		default:
			continue
		}
		card.Rows = append(card.Rows, row)
	}
	if e.m.cfg.az {
		for i, u := range neutralUnits {
			row := RowView{Text: u.Name, Detail: loyaltyText[u.ID] + " " + u.Text}
			switch owner := e.cardOwner(i); {
			case owner == seat:
				row.State = "done"
			case owner != 0:
				row.Text += " — card held by " + e.seatName(owner)
			default:
				row.Text += " — available for 2 Doom"
			}
			card.Rows = append(card.Rows, row)
		}
	}
	return card
}

// neutralHolder is the seat holding the neutral spellbook n, 0 if none.
func (e *eng) neutralHolder(n int) int {
	for _, s := range e.playing() {
		if e.d.heldNeutral(s, n) {
			return s
		}
	}
	return 0
}

// costOfType is the printed cost of a unit type (the High Priest 3).
func costOfType(typ string) int {
	if typ == uHighPriest {
		return hpSpec.Cost
	}
	for _, f := range factions {
		if u, ok := f.unitSpec(typ); ok {
			return u.Cost
		}
	}
	return 0
}

// loyaltyCard is the card of the other Loyalty Cards in the game — the
// Dreamlands and Beyond Time and Space Monsters, the Terror, the Independent
// Great Old Ones — and who holds each. It is nil when none is in the box.
func (e *eng) loyaltyCard(seat int) *CardView {
	card := &CardView{ID: "loyalty", Title: "Loyalty Cards"}
	for i := len(neutralUnits); i < len(cards); i++ {
		if !e.cardInGame(i) {
			continue
		}
		c := cards[i]
		u := c.Unit
		row := RowView{Text: u.Name, Detail: c.Buy + " " + u.Text}
		owner := e.cardOwner(i)
		switch {
		case c.Kind == cardIndependent:
			ib := cards[i].Book
			row.Detail = "Awaken: " + numbered(c.Awaken) + " Spellbook " + ib.Name + " (" + ib.Timing + "): " + ib.Text + " Requirement: " + c.Req
			switch {
			case owner == seat:
				row.State = "done"
				row.Text += " — yours"
				if e.igooBook(seat, u.ID) {
					row.Text += ", spellbook earned"
				} else {
					row.Text += ", spellbook not yet earned"
				}
			case owner != 0:
				row.Text += " — controlled by " + e.seatName(owner)
			default:
				row.Text += " — in the pool: awaken for " + cardCostText(c)
			}
		case owner == seat:
			row.State = "done"
		case owner != 0:
			row.Text += " — card held by " + e.seatName(owner)
		default:
			row.Text += " — available for " + cardCostText(c)
		}
		card.Rows = append(card.Rows, row)
	}
	if len(card.Rows) == 0 {
		return nil
	}
	return card
}
