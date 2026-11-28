# 0155. Major announcements, shown once to each player who was there

Status: accepted (2026-11-27), for issue #356. Builds on [ADR 0139](0139-a-welcome-tour.md) (the welcome tour,
whose card it shares) and [0067](0067-principals-and-guest-keys.md) (principals).

## Context

The server's operators want to tell the people who play about something big, such as a desktop client that can now be
downloaded, the next time they come to the site. The changelog in the help window says what changed, but it is
read only by those who open it, in a line each. The maintainer settled the shape of it with the issue: the tour's
card, once each, only on the main page, and a way to look back from Recent Changes.

## Decision

- **An announcement** is a title and a Markdown body that an administrator writes and publishes from the admin
  page, in the one language they write it in: it is not translated. It has no expiry. Withdrawing one stops it
  being offered or listed; it is kept, and the admin page still lists it. Only an administrator signed in, never
  by an API token, may write or withdraw one.
- **Who is shown one.** An announcement is waiting for a guest or account when it was published after the
  principal was made (`principals.created_at`) and that principal has not closed a card that showed it. So a
  newcomer is never shown the past, and somebody who was away is shown what came while they were gone, oldest
  first. A guest that is merged into another principal (signing in, taking another device's key) brings its seen
  marks with it, as it brings having seen the tour; a guest who signs up is the account and keeps them.
- **Where and when.** Only the main page shows them, as it offers the tour; a game's page does not, so somebody
  arriving by an invitation plays first. `/auth/me` names the ids waiting, so a visit with none loads nothing more.
  The tour comes first: a page that offers it does not show announcements, which wait for the next visit (a
  newcomer has none anyway).
- **Seen is closing.** Closing the card in any way (Done, Close, Esc) marks every announcement it showed as seen
  (`POST /auth/announcements-seen`), whether or not each slide was looked at: the card showed them all, a dot each.
- **Looking back.** Recent Changes links, under each release it shows, the announcements published from that
  release's date until the next one's (those after the newest release go under it), every one not withdrawn,
  including those from before the player existed. Opening one there shows it alone in the card and marks nothing.
  The links come from a call of their own (`GET /api/announcements`), made when the window opens, so the help
  window is as it was on a server without a changelog.
- **One card.** The tour and announcements share `js/tour-card.js`, and only one card is open at a time.
- **Links in the body** may be to a page of this server by its path (`/client`) as well as to other sites, in
  `js/markdown.js` for every Markdown it renders; they open in a new tab, so the card stays.

## Consequences

- The MCP server and bingle-3d show no announcements yet; `/auth/me`'s ids and `GET /api/announcements` are there
  for them.
- An announcement has no pictures of its own yet: a relative image would be looked for beside the main page. Giving
  it pictures, kept with it on this server, is #357's.
