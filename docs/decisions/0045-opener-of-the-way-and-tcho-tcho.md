# 0045. Cthulhu Wars: the Opener of the Way and the Tcho-Tcho

Status: accepted (2026-10-25)

## Context

Two more cults, from the wiki's faction pages: the Opener of the Way (Onslaught One), which moves Gates and promotes units, with Yog-Sothoth as a Gate himself; and the Tcho-Tcho (Onslaught Two), with High Priests, Proto-Shoggoths and Ubbo-Sathla. Both set up after the others.

## Decision

- **Table.** Eight cults (`ow`, `tt`; the table still seats up to five); their units come last in `unitTypes` and their moves last in `ActionNotations`, so the numbers of everything older do not move. Colours purple and brown. Setup: the Tcho-Tcho (an empty area with a glyph), the Windwalker, then the Opener (any empty area) choose in turn (`Data.Setup`, `SetupLater`).
- **Opener.** Yog-Sothoth is a Gate in `gatesOf`/`hasGate`/`gatesOnMap` (not in `Data.Gates`, so The Beyond One ignores him). The Beyond One moves a unit costing 3+, the Gate and its holder; the Gate may be an enemy's. Channel Power splits rolling from tallying: the dice wait in `Battle.Dice` while a `channel` prompt lets the Opener reroll. The Million Favored Ones are post-battle moves (a Pained unit that is replaced is replaced in the list of the Pained). Dread Curse runs on its own state (`CurseState`) and four prompts — the Opener picks the faction for each result, the faction the unit, the Opener the destination of each Pained unit. Dragon Ascending is offered in the Opener's own turn even with no Power (a turn with no Power would otherwise be skipped) and in the Doom phase; Dragon Descending is a variant of the Ritual, `ritual:descend`.
- **Tcho-Tcho.** Always three High Priests (the Oath too). Sycophancy asks the player who performs a Ritual (`syco:give/tt` or `syco:less/tt`). Hierophants places a High Priest after every earned book or grows Ubbo-Sathla (`Data.Growth`); Ubbo is awakened by a High Priest, free in the Doom phase; Hell's Banquet rolls at the start of each Doom phase after the awakening. Martyrdom is immediate: once the High Priest takes a Kill, the Kills still queued and those already taken by the other units of that battle become Pains (killed units come back Pained).
- **Borrowing.** The Sleeper's Ancient Sorcery lends The Beyond One and Sycophancy like any unique ability (`ability`).

## Interpretations

The Tsang tribe's six spellbooks only (the old Tcho-Tcho); the Leng and Sarkomand tribes of Final Onslaught are not in the game. "An empty area marked with a faction glyph" is an empty area with a glyph on this map. Idolatry's "another faction's starting glyph" is the start area or sigil of any cult but the Tcho-Tcho's own. "Dragon Ascending at any time" is a turn action and a Doom-phase action. "Ignoring normal Pain rules" lets the Opener drive a Pained unit to any adjacent area. The bots play Channel Power, promotions, Terror, Ubbo-Sathla and Dragon Descending with many Gates; they do not use The Beyond One or Idolatry.
