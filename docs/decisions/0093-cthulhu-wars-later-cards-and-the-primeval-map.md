# 0093. Cthulhu Wars: the later Loyalty Cards share one action, and the Primeval Map is drawn by hand

Status: accepted (2026-11-03)

## Context

The wiki lists many more neutral units and Independent Great Old Ones than ADR 0046 brought in, and alternate maps with rules of their own. Each ability in the engine so far has a verb of its own that has to be threaded through `Legal`, `Apply`, `Format`, `Parse`, `Steps`, the action space, the labels and the bots. That is about ten places per ability, which does not scale to dozens more.

## Decision

- **One action for the later abilities.** `card:<id>[.<unit>][@area][/faction]`, with a table (`cardActs`) that gives each decision its label, its text, its Power and its shape. The notation, the action space and the labels come from the table. A prompt of kind `card`, told apart by `Why`, covers the questions.
- **Questions that wait.** An ability triggered in the middle of an assignment or a retreat appends to `Data.Pending`. The end of `Apply` opens those first and play resumes where it was, the way `openPicks` already worked.
- **Numbers do not move.** New cards come after the old in `cards`, so the bits of `Data.Loyal` and `Data.IBooks` keep their meaning in stored positions, and new unit types come after Filth. `Data.Loyal` grew to `uint64` and `Data.IBooks` to `uint32`, and both read old documents unchanged. `independentCards` replaces the old assumption that the Independents are the last cards.
- **The action space gets a catch-all for abilities that reach any unit.** The arranged-table test found moves missing from it, with Shriveling, Undimensioned and the Shantak's carry, that random games from the start rarely reach. Those gaps are older than these cards. New moves are added at the end, so the numbers of existing ones stand.
- **Primeval is drawn by eye.** Earlier maps were traced from clean area diagrams, but all there is for Primeval is photographs of the board at 367 pixels. `scripts/cwprimeval.py` holds outlines drawn over those pictures, paints them land over sea and reads the borders and the adjacency back from the painting.

## Interpretations

- A capture in an area with a Cat from Venus is the Cat's. A Gug's rank does not count for capturing, so it captures only with another unit there.
- Cosmic Unity silences the strongest enemy Great Old One rather than asking, since nothing is gained by silencing a weaker one.
- Bokrug's *Doom that Came to Sarnath* has an Elder Sign eliminated at random, because the enemy choosing cannot see their values.
- Mummified Acolytes are counted per area and cult, as Cultists are not told apart.
- Y'Golonac's new owner earns his spellbook at once, as the requirement says.

## Not done

Insects from Shaggai (Mind Parasite gives another cult control of Acolytes) and Eihort (the Brood are a new kind of Cultist that every Cultist rule would have to know). From Below does not give Nyogtha his free second Capture or second Battle. Laziness has no alternative against a hibernating Windwalker.
