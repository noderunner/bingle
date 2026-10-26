# 0036. Fist: one move is one whole turn

Status: accepted (2026-10-24)

## Context

Fist (the game of Alysha Wishingrad's *The Verdigris Pawn*, open beta rules 1.1, Crab Fragment Labs) is a perfect-information abstract game: a challenger against a king's side, plus a neutral verdigris pawn either side may move. Its rules (pinning with allies and divided attention, jump chains with an optional capture per jump, the Ace's Convert, the mage's Empower, the pawn's control and "no straight back" rule) do not fit the declarative DSL.

## Decision

- Fist is a Go `state.Ruleset` (`internal/games/fist`) with a JSON definition for the board, seats, pieces and legend, like Courtfall. It has no hidden information and no dice. `Redact` only attaches the derived view (tints, badges, guard counters, the prompt).
- A decision is a whole turn. Legal lists every step (with each pin it could make) and every jump chain with a choice (leave, capture, capture and replace) at each jump. Notation: `e5-e6`, `e5-e6xf6`, `a1^c1x^e3c`. The clicks are the piece, each landing square with an action button for the choice, and an end button.
- The pawn is coloured `green`. The definition gained `extra_colors` so a position may hold a colour that no seat owns (and no reserve may).
- Values are frozen at the start of each turn, so Empower cannot change mid-turn; the pawn is never empowered and lends no help to other pieces' pins.
- The bots play through `BotGuide`: no capturing their own pieces and no jumping over an enemy piece they could take.
- The square-board renderer now draws a ruleset's tints and glyph badges (as the hex renderer already did).

## Interpretations of the beta rules

- A side with no legal move draws; the game is drawn after 400 plies; a chain stops when it takes the king.
- The king gains Empower only when the king's side captures the mage.
- The pawn may not end a move on the square it left on the previous turn (it may pass over it), and the restriction lapses if the pawn did not move on the previous turn.
- The squares of the rules' diamond board are the squares of a chess board turned 45 degrees: the king on e5, the Keep d4 e4 d5 e5, the mage on a8, the pawn on h1 and the ace on a1.
