package cthulhuwars

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
)

// The later packs of Loyalty Cards: the Dreamlands Underworld Monsters, the
// Cosmic Terrors, Something About Cats, the Ramsey Campbell Horrors and Great
// Old One Packs 2 to 4.
//
// Their decisions share one action, "card": Piece names the choice and To and
// Count say where and whom, so a new ability needs a line in cardActs rather
// than a new verb in every switch of the engine. A question that has to wait
// for the end of what is going on goes in Data.Pending, which the end of Apply
// opens with play resuming where it was.

// Unit ids of the later cards.
const (
	uGhast    = "ghast"
	uLeng     = "leng-spider"
	uGug      = "gug"
	uSatyr    = "satyr"
	uMercury  = "cat-from-mercury"
	uVenus    = "cat-from-venus"
	uAsteroid = "asteroid-cat"
	uYith     = "great-race-of-yith"
	uDhole    = "dhole"
	uQuachil  = "quachil-uttaus"
	uNeptune  = "cat-from-neptune"
	uAtlach   = "atlach-nacha"
	uBokrug   = "bokrug"
	uDagon    = "father-dagon"
	uGhatan   = "ghatanothoa"
	uGobogeg  = "gobogeg"
	uByatis   = "byatis"
	uNyogtha  = "nyogtha"
	uTulzscha = "tulzscha"
	uGlaaki   = "glaaki"
	uDaoloth  = "daoloth"
	uYgolonac = "ygolonac"
	uHagarg   = "hagarg-ryonis"
)

// The packs added after Great Old One Pack 1, as bits of gameCfg.packs.
const (
	packUnderworld = 1 << (iota + 4) // Dreamlands Underworld Monster Pack
	packCosmic                       // Cosmic Terrors
	packCats                         // Something About Cats
	packCampbell                     // the Satyr of Ramsey Campbell Horrors 2
)

const maxWebs = 6

// laterMonsters are the cards of the packs of Monsters and Terrors, after the
// first Independents in the order of the bits of Data.Loyal.
var laterMonsters = []cardSpec{
	{Kind: cardMonster, Pack: packUnderworld, Buy: "Pay 2 Doom to obtain this Loyalty Card, plus place all 4 Ghasts at your controlled Gate(s).",
		Unit: unitSpec{ID: uGhast, Name: "Ghast", Class: classMonster, Count: 4, Cost: 2, Combat: 0,
			Text:      "Hordeling: 2 Power brings every Ghast of your pool to your Gates at once.",
			Abilities: []abilitySpec{{Name: "Hordeling", Timing: "Ongoing", Text: "When you spend 2 Power to Summon Ghasts, all Ghasts in your pool are immediately placed on the map at any Gate(s) you control."}}}},
	{Kind: cardMonster, Pack: packUnderworld, Buy: "Pay 2 Doom to obtain this Loyalty Card, plus place 1 Leng Spider at your controlled Gate.",
		Unit: unitSpec{ID: uLeng, Name: "Leng Spider", Class: classMonster, Count: 3, Cost: 2, Combat: 1,
			Text:      "Bloodthirst: once per Spider in a battle, turn two Pain results into a Kill, on either side.",
			Abilities: []abilitySpec{{Name: "Bloodthirst", Timing: "Post-Battle", Text: "If a Leng Spider is involved in a Battle, you may exchange two Pain results for a Kill. You may do this once per Leng Spider in the Battle. Each use of this ability can either apply to your own results or your opponent's."}}}},
	{Kind: cardMonster, Pack: packUnderworld, Buy: "Pay 2 Doom to obtain this Loyalty Card, plus place 1 Gug at your controlled Gate.",
		Unit: unitSpec{ID: uGug, Name: "Gug", Class: classMonster, Count: 2, Cost: 1, Combat: 3,
			Text:      "Clumsy: a Gug cannot Capture a Cultist.",
			Abilities: []abilitySpec{{Name: "Clumsy", Timing: "Ongoing", Text: "A Gug cannot Capture a Cultist."}}}},
	{Kind: cardTerror, Pack: packCosmic, Buy: "Pay 2 Doom and 2 Power to obtain this Loyalty Card, then place the Great Race of Yith at your Controlled Gate.",
		Unit: unitSpec{ID: uYith, Name: "Great Race of Yith", Class: classTerror, Count: 1, Cost: 4, Combat: 3,
			Text:      "A Terror. Possession: it captures a Cultist whatever guards it, and in play earns 1 more Power per captive in Gather Power.",
			Abilities: []abilitySpec{{Name: "Possession", Timing: "Ongoing and Gather Power Phase", Text: "If your Great Race of Yith is in the Area, you can capture an enemy Cultist regardless of the presence of enemy Monsters, Terrors or Great Old Ones (or whether Windwalker's Ferox ability is in effect). In addition, if the Great Race of Yith is in play during the Gather Power Phase, earn 1 Power per Captured Cultist in addition to your normal reward (of 1 Power)."}}}},
	{Kind: cardTerror, Pack: packCosmic, Buy: "Pay 2 Doom and 2 Power to obtain this Loyalty Card, then place the Dhole at your Controlled Gate.",
		Unit: unitSpec{ID: uDhole, Name: "Dhole", Class: classTerror, Count: 1, Cost: 4, Combat: 5,
			Text:      "A Terror. Planetary Destruction: lost in a battle, it earns you 2 Elder Signs and your opponent 2 Doom or 2 Power, as you choose.",
			Abilities: []abilitySpec{{Name: "Planetary Destruction", Timing: "Post-Battle", Text: "If the Dhole is Killed or Eliminated in Battle, earn 2 Elder Signs. In addition, your opponent earns 2 Doom or 2 Power, your choice."}}}},
	{Kind: cardTerror, Pack: packCosmic, Buy: "Pay 2 Doom and 2 Power to obtain this Loyalty Card, then place Quachil Uttaus at your Controlled Gate.",
		Unit: unitSpec{ID: uQuachil, Name: "Quachil Uttaus", Class: classTerror, Count: 1, Cost: 4, Combat: 1,
			Text:      "A Terror. Dust to Dust: an enemy who lost units in its battle removes one from the game or gives you an Elder Sign.",
			Abilities: []abilitySpec{{Name: "Dust to Dust", Timing: "Post-Battle", Text: "If an enemy Unit is Killed or Eliminated in a Battle involving Quachil Uttaus, the owner must choose one of the following options: select one of his lost Units to be permanently removed from the game, or you receive an Elder Sign."}}}},
	{Kind: cardMonster, Pack: packCats, Buy: "Pay 2 Doom to obtain this Loyalty Card. Place a Cat from Mercury at your Controlled Gate.",
		Unit: unitSpec{ID: uMercury, Name: "Cat from Mercury", Class: classMonster, Count: 2, Cost: 1, Combat: 1,
			Text:      "Needs Affection: before a battle it is in, you may swap your Monsters there for Acolytes of your pool.",
			Abilities: []abilitySpec{{Name: "Needs Affection", Timing: "Pre-Battle", Text: "If a Cat from Mercury is in a Battle, you can choose one or more of your Monsters present at that Battle, and immediately replace them with your Acolyte Cultists from your Pool."}}}},
	{Kind: cardMonster, Pack: packCats, Buy: "Pay 2 Doom to obtain this Loyalty Card. Place a Cat from Venus at your Controlled Gate.",
		Unit: unitSpec{ID: uVenus, Name: "Cat from Venus", Class: classMonster, Count: 2, Cost: 1, Combat: 1,
			Text:      "Honeymoon: a Cultist it captures earns you 1 Power at once and goes straight home.",
			Abilities: []abilitySpec{{Name: "Honeymoon", Timing: "Ongoing", Text: "When a Cat from Venus Captures a Cultist, gain 1 Power and immediately return the Cultist to the owner's Pool."}}}},
	{Kind: cardMonster, Pack: packCats, Buy: "Pay 2 Doom to obtain this Loyalty Card. Place an Asteroid Cat at your Controlled Gate.",
		Unit: unitSpec{ID: uAsteroid, Name: "Asteroid Cat", Class: classMonster, Count: 2, Cost: 1, Combat: 1,
			Text:      "Abandoned to Lusts: it can leave the map for an empty spellbook slot of an enemy beside it; you then choose the next book they earn.",
			Abilities: []abilitySpec{{Name: "Abandoned to Lusts", Timing: "Action: Cost 0", Text: "If an Asteroid Cat shares an Area with a Unit from another Faction that has 5 or fewer Faction Spellbooks, remove the Cat from the Map and place it on one of that enemy's empty Spellbook slots. When that enemy earns that Spellbook, return the Asteroid Cat to your Controlled Gate, and you get to choose immediately which of his available Spellbooks he must take in that slot."}}}},
	{Kind: cardTerror, Pack: packCats, Buy: "Pay 2 Doom and 2 Power to obtain this Loyalty Card. Place the Cat from Neptune at your Controlled Gate.",
		Unit: unitSpec{ID: uNeptune, Name: "Cat from Neptune", Class: classTerror, Count: 1, Cost: 1, Combat: 3,
			Text:      "A Terror. The Final Ritual: Killed in a battle, it lets you perform a Ritual of Annihilation at once.",
			Abilities: []abilitySpec{{Name: "The Final Ritual", Timing: "Post-Battle", Text: "If the Cat from Neptune is Killed in a Battle, you may immediately perform a Ritual of Annihilation. If this causes the marker to move to Instant Death, the game ends after this action."}}}},
	{Kind: cardMonster, Pack: packCampbell, Buy: "Pay 2 Doom to obtain this Loyalty Card, plus place a Satyr and an Acolyte Cultist at your Controlled Gate.",
		Unit: unitSpec{ID: uSatyr, Name: "Satyr", Class: classMonster, Count: 3, Cost: 2, Combat: 1,
			Text:      "Fecund: each Satyr summoned brings an Acolyte of your pool with it.",
			Abilities: []abilitySpec{{Name: "Fecund", Timing: "Ongoing", Text: "Each time you Summon a Satyr, also place an Acolyte Cultist from your Pool in the same Area."}}}},
}

// laterIndependents are the Independent Great Old Ones after Great Old One Pack 1.
var laterIndependents = []independent{
	{Unit: unitSpec{ID: uAtlach, Name: "Atlach-Nacha", Class: classGOO, Count: 1, Cost: 6, Combat: 4,
		Abilities: []abilitySpec{{Name: "Spinnerets", Timing: "Action: Cost 1", Text: "If Atlach-Nacha is in an Area that is lacking a Web Token, place one there."}}},
		Book: bookSpec{ID: "cosmic-web", Name: "Cosmic Web", Timing: "Action: Cost 0", Text: "Immediately win the game, even with fewer than 6 Faction Spellbooks.", Units: []string{uAtlach}},
		Req:  "Web Tokens are in 6 different Areas.", Awaken: awakenIndependentFor("Atlach-Nacha", 6)},
	{Unit: unitSpec{ID: uBokrug, Name: "Bokrug", Class: classGOO, Count: 1, Cost: 4, Combat: 1,
		Abilities: []abilitySpec{{Name: "Ghosts of Ib", Timing: "Ongoing", Text: "If Bokrug is killed, do not lose this Loyalty Card. At the end of the next Doom phase, return him to the map to any Area not containing enemy units. If there is no such Area, he remains on the card until the next Doom phase."}}},
		Book: bookSpec{ID: "doom-that-came-to-sarnath", Name: "Doom that Came to Sarnath", Timing: "Ongoing", Text: "At the end of the Doom phase, select an enemy. Then select one of these two options: that enemy chooses a Monster or Cultist of yours to eliminate, or that enemy chooses one of your Elder Signs to eliminate.", Units: []string{uBokrug}},
		Req:  "Take this Loyalty Card, and hand it to the player of your choice. He then places Bokrug's spellbook on this card, and it becomes active.", Awaken: awakenIndependent("Bokrug")},
	{Unit: unitSpec{ID: uDagon, Name: "Father Dagon", Class: classGOO, Count: 1, Cost: 4, Combat: 2,
		Text:      "Combat is 2 on land and 6 in an Ocean Area.",
		Abilities: []abilitySpec{{Name: "Tsunami", Timing: "Action: Cost 1", Text: "Choose a Land area adjacent to an Ocean Area. All Cultists in the Area must be moved into Adjacent Ocean Areas by their owners (this includes your Cultists). In a dispute over who moves first, you decide."}}},
		Book: bookSpec{ID: "the-innsmouth-look", Name: "The Innsmouth Look", Timing: "Ongoing", Text: "During the Doom phase, remove one of your Acolyte Cultists from the map and out of the game permanently. Gain 6 Power. This is not optional. If you have no Acolyte Cultists on the map, there is no effect.", Units: []string{uDagon}},
		Req:  "You have 8 Units in Ocean Areas.", Awaken: awakenIndependent("Father Dagon")},
	{Unit: unitSpec{ID: uGhatan, Name: "Ghatanothoa", Class: classGOO, Count: 1, Cost: 4, Combat: 0,
		Text:      "Combat equals your opponent's total Cultists on the Map.",
		Abilities: []abilitySpec{{Name: "Mummify", Timing: "Action: Cost 1", Text: "Any enemy Acolyte Cultists sharing an Area with Ghatanothoa are immediately Mummified. Such Cultists cannot use the Move Action, do not participate in Battle, and during the next Gather Power Phase produce no Power. During the Doom Phase, stand the Cultist back up. A Mummified Cultist still Controls a Gate it is on and can be Captured."}}},
		Book: bookSpec{ID: "execration-of-mu", Name: "Execration of Mu", Timing: "Ongoing", Text: "The Mummify Ability is no longer an Action. It now occurs instantly when any enemy Acolyte Cultists share an Area with Ghatanothoa.", Units: []string{uGhatan}},
		Req:  "Choose one: EITHER your total Gates + Cultists is fewer than 6 in the Doom Phase OR, as your Action, Pay 4 Power.", Awaken: awakenIndependent("Ghatanothoa")},
	{Unit: unitSpec{ID: uGobogeg, Name: "Gobogeg", Class: classGOO, Count: 1, Cost: 0, Combat: 0,
		Text:      "Rolls 0 dice, but if Gobogeg is Killed or Pained, all units in the Area are Pained, whoever's they are.",
		Abilities: []abilitySpec{{Name: "Book of Law", Timing: "Ongoing", Text: "While Gobogeg is in play, whenever a Great Old One is Awakened, the owner receives 6 Power after the Awakening."}}},
		Book:   bookSpec{ID: "book-of-chaos", Name: "Book of Chaos", Timing: "Post-Game!", Text: "If the game ends while you control Gobogeg, immediately place this Spellbook and gloat. Then YOU get to pick the Factions, Expansions, Map, Neutral Monsters, and Independent Great Old Ones to be used the next time your group plays Cthulhu Wars.", Units: []string{uGobogeg}},
		Req:    "Someone wins the game (need not be you).",
		Awaken: []string{"At least one player has 6 Faction Spellbooks.", "Your Controlled Gate is in an Area with your Great Old One.", "Pay 0 Power, and place Gobogeg in the Area containing the Gate."}},
	{Unit: unitSpec{ID: uByatis, Name: "Byatis", Class: classGOO, Count: 1, Cost: 4, Combat: 4,
		Abilities: []abilitySpec{{Name: "Toad of Berkeley", Timing: "Ongoing", Text: "Byatis may not Move, nor can he be moved with movement-type abilities (such as Arctic Winds or Submerge). He can still be Pained. If there are no enemy Units in Byatis' Area during the Doom Phase, earn 1 Elder Sign."}}},
		Book: bookSpec{ID: "god-of-forgetfulness", Name: "God of Forgetfulness", Timing: "Action: Cost 1", Text: "Select all enemy Cultists in an Area adjacent to Byatis. Those Cultists are moved into Byatis' Area.", Units: []string{uByatis}},
		Req:  "Byatis survives a Battle in which at least one enemy Unit is Killed.", Awaken: awakenIndependent("Byatis")},
	{Unit: unitSpec{ID: uNyogtha, Name: "Nyogtha", Class: classGOO, Count: 2, Cost: 6, Combat: 1,
		Text:      "Two figures. Combat is 4 if Nyogtha's Faction declared the Battle, 1 if not.",
		Abilities: []abilitySpec{{Name: "From Below", Timing: "Ongoing", Text: "Nyogtha is two Units. When one Nyogtha Unit Moves, so may the other at no extra cost. You only lose this Loyalty Card if both Nyogtha Units have been Killed."}}},
		Book:   bookSpec{ID: "nightmare-web", Name: "Nightmare Web", Timing: "Ongoing", Text: "If one of the Nyogtha Units is in your pool, you can Awaken it for 2 Power, placing it in any Area in which you have at least one Unit.", Units: []string{uNyogtha}},
		Req:    "Nyogtha survives a Battle against an enemy Great Old One.",
		Awaken: []string{"Your Controlled Gate is in an Area with your Great Old One.", "Pay 6 Power, and place all Nyogtha Units from your Pool to the Area containing the Gate."}},
	{Unit: unitSpec{ID: uTulzscha, Name: "Tulzscha", Class: classGOO, Count: 1, Cost: 4, Combat: 1,
		Abilities: []abilitySpec{{Name: "Undying Flame", Timing: "Gather Power Phase", Text: "At the end of the Gather Power Phase, gain 1 Doom if at least one Faction has more Doom than you, gain 1 Elder Sign if at least one Faction has more Elder Signs than you, and gain 1 Power if at least one Faction has more Power than you."}}},
		Book: bookSpec{ID: "ceremony-of-annihilation", Name: "Ceremony of Annihilation", Timing: "Doom Phase", Text: "When you perform a Ritual of Annihilation you may choose to pay nothing, instead gain Power equal to the current position of the Ritual of Annihilation marker and then advance the marker 1 step. If you do not pay for the Ritual, you do not earn Doom points or Elder Signs.", Units: []string{uTulzscha}},
		Req:  "As your Action, each enemy Faction gains 2 Power.", Awaken: awakenIndependent("Tulzscha")},
	{Unit: unitSpec{ID: uGlaaki, Name: "Gla'aki", Class: classGOO, Count: 1, Cost: 4, Combat: 0,
		Text:      "Combat equals the number of Acolyte Cultists in your Pool.",
		Abilities: []abilitySpec{{Name: "The Tomb-Herd", Timing: "Gather Power Phase", Text: "Earn 1 Power per Acolyte Cultist in your Pool. (Count these before any enemy Sacrificing your Captured Cultists gives them back to you.)"}}},
		Book: bookSpec{ID: "green-decay", Name: "Green Decay", Timing: "Gather Power Phase", Text: "When a Captured Cultist is Sacrificed and returned, gain an Elder Sign instead of your normal reward (of 1 Power).", Units: []string{uGlaaki}},
		Req:  "You are the first player to reach 0 Power in the Action Phase. (Windwalker taking the Hibernate Action does NOT fulfill this requirement.)", Awaken: awakenIndependent("Gla'aki")},
	{Unit: unitSpec{ID: uDaoloth, Name: "Daoloth", Class: classGOO, Count: 1, Cost: 6, Combat: 0,
		Abilities: []abilitySpec{{Name: "Cosmic Unity", Timing: "Pre-Battle", Text: "In a Battle involving Daoloth, choose one enemy Great Old One. It rolls no Combat dice (it still gets its Battle Ability, if any)."}}},
		Book: bookSpec{ID: "interdimensional", Name: "Interdimensional", Timing: "Ongoing", Text: "When Daoloth enters an Area without a Gate, immediately place a Gate there.", Units: []string{uDaoloth}},
		Req:  "A Great Old One is Killed (anywhere on the map).", Awaken: awakenIndependentFor("Daoloth", 6)},
	{Unit: unitSpec{ID: uYgolonac, Name: "Y'Golonac", Class: classGOO, Count: 1, Cost: 2, Combat: 1,
		Abilities: []abilitySpec{{Name: "Orifices", Timing: "Post-Battle", Text: "If Y'Golonac is Killed in Battle, select a surviving enemy Terror, Monster, or Cultist. Replace it with Y'Golonac, then give Y'Golonac's Loyalty Card to that player. If no enemies survived, Y'Golonac dies normally. This ability is not optional."}}},
		Book: bookSpec{ID: "the-revelations", Name: "The Revelations", Timing: "Doom Phase", Text: "Every player except you gets 1 Elder Sign. This is not optional.", Units: []string{uYgolonac}},
		Req:  "You have just received Y'Golonac as a result of his Orifices ability.", Awaken: awakenIndependentFor("Y'Golonac", 2)},
	{Unit: unitSpec{ID: uHagarg, Name: "Hagarg Ryonis", Class: classGOO, Count: 1, Cost: 4, Combat: 0,
		Text:      "An Elder God, the Cat from Jupiter. She rolls no dice; instead she adds 3 Pains to your Combat total.",
		Abilities: []abilitySpec{{Name: "Subversion", Timing: "First Player Phase", Text: "Choose a player. If that player performs a Ritual of Annihilation in the Doom Phase, you steal 1 of the Elder Signs he earns (if any)."}}},
		Book: bookSpec{ID: "laziness", Name: "Laziness", Timing: "Action: Cost 0", Text: "Any faction with exactly 1 Power left loses that Power.", Units: []string{uHagarg}},
		Req:  "You have 0 Power.", Awaken: awakenIndependent("Hagarg Ryonis")},
}

// awakenIndependentFor is the usual way of awakening, at another price.
func awakenIndependentFor(name string, cost int) []string {
	return []string{"Your Controlled Gate is in an Area with your Great Old One.", fmt.Sprintf("Pay %d Power, and place %s in the Area containing the Gate.", cost, name)}
}

// LustState is an Asteroid Cat lying on an empty spellbook slot of Victim.
type LustState struct {
	Owner  int `json:"owner"`
	Victim int `json:"victim"`
}

// --- the "card" action -------------------------------------------------------------------------

const actCard = "card"

// cardAct is one decision of the action: Area and Seat say the move names an
// area or a cult, Units that its id ends in ".<unit type>".
type cardAct struct {
	ID, Label, Text string
	Cost            int
	Area, Seat      bool
	Units           bool
}

var cardActs = []cardAct{
	{ID: "horde", Label: "Place a Ghast", Text: "Hordeling: place a Ghast of your pool at a Gate you control.", Area: true},
	{ID: "blood", Label: "Bloodthirst", Text: "Turn two Pain results of this cult into a Kill.", Seat: true},
	{ID: "blood-pass", Label: "Keep the results", Text: "Bloodthirst: leave the results as they are."},
	{ID: "affection", Label: "Needs Affection", Text: "Replace this Monster with an Acolyte of your pool.", Units: true},
	{ID: "lust", Label: "Abandoned to Lusts", Text: "The Asteroid Cat leaves the map for an empty spellbook slot of this enemy.", Area: true, Seat: true},
	{ID: "spin", Label: "Spinnerets", Text: "Place a Web Token in Atlach-Nacha's area.", Cost: 1},
	{ID: "cosmic-web", Label: "Cosmic Web: win the game", Text: "Immediately win the game, even with fewer than 6 Faction Spellbooks."},
	{ID: "give-bokrug", Label: "Hand over Bokrug", Text: "Bokrug's requirement: hand him to this player, who gets his spellbook with him.", Seat: true},
	{ID: "ghost", Label: "Ghosts of Ib", Text: "Bokrug returns to this area.", Area: true},
	{ID: "sarnath-unit", Label: "Sarnath: a unit", Text: "This enemy chooses a Monster or Cultist of yours to eliminate.", Seat: true},
	{ID: "sarnath-sign", Label: "Sarnath: an Elder Sign", Text: "This enemy eliminates one of your Elder Signs, unseen.", Seat: true},
	{ID: "elim", Label: "Eliminate", Text: "Doom that Came to Sarnath: eliminate this unit.", Units: true},
	{ID: "tsunami", Label: "Tsunami", Text: "All Cultists in this land area move into adjacent oceans.", Area: true, Cost: 1},
	{ID: "innsmouth", Label: "The Innsmouth Look", Text: "Remove an Acolyte in this area from the game and gain 6 Power.", Area: true},
	{ID: "mummify", Label: "Mummify", Text: "The enemy Acolytes with Ghatanothoa cannot move, fight or give Power until the Doom phase.", Cost: 1},
	{ID: "ghatan-pay", Label: "Pay 4 Power to Ghatanothoa", Text: "It earns his spellbook, Execration of Mu.", Cost: 4},
	{ID: "forget", Label: "God of Forgetfulness", Text: "All enemy Cultists in this area adjacent to Byatis move into his area.", Area: true, Cost: 1},
	{ID: "nightmare", Label: "Nightmare Web", Text: "Awaken a Nyogtha of your pool in this area, where you have a unit.", Area: true, Cost: 2},
	{ID: "tulzscha-gift", Label: "Give every enemy 2 Power", Text: "Tulzscha's requirement: it earns his spellbook, Ceremony of Annihilation."},
	{ID: "ceremony", Label: "Ceremony of Annihilation", Text: "Instead of paying for a Ritual, gain Power equal to its cost and advance the marker. No Doom and no Elder Signs."},
	{ID: "laziness", Label: "Laziness", Text: "Every faction with exactly 1 Power left loses it."},
	{ID: "subvert", Label: "Subversion", Text: "If this player performs a Ritual in the Doom phase, you steal 1 of the Elder Signs it earns.", Seat: true},
	{ID: "orifice", Label: "Orifices", Text: "Y'Golonac replaces this surviving enemy unit, and its player gets his card.", Units: true},
	{ID: "dust", Label: "Remove from the game", Text: "Dust to Dust: this lost unit is gone for good.", Units: true},
	{ID: "dust-sign", Label: "Give an Elder Sign", Text: "Dust to Dust: Quachil Uttaus' owner receives an Elder Sign."},
	{ID: "planet-doom", Label: "They earn 2 Doom", Text: "Planetary Destruction: your opponent earns 2 Doom."},
	{ID: "planet-power", Label: "They earn 2 Power", Text: "Planetary Destruction: your opponent earns 2 Power."},
	{ID: "final-ritual", Label: "Perform the Final Ritual", Text: "The Final Ritual: perform a Ritual of Annihilation now."},
	{ID: "final-pass", Label: "No Ritual", Text: "The Final Ritual: do not perform a Ritual."},
	{ID: "cathedral", Label: "Build a Cathedral", Text: "The Ancients: build a Cathedral where you have a Cultist, for 3 Power, or 1 if not adjacent to another. Its glyph earns a spellbook.", Area: true},
	{ID: "an-lowest", Label: "Each enemy summons its cheapest Monster", Text: "The Ancients' requirement: each enemy Summons their lowest cost Monster at their Controlled Gate for free. It earns a spellbook."},
	{ID: "an-highest", Label: "Each enemy summons its dearest Monster", Text: "The Ancients' requirement: each enemy Summons their highest cost Monster at their Controlled Gate for free. It earns a spellbook."},
	{ID: "festival", Label: "Festival", Text: "This enemy gains 1 Power.", Seat: true},
	{ID: "unholy", Label: "Unholy Ground", Text: "Give up the Cathedral in this area: an enemy Great Old One in the battle is Eliminated.", Area: true},
	{ID: "demat", Label: "Dematerialization", Text: "Relocate your units from this area, all but the one on a Gate.", Area: true},
	{ID: "demat-to", Label: "Relocate here", Text: "Dematerialization: your units arrive in this area.", Area: true},
	{ID: "bu-pay", Label: "Pay 2 Power", Text: "Bubastis's requirement: it earns a spellbook.", Cost: 2},
	{ID: "catnap", Label: "Catnapping", Text: "Every unit in Bastet's area but her goes to the Moon.", Cost: 1},
	{ID: "savagery", Label: "Savagery", Text: "Pay 1 Power: every Cat from Saturn rolls 4 more dice this battle.", Cost: 1},
	{ID: "predator", Label: "Predator", Text: "The enemy must eliminate another unit of this type, anywhere.", Units: true},
	{ID: "bastet-ritual", Label: "Ritual of Requires Attention", Text: "Bastet shares an area with an enemy Cultist: a Ritual worth exactly 4 Doom, and Elder Signs if an enemy Gate or Great Old One is there."},
	{ID: "glacier", Label: "Place a Glacier", Text: "The Primeval Map: a Glacier covers this area with a Glacier glyph, and any Gate there is abandoned for good.", Area: true},
}

var cardActIndex = func() map[string]cardAct {
	m := map[string]cardAct{}
	for _, a := range cardActs {
		m[a.ID] = a
	}
	return m
}()

// cardActOf splits a piece value into its decision and the unit it names.
func cardActOf(piece string) (cardAct, string) {
	id, unit, _ := strings.Cut(piece, ".")
	return cardActIndex[id], unit
}

// cardActCost is the Power a "card" move costs.
func (e *eng) cardActCost(mv state.Move) int {
	a, _ := cardActOf(mv.Piece)
	if a.ID == "cathedral" && mv.To != "" {
		return e.cathedralCost(e.areaIdx(string(mv.To)))
	}
	return a.Cost
}

// Format, Parse and Steps of the action. card:id[.unit][@area][/key]

func formatCard(mv state.Move, key func(int) string) string {
	out := actCard + ":" + mv.Piece
	if mv.To != "" {
		out += "@" + string(mv.To)
	}
	if mv.Count != 0 {
		out += "/" + key(mv.Count)
	}
	return out
}

func parseCard(text string, seatOf func(string) int, area func(string) bool) (state.Move, bool) {
	rest, ok := strings.CutPrefix(text, actCard+":")
	if !ok {
		return state.Move{}, false
	}
	mv := state.Move{Action: actCard}
	if head, key, found := strings.Cut(rest, "/"); found {
		mv.Count = seatOf(key)
		if mv.Count == 0 {
			return state.Move{}, false
		}
		rest = head
	}
	if head, to, found := strings.Cut(rest, "@"); found {
		if !area(to) {
			return state.Move{}, false
		}
		mv.To = board.PositionID(to)
		rest = head
	}
	a, unit := cardActOf(rest)
	if a.ID == "" {
		return state.Move{}, false
	}
	if unit != "" {
		if _, known := unitTypeIndex[unit]; !known {
			return state.Move{}, false
		}
	}
	mv.Piece = rest
	return mv, true
}

// cardStepID is the id of the button of a "card" move.
func cardStepID(mv state.Move, key func(int) string) string {
	id := actCard + "-" + mv.Piece
	if mv.Count != 0 {
		id += "-" + key(mv.Count)
	}
	return id
}

// --- questions that wait -----------------------------------------------------------------------

// openPending asks the questions raised while something else was going on,
// first in the queue, and play resumes where it was.
func (e *eng) openPending() {
	d := e.d
	if len(d.Pending) == 0 {
		return
	}
	ps := d.Pending
	d.Pending = nil
	if d.Phase == phaseOver {
		return
	}
	d.Prompts = append(ps, d.Prompts...)
	if d.Stage != stagePrompt {
		d.Resume = d.Stage
		d.Stage = stagePrompt
		d.Then = "resume"
	}
	e.routed = true
	e.normalizePrompts()
	if len(d.Prompts) == 0 && d.Stage == stagePrompt {
		d.Stage = d.Resume
		e.runThen()
	}
}

// ask adds a question of a "card" prompt to the queue.
func cardPrompt(why string, seat, by, left int) Prompt {
	return Prompt{Kind: "card", Why: why, Seat: seat, By: by, Left: left}
}

// cardPromptMoves lists the answers to a "card" prompt.
func (e *eng) cardPromptMoves(p Prompt) []state.Move {
	d := e.d
	one := func(piece string) state.Move { return state.Move{Action: actCard, Piece: piece} }
	var out []state.Move
	switch p.Why {
	case "festival":
		for _, o := range e.others(p.Seat) {
			out = append(out, state.Move{Action: actCard, Piece: "festival", Count: o})
		}
	case "demat-to":
		for a := range e.m.ids {
			if e.m.ids[a] != board.PositionID(p.Area) {
				out = append(out, state.Move{Action: actCard, Piece: "demat-to", To: e.m.ids[a]})
			}
		}
	case "glacier":
		if p.Left <= 0 {
			return nil
		}
		for _, a := range e.glacierChoices() {
			out = append(out, state.Move{Action: actCard, Piece: "glacier", To: e.m.ids[a]})
		}
	case "horde":
		if e.pool(p.Seat, uGhast) <= 0 {
			return nil
		}
		for _, a := range e.gatesOf(p.Seat) {
			out = append(out, state.Move{Action: actCard, Piece: "horde", To: e.m.ids[a]})
		}
	case "blood":
		b := d.Bat
		if b == nil || p.Left <= 0 {
			return nil
		}
		for _, s := range []int{b.Att, b.Def} {
			if b.Pains[s] >= 2 {
				out = append(out, state.Move{Action: actCard, Piece: "blood", Count: s})
			}
		}
		if len(out) > 0 {
			out = append(out, one("blood-pass"))
		}
	case "ghost":
		if e.d.Ghost != p.Seat {
			return nil
		}
		for a := range e.m.ids {
			if e.enemiesIn(a, p.Seat) == 0 {
				out = append(out, state.Move{Action: actCard, Piece: "ghost", To: e.m.ids[a]})
			}
		}
	case "sarnath":
		for _, o := range e.others(p.Seat) {
			if e.sarnathUnits(p.Seat) != nil {
				out = append(out, state.Move{Action: actCard, Piece: "sarnath-unit", Count: o})
			}
			if len(d.Signs[p.Seat]) > 0 {
				out = append(out, state.Move{Action: actCard, Piece: "sarnath-sign", Count: o})
			}
		}
	case "sarnath-pick":
		for _, typ := range e.sarnathUnits(p.By) {
			out = append(out, one("elim."+typ))
		}
	case "innsmouth":
		for a := range e.m.ids {
			if e.count(a, p.Seat, uCultist) > 0 {
				out = append(out, state.Move{Action: actCard, Piece: "innsmouth", To: e.m.ids[a]})
			}
		}
	case "subvert":
		for _, o := range e.others(p.Seat) {
			out = append(out, state.Move{Action: actCard, Piece: "subvert", Count: o})
		}
	case "orifice":
		b := d.Bat
		if b == nil {
			return nil
		}
		for _, typ := range e.typesIn(e.areaIdx(b.Area), p.By) {
			if unitClassOf(typ) != classGOO {
				out = append(out, one("orifice."+typ))
			}
		}
	case "dust":
		out = append(out, one("dust-sign"))
		seen := map[string]bool{}
		for _, u := range d.Bat.Lost {
			if u.Seat == p.Seat && !seen[u.Type] && !isIndependent(u.Type) && e.pool(p.Seat, u.Type) > 0 {
				seen[u.Type] = true
				out = append(out, one("dust."+u.Type))
			}
		}
	case "planet":
		out = append(out, one("planet-power"), one("planet-doom"))
	case "final":
		if d.Power[p.Seat] >= e.ritualCostFor(p.Seat) {
			out = append(out, one("final-ritual"), one("final-pass"))
		}
	}
	return out
}

// cardPromptText is what a "card" prompt asks.
func (e *eng) cardImpulse(p Prompt) string {
	switch p.Why {
	case "festival":
		return "Festival — choose the enemy that gains 1 Power."
	case "demat-to":
		return "Dematerialization — click the area your units go to."
	case "glacier":
		return fmt.Sprintf("Click an area with a Glacier glyph for the next Glacier (%d left).", p.Left)
	case "horde":
		return fmt.Sprintf("Hordeling — place a Ghast at a Gate you control (%d left).", e.pool(p.Seat, uGhast))
	case "blood":
		return fmt.Sprintf("Bloodthirst — turn two Pains into a Kill on either side, or keep the results (%d left).", p.Left)
	case "ghost":
		return "Ghosts of Ib — click an area without enemies for Bokrug to return to."
	case "sarnath":
		return "Doom that Came to Sarnath — choose an enemy, and what it takes from you."
	case "sarnath-pick":
		return fmt.Sprintf("Doom that Came to Sarnath — choose a Monster or Cultist of %s to eliminate.", e.seatName(p.By))
	case "innsmouth":
		return "The Innsmouth Look — click an area to remove an Acolyte from the game for 6 Power."
	case "subvert":
		return "Subversion — choose the player whose Ritual you will rob of an Elder Sign."
	case "orifice":
		return fmt.Sprintf("Orifices — choose the surviving unit of %s that Y'Golonac replaces.", e.seatName(p.By))
	case "dust":
		return fmt.Sprintf("Dust to Dust — remove one of your lost units from the game, or give %s an Elder Sign.", e.seatName(p.By))
	case "planet":
		return fmt.Sprintf("Planetary Destruction — does %s earn 2 Power or 2 Doom?", e.seatName(p.By))
	case "final":
		return "The Final Ritual — perform a Ritual of Annihilation now?"
	}
	return "Choose."
}

// cardPromptAnswer applies an answer to the first prompt, a "card" one.
func (e *eng) cardPromptAnswer(p Prompt, mv state.Move) {
	d := e.d
	act, unit := cardActOf(mv.Piece)
	seat := p.Seat
	switch act.ID {
	case "festival":
		e.dropPrompt()
		d.Power[mv.Count]++
		e.say("Festival: %s gains 1 Power.", e.seatName(mv.Count))
	case "demat-to":
		e.dropPrompt()
		e.dematerialize(seat, e.areaIdx(p.Area), e.areaIdx(string(mv.To)))
	case "glacier":
		e.glacierOn(e.areaIdx(string(mv.To)))
		d.Prompts[0].Left--
		return
	case "horde":
		a := e.areaIdx(string(mv.To))
		if e.place(a, seat, uGhast) {
			e.say("A Ghast of %s comes to the Gate in %s.", e.seatName(seat), e.areaName(a))
		}
		return // the prompt stays until the pool is empty
	case "blood":
		b := d.Bat
		b.Pains[mv.Count] -= 2
		b.Kills[mv.Count]++
		if d.Last != nil {
			d.Last.Pains[mv.Count] -= 2
			d.Last.Kills[mv.Count]++
		}
		e.say("Bloodthirst: %s turns two Pains on %s into a Kill.", e.seatName(seat), e.seatName(mv.Count))
		d.Prompts[0].Left--
		return
	case "blood-pass":
		e.dropPrompt()
	case "ghost":
		e.dropPrompt()
		a := e.areaIdx(string(mv.To))
		d.Ghost = 0
		if e.place(a, seat, uBokrug) {
			e.say("Ghosts of Ib: Bokrug of %s returns in %s.", e.seatName(seat), e.areaName(a))
		}
	case "sarnath-unit":
		e.dropPrompt()
		e.say("The Doom that Came to Sarnath: %s chooses a unit of %s to eliminate.", e.seatName(mv.Count), e.seatName(seat))
		d.Prompts = append([]Prompt{cardPrompt("sarnath-pick", mv.Count, seat, 1)}, d.Prompts...)
	case "sarnath-sign":
		e.dropPrompt()
		if n := len(d.Signs[seat]); n > 0 {
			i := int(uint32(nextRandomFrom(e))) % n
			if v := d.Signs[seat][i]; v >= 1 && v <= 3 {
				d.Bag[v-1]++
			}
			d.Signs[seat] = append(d.Signs[seat][:i], d.Signs[seat][i+1:]...)
			e.say("The Doom that Came to Sarnath: %s eliminates an Elder Sign of %s.", e.seatName(mv.Count), e.seatName(seat))
		}
	case "elim":
		e.dropPrompt()
		for a := range e.m.ids {
			if e.count(a, p.By, unit) > 0 {
				e.say("The Doom that Came to Sarnath: %s's %s in %s is eliminated.", e.seatName(p.By), unitNameOf(unit), e.areaName(a))
				e.eliminate(a, p.By, unit, seat)
				break
			}
		}
	case "innsmouth":
		e.dropPrompt()
		a := e.areaIdx(string(mv.To))
		if e.removeForGood(a, seat, uCultist) {
			d.Power[seat] += 6
			e.say("The Innsmouth Look: %s removes an Acolyte in %s from the game and gains 6 Power.", e.seatName(seat), e.areaName(a))
		}
	case "subvert":
		e.dropPrompt()
		d.Subvert[seat] = mv.Count
		e.say("Subversion: %s has its eye on %s's Ritual.", e.seatName(seat), e.seatName(mv.Count))
	case "orifice":
		e.dropPrompt()
		e.orifice(seat, p.By, unit)
	case "dust":
		e.dropPrompt()
		if e.pool(seat, unit) > 0 {
			e.setPool(seat, unit, e.pool(seat, unit)-1)
			e.goneAdd(seat, unit)
			e.say("Dust to Dust: %s's %s is removed from the game.", e.seatName(seat), unitNameOf(unit))
		}
	case "dust-sign":
		e.dropPrompt()
		e.say("Dust to Dust: %s receives an Elder Sign.", e.seatName(p.By))
		e.gainSign(p.By)
	case "planet-doom":
		e.dropPrompt()
		e.addDoom(p.By, 2, doomCards)
		e.say("Planetary Destruction: %s earns 2 Doom.", e.seatName(p.By))
	case "planet-power":
		e.dropPrompt()
		d.Power[p.By] += 2
		e.say("Planetary Destruction: %s earns 2 Power.", e.seatName(p.By))
	case "final-ritual":
		e.dropPrompt()
		e.finalRitual(seat)
	case "final-pass":
		e.dropPrompt()
	}
}

// cardPromptDone says the first "card" prompt has run its course.
func cardPromptAuto(p Prompt) bool { return p.Why != "blood" }

// --- the choices of a turn ----------------------------------------------------------------------

// cardTurnMoves lists the "card" Actions seat may take in its turn.
func (e *eng) cardTurnMoves(seat int) []state.Move {
	d := e.d
	out := append(e.ancientsTurnMoves(seat), e.bubastisTurnMoves(seat)...)
	add := func(mv state.Move) {
		if d.Power[seat] >= e.cardActCost(mv) {
			out = append(out, mv)
		}
	}
	one := func(piece string) state.Move { return state.Move{Action: actCard, Piece: piece} }
	if a, ok := e.abilityWorks(seat, uAtlach); ok && !e.hasWeb(a) && len(d.Webs) < maxWebs {
		add(one("spin"))
	}
	if e.igooBook(seat, uAtlach) {
		add(one("cosmic-web"))
	}
	if e.holdsCard(seat, uBokrug) && !e.igooBook(seat, uBokrug) {
		for _, o := range e.others(seat) {
			add(state.Move{Action: actCard, Piece: "give-bokrug", Count: o})
		}
	}
	if _, ok := e.abilityWorks(seat, uDagon); ok {
		for a := range e.m.ids {
			if e.m.ocean[a] || !e.nextToOcean(a) || !e.anyCultistIn(a) {
				continue
			}
			add(state.Move{Action: actCard, Piece: "tsunami", To: e.m.ids[a]})
		}
	}
	if a, ok := e.abilityWorks(seat, uGhatan); ok && !e.igooBook(seat, uGhatan) {
		for _, o := range e.others(seat) {
			if e.count(a, o, uCultist) > e.mummies(a, o) {
				add(one("mummify"))
				break
			}
		}
	}
	if e.holdsCard(seat, uGhatan) && !e.igooBook(seat, uGhatan) {
		add(one("ghatan-pay"))
	}
	if e.igooBook(seat, uByatis) {
		if a, ok := e.gooArea(seat, uByatis); ok {
			for _, n := range e.m.adj[a] {
				for _, o := range e.others(seat) {
					if e.cultists(n, o) > 0 {
						add(state.Move{Action: actCard, Piece: "forget", To: e.m.ids[n]})
						break
					}
				}
			}
		}
	}
	if e.igooBook(seat, uNyogtha) && e.pool(seat, uNyogtha) > 0 {
		for a := range e.m.ids {
			if e.total(a, seat) > 0 {
				add(state.Move{Action: actCard, Piece: "nightmare", To: e.m.ids[a]})
			}
		}
	}
	if e.holdsCard(seat, uTulzscha) && !e.igooBook(seat, uTulzscha) {
		add(one("tulzscha-gift"))
	}
	if e.igooBook(seat, uHagarg) {
		for _, s := range e.playing() {
			if s != seat && d.Power[s] == 1 {
				add(one("laziness"))
				break
			}
		}
	}
	// Abandoned to Lusts: a Cat beside an enemy with a spellbook slot still empty.
	if e.holdsCard(seat, uAsteroid) {
		for a := range e.m.ids {
			if e.count(a, seat, uAsteroid) == 0 {
				continue
			}
			for _, o := range e.others(seat) {
				if e.total(a, o) > 0 && d.bookCount(o) <= 5 && !e.lustOn(o) {
					add(state.Move{Action: actCard, Piece: "lust", To: e.m.ids[a], Count: o})
				}
			}
		}
	}
	return out
}

// cardTurnAction settles a "card" Action taken in a turn. It says whether the
// action has routed the game onward itself.
func (e *eng) cardTurnAction(seat int, mv state.Move) bool {
	d := e.d
	d.Power[seat] -= e.cardActCost(mv)
	act, _ := cardActOf(mv.Piece)
	var to int
	if mv.To != "" {
		to = e.areaIdx(string(mv.To))
	}
	switch act.ID {
	case "bu-pay":
		e.say("%s pays 2 Power to the Moon.", e.seatName(seat))
		e.fulfil(seat, rqBUPay)
		e.mainTaken()
		e.thenAfterPrompts("after")
		e.beginPrompts("after")
		return true
	case "catnap":
		e.catnap(seat)
	case "cathedral":
		e.buildCathedral(seat, to)
		e.mainTaken()
		e.thenAfterPrompts("after")
		e.beginPrompts("after")
		return true
	case "an-lowest", "an-highest":
		e.giftMonsters(seat, act.ID == "an-highest")
		if act.ID == "an-highest" {
			e.fulfil(seat, rqANHighest)
		} else {
			e.fulfil(seat, rqANLowest)
		}
		e.mainTaken()
		e.thenAfterPrompts("after")
		e.beginPrompts("after")
		return true
	case "spin":
		a, _ := e.gooArea(seat, uAtlach)
		d.Webs = append(d.Webs, e.areaID(a))
		e.say("Spinnerets: %s spins a Web in %s (%d of %d).", e.seatName(seat), e.areaName(a), len(d.Webs), maxWebs)
	case "cosmic-web":
		e.say("Cosmic Web: %s completes the Spider God's bridge.", e.seatName(seat))
		e.winOutright(seat, "cosmic-web")
		return true
	case "give-bokrug":
		e.handOverBokrug(seat, mv.Count)
	case "tsunami":
		e.say("Tsunami: %s drives every Cultist out of %s.", e.seatName(seat), e.areaName(to))
		for _, s := range append([]int{seat}, e.others(seat)...) {
			if e.cultists(to, s) > 0 {
				d.Prompts = append(d.Prompts, Prompt{Kind: "sting", Why: "tsunami", Seat: s, By: seat, Area: e.areaID(to)})
			}
		}
		e.mainTaken()
		e.thenAfterPrompts("after")
		e.beginPrompts("after")
		return true
	case "mummify":
		a, _ := e.gooArea(seat, uGhatan)
		e.mummify(seat, a)
	case "ghatan-pay":
		e.say("%s pays 4 Power to Ghatanothoa.", e.seatName(seat))
		e.earnIGOOBook(seat, uGhatan)
	case "forget":
		a, _ := e.gooArea(seat, uByatis)
		e.say("God of Forgetfulness: the enemy Cultists in %s wander to %s.", e.areaName(to), e.areaName(a))
		for _, o := range e.others(seat) {
			for _, typ := range []string{uCultist, uHighPriest} {
				for e.count(to, o, typ) > 0 && e.take(to, o, typ) {
					e.add(a, o, typ)
				}
			}
			e.syncGate(to)
			e.takeAbandonedGate(a, o)
		}
	case "nightmare":
		if e.place(to, seat, uNyogtha) {
			e.say("Nightmare Web: %s awakens a Nyogtha in %s.", e.seatName(seat), e.areaName(to))
		}
	case "tulzscha-gift":
		for _, o := range e.others(seat) {
			d.Power[o] += 2
		}
		e.say("%s gives every enemy 2 Power.", e.seatName(seat))
		e.earnIGOOBook(seat, uTulzscha)
	case "laziness":
		for _, s := range e.playing() {
			if d.Power[s] == 1 {
				d.Power[s] = 0
				e.say("Laziness: %s loses its last Power.", e.seatName(s))
			}
		}
	case "lust":
		e.take(to, seat, uAsteroid)
		e.syncGate(to)
		d.Lust = append(d.Lust, LustState{Owner: seat, Victim: mv.Count})
		e.say("Abandoned to Lusts: %s's Asteroid Cat lies on an empty spellbook slot of %s.", e.seatName(seat), e.seatName(mv.Count))
	}
	e.mainTaken()
	return false
}

// --- Monsters and Terrors -------------------------------------------------------------------------

// cardArrived is told a Monster or Terror of a later pack has just come into
// play in area a, summoned or with its card.
func (e *eng) cardArrived(seat, a int, typ string) {
	switch typ {
	case uGhast:
		if e.pool(seat, uGhast) > 0 {
			e.d.Prompts = append(e.d.Prompts, cardPrompt("horde", seat, 0, 0))
		}
	case uUnMen:
		if e.is(seat, facAN) && e.d.hasBook(seat, bkFestival) {
			e.d.Prompts = append(e.d.Prompts, cardPrompt("festival", seat, 0, 0))
		}
	case uSatyr:
		if e.place(a, seat, uCultist) {
			e.say("Fecund: an Acolyte of %s comes with the Satyr in %s.", e.seatName(seat), e.areaName(a))
			e.takeAbandonedGate(a, seat)
		}
	}
}

// bloodthirst asks the owner of Leng Spiders in the battle about the results.
func (e *eng) bloodthirst(b *Battle) {
	if b.Pains[b.Att] < 2 && b.Pains[b.Def] < 2 {
		return
	}
	for _, s := range []int{b.Att, b.Def} {
		if n := e.battleUnits(b, s, uLeng); n > 0 {
			e.d.Prompts = append(e.d.Prompts, cardPrompt("blood", s, 0, n))
		}
	}
}

// captureRank is seat's rank for capturing in a: a Gug guards but does not
// capture.
func (e *eng) captureRank(a, seat int) int {
	if e.classCount(a, seat, classGOO) > 0 {
		return 2
	}
	if e.brainless(a, seat) {
		return 0
	}
	if e.classCount(a, seat, classMonster)+e.classCount(a, seat, classTerror)-e.count(a, seat, uGug) > 0 {
		return 1
	}
	return 0
}

// honeymoon returns a captive at once for 1 Power when a Cat from Venus made the
// capture. It says it did.
func (e *eng) honeymoon(seat, enemy, a int, piece string) bool {
	if e.count(a, seat, uVenus) == 0 || !isCultist(strings.TrimPrefix(piece, onGate)) && piece != "" {
		return false
	}
	d := e.d
	typ := uCultist
	if piece == uHighPriest {
		typ = uHighPriest
		d.CapHP[seat][enemy]--
	} else {
		d.Captured[seat][enemy]--
	}
	e.setPool(enemy, typ, e.pool(enemy, typ)+1)
	d.Power[seat]++
	e.say("Honeymoon: %s gains 1 Power and the Cultist goes home to %s.", e.seatName(seat), e.seatName(enemy))
	return true
}

// goneAdd notes a unit of seat that has left the game for good.
func (e *eng) goneAdd(seat int, typ string) {
	if e.d.Gone == nil {
		e.d.Gone = map[string]int{}
	}
	e.d.Gone[fmt.Sprintf("%d|%s", seat, typ)]++
}

// removeForGood takes a unit off the map and out of the game.
func (e *eng) removeForGood(a, seat int, typ string) bool {
	if !e.take(a, seat, typ) {
		return false
	}
	e.goneAdd(seat, typ)
	e.syncGate(a)
	return true
}

// lustOn says an Asteroid Cat already lies on a slot of seat.
func (e *eng) lustOn(seat int) bool {
	for _, l := range e.d.Lust {
		if l.Victim == seat {
			return true
		}
	}
	return false
}

// lustTakes hands the choice of a spellbook seat has earned to the owner of the
// Asteroid Cat lying on its slot, and brings the Cat home. It returns the owner
// (0: none).
func (e *eng) lustTakes(seat int) int {
	d := e.d
	for i, l := range d.Lust {
		if l.Victim != seat {
			continue
		}
		d.Lust = append(d.Lust[:i], d.Lust[i+1:]...)
		e.setPool(l.Owner, uAsteroid, e.pool(l.Owner, uAsteroid)+1)
		if gates := e.gatesOf(l.Owner); len(gates) > 0 && e.place(gates[0], l.Owner, uAsteroid) {
			e.say("The Asteroid Cat of %s comes home to %s.", e.seatName(l.Owner), e.areaName(gates[0]))
		}
		e.say("Abandoned to Lusts: %s chooses the spellbook %s takes.", e.seatName(l.Owner), e.seatName(seat))
		return l.Owner
	}
	return 0
}

// finalRitual performs a Ritual of Annihilation outside the Doom phase. If the
// marker reaches Instant Death the game ends after this action.
func (e *eng) finalRitual(seat int) {
	d := e.d
	cost := e.ritualCostFor(seat)
	d.Power[seat] -= cost
	g := e.doomGateCount(seat)
	e.addDoom(seat, g, doomRituals)
	signs := e.gooInPlay(seat)
	e.say("The Final Ritual: %s performs a Ritual of Annihilation (%d Power): +%d Doom and %s.", e.seatName(seat), cost, g, plural(signs, "Elder Sign", "Elder Signs"))
	for i := 0; i < signs; i++ {
		e.gainSign(seat)
	}
	e.advanceRitual()
	if d.Instant {
		d.EndNow = true
	}
}

// dustPrompts asks the enemy of Quachil Uttaus about the units it lost.
func (e *eng) dustPrompts(b *Battle) {
	for _, s := range []int{b.Att, b.Def} {
		if !b.Quachil[s] {
			continue
		}
		foe := b.foeOf(s)
		for _, u := range b.Lost {
			if u.Seat == foe {
				e.d.Prompts = append(e.d.Prompts, cardPrompt("dust", foe, s, 0))
				break
			}
		}
	}
}

// --- Independents ---------------------------------------------------------------------------------

func (e *eng) hasWeb(a int) bool {
	id := e.areaID(a)
	for _, w := range e.d.Webs {
		if w == id {
			return true
		}
	}
	return false
}

func (e *eng) nextToOcean(a int) bool {
	for _, n := range e.m.adj[a] {
		if e.m.ocean[n] {
			return true
		}
	}
	return false
}

func (e *eng) anyCultistIn(a int) bool {
	for _, s := range e.playing() {
		if e.cultists(a, s) > 0 {
			return true
		}
	}
	return false
}

// enemiesIn counts the units of seat's enemies in a.
func (e *eng) enemiesIn(a, seat int) int {
	n := 0
	for _, o := range e.others(seat) {
		n += e.total(a, o)
	}
	return n
}

// winOutright ends the game with seat the winner, whatever the spellbooks say.
func (e *eng) winOutright(seat int, reason string) {
	d := e.d
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
	d.Prompts = nil
	d.Winner, d.Reason = seat, reason
	e.say("%s wins the game (%s).", e.seatName(seat), reason)
}

// handOverBokrug gives Bokrug, his card and his spellbook to another player.
func (e *eng) handOverBokrug(from, to int) {
	d := e.d
	i := cardIndex(uBokrug)
	d.Loyal[from] &^= 1 << uint(i)
	d.Loyal[to] |= 1 << uint(i)
	if a, ok := e.gooArea(from, uBokrug); ok {
		e.take(a, from, uBokrug)
		e.setPool(from, uBokrug, 0)
		e.setPool(to, uBokrug, 1)
		e.place(a, to, uBokrug)
		e.syncGate(a)
	} else {
		e.setPool(from, uBokrug, 0)
		e.setPool(to, uBokrug, 1)
		if d.Ghost == from {
			d.Ghost = to
		}
	}
	e.say("%s hands Bokrug's card to %s, spellbook and all.", e.seatName(from), e.seatName(to))
	e.earnIGOOBook(to, uBokrug)
}

// sarnathUnits lists the Monsters and Cultists of seat on the map, nil when
// there are none.
func (e *eng) sarnathUnits(seat int) []string {
	var out []string
	for _, u := range e.units(seat) {
		if isMonsterOrCultist(u.ID) && e.onMap(seat, u.ID) > 0 {
			out = append(out, u.ID)
		}
	}
	return out
}

// mummies counts the Mummified Acolytes of seat in a.
func (e *eng) mummies(a, seat int) int {
	n := e.d.Mummies[fmt.Sprintf("%s|%d", e.areaID(a), seat)]
	if c := e.count(a, seat, uCultist); n > c {
		return c
	}
	return n
}

// mummiesOf counts all of seat's Mummified Acolytes.
func (e *eng) mummiesOf(seat int) int {
	n := 0
	for a := range e.m.ids {
		n += e.mummies(a, seat)
	}
	return n
}

// mummify lays down every enemy Acolyte in a.
func (e *eng) mummify(seat, a int) {
	d := e.d
	for _, o := range e.others(seat) {
		if c := e.count(a, o, uCultist); c > e.mummies(a, o) {
			if d.Mummies == nil {
				d.Mummies = map[string]int{}
			}
			d.Mummies[fmt.Sprintf("%s|%d", e.areaID(a), o)] = c
			e.say("Mummify: the Acolytes of %s in %s are laid low.", e.seatName(o), e.areaName(a))
		}
	}
}

// orifice replaces a surviving unit of foe with Y'Golonac, whose card it gets.
func (e *eng) orifice(owner, foe int, typ string) {
	d := e.d
	b := d.Bat
	a := e.areaIdx(b.Area)
	if !e.take(a, foe, typ) {
		return
	}
	e.setPool(foe, typ, e.pool(foe, typ)+1)
	for i, u := range b.Pained {
		if u.Seat == foe && u.Type == typ && e.painedCount(b, foe, typ) > e.count(a, foe, typ) {
			b.Pained = append(b.Pained[:i], b.Pained[i+1:]...)
			break
		}
	}
	i := cardIndex(uYgolonac)
	d.Loyal[foe] |= 1 << uint(i)
	e.setPool(foe, uYgolonac, 1)
	e.place(a, foe, uYgolonac)
	e.syncGate(a)
	e.say("Orifices: Y'Golonac bursts out of %s's %s in %s: the card is theirs now.", e.seatName(foe), unitNameOf(typ), e.areaName(a))
	e.earnIGOOBook(foe, uYgolonac)
}

// cardSweep keeps the rules that hold at every moment: it runs at the end of
// every decision.
func (e *eng) cardSweep() {
	d := e.d
	if d.Phase == phaseOver {
		return
	}
	for _, s := range e.playing() {
		if !e.holdsAnyLaterIndependent(s) {
			continue
		}
		// Interdimensional: where Daoloth stands there is a Gate.
		if e.igooBook(s, uDaoloth) {
			if a, ok := e.gooArea(s, uDaoloth); ok && !e.hasGate(a) {
				d.Gates[e.areaID(a)] = 0
				e.say("Interdimensional: a Gate opens under Daoloth in %s.", e.areaName(a))
				e.takeAbandonedGate(a, s)
			}
		}
		// Execration of Mu: Ghatanothoa mummifies by being there.
		if e.igooBook(s, uGhatan) {
			if a, ok := e.abilityWorks(s, uGhatan); ok {
				e.mummify(s, a)
			}
		}
		// Atlach-Nacha: six Webs.
		if e.holdsCard(s, uAtlach) && len(d.Webs) >= maxWebs {
			e.earnIGOOBook(s, uAtlach)
		}
		// Father Dagon: eight units in the oceans.
		if e.holdsCard(s, uDagon) && !e.igooBook(s, uDagon) {
			n := 0
			for a := range e.m.ids {
				if e.m.ocean[a] {
					n += e.total(a, s)
				}
			}
			if n >= 8 {
				e.earnIGOOBook(s, uDagon)
			}
		}
		if d.Phase == phaseAction && d.Power[s] == 0 {
			// Hagarg Ryonis: 0 Power. Hibernating does not reach Gla'aki's.
			if e.holdsCard(s, uHagarg) {
				e.earnIGOOBook(s, uHagarg)
			}
		}
	}
	// Who first reached 0 Power in this Action phase.
	if d.Phase == phaseAction && d.ZeroFirst == 0 {
		for _, s := range e.playing() {
			if d.Power[s] == 0 {
				d.ZeroFirst = s
				if e.holdsCard(s, uGlaaki) {
					e.earnIGOOBook(s, uGlaaki)
				}
				break
			}
		}
	}
}

func (e *eng) holdsAnyLaterIndependent(seat int) bool {
	for _, c := range e.ownedCards(seat) {
		if c.Kind == cardIndependent {
			return true
		}
	}
	return false
}

// doomStartCards runs the abilities of the later cards at the start of the Doom phase.
func (e *eng) doomStartCards() {
	d := e.d
	d.Mummies = nil
	for _, s := range e.playing() {
		if a, ok := e.abilityWorks(s, uByatis); ok && e.enemiesIn(a, s) == 0 {
			e.say("Toad of Berkeley: no enemy disturbs Byatis, and %s earns an Elder Sign.", e.seatName(s))
			e.gainSign(s)
		}
		if e.igooBook(s, uYgolonac) {
			e.say("The Revelations: every enemy of %s gets an Elder Sign.", e.seatName(s))
			for _, o := range e.others(s) {
				e.gainSign(o)
			}
		}
		if e.igooBook(s, uDagon) && e.onMap(s, uCultist) > 0 {
			d.Prompts = append(d.Prompts, cardPrompt("innsmouth", s, 0, 0))
		}
		if e.holdsCard(s, uGhatan) && !e.igooBook(s, uGhatan) && e.gateCount(s)+e.cultistsInPlay(s) < 6 {
			e.earnIGOOBook(s, uGhatan)
		}
		if _, ok := e.abilityWorks(s, uHagarg); ok {
			d.Prompts = append(d.Prompts, cardPrompt("subvert", s, 0, 0))
		}
	}
}

// doomEndCards queues the questions of the end of the Doom phase.
func (e *eng) doomEndCards() {
	d := e.d
	e.glaciersAdvance()
	if d.Ghost != 0 {
		d.Prompts = append(d.Prompts, cardPrompt("ghost", d.Ghost, 0, 0))
	}
	for _, s := range e.playing() {
		if e.igooBook(s, uBokrug) {
			d.Prompts = append(d.Prompts, cardPrompt("sarnath", s, 0, 0))
		}
	}
	for s := range d.Subvert {
		d.Subvert[s] = 0
	}
}

// subversion steals one of the Elder Signs the Ritual of seat has just earned.
func (e *eng) subversion(seat, earned int) {
	d := e.d
	if earned <= 0 {
		return
	}
	for _, s := range e.playing() {
		if d.Subvert[s] != seat {
			continue
		}
		n := len(d.Signs[seat])
		if n == 0 || d.Signs[seat][n-1] == 0 {
			return
		}
		v := d.Signs[seat][n-1]
		d.Signs[seat] = d.Signs[seat][:n-1]
		d.Signs[s] = append(d.Signs[s], v)
		e.say("Subversion: %s steals an Elder Sign from %s's Ritual.", e.seatName(s), e.seatName(seat))
		return
	}
}

// undyingFlame is Tulzscha's gain at the end of the Gather Power phase.
func (e *eng) undyingFlame() {
	d := e.d
	for _, s := range e.playing() {
		if _, ok := e.abilityWorks(s, uTulzscha); !ok {
			continue
		}
		var moreDoom, moreSigns, morePower bool
		for _, o := range e.others(s) {
			moreDoom = moreDoom || d.Doom[o] > d.Doom[s]
			moreSigns = moreSigns || len(d.Signs[o])+len(d.Shown[o]) > len(d.Signs[s])+len(d.Shown[s])
			morePower = morePower || d.Power[o] > d.Power[s]
		}
		if moreDoom {
			e.addDoom(s, 1, doomCards)
		}
		if morePower {
			d.Power[s]++
		}
		if moreSigns {
			e.gainSign(s)
		}
		if moreDoom || morePower || moreSigns {
			e.say("Undying Flame: Tulzscha burns for %s.", e.seatName(s))
		}
	}
}

// tombHerd is the Power Gla'aki earns from the Acolytes in the pools, before any
// captive goes home.
func (e *eng) tombHerd() [nSeats + 1]int {
	var out [nSeats + 1]int
	for _, s := range e.playing() {
		if _, ok := e.abilityWorks(s, uGlaaki); ok {
			out[s] = e.pool(s, uCultist)
		}
	}
	return out
}

// cosmicUnity is the enemy Great Old One Daoloth silences: the one with the most
// dice, as nothing would be gained by sparing it.
func (e *eng) cosmicUnity(b *Battle) {
	a := e.areaIdx(b.Area)
	for _, s := range []int{b.Att, b.Def} {
		if _, ok := e.abilityWorks(s, uDaoloth); !ok || e.battleUnits(b, s, uDaoloth) == 0 {
			continue
		}
		foe := b.foeOf(s)
		best, dice := "", -1
		for _, g := range e.typesIn(a, foe) {
			if unitClassOf(g) != classGOO || e.battleUnits(b, foe, g) == 0 {
				continue
			}
			if c := e.gooDice(foe, g, a, b, s); c > dice {
				best, dice = g, c
			}
		}
		if best != "" {
			b.Unity = best
			b.UnitySeat = foe
			e.say("Cosmic Unity: %s's %s rolls no dice.", e.seatName(foe), unitNameOf(best))
		}
	}
}

// battleFinishCards settles the requirements decided by the end of a battle.
func (e *eng) battleFinishCards(b *Battle) {
	a := e.areaIdx(b.Area)
	for _, s := range []int{b.Att, b.Def} {
		if e.holdsCard(s, uByatis) && e.count(a, s, uByatis) > 0 && b.Slain[s] > 0 {
			e.earnIGOOBook(s, uByatis)
		}
		if e.holdsCard(s, uNyogtha) && e.count(a, s, uNyogtha) > 0 && b.FoeGOO[s] {
			e.earnIGOOBook(s, uNyogtha)
		}
	}
}

// gobogegPains has every unit of the battle Pained when Gobogeg was Killed or
// Pained in it.
func (e *eng) gobogegPains(b *Battle) {
	hit := b.AllPain
	for _, u := range b.Pained {
		hit = hit || u.Type == uGobogeg
	}
	if !hit {
		return
	}
	b.AllPain = false
	a := e.areaIdx(b.Area)
	e.say("Gobogeg is struck: every unit in %s is Pained.", e.areaName(a))
	for _, s := range []int{b.Att, b.Def} {
		for _, typ := range e.typesIn(a, s) {
			for n := e.count(a, s, typ) - e.painedCount(b, s, typ); n > 0; n-- {
				b.Pained = append(b.Pained, PU{Seat: s, Type: typ})
			}
		}
	}
}

// bookOfLaw pays the 6 Power of Gobogeg's ability for a Great Old One awakened.
func (e *eng) bookOfLaw(seat int) {
	for _, s := range e.playing() {
		if _, ok := e.abilityWorks(s, uGobogeg); ok {
			e.d.Power[seat] += 6
			e.say("Book of Law: %s receives 6 Power for the awakening.", e.seatName(seat))
			return
		}
	}
}

// bookTaker is the seat that gets the spellbook of a "book" prompt: its Seat, or
// By when the owner of an Asteroid Cat is choosing for it.
func bookTaker(p Prompt) int {
	if p.By != 0 {
		return p.By
	}
	return p.Seat
}

// --- the Primeval Map: Glaciers ----------------------------------------------------------------

// hasGlacier says a Glacier covers area a.
func (e *eng) hasGlacier(a int) bool {
	id := e.areaID(a)
	for _, g := range e.d.Glaciers {
		if g == id {
			return true
		}
	}
	return false
}

// glacierGlyph says the printed map has a Glacier glyph in area a.
func (e *eng) glacierGlyph(a int) bool { return e.m.glacier[a] }

// glacierChoices lists the areas with a Glacier glyph and no Glacier yet.
func (e *eng) glacierChoices() []int {
	var out []int
	for a := range e.m.ids {
		if e.glacierGlyph(a) && !e.hasGlacier(a) {
			out = append(out, a)
		}
	}
	return out
}

// glacierOn places a Glacier in a: a Gate there is abandoned for good, and the
// unit that held it stays in the area. Yog-Sothoth, who is a Gate, is not covered.
func (e *eng) glacierOn(a int) {
	d := e.d
	if e.hasGlacier(a) || e.isMoon(a) {
		return
	}
	id := e.areaID(a)
	d.Glaciers = append(d.Glaciers, id)
	if ctl, ok := d.Gates[id]; ok {
		d.Gates[id] = 0
		delete(d.Holders, id)
		if ctl != 0 {
			e.say("A Glacier covers %s: %s loses its Gate there.", e.areaName(a), e.seatName(ctl))
			return
		}
	}
	e.say("A Glacier covers %s.", e.areaName(a))
}

// glaciersAdvance is the end of a Doom phase on the Primeval Map. After the first
// a Glacier appears in each cult's start area; after the others the first player
// chooses two more.
func (e *eng) glaciersAdvance() {
	d := e.d
	if !e.m.cfg.glaciers {
		return
	}
	if !d.GlaciersBegun {
		d.GlaciersBegun = true
		for _, s := range e.playing() {
			if a, ok := e.startAreaOf(s); ok {
				e.glacierOn(a)
			}
		}
		return
	}
	if len(e.glacierChoices()) > 0 {
		d.Prompts = append(d.Prompts, cardPrompt("glacier", d.First, 0, 2))
	}
}

// startAreaOf is where seat's cult began on this map, if it is a fixed area.
func (e *eng) startAreaOf(seat int) (int, bool) {
	id := startArea(e.game, *e.f(seat))
	if _, ok := e.m.index[board.PositionID(id)]; !ok {
		return 0, false
	}
	return e.areaIdx(id), true
}
