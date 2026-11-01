# Cthulhu Wars

*Cthulhu Wars* (Petersen Games) is a game of three to five cults racing to
wake their gods: Power buys Cultists, monsters, Gates and Great Old Ones, Gates
score Doom, battles are dice, and six spellbooks per cult turn fights into
victory. bingle implements the **core set with its four factions** — Great
Cthulhu, Crawling Chaos, Black Goat and Yellow Sign — the first expansion's
**Windwalker**, **Sleeper**, **Opener of the Way** and **Tcho-Tcho** cults and the optional **High Priests**, on a
**simplified world map**, with abstracted graphics. A table seats any three to
five of the six cults (see *Setting up a table*).

Like Courtfall it is a **Go ruleset** ([ADR 0025](decisions/0025-go-rulesets-and-hidden-information.md));
the board, seats, pieces, legend and the multi-page rules guide are data
(`internal/games/cthulhuwars/definition/*.yaml`, `…/guide/*.md`). It is the
first game with **more than two seats**, an **area-map board**
([ADR 0026](decisions/0026-area-maps-and-rules-guides.md)) and a hand-written,
tabbed **rules guide**.

## Setting up a table

A layout fixes the map and the number of seats (3, 4 or 5); *which cult sits in
which seat* and *whether the High Priests are in* are two **options** of the
game (`options` in the game definition: a `seat-choice` named `factions` — the
six cults, none twice — and a `bool` named `high-priests`). The cults keep
their colours: Great Cthulhu green, Crawling Chaos blue, Black Goat red,
Yellow Sign yellow, Windwalker white, Sleeper orange.

A table that differs from the layout's own — other cults, or the High Priests —
is a **derived game** ([ADR 0028](decisions/0028-derived-games-and-setup-options.md)):
the platform asks the ruleset (`catalog.Deriver`) for a merge patch of the
layout's definition (seats, turn order, `params.factions`, `params.high_priests`,
the gate badges) and gives the game an id of its own, `<layout>~<cults>[-hp][-az][-bg.shrivel…]`
— for instance `cthulhu-wars-5p~ww-sl-gc-cc-ys-hp`. The id is all a stored
game needs to be rebuilt, so nothing else is stored, and derived ids are not
in the catalog's list (`GET /api/game-defs` shows only the layouts). A table
left as the layout made it keeps the plain id. The default cults are Great
Cthulhu, Crawling Chaos, Black Goat and Yellow Sign — plus the Windwalker at a
five-player table.

## Playing it

* **Seats.** The home page lists, for every seat, the cult (a list of the six;
  no cult twice, choosing one that sits elsewhere swaps the two) and who plays
  it. It arranges the table seat by seat (you / open for a friend / a bot;
  `seat_bots` in `POST /games`, with a *Set every bot to…* shortcut; `options`
  carries the cults and the High Priests); a bot is named after the faction it
  plays and its kind (*Black Goat (MCTS)*, *Yellow Sign (Minimax)*) — in the
  player list, the log and the lobby. The arrangement is kept while the page
  stays open: choosing another layout of the game keeps who sits where, which
  cult and each bot's kind. *Play against a bot* fills every other seat. *You*
  may be chosen for **several seats** (`also_seats`): a game against oneself,
  each such seat a player of its own named *Alice · Black Goat*; the page plays
  for whichever is to move.
* **Lobby.** A table of three or more seats waits in a lobby (unless the
  creator opts out): the page shows an **invite link** — built from
  `BINGLE_PUBLIC_URL` when the server has one, else from the address the page
  was opened at — who sits where, *Sit here* to take another faction, bots for
  the seats nobody takes, and for the host (the first human to sit) *Remove*
  and **Start game**, which needs every seat taken
  (`POST /api/games/{code}/start|seat|leave|remove`, MCP `start_game`). Without
  a lobby the game starts as soon as the last seat is taken. Players and
  spectators share a **chat** (`GET|POST /api/games/{code}/chat`, MCP
  `send_chat`/`get_chat`; the latest 500 lines are kept, 500 characters a
  line, a burst limiter).
* **A decision is one move.** A turn is a series of small decisions —
  `recruit@asia`, `summon:deep-one@south-pacific`, `go:cultist@asia>europe`
  (the first one starts a Move action; every unit moves once, then `done`),
  `battle@asia/cc`, then pre-battle abilities,
  `assign:deep-one` for every Kill and Pain, `retreat:deep-one>europe`… The
  grammar is in the rules dialog's *Controls* tab and `get_game_info`.
* **Clicking.** Actions are buttons; targets are dashed-outline areas; units
  are tokens that glow when they can be picked (a Move's origin, a unit that
  takes a result, the unit that retreats or is sacrificed, a unit that sinks
  with Cthulhu). Forced choices are taken automatically. **Any** token can be
  clicked for its card — cost, Combat, units in the pool and the spellbooks
  that enhance it, earned or not (`view.seats[n].units`); a unit one of whose
  spellbooks has been earned wears a small gold star at a corner of its icon.
  **Icons** are silhouettes per unit — every faction's Cultist is the same
  hooded figure in its colour — and their size follows the unit's class,
  Cultist < Monster < Terror (no cult has one yet) < Great Old One, so the
  largest unit in an area is always its Great Old One.
* **The round bar.** The four steps of a round in rule-book order (*Action →
  Gather Power → First Player → Doom*, `view.phases`: done / current / todo),
  the order of play in the direction chosen (`view.order`), the **Ritual of
  Annihilation track** (`view.ritual`: the cost of every space, the marker and
  whether Instant Death has been reached) and the Elder Signs left in the bag
  (`view.bag`). When a first player chooses the
  direction the two options are spelled out as who plays after whom: clockwise
  runs *down* the player list (seat order, wrapping), counter-clockwise *up* it.
* **Reactions.** A spellbook that may be used on another player's turn — Great
  Cthulhu's *Devolve*, the Crawling Chaos's *Madness*, the Black Goat's
  *Necrophagy*, the Windwalker's *Cannibalism*, the Sleeper's *Energy Nexus*
  and every cult's *Unspeakable Oath* with the High Priests — is a
  **reaction**: after another player's action (or at the
  post-battle and retreat steps of a battle its owner is not in) the game asks
  the owner, as an ordinary decision with a *Pass* move. So that nobody holds the
  table up, **a person's reactions are passed for them by default**; they choose
  when to be asked instead with the *reaction settings* of the game options
  window — **when I'm targeted** (on by default), after any other player's
  action, or after chosen players' actions — which are there from the first
  move (before the spellbook is earned), can be changed at any time — also
  while the others play — and are repeated next to the question itself
  (`GET|PUT /api/games/{code}/reactions`, MCP `get_reactions`/`set_reactions`).
  **Each reaction has boxes of its own** (the Crawling Chaos' Madness and, with the
  High Priests, the Unspeakable Oath), and **Madness starts with every box ticked**: a
  unit of the player's can be sent away in any battle, so a person is asked whoever
  fights, until they change it ([ADR 0052](decisions/0052-calls-across-pages-top-bar-and-per-reaction-choices.md)).
  Built-in bots, and AI clients that joined over MCP, are asked every time. The
  passes the session makes are recorded as `auto` moves, marked *(auto)* in the
  move list and ignored by Undo. The engine side is the optional
  `state.Reactor` interface ([ADR 0027](decisions/0027-reactions-lobbies-and-chat.md),
  [ADR 0033](decisions/0033-first-player-reactions-reversed-and-auto-play.md)).
* **Targeted.** The engine records what each *action* is aimed at (`Data.Aim`,
  `noteAim`): the seats it names (a battle, a capture, Dreams, Zingaya, a gift,
  Ancient Sorcery, Avatar) or affects (The Thousand Forms, Ghroth, Boon, Bane),
  and the areas it enters or touches (every step of a Move, a recruit, a
  summon, an awakening, a Gate, Emerge, Shriek…). A reaction window carries
  `targeted` when its owner is named, or has a unit or a Gate controlled in
  one of those areas (`present`); the windows of a battle (Necrophagy,
  Cannibalism, Madness, Energy Nexus) when the owner stands in its area.
  Only actions have an aim: Madness sending a unit somewhere, a retreat, the
  free actions (stepping off a Gate, revealing Elder Signs) are not actions and
  open no window.
* **What happened.** The position carries a short event log (`ext.log`:
  `{ply, seat, lines}` per decision, trimmed to what is newer than every seat's
  last decision). The page shows what happened since the viewer's last turn,
  grouped by player, with the cults in their colours and the verbs in bold; the
  History tab is there even when nothing was narrated (see `docs/ui.md`).
* **The Auto-Play Timeout.** A person may let a tactical bot play their turn if
  they take too long (`GET|PUT /api/games/{code}/autoplay`, the options
  window): see [ADR 0033](decisions/0033-first-player-reactions-reversed-and-auto-play.md).
* **Hidden information.** Elder Signs are drawn from a bag (18 × 1, 12 × 2,
  6 × 3 Doom) and kept face down: you see your own values at any time (they
  are in your own `view.seats[n].signs`), others show only how many, and the
  count left in the bag is public. `reveal` turns **all** of a cult's signs
  face up: their total is added to its Doom and the values become public
  (`shown`). The dice generator never leaves the server and a game cannot be
  taken back across a die roll or a draw.
* **Rules window.** Eight tabs (overview, the round, actions, battle, factions,
  spellbooks, map, playing) plus *Controls*, a table of contents, search and
  inline piece icons; see [ui.md](ui.md). The map tab's tables are written by
  `scripts/cwmaps.py --guide`.

## Rules as implemented

| Area | Implementation |
|---|---|
| Setup | Each cult places a Gate and its 6 Cultists in its start area (South Pacific, Asia, South America, Europe; the Sleeper in its sigil area) and starts with 8 Power. The Windwalker sets up by *choosing* one of the two areas marked with its sigil — the first decision of the game (`start@arctic-ocean`). |
| Round | Action phase → Gather Power → determine first player (+ direction) → Doom phase; turn order is clockwise/counter-clockwise from the first player. A player with no Power — or one who is hibernating — is skipped; the Action phase ends when nobody has any. **The first round:** *Great Cthulhu is the first player of the game, wherever it sits*; at a table without it the first cult **by name** is ("Black Goat is before Yellow Sign", the rule book's own suggestion: Black Goat, Crawling Chaos, Great Cthulhu, Sleeper, Windwalker, Yellow Sign). The first player chooses the direction of play before anyone acts (the first decision of the game, after the Windwalker's setup), and the Doom phase is skipped in that first round. |
| Actions | Recruit 1; Summon (monster's cost, at your Gate); Awaken (per Great Old One); Build Gate 3; Move (1 per unit, once each); Battle 1; Capture (unit rank beats the enemy's best in the area); Pass. Putting a unit on a Gate, revealing your Elder Signs (and Black Goat's summoning) are unlimited; a Gate is given up by moving the unit that occupies it (below). With six spellbooks, Battle is unlimited. A Cultist (or Dark Young with the Red Sign) that arrives — by Move, Recruit, retreat, Emerge, Dreams — at an abandoned Gate takes control of it by itself, unless the Gate was given up that turn. A turn ends by itself once its action is done and no free action worth waiting for (not Reveal, not swapping the unit on a Gate of one's own) is left — an abandoned Gate with a unit of its former owner beside it keeps the turn open. |
| Gather Power | 1 per Cultist (the High Priest is one), 2 per Gate, 1 per abandoned Gate to everyone, 1 per captive to its captor; a hibernating Windwalker keeps the Power it banked (every other cult has spent or dropped all of its Power); the Sleeper's sacrificed captive Monsters give 1 each; the log lists every source; raised to half the leader's Power (rounded up). |
| Battle | Pre-battle abilities (attacker's, then defender's), then both roll their Combat; 6 = Kill, 4–5 = Pain. Kills are assigned first, then Pains, each by the player they hit (Hastur's *Vengeance* chooses for the foe); then post-battle abilities (Regenerate, Necrophagy, the Harbinger, the Emissary of the Outer Gods); then Pained units retreat to an adjacent area free of the enemy — Madness chooses where, in any battle — and if there is none, one is eliminated. |
| Doom phase | 1 Doom per Gate; then each cult may perform the Ritual of Annihilation once, reveal signs, and use Doom-phase books. The Ritual pushes a marker along the **track**, whose length follows the number of players — 3: 5, 6, 7, 8, 9, 10; 4: 5, 6, 7, 7, 8, 8, 9, 10; 5: 5, 6, 6, 7, 7, 8, 8, 9, 9, 10 — each space is the cost of the next Ritual, and the step after the last is **Instant Death**: the game ends once the Doom phase that is current (or the next one, if the marker was pushed in another phase) has been scored. Cults later in that Doom phase may still perform a Ritual, at 10. The game also ends at the end of a Doom phase with someone at 30 Doom. |
| Winning | Among cults with all six spellbooks, the most Doom (Elder Signs revealed first); nobody → humanity survives; a tie is a draw. |
| Expansion cults | The Windwalker and the Sleeper are in `factions.go` like the core cults; their abilities live in `actions.go` (Hibernate, Ice Age, Lethargy, the requirement actions, Ancient Sorcery, Cursed Slumber, Capture Monster), `battle.go` (Howl, Berserkergang, Cannibalism, Eternal, Demand Sacrifice, Energy Nexus, the Formless Spawn and Tsathoggua dice) and `doom.go` (Herald of the Outer Gods, Death from Below, the end of Ancient Sorcery). |
| Opener of the Way, Tcho-Tcho | `opener.go` and `tcho.go` (ADR [0045](decisions/0045-opener-of-the-way-and-tcho-tcho.md)): the two set up after the others (`Data.Setup`/`SetupLater`, order Tcho-Tcho, Windwalker, Opener), Yog-Sothoth is a Gate (`gatesOf`, `hasGate`), The Beyond One and Dragon Ascending/Descending, Channel Power (a prompt between rolling and tallying the dice), The Million Favored Ones (post-battle `upgrade`), Dread Curse (`CurseState`, four prompt kinds), Sycophancy (a prompt to the performer of a Ritual), Hierophants, Ubbo-Sathla's Growth counter, Terror, Idolatry, Martyrdom, Tablets. |
| Azathoth (neutral content) | The `azathoth` option adds the six **neutral spellbooks** and four **neutral Monsters** of the Azathoth faction (`neutral.go`, [ADR 0044](decisions/0044-azathoth-neutral-spellbooks-and-monsters.md)): a requirement may earn a neutral book in place of an own one (`Data.Neutral`, one holder each; `bookChoices`), Loyalty Cards cost 2 Doom in the Doom phase (`Data.Loyal`, `buy:`), Shamblers wait on the card (`Data.Card`) and walk after any action (a `react` prompt). The `book-swaps` option replaces Black Goat's Ghroth or Crawling Chaos' The Thousand Forms with a chosen neutral book without the faction. |
| Loyalty Cards, Terrors, Independents | The options `dreamlands`, `beyond` and `igoo-abhoth`/`-chaugnar`/`-cthugha`/`-hydra`/`-yig` add the Moonbeast, Gnorri, Shantak, Wamp, Voonith and Hound of Tindalos cards and the five Independent Great Old Ones ([ADR 0046](decisions/0046-loyalty-cards-terrors-and-independents.md); `loyalty.go`, `packs.go`, `independents.go`). One registry (`cards`) lists them with the Azathoth faction's Monsters; a card costs 2 Doom (a Terror also 2 Power) in the Doom turn, one a Doom phase; an Independent is awakened as an Action at a controlled Gate with a Great Old One of its own, dies back into the general pool with its spellbook (`Data.IBooks`), and gives no Elder Sign in a Ritual. A Moonbeast lies on a spellbook (`Data.Moon`, `Data.hasBook` = usable), Curse of Chaugnar Faugn turns Elder Signs into `pick` prompts (`Data.Picks`), Fire Vampires hold Killed units back (`Battle.Doomed`). |
| Neutral window, capture off a Gate | `View.Neutral` (`neutral_view.go`) lists the neutral spellbooks and Loyalty Cards still to take; the client draws it as the "Neutral items" card (`neutralCard`, `neutralTip` in `game.js`) and hides it when no neutral content is in play. Capturing a Cultist asks which one when the enemy has several beside a Gate it holds (`capture:cultist@a/f` or `capture:gate:cultist@a/f`); the captured occupant leaves the Gate abandoned and the captor's own Cultist takes it ([ADR 0051](decisions/0051-cthulhu-wars-capture-choice-and-neutral-window.md)). |
| High Priests | With the option each cult has a *High Priest* (`high-priest`, a Cultist class unit, cost 3 to recruit, 1 Power in Gather Power, may hold a Gate, may be captured) and the *Unspeakable Oath*. |

### Where the rules had to be interpreted

The core rulebook describes the round and battle precisely; the faction texts,
requirements and the track follow the current pages of the Cthulhu Wars wiki
(cthulhuwars.fandom.com) with their errata. Where the printed rules leave the
players to agree, the engine stands in for them:

* **Spellbook texts and requirements** are in `factions.go` and shown as such
  in the rules window; the wiki's wording is the reference, and a faction page
  that changes there is a one-place change here.
* **Submerge** costs 1 Power and always takes Cthulhu; the player then picks,
  one `bring:<unit>@<area>` at a time (or `bring:all`, then `done`), which of
  the units standing with him sink too. `emerge` brings everything back.
* **Great Cthulhu** gains an Elder Sign for every Great Old One he awakens
  (*Immortal*), receives a spellbook — and an Elder Sign — in the first Doom
  phase, and takes his last one in a Doom phase with another. He awakens only
  in his start area, whose Gate may be anyone's.
* **The Thousand Forms** is all or nothing: the die's roll is paid by the
  enemies between them, or — if they cannot pay all of it — gained by the
  Crawling Chaos. The engine settles the split as "each point from whoever has
  the most Power left".
* **Ghroth** makes the enemies eliminate as many Cultists as the die shows,
  split the same way (each Cultist from whoever has the most); with a high die
  the Black Goat places any faction's Cultist anywhere on the map.
* **Reactions** are asked as ordinary decisions of the reacting seat (see
  *Playing it*); the rulebook settles who goes first when several players want
  one by turn order, so the windows open in turn order from the first player.
  Devolve is offered after any action of another player in the Action phase
  and, when a battle is declared against Great Cthulhu with Cultists in the
  area, before anything else in that battle happens.
* **No-retreat** eliminates one pained unit of a side that cannot retreat
  anywhere — rare on a map where every area has at least two neighbours.
* **Gates are occupied.** Every controlled Gate has an occupant, the type of
  the unit standing on it (`Data.Holders`): an Acolyte, the High Priest or — with
  The Red Sign — a Dark Young. The first of them present when the Gate is built
  or taken (in that order of preference) is the occupant. The occupant is one of
  the units of its type there, told apart from the rest: `unitMoves` offers the
  rest as `go:<unit>@a>b` (only as many as stand beside the Gate) and the
  occupant as `leave:<unit>@a>b` — to any area it can reach or to its own
  (`leave:cultist@asia>asia`: free, no Move, once per Gate in a turn).
  A `leave` abandons the Gate whoever else stands there (`vacate`; the area
  goes in `TurnState.Toggled`, so nobody takes it back *by arriving* this turn).
  There is no Abandon action. The occupant is replaced only against its owner's
  will — killed, captured, eliminated: another unit that can hold the Gate steps
  on (`syncGate`), and with none left the Gate is abandoned.
* **Putting a unit on a Gate** is the free action `control:<unit>@a` (`controlMoves`):
  a unit of the seat that stands in the area of an abandoned Gate takes control
  of it, or — on a Gate of its own — takes the place of an occupant of another
  type. It is offered in the turn and in the middle of a Move (abandoned Gates
  only there); an abandoned Gate keeps the turn open for it
  (`worthwhileFreeAction`), a swap does not. The step list of `control` is the
  same clicks as a move that goes nowhere — pick the unit, then the Gate — and the
  occupant's step list names the piece `gate:<unit>` (`onGate`), so the page
  can offer the figure on the Gate and the stack beside it as different tokens.
  The bots drop step-offs and swaps (`withoutStepOffs`) and score a `leave` at
  −50, like a unit that would leave its Gate empty.
* **One button per monster and per Great Old One.** The steps of `summon` and
  `awaken` name the unit (`summon-deep-one`, `awaken-cthulhu`), so the page shows
  a button for each with the cost it has now (`view.costs`, the cheapest legal
  way — a Ritual's included — and `view.seats[n].units[].cost`). The action
  definitions (`Describe`) carry the rule of each button as its `hint`: the text
  of the spellbook or ability that creates it, or a summary written for it
  (*Build Gate*, *Recruit High Priest*, *Capture*…); the page opens it as a card
  on hover. `UnitView` also says where a unit may come into play (`enter`) and
  where each unit that is not in the pool is (`places`).
* **The Gather Power line** lists the sources of each cult's Power — Cultists,
  Gates × 2, abandoned Gates, captives, sacrificed Monsters, Y'ha-Nthlei, Feast,
  The Red Sign — and the Power a hibernating Windwalker kept.
* **Sigil areas** (`params.sigils`, per layout). The wiki names the Windwalker
  and Sleeper glyphs but not where they are printed on the maps; the engine
  puts the Windwalker's on the Arctic Ocean and Antarctica and the Sleeper's
  in North America (North America West on the maps that split it) — the polar
  areas for Rhan-Tegoth and Ithaqua, one North American area for Tsathoggua's
  cult to begin in. Moving them is a data change.
* **The Windwalker's setup** is a decision: it sets up after everyone else, in
  one of its two sigil areas. The platform asks the seat that leads the turn
  order first, so a derived game puts the Windwalker's seat first in
  `turn_order.seats`, else the seat of the first player of the game
  (`gameCfg.first`, `firstSeatOf`): Great Cthulhu's, wherever it sits, or the
  seat of the first cult by name. (A game before this rule began with seat 1
  as first player; such a game keeps its order — `Data.Opening` is only set in
  new games.) `Start` leaves the position at the direction stage with
  `Data.Opening`; the first player's `cw`/`ccw` begins the first Action phase
  (`beginFirstAction`) and skips the Doom phase; the phase bar shows *First
  Player* as the step under way.
* **First player (Windwalker's third requirement).** Met when the Windwalker is
  chosen the first player at the end of a round. (At the start of the game it
  never is: Great Cthulhu comes first by rule and, without it, the Black Goat,
  Crawling Chaos and Sleeper all come before the Windwalker by name.)
* **Doom has a ledger.** `Data.DoomBy` says, per seat, how much of its Doom came
  from Gates, Rituals, gifts and revealed Elder Signs (`addDoom`), so that the
  log can explain a total and the Doom counter can say *3 from Gates, 3 from
  gifts*; Doom a game carried before the ledger existed shows as *from earlier*.
* **Hibernate.** Cost 0; gain 1 Power per enemy Great Old One in play, at most
  what you have; the rest of the Action phase is skipped as with 0 Power;
  the Power is kept and the Gather Power income is added to it (a first
  version doubled it: the card says to *add* your current Power to your total
  in Gather Power, which only matters because the Power would otherwise be
  gone).
* **Ice Age.** One token, moved by the Windwalker's action; any enemy Action
  that *ends* in its area costs 1 Power more (a Move is charged once however
  many units go there; Ghroth, Thousand Forms and the like have no area).
* **Arctic Wind.** Units that stood with Ithaqua follow him for free during
  the same Move, to the area he moved to.
* **Berserkergang.** For each Gnoph-Keh that took a Kill the enemy chooses one
  of its own Monsters or Cultists to eliminate — anywhere on the map: the card
  (unlike Devour's "in the Battle" or Abduct's "from the Battle") names no area.
  A Great Old One is never chosen; with nothing else left, nothing happens.
* **Cannibalism** is a reaction of the Windwalker in every battle where an enemy
  unit was killed, also those it is not in; the unit it places takes no result.
* **Eternal** asks the Windwalker's player, as a decision, whenever a result
  would reach Rhan-Tegoth while it has Power; it is answered once per result.
* **Ferox** protects the Windwalker's Cultists from Monsters while Ithaqua is on
  the map; Great Old Ones can still capture them.
* **Lethargy** is offered only while another cult has Power and is not
  hibernating — otherwise it would stall the phase for ever.
* **Ancient Sorcery.** One Serpent Man per enemy faction card; the Sleeper
  has that faction's unique ability (Flight, Immortal, Fertility Cult, Feast,
  Hibernate; a unique ability naming a Great Old One includes Tsathoggua)
  until the end of the next Doom phase, then gains 1 Power and places the
  Serpent Man anywhere. A borrowed *Immortal* gives its Elder Signs and
  makes Tsathoggua cost 4 after the first awakening.
* **Cursed Slumber.** The sleeping Gate and its Cultist (a High Priest if no
  Acolyte stands there) leave the map; they count for Power and Doom, cannot
  be attacked, captured, abandoned or targeted, and one Gate sleeps at a time.
* **Demand Sacrifice.** Asked of the Sleeper's enemy before every battle with
  the Sleeper while Tsathoggua is in play: an Elder Sign for the Sleeper, or
  all that enemy's Kills against the Sleeper's units count as Pains.
* **Energy Nexus.** A turn of its own inside the battle's start: the Sleeper
  may take one Action that *starts in the battle area* (a unit that leaves it,
  or a Recruit/Summon/Awaken/Build Gate/… that targets it), for its normal
  cost; Battle and Actions without an area are not offered.
* **Desecrate** follows the King's card: with no token in his area, roll 1 die;
  a result no higher than the number of the cult's units there (the King
  included, not the unit the action places) places a Desecration token; **whether
  it succeeds or fails** a Monster or Cultist of cost 2 or less from the pool is
  placed in the area. The action is one move (`desecrate`) that pays and rolls;
  the unit is a prompt after it (`desecrate:<unit>`, `Prompt.Kind "desecrate"`,
  asked only when more than one kind is in the pool) — so the die is never
  behind a choice, and the move that rolled it is *locked*: the session stores
  `moves.locked` (`CanTakeBack` false — or the same seat's next move straight
  after one that was, like the unit it places), the move list and the move events
  say `locked`, and the page offers no *Undo my move* across a roll (the server
  refuses it with 409 in any case). A spellbook the glyph earns is chosen after
  the unit. Yellow Sign's card counts the tokens in play (`counters[0]`,
  `desecrations`).
* **Abilities of the Great Old Ones** are data (`unitSpec.Abilities`, from the
  faction pages): Devour, The Harbinger, Avatar, Desecrate, Vengeance, Eternal,
  Ferox, Lethargy; `UnitView.abilities` carries them to every card, and the rules
  list them (Actions: a table; each cult's page beside the steps of awakening).
* **Unspeakable Oath.** An unlimited action on its owner's turn; for the
  *interruptions* — "at the end of any player's Action" — its owner is asked
  after **every** action of another player while the High Priest stands on the
  map (a playtest was never asked: it had been limited to "when it can matter"),
  and again in Gather Power; in the Doom phase it is offered when 2 Power
  would pay for a Ritual. The asking can be declined for everyone with the
  reactions settings. The bots still sacrifice only when it matters (no Power
  left, the High Priest threatened, a Ritual or the first player to gain).
  A game's last Action may be followed by a sacrifice that gives Power and
  keeps the phase going.
* **Maps.** The four layouts of the core set's boards are variants of one
  game: `cthulhu-wars` (4 players, "5-3": North America and South America in two
  parts each, New Zealand beside Australia), `cthulhu-wars-3-5` (4 players: one
  North America, Scandinavia, North/South Asia, Arabia, West/East Africa),
  `cthulhu-wars-3p` (13 areas; three cults) and
  `cthulhu-wars-5p` (the 21-area board, five cults: the four core cults and
  the Windwalker — games stored before the expansion keep their four). Their areas and borders are traced from
  reference pictures of the printed boards by `scripts/cwmaps.py`
  (segmentation, shared-border vectorisation with straight edges, adjacency
  from shared borders — it reproduces the published area degrees, e.g.
  Australia 2, Europe 3, Antarctica 3, Asia 6). Start areas per layout are
  `params.starts` in the definition.

## Engine layout

| File | What it does |
|---|---|
| `factions.go` | unit/spellbook/requirement specs of the six cults and the High Priest |
| `config.go` | the table of a game (`gameCfg`: seats, cults, High Priests, Azathoth, spellbook swaps, sigils), the setup options and `Derive`/`FromSuffix` |
| `neutral.go` | the Azathoth faction's neutral spellbooks and Monsters: what a cult may choose, Walk Between Worlds, Shriveling, Recriminations, Undimensioned, The Mao Ceremony, Vampirism, Mind Control |
| `loyalty.go`, `packs.go`, `independents.go` | the Loyalty Cards: the card registry and buying, the Dreamlands and Beyond Time and Space Monsters and the Hound of Tindalos, the five Independent Great Old Ones (awakening, death, spellbooks, abilities) — ADR [0046](decisions/0046-loyalty-cards-terrors-and-independents.md) |
| `expansion.go` | abilities that are shared (Ancient Sorcery's borrowed ability, the Ice Age surcharge, the sleeping Gate, the Unspeakable Oath) |
| `data.go` | the position payload (`Data`) and the prompt/battle/doom sub-states |
| `engine.go` | the working view of a position (unit counts, Gates, dice) |
| `actions.go` | turn legality and execution: recruit, move, summon, awaken, capture |
| `books.go` | spellbook awards, prompt machinery, faction actions |
| `battle.go` | the battle state machine: dice, assignment, post-battle abilities, retreats |
| `reactions.go` | reactions: who may answer another player's action or a battle step, the `react` prompt and the `state.Reactor` implementation |
| `doom.go` | Gather Power, first player, the Doom phase and the end of the game |
| `view.go` | public views (phases, order, bag, unit cards, own Elder Signs), determinization, planes, heuristic, text |
| `guide.go`, `guide_expansion.go`, `guide_cards.go` | the strategy tips the bots follow (`state.BotGuide`), the expansion cults' and the Loyalty Cards' |
| `notation.go` | notation, steps, the fixed action space, client labels |

## Tests

`tests/cthulhuwars` pins the ruleset: setup, every action's legality and
costs, Flight, Gates, battle (kills/pains/retreat/Devour/combat values), the
spellbooks (Absorb, Regenerate, Madness, Necrophagy, Abduct, Seek and
Destroy, Dreams, Devolve, Submerge, Ghroth, Desecrate, …), gathering Power,
the first player, the Doom phase and the end of the game.
`expansion_test.go` and `expansion_rules_test.go` do the same for the
Windwalker, the Sleeper and the High Priests on derived tables, and play
random games of every kind of table. A contract test plays
random games and checks `Legal ⊆ Apply`, notation round-trips, determinism,
unit conservation, the fixed action space, planes and secrecy; a fuzz target
exercises `Parse`. `wiki_rules_test.go` pins the corrections of the wiki rules
pass (the Ritual track, Immortal, kill requirements, Avatar, Thousand Forms,
Harbinger, Devolve windows, …). The REST, MCP and browser behaviour is in
`tests/app/cthulhuwars_{rest,ui,round4_ui,playtest_ui}_test.go` and
`tests/app/lobby_chat_reactions_test.go`.

## Bots, loops and pace

The built-in bots are generic (random, one-move look-ahead, minimax, MCTS),
so on their own they spend Power at random and give Gates up. The ruleset
therefore implements `state.BotGuide` (`guide.go`): `GuideMoves` narrows a
bot's decision to the moves worth considering, and the session, the hint
feature, the training environments' opponents and `match_bots`, and the
search bots' playouts and inner nodes use it. It encodes the
strategy tips of the game:

1. **Income first** — a Gate is worth 2 Power a round and a Doom point, a
   Cultist 1 Power; the opening is a Cultist walking to a free neighbouring
   area, then a Gate there.
2. **Spend little at a time** — one action then end the turn; one unit per
   Move unless a plan needs another (taking an abandoned Gate, guarding a
   Cultist the first move exposed).
3. **Never leave Gates or Cultists unguarded** when an enemy monster stands
   there or can walk in; a Gate's last Cultist does not leave; a lone guard
   does not leave what it guards.
4. **Answer, don't anticipate** — an unguarded Gate is not summoned for ahead
   of time while enough Power is kept to summon in reply; when a turn would
   leave too little, the protector is the last action.
5. **Never abandon a Gate.** People still can.

It also takes free captures first, attacks only when the dice favour it, loses
the cheapest unit, performs a Ritual only at a good price (never the last,
Instant Death step unless that wins) and reveals Elder Signs only to win. The
`Heuristic` charges for Cultists left exposed next to an enemy monster.

Search bots look at positions, not histories, so a deterministic bot can end
up repeating itself. Three safeguards keep tables moving:

* **No idle decisions in the rules.** There is no "begin a Move" action to
  take and cancel (the first `go` *is* the Move action), and a Gate may change
  hands only once per turn (`TurnState.Toggled`) — otherwise a bot could play
  `move`/`done` or a unit put on a Gate and a step off it for ever (a step off in place is free but once per Gate in a turn — `Toggled` — and the bots never take one).
* **Shuffle guard.** The ruleset implements `state.RepetitionGuard`
  (`RepeatKey`, `Repositions`): when a bot is about to reposition a unit or a
  Gate into an arrangement of its pieces it had in its last dozen decisions,
  that move is taken off its list as long as other moves remain
  (`session.avoidShuffles`).
* **Background play.** A request answers after at most 1.5 s of bot play;
  slower bots carry on one decision at a time in the background (each under
  the room lock, so people can still look, undo or resign) and every move
  reaches clients as a move event. A single decision is capped at 1.5 s
  (searches answer with the best move found so far).
