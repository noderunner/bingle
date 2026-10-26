# Architecture Decision Records

Short records of choices made for the `bingle` project during the requirements interview (see `docs/architecture.md` for how they fit together). Each ADR is context + decision + consequences, kept intentionally short. Read these before reopening a choice — if a constraint changes, add a new ADR that supersedes the old one rather than editing history.

| # | Decision |
|---|----------|
| [0001](0001-json-dsl-with-go-escape-hatch.md) | Hybrid game definition: JSON DSL + Go escape hatch |
| [0002](0002-json-over-yaml-for-dsl.md) | JSON over YAML for the DSL |
| [0003](0003-board-as-graph-model.md) | Board modeled as a generic graph, not a fixed grid |
| [0004](0004-rest-and-websocket-transport.md) | REST + WebSocket transport |
| [0005](0005-room-code-matchmaking.md) | Room codes over a public lobby |
| [0006](0006-name-only-player-identity.md) | Name-only player identity, no session token |
| [0007](0007-spectator-support.md) | Read-only spectators via room code |
| [0008](0008-full-move-history-persistence.md) | Full move-history persistence |
| [0009](0009-sqlite-database.md) | SQLite over PostgreSQL |
| [0010](0010-async-turn-timing.md) | Async turn timing, no enforced clock |
| [0011](0011-generic-mcp-server.md) | Single generic MCP server, not per-game |
| [0012](0012-optional-tls.md) | TLS optional, off by default |
| [0013](0013-net-http-plus-lightweight-frontend.md) | net/http + lightweight frontend, no SPA build |
| [0014](0014-single-binary-process-model.md) | Single binary, goroutines instead of multiple processes |
| [0015](0015-engine-v2-stacks-and-actions.md) | Engine v2: stacks, reserves and declarative actions |
| [0016](0016-compatible-state-views.md) | Position document with a compatible legacy view |
| [0017](0017-notation-and-move-steps.md) | Canonical notation and click-by-click move steps |
| [0018](0018-undo-resign-and-bots.md) | Undo, resign and built-in bots |
| [0019](0019-training-environments.md) | In-memory gym-style environments for training |
| [0020](0020-move-time-limits.md) | Optional per-move time limits, enforced lazily |
| [0021](0021-custom-start-positions-and-takebacks.md) | Custom start positions and take-backs with consent |
| [0022](0022-slides-and-compound-moves.md) | Slides and compound moves |
| [0023](0023-model-observations-and-puzzles.md) | Model-ready observations and proven puzzles |
| [0024](0024-hints-imports-and-http-environments.md) | Hints, game import and environments over HTTP |
| [0025](0025-go-rulesets-and-hidden-information.md) | Go rulesets, chance and hidden information |
| [0026](0026-area-maps-and-rules-guides.md) | Area maps, more than two seats and rules guides |
| [0027](0027-reactions-lobbies-and-chat.md) | Reactions with auto-pass, lobbies with invite links, chat |
| [0028](0028-derived-games-and-setup-options.md) | Setup options and games derived from a definition (cults at the table, expansions) |
| [0029](0029-game-options-window-and-solo-seats.md) | The game options window; playing several seats of a table oneself |
| [0030](0030-gate-occupants-are-units-and-action-cards.md) | The unit on a Gate is a unit of its own; every action button has a price and a card |
| [0031](0031-free-step-off-oath-asked-and-spellbook-marks.md) | A free step-off, the Oath asked after every action, spellbook marks, captured units |
| [0032](0032-desecrate-rolls-first-and-locks-in.md) | Desecrate rolls first and the unit is chosen after; a rolled die locks the move in |
| [0033](0033-first-player-reactions-reversed-and-auto-play.md) | The first player of the game, reactions passed by default (and "when I'm targeted"), the Auto-Play Timeout, cults chosen in the lobby |
| [0034](0034-practice-batches-and-metrics.md) | Practice batches: a metrics observer, stored per-game records, reports |
| [0035](0035-phone-layout-for-cthulhu-wars.md) | A phone layout for Cthulhu Wars: swipeable full-screen views, a bottom bar, an automatic or forced choice |
| [0036](0036-fist-one-move-is-one-turn.md) | Fist: a Go ruleset where one move is one whole turn; extra piece colours |
| [0037](0037-onitama-cards-in-the-view.md) | Onitama: the cards travel in the view; Red always starts |
| [0038](0038-blooms-two-stage-turns.md) | Blooms: a turn of one or two placements, fenced blooms captured at the end of the turn |
| [0039](0039-hive-on-a-large-hex-field.md) | Hive: tiles on a large hex field, with the expansion insects |
| [0040](0040-fendo-fences-in-the-view.md) | Fendo: the fences travel in the view and are drawn on the board edges |
| [0041](0041-hive-without-a-board.md) | Hive without a board, in the Universal Hive Protocol (and rules pages with their own endings) |
| [0042](0042-yinsh-rings-and-rows.md) | Yinsh: ring moves, rows, and the opponent's decisions inside a turn |
| [0043](0043-card-games.md) | A card framework (decks in the DSL, zones, hidden hands, a card table) and Nyet! on it |
| [0044](0044-azathoth-neutral-spellbooks-and-monsters.md) | Cthulhu Wars: the Azathoth faction's neutral spellbooks and Monsters, and neutral books in place of timer books |
| [0045](0045-opener-of-the-way-and-tcho-tcho.md) | Cthulhu Wars: the Opener of the Way and the Tcho-Tcho |
| [0046](0046-loyalty-cards-terrors-and-independents.md) | Cthulhu Wars: Loyalty Cards (neutral Monsters, Terrors), the Independent Great Old Ones, the new packs and the faction glyphs |
| [0047](0047-home-page-tabs-profile-and-global-chat.md) | The home page: tabs, top-bar icons, a profile page, a global chat and fun facts |
| [0048](0048-optional-native-tls.md) | Optional native TLS with automatic Let's Encrypt certificates |
| [0049](0049-voice-chat.md) | Voice chat: a WebRTC mesh with signalling over REST and the room socket |
| [0050](0050-email-notifications.md) | Email notifications: contacts, seat tokens, TLS-only SMTP |
| [0051](0051-cthulhu-wars-capture-choice-and-neutral-window.md) | Cthulhu Wars: capturing the Cultist on a Gate, a window of the neutral items, glyphs off without Yellow Sign |
| [0052](0052-calls-across-pages-top-bar-and-per-reaction-choices.md) | A voice call that outlives the page, the top bar of a game, and a choice for each reaction |
| [0053](0053-game-versions-beta-rules-pages-and-themes.md) | Game versions and the Beta mark, one Rules page for the abstract games, coordinates outside themed frames |
| [0054](0054-changelog-nyet-table-and-removing-the-demo.md) | A changelog in the help window, Nyet!'s table (classic suits, animals, pictures for the board, phone views) and the Demo game removed |
