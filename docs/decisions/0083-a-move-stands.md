# 0083. A move stands once the turn has passed

Status: accepted (2026-11-02). Replaces the undo of ADR 0018 and the take-backs of ADR 0021, and the parts
of ADRs 0079, 0080 and 0082 that dealt with taking moves back. The steps of a turn it stores and takes back, and
`move-final`, go with the first phase of [ADR 0114](0114-impulses-selections-moves-and-gestures.md).

## Context

A game could show a *Back* button, an *Undo my move* with a *Really undo?* after it, and an *Ask to take
back* in its menu. The maintainer settled the rule: once a move has handed the turn to anybody, reactions
included, or brought in chance (a die, a draw, a shuffle), it stands, in every game: casual, rated or in a
tournament. Back is for a move that has not counted yet.

## Decision

- **Take-backs are gone**, from the session, the API, MCP and the page. So is undoing a finished game, a
  bot's reply or a pie-rule swap, which removes everything that had to rewind clocks and seats.
- **One thing can be taken back: the last step of the turn still being played.** It must still be that
  player's turn, and the step must not have rolled, drawn or been decided on seeing a roll (`moves.locked`,
  `CanTakeBack`). `POST /games/{code}/undo` and `undo_move` keep their names and now do only this. The rule
  is one rule, so rated and tournament games have it too: a stone of two taken back cannot change a result,
  as the game has not moved on and cannot have ended.
- **Eleven games send each click to the server**, because their rules are in Go and a turn can be several
  steps. So a step does reach the store before the turn ends. The page keeps those steps out of the move
  list and the History tab until the turn is over, and Back takes them back one at a time. The API's move list still has them,
  as everything that replays a game reads it.
- **The move that cannot come back waits for Submit, in every game.** `POST /games/{code}/move-final` says
  whether a move would hand the turn on, end the game or bring in chance. It tries the move on the real
  position and returns only that yes or no, which the rules already show. A step that can come back is
  sent at once. *Confirm moves* turns Submit off, for a player who would rather be quick. A tutorial's
  script has already said what to play, so it asks for no Submit.
- **A button is its own Submit.** *End turn*, *Done* or *Battle* says what it does, so pressing it plays it.
  What waits is a move finished by clicking the board, where a slip is easy.
- **One Back button** (`#undo-step`, Backspace): it clears a selection first, then a step already sent, and
  needs no second click, since nothing it takes back has counted.

## Consequences

- A misclick that was not caught at Submit stands, even against a bot or alone. *Analyze* is the place to
  try things out.
- "The turn has passed" means another seat is to act. In a game that gives one seat several turns running
  (the others are out of Power, or have no move) those turns stay open until somebody else acts: nobody has
  seen them answered, and the engine does not tell a new turn from another step. A roll still locks them.
- A reaction passed automatically asked nobody, so it does not end the turn; taking the step back removes it.
- Time spent is not refunded, and a step earns no increment, so taking steps back earns nothing.
- The `takeback_seat` and `takeback_seq` columns stay, unused, as dropping them gains nothing.
- ADR 0080 limited `stats` to rated games because an ordinary game could be undone after it ended. That
  reason has gone, so `stats` could count every game if achievements want it.
