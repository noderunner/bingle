# 0108. A Great Old One emerges: the art is code

Status: accepted (2026-11-05)

## Context

The maintainer asked for a short show on the map when a Great Old One is awakened for the first time,
with detailed animated art, starting with Cthulhu rising from the sea, and a badge in the history that
plays it again. The best way to make the art was left open.

## Decision

- **The art is procedural SVG in code** (`js/cw-cthulhu-art.js`), original, with nothing from the game's
  printed art. A scene is an SVG and a `seek(t)`: one clock draws everything, so there are no assets to
  license or load, the page has no build step to add, it stays sharp at any size, and what writhes can
  really writhe. Generated images would be flat, heavy, and would need their rights looked at.
- **The game says when, the page says how.** The engine adds the line `<name> Emerges` to the log only
  the first time a cult's Great Old One is awakened, from the `Awoken` it already keeps. It changes no
  rule. The history draws that line as a badge in the cult's colour.
- **Every Great Old One gets the badge; only those with a scene can play it.** Cthulhu is the first.
  Adding another is a scene and a line in `SCENES`, and old games' badges start working.
- **It is seen as it happens or asked for.** It plays on the moves that arrive while the page is open,
  not when a page opens on an old position. *Battle and Great Old One animations* turns the automatic
  showing off, and the badge still plays it. With reduced motion, the badge shows one still picture.
- **The stage is shared with the battle show** (`js/cw-map-stage.js`). A battle takes the map from it.

- **The five Great Old Ones of the core set each have a scene of their own**, with its own place and palette, not one
  scene recoloured: Cthulhu rises from the sea, the King in Yellow from the trap of a rotting stage, Nyarlathotep condenses out
  of the shadows, Hastur comes from a point in the abyss between the stars, and Shub-Niggurath heaves up out of a swamp.
- **Eight more have one too**: Tsathoggua stirs from his slumber in a cavern, Ithaqua walks out of a blizzard, Rhan-Tegoth breaks out
  of a glacier, Yog-Sothoth presses through a rift among the dials of time, Ubbo-Sathla throws the lid off Hell's Banquet, Bastet rises from
  a golden pyramid on the moon, Cthugha gathers in a rising fire, and Abhoth heaves up out of a sump and sheds his brood.
- **An Independent emerges like any other**, once for each cult that wakes it. Awoke's numbers set Cthulhu's price, so an
  Independent is kept in `Awoken` under its own name and cannot change it. Independents without a scene get the badge alone.
- **The Animation Tester** (a tab of Cthulhu Wars' setup) plays each show and a fixed battle with no game going, from data
  made up in `js/cw-animation-tester.js`. It is a tool for looking at them and is expected to go.

## Consequences

- Games from before this have no such line in their stored history, so no badge.
