# 0150. A game's own endings are worded in its definition

Status: accepted (2026-11-25), for #314. It follows [ADR 0147](0147-rules-as-webassembly-in-an-install-pack.md),
whose packs carry no page script, and [ADR 0124](0124-the-server-says-what-things-are-clients-decide-how-they-look.md).

## Context

When a game ends, the page says how: "Alice won by resignation", "a draw on points". It words the platform's own
reasons (a resignation, a timeout, no moves left, five in a row) from its catalogs, in every language. A game whose
rules report a reason of their own could word it only in its page script (`endReasons`, Cthulhu Wars' "with the
most Doom"). Every other game's reason was shown as its code: an install pack's game, which can carry no page
script, read "Alice won (marbles) [white]". Neither a client other than the page nor a bot reading the API had
any words for it either.

## Decision

**A game words its own endings in its definition, and the API sends them in the game's info** (`endings`, which
the maintainer settled on #314 as "the words can live in the bingle API"):

```yaml
endings:
  marbles: with a set of marbles          # after "won" or "a draw"
  marbles:white: with four white marbles  # this detail of the reason: the words say it
```

- A key is a reason the game's rules report, or `reason:detail` for one detail of it. The words follow "won" or
  "a draw", so they read in either. Words for a detail stand for the reason and its detail together, and the
  client shows no detail after them. A detail with no words of its own is shown after the reason's words, as before.
- **The platform's reasons stay the platform's.** A client words those itself, in its player's language, and a
  game's `endings` do not replace them.
- The words are the game's, like its pieces' and actions' names: in the game's language, and translated, if ever,
  with the rest of a game's own content (#253), not in the interface's catalogs.
- A variant patches them like any part of a definition, so Blitz can word its smaller sets.
- The catalog checks them: a key of lower-case letters, digits, dashes and underscores (and one `:detail`), and a
  few words on one line.

## Consequences

- Any game can word its endings with no code: a pack's, a declared game's (the Table fixture's `count` is "on
  rings"), and a Go ruleset's. A page script's `endReasons` still wins over them, so Cthulhu Wars reads as before,
  and its words could move into its definition.
- The game's info grows a field: `version.API` 1.16.0.
