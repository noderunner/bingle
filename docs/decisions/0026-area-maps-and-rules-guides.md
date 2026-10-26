# 0026 — Area maps, more than two seats and rules guides

## Context

Cthulhu Wars does not fit the board shapes bingle had: its world is a set of
irregular *areas* (continents and oceans, one of which wraps around the edge
of the map) with several units of several players in each, played by **four**
cults. Its rules are also long — far longer than the paragraph-and-bullets
"Rules" dialog could hold: tables of units and spellbooks, steps, examples,
pictures.

## Decision

**A board style `areas`.** Positions carry `polygons` (one or more outlines
as `[x, y]` corner lists on a `canvas`), an `anchor` for their name and
tokens, a display `name` and `tags` (`ocean`, `land`, `glyph-dragon`…).
Adjacency is still the graph the definition declares (`edges`), so movement,
planes and the ruleset see an ordinary board; polygons exist to be drawn.
Several polygons per position let an ocean wrap off both edges. The catalog
validates canvas and outlines. `GET /game-defs/{id}` returns `board.canvas`
and `board.areas`; the client draws straight-edged polygons, area names, Gate
and Desecration marks, an outline for the battle and one token per
(cult, unit kind) with a count.

**Tokens are clickable.** A token click stands for the `piece` (and `pos`)
steps of the move being built, found by searching the builder's options
(`tokenSteps`), so moves still come from `steps` and no game logic is in the
client. Forced choices (a single enemy, a single area) are taken
automatically.

**Rulesets may have any number of seats.** Nothing in the engine assumed two,
but the surrounding product did: a *Play as* control built from the game's
seats, bots that can be added to the open seats of a waiting table
(`POST /games/{code}/bots`, MCP `add_bot`), a table layout with a panel per
seat, no draw offers or take-back requests between more than two parties and
"End game" instead of "Resign".

**A rules guide is a set of Markdown pages.** A definition may list
`guide.tabs` (`id`, `title`, `file`); the module embeds the files
(`catalog.Module.Assets`, an `fs.FS`). The server returns them from
`GET /game-defs/{id}/guide`, serves images from `/game-defs/{id}/assets/*`
(inert: `sandbox` CSP, `nosniff`, path-checked) and generates a guide from the
definition for games without one, so every game gets the same window. The
virtual file `board.svg` draws an area map. The client's renderer builds DOM
nodes (never `innerHTML`) and supports headings, emphasis, lists, tables with
alignment, callouts, collapsible sections, columns, code, keycaps, images
(relative to the game's assets, or `piece:`/`seat:` icons) and links
(`#section`, `tab:id#section`). The window has tabs with roving focus, a table
of contents, search across pages and remembers the last tab. MCP agents read
the same pages with `get_game_guide`.

## Consequences

* Games with irregular maps (Risk-like, wargames) are a JSON file away; the
  rules text that goes with them is Markdown next to the definition.
* The rules window of every game is the same component; classic games get
  Overview / Pieces / Notation / Controls tabs without writing anything.
* The renderer is deliberately small and not a full CommonMark: raw HTML is
  not supported, which is what makes it safe to display pages from a module.

## Addendum: layouts, bots and big screens

* Cthulhu Wars' four board layouts are variants of one definition; their
  polygons and borders come from `scripts/cwmaps.py`, which vectorises
  reference pictures with shared, once-simplified borders. A layout names the
  cults' start areas in `params.starts`.
* `session.avoidShuffles` (optional `state.RepetitionGuard` on a ruleset)
  and background bot play (`runBotsFor`, `continueBots`) keep bot tables from
  idling in loops or holding a request open for minutes.
* `state.BotGuide` (optional, on a ruleset) lets a game narrow a bot's choice
  to its sensible moves (`GuideMoves`): the session applies it to every bot's
  decision and to hints, the environments to their opponent and to
  `match_bots`, and the search bots to their playouts and inner nodes.
  Cthulhu Wars encodes its strategy tips there (`guide.go`).
* Tokens are always clickable: a legal pick is taken, any unit opens its card
  (`view.seats[n].units`), and units enhanced by an earned spellbook are
  marked; a position keeps a short event log (`ext.log`) so a page can show
  what happened since the viewer's last move.
* The game page is no longer a fixed 1200 px column: a wide area map and its
  side column use the window, and the board's size follows its height.
