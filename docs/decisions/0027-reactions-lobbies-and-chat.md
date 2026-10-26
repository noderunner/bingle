# 0027. Reactions, lobbies and chat

## Context

The third Cthulhu Wars playtest asked for three things the platform did not
have. Great Cthulhu's *Devolve* may be used "after any player's Action, even on
their turn": an *interruption* that stops the game until its holder answers,
which is a common quirk of board games and should be a core feature with a way
to say "don't ask me". People also wanted table talk, and a way to invite
players from other networks into a game whose seats and parameters are set up
before it starts.

## Decision

**Reactions** are an optional ruleset interface, `state.Reactor`:

* `PendingReaction(game, st)` says whether the decision the position waits for
  is an optional reaction — who may answer, whose action opened the window,
  which ability and which move declines it;
* `ReactionAbilities(game, st, seat)` lists the reactions of the seat's faction
  (for settings), each flagged `held` once the seat can use it. They are listed
  from the first move: the auto-pass choice does not depend on having earned the
  spellbook, and a player must be able to make it before the first window opens
  (the playtester looked for Devolve's setting before earning it).

The ruleset models a reaction as an ordinary decision of the reacting seat
(Cthulhu Wars: a `react` prompt queued after another player's action, or the
post-battle / retreat stages of a battle the seat is not in). The game stops
for it like for any other decision; the *session* removes the stall: a player
may store, per game and seat, an auto-pass choice — everyone, or a list of
seats — and `runBotsFor` (which already plays whatever is automatic) plays the
declining move for a person whose pending reaction was opened by a chosen seat.
Such moves are recorded with `auto = 1` (shown in the move list, and ignored
when deciding whether the previous mover may still undo). The preference is
stored in `reaction_prefs` and exposed through `GET/PUT /games/{code}/reactions`
and the MCP tools `get_reactions` / `set_auto_pass`.

**Chat** is a small append-only table (`chat_messages`, the latest 500 lines
of a game are kept). Players speak under their seat, anyone else as a
spectator under a chosen name; there is a burst limit per name. Messages are
pushed to every WebSocket of the room as `chat` events and read with
`GET /games/{code}/chat`.

**Lobbies.** A game created with `lobby: true` stays `waiting` after its last
seat fills until its host (the first human who sat down) calls
`POST /games/{code}/start`. While waiting, players may choose or change seats
(`join` with a `seat`, `POST …/seat`), leave, be removed by the host and fill
seats with bots (`POST …/bots` with a `seat`). `BINGLE_PUBLIC_URL` is the base
URL used for the invite link returned as `invite_url` by `GET /games/{code}`
(falling back to the address of the request) and by `GET /config`.

The choice must be changeable at any time, above all while the other players
are acting — which is when the page is redrawn many times a second. A control
inside the redrawn area loses clicks that straddle a redraw (the click's target
is replaced between button-down and button-up) and a slow answer to an older
request can put an older choice back. The client therefore keeps the reactions
bar in a slot of the page template *outside* the redrawn area, updates it in
place, shows a choice before the server has answered and ignores answers older
than the player's last choice.

## Consequences

* The reaction machinery is generic: any ruleset can mark decisions as
  reactions; clients need only `reaction` in the view for the label.
* Auto-pass is advisory automation, not a rules change: the declined
  decision is a normal move in the history.
* Identity is still name-only (ADR 0006): the host is whoever created the game,
  and chat/lobby actions are authorised by name like moves.
* Bots are named after their seat's faction and kind when the table has three
  or more seats ("Black Goat (MCTS)"), once, when they sit down.

> **Later (ADR 0033):** the default was reversed. A person's reactions are now
> passed for them unless they ask to be asked (when targeted, after anyone's
> action, after chosen players'); the `all`/`seats` choice described above became
> `targeted`/`ask_all`/`ask_seats`, and bots are always asked.
