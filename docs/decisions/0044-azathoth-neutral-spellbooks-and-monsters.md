# 0044. Cthulhu Wars: neutral spellbooks and Monsters of the Azathoth faction

Status: accepted (2026-10-24)

## Context

The Azathoth faction (Onslaught One) is neutral: its units and spellbooks may be taken by any cult. Two of Black Goat's and Crawling Chaos' spellbooks — Ghroth (a minute for the enemies to decide who loses Cultists) and The Thousand Forms (a negotiation) — need a clock, which does not suit games whose turns are taken days apart. The six neutral spellbooks have no clock, so the player wanted them as a replacement, with or without the Azathoth faction in the game.

## Decision

- Two setup options (`config.go`, `neutral.go`): the bool `azathoth` and the `book-swaps` option (a new option kind, `book-swap`, with `slots` — the cult and the book it may give up — and `choices`, the six neutral books). Both are part of the derived game id (`-az`, `-bg.shrivel`) and the `params` (`azathoth`, `book_swaps`), so nothing else is stored.
- **Spellbooks.** Own books keep the numbers 0–5 in `Data.Books`; the neutral ones are 6–11, held in `Data.Neutral`, and count among the six a cult needs. A requirement still earns *a* book: `bookChoices` lists the own books not held (less one swapped out at setup), the neutral book set aside for the cult by a swap, and — with the faction in the game — every neutral book nobody holds or has set aside. A neutral book has one holder. A swapped-out book is never offered, so its rules never fire.
- **Effects.** Shriveling is a Pre-Battle move, Stars Are Right is paid in `reveal` during the Doom phase, Umr at-Tawil lowers the Gate cost, The Mao Ceremony is a prompt at the end of Gather Power (before the Oath prompts), Recriminations is an Action that removes a held book and queues a choice that excludes it, Undimensioned is an Action that opens a prompt of single-unit moves between the areas the cult had units in, leaving the unit on a Gate where it stands.
- **Monsters.** Dimensional Shambler, Star Vampire, Elder Thing and Servitor are unit types of every cult once its Loyalty Card is bought (`Data.Loyal`; `e.units(seat)` appends them). The card costs 2 Doom, once per Doom phase, in the Doom turn (`buy:`); the Servitor's goes to another player. Shamblers wait on the faction card (`Data.Card`) and are placed in a `react` prompt after any action, the actor's own included, which is always asked. Vampirism takes the first dice of the Star Vampires' share of the roll; Mind Control mutes Devour, the Harbinger, Avatar, Desecrate, Vengeance and Eternal; the Servitor's −1 comes off the total.
- The action space of the training environment only grows at the end (`ActionNotations`); the bots decline Recriminations, Undimensioned, The Mao Ceremony and the purchase of cards, take neutral books last, use Shriveling and walk their Shamblers.

## Interpretations

The wiki gives the card texts and "Neutral Monsters" rules but no rule for taking a neutral spellbook: it is taken in place of one of the cult's own (the game of the Azathoth set), held by one cult at a time. Azathoth himself (an Independent Great Old One) is not implemented. A Servitor's Combat of −1 is a die less for its owner. "Fixed" choices: Undimensioned does not move the unit that occupies a Gate, and the Mao Ceremony and Walk Between Worlds are asked as prompts.
