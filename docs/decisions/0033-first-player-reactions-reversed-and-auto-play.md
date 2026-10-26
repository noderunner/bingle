# 0033. The first player of the game, reactions passed by default, the Auto-Play Timeout

## Context

The playtest of game TQQJ96 (five cults, three of them people) turned up
thirteen things. Four of them were rules or engine bugs:

* **Black Goat was the first player of round one, not Great Cthulhu.** The
  engine started every game with seat 1 as first player and play running down
  the seats; a table that did not seat Great Cthulhu first was played in the
  wrong order for the whole first round. The rule book (Determine First Player
  Phase) says: *on the first turn of the game, Cthulhu is the First Player. If
  you are in a game without the Cthulhu Faction, choose the first player by any
  means you wish (we recommend ... alphabetic order per Faction name — i.e.,
  Black Goat is before Yellow Sign)*; the first player then chooses the
  direction of play for the round, the first Action phase included, and the Doom
  phase is skipped on the first turn.
* **The Black Goat could not end its turn after one summon.** Fertility Cult
  makes Summon an unlimited action, and the engine only offered *Done* once the
  turn's main action was taken, so a player who summoned a Ghoul and wanted
  nothing else had to take another action.
* **A Doom total nobody could trace.** Great Cthulhu had 6 Doom with 3 Gates:
  3 were a gift of Yellow Sign twenty moves earlier, 3 were the Gates. The log
  said only *advances 3 (now 6)*.
* **The History tab vanished.** The tab was only drawn when something had been
  narrated since the viewer's last decision; a Done and the passes of the others
  narrate nothing, so the tab disappeared until someone looked at an earlier
  position — whose window had lines.

The rest were features: reactions are a nuisance to answer all the time, so
they should be passed by default and asked about on chosen occasions (above
all when a player is *targeted*); a player who is away should not hold up a game
for ever; the lobby should let the table change a seat's cult; the chat should
have emoji.

## Decision

**The first player of the game** is Great Cthulhu wherever it sits, else the
cult first by name (Black Goat, Crawling Chaos, Great Cthulhu, Sleeper,
Windwalker, Yellow Sign). `gameCfg.first` (`firstSeatOf`) says whose seat, a
derived table puts that seat — or the Windwalker's, who sets up before anyone
plays — first in `turn_order.seats` (the seat the platform asks first), and
`Start` leaves the position at the direction stage with `Data.Opening`: the
first player's `cw`/`ccw` begins the first Action phase (`beginFirstAction`,
which logs *— Round 1 —* and skips the Doom phase). A game stored before this
decision has no `Opening` and keeps its order. The rules reference says so
(*The round*, *Setting up a table*).

**A Fertility Cult summon is an action.** `TurnState.Summoned` is set by it and
`legalTurn` offers *Done* (not *Pass*, which would drop the Power) from then on,
beside more summons and the main actions; a turn with nothing else to do ends by
itself. The bots treat that *Done* like *Pass* when choosing, so their play is
unchanged.

**Doom has a ledger.** `Data.DoomBy` (gates, rituals, gifts, signs) is kept by
`addDoom`; the lines of the Doom phase, a Ritual, a gift and a revelation give
the track before and after and what it is made of (*3 → 6 on the Doom track (3
from Gates, 3 from gifts)*); the Doom counter of a cult says the same on hover.

**The History tab is there whenever the game narrates.** Where nothing has been
told the pane says so; the window starts at the first decision of the viewer's
last turn, not at its last (a silent Done), and says when the others have added
nothing.

**Reactions are passed for a person unless they ask to be asked** — the opposite
of ADR 0027's default. The occasions to be asked (per game and seat, stored in
`reaction_prefs` as `{v:2, targeted, ask_all, ask_seats}`):

* `targeted` — on by default — when the action that opened the window was
  *aimed at* the reacting seat: the engine records, for each *action*, the seats
  it names or affects and the areas it touches (`Data.Aim`, `noteAim`), and a
  window carries `targeted` when its owner is among those seats or has a unit or
  a controlled Gate in one of those areas; the windows of a battle (Necrophagy,
  Cannibalism, Madness, Energy Nexus) when the owner stands in the battle's
  area. Only actions have an aim: the effects that follow a battle — Madness
  moving a unit, retreats — and the free actions open no window. A modal behind a
  **?** explains this to the player.
* `ask_all` after any other player's action (and in the Gather Power window);
* `ask_seats` after those seats' actions.

Rows saved by the earlier choice (`{all, seats}`: what to *pass*) are converted
when read (pass everything → never asked; pass these → asked after the others
and when targeted; pass nothing → asked after everyone). **Bots are asked every
time** (the engine prompts them and the guide decides) and have no settings; an
**AI client** that creates or joins a game over MCP starts with `ask_all` (a
row is written for it), so it is presented with every window as a bot is.
`PUT /games/{code}/reactions` and the MCP tool `set_reactions` replace the
earlier `all`/`seats` and `set_auto_pass`.

**The Auto-Play Timeout** — a person's option, per game and seat
(`autoplay_prefs`, off by default, 24 hours when on, one minute to a year): when
it is their turn to act (a turn, or a reaction waiting for their answer) and the
game has not changed for the timeout, the `tactical` bot plays the decisions of
that turn for them, until play passes to someone else. The wait is counted from
the game's last change (`games.updated_at`), so every move — the person's, the
bot's — starts it afresh, and time the server was down does not count (the wait
begins at the later of the last change and the moment the watcher came up).
`Manager.RunAutoPlay` (a goroutine started by `main`) looks every twenty seconds; `AutoPlayDue` does one pass and is what the
tests call. The moves are recorded as the person's (`moves.autoplay`,
`auto_play` in move events and lists), shown *(auto-play)* in the move list and
as a badge in the History, and can be taken back like any other move unless a
die was rolled. The options window has the switch, days/hours/minutes and a **?**
with the explanation (`GET|PUT /games/{code}/autoplay`).

**The cult of an open seat can be changed in the lobby** by anyone at the table
(`POST /games/{code}/seat-cult`, MCP `set_seat_cult`, a list in the lobby row):
the optional `state.SeatCults` gives the setup options of the table with the
change (a cult played from another *open* seat changes places with it; a cult
held by a taken seat is refused), the manager derives the new game id, stores the
new definition and a fresh start position, and publishes a `state` event with
reason `cult`, on which clients reload the definition.

**The chat has an emoji picker** (a button and a panel, the last twelve used
first).

## Consequences

* New games begin with one decision more (the direction); tests and clients that
  play from a default table `cw` first. Positions built by hand are unaffected.
* Reaction windows now depend on the engine's idea of "aimed at": new actions
  must record their aim in `noteAim`.
* A person whose reaction is passed for them can still use the same ability as
  an unlimited action on their own turn where it has one (Devolve, the Oath).
* The Auto-Play Timeout acts while nobody is looking: it costs a query per
  twenty seconds, and the bot may play a bad turn — it is a convenience, off by
  default.
