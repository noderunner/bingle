# 0133. A game may end in a draw shared by some seats, or a loss for every seat

Status: accepted (2026-11-16).

## Context

A finished game had one winner (`winner_seat`) or none, and none meant a draw for every seat. That is right for two
players. In a game of more seats it cannot say two other things that happen:

- **A shared victory.** Cthulhu Wars: "If two or more players tie for the most Doom and both have all 6 of their
  Spellbooks, the game ends in a draw. These players rejoice in their shared victory!" The other cults lost.
- **Everyone loses.** Cthulhu Wars: "If the game ends and no player has all 6 Spellbooks, the players lose and humanity
  wins!"

Both were recorded as a draw for every seat. So a cult that lost a shared victory was counted as having drawn, and in a
humanity win every cult was counted as having drawn.

## Decision

- A ruleset's `state.Outcome` with no `Winner` may say more: `Drawn` lists the seats that share a draw (the others
  lost), and `AllLost` says that every seat lost. With neither, every seat drew, as before. A `Winner` wins whatever
  else is set.
- The game keeps it in one column, `games.shared_result`: `drew:1,3`, `lost`, or empty for any other game.
  `store.Game.DrawnSeats` and `AllLost` read it. `Game` stays a comparable struct.
- Each seat's result is `won`, `drawn` or `lost` from those three: in the counts and badges (`seatOutcome`), and in an
  AI client's observation (`result`, `reward`).
- The REST game, the game and move events and MCP's `get_game` carry `drawn_seats` and `all_lost`, omitted when empty.
  The page says "every player lost", or names the seats that share the draw ("you share the draw with …", or "…; you
  lost"). A game's page script may still word its own ending first (`drawnMessage`).
- Ratings and tournaments rate two seats only. There, every seat losing counts as a draw, since a rating update cannot
  score both players a loss.

## Consequences

- Rulesets that never set the new fields are unchanged.
- Finished games stored before this have an empty `shared_result` and keep their old result.

## Alternatives

- **A list of winners.** A shared victory could be several winners, but the rulebook itself calls it a draw. Several
  winners would also leave "everyone lost" with no way to be said.
- **A per-seat result list in the outcome.** That would be more general than any game needs now, and it would change
  every reader of `winner_seat`.
