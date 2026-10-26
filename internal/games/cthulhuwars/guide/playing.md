# Playing the game

## Taking your turn

When it is your turn the status bar says what is being asked ("Your turn — take an action", "Choose where Deep One
retreats"…). Everything you can do is **highlighted**:

- **Buttons** in the move bar are actions (*Recruit*, *Build Gate*, *Summon*, *Awaken*, *Battle*…) or choices
  (*Done*, a spellbook, a faction). A button says what the action costs just now — *Build Gate (3)*, a unit's own
  number — and **hovering it** opens a card with the rule in a few lines (for a unit: its numbers and spellbooks).
  Free actions show nothing.
- **Areas** with a dashed outline are legal targets — click one.
- **Tokens** with a golden glow can be picked — click the unit itself.
- **Any unit** can be clicked to see its card: what it costs, how many dice it rolls, how many are left in the
  pool, and which of its cult's spellbooks enhance it (and whether they are earned). A unit that a spellbook of its
  cult enhances wears a small **golden star** in the corner of its icon on the map. Units are drawn in proportion to
  their class — Cultists smallest, then Monsters then Terrors (the Hound of Tindalos), Great Old Ones largest.

**Spellbook marks.** A small spellbook on a button means the action is an ability of a spellbook you have
unlocked; a greyed-out one means the action meets a spellbook *requirement* (paying Power, giving Doom or Power,
sacrificing Cultists, taking the Windwalker's free book). In the cult panels the same pictogram stands for each
spellbook and requirement — in colour when it is unlocked or met, greyed out when it is not.

A move is built one choice at a time. The pending choice is shown in the move bar; **Back** steps back, **Reset**
clears it ([[Esc]]).

### Examples

| I want to… | Click |
|---|---|
| Recruit a Cultist | *Recruit Cultist* → the area |
| Summon a monster | *Summon <monster>* (its button shows the cost) → your Gate |
| Awaken a Great Old One | *Awaken <name>* (its button shows the cost) → the area |
| Build a Gate | *Build Gate (3)* → the area |
| Move units | *Click a unit on the map to move* — a glowing unit token → its destination, repeat for more units, then *Done* |
| Give a Gate up | the figure on the Gate → the Gate again (free, where it stands) or another area (a Move step): the Gate is abandoned |
| Put a unit on a Gate | one of the units standing beside it → the Gate (free; also in the middle of a Move) |
| Take your last move back | *Undo my move*, among the actions |
| Battle | *Battle* → the area → the enemy faction |
| Retreat after a battle | the unit token → a highlighted area |
| End your turn | *Pass* (drops your Power to 0) |
| Begin as the Windwalker | the area with your sigil that you want, at the very start |
| Hibernate | *Hibernate* (and then wait for Gather Power) |
| Place the Ice Age token | *Ice Age* → the area |
| Desecrate | *Desecrate (2)* — the die rolls at once and cannot be taken back — then the Cultist or Monster to place |
| Recruit the High Priest | *Recruit High Priest (3)* → the area |
| Choose the spellbook a requirement earned | *Choose a spellbook to unlock:* the button (hover it for the rule) |
| Sacrifice the High Priest | *Unspeakable Oath* — at any time on your turn, or when you are asked |

## The screen

- **The bar at the top** — *Rules* (this window) and *Options*, the game options window: the reactions, the map layers,
  the display settings, the tools (flip, analyse, hint, image, export), the link to this game and, at the bottom,
  *End game*.
- **Round bar** — the round number and the four phases of a round in the order they run (*Action → Gather Power →
  First Player → Doom*; the current one is lit and those already played are ticked), the order of play and its
  direction, the **Ritual of Annihilation track** (a space for each Ritual, its cost on it, the marker on the next
  one, and the skull of Instant Death at the end) and the Elder Signs left in the bag (the icon and *left/total*).
- **The map** — areas with tokens, Gates (with the unit that occupies each — apart from the stack of its kind beside
  it), Desecration marks and the battle outline. The **Key** tab of this window explains every mark.
- **Battle panel** — the dice and results of the current or last battle.
- **Cult panels** — for each cult its Power, Doom, Elder Signs and Gates as pictograms (⚡ ☠ ★ ◎), and sections to open:
  the **faction ability**, the **spellbooks** and **requirements** (the ones unlocked or met wear a coloured spellbook, the others a greyed-out one), the **Elder Signs**
  (your own with their values — only you see them; revealed ones are public — the section opens only when there are
  some) and the cult's **units**, one icon each: in full colour while it is in the pool, **greyed out** while it
  stands on the map, is held by an enemy or sleeps — hover one for the unit's card, which says where that one is. Under
  them a **Captured** zone shows the units of other cults this cult holds, until Gather Power sends them home.
- **Moves and History** — one window with two tabs: the move list, and *History*, everything that happened since *your*
  last turn, grouped by player, with the cults in their colours and the verbs in bold; it includes every die and says
  where each Doom total comes from. A move a tactical bot played for a player (see *Auto-Play Timeout*) is marked
  *auto-play* in both. The History tab is always there; where nothing has been told yet it says so.
- **Chat** — table talk for the players and anyone watching (spectators choose a name for it). The smiley beside the
  message box opens a panel of emoji — faces, hands, game and mythos symbols, hearts — and the last ones you used.
- **Reactions** — in the game options: every cult with a reaction has them from the first move, even before the
  spellbook is earned — Great Cthulhu, Crawling Chaos, Black Goat, Windwalker and Sleeper — and every cult has the
  *Unspeakable Oath* when the High Priests are in. They are passed for you unless you ask to be asked: *When I'm
  targeted* (on by default, with a **?** that explains it), after any other player's action, or after chosen players'
  actions. Change it whenever you like, also while the others are taking their turns.
- **Auto-Play Timeout** — in the game options, for people (not for computer players): a tactical bot plays your turn
  if it is your turn to act and you have not acted within the time you set — days, hours and minutes, 24 hours when
  you switch it on; it is off by default. The wait counts from the last move in the game (not from before the server was last started), so every move — yours or the
  bot's — starts it again. The bot plays the decisions of that turn until play passes to someone else, and its moves
  are yours, marked *auto-play*.

## Creating a game

Pick *Cthulhu Wars* on the home page and choose

- **the layout** (3, 4 or 5 players, and which map) with the layout chips,
- **who sits where**: for every seat of the table a list of the six cults (no cult twice; choosing one that sits
  elsewhere swaps the two) and who plays it — you, an open seat for a friend, or a bot of a kind you choose,
- **the High Priests** and **the neutral cards** — Azathoth's spellbooks and Monsters, the Dreamlands Surface Monsters,
  Beyond Time and Space and any of the five Independent Great Old Ones — with the check boxes under the seats.

Every seat can be *You*: play more than one — or all of them — to try a cult or a position out alone. The other seats of
yours are players of their own, named after you and the cult (*Alice · Black Goat*); the page plays for whichever one
is to move.

A table left as the layout made it keeps the plain game; any other table gets a game of its own (see
[the map](tab:map) for how it is named). The Windwalker, when it is at the table, is asked first of all where it
begins.

## Starting a game with friends

Create the game with **Wait in a lobby** ticked (the default for a table of three or more). You land in the lobby:
copy the **invite link** and send it to the others (the server's *public URL* setting makes it a link they can
reach), watch them take their factions — everyone can pick or change their own seat, and **anyone at the table can
change the cult of a seat nobody holds yet** from the list in its row (a cult played from another open seat changes
places with it; the cults of players and bots stay) — add bots for the seats nobody takes, and press **Start game**
when the table is ready. Only the host (the player who created the game) starts it or removes someone. Once the game
has started the cults are fixed.

## Secrets

The only hidden information is the **value of Elder Signs** (and the dice generator). You see your own signs at any
time; others show only their count until their owner reveals them.

## Games with computers

*Play against a bot* fills every other seat; a table of people can also fill empty seats with bots from the game's
lobby (*Add bot*, or *Fill the open seats with…*) before the game begins. A bot is named after the faction it plays
and its kind — *Black Goat (MCTS)*. Every kind plays by the same few strategy tips: build income
first (a Cultist walks to a free neighbouring area and builds a Gate there), spend as little as possible at a time —
one action, one unit per Move — so Power is left to answer the others, keep Gates and Cultists guarded by a monster,
summon a protector in reply (or as the last action of a turn that would leave too little), and never abandon a Gate.
On top of that the search bots plan with a heuristic based on Doom, Gates, Power, spellbooks and unit strength and
sample the hidden Elder Sign values.

## For AI agents

The game is fully scriptable over the REST and MCP APIs; the legal moves of any position are listed with their
notation (see the **Controls** tab for the grammar). A few examples:

```
recruit@asia                  summon:deep-one@south-pacific
go:cultist@asia>europe        battle@asia/cc
leave:cultist@asia>europe     control:cultist@asia   awaken:cthulhu@south-pacific
book:absorb                   retreat:cultist>middle-east
start@arctic-ocean            hibernate               iceage@asia
recruit:high-priest@asia      oath                    sorcery@asia/gc
```

A table is created with `options` — `{"factions": ["ww", "sl", "gc", "cc", "ys"], "high-priests": true}` — which names the
cults seat by seat (`gc` Great Cthulhu, `cc` Crawling Chaos, `bg` Black Goat, `ys` Yellow Sign, `ww` Windwalker, `sl`
Sleeper) and switches the High Priests on; the game created reports its own id (`cthulhu-wars-5p~ww-sl-gc-cc-ys-hp`).
