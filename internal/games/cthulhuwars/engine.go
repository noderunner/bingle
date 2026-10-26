package cthulhuwars

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// mapData is the world map in integer form.
type mapData struct {
	ids   []board.PositionID
	index map[board.PositionID]int
	names []string
	adj   [][]int
	// reach2 lists the areas up to two steps away (Crawling Chaos' Flight),
	// excluding the area itself.
	reach2 [][]int
	ocean  []bool
	glyph  []string
	// cfg is the table: who sits where, which expansion is in the box.
	cfg *gameCfg
	// sigil lists, per faction, the areas marked with its sigil (indexes).
	sigil [nFactions][]int
}

var mapCache sync.Map // *catalog.CompiledGame → *mapData

func mapOf(game *catalog.CompiledGame) *mapData {
	if v, ok := mapCache.Load(game); ok {
		return v.(*mapData)
	}
	m := buildMap(game)
	v, _ := mapCache.LoadOrStore(game, m)
	return v.(*mapData)
}

func buildMap(game *catalog.CompiledGame) *mapData {
	b := game.Board
	ids := b.Positions()
	m := &mapData{ids: ids, index: map[board.PositionID]int{}}
	for i, id := range ids {
		m.index[id] = i
	}
	n := len(ids)
	m.names = make([]string, n)
	m.adj = make([][]int, n)
	m.reach2 = make([][]int, n)
	m.ocean = make([]bool, n)
	m.glyph = make([]string, n)
	m.cfg = buildCfg(game.Definition)
	tags := map[string][]string{}
	names := map[string]string{}
	for _, p := range game.Definition.Board.Positions {
		tags[p.ID] = p.Tags
		names[p.ID] = p.Name
	}
	for i, id := range ids {
		m.names[i] = names[string(id)]
		if m.names[i] == "" {
			m.names[i] = string(id)
		}
		for _, nb := range b.Neighbors(id) {
			m.adj[i] = append(m.adj[i], m.index[nb])
		}
		sort.Ints(m.adj[i])
		for _, t := range tags[string(id)] {
			switch {
			case t == "ocean":
				m.ocean[i] = true
			case strings.HasPrefix(t, "glyph-"):
				m.glyph[i] = strings.TrimPrefix(t, "glyph-")
			}
		}
	}
	for i := range ids {
		seen := map[int]bool{i: true}
		var list []int
		for _, nb := range m.adj[i] {
			if !seen[nb] {
				seen[nb] = true
				list = append(list, nb)
			}
		}
		for _, nb := range m.adj[i] {
			for _, n2 := range m.adj[nb] {
				if !seen[n2] {
					seen[n2] = true
					list = append(list, n2)
				}
			}
		}
		sort.Ints(list)
		m.reach2[i] = list
	}
	for f := range m.cfg.sigils {
		for _, id := range m.cfg.sigils[f] {
			if i, ok := m.index[board.PositionID(id)]; ok {
				m.sigil[f] = append(m.sigil[f], i)
			}
		}
		sort.Ints(m.sigil[f])
	}
	return m
}

// eng is a working view of one position: the data, the units per area and
// the pools. Apply works on a clone it owns and mutates through eng.
type eng struct {
	game *catalog.CompiledGame
	m    *mapData
	st   *state.BoardState
	d    *Data
	ev   []string
	// n is the number of players at the table.
	n int
	// routed is set once a flow has handed over to the prompt machinery
	// (which continues the turn itself), so callers must not continue it.
	routed bool
	// cnt[(area*(nSeats+1)+seat)*nT + type] is how many such units stand in
	// the area.
	cnt []int
}

var nT = len(unitTypes)

func (rs *Ruleset) newEng(game *catalog.CompiledGame, st *state.BoardState, d *Data) *eng {
	e := &eng{game: game, m: mapOf(game), st: st, d: d}
	e.n = e.m.cfg.n
	switch {
	case d.Players > 0 && d.Players <= e.m.cfg.n:
		e.n = d.Players
	case d.Players == 0 && game.Definition.ID == "cthulhu-wars-5p" && e.m.cfg.n == 5 && d.Round > 0:
		// Positions saved before the 5-player map had its fifth seat.
		e.n = 4
	}
	e.cnt = make([]int, len(e.m.ids)*(nSeats+1)*nT)
	for _, id := range st.Occupied() {
		a, ok := e.m.index[id]
		if !ok {
			continue
		}
		for _, o := range st.Stack(id) {
			seat := e.seatByColor(o.Color)
			t, ok := unitTypeIndex[o.Piece]
			if seat == 0 || !ok {
				continue
			}
			e.cnt[e.idx(a, seat, t)]++
		}
	}
	return e
}

func (e *eng) idx(area, seat, t int) int { return (area*(nSeats+1)+seat)*nT + t }

func (e *eng) areaIdx(id string) int { return e.m.index[board.PositionID(id)] }

func (e *eng) areaID(a int) string { return string(e.m.ids[a]) }

func (e *eng) areaName(a int) string { return e.m.names[a] }

// count is how many units of type typ seat has in area a.
func (e *eng) count(a, seat int, typ string) int {
	t, ok := unitTypeIndex[typ]
	if !ok {
		return 0
	}
	return e.cnt[e.idx(a, seat, t)]
}

// total is how many units of seat stand in area a; cultists, monsters and
// Great Old Ones counted separately by classCount.
func (e *eng) total(a, seat int) int {
	n := 0
	for t := range unitTypes {
		n += e.cnt[e.idx(a, seat, t)]
	}
	return n
}

func (e *eng) classCount(a, seat, class int) int {
	n := 0
	base := e.idx(a, seat, 0)
	for t := range unitTypes {
		if unitClassByType[t] == class {
			n += e.cnt[base+t]
		}
	}
	return n
}

// typesIn lists the unit types seat has in area a (stable order).
func (e *eng) typesIn(a, seat int) []string {
	var out []string
	for t, id := range unitTypes {
		if e.cnt[e.idx(a, seat, t)] > 0 {
			out = append(out, id)
		}
	}
	return out
}

// onMap is how many units of type typ seat has on the map.
func (e *eng) onMap(seat int, typ string) int {
	n := 0
	for a := range e.m.ids {
		n += e.count(a, seat, typ)
	}
	return n
}

func (e *eng) pool(seat int, typ string) int { return e.st.Reserve(e.colorOf(seat), typ) }

func (e *eng) setPool(seat int, typ string, n int) { e.st.SetReserve(e.colorOf(seat), typ, n) }

// add puts a unit of seat in area a (not touching the pool).
func (e *eng) add(a, seat int, typ string) {
	t := unitTypeIndex[typ]
	e.cnt[e.idx(a, seat, t)]++
	e.st.Push(e.m.ids[a], state.Occupant{Piece: typ, Color: e.colorOf(seat)})
}

// take removes one unit of seat from area a (not touching the pool).
func (e *eng) take(a, seat int, typ string) bool {
	t := unitTypeIndex[typ]
	i := e.idx(a, seat, t)
	if e.cnt[i] == 0 {
		return false
	}
	e.cnt[i]--
	id := e.m.ids[a]
	stack := e.st.Stack(id)
	color := e.colorOf(seat)
	for k := len(stack) - 1; k >= 0; k-- {
		if stack[k].Piece == typ && stack[k].Color == color {
			stack = append(stack[:k], stack[k+1:]...)
			break
		}
	}
	e.st.SetStack(id, stack)
	return true
}

// place puts a unit from seat's pool into area a.
func (e *eng) place(a, seat int, typ string) bool {
	if e.pool(seat, typ) <= 0 {
		return false
	}
	e.setPool(seat, typ, e.pool(seat, typ)-1)
	e.add(a, seat, typ)
	return true
}

// maxLog bounds the entries kept in a position's event log.
const maxLog = 64

// logDecision appends what the decision of seat at ply did to the event log
// and drops what every seat has already seen. A player reads what happened
// since their own last move — the last decision of theirs that someone else's
// decision followed — so nothing older than the oldest of those anchors is
// needed (and never more than maxLog entries). A seat with no such decision
// has not been through a full round yet and keeps everything.
func (e *eng) logDecision(ply, seat int) {
	d := e.d
	d.Log = append(d.Log, LogEntry{Ply: ply, Seat: seat, Lines: append([]string(nil), e.ev...)})
	anchor := map[int]int{}
	later := 0 // bit set of the seats that decided after the entry looked at
	for i := len(d.Log) - 1; i >= 0; i-- {
		le := d.Log[i]
		if later&^(1<<uint(le.Seat)) != 0 {
			if _, ok := anchor[le.Seat]; !ok {
				anchor[le.Seat] = le.Ply
			}
		}
		later |= 1 << uint(le.Seat)
	}
	cut := ply
	for s := 1; s <= nSeats; s++ {
		if !e.active(s) {
			continue
		}
		a, ok := anchor[s]
		if !ok {
			cut = 0
			break
		}
		if a < cut {
			cut = a
		}
	}
	keep := d.Log[:0:0]
	for _, le := range d.Log {
		if le.Ply >= cut {
			keep = append(keep, le)
		}
	}
	if len(keep) > maxLog {
		keep = keep[len(keep)-maxLog:]
	}
	d.Log = keep
}

// say records an event line.
func (e *eng) say(format string, args ...any) {
	e.ev = append(e.ev, fmt.Sprintf(format, args...))
}

// seatName names the faction in seat.
func (e *eng) seatName(seat int) string {
	if f := e.fac(seat); f >= 0 {
		return factions[f].Name
	}
	return fmt.Sprintf("Seat %d", seat)
}

// --- the table ----------------------------------------------------------------

// fac is the faction index in seat, -1 for a seat nobody sits in.
func (e *eng) fac(seat int) int {
	if seat < 1 || seat > nSeats {
		return -1
	}
	return e.m.cfg.fac[seat]
}

// f is the faction in seat (a seat that is at the table).
func (e *eng) f(seat int) *faction { return &factions[e.fac(seat)] }

// is reports whether seat plays faction f.
func (e *eng) is(seat, f int) bool { return e.fac(seat) == f }

// seatOf is the seat of faction f, 0 when it is not at the table.
func (e *eng) seatOf(f int) int {
	if f < 0 || f >= nFactions {
		return 0
	}
	if s := e.m.cfg.seatOf[f]; s >= 1 && s <= e.n {
		return s
	}
	return 0
}

// colorOf is the color of the pieces of seat.
func (e *eng) colorOf(seat int) string {
	if f := e.fac(seat); f >= 0 {
		return factions[f].Color
	}
	return "white"
}

// seatByColor is the seat that plays color, 0 for none.
func (e *eng) seatByColor(color string) int {
	f := factionByColor(color)
	if f < 0 {
		return 0
	}
	return e.seatOf(f)
}

// hasHP says the High Priests are in the box.
func (e *eng) hasHP() bool { return e.m.cfg.hp }

// units lists the kinds of unit in seat's box: the faction's own, and its High
// Priest when the expansion is in play.
func (e *eng) units(seat int) []unitSpec {
	f := e.f(seat)
	out := f.Units
	switch {
	case e.is(seat, facTT):
		out = append(append([]unitSpec(nil), out...), ttHP)
	case e.hasHP():
		out = append(append([]unitSpec(nil), out...), hpSpec)
	}
	if own := e.ownedCards(seat); len(own) > 0 {
		out = append([]unitSpec(nil), out...)
		for _, c := range own {
			out = append(out, c.Unit)
			// Abhoth brings the Filth Tokens.
			if c.Unit.ID == uAbhoth {
				out = append(out, filthSpec)
			}
		}
	}
	return out
}

// cultists counts seat's Cultists (Acolytes and its High Priest) in area a.
func (e *eng) cultists(a, seat int) int {
	return e.count(a, seat, uCultist) + e.count(a, seat, uHighPriest)
}

// --- gates ---------------------------------------------------------------

// canControlGate reports whether seat has a unit in a that can control a Gate.
func (e *eng) canControlGate(seat, a int) bool {
	return e.pickHolder(a, seat) != ""
}

// holderTypes lists, in order of preference, the unit types seat can occupy a
// Gate with: its Acolytes, its High Priest and — with The Red Sign — the Dark
// Young of the Black Goat.
func (e *eng) holderTypes(seat int) []string {
	out := []string{uCultist}
	if e.hpFor(seat) {
		out = append(out, uHighPriest)
	}
	if e.is(seat, facBG) && e.d.hasBook(seat, bkRedSign) {
		out = append(out, uYoung)
	}
	return out
}

// pickHolder is the unit type seat would put on the Gate of area a: the first
// of its holder types that stands there ("" when none does).
func (e *eng) pickHolder(a, seat int) string {
	for _, typ := range e.holderTypes(seat) {
		if e.count(a, seat, typ) > 0 {
			return typ
		}
	}
	return ""
}

// holderAt is the type of the unit occupying the Gate of area a ("" for an
// abandoned Gate or none): the recorded one, or — for a position saved before
// Gates were occupied — the one its controller would choose.
func (e *eng) holderAt(a int) string {
	id := e.areaID(a)
	ctl, ok := e.d.Gates[id]
	if !ok || ctl == 0 {
		return ""
	}
	if t := e.d.Holders[id]; t != "" && e.count(a, ctl, t) > 0 {
		return t
	}
	return e.pickHolder(a, ctl)
}

// occupy puts a unit of seat on the Gate of area a, giving the Gate to seat:
// typ when it stands there, else the unit seat would choose.
func (e *eng) occupy(a, seat int, typ string) {
	id := e.areaID(a)
	e.d.Gates[id] = seat
	if typ == "" || e.count(a, seat, typ) == 0 {
		typ = e.pickHolder(a, seat)
	}
	if e.d.Holders == nil {
		e.d.Holders = map[string]string{}
	}
	if typ == "" {
		delete(e.d.Holders, id)
		return
	}
	e.d.Holders[id] = typ
}

// vacate takes the occupant off the Gate of area a and leaves it abandoned.
func (e *eng) vacate(a int) {
	id := e.areaID(a)
	if _, ok := e.d.Gates[id]; ok {
		e.d.Gates[id] = 0
	}
	delete(e.d.Holders, id)
}

// syncGate keeps the occupant of the Gate of area a in step with the units
// that stand there: when the figure on the Gate is gone another unit of its
// controller takes its place, and a Gate nobody can hold is abandoned.
func (e *eng) syncGate(a int) {
	id := e.areaID(a)
	ctl, ok := e.d.Gates[id]
	if !ok {
		delete(e.d.Holders, id)
		return
	}
	if ctl == 0 {
		delete(e.d.Holders, id)
		return
	}
	if t := e.d.Holders[id]; t != "" && e.count(a, ctl, t) > 0 && e.holdsGates(ctl, t) {
		return
	}
	if t := e.pickHolder(a, ctl); t != "" {
		if e.d.Holders == nil {
			e.d.Holders = map[string]string{}
		}
		e.d.Holders[id] = t
		return
	}
	e.vacate(a)
	e.say("The Gate in %s is abandoned.", e.areaName(a))
}

// holdsGates says units of type typ can occupy seat's Gates.
func (e *eng) holdsGates(seat int, typ string) bool {
	return containsStr(e.holderTypes(seat), typ)
}

// takeAbandonedGate gives seat the Gate of area a when it stands abandoned and
// a unit of seat that can control it has just arrived: taking control is a
// free action nobody declines, so it happens by itself. A Gate the active
// player gave up this turn stays abandoned until the next one.
func (e *eng) takeAbandonedGate(a, seat int) {
	id := e.areaID(a)
	if ctl, ok := e.d.Gates[id]; !ok || ctl != 0 || !e.canControlGate(seat, a) {
		return
	}
	if e.d.Turn != nil && containsStr(e.d.Turn.Toggled, id) {
		return
	}
	e.occupy(a, seat, "")
	e.say("%s takes control of the Gate in %s.", e.seatName(seat), e.areaName(a))
}

// syncAllGates re-checks every Gate.
func (e *eng) syncAllGates() {
	for id := range e.d.Gates {
		e.syncGate(e.areaIdx(id))
	}
}

// gatesOf lists the areas whose Gate seat controls.
func (e *eng) gatesOf(seat int) []int {
	var out []int
	for id, ctl := range e.d.Gates {
		if ctl == seat {
			out = append(out, e.areaIdx(id))
		}
	}
	// Yog-Sothoth is a Gate of the Opener's, wherever he stands.
	if e.is(seat, facOW) {
		for a := range e.m.ids {
			if e.count(a, seat, uYog) > 0 && !containsInt(out, a) {
				out = append(out, a)
			}
		}
	}
	sort.Ints(out)
	return out
}

func (e *eng) hasGate(a int) bool {
	_, ok := e.d.Gates[e.areaID(a)]
	return ok || e.yogIn(a)
}

// --- units -----------------------------------------------------------------

// eliminate removes a unit from the map to its owner's pool. by is the enemy
// seat that caused it (0 for none); cultists lost to an enemy feed Passion.
func (e *eng) eliminate(a, seat int, typ string, by int) {
	if !e.take(a, seat, typ) {
		return
	}
	e.setPool(seat, typ, e.pool(seat, typ)+1)
	// An Independent that dies goes back to the general pool, card and all.
	if isIndependent(typ) {
		e.igooDies(seat, typ)
	}
	e.syncGate(a)
	if isCultist(typ) && by != 0 && by != seat {
		if e.d.Bat != nil {
			e.d.Bat.LostCultists[seat]++
		} else {
			e.passion(seat)
		}
	}
}

// passion pays Yellow Sign's Passion: 1 Power when Cultists are lost to an
// enemy.
func (e *eng) passion(seat int) {
	if e.is(seat, facYS) && e.d.hasBook(seat, bkPassion) {
		e.d.Power[seat]++
		e.say("Passion: %s gains 1 Power.", e.seatName(seat))
	}
}

// goo reports whether a Great Old One type is on the map for seat, and where.
func (e *eng) gooArea(seat int, typ string) (int, bool) {
	for a := range e.m.ids {
		if e.count(a, seat, typ) > 0 {
			return a, true
		}
	}
	return 0, false
}

// gooInPlay counts seat's Great Old Ones on the map (submerged ones count as
// in play).
func (e *eng) gooInPlay(seat int) int {
	n := 0
	for _, u := range e.f(seat).Units {
		if u.Class == classGOO {
			n += e.onMap(seat, u.ID)
			if e.is(seat, facGC) {
				n += e.d.Away[u.ID]
			}
		}
	}
	return n
}

// cultistsInPlay counts seat's Cultists on the map (a slumbering Cultist of
// the Sleeper counts: it is only off the map).
func (e *eng) cultistsInPlay(seat int) int {
	return e.onMap(seat, uCultist) + e.onMap(seat, uHighPriest) + e.slumberCultists(seat)
}

// areasWithUnits counts the areas where seat has at least one unit.
func (e *eng) areasWithUnits(seat int) int {
	n := 0
	for a := range e.m.ids {
		if e.total(a, seat) > 0 {
			n++
		}
	}
	return n
}

// hasAnyUnits reports whether seat has any unit on the map.
func (e *eng) hasAnyUnits(seat int) bool { return e.areasWithUnits(seat) > 0 }

// --- power, doom -------------------------------------------------------------

// ritualTracks are the Ritual of Annihilation tracks of the box, by number of
// players: the Power each space costs, in order. The marker starts on the first
// space and every Ritual moves it one step; the step after the last space is
// Instant Death, and the game ends when that Doom phase has been scored. (A
// two-player game uses the four-player track.)
var ritualTracks = map[int][]int{
	3: {5, 6, 7, 8, 9, 10},
	4: {5, 6, 7, 7, 8, 8, 9, 10},
	5: {5, 6, 6, 7, 7, 8, 8, 9, 9, 10},
}

// ritualTrack is the track this game uses: its length follows the number of
// players.
func (e *eng) ritualTrack() []int {
	n := e.n
	switch {
	case n < 3:
		n = 4
	case n > 5:
		n = 5
	}
	return ritualTracks[n]
}

// ritualCost is what the next Ritual of Annihilation costs: the number on the
// space the marker stands on (ten once it is on Instant Death).
func (e *eng) ritualCost() int {
	t := e.ritualTrack()
	i := e.d.Ritual
	if i >= len(t) {
		i = len(t) - 1
	}
	if i < 0 {
		i = 0
	}
	return t[i]
}

// advanceRitual moves the Ritual marker one space. Reaching the end of the
// track sets Instant Death: the game ends once the Doom phase — this one, or
// the next if the marker moved at another time — has been scored.
func (e *eng) advanceRitual() {
	d := e.d
	last := len(e.ritualTrack())
	if d.Ritual >= last {
		return
	}
	d.Ritual++
	if d.Ritual >= last {
		d.Instant = true
		if d.Phase == phaseDoom {
			e.say("The Ritual marker reaches Instant Death: the game ends when this Doom phase has been scored.")
		} else {
			e.say("The Ritual marker reaches Instant Death: the game ends when the next Doom phase has been scored.")
		}
	}
}

// turnOrder lists the playing seats in the order they act: from first, round
// the table in the direction dir (+1: clockwise, down the player list).
func (e *eng) turnOrder(first, dir int) []int {
	var out []int
	for k := 0; k < e.n; k++ {
		s := ((first - 1) + k*dir) % e.n
		if s < 0 {
			s += e.n
		}
		out = append(out, s+1)
	}
	return out
}

// others lists the other seats in table order from seat.
func (e *eng) others(seat int) []int {
	var out []int
	for k := 1; k < e.n; k++ {
		out = append(out, (seat-1+k)%e.n+1)
	}
	return out
}

// active reports whether seat plays in this game.
func (e *eng) active(seat int) bool { return seat >= 1 && seat <= e.n }

// playing lists the seats at the table, in seat order.
func (e *eng) playing() []int {
	out := make([]int, 0, e.n)
	for s := 1; s <= e.n; s++ {
		out = append(out, s)
	}
	return out
}

// --- dice -------------------------------------------------------------------

// nextRandom advances a splitmix64 generator state and returns its output.
func nextRandom(state *uint64) uint64 {
	*state += 0x9e3779b97f4a7c15
	z := *state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// RollDice returns the next n dice (1-6) a generator state produces and the
// state after them: every Cthulhu Wars roll comes from here.
func RollDice(state uint64, n int) (dice []int, after uint64) {
	dice = make([]int, n)
	for i := range dice {
		dice[i] = int((uint64(uint32(nextRandom(&state)>>32))*6)>>32) + 1
	}
	return dice, state
}

func (e *eng) roll(n int) []int {
	if e.d.RNG == 0 {
		e.d.RNG = freshSeed()
	}
	dice, after := RollDice(e.d.RNG, n)
	e.d.RNG = after
	return dice
}

func (e *eng) die() int { return e.roll(1)[0] }

// article puts "a" or "an" in front of a unit name.
func article(name string) string {
	if name == "" {
		return name
	}
	switch name[0] {
	case 'A', 'E', 'I', 'O', 'U':
		return "an " + name
	}
	return "a " + name
}
