# 0119. A seeded deal at the start

Status: accepted (2026-11-06). It is addition D of the DSL coverage audit (`docs/history/dsl-coverage-audit-plan.md`
on `more-plans`), after additions A to C ([ADR 0115](0115-a-declared-turn-of-several-selections.md),
[0116](0116-effects-at-the-end-of-a-turn.md), [0117](0117-groups-by-colour-enemies-by-seat.md)).

## Context

The audit found that **nothing in a declared game is random**. The engine is seeded and deterministic
(`InitialStateSeeded`), and a definition can already describe cards (`cards`: suits, decks, zones, ADR 0043), but only a
Go ruleset can put them anywhere: `internal/engine/cards` shuffles, deals and redacts a `Table` of zones for Nyet! and
Solitaire, and Onitama keeps its five Move cards in its own data.

The audit's addition D covers three things: a shuffle and deal into zones **at setup**, dealing and drawing **during
play**, and **dice pools** counting hits at a threshold. They are not equally ready:

- **Onitama** needs only the first: five of sixteen cards, two to each player and one to the side, face up, before the
  first move. Everything after that is moving cards between zones, which is addition E's.
- **Solitaire**'s draws from the stock and its redeals belong to the patience vocabulary (addition F, a plan of its
  own), which will say how a draw is played.
- **Dice** unlock no game in the catalog: the dice games, Courtfall above all, stay in Go. Chance during play also ends
  an impulse (ADR 0114), so a declared draw or roll has to say where the impulse ends, which is best decided with the
  first game that needs one.

Additions earn their place by the games they unlock. This ADR takes the part a game is waiting for.

## Decision

### Cards in a declared game

A declared game may declare `cards` as a ruleset game does: its decks and its zones, with their `scope`, `visibility`
and `layout`. Its position then holds a table of cards: each zone's cards in order, and whether each is face up. It is
the same `cards.Table` the rulesets keep, stored in the position document as `cards`.

### The deal

A top-level **`deal`** lists what happens to the cards before the first move, in order:

```yaml
deal:
  - {shuffle: deck}                         # the deck's cards, in a seeded random order
  - {from: deck, to: hand, each: 2, face: up}
  - {from: deck, to: side, count: 1, face: up}
```

| field | meaning |
|---|---|
| `shuffle` | a zone of the table: fill it with every card of the game's deck and shuffle it |
| `from`, `to` | zones; `to` may be a seat's zone (`scope: seat`) |
| `each` | for a seat zone: that many cards to each seat, one at a time in seat order, as `cards.Deal` does |
| `count` | for a table zone: that many cards |
| `face` | `up` or `down` (default `down`) |

- **The shuffle is seeded** by the game's seed, through `cards.RNG` and `Zone.Shuffle`, so the same seed deals the same
  cards on every server, and a replay or a differential test sees the same deal.
- A deal that asks for more cards than `from` holds is refused when the game is compiled, since the deck is known then.
- A game with several decks names the one a shuffle uses: `{shuffle: deck, deck: main}`.

### Who sees what

A declared game's cards follow their zones' `visibility`, as a ruleset's do: what the server sends to a player is the
table **redacted** for that seat (`Table.Redact`), and a bot that plans works on a **determinized** copy, the hidden
cards dealt again at random from those it cannot see. A game whose zones all show their faces (Onitama) has no secrets,
and plays as any open game does.

### Not now

- **Dealing or drawing during play**, and the impulse it ends, waits for addition F or the first game that needs it.
- **Dice** wait for a game in the catalog that would become declarable with them.
- **Using a card**, and what a card says, is addition E.

## Consequences

- With E, Onitama's cards are declared: its sixteen cards as a deck, three zones (each seat's hand, the side), and this
  deal. To pass the differential harness, Onitama's ruleset first changes to deal through `cards.RNG` and
  `Zone.Shuffle`, which changes which cards a given seed deals for new games only; a stored game keeps the cards it
  was dealt.
- The schema, `docs/dsl.md`, the compiler (an unknown zone, `each` on a table zone or `count` on a seat zone, a deal
  bigger than the deck), the engine's start, the position document (its `cards` table), redaction and determinization
  for declared games, and the session's card views change together.
- Declared games without `deal` or `cards` play as now; the harness holds every shipped game to it.
