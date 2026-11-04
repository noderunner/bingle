# 0096. Earth's glyphs and two cults' starts follow the printed boards

Status: accepted (2026-11-03)

## Context

ADR 0095's trace of the 3-5 map showed its board disagreeing with the game. The glyph tags had been assigned by continent, not read from a board, and the sigils of two cults stand elsewhere than they began. Scans of the other three Earth boards show the same on all of them.

## Decision

- **Glyphs:** the Americas are Dragon; Africa, Australia and New Zealand are Chevron; Europe, Scandinavia, Arabia and Asia are Thorns, read from each of the four boards. The names are those of the Primeval layouts, where tags and print already agreed: chevron an arch, thorns a tuft, dragon a swirl.
- **Starts:** the Black Goat begins in Africa (West Africa where it is split), not South America. The Crawling Chaos begins in Asia (South Asia where it is split), not North Asia. These are where the boards print their sigils. The maintainer chose this knowing that it changes how those cults open.

## Consequences

- The Black Goat no longer opens next to the Pacific, which changes its early game and its neighbours' on every Earth layout. `tuned.json`'s weights were tuned on the old starts and have not been tuned again; they name no areas, so they still load and work.
- A game under way keeps its position, and a saved position naming a Gate at the old start is still valid: only new games begin differently.
- The High Priests tutorial's written-out position moves the Black Goat's Gate to Africa.
