# 0042. Yinsh: ring moves, rows and the opponent's decisions

Status: accepted (2026-10-24)

## Context

Yinsh (Kris Burm, 2003) is played on an 85-point board joined by lines in three directions. Each player has five rings; the two share 51 markers, white on one side and black on the other. A turn drops a marker of your colour in one of your rings and moves that ring along a line, over empty points and over groups of markers (each of which is turned over), landing on the first empty point after a group; rings are never passed. A row of five markers of your colour is removed with one of your rings; the first to remove three rings wins (one in the "blitz" variant). A move can leave rows for both players: the mover removes theirs first, then the opponent removes theirs.

## Decision

- A Go `state.Ruleset` (`internal/games/yinsh`). The board is an explicit graph in the definition: 85 points named like the published board (a2–k10) and their line neighbours, so the renderer draws the lattice. The ring is drawn as a donut and the marker as a small chip.
- Decisions: place a ring (setup, 10 in all), move (`e5-e9`: the marker is dropped where the ring stood, so the click flow is two clicks), choose a row (`row:b5-f5`, its two end markers; a row of six or seven offers each five) and choose a ring to remove (`ring:e9`). After a move the ruleset hands the next decision to the mover while they have rows, then to the opponent while they have rows, and when none is left the turn passes; so `ToMove` can be the other seat in the middle of a turn.
- Variants: `yinsh-blitz` (first ring wins).
- The view tints the last ring's start and end, the markers it turned over, and the rows a player may choose from.
- Bots: a heuristic of rings removed and long runs of markers; `BotGuide` keeps the decisions that leave the bot best off by it.

## Interpretations

Removed markers return to the pool; if all 51 are on the board at the start of a turn the game ends and the player with more rings removed wins (equal is a draw); a player whose rings cannot move passes; 1000 plies end the game on rings removed.
