package cthulhuwars

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Phases of the game.
const (
	phaseAction = "action"
	phaseDoom   = "doom"
	phaseOver   = "over"
)

// Stages: what the position is waiting for.
const (
	stageTurn      = "turn"      // a player chooses actions
	stageMoving    = "moving"    // a Move action in progress
	stagePre       = "pre"       // pre-battle abilities
	stagePost      = "post"      // post-battle abilities
	stageAssign    = "assign"    // assigning kill and pain results
	stageRetreat   = "retreat"   // retreating pained units
	stagePrompt    = "prompt"    // a forced choice: spellbook, unit to lose…
	stageDirection = "direction" // the new first player picks the turn direction
	stageDoom      = "doom"      // a player's Doom-phase decisions
	stageSetup     = "setup"     // Windwalker chooses where it begins
)

// TurnState is the player taking an action.
type TurnState struct {
	Seat int `json:"seat"`
	// Nexus says the turn is a Sleeper's Energy Nexus Action taken just before
	// a battle (the battle waits in Data.Bat; the interrupted turn is in
	// Data.Saved).
	Nexus bool `json:"nexus,omitempty"`
	// Main says the turn's main action has been taken; only unlimited actions
	// (and ending the turn) remain.
	Main bool `json:"main,omitempty"`
	// Summoned says the Black Goat summoned a Monster this turn with Fertility
	// Cult (an unlimited action): that is an action too, so the turn may end
	// with Done, a single summon being all the player wanted.
	Summoned bool `json:"summoned,omitempty"`
	// Second says He Who is Not to be Named allows one more, different action.
	Second bool `json:"second,omitempty"`
	// SecondTaken says that second action has been used.
	SecondTaken bool `json:"second_taken,omitempty"`
	// Toggled lists areas whose Gate was controlled or abandoned this turn:
	// each may change hands once, so the free actions cannot loop forever.
	Toggled []string `json:"toggled,omitempty"`
}

// MoveState is a Move action in progress.
type MoveState struct {
	// Moved lists "area|type" for each unit that arrived during this action;
	// such a unit may not move again.
	Moved []string `json:"moved,omitempty"`
	Spent int      `json:"spent,omitempty"`
	// Origin, when set, restricts the Move to units leaving that area (an
	// Energy Nexus Action starts in the battle area).
	Origin string `json:"origin,omitempty"`
	// WindFrom and WindTo name the area Ithaqua just left and the one he went
	// to: with Arctic Wind the units that stood with him follow for nothing.
	WindFrom string `json:"wind_from,omitempty"`
	WindTo   string `json:"wind_to,omitempty"`
	// Taxed says the Ice Age surcharge was paid for this Move.
	Taxed bool `json:"taxed,omitempty"`
	// Carry lists, as "from>to", the free seats Shantaks that have moved offer a
	// Cultist (Horror Steed).
	Carry []string `json:"carry,omitempty"`
}

// PU is a unit of one seat: type and, for a retreat, nothing more.
type PU struct {
	Seat int    `json:"seat"`
	Type string `json:"type"`
}

// Assign is one battle result waiting to be assigned to a unit.
type Assign struct {
	// Seat owns the units that take the result; Pain says Pain rather than
	// Kill; Who decides which unit takes it.
	Seat int  `json:"seat"`
	Pain bool `json:"pain,omitempty"`
	Who  int  `json:"who"`
}

// Battle is a battle being fought.
type Battle struct {
	Area string `json:"area"`
	Att  int    `json:"att"`
	Def  int    `json:"def"`
	Step string `json:"step"` // pre | post | assign | retreat
	// Who is the seat deciding in pre/post; Done marks seats that finished.
	Who  int              `json:"who"`
	Done [nSeats + 1]bool `json:"done"`
	// Exempt holds "seat|type" → how many such units Invisibility hid, and
	// Invis how many Crawling Chaos has hidden in all (one per Flying Polyp).
	Exempt map[string]int `json:"exempt,omitempty"`
	Invis  int            `json:"invis,omitempty"`
	// Absorbed is how many units were fed to Shoggoths (3 dice each).
	Absorbed int `json:"absorbed,omitempty"`
	// Dice are the rolled dice per seat, Kills and Pains the results to
	// apply to that seat's units.
	Dice   [nSeats + 1][]int `json:"dice"`
	Kills  [nSeats + 1]int   `json:"kills"`
	Pains  [nSeats + 1]int   `json:"pains"`
	Rolled bool              `json:"rolled,omitempty"`
	// Queue of results to assign; Pained are units that took a Pain and
	// must retreat; Spare counts Star Spawn that took exactly one result and,
	// with Regenerate, can take one more.
	Queue  []Assign `json:"queue,omitempty"`
	Pained []PU     `json:"pained,omitempty"`
	Spare  int      `json:"spare,omitempty"`
	// Assigned says the dice results were applied; the post-battle abilities
	// (Necrophagy, the Harbinger) follow, and Extra holds the Pain results
	// they add, per seat.
	Assigned bool            `json:"assigned,omitempty"`
	Extra    [nSeats + 1]int `json:"extra,omitempty"`
	// Called counts, as "seat|type", the units Necrophagy dragged in: they
	// take no part in the results of this battle.
	Called map[string]int `json:"called,omitempty"`
	// Hit lists the Great Old Ones that were Pained or Killed, owner and
	// type: what the Harbinger pays for. Paid counts those already settled.
	Hit  []PU `json:"hit,omitempty"`
	Paid int  `json:"paid,omitempty"`
	// Slain counts enemy units each seat killed or devoured here.
	Slain [nSeats + 1]int `json:"slain"`
	// Lost counts a seat's cultists eliminated by an enemy (for Passion).
	LostCultists [nSeats + 1]int `json:"lost_cultists"`
	// Retreater is who chooses where pained units go (Madness); Declined says
	// Crawling Chaos left the choice to the units' owners.
	Retreater int  `json:"retreater,omitempty"`
	Declined  bool `json:"declined,omitempty"`
	// Main says the battle is the turn's main action (not an unlimited one).
	Main bool `json:"main,omitempty"`
	// Berserk counts the Gnoph-Keh that were assigned a Kill: the enemy then
	// eliminates as many Monsters or Cultists. Killed counts the units of
	// factions other than Windwalker that died (Cannibalism).
	Berserk int `json:"berserk,omitempty"`
	Killed  int `json:"killed,omitempty"`
	// Rhan says Rhan-Tegoth has received his one result of the battle.
	Rhan bool `json:"rhan,omitempty"`
	// Demand says the Sleeper's Demand Sacrifice was settled for this battle;
	// Soften says the enemy chose that its Kills against the Sleeper count as
	// Pains.
	Demand bool `json:"demand,omitempty"`
	Soften bool `json:"soften,omitempty"`
	// Nexus says the Energy Nexus window before the battle has passed.
	Nexus bool `json:"nexus,omitempty"`
	// Doomed lists the units that took a Kill while Cthugha stood in the battle:
	// their owner's enemy may spare them (Fire Vampires) before they die. Venom
	// counts, per seat, the extra Kills Yig's Snakebite gives it; CultistKills the
	// Cultists of each seat that were Killed in this battle.
	Doomed       []PU            `json:"doomed,omitempty"`
	Venom        [nSeats + 1]int `json:"venom,omitempty"`
	CultistKills [nSeats + 1]int `json:"cultist_kills,omitempty"`
	// Terror is, per seat, the choice of the Tcho-Tcho's Terror in this
	// battle: 1 lowers the enemy's Combat, 2 raises its own. Fallen are the
	// units Martyrdom could bring back: killed in this battle.
	Terror [nSeats + 1]int8 `json:"terror,omitempty"`
	Fallen []PU             `json:"fallen,omitempty"`
	// Shrivelled marks the seats that used Shriveling in this battle.
	Shrivelled [nSeats + 1]bool `json:"shrivelled,omitempty"`
	// Howled says Windwalker's Howl was used in this battle.
	Howled bool `json:"howled,omitempty"`
	// SixDice marks a seat that rolled six or more dice (the Sleeper's
	// requirement is met when the battle is over).
	SixDice [nSeats + 1]bool `json:"six_dice,omitempty"`
}

// Prompt is a forced choice outside the normal flow.
type Prompt struct {
	// Kind is "book" (choose a spellbook), "lose" (choose a unit to
	// eliminate), "submerge" (choose which units sink with Cthulhu), "scream"
	// (which Undead walk with the King), "shriek" (which Byakhee fly),
	// "avatar" (which unit swaps places with Shub-Niggurath), "place"
	// (where Ghroth puts a Cultist), "react" (an ability a player may use
	// after another player's action — see reactions.go), "howl" (which unit
	// retreats from Howl, and where), "eternal" (whether to pay for
	// Rhan-Tegoth's Eternal), "demand" (the answer to Demand Sacrifice),
	// "restore" (where the Serpent Man of Ancient Sorcery returns) or
	// "cannibal" (which unit Cannibalism places) or "desecrate" (which Monster
	// or Cultist the King's Desecration places, whatever the roll was).
	Kind string `json:"kind"`
	Seat int    `json:"seat"`
	// Area is where a unit must be eliminated (kind "lose"), from where
	// units are taken (kinds "submerge", "scream") or the place a choice is
	// about (kinds "shriek", "avatar", "react").
	Area string `json:"area,omitempty"`
	// Dest is where units taken along (kind "scream") end up.
	Dest string `json:"dest,omitempty"`
	// By is the enemy seat that caused it (for Passion), 0 for none.
	By int `json:"by,omitempty"`
	// Why names the cause ("devour", "abduct", "dreams", …).
	Why string `json:"why,omitempty"`
	// Left is how many units are still to be eliminated, Lost how many have
	// been.
	Left int `json:"left,omitempty"`
	Lost int `json:"lost,omitempty"`
	// CultistsOnly restricts a loss to Cultists.
	CultistsOnly bool `json:"cultists_only,omitempty"`
	// Only restricts a loss to these unit types.
	Only []string `json:"only,omitempty"`
	// Skip is the book (global number + 1) a "book" prompt must not offer:
	// the one Recriminations has just set aside.
	Skip int `json:"skip,omitempty"`
	// Targeted (kind "react"): the action that opened the window was aimed at
	// the reacting seat — see Aim.
	Targeted bool `json:"targeted,omitempty"`
}

// Aim is what an action is aimed at: the seats it affects and the areas it
// touches. A seat it names, or with a unit or a Gate in one of the areas, is
// "targeted" by it — what the reaction setting "When I'm targeted" asks about.
// Only actions have an aim: the effects that follow a battle (Madness
// moving a unit, say) are not actions and open no reaction windows.
type Aim struct {
	Seats []int    `json:"seats,omitempty"`
	Areas []string `json:"areas,omitempty"`
}

// SlumberState is the Gate (and the Cultist that held it) the Sleeper has put
// to sleep on its faction card.
type SlumberState struct {
	// Cultist is the type of the Cultist that sleeps with the Gate.
	Cultist string `json:"cultist"`
}

// SorceryState is a Serpent Man on an enemy's faction card: its owner has that
// faction's unique ability until the end of the Doom phase.
type SorceryState struct {
	// Seat is the seat whose card holds the Serpent Man.
	Seat int `json:"seat"`
	// Ending says the Doom phase is over: the Serpent Man waits to be placed
	// and the borrowed ability is gone.
	Ending bool `json:"ending,omitempty"`
}

// DoomState tracks the Doom phase.
type DoomState struct {
	// Order lists the seats still to act; Rituals and Sacrificed mark what
	// the current seat has already done.
	Order      []int `json:"order"`
	Ritualled  bool  `json:"ritualled,omitempty"`
	Sacrificed bool  `json:"sacrificed,omitempty"`
	// Death says the Sleeper placed its Monster with Death from Below.
	Death bool `json:"death,omitempty"`
	// Bought says the current seat bought a Loyalty Card in this Doom phase;
	// Crypt that it had its Wamps placed (Crypt Dweller).
	Bought bool `json:"bought,omitempty"`
	Crypt  bool `json:"crypt,omitempty"`
}

// LastBattle is kept for display: the last battle's dice and outcome.
type LastBattle struct {
	Area string `json:"area"`
	Att  int    `json:"att"`
	Def  int    `json:"def"`
	// Combat and Dice per seat (attacker and defender only).
	Combat [nSeats + 1]int   `json:"combat"`
	Dice   [nSeats + 1][]int `json:"dice"`
	Kills  [nSeats + 1]int   `json:"kills"`
	Pains  [nSeats + 1]int   `json:"pains"`
}

// Data is the Cthulhu Wars part of a position: everything the shared board
// state (units in areas, pools in reserves, ply) does not hold.
type Data struct {
	Phase string `json:"phase"`
	Round int    `json:"round"`
	// Players is how many sit at the table (0 in positions saved before it
	// was recorded: the definition's seats).
	Players int `json:"players,omitempty"`
	// First is the first-player seat, Dir the direction of play round the
	// table (+1: seat order, -1: reverse).
	First int `json:"first"`
	Dir   int `json:"dir"`
	// Opening says the game has not begun its first Action phase: the first
	// player (Great Cthulhu, else the first cult by name) still has to choose
	// the direction of play — the Doom phase is skipped in the first round.
	Opening bool `json:"opening,omitempty"`

	Power [nSeats + 1]int `json:"power"`
	Doom  [nSeats + 1]int `json:"doom"`
	// DoomBy says where each seat's Doom came from, so the log can explain a
	// total (see addDoom). Doom a position carried before it was kept is the
	// remainder.
	DoomBy [nSeats + 1]DoomLedger `json:"doom_by"`
	// Ritual is the Ritual of Annihilation marker: the step it stands on, 0
	// for the first space of the track (see ritualTrack); one past the last
	// space is Instant Death.
	Ritual int `json:"ritual"`

	// Gates maps an area to the seat controlling its Gate (0 = abandoned);
	// an area absent from the map has no Gate. Desec marks Desecration
	// tokens.
	Gates map[string]int  `json:"gates,omitempty"`
	Desec map[string]bool `json:"desec,omitempty"`
	// Holders maps the area of a controlled Gate to the type of the unit that
	// occupies it (an Acolyte, a High Priest or a Dark Young): the figure
	// standing on the Gate. It is kept up to date by syncGate; a position
	// saved before it existed is filled in when it is first needed.
	Holders map[string]string `json:"holders,omitempty"`

	// Books and Reqs are bit masks per seat: spellbooks earned (the
	// cult's own) and requirement slots used.
	Books [nSeats + 1]uint8 `json:"books"`
	// Neutral is the mask, per seat, of the neutral spellbooks it holds (bit n
	// is neutralBooks[n]); they count among its six spellbooks.
	Neutral [nSeats + 1]uint8 `json:"neutral,omitempty"`
	Reqs    [nSeats + 1]uint8 `json:"reqs"`

	// Signs are each seat's unrevealed Elder Signs (values hidden from
	// others); Shown are the values a seat has revealed — face up for all —
	// and Bag counts the tokens left to draw: 1s, 2s and 3s.
	Signs [nSeats + 1][]int `json:"signs"`
	Shown [nSeats + 1][]int `json:"shown"`
	Bag   [3]int            `json:"bag"`

	// Loyal is, per seat, the mask of neutral Monsters whose Loyalty Card it
	// holds (bit i is neutralUnits[i]); Card counts the Dimensional Shamblers
	// waiting on its faction card (Walk Between Worlds).
	Loyal [nSeats + 1]uint16 `json:"loyal,omitempty"`
	Card  [nSeats + 1]int    `json:"card,omitempty"`
	// IBooks says which Independent Great Old Ones' spellbooks lie on their cards
	// (bit i is cards[firstIndependent+i]); Picks lists the seats that received
	// an Elder Sign whose value the holder of Chaugnar Faugn's Curse has still to
	// choose (the sign is in Signs as 0). Moon lists the Moonbeasts lying on
	// spellbooks.
	IBooks uint8       `json:"ibooks,omitempty"`
	Picks  []int       `json:"picks,omitempty"`
	Moon   []MoonState `json:"moon,omitempty"`

	// Captured[by][owner] counts Cultists held on a faction card.
	Captured [nSeats + 1][nSeats + 1]int `json:"captured"`
	// Away lists the units Great Cthulhu has submerged onto its faction
	// card: unit type → count.
	Away map[string]int `json:"away,omitempty"`

	// WWStart is the area where Windwalker began: its second sigil area is the
	// one its second requirement asks for.
	WWStart string `json:"ww_start,omitempty"`
	// Hib marks the seats that hibernated this Action phase: they take no more
	// turns, and the Power they banked is added to what they gather.
	Hib [nSeats + 1]bool `json:"hib,omitempty"`
	// Ice is the area of Windwalker's Ice Age token ("" when it is not placed).
	Ice string `json:"ice,omitempty"`
	// Slumber is the Sleeper's Gate and Cultist on its faction card (Cursed
	// Slumber); Sorcery the Serpent Man it put on an enemy's card (Ancient
	// Sorcery); Held the enemy Monsters its Tsathoggua captured, waiting to be
	// sacrificed in Gather Power. Death marks that Death from Below was used
	// this Doom phase.
	Slumber *SlumberState  `json:"slumber,omitempty"`
	Sorcery []SorceryState `json:"sorcery,omitempty"`
	Held    []PU           `json:"held,omitempty"`
	// CapHP[by][owner] counts High Priests held on a faction card.
	CapHP [nSeats + 1][nSeats + 1]int `json:"cap_hp,omitempty"`
	// Setup is the seat that must still choose where it begins (Tcho-Tcho,
	// Windwalker, Opener of the Way, in that order); SetupLater are the seats
	// that follow it.
	Setup      int   `json:"setup,omitempty"`
	SetupLater []int `json:"setup_later,omitempty"`
	// TTStart is the area where Tcho-Tcho began: its Gate there is the one its
	// fourth requirement asks it to remove. Growth is Ubbo-Sathla's Growth
	// counter.
	TTStart string `json:"tt_start,omitempty"`
	Growth  int    `json:"growth,omitempty"`
	// Curse is the Dread Curse of Azathoth being settled.
	Curse *CurseState `json:"curse,omitempty"`
	// Used is, per seat, the once-only spellbooks spent (bit 0 Dragon Ascending,
	// bit 1 Dragon Descending).
	Used [nSeats + 1]uint8 `json:"used,omitempty"`
	// Saved is the turn a Sleeper's Energy Nexus Action interrupted.
	Saved *TurnState `json:"saved,omitempty"`

	// Battled lists, per seat, the areas it has battled in this Action phase.
	Battled [nSeats + 1][]string `json:"battled"`

	Stage string     `json:"stage"`
	Turn  *TurnState `json:"turn,omitempty"`
	Move  *MoveState `json:"move,omitempty"`
	Bat   *Battle    `json:"battle,omitempty"`
	Doomx *DoomState `json:"doomx,omitempty"`
	// Prompts are forced choices (FIFO); Then names what happens when they
	// have all been answered and Resume the stage to return to.
	Prompts []Prompt `json:"prompts,omitempty"`
	Then    string   `json:"then,omitempty"`
	Resume  string   `json:"resume,omitempty"`

	// EndPending: someone reached the Doom goal; the game ends at the end of
	// this turn or phase. Instant says the instant-death space was reached:
	// the game ends when the Doom phase's scoring is done (the current one,
	// or the next when something other than a Ritual moved the marker).
	EndPending bool `json:"end_pending,omitempty"`
	Instant    bool `json:"instant,omitempty"`
	// FormsUsed: The Thousand Forms was used this Action phase (it is turned
	// face up again in Gather Power).
	FormsUsed bool `json:"forms_used,omitempty"`
	// ReactOwed is the seat whose action just resolved: the players holding
	// reactions are offered them before the next action (see reactions.go).
	// Aim is what the action under way is aimed at, ReactAim what the action
	// that ReactOwed is about was aimed at.
	ReactOwed int  `json:"react_owed,omitempty"`
	Aim       *Aim `json:"aim,omitempty"`
	ReactAim  *Aim `json:"react_aim,omitempty"`
	// TiedFirst lists the players tied for most Power, the previous first
	// player choosing between them.
	TiedFirst []int `json:"tied_first,omitempty"`

	// Awoken marks Great Old Ones awakened before ("seat|n").
	Awoken map[string]bool `json:"awoken,omitempty"`

	// RNG is the dice generator's state — secret in public views.
	RNG uint64 `json:"rng"`

	// Result of a finished game.
	Winner int    `json:"winner,omitempty"`
	Reason string `json:"reason,omitempty"`

	Last   *LastBattle `json:"last,omitempty"`
	Events []string    `json:"events,omitempty"`
	// Log keeps what the latest decisions did, so a player can read what the
	// others did since their own last move (see trimLog).
	Log []LogEntry `json:"log,omitempty"`
}

// DoomLedger is what a seat's Doom is made of: the Gates scored in Doom
// phases, the Gates of its Rituals of Annihilation, the Doom other cults gave
// it (Yellow Sign's gift) and the Elder Signs it revealed.
type DoomLedger struct {
	Gates   int `json:"gates,omitempty"`
	Rituals int `json:"rituals,omitempty"`
	Gifts   int `json:"gifts,omitempty"`
	Signs   int `json:"signs,omitempty"`
	// Cards is what Loyalty Cards earned: Gnorri's Grottos, Messenger of Yig.
	Cards int `json:"cards,omitempty"`
}

// LogEntry is one decision's part of the event log: the ply it was made at,
// the seat that made it and the lines that narrate what it did.
type LogEntry struct {
	Ply   int      `json:"ply"`
	Seat  int      `json:"seat"`
	Lines []string `json:"lines"`
}

func (a *Aim) clone() *Aim {
	if a == nil {
		return nil
	}
	return &Aim{Seats: append([]int(nil), a.Seats...), Areas: append([]string(nil), a.Areas...)}
}

// CloneExt implements state.ExtClone.
func (d *Data) CloneExt() any { return d.clone() }

func (d *Data) clone() *Data {
	c := *d
	// A position that went through JSON has no empty maps (omitempty): the
	// clone always has them, so the rules can write into them.
	cloneInt := func(m map[string]int) map[string]int {
		o := make(map[string]int, len(m))
		for k, v := range m {
			o[k] = v
		}
		return o
	}
	cloneBool := func(m map[string]bool) map[string]bool {
		o := make(map[string]bool, len(m))
		for k, v := range m {
			o[k] = v
		}
		return o
	}
	c.Gates = cloneInt(d.Gates)
	c.Desec = cloneBool(d.Desec)
	c.Holders = make(map[string]string, len(d.Holders))
	for k, v := range d.Holders {
		c.Holders[k] = v
	}
	if d.Slumber != nil {
		s := *d.Slumber
		c.Slumber = &s
	}
	c.Sorcery = append([]SorceryState(nil), d.Sorcery...)
	c.Held = append([]PU(nil), d.Held...)
	if d.Saved != nil {
		t := *d.Saved
		c.Saved = &t
	}
	c.Away = nil
	if len(d.Away) > 0 {
		c.Away = cloneInt(d.Away)
	}
	c.Awoken = cloneBool(d.Awoken)
	for i := range d.Signs {
		c.Signs[i] = append([]int(nil), d.Signs[i]...)
		c.Shown[i] = append([]int(nil), d.Shown[i]...)
	}
	for i := range d.Battled {
		c.Battled[i] = append([]string(nil), d.Battled[i]...)
	}
	if d.Turn != nil {
		t := *d.Turn
		c.Turn = &t
	}
	if d.Move != nil {
		m := *d.Move
		m.Moved = append([]string(nil), d.Move.Moved...)
		m.Carry = append([]string(nil), d.Move.Carry...)
		c.Move = &m
	}
	if d.Bat != nil {
		b := *d.Bat
		b.Exempt = cloneInt(d.Bat.Exempt)
		for i := range b.Dice {
			b.Dice[i] = append([]int(nil), d.Bat.Dice[i]...)
		}
		b.Called = cloneInt(d.Bat.Called)
		b.Queue = append([]Assign(nil), d.Bat.Queue...)
		b.Pained = append([]PU(nil), d.Bat.Pained...)
		b.Hit = append([]PU(nil), d.Bat.Hit...)
		b.Fallen = append([]PU(nil), d.Bat.Fallen...)
		b.Doomed = append([]PU(nil), d.Bat.Doomed...)
		c.Bat = &b
	}
	if d.Doomx != nil {
		x := *d.Doomx
		x.Order = append([]int(nil), d.Doomx.Order...)
		c.Doomx = &x
	}
	c.SetupLater = append([]int(nil), d.SetupLater...)
	c.Picks = append([]int(nil), d.Picks...)
	c.Moon = append([]MoonState(nil), d.Moon...)
	c.Curse = d.Curse.clone()
	c.Prompts = append([]Prompt(nil), d.Prompts...)
	c.TiedFirst = append([]int(nil), d.TiedFirst...)
	c.Aim, c.ReactAim = d.Aim.clone(), d.ReactAim.clone()
	if d.Last != nil {
		l := *d.Last
		for i := range l.Dice {
			l.Dice[i] = append([]int(nil), d.Last.Dice[i]...)
		}
		c.Last = &l
	}
	c.Events = append([]string(nil), d.Events...)
	c.Log = make([]LogEntry, len(d.Log))
	copy(c.Log, d.Log) // entries are never changed once written
	if len(c.Log) == 0 {
		c.Log = nil
	}
	return &c
}

// dataOf reads a position's Cthulhu Wars data. A position that came straight
// from a document is decoded on every call; positions the ruleset produced
// hold the typed value.
func dataOf(st *state.BoardState) (*Data, error) {
	switch v := st.Ext().(type) {
	case *Data:
		return v, nil
	case *publicExt:
		return v.Data, nil
	case json.RawMessage:
		d := &Data{}
		if err := json.Unmarshal(v, d); err != nil {
			return nil, fmt.Errorf("cthulhuwars: bad position payload: %w", err)
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("cthulhuwars: position has no game payload")
	default:
		return nil, fmt.Errorf("cthulhuwars: position payload has type %T", v)
	}
}

// publicExt is how a redacted position serialises.
type publicExt struct {
	*Data
	View *View `json:"view"`
}

// CloneExt implements state.ExtClone for redacted positions.
func (p *publicExt) CloneExt() any { return &publicExt{Data: p.clone(), View: p.View} }

func has(mask uint8, bit int) bool { return mask&(1<<uint(bit)) != 0 }

// hasBook says seat can use its own spellbook: it holds it and no Moonbeast
// lies on it. (holds says only that it holds it.)
func (d *Data) hasBook(seat, book int) bool { return has(d.Books[seat], book) && !d.mooned(seat, book) }

func (d *Data) bookCount(seat int) int {
	n := 0
	for i := 0; i < bookSlots+nNeutralBooks; i++ {
		if d.holds(seat, i) {
			n++
		}
	}
	return n
}

func (d *Data) reqDone(seat, req int) bool { return has(d.Reqs[seat], req) }
