# Working on bingle, for an agent

Bingle is one static Go binary (`CGO_ENABLED=0`) that serves abstract board games over a browser
UI, a REST/WebSocket API and MCP, with an embedded SQLite database and seven direct dependencies.
Keeping it small is a feature. [CONTRIBUTING.md](CONTRIBUTING.md) covers building and testing;
this file is the working agreement.

## Secrets and host paths

Credentials and identities come from the environment, and are referred to **only** by the name of
the variable that holds them. **Never write a token, a key, an account name, a key's path or a path
on the maintainer's machine into a file in this repository** — not in a comment, a test fixture, a
commit message or a pull request. If you need one of these and the variable is unset, say so and
stop; do not guess, and do not read it out of a dotfile into anything you commit.

| Variable | What it holds |
|---|---|
| `BINGLE_GH_USER` | the account agent work acts as: its pushes, pull requests and comments |
| `BINGLE_GH_TOKEN` | that account's token |
| `BINGLE_GH_REMOTE` | the repository's HTTPS URL, for pushing |
| `BINGLE_BROWSER` | path to Chrome or Chromium for the browser tests, if it is not on `PATH` |

Repository administration uses a separate maintainer-held credential that agent work never needs
and should never look for.

## Branches, pushes and merges

- Work on a branch and open a pull request. **Never merge.** The maintainer merges.
- **Push over HTTPS as the account in `$BINGLE_GH_USER`**, never with a key that authenticates as
  the maintainer:

  ```sh
  git -c credential.helper='!f() { echo "username=$BINGLE_GH_USER"; echo "password=$BINGLE_GH_TOKEN"; }; f' \
      push "$BINGLE_GH_REMOTE" <branch>
  ```

  The `main` ruleset requires the most recent push to be approved by someone who did not make it,
  so a push attributed to the maintainer leaves them unable to approve their own push and the PR
  needs a force merge. The credential helper also keeps the token out of the process list.
- **Sign every commit** — the ruleset requires it, and git is configured for it locally.
- `main` additionally requires a squash merge, linear history, one approval, and **every review
  thread resolved**: an unresolved line comment blocks the merge even with an approval.

## Issues

- **An issue assigned to the account in `$BINGLE_GH_USER` is yours to act on. An unassigned one is
  not**, however clear it looks.
- One issue, one branch, one pull request. Put `Fixes #N` in the PR body so the merge closes it.
- Comment a short plan first on anything that is not a plain bug fix, and wait.
- **Say so rather than build something wrong.** If the request conflicts with
  [docs/decisions/](docs/decisions/README.md) or a plan in [docs/history/](docs/history/README.md),
  or an ambiguity would change the work materially, comment and label `needs-decision`. No PR.
- Label `blocked` when it waits on another issue, PR or an unimplemented plan.
- Label `no-changelog` (or honour it) when the change is documentation or CI only: then skip the
  changelog entry and the version bump.
- The maintainer also keeps `bingle_tasks.md` (untracked) for themed batches of work. Issues are
  for small standalone items, and an issue wins where the two disagree.

## Before pushing

- `make test` and `make lint` pass. `make test` is everything, including the Cthulhu Wars browser
  tests, which are off by default — see [docs/test.md](docs/test.md) for the groups, the shards and
  `make test-cw-ui`. Run the Cthulhu Wars UI tests when touching that game or the interface it
  shares with the rest.
- A test covers the change. For a bug, one that fails before the fix.
- `CHANGELOG.md` gets an entry, written for the people who play rather than for developers. The
  newest entry's version is `internal/version`. Bump `internal/version` for engine or platform
  changes and each game's JSON `version` separately; minor or patch by judgement, major only when
  told.
- `docs/` updated, and an ADR in `docs/decisions/` when the change settles a question rather than
  just adding a mechanism.
- Push back, in the PR or the issue, on anything that hurts performance, simplicity, security or
  normal UI practice — before building it.

## The shape of the code

- `internal/engine` — `dsl` (the JSON game definition), `catalog` (compiles definitions, variants
  by merge patch, derived games), `state` (the engine), `board`, `cards`.
- `internal/session` — room locks, the event hub, clocks, presence, direct messages, notifications.
- `internal/api`, `internal/mcpserver` — two front doors over the same session layer.
- `internal/web/static/js` — plain ES modules, **no build step**.
- `internal/games` — 15 games are a JSON definition and nothing else; 10 have a Go `state.Ruleset`.
  **`Legal` and `Apply` take no `context`, do no I/O, and must stay pure and deterministic.**
- `docs/history/` holds the plans, several still marked *planning in progress*; `docs/decisions/`
  holds the ADRs. Read the relevant one before changing an area it covers.
