# 0156. Blocking a player, silently

Status: accepted (2026-11-27), for issue #247. Builds on [ADR 0105](0105-friends.md) (friends),
[0106](0106-presence-can-be-hidden.md) (hidden presence) and [0109](0109-scheduled-games.md) (musters).

## Context

The friends list had no way to stop somebody. A declined request only held off another from the same person for 30
days, and an invitation to a muster can come from anybody by handle. The maintainer settled it on the issue: a block
stops everything the issue named, it is silent, global and game chat are not blocked, blocks are listed and undone in
the profile beside the friend settings, and nobody else can see them yet (an administrator's page may come later).

## Decision

- **A block is a row (blocker, blocked)**, made by handle from the friends window: on a friend, a request or an
  invitation. It also does what *Remove friend* does: a friendship, or the blocker's own request, goes. Any call or
  ring between the two ends as if the blocker had hung up or declined.
- **It stops** friend requests, invitations to games (kept seats included) and to musters, direct messages and calls.
  Messages, calls and invitations to games are already for friends only, so after the block they get the answer anybody
  who is not a friend gets. Invitations already sent, and a request already waiting, leave the blocker's list while
  the block lasts, and show again if it is lifted.
- **Silent means the answers of somebody who removed you and does not respond.** A request from the blocked person is
  taken (`200`) and waits in their *Waiting for an answer*, but the blocker never sees it, gets no event and no mail.
  So is an invitation to a muster: the one who made it sees an invitation nobody answers, and nothing of it, not the
  invitation, new times, a time set or a closing, is shown or mailed to the blocker. The 30-day hold after a refusal
  still answers as it did. A mail that is not sent shortens the request by no more than turning that mail off does.
- **The blocker is told plainly** when they try to ask, answer or invite to a muster somebody they blocked: they know.
- **Chat is not blocked** (the maintainer's word, which is later and more particular than "chat in shared games" in the
  issue's list): nothing in the global chat or a game's chat is hidden or refused. A handle's badge in a chat still
  opens its card.
- **Reactions are not touched.** Bingle has no reactions between people (none to a chat line or a message); a
  reaction is a game's rule ([ADR 0027](0027-reactions-lobbies-and-chat.md)), and a block does not change a game.
- **Only the blocker sees a block**, in their profile, where *Unblock* lifts it. `GET /friends/blocks` lists one's own.

## Consequences

- A blocked person can still open a game by its code and watch or sit at an open seat, as anybody can (ADR 0107), and
  can still see the blocker in *Here now* and at a table they share.
