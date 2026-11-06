# 0114. Impulses, selections, moves and gestures

Status: accepted (2026-11-05); not yet implemented (see *Migration*). It replaces the line "Each step is an impulse of
its own" of [ADR 0099](0099-impulses.md) and, from its first phase, the steps of a turn that
[ADR 0083](0083-a-move-stands.md) stores and takes back and the move per unit that
[ADR 0100](0100-a-move-gathers-its-units.md) sends.

## Context

**One word did several jobs.** *Step* named the click path the server gives each legal move (`state.Step`,
`Ruleset.Steps`, `?steps=1`), a part of a turn sent to the server on its own (ADR 0083) and the page's Back
(`#undo-step`), besides the DSL's `step` action, a tutorial's steps and a training environment's step. `docs/ui.md` used
*selection* for what the player clicked, and ADR 0099 made each step an impulse.

**A turn is split across calls.** Eleven games send each part of a turn to the server as a move of its own (ADR 0083):
a Blooms stone, then another or `done` (ADR 0038); each unit of a Cthulhu Wars Move, then `done` (ADR 0100); each
decision of a Courtfall turn. The parts are stored before the turn ends and kept out of the History until it does. Back
takes them back (`POST …/undo`, `moves.locked`), and `move-final` tells the page which part cannot come back. A queued
Move sends one call per unit, and if one is refused the ones before it stand.

**The split has a reason.** A whole Blooms turn is about 7,000 possible moves on the standard board, against about 120
for each stone (ADR 0038), and a Move of several units is far more. Bots, MCP agents and the training environments all
work from a list of legal moves.

## Decision

### The words

These words are used everywhere: in Go, the page, REST, MCP, the docs and the names of things. A game's rules and its
players keep their own (turn, round, action, phase, reaction).

| Word | Meaning |
|---|---|
| **Impulse** | The choices one seat (a player, a bot or an agent) makes before the game needs something from elsewhere. It is submitted in one call and applied as one. It is independent of a game's rounds, turns, actions and phases: a turn may be several impulses, and an impulse may run on into the seat's next turn when nothing comes between. |
| **Impulse seat** | The seat whose impulse the game is waiting for, as in ADR 0099 (`impulse_seat`, `your_impulse`, the impulse card). |
| **Selection** | One self-contained choice within an impulse, with its own notation and a position after it: a stone, one unit's move, a card. It is what one `Apply` handles today. |
| **Move** | The selections of one impulse, in order: what is submitted, stored, replayed and written in notation. Until it is submitted it is a **pending move**. |
| **Gesture** | One thing a player does in an interface while building a move: a click, a tap, a drag, a key, Submit or Back. |
| **Ply** | One impulse applied: a game's ply count is the number of impulses played. |

Selections and gestures do not match one for one. A Blooms stone takes two gestures (its colour, then its cell); one
gesture on a destination completes the selections of every unit in a Cthulhu Wars queue; Submit and Back make no
selection at all. So that a client needs no code of a game's own, the server still describes the gestures that enter
each selection. That is **gesture data**, part of a game's presentation like its view, and the rules never depend on it.

*Step* keeps only its other meanings: the DSL's `step` action (a piece moving one square), a tutorial's steps, a training
environment's step (the reinforcement-learning term: one step applies one selection), and plain English, as in stepping
through the history.

### Where an impulse ends

**An impulse ends exactly where the next choice belongs to another seat (or to someone else in the same seat, as after
the pie-rule swap), or depends on something the seat cannot know yet — a roll, a draw, a shuffle, a card turned up — or
the game ends. It ends nowhere else.**

- The boundary follows from the game state and the rules alone, never from a seat's settings or from who is playing it.
- A game may not end an impulse anywhere else, and a seat may not submit part of one: there are no pauses. A pending
  move lives in its client, which may keep it across a reload.
- **A game for one seat is the exception** (amended 2026-11-06): its seat may end an impulse after any selection. Nobody
  waits on it and there is no clock or rating to game by pausing, while without it a game of FreeCell, which turns no
  card up, would be one impulse from the deal to the end. A game of patience is sent move by move, and it keeps
  `POST …/undo` for a move that showed no card (ADR 0086, as narrowed here).
- Stopping is a choice, so it is a selection: Blooms' `done` after one stone, a Move's `done`. Every impulse ends at a
  leaf where nothing is left to choose, and a client submits when it gets there.
- Chance that nothing later in the impulse depends on is resolved when the impulse ends, instead of splitting it.

### Reactions, and the dummy

- A ruleset opens a reaction window from what everyone can see, and the acting seat's impulse ends there, every time.
- The reacting seat then has an impulse. If its settings say not to ask, the session submits the decline for it
  (`Auto`), reading the settings when that impulse begins: a dummy plays for the seat. This is also how the Auto-Play
  Timeout and a bot in a seat work: something other than the person submits the seat's impulse, and the engine cannot
  tell the difference.
- Where whether a seat can react is secret, the window opens for every seat that might, and the dummy answers for any
  that cannot, as it does for one whose settings say not to ask, so a decline and an inability look alike.
- When several seats may react, each has an impulse of its own, in the order the rules give.
- Outside reaction windows the dummy plays nothing: an impulse with only one possible move still waits for its
  player's Submit, so that they see it go by.

### The selections query

**`state.NextSelections(game, position, seat, made)`** lists what may come next after the selections `made`. It is pure
and deterministic, like `Legal`. REST serves it as `GET /api/games/{code}/selections?ply=N&made=…` and MCP as
`list_selections`; bots and the training environments call it in process (an environment holds the selections made
between its steps). It answers with:

- the options for the next selection, each with its notation, its gesture data and a prompt in the game's own words
  (ADR 0099);
- the position as that seat would see it after the selections made, unless one of them ended the impulse. Then it says
  only that the move is complete: a query must never play a selection that rolls or draws, or a client could ask what a
  battle would bring and back out. That is why `move-final` answers only yes or no, and why `simulate` samples whatever
  its viewer cannot know (`state.Determinize`);
- optionally, the options below to some depth while that tree is small, so that a client need not ask at every gesture.
  Today's full list of legal moves is the deepest such answer.

Nothing is kept between queries, and the query is answered on the same terms as legal moves (ADR 0112). The query and
the submission (`POST /api/games/{code}/moves`) both carry the `ply` they were made against, and are refused if the game
has moved on: a move may take minutes to build, and a resignation, a vote-kick or a timeout can come in between. A
move's notation is its selections' notations joined by `;` (`red@d4; yellow@e5`), as Hive and Mycelium have spaces
inside one selection and boards labelled `xy` have commas.

### Names

| Today | After the first phase |
|---|---|
| `state.Move` (one `Apply`) | `state.Selection` |
| none | `state.Move`: the selections of one impulse |
| `state.Step`, `Ruleset.Steps`, the `steps` field, `?steps=1`, a tutorial guide's `play_steps` | `state.Gesture`, `Ruleset.Gestures`, `gestures`, `play_gestures` |
| `state.LegalMoves`, `GET …/moves/legal`, `GET …/legal-moves?from=` | `state.NextSelections`, `GET …/selections` |
| `POST …/move-final` | removed |
| `POST …/undo`, MCP `undo_move`, `moves.locked` | kept for games for one seat only; removed elsewhere, where nothing is sent that could be taken back |
| MCP `get_legal_moves`, `list_legal_moves` | `list_selections` |
| MCP `auto_play_turn` | `auto_play_impulse` |
| `#undo-step` | `#back` |
| the "selection" phase of the search bots (`internal/bots`) | the descent |
| `POST /envs/{id}/step` | unchanged |

As in ADR 0099, the old names go without aliases, so a client of the REST or MCP API changes with them.

## Migration

**Phase 1: the engine and the API, for every game at once.** An impulse is today's parts, grouped by the test
`MoveIsFinal` already makes (the game ends, the next impulse seat is another, the pie-rule swap, or the ruleset's guard
says the part rolled, drew or turned something up), moved into `state` as a pure function. The page builds a pending
move and submits it once, so a queued Move is one submission. Stored games are regrouped by the same test, and the
renames above land. No ruleset changes, except that every Go ruleset's guard must count a card turned up as well as a
roll or a draw: Solitaire has no guard yet.

**Phase 2: game by game.** Games whose parts do not fit are re-cut: Courtfall's turn flow and Mycelium's node-by-node
conversions. Blooms' `max_plies` is halved on every board, since a ply becomes a turn, and tutorial scripts follow their
games. Then the DSL gains impulses of several selections, the first addition of the DSL coverage audit plan.

## Consequences

- Nothing is stored before an impulse ends, so the History never hides part of a turn, a queued Move cannot be half
  played, and Back is the client's alone: ADR 0083's last take-back goes, and `move-final` with it, and `/undo` and `locked` but in games
  for one seat.
- A pending move is lost if its player changes device in the middle of it.
- Regrouping changes the move numbers of stored games in the split games: history cards, the journal, `simulate`'s `at`
  and tutorials that count plies.
- In Solitaire each move is sent as it is made, and Back takes back any of them but one that turned a card up. Only
  games for one seat keep `/undo`.
- The engine still has a single impulse seat, so simultaneous choices (Nyet!'s passing to the left, Courtfall's secret
  split) are taken in turn, with what is hidden kept hidden. The definition allows several impulses outstanding at once,
  for later.
- A dummy answers at once and a person after a pause: the one tell left, as at a table.
- Bots work from the selections query instead of a full list of legal moves. Their branching and the training action
  spaces stay as they are, since a selection is what a move is today.
