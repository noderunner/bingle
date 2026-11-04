package courtfall

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

var (
	_ state.HiddenInfo    = (*Ruleset)(nil)
	_ state.Heuristic     = (*Ruleset)(nil)
	_ state.PlaneEncoder  = (*Ruleset)(nil)
	_ state.ActionLister  = (*Ruleset)(nil)
	_ state.TextRenderer  = (*Ruleset)(nil)
	_ state.Describer     = (*Ruleset)(nil)
	_ state.SeededRuleset = (*Ruleset)(nil)
	_ state.UndoGuard     = (*Ruleset)(nil)
	_ state.Searchable    = (*Ruleset)(nil)
)

// View is what clients draw beyond the units on hexes: tints, badges,
// counters and the pending decision. It is derived from the position and
// carried in the "view" key of a public position's ext payload, so every
// way of reading a position (state, events, history, simulation) has it.
type View struct {
	Phase  string `json:"phase"`
	Stage  string `json:"stage,omitempty"`
	ToMove int    `json:"to_move,omitempty"`
	// Prompt says what the position is waiting for.
	Prompt string `json:"prompt"`
	// Tints colours hexes by legend key; Badges marks hexes; Notes are
	// hover texts.
	Tints  map[string]string   `json:"tints,omitempty"`
	Badges map[string][]string `json:"badges,omitempty"`
	Notes  map[string]string   `json:"notes,omitempty"`
	// Seats holds each seat's scoreboard, keyed "1" and "2".
	Seats map[string]*SeatView `json:"seats"`
	// Active is the hex of the unit spending its actions.
	Active string `json:"active,omitempty"`
	// ActionsLeft counts its remaining actions.
	ActionsLeft int `json:"actions_left,omitempty"`
	// Contest is the exchange being resolved, or the last one resolved.
	Contest *ContestView `json:"contest,omitempty"`
	// TilesLeft counts the tiles still to draft, by color.
	TilesLeft map[string]int `json:"tiles_left,omitempty"`
}

// SeatView is one player's scoreboard.
type SeatView struct {
	Court      string        `json:"court,omitempty"`
	CourtLabel string        `json:"court_label,omitempty"`
	Counters   []CounterView `json:"counters"`
	// StoneReady is true when a Stone Court swap is available.
	StoneReady bool `json:"stone_ready,omitempty"`
}

// CounterView is a labelled number.
type CounterView struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value int    `json:"value"`
	Max   int    `json:"max,omitempty"`
}

// ContestView shows an exchange: both sides' dice once rolled.
type ContestView struct {
	Kind     string      `json:"kind"`
	Resolved bool        `json:"resolved"`
	Rolled   bool        `json:"rolled"`
	Sides    [2]SideView `json:"sides"`
}

// SideView is one side of a contest.
type SideView struct {
	Seat int    `json:"seat"`
	Unit string `json:"unit"`
	Cell string `json:"cell,omitempty"`
	// Committed says the side has chosen its split (the count may be
	// hidden: Attack is then -1).
	Committed bool      `json:"committed"`
	Attack    int       `json:"attack"` // dice on attack, -1 hidden/unknown
	Defense   int       `json:"defense"`
	AtkDice   []DieView `json:"atk_dice,omitempty"`
	DefDice   []DieView `json:"def_dice,omitempty"`
	AtkNeed   int       `json:"atk_need,omitempty"`
	DefNeed   int       `json:"def_need,omitempty"`
	AtkWins   int       `json:"atk_successes"`
	DefWins   int       `json:"def_successes"`
	Rerolled  bool      `json:"rerolled,omitempty"`
}

// DieView is one rolled die.
type DieView struct {
	V  int  `json:"v"`
	OK bool `json:"ok"`
}

func dieViews(vals []int, need int) []DieView {
	out := make([]DieView, len(vals))
	for i, v := range vals {
		out[i] = DieView{V: v, OK: v >= need}
	}
	return out
}

// Redact returns st as viewer may see it: the dice generator is hidden from
// everyone, a committed-but-unanswered attack split from everyone but its
// owner, and the derived view is attached.
func (rs *Ruleset) Redact(game *catalog.CompiledGame, st *state.BoardState, viewer int) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	e := rs.newEng(game, st, d)
	v := e.buildView(viewer)
	c := d.clone()
	c.RNG = 0
	if c.Con != nil && c.Stage == stageRespond && viewer != c.Con.ASeat {
		c.Con.AAtk = -1
	}
	out := st.Clone()
	out.SetExt(&publicExt{Data: c, View: v})
	return out
}

// WorthSearching is false during setup (tiles, Courts, deployment), where
// there is nothing for a few plies of search to measure.
func (rs *Ruleset) WorthSearching(game *catalog.CompiledGame, st *state.BoardState) bool {
	d, err := dataOf(st)
	return err == nil && d.Phase == phasePlay
}

// CanTakeBack forbids rewinding across a die roll: a player who has seen
// the dice must not get to choose again with that knowledge.
func (rs *Ruleset) CanTakeBack(game *catalog.CompiledGame, before, after *state.BoardState) bool {
	b, err1 := dataOf(before)
	a, err2 := dataOf(after)
	if err1 != nil || err2 != nil {
		return true
	}
	return b.RNG == a.RNG
}

// RedactMove hides an unanswered strike's committed dice in the move list.
func (rs *Ruleset) RedactMove(game *catalog.CompiledGame, st *state.BoardState, mv state.Move, notation string) (state.Move, string) {
	if mv.Action != actStrike {
		return mv, notation
	}
	d, err := dataOf(st)
	if err != nil || d.Phase != phasePlay || d.Stage != stageRespond || d.Con == nil {
		return mv, notation
	}
	mv.Count = -1
	return mv, rs.Format(game, mv)
}

// Determinize samples everything viewer cannot know: the dice generator and
// an opponent's committed split.
func (rs *Ruleset) Determinize(game *catalog.CompiledGame, st *state.BoardState, viewer int, rng *rand.Rand) *state.BoardState {
	d, err := dataOf(st)
	if err != nil {
		return st
	}
	c := d.clone()
	c.RNG = rng.Uint64() | 1
	if c.Con != nil && c.Stage == stageRespond && (viewer != c.Con.ASeat || c.Con.AAtk < 0) {
		tmp := rs.newEng(game, st, c)
		if a := tmp.unitByKey(c.Con.A); a != nil {
			c.Con.AAtk = rng.Intn(tmp.cv(a) + 1)
		}
	}
	out := st.Clone()
	out.SetExt(c)
	return out
}

func (e *eng) counters(seat int) []CounterView {
	return []CounterView{
		{ID: "momentum", Label: "Momentum", Value: e.mom(seat), Max: e.momCap},
		{ID: "vp", Label: "VP", Value: e.d.VP[seat-1], Max: e.vpWin},
	}
}

func (e *eng) buildView(viewer int) *View {
	d := e.d
	v := &View{
		Phase:  d.Phase,
		Stage:  d.Stage,
		ToMove: toMove(d),
		Prompt: e.prompt(),
		Seats:  map[string]*SeatView{},
	}
	if d.Phase == phaseTerrain || d.Phase == phaseDeploy {
		v.TilesLeft = map[string]int{}
		for i, c := range tileColors {
			v.TilesLeft[c] = d.Left[i]
		}
	}
	// Tints.
	v.Tints = map[string]string{}
	for c, t := range e.tiles {
		if t != 0 {
			v.Tints[e.cellName(c)] = tileName[t]
		}
	}
	if e.g.eye >= 0 && e.tiles[e.g.eye] == 0 {
		v.Tints[e.cellName(e.g.eye)] = "eye"
	}
	if d.Phase == phaseTerrain || d.Phase == phaseCourt || d.Phase == phaseDeploy {
		for c := range e.g.ids {
			if z := e.g.zone[c]; z != 0 {
				v.Tints[e.cellName(c)] = fmt.Sprintf("zone-%d", z)
			}
		}
	}
	// Seats.
	for s := 1; s <= 2; s++ {
		sv := &SeatView{Counters: e.counters(s)}
		if c := d.Courts[s-1]; c != "" {
			sv.Court = c
			sv.CourtLabel = courtLabel(c)
			sv.StoneReady = c == courtStone && !d.StoneUsed[s-1]
		}
		v.Seats[fmt.Sprint(s)] = sv
	}
	// Units: badges and notes.
	v.Badges = map[string][]string{}
	v.Notes = map[string]string{}
	for i := range e.units {
		u := &e.units[i]
		id := e.cellName(u.cell)
		switch e.cond(u) {
		case condFatigued:
			v.Badges[id] = append(v.Badges[id], "fatigued")
		case condExhausted:
			v.Badges[id] = append(v.Badges[id], "exhausted")
		}
		v.Notes[id] = e.describeUnit(u)
	}
	if d.Phase == phasePlay && d.Act != nil {
		if u := e.activeUnit(); u != nil {
			v.Active = e.cellName(u.cell)
			v.ActionsLeft = d.Act.Left
			v.Badges[v.Active] = append(v.Badges[v.Active], "active")
		}
	}
	if len(v.Badges) == 0 {
		v.Badges = nil
	}
	// Contest.
	con, resolved := d.Con, false
	if con == nil {
		con, resolved = d.Last, true
	}
	if con != nil && d.Phase != phaseTerrain {
		v.Contest = e.contestView(con, resolved, viewer)
	}
	return v
}

func (e *eng) contestView(c *Contest, resolved bool, viewer int) *ContestView {
	cv := &ContestView{Kind: c.Kind, Resolved: resolved, Rolled: c.Rolled}
	a, t := e.unitByKey(c.A), e.unitByKey(c.T)
	side := func(u *unit, key string, seat int) SideView {
		sv := SideView{Seat: seat, Attack: -1, Defense: -1}
		if u != nil {
			sv.Unit = e.label(u)
			sv.Cell = e.cellName(u.cell)
		}
		return sv
	}
	sa, st := side(a, c.A, c.ASeat), side(t, c.T, c.TSeat)
	sa.Committed = true
	switch c.Kind {
	case "strike":
		known := c.Rolled || resolved || viewer == c.ASeat
		if known && c.AAtk >= 0 {
			sa.Attack = c.AAtk
			if a != nil {
				sa.Defense = len(c.AF)
				if !c.Rolled {
					sa.Defense = e.cv(a) - c.AAtk
				}
			}
		}
		st.Committed = c.TAtk >= 0
		if c.TAtk >= 0 {
			st.Attack = c.TAtk
			st.Defense = len(c.TF)
		}
	case "shoot":
		sa.Attack = len(c.AD)
		st.Defense = len(c.TF)
	case "shove":
		sa.Attack = len(c.AD)
		st.Defense = len(c.TF)
	}
	if c.Rolled {
		sa.AtkDice, sa.AtkNeed = dieViews(c.AD, c.ADN), c.ADN
		sa.AtkWins = successes(c.AD, c.ADN)
		if c.Kind == "strike" {
			sa.DefDice, sa.DefNeed = dieViews(c.AF, c.AFN), c.AFN
			sa.DefWins = successes(c.AF, c.AFN)
			st.AtkDice, st.AtkNeed = dieViews(c.TD, c.TDN), c.TDN
			st.AtkWins = successes(c.TD, c.TDN)
		}
		st.DefDice, st.DefNeed = dieViews(c.TF, c.TFN), c.TFN
		st.DefWins = successes(c.TF, c.TFN)
	}
	sa.Rerolled, st.Rerolled = c.Used[0], c.Used[1]
	cv.Sides = [2]SideView{sa, st}
	return cv
}

// --- text -------------------------------------------------------------------

// Text draws the position as monospace text for terminals and models.
func (rs *Ruleset) Text(game *catalog.CompiledGame, st *state.BoardState) string {
	d, err := dataOf(st)
	if err != nil {
		return "(no position)"
	}
	e := rs.newEng(game, st, d)
	g := e.g
	var b strings.Builder

	cell := func(c int) string {
		if u := e.unitAt(c); u != nil {
			s := string(e.color[u.seat][0]) + unitCode[u.piece]
			switch e.cond(u) {
			case condFatigued:
				s += "'"
			case condExhausted:
				s += "!"
			}
			return s
		}
		switch e.tiles[c] {
		case 1:
			return "g"
		case 2:
			return "m"
		case 3:
			return "^"
		}
		if c == g.eye {
			return "*"
		}
		return "."
	}

	span := 7
	fmt.Fprintf(&b, "   ")
	for q := 0; q < span; q++ {
		fmt.Fprintf(&b, "%-4s", string(rune('a'+q)))
	}
	b.WriteByte('\n')
	for r := 0; r < span; r++ {
		fmt.Fprintf(&b, "%2d %s", r+1, strings.Repeat("  ", r))
		for q := 0; q < span; q++ {
			id, ok := game.Board.At(board.Coord{X: q, Y: r})
			if !ok {
				b.WriteString("    ")
				continue
			}
			fmt.Fprintf(&b, "%-4s", cell(g.index[id]))
		}
		b.WriteByte('\n')
	}
	b.WriteString("Legend: . open  g Overgrowth  m Mire  ^ Hazard  * the Eye;  w/b = Light/Dark, P N B R Q;  ' Fatigued  ! Exhausted\n")
	for s := 1; s <= 2; s++ {
		fmt.Fprintf(&b, "%s (%s): Court %s, Momentum %d/%d, VP %d/%d\n", e.seatLabel(s), e.color[s],
			courtOrNone(d.Courts[s-1]), e.mom(s), e.momCap, d.VP[s-1], e.vpWin)
		var parts []string
		for _, u := range e.ownUnits(s) {
			cond := "Fresh"
			switch e.cond(u) {
			case condFatigued:
				cond = "Fatigued"
			case condExhausted:
				cond = "Exhausted"
			}
			parts = append(parts, fmt.Sprintf("%s %s@%s %s CV%d", unitCode[u.piece], unitName[u.piece], e.cellName(u.cell), cond, e.cv(u)))
		}
		if len(parts) > 0 {
			b.WriteString("  " + strings.Join(parts, "; ") + "\n")
		}
	}
	if d.Phase == phaseTerrain || d.Phase == phaseDeploy {
		fmt.Fprintf(&b, "Tiles left to draft: Overgrowth %d, Mire %d, Hazard %d\n", d.Left[0], d.Left[1], d.Left[2])
	}
	fmt.Fprintf(&b, "Phase: %s. %s\n", d.Phase, e.prompt())
	con, label := d.Con, "Dice on the table"
	if con == nil {
		con, label = d.Last, "Dice of the last contest"
	}
	if c := con; c != nil && c.Rolled && len(c.AD)+len(c.AF)+len(c.TD)+len(c.TF) > 0 {
		a, t := e.unitByKey(c.A), e.unitByKey(c.T)
		an, tn := "attacker", "target"
		if a != nil {
			an = e.label(a)
		}
		if t != nil {
			tn = e.label(t)
		}
		fmt.Fprintf(&b, "%s (%s): %s attack %v (%d+) defense %v (%d+); %s attack %v (%d+) defense %v (%d+)\n",
			label, c.Kind, an, c.AD, c.ADN, c.AF, c.AFN, tn, c.TD, c.TDN, c.TF, c.TFN)
	}
	if len(d.Events) > 0 {
		b.WriteString("Last move:\n")
		for _, ev := range d.Events {
			b.WriteString("  " + ev + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func courtOrNone(c string) string {
	if c == "" {
		return "(none yet)"
	}
	return c
}

// --- planes -----------------------------------------------------------------

var (
	planePhases = []string{phaseTerrain, phaseCourt, phaseDeploy, phasePlay}
	planeStages = []string{stageRecover, stageMain, stageAct, stageRespond, stageReroll, stagePush, stageShift, stageSeize}
	planeRel    = []string{"me", "opp"}
)

// PlaneLayout names the channels: units, conditions and terrain as boards,
// then scalars as constant planes.
func (rs *Ruleset) PlaneLayout(game *catalog.CompiledGame) ([]string, int, int) {
	var names []string
	for _, r := range planeRel {
		for _, p := range unitTypes {
			names = append(names, "unit/"+r+"/"+p)
		}
	}
	for _, r := range planeRel {
		names = append(names, "cond/"+r+"/fatigued", "cond/"+r+"/exhausted")
	}
	names = append(names, "tile/green", "tile/blue", "tile/red", "eye", "zone/me", "zone/opp", "active")
	names = append(names, "momentum/me", "momentum/opp", "vp/me", "vp/opp")
	for _, p := range planePhases {
		names = append(names, "phase/"+p)
	}
	for _, s := range planeStages {
		names = append(names, "stage/"+s)
	}
	names = append(names, "to-move", "actions-left", "contest/attack-dice", "stone-ready/me", "streak", "ply",
		"tiles-left/green", "tiles-left/blue", "tiles-left/red")
	for _, r := range planeRel {
		for _, c := range courts {
			names = append(names, "court/"+r+"/"+c)
		}
	}
	return names, 7, 7
}

// PlaneCells lists the hex at each plane cell (row-major, 7×7).
func (rs *Ruleset) PlaneCells(game *catalog.CompiledGame) []string {
	out := make([]string, 49)
	for _, id := range game.Board.Positions() {
		if c, ok := game.Board.Coord(id); ok && c.X >= 0 && c.X < 7 && c.Y >= 0 && c.Y < 7 {
			out[c.Y*7+c.X] = string(id)
		}
	}
	return out
}

// EncodePlanes encodes st for viewer ("me" is the viewer).
func (rs *Ruleset) EncodePlanes(game *catalog.CompiledGame, st *state.BoardState, viewer int) []float32 {
	names, _, _ := rs.PlaneLayout(game)
	const cells = 49
	data := make([]float32, len(names)*cells)
	d, err := dataOf(st)
	if err != nil {
		return data
	}
	e := rs.newEng(game, st, d)
	idx := map[string]int{}
	for i, n := range names {
		idx[n] = i
	}
	cellPos := func(c int) int {
		co, _ := game.Board.Coord(e.g.ids[c])
		return co.Y*7 + co.X
	}
	set := func(name string, c int, v float32) {
		if i, ok := idx[name]; ok {
			data[i*cells+cellPos(c)] = v
		}
	}
	fill := func(name string, v float32) {
		if v == 0 {
			return
		}
		if i, ok := idx[name]; ok {
			for j := 0; j < cells; j++ {
				data[i*cells+j] = v
			}
		}
	}
	rel := func(seat int) string {
		if seat == viewer {
			return "me"
		}
		return "opp"
	}
	for i := range e.units {
		u := &e.units[i]
		r := rel(u.seat)
		set("unit/"+r+"/"+u.piece, u.cell, 1)
		switch e.cond(u) {
		case condFatigued:
			set("cond/"+r+"/fatigued", u.cell, 1)
		case condExhausted:
			set("cond/"+r+"/exhausted", u.cell, 1)
		}
	}
	for c := range e.g.ids {
		if t := e.tiles[c]; t != 0 {
			set("tile/"+tileName[t], c, 1)
		}
		if c == e.g.eye {
			set("eye", c, 1)
		}
		if z := int(e.g.zone[c]); z != 0 {
			set("zone/"+rel(z), c, 1)
		}
	}
	other := 3 - viewer
	fill("momentum/me", float32(e.mom(viewer))/float32(e.momCap))
	fill("momentum/opp", float32(e.mom(other))/float32(e.momCap))
	fill("vp/me", float32(d.VP[viewer-1])/float32(e.vpWin))
	fill("vp/opp", float32(d.VP[other-1])/float32(e.vpWin))
	fill("phase/"+d.Phase, 1)
	if d.Phase == phasePlay {
		fill("stage/"+d.Stage, 1)
		if toMove(d) == viewer {
			fill("to-move", 1)
		}
		if d.Act != nil {
			fill("actions-left", float32(d.Act.Left)/2)
			if u := e.activeUnit(); u != nil {
				set("active", u.cell, 1)
			}
		}
		fill("streak", float32(d.Streak)/2)
	} else if toMove(d) == viewer {
		fill("to-move", 1)
	}
	if c := d.Con; c != nil && (viewer == c.ASeat || c.Rolled) && c.AAtk >= 0 {
		fill("contest/attack-dice", float32(c.AAtk)/4)
	}
	if d.Courts[viewer-1] == courtStone && !d.StoneUsed[viewer-1] {
		fill("stone-ready/me", 1)
	}
	fill("ply", float32(st.Ply())/float32(e.maxPlies))
	fill("tiles-left/green", float32(d.Left[0])/4)
	fill("tiles-left/blue", float32(d.Left[1])/3)
	fill("tiles-left/red", float32(d.Left[2])/3)
	for s := 1; s <= 2; s++ {
		if c := d.Courts[s-1]; c != "" {
			fill("court/"+rel(s)+"/"+c, 1)
		}
	}
	return data
}

// --- heuristic ----------------------------------------------------------------

// Heuristic values st for seat in [-1, 1]: victory points first, then
// banked Momentum, healthy units and the Eye.
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
	me, opp := seat, 3-seat
	score := float64(d.VP[me-1]-d.VP[opp-1]) * 0.17
	score += float64(d.Mom[me-1]-d.Mom[opp-1]) * 0.045
	health := func(s int) float64 {
		h := 0.0
		for _, u := range e.ownUnits(s) {
			w := 1.0
			if u.piece == "queen" {
				w = 1.5
			}
			switch e.cond(u) {
			case condFresh:
				h += w
			case condFatigued:
				h += w * 0.5
			}
		}
		return h
	}
	score += (health(me) - health(opp)) * 0.035
	if e.g.eye >= 0 {
		if u := e.unitAt(e.g.eye); u != nil && e.cond(u) != condExhausted {
			if u.seat == me {
				score += 0.04
			} else {
				score -= 0.04
			}
		}
	}
	if score > 0.98 {
		score = 0.98
	}
	if score < -0.98 {
		score = -0.98
	}
	return score
}
