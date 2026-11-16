# 0134. The API has a version of its own, and a JSON Schema of its messages

Status: accepted (2026-11-16). Adds to [0113](0113-the-api-describes-itself.md).

## Context

[bingle-3d](https://github.com/noderunner/bingle-3d), the native client, plays through the API alone (ADR 0124) and
is built and released apart from the server, with hand-written Rust types for the parts it uses (issue #212). Two
things were missing for it:

- **It could not tell whether a server spoke the API it was built for.** `GET /api` reported the engine's version,
  which moves with every change to the engine or the platform, and so says nothing about a client. A mismatch showed
  up mid-game, as a missing field or a `400`.
- **The messages were described in prose only.** `docs/api.md` is for people; nothing could be checked against it.

ADR 0113 declined an OpenAPI document, a second description of some 150 routes kept true by hand or by a generator
with its dependencies. What a client needs is narrower: the shapes of the messages it reads and sends.

## Decision

- **`version.API`, a semantic version of the messages, separate from `version.Engine`**, starting at `1.0.0`: the
  API's stability is a question apart from the games' (every one of which is still a beta). `GET /api` reports it as
  `api_version` beside `version`, and every answer under `/api`, an error included, and the WebSocket handshake carry
  it in the `Bingle-API-Version` header, so a client can check any response.
  - *major*: a change that can break a client written against the last version: a field or endpoint removed or
    renamed, a field's type or meaning changed, a value a client must handle added to a closed set (a WebSocket event
    it must act on, a game status);
  - *minor*: what a client may ignore: a new endpoint, an optional field, an event it may skip;
  - *patch*: the schema's descriptions alone.

  Clients ignore the fields they do not know, which is what keeps an addition minor.
- **`docs/api.schema.json`, a JSON Schema (2020-12) of the messages, generated from the Go types** the handlers send
  and read, by the generator that writes the game definition's schema (`schemagen`, now able to read doc comments
  from every package of the repository), with each type's and field's doc comment as its description.
  `go run ./scripts/genschema` writes it, a test fails while it is stale, and the server serves the one it was built
  with at `GET /api/schema`, so a client reads the schema of the server it is talking to. It records the version it
  describes (`x-api-version`), so whoever regenerates it also decides whether the version moves.
- **It covers the index's endpoints and the game socket's events, not the routes.** `x-endpoints` maps each endpoint
  of `GET /api`'s index to its request and response, `x-error` is the error body, and `WebSocketEvent` is the
  envelope of `/ws/{code}`'s `joined`, `move`, `state`, `chat`, `connection`, `kick_vote`, `seat_replaced` and
  `slow_turn` (voice and `rtc` can wait). A test checks that every endpoint the index names is in it, and plays a game
  through them, checking each answer and event against it.
- **A response's object lists as `required` the fields it always carries** (null allowed where Go may send it), and
  lets unknown fields through. A request body requires only the fields the handler refuses to go without.
- **Game-specific content stays open.** A position (`state`, and a ruleset's `ext.view` in it) is any JSON value: its
  keys are named by meaning (ADR 0124), but each game has its own.
- **MCP does not share the version.** Its tools describe themselves to an agent at every connection; it gets a
  version of its own when a client needs one.
- The handlers and events already had Go types; those of the events, in `internal/session`, are exported for the
  generator. They stay where they are: `internal/wire` imports the session, so the events cannot move into it.

## Consequences

A change to a message's Go type fails a test until the schema is regenerated, and the schema's diff is what to judge
the version by. The schema describes only what has a Go type: an endpoint outside the index, or an event not listed,
is still described in `docs/api.md` alone, and is added when a client needs it. Without a validator as a dependency,
the test checks answers with a small one of its own that knows the parts of JSON Schema the generator writes.
