# 0140. The private overlay's games are marked private

Status: accepted (2026-11-22). It builds on [ADR 0127](0127-commercial-games-in-a-private-overlay.md) (commercial
games are built from a private overlay) and [0134](0134-an-api-version-and-a-schema-of-its-messages.md) (the API's version).

## Context

A client built apart from the server, such as the native client, may have a public build that must not show
commercial games until it has a private build of its own, as the server does. It cannot tell which games those
are: it may not name them itself (ADR 0127), and `publisher` is no sign, since public games have publishers too.

## Decision

- **The overlay registers its families with `registry.RegisterPrivate`**, not `registry.Register`. Their modules
  carry `catalog.Module.Private`, and so do every game compiled from them: their variants and derived games.
- **`GET /api/game-defs` and `GET /api/game-defs/{id}` say `private: true`** for those games and leave the field out
  for every other. A public build marks nothing.
- **One flag, not a word for the game's origin** (`origin: "private"`). A client needs to know only whether to
  leave a game out; a kind of game that needs more can add its own field, a minor version as this one is (API 1.8.0).

## Consequences

- A client that requires API 1.8.0 can hide the private games from a public build of itself.
- The marker says where a game was built from, not what its licence is: a game moved between the repositories
  changes its mark with it.
