# 0107. Reserved seats, friends-only games and invite-only games

Status: accepted (2026-11-04); changes one sentence of [ADR 0058](0058-all-games-who-is-here-and-the-icons.md)

## Context

The maintainer asked that a game's setup can keep seats for friends, that a game with all its seats
kept becomes "friends only" and leaves the public lists, that a friend's such game is listed for its
creator's friends, and that a game can be "invite only", on no list at all.

ADR 0058 said "a game that should not be found is one nobody should be able to join by code either".
That no longer holds. A game can now be unlisted and still be opened by its link, which is what a
link is for.

## Decision

- **Lists and seats are what is restricted, not the game.** Anybody with the room code can open any
  game and watch it (the maintainer's choice). A list shows less, and a kept seat takes only its
  friend. Nothing here makes a game's moves or chat private, and the setup does not say it does.
- **A seat is kept for one friend of the creator**, who must be signed in. Nobody else can sit there,
  or move to it, and a bot goes there only when the host asks for that seat, which gives it up. The
  friend also gets an invitation in their friends list.
- **A game with a kept seat always gathers in a lobby**, even for two players, so that the host can
  *Release* a seat if a friend does not come.
- **"Friends only" is worked out, not stored.** It is true while a game has kept seats and no open
  seat that anybody could take. Giving up a seat makes the game public, as the seat is then for
  anybody. So nothing can fall out of step with the seats.
- **Friends of the creator see a friends-only game** (the maintainer's choice), and so does anybody a
  seat is kept for. One with no seat kept for them is offered to watch.
- **Invite only** is stored, chosen at setup and permanent. Such a game is on no list. It takes
  precedence over the rest, and an invitation or the link is the way in. A guest can choose it too:
  it asks nothing of an account.
- **The server decides** what a list shows, for `GET /games` as for the page, and a seat's count is
  of the seats this viewer could sit at.

## Consequences

- `GET /games` now scans up to 1000 games on every request, not the first few, since some are not
  for this viewer.
