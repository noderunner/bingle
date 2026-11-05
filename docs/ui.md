# Web UI

Plain ES modules served from `internal/web/static` — no build step
(ADR 0013). Three pages:

* **Home** (`/`, `js/home.js`): an italic *fun fact* (`js/facts.js`, a new one
  at random on every load) over one card with three tabs — *Games*, *Puzzles*
  and *Global Chat* (see *The home page* below). The bar at the top has three
  icons: *Active games*, *Join Game* and the *profile*.
* **Profile** (`/profile`, `js/profile.js`): name, email and the email
  notification switch, and the finished games.
* **Game** (`/game/{code}`, `js/game.js`): board, players, hands, move list,
  history, undo/resign, analysis, rules, export.

## Identity

A player is a guest (a browser, by its guest key) or a signed-in account ([ADR 0067](decisions/0067-principals-and-guest-keys.md),
[ADR 0068](decisions/0068-accounts-sessions-and-passwords.md)); a seat is its owner's. The browser remembers
`{room code → name}` in `localStorage` (`bingle.games`), merged with the server's "My games". Opening a game you have no name for shows a
small gate: join an open seat, continue as a seated player's name, or watch.

## The fitted game page

Every game, once out of the lobby, is drawn by `renderFitted` (a phone has its own pages for the area maps and the card
games, in `renderMobile` and `renderMobileCards`: see [the phone page of a map](#the-phone-page-of-a-map)) ([ADR 0092](decisions/0092-a-game-page-that-fits-the-screen.md)). `body.fit-screen` makes
the page exactly as tall as the window, with no page scroll and no footer. The `.board-column` is as wide as the board
can be drawn at that height (`--board-aspect`, from the board's `viewBox`; the map's canvas; a fixed shape for a board
that grows, such as Hive's), leaving `#play-rail` at least `--rail-least`.
The rail has what is left, up to 40rem, and any width beyond that is shared by the outer margins. It holds the cards in
one column and scrolls by itself, keeping its place across redraws: the tutorial and any notices, `#impulse-card`,
`#players-card` (`#cult-stack` at a table of three or more), voice, chat and history. On an upright screen under 980px
wide, the rail goes under the board, whose row is no taller than the board.

- **A table of cards** (`.boardless`) has no shape of its own. The rail is at most 26rem and the table takes the rest of
  the width, with cards sized to the window. The column scrolls if a pile of cards runs long.
- **A map** (`.areas`) sits at the top of its column under the round bar, with a wider rail (`--rail-least`). What is
  under it shares the height left: the battle, the card of a unit, an exchange. The rail's cards take turns behind
  `.rail-tabs`: `#rail-tab-cults`, `#rail-tab-talk` and `#rail-tab-history`. Every pane stays on the page, so a message being typed survives a change of tab.
  The talk and the history fill the height the rail has left.
- **A cult's card** is a summary: its numbers and an `.expand-toggle`. Open (`.seat-more`), Cthulhu Wars has a line between its sections and its words in the cult's colour (`--seat-ink`); another game's
  have a `.faction-heading`. `.spellbook-grid` holds `.spell-tile`s in two columns: the six requirements, then the spellbooks
  to come. A spellbook earned takes the place of a met requirement, in listed order, since the server keeps which are
  met and which are held but not which paid for which. `S.expanded` is the one card open, a cult's or `#neutral-window`.
  A phone shows each cult whole, on a page of its own.

No `.status-bar` is drawn on this page. While a game is being played, `#status-message` stays as `.sr-only` for screen
readers. For a result, history, analysis or a game waiting for an opponent, the message and the bar's buttons (`#rematch`,
`#return-live`, `#analyze-here`, `#exit-analysis`, `#play-from-here`) are in `#impulse-card`, whose heading says which it
is. The game's badges are in `.table-badges`, with `#conn-badge` shown only when the connection is not live.

`#impulse-card` holds the move bar and what the player chooses from: their hand and the move cards (`#cards-panel`).
Its height is limited to the rail's, and it scrolls inside if it needs more. On the player's turn it has `.acting`, which pins it to the edges of the rail, so it
is in view however far the rail is scrolled. It neither moves the rail nor takes the keyboard.

Once a piece is chosen, `#pending-move` shows the game's `piece_action` word and the piece's picture. *Back* (`#undo-step`)
is the only button that undoes a selection here: `#reset-move` is not drawn, and Esc still clears the move. A move in Blooms
is a stone and a cell, so the two did the same thing; a game with longer moves will need to look at this when it joins.

## The phone page of a map

Turning the phone changes what it shows (`renderMobile`). Upright, it has the cards, swiped between: the actions (with the
status, any tutorial and notices), the round, each cult, the history and the chat. On its side, `body.m-landscape`
gives the whole screen to the map, with no bars, for tapping areas, moving units, pinching and dragging. On the player's
turn `#m-turn-note` says so, and asks them to hold the phone upright when the next choice is not on the map. A selection
under way carries across the turn of the phone.

`isLandscape` asks the device (`screen.orientation`), not the window: an on-screen keyboard makes an upright window
wider than it is tall. With a mouse, on the phone layout asked for in the options, the window's shape decides. A touch
screen no taller than 500px counts as a phone, since a large phone on its side is wider than the 900px that otherwise
marks one. A player with the phone's rotation locked sees only the cards.

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
square previews the piece a click would place, in the colour it will have.

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
* An optional **game clock** (a chess clock, for a game of two seats or more, with `#clock-handicap` only at two; off
  unless the setup asks for it): *every* seat strip carries its own clock
  (`.clock[data-clock="<seat>"]`), the running one marked `.running` and
  counting down to the turn's deadline while the others hold still. The first
  player to run out forfeits, told to the page the same way. The field in the
  setup is `#clock-field`: `#clock_seconds` chooses no clock, minutes or a long
  time (2 hours, 1 day), and with minutes `#clock_minutes` takes any whole number
  from 1 to 60, starting at the game's `default_clock_minutes` (10 if it names
  none) until the player types their own. `#clock-handicap` gives each player the
  same pair (`#clock_seconds_1` with `#clock_minutes_1`, and `_2`); choosing
  a clock clears the time per move and the other way round, as a game is
  played with one or the other. With a clock chosen, `#clock-increment-row`
  offers `#clock_increment`, the time added for each move, which the seat's
  clock mentions in its tooltip.

* **Rated games.** The setup offers `#rated-field` with `#rated` to a signed-in
  player at a table of two. Choosing it picks *A friend*, shuts the other opponents,
  and sets a clock if there is none; a handicap, a time per move or taking the clock
  away gives the choice up. The game page has `#rated-badge`, each seat a
  `.badge.rating[data-rating]` with the change once the game is over, and
  `#rating-note` when a result did not count. Auto-Play is not offered. The home page's *Ratings* tab (`#panel-ratings`, `js/ratings.js`) has
  `#leaderboard-game` and `#leaderboard tr[data-handle]`, and the profile has
  `#ratings-box`.

* **Tournaments.** The home page's *Tournaments* tab (`#panel-tournaments`,
  `js/tournaments.js`) lists them in `#tournament-list`, and offers `#tournament-new`
  to whoever holds `create_tournament`. `/tournament/{id}` (`#tournament-page`) has
  `#tournament-status`, `#tournament-actions` (`#t-open`, `#t-enter`, `#t-leave`,
  `#t-start`, `#t-abandon`, `#t-edit`: only what the server would allow),
  `#tournament-entries` before it starts, `#tournament-standings` after, and
  `#tournament-schedule`. It reads the tournament again every 30 seconds while it is
  open or running. A tournament game has `#tournament-badge`, a link back, and no
  Auto-Play. No rule of a tournament lives in the page. A real-time tournament reads
  itself every 5 seconds and adds `#t-check-in` and `#tournament-check-in` during its
  check-in, `#tournament-your-game` while the player has a game going on and
  `#tournament-break` between rounds. Its game has `#first-move` until the player's first
  move, and `#next-game` once their next game has started.

## History, Back, analysis

* The move list and ⏮ ◀ ▶ ⏭ browse any past position read-only (the position
  comes from `GET …/state?at_seq=N`).
* **Replay game** steps through the moves automatically; the address bar
  carries `#move=N` while you browse so a position can be shared.
* **Offer draw** / **Resign**: a draw offer shows as a banner on the
  opponent's page with *Accept* / *Decline*; any move withdraws it.
* **A move stands once the turn has passed**, in every game ([ADR 0083](decisions/0083-a-move-stands.md)).
  A move that cannot come back waits for *Submit* (`#submit-move`, Enter), unless
  the player turns off `#opt-confirm`. **Back** (`#undo-step`, Backspace) clears a
  selection, then takes back a step of this turn already sent, one at a time, and is
  not offered once the turn has passed or a die has been rolled. A button such as
  *End turn* plays at once: pressing it is the Submit. A move with nothing left to choose, such as a forced pass, always gets `#submit-move`, as nothing else could play it. The steps of a turn stay out
  of the move list and the History tab until the turn is over.
* **Resign** concedes, and needs a second click to confirm.
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

**Tabs** (`#tab-games`, `#tab-puzzles`, `#tab-analysis`, `#tab-chat`; arrow keys
move between them, and the URL hash — `/#puzzles`, `/#analysis`, `/#chat` —
selects one).

* *Games*: the gallery (`.game-card`). Picking a game replaces the gallery by
  its **setup** (`#game-setup`, with tabs: `#setup-tab-new` *Create Game*, `#setup-tab-tutorials`, `#setup-tab-practice`, `#setup-tab-active`;
  a preview, the description, the variants as the list `#setup-variant` in `#setup-variants`, then `#create-game-form`, whose name is the profile's); *All games*
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
* *Analysis* (`js/analysis.js`, built the first time the tab is shown): where a
  batch of bot-versus-bot games is set going for **any** game and read. The form
  (`#analysis-game`, one `#analysis-seats .analysis-bot` per seat of the chosen
  game, `#analysis-games`, `#analysis-rotate`, `#analysis-start`), the batches
  (`#analysis-batches li[data-batch]`, with *Report* and a link to the full one
  at `/batches/{id}`) and the report of the one picked (`#analysis-report`:
  `#analysis-seat-card` — the win rate of each seat with a 95% Wilson interval
  and a marker at an even share — `#analysis-endings` and, when two bots played,
  `#analysis-bot-card`). Under it, **the guide to the bots** (`#analysis-guide`):
  `GET /api/bot-guide`, Markdown rendered by `js/markdown.js` with its drawings
  from `/api/bot-guide/assets/`, and a table of contents of its sections.
* *Global Chat* (`js/global-chat.js`): one room everybody shares
  (`#global-chat-log`, `#global-chat-name`, `#global-chat-input`,
  `#global-chat-send`, the emoji picker of the game chat). History comes from
  `GET /api/chat`; new messages arrive over `/ws-chat` (reconnecting), and a
  badge (`#chat-tab-badge`) counts the messages the person has not read: those
  that arrive while another tab shows, and those said while this browser was
  away — the newest message it has shown is remembered
  (`localStorage` `bingle.chat.seen`), so coming back to the page says how much
  was missed. Reading the tab clears it.

**A picture behind a map.** A game whose `params.art` names a file draws it behind its areas (`image.map-art`, `svg.has-art`): the areas are then transparent, there only to be clicked and lit up. The same element is used on every redraw, so a move does not make it flash. `params.ice_glyphs` draws `.ice-glyphs` over it, with the layer `#opt-layer-ice` in the game options (`bingle.layer.ice`). `params.glyph_marks` puts an area's Desecrate glyph where the board prints it (`.area-glyph-badge.printed`, no disc); elsewhere it stands in a round badge beside the name. The glyphs' shapes are in `cw-map-glyphs.js`, and for `board.svg` in `internal/session/map_glyphs.go`, both generated.

**Setup options.** A game's `bool` options are check boxes: under the seats for a
table of three or more (`#slots-field`) and in a field
of the game's own for a game of two (`#setup-options`, hidden when the game has
none). The **pie rule** (`#option-pie-rule`) is one of these: it starts ticked
for a game normally played with it (Hex, Gomoku) and unticked for a game that
only offers it, and it is sent as an option only when it differs from that, so
the plain game keeps the plain id and the other choice opens `<id>~pie` or
`<id>~nopie`.

**Top bar** (`js/topbar.js`, also on the profile page). Its icons are drawn by
`js/ui-icons.js` (each `svg.ui-icon[data-icon]`, so a page or a test can tell
which picture a button carries).

*Active games* (`#open-active-games`) opens `#active-games-dialog`: the list of this browser's games that are not finished
(`#my-games li`), with title, opponent, *Your turn* / *Their turn* / *Waiting* / *In the
lobby*, how many seats are still open (`.badge.open-seats[data-open-seats]`), *Open*, and
✕. The ✕ resigns from a game in play after asking, and says so if that ends it for the whole
table; a game that has not started only leaves the list, after asking
([ADR 0101](decisions/0101-the-x-resigns-and-games-are-found-where-they-are-wanted.md)).

The games of a game are in its setup (`#setup-tab-active`: `#setup-active-list li`, each with
*Watch*), and *Join Game* (`#open-join`) lists the tables waiting for players
(`#join-open-games li`, each with `button[data-join]`).

Its badge (`#active-badge`) counts the games where it is your turn, which also
shows in the tab title and on the tab icon. *Join Game* (`#open-join`, a plus, `data-icon="plus"`) opens `#join-dialog` (`#join-game-form`: `#room_code`, which takes
a code or a link, and `#join-submit`; then *Public Lobbies*). It takes an open seat, or a place to watch at a full table, under the
name in the profile ([ADR 0102](decisions/0102-joining-and-reading-back.md)). An unread count is `.unread-count` on *Open*.

The speech-bubble icon (`#open-friends`, its badge `#dm-badge` counting what waits to be read or answered) opens
**`#friends-dialog`**, for signed-in accounts ([ADR 0105](decisions/0105-friends.md)). On the left: adding by handle
(`#friend-add`), *Friend requests* (`#friend-requests`, `[data-accept]`, `[data-decline]`), *Game invitations*
(`#friend-invites`, *Accept* is `[data-join-invite]`, *Decline* or for news *Dismiss* is `[data-dismiss]`), the friends with a dot for who is online (`#friends-list`,
`.friend-dot.online`) and *Waiting for an answer* (`#friend-outgoing`, `[data-cancel]`). On the right, for the friend
chosen (`#dm-title`): *Call*, *Invite to game* (`#friend-invite`, then `#friend-invite-game` and `#friend-invite-send`),
*Remove friend* (`#friend-remove`) and the conversation. A friend who is offline cannot be written to or called
(`#dm-offline`). The profile page has the switches for mail about requests (`#friend-mail`) for being invited to games (`#friend-invites`, on by default) and for being seen online (`#friend-presence`).

In the setup, a signed-in player can keep a seat for a friend: in the seat's list under *Reserve for a friend* (`.slot-select`, value `friend:<handle>`), or
for a table of two under *A friend* (`#reserve-friend`). *Invite only* (`#invite-only`) keeps a game off every list. In the lobby a kept seat has `[data-reserved]`, the host
can give it up (`[data-act="release"]`), and a visitor with no seat to take sees `#lobby-all-reserved`. ([ADR 0107](decisions/0107-who-can-find-a-game.md))

In every chat a player's handle is a badge (`.handle-badge[data-handle]`). To a signed-in player it offers a friend request (`#friend-pop`,
`#friend-pop-send`, or `#friend-pop-accept` if they have asked); it does nothing for a friend, for yourself, or for a guest.

The person icon (`#open-profile`) opens **`#profile-dialog`**, a floating window
rather than a page: *My Name* (`#profile-window-name`, saved as you leave the field; a guest who has not chosen one
has a two-word name from `js/names.js`), for a guest *Move to Another Device* (`#guest-box`), for a signed-in account **Here Now**
(`#profile-here`, `#here-list li[data-address]`, `#here-count`) — everybody whose browser has
said it is here in the last few minutes, your own entry marked `.me`, with a
button beside each of the others (`[data-friend-action]`): *Add friend*, *Requested*, *Answer*, or for a friend
*Message*, which opens the friends list on them — and *All Settings* (`#profile-page-link`), the page of the
email notifications and the finished games. `js/here.js` tells the server every
minute that somebody is here (every eight seconds while a window that shows the
list is open). Only a signed-in account is listed, under its handle; a guest is told that guests are not
listed ([ADR 0073](decisions/0073-account-only-features.md)).

**The top bar of a game** carries the same icons as the main page — the active games with the
count of those waiting for you (`#open-active-games`), joining a game (`#open-join`) and the
profile (`#open-profile`) — after *Rules* and *Options*, so it is easy to see that it is your turn in
another game and to switch to it. The title and icon of the tab stay the game's own.

**Profile** (`/profile`). The settings are kept in this browser: the name (`bingle.name`, offered when creating or joining), and
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

## Themed sets of cards

The create form has a **Cards** field (`#card-set-field`, `#card-set`) for a game whose deck is
standard (`cards.standard` in the game's info: at most eight suits, and nothing but plain cards of a
suit and a rank, and jokers). `#card-set-preview` shows a set's back, its eight suits (those the game
does not reach dimmed, `.ts-unused`) and its four jokers.

The set belongs to the table ([ADR 0085](decisions/0085-card-sets-belong-to-the-table.md)): it is
stored with the game, sent to everyone as `card_set`, kept by a rematch and by *Play from here*, and
there is no way to change it at the table. `js/card-sets.js` holds the sets and draws them;
`js/card-lettering.js` letters the ranks. A themed card has the class `themed` and `data-set`. Its
tooltip gives the set's name for the suit and the one the rules use: *9 of Tree Frogs (hearts)*. The
suit pictures on Nyet!'s board of terms follow the set.

## Games of patience (Solitaire)

A view with `solitaire` is drawn by `js/solitaire.js` (`.sol-table`, still `#card-table`), not `cards.js`'s
table. **Klondike, Spider and FreeCell** have `.sol-top` (`#sol-stock`, `.sol-waste`, the free cells and
foundations) over `.sol-columns` of `.sol-column`; a pile has `data-pile` (`t3`, `w`, `f1`, `c2`) and a pile that
takes the cards in hand now has the class `sol-target`.

A move is the server's two steps, `from-<pile>-<count>` and `to-<pile>`: a card a move can start with is a
`button[data-from]`. Clicking one picks it up (`sol-picked`), or plays it at once if it has one place to go. Clicking
it again sends it to its foundation if it can go, and otherwise lets go. It can also be dragged: `.sol-ghost` follows
the pointer, the cards on top of it come along, and a drop on a pile that does not take them plays nothing. The width
of the table decides the size of the cards, so ten columns fit a phone.

Where colours are the rule (`view.alternate`), a card at a table with a themed set also shows its classic suit,
`.sol-colour`, since the set's suits are not red and black.

**Bowling Solitaire** has `#sol-sheet` (ten `.sol-frame[data-frame]`, each with `.sol-mark`s and `.sol-frame-total`),
`#sol-rack` (`.sol-pin[data-pin]`, a button when it can be part of a play, `.down` when it is down), `#sol-balls` and
`#sol-gone`. Pins are chosen in any order and held in the page, and a ball (`button[data-throw]`) is offered only when
it can take exactly those. Throwing it sends the steps in the server's order: the pins by letter, then
`ball-<pile>`. In a tutorial only the step's pins can be chosen (`sol-wanted`).

A face-up card anywhere on the table shows twice as large under the pointer or with the focus (`.pc-zoom`, as in a
hand), and not while one is being dragged. In a game's setup, `#setup-preview` draws `.sol-preview`, a table with nothing
to click, from the game info's `sample_view`, and it wears the set chosen under *Cards*.

A game with one seat has no Submit, its Resign button says **Give up**, and the setup form asks for no opponent.

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
**zooms** (wheel, pinch) and **pans** (drag); the view survives redraws.

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
clicked token carries a dashed ring (`.token-inspected`). This card is for a phone,
which has no hover: a click on a token elsewhere only picks it up.

An area's name and its glyphs are a faint layer under the pieces. The name sits at
the area's visual centre (`areaCentre`: the centre of the largest circle that fits,
with the land counting as the edge of a sea), and the glyphs go above, below or
beside it, wherever they stay inside the border (`marksPlace`). The options turn
each layer off: names, glyphs, sigils and units. A unit the player can pick up now
stays drawn with the units off, so a move can still be made.

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
in the side column, beside the map, listed in the order of play, the first player on top, and they re-order when it
changes. A panel's header is the swatch, the faction's glyph (`img.seat-glyph`, which stands for its name), the player's name and the cult's
numbers as pictograms — ⚡ Power, ☠ Doom, ★ Elder Signs, ◎ Gates
(`.seat-stats .score.stat[data-counter]`, each an `svg.stat-icon` and a value;
`js/icons.js` `STAT_ICONS`) — and under it collapsible sections, remembered
between moves (`bingle.cards2`): **Faction Ability** (`data-card="ability"`,
first; for the Sleeper also the abilities it borrows), **Spellbooks**,
**Requirements** (a neutral spellbook a cult holds, or one swapped in at setup, is one more
of its spellbooks; what is left to take is under Neutral items), **Elder Signs** (`data-card="signs"`, tokens: your own with
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
ticked), the **Ritual of Annihilation
track** (`#ritual-track`: one space per Ritual that may still be performed — its
Power cost in the space, `li.ritual-step[data-step,data-cost,data-state]` with
the state `past`, `next` or `ahead` — closed by a ☠ for Instant Death; its length
follows the number of players) and the Elder Signs left in the bag (`#elder-bag`: the Elder Sign icon and
*left/total*). The direction of play is
chosen with two buttons (`#action-cw`, `#action-ccw`) that spell out who plays
after whom. A cult's Elder Signs (`#signs-<seat>`) show their values to their
owner only; revealed ones are public.

**The history** is one card (`#history-card`) with one set of buttons to step through it (`#hist-prev`, `#hist-next`…), written
out in one of two ways by the toggle in its head (`#history-friendly`, `#history-notation`; the choice is remembered, Friendly is the default
of a game that narrates, and a game that does not has no toggle): what happened since your own last
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
page offers no *Back* across it. Stepping through the moves, the card is titled `History: n` and tells the move as the History tab does (`#history-told`).
The impulse card says what is being asked, whoever is looking: its title (`#impulse-title`) is the name of the seat on
impulse and `#impulse-text` the question, from the view's `impulse` (ADR 0099). A choice the player has made and not sent
is a step of their own, with the action's `impulse` as its question: *Choose an area to place a gate.*
A Move gathers its units in the card first (`#move-queue`): clicking a token lifts it off the map into the card, from one area, as far as the Power
goes, with `.queue-unit[data-piece]` to take one out and `#move-queue-cost` for the running cost. A click on a highlighted
area sends one step per unit ([ADR 0100](decisions/0100-a-move-gathers-its-units.md)).
*Back* is one of the actions too (`#undo-step`, offered only while there is a step
of this turn to take back).

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
Done and no *Back* offered (`leave:<unit>@<area>><area>`). There is no
Abandon button.
The glyphs and sigils are badges of their own: a faction glyph is the cult's own
ring-shaped drawing on a dark disc before the name, so it is not taken for a Gate. The water and the ice carry no
texture.

On big screens the page grows with the window: the game page may be 1900 px
wide (2600 for an area map), the board's height limit follows the window
height instead of a fixed 760 px, and the side column widens. There is no *Move* button on an area
map: the glowing tokens are the units that can move, and clicking one starts
the Move action. The home page's *Seats* control lists the game's seats — each is *You*, a
friend's open seat or a bot of a kind (*You* may be chosen for as many seats as
you like: a game against oneself, `also_seats`), and the arrangement survives
choosing another layout of the game. A game whose description says `setup_catalog` (Cthulhu Wars) draws
**a picker** above the seats (`cw-setup.js`, `#cw-setup`, [ADR 0094](decisions/0094-the-cthulhu-wars-setup-picker.md)) instead of
check boxes and the list of layouts, which stays in the page hidden:
`.cw-tab[data-tab]` over `#cw-panel` of `.cw-tile[data-item][data-kind]` (`.on` when chosen), and `#cw-chosen` in `#cw-current`, the same tiles, where a click takes one out (the map's opens the Maps tab: a table always has a map).
A faction takes the first open seat, and taking one out leaves its seat open, so the other seats keep theirs; a row of the seats shows it in
`.slot-cult[data-cult-seat]`, `data-value` empty for an open seat, which the Create button refuses. `#cw-seat-notice` says so. A faction past the last seat is spare: the table then sends `faction-pool`, `#lobby` is ticked and disabled, and taking out a seated faction seats a spare one;
`#cw-notice` says when the neutral units and Independents pass one fewer than the seats. `#cw-shuffle` fills open
seats and deals up to that number, keeping what was chosen by hand and replacing what it dealt before. `#cw-profile` lists ready-made and saved setups;
`#cw-profile-save` (accounts only) opens `#cw-save`, and `#cw-profile-delete` shows for one's own. A table with a sand-timer spellbook has a card for each among *Other Options* (`#book-swaps select.swap-select[data-swap]`,
`.swap-row[data-swap-card]`; a neutral spellbook chosen in one list is taken from the others, and a slot's `recommended` choice says so).
The choice is sent as `options` only when the
table differs from the layout's own. An open seat of
a waiting table can be filled with a bot (in the options window); draw offers and
two-party concepts are hidden and *Resign* becomes *End game*.

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
  the board style of a hex board (`#opt-hex-style`, below), the board theme,
  preview-and-confirm (games without dice), turn notifications;
* **Tools** — *Flip board*, *Analyze*, *Hint*, *Image*, *Export*, *TPS*/*PTN*;
* **Share this game** — the link (`#share-link`, `#copy-link`);
* **Table** — fill an open seat with a bot, or take one as a spectator;
* **Game** — *Offer draw*, *Resign*/*End game* (`#resign`).

There is no Players card or Tools card in the side column any more: the cult
panels (or the seat strips of a two-player game) show who sits where.

### The board style of a hex board

A hex board can be drawn as **Hexagons** or as a **Hex grid**, where each space is a point of a
triangular lattice and the stones sit on the points, as on a Go board. The choice is the browser's,
per game family (`bingle.hexStyle.<family>`), and changes only the picture: the positions, their
names and the moves are the same. Until a player chooses, the definition's `board.style` decides
(`points` is the grid, so Meridians starts there). The board keeps its orientation in either style.
Hive offers no choice, because it has no board to draw. See
[ADR 0078](decisions/0078-hex-board-styles.md).

### Playing several seats

A game created with `also_seats` (the home page's *Myself* opponent, or *You*
for several seats of a table) seats the creator more than once: the other seats
are players of their own, named *Alice · Black Goat*. The page knows them by
that name (`S.mine`) and acts for the one to move, so a mock game alone needs
no second browser; every reaction is put to the player there (it is not passed
for them, and the page offers no reaction settings).

### Accounts

`js/account.js` drives the four account pages ([ADR 0068](decisions/0068-accounts-sessions-and-passwords.md)), one
template (`account.html.tmpl`) chosen by `data-page`: **sign in** (`/login`: `#login-handle`, `#login-password`,
`#login-go`; a `?return=` path is where it goes after, and an account given its password goes to `/account` instead),
**sign up** (`/signup`: `#signup-handle`, `#signup-email`, `#signup-password`), **confirm** (`/account/confirm?t=`:
`#confirm-go`, because following the link alone must not confirm) and **the account** (`/account`: `#account-who`,
`#account-must-change`, the password form `#password-old`/`#password-new`/`#password-go`, `#logout-go`). Each reports in
`#account-result`. Sign-in has a second step (`#second-form`, `#second-code`: six digits are a code, anything else a
recovery code) and a link to **forgot** (`/forgot`: `#forgot-who`, `#forgot-go`) and **reset** (`/account/reset?t=`:
`#reset-password`, `#reset-code`, `#reset-go`). The account page's two-step box (`#tf-box`, ADR 0069) shows the state
(`#tf-state`, `#tf-needed` for an administrator who must set it up), asks for the password (`#tf-password`), sets it up
(`#tf-setup`, then the QR code `#tf-qr` (ADR 0074), `#tf-secret`, `#tf-uri`, `#tf-code`, `#tf-enable`), lists the recovery codes once (`#tf-codes`), and
turns it off or renews the codes (`#tf-off-code`, `#tf-off`, `#tf-renew`). Providers (ADR 0070) add a button each to sign in and sign up (`#oidc-buttons`, `#signup-oidc-buttons`: `#oidc-<id>`),
the handle a newcomer chooses (`#oidc-signup-form`, `#oidc-handle`, `#oidc-go`), and the account page's linked sign-ins
(`#oidc-box`: `#oidc-link-<id>`, `#oidc-unlink-<id>`, which takes the password from `#tf-password`). An account without a
password sets one (`#password-title` reads "Set a password", with no old one to give). `#login-error` and `#oidc-error`
show the `?error=` a provider's callback came back with. The account page lists the account's API tokens (`#tokens-list`, `#token-revoke-<id>`), makes one (`#token-name`,
`#token-scope-<scope>`, `#token-go`; `#token-new` shows it once) and links administrators to **administration**
(`/admin`, `js/admin.js`, ADR 0071): the mode (`#caps-mode`), the drift badge (`#caps-drift`), a box per class and
capability (`#cap-user-<cap>`, `#cap-guest-<cap>`, saved on change, `#caps-result`), the limits (`#caps-limits`, a row a kind and a column a class), and
invitations (`#invite-go`, `#invite-link`); its tables are `table.admin-table`. Anybody else is told (`#admin-refused`); an
administrator who only lacks the second factor off the server's own machine is told so, with a link to it (`#admin-second-factor`). A seat's strip and a chat line show who holds it beside the name (ADR 0072): `.badge.account` (`@handle`) or
`.badge.guest`, and `.handle-badge`, `.chat-guest`. The profile window (`#open-profile`) of a guest has `#guest-box`, *Move to another device*: `#guest-copy` (the key
and its `#guest=` link, `#guest-link`, `#guest-key`, copied to the clipboard), `#guest-rotate`, `#guest-discard`, and
`#guest-use-key` with `#guest-use` to adopt a key; any page opened with `#guest=bgg_…` adopts it, takes it out of the
address and reloads. "My games" merges `GET /api/me/games` into `bingle.games`. The account page adds the address
(`#account-email`), the handle (`#handle-new`, `#handle-go`) and the sessions (`#sessions-list`, `#sessions-end`); the
admin page the audit log (`#events-rows`: local time, account, the kind as a tag, `.event-kind.bad` for failures). The top bar's profile window starts with who the browser is (`#profile-account`): a guest gets *Sign in*
(`#profile-signin`, back to the page afterwards) and, where the server allows it, *Sign up* (`#profile-signup`); an
account gets `#profile-account-who` ("Signed in as @ana"), *Your account* (`#profile-account-link`), *Administration*
for an administrator (`#profile-admin-link`) and *Sign out* (`#profile-signout`). The icon's title says the same.
A page offers only what the browser may do (its `caps` from `GET /auth/me`): without `dm`, or to a guest, the friends icon
(`#open-friends`) stays hidden; without `run_batch` the home page's practice card is hidden and the Analysis tab shows
`#analysis-signin` instead of its form; without `notify` the game options have no email section and the profile page
shows `#notify-accounts-only` instead of the address and the switch. The profile page's `#account-box` says whether the browser is a guest (`#login-link`, `#signup-link`)
or signed in (`#account-link`). None of them keeps anything in the browser's storage: the cookies are `HttpOnly`.

### Email notifications

`js/notify-ui.js` holds the browser's side: the contact this browser made (`bingle.contact` = `{id, secret, address}`; the
server never sends an address back) and the per-game setting, which only the guest that holds the seat may read or
change (ADR 0067): a browser that did not take the seat is told so (`#notify-no-token`). The **profile page** has the address, the switch, the status
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
*Take this seat* list shows the new cults. A table set up with a pool of factions
says so in `#lobby-pool`, lists only those, and gives a player the list on their own row too.

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
chosen in the setup (`#setup-variant`), whose button for the rules is `#game-rules-button`
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

### The battle show

Cthulhu Wars plays each battle on the map (`static/js/cw-battle.js`, *Battle animation* in the options;
off under the OS "reduce motion" setting). It is a layer over a copy of the map, fed only with snapshots
of positions, so the game never waits for it and the page underneath stays current. The copy zooms to the
area, which stays centred even at the map's edge, and the dark has the area's shape cut out of it. Each side
stands to one side, large (a big army shrinks to fit), with its dice between them.

The order is: the pools of dice, one die at a time on both sides; the Pre-Battle Phase; the throw; the
casualties, each with a red X; the Post-Battle Phase; and the zoom out at the `retreat` step, which is played
on the map. A phase is shown only if it has abilities, which come from `abilities` in the position's battle
view: the abilities its prompts are about and those a cult's moves can still use, each with the cult it
belongs to and whether the game is waiting on it. An ability that applies by itself offers no choice and is
not listed. Live, the show has nothing more to play until a choice is made, so it pauses by itself; a replay
holds on each for a moment.

Casualties that are the player's to choose can be clicked on the stage. A choice elsewhere on the map hides
the stage until it is made. Moves that arrive faster than the page draws them are fetched and played in order.

A phone gives the whole screen to the show. Held upright its page has no map, so one is drawn off the page to
be copied, and the stage is turned 90° with CSS, because a page can only lock the real orientation in full
screen after a tap (never on iOS), and a battle often comes from another player's move. On a phone the stage
also steps aside when the show has caught up with a choice that needs a button.

A Great Old One's first awakening has a show of its own (`#emerge-stage`, `js/cw-emerge.js`), Cthulhu's for now
(`js/cw-cthulhu-art.js`, about seven seconds, [ADR 0108](decisions/0108-a-great-old-one-emerges.md)). In the history it is a badge in the cult's colour
(`.log-emerges`; `.playable[data-emerges]` plays it again).

Selecting a move of a battle in the history plays that battle from its positions. Moving on to another move
of the same battle does not start it over; moving to a position outside the battle ends it. A battle can
finish inside the move that declares it, so the units that stood there beforehand come from the position
before that move. The browser tests turn the show off by default (`openGameAs`), as it covers the map.

### Illustrated board themes

Games played with stones on a board of cells (Blooms, Hex, Havannah, Yavalath, Yinsh,
Go, Gomoku, Pente, Reversi, Tak, Ataxx, Amazons, draughts and the like: every
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
`#submit-move`, `#reset-move`, `#undo-step`, `#resign`,
`.seat-strip .clock[data-clock]` (a seat number with a game clock, `turn` for a
per-move allowance), `#clock-field` with `#clock_seconds`, `#clock-handicap`,
`#clock_seconds_1|2`, `#clock-increment-row` with `#clock_increment`,
`#move-list button.mv[data-seq]`, `#hist-*`, `#analyze`, `#exit-analysis`,
`#rematch`, `#status-message`, `#conn-badge`, `#hint`, `#open-options`,
`#game-options`, `#share-link`,
`#save-image`, `.ghost-piece`,
`.hint-ring`, `#puzzle-list .puzzle-card` (the name for a puzzle game is `#puzzle_player_name`),
`#import-text`, `#fun-fact`, `#game-setup`, `#global-chat-log`,
`#active-games-dialog` with `#my-games li` and `.badge.open-seats`, `#setup-active-list li`, `#join-open-games li`, `#profile-dialog` with
`#profile-window-name`, `#here-list li[data-address]` and `#profile-page-link`,
and `svg.ui-icon[data-icon]` for the icons of the bar;
area maps add `.cell.area[data-position]` with `.area-bg`, `.mark-gate`,
`.token[data-piece,data-color,data-count]` (`.token-target` when it can be
picked), `#round-bar`, `#battle-panel`, `.seat-grid`, `.faction-card`,
`.summon-button[data-piece,data-cost]`, `#unit-info`, `.token-enhanced`, `.sign-chip`,
`.pool-unit`, `.gate-chip .gate-holder[data-holder]`, `#history-card` with
`#history-friendly` / `#history-notation`, `.log-step[data-seq]`,
`#phase-steps`, `#first-mark`, `#pass-confirm`, `#elder-bag`, `#ritual-track .ritual-step`,
`#events .log-fac`, `#options-reactions` with, per reaction (`<id>`:
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
