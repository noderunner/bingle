# Contributing to bingle

Thank you for helping. Bingle should be easy to download, build, test and change; if something
here does not work for you, that is a bug in the project, so please say so.

## Set up

```sh
git clone https://github.com/noderunner/bingle.git && cd bingle
make build         # bin/bingle (needs only Go: see go.mod for the version)
make test          # the whole suite
make lint          # gofmt, and golangci-lint if it is installed
```

The browser-driven tests (`tests/app`) need Chrome or Chromium on your `PATH`, or `BINGLE_BROWSER`
set to its path; without one they are skipped. `make test-short` skips the slowest tests.
[docs/test.md](docs/test.md) says what each group of tests covers.

## Making a change

1. **Open an issue first** for anything big (a new game, a change to the API or the database),
   so we can agree on the shape before you spend time on it.
2. **Write the test first** where you can: a failing test that shows the bug or the rule, then the
   change that makes it pass. Tests live in `tests/<package>/` and use only the exported API of the
   package they test. UI behaviour is tested in a real browser in `tests/app`.
3. **Keep to the shape of the code around you**: comments say *why*, not what, and the naming
   and layout of the neighbouring files is the style guide. `gofmt` and `make lint` must pass.
4. **Document it**: a user-visible change goes in the relevant file under `docs/`; a decision that
   was hard or that someone will ask about later gets an ADR in `docs/decisions/`
   (copy the newest one for the shape, and add it to the index).
5. **Version it**: change a game's definition or Go code and you raise its `version` in its JSON
   (patch for a fix, minor for a new rule or option; a game stays below 1.0.0 until its owner
   calls it finished). A change to the engine or platform raises `internal/version`.
6. **Say it in the changelog**: add what changed, for players, to the newest entry of
   [CHANGELOG.md](CHANGELOG.md), or start a new entry (`## 0.4.0 - 2026-10-26`, the engine's
   version) when you raise `internal/version`. The help window of the game shows that entry
   under *Recent changes*, so write it for the people who play.
7. Open a pull request that says what changed and why, and how you checked it.

## Adding a game

Most games are one JSON file: board, pieces, actions, win rules. Start from
`internal/games/classics/tictactoe.json` and read [docs/dsl.md](docs/dsl.md). Games with chance,
hidden information or rules the JSON cannot say are a Go `state.Ruleset` behind the same API
(ADR 0025; Courtfall and Hive are good examples). Every game needs:

- a definition with `version`, `designers`, `publisher` (when known) and a short `description`;
- a rules guide: one `rules.md` that teaches a newcomer the whole game, with diagrams where a
  picture helps (`scripts/guide_diagrams.py` draws the SVGs in the existing guides);
- registration in `internal/app/app.go`;
- a place in the cross-game test battery (`tests/allgames` picks up every registered game by itself).

## Conduct

Be kind and assume good faith. Disagree about the code, not the person.

By contributing you agree that your contribution is released under the project's [MIT license](LICENSE).
