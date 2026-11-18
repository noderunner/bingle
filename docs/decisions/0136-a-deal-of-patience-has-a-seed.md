# 0136. A deal of patience has a seed, a deal played again is not scored, and an account keeps a history of its deals

Status: accepted (2026-11-18), for issue #218. It builds on [ADR 0122](0122-a-patience-vocabulary.md) (the patience
vocabulary) and [0135](0135-patience-is-scored-by-its-moves.md) (the score and the quick-win badges).

## Context

A game of patience was already dealt from a random 64-bit seed, but the seed was not kept: only the
position it dealt was. The maintainer asked for a seed a player can copy and give to a new game to play the
same deal again, and for a history of each account's deals with their seeds and scores. A deal a player has
already solved makes the best score and the quick-win badges easy to farm.

## Decision

- **A game of patience records its seed** in `games.seed`, as a decimal number. A new deal draws one from
  1 to 2³² − 1, short enough to copy by hand. A player may give any whole number from 0 to 2⁶⁴ − 1
  (`POST /games`: `seed`; MCP `create_game`: `seed`). The same seed deals the same cards in the same order
  (`state.DealtFrom`). A seed with a start position, or
  for a game that is not dealt from one, is refused (`session.ErrInvalidSeed`, 400). `GET /game-defs/{id}`
  says `seeded` for a game that takes a seed.
- **The seed is shown from the start** (`GET /games/{code}`: `seed`). It does say where every face-down card
  lies, but nobody can work that out from a number while playing. The maintainer settled this: a first draft
  hid the seed until the game was over.
- **A deal the player chose keeps no best score and earns no quick-win badge** (a badge with `moves`).
  It is played and counted like any other game, for wins, games played and every other badge.
  `games.start_chosen` records the choice, and `GET /games/{code}` sends it as `start_chosen`. It is set for
  a chosen seed and for a start position (`start_state`, `start_tps`), which could carry a known deal too.
  The maintainer chose this over scoring a chosen deal, or not counting it at all.
- **The page** offers *Deal* in the setup of a game that takes a seed. It is empty for a new deal, and
  `/?game=solitaire&seed=N` fills it. The players card shows *Seed N* as a badge that copies it, and a
  finished game offers *Play this deal again*.
- **A signed-in player has a deal history** for each variant: the setup's *Deal History* tab, offered for a
  game that takes a seed, from `GET /me/deals?game_def_id=` (a guest gets `403`). It lists the account's
  finished games of that variant that have a seed, newest first, at most 200. Each row has the seed, a score
  (the moves for a win, which is ADR 0135's score, and *Not won* otherwise) and when it ended. A chosen deal
  is marked, since it kept no best score. Clicking a seed fills in *Deal*. The list is read from `games`, so
  retention limits how far back it goes. `scores` outlives it, but has wins only.

## Consequences

- A game created before this change has no seed, and shows none.
- A deal is tied to the shuffle in `cards.RNG` and the deal order in `patienceDeal`. Changing either changes
  what every seed deals, so it needs a new ADR and a note in the changelog.
