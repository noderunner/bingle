# HTTP & WebSocket API

Base path `/api` (JSON). Every error is `{"error": "<message>"}` with a status
code: `400` bad input or an illegal move, `403` not your turn / not a player,
`404` unknown room or game, `409` wrong game state (not active, seats full,
nothing to undo). Room codes are case-insensitive.

**Who is asking** ([ADR 0067](decisions/0067-principals-and-guest-keys.md)). A request comes from a
*guest* when it carries a guest key — the `bingle_guest` cookie of a browser, or
`Authorization: Bearer bgg_…` from a script — and is anonymous otherwise. A seat belongs to the
guest that took it: `player_name` says which seat a request means, and the seat's owner alone may
act for it (`403` for anybody else). A seat taken by an anonymous request belongs to nobody and is
acted for by name, as every seat once was, until the first guest to act for it becomes its owner.
`POST /api/guest` makes a guest for a script (`201 {key}`; `429` past 60 an hour from one address);
an unknown or expired bearer key is `401`. A browser never calls it: its first state-changing
request from bingle's page makes its guest and sets the cookie. A state-changing request proved by
the cookie must carry the header `X-Bingle-Session` (bingle's page sends it; a cross-site form
cannot), or it is `403`. A key unused for `BINGLE_GUEST_IDLE` (90 days) stops working.

Position documents appear as `state` (full: `cells` = stacks bottom-to-top,
`reserves`, `ply`) next to the legacy `board_state` (position → top piece).

## Games

| Method & path | Body / query | Response |
|---|---|---|
| `GET /game-defs` | | `[{id, name, seats_required, description, board_width, board_height, family, version, designers?, publisher?}]` (`version` is the game's semantic version: below 1.0.0 is a beta) |
| `GET /game-defs/{id}` | | full description (`start` is the starting position; `preview`, when the game starts empty, a sample game in play for pictures): `rules`, `notation_help`, `board` (positions with layout, regions, borders), `pieces`, `pools`, `actions`, `opening`, `win_rules`, `scores` |
| `GET /game-defs/{id}/guide` | | the rules guide as Markdown pages `{game, generated?, tabs:[{id, title, markdown}]}` — the game's own, or one generated from its definition (`generated: true`) |
| `GET /game-defs/{id}/assets/{path}` | | a file of the game's guide (images); `board.svg` draws an area map. Served inert (`Content-Security-Policy: sandbox`, `nosniff`); `404` for anything outside the module's files |
| `GET /bots` | | `[{id, label, description}]` |
| `GET /puzzles` | `?game_def_id=&max_win_in=` | `[{id, game_def_id, title, to_move, win_in, plies, tps?, state}]` — positions where the side to move wins by force |
| `GET /puzzles/{id}` | `?reveal=1` adds `first_moves` and a `solution` line | one puzzle; `404` if unknown |
| `GET /games` | `?limit=` (60 max) | `{games: [{room_code, game_def_id, name, family, status, lobby?, seats, open_seats, players[{seat,color,name,bot}], turn_seat, move_seq, updated_at}]}` — every unfinished game |
| `GET /bot-guide` | | the guide to the bots as Markdown (its drawings under `/bot-guide/assets/`) |
| `GET /here` | | `{people: [{address, name, since, seen}]}` — the browsers that have said they are here lately |
| `POST /here` | `{secret, player_name}` (an empty name leaves) | `{address, people[...]}` |
| `POST /dm` | `{secret, to, text}` | the message `{id, from, name, to, text, at}` |
| `POST /dm/history` | `{secret, with, after?}` | `{messages: [...]}` |
| `POST /dm/threads` | `{secret}` | `{address, threads: [{with, name, last_id, last_at, text, count}]}` |
| `POST /dm/call` | `{secret, to, join}` | `{ok}` — ring (or answer) somebody, or hang up |
| `POST /dm/signal` | `{secret, to, kind, data}` | `{ok}` — one WebRTC message for the person being called |
| `GET /ws-dm` | first frame `{type: "hello", secret}` | pushes `ready`, `dm`, `dm-call` and `dm-rtc` events for that browser |
| `POST /games` | `{game_def_id, player_name, seat?, opponent?, seat_bots? (`{"2": "random"}`: a bot in chosen seats; the rest stay open), lobby? (wait for the host's `start` even with every seat filled), move_time_seconds?, clock_seconds?, options? (setup choices — see below), start_tps?, start_state?, start_seat?}` | `{room_code, seat, color, status, bot_moves?}` |
| `GET /config` | | `{public_url}`: the configured `BINGLE_PUBLIC_URL` ("" when unset) |
| `POST /bot-matches` | `{game_def_id, bot_a, bot_b, delay_ms?}`: a two-player room where two built-in bots play each other at a watchable pace (150 ms – 10 s per move); open it as a spectator (`/games/{code}/join` with role spectator, or the WebSocket). Stops when the game ends, after 2 minutes with nobody watching, or at the move cap; at most 12 run at once | `{room_code, status}` |
| `POST /games/import` | `{text, game_def_id?, player1?, player2?}`: load a PTN document or a plain move list into a new room (full history; the rules engine validates every move, an illegal one is reported by number; a `[TPS]` header sets the start) | `{room_code, game_def_id, status, moves, players}` |
| `GET /games/{code}` | | `{game_def_id, status, seats[{seat,color,name,label,bot}], turn_seat, move_seq, winner_seat, end_reason, end_detail, move_time_seconds, clock?, turn_deadline, server_time, draw_offer_seat, takeback_seat, takeback_to_seq, start_state?, start_turn_seat?, lobby?, host_seat?, invite_url}` |
| `POST /games/{code}/join` | `{player_name, role: player\|spectator, seat?}`: `seat` asks for a particular open seat (`409` if taken); a player already seated keeps theirs | `{role, seat, color, status}` |
| `POST /games/{code}/bots` | `{player_name, bot, seat?}`: a seated player fills the lowest open seat (or `seat`) of a **waiting** game with a built-in bot, named after the seat's faction/colour and its kind (`Black Goat (MCTS)`); a game without a lobby starts when the last seat fills | `{seat, name, color, bot, status}`; `403` not seated, `409` full or already started |
| `POST /games/{code}/start` | `{player_name}`: the **host** begins a game waiting in its lobby; every seat must be filled | `{status, lobby, host_seat, move_seq, turn_seat}`; `403` not the host, `409` seats open / already started |
| `POST /games/{code}/seat` | `{player_name, seat}`: before the game starts, move to an open seat | as `start`; `409` seat taken |
| `POST /games/{code}/leave` | `{player_name}`: give up your seat before the game starts (the host role passes to the next human) | as `start` |
| `POST /games/{code}/remove` | `{player_name, seat}`: the host clears another seat (a person or a bot) before the game starts | as `start`; `403` not the host |
| `POST /games/{code}/seat-cult` | `{player_name, seat, cult}`: before the game starts, anyone at the table changes the cult (`gc`, `cc`, `bg`, `ys`, `ww`, `sl`) of an **open** seat — a cult played from another open seat changes places with it; the cults of seats a player or bot holds stay. The game takes the id of the new table (`cthulhu-wars~gc-cc-sl-ys`) and a fresh start position | as `start`; `409` seat taken / cult held by a taken seat / game started / game without cults, `400` unknown cult |
| `GET /games/{code}/reactions` | `?player_name=`: the reactions of your faction (`abilities[{id,name,text,held}]`; `held` is false until the spellbook is earned — the choice below can be made earlier), when the game asks you about them — the choice for each reaction, by ability id (`choices{<id>: {targeted, ask_all, ask_seats}}`; Madness starts with every box ticked, the others with `targeted` only — `ask_always` marks the reaction that asks every time until you say otherwise); `targeted`, `ask_all` and `ask_seats` at the top level are the choice for any reaction not set apart —, the seats that may name (`others`) and the reaction waiting for your answer (`pending {seat, trigger, ability, targeted}`) | settings; `403` not a player |
| `PUT /games/{code}/reactions` | `{player_name, ability?, targeted?, ask_all?, ask_seats?}`: a person's reactions are **declined for them** unless they ask to be asked — `targeted` when an action is aimed at them (on by default), `ask_all` after any other player's action, `ask_seats` after those seats' actions. Fields left out keep their value. With `ability` (an id from `abilities`; `400` for any other) the choice is for that reaction alone — the Crawling Chaos' Madness and the Unspeakable Oath can differ —; without it, it is made for every reaction. A reaction already waiting is declined at once unless one of these asks for it. Built-in bots are always asked; an AI client that joins over MCP starts with `ask_all`. The earlier body `{all?, seats?}` (what to *pass*) is still understood when none of the fields above is sent | settings |
| `GET /games/{code}/autoplay` | `?player_name=`: your Auto-Play Timeout — `{seat, enabled, timeout_seconds, default_timeout_seconds (86400), deadline? (RFC 3339: when a bot will play, while it is your turn and the option is on), server_time}` | settings; `403` not a person at the table |
| `PUT /games/{code}/autoplay` | `{player_name, enabled?, timeout_seconds?}`: when it is your turn to act (a turn of your own or a reaction waiting for your answer) and nothing has happened in the game for `timeout_seconds` (60 – 31 536 000), a tactical bot plays the decisions of that turn for you. Off by default, 24 hours when on. Its moves are your moves, flagged `auto_play`. `400` for a timeout out of range | settings |
| `GET /games/{code}/chat` | `?after=ID&limit=N`: the latest messages (default 100), or those after `ID` | `[{id, seat (0 = spectator), color, name, text, at}]` |
| `POST /games/{code}/chat` | `{player_name, text}`: say something (a seated player under their seat, anyone else as a spectator); ≤ 500 characters, bursts limited (`429`) | the message |
| `GET /chat` | the **global chat** of the home page; `?after=ID&limit=N` as the game chat. No game, no seat: everybody reads it | `[{id, name, text, at}]` |
| `POST /chat` | `{player_name, text}`: say something to everybody; ≤ 500 characters, bursts limited (`429`); the latest 500 messages are kept | the message |
| `GET /notify/config` | whether email notifications are set up | `{enabled, from?}` (`from`: the sender address, so players can allow it) |
| `POST /notify/contact` | `{address, contact_id?, contact_secret?}`: ask to be mailed at an address. Without the id and secret a new contact is made (`contact_secret` is returned once); with them its address is changed. One confirmation link is mailed to the address, nothing else until it is followed. The address is never returned | `{contact_id, contact_secret?, status: pending\|verified\|stopped, masked, confirmation_sent, note?}`; `400` bad address, `403` wrong secret, `404` feature off, `409` the address was stopped |
| `POST /notify/contact/status`, `/resend`, `/forget` | `{contact_id, contact_secret}`: the state of the contact, mail the confirmation again (limited), delete it and every game setting that used it | as above; `403` wrong secret |
| `GET /games/{code}/notify` | `?player_name=`: the seat's setting, for the seat's owner (a guest; never an anonymous request) | `{configured, enabled, away_minutes, contact?: {status, masked}}`; `403` not the seat's owner |
| `PUT /games/{code}/notify` | `{player_name, contact_id, contact_secret, enabled, away_minutes}`: mail the seat when it is its turn (`away_minutes` 0: every time; N: only when the turn waited N minutes; at most 1440). For the seat's owner and, to switch on, a confirmed contact the caller owns | the setting; `403` not the owner or a bad contact secret, `409` address not confirmed or stopped |
| `GET /games/{code}/voice` | who is in the game's voice chat (see [voice.md](voice.md)) | `{peers: [{seat, name}]}` |
| `POST /games/{code}/voice` | `{player_name, active}`: a seated player joins the call (`active: true`, repeated every ~15 s as a heartbeat; a place lapses after 45 s) or leaves it | `{peers}`; `403` not a player |
| `POST /games/{code}/voice/signal` | `{player_name, to, kind, data}`: relay an `offer`, `answer`, `ice` or `retry` message (≤ 16 KB of JSON, ≤ 300 per 10 s) to seat `to`, delivered only to that seat's WebSocket | `{ok}`; `403` sender not in the call, `409` target not in it, `429` too fast |
| `GET /rtc/config` | the `iceServers` a voice call uses: STUN, and a TURN relay with credentials that expire after 6 hours when configured | `{ice_servers: [{urls, username?, credential?}]}` |
| `GET /games/{code}/state` | `?at_seq=N` (0 = start) | `{board_state, state, turn_seat, status, move_seq, winner_seat, end_reason}` |

**A game against oneself.** `also_seats: [2, 3]` seats the creator in those seats
too (besides `seat`): each is a player of its own named after the creator and the
seat (`Alice · Black Goat`, at most 40 characters), so the game still has a
player for every seat — it starts at once when the table is full (or waits in the
lobby for the host's `start`). Moves are made with the name of the seat to move;
`undo` takes back the last move together with the later moves of one's other
seats. An unknown seat is a `400`.

**Setup options.** A game may list `options` in `GET /game-defs/{id}` — for
Cthulhu Wars `factions` (a `seat-choice`: which of the six cults sits in each
seat, in seat order, none twice) and `high-priests` (a `bool`). `POST /games`
takes their values as `"options": {"factions": ["ww","sl","gc","cc","ys"],
"high-priests": true}`; the game created has an id of its own, `base~choice`
(`cthulhu-wars-5p~ww-sl-gc-cc-ys-hp`), reported as `game_def_id` by
`GET /games/{code}` and accepted by `GET /game-defs/{id}` and `POST /games`
like any other (see [ADR 0028](decisions/0028-derived-games-and-setup-options.md));
`GET /game-defs` lists only the base games. Options that do not fit the
layout (wrong number of cults, a cult twice, an unknown key) are a `400`.

`status` is `waiting`, `active` or `finished`. `opponent` seats a built-in bot
in every other seat (the game starts immediately); `seat` chooses the
creator's seat; `move_time_seconds` gives each move a time limit — the player
to move loses on time when it runs out (the clock only runs while the game is
active, and is evaluated whenever the game is read or moved in).

**The game clock.** `clock_seconds` plays the game with a chess clock instead:
a number gives both players that many seconds, a list of one time per seat
gives them their own (handicap play). It is for a game of two seats and not
together with `move_time_seconds`; anything else is a `400`. Each player's
clock runs only while it is their turn — a move takes the time it took off the
mover's own clock — and the first player to run out forfeits, enforced the same
lazy way as a per-move limit. `GET /games/{code}` reports it as
`clock: {running, seats: [{seat, initial_seconds, seconds_left}]}`, the running
seat counting down to `turn_deadline`; `move` and `state` events carry the same
object, and `get_observation`'s `seconds_left` follows it.

**Who is here, and what is going on.** `GET /games` lists every game on the
server that has not finished — waiting for players, sitting in a lobby or being
played — newest change first, with who is seated and how many seats nobody
holds; the *Active games* window shows it beside the browser's own games, so a
person can find a game to watch or an open seat to take. A room code is all it
has ever taken to join a game, so this makes the rooms public to look at: a game
nobody should find is one nobody should be able to join by code either.

`POST /here` says that somebody is looking at bingle. There are no accounts, so
a browser identifies itself with a `secret` of its own making, sent to the
server and to nobody else; what the list carries is the **address** that secret
hashes to (the first 8 bytes of its SHA-256), so a person can be found —
and written to — without their address being forgeable. A browser is forgotten
two and a half minutes after it last said anything, and nothing is stored.

**Direct messages.** Two people who are here (`POST /here`) can write to each
other and put a voice call up between them. A message is addressed to the
**address** of a browser and sent with the sender's own **secret**, so the
sender cannot be faked while anybody can be written to; what a message proves is
that it came from the same browser as the last one, not whose browser that is.
Nothing is stored: the last 200 messages of a pair live in memory and are
forgotten two hours after the last one (and at a restart). A message to somebody
who is not here is a `404`, writing without having said who you are a `403`, and
a flood a `429`. The history is a POST because a secret belongs in a body, not
in a URL that ends up in logs.

The **call** is WebRTC between the two browsers, with `/dm/signal` relaying the
offer, the answer and the candidates to the other side's `/ws-dm` socket, as a
game's voice chat does over its room. Both sides must have joined
(`/dm/call` with `join: true` — the second one is answering) before a signal is
relayed; otherwise `409`.

**One voice at a time.** A browser that sends its `secret` when joining a
game's voice chat (`POST /games/{code}/voice`) or a direct call is held to one
call: joining a new one takes it out of the old, which is told about it (the
room gets a `voice` leave event, the other end of a direct call a `dm-call` end
with `dropped: true`). An API client that sends no secret is not tracked and
keeps the old free-for-all.

**Custom start positions.** `start_tps` (PTN games such as Tak; the TPS says
who moves first) or `start_state` (a position document — the `state` object of
any state, observation or simulate response — with `start_seat` moving first;
default is the game's first seat) begins the game from that position instead
of the standard one: puzzles, endgame practice, "play from here". The
position is validated (squares, pieces and colours must exist) and must not
already be decided; otherwise `400`. The room remembers it: `?at_seq=0`,
undo-to-start and `export` use it, and `GET /games/{code}` returns it as
`start_state` / `start_turn_seat`.

Games with `engine: "ruleset"` (Courtfall, Cthulhu Wars) return a *public view* of every
position: secrets (the dice generator, a committed choice) are masked, the
ruleset's derived `state.ext.view` is attached, and `start_state` / exports of
a live game carry no seed. `GET /api/game-defs/{id}` adds `engine`, `display`
(tint/badge legend), `count_labels`, `params` and — for `board.style: "areas"` —
`board.canvas` and `board.areas[{id, name, polygons, anchor, tags}]`; `guide` lists the
tabs of a hand-written rules guide; its `actions` list the verbs
that appear as buttons. Undo and take-back are refused (409) once a die has
been rolled since the move.

## Moves

| Method & path | Notes |
|---|---|
| `POST /games/{code}/moves` | body `{player_name, notation}` **or** `{player_name, action?, piece?, from?, to?, count?, path?, drops?, aim?}` **or** legacy `{player_name, from, to}`. Response `{move, state, bot_moves?}`; `bot_moves` are replies a bot opponent played immediately (already reflected in `state`). |
| `GET /games/{code}/moves` | history `[{seq, seat, color, from, to, captured, action, piece, count, path, drops, aim, notation, created_at}]`; `?since_seq=N` |
| `GET /games/{code}/moves/legal` | every legal move for the seat to move (`?seat=N`): `{seat, moves:[{notation, action, piece, from, to, count, path, drops, aim, steps?}]}`; `?steps=1` adds the click-by-click selection steps |
| `GET /games/{code}/legal-moves?from=SQ` | v1: destinations reachable from one square |
| `POST /games/{code}/undo` | `{player_name}`: take back your last move (and bot replies). `409` once another human has moved |
| `POST /games/{code}/resign` | `{player_name}` |
| `POST /games/{code}/hint` | `{player_name, at_seq?, moves?, bot?}`: a bot's choice (default: `minimax` where piece counts matter — Tak, Reversi, draughts — otherwise `mcts`) for the current position, or after `at_seq` + a variation → `{notation, steps, turn_seat, bot, win_rate?, alternatives?}` (MCTS reports its estimated chance of winning, 0–1, and the best few moves; minimax only the move). Only for players, and only against bots or after the game (`409` otherwise) |
| `POST /games/{code}/takeback` | `{player_name, action: request\|cancel\|accept\|decline}`: once a human opponent has replied, ask to take back your last move; accepting rewinds to the position before it, requester to move. Any move withdraws a request. `409` for nonsensical requests (nothing to take back, `undo` would work directly, bot opponent). Open requests are `takeback_seat` / `takeback_to_seq` in game/state responses. |
| `POST /games/{code}/draw` | `{player_name, action: offer\|cancel\|accept\|decline}`: offer a draw to a human opponent; accepting ends the game (`end_reason: agreement`); any move withdraws an offer. `409` for nonsensical requests (no offer to answer, bot opponent). The offer is `draw_offer_seat` in game/state responses. |
| `POST /games/{code}/simulate` | `{moves:[notation…], at_seq?}`: apply a line to the current or a past position without changing the game; returns the resulting `state`, `turn_seat`, `game_ended`, `legal_moves` and, for Tak, the position as `tps` |

## Agent and archive helpers

| Method & path | Notes |
|---|---|
| `GET /games/{code}/observe?player_name=` | decision context: `board_text`, `tps`, `state`, `legal_moves` (when it is your turn), `last_move`, `done`, `result`, `reward` (+1/−1/0), `seconds_left` |
| `GET /games/{code}/export` | game record: moves + notation, initial/final state, result; `?format=ptn` for a PTN document (Tak) |

## WebSocket `GET /ws/{code}?name=NAME&role=player|spectator[&only=voice]`

Server-push only. With `only=voice` the socket belongs to a **voice call**, not to a page: it
carries just the `voice` and `rtc` events (below), is the only one that receives `rtc`, and does
not count as the player looking at the game (no effect on "your turn" mail). The server has
subscribed before it sends `joined`, so a client that waits for `joined` misses no call message. The first message is
`{"type":"joined","data":{role,seat,color,status}}`; afterwards:

* `{"type":"move","data":{seq, seat, color, notation, from, to, action, piece, count, path, drops, aim, turn_seat, game_ended, winner_seat, end_reason, status, board_state, state, auto?, auto_play?, locked?, turn_deadline?, server_time?}}`
* `{"type":"state","data":{reason, removed?, status, turn_seat, move_seq, winner_seat, end_reason, seats[], board_state, state, …}}`
  — sent when something other than a move changed the game (`reason` is
  `join`, `leave`, `seat`, `cult`, `start`, `undo`, `resign`, `timeout`, `draw-offer`, `draw-declined`,
  `draw-cancelled`, `draw-agreed`, `takeback-request`, `takeback-declined` or
  `takeback-cancelled`; an accepted take-back arrives as `undo` with
  `removed`). State events also carry `draw_offer_seat`, `takeback_seat` and
  `takeback_to_seq`.

* `{"type":"chat","data":{id, seat, color, name, text, at}}` — a chat message.
* `{"type":"voice","data":{kind: "join"|"leave", seat, name}}` — somebody joined or left the voice chat.
* `{"type":"rtc","data":{from, kind, data}}` — a voice-call message from seat `from`; sent
  **only to the `only=voice` connection of the seat it is addressed to** (a player's socket), never to the room.

## WebSocket `GET /ws-chat`

Server-push only, no parameters: every message of the global chat (`POST /api/chat`) as
`{"type":"chat","data":{id, name, text, at}}`.

A move event may carry `auto: true`: the server declined a reaction for a person
(their reactions are declined unless they asked to be asked — see `/reactions`),
or `auto_play: true`: a tactical bot played it for a person whose Auto-Play
Timeout ran out (see `/autoplay`). The move lists (`GET …/moves`) carry the same
flags.

A client that misses events can always re-read `GET /games/{code}` and
`/state`. Joining over the socket as a `player` with an unknown name takes an
open seat (same rules as `POST …/join`).

## MCP

`POST /mcp` speaks Streamable HTTP MCP. See [mcp.md](mcp.md).

## Training environments

`/envs`, `/env-specs/{game}` and `/matches` expose the in-memory environments
of the MCP `env_*` tools over plain HTTP — see
[mcp.md](mcp.md#training-over-plain-http-and-python) for the table and the
bundled Python client.

## Practice batches (`/api/batches`)

Bot-versus-bot Cthulhu Wars games run in the background and measured (see ADR 0034).

| Route | |
|---|---|
| `POST /api/batches` | start (queue) a batch. Body: `name`, `base` (layout id), `high_priests`, `players` [`{faction: gc\|cc\|bg\|ys\|ww\|sl, bot: random\|tactical\|minimax\|mcts, mcts_iterations, minimax_nodes}`] one per seat, `games` (≤ 5000), `seed`, `rotate`, `max_plies`, `event_games` (default 0; the wizard sends 20), `workers`. 201 with the batch |
| `GET /api/batches` | `{batches, default_workers}`, newest first |
| `GET /api/batches/{id}` | the batch: status (`queued running done cancelled failed interrupted`), `done`/`games`, `games_per_minute`, spec |
| `GET /api/batches/{id}/report` | `{batch, report}`: the aggregate of the games finished so far (per cult win rate + interval, by seat, by round, spellbooks, units, actions, areas, each with a win-rate lift) |
| `GET /api/batches/{id}/games` | one row per finished game (seating, winner, end, plies, rounds) |
| `GET /api/batches/{id}/games/{n}/events` | the decisions of game *n* (only the first `event_games` keep them) |
| `POST /api/batches/{id}/cancel`, `DELETE /api/batches/{id}` | stop; delete with its games |

MCP: `start_batch`, `get_batch`, `list_batches`, `cancel_batch`. Pages: `/batches`, `/batches/{id}`.

### GET /api/games/{code}/history

What every move of the game did, as the game narrated it, oldest first: `[{"ply":12,"seat":2,"lines":["..."]}]`. A position's own `ext.log` keeps only the latest events; this joins the part each move added. `[]` for a game that does not narrate. The Cthulhu Wars History tab reads it.

### Presence (present / away)

`GET /api/games/{code}/presence` returns `{"seats":[1,3]}`: the seats whose players say they are watching the game right now. `POST /api/games/{code}/presence` with `{"player_name":"Alice","present":true}` sets or clears the caller's own seat (403 for a name without a human seat) and returns the same list. A mark lapses after 150 seconds unless posted again (the page renews it every minute); it lives in memory only. Every change is pushed to the room as a WebSocket event `{"type":"presence","data":{"seat":1,"present":true}}`.
