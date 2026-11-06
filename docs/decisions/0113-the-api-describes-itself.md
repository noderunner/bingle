# 0113. The API says where to start, and serves its own docs

Status: accepted (2026-11-05)

## Context

A client with curl and no checkout found nothing at `/api`: a plain-text `404 page not found`, the
same for every path the API lacked, though the docs promise JSON errors. `docs/api.md` existed only
in the repository. Once a client knew `GET /api/game-defs` the games explained themselves (rules,
`notation_help`, the legal moves), but the first step had to be read from the source.

## Decision

- **`GET /api` is an index**: the version, where the docs, MCP and the WebSocket are, four steps to a
  first move, and the fifteen or so endpoints needed to find and play a game. It is a short list
  kept by hand, not every route; a test checks that each one is a route and is in the docs.
- **`GET /api/docs` serves `docs/api.md`**, built into the binary as `CHANGELOG.md` is, as plain text
  so a browser shows it rather than saving it. The help window links to it.
- **Under `/api`, an unknown path is `404` and a wrong method `405`**, both JSON that point at `GET /api`.
- **The walkthrough at the top of `docs/api.md` plays Hex**: it is the simplest game with a real
  decision on every move (the maintainer's choice). A test plays the same steps. A walkthrough for a
  game with hidden information is added when a simple one is built; Cthulhu Wars is too large to
  teach the API with.
- **No OpenAPI document.** It would be a second description of some 150 routes to keep true by hand,
  or a generator and its dependencies; the index and the served docs give most of the benefit.

## Consequences

A route named in the index cannot be renamed without the index, the docs and a test noticing. The
served docs are whatever `docs/api.md` said when the binary was built, so they match the server they
come from.
