# Working on bingle, for an agent

Bingle is one static Go binary (`CGO_ENABLED=0`) that serves abstract board games over a browser
UI, a REST/WebSocket API and MCP, with an embedded SQLite database and eleven direct dependencies.
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
| `BINGLE_GH_APP_ID` | the client ID of the repository's GitHub App, which agent work acts as |
| `BINGLE_GH_APP_KEY` | the path to that app's private key |
| `BINGLE_GH_REMOTE` | the repository's HTTPS URL |
| `BINGLE_BROWSER` | path to Chrome or Chromium for the browser tests, if it is not on `PATH` |

Repository administration uses a separate maintainer-held credential that agent work never needs
and should never look for.

## Acting on GitHub

**Agent work acts on GitHub only as the repository's GitHub App**, through `scripts/ghapp`. Never
through a personal account: not the maintainer's, and not a second account made for an agent.
GitHub's terms allow one person a single free machine account, and a second account whose pull
requests its owner approves looks like inauthentic activity; the account first used for this was
suspended. The app is GitHub's supported way to run a bot, and its pushes are its own.

`scripts/ghapp` mints an installation token for an hour at a time and never prints it except for
`token`, which only a credential helper should call. `api` sends the path exactly as given: it
does **not** fill in `{owner}/{repo}` (that would 404), so take the repository from
`$BINGLE_GH_REMOTE` and write it out:

```sh
R=${BINGLE_GH_REMOTE#https://github.com/}; R=${R%.git}           # owner/repo
go run ./scripts/ghapp api GET "/repos/$R/pulls?state=open"      # any REST call, as the app
go run ./scripts/ghapp api POST "/repos/$R/pulls" < pr.json
go run ./scripts/ghapp api GET "/repos/$R/commits/<sha>/check-runs"   # a pull request's CI
go run ./scripts/ghapp push <branch>                              # see below
git -c credential.helper='!f() { echo username=x-access-token; echo "password=$(go run ./scripts/ghapp token)"; }; f' \
    fetch "$BINGLE_GH_REMOTE" <branch>
```

## Branches, pushes and merges

- Work on a branch and open a pull request. **Never merge.** The maintainer merges.
- **Commit locally as usual, then push with `go run ./scripts/ghapp push <branch>`, never with
  `git push`.** `main` takes only signed commits, and unsigned commits on a pull request's branch
  block its squash merge. An app cannot sign what it pushes with git: GitHub signs only the commits
  an app makes through the API. So `push` recreates each local commit since `main` with GraphQL's
  `createCommitOnBranch`, which GitHub signs as the app, checks that each has the local commit's
  tree, and prints the two commands that fetch the branch and move the local one onto GitHub's
  commits. Run them, so the next push continues from there.
- **Do not sign commits with anybody's key**, and do not author them as the maintainer: the commits
  on GitHub are the app's, and the local ones are only their source.
- After a rebase or an amend, `push -force` rebuilds the branch on GitHub from `main`, on a branch of its own that it then moves the pull request's branch to: a pull request whose branch points at `main`, even for a moment, has no commits and is closed. A branch
  must be a straight line of commits on `main` (no merge commits), and `push` refuses a commit that
  adds an executable or a link or changes a file's mode, since the API sends contents only: say so
  and leave that one to the maintainer.
- `main` requires a squash merge, linear history, one approval of the most recent push by someone
  who did not make it, and **every review thread resolved**: an unresolved line comment blocks the
  merge even with an approval. The app makes the pushes, so the maintainer's approval counts.

## Issues

- **An issue labelled `agent` is yours to act on. Any other is not**, however clear it looks.
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

- `make test` and `make lint` pass — see [docs/test.md](docs/test.md) for the groups and the
  shards. With the private overlay checked out at `internal/games/private` (ADR 0127), `make
  test-private` runs the suite with its games too: do so when touching a shared interface they use.
- A test covers the change. For a bug, one that fails before the fix.
- `CHANGELOG.md` gets an entry, written for the people who play rather than for developers. The
  newest entry's version is `internal/version`. Bump `internal/version` for engine or platform
  changes and each game's definition `version` separately; minor or patch by judgement, major
  only when told. A prototype's `version` stays as it is: its rules change in its folder, and a new
  version folder is made only when the maintainer asks for one (`docs/dsl.md`).
- `docs/` updated, and an ADR in `docs/decisions/` when the change settles a question rather than
  just adding a mechanism.
- Push back, in the PR or the issue, on anything that hurts performance, simplicity, security or
  normal UI practice — before building it.

## Words

[ADR 0114](docs/decisions/0114-impulses-selections-moves-and-gestures.md) defines these; use them the same way in
code, docs, API paths and names.

- An **impulse** is the choices one seat makes before the game needs something from elsewhere: another seat's
  choice (a reaction included), something the seat can't know yet (a roll, a draw, a card turned up), or the end of
  the game. It is submitted in one call and applied as one, whatever the game calls it. The **impulse seat** is the
  seat whose impulse the game is waiting for, and the **impulse card** shows it (ADR 0099).
- A reaction window always ends the acting seat's impulse; a seat's settings only decide whether its player or the
  session answers for it.
- A **selection** is one self-contained choice within an impulse, with its own notation: a stone, one unit's move. A
  **move** is the selections of one impulse, a **pending move** one not yet submitted, and a **ply** one impulse.
- A **gesture** is one thing a player does in an interface: a click, a tap, a drag, a key, Submit or Back. The rules
  never depend on gestures.
- *Step* is none of these: it keeps the DSL's `step` action, a tutorial's steps and a training environment's step.
- In Go, `state.Selection` is one selection, `state.Move` the selections of one impulse and `state.Gesture` one
  gesture; the API's `selections` query lists selections with their `gestures`.

A game's own rules and its players' words keep their terms.

## The shape of the code

- `internal/engine` — `dsl` (the YAML game definition, loaded as JSON), `catalog` (compiles
  definitions, variants by merge patch, derived games), `state` (the engine), `board`, `cards`.
- `internal/session` — room locks, the event hub, clocks, presence, direct messages, notifications.
- `internal/api`, `internal/mcpserver` — two front doors over the same session layer.
- `internal/web/static/js` — plain ES modules, **no build step**.
- `internal/games` — 15 games are a definition and nothing else; 12 have a Go `state.Ruleset`.
  **`Legal` and `Apply` take no `context`, do no I/O, and must stay pure and deterministic.**
- `docs/history/` holds the plans, several still marked *planning in progress*; `docs/decisions/`
  holds the ADRs. Read the relevant one before changing an area it covers.
