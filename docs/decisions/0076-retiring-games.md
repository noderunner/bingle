# 0076. Retiring a game: what was played stays a record

Status: accepted (2026-11-01)

## Context

Tic-Tac-Toe and Connect Four were among the first games, there to prove the engine. The bots and the
other games now show everything they did, and they are unlikely to be played. Taking them out raises
a question every later removal will raise too — a data pack uninstalled, a game moved to a private
build: what becomes of the games already played? `games.game_def_id` is a bare string, and without
the definition the server answered every request about such a game with an error, left unfinished
ones `active` forever, and the game page could not open. Decided with the plugin plan
(`docs/history/game-plugins-plan.md`, on the plans branch).

## Decision

- **A retired game is listed by id and name** (`app.RetiredGames`, `catalog.Retire`). A game derived
  from it (`connect-four~pie`) is retired with it. A game that is registered is never retired, so a
  test can still register the old definition.
- **Its unfinished games end when the server starts**: waiting, in a lobby or in play, they become
  finished with no winner and the end reason `retired`. Finished games keep their result. The time of
  the last activity is not changed, so retention treats them as before.
- **Every game of it stays readable as a record.** `GET /api/games/{code}` answers `retired: true`,
  the game's old name, the players, the result and the number of moves; `GET .../moves` still lists
  each move with its notation, which the store has always kept. The position cannot be drawn without
  the definition, and is not.
- **Nothing more can be done in it.** Every other request about the game is refused with **410 Gone**
  and says which game was retired (`session.ErrGameRetired`, through one lookup, `compiledFor`, that
  replaced eighteen copies). A game whose kind is simply unknown is now 404 rather than 400.
- **The game page shows the record**: the name, the players, how it ended and the moves, with a way
  back to the lobby — never an error.
- **"My games" names it** by its old name.
- Tic-Tac-Toe and Connect Four are retired. Their definitions live on in `tests/testdata` under their
  old ids: Tic-Tac-Toe is the smallest complete game, which many tests play to the end, and Connect
  Four is the only game that drops its pieces (gravity). Their rules pages, diagrams and twelve
  Connect Four puzzles go.

## Consequences

- Removing a game no longer strands its players: the plugin plan's uninstall reuses all of this.
- A record is less than a replay. Showing the board of a retired game would need its definition kept,
  which is exactly what retiring it gives up.
- An unfinished game of a retired kind ends without asking anyone; the changelog says so.
