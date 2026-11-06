# 0112. In a game with secrets, legal moves go to whoever holds the seat

Status: accepted (2026-11-05)

## Context

`GET /games/{code}/moves/legal` listed the moves of the seat to move to anybody with the room code,
with no credentials. In Nyet! those moves are the cards in that seat's hand, which the position
document has always hidden. The API-only play test led to this. MCP's `get_legal_moves` and
`GET /games/{code}/legal-moves?from=` read the same position.

## Decision

- **In a game whose ruleset implements `HiddenInfo`, a seat's moves go only to whoever holds it.**
  Anybody else gets `200` with the seat to move and an empty list: a request that may not see a
  seat's private view is shown what a spectator sees, as the position already does.
- **The check lives in the session layer**, so REST and MCP cannot differ.
- **A seat nobody owns hides nothing.** It is acted for by name, by anybody (ADR 0067), so refusing to
  list its moves would protect nothing.
- **Asking only reads.** It does not use `mayView`, which binds a seat nobody owns to whoever asks: a
  stranger's GET must not take a seat.
- **Declarative games stay public**, for analysis and spectators.

`simulate` is unchanged: it works on a position determinized for the public, so the moves it lists
come from a sampled hand and not the real one.

## Consequences

`HiddenInfo` is broader than games with real secrets: Hive, Onitama and others implement it only to
attach a view. Their spectators lose a list no page of ours asks them for. That is the price of a
rule a new game with secrets cannot forget to opt in to.
