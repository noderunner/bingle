package cthulhuwars

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/state"
)

// fulfil marks a spellbook requirement of seat as met (once), pays what the
// requirement gives with it and queues the choice of the spellbook it earns.
// It reports whether the requirement was newly met.
func (e *eng) fulfil(seat, req int) bool {
	d := e.d
	if seat == 0 || d.reqDone(seat, req) {
		return false
	}
	d.Reqs[seat] |= 1 << uint(req)
	e.say("%s fulfils a spellbook requirement: %s", e.seatName(seat), e.f(seat).Reqs[req].Text)
	e.reqReward(seat, req)
	if d.bookCount(seat) >= 6 {
		return true
	}
	d.Prompts = append(d.Prompts, Prompt{Kind: "book", Seat: seat})
	return true
}

// reqReward pays what a spellbook requirement gives besides the book: an
// Elder Sign for Great Cthulhu's first-Doom and last-book requirements and
// for Yellow Sign's Hastur, a Power per enemy for Black Goat's shared areas,
// Elder Signs for Windwalker's "at any time" book.
func (e *eng) reqReward(seat, req int) {
	switch e.fac(seat) {
	case facGC:
		if req == rqGCFirstDoom || req == rqGCFive {
			e.gainSign(seat)
		}
	case facBG:
		if req == rqBGShare {
			n := len(e.others(seat))
			e.d.Power[seat] += n
			e.say("%s gains %s for sharing areas with every enemy.", e.seatName(seat), plural(n, "Power", "Power"))
		}
	case facYS:
		if req == rqYSHastur {
			e.gainSign(seat)
		}
	case facWW:
		if req == rqWWTake {
			n := e.enemiesWithSixBooks(seat)
			if n > 3 {
				n = 3
			}
			for i := 0; i < n; i++ {
				e.gainSign(seat)
			}
		}
	}
}

func (e *eng) awardBook(seat, book int) {
	d := e.d
	d.setBook(seat, book, true)
	if book >= bookSlots {
		e.say("%s takes the neutral spellbook %s.", e.seatName(seat), e.bookSpecOf(seat, book).Name)
		return
	}
	e.say("%s earns the spellbook %s.", e.seatName(seat), e.f(seat).Books[book].Name)
	// Hierophants: every faction spellbook the Tcho-Tcho earns, this one
	// included, brings a High Priest.
	if e.is(seat, facTT) && d.hasBook(seat, bkHiero) {
		e.hierophants(seat, book == bkHiero)
	}
}

// checkRequirements evaluates the requirements that depend on the state of
// the map and fulfils those that hold.
func (e *eng) checkRequirements() {
	d := e.d
	// Great Cthulhu: Gates in oceans.
	if s := e.seatOf(facGC); s != 0 && !d.reqDone(s, rqGCOceans) {
		ctl, all := 0, 0
		for id, c := range d.Gates {
			if e.m.ocean[e.areaIdx(id)] {
				all++
				if c == s {
					ctl++
				}
			}
		}
		if ctl >= 3 || all >= 4 {
			e.fulfil(s, rqGCOceans)
		}
	}
	// Crawling Chaos: Gates or Power.
	if s := e.seatOf(facCC); s != 0 {
		g := len(e.gatesOf(s))
		if !d.reqDone(s, rqCCGates3) && (g >= 3 || d.Power[s] >= 12) {
			e.fulfil(s, rqCCGates3)
		}
		if !d.reqDone(s, rqCCGates4) && (g >= 4 || d.Power[s] >= 15) {
			e.fulfil(s, rqCCGates4)
		}
	}
	// Black Goat: units in several areas, sharing areas with every enemy.
	if s := e.seatOf(facBG); s != 0 {
		n := e.areasWithUnits(s)
		for i, need := range []int{4, 6, 8} {
			if !d.reqDone(s, rqBGAreas4+i) && n >= need {
				e.fulfil(s, rqBGAreas4+i)
			}
		}
		if !d.reqDone(s, rqBGShare) {
			all := true
			for _, enemy := range e.others(s) {
				shared := false
				for a := range e.m.ids {
					if e.total(a, s) > 0 && e.total(a, enemy) > 0 {
						shared = true
						break
					}
				}
				if !shared {
					all = false
					break
				}
			}
			if all {
				e.fulfil(s, rqBGShare)
			}
		}
	}
	// Opener of the Way: Gates on the map, units at enemy Gates, meeting a
	// Great Old One.
	if s := e.seatOf(facOW); s != 0 {
		g := e.gatesOnMap()
		need := 12
		if e.n <= 3 {
			need = 10
		}
		if !d.reqDone(s, rqOWGates8) && g >= 8 {
			e.fulfil(s, rqOWGates8)
		}
		if !d.reqDone(s, rqOWGates12) && g >= need {
			e.fulfil(s, rqOWGates12)
		}
		if !d.reqDone(s, rqOWEnemyGates) {
			n := 0
			for id, ctl := range d.Gates {
				if ctl != 0 && ctl != s && e.total(e.areaIdx(id), s) > 0 {
					n++
				}
			}
			if n >= 2 {
				e.fulfil(s, rqOWEnemyGates)
			}
		}
		if !d.reqDone(s, rqOWMeet) {
			for a := range e.m.ids {
				if e.count(a, s, uYog) == 0 {
					continue
				}
				for _, o := range e.others(s) {
					if e.classCount(a, o, classGOO) > 0 {
						e.fulfil(s, rqOWMeet)
					}
				}
			}
		}
	}
	// Tcho-Tcho: another player with 15 Doom, three Elder Signs.
	if s := e.seatOf(facTT); s != 0 {
		if !d.reqDone(s, rqTTRitual) {
			for _, o := range e.others(s) {
				if d.Doom[o] >= 15 {
					e.fulfil(s, rqTTRitual)
					break
				}
			}
		}
		if !d.reqDone(s, rqTTThree) && len(d.Signs[s])+len(d.Shown[s]) >= 3 {
			e.fulfil(s, rqTTThree)
		}
	}
	// Independent Great Old Ones: the spellbooks whose requirement is a state of
	// the map.
	for _, s := range e.playing() {
		if e.holdsCard(s, uAbhoth) && !e.igooBook(s, uAbhoth) {
			types, total := 0, 0
			for _, u := range e.units(s) {
				if u.Class != classMonster {
					continue
				}
				if n := e.onMap(s, u.ID); n > 0 {
					types++
					total += n
				}
			}
			if types >= 4 || total >= 8 {
				e.earnIGOOBook(s, uAbhoth)
			}
		}
		if e.holdsCard(s, uHydra) && !e.igooBook(s, uHydra) {
			own, enemy := e.gooInOceans(s), false
			for _, o := range e.others(s) {
				if e.gooInOceans(o) {
					enemy = true
				}
			}
			if !own || !enemy {
				e.earnIGOOBook(s, uHydra)
			}
		}
	}
	// Windwalker: a Gate in the other sigil area; another cult with six books.
	if s := e.seatOf(facWW); s != 0 {
		if !d.reqDone(s, rqWWGate) && d.WWStart != "" {
			for _, a := range e.sigilAreas(facWW) {
				if e.areaID(a) != d.WWStart && e.hasGate(a) {
					e.fulfil(s, rqWWGate)
					break
				}
			}
		}
		if !d.reqDone(s, rqWWSix) && e.enemiesWithSixBooks(s) > 0 {
			e.fulfil(s, rqWWSix)
		}
	}
}

// --- prompts ----------------------------------------------------------------

func (e *eng) thenAfterPrompts(then string) { e.d.Then = then }

// beginPrompts enters the prompt stage if prompts are queued, else carries on.
func (e *eng) beginPrompts(_ string) {
	d := e.d
	e.routed = true
	e.normalizePrompts()
	if len(d.Prompts) == 0 {
		e.runThen()
		return
	}
	if d.Stage != stagePrompt {
		d.Resume = d.Stage
	}
	d.Stage = stagePrompt
}

// dropPrompt removes the front prompt. A seat that lost Cultists to Ghroth
// gains one Power from Passion for the whole of it, not one per Cultist.
func (e *eng) dropPrompt() {
	d := e.d
	p := d.Prompts[0]
	d.Prompts = d.Prompts[1:]
	if p.Kind == "lose" && p.Why == "Ghroth" && p.Lost > 0 {
		e.passion(p.Seat)
	}
}

// lossOptions lists the units a "lose" prompt lets its seat eliminate:
// (area, type) pairs.
func (e *eng) lossOptions(p Prompt) []PUArea {
	var out []PUArea
	allowed := func(typ string) bool {
		if p.CultistsOnly && !isCultist(typ) {
			return false
		}
		return len(p.Only) == 0 || containsStr(p.Only, typ)
	}
	if p.Area != "" {
		a := e.areaIdx(p.Area)
		for _, typ := range e.typesIn(a, p.Seat) {
			if !allowed(typ) {
				continue
			}
			if unitClassOf(typ) == classGOO && len(p.Only) == 0 {
				continue
			}
			// Devour and Abduct name Monsters and Cultists: a Terror is neither.
			if (p.Why == "devour" || p.Why == "abduct") && !isMonsterOrCultist(typ) {
				continue
			}
			out = append(out, PUArea{Area: a, Type: typ})
		}
		return out
	}
	// Anywhere on the map.
	for a := range e.m.ids {
		for _, typ := range e.typesIn(a, p.Seat) {
			if allowed(typ) {
				out = append(out, PUArea{Area: a, Type: typ})
			}
		}
	}
	return out
}

// PUArea is a unit type standing in an area.
type PUArea struct {
	Area int
	Type string
}

// normalizePrompts drops prompts that cannot be answered and answers those
// with a single possible answer.
func (e *eng) normalizePrompts() {
	d := e.d
	for len(d.Prompts) > 0 {
		p := d.Prompts[0]
		switch p.Kind {
		case "book":
			un := e.bookChoices(p.Seat, p.Skip-1)
			if len(un) == 0 {
				e.dropPrompt()
				continue
			}
			if len(un) == 1 {
				e.dropPrompt()
				e.awardBook(p.Seat, un[0])
				continue
			}
			return
		case "lose":
			opts := e.lossOptions(p)
			if len(opts) == 0 || p.Left <= 0 {
				e.dropPrompt()
				continue
			}
			// One kind of unit, or no more units than must go: nothing to
			// choose.
			if distinct(opts) == 1 || e.lossTotal(p, opts) <= p.Left {
				e.applyLoss(p, opts[0])
				e.popLoss()
				continue
			}
			return
		case "desecrate":
			// Nothing to place, or one kind only: nothing to choose.
			opts := e.desecratePieces(p.Seat)
			switch len(opts) {
			case 0:
				e.dropPrompt()
				continue
			case 1:
				e.dropPrompt()
				e.placeDesecrated(p.Seat, p.Area, opts[0])
				continue
			}
			return
		case "submerge", "scream":
			// Nothing left to take along: the choice is over.
			if len(e.bringOptions(p)) == 0 {
				e.dropPrompt()
				continue
			}
			return
		case "shriek":
			if len(e.shriekOptions(p)) == 0 {
				e.dropPrompt()
				continue
			}
			return
		case "avatar":
			opts := e.swapOptions(p)
			switch len(opts) {
			case 0:
				e.dropPrompt()
				continue
			case 1:
				e.dropPrompt()
				e.swapWithShub(p, opts[0])
				continue
			}
			return
		case "place":
			if len(e.placeOptions()) == 0 {
				e.dropPrompt()
				continue
			}
			return
		case "react":
			if len(e.reactMoves(p)) == 0 {
				e.dropPrompt()
				continue
			}
			return
		case "howl":
			if len(e.howlMoves(p)) == 0 {
				e.dropPrompt()
				continue
			}
			return
		case "eternal":
			// Nothing to pay with: the result stands.
			if d.Power[p.Seat] < 1 {
				e.resolveEternal(false)
				continue
			}
			return
		case "curse-target":
			opts := e.curseTargets()
			switch len(opts) {
			case 0:
				e.dropPrompt()
				e.curseNext()
				continue
			case 1:
				e.dropPrompt()
				e.curseGive(opts[0])
				continue
			}
			return
		case "curse-hit":
			opts := e.curseVictims()
			switch len(opts) {
			case 0:
				e.dropPrompt()
				e.curseNext()
				continue
			case 1:
				e.dropPrompt()
				e.curseHit(opts[0])
				continue
			}
			return
		case "curse-move":
			opts := e.curseMoveMoves()
			switch len(opts) {
			case 0:
				e.dropPrompt()
				e.d.Curse = nil
				continue
			case 1:
				e.dropPrompt()
				e.curseMove(string(opts[0].To))
				continue
			}
			return
		case "hiero":
			opts := e.hieroMoves(p)
			switch len(opts) {
			case 0:
				e.dropPrompt()
				continue
			case 1:
				e.dropPrompt()
				e.priestAt(p.Seat, e.areaIdx(string(opts[0].To)))
				continue
			}
			return
		case "idol":
			if len(e.idolMoves(p)) == 0 {
				e.dropPrompt()
				continue
			}
			return
		case "channel":
			if len(e.channelMoves(p.Seat)) == 0 {
				e.dropPrompt()
				continue
			}
			return
		case "syco":
			return
		case "mao":
			if len(e.maoMoves(p)) == 0 {
				e.dropPrompt()
				continue
			}
			return
		case "undim":
			if len(e.undimMoves(p)) == 0 {
				e.dropPrompt()
				continue
			}
			return
		case "demand", "restore", "nexus":
			return
		case "moon":
			switch opts := e.moonOptions(p.Seat); len(opts) {
			case 0:
				e.dropPrompt()
				continue
			case 1:
				e.dropPrompt()
				e.moonPlace(p.Seat, opts[0])
				continue
			}
			return
		case "moonland":
			switch opts := e.moonLandMoves(p.Seat); len(opts) {
			case 0:
				e.dropPrompt()
				continue
			case 1:
				e.dropPrompt()
				e.moonLand(p.Seat, e.areaIdx(string(opts[0].To)))
				continue
			}
			return
		case "crypt-pick":
			switch opts := e.cryptMoves(p.Seat); len(opts) {
			case 0:
				e.dropPrompt()
				continue
			case 1:
				e.dropPrompt()
				e.cryptChoose(p.Seat, opts[0].Count)
				continue
			}
			return
		case "crypt":
			opts := e.cryptPlaceMoves(p)
			if len(opts) == 0 || p.Left <= 0 {
				e.dropPrompt()
				continue
			}
			if len(opts) == 1 {
				e.cryptPlace(p, e.areaIdx(string(opts[0].To)))
				continue
			}
			return
		case "sting":
			opts := e.stingMovesFor(p)
			switch len(opts) {
			case 0:
				e.dropPrompt()
				continue
			case 1:
				e.stingMove(p, opts[0])
				continue
			}
			return
		case "spare":
			if len(e.spareMoves(p)) == 0 {
				e.dropPrompt()
				continue
			}
			return
		case "pick":
			vals := e.pickValues()
			if len(vals) > 1 {
				return
			}
			e.dropPrompt()
			v := 0
			if len(vals) == 1 {
				v = vals[0]
			}
			e.settlePick(p.By, v)
			continue
		case "donate":
			if len(e.donateMoves(p)) < 2 {
				e.dropPrompt()
				e.donate(p, false)
				continue
			}
			return
		case "zygote":
			opts := e.zygoteMoves(p.Seat)
			if e.pool(p.Seat, uCultist) <= 0 || p.Left <= 0 || len(opts) == 0 {
				e.dropPrompt()
				continue
			}
			if len(opts) == 1 {
				a := e.areaIdx(string(opts[0].To))
				for e.pool(p.Seat, uCultist) > 0 {
					e.zygotePlace(p.Seat, a)
				}
				e.dropPrompt()
				continue
			}
			return
		default:
			e.dropPrompt()
		}
	}
}

// bringOptions lists the unit types still standing where Cthulhu sank (or
// where the King in Yellow stood) that the player may take along: any
// Monster or Cultist for Cthulhu, the Undead for the King.
func (e *eng) bringOptions(p Prompt) []string {
	if p.Area == "" {
		return nil
	}
	a := e.areaIdx(p.Area)
	var out []string
	for _, typ := range e.typesIn(a, p.Seat) {
		if p.Kind == "scream" {
			if typ != uUndead {
				continue
			}
		} else if unitClassOf(typ) == classGOO {
			continue
		}
		out = append(out, typ)
	}
	return out
}

// shriekOptions lists the areas (other than the destination) where Byakhee
// stand that Shriek may still call.
func (e *eng) shriekOptions(p Prompt) []int {
	if p.Area == "" {
		return nil
	}
	dest := e.areaIdx(p.Area)
	var out []int
	for a := range e.m.ids {
		if a != dest && e.count(a, p.Seat, uByak) > 0 {
			out = append(out, a)
		}
	}
	return out
}

// swapOptions lists the unit types (Monsters and Cultists) of the chosen
// faction standing in the area an Avatar swap is about.
func (e *eng) swapOptions(p Prompt) []string {
	a := e.areaIdx(p.Area)
	var out []string
	for _, typ := range e.typesIn(a, p.Seat) {
		if !isMonsterOrCultist(typ) {
			continue
		}
		out = append(out, typ)
	}
	return out
}

// swapWithShub trades the places of Shub-Niggurath and one unit of p.Seat
// in p.Area: the Avatar action, once the owner has chosen the unit.
func (e *eng) swapWithShub(p Prompt, typ string) {
	bg := e.seatOf(facBG)
	if bg == 0 {
		return
	}
	from, ok := e.gooArea(bg, uShub)
	if !ok {
		return
	}
	to := e.areaIdx(p.Area)
	if !e.take(to, p.Seat, typ) {
		return
	}
	e.take(from, bg, uShub)
	e.add(to, bg, uShub)
	e.add(from, p.Seat, typ)
	e.say("Avatar: Shub-Niggurath appears in %s and %s's %s is taken to %s.", e.areaName(to), e.seatName(p.Seat), unitNameOf(typ), e.areaName(from))
	e.syncGate(from)
	e.syncGate(to)
	if isCultist(typ) && p.Seat == bg {
		e.takeAbandonedGate(from, bg)
	}
}

// placeOptions lists the seats that have a Cultist in their pool: Ghroth may
// place any faction's Acolyte.
func (e *eng) placeOptions() []int {
	var out []int
	for _, s := range e.playing() {
		if e.pool(s, uCultist) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// distinct counts distinct (area, type) pairs, treating the same type in the
// same area as one choice.
func distinct(opts []PUArea) int { return len(opts) }

// lossTotal counts the units a loss prompt can take in all.
func (e *eng) lossTotal(p Prompt, opts []PUArea) int {
	n := 0
	for _, o := range opts {
		n += e.count(o.Area, p.Seat, o.Type)
	}
	return n
}

func (e *eng) applyLoss(p Prompt, o PUArea) {
	e.say("%s eliminates %s in %s (%s).", e.seatName(p.Seat), article(unitNameOf(o.Type)), e.areaName(o.Area), p.Why)
	by := p.By
	if p.Why == "Ghroth" {
		by = 0 // Passion pays once for the whole of Ghroth (see dropPrompt)
	}
	e.eliminate(o.Area, p.Seat, o.Type, by)
	if p.Why == "devour" && e.d.Bat != nil && p.By > 0 {
		e.d.Bat.Slain[p.By]++
	}
	e.d.Prompts[0].Lost++
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// popLoss decrements the front "lose" prompt and removes it when done.
func (e *eng) popLoss() {
	d := e.d
	d.Prompts[0].Left--
	if d.Prompts[0].Left <= 0 {
		e.dropPrompt()
	}
}

func (e *eng) legalPrompt() []state.Move {
	d := e.d
	if len(d.Prompts) == 0 {
		return nil
	}
	p := d.Prompts[0]
	var out []state.Move
	switch p.Kind {
	case "book":
		for _, b := range e.bookChoices(p.Seat, p.Skip-1) {
			out = append(out, state.Move{Action: actBook, Piece: e.bookSpecOf(p.Seat, b).ID, Count: b + 1})
		}
	case "mao":
		out = e.maoMoves(p)
	case "curse-target":
		for _, s := range e.curseTargets() {
			out = append(out, state.Move{Action: actCurse, Count: s})
		}
	case "curse-hit":
		for _, typ := range e.curseVictims() {
			out = append(out, state.Move{Action: actAssign, Piece: typ})
		}
	case "curse-move":
		out = e.curseMoveMoves()
	case "hiero":
		out = e.hieroMoves(p)
	case "idol":
		out = e.idolMoves(p)
	case "syco":
		out = e.sycophancyMoves(p)
	case "channel":
		out = []state.Move{{Action: actChannel}, {Action: actDone}}
	case "undim":
		out = e.undimMoves(p)
	case "lose":
		for _, o := range e.lossOptions(p) {
			out = append(out, state.Move{Action: actLose, Piece: o.Type, To: e.m.ids[o.Area]})
		}
	case "submerge", "scream":
		total := 0
		for _, typ := range e.bringOptions(p) {
			out = append(out, state.Move{Action: actBring, Piece: typ})
			total += e.count(e.areaIdx(p.Area), p.Seat, typ)
		}
		if total > 1 {
			out = append(out, state.Move{Action: actBring, Piece: bringAll})
		}
		out = append(out, state.Move{Action: actDone})
	case "shriek":
		total := 0
		for _, a := range e.shriekOptions(p) {
			out = append(out, state.Move{Action: actBring, Piece: uByak, From: e.m.ids[a]})
			total += e.count(a, p.Seat, uByak)
		}
		if total > 1 {
			out = append(out, state.Move{Action: actBring, Piece: bringAll})
		}
		out = append(out, state.Move{Action: actDone})
	case "avatar":
		for _, typ := range e.swapOptions(p) {
			out = append(out, state.Move{Action: actSwap, Piece: typ})
		}
	case "desecrate":
		for _, typ := range e.desecratePieces(p.Seat) {
			out = append(out, state.Move{Action: actDesecrate, Piece: typ})
		}
	case "place":
		for _, s := range e.placeOptions() {
			for a := range e.m.ids {
				out = append(out, state.Move{Action: actPlace, To: e.m.ids[a], Count: s})
			}
		}
	case "react":
		out = e.reactMoves(p)
	case "howl":
		out = e.howlMoves(p)
	case "eternal":
		out = []state.Move{{Action: actEternal}, {Action: actDone}}
	case "demand":
		out = []state.Move{{Action: actDemand, Piece: "signs"}, {Action: actDemand, Piece: "pains"}}
	case "restore":
		for a := range e.m.ids {
			out = append(out, state.Move{Action: actRestore, To: e.m.ids[a]})
		}
	case "nexus":
		out = []state.Move{{Action: actNexus}, {Action: actDone}}
	case "moon":
		out = e.moonOptions(p.Seat)
	case "moonland":
		out = e.moonLandMoves(p.Seat)
	case "crypt-pick":
		out = e.cryptMoves(p.Seat)
	case "crypt":
		out = e.cryptPlaceMoves(p)
	case "sting":
		out = e.stingMovesFor(p)
	case "spare":
		out = e.spareMoves(p)
	case "pick":
		for _, v := range e.pickValues() {
			out = append(out, state.Move{Action: actPick, Count: v})
		}
	case "donate":
		out = e.donateMoves(p)
	case "zygote":
		out = e.zygoteMoves(p.Seat)
	}
	return out
}

// bringAll is the Piece of the move that takes every remaining unit along.
const bringAll = "all"

func (e *eng) doPrompt(seat int, mv state.Move) {
	d := e.d
	p := d.Prompts[0]
	switch mv.Action {
	case actBook:
		e.dropPrompt()
		e.awardBook(seat, mv.Count-1)
	case actMao:
		e.mao(seat, mv)
	case actCurse:
		e.dropPrompt()
		e.curseGive(mv.Count)
	case actAssign:
		e.dropPrompt()
		e.curseHit(mv.Piece)
	case actHiero:
		e.dropPrompt()
		e.priestAt(seat, e.areaIdx(string(mv.To)))
	case actSyco:
		e.dropPrompt()
		e.sycophancy(seat, mv)
	case actChannel:
		e.channel(seat)
	case actRearrange:
		e.rearrange(seat, mv)
	case actLose:
		e.applyLoss(p, PUArea{Area: e.areaIdx(string(mv.To)), Type: mv.Piece})
		e.popLoss()
	case actBring:
		switch p.Kind {
		case "shriek":
			e.flyByakhee(p, mv)
		case "idol":
			e.idolMove(p, mv)
		default:
			e.bringAlong(p, mv)
		}
	case actDone:
		switch p.Kind {
		case "eternal":
			// Declined: the result stands.
			e.resolveEternal(false)
		case "nexus":
			// No Action before the battle.
			e.dropPrompt()
		default:
			// Enough units went along (or the reaction is declined): the choice
			// is over.
			e.dropPrompt()
		}
	case actEternal:
		e.resolveEternal(true)
	case actDesecrate:
		e.dropPrompt()
		e.placeDesecrated(seat, p.Area, mv.Piece)
	case actSwap:
		e.dropPrompt()
		e.swapWithShub(p, mv.Piece)
	case actPlace:
		e.dropPrompt()
		to := e.areaIdx(string(mv.To))
		owner := mv.Count
		if e.place(to, owner, uCultist) {
			e.say("Ghroth: %s places a %s Cultist in %s.", e.seatName(seat), e.seatName(owner), e.areaName(to))
			if owner == seat {
				e.takeAbandonedGate(to, seat)
			}
		}
	case actWalk:
		e.walk(seat, e.areaIdx(string(mv.To)))
	case actMoon:
		e.dropPrompt()
		if p.Kind == "moonland" {
			e.moonLand(seat, e.areaIdx(string(mv.To)))
		} else {
			e.moonPlace(seat, mv)
		}
	case actCrypt:
		if p.Kind == "crypt-pick" {
			e.dropPrompt()
			e.cryptChoose(seat, mv.Count)
		} else {
			e.cryptPlace(p, e.areaIdx(string(mv.To)))
		}
	case actSting:
		e.stingMove(p, mv)
	case actSpare:
		e.spare(p, mv)
	case actPick:
		e.dropPrompt()
		e.settlePick(p.By, mv.Count)
	case actDonate:
		e.dropPrompt()
		e.donate(p, mv.Piece == "give")
	case actZygote:
		e.zygotePlace(seat, e.areaIdx(string(mv.To)))
		d.Prompts[0].Left--
		if d.Prompts[0].Left <= 0 || e.pool(seat, uCultist) <= 0 {
			e.dropPrompt()
		}
	case actDevolve:
		e.devolve(seat, e.areaIdx(string(mv.To)))
		d.Prompts[0].Lost++
	case actOath:
		e.sacrificePriest(seat)
		e.dropPrompt()
	case actRetreat:
		e.dropPrompt()
		if p.Kind == "curse-move" {
			e.curseMove(string(mv.To))
			break
		}
		// Howl: the enemy chooses which unit retreats, and where.
		e.howlRetreat(p, mv)
	case actDemand:
		e.dropPrompt()
		e.answerDemand(p, mv.Piece)
	case actRestore:
		e.dropPrompt()
		e.restoreSerpent(seat, e.areaIdx(string(mv.To)))
	case actNexus:
		e.dropPrompt()
		e.beginNexus(p)
		return
	}
	e.normalizePrompts()
	if len(d.Prompts) == 0 {
		d.Stage = d.Resume
		e.runThen()
	}
}

// bringAlong takes units with Cthulhu (to his faction card) or with the King
// in Yellow (to the area he walked to).
func (e *eng) bringAlong(p Prompt, mv state.Move) {
	d := e.d
	seat := p.Seat
	a := e.areaIdx(p.Area)
	types := []string{mv.Piece}
	all := mv.Piece == bringAll
	if all {
		types = e.bringOptions(p)
	}
	for _, typ := range types {
		n := 1
		if all {
			n = e.count(a, seat, typ)
		}
		for i := 0; i < n; i++ {
			if !e.take(a, seat, typ) {
				break
			}
			if p.Kind == "scream" {
				e.add(e.areaIdx(p.Dest), seat, typ)
				e.say("%s's %s walks with the King to %s.", e.seatName(seat), unitNameOf(typ), e.areaName(e.areaIdx(p.Dest)))
				continue
			}
			if d.Away == nil {
				d.Away = map[string]int{}
			}
			d.Away[typ]++
			e.say("%s takes %s down with him.", e.seatName(seat), article(unitNameOf(typ)))
		}
	}
}

// flyByakhee moves one Byakhee (or all of them) to where Shriek calls them.
func (e *eng) flyByakhee(p Prompt, mv state.Move) {
	seat := p.Seat
	dest := e.areaIdx(p.Area)
	froms := []int{}
	if mv.Piece == bringAll {
		froms = e.shriekOptions(p)
	} else {
		froms = append(froms, e.areaIdx(string(mv.From)))
	}
	moved := 0
	for _, from := range froms {
		n := 1
		if mv.Piece == bringAll {
			n = e.count(from, seat, uByak)
		}
		for i := 0; i < n && e.take(from, seat, uByak); i++ {
			e.add(dest, seat, uByak)
			moved++
		}
		e.syncGate(from)
	}
	e.say("Shriek of the Byakhee: %s fly to %s.", plural(moved, "Byakhee", "Byakhee"), e.areaName(dest))
}

// runThen continues after the prompts: what d.Then names.
func (e *eng) runThen() {
	d := e.d
	then := d.Then
	d.Then = ""
	switch then {
	case "after":
		e.afterAction()
	case "turn":
		d.Stage = stageTurn
		e.continueTurn()
	case "react":
		// The window after another player's action closed: the turn is where
		// it was.
		d.Stage = stageTurn
	case "endturn":
		// The window after the last Action of the phase closed.
		d.Stage = stageTurn
		e.endTurn()
	case "first":
		// The Gather Power window closed: the first player is chosen.
		e.determineFirst()
	case "resume":
		// d.Stage was restored by the prompts.
	case "battle-start":
		e.battlePreStart()
	case "battle-demand":
		e.advancePre(d.Bat.Att)
	case "battle-spared":
		e.afterKills()
	case "battle-pre":
		d.Stage = stagePre
		if len(e.preMoves(d.Bat.Who)) == 0 {
			d.Bat.Done[d.Bat.Who] = true
			e.advancePre(d.Bat.Who)
		}
	case "battle-tally":
		e.tallyBattle()
	case "battle-assign":
		e.advanceAssign()
	case "battle-post":
		e.advancePost(d.Bat.Att, true)
	case "battle-retreat":
		e.advanceRetreat()
	case "doom":
		e.continueDoom()
	case "doom-start":
		e.beginDoomPlayers()
	case "doom-end":
		e.finishDoomPhase()
	default:
		d.Stage = stageTurn
		if d.Phase == phaseAction && d.Turn != nil {
			e.continueTurn()
		}
	}
}

// --- faction actions ---------------------------------------------------------

func (e *eng) factionActions(seat int, second bool) []state.Move {
	d := e.d
	p := d.Power[seat]
	var out []state.Move
	add := func(m state.Move) {
		if p >= e.moveCost(seat, m) {
			out = append(out, m)
		}
	}
	// Hibernate: Windwalker's unique ability (and the Sleeper's, borrowed).
	if e.ability(seat, abHibernate) && !d.Hib[seat] {
		out = append(out, state.Move{Action: actHibernate})
	}
	// The Beyond One (the Opener's, and the Sleeper's when borrowed).
	if e.ability(seat, abBeyond) {
		for _, mv := range e.beyondMoves(seat) {
			add(mv)
		}
	}
	// Dread Curse of Azathoth, Idolatry and the Tcho-Tcho's requirement.
	for _, mv := range e.curseMoves(seat) {
		add(mv)
	}
	for _, mv := range e.idolatryMoves(seat) {
		add(mv)
	}
	out = append(out, e.dismantleMoves(seat)...)
	switch e.fac(seat) {
	case facGC:
		if d.hasBook(seat, bkSubmerge) {
			if p >= 1 {
				if a, ok := e.gooArea(seat, uCthulhu); ok && e.m.ocean[a] {
					out = append(out, state.Move{Action: actSubmerge})
				}
			}
			if len(d.Away) > 0 {
				for a := range e.m.ids {
					add(state.Move{Action: actEmerge, To: e.m.ids[a]})
				}
			}
		}
		if d.hasBook(seat, bkDreams) && p >= 2 && e.pool(seat, uCultist) > 0 {
			for a := range e.m.ids {
				for _, enemy := range e.others(seat) {
					if e.count(a, enemy, uCultist) > 0 {
						add(state.Move{Action: actDreams, To: e.m.ids[a], Count: enemy})
					}
				}
			}
		}
	case facCC:
		if d.hasBook(seat, bkForms) && !d.FormsUsed && e.onMap(seat, uNyar) > 0 {
			out = append(out, state.Move{Action: actForms})
		}
		if !d.reqDone(seat, rqCCPay4) && p >= 4 {
			out = append(out, state.Move{Action: actPay, Count: 4})
		}
		if !d.reqDone(seat, rqCCPay6) && p >= 6 {
			out = append(out, state.Move{Action: actPay, Count: 6})
		}
		if !d.reqDone(seat, rqCCPay4) && !d.reqDone(seat, rqCCPay6) && p >= 10 {
			out = append(out, state.Move{Action: actPay, Count: 10})
		}
	case facBG:
		if d.hasBook(seat, bkGhroth) && p >= 2 {
			out = append(out, state.Move{Action: actGhroth})
		}
		if !d.reqDone(seat, rqBGSac) && e.cultistsInPlay(seat) >= 2 {
			out = append(out, state.Move{Action: actSacrifice})
		}
		// Avatar: Shub-Niggurath swaps places with a Monster or Cultist of a
		// faction in another area.
		if shub, ok := e.gooArea(seat, uShub); ok && p >= 1 && !e.mindControlled(shub, seat) {
			for a := range e.m.ids {
				if a == shub {
					continue
				}
				for _, s := range e.playing() {
					if e.classCount(a, s, classMonster)+e.cultists(a, s) > 0 {
						add(state.Move{Action: actAvatar, To: e.m.ids[a], Count: s})
					}
				}
			}
		}
	case facYS:
		kingArea, king := e.gooArea(seat, uKing)
		if king {
			if d.hasBook(seat, bkScreaming) && p >= 1 && !second {
				for _, n := range e.m.adj[kingArea] {
					add(state.Move{Action: actScream, To: e.m.ids[n]})
				}
			}
			// One move: the die is rolled at once; the Monster or Cultist
			// that is placed whatever the roll was is chosen after it.
			if !d.Desec[e.areaID(kingArea)] && p >= e.desecrateCost(seat) && !e.mindControlled(kingArea, seat) {
				out = append(out, state.Move{Action: actDesecrate})
			}
		}
		if d.hasBook(seat, bkShriek) && p >= 1 && e.onMap(seat, uByak) > 0 {
			for a := range e.m.ids {
				add(state.Move{Action: actShriek, To: e.m.ids[a]})
			}
		}
		if d.hasBook(seat, bkZingaya) && p >= 1 && e.pool(seat, uUndead) > 0 {
			for a := range e.m.ids {
				if e.count(a, seat, uUndead) == 0 {
					continue
				}
				for _, enemy := range e.others(seat) {
					if e.cultists(a, enemy) > 0 {
						add(state.Move{Action: actZingaya, To: e.m.ids[a], Count: enemy})
					}
				}
			}
		}
		if d.hasBook(seat, bkHWINN) && p >= 1 && !second {
			if _, ok := e.gooArea(seat, uHastur); ok {
				for a := range e.m.ids {
					cultist := false
					for _, s := range e.playing() {
						if e.cultists(a, s) > 0 {
							cultist = true
						}
					}
					if cultist {
						add(state.Move{Action: actHWINN, To: e.m.ids[a]})
					}
				}
			}
		}
		// The fourth requirement is an action: another player gains 3 Doom.
		if !d.reqDone(seat, rqYSGift) {
			for _, other := range e.others(seat) {
				out = append(out, state.Move{Action: actGrant, Count: other})
			}
		}
	case facWW:
		// Ice Age: the token goes into any area.
		if d.hasBook(seat, bkIceAge) && p >= 1 {
			for a := range e.m.ids {
				if e.areaID(a) != d.Ice {
					add(state.Move{Action: actIceAge, To: e.m.ids[a]})
				}
			}
		}
	case facSL:
		// Lethargy: do nothing while another player still has Power to spend.
		if e.onMap(seat, uTsath) > 0 && e.anotherHasPower(seat) {
			out = append(out, state.Move{Action: actLethargy})
		}
		// The first three requirements are Actions of their own.
		if p >= 3 {
			if !d.reqDone(seat, rqSLBoon) {
				out = append(out, state.Move{Action: actBoon})
			}
			if !d.reqDone(seat, rqSLGift) {
				for _, other := range e.others(seat) {
					out = append(out, state.Move{Action: actBestow, Count: other})
				}
			}
			if !d.reqDone(seat, rqSLBane) {
				out = append(out, state.Move{Action: actBane})
			}
		}
		// Ancient Sorcery: a Serpent Man goes onto an enemy's faction card.
		if d.hasBook(seat, bkSorcery) && p >= 1 {
			for a := range e.m.ids {
				if e.count(a, seat, uSerpent) == 0 {
					continue
				}
				for _, enemy := range e.others(seat) {
					if e.sorceryOn(enemy) {
						continue
					}
					add(state.Move{Action: actSorcery, To: e.m.ids[a], Count: enemy})
				}
			}
		}
		// Cursed Slumber: a Gate and its Cultist go to sleep, or wake.
		if d.hasBook(seat, bkSlumber) && p >= 1 {
			if d.Slumber == nil {
				for _, a := range e.gatesOf(seat) {
					if e.cultists(a, seat) > 0 {
						add(state.Move{Action: actSlumber, To: e.m.ids[a]})
					}
				}
			} else {
				for a := range e.m.ids {
					if !e.hasGate(a) {
						add(state.Move{Action: actWake, To: e.m.ids[a]})
					}
				}
			}
		}
	}
	return out
}

// sorceryOn says one of the Sleeper's Serpent Men already lies on seat's
// faction card.
func (e *eng) sorceryOn(seat int) bool {
	for _, s := range e.d.Sorcery {
		if s.Seat == seat {
			return true
		}
	}
	return false
}

// desecrate is the King in Yellow's action: with no Desecration token in his
// area, roll one die; a result no higher than the number of the cult's units
// there (the King included) places a token. Token or none, a Monster or
// Cultist of cost 2 or less from the pool is then placed in the area — chosen
// in a prompt that follows the roll, which is what locks the move in.
func (e *eng) desecrate(seat int) {
	d := e.d
	e.pay(seat, state.Move{Action: actDesecrate})
	a, _ := e.gooArea(seat, uKing)
	roll := e.die()
	units := e.total(a, seat)
	ok := roll <= units
	e.say("Desecrate: the die shows %d against %d units in %s.", roll, units, e.areaName(a))
	if ok {
		d.Desec[e.areaID(a)] = true
		e.say("A Desecration token is placed in %s.", e.areaName(a))
		if d.hasBook(seat, bkThirdEye) && e.onMap(seat, uHastur) > 0 {
			e.gainSign(seat)
		}
	} else {
		e.say("The Desecration fails.")
	}
	// The unit comes first, the spellbook the glyph earns after it.
	d.Prompts = append(d.Prompts, Prompt{Kind: "desecrate", Seat: seat, Area: e.areaID(a)})
	if ok {
		switch e.m.glyph[a] {
		case "dragon":
			e.fulfil(seat, rqYSDragon)
		case "chevron":
			e.fulfil(seat, rqYSChevron)
		case "thorns":
			e.fulfil(seat, rqYSThorns)
		}
	}
	e.mainTaken()
}

// desecratePieces lists the unit types Desecrate may place: a Monster or
// Cultist with a cost of 2 or less that is in the pool.
func (e *eng) desecratePieces(seat int) []string {
	var out []string
	for _, u := range e.units(seat) {
		if (u.Class != classMonster && u.Class != classCultist) || u.Token || u.Cost > 2 || e.pool(seat, u.ID) <= 0 {
			continue
		}
		out = append(out, u.ID)
	}
	return out
}

// placeDesecrated puts the unit the Desecration brings in the King's area.
func (e *eng) placeDesecrated(seat int, area, typ string) {
	a := e.areaIdx(area)
	if e.place(a, seat, typ) {
		e.say("Desecrate: %s places %s in %s.", e.seatName(seat), article(unitNameOf(typ)), e.areaName(a))
		e.takeAbandonedGate(a, seat)
	}
}

func (e *eng) desecrateCost(seat int) int {
	c := 2
	if e.d.hasBook(seat, bkThirdEye) && e.onMap(seat, uHastur) > 0 {
		c--
	}
	return c
}

// ghrothShares spreads the n Cultists Ghroth eliminates over the enemies in
// the way they would be most likely to agree: each in turn gives up one from
// whoever has the most on the map. (Rules: they decide between them; a seat
// with fewer Cultists than its share loses all it has and the excess is
// ignored.)
func (e *eng) ghrothShares(seat, n int) map[int]int {
	shares := map[int]int{}
	have := map[int]int{}
	for _, s := range e.others(seat) {
		have[s] = e.cultistsInPlay(s)
	}
	for ; n > 0; n-- {
		best := 0
		for _, s := range e.others(seat) {
			if have[s] > 0 && (best == 0 || have[s] > have[best]) {
				best = s
			}
		}
		if best == 0 {
			break
		}
		have[best]--
		shares[best]++
	}
	return shares
}

// eliminateCultist eliminates one Cultist of enemy in a — an Acolyte first,
// the High Priest when no Acolyte stands there.
func (e *eng) eliminateCultist(a, enemy, by int) {
	typ := uCultist
	if e.count(a, enemy, uCultist) == 0 {
		typ = uHighPriest
	}
	e.eliminate(a, enemy, typ, by)
}

// factionAction executes a spellbook or faction action.
func (e *eng) factionAction(seat int, mv state.Move, to int) {
	d := e.d
	switch mv.Action {
	case actBeyond:
		e.pay(seat, mv)
		e.beyond(seat, mv)
		e.mainTaken()
	case actCurse:
		e.beginCurse(seat, string(mv.To))
		return
	case actIdolatry:
		e.pay(seat, mv)
		e.say("Idolatry: %s calls their units to %s.", e.seatName(seat), e.areaName(to))
		d.Prompts = append(d.Prompts, Prompt{Kind: "idol", Seat: seat, Area: e.areaID(to)})
		e.mainTaken()
		e.thenAfterPrompts("after")
		e.beginPrompts("after")
		return
	case actDismantle:
		e.dismantle(seat, to)
	case actSubmerge:
		d.Power[seat]--
		a, _ := e.gooArea(seat, uCthulhu)
		if d.Away == nil {
			d.Away = map[string]int{}
		}
		// Cthulhu always goes under; the player then picks which of the
		// units standing with him go too.
		e.take(a, seat, uCthulhu)
		d.Away[uCthulhu]++
		e.say("%s submerges Cthulhu from %s.", e.seatName(seat), e.areaName(a))
		e.mainTaken()
		p := Prompt{Kind: "submerge", Seat: seat, Area: e.areaID(a)}
		if len(e.bringOptions(p)) > 0 {
			d.Prompts = append(d.Prompts, p)
			e.thenAfterPrompts("after")
			e.beginPrompts("after")
			return
		}
		e.syncGate(a)
	case actEmerge:
		e.pay(seat, mv)
		for _, typ := range unitTypes {
			for i := d.Away[typ]; i > 0; i-- {
				e.add(to, seat, typ)
			}
		}
		d.Away = nil
		e.say("Cthulhu and his minions rise in %s.", e.areaName(to))
		e.takeAbandonedGate(to, seat)
		e.mainTaken()
	case actDreams:
		d.Power[seat] -= 2
		enemy := mv.Count
		e.say("Dreams: %s's Cultist in %s is replaced by one of %s's.", e.seatName(enemy), e.areaName(to), e.seatName(seat))
		e.eliminate(to, enemy, uCultist, seat)
		e.place(to, seat, uCultist)
		e.takeAbandonedGate(to, seat)
		e.mainTaken()
	case actForms:
		n := e.die()
		d.FormsUsed = true
		e.say("The Thousand Forms: the die shows %d.", n)
		enemies := e.others(seat)
		total := 0
		for _, s := range enemies {
			total += d.Power[s]
		}
		if total < n {
			// They cannot pay it between them: the Power is Crawling Chaos'.
			d.Power[seat] += n
			e.say("The enemies cannot pay it: %s gains %d Power.", e.seatName(seat), n)
		} else {
			// Standing in for the foes agreeing: each point comes from whoever
			// has the most Power left.
			lost := map[int]int{}
			for i := 0; i < n; i++ {
				best := 0
				for _, s := range enemies {
					if d.Power[s] > 0 && (best == 0 || d.Power[s] > d.Power[best]) {
						best = s
					}
				}
				d.Power[best]--
				lost[best]++
			}
			var parts []string
			for _, s := range enemies {
				if lost[s] > 0 {
					parts = append(parts, fmt.Sprintf("%s %d", e.seatName(s), lost[s]))
				}
			}
			e.say("The enemies lose %d Power between them: %s.", n, strings.Join(parts, ", "))
		}
		e.mainTaken()
	case actGhroth:
		d.Power[seat] -= 2
		n := e.die()
		fungiAreas := 0
		for a := range e.m.ids {
			if e.count(a, seat, uFungi) > 0 {
				fungiAreas++
			}
		}
		e.say("Ghroth: the die shows %d; Fungi stand in %d areas.", n, fungiAreas)
		if n <= fungiAreas {
			shares := e.ghrothShares(seat, n)
			var parts []string
			for _, s := range e.others(seat) {
				if shares[s] > 0 {
					parts = append(parts, fmt.Sprintf("%s %d", e.seatName(s), shares[s]))
					d.Prompts = append(d.Prompts, Prompt{Kind: "lose", Seat: s, Why: "Ghroth", Left: shares[s], CultistsOnly: true, By: seat})
				}
			}
			if len(parts) == 0 {
				e.say("The enemies have no Cultists to lose.")
			} else {
				e.say("The enemies must eliminate %s between them: %s.", plural(n, "Cultist", "Cultists"), strings.Join(parts, ", "))
			}
		} else if len(e.placeOptions()) > 0 {
			// Any faction's Acolyte goes anywhere on the map.
			d.Prompts = append(d.Prompts, Prompt{Kind: "place", Seat: seat, Why: "Ghroth"})
		}
		e.mainTaken()
		e.thenAfterPrompts("after")
		e.beginPrompts("after")
	case actAvatar:
		e.pay(seat, mv)
		e.say("Avatar: %s swaps Shub-Niggurath with a unit of %s in %s.", e.seatName(seat), e.seatName(mv.Count), e.areaName(to))
		d.Prompts = append(d.Prompts, Prompt{Kind: "avatar", Seat: mv.Count, Area: e.areaID(to), By: seat})
		e.mainTaken()
		e.thenAfterPrompts("after")
		e.beginPrompts("after")
	case actScream:
		e.pay(seat, mv)
		from, _ := e.gooArea(seat, uKing)
		e.take(from, seat, uKing)
		e.add(to, seat, uKing)
		e.say("The King in Yellow walks from %s to %s.", e.areaName(from), e.areaName(to))
		e.syncGate(from)
		// Any of the Undead with him may go along for free; then a second,
		// different action follows (not He Who is Not to be Named).
		d.Turn.Main, d.Turn.Second, d.Turn.SecondTaken = true, true, false
		e.acted()
		p := Prompt{Kind: "scream", Seat: seat, Area: e.areaID(from), Dest: e.areaID(to)}
		if len(e.bringOptions(p)) > 0 {
			d.Prompts = append(d.Prompts, p)
			e.thenAfterPrompts("after")
			e.beginPrompts("after")
		}
	case actShriek:
		e.pay(seat, mv)
		p := Prompt{Kind: "shriek", Seat: seat, Area: e.areaID(to)}
		e.say("%s calls the Byakhee to %s.", e.seatName(seat), e.areaName(to))
		e.mainTaken()
		if len(e.shriekOptions(p)) > 0 {
			d.Prompts = append(d.Prompts, p)
			e.thenAfterPrompts("after")
			e.beginPrompts("after")
		}
	case actZingaya:
		e.pay(seat, mv)
		enemy := mv.Count
		e.say("Zingaya: %s loses a Cultist in %s and %s gains an Undead there.", e.seatName(enemy), e.areaName(to), e.seatName(seat))
		e.eliminateCultist(to, enemy, seat)
		e.place(to, seat, uUndead)
		e.mainTaken()
	case actDesecrate:
		e.desecrate(seat)
	case actGrant:
		target := mv.Count
		before := d.Doom[target]
		e.addDoom(target, 3, doomGifts)
		e.say("%s grants 3 Doom to %s: %d → %d on the Doom track.", e.seatName(seat), e.seatName(target), before, d.Doom[target])
		if d.Doom[target] >= e.game.ParamInt("doom_to_win", 30) {
			d.EndPending = true
		}
		e.fulfil(seat, rqYSGift)
		e.mainTaken()
	case actHWINN:
		e.pay(seat, mv)
		from, _ := e.gooArea(seat, uHastur)
		e.take(from, seat, uHastur)
		e.add(to, seat, uHastur)
		e.say("Hastur appears in %s.", e.areaName(to))
		e.syncGate(from)
		// A second, different action follows.
		d.Turn.Main = true
		d.Turn.Second = true
		d.Turn.SecondTaken = false
		e.acted()

	// --- Windwalker ---
	case actHibernate:
		gain := e.enemyGreatOldOnes(seat)
		if gain > d.Power[seat] {
			gain = d.Power[seat]
		}
		d.Power[seat] += gain
		d.Hib[seat] = true
		e.say("Hibernate: %s gains %s (now %d) and takes no more Actions this phase.", e.seatName(seat), plural(gain, "Power", "Power"), d.Power[seat])
		e.mainTaken()
	case actIceAge:
		e.pay(seat, mv)
		d.Ice = e.areaID(to)
		e.say("Ice Age: %s puts the Ice Age token in %s.", e.seatName(seat), e.areaName(to))
		e.mainTaken()
	case actClaim:
		e.fulfil(seat, rqWWTake)

	// --- Sleeper ---
	case actLethargy:
		e.say("Lethargy: %s does nothing.", e.seatName(seat))
		e.mainTaken()
	case actBoon:
		d.Power[seat] -= 3
		for _, o := range e.others(seat) {
			d.Power[o]++
		}
		e.say("%s pays 3 Power: every other player gains 1 Power.", e.seatName(seat))
		e.fulfil(seat, rqSLBoon)
		e.mainTaken()
	case actBestow:
		d.Power[seat] -= 3
		d.Power[mv.Count] += 3
		e.say("%s pays 3 Power: %s gains 3 Power.", e.seatName(seat), e.seatName(mv.Count))
		e.fulfil(seat, rqSLGift)
		e.mainTaken()
	case actBane:
		d.Power[seat] -= 3
		for _, o := range e.others(seat) {
			if d.Power[o] > 0 {
				d.Power[o]--
			}
		}
		e.say("%s pays 3 Power: every other player loses 1 Power.", e.seatName(seat))
		e.fulfil(seat, rqSLBane)
		e.mainTaken()
	case actSorcery:
		e.pay(seat, mv)
		e.take(to, seat, uSerpent)
		d.Sorcery = append(d.Sorcery, SorceryState{Seat: mv.Count})
		e.say("Ancient Sorcery: a Serpent Man of %s goes onto %s's faction card — until the end of the Doom phase %s has its unique ability.", e.seatName(seat), e.seatName(mv.Count), e.seatName(seat))
		e.mainTaken()
	case actSlumber:
		e.pay(seat, mv)
		typ := uCultist
		if e.count(to, seat, uCultist) == 0 {
			typ = uHighPriest
		}
		e.take(to, seat, typ)
		delete(d.Gates, e.areaID(to))
		delete(d.Holders, e.areaID(to))
		d.Slumber = &SlumberState{Cultist: typ}
		e.say("Cursed Slumber: %s puts the Gate in %s and its %s to sleep.", e.seatName(seat), e.areaName(to), unitNameOf(typ))
		e.mainTaken()
	case actWake:
		e.pay(seat, mv)
		typ := uCultist
		if d.Slumber != nil {
			typ = d.Slumber.Cultist
		}
		d.Slumber = nil
		e.add(to, seat, typ)
		e.occupy(to, seat, typ)
		e.say("%s wakes the Gate in %s.", e.seatName(seat), e.areaName(to))
		e.mainTaken()
	}
}
