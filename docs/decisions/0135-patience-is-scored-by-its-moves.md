# 0135. A game of patience is scored by the moves it was won in

Status: accepted (2026-11-18). It builds on [ADR 0122](0122-a-patience-vocabulary.md) (the patience
vocabulary), [0114](0114-impulses-selections-moves-and-gestures.md) (a ply is one impulse), [0086](0086-games-for-one-player.md) (a
game for one seat takes moves back) and [0130](0130-badges.md) (badges).

## Context

A game of patience has one player and nobody to beat, so a win alone says little. The maintainer asked for a
score: count the moves as the game is played, keep the number a game was won in, make each player's best a
lookup, and give badges for a win in fewer moves than most.

## Decision

- **The score is the moves, and fewer is better.** A move is a ply: one impulse, whatever it does. Drawing,
  dealing a row, turning the waste over and Finish each count once. Finish sends everything home in one move,
  as the game already offers it. `state.ScoredByMoves` says which games keep this score: every game whose
  `win_condition` is `patience`. Bowling Solitaire has its own score, its pins, and does not keep this one.
- **A move taken back does not count.** The counter is the position's ply, and undo returns to an earlier
  position. A move that showed a card already stands (ADR 0086), so a take-back changes only what could
  already be seen. Counting undone moves would need a count kept outside the position, which nothing else needs.
- **The page shows the count** from `view.moves`. A win's detail says how many moves it took ("52 of 52
  cards home in 104 moves").
- **The score is kept by the results worker**, in `scores` (`game_id`, `account_id`, `game_def_id`,
  `score`), written with the game's counts in one transaction, so a game is scored once. A score is kept only
  for an account that won a game of people only, which a game of patience always is. A guest, a bot at
  the seat, a tutorial or a game with no move keeps none, as with the counts. The game is kept by id without
  a reference, since retention may delete it and the best should outlive it. `store.BestScore` is an
  account's fewest moves in one game, variants apart, using an index on (account, game, score).
- **A badge asks for a win within so many moves** with a new part, `moves: {at_most: N}`: the game ended
  within *N* plies, every seat's counted. It is known only at the end. It is general: in a game of two it
  counts both players' moves.
- **Solitaire's six badges** ask for a win in fewer moves than a typical win: 110 for Klondike, 100 for draw
  three, 110 for FreeCell (where every card home is a move), and 150, 200 and 300 for Spider with one, two
  and four suits. These are estimates. The game's own hint player wins too rarely to measure them: 4 of 300
  deals of Klondike, in 87 to 110 moves. They can be tuned once real games have been played. A badge's
  number can change without retiring it, since its awards keep the description they were given with.

## Consequences

- Nothing about the score is sent by the API yet beyond `view.moves` and the end detail. A page of best
  scores, or a leaderboard, would read `scores`.
- A game that wants a score where more is better needs a direction of its own, by ADR. This one has none.
