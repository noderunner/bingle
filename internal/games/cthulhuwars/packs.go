package cthulhuwars

import (
	"strings"

	"github.com/noderunner/bingle/internal/engine/state"
)

// The Loyalty Cards of the Dreamlands Surface Monster Pack (Moonbeast, Gnorri,
// Shantak) and of Beyond Time and Space (Wamp, Voonith, the Hound of Tindalos).

// MoonState is a Moonbeast lying on a spellbook of an enemy: the seat that owns
// it, the seat whose spellbook it is, and that spellbook (a global number: its
// own 0–5, a neutral one bookSlots+n).
type MoonState struct {
	Owner  int `json:"owner"`
	Victim int `json:"victim"`
	Book   int `json:"book"`
}

// mooned says a Moonbeast lies on seat's spellbook: it cannot use it, although
// the book still counts for every other purpose.
func (d *Data) mooned(seat, book int) bool {
	for _, m := range d.Moon {
		if m.Victim == seat && m.Book == book {
			return true
		}
	}
	return false
}

// --- Moonbeast: Blasphemous Obeisance ----------------------------------------------------------

// moonOptions lists where owner may put a Moonbeast: a spellbook an enemy holds
// that has none on it yet.
func (e *eng) moonOptions(owner int) []state.Move {
	var out []state.Move
	for _, s := range e.others(owner) {
		for b := 0; b < bookSlots+nNeutralBooks; b++ {
			if !e.d.holds(s, b) || e.d.mooned(s, b) {
				continue
			}
			out = append(out, state.Move{Action: actMoon, Piece: e.bookSpecOf(s, b).ID, Count: s})
		}
	}
	return out
}

// moonBegin has owner place a Moonbeast (just summoned or bought) on an enemy
// spellbook: at once when only one is possible, else by a prompt.
func (e *eng) moonBegin(owner int) {
	opts := e.moonOptions(owner)
	switch len(opts) {
	case 0:
		e.say("No enemy has a spellbook to lay the Moonbeast on: it stays in %s's pool.", e.seatName(owner))
		return
	case 1:
		e.moonPlace(owner, opts[0])
		return
	}
	e.d.Prompts = append(e.d.Prompts, Prompt{Kind: "moon", Seat: owner})
}

// moonPlace lays a Moonbeast of owner on the spellbook mv names.
func (e *eng) moonPlace(owner int, mv state.Move) {
	b, ok := bookByID(mv.Piece)
	if !ok || e.pool(owner, uMoonbeast) <= 0 {
		return
	}
	e.setPool(owner, uMoonbeast, e.pool(owner, uMoonbeast)-1)
	e.d.Moon = append(e.d.Moon, MoonState{Owner: owner, Victim: mv.Count, Book: b})
	e.say("Blasphemous Obeisance: %s lays a Moonbeast on %s's spellbook %s — it cannot be used until the Moonbeast leaves (next Doom phase, or 1 Doom).",
		e.seatName(owner), e.seatName(mv.Count), e.bookSpecOf(mv.Count, b).Name)
}

// moonFree sends the Moonbeast lying on seat's spellbook b (if any) back to its
// owner's pool: the book is gone.
func (e *eng) moonFree(seat, b int) {
	for i, m := range e.d.Moon {
		if m.Victim == seat && m.Book == b {
			e.d.Moon = append(e.d.Moon[:i], e.d.Moon[i+1:]...)
			e.setPool(m.Owner, uMoonbeast, e.pool(m.Owner, uMoonbeast)+1)
			return
		}
	}
}

// moonLandMoves lists the Gates a Moonbeast may be put on: those its owner
// controls.
func (e *eng) moonLandMoves(owner int) []state.Move {
	var out []state.Move
	for _, a := range e.gatesOf(owner) {
		out = append(out, state.Move{Action: actMoon, To: e.m.ids[a]})
	}
	return out
}

// moonLand puts a Moonbeast that has left a spellbook on owner's Gate in a.
func (e *eng) moonLand(owner, a int) {
	if e.place(a, owner, uMoonbeast) {
		e.say("A Moonbeast of %s comes to the Gate in %s.", e.seatName(owner), e.areaName(a))
	}
}

// moonReturn brings the Moonbeasts home at the start of a Doom phase: each is
// put on one of its owner's Gates (a choice, when there are several).
func (e *eng) moonReturn() {
	d := e.d
	moons := d.Moon
	d.Moon = nil
	for _, m := range moons {
		e.setPool(m.Owner, uMoonbeast, e.pool(m.Owner, uMoonbeast)+1)
		e.say("The Moonbeast of %s leaves %s's spellbook %s.", e.seatName(m.Owner), e.seatName(m.Victim), e.bookSpecOf(m.Victim, m.Book).Name)
		d.Prompts = append(d.Prompts, Prompt{Kind: "moonland", Seat: m.Owner})
	}
}

// ransomMoves lists the Moonbeasts seat may buy off for 1 Doom: any time, no
// Action.
func (e *eng) ransomMoves(seat int) []state.Move {
	if e.d.Doom[seat] < 1 {
		return nil
	}
	var out []state.Move
	for _, m := range e.d.Moon {
		if m.Victim == seat {
			out = append(out, state.Move{Action: actRansom, Piece: e.bookSpecOf(seat, m.Book).ID})
		}
	}
	return out
}

// ransom pays 1 Doom to free the spellbook: its Moonbeast goes back to the map.
func (e *eng) ransom(seat int, mv state.Move) {
	b, ok := bookByID(mv.Piece)
	if !ok {
		return
	}
	for i, m := range e.d.Moon {
		if m.Victim != seat || m.Book != b {
			continue
		}
		e.d.Moon = append(e.d.Moon[:i], e.d.Moon[i+1:]...)
		e.setPool(m.Owner, uMoonbeast, e.pool(m.Owner, uMoonbeast)+1)
		e.d.Doom[seat]--
		e.say("%s pays 1 Doom: the Moonbeast leaves %s (Doom now %d).", e.seatName(seat), e.bookSpecOf(seat, b).Name, e.d.Doom[seat])
		e.d.Prompts = append(e.d.Prompts, Prompt{Kind: "moonland", Seat: m.Owner})
		return
	}
}

// --- Gnorri: Grottos -----------------------------------------------------------------------------

// grottos is the extra Doom a seat's Gnorri earn in the Doom phase: 1 for two in
// play, 2 for three.
func (e *eng) grottos(seat int) int {
	if !e.holdsCard(seat, uGnorri) {
		return 0
	}
	switch n := e.onMap(seat, uGnorri); {
	case n >= 3:
		return 2
	case n == 2:
		return 1
	}
	return 0
}

// --- Shantak: Horror Steed --------------------------------------------------------------------------

// reachFor lists the areas a unit of type typ of seat may move to from a: its
// cult's reach, or any area for a Shantak.
func (e *eng) reachFor(seat, a int, typ string) []int {
	if typ == uShantak {
		out := make([]int, 0, len(e.m.ids)-1)
		for b := range e.m.ids {
			if b != a {
				out = append(out, b)
			}
		}
		return out
	}
	return e.reach(seat, a)
}

// carried says a Cultist may ride a Shantak's step from → to for nothing, and
// where in the Move's list of free seats that is.
func (e *eng) carried(mv state.Move) int {
	m := e.d.Move
	if m == nil || !isCultist(mv.Piece) {
		return -1
	}
	key := string(mv.From) + ">" + string(mv.To)
	for i, c := range m.Carry {
		if c == key {
			return i
		}
	}
	return -1
}

// --- Wamp: Crypt Dweller ----------------------------------------------------------------------------

// gatelessAreas lists the areas with no Gate: where Wamps may be placed.
func (e *eng) gatelessAreas() []int {
	var out []int
	for a := range e.m.ids {
		if !e.hasGate(a) {
			out = append(out, a)
		}
	}
	return out
}

// cryptMoves are the enemies owner may choose to place its Wamps (Doom phase).
func (e *eng) cryptMoves(owner int) []state.Move {
	if e.pool(owner, uWamp) <= 0 || len(e.gatelessAreas()) == 0 {
		return nil
	}
	var out []state.Move
	for _, s := range e.others(owner) {
		out = append(out, state.Move{Action: actCrypt, Count: s})
	}
	return out
}

// cryptBegin has owner choose the enemy that places its Wamps: at once when
// there is one enemy.
func (e *eng) cryptBegin(owner int) {
	opts := e.cryptMoves(owner)
	switch len(opts) {
	case 0:
		return
	case 1:
		e.cryptChoose(owner, opts[0].Count)
		return
	}
	e.d.Prompts = append(e.d.Prompts, Prompt{Kind: "crypt-pick", Seat: owner})
}

// cryptChoose hands the placing of owner's Wamps to the enemy.
func (e *eng) cryptChoose(owner, enemy int) {
	n := e.pool(owner, uWamp)
	if n <= 0 {
		return
	}
	e.say("Crypt Dweller: %s chooses %s to place %s.", e.seatName(owner), e.seatName(enemy), plural(n, "Wamp", "Wamps"))
	e.d.Prompts = append(e.d.Prompts, Prompt{Kind: "crypt", Seat: enemy, By: owner, Left: n})
}

// cryptPlaceMoves are the areas the enemy may put the next Wamp in.
func (e *eng) cryptPlaceMoves(p Prompt) []state.Move {
	var out []state.Move
	if e.pool(p.By, uWamp) <= 0 {
		return nil
	}
	for _, a := range e.gatelessAreas() {
		out = append(out, state.Move{Action: actCrypt, To: e.m.ids[a]})
	}
	return out
}

// cryptPlace puts one of By's Wamps in the area.
func (e *eng) cryptPlace(p Prompt, a int) {
	if e.place(a, p.By, uWamp) {
		e.say("%s places a Wamp of %s in %s.", e.seatName(p.Seat), e.seatName(p.By), e.areaName(a))
	}
	e.d.Prompts[0].Left--
	if e.d.Prompts[0].Left <= 0 || e.pool(p.By, uWamp) <= 0 {
		e.dropPrompt()
	}
}

// --- the Hound of Tindalos ----------------------------------------------------------------------------

// houndHops lists the free moves of seat's Hound in the Move under way: it
// jumps from a Gate to any other Gate once another unit has moved.
func (e *eng) houndHops(seat int) []state.Move {
	d := e.d
	if d.Move == nil || !e.holdsCard(seat, uHound) || e.onMap(seat, uHound) == 0 {
		return nil
	}
	others := false
	arrived := map[string]bool{}
	for _, k := range d.Move.Moved {
		area, piece, _ := strings.Cut(k, "|")
		if piece == uHound {
			arrived[area] = true
		} else {
			others = true
		}
	}
	if !others {
		return nil
	}
	var out []state.Move
	for a := range e.m.ids {
		if e.count(a, seat, uHound) == 0 || !e.hasGate(a) || arrived[e.areaID(a)] {
			continue
		}
		if d.Move.Origin != "" && e.areaID(a) != d.Move.Origin {
			continue
		}
		for b := range e.m.ids {
			if b != a && e.hasGate(b) {
				out = append(out, state.Move{Action: actGo, Piece: uHound, From: e.m.ids[a], To: e.m.ids[b]})
			}
		}
	}
	return out
}

// angles is Angles of Time: a Hound in an area without a Gate is eliminated.
func (e *eng) angles() {
	for _, s := range e.playing() {
		if !e.holdsCard(s, uHound) {
			continue
		}
		for a := range e.m.ids {
			if e.count(a, s, uHound) > 0 && !e.hasGate(a) {
				e.say("Angles of Time: the Hound of Tindalos of %s is lost in %s, where no Gate stands.", e.seatName(s), e.areaName(a))
				for e.count(a, s, uHound) > 0 {
					e.eliminate(a, s, uHound, 0)
				}
			}
		}
	}
}
