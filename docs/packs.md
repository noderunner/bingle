# Data packs

A data pack is one game as a zip of data, which an admin installs on a running server and can pass to
another ([ADR 0128](decisions/0128-data-packs.md)). A pack has no code: a game that needs rules in Go is part of
a build ([ADR 0127](decisions/0127-commercial-games-in-a-private-overlay.md)).

## What is in one

At the root of the zip:

```
game.yaml        the definition (see dsl.md); its id is the pack's name
web.yaml         how the web page draws it (optional; see ui.md)
guide/rules.md   its guide's pages and pictures, wherever game.yaml's guide says
guide/line.png
```

`game.yaml` is an ordinary definition, with these rules:

- `id` is lower-case words joined by dashes, up to 40 characters. The server adds `pack-`.
- `version` is required, as `major.minor.patch`.
- A variant's id is the pack's id, a dash and a word: `lines-big` for a pack called `lines`.
- `family` and `prototype` are the server's to set.
- `engine`, `callbacks`, a `callback` or `ruleset` win condition and `options` need Go, and are refused.

`web.yaml` is an ordinary look without `script` or `style`, since a pack runs nothing in a player's browser. Every
`color` has to be a colour: a name, a hex value, or `rgb()` or `hsl()`.

The zip holds YAML, Markdown and pictures (PNG, JPEG, GIF, WebP, SVG), with no links. It may be 20 MB, and unpack
to 40 MB in 300 files of 8 MB at most. A definition may come to 1 MiB and declare 64 variants.

## Versions

Version 1.2.0 of `lines` is the game `pack-lines-1.2.0`, and `pack-lines` means the newest version installed. A
new version installs beside the old: new games use it, and a game in progress finishes on the rules it started
with. Games of a pack are not rated.
