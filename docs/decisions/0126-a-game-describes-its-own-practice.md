# 0126. A game describes its own practice

Status: accepted (2026-11-07). Part of the game plugins plan's step 3 ([docs/history/game-plugins-plan.md](../history/game-plugins-plan.md)): nothing outside a game names it. It generalises the practice batches of
[ADR 0034](0034-practice-batches-and-metrics.md) and the guide tuning built on them.

## Context

Practice batches play bot-versus-bot games in memory and report on them. Any game could be batched, but
everything beyond the outcome was Cthulhu Wars': `internal/batch` imported it for its cults, its observer, its
per-cult report and its guide tuning, and `internal/tune` searched Cthulhu Wars' `GuideTuning` for a cult. So a
build without Cthulhu Wars (ADR 0125) still compiled it, and it could not leave the public repository without
taking the batches with it. The maintainer chose to keep batches public and make them general (2026-11-07).

## Decision

- **A game family may register a `batch.Study`** with `batch.RegisterStudy`, from the same file of
  `internal/games/all` that registers the game, so a build that leaves the game out leaves its study out too.
  A study says:
  - how a table is played: a pick per seat (Cthulhu Wars' cults) and options (`hp`), as a game id;
  - an observer for each game, whose measurements are the record's opaque `metrics`;
  - the game's own part of the report, as top-level fields beside the common ones;
  - the knobs of its bots' guide, the shipped tuning of a pick, and the guide's moves under a tuning;
  - the measures a tuning search may count besides wins, with their weights and scales, and a pick's score in a
    game.
- **`internal/batch` and `internal/tune` name no game.** A tuning is a map of knob names to numbers, which for
  Cthulhu Wars are `GuideTuning`'s JSON names, so `tuned.json` keeps its shape. `tune` searches any study with
  knobs; its fitness is the win rate plus each measure's weight times its mean over its scale.
- **The JSON is unchanged where it was used**: a player's pick is still `faction`, a report's `factions` and
  `area_names` are where they were, and a spec saved with `high_priests` reads as the option `hp`.
- A game without a study is batched by its outcome, and refuses picks, options and tunings.

## Consequences

- With ADR 0125, a `no_cthulhuwars` build no longer compiles Cthulhu Wars at all, and the game can move into the
  private overlay with its study, while batches stay public.
- `scripts/tune` takes `-pick`, `-options`, `-weights name=w` and `-ship FILE` in place of `-faction`, `-hp`,
  `-doom-weight`, `-book-weight` and `-ship`.
- The batches page still draws a report per cult and a picker of cults: that is its Cthulhu Wars knowledge, which
  the page's own step (the web page stops naming games) moves into the game's web scripts.
