# 0102. Joining with one field, unread chat, and reading back through a game

Status: accepted (2026-11-04)

## Decision

**Join Game asks only for a code or a link.** It takes an open seat, or the seat the name already holds. At a full table it joins
as a spectator, with no separate *Watch* button. A person who wants to watch a table with an open seat opens its link, or uses
*Watch* in the game's Active Games tab. The name is the profile's, then the account's, then a generated guest name.

**Unread chat is the browser's to count.** It remembers the last message of each game it had on screen, and when the active
games list opens it asks each game for the messages after that, leaving out the player's own. So it costs nothing until the
list is opened, and a game that has never been opened in this browser counts its messages from the start.

**The rule of the History tab goes to the player's last move, in the Moves tab**, with the focus on *Next*. Each step of history
then says what happened in the words of the History tab, where it has any, and otherwise gives the notation.

## Consequences

- Nothing in the game changes a player's name once they have joined: the name is the identity of a seat. Changing it means
  changing the profile before joining.
- If the name is another player's at that table, the server's refusal shows in the window, and the profile is where to change it.
