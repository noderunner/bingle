package cthulhuwars

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

var (
	_ state.HiddenInfo    = (*Ruleset)(nil)
	_ state.Heuristic     = (*Ruleset)(nil)
	_ state.PlaneEncoder  = (*Ruleset)(nil)
	_ state.TextRenderer  = (*Ruleset)(nil)
	_ state.Describer     = (*Ruleset)(nil)
	_ state.SeededRuleset = (*Ruleset)(nil)
	_ state.UndoGuard     = (*Ruleset)(nil)
	_ state.Searchable    = (*Ruleset)(nil)

	_ state.RepetitionGuard = (*Ruleset)(nil)
)

// View is what clients draw beyond the units in areas: Gates, scoreboards,
// each cult's spellbooks, the battle on the table and the pending decision.
// It is derived from the position and carried in the "view" key of a
// public position's ext payload.
type View struct {
	Phase  string `json:"phase"`
	Stage  string `json:"stage,omitempty"`
	Round  int    `json:"round"`
	ToMove int    `json:"to_move,omitempty"`
	Prompt string `json:"prompt"`
	First  int    `json:"first"`
	Dir    int    `json:"dir"`
	// Ritual is the Ritual of Annihilation marker.
	Ritual RitualView `json:"ritual"`
	// Reaction is set while the decision waiting is a reaction window.
	Reaction *ReactionView `json:"reaction,omitempty"`
	// Phases lists the steps of a round in the order they run, with the
	// current one marked; Order is the turn order both ways round; Bag is
	// the Elder Sign bag.
	Phases []PhaseView `json:"phases"`
	Order  OrderView   `json:"order"`
	Bag    BagView     `json:"bag"`
	// Tints colours areas by legend key; Badges marks areas (Gates,
	// Desecration, the battle); Notes are hover texts.
	Tints  map[string]string   `json:"tints,omitempty"`
	Badges map[string][]string `json:"badges,omitempty"`
	Notes  map[string]string   `json:"notes,omitempty"`
	// Holders names, for every controlled Gate, the type of the unit that
	// occupies it (the icon drawn on the Gate).
	Holders map[string]string `json:"holders,omitempty"`
	// Costs is the Power each action button costs the player who has to
	// decide, as it stands now (the cheapest way to take it): keyed by the
	// id of the action.
	Costs map[string]int `json:"costs,omitempty"`
	// Seats holds each cult's scoreboard and cards, keyed "1".."4".
	Seats map[string]*SeatView `json:"seats"`
	// Battle is the battle being fought, or the last one fought.
	Battle *BattleView `json:"battle,omitempty"`
	// Neutral is what is left of the neutral spellbooks and Loyalty Cards; nil
	// at a table that plays without them.
	Neutral *NeutralView `json:"neutral,omitempty"`
}

// PhaseView is one step of the round: "action", "gather", "first" or
// "doom"; State is "done", "current" or "todo" for the round in progress.
type PhaseView struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	State string `json:"state"`
}

// OrderView lists the playing seats from the first player in each direction:
// clockwise is down the player list, counter-clockwise up it.
type OrderView struct {
	CW  []int `json:"cw"`
	CCW []int `json:"ccw"`
}

// BagView counts the Elder Signs still in the bag (their values are secret —
// unless Chaugnar Faugn's Curse is in play: then the pool is face up and Values
// counts the 1s, 2s and 3s).
type BagView struct {
	Left   int    `json:"left"`
	Total  int    `json:"total"`
	Values [3]int `json:"values,omitempty"`
}

// ReactionView says the decision waiting is a reaction: an ability a player
// may use on another player's turn, who may, and whose action opened it.
type ReactionView struct {
	Seat     int    `json:"seat"`
	Trigger  int    `json:"trigger"`
	Ability  string `json:"ability"`
	Targeted bool   `json:"targeted,omitempty"`
}

// RitualView is the Ritual of Annihilation track: the Power each space costs
// (its length follows the number of players), the step the marker stands on
// (len(Track) is Instant Death) and the cost of the next Ritual.
type RitualView struct {
	Track   []int `json:"track"`
	Cost    int   `json:"cost"`
	Marker  int   `json:"marker"`
	Instant bool  `json:"instant,omitempty"`
}

// SeatView is one cult's scoreboard.
type SeatView struct {
	Faction  string        `json:"faction"`
	Counters []CounterView `json:"counters"`
	Cards    []CardView    `json:"cards,omitempty"`
	// Units describes each kind of unit of the cult; Hidden is how many Elder
	// Signs it holds face down, Shown the values it has revealed and Signs
	// the hidden values — filled in only for the cult's own player.
	Units []UnitView `json:"units,omitempty"`
	// Captured are the units of other cults this cult holds: they go home to
	// their owners' pools in Gather Power.
	Captured []CapturedView `json:"captured,omitempty"`
	Hidden   int            `json:"hidden_signs"`
	Shown    []int          `json:"shown_signs,omitempty"`
	Signs    []int          `json:"signs,omitempty"`
}

// CapturedView is one unit held by a cult: whose it is, what kind, and what
// becomes of it in Gather Power.
type CapturedView struct {
	// Owner is the seat the unit belongs to, Unit its type.
	Owner int    `json:"owner"`
	Unit  string `json:"unit"`
	// Fate is "home" for a Cultist that returns to its owner's pool and
	// "sacrificed" for a Monster the Sleeper holds (1 Power, then home).
	Fate string `json:"fate"`
}

// AbilityView is one named ability of a unit.
type AbilityView struct {
	Name   string `json:"name"`
	Timing string `json:"timing"`
	Text   string `json:"text"`
}

// UnitView is what a player may learn about one kind of unit: its numbers,
// how many are where, and the spellbooks that bear on it.
type UnitView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Class string `json:"class"` // cultist | monster | goo
	// Cost is the Power to recruit, summon or awaken it now.
	Cost int `json:"cost"`
	// Combat says how many dice it rolls ("2", or a rule for the units whose
	// Combat is computed).
	Combat string `json:"combat"`
	Text   string `json:"text,omitempty"`
	// Total units of the kind the cult owns; Pool waiting to be placed;
	// OnMap standing on the map; Away submerged; Captured Cultists held by
	// other cults.
	Total    int `json:"total"`
	Pool     int `json:"pool"`
	OnMap    int `json:"on_map"`
	Away     int `json:"away,omitempty"`
	Captured int `json:"captured,omitempty"`
	// Enter says where the unit may come into play; Awaken, for a Great Old
	// One, the steps of awakening it; Places says where each unit that is not
	// in the pool is: on the map (the area; the unit on a Gate says so),
	// captured, held, asleep, submerged…
	Enter  string   `json:"enter,omitempty"`
	Awaken []string `json:"awaken,omitempty"`
	Places []string `json:"places,omitempty"`
	// Abilities are the named abilities of the unit's card (a Great Old
	// One's Desecrate, Vengeance, Devour…).
	Abilities []AbilityView `json:"abilities,omitempty"`
	// Books are the spellbooks that enhance the unit; Enhanced says that at
	// least one has been earned.
	Books    []UnitBookView `json:"books,omitempty"`
	Enhanced bool           `json:"enhanced,omitempty"`
}

// UnitBookView is a spellbook relevant to a unit.
type UnitBookView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Timing   string `json:"timing"`
	Text     string `json:"text"`
	Unlocked bool   `json:"unlocked"`
}

// CounterView is a labelled number.
type CounterView struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value int    `json:"value"`
	Max   int    `json:"max,omitempty"`
	// Note explains the value (what the Doom is made of).
	Note string `json:"note,omitempty"`
}

// CardView is a titled list of rows: a faction card section.
type CardView struct {
	ID    string    `json:"id"`
	Title string    `json:"title"`
	Rows  []RowView `json:"rows"`
}

// RowView is one line of a card; State is "done", "todo" or "".
type RowView struct {
	Text   string `json:"text"`
	Detail string `json:"detail,omitempty"`
	State  string `json:"state,omitempty"`
}

// BattleView shows a battle.
type BattleView struct {
	Area     string            `json:"area"`
	AreaName string            `json:"area_name"`
	Att      int               `json:"att"`
	Def      int               `json:"def"`
	Step     string            `json:"step,omitempty"`
	Resolved bool              `json:"resolved"`
	Sides    [2]BattleSideView `json:"sides"`
}

// BattleSideView is one side of a battle.
type BattleSideView struct {
	Seat   int    `json:"seat"`
	Name   string `json:"name"`
	Combat int    `json:"combat"`
	Dice   []int  `json:"dice,omitempty"`
	// Kills and Pains are the results rolled *against* this side.
	Kills int `json:"kills"`
	Pains int `json:"pains"`
}

// Redact returns st as viewer may see it: the dice generator and other
// players' Elder Signs are hidden, and the derived view is attached.
func (rs *Ruleset) Redact(game *catalog.CompiledGame, st *state.BoardState, viewer int) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	e := rs.newEng(game, st, d)
	v := e.buildView(viewer)
	c := d.clone()
	c.RNG = 0
	c.Bag = [3]int{}
	for s := 1; s <= nSeats; s++ {
		if s == viewer {
			continue
		}
		for i := range c.Signs[s] {
			c.Signs[s][i] = 0
		}
	}
	out := st.Clone()
	out.SetExt(&publicExt{Data: c, View: v})
	return out
}

// RedactMove: nothing in a move record is secret in this game.
func (rs *Ruleset) RedactMove(game *catalog.CompiledGame, st *state.BoardState, mv state.Move, notation string) (state.Move, string) {
	return mv, notation
}

// Determinize samples what viewer cannot know: the dice generator and the
// values of the other players' Elder Signs.
func (rs *Ruleset) Determinize(game *catalog.CompiledGame, st *state.BoardState, viewer int, rng *rand.Rand) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	c := d.clone()
	c.RNG = rng.Uint64() | 1
	// Everything hidden: the bag plus other players' signs (values known
	// only as a pool of 1s, 2s and 3s).
	pool := []int{}
	for v := 0; v < 3; v++ {
		for i := 0; i < c.Bag[v]; i++ {
			pool = append(pool, v+1)
		}
	}
	for s := 1; s <= nSeats; s++ {
		if s == viewer {
			continue
		}
		for _, v := range c.Signs[s] {
			if v >= 1 && v <= 3 {
				pool = append(pool, v)
			}
		}
	}
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	k := 0
	for s := 1; s <= nSeats; s++ {
		if s == viewer {
			continue
		}
		for i := range c.Signs[s] {
			if k < len(pool) {
				c.Signs[s][i] = pool[k]
				k++
			}
		}
	}
	c.Bag = [3]int{}
	for _, v := range pool[k:] {
		c.Bag[v-1]++
	}
	out := st.Clone()
	out.SetExt(c)
	return out
}

// CanTakeBack forbids rewinding across a die roll or an Elder Sign draw.
func (rs *Ruleset) CanTakeBack(game *catalog.CompiledGame, before, after *state.BoardState) bool {
	b, err1 := dataOf(before)
	a, err2 := dataOf(after)
	if err1 != nil || err2 != nil {
		return true
	}
	return b.RNG == a.RNG
}

// WorthSearching is always true: every decision matters.
func (rs *Ruleset) WorthSearching(game *catalog.CompiledGame, st *state.BoardState) bool {
	return true
}

func (e *eng) buildView(viewer int) *View {
	d := e.d
	v := &View{
		Phase: d.Phase, Stage: d.Stage, Round: d.Round, ToMove: toMove(d),
		Prompt: e.prompt(), First: d.First, Dir: d.Dir,
		Ritual: RitualView{Track: e.ritualTrack(), Cost: e.ritualCost(), Marker: d.Ritual, Instant: d.Instant},
		Tints:  map[string]string{}, Badges: map[string][]string{}, Notes: map[string]string{},
		Holders: map[string]string{},
		Seats:   map[string]*SeatView{},
	}
	for a, id := range e.m.ids {
		sid := string(id)
		switch {
		case d.Desec[sid]:
			v.Tints[sid] = "desecrated"
		case e.m.ocean[a]:
			v.Tints[sid] = "ocean"
		default:
			v.Tints[sid] = "land"
		}
		if ctl, ok := d.Gates[sid]; ok {
			v.Badges[sid] = append(v.Badges[sid], fmt.Sprintf("gate-%d", ctl))
			if h := e.holderAt(a); h != "" {
				v.Holders[sid] = h
			}
		}
		if d.Desec[sid] {
			v.Badges[sid] = append(v.Badges[sid], "desecrated")
		}
		if d.Ice == sid {
			v.Badges[sid] = append(v.Badges[sid], "iceage")
		}
		if d.Bat != nil && d.Bat.Area == sid {
			v.Badges[sid] = append(v.Badges[sid], "target")
		}
		v.Notes[sid] = e.areaNote(a)
	}
	for s := 1; s <= nSeats; s++ {
		if e.active(s) {
			v.Seats[fmt.Sprint(s)] = e.seatView(s, viewer)
		}
	}
	if r := e.pendingReaction(); r != nil {
		v.Reaction = &ReactionView{Seat: r.Seat, Trigger: r.Trigger, Ability: r.Ability, Targeted: r.Targeted}
	}
	v.Costs = e.actionCosts()
	v.Neutral = e.neutralView()
	v.Phases = e.phaseViews()
	v.Order = OrderView{CW: e.turnOrder(d.First, 1), CCW: e.turnOrder(d.First, -1)}
	v.Bag = BagView{Left: d.Bag[0] + d.Bag[1] + d.Bag[2], Total: e.bagTotal()}
	if e.curseOwner() != 0 {
		v.Bag.Values = d.Bag
	}
	if b := d.Bat; b != nil {
		v.Battle = e.battleView(b.Area, b.Att, b.Def, b.Step, false, nil)
		for _, s := range []int{b.Att, b.Def} {
			for i := range v.Battle.Sides {
				if v.Battle.Sides[i].Seat == s {
					v.Battle.Sides[i].Combat = e.combatOf(s, e.areaIdx(b.Area), b, b.foeOf(s))
					v.Battle.Sides[i].Dice = append([]int(nil), b.Dice[s]...)
					v.Battle.Sides[i].Kills = b.Kills[s]
					v.Battle.Sides[i].Pains = b.Pains[s]
				}
			}
		}
	} else if l := d.Last; l != nil {
		v.Battle = e.battleView(l.Area, l.Att, l.Def, "", true, l)
	}
	return v
}

// phaseViews lists the four steps of a round in rule-book order — Action,
// Gather Power, Determine First Player, Doom — marking the one under way. The
// second and third happen as soon as the Action phase ends, so a player
// waits at the third (the direction of play) rather than at the second.
func (e *eng) phaseViews() []PhaseView {
	d := e.d
	steps := []PhaseView{
		{ID: "action", Label: "Action"},
		{ID: "gather", Label: "Gather Power"},
		{ID: "first", Label: "First Player"},
		{ID: "doom", Label: "Doom"},
	}
	cur := 0
	switch {
	case d.Phase == phaseOver:
		cur = len(steps)
	case d.Phase == phaseDoom:
		cur = 3
	case d.Opening && d.Stage == stageDirection:
		// Before the first Action phase: the first player has yet to choose the
		// direction of play, and the Doom phase is skipped this round.
		steps[0].State, steps[1].State, steps[2].State, steps[3].State = "todo", "done", "current", "todo"
		return steps
	case d.Stage == stageFirst || d.Stage == stageDirection:
		cur = 2
	}
	for i := range steps {
		switch {
		case i < cur:
			steps[i].State = "done"
		case i == cur:
			steps[i].State = "current"
		default:
			steps[i].State = "todo"
		}
	}
	return steps
}

// bagTotal is how many Elder Signs the bag held at the start.
func (e *eng) bagTotal() int {
	total := 0
	if l, ok := e.game.Definition.Params["elder_signs"].([]any); ok && len(l) == 3 {
		for _, v := range l {
			if f, ok := v.(float64); ok {
				total += int(f)
			}
		}
	}
	if total == 0 {
		total = 18 + 12 + 6
	}
	return total
}

func (e *eng) battleView(area string, att, def int, step string, resolved bool, l *LastBattle) *BattleView {
	bv := &BattleView{Area: area, AreaName: e.areaName(e.areaIdx(area)), Att: att, Def: def, Step: step, Resolved: resolved}
	bv.Sides[0] = BattleSideView{Seat: att, Name: e.seatName(att)}
	bv.Sides[1] = BattleSideView{Seat: def, Name: e.seatName(def)}
	if l != nil {
		for i := range bv.Sides {
			s := bv.Sides[i].Seat
			bv.Sides[i].Combat = l.Combat[s]
			bv.Sides[i].Dice = append([]int(nil), l.Dice[s]...)
			bv.Sides[i].Kills = l.Kills[s]
			bv.Sides[i].Pains = l.Pains[s]
		}
	}
	return bv
}

// areaNote describes what stands in an area.
func (e *eng) areaNote(a int) string {
	d := e.d
	var parts []string
	if ctl, ok := d.Gates[e.areaID(a)]; ok {
		switch ctl {
		case 0:
			parts = append(parts, "abandoned Gate")
		default:
			note := "Gate of " + e.seatName(ctl)
			if h := e.holderAt(a); h != "" {
				note += ", held by " + article(unitNameOf(h))
			}
			parts = append(parts, note)
		}
	}
	if d.Desec[e.areaID(a)] {
		parts = append(parts, "Desecrated")
	}
	for s := 1; s <= nSeats; s++ {
		var us []string
		for _, typ := range e.typesIn(a, s) {
			n := e.count(a, s, typ)
			if n > 1 {
				us = append(us, fmt.Sprintf("%d %s", n, unitNameOf(typ)))
			} else {
				us = append(us, unitNameOf(typ))
			}
		}
		if len(us) > 0 {
			parts = append(parts, e.seatName(s)+": "+strings.Join(us, ", "))
		}
	}
	kind := "land"
	if e.m.ocean[a] {
		kind = "ocean"
	}
	if g := e.m.glyph[a]; g != "" {
		kind += ", " + g + " glyph"
	}
	return fmt.Sprintf("%s (%s)", e.areaName(a), kind) + ifNotEmpty(" — ", strings.Join(parts, "; "))
}

func ifNotEmpty(prefix, s string) string {
	if s == "" {
		return ""
	}
	return prefix + s
}

func (e *eng) seatView(s, viewer int) *SeatView {
	d := e.d
	f := e.f(s)
	sv := &SeatView{Faction: f.Key}
	signs := len(d.Signs[s])
	sv.Counters = []CounterView{
		{ID: "power", Label: "Power", Value: d.Power[s]},
		{ID: "doom", Label: "Doom", Value: d.Doom[s], Max: e.game.ParamInt("doom_to_win", 30), Note: e.doomSources(s)},
		{ID: "signs", Label: "Elder Signs", Value: signs},
		{ID: "books", Label: "Spellbooks", Value: d.bookCount(s), Max: 6},
		{ID: "gates", Label: "Gates", Value: e.gateCount(s)},
	}
	if e.is(s, facWW) && d.hasBook(s, bkHerald) {
		sv.Counters = append(sv.Counters, CounterView{ID: "ritual", Label: "Ritual", Value: e.ritualCostFor(s)})
	}
	// Ubbo-Sathla's Growth counter.
	if e.is(s, facTT) {
		sv.Counters = append(sv.Counters, CounterView{ID: "growth", Label: "Ubbo-Sathla's Growth", Value: d.Growth, Note: "His Combat. After he has been awakened it rises by a die each Doom phase (Hell's Banquet) and by 1 for every High Priest Hierophants cannot place."})
	}
	// Yellow Sign counts the Desecration tokens in play, ahead of the rest.
	if e.is(s, facYS) {
		n := 0
		for _, on := range d.Desec {
			if on {
				n++
			}
		}
		sv.Counters = append([]CounterView{{ID: "desecrations", Label: "Desecration tokens", Value: n}}, sv.Counters...)
	}
	// Spellbooks.
	books := CardView{ID: "books", Title: "Spellbooks"}
	for i, b := range f.Books {
		// A cult that set up with a neutral spellbook in place of its timer
		// book shows that one in the slot.
		if i == e.swappedOut(s) {
			n := e.swapOf(s)
			nb := neutralBooks[n]
			row := RowView{Text: nb.Name + " (neutral, in place of " + b.Name + ")", Detail: nb.Timing + ": " + nb.Text, State: "todo"}
			if d.heldNeutral(s, n) {
				row.State = "done"
				if d.mooned(s, bookSlots+n) {
					row.Text += " — a Moonbeast lies on it"
				}
			}
			books.Rows = append(books.Rows, row)
			continue
		}
		row := RowView{Text: b.Name, Detail: b.Timing + ": " + b.Text, State: "todo"}
		if d.holds(s, i) {
			row.State = "done"
			if d.mooned(s, i) {
				row.Text += " — a Moonbeast lies on it"
			}
		}
		books.Rows = append(books.Rows, row)
	}
	reqs := CardView{ID: "reqs", Title: "Requirements"}
	for i, r := range f.Reqs {
		row := RowView{Text: r.Text, State: "todo"}
		if d.reqDone(s, i) {
			row.State = "done"
		}
		reqs.Rows = append(reqs.Rows, row)
	}
	units := CardView{ID: "units", Title: "Units"}
	for _, u := range e.units(s) {
		onMap := e.onMap(s, u.ID)
		detail := fmt.Sprintf("%d on the map, %d in pool", onMap, e.pool(s, u.ID))
		if u.ID == uShambler && d.Card[s] > 0 {
			detail += fmt.Sprintf(", %d waiting on the faction card", d.Card[s])
		}
		switch u.ID {
		case uCultist, uHighPriest:
			capt := 0
			for by := 1; by <= nSeats; by++ {
				if u.ID == uCultist {
					capt += d.Captured[by][s]
				} else {
					capt += d.CapHP[by][s]
				}
			}
			if capt > 0 {
				detail += fmt.Sprintf(", %d captured", capt)
			}
			if d.Slumber != nil && e.is(s, facSL) && d.Slumber.Cultist == u.ID {
				detail += ", 1 asleep"
			}
		}
		if n := d.Away[u.ID]; n > 0 && e.is(s, facGC) {
			detail += fmt.Sprintf(", %d submerged", n)
		}
		if e.is(s, facSL) && u.ID == uSerpent {
			if n := len(d.Sorcery); n > 0 {
				detail += fmt.Sprintf(", %d on an enemy's card", n)
			}
		}
		held := 0
		for _, h := range d.Held {
			if h.Seat == s && h.Type == u.ID {
				held++
			}
		}
		if held > 0 {
			detail += fmt.Sprintf(", %d held by the Sleeper", held)
		}
		cost := ""
		if u.Class == classMonster {
			cost = fmt.Sprintf(" · cost %d", e.summonCost(s, u.ID))
		}
		units.Rows = append(units.Rows, RowView{Text: u.Name, Detail: detail + cost})
	}
	// The cult's faction ability — and, for the Sleeper, the ones it borrows.
	ability := CardView{ID: "ability", Title: "Faction Ability"}
	name, text := splitAbility(f.Ability)
	ability.Rows = append(ability.Rows, RowView{Text: name, Detail: text})
	if e.is(s, facSL) {
		for _, so := range d.Sorcery {
			if so.Ending {
				continue
			}
			if of := e.fac(so.Seat); of >= 0 && of != facSL {
				n, t := splitAbility(factions[of].Ability)
				ability.Rows = append(ability.Rows, RowView{
					Text:   "Borrowed: " + n,
					Detail: t + " (Ancient Sorcery: until the end of the Doom phase, from " + e.seatName(so.Seat) + ")",
				})
			}
		}
	}
	sv.Cards = []CardView{ability, books, reqs, units}
	if neutral := e.neutralCard(s); neutral != nil {
		sv.Cards = append(sv.Cards, *neutral)
	}
	if loyalty := e.loyaltyCard(s); loyalty != nil {
		sv.Cards = append(sv.Cards, *loyalty)
	}
	for _, u := range e.units(s) {
		sv.Units = append(sv.Units, e.unitView(s, f, u))
	}
	sv.Captured = e.capturedBy(s)
	sv.Hidden = signs
	sv.Shown = append([]int(nil), d.Shown[s]...)
	if s == viewer {
		sv.Signs = append([]int(nil), d.Signs[s]...)
	}
	return sv
}

// enterText says where a unit may come into play, or where a Gate may be
// built (id actGate): the rules in a sentence for the cards and tooltips. For
// a Great Old One see awakenSteps.
func enterText(id string) string {
	switch id {
	case actGate:
		return "One of your Cultists — or the High Priest — builds a Gate in the area it stands in, which must have no Gate yet: " +
			"one Gate to an area, on land or in an ocean. You control it at once. A Gate gives you 2 Power in Gather Power and 1 Doom in the Doom phase, " +
			"and it is where monsters are summoned and most Great Old Ones awakened. " +
			"(Black Goat with The Red Sign: a Dark Young can build one too.)"
	case uCultist:
		return "Recruit it in an area where you already have a unit — anywhere, when you have no units on the map."
	case uHighPriest:
		return "Recruit him in an area where you already have a unit."
	}
	switch unitClassOf(id) {
	case classMonster:
		if id == uFilth {
			return "Abhoth puts it in any area (Action, 1 Power); it is never summoned or moved."
		}
		return "Summon it at a Gate you control."
	case classTerror:
		return "Summon it at a Gate you control (it is a Terror, not a Monster)."
	}
	return ""
}

// awakenSteps are the steps of awakening a Great Old One, as the faction pages
// of the rulebook give them: what must stand on the map, what it costs and
// where the Great Old One appears. start names the area the cult began in and
// sigils the areas marked with the Windwalker's sigil; blank, the text speaks
// of them generally.
func awakenSteps(id, start, sigils string) []string {
	or := func(s, def string) string {
		if s != "" {
			return s
		}
		return def
	}
	switch id {
	case uCthulhu:
		where := or(start, "your starting area")
		return []string{
			"There must be a Gate in " + where + " — yours, abandoned or an enemy's.",
			"Pay 10 Power; 4 Power every time after the first (Immortal).",
			"Cthulhu appears in " + where + ", and you gain 1 Elder Sign (Immortal).",
		}
	case uNyar:
		return []string{
			"You must control a Gate.",
			"Pay 10 Power.",
			"Nyarlathotep appears at that Gate.",
		}
	case uShub:
		return []string{
			"You must control a Gate and have at least 2 Cultists on the map — in any areas.",
			"Pay 8 Power.",
			"Eliminate 2 of your Cultists of your choice (they return to your pool); Shub-Niggurath appears at that Gate.",
		}
	case uKing:
		return []string{
			"You must have a unit in an area that has no Gate.",
			"Pay 4 Power.",
			"The King in Yellow appears in that area.",
		}
	case uHastur:
		return []string{
			"You must control a Gate in an area where the King in Yellow stands.",
			"Pay 10 Power.",
			"Hastur appears in that area.",
		}
	case uRhan:
		return []string{
			"Pay 6 Power.",
			"Rhan-Tegoth appears in an area marked with the Windwalker sigil (" + or(sigils, "the polar areas") + ").",
		}
	case uIthaqua:
		return []string{
			"Rhan-Tegoth has been awakened (he need not be in play).",
			"A Gate must stand in an area marked with the Windwalker sigil (" + or(sigils, "the polar areas") + ") — anyone's: you need not control it.",
			"Pay 6 Power: Ithaqua replaces that Gate.",
		}
	case uYog:
		return []string{
			"You must have a Spawn of Yog-Sothoth on the map.",
			"Pay 6 Power. Yog-Sothoth replaces the Spawn.",
		}
	case uUbbo:
		return []string{
			"You must control a Gate and have a High Priest in play (he need not be with the Gate), in the Doom phase or the Action phase.",
			"In the Doom phase pay 0 Power; in the Action phase pay 6 Power.",
			"Eliminate the High Priest; Ubbo-Sathla appears at your controlled Gate.",
		}
	case uTsath:
		return []string{
			"You must have a Formless Spawn on the map.",
			"Pay 8 Power (4 after the first time, when Immortal is lent to you by Ancient Sorcery).",
			"Tsathoggua appears in the area of a Formless Spawn.",
		}
	}
	return nil
}

// awakenText is awakenSteps as one numbered paragraph.
func awakenText(id string) string {
	steps := awakenSteps(id, "", "")
	for i, s := range steps {
		steps[i] = fmt.Sprintf("%d. %s", i+1, s)
	}
	return strings.Join(steps, " ")
}

// capturedBy lists the units seat holds, owner by owner: Cultists and High
// Priests captured in the Action phase, and the Monsters of the Sleeper's
// Capture Monster.
func (e *eng) capturedBy(seat int) []CapturedView {
	d := e.d
	var out []CapturedView
	for owner := 1; owner <= nSeats; owner++ {
		if owner == seat {
			continue
		}
		for i := 0; i < d.Captured[seat][owner]; i++ {
			out = append(out, CapturedView{Owner: owner, Unit: uCultist, Fate: "home"})
		}
		for i := 0; i < d.CapHP[seat][owner]; i++ {
			out = append(out, CapturedView{Owner: owner, Unit: uHighPriest, Fate: "home"})
		}
	}
	// The Monsters the Sleeper's Tsathoggua holds are the Sleeper's captives.
	if e.is(seat, facSL) {
		for _, h := range d.Held {
			out = append(out, CapturedView{Owner: h.Seat, Unit: h.Type, Fate: "sacrificed"})
		}
	}
	return out
}

// splitAbility cuts a faction ability "Name (timing): what it does" into the
// name and the text.
func splitAbility(s string) (name, text string) {
	if i := strings.Index(s, ": "); i > 0 {
		return s[:i], s[i+2:]
	}
	return s, ""
}

// unitView describes one kind of unit of seat for its information card.
func (e *eng) unitView(seat int, f *faction, u unitSpec) UnitView {
	d := e.d
	uv := UnitView{
		ID: u.ID, Name: u.Name, Text: u.Text,
		Total: u.Count, Pool: e.pool(seat, u.ID), OnMap: e.onMap(seat, u.ID),
		Combat: e.combatText(seat, u),
		Enter:  enterText(u.ID),
		Places: e.placesOf(seat, u.ID),
	}
	for _, a := range u.Abilities {
		uv.Abilities = append(uv.Abilities, AbilityView(a))
	}
	if u.Class == classGOO {
		if i := cardIndex(u.ID); i >= firstIndependent {
			uv.Awaken = cards[i].Awaken
			uv.Enter = "An Independent Great Old One: awakened in the Action phase, never summoned."
		} else {
			// The area names are those of this map.
			start := e.areaName(e.areaIdx(startArea(e.game, *f)))
			var sig []string
			for _, a := range e.sigilAreas(facWW) {
				sig = append(sig, e.areaName(a))
			}
			uv.Awaken = awakenSteps(u.ID, start, strings.Join(sig, " or "))
		}
	}
	switch u.Class {
	case classCultist:
		uv.Class = "cultist"
		uv.Cost = 1
		if u.ID == uHighPriest {
			uv.Cost = hpSpec.Cost
		}
		for by := 1; by <= nSeats; by++ {
			if u.ID == uCultist {
				uv.Captured += d.Captured[by][seat]
			} else {
				uv.Captured += d.CapHP[by][seat]
			}
		}
	case classMonster:
		uv.Class = "monster"
		uv.Cost = e.summonCost(seat, u.ID)
	case classTerror:
		uv.Class = "terror"
		uv.Cost = e.summonCost(seat, u.ID)
	default:
		uv.Class = "goo"
		uv.Cost = e.awakenCost(seat, u.ID)
		if uv.Cost == 0 {
			uv.Cost = u.Cost
		}
	}
	if e.is(seat, facGC) {
		uv.Away = d.Away[u.ID]
	}
	for i, b := range f.Books {
		if !containsStr(b.Units, u.ID) {
			continue
		}
		on := d.hasBook(seat, i)
		uv.Books = append(uv.Books, UnitBookView{ID: b.ID, Name: b.Name, Timing: b.Timing, Text: b.Text, Unlocked: on})
		if on {
			uv.Enhanced = true
		}
	}
	// An Independent's spellbook lies on its card.
	for _, ig := range e.independentsOf(seat) {
		b := cards[cardIndex(ig)].Book
		if !containsStr(b.Units, u.ID) {
			continue
		}
		on := e.igooBook(seat, ig)
		uv.Books = append(uv.Books, UnitBookView{ID: b.ID, Name: b.Name, Timing: b.Timing, Text: b.Text, Unlocked: on})
		if on {
			uv.Enhanced = true
		}
	}
	return uv
}

// placesOf says where each unit of kind typ of seat that is not in its pool
// is: on the map (area by area, the one on a Gate first in its area), held on
// another cult's card, asleep, submerged with Cthulhu or waiting to be placed.
func (e *eng) placesOf(seat int, typ string) []string {
	d := e.d
	var out []string
	for a := range e.m.ids {
		n := e.count(a, seat, typ)
		onGate := d.Gates[e.areaID(a)] == seat && e.holderAt(a) == typ
		for i := 0; i < n; i++ {
			if i == 0 && onGate {
				out = append(out, "on the Gate in "+e.areaName(a))
			} else {
				out = append(out, "in "+e.areaName(a))
			}
		}
	}
	if typ == uShambler {
		for i := 0; i < d.Card[seat]; i++ {
			out = append(out, "waiting on the faction card (Walk Between Worlds)")
		}
	}
	if typ == uMoonbeast {
		for _, m := range d.Moon {
			if m.Owner == seat {
				out = append(out, "on "+e.seatName(m.Victim)+"'s spellbook "+e.bookSpecOf(m.Victim, m.Book).Name)
			}
		}
	}
	for by := 1; by <= nSeats; by++ {
		n := 0
		switch typ {
		case uCultist:
			n = d.Captured[by][seat]
		case uHighPriest:
			n = d.CapHP[by][seat]
		}
		for i := 0; i < n; i++ {
			out = append(out, "captured by "+e.seatName(by))
		}
	}
	for _, h := range d.Held {
		if h.Seat == seat && h.Type == typ {
			out = append(out, "held by the Sleeper")
		}
	}
	if e.is(seat, facGC) {
		for i := 0; i < d.Away[typ]; i++ {
			out = append(out, "submerged with Cthulhu")
		}
	}
	if e.is(seat, facSL) {
		if d.Slumber != nil && d.Slumber.Cultist == typ {
			out = append(out, "asleep on its faction card (Cursed Slumber)")
		}
		if typ == uSerpent {
			for _, so := range d.Sorcery {
				if so.Ending {
					out = append(out, "waiting to be placed (Ancient Sorcery)")
				} else {
					out = append(out, "on "+e.seatName(so.Seat)+"'s faction card (Ancient Sorcery)")
				}
			}
		}
	}
	return out
}

// actionCosts is the Power each action button costs the seat that has to
// decide: the cheapest of the moves that start with the action.
func (e *eng) actionCosts() map[string]int {
	d := e.d
	seat := toMove(d)
	if seat == 0 || d.Phase == phaseOver {
		return nil
	}
	out := map[string]int{}
	rs := New()
	for _, mv := range e.legal() {
		steps := rs.Steps(e.game, mv)
		if len(steps) == 0 || steps[0].Kind != state.StepAction || mv.Action == actBuy || mv.Action == actWalk {
			continue
		}
		c := e.moveCost(seat, mv)
		switch mv.Action {
		case actRitual:
			c = e.ritualCostFor(seat)
		case actEternal:
			c = 1 // paid when the choice is made, not by an Action
		}
		if cur, ok := out[steps[0].Value]; !ok || c < cur {
			out[steps[0].Value] = c
		}
	}
	return out
}

// combatText says how many dice a unit rolls: a number, or the rule for the
// units whose Combat is computed (with its value now).
func (e *eng) combatText(seat int, u unitSpec) string {
	d := e.d
	switch u.ID {
	case uCultist, uHighPriest:
		if e.is(seat, facBG) && d.hasBook(seat, bkFrenzy) {
			return "1 (Frenzy)"
		}
		return "0"
	case uIthaqua:
		return "half of the enemy's Doom, rounded up"
	case uFormless:
		return fmt.Sprintf("1 die per Formless Spawn on the map and Tsathoggua in play — now %d for each", e.onMap(seat, uFormless)+e.onMap(seat, uTsath))
	case uTsath:
		return "2, or the enemy's Power when that is more"
	case uNyar:
		return fmt.Sprintf("%d + your enemy's — one per spellbook on your faction card and your enemy's in the battle", d.bookCount(seat))
	case uShub:
		n := len(e.gatesOf(seat)) + e.cultistsInPlay(seat)
		note := "your Gates plus your Cultists in play"
		if d.hasBook(seat, bkRedSign) {
			n += e.onMap(seat, uYoung)
			note += ", plus 1 per Dark Young (The Red Sign)"
		}
		return fmt.Sprintf("%d — %s", n, note)
	case uUndead:
		return "one fewer than the Undead in the battle"
	case uByak:
		return "one more than the Byakhee in the battle"
	case uHastur:
		return fmt.Sprintf("%d — the cost of a Ritual of Annihilation now", e.ritualCost())
	case uServitor:
		return "−1 — each Servitor in a battle takes a die away from its owner"
	case uAbhoth:
		return fmt.Sprintf("%d — the Filth Tokens in play", e.filthInPlay())
	case uCthugha:
		return "that of an enemy Great Old One in the battle (the best one); 0 when there is none"
	case uHydra:
		return "6 minus the enemy units in the area (at least 1)"
	case uYog:
		return fmt.Sprintf("%d — twice the number of enemy-controlled Great Old Ones in play", e.yogCombat(seat))
	case uUbbo:
		return fmt.Sprintf("%d — the Growth counter", d.Growth)
	}
	return fmt.Sprint(u.Combat)
}

// --- text -------------------------------------------------------------------

// Text draws the position as monospace text for terminals and models.
func (rs *Ruleset) Text(game *catalog.CompiledGame, st *state.BoardState) string {
	d, err := dataOf(st)
	if err != nil {
		return "(no position)"
	}
	e := rs.newEng(game, st, d)
	var b strings.Builder
	fmt.Fprintf(&b, "Cthulhu Wars — round %d, %s phase", d.Round, d.Phase)
	if d.Phase != phaseOver {
		fmt.Fprintf(&b, " — first player %s, play %s, Ritual cost %d", e.seatName(d.First), map[int]string{1: "in seat order", -1: "in reverse seat order"}[d.Dir], e.ritualCost())
	}
	b.WriteByte('\n')
	ids := make([]int, len(e.m.ids))
	for i := range ids {
		ids[i] = i
	}
	sort.Slice(ids, func(i, j int) bool { return e.m.ids[ids[i]] < e.m.ids[ids[j]] })
	b.WriteString("Areas:\n")
	for _, a := range ids {
		var parts []string
		if ctl, ok := d.Gates[e.areaID(a)]; ok {
			if ctl == 0 {
				parts = append(parts, "Gate (abandoned)")
			} else {
				gate := "Gate (" + e.f(ctl).Key
				if h := e.holderAt(a); h != "" {
					gate += ", held by " + h
				}
				parts = append(parts, gate+")")
			}
		}
		if d.Desec[e.areaID(a)] {
			parts = append(parts, "Desecrated")
		}
		for _, s := range e.playing() {
			var us []string
			for _, typ := range e.typesIn(a, s) {
				us = append(us, fmt.Sprintf("%dx%s", e.count(a, s, typ), typ))
			}
			if len(us) > 0 {
				parts = append(parts, e.f(s).Key+": "+strings.Join(us, ","))
			}
		}
		kind := "land"
		if e.m.ocean[a] {
			kind = "ocean"
		}
		var adj []string
		for _, n := range e.m.adj[a] {
			adj = append(adj, e.areaID(n))
		}
		fmt.Fprintf(&b, "  %-16s [%s] %s | next to: %s\n", e.areaID(a), kind, strings.Join(parts, " ; "), strings.Join(adj, ", "))
	}
	// A public position has its bag zeroed: the count is in its view.
	left := d.Bag[0] + d.Bag[1] + d.Bag[2]
	if pe, ok := st.Ext().(*publicExt); ok && pe.View != nil {
		left = pe.View.Bag.Left
	}
	fmt.Fprintf(&b, "Elder Signs in the bag: %d\n", left)
	b.WriteString("Cults:\n")
	for _, s := range e.playing() {
		f := *e.f(s)
		var books []string
		for i, bk := range f.Books {
			if d.holds(s, i) {
				books = append(books, bk.Name)
			}
		}
		for n, bk := range neutralBooks {
			if d.heldNeutral(s, n) {
				books = append(books, bk.Name+" (neutral)")
			}
		}
		// A redacted position shows a viewer only their own values.
		hidden := fmt.Sprintf("%d hidden Elder Sign(s)", len(d.Signs[s]))
		for _, v := range d.Signs[s] {
			if v > 0 {
				hidden += fmt.Sprintf(" %v", d.Signs[s])
				break
			}
		}
		if len(d.Shown[s]) > 0 {
			hidden += fmt.Sprintf(", revealed %v", d.Shown[s])
		}
		fmt.Fprintf(&b, "  %s (%s, seat %d): Power %d, Doom %d, %s, Gates %d, spellbooks %d/6 [%s]\n",
			f.Name, f.Color, s, d.Power[s], d.Doom[s], hidden, e.gateCount(s), d.bookCount(s), strings.Join(books, ", "))
		var pool []string
		for _, u := range e.units(s) {
			if n := e.pool(s, u.ID); n > 0 {
				pool = append(pool, fmt.Sprintf("%dx%s", n, u.ID))
			}
		}
		if len(pool) > 0 {
			fmt.Fprintf(&b, "      pool: %s\n", strings.Join(pool, ", "))
		}
	}
	fmt.Fprintf(&b, "Stage: %s. %s\n", d.Stage, e.prompt())
	if bt := d.Bat; bt != nil {
		fmt.Fprintf(&b, "Battle in %s: %s attacks %s", bt.Area, e.seatName(bt.Att), e.seatName(bt.Def))
		if bt.Rolled {
			fmt.Fprintf(&b, "; dice: %s %v, %s %v", e.f(bt.Att).Key, bt.Dice[bt.Att], e.f(bt.Def).Key, bt.Dice[bt.Def])
		}
		b.WriteByte('\n')
	}
	if len(d.Events) > 0 {
		b.WriteString("Last move:\n")
		for _, ev := range d.Events {
			b.WriteString("  " + ev + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// --- planes -----------------------------------------------------------------

var planeStages = []string{stageTurn, stageMoving, stagePre, stagePost, stageAssign, stageRetreat, stagePrompt, stageFirst, stageDirection, stageDoom}

// unitMax is the most pieces of a type a cult owns, to scale unit counts into
// [0, 1].
var unitMax = func() map[string]int {
	out := map[string]int{}
	for _, f := range factions {
		for _, u := range f.Units {
			if u.Count > out[u.ID] {
				out[u.ID] = u.Count
			}
		}
	}
	out[uHighPriest] = 1
	for _, u := range cardUnits {
		out[u.ID] = u.Count
	}
	out[uFilth] = filthSpec.Count
	return out
}()

func clamp01(v float32) float32 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	}
	return v
}

// PlaneLayout names the channels: one row of areas (height 1).
func (rs *Ruleset) PlaneLayout(game *catalog.CompiledGame) ([]string, int, int) {
	var names []string
	for r := 0; r < nSeats; r++ {
		rel := "me"
		if r > 0 {
			rel = fmt.Sprintf("opp%d", r)
		}
		for _, t := range unitTypes {
			names = append(names, "unit/"+rel+"/"+t)
		}
	}
	names = append(names, "gate/abandoned", "gate/me", "gate/opp1", "gate/opp2", "gate/opp3", "gate/opp4", "desecrated", "ocean")
	for r := 0; r < nSeats; r++ {
		rel := "me"
		if r > 0 {
			rel = fmt.Sprintf("opp%d", r)
		}
		for _, k := range []string{"power", "doom", "books", "signs", "gates", "cultists"} {
			names = append(names, k+"/"+rel)
		}
	}
	names = append(names, "ritual-cost", "round", "to-move", "first")
	for _, s := range planeStages {
		names = append(names, "stage/"+s)
	}
	return names, 1, len(mapOf(game).ids)
}

// PlaneCells lists the area at each plane cell.
func (rs *Ruleset) PlaneCells(game *catalog.CompiledGame) []string {
	m := mapOf(game)
	out := make([]string, len(m.ids))
	for i, id := range m.ids {
		out[i] = string(id)
	}
	return out
}

// EncodePlanes encodes st for viewer ("me" is the viewer; other cults follow
// in seat order).
func (rs *Ruleset) EncodePlanes(game *catalog.CompiledGame, st *state.BoardState, viewer int) []float32 {
	names, _, w := rs.PlaneLayout(game)
	data := make([]float32, len(names)*w)
	d, err := dataOf(st)
	if err != nil {
		return data
	}
	e := rs.newEng(game, st, d)
	idx := map[string]int{}
	for i, n := range names {
		idx[n] = i
	}
	rel := func(seat int) string {
		r := ((seat - viewer) + nSeats) % nSeats
		if r == 0 {
			return "me"
		}
		return fmt.Sprintf("opp%d", r)
	}
	set := func(name string, cell int, v float32) {
		if i, ok := idx[name]; ok {
			data[i*w+cell] = v
		}
	}
	fill := func(name string, v float32) {
		v = clamp01(v)
		if v == 0 {
			return
		}
		if i, ok := idx[name]; ok {
			for c := 0; c < w; c++ {
				data[i*w+c] = v
			}
		}
	}
	goal := float32(e.game.ParamInt("doom_to_win", 30))
	for a := range e.m.ids {
		for s := 1; s <= nSeats; s++ {
			for _, typ := range e.typesIn(a, s) {
				set("unit/"+rel(s)+"/"+typ, a, clamp01(float32(e.count(a, s, typ))/float32(unitMax[typ])))
			}
		}
		if ctl, ok := d.Gates[e.areaID(a)]; ok {
			if ctl == 0 {
				set("gate/abandoned", a, 1)
			} else {
				set("gate/"+rel(ctl), a, 1)
			}
		}
		if d.Desec[e.areaID(a)] {
			set("desecrated", a, 1)
		}
		if e.m.ocean[a] {
			set("ocean", a, 1)
		}
	}
	for s := 1; s <= nSeats; s++ {
		r := rel(s)
		fill("power/"+r, float32(d.Power[s])/20)
		fill("doom/"+r, float32(d.Doom[s])/goal)
		fill("books/"+r, float32(d.bookCount(s))/6)
		fill("signs/"+r, float32(len(d.Signs[s]))/6)
		fill("gates/"+r, float32(e.gateCount(s))/6)
		fill("cultists/"+r, float32(e.cultistsInPlay(s))/6)
	}
	fill("ritual-cost", float32(e.ritualCost())/10)
	fill("round", float32(d.Round)/20)
	if toMove(d) == viewer {
		fill("to-move", 1)
	}
	if d.First == viewer {
		fill("first", 1)
	}
	fill("stage/"+d.Stage, 1)
	return data
}

// --- heuristic ----------------------------------------------------------------

// Heuristic values st for seat in [-1, 1]: Doom first, then Gates, Power,
// spellbooks, Elder Signs and the strength of its units.
func (rs *Ruleset) Heuristic(game *catalog.CompiledGame, st *state.BoardState, seat int) float64 {
	d, err := dataOf(st)
	if err != nil {
		return 0
	}
	if d.Phase == phaseOver {
		switch d.Winner {
		case seat:
			return 1
		case 0:
			return 0
		default:
			return -1
		}
	}
	e := rs.newEng(game, st, d)
	score := func(s int) float64 {
		v := float64(d.Doom[s]) * 1.0
		for _, x := range d.Signs[s] {
			v += float64(x) * 0.6 // hidden in public views: 0 then
		}
		v += float64(len(d.Signs[s])) * 0.8
		v += float64(e.gateCount(s)) * 3.0
		v += float64(d.Power[s]) * 0.25
		v += float64(d.bookCount(s)) * 1.5
		v += float64(e.cultistsInPlay(s)) * 0.4
		// Cultists with no guard beside an enemy monster are as good as
		// captured.
		v -= 1.2 * float64(e.exposedAreas(s))
		for _, f := range e.units(s) {
			n := e.onMap(s, f.ID)
			switch f.Class {
			case classMonster, classTerror:
				v += float64(n) * float64(f.Cost) * 0.35
			case classGOO:
				v += float64(n) * 5
			}
		}
		return v
	}
	mine := score(seat)
	best := 0.0
	for _, o := range e.others(seat) {
		if s := score(o); s > best {
			best = s
		}
	}
	diff := (mine - best) / 40
	if diff > 0.98 {
		diff = 0.98
	}
	if diff < -0.98 {
		diff = -0.98
	}
	return diff
}

// RepeatKey says where seat's units stand and which Gates it controls, so a
// bot's shuffles (a unit sent to and fro, a Gate taken and given up) show up
// as arrangements it has had before.
func (rs *Ruleset) RepeatKey(game *catalog.CompiledGame, st *state.BoardState, seat int) string {
	d, err := dataOf(st)
	if err != nil {
		return ""
	}
	e := rs.newEng(game, st, d)
	var b strings.Builder
	for a := range e.m.ids {
		for t := range unitTypes {
			if n := e.cnt[e.idx(a, seat, t)]; n > 0 {
				fmt.Fprintf(&b, "%d:%d:%d;", a, t, n)
			}
		}
	}
	b.WriteString("|")
	for _, a := range e.gatesOf(seat) {
		fmt.Fprintf(&b, "%d,", a)
	}
	return b.String()
}

// Repositions: moving a unit and taking or giving up a Gate only rearrange.
func (rs *Ruleset) Repositions(mv state.Move) bool {
	switch mv.Action {
	case actGo, actLeave, actControl, actAbandon:
		return true
	}
	return false
}
