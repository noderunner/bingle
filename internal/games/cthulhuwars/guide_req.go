package cthulhuwars

import "github.com/noderunner/bingle/internal/engine/state"

// reqUnit is the value of one step toward a spellbook requirement before the
// tuning shifts it. Nobody wins without all six spellbooks, and a book is only
// ever earned by meeting a requirement, so the guide rewards the moves that do.
const reqUnit = 12

// reqBonus is what a move is worth on top of its own score for bringing seat
// closer to a spellbook requirement it has not met. base is the move's score
// without it, so the bonus only lifts moves that are already sensible.
func (g *guide) reqBonus(mv state.Move, base float64) float64 {
	e, d, seat := g.e, g.e.d, g.seat
	u := reqUnit + g.t.Req
	if u <= 0 {
		return 0
	}
	todo := func(r int) bool { return !d.reqDone(seat, r) }
	area := -1
	if mv.To != "" {
		area = e.areaIdx(string(mv.To))
	}
	if mv.Action == actAwaken {
		if r, ok := awakenReq(e.fac(seat), mv.Piece); ok && todo(r) {
			return u
		}
		return 0
	}
	switch e.fac(seat) {
	case facGC:
		switch mv.Action {
		case actGate:
			if todo(rqGCOceans) && e.m.ocean[area] {
				return 1.5 * u
			}
		case actGo:
			if todo(rqGCOceans) && e.m.ocean[area] && unitClassOf(mv.Piece) == classCultist && base > 40 {
				return 0.5 * u
			}
		case actBattle:
			if (todo(rqGCKill1) || todo(rqGCKill2)) && base >= 40 {
				return 0.75 * u
			}
		}
	case facCC:
		switch mv.Action {
		case actPay:
			if (mv.Count == 4 && todo(rqCCPay4)) || (mv.Count == 6 && todo(rqCCPay6)) ||
				(mv.Count >= 10 && (todo(rqCCPay4) || todo(rqCCPay6))) {
				return 1.5 * u
			}
		case actCapture:
			if todo(rqCCCapture) && (mv.Piece == "" || mv.Piece == uCultist || mv.Piece == onGate+uCultist || mv.Piece == uHighPriest) {
				return u
			}
		case actGate:
			if todo(rqCCGates3) || todo(rqCCGates4) {
				return 0.5 * u
			}
		}
	case facBG:
		switch mv.Action {
		case actSacrifice:
			if todo(rqBGSac) && e.cultistsInPlay(seat) >= 3 {
				return 2.5 * u
			}
		case actGo:
			if base <= 0 || area < 0 {
				return 0
			}
			bonus := 0.0
			if (todo(rqBGAreas4) || todo(rqBGAreas6) || todo(rqBGAreas8)) && e.total(area, seat) == 0 {
				// Past four areas the rest is the slow part: push harder.
				if e.areasWithUnits(seat) >= 4 {
					bonus += 3.5 * u
				} else {
					bonus += 2.5 * u
				}
			}
			if todo(rqBGShare) && g.sharesNewEnemy(area) && (g.enemyRank(area) == 0 || unitClassOf(mv.Piece) != classCultist) {
				bonus += 1.5 * u
			}
			return bonus
		}
	case facYS:
		switch mv.Action {
		case actScream:
			// The King walks for free: toward a glyph still to be desecrated.
			if k, ok := e.gooArea(seat, uKing); ok && area >= 0 {
				if g.glyphWanted(area) {
					return 3 * u
				}
				if d0, d1 := g.glyphDist(k), g.glyphDist(area); d1 < d0 {
					return 2 * u
				}
			}
		case actShriek:
			// Byakhee fly to the King so the Desecration die has units to beat.
			if k, ok := e.gooArea(seat, uKing); ok && area == k && g.glyphWanted(k) && e.total(k, seat) < 4 {
				return 3 * u
			}
		case actDesecrate:
			if a, ok := e.gooArea(seat, uKing); ok {
				if r, ok := glyphReq(e.m.glyph[a]); ok && todo(r) {
					// The die must show no more than the units there.
					if e.total(a, seat) >= 3 {
						return 4 * u
					}
					return u
				}
			}
		case actGrant:
			if todo(rqYSGift) && base >= 40 {
				return 1.5 * u
			}
		case actGo:
			// Others walk to the King so the Desecration die has units to beat.
			if k, ok := e.gooArea(seat, uKing); ok && mv.Piece != uKing && area == k && base > 0 && !d.Desec[e.areaID(k)] && e.total(k, seat) < 4 {
				if r, ok := glyphReq(e.m.glyph[k]); ok && todo(r) {
					return 2 * u
				}
			}
			if mv.Piece == uKing && area >= 0 && base > 0 {
				if g.glyphWanted(area) {
					return 2 * u
				}
				if from := e.areaIdx(string(mv.From)); !g.glyphWanted(from) && g.glyphDist(area) < g.glyphDist(from) {
					return 1.5 * u
				}
			}
		}
	}
	return 0
}

// awakenReq is the requirement that awakening the piece meets, if any.
func awakenReq(fac int, piece string) (int, bool) {
	switch {
	case fac == facGC && piece == uCthulhu:
		return rqGCAwaken, true
	case fac == facCC && piece == uNyar:
		return rqCCAwaken, true
	case fac == facBG && piece == uShub:
		return rqBGAwaken, true
	case fac == facYS && piece == uKing:
		return rqYSKing, true
	case fac == facYS && piece == uHastur:
		return rqYSHastur, true
	case fac == facWW && piece == uRhan:
		return rqWWRhan, true
	case fac == facWW && piece == uIthaqua:
		return rqWWIthaqua, true
	case fac == facSL && piece == uTsath:
		return rqSLAwaken, true
	}
	return 0, false
}

// glyphReq is the Yellow Sign requirement a successful Desecration on the
// glyph meets.
func glyphReq(glyph string) (int, bool) {
	switch glyph {
	case "dragon":
		return rqYSDragon, true
	case "chevron":
		return rqYSChevron, true
	case "thorns":
		return rqYSThorns, true
	}
	return 0, false
}

// sharesNewEnemy reports whether an enemy that seat shares no area with yet has
// units in area.
func (g *guide) sharesNewEnemy(area int) bool {
	e, seat := g.e, g.seat
	for _, en := range e.others(seat) {
		if e.total(area, en) == 0 {
			continue
		}
		shared := false
		for a := range e.m.ids {
			if e.total(a, seat) > 0 && e.total(a, en) > 0 {
				shared = true
				break
			}
		}
		if !shared {
			return true
		}
	}
	return false
}

// glyphWanted reports whether a successful Desecration in area would meet a
// requirement still open: its glyph is one of the three and it holds no token.
func (g *guide) glyphWanted(area int) bool {
	e := g.e
	if area < 0 || e.d.Desec[e.areaID(area)] {
		return false
	}
	r, ok := glyphReq(e.m.glyph[area])
	return ok && !e.d.reqDone(g.seat, r)
}

// glyphDist is the number of steps from area to the nearest area glyphWanted
// (0 for such an area itself; a large number when there is none).
func (g *guide) glyphDist(area int) int {
	adj := g.e.m.adj
	seen := map[int]bool{area: true}
	frontier := []int{area}
	for d := 0; len(frontier) > 0; d++ {
		var next []int
		for _, a := range frontier {
			if g.glyphWanted(a) {
				return d
			}
			for _, nb := range adj[a] {
				if !seen[nb] {
					seen[nb] = true
					next = append(next, nb)
				}
			}
		}
		frontier = next
	}
	return 99
}
