# 0094. Cthulhu Wars' setup is a picker of single items, and saved setups are an account's

Status: accepted (2026-11-03)

## Context

Cthulhu Wars' setup had grown to ten cults in a list per seat and some thirty check boxes, most with a paragraph of text, and an option could only bring a whole pack. Players want to choose one Terror without the rest of its box, see what a thing does before adding it, and keep a table they like.

## Decision

- **Single items are options.** `neutral-units` and `neutral-books` (kind `multi-choice`) name Loyalty Cards and neutral spellbooks one by one. `gameCfg` holds them as masks beside the packs, and every rule that asked for a whole pack asks for the item instead (`cardInGame`, `bookInGame`, `elderThings`). The old options stay: the API, MCP, stored games and their ids mean what they did.
- **One table, one id.** The id says single items as hex masks in card order (`-nu210`, `-nb9`), which ADR 0093 already keeps stable. `extras.canonical` drops what a pack already brings and turns a pack named card by card into that pack, so the same table is never two games.
- **The picker is a client of the options.** `GET /game-defs/{id}/setup-catalog` describes each tile and which option chooses it; the page sends an ordinary create call. A ruleset offers this through `state.SetupCatalogs`, and the page itself is specific to Cthulhu Wars (`cw-setup.js`) until a second game wants one. Catalog text comes from the definition and the card tables, so it cannot drift from the rules.
- **Thumbnails are `board.svg`, 20 KB each,** not a layout's full description, 250 KB each: the Maps tab would otherwise fetch 1.5 MB to show six pictures.
- **Seats do not shift.** Taking a faction out leaves its seat open and the next one fills it, so who plays which seat holds. A game cannot be created with an open seat.
- **The recommended number is one fewer than the seats, counted over neutral Monsters, Terrors and Independents together.** The rule book only says this of Independents, for a first game; counting them together is a choice for a simple budget for the notice and the shuffle. It is advice and never blocks. Rules and spellbooks do not count.
- **More factions than seats is a pool for the lobby.** `faction-pool` goes in the id (`-fp<mask>`), as the id is a game's only record, and every seat still has a faction from the start, so nobody has to wait: anyone who doesn't choose plays the one their seat has. The lobby lists the pool alone, the server refuses others, and a player may change their own seat, which at any other table stays refused. The page turns the lobby on for a pool; the server does not insist, since without a lobby the pool just has no effect.
- **Saved setups belong to an account** (`setup_profiles`): a layout and the options of a create call, checked by `Derive` when saved, so one can always be started. Saving under a name replaces. A setup is private unless shared; guests see ready-made and shared ones. A shared one shows its owner's handle, an administrator can delete it, and an account may keep 30 per family, with the list capped at the newest 100 shared.

## Consequences

- The old `seat-choice` controls and the per-seat cult lists are gone from the home page. The option remains for other clients.
- Seat order follows the order factions were chosen. Trading two seats means taking both out and adding them back.
- A bot in a seat keeps that seat's faction, so seating bots at setup takes those factions out of the choice. A timer-book swap belongs to a faction at the table when the game is made, and is lost if that faction leaves it in the lobby.
- A saved setup can name a card that a later version removes: the page ignores what it does not know.
