# 0105. Friends: requests, messages between friends, invitations

Status: accepted (2026-11-04); changes who may use direct messages under [ADR 0059](0059-direct-messages-and-their-calls.md) and [ADR 0073](0073-account-only-features.md)

## Context

The maintainer asked for a friends list to replace the direct messages window, for registered users
only: messages, invitations to games, removal, and mail about friend requests for those who want it.
The accounts plan had kept direct messages to accounts "to grow a friends list later".

## Decision

- **A friendship is a row between two accounts**, asked by one and accepted or declined by the other.
  Asking somebody who has already asked you accepts at once. Friends are found by handle, in the
  window or from *Here now* in the profile window.
- **Direct messages and direct calls are between friends** (the maintainer's choice). A stranger's
  message or call is a `403`, whether or not they are online, and that is decided first so that it
  says nothing of who is here.
- **Messages stay live** (the maintainer's choice): in memory only, and refused for a friend who is
  offline, as ADR 0059 has it. The window says so and turns off the box.
- **Invitations are stored**, unlike messages, because their point is to wait for a friend who is
  away. One per game and friend, from somebody with a seat in the game, listed until dismissed or
  the game is over. Accepting is opening the game.
- **A refusal holds for 30 days** against the same person asking again, and removing the row does
  not get round it. The one who refused may ask at any time. Without this, the mail below could be
  used to pester somebody. A request is told apart from a refusal only by this answer.
- **Mail about a request is off unless asked for** in the profile, goes only to a confirmed address,
  says only who asked, and counts towards the server's limits with at most five a day per address.
- **The `dm` capability gates all of it.** Friends need an account as well, even where an
  administrator has opened `dm` to guests.

## Consequences

- *Here now* offers *Add friend* where it offered *Message*.
- Existing conversations are not carried over: they were in memory and for two hours.
