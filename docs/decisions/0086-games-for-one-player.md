# 0086. Games for one player

Status: accepted (2026-11-02). Its "every move can be taken back" is narrowed by [ADR 0114](0114-impulses-selections-moves-and-gestures.md): a move that
shows the player a card they had not seen stands, as in every other game.

## Context

Solitaire is the first game with one seat. Everything before it assumed somebody else: a game waited in
`waiting` until a second player sat down, a result was a winner or a draw, and a move that could not come back
waited for Submit because it handed something over to an opponent. A game of patience has no opponent, and very
often no result worth the name.

## Decision

**A game with one seat starts as it is created.** It goes straight to `active`, with no lobby and no opponent to
choose. Nobody else can sit down; anyone with the link can watch.

**Every move can be taken back.** Back (ADR 0083) is about what the turn hands on and what chance has shown an
opponent, and neither applies here: the turn never passes, so a player can go back to the deal. That includes a
move that turned over a card, which a stricter game would forbid. Nothing is rated, so there is nothing to protect,
and it is what every computer solitaire does. So the ruleset is not a `state.UndoGuard`, even though the cards
are hidden. (Since ADR 0114 it is one: a move that turns a card up or draws from the stock stands.)

**A move never waits for Submit.** There is nothing to look at twice before handing it over, and a game of
Klondike would otherwise ask at every card turned up.

**Only clearing the table is a win.** A game with one seat has nobody to lose to, so getting stuck, giving up and
finishing a game of bowling all end with no winner, and the end reason says which (`stuck`, `resignation`,
`score`). The store's "no winner" also means a draw for other games, so the page does not say so when there is one
seat: it says what happened, with the end detail, *12 of 52 cards home* or *143 points*.

**It cannot be rated, clocked or played in a tournament,** which the existing rules already say of a game that does
not have two seats. A rating needs an opponent, and a chess clock needs two sides.

**One ruleset, many games.** Klondike, Spider, FreeCell and Bowling Solitaire are variants of one family, each
naming its rules with the param `mode`, so that they share a place in the list of games, the themed sets (ADR 0085)
and the code for dealing, redacting and writing moves. Bowling has little in common with the rest except the deck,
and is its own file.

**Red and black are the suits' places.** The rules read a card's colour from its suit's place in `suit_order`:
hearts, spades, diamonds, clubs alternate. A set wears the suits by place too (ADR 0085), so the rules do not know
about sets. But a set's suits are not red and black, so where the rule depends on it, the page adds the classic suit
to the corner of a themed card, and the rule stays readable on a phone, which has no tooltips.

**Bowling Solitaire's rules are from secondary sources.** BoardGameGeek's page of the rules could not be read when
this was written. The rules follow Sackson's game as described by Wikipedia and by the summaries of the Eagle-Gryphon
rulebook, and they agree. Where they say nothing, the game's rules list its interpretations: three pins touch in a
chain, not necessarily all at once; the limits on the first play apply to the first play made on a rack, whichever
ball that falls on; and after a tenth-frame strike the next rack plays its own two balls.

## Consequences

A second game for one player needs nothing from the platform. The hint comes from `GuideMoves`, a ranking of the
legal moves, not from search: `WorthSearching` is false, because playing patience out at random says nothing about
a position. It suggests a sensible move and can miss a clever one. A list of past games shows a game with no winner
as it shows a draw for any other game.
