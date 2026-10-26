package cthulhuwars

import (
	"fmt"
	"sort"
	"strings"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Stage for choosing between tied players for first player.
const stageFirst = "first"

// endActionPhase runs the rest of the round after the last player runs out
// of Power: Gather Power, Determine First Player, then the Doom phase.
func (e *eng) endActionPhase() {
	d := e.d
	d.Turn = nil
	d.Move = nil
	d.Saved = nil
	d.Stage = ""
	d.ReactOwed = 0
	d.FormsUsed = false // The Thousand Forms is turned face up again
	e.say("— Gather Power —")
	e.gatherPower()
	d.Hib = [nSeats + 1]bool{}
	// The Mao Ceremony: at the end of Gather Power, after the Power has been
	// added, a holder may sacrifice Cultists for Power.
	for _, s := range e.turnOrder(d.First, d.Dir) {
		if d.hasNeutral(s, nbMao) {
			d.Prompts = append(d.Prompts, Prompt{Kind: "mao", Seat: s})
		}
	}
	// A High Priest may be sacrificed in Gather Power, before the first player
	// is chosen: every holder is asked.
	for _, s := range e.turnOrder(d.First, d.Dir) {
		if e.oathAvailable(s) {
			d.Prompts = append(d.Prompts, Prompt{Kind: "react", Seat: s, Why: "oath"})
		}
	}
	if len(d.Prompts) > 0 {
		e.thenAfterPrompts("first")
		e.beginPrompts("first")
		return
	}
	e.determineFirst()
}

// oathAtGather says seat's High Priest could make it the first player: it
// trails the others now, and with 2 Power more would not.
func (e *eng) oathAtGather(seat int) bool {
	if !e.oathAvailable(seat) {
		return false
	}
	lead := 0
	for _, o := range e.others(seat) {
		if e.d.Power[o] > lead {
			lead = e.d.Power[o]
		}
	}
	p := e.d.Power[seat]
	return p <= lead && p+2 > lead
}

func (e *eng) gatherPower() {
	d := e.d
	abandoned := 0
	for _, ctl := range d.Gates {
		if ctl == 0 {
			abandoned++
		}
	}
	for _, s := range e.playing() {
		// What the cult kept from the Action phase: Power it banked by
		// hibernating (every other cult has spent or dropped all of it).
		banked := d.Power[s]
		var parts []string
		gain := 0
		add := func(n int, text string) {
			if n > 0 {
				gain += n
				parts = append(parts, text)
			}
		}
		cultists := e.cultistsInPlay(s)
		add(cultists, fmt.Sprintf("%d %s", cultists, plural1(cultists, "Cultist", "Cultists")))
		gates := e.gateCount(s)
		add(2*gates, fmt.Sprintf("%d %s × 2 = %d", gates, plural1(gates, "Gate", "Gates"), 2*gates)+asleep(e.slumberGates(s)))
		add(abandoned, fmt.Sprintf("%d %s", abandoned, plural1(abandoned, "abandoned Gate", "abandoned Gates")))
		// Captured Cultists go home; their captors earn 1 Power each.
		captives := 0
		for owner := 1; owner <= nSeats; owner++ {
			// Soulless: the Tcho-Tcho's captured Cultists give 0 Power.
			worth := 1
			if e.is(owner, facTT) && d.hasBook(owner, bkSoulless) {
				worth = 0
			}
			n := d.Captured[s][owner]
			if n > 0 {
				d.Captured[s][owner] = 0
				e.setPool(owner, uCultist, e.pool(owner, uCultist)+n)
				captives += worth * n
			}
			if n := d.CapHP[s][owner]; n > 0 {
				d.CapHP[s][owner] = 0
				e.setPool(owner, uHighPriest, e.pool(owner, uHighPriest)+n)
				captives += worth * n
			}
		}
		add(captives, fmt.Sprintf("%d %s", captives, plural1(captives, "captive", "captives")))
		// Monsters the Sleeper captured are sacrificed for 1 Power each.
		if e.is(s, facSL) && len(d.Held) > 0 {
			n := len(d.Held)
			for _, h := range d.Held {
				e.setPool(h.Seat, h.Type, e.pool(h.Seat, h.Type)+1)
			}
			add(n, fmt.Sprintf("%d %s sacrificed", n, plural1(n, "captured Monster", "captured Monsters")))
			d.Held = nil
		}
		if e.is(s, facGC) && d.hasBook(s, bkYha) && e.gooInPlay(s) > 0 {
			n := 0
			for id, ctl := range d.Gates {
				if ctl != 0 && ctl != s && e.m.ocean[e.areaIdx(id)] {
					n++
				}
			}
			add(n, fmt.Sprintf("Y'ha-Nthlei: %d enemy ocean %s", n, plural1(n, "Gate", "Gates")))
		}
		// Feast: Yellow Sign's unique ability (also the Sleeper's, borrowed).
		if e.ability(s, abFeast) {
			n := 0
			for id := range d.Desec {
				if e.total(e.areaIdx(id), s) > 0 {
					n++
				}
			}
			add(n, fmt.Sprintf("Feast: %d %s", n, plural1(n, "Desecrated area", "Desecrated areas")))
		}
		// The Red Sign: each Dark Young provides 1 Power.
		if e.is(s, facBG) && d.hasBook(s, bkRedSign) {
			n := e.onMap(s, uYoung)
			add(n, fmt.Sprintf("The Red Sign: %d Dark Young", n))
		}
		d.Power[s] += gain
		line := fmt.Sprintf("%s gathers %d Power", e.seatName(s), gain)
		if len(parts) > 0 {
			line += " (" + strings.Join(parts, " + ") + ")"
		}
		if banked > 0 {
			line += fmt.Sprintf(" and keeps the %d it banked", banked)
		}
		e.say("%s: now %d.", line, d.Power[s])
	}
	// Nobody falls below half the leader.
	lead := 0
	for _, s := range e.playing() {
		if d.Power[s] > lead {
			lead = d.Power[s]
		}
	}
	half := (lead + 1) / 2
	for _, s := range e.playing() {
		if d.Power[s] < half {
			e.say("%s is raised to half the leader's Power (%d).", e.seatName(s), half)
			d.Power[s] = half
		}
	}
}

// plural1 picks the singular or plural noun for n.
func plural1(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// asleep notes the Sleeper's sleeping Gate in a Gate count.
func asleep(n int) string {
	if n > 0 {
		return " (one asleep)"
	}
	return ""
}

func (e *eng) determineFirst() {
	d := e.d
	best := -1
	var tied []int
	for _, s := range e.playing() {
		switch {
		case d.Power[s] > best:
			best, tied = d.Power[s], []int{s}
		case d.Power[s] == best:
			tied = append(tied, s)
		}
	}
	if len(tied) > 1 {
		// The previous first player decides among the tied players.
		d.Stage = stageFirst
		d.TiedFirst = tied
		return
	}
	e.setFirst(tied[0])
}

// setFirst makes seat the first player; the Windwalker's requirement is met
// when it is.
func (e *eng) setFirst(seat int) {
	d := e.d
	d.First = seat
	d.TiedFirst = nil
	e.say("%s is the first player.", e.seatName(d.First))
	d.Stage = stageDirection
	if e.is(seat, facWW) {
		e.fulfil(seat, rqWWFirst)
	}
	if len(d.Prompts) > 0 {
		e.thenAfterPrompts("resume")
		e.beginPrompts("resume")
	}
}

func (e *eng) legalFirst() []state.Move {
	var out []state.Move
	for _, s := range e.d.TiedFirst {
		out = append(out, state.Move{Action: "first", Count: s})
	}
	return out
}

// --- Doom ----------------------------------------------------------------------

// Where Doom comes from (see DoomLedger).
const (
	doomGates   = "gates"
	doomRituals = "rituals"
	doomGifts   = "gifts"
	doomSigns   = "signs"
	doomCards   = "cards" // Gnorri's Grottos, Messenger of Yig
)

// addDoom raises seat's Doom by n and notes what it came from.
func (e *eng) addDoom(seat, n int, source string) {
	l := &e.d.DoomBy[seat]
	e.d.Doom[seat] += n
	switch source {
	case doomGates:
		l.Gates += n
	case doomRituals:
		l.Rituals += n
	case doomGifts:
		l.Gifts += n
	case doomSigns:
		l.Signs += n
	case doomCards:
		l.Cards += n
	}
}

// doomSources spells out what seat's Doom is made of: "3 from Gates, 3 from
// gifts". Doom from before the ledger was kept shows as "earlier".
func (e *eng) doomSources(seat int) string {
	l := e.d.DoomBy[seat]
	var parts []string
	add := func(n int, text string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, text))
		}
	}
	add(l.Gates, "from Gates")
	add(l.Rituals, "from Rituals")
	add(l.Gifts, "from gifts")
	add(l.Signs, "from Elder Signs")
	add(l.Cards, "from Loyalty Cards")
	add(e.d.Doom[seat]-l.Gates-l.Rituals-l.Gifts-l.Signs-l.Cards, "from earlier")
	if len(parts) == 0 {
		return "none yet"
	}
	return strings.Join(parts, ", ")
}

// --- the Doom phase -------------------------------------------------------------

func (e *eng) startDoomPhase() {
	d := e.d
	d.Phase = phaseDoom
	e.say("— Doom Phase —")
	// Blasphemous Obeisance: the Moonbeasts come back to their owners' Gates.
	e.moonReturn()
	// Spellbooks that arrive in the Doom phase.
	if gc := e.seatOf(facGC); gc != 0 {
		// First Doom phase: a spellbook and an Elder Sign (the reward is paid
		// by fulfil).
		// A last spellbook must be taken in a Doom phase, with an Elder Sign;
		// one Doom phase gives one spellbook.
		if !e.fulfil(gc, rqGCFirstDoom) && d.bookCount(gc) == 5 {
			e.fulfil(gc, rqGCFive)
		}
	}
	e.checkRequirements()
	// Doom track advancement: every controlled Gate scores 1. The log says
	// what each total is made of, so that a score that looks too high can be
	// traced to the Gates, a Ritual, a gift or the Elder Signs.
	goal := e.game.ParamInt("doom_to_win", 30)
	for _, s := range e.playing() {
		g := e.doomGateCount(s)
		before := d.Doom[s]
		e.addDoom(s, g, doomGates)
		if spoiled := e.gateCount(s) - g; spoiled > 0 {
			e.say("The Brood: %s's %s do not count (Filth lies there).", e.seatName(s), plural(spoiled, "Gate", "Gates"))
		}
		// Grottos: Gnorri in play earn extra Doom.
		if n := e.grottos(s); n > 0 {
			e.addDoom(s, n, doomCards)
			e.say("Grottos: %s's %s earn %d extra Doom.", e.seatName(s), plural(e.onMap(s, uGnorri), "Gnorri", "Gnorri"), n)
		}
		if g > 0 {
			e.say("%s scores %d Doom for %s: %d → %d on the Doom track (%s).", e.seatName(s), g, plural(g, "Gate", "Gates"), before, d.Doom[s], e.doomSources(s))
		} else if d.Doom[s] > 0 {
			e.say("%s controls no Gates and scores no Doom: %d on the Doom track (%s).", e.seatName(s), d.Doom[s], e.doomSources(s))
		}
		if d.Doom[s] >= goal {
			d.EndPending = true
		}
	}
	// Messenger of Yig: every other cult gives 1 Power, or its owner gains 1 Doom.
	if y := e.messengerOwner(); y != 0 {
		for _, o := range e.others(y) {
			d.Prompts = append(d.Prompts, Prompt{Kind: "donate", Seat: o, By: y})
		}
	}
	// Ubbo-Sathla feeds: Hell's Banquet.
	e.hellsBanquet()
	e.thenAfterPrompts("doom-start")
	e.beginPrompts("doom-start")
}

// beginDoomPlayers starts the per-player Doom decisions in turn order.
func (e *eng) beginDoomPlayers() {
	d := e.d
	d.Doomx = &DoomState{Order: e.turnOrder(d.First, d.Dir)}
	d.Stage = stageDoom
	e.settleDoom()
}

// settleDoom skips players with nothing to decide.
func (e *eng) settleDoom() {
	d := e.d
	for d.Doomx != nil && len(d.Doomx.Order) > 0 {
		if len(e.doomMoves(d.Doomx.Order[0])) > 1 {
			d.Stage = stageDoom
			return
		}
		d.Doomx.Order = d.Doomx.Order[1:]
		d.Doomx.Ritualled, d.Doomx.Sacrificed, d.Doomx.Death, d.Doomx.Bought, d.Doomx.Crypt = false, false, false, false, false
	}
	e.endDoomPhase()
}

// ritualCostFor is what seat pays for the Ritual of Annihilation: the track's
// price, or 5 for Windwalker with Herald of the Outer Gods.
func (e *eng) ritualCostFor(seat int) int {
	if e.is(seat, facWW) && e.d.hasBook(seat, bkHerald) {
		return 5
	}
	return e.ritualCost()
}

func (e *eng) doomMoves(seat int) []state.Move {
	d := e.d
	x := d.Doomx
	var out []state.Move
	cost := e.ritualCostFor(seat)
	if !x.Ritualled && d.Power[seat] >= cost {
		out = append(out, state.Move{Action: actRitual})
		if e.descendAvailable(seat) {
			out = append(out, state.Move{Action: actRitual, Piece: "descend"})
		}
	}
	// Ubbo-Sathla is awakened for nothing in the Doom phase.
	if d.Phase == phaseDoom {
		for _, a := range e.ubboAreas(seat) {
			out = append(out, state.Move{Action: actAwaken, Piece: uUbbo, To: e.m.ids[a]})
		}
	}
	out = append(out, e.ascendMoves(seat)...)
	if e.is(seat, facBG) && !x.Sacrificed && d.hasBook(seat, bkBlood) && e.gooInPlay(seat) > 0 {
		for a := range e.m.ids {
			if e.cultists(a, seat) > 0 {
				out = append(out, state.Move{Action: actBlood, To: e.m.ids[a]})
			}
		}
	}
	// Death from Below: the cheapest Monster of the pool, into an area where
	// the Sleeper has a unit.
	if e.ability(seat, abDeath) && !x.Death && e.lowestCostMonster(seat) != "" {
		for a := range e.m.ids {
			if e.total(a, seat) > 0 {
				out = append(out, state.Move{Action: actDeath, To: e.m.ids[a]})
			}
		}
	}
	// Unspeakable Oath, when 2 Power more would pay for the Ritual.
	if e.oathAvailable(seat) && !x.Ritualled && d.Power[seat] < cost && d.Power[seat]+2 >= cost {
		out = append(out, state.Move{Action: actOath})
	}
	// Windwalker's "at any time" spellbook, once an enemy has all six.
	if e.is(seat, facWW) && !d.reqDone(seat, rqWWTake) && d.bookCount(seat) < 6 && e.enemiesWithSixBooks(seat) > 0 {
		out = append(out, state.Move{Action: actClaim})
	}
	if len(d.Signs[seat]) > 0 {
		out = append(out, state.Move{Action: actReveal})
	}
	out = append(out, e.buyMoves(seat)...)
	// Crypt Dweller: an enemy places the Wamps of the pool.
	if !x.Crypt {
		out = append(out, e.cryptMoves(seat)...)
	}
	out = append(out, e.ransomMoves(seat)...)
	return append(out, state.Move{Action: actSkip})
}

func (e *eng) legalDoom() []state.Move {
	d := e.d
	if d.Doomx == nil || len(d.Doomx.Order) == 0 {
		return nil
	}
	return e.doomMoves(d.Doomx.Order[0])
}

func (e *eng) doDoom(seat int, mv state.Move) {
	d := e.d
	x := d.Doomx
	switch mv.Action {
	case actSkip:
		x.Order = x.Order[1:]
		x.Ritualled, x.Sacrificed, x.Death, x.Bought, x.Crypt = false, false, false, false, false
		e.settleDoom()
		return
	case actRitual:
		cost := e.ritualCostFor(seat)
		d.Power[seat] -= cost
		g := e.doomGateCount(seat)
		before := d.Doom[seat]
		// Dragon Descending: twice the Doom, once in the game.
		if mv.Piece == "descend" {
			d.Used[seat] |= 2
			g *= 2
			e.say("Dragon Descending: %s's Ritual earns twice the Doom.", e.seatName(seat))
		}
		e.addDoom(seat, g, doomRituals)
		signs := e.gooInPlay(seat)
		x.Ritualled = true
		e.say("%s performs a Ritual of Annihilation (%d Power): +%d Doom for %s, %d → %d on the Doom track (%s), and %s for %s.",
			e.seatName(seat), cost, g, plural(g, "Gate", "Gates"), before, d.Doom[seat], e.doomSources(seat),
			plural(signs, "Elder Sign", "Elder Signs"), plural(signs, "Great Old One", "Great Old Ones"))
		for i := 0; i < signs; i++ {
			e.gainSign(seat)
		}
		// Tablets of the Gods: more Elder Signs, and the High Priests go.
		if e.is(seat, facTT) && d.hasBook(seat, bkTablets) {
			e.tablets(seat)
		}
		// Sycophancy: the Tcho-Tcho gains 1 Doom, or the performer earns 1 less.
		for _, s := range e.sycophants(seat) {
			d.Prompts = append(d.Prompts, Prompt{Kind: "syco", Seat: seat, By: s, Left: g})
			if e.is(s, facTT) {
				e.fulfil(s, rqTTRitual)
			}
		}
		e.advanceRitual()
		if e.is(seat, facSL) {
			e.fulfil(seat, rqSLRitual)
		}
		if d.Doom[seat] >= e.game.ParamInt("doom_to_win", 30) {
			d.EndPending = true
		}
	case actBlood:
		a := e.areaIdx(string(mv.To))
		typ := uCultist
		if e.count(a, seat, uCultist) == 0 {
			typ = uHighPriest
		}
		e.eliminate(a, seat, typ, 0)
		x.Sacrificed = true
		e.say("Blood Sacrifice: %s eliminates a Cultist for an Elder Sign.", e.seatName(seat))
		e.gainSign(seat)
	case actDeath:
		typ := e.lowestCostMonster(seat)
		a := e.areaIdx(string(mv.To))
		if typ != "" && e.place(a, seat, typ) {
			e.say("Death from Below: %s places %s in %s.", e.seatName(seat), article(unitNameOf(typ)), e.areaName(a))
		}
		x.Death = true
	case actOath:
		e.sacrificePriest(seat)
	case actClaim:
		e.fulfil(seat, rqWWTake)
	case actReveal:
		e.reveal(seat)
	case actBuy:
		e.buy(seat, mv)
	case actCrypt:
		x.Crypt = true
		e.cryptChoose(seat, mv.Count)
	case actRansom:
		e.ransom(seat, mv)
	case actAwaken:
		// Ubbo-Sathla, for nothing, in the Doom phase.
		e.awakenUbbo(seat, e.areaIdx(string(mv.To)))
		e.say("%s awakens Ubbo-Sathla in %s: his Growth counter is %d.", e.seatName(seat), e.areaName(e.areaIdx(string(mv.To))), d.Growth)
	case actAscend:
		e.ascend(seat, mv.Count)
	}
	e.checkRequirements()
	if len(d.Prompts) > 0 {
		e.thenAfterPrompts("doom")
		e.beginPrompts("doom")
		return
	}
	e.continueDoom()
}

func (e *eng) continueDoom() {
	d := e.d
	d.Stage = stageDoom
	e.settleDoom()
}

func (e *eng) endDoomPhase() {
	d := e.d
	d.Doomx = nil
	// Ancient Sorcery wears off with the Doom phase: each Serpent Man comes
	// back (its owner places him) and brings 1 Power.
	if len(d.Sorcery) > 0 && !d.EndPending && !d.Instant {
		sl := e.seatOf(facSL)
		for i := range d.Sorcery {
			d.Sorcery[i].Ending = true
			if sl != 0 {
				d.Power[sl]++
				d.Prompts = append(d.Prompts, Prompt{Kind: "restore", Seat: sl})
			}
		}
		e.say("Ancient Sorcery wears off: the Sleeper gains %s and takes its Serpent Men back.", plural(len(d.Sorcery), "Power", "Power"))
		e.thenAfterPrompts("doom-end")
		e.beginPrompts("doom-end")
		return
	}
	e.finishDoomPhase()
}

// restoreSerpent puts a Serpent Man the Sleeper took back from an enemy's
// faction card into area a.
func (e *eng) restoreSerpent(seat, a int) {
	d := e.d
	for i, s := range d.Sorcery {
		if s.Ending {
			d.Sorcery = append(d.Sorcery[:i], d.Sorcery[i+1:]...)
			break
		}
	}
	e.add(a, seat, uSerpent)
	e.say("The Serpent Man of %s returns to %s.", e.seatName(seat), e.areaName(a))
}

// finishDoomPhase ends the round: the game, or the next Action phase.
func (e *eng) finishDoomPhase() {
	d := e.d
	if d.EndPending || d.Instant {
		reason := "doom"
		if d.Instant {
			reason = "instant-death"
		}
		e.finishGame(reason)
		return
	}
	d.Round++
	d.Phase = phaseAction
	d.Stage = stageTurn
	d.Turn = &TurnState{Seat: d.First}
	for s := range d.Battled {
		d.Battled[s] = nil
	}
	e.say("— Round %d —", d.Round)
	e.continueTurn()
}

// finishGame ends the game: every Elder Sign is revealed and the winner is
// the player with the most Doom among those with all six spellbooks.
func (e *eng) finishGame(reason string) {
	d := e.d
	// An Elder Sign whose value was still to be chosen is drawn at random.
	for _, s := range d.Picks {
		e.settlePick(s, 0)
	}
	d.Picks = nil
	for _, s := range e.playing() {
		if len(d.Signs[s]) > 0 {
			e.reveal(s)
		}
	}
	d.Phase = phaseOver
	d.Stage = ""
	d.Turn, d.Move, d.Bat, d.Doomx = nil, nil, nil, nil
	var cands []int
	for _, s := range e.playing() {
		if d.bookCount(s) >= 6 {
			cands = append(cands, s)
		}
	}
	if len(cands) == 0 {
		d.Winner, d.Reason = 0, "humanity"
		e.say("Nobody has all six spellbooks: humanity survives, and every cult loses.")
		return
	}
	sort.Slice(cands, func(i, j int) bool { return d.Doom[cands[i]] > d.Doom[cands[j]] })
	if len(cands) > 1 && d.Doom[cands[0]] == d.Doom[cands[1]] {
		d.Winner, d.Reason = 0, "draw"
		e.say("The game ends in a draw.")
		return
	}
	d.Winner = cands[0]
	d.Reason = reason
	e.say("%s wins with %d Doom and all six spellbooks (%s).", e.seatName(d.Winner), d.Doom[d.Winner], reason)
}
