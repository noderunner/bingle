# 0106. An account can choose not to be seen online

Status: accepted (2026-11-04)

## Context

The maintainer asked that an account shows friends its presence by default, with a way to turn that
off for privacy. *Here now* lists every signed-in account to every other, so hiding from friends alone
would have hidden nothing.

## Decision

- **Off means offline everywhere** (the maintainer's choice): off the friends list's dots and off
  *Here now*. A change applies at once.
- **It must not be possible to tell hidden from gone.** A message or call to a hidden friend gets the
  same `404` as one to a friend who has gone, and the list of conversations names neither.
- **A hidden account can still speak first.** It can write and ring its friends, and whoever it has
  spoken to can answer, as long as the conversation is kept. Doing so does tell that friend it is
  online, which is the account's own choice. `can_write` in `GET /friends` says when the box is open
  though `online` is false.

## Consequences

- A person's seat in a game still shows whether they are connected ([ADR 0088](0088-connection-replaces-presence.md)),
  to anybody at that game. That is about the game, and a game they are playing in shows they are there.
