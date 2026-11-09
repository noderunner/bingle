# 0128. Data packs: a game as a file an admin installs

Status: Withdrawn (2026-11-09), before anything installed a pack. The private overlay (ADR 0127) already lets a
game be kept unpublished, Go or data alike, so installing games on a running server bought too little for what it
costs: a reader for hostile zips, an admin flow and its audit, versioning and uninstalling. The reader that #177
merged was removed again, and `install_plugins` with it (ADR 0071). A game is part of a build. The decision is kept
below as it was written.

Was: accepted (2026-11-08). Step 6 of the game plugins plan (`docs/history/game-plugins-plan.md` on
`more-plans`), whose decisions 1, 3, 4 and 5 this builds on. It relies on ADR 0071 (capabilities), 0076
(retiring a game), 0098 (a version is a game of its own), 0124 (a game's look is data) and 0127 (rules in Go
are part of a build).

## Context

A game that is pure data needs no build: the definition, its look, its guide and its pictures are all a
server needs. A pack is that game as one file, so an admin can install one on a running server and pass it to
another. A file like that comes from anywhere, and what it holds reaches every player's browser, so the server
has to treat it as hostile until it has checked it.

## Decision

**A pack is a zip of data, and never code.** At its root are `game.yaml` (the definition, whose `id` is the
pack's name) and optionally `web.yaml` (its look); the rest is the guide's Markdown and pictures. `internal/pack`
reads one, in memory, without writing anything to disk.

**What is refused:**

- *Anything that needs Go*: `engine`, callbacks, a `callback` or `ruleset` win condition, setup `options`.
  This is checked on each game as its variants' patches leave it, because a patch can ask too.
- *Anything that would run in a player's browser*: a look's `script` or `style`, at its top or in a variant, and
  any file that is not YAML, Markdown or a picture.
- *A colour that is not a colour.* A colour reaches a style attribute, so `url(...)` would have a player's
  browser call somebody's server, and a semicolon would add a rule of its own.
- *A file that is not what it says*: a picture is sniffed, and text must be text.
- *An unsafe zip*: a path that leaves the pack, a link, the same file twice (ignoring case), and more than the
  limits, which are enforced on what is really read and not on what the zip claims.
- *A game that does not work*: it has to compile, its look has to fit it (`look.Check`), and each game plays a
  short random trial first, so one that panics or takes far too long to think stops at the door.

SVG pictures are allowed, because they are safe as served: an `<img>` runs no script, and opened directly an
asset gets a sandboxing `Content-Security-Policy` and `nosniff`, as every game's has. A guide's Markdown loads no
picture from outside the game's own files.

**A pack can never shadow a built-in game.** Its id is `pack-name`, and `tests/registry` holds that no built-in
module has the prefix. A dash, like ADR 0098's, because `encodeURIComponent` leaves it alone.

**Each version is a game of its own**, on ADR 0098's terms: `pack-name-version`, with variants
`pack-name-version-word`, and the family's name `pack-name` meaning the newest. The server sets `family` and
`prototype` itself and refuses a pack that sets them. So **an upgrade disturbs nobody**: a new version installs
beside the old, new games use the newest, and a game in progress finishes on the rules it started with. An old
version is dropped once it has no unfinished games. Being a `prototype`, a pack's game is **not rated**, which also
suits rules from somewhere the server does not vouch for.

**Uninstalling ends a pack's unfinished games**, once the admin has confirmed how many, with no result and the
end reason of ADR 0076. Its finished games stay readable as a record.

**Installing is an admin's act**: the `install_plugins` capability, the password again, and an entry in the audit
log with the zip's hash.

## Consequences

- Packs carry no tutorials yet. A tutorial names the version it plays (ADR 0098), so it has to be renamed with the
  game, and nothing asks for it so far.
- A pack draws with what the page already draws: its shapes, tints, badges, cards and pictures. A game that needs
  its own drawing needs a build.
- How a non-web client gets a pack's pictures stays undecided, as the plugins plan left it.
