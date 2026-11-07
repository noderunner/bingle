# Plan: YAML for game definitions

Status: **settled**, ready to implement (opened 2026-10-28, settled 2026-10-31). The
sections before *Decisions* are the initial thinking, kept as written. Nothing is
implemented.

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

## Decisions (settled 2026-10-31)

Two facts changed since the initial thinking: the auth work already made `go.yaml.in/yaml/v3`
a direct dependency (`internal/auth/caps.go`), so YAML costs no new module; and Cthulhu Wars'
10,580-line definition is leaving the public repository (see the plugin plan), so the largest
public definition is a few hundred lines. These become an ADR superseding
[0002](../decisions/0002-json-over-yaml-for-dsl.md) when the work lands; 0002 itself foresaw
"a thin YAML→JSON preprocessing step".

1. **YAML is only for authoring.** A YAML definition is turned into JSON as it is loaded, and
   that is the only place YAML exists. `parse.go`, variant merge patches, the catalog,
   `/api/game-defs/{id}`, game state, moves and the MCP tools are unchanged and stay JSON.
   A client never needs a YAML parser.
2. **Every shipped definition is converted**, in one mechanical commit, with a test that each
   YAML file produces exactly the JSON of the file it replaces. Definition history restarts at
   that commit; the old history stays reachable under the deleted `.json` path. Conversion
   does not wait for the private-build work.
3. **Anchors and aliases are allowed** within a file. yaml.v3 refuses excessive aliasing;
   data packs add a size cap on the expanded document.
4. **Unknown fields are rejected**, in either format, naming the file and line. Today
   `dsl.Parse` silently ignores a misspelled field. Stray fields this uncovers in shipped
   definitions are fixed in the same change.
5. **A JSON Schema is generated from the Go structs** and checked in, so editors complete and
   check YAML through a `# yaml-language-server: $schema=...` comment. `google/jsonschema-go`
   is already in the module graph.
6. **Hand-written documents are YAML; machine-exchanged ones are JSON.** YAML: game
   definitions, data-pack manifests, tutorial scripts, badge definitions and the capability
   file (already). JSON: everything on the wire and in the database.
7. **`bingle dsl convert`** converts a definition either way.

## Includes: one game across several files

Decided 2026-10-31: a large definition should be split across files in a logical way, and
assembled with includes.

- **An `!include path` tag on any YAML node** replaces that node with the contents of the
  named file. A list may hold one include per item (`variants: [ !include variants/a.yaml,
  ... ]`).
- Paths are relative to the including file and resolved **inside the game's own file system**
  (its embedded directory, or a data pack's zip). Absolute paths, `..` out of that root and
  symlinks are refused; cycles are an error; nesting depth and the size of the assembled
  document are capped. This matters for data packs, which are untrusted.
- Includes are resolved **before** conversion to JSON, so everything after the loader still
  sees one document, and `/api/game-defs/{id}` serves the assembled result.
- Errors name the file and line where the problem is, including inside an included file.
- JSON definitions do not include; only YAML does.
- A suggested layout for a large game, a convention rather than a rule: `game.yaml` (identity,
  players, options, display), `board.yaml`, `pieces.yaml` and `variants/<name>.yaml`.

Which public definitions are worth splitting is decided during conversion; most are under 300
lines. The case that motivated it is Cthulhu Wars, whose definition is mostly map variants
(about 7,400 lines) and board (about 2,400): it is split in the private overlay, with the same
`convert` command and the same include mechanism.

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
- 2026-10-31: settled. YAML for authoring only, converted to JSON as it loads; every shipped
  definition converted; anchors allowed; unknown fields rejected; a generated JSON Schema;
  hand-written documents in YAML. Added `!include` for splitting large definitions across
  files. yaml.v3 is already a dependency through the auth work.
- 2026-10-31: implementation started on branch `dsl-yaml` (ADR 0075). Found while building:
  a number written `248.0` comes back `248` through the JSON form; unquoted dates are kept as
  the text written rather than turned into times.
