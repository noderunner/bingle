# 0016 — Position document with a compatible legacy view

## Context

v1 clients (and the v1 test suite) read `board_state`: a flat JSON object
`{position: {"Piece","Color"}}`. Stacks and reserves do not fit in it.

## Decision

* The **persisted and preferred** representation is a versioned document
  `{"v":2,"cells":{pos:[{piece,color}…]},"reserves":{color:{pool:n}},"ply":n}`
  (`state` in API responses and events). `BoardState.UnmarshalJSON` also reads
  the v1 flat shape, so old database rows still load.
* Every API response and event that carried `board_state` keeps carrying it as
  the **top-of-stack view** (`BoardState.LegacyJSON`), and gains `state`.
* New fields are additive everywhere (`winner_seat`, `end_reason`,
  `notation`, …); no existing field changed meaning.

## Consequences

* The whole v1 test suite kept passing unmodified at the API level.
* There are two views of one position; new code should read `state`.
* Database columns were added in place (`ALTER TABLE … ADD COLUMN`, guarded by
  `PRAGMA table_info`) so existing `bingle.db` files upgrade on open.
