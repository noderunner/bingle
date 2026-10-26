# 0043. A card framework, and Nyet! on it

Status: accepted (2026-10-24)

## Context

Bingle's games were boards and pieces. Card games need things the engine had no place for: decks defined as data (suits, values, symbols, text), shuffling and dealing, cards that are private (a hand), face down, face up or public (a trick, a play area, splayed piles), and a client that draws them. Nyet! (Stefan Dorra) is the first: a trick-taking game whose round terms (leader, discards, trump, super trump, trick value) are fixed by crossing choices off a board, then a team is chosen and tricks are played with trumps and loot.

## Decision

- **DSL** (`dsl.CardsDef`, a `cards` section): `suits` (label, colour, symbol), `decks` (a template of suits × `ranks` with a count, text, symbols and tags each, plus explicit `extra` cards and `remove`), a card `back`, and `zones` (scope seat or table, visibility owner/all/flag/none, layout fan/row/stack/splay/grid). The catalog expands decks into card types and canonical instance ids (`blue-1#2`) and validates everything (`catalog.CompiledCards`). Board `style: "none"` draws no board.
- **Runtime** (`internal/engine/cards`): `Card` (an instance id and a face-up flag), `Zone` (push, take, draw, flip, shuffle, deal), `Table` (all zones), a one-number `RNG` (splitmix64) a position can carry and keep secret. `Table.Redact` and `Table.View` give a viewer only the faces they may see; a hidden card is the empty `Card`, so neither the position nor the view reveals it, and a hidden pile sends only its count.
- **Client** (`static/js/cards.js`): the card table (each seat's zones, a fan of faces for your hand and of backs for others, piles with counts, rows with captions for a trick, splayed loot), flip and deal animations from the change between renders, cards clickable when the game offers `card-<type>` actions, and a generic board of choices (`view.grid`, boxes chosen by `cross-<line>-<box>` actions). Action ids drawn by panels are named in `view.panel_actions` so they are not repeated as buttons, and `view.labels` names dynamic actions (a team to pick). Game info carries `cards` (suits, card types, zones).
- **Nyet!** (`internal/games/nyet`): a Go ruleset; every decision is one click (cross a box, pick the team, bonus holder, discard or pass a card, play a card). Variants for 2, 3 and 5 players. A card is played, discarded or passed by its type (`blue-9`), not instance. The ruleset hides hands, the discards and the shuffling state in `Redact` and re-deals the cards the viewer cannot see in `Determinize`, so the search bots work. The same fix made the generated rules page say each game's own ending (`RulesetInfo.WinRules`).

## Interpretations (Nyet!)

The published board's boxes are not all in the rulebook text: the discard line here has Nyet!, 1, 2, except 1 and pass left; the trump line has the four colours; the super trump line the colours and Nyet!. The first dealer is the first seat and the deal passes to the left. A tie at the end is a draw (the rules call them joint winners). Players choose which card to discard in turn rather than all at once (hidden from the others); passing left is simultaneous.
