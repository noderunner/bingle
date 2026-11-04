# 0095. A map may be traced from a scan, with a generated picture behind it

Status: accepted (2026-11-03)

## Context

ADR 0093 drew the Primeval Map by eye over photographs 367 pixels wide. A scan at 3000 pixels is now to hand. Players want the map to look like the board, with its colour and texture, and the Glacier glyphs in place. But the board's art is not ours to publish, and the game page redraws the map on every move.

## Decision

- **The trace floods from marks.** `scripts/cwprimevalart.py` starts from strokes placed inside each area (a sea's on both sides of its shallows, whose thin line is no border) and floods out to the board's dark lines, so borders and adjacency are both read from the print. The black seam between the board's halves is painted out first, or the flood runs along it.
- **The picture is generated: no pixel of the scan is in the repository.** Each area's colours are blurred (sigma 6 pixels of 1500 at the least, and broader where lettering or drawings were masked out) so that only their flow is left. Mottling, cracks, etching and ink come from seeded noise; the coastal line's distance is measured, and its shape is noise. The scan is a command-line argument and stays outside the repository.
- **A baked image, not live SVG filters.** Noise filters across the map would be repainted on every move. A 110 KB WebP costs one decode, and one element is reused across redraws. Areas above it are transparent, so hit testing and every overlay work as before.
- **The Glacier glyph is vector,** the twelve printed copies averaged and stacked as five tones, in a layer the player can hide. It is a tracing of a small part of the board's art, done at the maintainer's request.
- **One layout for now,** to judge the result before doing the others. Layouts without `params.art` draw as before.

## Consequences

- The trace found two borders the eye-drawn map lacked: Upper Mu with the Rheic Ocean, and the Paratethys Ocean with Poseidonis. All 54 others held, and so did the twelve Glacier areas. These two change the rules at this table, by the board's own print; a game already in progress on this map gets them on its next move.
- The layout's definition grows from 14 KB to 60 KB of text, as it has far more points. Those polygons are hit areas and overlays now, not drawing.
- The picture does not follow the light and dark themes: it looks as the board does in both.
