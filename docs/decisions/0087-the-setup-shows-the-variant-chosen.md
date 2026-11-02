# 0087. The setup shows the variant chosen

Status: accepted (2026-11-02)

## Context

The setup of a game offered its variants as a row of badges, labelled with whatever was in brackets in the name and
otherwise the board size. That worked for sizes (*5×5*), and it fell apart for Solitaire, whose variants share a table
and are different games: four of its seven said *1×1*. A family also took its name from whichever variant sorted first,
which called Solitaire *Bowling Solitaire*. And a tutorial belonged to the family, though Bowling's teaches nothing about
Klondike.

## Decision

**Variants are chosen from a list, under their full names,** for every game. A native `select` copes with any number of
them and any length of name, and works the same on a phone. Among variants of one size the base game comes first.

**A family has the name of its base game,** whatever sorts first.

**A table of cards shows a game in progress, from the game's own rules.** A board can be pictured by pieces on it
(`preview`), but a table of patience has no board. A ruleset can be a `state.Sampler`: it returns a view, in the shape
a spectator gets, that the game info carries as `sample_view`. Solitaire plays its own hints from a fixed deal and shows
where that gets to. So the picture is always a real position under the current rules, needs no position written by hand,
and hides what a spectator cannot see. It is worked out once per game. It costs a few fields in the game info, and only
for a game that opts in.

**A tutorial can belong to one variant,** with `variant_only: true`. It cannot be inferred: Blooms is taught on its small
board for the sake of every board, and the High Priests are a `variant` tutorial of a layout that any layout can use, so
neither the game a tutorial names nor its `kind` says who it is for. The server does the filtering
(`GET /game-defs/{id}/tutorials`), so every client and the rules window agree, and the count in `GET /game-defs` is that
game's. A list of games marks a family if any of its games has one.

## Consequences

Nyet! and Cthulhu Wars keep their previews as they are, since nobody has asked for more; a `Sampler` is there if they want
one, with a drawing of its view on the page. A tutorial that is `variant_only` can still be fetched by its family's name,
because that is how it is started.
