package cthulhuwars

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Loyalty Cards: the units that belong to no cult until a player takes their
// card. There are three kinds.
//
//   - Neutral Monsters (the Azathoth faction's four, the Dreamlands Surface
//     Monster Pack's three, the two of Beyond Time and Space): a Loyalty Card is
//     bought in the Doom phase for 2 Doom and is the buyer's for the rest of the
//     game.
//   - Terrors (the Hound of Tindalos): bought the same way for 2 Doom and 2
//     Power. A Terror is summoned like a Monster but is another kind of unit.
//   - Independent Great Old Ones (Great Old One Pack 1): awakened in the Action
//     phase with a Great Old One of one's own at a controlled Gate. The card goes
//     back to the general pool when the Independent dies.
//
// Data.Loyal holds, per seat, the cards it has: bit i is cards[i]. The first
// four are the Azathoth faction's Monsters (neutralUnits); the order of the rest
// is fixed because positions are stored with it.

// Kinds of Loyalty Card.
const (
	cardMonster = iota
	cardTerror
	cardIndependent
)

// The packs a table may play with, as bits of gameCfg.packs.
const (
	packAzathoth   = 1 << iota // the Azathoth faction's neutral Monsters (and spellbooks: gameCfg.az)
	packDreamlands             // Dreamlands Surface Monster Pack
	packBeyond                 // Beyond Time and Space
	packGOO1                   // Great Old One Pack 1
)

// Unit ids of the new Loyalty Cards.
const (
	uMoonbeast = "moonbeast"
	uGnorri    = "gnorri"
	uShantak   = "shantak"
	uWamp      = "wamp"
	uVoonith   = "voonith"
	uHound     = "hound-of-tindalos"
	uAbhoth    = "abhoth"
	uChaugnar  = "chaugnar-faugn"
	uCthugha   = "cthugha"
	uHydra     = "mother-hydra"
	uYig       = "yig"
	uFilth     = "filth"
)

// cardSpec is one Loyalty Card: its figure and how it is obtained.
type cardSpec struct {
	Unit unitSpec
	Kind int
	Pack int
	// Buy says how the card is obtained (the text of the button and the guide).
	Buy string
	// Independent Great Old Ones only: the card's spellbook, the requirement
	// that earns it and the steps of awakening.
	Book   bookSpec
	Req    string
	Awaken []string
}

// filthSpec is Abhoth's Filth Token: a Monster of Combat 0 that never moves or
// takes an Action. Twelve of them come with Abhoth.
var filthSpec = unitSpec{
	ID: uFilth, Name: "Filth", Class: classMonster, Count: 12, Cost: 0, Combat: 0, Token: true,
	Text: "A Monster of Combat 0 that belongs to the faction that controls Abhoth. It never moves and takes no Action; it can be Killed or Pained like any Monster and is hit by the abilities that name Monsters.",
}

// cards lists every Loyalty Card, in the order of the bits of Data.Loyal.
var cards = func() []cardSpec {
	out := make([]cardSpec, 0, 16)
	for _, u := range neutralUnits {
		out = append(out, cardSpec{Unit: u, Kind: cardMonster, Pack: packAzathoth, Buy: loyaltyText[u.ID]})
	}
	out = append(out,
		cardSpec{Kind: cardMonster, Pack: packDreamlands, Buy: "Pay 2 Doom to obtain this Loyalty Card, then place 1 Moonbeast on an enemy Faction Spellbook.",
			Unit: unitSpec{ID: uMoonbeast, Name: "Moonbeast", Class: classMonster, Count: 4, Cost: 2, Combat: 0,
				Text:      "Blasphemous Obeisance: when summoned it goes on an enemy's spellbook, which that enemy cannot use while it lies there; in the next Doom phase it comes to one of your Gates.",
				Abilities: []abilitySpec{{Name: "Blasphemous Obeisance", Timing: "Ongoing", Text: "When a Moonbeast is Summoned, place it on an enemy's Faction Spellbook. While the Moonbeast is there, the player cannot use that Spellbook (it still counts for other purposes such as Unlimited Battle and winning the game). The next Doom phase, place the Moonbeast on one of your Gates. A Moonbeast can be prematurely returned to the map if the victim spends 1 Doom point (at any time, not as an Action)."}}}},
		cardSpec{Kind: cardMonster, Pack: packDreamlands, Buy: "Pay 2 Doom to obtain this Loyalty Card, plus place 1 Gnorri at your controlled Gate.",
			Unit: unitSpec{ID: uGnorri, Name: "Gnorri", Class: classMonster, Count: 3, Cost: 3, Combat: 2,
				Text:      "Grottos: in the Doom phase, 2 Gnorri in play earn you 1 extra Doom and 3 Gnorri 2 extra.",
				Abilities: []abilitySpec{{Name: "Grottos", Timing: "Ongoing", Text: "During the Doom phase, if you have two (2) Gnorri in play, you earn one extra Doom point. If you have three (3) Gnorri in play, you earn two extra Doom points."}}}},
		cardSpec{Kind: cardMonster, Pack: packDreamlands, Buy: "Pay 2 Doom to obtain this Loyalty Card, plus place 1 Shantak at your controlled Gate.",
			Unit: unitSpec{ID: uShantak, Name: "Shantak", Class: classMonster, Count: 2, Cost: 2, Combat: 2,
				Text:      "Horror Steed: a Shantak that moves can reach any area on the map, and may carry one of your Cultists with it for free.",
				Abilities: []abilitySpec{{Name: "Horror Steed", Timing: "Ongoing", Text: "When Moving a Shantak, it can reach any Area on the map. In addition, the Shantak may carry one of your Cultists with it for free."}}}},
		cardSpec{Kind: cardMonster, Pack: packBeyond, Buy: "Pay 2 Doom to obtain this Loyalty Card, then choose an enemy to place all 4 Wamps in any Area or Areas without a Gate.",
			Unit: unitSpec{ID: uWamp, Name: "Wamp", Class: classMonster, Count: 4, Cost: 1, Combat: 0,
				Text:      "Crypt Dweller: in the Doom phase you may choose an enemy, who places all the Wamps of your pool in areas without a Gate.",
				Abilities: []abilitySpec{{Name: "Crypt Dweller", Timing: "Doom Phase", Text: "Choose an enemy to place all Wamps from your Pool into any Area or Areas without a Gate. (You may still Summon Wamps normally.)"}}}},
		cardSpec{Kind: cardMonster, Pack: packBeyond, Buy: "Pay 2 Doom to obtain this Loyalty Card, then place 1 Voonith at one of your Controlled Gates.",
			Unit: unitSpec{ID: uVoonith, Name: "Voonith", Class: classMonster, Count: 2, Cost: 3, Combat: 1,
				Text:      "Vicious: for each Kill you score fewer than the number of Vooniths in the battle, add 1 Kill.",
				Abilities: []abilitySpec{{Name: "Vicious", Timing: "Post-Battle", Text: "For each Kill you score fewer than the number of Vooniths involved in the Battle, add 1 Kill (i.e. if you have 2 Vooniths in the Battle and roll 1 Kill, add 1 extra Kill for a total of 2; if you rolled no Kills, add 2 Kills)."}}}},
		cardSpec{Kind: cardTerror, Pack: packBeyond, Buy: "Pay 2 Doom and 2 Power to obtain this Loyalty Card, then place the Hound of Tindalos at any Gate (even one you don't Control).",
			Unit: unitSpec{ID: uHound, Name: "Hound of Tindalos", Class: classTerror, Count: 1, Cost: 4, Combat: 4,
				Text: "A Terror. Chronophage: it cannot Move by itself, but moves for free whenever you Move another unit, from a Gate to any other Gate. Angles of Time: it cannot be assigned a Kill, and it is eliminated when it stands in an area without a Gate.",
				Abilities: []abilitySpec{
					{Name: "Chronophage", Timing: "Ongoing", Text: "The Hound cannot perform the Move Action by itself. The Hound Moves for free whenever you Move any other Unit. In doing so, the Hound teleports directly from an Area with a Gate to another Area with a Gate, neither of which need to be Controlled."},
					{Name: "Angles of Time", Timing: "Ongoing", Text: "The Hound cannot be assigned a Kill in Battle. However, if a Hound is ever in an Area without a Gate (due to being Pained to such an Area, or the Gate itself being destroyed or moved, etc.), it is Eliminated. It can also be Eliminated if it cannot be Pained due to Enemy presence in adjacent Areas (per normal Pain rules)."}}}},
	)
	for _, ind := range independents {
		out = append(out, cardSpec{Unit: ind.Unit, Kind: cardIndependent, Pack: packGOO1, Buy: ind.Unit.Name + " is awakened in the Action phase (see Awaken).", Book: ind.Book, Req: ind.Req, Awaken: ind.Awaken})
	}
	return out
}()

// cardUnits lists the figures of the Loyalty Cards, in card order.
var cardUnits = func() []unitSpec {
	out := make([]unitSpec, len(cards))
	for i, c := range cards {
		out[i] = c.Unit
	}
	return out
}()

// cardIndex is the number of the Loyalty Card whose figure is typ, -1 for any
// other unit.
func cardIndex(typ string) int {
	for i := range cards {
		if cards[i].Unit.ID == typ {
			return i
		}
	}
	return -1
}

// isCardUnit says typ is the figure of a Loyalty Card.
func isCardUnit(typ string) bool { return cardIndex(typ) >= 0 }

// isIndependent says typ is an Independent Great Old One.
func isIndependent(typ string) bool {
	i := cardIndex(typ)
	return i >= 0 && cards[i].Kind == cardIndependent
}

// isMonsterOrCultist says a unit is a Monster or a Cultist: the units the
// abilities that name them (Devour, Abduct, Shriveling…) reach. A Terror is
// neither.
func isMonsterOrCultist(typ string) bool {
	c := unitClassOf(typ)
	return c == classMonster || c == classCultist
}

// lateCard says typ is a figure added after the first neutral Monsters: it
// comes last in the lists the agents' action space is made of.
func lateCard(typ string) bool {
	i := cardIndex(typ)
	return i >= len(neutralUnits) || typ == uFilth
}

// firstIndependent is the number of the first Independent Great Old One card.
var firstIndependent = func() int {
	for i, c := range cards {
		if c.Kind == cardIndependent {
			return i
		}
	}
	return len(cards)
}()

// --- what the table plays with ------------------------------------------------------------

// cardInGame says the pack of card i is in the box of this game.
func (e *eng) cardInGame(i int) bool {
	c := cards[i]
	switch {
	case c.Pack == packAzathoth:
		return e.m.cfg.az
	case c.Kind == cardIndependent:
		return e.m.cfg.indep&(1<<uint(i-firstIndependent)) != 0
	}
	return e.m.cfg.packs&c.Pack != 0
}

// independentsInGame says the Independent Great Old Ones are in the game.
func (e *eng) independentsInGame() bool { return e.m.cfg.packs&packGOO1 != 0 }

// --- the cards seats hold ---------------------------------------------------------------

// ownedCards lists the cards seat holds, in card order.
func (e *eng) ownedCards(seat int) []cardSpec {
	var out []cardSpec
	for i, c := range cards {
		if e.d.loyal(seat, i) {
			out = append(out, c)
		}
	}
	return out
}

// holdsCard says seat has the card of the figure typ.
func (e *eng) holdsCard(seat int, typ string) bool {
	i := cardIndex(typ)
	return i >= 0 && e.d.loyal(seat, i)
}

// independentsOf lists the Independent Great Old Ones seat controls.
func (e *eng) independentsOf(seat int) []string {
	var out []string
	for i := firstIndependent; i < len(cards); i++ {
		if e.d.loyal(seat, i) {
			out = append(out, cards[i].Unit.ID)
		}
	}
	return out
}

// gooTotal counts seat's Great Old Ones on the map, its Independents too (they
// give no Elder Sign in a Ritual, so gooInPlay leaves them out).
func (e *eng) gooTotal(seat int) int {
	n := e.gooInPlay(seat)
	for _, typ := range e.independentsOf(seat) {
		n += e.onMap(seat, typ)
	}
	return n
}

// seatGOOAt lists the types of Great Old One seat has in area a, Independents
// included.
func (e *eng) seatGOOAt(a, seat int) []string {
	var out []string
	for _, typ := range e.typesIn(a, seat) {
		if unitClassOf(typ) == classGOO {
			out = append(out, typ)
		}
	}
	return out
}

// printed number of Gates a card says: kept here so notation and views agree.
func cardCostText(c cardSpec) string {
	switch c.Kind {
	case cardTerror:
		return "2 Doom and 2 Power"
	case cardIndependent:
		return fmt.Sprintf("%d Power", c.Unit.Cost)
	}
	return "2 Doom"
}

// --- buying a card ---------------------------------------------------------------------------

// buyCost is what the card costs: Doom, and Power for a Terror.
func buyCost(c cardSpec) (doom, power int) {
	if c.Kind == cardTerror {
		return 2, 2
	}
	return 2, 0
}

// buyMoves lists the Loyalty Cards seat may buy in its Doom turn: it can pay,
// has not bought one this phase, and the card is in the game and still in the
// general pool.
func (e *eng) buyMoves(seat int) []state.Move {
	d := e.d
	if d.Doomx == nil || d.Doomx.Bought {
		return nil
	}
	var out []state.Move
	for i, c := range cards {
		if c.Kind == cardIndependent || !e.cardInGame(i) || e.cardOwner(i) != 0 {
			continue
		}
		doom, power := buyCost(c)
		if d.Doom[seat] < doom || d.Power[seat] < power {
			continue
		}
		u := c.Unit
		switch u.ID {
		case uShambler, uMoonbeast, uWamp:
			// The figures go to the faction card, an enemy's spellbook or an
			// enemy's choice of places.
			out = append(out, state.Move{Action: actBuy, Piece: u.ID})
		case uServitor:
			for _, o := range e.others(seat) {
				out = append(out, state.Move{Action: actBuy, Piece: u.ID, Count: o})
			}
		case uHound:
			// At any Gate, even one the buyer does not control.
			for a := range e.m.ids {
				if e.hasGate(a) {
					out = append(out, state.Move{Action: actBuy, Piece: u.ID, To: e.m.ids[a]})
				}
			}
		default:
			gates := e.gatesOf(seat)
			for _, a := range gates {
				out = append(out, state.Move{Action: actBuy, Piece: u.ID, To: e.m.ids[a]})
			}
			if len(gates) == 0 {
				out = append(out, state.Move{Action: actBuy, Piece: u.ID})
			}
		}
	}
	return out
}

// buy settles the purchase of a Loyalty Card.
func (e *eng) buy(seat int, mv state.Move) {
	d := e.d
	i := cardIndex(mv.Piece)
	c := cards[i]
	u := c.Unit
	doom, power := buyCost(c)
	d.Doom[seat] -= doom
	d.Power[seat] -= power
	d.Doomx.Bought = true
	owner := seat
	if mv.Piece == uServitor {
		owner = mv.Count
	}
	d.Loyal[owner] |= 1 << uint(i)
	e.setPool(owner, u.ID, u.Count)
	paid := plural(doom, "Doom", "Doom")
	if power > 0 {
		paid += " and " + plural(power, "Power", "Power")
	}
	switch u.ID {
	case uServitor:
		e.say("%s pays %s for the Servitor of the Outer Gods and hands the card to %s: Doom now %d.", e.seatName(seat), paid, e.seatName(owner), d.Doom[seat])
	case uShambler:
		e.setPool(owner, u.ID, u.Count-1)
		d.Card[owner]++
		e.say("%s pays %s for the Dimensional Shambler card (Doom now %d) and puts a Shambler on their faction card.", e.seatName(seat), paid, d.Doom[seat])
	case uMoonbeast:
		e.say("%s pays %s for the Moonbeast card (Doom now %d).", e.seatName(seat), paid, d.Doom[seat])
		e.moonBegin(seat)
	case uWamp:
		e.say("%s pays %s for the Wamp card (Doom now %d).", e.seatName(seat), paid, d.Doom[seat])
		e.cryptBegin(seat)
	default:
		e.say("%s pays %s for the %s card (Doom now %d).", e.seatName(seat), paid, u.Name, d.Doom[seat])
		if mv.To != "" {
			e.place(e.areaIdx(string(mv.To)), seat, u.ID)
			e.say("%s appears at the Gate in %s.", article(u.Name), e.areaName(e.areaIdx(string(mv.To))))
		}
	}
}

// --- losing a card ------------------------------------------------------------------------

// igooDies is told that seat's Independent typ has been eliminated: its card,
// spellbook and unused tokens go back to the general pool (the tokens on the
// map stay where they are).
func (e *eng) igooDies(seat int, typ string) {
	i := cardIndex(typ)
	if i < firstIndependent || i < 0 || !e.d.loyal(seat, i) {
		return
	}
	d := e.d
	d.Loyal[seat] &^= 1 << uint(i)
	d.IBooks &^= 1 << uint(i-firstIndependent)
	e.setPool(seat, typ, 0)
	if typ == uAbhoth {
		e.setPool(seat, uFilth, 0)
	}
	e.say("%s is lost to %s: its card, its spellbook and its unused tokens go back to the general pool.", unitNameOf(typ), e.seatName(seat))
	if typ == uChaugnar {
		d.Picks = nil
	}
}

// --- unit specs of the cards, for the lookups ----------------------------------------------

// cardUnitSpec finds the spec of the figure typ among the Loyalty Cards and
// Abhoth's Filth.
func cardUnitSpec(typ string) (unitSpec, bool) {
	if typ == uFilth {
		return filthSpec, true
	}
	if i := cardIndex(typ); i >= 0 {
		return cards[i].Unit, true
	}
	return unitSpec{}, false
}
