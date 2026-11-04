# 0097. Every board up to five players is traced, and the print decides

Status: accepted (2026-11-03)

## Context

ADR 0095 traced two layouts. The other four were still drawn by eye or from small pictures, and the expansion's two four-player Primeval boards were not in the game. The scans show that each board is two halves, a three-player or a five-player one on each side, with one set of coasts: the corners of the board say which.

## Decision

- **A map in `cwmapart.py` is its two halves** and the seas all boards share. The eight boards reuse each other's strokes, so a trace of one half is checked by the others: on the 4-player Primeval boards, every border also appears on a three-player or five-player board.
- **The two four-player Primeval boards are layouts,** `cthulhu-wars-primeval-3-5` and `-5-3`, named as Earth's are, left half first. They seat the same four cults as the base game, and need no rules of their own.
- **The print decides on every layout,** as ADR 0095 decided for those it traced:
  - Earth, 5-3 and 5-player: Central America runs on along the north of South America with no line across, so it borders South America East too. On the 5-player layout South America West meets the South Atlantic at its tail and not the North Atlantic, as it already did on the 5-3.
  - Primeval, 3-player: Upper Mu meets the Rheic Ocean, as on the 5-player board, and Poseidonis has a Glacier glyph, the one that stands in Lomar on the boards that split it. It was missing, so this layout goes from eight Glacier areas to nine.
  - Primeval 5-3: the east of Atlantis has no glyph printed, so there it has no Desecrate glyph. It has one on the boards whose print shows it.
- **`cwprimeval.py` is gone,** as nothing is left for it to write. `cwmaps.py` keeps only its `--guide` table writer.
- **Six-player boards and above stay out.** They have areas of their own (the Bering Sea, the Mediterranean, the Mountains of Madness) and the game has no layout with more than five seats. A trace would be the small part of adding them.

## Consequences

- The six new pictures add about 780 KB to the binary. A game loads only its own.
- A game in progress on a changed layout gets the new borders on its next move, and keeps any Glaciers already placed. On the three-player Primeval map, Poseidonis can now be chosen for a Glacier.
- Every Earth layout now shows its glyphs as printed rather than in badges. Primeval keeps its badges beside the name, as its five-player layout already did. Atlantis prints its glyph twice on the 3-5 board, so a move to printed glyphs there would need a choice.
- A variant's `params` objects merge over the base's, so `glyph_marks` and `area_fill` can hold keys for the base layout's areas. Lookups go by area, so nothing reads them; Primeval's `glyph_marks: null` is what stops Earth's standing on its maps.
- `tuned.json` was tuned on the old borders, as ADR 0096 said for the old starts, and names no areas.
