# 0139. A welcome tour, offered once to each guest and account

Status: accepted (2026-11-20). It builds on [ADR 0067](0067-principals-and-guest-keys.md) (principals and guest keys),
[0073](0073-account-only-features.md) (what is an account's) and [0109](0109-scheduled-games.md) (scheduled games,
whose offer of several times is called a *muster* from now on).

## Context

The maintainer asked for a short tour for newcomers: a floating card on the main page with a few slides, each a
feature worth knowing about, a picture and a few words. A newcomer, and an account that has not been offered it,
sees it once; skipping or finishing it is remembered by the server, and the help window offers it again. The
slides were chosen with the maintainer, including one for guests about their key and what an account adds, one
for analysis batches and one for musters.

## Decision

- **The slides**, in order: Welcome (the list of games), Play anyone (friends, bots, two bots), Learn a game
  (rules and tutorials), At the table, Sharpen up (puzzles, hints, replay), Analysis, Never lose a game (the
  active games), Compete (ratings, tournaments, badges), Friends, Muster, and, for a guest only, You're playing as
  a guest: where the key is, how to move to another device and what an account adds.
- **The server's offer decides.** A slide whose feature nobody here may use is left out; one only an account may
  use says *With an account* to a guest. So `/auth/me` tells a guest or newcomer `account_caps`, what an account
  may do here, beside `caps`; ratings, badges and musters are accounts' in the code itself and need only
  `accounts`. The guest's list of what an account adds is built the same way.
- **The pictures are drawn, not screenshots.** Each is made by the page's own code from sample data: the game
  cards and boards by `renderBoard`, a real puzzle, the batch chart by `rateBars`, the active games by `gameRow`, a
  badge by `badgeItem`, the guest's key by `guestKeySection`. The rest (the opponents, the rules window, a rating
  table, the friends window, a muster) are small copies built of the page's classes. So they follow the light or
  dark theme, change when the page does and add nothing to the binary. A picture is inert, keeps no id the page
  has, and is read as one description.
- **Seen is a mark on the principal**: `principals.tour_seen_at`, empty for every principal made before, so that
  every existing guest and account is offered the tour once. `POST /auth/tour-seen` sets it; a page's first
  request makes its guest, so a newcomer's Skip both makes the guest and marks it. A guest who signs up is the
  account and keeps the mark; when a guest is merged (signing in, taking another device's key), the one that
  remains has seen the tour if either had.
- **The browser remembers too** (`localStorage` `bingle.tour.seen`), and the tour is offered only when neither the
  browser nor the guest or account has seen it. Without that a browser signed out, which is nobody until its next
  game, would be offered the tour again.
- **Where and when.** Only the main page offers it, once it knows who is there; a game's page never does, so that
  somebody arriving by an invitation plays first. Skip, Done and Esc all mark it seen. The help window's *Take the
  Tour* shows it on any page, whoever has seen it.

## Consequences

- A new feature worth a slide gets one in `js/welcome-tour.js`, with a picture drawn the same way.
- Browser tests start with the tour seen (a script sets the browser's mark), so that it does not lie over the page
  of a test about something else; the tour's own tests use a browser without it.
- There is no server-wide switch to turn the tour off.
