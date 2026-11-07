# Plan: YAML for game definitions

Status: **planning in progress** (opened 2026-10-28). Initial thoughts written before the
design discussion, not settled decisions. Nothing is implemented.

## What was asked for

- Make defined games friendlier to author by moving the DSL to YAML.
- Consider the costs of switching away from JSON.
- If the costs warrant it, consider supporting both, or translating YAML to JSON — but the hope
  is to avoid needing that.

## What exists today

- `dsl.GameDefinition` is a Go struct with `json` tags — about 670 lines of struct across 30-odd
  types — parsed by `internal/engine/dsl/parse.go` (180 lines) and compiled by `catalog`.
- 25 definitions ship as embedded JSON, 15 of them with no Go code at all.
- **Variants are JSON merge patches** (`VariantDef.Patch`), and derived games are produced by
  patching a parent definition. This is the detail that decides the whole question.
- JSON is also the format of board state, moves, position documents, puzzles, batch reports and
  the whole API. The DSL being JSON is not an accident, but it is also not the same decision.

## Initial thinking

Starting with the honest version of the costs, since that is what was asked:

- **A new dependency.** The repository has seven direct dependencies on purpose. `yaml.v3` is
  the obvious choice but is effectively feature-frozen upstream; `goccy/go-yaml` is actively
  developed; `sigs.k8s.io/yaml` is interesting because it converts YAML to JSON and then uses
  `encoding/json`, so the existing `json` tags keep working unchanged. Worth re-checking each
  at implementation time. Note the auth plan **already proposes a YAML dependency** for the
  capability defaults file, so this may be one dependency serving two features.
- **Merge patches.** Variants are JSON merge patch today. A YAML-to-JSON conversion keeps that
  working untouched; a native YAML parse means reimplementing patching over YAML nodes. This is
  the strongest technical argument for conversion over replacement.
- **Anything that writes a definition** — a future builder UI, the derived-game patcher, tests
  that assert on definitions — would need to emit YAML too, unless JSON stays the internal form.
- **Anchors and aliases** are a genuine authoring win for repetitive piece, region and
  direction blocks, but they do not round-trip to JSON and they are an expansion-bomb surface,
  so a size cap and an alias limit are needed.
- **Tooling** is a wash and arguably a win: JSON has schema completion in editors, but a
  generated JSON Schema works for YAML in the same editors, and `yaml.v3`-family parse errors
  carry line and column numbers, which beats JSON's byte offsets for an author.

And the two wins that I think actually settle it: **comments** — a definition cannot be
commented today, which is a real cost for a 600-line game file — and **no quoting or trailing
comma noise**.

**So my initial recommendation is the thing the request hoped to avoid, for a reason I think is
good: YAML as the authoring surface, JSON as the internal form.** Load either extension,
convert YAML to JSON at read time, and leave `parse.go`, merge patching, every test and every
tool untouched. One format per game — never both for the same game — so there is still exactly
one truth. Add a `bingle dsl convert` command for going either way, and generate a JSON Schema
from the Go structs, which is worth doing whatever we decide because it helps authors today.

What I would **not** do is change the wire format, board state, or position documents. Those
are machine-to-machine and JSON is correct there.

## Open questions for the discussion

1. Do the 25 shipped definitions get converted to YAML (a large diff, and git history for the
   definitions effectively restarts), or does YAML exist only for new and hand-authored games?
2. Is "YAML in, JSON inside" acceptable, given the hope was to avoid a translation step? My
   case is that the translation is one library call and it buys us untouched variants and tests.
3. Which library, and does the auth plan's capability file use the same one? It should.
4. Shall we generate a JSON Schema regardless, as a smaller independent improvement?
5. Does anything else move to YAML — tutorial documents, badge definitions, plugin manifests?
   They are all hand-authored, so probably yes, and that strengthens the dependency's case.

## Dependencies and cross-plan effects

- **Auth plan**: already proposes a YAML capability file. Same dependency, one decision.
- **Plugin plan**: a plugin's manifest and definition want the same format, and a single
  authoring format makes a plugin a nicer file to hand-write.
- **DSL coverage audit**: that audit will *add* vocabulary to the DSL. Doing the format change
  first means new fields are authored once; doing the audit first means a bigger conversion.
  Worth sequencing deliberately — my instinct is the format change is cheap and should go first.
- **Tutorials and achievements**: both want hand-authored documents with comments.

## Status log

- 2026-10-28: opened with initial thoughts. Costs enumerated honestly; recommendation is YAML as
  the authoring surface with JSON as the internal form, chiefly to keep JSON merge patch
  variants and the entire test suite untouched. Planning in progress.
