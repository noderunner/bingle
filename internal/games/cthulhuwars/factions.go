package cthulhuwars

// Unit classes, smallest to largest. The core set has no Terrors (they come
// with expansions); the class exists so the map can size them between
// monsters and Great Old Ones.
const (
	classCultist = iota
	classMonster
	classTerror
	classGOO
)

// unitSpec is one kind of unit in a faction's box.
type unitSpec struct {
	ID     string
	Name   string
	Class  int
	Count  int // how many the faction owns
	Cost   int // Power to summon a monster / awaken a Great Old One
	Combat int // base Combat (some units compute theirs: see combat.go)
	Text   string
	// Abilities are the named abilities of the unit's card: what a Great Old
	// One can do besides fighting.
	Abilities []abilitySpec
	// Token marks a piece that is never summoned or moved by its owner: Abhoth's
	// Filth Tokens.
	Token bool
}

// abilitySpec is one named ability of a unit: "Desecrate", its timing
// ("Action: Cost 2", "Pre-Battle", "Ongoing") and what it does.
type abilitySpec struct {
	Name   string
	Timing string
	Text   string
}

// bookSpec is one of a faction's six spellbooks.
type bookSpec struct {
	ID     string
	Name   string
	Timing string // Ongoing, Action: Cost 1, Pre-Battle, …
	Text   string
	// Units lists the unit types the spellbook enhances — it gives them an
	// ability or a bonus. Once the book is earned the map marks those units
	// and their information card lists the book.
	Units []string
}

// reqSpec is one spellbook requirement of a faction card.
type reqSpec struct {
	ID   string
	Text string
}

// faction is one of the cults: the four of the core set and the two of the
// first expansion (Windwalker and Sleeper).
type faction struct {
	Key   string
	Name  string
	Color string
	// Seat is the seat the faction has at a table of the default line-up
	// (the cults in this order); a derived game seats them as chosen.
	Seat    int
	Start   string // start area id
	Ability string // the faction ability, in a sentence
	Units   []unitSpec
	Books   []bookSpec
	Reqs    []reqSpec
}

// Faction indexes.
const (
	facGC = iota // Great Cthulhu
	facCC        // Crawling Chaos
	facBG        // Black Goat
	facYS        // Yellow Sign
	facWW        // Windwalker
	facSL        // Sleeper
	facOW        // Opener of the Way
	facTT        // Tcho-Tcho

	nFactions
)

// Unit ids shared by rules code.
const (
	uCultist = "cultist"

	uDeepOne   = "deep-one"
	uShoggoth  = "shoggoth"
	uStarspawn = "starspawn"
	uCthulhu   = "cthulhu"

	uNightgaunt = "nightgaunt"
	uPolyp      = "flying-polyp"
	uHorror     = "hunting-horror"
	uNyar       = "nyarlathotep"

	uGhoul  = "ghoul"
	uFungi  = "fungi"
	uYoung  = "dark-young"
	uShub   = "shub-niggurath"
	uUndead = "undead"
	uByak   = "byakhee"
	uKing   = "king-in-yellow"
	uHastur = "hastur"

	uWendigo  = "wendigo"
	uGnoph    = "gnoph-keh"
	uRhan     = "rhan-tegoth"
	uIthaqua  = "ithaqua"
	uWizard   = "wizard"
	uSerpent  = "serpent-man"
	uFormless = "formless-spawn"
	uTsath    = "tsathoggua"

	uMutant = "mutant"
	uAbom   = "abomination"
	uSpawn  = "spawn-of-yog-sothoth"
	uYog    = "yog-sothoth"
	uProto  = "proto-shoggoth"
	uUbbo   = "ubbo-sathla"

	// uHighPriest is the High Priest of the expansion of that name: a Cultist
	// of its own kind that every cult may have one of.
	uHighPriest = "high-priest"
)

// Spellbook indexes per faction (the order of faction.Books).
const (
	// Great Cthulhu
	bkAbsorb = iota
	bkDevolve
	bkDreams
	bkRegenerate
	bkSubmerge
	bkYha
)

const (
	// Crawling Chaos
	bkAbduct = iota
	bkInvisibility
	bkSeek
	bkEmissary
	bkMadness
	bkForms
)

const (
	// Black Goat
	bkBlood = iota
	bkFrenzy
	bkGhroth
	bkRedSign
	bkNecro
	bkThousand
)

const (
	// Yellow Sign
	bkPassion = iota
	bkHWINN
	bkThirdEye
	bkShriek
	bkScreaming
	bkZingaya
)

const (
	// Windwalker
	bkArctic = iota
	bkBerserk
	bkCannibal
	bkHerald
	bkHowl
	bkIceAge
)

const (
	// Sleeper
	bkSorcery = iota
	bkBurrow
	bkCaptureMonster
	bkSlumber
	bkDemand
	bkNexus
)

const (
	// Opener of the Way
	bkChannel = iota
	bkAscend
	bkDescend
	bkCurse
	bkMillion
	bkBreak
)

const (
	// Tcho-Tcho
	bkHiero = iota
	bkSoulless
	bkTerror
	bkIdolatry
	bkMartyr
	bkTablets
)

// Requirement indexes per faction (the order of faction.Reqs).
const (
	rqGCFirstDoom = iota
	rqGCKill1
	rqGCKill2
	rqGCOceans
	rqGCAwaken
	rqGCFive
)

const (
	rqCCPay4 = iota
	rqCCPay6
	rqCCGates3
	rqCCGates4
	rqCCCapture
	rqCCAwaken
)

const (
	rqBGAreas4 = iota
	rqBGAreas6
	rqBGAreas8
	rqBGSac
	rqBGShare
	rqBGAwaken
)

const (
	rqYSKing = iota
	rqYSDragon
	rqYSChevron
	rqYSThorns
	rqYSHastur
	rqYSGift
)

const (
	rqWWRhan = iota
	rqWWGate
	rqWWFirst
	rqWWSix
	rqWWTake
	rqWWIthaqua
)

const (
	rqSLBoon = iota
	rqSLGift
	rqSLBane
	rqSLRoll
	rqSLRitual
	rqSLAwaken
)

const (
	rqOWGates8 = iota
	rqOWGates12
	rqOWEnemyGates
	rqOWLose
	rqOWMeet
	rqOWAwaken
)

const (
	rqTTRitual = iota
	rqTTSign
	rqTTThree
	rqTTGate
	rqTTKilled
	rqTTAwaken
)

// The eight factions, in the order of the default line-up.
var factions = [nFactions]faction{
	{
		Key: "gc", Name: "Great Cthulhu", Color: "green", Seat: 1, Start: "south-pacific",
		Ability: "Immortal: once Cthulhu has awakened he costs only 4 Power each time after, and whenever you awaken any Great Old One you gain 1 Elder Sign.",
		Units: []unitSpec{
			{ID: uCultist, Name: "Acolyte Cultist", Class: classCultist, Count: 6, Cost: 1, Combat: 0},
			{ID: uDeepOne, Name: "Deep One", Class: classMonster, Count: 4, Cost: 1, Combat: 1},
			{ID: uShoggoth, Name: "Shoggoth", Class: classMonster, Count: 2, Cost: 2, Combat: 2, Text: "Absorb (spellbook): +3 dice for each of your units fed to it."},
			{ID: uStarspawn, Name: "Star Spawn", Class: classMonster, Count: 2, Cost: 3, Combat: 3, Text: "Regenerate (spellbook): takes two results before it falls."},
			{ID: uCthulhu, Name: "Cthulhu", Class: classGOO, Count: 1, Cost: 10, Combat: 6, Abilities: []abilitySpec{{Name: "Devour", Timing: "Pre-Battle", Text: "The enemy player chooses and eliminates one of his Monsters or Cultists in the battle."}}},
		},
		Books: []bookSpec{
			{ID: "absorb", Name: "Absorb", Timing: "Pre-Battle", Text: "If a Shoggoth is in the battle, eliminate any number of your Monsters or Cultists there; for each one the Shoggoth rolls 3 more dice.", Units: []string{uShoggoth}},
			{ID: "devolve", Name: "Devolve", Timing: "Ongoing (reaction)", Text: "After any player's Action — even when it is not your turn — you may replace any number of your Acolyte Cultists on the map with Deep Ones from your pool. It costs nothing.", Units: []string{uCultist, uDeepOne}},
			{ID: "dreams", Name: "Dreams", Timing: "Action: Cost 2", Text: "Choose an area holding an enemy Acolyte Cultist: that enemy eliminates one of its Acolytes there and you place one of yours.", Units: []string{uCultist}},
			{ID: "regenerate", Name: "Regenerate", Timing: "Post-Battle", Text: "Assign up to 2 Kill or Pain results to the same Star Spawn. If 2 Kills are applied it dies; any other combination only Pains it.", Units: []string{uStarspawn}},
			{ID: "submerge", Name: "Submerge", Timing: "Action: Cost 1", Text: "If Cthulhu is in an ocean, take him — and any of your units there that you choose — off the map onto your faction card. Later, for 0 Power, put them all into any area.", Units: []string{uCthulhu}},
			{ID: "yha", Name: "Y'ha-Nthlei", Timing: "Gather Power", Text: "If Cthulhu is in play, gain 1 Power for each enemy-controlled Gate in an ocean area.", Units: []string{uCthulhu}},
		},
		Reqs: []reqSpec{
			{ID: "first-doom", Text: "Receive this spellbook in the first Doom phase, and 1 Elder Sign."},
			{ID: "kill-1", Text: "Kill and/or Devour an enemy unit in a battle."},
			{ID: "kill-2", Text: "Kill and/or Devour 2 enemy units in a battle. (Both kill requirements can be met in one battle by killing or devouring 3 or more.)"},
			{ID: "oceans", Text: "Control 3 Gates in ocean areas, or 4 Gates exist in ocean areas."},
			{ID: "awaken", Text: "Awaken Cthulhu."},
			{ID: "five-books", Text: "This must be the last spellbook you receive, taken in a Doom phase. Receive 1 Elder Sign."},
		},
	},
	{
		Key: "cc", Name: "Crawling Chaos", Color: "blue", Seat: 2, Start: "asia",
		Ability: "Flight: all your units fly — a Move takes them up to 2 areas, over enemy units.",
		Units: []unitSpec{
			{ID: uCultist, Name: "Acolyte Cultist", Class: classCultist, Count: 6, Cost: 1, Combat: 0},
			{ID: uNightgaunt, Name: "Nightgaunt", Class: classMonster, Count: 3, Cost: 1, Combat: 0, Text: "Abduct (spellbook): trade a Nightgaunt for an enemy monster or cultist."},
			{ID: uPolyp, Name: "Flying Polyp", Class: classMonster, Count: 3, Cost: 2, Combat: 1, Text: "Invisibility (spellbook): hides a unit from the battle."},
			{ID: uHorror, Name: "Hunting Horror", Class: classMonster, Count: 2, Cost: 3, Combat: 2, Text: "Seek and Destroy (spellbook): summoned into a battle from anywhere."},
			{ID: uNyar, Name: "Nyarlathotep", Class: classGOO, Count: 1, Cost: 10, Combat: 0, Abilities: []abilitySpec{{Name: "The Harbinger", Timing: "Post-Battle", Text: "If Nyarlathotep is in a battle in which one or more enemy Great Old Ones are Pained or Killed, you receive Power equal to half of the cost of awakening those Great Old Ones. For each enemy Great Old One Pained or Killed, you may choose to receive 2 Elder Signs instead of Power."}}},
		},
		Books: []bookSpec{
			{ID: "abduct", Name: "Abduct", Timing: "Pre-Battle", Text: "Eliminate one or more of your Nightgaunts from the battle; for each one the enemy must eliminate one of its Monsters or Cultists there.", Units: []string{uNightgaunt}},
			{ID: "invisibility", Name: "Invisibility", Timing: "Pre-Battle", Text: "For each Flying Polyp present, choose one Monster or Cultist in the battle — yours or an enemy's: it takes no part in the rest of the battle.", Units: []string{uPolyp}},
			{ID: "seek", Name: "Seek and Destroy", Timing: "Pre-Battle", Text: "Immediately move any or all Hunting Horrors from any area into the battle area.", Units: []string{uHorror}},
			{ID: "emissary", Name: "Emissary of the Outer Gods", Timing: "Post-Battle", Text: "Unless an enemy Great Old One is in the battle, a Kill applied to Nyarlathotep becomes a Pain; if he cannot be Pained because he is surrounded he is not eliminated.", Units: []string{uNyar}},
			{ID: "madness", Name: "Madness", Timing: "Post-Battle", Text: "After all Pains are assigned, you — in every battle, even those you are not in — choose where the pained units go, in any order (not into the area of the faction they fought).", Units: []string{uNyar}},
			{ID: "forms", Name: "The Thousand Forms", Timing: "Action: Cost 0", Text: "Once per Action phase, if Nyarlathotep is in play, roll a die: your enemies lose that much Power between them. If they cannot pay it all, you gain that much Power instead.", Units: []string{uNyar}},
		},
		Reqs: []reqSpec{
			{ID: "pay-4", Text: "As your action, pay 4 Power."},
			{ID: "pay-6", Text: "As your action, pay 6 Power. (Both requirements can be met in one action by paying 10.)"},
			{ID: "gates-3", Text: "Control 3 Gates, or have 12 Power."},
			{ID: "gates-4", Text: "Control 4 Gates, or have 15 Power."},
			{ID: "capture", Text: "Capture an enemy Cultist."},
			{ID: "awaken", Text: "Awaken Nyarlathotep."},
		},
	},
	{
		Key: "bg", Name: "Black Goat", Color: "red", Seat: 3, Start: "south-america",
		Ability: "Fertility Cult: summoning monsters is an Unlimited action for you.",
		Units: []unitSpec{
			{ID: uCultist, Name: "Acolyte Cultist", Class: classCultist, Count: 6, Cost: 1, Combat: 0},
			{ID: uGhoul, Name: "Ghoul", Class: classMonster, Count: 2, Cost: 1, Combat: 0, Text: "Necrophagy (spellbook): drawn into any battle."},
			{ID: uFungi, Name: "Fungi", Class: classMonster, Count: 4, Cost: 2, Combat: 1, Text: "Ghroth (spellbook) counts the areas that hold Fungi."},
			{ID: uYoung, Name: "Dark Young", Class: classMonster, Count: 3, Cost: 3, Combat: 2, Text: "The Red Sign (spellbook): can create and control Gates."},
			{ID: uShub, Name: "Shub-Niggurath", Class: classGOO, Count: 1, Cost: 8, Combat: 0, Abilities: []abilitySpec{{Name: "Avatar", Timing: "Action: Cost 1", Text: "Choose an area and a faction. Swap the location of Shub-Niggurath with that of a Monster or Cultist in the chosen area. The owner of the chosen faction chooses which unit to relocate."}}},
		},
		Books: []bookSpec{
			{ID: "blood", Name: "Blood Sacrifice", Timing: "Doom Phase", Text: "If Shub-Niggurath is in play you may eliminate one of your Cultists to gain 1 Elder Sign.", Units: []string{uShub, uCultist}},
			{ID: "frenzy", Name: "Frenzy", Timing: "Ongoing", Text: "Your Cultists have 1 Combat (they can still be captured).", Units: []string{uCultist}},
			{ID: "ghroth", Name: "Ghroth", Timing: "Action: Cost 2", Text: "Roll a die. If it is no more than the number of areas holding your Fungi, your enemies between them eliminate that many Cultists (you choose how they share it if they cannot agree); otherwise you place any faction's Acolyte anywhere on the map.", Units: []string{uFungi}},
			{ID: "redsign", Name: "The Red Sign", Timing: "Ongoing", Text: "Your Dark Young can create and control Gates (each adds 1 to Shub-Niggurath's Combat and 1 Power in Gather Power).", Units: []string{uYoung}},
			{ID: "necro", Name: "Necrophagy", Timing: "Post-Battle", Text: "Move any or all Ghouls that did not take part in the battle into the battle area — even in a battle you are not in. Each side suffers an extra Pain for each Ghoul moved.", Units: []string{uGhoul}},
			{ID: "thousand", Name: "The Thousand Young", Timing: "Ongoing", Text: "While Shub-Niggurath is in play, Ghouls, Fungi and Dark Young each cost 1 less to summon.", Units: []string{uGhoul, uFungi, uYoung}},
		},
		Reqs: []reqSpec{
			{ID: "areas-4", Text: "Have units in 4 areas."},
			{ID: "areas-6", Text: "Have units in 6 areas."},
			{ID: "areas-8", Text: "Have units in 8 areas."},
			{ID: "sacrifice", Text: "As your action, eliminate 2 of your Cultists from anywhere on the map."},
			{ID: "share", Text: "Share areas with all enemies (you and each enemy have units in some area). Gain 1 Power per enemy."},
			{ID: "awaken", Text: "Awaken Shub-Niggurath."},
		},
	},
	{
		Key: "ys", Name: "Yellow Sign", Color: "yellow", Seat: 4, Start: "europe",
		Ability: "Feast: in Gather Power gain 1 Power for each area that holds a Desecration token and at least one of your units.",
		Units: []unitSpec{
			{ID: uCultist, Name: "Acolyte Cultist", Class: classCultist, Count: 6, Cost: 1, Combat: 0},
			{ID: uUndead, Name: "Undead", Class: classMonster, Count: 6, Cost: 1, Combat: 0, Text: "Combat: roll one die fewer than the number of Undead in the battle."},
			{ID: uByak, Name: "Byakhee", Class: classMonster, Count: 4, Cost: 2, Combat: 0, Text: "Combat: roll one die more than the number of Byakhee in the battle."},
			{ID: uKing, Name: "King in Yellow", Class: classGOO, Count: 1, Cost: 4, Combat: 0, Abilities: []abilitySpec{{Name: "Desecrate", Timing: "Action: Cost 2", Text: "If the King is in an area with no Desecration token, roll 1 die. If the roll is equal to or less than the number of your units in the area (including the King), place a Desecration token in the area. Whether you succeed or fail, place a Monster or Cultist with a cost of 2 or less in the area."}}},
			{ID: uHastur, Name: "Hastur", Class: classGOO, Count: 1, Cost: 10, Combat: 0, Abilities: []abilitySpec{{Name: "Vengeance", Timing: "Post-Battle", Text: "If Hastur is involved in a battle, choose which combat results are applied to which enemy units (e.g., apply a Kill to a particular Great Old One)."}}},
		},
		Books: []bookSpec{
			{ID: "passion", Name: "Passion", Timing: "Ongoing", Text: "When one or more of your Cultists are eliminated by an enemy, gain 1 Power.", Units: []string{uCultist}},
			{ID: "hwinn", Name: "He Who is Not to be Named", Timing: "Action: Cost 1", Text: "Move Hastur to any area containing a Cultist of any faction. You may then take a second, different action — not The Screaming Dead.", Units: []string{uHastur}},
			{ID: "thirdeye", Name: "The Third Eye", Timing: "Ongoing", Text: "While Hastur is in play, Desecrating costs 1 less; if it succeeds you also gain 1 Elder Sign.", Units: []string{uHastur, uKing}},
			{ID: "shriek", Name: "Shriek of the Byakhee", Timing: "Action: Cost 1", Text: "Move any or all Byakhee from where they stand to one area on the map.", Units: []string{uByak}},
			{ID: "screaming", Name: "The Screaming Dead", Timing: "Action: Cost 1", Text: "Move the King in Yellow to an adjacent area; any Undead with him may come along for free. You may then take a second, different action — not He Who is Not to be Named.", Units: []string{uKing, uUndead}},
			{ID: "zingaya", Name: "Zingaya", Timing: "Action: Cost 1", Text: "Choose an area with your Undead and an enemy Cultist: that enemy eliminates one of its Cultists there and you place an Undead from your pool.", Units: []string{uUndead}},
		},
		Reqs: []reqSpec{
			{ID: "king", Text: "Awaken the King in Yellow."},
			{ID: "dragon", Text: "Successfully Desecrate an area marked with the Dragon glyph."},
			{ID: "chevron", Text: "Successfully Desecrate an area marked with the Chevron glyph."},
			{ID: "thorns", Text: "Successfully Desecrate an area marked with the Thorns glyph."},
			{ID: "hastur", Text: "Awaken Hastur. Receive 1 Elder Sign."},
			{ID: "gift", Text: "As your action, choose another player to gain 3 Doom."},
		},
	},
	{
		Key: "ww", Name: "Windwalker", Color: "white", Seat: 5, Start: "arctic-ocean",
		Ability: "Hibernate (Action, cost 0): gain 1 Power for each enemy Great Old One in play, at most as much as you already have; you take no more actions this Action phase and keep your Power: in the next Gather Power phase it is added to what you gather.",
		Units: []unitSpec{
			{ID: uCultist, Name: "Acolyte Cultist", Class: classCultist, Count: 6, Cost: 1, Combat: 0, Text: "Cannibalism (spellbook): can be placed in any battle where an enemy unit was killed."},
			{ID: uWendigo, Name: "Wendigo", Class: classMonster, Count: 4, Cost: 1, Combat: 1, Text: "Howl (spellbook): before a battle, forces the enemy to retreat one unit. Cannibalism (spellbook): can be placed in any battle where an enemy unit was killed."},
			{ID: uGnoph, Name: "Gnoph-Keh", Class: classMonster, Count: 4, Cost: 4, Combat: 3, Text: "Costs as many Power as there are Gnoph-Keh in your pool. Berserkergang (spellbook): for each Gnoph-Keh assigned a Kill the enemy eliminates a Monster or Cultist."},
			{ID: uRhan, Name: "Rhan-Tegoth", Class: classGOO, Count: 1, Cost: 6, Combat: 3, Abilities: []abilitySpec{{Name: "Eternal", Timing: "Post-Battle", Text: "If Rhan-Tegoth receives a Pain or a Kill, you may pay 1 Power to cancel its effect on him. He can only receive one combat result per battle."}}},
			{ID: uIthaqua, Name: "Ithaqua", Class: classGOO, Count: 1, Cost: 6, Combat: 0, Abilities: []abilitySpec{{Name: "Ferox", Timing: "Ongoing", Text: "While Ithaqua is in play, your Cultists cannot be captured by enemy Monsters or Terrors. They are still vulnerable to enemy Great Old Ones."}}},
		},
		Books: []bookSpec{
			{ID: "arctic-wind", Name: "Arctic Wind", Timing: "Ongoing", Text: "When Ithaqua uses the Move Action, any or all your units in the same area can move with him for no additional cost.", Units: []string{uIthaqua}},
			{ID: "berserkergang", Name: "Berserkergang", Timing: "Post-Battle", Text: "For each Gnoph-Keh assigned a Kill in battle, the enemy must eliminate one of its own Monsters or Cultists (anywhere on the map).", Units: []string{uGnoph}},
			{ID: "cannibalism", Name: "Cannibalism", Timing: "Post-Battle", Text: "After all battle results have been applied, if one or more enemy units were killed, you may place a Wendigo or Acolyte from your pool into the battle area — even if you were not in the battle.", Units: []string{uWendigo, uCultist}},
			{ID: "herald", Name: "Herald of the Outer Gods", Timing: "Ongoing", Text: "Pay 5 Power for Windwalker's Ritual of Annihilation, whatever the Ritual track says.", Units: []string{uRhan, uIthaqua}},
			{ID: "howl", Name: "Howl", Timing: "Pre-Battle", Text: "Before a battle, if any Wendigo is present, you may force the enemy to retreat one unit (its choice) out of the area into an adjacent one. It is not a Pain: the unit may go to an area that holds your units.", Units: []string{uWendigo}},
			{ID: "ice-age", Name: "Ice Age", Timing: "Action: Cost 1", Text: "Place your Ice Age token in any area. When an enemy takes an Action that ends in that area it must spend 1 Power more.", Units: nil},
		},
		Reqs: []reqSpec{
			{ID: "rhan", Text: "Awaken Rhan-Tegoth."},
			{ID: "gate", Text: "A Gate exists in the area marked with the Windwalker glyph in which you did not start."},
			{ID: "first", Text: "You are the First Player."},
			{ID: "six", Text: "Another faction has 6 spellbooks on its faction card."},
			{ID: "take", Text: "Take this spellbook at any time. Gain 1 Elder Sign for each enemy with 6 spellbooks, to a maximum of 3."},
			{ID: "ithaqua", Text: "Awaken Ithaqua."},
		},
	},
	{
		Key: "sl", Name: "Sleeper", Color: "orange", Seat: 6, Start: "north-america-west",
		Ability: "Death from Below (Doom phase): place the lowest-cost Monster from your pool into any area that holds at least one of your units.",
		Units: []unitSpec{
			{ID: uCultist, Name: "Acolyte Cultist", Class: classCultist, Count: 6, Cost: 1, Combat: 0},
			{ID: uWizard, Name: "Wizard", Class: classMonster, Count: 2, Cost: 1, Combat: 0, Text: "Energy Nexus (spellbook): just before a battle in its area you may take an Action that starts there."},
			{ID: uSerpent, Name: "Serpent Man", Class: classMonster, Count: 3, Cost: 2, Combat: 1, Text: "Ancient Sorcery (spellbook): placed on an enemy's faction card, it lends you that faction's unique ability."},
			{ID: uFormless, Name: "Formless Spawn", Class: classMonster, Count: 4, Cost: 3, Combat: 0, Text: "Combat: each rolls one die per Formless Spawn on the map, and one more while Tsathoggua is in play."},
			{ID: uTsath, Name: "Tsathoggua", Class: classGOO, Count: 1, Cost: 8, Combat: 2, Abilities: []abilitySpec{{Name: "Lethargy", Timing: "Action: Cost 0", Text: "If Tsathoggua is in play, do nothing. This counts as an Action."}}},
		},
		Books: []bookSpec{
			{ID: "ancient-sorcery", Name: "Ancient Sorcery", Timing: "Action: Cost 1", Text: "Remove a Serpent Man from the map and put him on an enemy's faction card: until the end of the next Doom phase you have that faction's unique ability (where it names a Great Old One it includes Tsathoggua). Then gain 1 Power and put the Serpent Man anywhere on the map.", Units: []string{uSerpent}},
			{ID: "burrow", Name: "Burrow", Timing: "Ongoing", Text: "After a Move Action in which you spent 2 or more Power moving units, regain 1 Power.", Units: nil},
			{ID: "capture-monster", Name: "Capture Monster", Timing: "Action: Cost 1", Text: "Tsathoggua can capture enemy Monsters the way Cultists are captured. Each is sacrificed for 1 Power in the next Gather Power phase.", Units: []string{uTsath}},
			{ID: "cursed-slumber", Name: "Cursed Slumber", Timing: "Action: Cost 1", Text: "Remove one of your controlled Gates and its Cultist from the map onto your faction card: they still give Doom and Power but are immune to enemy abilities and spellbooks. For 1 Power, as an Action, put them back into any area without a Gate. Only one Gate may sleep at a time.", Units: nil},
			{ID: "demand-sacrifice", Name: "Demand Sacrifice", Timing: "Pre-Battle", Text: "If Tsathoggua is in play, before a battle with you your enemy chooses: you gain an Elder Sign, or all its Kill results against your units in this battle count as Pains.", Units: []string{uTsath}},
			{ID: "energy-nexus", Name: "Energy Nexus", Timing: "Ongoing", Text: "Just before a battle in an area that holds a Wizard, you may take one Action that starts in that area, for its normal Power cost. The battle then goes on from its Pre-Battle abilities.", Units: []string{uWizard}},
		},
		Reqs: []reqSpec{
			{ID: "boon", Text: "As your Action, spend 3 Power. Each other player gains 1 Power."},
			{ID: "gift", Text: "As your Action, spend 3 Power. Select another player, who gains 3 Power."},
			{ID: "bane", Text: "As your Action, spend 3 Power. Each other player loses 1 Power."},
			{ID: "six-dice", Text: "Roll 6 combat dice in a single battle."},
			{ID: "ritual", Text: "Perform a Ritual of Annihilation."},
			{ID: "awaken", Text: "Awaken Tsathoggua."},
		},
	},
	{
		Key: "ow", Name: "Opener of the Way", Color: "purple", Seat: 7, Start: "",
		Ability: "The Beyond One (Action, cost 1): choose one of your units that costs 3 or more, in an area with a Gate and no enemy Great Old One. Move that unit, the Gate and any unit controlling it to any area that has no Gate.",
		Units: []unitSpec{
			{ID: uCultist, Name: "Acolyte Cultist", Class: classCultist, Count: 6, Cost: 1, Combat: 0, Text: "The Million Favored Ones (spellbook): becomes a Mutant after a battle."},
			{ID: uMutant, Name: "Mutant", Class: classMonster, Count: 4, Cost: 2, Combat: 1, Text: "The Million Favored Ones (spellbook): becomes an Abomination after a battle."},
			{ID: uAbom, Name: "Abomination", Class: classMonster, Count: 3, Cost: 3, Combat: 2, Text: "The Million Favored Ones (spellbook): becomes a Spawn of Yog-Sothoth after a battle. Dread Curse of Azathoth rolls a die for each."},
			{ID: uSpawn, Name: "Spawn of Yog-Sothoth", Class: classMonster, Count: 2, Cost: 4, Combat: 3, Text: "Dread Curse of Azathoth rolls a die for each. Yog-Sothoth is awakened by replacing one."},
			{ID: uYog, Name: "Yog-Sothoth", Class: classGOO, Count: 1, Cost: 6, Combat: 0, Abilities: []abilitySpec{{Name: "The Key and the Gate", Timing: "Ongoing", Text: "Yog-Sothoth counts as a Gate for every purpose, except for the Beyond One ability. He is not controlled by any Cultist, and he can exist in the same area as another Gate."}}},
		},
		Books: []bookSpec{
			{ID: "channel-power", Name: "Channel Power", Timing: "Battle", Text: "After rolling Battle dice, you may pay 1 Power to reroll all dice that were not Kills or Pains. You may do this more than once.", Units: nil},
			{ID: "dragon-ascending", Name: "Dragon Ascending", Timing: "Once Only", Text: "Once during the game, at any time, set your Power equal to the current Power of one chosen enemy faction. It cannot be used again.", Units: nil},
			{ID: "dragon-descending", Name: "Dragon Descending", Timing: "Once Only", Text: "Once during the game when you perform a Ritual of Annihilation, you receive twice the normal Doom points. It cannot be used again.", Units: nil},
			{ID: "dread-curse", Name: "Dread Curse of Azathoth", Timing: "Action: Cost 2", Text: "Select an area and roll 1 die for each Abomination and Spawn of Yog-Sothoth in play. Apply the Kills and Pains to enemy factions in the area: you choose which factions receive which results, they choose which of their units takes each, and you choose the area each Pained unit goes to. No Battle abilities apply.", Units: []string{uAbom, uSpawn}},
			{ID: "million-favored", Name: "The Million Favored Ones", Timing: "Post-Battle", Text: "After Pains and Kills are resolved, replace any or all surviving Acolytes with Mutants, Mutants with Abominations and Abominations with Spawns of Yog-Sothoth. A Spawn can be replaced by as many Mutants as are in your pool.", Units: []string{uCultist, uMutant, uAbom, uSpawn}},
			{ID: "they-break-through", Name: "They Break Through", Timing: "Ongoing", Text: "You can summon Monsters at enemy-controlled and abandoned Gates; you need no units in the area.", Units: nil},
		},
		Reqs: []reqSpec{
			{ID: "gates-8", Text: "There are 8 Gates on the map."},
			{ID: "gates-12", Text: "There are 12 Gates on the map. (In a 2–3 player game, only 10.)"},
			{ID: "enemy-gates", Text: "You have units in at least 2 areas that hold enemy-controlled Gates."},
			{ID: "lose", Text: "Lose 1 of your own units in a battle."},
			{ID: "meet", Text: "Your Great Old One is in the same area as an enemy Great Old One."},
			{ID: "awaken", Text: "Awaken Yog-Sothoth."},
		},
	},
	{
		Key: "tt", Name: "Tcho-Tcho", Color: "brown", Seat: 8, Start: "",
		Ability: "Sycophancy (Doom phase): when an enemy performs a Ritual of Annihilation, either you gain 1 Doom or he earns 1 fewer Doom, his choice.",
		Units: []unitSpec{
			{ID: uCultist, Name: "Acolyte Cultist", Class: classCultist, Count: 6, Cost: 1, Combat: 0},
			{ID: uProto, Name: "Proto-Shoggoth", Class: classMonster, Count: 6, Cost: 2, Combat: 1, Text: "Terror (spellbook): each lowers the enemy's Combat or raises yours by 1."},
			{ID: uUbbo, Name: "Ubbo-Sathla", Class: classGOO, Count: 1, Cost: 6, Combat: 0, Abilities: []abilitySpec{{Name: "Hell's Banquet", Timing: "Doom Phase", Text: "Once Ubbo-Sathla has been awakened, starting with the following Doom phase, and whether or not he is still in play, roll 1 die and increase his Growth counter by the result. His Combat equals the Growth counter."}}},
		},
		Books: []bookSpec{
			{ID: "hierophants", Name: "Hierophants", Timing: "Ongoing", Text: "When you earn a faction spellbook (this one too), place a High Priest at one of your Gates. If there is none in your pool, advance Ubbo-Sathla's Growth counter by 1 instead. When this spellbook is first taken with the High Priests option, every faction places a High Priest at one of its Gates.", Units: []string{uHighPriest}},
			{ID: "soulless", Name: "Soulless", Timing: "Ongoing", Text: "When your captured Cultists are sacrificed they provide 0 Power (instead of the normal 1).", Units: []string{uCultist}},
			{ID: "terror", Name: "Terror", Timing: "Battle", Text: "Choose one: your enemy's Combat is reduced by 1 per Proto-Shoggoth in the battle; or your Combat is increased by 1 per Proto-Shoggoth in the battle.", Units: []string{uProto}},
			{ID: "idolatry", Name: "Idolatry", Timing: "Action: Cost 1", Text: "Select an area containing another faction's starting glyph (even if that faction is not in play). Move any or all of your units from adjacent areas into it.", Units: nil},
			{ID: "martyrdom", Name: "Martyrdom", Timing: "Post-Battle", Text: "If your High Priest is assigned a Kill, all Kills assigned to your other units become Pains instead.", Units: []string{uHighPriest}},
			{ID: "tablets", Name: "Tablets of the Gods", Timing: "Doom Phase", Text: "When you perform a Ritual of Annihilation, you also receive 1 Elder Sign for each Gate at which you have any High Priests. Then eliminate all your High Priests (not optional).", Units: []string{uHighPriest}},
		},
		Reqs: []reqSpec{
			{ID: "ritual", Text: "Another player performs a Ritual of Annihilation, or another player has 15 Doom."},
			{ID: "sign", Text: "Earn an Elder Sign."},
			{ID: "three-signs", Text: "Own 3 or more Elder Signs."},
			{ID: "dismantle", Text: "As an Action, remove your controlled Gate in your start area."},
			{ID: "killed", Text: "Any Great Old One is Killed in a battle."},
			{ID: "awaken", Text: "Awaken Ubbo-Sathla."},
		},
	},
}

// hpSpec is the High Priest of the High Priests expansion: a Cultist of its
// own kind — recruited like an Acolyte, for 3 Power — that every cult has one
// of when the expansion is in the game.
var hpSpec = unitSpec{
	ID: uHighPriest, Name: "High Priest", Class: classCultist, Count: 1, Cost: 3, Combat: 0,
	Text: "A Cultist: recruited like an Acolyte (3 Power), it gives 1 Power in Gather Power, can create and control a Gate and can be captured. Unspeakable Oath: at the end of any player's Action, in Gather Power or in the Doom phase, sacrifice him (back to your pool) to gain 2 Power.",
}

// unitSpec finds a unit's spec in a faction's box (the High Priest belongs to
// every box when the expansion is in play).
func (f *faction) unitSpec(id string) (unitSpec, bool) {
	for _, u := range f.Units {
		if u.ID == id {
			return u, true
		}
	}
	if id == uHighPriest {
		return hpSpec, true
	}
	return cardUnitSpec(id)
}

// unitTypes lists every piece type id once, in a stable order (the index of
// a type in this list is its unit-type number). The core cults come first, so
// their numbers do not change when an expansion cult is added.
var unitTypes = func() []string {
	seen := map[string]bool{}
	var out []string
	add := func(from, to int) {
		for _, f := range factions[from:to] {
			for _, u := range f.Units {
				if !seen[u.ID] {
					seen[u.ID] = true
					out = append(out, u.ID)
				}
			}
		}
	}
	add(0, facOW)
	out = append(out, uHighPriest)
	for _, u := range neutralUnits {
		out = append(out, u.ID)
	}
	// The Opener's and the Tcho-Tcho's units come last: the numbers of the
	// others do not move.
	add(facOW, nFactions)
	// The Loyalty Cards after the first neutral Monsters, and Abhoth's Filth.
	for _, u := range cardUnits[len(neutralUnits):] {
		out = append(out, u.ID)
	}
	out = append(out, uFilth)
	return out
}()

var unitTypeIndex = func() map[string]int {
	m := map[string]int{}
	for i, id := range unitTypes {
		m[id] = i
	}
	return m
}()

// unitClassByType holds the class of every unit type, by unit-type number.
var unitClassByType = func() []int {
	out := make([]int, len(unitTypes))
	for i, id := range unitTypes {
		for _, f := range factions {
			if u, ok := f.unitSpec(id); ok {
				out[i] = u.Class
				break
			}
		}
	}
	return out
}()

// unitClassOf says whether a unit type is a cultist, monster or Great Old One.
func unitClassOf(id string) int {
	if t, ok := unitTypeIndex[id]; ok {
		return unitClassByType[t]
	}
	return classCultist
}

// unitNameOf is a unit type's display name.
func unitNameOf(id string) string {
	for _, f := range factions {
		if u, ok := f.unitSpec(id); ok {
			return u.Name
		}
	}
	return id
}

// isCultist says whether a unit type is a Cultist: an Acolyte or a High
// Priest.
func isCultist(typ string) bool { return typ == uCultist || typ == uHighPriest }

// factionIndex is the index of the faction with key k, or -1.
func factionIndex(k string) int {
	for i := range factions {
		if factions[i].Key == k {
			return i
		}
	}
	return -1
}

// factionByColor is the index of the faction that plays color, or -1.
func factionByColor(color string) int {
	for i := range factions {
		if factions[i].Color == color {
			return i
		}
	}
	return -1
}
