# 0127. Commercial games are built from a private overlay

Status: accepted (2026-11-07). Step 5 of the game plugins plan ([docs/history/game-plugins-plan.md](../history/game-plugins-plan.md)), whose decisions moved the commercial games out of the public repository. It builds on
ADR 0124 (a game's look and page scripts are its own), 0125 (a binary's games are chosen when it is
built) and 0126 (a game describes its own practice).

## Context

Bingle's public repository holds public-domain games, openly licensed games, games whose designers
allow implementations with credit, and the maintainer's own. A commercial game is implemented for a
private build only. Cthulhu Wars, the largest of them, was compiled into the public binary, and its
code, data, art and interface were in the repository. The earlier steps left nothing public naming
it except its own package, its tests and its docs. Go code cannot be loaded at run time (the plugins
plan's first decision), and the game imports `internal/...` packages, which a separate module may not.

## Decision

- **The private overlay is a repository of its own** (`bingle-private`), checked out
  inside a bingle clone at `internal/games/private`, which the public repository ignores. It is not
  a module: its packages are bingle's (`github.com/noderunner/bingle/internal/games/private/...`),
  so they import the engine like any game.
- **It is compiled in by the build tag `private`.** `internal/games/all/private.go`, the only public
  file that names it, imports the overlay's root package under that tag. The root package registers
  its games as `internal/games/all` does, each from a file with a `no_<package>` tag. Without the tag
  nothing of the overlay is compiled, checked out or not, and `tests/registry` checks it.
- **Cthulhu Wars moves into it whole**: its package (rules, definition, guide, tutorials, page
  scripts, study, tuned weights), its tests, the docs of its implementation and interface, and the
  scripts that built its maps.
- **Its tests run against the public helpers.** The browser tests share `tests/app`'s, so the overlay
  keeps them in `tests/_app` (a folder Go ignores) and `make test-private` copies them into `tests/app`
  as `private_*_test.go`, which the public repository ignores, while the suite runs with `-tags
  private`. Every private test file carries the tag, so an untagged run never compiles them.
- **The overlay has its own CI**, which checks out the public repository around it and runs `make
  test-private`. Public CI sees none of it, and needs none: the public suite tests every shared
  feature through public games and the fixture games (`tests/testdata`: Council, Tricks and Table).
- **Its look has not moved yet**: Cthulhu Wars still carries its look in its definition, and the
  public test of ADR 0124's line skips it.

## Consequences

- The public binary and repository have no Cthulhu Wars. A private build is `go build -tags private`
  in a clone with the overlay checked out; a server that ran it before keeps its games, which the
  private build reads as before.
- `make test-cw-ui`, `$BINGLE_TEST_CW_UI` and the skip that made Cthulhu Wars' browser tests opt-in
  are gone: the overlay's CI runs them every time.
- A game that is to become private follows the same path: its package and tests into the overlay,
  a registration file there, and what only it exercised covered first by a public game or a fixture.
- Git history still holds Cthulhu Wars as it was: rewriting `main` was not wanted.

## Follow-up (2026-11-08)

Yinsh, Onitama, Hive and Nyet!, the rest of the plugins plan's private list, followed the same
path: their packages, rules tests and browser tests into the overlay, each registered from its own
file there (`no_yinsh`, `no_onitama`, `no_hive`, `no_nyet`). What only they exercised was covered
first: the parts of the page only one of them drew became its page script (#173), and the shared
tests play the Tricks and Table fixtures instead (#174). The diagrams of Onitama's and Yinsh's
guides are drawn by the overlay's own script, which uses the public drawing helpers.

