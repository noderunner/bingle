package cthulhuwars

import (
	"fmt"
	"sort"
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset implements Cthulhu Wars (the core set's four factions) for the
// state package.
type Ruleset struct{}

// New returns the Cthulhu Wars ruleset.
func New() *Ruleset { return &Ruleset{} }

var _ state.Ruleset = (*Ruleset)(nil)

// Action ids carried by moves.
const (
	actRecruit   = "recruit"
	actSummon    = "summon"
	actAwaken    = "awaken"
	actGate      = "gate"
	actGo        = "go"    // moves one unit during a Move action
	actLeave     = "leave" // the unit that stands on a Gate moves away (or steps off): the Gate is abandoned
	actBattle    = "battle"
	actCapture   = "capture"
	actControl   = "control"
	actAbandon   = "abandon"
	actPass      = "pass"
	actDone      = "done"
	actReveal    = "reveal"
	actPay       = "pay"
	actSacrifice = "sacrifice"
	actSubmerge  = "submerge"
	actBring     = "bring" // a unit that sinks with Cthulhu
	actEmerge    = "emerge"
	actDreams    = "dreams"
	actDevolve   = "devolve"
	actForms     = "forms"
	actGhroth    = "ghroth"
	actScream    = "scream"
	actShriek    = "shriek"
	actZingaya   = "zingaya"
	actDesecrate = "desecrate"
	actHWINN     = "hwinn"
	actRitual    = "ritual"
	actSkip      = "skip"
	actBlood     = "blood"
	actAbsorb    = "absorb"
	actAbduct    = "abduct"
	actInvisible = "invisible"
	actSeek      = "seek"
	actNecro     = "necro"
	actAssign    = "assign"
	actRetreat   = "retreat"
	actLose      = "lose"
	actBook      = "book"
	actCW        = "cw"
	actCCW       = "ccw"

	actAvatar    = "avatar"    // Shub-Niggurath swaps places with a unit in an area
	actSwap      = "swap"      // the unit that takes Shub-Niggurath's place
	actPlace     = "place"     // Ghroth: put a faction's Cultist in an area
	actGrant     = "grant"     // Yellow Sign's requirement: another player gains 3 Doom
	actHarbinger = "harbinger" // Power or Elder Signs for a Great Old One that was hit

	// Windwalker
	actStart     = "start"     // choose the sigil area the cult begins in
	actHibernate = "hibernate" // bank the Power, take no more Actions this phase
	actIceAge    = "iceage"    // put the Ice Age token in an area
	actClaim     = "claim"     // take the spellbook whose requirement is "at any time"
	actHowl      = "howl"      // force the enemy to retreat a unit before the battle
	actCannibal  = "cannibal"  // place a Wendigo or Acolyte after a battle
	actEternal   = "eternal"   // pay 1 Power: Rhan-Tegoth shrugs a result off

	// Sleeper
	actLethargy = "lethargy" // do nothing, as an Action
	actBoon     = "boon"     // requirement: every other player gains 1 Power
	actBestow   = "bestow"   // requirement: one player gains 3 Power
	actBane     = "bane"     // requirement: every other player loses 1 Power
	actSorcery  = "sorcery"  // put a Serpent Man on an enemy's faction card
	actRestore  = "restore"  // where the Serpent Man returns
	actSlumber  = "slumber"  // put a Gate and its Cultist to sleep
	actWake     = "wake"     // put them back on the map
	actDeath    = "death"    // Death from Below, in the Doom phase
	actDemand   = "demand"   // the enemy's answer to Demand Sacrifice
	actNexus    = "nexus"    // take an Action just before a battle

	// High Priests
	actOath = "oath" // Unspeakable Oath: sacrifice the High Priest for 2 Power

	// Opener of the Way
	actBeyond  = "beyond"  // The Beyond One: move a unit, a Gate and its controller
	actAscend  = "ascend"  // Dragon Ascending: set Power to an enemy's
	actChannel = "channel" // Channel Power: reroll the dice that were not hits
	actUpgrade = "upgrade" // The Million Favored Ones: replace a unit with the next kind
	actCurse   = "curse"   // Dread Curse of Azathoth

	// Tcho-Tcho
	actSyco      = "syco"      // Sycophancy: give the Tcho-Tcho Doom, or earn less
	actHiero     = "hiero"     // Hierophants: where the High Priest goes
	actTerror    = "terror"    // Terror: lower the enemy's Combat or raise one's own
	actIdolatry  = "idolatry"  // call units into an area with another cult's glyph
	actDismantle = "dismantle" // remove the Gate in the start area

	// Azathoth: neutral spellbooks and Monsters
	actShrivel   = "shrivel"       // Shriveling: eliminate an enemy Monster or Cultist before a battle
	actRecrim    = "recriminate"   // Recriminations: swap a spellbook for another
	actUndim     = "undimensioned" // Undimensioned: begin rearranging the units
	actRearrange = "rearrange"     // Undimensioned: move one unit
	actMao       = "mao"           // The Mao Ceremony: sacrifice a Cultist for Power
	actBuy       = "buy"           // buy a neutral Monster's Loyalty Card with Doom
	actWalk      = "walk"          // Walk Between Worlds: put a Shambler on the map

	// Dreamlands Surface Monsters, Beyond Time and Space
	actMoon   = "moon"   // Blasphemous Obeisance: lay a Moonbeast on an enemy spellbook (or, in a Doom phase, put it on a Gate)
	actRansom = "ransom" // pay 1 Doom to free a spellbook from its Moonbeast
	actCrypt  = "crypt"  // Crypt Dweller: choose the enemy that places the Wamps, then the areas

	// Independent Great Old Ones
	actFilth   = "filth"   // Abhoth: place a Filth Token in any area
	actSting   = "sting"   // Mother Hydra: The Agony Sting empties an ocean of Cultists (then each Cultist's area)
	actZygote  = "zygote"  // The Zygote: place an Acolyte from the pool
	actDiscard = "discard" // Chaugnar Faugn's requirement: discard 2 Elder Signs
	actGateOff = "gateoff" // Yig's requirement: remove one of your controlled Gates
	actSpare   = "spare"   // Fire Vampires: spare a unit whose Kill becomes a Pain
	actPick    = "pick"    // Curse of Chaugnar Faugn: the value of an Elder Sign
	actDonate  = "donate"  // Messenger of Yig: give 1 Power, or refuse
)

// Seeded reports that the start position carries a seed to record.
func (*Ruleset) Seeded() bool { return true }

// Start builds the opening position: every cult's start area holds a
// controlled Gate and its six Cultists.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	st := state.NewBoardState()
	if seed == 0 {
		seed = 0x2545f4914f6cdd1d
	}
	startPower := game.ParamInt("start_power", 8)
	cfg := cfgOf(game)
	d := &Data{
		Phase: phaseAction, Round: 1, First: cfg.first, Dir: 1, Players: cfg.n,
		Gates: map[string]int{}, Desec: map[string]bool{},
		Bag: [3]int{18, 12, 6}, RNG: seed,
		Opening: true,
	}
	if l := game.Definition.Params["elder_signs"]; l != nil {
		if arr, ok := l.([]any); ok && len(arr) == 3 {
			for i, v := range arr {
				if f, ok := v.(float64); ok {
					d.Bag[i] = int(f)
				}
			}
		}
	}
	e := rs.newEng(game, st, d)
	var setup []int
	for seat := 1; seat <= e.n; seat++ {
		f := e.f(seat)
		for _, u := range e.units(seat) {
			st.SetReserve(f.Color, u.ID, u.Count)
		}
		d.Power[seat] = startPower
		if e.setsUpLater(seat) {
			// The Tcho-Tcho, the Windwalker and the Opener of the Way set up
			// after the others, in an area they choose.
			setup = append(setup, seat)
			continue
		}
		start := startArea(game, *f)
		a := e.areaIdx(start)
		// Bubastis begins with six Earth Cats on the Moon, which is its Gate.
		if e.is(seat, facBU) {
			for i := 0; i < 6; i++ {
				e.place(a, seat, uEarthCat)
			}
			d.Gates[moonArea] = seat
			continue
		}
		for i := 0; i < 6; i++ {
			e.place(a, seat, uCultist)
		}
		e.occupy(a, seat, uCultist)
	}
	if len(setup) > 0 {
		sort.SliceStable(setup, func(i, j int) bool { return setupRank(e.fac(setup[i])) < setupRank(e.fac(setup[j])) })
		d.Setup, d.SetupLater = setup[0], setup[1:]
		d.Stage = stageSetup
	} else {
		// The first player chooses the direction of play before the first
		// Action phase (the Doom phase is skipped in the first round).
		d.Stage = stageDirection
	}
	st.SetExt(d)
	return st, nil
}

// startArea is where a faction begins: the layout's params.starts entry for
// its key, else the first area marked with its sigil, else the faction's own
// default.
func startArea(game *catalog.CompiledGame, f faction) string {
	if m, ok := game.Definition.Params["starts"].(map[string]any); ok {
		if id, ok := m[f.Key].(string); ok && id != "" {
			return id
		}
	}
	if idx := factionIndex(f.Key); idx >= 0 {
		if l := cfgOf(game).sigils[idx]; len(l) > 0 {
			return l[0]
		}
	}
	return f.Start
}

// ImpulseSeat says whose decision the position waits for.
func (rs *Ruleset) ImpulseSeat(game *catalog.CompiledGame, st *state.BoardState) int {
	d, err := dataOf(st)
	if err != nil {
		return 0
	}
	return impulseSeat(d)
}

func impulseSeat(d *Data) int {
	if d.Phase == phaseOver {
		return 0
	}
	switch d.Stage {
	case stageSetup:
		return d.Setup
	case stageTurn, stageMoving:
		if d.Turn != nil {
			return d.Turn.Seat
		}
	case stagePre, stagePost:
		if d.Bat != nil {
			return d.Bat.Who
		}
	case stageAssign:
		if d.Bat != nil && len(d.Bat.Queue) > 0 {
			return d.Bat.Queue[0].Who
		}
	case stageRetreat:
		if d.Bat != nil && len(d.Bat.Pained) > 0 {
			if d.Bat.Retreater != 0 {
				return d.Bat.Retreater
			}
			return d.Bat.Pained[0].Seat
		}
	case stagePrompt:
		if len(d.Prompts) > 0 {
			return d.Prompts[0].Seat
		}
	case stageDirection:
		return d.First
	case stageFirst:
		return d.First // the previous first player decides
	case stageDoom:
		if d.Doomx != nil && len(d.Doomx.Order) > 0 {
			return d.Doomx.Order[0]
		}
	}
	return 0
}

// Outcome reports a finished game.
func (rs *Ruleset) Outcome(game *catalog.CompiledGame, st *state.BoardState) state.Outcome {
	d, err := dataOf(st)
	if err != nil || d.Phase != phaseOver {
		return state.Outcome{}
	}
	out := state.Outcome{Ended: true, Reason: d.Reason, Detail: doomLine(rs.newEng(game, st, d))}
	if d.Winner != 0 {
		w := d.Winner
		out.Winner = &w
	}
	return out
}

func doomLine(e *eng) string {
	var parts []string
	for _, s := range e.playing() {
		parts = append(parts, fmt.Sprintf("%s %d", e.seatName(s), e.d.Doom[s]))
	}
	return "Doom: " + strings.Join(parts, ", ")
}

// Validate checks a position document.
func (rs *Ruleset) Validate(game *catalog.CompiledGame, st *state.BoardState) error {
	d, err := dataOf(st)
	if err != nil {
		return err
	}
	switch d.Phase {
	case phaseAction, phaseDoom, phaseOver:
	default:
		return fmt.Errorf("cthulhuwars: unknown phase %q", d.Phase)
	}
	m := mapOf(game)
	for id, ctl := range d.Gates {
		if _, ok := m.index[boardID(id)]; !ok {
			return fmt.Errorf("cthulhuwars: gate on unknown area %q", id)
		}
		if ctl < 0 || ctl > nSeats {
			return fmt.Errorf("cthulhuwars: gate controlled by seat %d", ctl)
		}
	}
	for id := range d.Desec {
		if _, ok := m.index[boardID(id)]; !ok {
			return fmt.Errorf("cthulhuwars: desecration on unknown area %q", id)
		}
	}
	for s := 1; s <= nSeats; s++ {
		if d.Power[s] < 0 || d.Doom[s] < 0 {
			return fmt.Errorf("cthulhuwars: negative power or doom for seat %d", s)
		}
	}
	// Unit conservation: every unit is on the map, in the pool, held as a
	// captured cultist or submerged.
	e := rs.newEng(game, st, d)
	for _, seat := range e.playing() {
		f := e.f(seat)
		for _, u := range e.units(seat) {
			n := e.onMap(seat, u.ID) + e.pool(seat, u.ID)
			switch u.ID {
			case uCultist:
				for by := 1; by <= nSeats; by++ {
					n += d.Captured[by][seat]
				}
			case uHighPriest:
				for by := 1; by <= nSeats; by++ {
					n += d.CapHP[by][seat]
				}
			}
			if e.is(seat, facGC) {
				n += d.Away[u.ID]
			}
			if u.ID == uShambler {
				n += d.Card[seat]
			}
			if u.ID == uMoonbeast {
				for _, m := range d.Moon {
					if m.Owner == seat {
						n++
					}
				}
			}
			if u.ID == uFilth {
				// The Tokens left on the map by a former owner of Abhoth are
				// still in play.
				for _, o := range e.playing() {
					if o != seat {
						n += e.onMap(o, uFilth)
					}
				}
			}
			if d.Slumber != nil && e.is(seat, facSL) && d.Slumber.Cultist == u.ID {
				n++
			}
			if e.is(seat, facSL) && u.ID == uSerpent {
				n += len(d.Sorcery)
			}
			n += d.Gone[fmt.Sprintf("%d|%s", seat, u.ID)]
			if u.ID == uAsteroid {
				for _, l := range d.Lust {
					if l.Owner == seat {
						n++
					}
				}
			}
			for _, h := range d.Held {
				if h.Seat == seat && h.Type == u.ID {
					n++
				}
			}
			if n != u.Count {
				return fmt.Errorf("cthulhuwars: %s has %d %s accounted for, want %d", f.Name, n, u.ID, u.Count)
			}
		}
	}
	return nil
}

// --- Apply -------------------------------------------------------------------

// Apply validates and applies one decision.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Move) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	if d.Phase == phaseOver {
		return nil, fmt.Errorf("cthulhuwars: the game is over")
	}
	want := impulseSeat(d)
	if seat != want {
		return nil, fmt.Errorf("cthulhuwars: it is not seat %d's decision (waiting for seat %d)", seat, want)
	}
	ns := st.Clone()
	nd := d.clone()
	ns.SetExt(nd)
	e := rs.newEng(game, ns, nd)

	var legal state.Move
	found := false
	for _, cand := range e.legal() {
		if sameMove(cand, mv) {
			legal, found = cand, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("cthulhuwars: %q is not legal now — %s", rs.Format(game, mv), e.prompt())
	}
	e.execute(seat, legal)
	nd.Events = e.ev
	ns.SetPly(st.Ply() + 1)
	e.logDecision(st.Ply()+1, seat)

	res := &state.MoveResult{NewState: ns, Move: legal, Notation: rs.Format(game, legal)}
	if nd.Phase == phaseOver {
		res.GameEnded = true
		res.NextImpulseSeat = seat
		if nd.Winner != 0 {
			w := nd.Winner
			res.WinnerSeat = &w
		}
		res.EndReason = nd.Reason
		res.EndDetail = doomLine(e)
	} else {
		res.NextImpulseSeat = impulseSeat(nd)
	}
	return res, nil
}

// Legal lists seat's decisions.
func (rs *Ruleset) Legal(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.Move {
	d, err := dataOf(st)
	if err != nil || d.Phase == phaseOver || impulseSeat(d) != seat {
		return nil
	}
	return rs.newEng(game, st, d).legal()
}

// sameMove reports whether two moves are the same decision: a decision is
// identified by its action, piece, squares and count.
func sameMove(a, b state.Move) bool {
	return a.Action == b.Action && a.Piece == b.Piece && a.From == b.From && a.To == b.To && a.Count == b.Count
}

func (e *eng) legal() []state.Move {
	d := e.d
	switch d.Stage {
	case stageSetup:
		return e.legalSetup()
	case stageTurn:
		return e.legalTurn()
	case stageMoving:
		return e.legalMoving()
	case stagePre:
		return e.legalPre()
	case stagePost:
		return e.legalPost()
	case stageAssign:
		return e.legalAssign()
	case stageRetreat:
		return e.legalRetreat()
	case stagePrompt:
		return e.legalPrompt()
	case stageDirection:
		return []state.Move{{Action: actCW}, {Action: actCCW}}
	case stageFirst:
		return e.legalFirst()
	case stageDoom:
		return e.legalDoom()
	}
	return nil
}

// execute applies a legal move.
func (e *eng) execute(seat int, mv state.Move) {
	d := e.d
	e.ev = nil
	e.noteAim(seat, mv)
	switch d.Stage {
	case stageSetup:
		e.doSetup(seat, mv)
	case stageTurn:
		e.doTurn(seat, mv)
	case stageMoving:
		e.doMoving(seat, mv)
	case stagePre:
		e.doPre(seat, mv)
	case stagePost:
		e.doPost(seat, mv)
	case stageAssign:
		e.doAssign(seat, mv)
	case stageRetreat:
		e.doRetreat(seat, mv)
	case stagePrompt:
		e.doPrompt(seat, mv)
	case stageFirst:
		e.setFirst(mv.Count)
	case stageDirection:
		if d.Opening {
			if e.is(d.First, facGC) {
				e.say("%s is the first player: Great Cthulhu always begins.", e.seatName(d.First))
			} else {
				e.say("%s is the first player: without Great Cthulhu the cult first by name begins.", e.seatName(d.First))
			}
		}
		d.Dir = 1
		if mv.Action == actCCW {
			d.Dir = -1
		}
		e.say("%s chooses to play %s: %s.", e.seatName(d.First), dirName(d.Dir), e.orderNames(d.First, d.Dir))
		if d.Opening {
			e.beginFirstAction()
			break
		}
		e.startDoomPhase()
	case stageDoom:
		e.doDoom(seat, mv)
	}
	// Curse of Chaugnar Faugn: the holder says which Elder Signs were given.
	if len(d.Picks) > 0 {
		e.openPicks()
	}
	e.cardSweep()
	e.bubastisSweep()
	if len(d.Pending) > 0 {
		e.openPending()
	}
	if d.EndNow && d.Bat == nil && d.Stage != stagePrompt && d.Phase != phaseOver {
		d.EndNow = false
		e.finishGame("instant-death")
	}
	// After another player's action the players holding reactions are asked.
	if d.ReactOwed != 0 {
		switch {
		case d.Phase != phaseAction || d.Turn == nil:
			d.ReactOwed = 0 // the phase moved on: nothing is left to react to
		case d.Turn.Nexus:
			// An Energy Nexus Action is part of a battle: the windows wait for its end.
		case d.Stage == stageTurn && len(d.Prompts) == 0:
			e.openReactions("react")
		}
	}
}

// dirName names a direction of play.
func dirName(dir int) string {
	if dir < 0 {
		return "counter-clockwise"
	}
	return "clockwise"
}

// orderNames spells out the turn order from first in direction dir:
// "Black Goat → Yellow Sign → Great Cthulhu → Crawling Chaos".
func (e *eng) orderNames(first, dir int) string {
	var names []string
	for _, s := range e.turnOrder(first, dir) {
		names = append(names, e.seatName(s))
	}
	return strings.Join(names, " → ")
}

// prompt says what the position is waiting for, in a sentence, naming who.
func (e *eng) prompt() string {
	d := e.d
	if d.Phase == phaseOver {
		if d.Winner == 0 {
			return "The game is over: humanity wins — or nobody does."
		}
		return fmt.Sprintf("%s wins.", e.seatName(d.Winner))
	}
	seat := impulseSeat(d)
	if seat <= 0 {
		return e.impulse()
	}
	who := e.seatName(seat)
	// A client that reads only this sentence still needs what the card's buttons
	// and counters show: the two orders of play, and the Power there is to spend.
	switch d.Stage {
	case stageDirection:
		lead := "is the first player"
		if d.Opening {
			lead = "is the first player of the game"
		}
		return fmt.Sprintf("%s %s: choose the direction of play — clockwise (%s) or counter-clockwise (%s).",
			who, lead, e.orderNames(d.First, 1), e.orderNames(d.First, -1))
	case stageTurn:
		return fmt.Sprintf("%s: %s (Power %d).", who, strings.TrimSuffix(e.impulse(), "."), d.Power[seat])
	}
	return who + ": " + e.impulse()
}

// impulse says what the seat on impulse is being asked to choose. Everyone at the
// table is shown the same words, under that seat's name.
func (e *eng) impulse() string {
	d := e.d
	seat := impulseSeat(d)
	if d.Phase == phaseOver {
		return ""
	}
	switch d.Stage {
	case stageSetup:
		switch e.fac(seat) {
		case facTT:
			return "Choose an empty area marked with a faction glyph where your cult begins."
		case facOW:
			return "Choose an empty area where your cult begins — after all the others have set up."
		case facAN:
			return "Choose an empty area with no start glyph where your cult begins."
		}
		return "Choose the area marked with your sigil where your cult begins."
	case stageTurn:
		if d.Turn != nil && d.Turn.Nexus {
			return fmt.Sprintf("Energy Nexus — take an Action that starts in %s before the battle, or press Done.", e.areaName(e.areaIdx(d.Bat.Area)))
		}
		if d.Turn != nil && d.Turn.Main {
			return "Choose an action, or end your turn."
		}
		if d.Turn != nil && e.summonedThisTurn(seat) {
			return "Summon more Monsters, choose another action, or end your turn."
		}
		return "Choose an action."
	case stageMoving:
		return e.moveSummary()
	case stagePre:
		return "Choose a pre-battle ability or pass."
	case stagePost:
		return "Choose a post-battle ability or pass."
	case stageAssign:
		k := "Kill"
		if len(d.Bat.Queue) > 0 && d.Bat.Queue[0].Pain {
			k = "Pain"
		}
		target := ""
		if len(d.Bat.Queue) > 0 {
			target = e.seatName(d.Bat.Queue[0].Seat)
		}
		return fmt.Sprintf("Assign a %s result to one of %s's units.", k, target)
	case stageRetreat:
		return "Choose where a pained unit retreats."
	case stagePrompt:
		if len(d.Prompts) > 0 {
			p := d.Prompts[0]
			if p.Kind == "card" {
				return e.cardImpulse(p)
			}
			if p.Kind == "book" {
				if p.By != 0 {
					return fmt.Sprintf("Abandoned to Lusts — choose the spellbook %s must take.", e.seatName(p.By))
				}
				if p.Skip != 0 {
					return "Recriminations — choose the spellbook that takes the place of the one you set aside."
				}
				return "You earned a spellbook — choose which one."
			}
			switch p.Kind {
			case "submerge":
				return "Choose the units that sink with Cthulhu — click each one, then press Done."
			case "scream":
				return "Choose the Undead that walk with the King in Yellow — click each one, then press Done."
			case "shriek":
				return fmt.Sprintf("Choose the Byakhee that fly to %s — click each one, then press Done.", e.areaName(e.areaIdx(p.Area)))
			case "avatar":
				return fmt.Sprintf("Shub-Niggurath swaps places with one of your units in %s — choose which.", e.areaName(e.areaIdx(p.Area)))
			case "place":
				return "Ghroth places an Acolyte anywhere on the map — choose whose, then the area."
			case "react":
				if p.Why == "oath" {
					if p.By == 0 {
						return "Unspeakable Oath — sacrifice your High Priest for 2 Power before the first player is chosen, or press Pass."
					}
					return fmt.Sprintf("Unspeakable Oath is available after %s's action — sacrifice your High Priest for 2 Power, or press Pass.", e.seatName(p.By))
				}
				if p.Why == "walk" {
					return "Walk Between Worlds — place a Dimensional Shambler from your faction card in any area, or press Pass."
				}
				return fmt.Sprintf("%s is available after %s's action — click a Cultist to replace it with a Deep One, or press Pass.", reactionName(p.Why), e.seatName(p.By))
			case "channel":
				return "Channel Power — pay 1 Power to reroll every die that was not a Kill or a Pain, as often as you like, or press Done."
			case "curse-target":
				return fmt.Sprintf("Dread Curse — choose the enemy that takes this %s.", map[bool]string{true: "Pain", false: "Kill"}[e.d.Curse != nil && e.d.Curse.Pain])
			case "curse-hit":
				return fmt.Sprintf("Dread Curse — a %s falls on you in %s: choose which of your units takes it.", map[bool]string{true: "Pain", false: "Kill"}[e.d.Curse != nil && e.d.Curse.Pain], e.areaName(e.areaIdx(p.Area)))
			case "curse-move":
				return "Dread Curse — choose the area the Pained unit is driven to."
			case "hiero":
				return "Hierophants — place a High Priest at one of your Gates."
			case "idol":
				return fmt.Sprintf("Idolatry — click a unit in an area beside %s to bring it (and the ones with it) there, or all of them; press Done when you have finished.", e.areaName(e.areaIdx(p.Area)))
			case "syco":
				return "Sycophancy — your Ritual: give the Tcho-Tcho 1 Doom, or earn 1 Doom fewer."
			case "mao":
				return "The Mao Ceremony — click a Cultist to sacrifice it for 1 Power, as many as you like, then press Done."
			case "undim":
				return "Undimensioned — click a unit and the area it goes to (only areas where you had units); press Done when the units are where you want them."
			case "howl":
				return fmt.Sprintf("Howl — choose a unit in %s to retreat, and where it goes.", e.areaName(e.areaIdx(p.Area)))
			case "eternal":
				return "Rhan-Tegoth received a result — pay 1 Power to cancel it, or press Done."
			case "demand":
				return "Demand Sacrifice — let the Sleeper gain an Elder Sign, or have all your Kills against its units count as Pains."
			case "restore":
				return "The Serpent Man returns — choose the area to place him in."
			case "desecrate":
				return fmt.Sprintf("Desecrate — choose the Cultist or Monster (cost 2 or less) that is placed in %s, whatever the die showed.", e.areaName(e.areaIdx(p.Area)))
			case "nexus":
				return fmt.Sprintf("Energy Nexus — take an Action that starts in %s before the battle, or press Done.", e.areaName(e.areaIdx(p.Area)))
			case "moon":
				return "Blasphemous Obeisance — choose the enemy spellbook the Moonbeast lies on; that enemy cannot use it until the Moonbeast leaves."
			case "moonland":
				return "The Moonbeast is free — choose the Gate of yours where it appears."
			case "crypt-pick":
				return "Crypt Dweller — choose the enemy that places your Wamps in areas without a Gate."
			case "crypt":
				return fmt.Sprintf("Crypt Dweller — place a Wamp of %s in an area without a Gate (%d left).", e.seatName(p.By), p.Left)
			case "sting":
				if p.Why == "tsunami" {
					return fmt.Sprintf("Tsunami — move a Cultist out of %s into an adjacent ocean.", e.areaName(e.areaIdx(p.Area)))
				}
				return fmt.Sprintf("The Agony Sting — move a Cultist out of %s into an adjacent land area.", e.areaName(e.areaIdx(p.Area)))
			case "spare":
				return "Fire Vampires — click a unit that took a Kill to spare it (it is Pained instead, and you gain 1 Power), or press Done to let the rest die."
			case "pick":
				return fmt.Sprintf("Curse of Chaugnar Faugn — choose the value of the Elder Sign %s receives.", e.seatName(p.By))
			case "donate":
				return fmt.Sprintf("Messenger of Yig — give 1 Power to %s, or refuse and let %s gain 1 Doom.", e.seatName(p.By), e.seatName(p.By))
			case "zygote":
				return fmt.Sprintf("The Zygote — click an area where you have a unit to place an Acolyte there (%d left).", p.Left)
			}
			return fmt.Sprintf("Eliminate a unit (%s).", p.Why)
		}
	case stageFirst:
		return "Choose the first player: the most Power is tied."
	case stageDirection:
		return "Choose a turn order."
	case stageDoom:
		return fmt.Sprintf("Choose a Doom phase action: a Ritual of Annihilation (%d Power), reveal Elder Signs, buy a Loyalty Card (2 Doom; a Terror also 2 Power) when a pack with cards is in play, or finish.", e.ritualCost())
	}
	return ""
}

// moveSummary says what the Move action has done so far, for the card that asks to
// confirm it: "Moved 2 Acolyte Cultists and a Deep One to Asia for 3 Power."
func (e *eng) moveSummary() string {
	m := e.d.Move
	if m == nil || len(m.Moved) == 0 {
		return "Choose a unit to move, or press Done."
	}
	var areas []string
	counts := map[string]map[string]int{}
	types := map[string][]string{}
	for _, k := range m.Moved {
		area, typ, _ := strings.Cut(k, "|")
		if counts[area] == nil {
			counts[area] = map[string]int{}
			areas = append(areas, area)
		}
		if counts[area][typ] == 0 {
			types[area] = append(types[area], typ)
		}
		counts[area][typ]++
	}
	var parts []string
	for _, area := range areas {
		var units []string
		for _, typ := range types[area] {
			name := unitNameOf(typ)
			if n := counts[area][typ]; n > 1 {
				units = append(units, fmt.Sprintf("%d %ss", n, name))
			} else {
				units = append(units, article(name))
			}
		}
		parts = append(parts, strings.Join(units, " and ")+" to "+e.areaName(e.areaIdx(area)))
	}
	return fmt.Sprintf("Moved %s for %d Power. Press Done, or choose more units to move.", strings.Join(parts, ", "), m.Spent)
}
