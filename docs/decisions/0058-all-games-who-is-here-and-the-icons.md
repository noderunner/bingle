# 0058. All games, who is here, and the icons of the bar

Status: accepted (2026-10-27)

## Context

`bingle_tasks.md` asked for several things about the bar at the top of every page: two tabs in the
*Active games* window (the browser's own games, and every game on the server, "so that the logged
in user can easily view all games and join games to spectate"); games waiting in a lobby in both
tabs, "with an indicator for how many open seats are still available"; an indicator on the *Global
Chat* tab "if there are new messages in the Global Chat that the current user has not yet seen"; a
*Join Game* icon that "does not look like an 'exit' symbol, perhaps something indicating making a
connection (two plugs joining)"; *Your Profile* as "a floating window" with "a list of all
currently 'active' unique users with active browser sessions"; and a generic rule-book icon in
place of the word *Rules*.

## Decision

**`GET /api/games`** lists every game that has not finished — waiting for players, in a lobby, or
being played — newest change first, with who is seated (people and bots) and how many seats nobody
holds. One query over `games` with a counted subquery, then the catalog for each game's name and
seat count; at most 60 games.

*This makes the rooms public to look at.* A room code has always been the only thing between a
person and a game (ADR 0006: no accounts, no auth), so the list does not give away a power that
was not there — but it does make every unfinished game **findable** rather than merely reachable.
That is what was asked for, and it is the right trade for a casual site whose games are not
secrets; a game that should not be found is one nobody should be able to join by code either. It is
written down here so that nobody is surprised by it later.

**Who is here** (`internal/session/here.go`, `POST /here`): a browser says, once a minute, that
somebody is looking at bingle. There are no accounts, so it identifies itself with a **secret of
its own making** (`localStorage` `bingle.session`), sent to the server and to nobody else; what the
list carries is the **address** that secret hashes to (the first 8 bytes of its SHA-256). So a
person can be found — and, with direct messages, written to — without their address being
forgeable: knowing where to send a message does not let anyone send one *as* them. The list lives
in memory, is forgotten two and a half minutes after a browser last spoke, holds at most 500
browsers, and nothing about it is stored. Names stay self-chosen, and the window says so.

A browser with no name yet reads the list without joining it: an unnamed entry would say nothing.

**The profile is a window** (`#profile-dialog`), not a page: the name this browser plays under,
*Here now*, and a link on to `/profile` for the email notifications and the finished games, which
are nobody else's business and want the room of a page. The page stays exactly as it was.

**The chat badge counts what has not been read**, not only what arrived while another tab showed:
the browser remembers the newest message it has shown (`bingle.chat.seen`), so the history it reads
back on a later visit is counted against it. A message of one's own never counts, even read back
out of the history.

**The icons** move to `js/ui-icons.js`, each drawn with its name in `data-icon` so a page or a test
can tell which picture a button carries. *Join a game* becomes **two plugs coming together** (the
door with an arrow through it read as *leaving*), and every **Rules** button becomes a **rule
book** with *Rules* as its accessible name, in the top bar of a game and in the setup on the home
page.

## Consequences

* The *All games* tab is read when it is opened and whenever the window is reopened on it, not on a
  timer: it is a list somebody goes looking at.
* `js/here.js` is the base the direct messages build on (ADR 0059): the same address, the same
  secret, the same list.
* One behaviour was deliberately kept: `#open-profile` was a link to `/profile` and is now a
  button. A test had to change with it.
