package cthulhuwars

import (
	"encoding/json"

	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Bubastis: the cats on the Moon. It has no Cultists. Its only Gate is the Moon,
// an area added to the map for it, which its units reach from anywhere and
// everybody else may only leave. Every unit it has in play gives 1 Power.

const moonArea = "the-moon"

const (
	uEarthCat = "earth-cat"
	uMars     = "cat-from-mars"
	uSaturn   = "cat-from-saturn"
	uUranus   = "cat-from-uranus"
	uBastet   = "bastet"
)

// The spellbooks of Bubastis, in the order of the card.
const (
	bkAilurophobia = iota
	bkCatabolism
	bkCatnapping
	bkPredator
	bkSavagery
	bkZagazig
)

// The requirements of Bubastis.
const (
	rqBUPay = iota
	rqBUMoonEmpty
	rqBUEverywhere
	rqBUMarsSaturn
	rqBUUranus
	rqBUAwaken
)

var bubastisFaction = faction{
	Key: "bu", Name: "Bubastis", Color: "sand", Seat: 10, Start: moonArea,
	Ability: "Lunacy: Bubastis has no Cultists and cannot create or control a Gate. The Moon is its Gate, and every unit it has in play gives 1 Power.",
	Units: []unitSpec{
		{ID: uEarthCat, Name: "Earth Cat", Class: classMonster, Count: 6, Cost: 1, Combat: 0, Text: "Gives 1 Power in Gather Power. Catabolism (spellbook): Earth Cats can bring Monsters into play where they stand."},
		{ID: uMars, Name: "Cat from Mars", Class: classMonster, Count: 2, Cost: 2, Combat: 1, Text: "Gives 1 Power in Gather Power. Zagazig (spellbook): in its battle your rolled Pains become Kills and your rolled Kills become Pains."},
		{ID: uSaturn, Name: "Cat from Saturn", Class: classMonster, Count: 2, Cost: 3, Combat: 2, Text: "Gives 1 Power in Gather Power. Savagery (spellbook): pay 1 Power before a battle for +4 Combat each."},
		{ID: uUranus, Name: "Cat from Uranus", Class: classMonster, Count: 2, Cost: 4, Combat: 3, Text: "Gives 1 Power in Gather Power. Predator (spellbook): an enemy who lost a unit in its battle must eliminate another of that type."},
		{ID: uBastet, Name: "Bastet", Class: classGOO, Count: 1, Cost: 6, Combat: 0, Text: "An Elder God. She rolls no dice: she adds 1 Kill to your total, and the enemy's total is 1 Kill lower.",
			Abilities: []abilitySpec{{Name: "Requires Attention", Timing: "Doom Phase", Text: "During the Doom Phase, if Bastet is in an Area containing an enemy Cultist, you may perform a Ritual of Annihilation that adds exactly 4 Doom points. If her Area has an Enemy-Controlled Gate, gain 1 Elder Sign. If it has an Enemy Great Old One, gain 2 Elder Signs."}}},
	},
	Books: []bookSpec{
		{ID: "ailurophobia", Name: "Ailurophobia", Timing: "Doom Phase", Text: "Gain 1 Doom per Faction Monster Cat Variety sharing non-Moon Area(s) with Unit(s) from enemy Factions."},
		{ID: "catabolism", Name: "Catabolism", Timing: "Ongoing", Text: "Earth Cats can Recruit monsters, as if the monsters were cultists, instead of needing to Summon them.", Units: []string{uEarthCat}},
		{ID: "catnapping", Name: "Catnapping", Timing: "Action: Cost 1", Text: "Move all units (except Bastet) belonging to any faction in Bastet's Area from that Area to the Moon. When an enemy unit on the Moon leaves, you gain the Power they spend on movement.", Units: []string{uBastet}},
		{ID: "predator", Name: "Predator", Timing: "Post-Battle", Text: "If a Cat from Uranus was in the battle, you can select one unit lost by the enemy. He must eliminate a second unit of that type anywhere on the map, if possible.", Units: []string{uUranus}},
		{ID: "savagery", Name: "Savagery", Timing: "Pre-Battle", Text: "Pay 1 Power in Pre-Battle to increase the combat of all Cats from Saturn by 4 for this battle.", Units: []string{uSaturn}},
		{ID: "zagazig", Name: "Zagazig", Timing: "Pre-Battle", Text: "If a Cat from Mars is in the battle, all rolled Pains become Kills, and all rolled Kills become Pains.", Units: []string{uMars}},
	},
	Reqs: []reqSpec{
		{ID: "pay", Text: "As your Action, Pay 2 Power."},
		{ID: "moon-empty", Text: "No Earth Cats are on the Moon."},
		{ID: "everywhere", Text: "A Cat is in every enemy Faction's Start Area; gain 1 Power per enemy Faction."},
		{ID: "mars-saturn", Text: "A Cat from Mars or Saturn is Killed or Eliminated."},
		{ID: "uranus", Text: "A Cat from Uranus is Killed or Eliminated."},
		{ID: "awaken", Text: "Awaken Bastet."},
	},
}

// moonPosition is the area added to the map when Bubastis is at the table: a
// disc in a corner of the board.
func moonPosition(width float64) map[string]any {
	const r = 34.0
	cx, cy := width-r-6, r+6
	var poly [][]float64
	for _, o := range [][]float64{{0, -1}, {.7, -.7}, {1, 0}, {.7, .7}, {0, 1}, {-.7, .7}, {-1, 0}, {-.7, -.7}} {
		poly = append(poly, []float64{cx + o[0]*r, cy + o[1]*r})
	}
	return map[string]any{"id": moonArea, "name": "The Moon", "tags": []any{"land", "c-ice", "moon"}, "anchor": []float64{cx, cy}, "polygons": [][][]float64{poly}}
}

// addMoonToPatch puts the Moon in a derived game's board.
func addMoonToPatch(base *dsl.GameDefinition, patch map[string]any) error {
	raw, err := json.Marshal(base.Board.Positions)
	if err != nil {
		return err
	}
	var positions []any
	if err := json.Unmarshal(raw, &positions); err != nil {
		return err
	}
	width := 1000.0
	if base.Board.Canvas != nil && base.Board.Canvas.Width > 0 {
		width = float64(base.Board.Canvas.Width)
	}
	positions = append(positions, moonPosition(width))
	board, _ := patch["board"].(map[string]any)
	if board == nil {
		board = map[string]any{}
	}
	board["positions"] = positions
	patch["board"] = board
	return nil
}

// isMoon says a is the Moon.
func (e *eng) isMoon(a int) bool { return e.m.ids[a] == moonArea }

// moon is the index of the Moon, if it is on this map.
func (e *eng) moon() (int, bool) {
	if a, ok := e.m.index[moonArea]; ok {
		return a, true
	}
	return 0, false
}

// bubastisIn says seat is Bubastis.
func (e *eng) bubastisIn(seat int) bool { return e.is(seat, facBU) }

// reachWithMoon adds the Moon's rule to where a unit may go from a: from the Moon
// anyone may go anywhere, and only Bubastis's units may go to it.
func (e *eng) reachWithMoon(seat, a int, list []int) []int {
	m, ok := e.moon()
	if !ok {
		return list
	}
	if a == m {
		out := make([]int, 0, len(e.m.ids))
		for n := range e.m.ids {
			if n != m {
				out = append(out, n)
			}
		}
		return out
	}
	if e.bubastisIn(seat) {
		return append(append([]int(nil), list...), m)
	}
	return list
}

// catsInPlay counts Bubastis's Monsters on the map: each gives 1 Power.
func (e *eng) catsInPlay(seat int) int {
	if !e.bubastisIn(seat) {
		return 0
	}
	return e.onMap(seat, uEarthCat) + e.onMap(seat, uMars) + e.onMap(seat, uSaturn) + e.onMap(seat, uUranus)
}

// bubastisTurnMoves lists the Actions only Bubastis has.
func (e *eng) bubastisTurnMoves(seat int) []state.Move {
	d := e.d
	if !e.bubastisIn(seat) {
		return nil
	}
	var out []state.Move
	if !d.reqDone(seat, rqBUPay) && d.Power[seat] >= 2 {
		out = append(out, state.Move{Action: actCard, Piece: "bu-pay"})
	}
	if a, ok := e.gooArea(seat, uBastet); ok && d.hasBook(seat, bkCatnapping) && !e.isMoon(a) && d.Power[seat] >= 1 && e.catnapTargets(a, seat) > 0 {
		out = append(out, state.Move{Action: actCard, Piece: "catnap"})
	}
	return out
}

func (e *eng) catnapTargets(a, seat int) int {
	n := e.total(a, seat) - e.count(a, seat, uBastet)
	return n + e.enemiesIn(a, seat)
}

// catnap sends everyone in Bastet's area but her to the Moon.
func (e *eng) catnap(seat int) {
	a, ok := e.gooArea(seat, uBastet)
	m, moon := e.moon()
	if !ok || !moon {
		return
	}
	e.say("Catnapping: %s sends everyone in %s to the Moon.", e.seatName(seat), e.areaName(a))
	for _, s := range e.playing() {
		for _, typ := range e.typesIn(a, s) {
			if typ == uBastet {
				continue
			}
			for e.count(a, s, typ) > 0 && e.take(a, s, typ) {
				e.add(m, s, typ)
			}
		}
	}
	e.syncGate(a)
}

// moonToll pays Bubastis the Power an enemy spends on moving off the Moon.
func (e *eng) moonToll(seat int, from string, paid int) {
	s := e.seatOf(facBU)
	if s == 0 || s == seat || from != moonArea || paid <= 0 || !e.d.hasBook(s, bkCatnapping) {
		return
	}
	e.d.Power[s] += paid
	e.say("Catnapping: %s gains the %s %s spent leaving the Moon.", e.seatName(s), plural(paid, "Power", "Power"), e.seatName(seat))
}

// bubastisSweep meets the requirements that are states of the map, and those
// marked by cats that fell.
func (e *eng) bubastisSweep() {
	d := e.d
	s := e.seatOf(facBU)
	if s == 0 || d.Phase == phaseOver {
		return
	}
	later := func(req int) {
		n := len(d.Prompts)
		if e.fulfil(s, req) && len(d.Prompts) > n {
			d.Pending = append(d.Pending, d.Prompts[n:]...)
			d.Prompts = d.Prompts[:n]
		}
	}
	if m, ok := e.moon(); ok && !d.reqDone(s, rqBUMoonEmpty) && e.count(m, s, uEarthCat) == 0 && d.Round >= 1 {
		later(rqBUMoonEmpty)
	}
	if d.CatsFell&1 != 0 && !d.reqDone(s, rqBUMarsSaturn) {
		later(rqBUMarsSaturn)
	}
	if d.CatsFell&2 != 0 && !d.reqDone(s, rqBUUranus) {
		later(rqBUUranus)
	}
	if !d.reqDone(s, rqBUEverywhere) {
		all, enemies := true, 0
		for _, o := range e.others(s) {
			enemies++
			a, fixed := e.startAreaOf(o)
			if !fixed || e.catsIn(a, s) == 0 {
				all = false
				break
			}
		}
		if all && enemies > 0 {
			d.Power[s] += enemies
			later(rqBUEverywhere)
		}
	}
}

func (e *eng) catsIn(a, seat int) int {
	return e.count(a, seat, uEarthCat) + e.count(a, seat, uMars) + e.count(a, seat, uSaturn) + e.count(a, seat, uUranus)
}

// ailurophobia pays Doom per variety of cat beside an enemy, off the Moon.
func (e *eng) ailurophobia() {
	d := e.d
	s := e.seatOf(facBU)
	if s == 0 || !d.hasBook(s, bkAilurophobia) {
		return
	}
	n := 0
	for _, typ := range []string{uEarthCat, uMars, uSaturn, uUranus} {
		for a := range e.m.ids {
			if !e.isMoon(a) && e.count(a, s, typ) > 0 && e.enemiesIn(a, s) > 0 {
				n++
				break
			}
		}
	}
	if n > 0 {
		e.addDoom(s, n, doomCards)
		e.say("Ailurophobia: %s gains %d Doom.", e.seatName(s), n)
	}
}

// bastetRitualArea says Bastet may perform her Ritual: an enemy Cultist shares her area.
func (e *eng) bastetRitualArea(seat int) (int, bool) {
	a, ok := e.gooArea(seat, uBastet)
	if !ok || !e.bubastisIn(seat) {
		return 0, false
	}
	for _, o := range e.others(seat) {
		if e.cultists(a, o) > 0 {
			return a, true
		}
	}
	return 0, false
}

// bastetRitual is the Ritual of Requires Attention: exactly 4 Doom, and Elder Signs
// for what shares her area.
func (e *eng) bastetRitual(seat int) {
	d := e.d
	a, ok := e.bastetRitualArea(seat)
	if !ok {
		return
	}
	d.Power[seat] -= e.ritualCostFor(seat)
	e.addDoom(seat, 4, doomRituals)
	signs := 0
	if ctl := d.Gates[e.areaID(a)]; ctl != 0 && ctl != seat {
		signs++
	}
	for _, o := range e.others(seat) {
		if e.classCount(a, o, classGOO) > 0 {
			signs += 2
			break
		}
	}
	e.say("Requires Attention: %s performs a Ritual for 4 Doom and %s.", e.seatName(seat), plural(signs, "Elder Sign", "Elder Signs"))
	for i := 0; i < signs; i++ {
		e.gainSign(seat)
	}
	e.advanceRitual()
}

// predatorMoves lists the unit types lost by the enemy that Predator can hit.
func (e *eng) predatorMoves(seat int) []state.Move {
	d := e.d
	b := d.Bat
	if !e.bubastisIn(seat) || !d.hasBook(seat, bkPredator) || b == nil || !b.Uranus || b.Preyed {
		return nil
	}
	foe := b.foeOf(seat)
	seen := map[string]bool{}
	var out []state.Move
	for _, u := range b.Lost {
		if u.Seat == foe && !seen[u.Type] && e.onMap(foe, u.Type) > 0 && !isIndependent(u.Type) {
			seen[u.Type] = true
			out = append(out, state.Move{Action: actCard, Piece: "predator." + u.Type})
		}
	}
	return out
}

// predator eliminates a second unit of the type, anywhere.
func (e *eng) predator(seat int, typ string) {
	b := e.d.Bat
	b.Preyed = true
	foe := b.foeOf(seat)
	for a := range e.m.ids {
		if e.count(a, foe, typ) > 0 {
			e.say("Predator: %s must eliminate another %s, in %s.", e.seatName(foe), unitNameOf(typ), e.areaName(a))
			e.eliminate(a, foe, typ, seat)
			return
		}
	}
}

// catFell notes a cat of Bubastis lost, for the requirements.
func (e *eng) catFell(seat int, typ string) {
	if !e.bubastisIn(seat) {
		return
	}
	switch typ {
	case uMars, uSaturn:
		e.d.CatsFell |= 1
	case uUranus:
		e.d.CatsFell |= 2
	}
}
