# 0072. Identity in the interface: handles, markers, "My games" and a guest key that travels

Status: accepted (2026-10-31)

## Context

Stage 7 of [the accounts plan](../history/auth-and-accounts-plan.md) (§3a, §6): handles kept for
their accounts, a guest told apart from an account wherever a player is named, the account page,
"My games" on the server, the audit log for administrators — and the owner's request of
2026-10-31, copying a guest key to another device to carry on the games there.

## Decision

- **Handles are globally reserved.** A name that is an account's handle (`ana`, `@Ana`), or one
  nobody may have (`admin`, `support`, …), may not be typed by anybody but that account: not for a
  new game, a new seat, a spectator's chat line or the global chat (`409`). A name that only begins
  like one ("Ana Lima") is fine. Seats taken before keep their names.
- **A rename holds the old handle** for its account for 30 days (`handle_holds`), so nobody can take
  it and pass for its owner; the owner may take it back.
- **Who holds a seat** is in every game answer and state event (`handle` for an account, `guest` for
  a guest, neither for an anonymous client), and in every chat line; `yours` says a seat is the
  asker's. The page shows them as badges beside the name — `@ana`, `guest` — rather than in it, so
  a name stays the name.
- **"My games" is the server's** (`GET /api/me/games`), for every principal: the games it has a seat
  in. The page merges it into the browser's own list, so a game started on another device appears,
  and a game page whose browser has no name for the game takes the seat the server says is `yours`.
- **The guest key travels** (§3a). The profile page of a guest copies the key, as text and as a link
  `…/#guest=bgg_…` (`GET /api/guest/key`, the page only; the key is the cookie's, read back on
  request). Opening the link on another device — or pasting the key there — adopts it
  (`POST /api/guest/adopt`): that browser becomes the guest, its own guest merged in so it loses
  nothing, and the fragment is taken out of the address at once. A signed-in account adopting a key
  takes that guest's games into itself. Adoption answers every key that does not work the same way
  and is limited to ten tries an hour per address.
- **A new key, same me** (`POST /api/guest/rotate`) retires every copy of the old key and keeps the
  seats; **starting over** (`POST /api/guest/discard`) retires the guest, whose seats stay with it.
- **The account page** shows the account's own address (and no other's), changes the handle, lists
  where it is signed in with "sign out everywhere else", and keeps the password, second factor,
  providers and tokens of the earlier stages. The **admin page** adds the latest 200 lines of the
  audit log.

## Left out

The one-time transfer code the plan considered is not built: the key and its link are enough, and
a code can come later beside them. Changing an account's address is not on the page yet.
