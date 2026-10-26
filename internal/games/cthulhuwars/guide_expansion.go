package cthulhuwars

import "github.com/noderunner/bingle/internal/engine/state"

// What the computer players know about the expansion cults and the High
// Priests: where Windwalker begins, when to hibernate, how to answer Demand
// Sacrifice, where a Serpent Man returns.

// setup chooses where Windwalker begins: the sigil area with the fewest
// enemy units next to it.
func (g *guide) setup(legal []state.Move) []state.Move {
	e := g.e
	var list []scoredMove
	for _, mv := range legal {
		if mv.Action != actStart {
			return legal
		}
		a := e.areaIdx(string(mv.To))
		s := 0.0
		for _, en := range e.others(g.seat) {
			s -= 3 * float64(e.total(a, en))
			for _, nb := range e.m.adj[a] {
				s -= float64(e.total(nb, en))
			}
		}
		list = append(list, scoredMove{mv, s})
	}
	return top(list, 0.01)
}

// claimNow says Windwalker should take its "at any time" spellbook now: two
// enemies (or one, late in the game) have all six.
func (g *guide) claimNow() bool {
	n := g.e.enemiesWithSixBooks(g.seat)
	return n >= 2 || (n >= 1 && g.e.d.Round >= 6)
}

// scoreExpansion values the turn actions of the expansion cults.
func (g *guide) scoreExpansion(mv state.Move, area int) (float64, bool) {
	e, d, seat := g.e, g.e.d, g.seat
	p := d.Power[seat]
	switch mv.Action {
	case actHibernate:
		// Banking the Power pays when enemy Great Old Ones are out and the
		// round's Power would otherwise be spent on little.
		gain := e.enemyGreatOldOnes(seat)
		if gain > p {
			gain = p
		}
		if gain >= 2 && p <= 4 {
			return 56, true
		}
		return 6, true
	case actIceAge:
		// A tax where the enemies' Gates stand.
		s := 8.0
		if p >= 4 {
			if _, ok := d.Gates[e.areaID(area)]; ok && d.Gates[e.areaID(area)] != seat {
				s = 36
			}
		}
		return s, true
	case actLethargy:
		// Waiting costs nothing: better than spending Power for nothing.
		return 18, true
	case actBoon:
		return 22, true
	case actBestow:
		// The most helpless of the others gets the Power.
		lowest := true
		for _, en := range e.others(seat) {
			if d.Doom[en] < d.Doom[mv.Count] {
				lowest = false
			}
		}
		if lowest {
			return 30, true
		}
		return 16, true
	case actBane:
		if p >= 6 {
			return 44, true
		}
		return 20, true
	case actSorcery:
		// The most useful abilities to borrow: Flight, then Fertility Cult.
		switch e.fac(mv.Count) {
		case facCC:
			return 34, true
		case facBG, facYS:
			return 26, true
		}
		return 14, true
	case actSlumber:
		// A threatened Gate sleeps safe.
		if g.threatAt(area) == 2 {
			return 64, true
		}
		return 4, true
	case actWake:
		if g.threatAt(area) == 0 && !g.enemyIn(area) {
			return 38, true
		}
		return 10, true
	}
	return 0, false
}

// guideExpansionPrompt answers the prompts of the expansion cults, reporting
// whether it handled the prompt kind.
func (g *guide) guideExpansionPrompt(p Prompt, legal []state.Move) ([]state.Move, bool) {
	e, seat := g.e, g.seat
	var list []scoredMove
	switch p.Kind {
	case "howl":
		// The cheapest unit leaves, to the area with the most friends.
		for _, mv := range legal {
			a := e.areaIdx(string(mv.To))
			from := e.areaIdx(p.Area)
			v := g.unitValue(seat, mv.Piece, from)
			s := -v + 0.5*float64(e.total(a, seat)) - 2*float64(g.enemyRank(a))
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "eternal":
		// Rhan-Tegoth is worth a Power.
		for _, mv := range legal {
			s := 0.0
			if mv.Action == actEternal {
				s = 90
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "demand":
		// Give the Elder Sign unless the Sleeper's units are what we mean to
		// kill: then the Kills matter more.
		b := e.d.Bat
		kills := 0
		if b != nil {
			kills = e.combatOf(seat, e.areaIdx(b.Area), nil, p.By)
		}
		for _, mv := range legal {
			s := 0.0
			switch {
			case mv.Piece == "signs" && kills < 5:
				s = 60
			case mv.Piece == "pains" && kills >= 5:
				s = 60
			default:
				s = 40
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "restore":
		for _, mv := range legal {
			a := e.areaIdx(string(mv.To))
			s := 2*float64(e.total(a, seat)) - 3*float64(g.enemyRank(a))
			if e.d.Gates[e.areaID(a)] == seat {
				s += 6
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	case "nexus":
		// Declining is simplest: Done.
		for _, mv := range legal {
			s := 0.0
			if mv.Action == actDone {
				s = scoreDoneTurn
			}
			list = append(list, scoredMove{mv, s})
		}
		return top(list, 0.01), true
	}
	return nil, false
}
