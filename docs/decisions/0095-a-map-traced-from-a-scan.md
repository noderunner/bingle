# 0095. A map may be traced from a scan, with a generated picture behind it

Status: accepted (2026-11-03)

## Context

ADR 0093 drew the Primeval Map by eye over photographs 367 pixels wide. A scan at 3000 pixels is now to hand. Players want the map to look like the board, with its colour and texture, and the Glacier glyphs in place. But the board's art is not ours to publish, and the game page redraws the map on every move.

## Decision

- **The trace floods from marks.** `scripts/cwmapart.py` starts from strokes placed inside each area (a sea's on both sides of its shallows, whose thin line is no border) and floods out to the board's dark lines, so borders and adjacency are both read from the print. The black seam between the board's halves is painted out first, or the flood runs along it.
- **The picture is generated: no pixel of the scan is in the repository.** Each area's colours are blurred (sigma 6 pixels of 1500 at the least, and broader where lettering or drawings were masked out) so that only their flow is left. Mottling, cracks, etching and ink come from seeded noise; the coastal line's distance is measured, and its shape is noise. The scan is a command-line argument and stays outside the repository.
- **A baked image, not live SVG filters.** Noise filters across the map would be repainted on every move. A 110 KB WebP costs one decode, and one element is reused across redraws. Areas above it are transparent, so hit testing and every overlay work as before.
- **The Glacier glyph is vector,** the twelve printed copies averaged and stacked as five tones, in a layer the player can hide. It is a tracing of a small part of the board's art, done at the maintainer's request.
- **Layouts one at a time,** the five-player Primeval and the 3-5 Earth at first; [ADR 0097](0097-every-map-is-traced.md) did the rest. Layouts without `params.art` draw as before.
- **The board is the authority for glyphs it prints.** The Earth layouts' glyph tags came from a table by continent, which the 3-5 board contradicts in eight of its ten areas, reading the names from the Primeval layouts (chevron an arch, thorns a tuft, dragon a swirl). A map cannot print one glyph and score another, so `cwmapart.py` writes this layout's tags from what it finds, and small islands, which are scenery, are drawn in too.
- **The Desecrate glyphs are vector, traced from the board,** and replace the old line drawings on every map.

## Consequences

- The trace found two borders the eye-drawn map lacked: Upper Mu with the Rheic Ocean, and the Paratethys Ocean with Poseidonis. All 54 others held, and so did the twelve Glacier areas. These two change the rules at this table, by the board's own print; a game already in progress on this map gets them on its next move.
- The layout's definition grows from 14 KB to 60 KB of text, as it has far more points. Those polygons are hit areas and overlays now, not drawing.
- The other Earth layouts' glyphs and two cults' starts were then settled by [ADR 0096](0096-earth-glyphs-and-starts-follow-the-board.md).
- The picture does not follow the light and dark themes: it looks as the board does in both.
