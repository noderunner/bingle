# 0051. Cthulhu Wars: capturing the Cultist on a Gate, a window of the neutral items, glyphs off without Yellow Sign

Status: accepted (2026-10-26)

## Context

The player's playtest list asked for four things: a side window that lists the neutral spellbooks, Monsters, Terrors and Great Old Ones still to be taken; no literal `&nbsp;` in the rules; the Yellow Sign glyph badges off by default when Yellow Sign is not at the table; and a choice, when capturing a Cultist, between the stack beside a Gate and the Cultist that holds it — with the Gate left abandoned and the captor's own Cultist stepping onto it.

## Decision

- **Capture** (`captureMoves`, `capture` in `actions.go`): when the enemy has more than one Cultist in the area and the one that holds the Gate is a Cultist (`gateHolderOf`), two moves are offered: `capture:cultist@area/faction` (from the stack) and `capture:gate:cultist@area/faction` (the one on the Gate, `onGate+uCultist`, the same piece value the client already uses for "the unit on the Gate"). Otherwise the move is the old `capture@area/faction`, which takes the occupant when it is the only Cultist. The steps are `capture → area → piece → vs-faction`, so the area is clicked first and the piece buttons ("Acolyte", "Acolyte on the Gate") appear only where there is a choice; the forced piece step is taken by the builder's `autoAdvance`.
- **The Gate**: capturing the unit that holds a Gate (a Cultist or the High Priest) no longer hands the Gate to another unit of its owner (`syncGate`); it is **abandoned** (`vacate`), and the captor's own Cultist or High Priest in the area takes it at once (`canControlGate`, `occupy` — no `Toggled` check: the Gate was not the captor's to give up). The owner may put a unit of its own on it in its own turn with the free `control` action. Killing the unit on a Gate still hands it to another unit (ADR 0030); capturing a Monster with Capture Monster is unchanged.
- **Bots** (`guide.go`): the Cultist on the Gate scores 89, a plain capture 88 and the explicit stack capture 60, so the bots always take the occupant. `capture:gate:cultist` is appended to the end of `ActionNotations`.
- **Neutral window** (`neutral_view.go`): `View.Neutral` lists what is still to take — neutral spellbooks nobody holds and none was set aside for (Azathoth option), and every Loyalty Card in the box that nobody holds (a dead Independent is back in the pool). It is nil, and the window absent, when the table plays with neither the Azathoth faction's content nor a pack. The client (`neutralCard` in `game.js`) shows it as a card under the cults in the side column (the Round page on a phone): a unit is only its icon, a spellbook the book pictogram and its name; hovering opens the rules, price and numbers (`neutralTip`).
- **Rules text**: `markdown.js` decodes a small set of character references (`&nbsp;`, `&amp;`, `&mdash;`, numeric ones…) in inline text, always into text nodes.
- **Glyph layer**: `S.settings.glyphs` is `null` until the player decides; then the badges show only when Yellow Sign is among the seats (`glyphLayerOn`). The glyph badges mark what Yellow Sign must Desecrate and mean nothing to the other cults.

## Interpretations

The window is a card docked beside the cult cards rather than a floating window. "Available to take" means: not held by any cult, not set aside at setup for a cult that swapped it in. The High Priest holding a Gate is captured the same way (no choice: there is only one). A captor with a Cultist and a High Priest in the area puts the Cultist on the Gate (`pickHolder` order).
