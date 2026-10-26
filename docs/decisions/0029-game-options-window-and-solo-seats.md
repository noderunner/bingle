# 0029. The game options window and games against oneself

## Context

The game page had grown a column of cards — the game's name and link, the
players, the moves, what happened, the key, the board's tools and settings —
and a bar of reaction settings above everything. The playtests said that most
of it is rarely needed, and that the map and the cult panels should have the
room. People also wanted to play a table alone to try a cult or a position out,
which the platform, built on one player per seat, did not offer.

## Decision

**One window for the settings.** The bar at the top of the page has *Rules* and
*Options*. *Options* opens a floating, non-modal window (`js/options.js`) that
holds the reactions, the map layers, the display settings, the tools, the
invite link and the game-ending buttons. It is a child of `body`, not of the
area the page redraws on every move, so a toggle is never lost to a redraw (the
lesson of ADR 0027's reactions bar, which it keeps: sections are built once and
updated in place by signature). The side column keeps the cult panels (or the
seat strips), the chat and *one* card with two tabs, *Moves* and *History*. The
legend of the board lives in the rules window as its *Key* tab, built from the
definition's `display` for every ruleset game.

**A game against oneself is several players.** `also_seats` seats the creator in
more seats, each a player of its own, named `<name> · <seat label>`. Nothing
else in the platform changes: every seat has a player to move for it, moves
carry the name of the seat to move, bots are unaffected, and the lobby, undo
(which treats one's own aliases as one's own) and resignation work as before.
The browser recognises the aliases by the prefix and acts for the seat to move
(`S.mine`, `syncMe`), so one tab plays the whole table. Identity stays
name-only (ADR 0006).

## Consequences

* A new setting goes into a section of the options window; nothing is added to
  the side column.
* The `players` table needed no change (names are unique per game, which the
  alias names respect).
* A person cannot tell apart, in the game's own data, a friend named
  "Alice · Black Goat" from Alice's second seat: both are people at a table.
