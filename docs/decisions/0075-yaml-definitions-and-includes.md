# 0075. Game definitions are written in YAML, with includes, and loaded as JSON

Status: accepted (2026-10-31). Supersedes [ADR 0002](0002-json-over-yaml-for-dsl.md).

## Context

[ADR 0002](0002-json-over-yaml-for-dsl.md) chose JSON for game definitions and said that if
hand-authoring became painful, "a thin YAML→JSON preprocessing step could be added later without
changing the engine". It has become painful: a definition cannot carry a comment, the quoting and
commas are noise, and Cthulhu Wars' definition had grown to 10,580 lines in one file. A misspelled
field was also silently ignored, because `dsl.Parse` used `json.Unmarshal`.

The plan is `docs/history/dsl-yaml-plan.md` (on the plans branch, `more-plans`), settled
2026-10-31. Two facts made it cheap: the accounts work already made `go.yaml.in/yaml/v3` a
direct dependency, and variants are JSON merge patches, which a conversion keeps working untouched.

## Decision

- **YAML is only for authoring.** `dsl.Load` turns a `.yaml` definition into JSON as it is read,
  keeping its keys in the order written; nothing after it changes. Variants, the catalog,
  `/api/game-defs/{id}`, game state, moves and the MCP tools stay JSON, so no client needs a YAML
  parser. A `.json` definition still loads, unchanged.
- **Every shipped definition is YAML**, converted in one step and checked to produce exactly the
  JSON it replaced, key order included.
- **`!include path`** replaces any value with another file, relative to the including file. An
  include never leaves the game's own files (no absolute paths, no `..` out, no symbolic links), may
  not form a cycle and nests at most 16 deep; the files read are capped at 16 MB and the expanded
  document at 64 MB. These matter for data packs, which are untrusted. A large definition lives in a
  `definition/` folder: a short main file and its parts (board, pieces, options, one file per
  variant). A JSON definition cannot include.
- **Anchors, aliases and merge keys** (`<<`) are allowed; the loader expands them itself, within
  the same output cap.
- **Unknown fields are refused**, in either format. `dsl.Load` names the file, line and column of
  the field, inside included files and variant patches too, and matches names exactly; `dsl.Parse`
  refuses them as well (`DisallowUnknownFields`), for definitions that arrive another way.
- **A JSON Schema is generated** from the Go types, field comments as descriptions, into
  `docs/game-definition.schema.json` by `go run ./scripts/genschema`, by a package of our own
  rather than a twelfth dependency. A test fails while it is out of date. Each top-level definition
  names it in a `# yaml-language-server: $schema=` comment, so editors complete and check fields.
- **`bingle dsl convert`** turns a JSON definition into YAML and a YAML one into the JSON it loads
  as. It needs no configuration or database.
- The rule for every future format choice: **documents people write by hand are YAML** (game
  definitions, data-pack manifests, tutorial scripts, badge definitions, the capability file);
  **what machines exchange is JSON**.

## Consequences

- A definition can be commented, split and checked as it is typed; a typo is an error with a line
  number instead of a rule that quietly does not apply.
- The definitions' git history restarts at the conversion; the old history is reachable under the
  deleted `.json` paths.
- A number written `248.0` and loaded through the JSON form comes back `248`: equal, but a tool
  that rewrites a definition from its JSON (`scripts/cwmaps.py`) will drop the `.0`.
- An editor's YAML language server checks the value at an `!include` as the file name it is, and
  may flag that line; the setting `"yaml.customTags": ["!include scalar"]` stops it refusing the
  tag itself.
- ADR 0002 is superseded; its reasons for JSON on the wire still hold and are kept.
