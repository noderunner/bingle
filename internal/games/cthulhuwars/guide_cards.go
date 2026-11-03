package cthulhuwars

import "github.com/noderunner/bingle/internal/engine/state"

// What the computer players know about the Loyalty Cards: which to buy, where
// to awaken the Independents and what to do with them. The notes follow the
// strategy sections of the Cthulhu Wars wiki (see guide/independents.md):
// Filth makes cheap guards, Mother Hydra's Zygote turns Power into Cultists
// and Cultists into Power (with the Mao Ceremony), Chaugnar Faugn defends
// Gates, Cthugha takes over from a Great Old One that is cheap to bring back,
// and Fire Vampires spare Great Old Ones (so the Harbinger and Firestorm pay).

// cardBuyScore values buying the Loyalty Card of mv in the Doom phase; 0 or
// less means not to.
func (g *guide) cardBuyScore(mv state.Move) float64 {
	e, d, seat := g.e, g.e.d, g.seat
	goal := e.game.ParamInt("doom_to_win", 30)
	// Doom is the victory points: pay only with a margin, and early enough for
	// the card to matter.
	if d.Doom[seat] < 4 || d.Doom[seat]+8 >= goal || d.Round < 2 {
		return 0
	}
	gates := float64(len(e.gatesOf(seat)))
	switch mv.Piece {
	case uGnorri:
		// Two or three Gnorri earn 1 or 2 Doom every Doom phase: worth it with
		// the Power to summon them and a game still to run.
		if d.Power[seat] >= 6 && goal-d.Doom[seat] >= 12 {
			return 62
		}
	case uShantak:
		return 46 + 2*gates
	case uVoonith:
		return 48 + 2*gates
	case uMoonbeast:
		if len(e.moonOptions(seat)) > 0 {
			return 50
		}
	case uHound:
		// The Hound hops between Gates: it wants several, and Power to summon.
		if gates >= 2 && d.Power[seat] >= 6 {
			return 52
		}
	case uShambler:
		return 55
	case uElderThing, uStarVamp:
		return 49
	case uLeng, uSatyr, uMercury, uVenus, uAsteroid:
		return 47
	case uGug:
		return 50 + gates
	case uGhast:
		if gates >= 1 {
			return 45
		}
	case uYith, uDhole, uQuachil, uNeptune:
		if d.Power[seat] >= 4 {
			return 51
		}
	}
	return 0
}

// scoreCards values the turn actions that come with the Loyalty Cards.
func (g *guide) scoreCards(mv state.Move, area int) (float64, bool) {
	e, d, seat := g.e, g.e.d, g.seat
	switch mv.Action {
	case actCard:
		return g.scoreCardAct(mv), true
	case actAwaken:
		if !isIndependent(mv.Piece) {
			return 0, false
		}
		return g.scoreIndependent(mv, area), true
	case actFilth:
		// Cheap guards: where Cultists or a Gate stand with nobody to protect
		// them. Filth elsewhere only when Abhoth's own spellbook wants more
		// kinds of Monster and nothing better is to do.
		s := 22.0
		switch {
		case e.protectionRank(area, seat) == 0 && e.cultists(area, seat) > 0 && g.threatAt(area) > 0:
			s = 74
		case e.protectionRank(area, seat) == 0 && e.d.Gates[e.areaID(area)] == seat:
			s = 56
		case e.protectionRank(area, seat) == 0 && e.cultists(area, seat) > 0:
			s = 40
		}
		if e.filthInPlay() >= 8 {
			s -= 12
		}
		return s, true
	case actSting:
		// Drive the Cultists out of an ocean: best where an enemy Gate is held by one.
		s := 4.0
		for _, en := range e.others(seat) {
			n := e.cultists(area, en)
			s += 8 * float64(n)
			if d.Gates[e.areaID(area)] == en {
				s += 24
			}
		}
		return s, true
	case actZygote:
		// Every Acolyte of the pool at once for 1 Power.
		if n := e.pool(seat, uCultist); n >= 2 {
			return 68 + 2*float64(n), true
		}
		return 24, true
	case actDiscard:
		// Two Elder Signs are Doom: only the cheapest pair, and only when the
		// Curse is worth it.
		if mv.Count/10 == 1 && mv.Count%10 <= 2 {
			return 44, true
		}
		return 6, true
	case actGateOff:
		// A Gate is income and Doom: only when there are plenty.
		if len(e.gatesOf(seat)) >= 5 {
			return 42, true
		}
		return 4, true
	case actRansom:
		if d.Doom[seat] >= 5 {
			return 62, true
		}
		return -1, true
	case actCrypt:
		return 30, true
	}
	return 0, false
}

// scoreIndependent values awakening an Independent.
func (g *guide) scoreIndependent(mv state.Move, area int) float64 {
	e, seat := g.e, g.seat
	switch mv.Piece {
	case uCthugha:
		// He replaces a Great Old One of ours: worth it only when that one is
		// cheap to bring back (Cthulhu, and Ubbo-Sathla who comes back free)
		// — the wiki's two combos — and the Power buys a Great Old One's worth.
		switch replacedBy(mv) {
		case uCthulhu:
			if e.d.Awoke(seat, 0) {
				return 80
			}
		case uUbbo:
			return 78
		}
		return 18
	case uAtlach:
		return 66
	case uGobogeg:
		return 90 // free, and pays 6 Power
	case uTulzscha, uByatis, uGlaaki, uDagon, uGhatan, uHagarg:
		return 60
	case uBokrug, uYgolonac, uDaoloth, uNyogtha:
		return 55
	case uChaugnar:
		return 78 + float64(e.cultists(area, seat))
	case uHydra:
		return 74
	case uAbhoth:
		return 70
	case uYig:
		return 72
	}
	return 50
}

// guideCardsPrompt answers the prompts of the Loyalty Cards, reporting
// whether it handled the kind.
func (g *guide) guideCardsPrompt(p Prompt, legal []state.Move) ([]state.Move, bool) {
	if p.Kind == "card" {
		return g.answerCardPrompt(p, legal), true
	}
	e, d, seat := g.e, g.e.d, g.seat
	var list []scoredMove
	switch p.Kind {
	case "moon":
		// The Moonbeast lies on the best spellbook of the leader in Doom.
		for _, mv := range legal {
			if mv.Action != actMoon {
				return legal, true
			}
			s := float64(d.Doom[mv.Count]) / 2
			b, _ := bookByID(mv.Piece)
			if b < bookSlots {
				order := bookOrder[e.fac(mv.Count)]
				for i, x := range order {
					if x == b {
						s += float64(len(order)-i) * 1.5
					}
				}
			} else {
				s += 2
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "moonland":
		for _, mv := range legal {
			a := e.areaIdx(string(mv.To))
			s := 3*float64(e.total(a, seat)) - 2*float64(g.enemyRank(a))
			if g.need[a] > 0 {
				s += 10
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "crypt-pick":
		// The enemy with the fewest units around has the fewest places to put
		// them to good use.
		for _, mv := range legal {
			n := 0
			for a := range e.m.ids {
				n += e.total(a, mv.Count)
			}
			list = append(list, scoredMove{mv, -float64(n)})
		}
		return top(list, 0.01), true
	case "crypt":
		// Somebody else's Wamps go where they do the least harm: away from our
		// unguarded Cultists, and as far from the owner's own as the map allows.
		for _, mv := range legal {
			a := e.areaIdx(string(mv.To))
			s := -6 * float64(e.cultists(a, seat))
			for _, nb := range e.m.adj[a] {
				s -= 2 * float64(e.cultists(nb, seat))
				s += float64(e.total(nb, p.By)) * -0.5
			}
			s -= 3 * float64(e.total(a, p.By))
			s += 2 * float64(e.protectionRank(a, seat))
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "sting":
		for _, mv := range legal {
			a := e.areaIdx(string(mv.To))
			s := 3*float64(e.total(a, seat)) - 4*float64(g.enemyRank(a))
			if d.Gates[e.areaID(a)] == seat {
				s += 6
			}
			if _, ok := d.Gates[e.areaID(a)]; ok && d.Gates[e.areaID(a)] == 0 {
				s += 8
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "spare":
		// Great Old Ones are spared (their Pain pays the Harbinger and they live
		// to be Killed another day); with Firestorm every unit is an Elder Sign.
		for _, mv := range legal {
			s := 0.0
			switch {
			case mv.Action == actDone:
				s = 10
			case unitClassOf(mv.Piece) == classGOO, e.igooBook(seat, uCthugha):
				s = 60
			default:
				s = 0
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "pick":
		for _, mv := range legal {
			s := float64(mv.Count)
			if p.By != seat {
				s = -s
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "donate":
		// One Power is cheaper than a Doom point for the one who asks.
		for _, mv := range legal {
			s := 0.0
			if mv.Piece == "give" && d.Power[seat] >= 3 {
				s = 10
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "zygote":
		for _, mv := range legal {
			a := e.areaIdx(string(mv.To))
			s := float64(e.cultists(a, seat)) - 4*float64(g.enemyRank(a))
			if d.Gates[e.areaID(a)] == seat {
				s += 6
			}
			if ctl, ok := d.Gates[e.areaID(a)]; ok && ctl == 0 {
				s += 12
			}
			if e.protectionRank(a, seat) > 0 {
				s += 4
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	}
	return nil, false
}

// scoreCardAct values the "card" Actions of the later Independents.
func (g *guide) scoreCardAct(mv state.Move) float64 {
	e, d, seat := g.e, g.e.d, g.seat
	act, _ := cardActOf(mv.Piece)
	switch act.ID {
	case "cosmic-web":
		return 1000 // the game
	case "spin":
		return 58 + 6*float64(len(d.Webs))
	case "ghatan-pay":
		return 24
	case "tulzscha-gift":
		// Two Power per enemy is a real price: late, when the book is worth it.
		if d.Round >= 3 {
			return 26
		}
		return 2
	case "mummify":
		return 48
	case "forget":
		return 44
	case "nightmare":
		return 40
	case "laziness":
		return 60
	case "lust":
		return 18
	case "tsunami":
		// Worth it where it empties an enemy Gate.
		a := e.areaIdx(string(mv.To))
		if ctl, ok := d.Gates[e.areaID(a)]; ok && ctl != 0 && ctl != seat {
			return 52
		}
		return 6
	case "give-bokrug":
		// To the leader: the book hurts its holder.
		best := true
		for _, o := range e.others(seat) {
			if d.Doom[o] > d.Doom[mv.Count] {
				best = false
			}
		}
		if best && d.Round >= 3 {
			return 30
		}
		return 1
	}
	return 10
}

// answerCardPrompt chooses an answer to a "card" prompt.
func (g *guide) answerCardPrompt(p Prompt, legal []state.Move) []state.Move {
	e, d, seat := g.e, g.e.d, g.seat
	var list []scoredMove
	for _, mv := range legal {
		act, unit := cardActOf(mv.Piece)
		s := 0.0
		switch act.ID {
		case "blood":
			// A Kill on the enemy, never on ourselves.
			if mv.Count != seat {
				s = 10
			} else {
				s = -10
			}
		case "glacier":
			// Over an enemy's Gate, never our own.
			a := e.areaIdx(string(mv.To))
			switch ctl := d.Gates[e.areaID(a)]; {
			case ctl == seat:
				s = -10
			case ctl != 0:
				s = 10 + float64(d.Doom[ctl])
			}
		case "planet-power":
			s = 5
		case "planet-doom":
			s = 1
		case "final-ritual":
			s = 5
		case "ghost":
			s = float64(e.total(e.areaIdx(string(mv.To)), seat))
		case "horde", "innsmouth":
			a := e.areaIdx(string(mv.To))
			s = -float64(e.count(a, seat, uCultist))
		case "sarnath-unit", "sarnath-sign":
			// The enemy with the least Doom takes it.
			s = -float64(d.Doom[mv.Count])
		case "elim", "orifice", "dust":
			s = float64(e.summonCost(p.By, unit))
			if act.ID == "dust" {
				s = -s
			}
		case "dust-sign":
			s = -1.5
		case "subvert":
			s = float64(e.gooInPlay(mv.Count))
		}
		list = append(list, scoredMove{mv, s})
	}
	return top(list, 0.01)
}
