# The plans, and how they depend on each other

Status: **planning in progress** (opened 2026-10-28). A map of the open plans, written so the
order of work can be chosen deliberately. Provisional: it changes as each plan is finished.

## The plans

| Plan | Status | Needs accounts |
|---|---|---|
| [observability-plan.md](observability-plan.md) | **settled**, ready to implement | no |
| [auth-and-accounts-plan.md](auth-and-accounts-plan.md) | **settled**, ready to implement | — |
| [dsl-yaml-plan.md](dsl-yaml-plan.md) | **settled**, ready to implement | no |
| [game-tutorials-plan.md](game-tutorials-plan.md) | **settled**, ready to implement | no (guests welcome) |
| [dsl-coverage-audit-plan.md](dsl-coverage-audit-plan.md) | planning in progress | no |
| [patience-vocabulary-plan.md](patience-vocabulary-plan.md) | planning in progress (addition F of the DSL audit) | no |
| [game-plugins-plan.md](game-plugins-plan.md) | questions settled, not started | only for packs |
| [real-time-play-plan.md](real-time-play-plan.md) | **settled**, ready to implement | no |
| [tournaments-and-rankings-plan.md](tournaments-and-rankings-plan.md) | **settled**, ready to implement; the next major feature | yes |
| [achievements-plan.md](achievements-plan.md) | planning in progress | yes |
| [code-and-architecture-audit-plan.md](code-and-architecture-audit-plan.md) | planning in progress | no |

## How they constrain each other

```
  observability ──────────────┐
    (logging conventions)     ├──► code & architecture audit   (refactor after both sweeps;
  auth & accounts ────────────┘                                 measure any time)
    │
    ├──► plugins ──────── install_plugins, admin UI, reauth
    ├──► tournaments ──┐
    └──► achievements ─┴── one shared per-account statistics table, written at game end
                           (retention deletes games: never recompute from history)

  real-time play ──► tournaments   (clock fixes for all; connection and the setting for real-time)

  YAML ──► plugins        (one authoring format for manifest + definition)
   │  └──► tutorials, achievements   (hand-authored documents want comments)
   └──► DSL coverage audit (add new vocabulary once, in the final format)

  DSL coverage audit ──► plugins     (every fully-declared game is installable as data)
                     └─► code audit  (a converted game deletes code the audit would tidy)

  tutorials ──► tournaments, achievements   (the on-ramp that makes them worth having;
                                             tutorials themselves earn no badges)
```

The couplings that would cost the most if missed:

1. **Tournaments and achievements both want one durable statistics table** written when a game
   ends. Build it once. Tournaments land first, so they own it.
2. **Retention deletes inactive games.** Ratings, badges and counters must be written at event
   time, not derived later. This breaks both plans silently if forgotten.
3. **The YAML format change is cheap now and expensive after the DSL audit adds vocabulary.**
4. **The code audit should not run against code the DSL audit is about to delete**, nor during
   the auth and observability sweeps, which touch every package.
5. **Plugins need the DSL audit to be worth much**, because a fully-declared game installs as
   data with no sandbox. Since 2026-10-31 that is the *only* way to install a game: Go code is
   always compiled in.
6. **Cthulhu Wars leaves the public repository** (plugins, decided 2026-10-31). The code audit
   should not tidy code that is about to move, and should shape the game registry and the
   presentation seams the plugin plan needs.

## A provisional order, for discussion

**To finish planning**, roughly cheapest and most unblocking first:

1. **YAML** — one or two decisions, and it unblocks authoring for tutorials, badges and plugins.
2. **Tutorials** — small, self-contained, needs no accounts, and the highest ratio of player
   value to new machinery.
3. **Plugins** — settled 2026-10-31: data-only packs, Go games compiled in, Cthulhu Wars
   to a private build.
4. **DSL coverage audit** — mostly a question of what the goal is, and the answer depends on
   whether plugins are the point.
5. **Tournaments and achievements together** — they share a table, and the prize-pool question
   needs a decision either way.
6. **Code and architecture audit** last, because its scope depends on the outcome of 3 and 4.

**To implement**, the order I would argue for:

1. **Observability** — it is settled, it is independent, and it is what makes everything after
   it debuggable.
2. **Auth and accounts** — settled, and three other plans are blocked on it.
3. **YAML** — small, and every later authored document benefits.
4. **Tutorials** — visible value, no dependency on accounts.
5. **DSL coverage audit**, then the conversions it recommends.
6. **Plugins**, which by then install mostly-declared games.
7. **Achievements**, then **tournaments** on the same statistics table.
8. **The code audit's refactoring**, with its measurement pass run much earlier for information.

## Status log

- 2026-10-28: written alongside the seven new plan drafts, to support choosing an order.
- 2026-10-31: plugins settled; noted that Cthulhu Wars leaves the public repository, which
  the code audit must account for.
- 2026-10-31: YAML settled; plugins extended to move every commercial game private.
- 2026-11-01: tutorials settled; YAML, plugins (questions) and retirement done or settled.
- 2026-11-01: tournaments settled and chosen as the next major feature, ahead of the provisional
  order; real-time play split out as a plan of its own, which they depend on. Tournaments now land
  before achievements and own the statistics table. The rest of the provisional order is unchanged.
