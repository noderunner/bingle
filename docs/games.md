# Game catalog

Generated from the registered definitions by `go run scripts/gendocs/main.go`;
each game is one JSON document under `internal/games`. See [dsl.md](dsl.md) for the
vocabulary and [mcp.md](mcp.md) for the training side (planes, action IDs).

| Game | Version | Board | Seats | Actions | Ends by | Variants |
|---|---|---|---|---|---|---|
| **Amazons** (`amazons`) | 0.2.0 | 10×10 squares | 2 | slide | end/mover-wins | `amazons` `amazons-6` |
| **Ataxx** (`ataxx`) | 0.2.0 | 7×7 squares | 2 | leap | end/most |  |
| **Blooms** (`blooms`) | 0.1.0 | hexagon, 5 per side | 2 | ruleset (Go) | victory points | `blooms` `blooms-4` `blooms-6` |
| **Brandubh** (`brandubh`) | 0.1.0 | 7×7 squares | 2 | slide | absent, end/mover-wins, reach |  |
| **Breakthrough** (`breakthrough`) | 0.1.0 | 8×8 squares | 2 | step | end/mover-wins, reach |  |
| **Connect Four** (`connect-four`) | 0.2.0 | 7×6 squares | 2 | place | end/draw, line |  |
| **Courtfall** (`courtfall`) | 0.2.0 | hexagon, 4 per side | 2 | ruleset (Go) | victory points |  |
| **Cthulhu Wars** (`cthulhu-wars`) | 0.1.0 | area map, 17 areas | 4 | ruleset (Go) | doom | `cthulhu-wars` `cthulhu-wars-3-5` `cthulhu-wars-3p` `cthulhu-wars-5p` |
| **English Draughts (Checkers)** (`draughts`) | 0.1.0 | 8×8 squares | 2 | jump, step | end/draw, end/mover-wins |  |
| **Fendo** (`fendo`) | 0.1.0 | 7×7 squares | 2 | ruleset (Go) | victory points | `fendo` `fendo-11` `fendo-9` |
| **Fist** (`fist`) | 0.1.1 | 8×8 squares | 2 | ruleset (Go) | victory points |  |
| **Go** (`go-9x9`) | 0.2.0 | 9×9 points | 2 | pass, place | end/score | `go-13x13` `go-19x19` `go-9x9` |
| **Gomoku (15×15)** (`gomoku`) | 0.2.0 | 15×15 points | 2 | place, swap | end/draw, line |  |
| **Havannah** (`havannah-8`) | 0.2.0 | hexagon, 8 per side | 2 | place | connect, end/draw, ring | `havannah-10` `havannah-4` `havannah-6` `havannah-8` |
| **Hex** (`hex-11`) | 0.2.0 | hex 11×11 | 2 | place, swap | connect | `hex-11` `hex-13` `hex-7` `hex-9` |
| **Hive** (`hive`) | 0.1.0 | hexagon, 10 per side | 2 | ruleset (Go) | victory points | `hive` `hive-base` |
| **Meridians** (`meridians`) | 0.1.0 | hexagon, 6 and 7 per side | 2 | ruleset (Go) | victory points | `meridians` `meridians-5` `meridians-7` |
| **Nyet!** (`nyet`) | 0.2.0 | 1×1 squares | 4 | ruleset (Go) | victory points | `nyet` `nyet-2` `nyet-3` `nyet-5` |
| **Onitama** (`onitama`) | 0.1.0 | 5×5 squares | 2 | ruleset (Go) | victory points |  |
| **Pente** (`pente`) | 0.2.0 | 13×13 points | 2 | place | end/draw, line, pool |  |
| **Reversi (Othello)** (`reversi`) | 0.2.0 | 8×8 squares | 2 | place | end/most |  |
| **Tak** (`tak-5x5`) | 0.1.0 | 5×5 squares | 2 | move-stack, place | connect, end/most | `tak-3x3` `tak-4x4` `tak-5x5` `tak-6x6` `tak-7x7` `tak-8x8` |
| **Tic-Tac-Toe** (`tic-tac-toe`) | 0.1.0 | 3×3 squares | 2 | place | end/draw, line |  |
| **Yavalath** (`yavalath`) | 0.2.0 | hexagon, 5 per side | 2 | place | end/draw, line, line (lose) |  |
| **Yinsh** (`yinsh`) | 0.1.0 | graph, 85 nodes | 2 | ruleset (Go) | victory points | `yinsh` `yinsh-blitz` |

Variants are separate games derived from the base document by a JSON merge patch.

**Version** is the game implementation's semantic version (`version` in its JSON). Raise it with
every change to the game's definition or Go code; a version below 1.0.0 is shown as a Beta in the
list of games. The server records each version it has run in the `game_versions` table.
