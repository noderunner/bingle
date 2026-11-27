# 0152. A game is named for the rules it plays, and a renamed game keeps its past

Status: accepted (2026-11-27)

## Context

The game called Reversi plays Othello's rules: four discs start in the middle and Black moves first.
Reversi as first published starts empty. Core 0.30.0 had dropped "(Othello)" from its name because
Othello is a trademark ([docs/history/game-plugins-plan.md](../history/game-plugins-plan.md)), so
the name and the rules disagreed (#310). A game's id is kept with every game played, its ratings,
tournaments, scores and saved setups, and is in links, the API and other clients' files.

## Decision

- **A game takes the name of the rules it plays, and credits their designer.** Reversi becomes
  **Othello** (`othello`), by Goro Hasegawa; its guide says how it grew from Waterman and Mollett's
  Reversi. The maintainer judges Othello an open game for the public repository: it is so widely
  played that its removal is unlikely to be asked for. This supersedes 0.30.0's choice.
- **A renamed game lists its former ids** (`catalog.Module.Formerly`). A former id, alone or with a
  derived suffix (`reversi~pie`), finds the game as it is now (`Catalog.Pin`, so `Get`, a create, a
  bot match or a tournament with it all reach `othello`), and a game made with it is stored under the
  new id.
- **What was stored under a former id moves once, when the server starts** (`store.RenameGames`):
  games and their tutorial rooms, batches, tournaments and their game's name, setup profiles,
  scores, ratings and rating history with the family's name, badges and the versions seen. It finds
  nothing to do once it has run.
- Records stay as they were written: past changelog entries, plans and accepted ADRs say Reversi.

## Consequences

- Old links, API calls and other clients that name the old id keep working, so bingle-3d can move
  to the new id when it likes; nothing in what the API sends or reads changes shape.
- A former id can never be the id of another game: the catalog refuses it.
