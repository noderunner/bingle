# 0002. JSON over YAML for the DSL

Status: Accepted

## Context
The game-definition DSL (see [0001](0001-json-dsl-with-go-escape-hatch.md)) needs a concrete serialization format. YAML is more pleasant to hand-author (comments, less punctuation); JSON is simpler to parse/validate with `encoding/json` and easier for tooling and AI agents to generate programmatically.

## Decision
Use JSON for game definitions.

## Consequences
- No YAML parsing dependency needed.
- Hand-authoring a game definition is slightly more verbose than YAML would be; if this becomes painful, a thin YAML→JSON preprocessing step could be added later without changing the engine.
