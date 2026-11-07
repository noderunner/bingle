# 0125. A binary's games are chosen when it is built

Status: accepted (2026-11-07). Step 2 of the game plugins plan (`docs/history/game-plugins-plan.md` on
`more-plans`), whose first decision keeps Go games compiled in, never loaded at run time.

## Context

`internal/app` built the server's list of games by naming every game package: the one place a game had to
be added, and the reason every binary had every game. The plugins plan moves the commercial games into a
private overlay, which a public build must be able to leave out, so which games a binary has has to be
decided without editing the server.

## Decision

- **A game family registers itself** with `registry.Register(name, func() []catalog.Module)`
  (`internal/games/registry`), under a name of its own (`tak`, `classics`, `cthulhu-wars`). Registering
  twice panics. The function is only called when the modules are wanted, so registration costs nothing.
- **The registration is a file of package `all`**, one per game package (`internal/games/all/tak.go`),
  not an `init` in the game's own package. A package that imports a game for another reason, as
  `internal/batch` imports Cthulhu Wars, therefore never registers it by accident.
- **Each file has a build tag that leaves it out**: `//go:build !no_<package>`. `go build -tags
  no_cthulhuwars` builds a server without Cthulhu Wars, and `-tags no_tak,no_classics` one without
  Tak and the classics. The server imports `all`, and `app.BuiltinModules` is the registry's modules,
  family by family in the order of their names.
- The private overlay (plugins plan, step 5) will register its games the same way, from a file of its own.

## Consequences

- Adding a game is a package and one file in `internal/games/all`. Nothing in `internal/app` names a game.
- `tests/registry` checks that the default build registers every family, and that a tag leaves a game's
  package out of the build while the server still builds.
- **Not yet true for Cthulhu Wars**: `internal/batch` (practice batches) and `internal/tune` import it for
  its practice report and bot tuning. A `no_cthulhuwars` build leaves the game out of the server's list,
  but still compiles its package. The plan's step 3 has a game describe its own practice report, which
  removes those imports.

## Addendum (2026-11-07)

Since [ADR 0126](0126-a-game-describes-its-own-practice.md) practice batches and tuning name no game, so a
`no_cthulhuwars` build compiles none of Cthulhu Wars, and `tests/registry` checks it.
