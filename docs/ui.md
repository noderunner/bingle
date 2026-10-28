# Web UI

Plain ES modules served from `internal/web/static` — no build step
(ADR 0013). Three pages:

* **Home** (`/`, `js/home.js`): an italic *fun fact* (`js/facts.js`, a new one
  at random on every load) over one card with three tabs — *Games*, *Puzzles*
  and *Global Chat* (see *The home page* below). The bar at the top has three
  icons: *Active games*, *Join a game* and the *profile*.
* **Profile** (`/profile`, `js/profile.js`): name, email and the email
  notification switch, and the finished games.
* **Game** (`/game/{code}`, `js/game.js`): board, players, hands, move list,
  history, undo/resign, analysis, rules, export.

## Identity

There are no accounts (ADR 0006). The browser remembers `{room code → name}`
in `localStorage` (`bingle.games`). Opening a game you have no name for shows a
small gate: join an open seat, continue as a seated player's name, or watch.

## How moves are entered

The server lists every legal move of the position
(`GET /api/games/{code}/moves/legal?steps=1`). Each move carries `steps`, the
ordered selections that build it:

| action | steps |
|---|---|
| step | `pos(from)`, `pos(to)` |
| place | `piece(type)`, `pos(square)` |
| move-stack | `pos(from)`, `count(n)`, then one `pos` per piece dropped |
| jump | `pos(from)`, then one `pos` per landing square |
| slide, leap | `pos(from)`, `pos(to)` — and `pos(aim)` when the slide shoots |
| pass, swap | `action(id)` — a single button |

`js/builder.js` (`MoveBuilder`) filters that list by the selections made so far
and reports which selections may come next. The page highlights next squares,
enables next piece buttons in the hand, and shows a pick-up count picker. Forced
choices (one piece type, one count) are taken automatically. When the
selections equal a legal move it is complete: with *preview and confirm* on
(default) the resulting position is previewed via `simulate` and a **Submit**
button appears; otherwise the move is sent immediately.

Because the UI only follows the server's list, any new game works without UI
changes. (Keyboard play is described at the end.) Hovering an empty target
square previews the piece a click would place.

## Turn tracking

* Status bar: whose turn, a *Your turn* badge, move number, connection state.
* The seat strip of the player to move is highlighted.
* The tab title gets `● Your turn — …` and the tab icon an orange dot;
  opt-in browser notifications fire when it becomes your turn while the tab is
  hidden. The lobby tab shows `(n) Bingle` for n games waiting on you and
  refreshes every 20 seconds.
* "Last move N min ago" and the lobby's *Your games* list (turn-flagged).
* Optional **per-move time limits** (set when creating the game): a countdown
  appears on the strip of the player to move, turns red in the last minute,
  and when it reaches zero the page asks the server, which ends the game on
  time and tells everyone.
* An optional **game clock** (a chess clock, for a game of two seats; off
  unless the setup asks for it): *every* seat strip carries its own clock
  (`.clock[data-clock="<seat>"]`), the running one marked `.running` and
  counting down to the turn's deadline while the others hold still. The first
  player to run out forfeits, told to the page the same way. The field in the
  setup is `#clock-field` (`#clock_seconds`, or `#clock-handicap` with
  `#clock_seconds_1` and `#clock_seconds_2` for two different times); choosing
  a clock clears the time per move and the other way round, as a game is
  played with one or the other.

## History, undo, analysis

* The move list and ⏮ ◀ ▶ ⏭ browse any past position read-only (the position
  comes from `GET …/state?at_seq=N`).
* **Replay game** steps through the moves automatically; the address bar
  carries `#move=N` while you browse so a position can be shared.
* **Offer draw** / **Resign**: a draw offer shows as a banner on the
  opponent's page with *Accept* / *Decline*; any move withdraws it.
* **Undo** takes back your last move (and a bot's reply) if nobody else has
  moved since; **Resign** concedes. Both need a second click to confirm.
* **Ask to take back** appears once a human opponent has replied: they get a
  banner with *Allow* / *Refuse*; allowing rewinds to before your move with you
  to move. Any move withdraws an open request.
* **Hint** (only in games against bots, or in analysis / review of a finished
  game) asks a bot what it would play (minimax where piece counts matter, MCTS
  elsewhere): the squares are ringed, the banner shows the move — with MCTS its
  estimated chance of winning and the next-best moves — and *Use it* fills it
  in for you.
* **Analyze** (from the live position or any past one) lets you play moves for
  both sides in a scratch variation. Each move is applied by
  `POST …/simulate`; nothing is stored. *Take back* steps the line back.
  *Import moves…* plays a pasted move list (PTN or one move per line; move
  numbers, results and comments are ignored) out from the analysis position,
  stopping at the first illegal move. *Play from here…* turns the position into a new game (against a
  bot or a friend, from either seat): the new room starts from that position.

The *TPS* button (Tak) copies the position on screen — live, historical or in
analysis — as a TPS string for other Tak tools, and *Image* downloads it as an
SVG.

## The home page

**Fun fact.** `js/facts.js` holds the snippets — game theory in game design,
the algorithms that play games (minimax, alpha-beta, Monte Carlo tree search)
and facts about the implemented games. One is drawn at random when the page
loads (`#fun-fact`). Keep them checkable: the Tak figures are *computed*, not
quoted — the number of ways a board can be filled within each side's pieces
(a stack is any sequence of colours with a flat, a wall or a capstone on top;
a generating function over the piece budgets), an upper bound on the positions
that can arise: 2.6 × 10^37 for 5×5 and 3.7 × 10^53 for 6×6.

**Tabs** (`#tab-games`, `#tab-puzzles`, `#tab-chat`; arrow keys move between
them, and the URL hash — `/#puzzles`, `/#chat` — selects one).

* *Games*: the gallery (`.game-card`). Picking a game replaces the gallery by
  its **setup** (`#game-setup`: a preview, the description, the variants as
  `#setup-variants .chip[data-id]`, then `#create-game-form`); *All games*
  goes back, and `/?game=ID` opens the setup of a game directly. For a game of
  two seats the *Opponent* list also has **Two bots** (`@bots`: nobody plays;
  `#botmatch-a|b|speed` choose the bots and the pace, and you watch the match
  as a spectator — `POST /api/bot-matches`). **Start from** (`#start-mode-field`,
  for two-seat games without chance) chooses *A move log*: upload a file
  (`#import-file`) or paste (`#import-text`) a PTN document (a `[Size]` header
  is enough to pick the Tak board) or just a list of moves. The record is
  validated move by move — an illegal move is reported by its number — and
  then opened in a review room as a spectator, with the full move list, replay
  and analysis. `[Player1]` / `[Player2]` headers name the seats and a `[TPS]`
  header sets the start position.
* *Puzzles*: see *Puzzles* below.
* *Global Chat* (`js/global-chat.js`): one room everybody shares
  (`#global-chat-log`, `#global-chat-name`, `#global-chat-input`,
  `#global-chat-send`, the emoji picker of the game chat). History comes from
  `GET /api/chat`; new messages arrive over `/ws-chat` (reconnecting), and a
  badge (`#chat-tab-badge`) counts the messages the person has not read: those
  that arrive while another tab shows, and those said while this browser was
  away — the newest message it has shown is remembered
  (`localStorage` `bingle.chat.seen`), so coming back to the page says how much
  was missed. Reading the tab clears it.

**Setup options.** A game's `bool` options are check boxes: under the seats for a
table of three or more (`#slots-field`, Cthulhu Wars' expansions) and in a field
of the game's own for a game of two (`#setup-options`, hidden when the game has
none). The **pie rule** (`#option-pie-rule`) is one of these: it starts ticked
for a game normally played with it (Hex, Gomoku) and unticked for a game that
only offers it, and it is sent as an option only when it differs from that, so
the plain game keeps the plain id and the other choice opens `<id>~pie` or
`<id>~nopie`.

**Top bar** (`js/topbar.js`, also on the profile page). Its icons are drawn by
`js/ui-icons.js` (each `svg.ui-icon[data-icon]`, so a page or a test can tell
which picture a button carries).

*Active games* (`#open-active-games`) opens `#active-games-dialog`, which has
**two tabs** (`#tab-my-games`, `#tab-all-games`; the last one open is
remembered in `bingle.gamesTab`):

* *My games* (`#my-games li`): the games of this browser that are not finished,
  in the layout of the former *Your games* card — title, opponent, *Your turn* /
  *Their turn* / *Waiting* / *In the lobby*, how many seats are still open
  (`.badge.open-seats[data-open-seats]`), *Open*, remove;
* *All games* (`#all-games li`): every unfinished game on the server
  (`GET /api/games`), whoever made it — who is at the table, how far it has
  gone, *In the lobby* and the open seats, and *Open* (or *Watch* at a full
  table), with *yours* on the browser's own. It is how a person finds a game to
  watch or a seat to take without being sent a link.

Its badge (`#active-badge`) counts the games where it is your turn, which also
shows in the tab title and on the tab icon. *Join a game* (`#open-join`, two
plugs coming together — `data-icon="plugs"`, not a door with an arrow, which
reads as leaving) opens `#join-dialog` (`#join-game-form`: `#room_code`,
`#join_player_name`, *Join*, `#watch-game`).

The person icon (`#open-profile`) opens **`#profile-dialog`**, a floating window
rather than a page: the name this browser plays under
(`#profile-window-name`, saved as you leave the field), **Here now**
(`#here-list li[data-address]`, `#here-count`) — everybody whose browser has
said it is here in the last few minutes, your own entry marked `.me`, with a
*Message* button beside each of the others that opens the direct messages on
that person — and a link on to the profile page (`#profile-page-link`) for the
email notifications and the finished games. `js/here.js` tells the server every
minute that somebody is here (every eight seconds while a window that shows the
list is open), under the name of `bingle.name`, with this browser's own secret
(`bingle.session`); names are self-chosen, which the window says.

**The top bar of a game** carries the same icons as the main page — the active games with the
count of those waiting for you (`#open-active-games`), joining a game (`#open-join`) and the
profile (`#open-profile`) — after *Rules* and *Options*, so it is easy to see that it is your turn in
another game and to switch to it. The title and icon of the tab stay the game's own.

**Profile** (`/profile`). The settings are kept in this browser, as there are
no accounts: the name (`bingle.name`, offered when creating or joining), and
`bingle.profile` = `{email, notify}`. The email and the switch are *stored
only* — nothing is sent or read by the server until the email notifications
(`docs/history/email-notifications-plan.md`) are built. *Finished games*
(`#finished-games li`) lists the games of this browser that have ended, with
*Review*.

## Games with a Go ruleset (Courtfall)

Games whose `info.engine` is `"ruleset"` draw a few extra things from the
position's derived view (`state.ext.view`, so it also works in history and
analysis) and the game's `display` legend:

* terrain **tints** (and glyphs) on hexes, condition **stones** beside units,
  the active unit's dashed ring, deployment zones during setup;
* a scoreboard per seat in the seat strip (Court, Momentum and VP with pips);
* the **prompt** — what the position is waiting for — in the status bar;
* an **exchange panel**: both sides' dice with their success numbers and
  tallies, the opponent's commit as "committed in secret", and your failed
  dice as buttons while you may reroll;
* a *History* tab narrating the last decision (next to the *Moves* tab), and a
  *Key* tab in the rules window that explains the tints and marks;
* a flat decision list instead of paired moves.

Decisions are entered position-first: click a unit or hex, then pick the verb
that applies (Activate, Strike, Shove…) and, where it asks, a number (attack
dice, VP to buy). Decisions are submitted at once — there is no preview step
for these games — and moves that depend on dice are never previewed.

## Card tables and hands (Nyet!)

A game with a `cards` section draws a **card table** (`js/cards.js`, `#card-table`): what the
players share in the middle (the trick, the piles, the last trick), the board of terms beside it
where the game has one, and the viewer's own zones below. The other players' hands are not drawn;
the list of players says how many cards each one holds.

**Your own hand** — any zone whose visibility is `owner`, so this is every card game, not Nyet!
alone:

* it arrives **in order**: the server sorts a private hand by suit, in the game's own order of
  suits, then by the value of the rank, with cards of no suit last (`cards.Table.View`), so a hand
  reads the same way however it was dealt (and an agent reading the state sees the same);
* a card shows **twice as large** while the pointer is over it or it has the focus;
* it can be **rearranged**: press a card and drag it along the hand, and the cards it would land in
  front of make room for it (`.pc-dragging`, `.pc-shift`). The order is this browser's own
  (`localStorage` `bingle.handOrder`, by room, zone and seat), not the server's, so it survives
  every redraw and reload; a card the order does not know — one just dealt or drawn — goes after
  the ones it does, which means a fresh deal comes out sorted again. Dragging a card never plays
  it: the click that ends the drag is swallowed. **Alt+←** and **Alt+→** move the card that has
  the focus, for playing without a pointer.

## Area maps and multi-seat tables (Cthulhu Wars)

`board.style: "areas"` games are drawn by `renderAreaMap`: straight-edged
polygons with a colour gradient per ocean and per continent (by tag; light and
dark themes), waves on the seas, frost on the ice and a pale shore round every
coast, the area's name (italic for seas, on two lines when the area is narrow),
a glyph badge after it, a **Gate chip** (a dark portal ringed in the controlling
cult's colour; dashed grey when abandoned) and a ✠ chip for a Desecration among
the tokens, a red outline for the battle, and one token per (cult, unit kind)
with a count badge. Names and tokens are laid out so they stay inside their
area and clear of each other, and are drawn above every outline. The map
**zooms** (wheel, pinch, +/−) and **pans** (drag); the view survives redraws.

Tokens are clickable when the move being built can use them: a token click is
translated to the `piece`/`pos` steps of the move by searching the builder's
options (a Move's origin, the unit that takes a result, retreats, is
sacrificed or sinks with Cthulhu — see ADR 0026). Unit kinds on offer also appear as buttons in the
move bar for games without hands.

**Every** token can be clicked: a legal pick is taken and the unit's **card**
(`#unit-info`, under the map) opens as well; any other unit just opens it. The
card lists the unit's cost, Combat, how many stand here, on the map and in the
pool, its ability text and the spellbooks that bear on it, ticked once earned
(from `view.seats[n].units`). A second click, ×, or Esc closes it; the
clicked token carries a dashed ring (`.token-inspected`). Units a spellbook of
their cult enhances wear a small gold star at a corner of their icon
(`.token-enhanced`, the star itself `.token-star`) — no ring is drawn round them.

**Icons** (`js/icons.js`): every unit has its own silhouette, drawn as path data
in the cult's colour — all Cultists share one hooded figure (the High Priest
is the same figure with a mitre) — and its size
follows its class, so units in one area always read at a glance:
Cultist < Monster < Terror (a class no cult uses yet) < Great Old One
(`CLASS_SCALE`). A shape without an icon falls back to its code letters.
The two expansion cults bring two colours, white (Windwalker) and orange
(Sleeper), with a fill and a stroke token each (`--piece-white-*`,
`--piece-orange-*`; every place that coloured yellow has a rule for them).

With the High Priests in the game the Action list has a button of its own for
him (`#action-recruit-high-priest`, *Recruit High Priest*, 3 Power) beside
*Recruit Cultist*; the Unspeakable Oath is an ordinary action button
(`#action-oath`) and, as a reaction, a question with the usual *Pass*.

**Faction glyphs and the Ice Age.** The area where a cult begins (`params.starts`), and the
areas marked with the Windwalker's and the Sleeper's sigils (`params.sigils`), carry that
faction's **glyph** — the sigil of the printed game, traced to SVG by
`scripts/cwglyphs.py` (`js/cw-glyphs.js`, `guide/glyphs/<key>.svg`) — on a dark disc next to
the area's name (`.area-sigil-badge[data-sigil]`), drawn only while the cult is at the table.
The Factions page of the rules shows each glyph, with placeholders for the factions not in the game. The area that holds the Windwalker's Ice
Age token gets a ❄ chip among the tokens (`.iceage-chip`).

**Glyph badges and the neutral items.** The glyph requirements of Yellow Sign (chevron, dragon,
thorns) are badges after the area names; the *Glyphs* layer of the options window is off until the
player chooses, unless Yellow Sign is at the table (`glyphLayerOn`; the choice is
`localStorage` `bingle.layer.glyphs`). When the table plays with the Azathoth faction's content or a
pack of Loyalty Cards, a **Neutral items** card (`#neutral-window`) sits under the cult cards (on
the Round page on a phone): spellbooks nobody holds as the book pictogram with their name
(`.neutral-book`), Monsters, Terrors and Independent Great Old Ones as just their icon
(`.neutral-unit`); hovering opens the rules, price and numbers. It is absent at other tables.
Capturing a Cultist where the enemy has several beside a Gate it holds asks which one: the
piece buttons offer "Acolyte" and "Acolyte on the Gate".

With three or more seats the page lays out a **round bar**, the map, the move
bar and a **battle panel** (Combat, dice and results); the **cult panels** sit
in the side column, beside the map, listed in table order from the viewer so
that clockwise is down the list. A panel's header is the name and the cult's
numbers as pictograms — ⚡ Power, ☠ Doom, ★ Elder Signs, ◎ Gates
(`.seat-stats .score.stat[data-counter]`, each an `svg.stat-icon` and a value;
`js/icons.js` `STAT_ICONS`) — and under it collapsible sections, remembered
between moves (`bingle.cards2`): **Faction Ability** (`data-card="ability"`,
first; for the Sleeper also the abilities it borrows), **Spellbooks**,
**Requirements**, **Elder Signs** (`data-card="signs"`, tokens: your own with
their values, the others' face down, revealed ones public — a cult with none
shows a heading that does not open, `.faction-static`, and no "None yet") and **Units**
(`data-card="units"`, open by default: one icon for every unit of the cult,
`.pool-unit[data-piece][data-zone]` — in full colour while it is in the pool,
`.away` and greyed out while it stands on the map, is held by an enemy, asleep or
submerged — with a hover card: name, class, cost, where this one is
(`units[].places`), where the unit may come into play (`units[].enter`; for a
Great Old One its named abilities, `units[].abilities`, and the numbered steps of
awakening it, `units[].awaken`),
Combat, its text and the spellbooks that bear on it, earned or not). Under the
icons, when the cult holds anybody, a **Captured** zone (`#captured-<seat>`,
`.pool-unit.captured[data-owner]`): the units of other cults it has taken, in
their owners' colours, until Gather Power sends them home
(`seats[n].captured`). Yellow Sign's header numbers start with a **Desecration
token count** (`[data-counter="desecrations"]`, the diamond-and-cross pictogram of the map
chip), then Power, Doom, Elder Signs and Gates. Spellbooks and requirements are marked with the
spellbook pictogram (`.spell-mark.on` — unlocked, met — and `.spell-mark.off`,
greyed out) instead of ticks.
The round bar shows the round, the phases of the round left to right (`#phase-steps
li.phase-step[data-phase,data-state]`, the current one lit, those played
ticked), the order of play and its direction (`#turn-order`), the **Ritual of Annihilation
track** (`#ritual-track`: one space per Ritual that may still be performed — its
Power cost in the space, `li.ritual-step[data-step,data-cost,data-state]` with
the state `past`, `next` or `ahead` — closed by a ☠ for Instant Death; its length
follows the number of players) and the Elder Signs left in the bag (`#elder-bag`: the Elder Sign icon and
*left/total*). The direction of play is
chosen with two buttons (`#action-cw`, `#action-ccw`) that spell out who plays
after whom. A cult's Elder Signs (`#signs-<seat>`) show their values to their
owner only; revealed ones are public.

**Moves and History** share one card (`#history-card`) with two tabs
(`#tab-moves`, `#tab-history`; the choice is remembered, History is the default
of a game that narrates): the move list, and what happened since your own last
turn (`ext.log`; the whole of your last turn, not only its last decision),
grouped by player, cults in their colours (`.log-fac`), verbs in bold
(`.log-verb`). The History tab is always there in a game that narrates: where
nothing has been told yet (the start of the game, `#history-empty`) or where the
others have said nothing since your last move (`#history-quiet`; or, when nothing
at all was narrated since, the latest events under *Nothing new since your last
move*) the pane says so instead of the tab vanishing, as it used to when only
silent decisions (Done, a reaction passed for a player) followed. Moves a
tactical bot played for a person (see *Auto-Play Timeout*) are marked
*(auto-play)* in the move list and with a badge on the player's turn in the
History (`.badge.auto-play`).

**Doom.** The Doom counter of a cult says what it is made of when hovered (*3
from Gates, 3 from gifts*); the log lines of the Doom phase, a Ritual, a gift
and a revelation of Elder Signs give the track before and after (*Great
Cthulhu scores 3 Doom for 3 Gates: 3 → 6 on the Doom track (3 from Gates, 3 from
gifts)*).

**Action buttons.** Every monster that can be summoned and every Great Old One
that can be awakened has a button of its own in the list of actions
(`.unit-button#action-summon-<unit>` / `#action-awaken-<unit>`, with
`data-cost`): its icon, *Summon <name>* / *Awaken <name>* and the Power it costs
just now (`view.costs`, worked out by the server: Gnoph-Keh's price, Thousand
Young, the Ice Age surcharge). Hovering it opens the unit's card. Every other
button states what it costs in its text when it costs anything — *Build Gate
(3)*, *Recruit High Priest (3)*, *Ice Age (1)* — and opens a card on hover with
the rule in a few lines (`.action-tip`, the action's `hint`; for a spellbook the
timing and text). A button that exists because of an unlocked spellbook wears the
coloured spellbook pictogram, one that meets a spellbook requirement the greyed
one (`button[data-source="book"|"requirement"]`, from the action's `source`).
Move events and the move list carry `locked` for a move that rolled a die: the
page offers no *Undo my move* across it. The line above the buttons says what is being asked
(`#move-hint`): *Click a unit on the map to move, or choose an action:* on a
normal turn, *Choose a spellbook to unlock:* when a requirement earned one.
*Undo my move* is one of the actions too (`#undo-move`, offered only while there
is a move of yours to take back; it asks twice).

**Gates and their occupants.** A Gate is a chip among the tokens; the figure
drawn on it is the unit that occupies it (`.gate-holder[data-holder]`, from
`view.holders`) and is *not* counted in the stack of its kind beside the Gate
(6 Acolytes: one on the Gate, a stack of 5). An abandoned Gate is a dashed grey
ring with a plain star. The figure is a unit of its own: the chip is clickable
(`.gate-chip.gate-pickable`, piece value `gate:<unit>`) and picking it lights up
the areas it can reach and its own area; choosing one moves it and abandons the
Gate (`leave:<unit>@<area>>…`). To put a unit on a Gate (free), pick one of the
units standing beside it and click the Gate (`.gate-chip.gate-target`;
`control:<unit>@<area>`), also in the middle of a Move. Clicking the Gate itself
after picking its figure steps the figure off where it stands: free, no Move, no
Done and no *Undo my move* offered (`leave:<unit>@<area>><area>`). There is no
Abandon button.
The glyphs and sigils are badges of their own: a faction glyph is the cult's own
ring-shaped drawing on a dark disc before the name, so it is not taken for a Gate. The water and the ice carry no
texture.

On big screens the page grows with the window: the game page may be 1900 px
wide (2600 for an area map), the board's height limit follows the window
height instead of a fixed 760 px, the side column widens, and from 1500 px the
move list and the log move under the map. There is no *Move* button on an area
map: the glowing tokens are the units that can move, and clicking one starts
the Move action. The home page's *Seats* control lists the game's seats — each is *You*, a
friend's open seat or a bot of a kind (*You* may be chosen for as many seats as
you like: a game against oneself, `also_seats`), and the arrangement survives
choosing another layout of the game. A game with a *seat-choice* option (Cthulhu Wars)
shows a list of the cults instead of the fixed cult name in every row
(`select.faction-select[data-cult-seat]`; a cult chosen elsewhere is swapped,
the colour swatch follows), and its *bool* options as check boxes under the
seats (`#option-high-priests`, `#option-azathoth`, `#option-dreamlands`, `#option-beyond`, `#option-igoo-abhoth` … `#option-igoo-yig`), and a *book-swap* option as one list per
cult that has a book it may give up while that cult sits at the table
(`#book-swaps select.swap-select[data-swap]`; a neutral spellbook chosen in one
list is taken from the others). The choice is sent as `options` only when the
table differs from the layout's own. An open seat of
a waiting table can be filled with a bot (in the options window); draw offers and
take-back requests (two-party concepts) are hidden and *Resign* becomes *End game*.

### The game options window

The bar at the top of the page has **Rules** (`#show-rules`) and **Options**
(`#open-options`). *Options* opens `#game-options`, a floating, non-modal panel
(it stays open while the game goes on and closes with ×, Esc or the button), built
by `js/options.js` and filled by `game.js`; it lives outside the area that is
redrawn on every move, and a section is rebuilt only when its signature changes.
Its sections:

* **Reactions** (`#options-reactions`) — when the game asks you about your
  reactions, described below;
* **Auto-Play Timeout** (`#options-autoplay`) — a person's option, described
  below;
* **Map layers** (area maps) — *Area names*, *Glyphs*, *Sigils*
  (`#opt-layer-names|glyphs|sigils`, remembered as `bingle.layer.*`);
* **Display** — coordinates, legal-move and last-move highlights, animation,
  the board theme, preview-and-confirm (games without dice), turn notifications;
* **Tools** — *Flip board*, *Analyze*, *Hint*, *Image*, *Export*, *TPS*/*PTN*;
* **Share this game** — the link (`#share-link`, `#copy-link`);
* **Table** — fill an open seat with a bot, or take one as a spectator;
* **Game** — *Ask to take back*, *Offer draw*, *Resign*/*End game* (`#resign`).

There is no Players card or Tools card in the side column any more: the cult
panels (or the seat strips of a two-player game) show who sits where.

### Playing several seats

A game created with `also_seats` (the home page's *Myself* opponent, or *You*
for several seats of a table) seats the creator more than once: the other seats
are players of their own, named *Alice · Black Goat*. The page knows them by
that name (`S.mine`) and acts for the one to move, so a mock game alone needs
no second browser; every reaction is put to the player there (it is not passed
for them, and the page offers no reaction settings), and *Undo* takes back
the last move together with the later moves of one's other seats.

### Email notifications

`js/notify-ui.js` holds the browser's side: the contact this browser made (`bingle.contact` = `{id, secret, address}`; the
server never sends an address back), the seat secrets (`bingle.tokens`, by room code, saved by `api.createGame` and
`api.join` from `seat_token`) and the per-game setting. The **profile page** has the address, the switch, the status
(`#notify-status`, polled while the address waits for its confirmation), *Send the confirmation again*
(`#notify-resend`), *Forget my address* (`#notify-forget`) and the note naming the sender (`#notify-whitelist`,
`.notify-from`); the game's **options window** has an *Email notifications* section (`#options-notify`: `#notify-on`,
`#notify-mode-always`, `#notify-mode-away` with `#notify-away`, `#notify-hint`, the same note). Both say where the
mail comes from so the person can whitelist it. The links in the mails open `/notify/confirm?t=` and `/notify/stop?t=`
(`js/notify.js`), pages whose button POSTs the token: following a link alone changes nothing. See [email.md](email.md).

### Voice chat

`js/voice.js` is the **Voice card** (`#voice-card`, above the chat card, and on the chat page
of the phone layout): join, mute, deafen, push to talk, per-player volume and device pickers
for seated players when somebody else could be in the call. The call lives in module state
(the page redraws on every move; the card is rebuilt from it) and has a **WebSocket of its own**
(`/ws/{code}?only=voice`), so it does not depend on the page; `game.js` still hands it the
`voice` events of the page's socket (who is in the game's call, for the hint before joining)
and asks it to resync when that socket reconnects. Speaking players' dots light up
(`.speaking`) from an `AnalyserNode` on each stream. See [voice.md](voice.md).

**The call outlives the page.** While a call is up the site moves between its pages (`/`,
`/game/{code}`, `/profile`) **without reloading the document** (`js/nav.js`): a click on a link
fetches the page, swaps the content of the body, and runs the new page's script as a fresh module;
the call's module, socket and `<audio>` elements (`data-persist`) stay. What the old page made
(window/document listeners, timers, sockets, media-query listeners) is undone when it is left, and
the modules that keep a page's state (`chat.js`, `render.js`, `tooltip.js`) reset on each move
(`onNavigate`). With no call up, links load pages as they always did. A **button for the call**
(`#voice-chip`, a microphone with a green dot, orange when muted) sits in the top bar of every
page while the call is up, with the room code when it is not the game on screen; it opens a
panel (`#voice-pop`) with *Go to the game*, *Mute*, *Deafen* and *Leave*. The voice card of another
game says where the call is (`#voice-elsewhere`) and offers to *talk in this game instead*.

### Lobbies, chat and reactions

A game made with *wait in a lobby* (or any table of more than two seats that
has not started) shows `js/lobby.js` instead of the board: the **invite link**
(`#invite-link`, *Copy link* `#copy-invite`; built from `BINGLE_PUBLIC_URL`
when the server has one, else from the address the page was opened at), one row
per seat (`#lobby-seats li[data-seat]`: who sits there, *Sit here* to move,
*Add bot* with the kind to add, the host's *Remove*), a form for a visitor with
no seat (`#lobby-join`) and, for the host — the first human to sit — *Fill the
open seats with* bots (`#fill-bots`) and **Start game** (`#start-game`, enabled
once every seat holds a player or a bot). Bots are named after the faction they
play and their kind (*Black Goat (MCTS)*), and the lobby shows *MCTS bot*.

In a game whose seats are cults (Cthulhu Wars) anyone at the table can change
the **cult of an open seat** from the list in its row (`select.lobby-cult-select`,
`#cult-<seat>`): the cults held by a player or bot are greyed out, a cult played
from another open seat changes places with it (the Sleeper chosen for seat 4
while seat 3 plays it moves seat 3 to seat 4's cult). The game takes the id of
the new table, the page loads its names and colours again, and a visitor's
*Take this seat* list shows the new cults.

`js/chat.js` is the **chat card** (`#chat-card`: `#chat-log` lines
`.chat-line[data-id,data-seat]`, `#chat-input`, `#chat-send`, and `#chat-name`
for a spectator, who speaks under a name of their own). An **emoji** button
(`#emoji-button`) opens a panel (`#emoji-panel`, groups of `.emoji-cell
[data-emoji]`: faces, hands, game, mythos, hearts, and the last twelve used
first, remembered as `bingle.recentEmoji`) whose emoji go in at the cursor; the
panel stays open for several picks and closes with Esc, the button or a click
elsewhere. Seated players and
spectators share it, in the lobby and in the game; new lines arrive over the
game's WebSocket as `chat` events, the draft and the focus survive the page's
redraws, and the badge `#chat-unread` counts lines that came in while the tab
was hidden.

A player whose faction has a **reaction** (Great Cthulhu's Devolve, the
Crawling Chaos's Madness, the Black Goat's Necrophagy, the Windwalker's
Cannibalism, the Sleeper's Energy Nexus, and every cult's Unspeakable Oath when
the High Priests are in) gets the **reaction settings** in the game options
window (`#options-reactions`), from the first move (*not earned yet* until the
spellbook is, so the choice can be made early) and for as long as the game is
played. A reaction asks its owner for an answer, which holds the table up, so
the game **passes it for the player unless they ask to be asked** — the other
way round from earlier versions. The occasions to be asked on:

Every reaction has a block of its own (`.reaction-ability[data-ability=<id>]`: the
Crawling Chaos has *Madness* and, with the High Priests, the *Unspeakable Oath*), with these
boxes, whose ids carry the ability id:

* **When I'm targeted** (`#ask-<id>-targeted`, on by default) with a **?** button
  (`#ask-<id>-help`) that opens a modal (`#ask-targeted-dialog`, closed with
  *Got it*, `#ask-targeted-dialog-close`) explaining the option: an opponent's
  action aimed at the player, at one of its units or at an area where it has a
  unit or a Gate asks; what happens inside a battle (Madness moving a unit, a
  retreat…) is not an action and never does;
* **After any other player's action** (`#ask-<id>-all`);
* **After chosen players' actions** (`#reactions-<id>`, one checkbox `#ask-<id>-seat-<seat>`
  per other player);

and a sentence saying what the choice means (`#ask-<id>-state`). **Madness starts with every
box ticked** (a unit of the player's may be sent away in any battle, so a person is asked
whoever fights); the others start with *when I'm targeted*. It can be changed at
any time, above all while the other players are acting — the window may stay
open. Built-in bots are asked every time and have no settings; an AI client
that joins over MCP starts with *after any other player's action*.

The page is rebuilt on every move, so the settings live *outside* the rebuilt
area (the options window is a child of `body`, not part of `#game-root`): they
are built once, updated in place, show a choice before the server has answered,
send their requests one after another and never let an answer to an older
question put an older choice back — a click that straddles a move is not lost.

When the game asks, the move bar offers the reaction's moves and *Pass (do not use
X)*, and beside it *Stop being asked:* with one button for each reason that made
it ask — *when I'm targeted* (`#ask-stop-targeted`), *after <player>'s actions*
(`#ask-stop-trigger`), *after any action* (`#ask-stop-all`) — which make the same
choice on the spot and pass the question when nothing else asks for it. The move
list marks the passes the game made for a player with *(auto)*.

### The Auto-Play Timeout

A person's option in the game options window (`#options-autoplay`; not shown to
bots, spectators or in a lobby): a switch **Let a tactical bot play my turn if I
take too long** (`#autoplay-on`), a timeout in **days, hours and minutes**
(`#autoplay-days|hours|minutes`, *Set* `#autoplay-save` or Enter saves them; a
minute at least, 365 days at most), a **?** (`#autoplay-help`, modal
`#autoplay-help-dialog`) and a sentence (`#autoplay-state`) saying whether it is
on and, while it is the player's turn, when the bot would play. It is **off by
default** and starts at **24 hours**. When the player's turn to act (their own
turn, or a reaction waiting for their answer) has waited for the timeout since
the last move in the game, a tactical bot takes the decisions of that turn for
them until play passes to someone else (`auto_play` on the moves, *(auto-play)* in
the move list, a badge in the History). Every move — the person's, the bot's —
starts the count again.

## The rules window

The **Rules** button is a **rule book** (`js/ui-icons.js`,
`svg[data-icon="rules"]`, named *Rules* for a screen reader) wherever the word
used to stand: the top bar of a game (`#show-rules`) and the setup of a game on
the home page (`#game-rules-button`). It opens a floating dialog with one tab
per page of the game's guide (see ADR 0026), plus a client-side *Controls* tab:

* tabs with arrow/Home/End keyboard navigation (roving tab stop), the last
  tab per game remembered;
* Markdown pages rendered by `js/markdown.js` without `innerHTML`: headings,
  emphasis, lists, tables with alignment, callouts, collapsible sections,
  columns, keycaps, code, images and inline piece icons;
* a *On this page* table of contents beside long pages and a search box that
  looks through every page, listing hits as *Tab › Heading* with a snippet;
* games without their own guide get one *Rules* page built from the definition;
  the abstract games ship one *Rules* tab with the full rules and diagrams
  (ADR 0053).

## The footer and the help button

Every page ends with a small footer (`.site-footer`): `bingle v0.4.0 · MIT License · GitHub` — the version of the
engine itself (`internal/version`, apart from the versions of the games) and a link to the project.
The top bar of the main page, the profile and every game ends with a **?** button (`#open-help`, `js/topbar.js`)
that opens `#help-dialog`: the version first (`.help-version`, read from `<meta name="bingle-version">`),
then *Bingle on GitHub* (`#help-github`) and *Report Issues* (`#help-issues`, the project's issues page).

## The list of games

The *Games* tab shows a card for each game family (`.game-card[data-family]`): the picture
of the start position, or of a sample game in play when the game starts with an empty board
(`preview` in the game's JSON; Cthulhu Wars, Hive and Nyet! have pictures of their own, `js/game-icons.js`),
the name, a **Beta** badge (`.badge.beta`, while the game's `version` is below 1.0.0; its title
is the version) and the players it takes (`.game-card-players`: the range of the seat counts of
the game and its variants). While the pointer is over a card, or it has the focus, `#game-tip`
shows the name, designers, publisher, description and version. The board size of a game is
chosen in the setup (`#setup-variants`), whose button for the rules is `#game-rules-button`
("Rules").

## Puzzles

The home page's *Puzzles* tab lists positions where the side to move has a forced
win (win at once, or in two or three moves), grouped by game. *Solve* starts a game
from the position against the tactical bot with you on the winning side; a
win means solved. They are generated and proven by `scripts/genpuzzles`
(`internal/puzzles`): bot games are mined for the positions before the
decisive moves and an exhaustive solver confirms that every defence loses.

## Display settings

Stored per browser: show coordinates, highlight legal squares, highlight the
last move, move animations (pieces slide, drop in or flip; honours the OS
"reduce motion" setting), board theme (wood, slate, green felt, paper),
preview-and-confirm, turn notifications. Hovering a square shows what is on it (useful for stacks).

### Illustrated board themes

Games played with stones on a board of cells (Blooms, Hex, Havannah, Yavalath, Yinsh,
Go, Gomoku, Pente, Reversi, Connect Four, Tak, Ataxx, Amazons, draughts and the like: every
piece is a disc, ring, chip, cross, cap, wall or flat stone, and the board is no map or free
table) offer six more themes in the *Board theme* list of the options window, under
*Illustrated themes*: **Hot Synthwave Vibes**, **Celtic Rune Fantasy**, **Southwestern
Solstice**, **Moroccan Bazaar**, **Coral Abyss** and **Precious Gemstones**. Everything is drawn in code
(`js/themes.js`): gradients, patterns and filters in one hidden `<svg id="theme-defs">`, a
generated stylesheet (`<style id="theme-style">`, rules for `:root[data-board="…"]`), a
decorated frame inserted behind the cells (`g.theme-decor`: a dusk sky with a neon border,
interlaced knotwork with carved runes, a sunburst over mesas with a Navajo border, a lattice
with arches, a sea with light rays, kelp and bubbles, velvet with a gold rail set with gems) and a small mark on every round stone
(`.theme-mark`: retro slits, a rune, a sun or stepped diamond, an eight-point star, a shine, a cut gem).
With a theme on, the coordinates (`text.coord.coord-out`) stand in a strip outside the frame, in
the page's text colour, not on the picture (the `viewBox` of the board grows by the strip).
In every theme the first player's stones are **warm** (reds, golds, oranges, cream) and the
second player's **cold** (blues, violets, near-black), whatever the game calls the colours;
`themes.css` holds the ambient motion (neon, rays, kelp, bubbles), which runs only with *Animate
moves* on and the OS not asking for less motion. A theme the game does not offer (another
game's choice, remembered in `bingle.board`) falls back to wood. Choosing one redraws the board.

## Rendering

`js/render.js` draws the SVG board from the game description
(`GET /api/game-defs/{id}`): grid boards as checkered `squares` or Go-style
`points`, explicit boards as node/edge graphs; coordinates for algebraic
boards; stacks as piles with a height badge; last-move, selection and
legal-target overlays. Piece appearance comes from `shape` and the seat color.

## DOM contract used by tests

`#board svg`, `.cell[data-position]` (`.clickable` when a legal next click),
`.piece[data-position,data-piece,data-color,data-level,data-top]`,
`button.hand-piece[data-piece]`, `#count-picker button[data-count]`,
`#submit-move`, `#reset-move`, `#undo-step`, `#undo-move`, `#resign`,
`.seat-strip .clock[data-clock]` (a seat number with a game clock, `turn` for a
per-move allowance), `#clock-field` with `#clock_seconds`, `#clock-handicap`,
`#clock_seconds_1|2`,
`#move-list button.mv[data-seq]`, `#hist-*`, `#analyze`, `#exit-analysis`,
`#rematch`, `#status-message`, `#conn-badge`, `#hint`, `#open-options`,
`#game-options`, `#share-link`,
`#save-image`, `#request-takeback`, `#takeback-accept`, `.ghost-piece`,
`.hint-ring`, `#puzzle-list .puzzle-card` (the name for a puzzle game is `#puzzle_player_name`),
`#import-text`, `#fun-fact`, `#game-setup`, `#global-chat-log`,
`#active-games-dialog` with `#tab-my-games` / `#tab-all-games`, `#my-games li`,
`#all-games li` and `.badge.open-seats`, `#profile-dialog` with
`#profile-window-name`, `#here-list li[data-address]` and `#profile-page-link`,
and `svg.ui-icon[data-icon]` for the icons of the bar;
area maps add `.cell.area[data-position]` with `.area-bg`, `.mark-gate`,
`.token[data-piece,data-color,data-count]` (`.token-target` when it can be
picked), `#round-bar`, `#battle-panel`, `.seat-grid`, `.faction-card`,
`.summon-button[data-piece,data-cost]`, `#unit-info`, `.token-enhanced`, `.sign-chip`,
`.pool-unit`, `.gate-chip .gate-holder[data-holder]`, `#history-card` with
`#tab-moves` / `#tab-history`,
`#phase-steps`, `#turn-order`, `#elder-bag`, `#ritual-track .ritual-step`,
`.token-star`, `#events .log-fac`, `#options-reactions` with, per reaction (`<id>`:
`devolve`, `madness`, `unspeakable-oath`…), `#ask-<id>-targeted` (and its ?,
`#ask-<id>-help`), `#ask-<id>-all`, `#reactions-<id>` (the list),
`#ask-<id>-seat-<seat>` and `#ask-<id>-state`, `#options-autoplay` with `#autoplay-on`,
`#autoplay-days|hours|minutes`, `#autoplay-save`, `#autoplay-help` and
`#autoplay-state`, `#chat-card` with `#chat-log`,
`#chat-input`, `#chat-send`, `#chat-name`, `#emoji-button`, `#emoji-panel`, the card table
`#card-table` (`.ct-seat.me .zone-hand`, `.pc-slot`, `.pcard.face[data-card]`, `.pc-zoom`, and
`.zone.reorderable` with `.pc-dragging` / `.pc-shift` while a card is dragged), the voice card
`#voice-card` (`#voice-join`, `#voice-leave`, `#voice-mute`, `#voice-deafen`, `#voice-ptt`,
`#voice-peers li[data-voice-seat][data-state]`, `#voice-badge`, `#voice-error`, `#voice-elsewhere` and
`#voice-switch` when the call is in another game), the top-bar button of a call (`#voice-chip` with
`#voice-chip-code`, and its panel `#voice-pop` with `#voice-pop-go|mute|deafen|leave`), and the lobby: `#lobby`,
`#invite-link`, `#copy-invite`, `#lobby-seats li[data-seat]`, `#lobby-join`,
`#lobby-name`, `#lobby-seat`, `#lobby-take`, `#cult-<seat>`, `#fill-bots`, `#start-game`,
`#leave-seat`, `button[data-act=add-bot|move|remove]`; the rules window is `#rules-dialog` with
`[role=tab][data-tab]`, `#rules-panel-<id>`, `.rules-toc`, `#rules-search` and
`#rules-results .rules-hit`; ruleset games add `.cell-bg.tint[data-tint]`, `[data-badge]` stones and rings,
`.action-buttons button[data-action]`, `#exchange[data-resolved]` with
`.die` / `button.die.rerollable[data-die]`, `#events li` and
`.seat-strip [data-counter]` / `.court-tag[data-court]`.

## Keyboard

The board is one tab stop; the arrow keys move between squares (as drawn, so a
flipped board works), Enter/Space selects the square, Backspace steps back
inside a move, Esc clears it and Enter submits a previewed move. With focus
outside the board, ←/→ browse the history and Home/End jump to the start / the
live position. Alt+←/→ moves the card of your own hand that has the focus. Every square has an accessible name ("c3: stack of 2 …").
