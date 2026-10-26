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
	}
	return 0
}

// scoreCards values the turn actions that come with the Loyalty Cards.
func (g *guide) scoreCards(mv state.Move, area int) (float64, bool) {
	e, d, seat := g.e, g.e.d, g.seat
	switch mv.Action {
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
