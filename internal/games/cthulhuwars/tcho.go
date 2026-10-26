package cthulhuwars

import (
	"sort"

	"github.com/noderunner/bingle/internal/engine/state"
)

// The Tcho-Tcho (Onslaught Two): High Priests, Proto-Shoggoths and Ubbo-Sathla,
// whose Growth counter feeds on the Doom phases. This file holds the six
// spellbooks of the Tsang tribe — the three every tribe shares (Hierophants,
// Soulless, Terror) and Idolatry, Martyrdom and Tablets of the Gods — the
// Sycophancy ability and what the cult's requirements ask.

// hpFor says seat has High Priests: every cult with the option, and the
// Tcho-Tcho always (three of them).
func (e *eng) hpFor(seat int) bool { return e.hasHP() || e.is(seat, facTT) }

// ttHP is the Tcho-Tcho's High Priest: three of them, cost 3.
var ttHP = func() unitSpec {
	u := hpSpec
	u.Count = 3
	u.Text = "A Cultist: recruited like an Acolyte (3 Power), it gives 1 Power in Gather Power, can create and control a Gate and can be captured. The Tcho-Tcho have three. Unspeakable Oath: at the end of any player's Action, in Gather Power or in the Doom phase, sacrifice him (back to your pool) to gain 2 Power. Ubbo-Sathla is awakened by eliminating one."
	return u
}()

// --- setup --------------------------------------------------------------------------------

// glyphAreas lists the empty areas marked with a faction glyph: where the
// Tcho-Tcho may begin.
func (e *eng) glyphAreas() []int {
	var out []int
	for _, a := range e.emptyAreas() {
		if e.m.glyph[a] != "" {
			out = append(out, a)
		}
	}
	return out
}

// startGlyphAreas are the areas that hold another faction's starting glyph —
// the starting area of every cult of the game (even those not at the table),
// the sigil areas of the Windwalker and the Sleeper included.
func (e *eng) startGlyphAreas(seat int) []int {
	seen := map[int]bool{}
	var out []int
	add := func(id string) {
		if id == "" {
			return
		}
		a, ok := e.m.index[boardID(id)]
		if ok && !seen[int(a)] {
			seen[int(a)] = true
			out = append(out, int(a))
		}
	}
	if m, ok := e.game.Definition.Params["starts"].(map[string]any); ok {
		for _, v := range m {
			if s, ok := v.(string); ok {
				add(s)
			}
		}
	}
	for f := range e.m.cfg.sigils {
		for _, id := range e.m.cfg.sigils[f] {
			add(id)
		}
	}
	// The Tcho-Tcho's own start is not another faction's.
	if own := e.d.TTStart; own != "" && e.is(seat, facTT) {
		if a, ok := e.m.index[boardID(own)]; ok {
			delete(seen, int(a))
		}
	}
	sort.Ints(out)
	kept := out[:0]
	for _, a := range out {
		if seen[a] {
			kept = append(kept, a)
		}
	}
	return kept
}

// --- Sycophancy ----------------------------------------------------------------------------

// sycophants lists the seats that have Sycophancy (the Tcho-Tcho, or a
// Sleeper that borrowed it) other than the one performing the Ritual.
func (e *eng) sycophants(performer int) []int {
	var out []int
	for _, s := range e.playing() {
		if s != performer && e.ability(s, abSycophancy) {
			out = append(out, s)
		}
	}
	return out
}

// sycophancyMoves are the two answers of the player whose Ritual gave Doom
// cause: give the Tcho-Tcho 1 Doom or earn 1 fewer (impossible when the Ritual
// earned none).
func (e *eng) sycophancyMoves(p Prompt) []state.Move {
	out := []state.Move{{Action: actSyco, Piece: "give", Count: p.By}}
	if p.Left > 0 {
		out = append(out, state.Move{Action: actSyco, Piece: "less", Count: p.By})
	}
	return out
}

// sycophancy settles the answer.
func (e *eng) sycophancy(seat int, mv state.Move) {
	d := e.d
	goal := e.game.ParamInt("doom_to_win", 30)
	if mv.Piece == "less" {
		if d.Doom[seat] > 0 {
			d.Doom[seat]--
			if l := &d.DoomBy[seat]; l.Rituals > 0 {
				l.Rituals--
			}
		}
		e.say("Sycophancy: %s earns 1 Doom fewer from the Ritual (%d on the Doom track).", e.seatName(seat), d.Doom[seat])
		return
	}
	e.addDoom(mv.Count, 1, doomGifts)
	e.say("Sycophancy: %s gains 1 Doom (%d on the Doom track).", e.seatName(mv.Count), d.Doom[mv.Count])
	if d.Doom[mv.Count] >= goal {
		d.EndPending = true
	}
}

// --- Hierophants ----------------------------------------------------------------------------

// hierophants runs when seat earns a faction spellbook: a High Priest at one
// of its Gates, else Ubbo-Sathla grows by 1. first says it is the earning of
// Hierophants itself: with the High Priests option every cult places one.
func (e *eng) hierophants(seat int, first bool) {
	d := e.d
	e.placePriest(seat)
	if first && e.hasHP() {
		for _, o := range e.others(seat) {
			if e.pool(o, uHighPriest) > 0 && len(e.gatesOf(o)) > 0 {
				d.Prompts = append(d.Prompts, Prompt{Kind: "hiero", Seat: o})
			}
		}
	}
}

// placePriest queues the placing of seat's High Priest at one of its Gates, or
// grows Ubbo-Sathla when none is left in the pool (Tcho-Tcho only).
func (e *eng) placePriest(seat int) {
	d := e.d
	if e.pool(seat, uHighPriest) <= 0 {
		d.Growth++
		e.say("Hierophants: no High Priest is left, so Ubbo-Sathla's Growth counter rises to %d.", d.Growth)
		return
	}
	if len(e.gatesOf(seat)) == 0 {
		return
	}
	d.Prompts = append(d.Prompts, Prompt{Kind: "hiero", Seat: seat})
}

// hieroMoves lists the Gates seat may put its High Priest at.
func (e *eng) hieroMoves(p Prompt) []state.Move {
	if e.pool(p.Seat, uHighPriest) <= 0 {
		return nil
	}
	var out []state.Move
	for _, a := range e.gatesOf(p.Seat) {
		out = append(out, state.Move{Action: actHiero, To: e.m.ids[a]})
	}
	return out
}

// priestAt places the High Priest.
func (e *eng) priestAt(seat, a int) {
	if e.place(a, seat, uHighPriest) {
		e.say("Hierophants: %s places a High Priest at the Gate in %s.", e.seatName(seat), e.areaName(a))
	}
}

// --- Ubbo-Sathla ----------------------------------------------------------------------------

// ubboAreas lists where seat may awaken Ubbo-Sathla: at a Gate it controls,
// while it has a High Priest in play (on the map).
func (e *eng) ubboAreas(seat int) []int {
	if e.pool(seat, uUbbo) <= 0 || e.onMap(seat, uHighPriest) == 0 {
		return nil
	}
	return e.gatesOf(seat)
}

// eliminatePriest eliminates one of seat's High Priests on the map: the one
// that does not hold a Gate goes first.
func (e *eng) eliminatePriest(seat int) {
	best, any := -1, -1
	for x := range e.m.ids {
		if e.count(x, seat, uHighPriest) == 0 {
			continue
		}
		if any < 0 {
			any = x
		}
		holds := e.d.Gates[e.areaID(x)] == seat && e.holderAt(x) == uHighPriest && e.count(x, seat, uHighPriest) == 1
		if !holds && best < 0 {
			best = x
		}
	}
	if best < 0 {
		best = any
	}
	if best >= 0 {
		e.eliminate(best, seat, uHighPriest, 0)
	}
}

// awakenUbbo awakens Ubbo-Sathla in the Doom phase, for nothing: a High Priest
// is eliminated and he stands at the Gate.
func (e *eng) awakenUbbo(seat, a int) {
	e.eliminatePriest(seat)
	e.place(a, seat, uUbbo)
	e.d.markAwoke(seat, uUbbo)
	e.fulfil(seat, rqTTAwaken)
}

// hellsBanquet grows Ubbo-Sathla in the Doom phase after he was awakened.
func (e *eng) hellsBanquet() {
	d := e.d
	for _, s := range e.playing() {
		if e.is(s, facTT) && d.Awoke(s, 0) {
			n := e.die()
			d.Growth += n
			e.say("Hell's Banquet: the die shows %d, and Ubbo-Sathla's Growth counter rises to %d.", n, d.Growth)
		}
	}
}

// --- Terror, Idolatry, Martyrdom, Tablets ---------------------------------------------------

// terrorMoves: once in a battle, with Proto-Shoggoths in it, lower the enemy's
// Combat or raise one's own.
func (e *eng) terrorMoves(seat int) []state.Move {
	d := e.d
	b := d.Bat
	if b == nil || !e.is(seat, facTT) || !d.hasBook(seat, bkTerror) || b.Terror[seat] != 0 || e.battleUnits(b, seat, uProto) == 0 {
		return nil
	}
	return []state.Move{{Action: actTerror, Piece: "foe"}, {Action: actTerror, Piece: "self"}}
}

// idolatryMoves lists the areas Idolatry may call units to.
func (e *eng) idolatryMoves(seat int) []state.Move {
	d := e.d
	if !e.is(seat, facTT) || !d.hasBook(seat, bkIdolatry) {
		return nil
	}
	var out []state.Move
	for _, a := range e.startGlyphAreas(seat) {
		for _, n := range e.m.adj[a] {
			if e.total(n, seat) > 0 {
				out = append(out, state.Move{Action: actIdolatry, To: e.m.ids[a]})
				break
			}
		}
	}
	return out
}

// idolOptions lists the (area, type) stacks of seat in areas beside p.Area.
func (e *eng) idolOptions(p Prompt) []PUArea {
	a := e.areaIdx(p.Area)
	var out []PUArea
	for _, n := range e.m.adj[a] {
		for _, typ := range e.typesIn(n, p.Seat) {
			if typ == uFilth {
				continue // Filth never moves
			}
			out = append(out, PUArea{Area: n, Type: typ})
		}
	}
	return out
}

// idolMoves lists the moves of the Idolatry prompt: a stack, or everything.
func (e *eng) idolMoves(p Prompt) []state.Move {
	opts := e.idolOptions(p)
	if len(opts) == 0 {
		return nil
	}
	var out []state.Move
	for _, o := range opts {
		out = append(out, state.Move{Action: actBring, Piece: o.Type, From: e.m.ids[o.Area]})
	}
	if len(opts) > 1 {
		out = append(out, state.Move{Action: actBring, Piece: bringAll})
	}
	return append(out, state.Move{Action: actDone})
}

// idolMove calls a stack of units (or all of them) into the area.
func (e *eng) idolMove(p Prompt, mv state.Move) {
	dest := e.areaIdx(p.Area)
	var stacks []PUArea
	if mv.Piece == bringAll {
		stacks = e.idolOptions(p)
	} else {
		stacks = []PUArea{{Area: e.areaIdx(string(mv.From)), Type: mv.Piece}}
	}
	for _, s := range stacks {
		n := e.count(s.Area, p.Seat, s.Type)
		for i := 0; i < n; i++ {
			if e.take(s.Area, p.Seat, s.Type) {
				e.add(dest, p.Seat, s.Type)
			}
		}
		if n > 0 {
			e.say("Idolatry: %s's %s from %s come to %s.", e.seatName(p.Seat), plural(n, unitNameOf(s.Type), unitNameOf(s.Type)), e.areaName(s.Area), e.areaName(dest))
		}
		e.syncGate(s.Area)
	}
	e.takeAbandonedGate(dest, p.Seat)
}

// martyrdom: the High Priest of a Tcho-Tcho with Martyrdom took a Kill — every
// Kill that other units of the seat took becomes a Pain, those still to be
// assigned included.
func (e *eng) martyrdom(seat int) {
	b := e.d.Bat
	a := e.areaIdx(b.Area)
	changed := 0
	for _, f := range b.Fallen {
		if f.Seat != seat {
			continue
		}
		if e.place(a, seat, f.Type) {
			b.Pained = append(b.Pained, PU{Seat: seat, Type: f.Type})
			b.Slain[b.foeOf(seat)]--
			if b.Killed > 0 && !e.is(seat, facWW) {
				b.Killed--
			}
			changed++
		}
	}
	kept := b.Fallen[:0]
	for _, f := range b.Fallen {
		if f.Seat != seat {
			kept = append(kept, f)
		}
	}
	b.Fallen = kept
	for i := range b.Queue {
		if b.Queue[i].Seat == seat && !b.Queue[i].Pain {
			b.Queue[i].Pain = true
			changed++
		}
	}
	if changed > 0 {
		e.say("Martyrdom: the High Priest takes the Kill, and %s of %s's other units become Pains.", plural(changed, "Kill", "Kills"), e.seatName(seat))
	}
}

// tablets gives the extra Elder Signs of a Ritual — 1 per Gate at which
// seat has a High Priest — and eliminates every High Priest.
func (e *eng) tablets(seat int) {
	n := 0
	for _, a := range e.gatesOf(seat) {
		if e.count(a, seat, uHighPriest) > 0 {
			n++
		}
	}
	for i := 0; i < n; i++ {
		e.gainSign(seat)
	}
	priests := 0
	for a := range e.m.ids {
		for e.count(a, seat, uHighPriest) > 0 {
			e.eliminate(a, seat, uHighPriest, 0)
			priests++
		}
	}
	e.say("Tablets of the Gods: %s receives %s and its %s are eliminated.", e.seatName(seat), plural(n, "extra Elder Sign", "extra Elder Signs"), plural(priests, "High Priest", "High Priests"))
}

// --- the requirements ------------------------------------------------------------------------

// dismantleMoves: as an Action remove the Gate in the start area that seat
// controls.
func (e *eng) dismantleMoves(seat int) []state.Move {
	d := e.d
	if !e.is(seat, facTT) || d.reqDone(seat, rqTTGate) || d.TTStart == "" || d.Gates[d.TTStart] != seat {
		return nil
	}
	return []state.Move{{Action: actDismantle, To: boardID(d.TTStart)}}
}

// dismantle removes the Gate.
func (e *eng) dismantle(seat int, a int) {
	id := e.areaID(a)
	delete(e.d.Gates, id)
	delete(e.d.Holders, id)
	e.say("%s removes their Gate in %s.", e.seatName(seat), e.areaName(a))
	e.fulfil(seat, rqTTGate)
	e.mainTaken()
}

// noteKill is told that victim's unit of type typ was killed by killer: the
// Tcho-Tcho's requirement for a Great Old One killed in a battle, and the
// Opener's for losing a unit.
func (e *eng) noteKill(killer, victim int, typ string) {
	if unitClassOf(typ) == classGOO {
		if tt := e.seatOf(facTT); tt != 0 {
			e.fulfil(tt, rqTTKilled)
		}
		// Cthugha's requirement: an enemy Great Old One killed in a battle.
		if killer != victim && e.holdsCard(killer, uCthugha) {
			e.earnIGOOBook(killer, uCthugha)
		}
	}
	if e.is(victim, facOW) {
		e.fulfil(victim, rqOWLose)
	}
}
