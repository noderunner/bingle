# Plan: a code and architecture audit

Status: **planning in progress** (opened 2026-10-28). Initial thoughts written before the
design discussion, not settled decisions. Nothing is implemented.

(The request listed "would require authenticated personal accounts" against this item; I have
read that as a copy-and-paste from the achievements item, since an audit needs no accounts. Say
if it meant something else.)

## What was asked for

- Another audit pass across the JavaScript, the Go and the JSON.
- Evaluate for simplicity, readability and maintainability.
- Consider changes to the architecture that better fit how bingle has actually evolved.

## Where the code is, measured

| Area | Lines (excluding tests) |
|---|---|
| `internal/games` | 30,048 |
| `internal/engine` | 9,543 (state 4,992 · catalog 2,097 · dsl 962 · board 889 · cards 404) |
| `internal/session` | 7,869 |
| `internal/api` | 3,039 |
| `internal/mcpserver` | 1,968 |
| `internal/store` | 1,812 |
| everything else | ~6,800 (batch, envs, bots, mail, config, tune, puzzles, httpserve, app, web, wire) |
| **Go total** | **61,083**, plus 51,587 lines of tests |
| Front end | 17,495 lines of JavaScript across ~25 ES modules, 1,825 of CSS, no build step |

Two numbers stand out. **Half the Go is games**, and 17,628 of that is Cthulhu Wars — so "the
platform" is really about 31,000 lines. And **`game.js` is 5,239 lines**, three times the next
largest front-end module.

## Initial thinking

**Measure before opining.** The first deliverable is not a refactor, it is a findings document
ranked by risk times churn: a file-size and function-length histogram, per-package fan-in and
fan-out, dead code and unused exports (`staticcheck`'s unused passes plus the `deadcode` tool),
duplication across the ten game packages, and the slowest and flakiest tests. Facts first,
opinions second, patches third.

**The candidates I would expect it to find, stated now so the audit can confirm or refute them:**

1. **`internal/session` has become several packages wearing one coat.** Room locks and the hub
   are the core; presence, direct messages, call claims, voice, notifications and clocks have
   each grown into it. Splitting the ones with no business in a room lock behind narrow
   interfaces is the clearest structural win, and it is also what makes the capability checks
   from the auth plan land in obvious places.
2. **`game.js` at 5,239 lines.** Board interaction, seat strips, panels, history, dialogs and
   keyboard handling in one module. It can be split along those seams while keeping plain ES
   modules and no build step — the constraint stays.
3. **Duplication across the ten game packages.** Each has grown its own `view.go`,
   `notation.go`, `data.go` and heuristics with similar shapes. A shared "game kit" for the
   repeated parts would shrink every one of them, and the DSL coverage audit may delete some
   outright — so these two plans must be sequenced, not run at once.
4. **Two front doors over one session layer.** `internal/api` (3,039) and `internal/mcpserver`
   (1,968) expose overlapping capability. Worth checking whether the MCP layer can be a thin
   projection of the REST handlers' logic rather than a parallel implementation.
5. **Definition files.** 25 JSON definitions, several with repeated blocks that generators or
   merge-patch variants should express once. Cthulhu Wars' definition is 64 KB.
6. **The test suite.** 51,587 lines of tests and a ~6.5-minute browser suite. There is real
   value in a pass over what is slow, what is duplicated, and what is flaky — the recurring
   browser-test flakiness has a known shape and deserves to be fixed systemically rather than
   per-test.
7. **Error and logging consistency** will be touched by the observability plan anyway, so the
   audit should defer to it rather than invent a second convention.

**Method: many small themed pull requests, each green, never one big one.** A platform that
works is worth more than a platform that is elegant, and the browser suite is the safety net —
anything that cannot be covered by a test does not get refactored on faith. Behaviour-preserving
is the rule; where the audit finds a *behaviour* bug, that becomes its own fix with its own test.

**On timing.** The observability and auth implementations both touch every package — structured
logging, middleware, a capability check at each call site. Auditing and refactoring in parallel
with those would mean conflicts and wasted review. But the *measurement* pass is cheap and
independent, and its findings would usefully inform both. So: measure early, refactor after.

## Open questions for the discussion

1. Is a JavaScript build step permanently off the table? I assume yes, and it shapes every
   front-end option.
2. How much appetite is there for moving internal packages around (import-path churn across
   61,000 lines) versus only splitting files within packages?
3. Does Cthulhu Wars leave the public repository under the plugin plan? That halves the games
   tree and changes what the audit should even read.
4. Do we want target metrics (no file over N lines, no function over M) or just a findings list?
   I lean findings-only; mechanical limits produce mechanical code.
5. Should the audit cover the documentation set as well? There are now nine plan documents and
   62 ADRs, and the auth plan already requires a sweep of the ones that say "there are no
   accounts".

## Dependencies and cross-plan effects

- **After the observability implementation** (logging conventions touch everything) and **after
  the auth implementation** (capability checks touch every handler). Measuring can happen now.
- **Coupled to the DSL coverage audit**: converting a game to a declaration deletes the code
  this audit would otherwise tidy. Convert first, tidy second.
- **Coupled to the plugin plan**: a module boundary clean enough for a plugin to plug into is
  the same boundary this audit wants. If plugins are coming, the audit should shape the seams to
  suit them.
- Nothing here depends on accounts.

## Status log

- 2026-10-28: opened with initial thoughts, including measured sizes and seven predicted
  findings. Recommendation: a cheap measurement pass now, the refactoring after the
  observability and auth implementations, in many small themed pull requests. Planning in
  progress.
