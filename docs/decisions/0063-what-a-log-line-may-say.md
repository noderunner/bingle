# 0063. What a log line may say

Status: accepted (2026-10-28)

## Context

Stage 1 of [the observability plan](../history/observability-plan.md) moves bingle onto
`log/slog` and adds a line per request. That is the first time the server writes down what
people are doing with it, and bingle is an unusual place to start doing that:

- **There is no authentication anywhere.** A room code is the only thing between a stranger and
  a game (ADR 0005), and identity is a name a player types (ADR 0006).
- **A room code travels in the path.** `/api/games/AB12CD/chat`, `/game/AB12CD`,
  `/ws/AB12CD`. The obvious access-log line — the one every framework writes by default — is a
  list of keys to live games.
- **Browsers hold secrets of their own.** The "who is here" box and direct messages are
  authorised by a secret the browser generates and an address derived from it (ADR 0058,
  ADR 0059), and both travel in request bodies.
- **The log outlives the game.** Retention deletes games nobody has touched; a log file or a
  collector keeps whatever it was given, for as long as it was configured to.

So the question this settles is not "how do we log" but **what a line is allowed to contain** —
before stages 2 to 7 add metric labels, a slow-request journal, repro bundles and a debug mode
that mirrors every move.

## Decision

**A log line never carries anything that would let its reader join a game, read a
conversation, or identify a person.** Concretely, and at every level including `debug`:

| Never, at any level | Instead |
|---|---|
| the path a request came to | chi's route pattern, `/api/games/{code}/chat` (`logging.Route`) |
| a room code | its first two characters and twelve hex characters of its SHA-256 (`logging.Room`) |
| a guest key, a session cookie or an API token | nothing; these have no reference form (a test checks the keys, ADR 0067) |
| the body of a chat message or a direct message | nothing |
| an email address | twelve hex characters of the SHA-256 of the lower-cased address (`logging.Addr`) |
| a WebRTC offer, answer or candidate | nothing |
| a player's name | omitted; hashed (`logging.Name`) only where the line would say nothing without one |
| a credential from the configuration | `config.Secret`, which prints `[redacted]` through every formatter |

**The helpers are the policy.** `internal/logging` owns `Room`, `Addr`, `Name` and `Route`, and
they are the only sanctioned way for any of those things to reach a line. The hash is twelve hex
characters of SHA-256 — too little to reverse by hand, and enough that two lines about the same
room or the same person match. Room codes are upper-cased and addresses lower-cased first, so
one room is one reference however the code reached the line.

**Two characters of a room code, not none.** A reference nobody can connect to a room they are
looking at is much less useful when a player writes in to say something went wrong in `AB12CD`.
Two characters narrow a log to a handful of rooms and are no use for guessing the other four.

**The default level says nothing per request.** A request is logged at `debug`; at `info` only a
5xx is. A server left at the default writes a line when it starts and then only when something
is wrong, which is also what keeps a verbose log a deliberate act rather than the normal state.

**It is a test, not a rule in a document.** `tests/app/logging_test.go` runs a real server at
`debug`, plays a game, talks in the room, sends a direct message, says a browser is here, offers
an email address and opens a socket, then fails if the log holds the room code, either secret,
either message, the address, the browser's address or a player's name. A future line that leaks
one of them fails CI rather than being noticed a year later.

## Consequences

- **Stage 2's metric labels inherit this.** The plan's label whitelist — route patterns, game
  families, statement names, pool names, status classes, and never room codes, names, addresses
  or positions — is this decision applied to a second channel, and it gets a series-count ceiling
  test for the same reason this gets a redaction test.
- **Debugging a single room needs a deliberate switch.** `BINGLE_DEBUG_ROOM` (Stage 4) is how an
  operator gets full detail about one room, and `BINGLE_DEBUG_SECRETS` is how they get positions
  in full. Both are development-only by this ADR's logic, and the configuration should say so.
- **Repro bundles (Stage 7) are the exception, deliberately.** A bundle is full fidelity — that
  is the point of it — so it goes to a file in `BINGLE_DEBUG_DIR` and never to the log stream,
  which may be shipped somewhere else. Only its id is logged.
- **A 404 says very little.** An unmatched request logs the pattern it fell through to (`/api/*`)
  and not what was asked for, so a typo in a client is harder to diagnose from the log alone.
  That is the trade; the request id and the response are still there.
- **Hashes are not anonymity.** A known address or a known name can be confirmed against its hash
  by anyone who can compute SHA-256. These references hide a value from a reader who does not
  already have it; they are not a promise that a determined reader with a guess learns nothing.
