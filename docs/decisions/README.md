# Architecture Decision Records

Short records of choices made for the `bingle` project during the requirements interview (see `docs/architecture.md` for how they fit together). Each ADR is context + decision + consequences, kept intentionally short. Read these before reopening a choice — if a constraint changes, add a new ADR that supersedes the old one rather than editing history.

| # | Decision |
|---|----------|
| [0001](0001-json-dsl-with-go-escape-hatch.md) | Hybrid game definition: JSON DSL + Go escape hatch |
| [0002](0002-json-over-yaml-for-dsl.md) | JSON over YAML for the DSL (superseded by 0075) |
| [0003](0003-board-as-graph-model.md) | Board modeled as a generic graph, not a fixed grid |
| [0004](0004-rest-and-websocket-transport.md) | REST + WebSocket transport |
| [0005](0005-room-code-matchmaking.md) | Room codes over a public lobby |
| [0006](0006-name-only-player-identity.md) | Name-only player identity, no session token (superseded by the accounts plan, ADR 0067–0073) |
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
| [0018](0018-undo-resign-and-bots.md) | Undo (replaced by 0083), resign and built-in bots |
| [0019](0019-training-environments.md) | In-memory gym-style environments for training |
| [0020](0020-move-time-limits.md) | Optional per-move time limits, enforced lazily |
| [0021](0021-custom-start-positions-and-takebacks.md) | Custom start positions, and take-backs with consent (replaced by 0083) |
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
| [0055](0055-the-order-of-a-hand.md) | The order of a hand: sorted by suit and value in the view, rearranged by dragging in the browser |
| [0056](0056-the-pie-rule-as-a-setup-choice.md) | The pie rule as a setup choice: the `pie_rule` attribute, the catalog's own swap action and deriver |
| [0057](0057-the-game-clock.md) | The game clock: a store of time per seat in one JSON column, charged by the move, with the forfeit on the existing lazy enforcement |
| [0058](0058-all-games-who-is-here-and-the-icons.md) | All the games of the server in a second tab, who is here (a browser's secret and the address it hashes to), the profile as a window, and the icons of the bar |
| [0059](0059-direct-messages-and-their-calls.md) | Direct messages addressed to a browser and signed by its secret, kept in memory only, with a one-to-one voice call and one voice per person at a time |
| [0060](0060-the-analysis-tab.md) | The Analysis tab: bot batches generalised to every game, win rates per seat and per bot with intervals, and an embedded guide to the bots |
| [0061](0061-courtfall-odds-and-the-standard-arena.md) | Courtfall: a tested table of the odds of a dice pool, and a standard arena as a setup option (`courtfall~arena`) for quick games and for batches |
| [0062](0062-meridians.md) | Meridians: a centreless hexagon from the generalised hexhex generator, and a ruleset where a group with no path to a friend dies |
| [0063](0063-what-a-log-line-may-say.md) | What a log line may say: route patterns not paths, a room code as two characters and a hash, and a test that enforces it |
| [0064](0064-metrics-on-a-second-listener.md) | Metrics: `prometheus/client_golang` on a registry of our own, a private listener with a token off loopback, labels as a whitelist, and the histogram buckets chosen once |
| [0065](0065-the-connection-must-be-secure.md) | The connection must be secure: TLS, a generated certificate or a trusted proxy; the default bind becomes loopback, and a trusted proxy turns every loopback exemption off |
| [0066](0066-mycelium.md) | Mycelium: a confluence as one decision per spread and per hyphae conversion, curved strands found by a bounded exact search, and the spent cells that let a cluster spread once a phase |
| [0067](0067-principals-and-guest-keys.md) | Principals and guest keys: a seat is its owner's, resolved once per request into the context; anonymous requests keep unowned seats; seat tokens retired |
| [0068](0068-accounts-sessions-and-passwords.md) | Accounts, server-side sessions and Argon2id passwords; signing up with a confirmed address; the guest becomes the account; the seeded administrator changes its password first |
| [0069](0069-second-factor-and-lockout.md) | TOTP on the standard library with replay protection, ten recovery codes, the admin group's second factor (waived on genuine loopback), backoff, and resetting a forgotten password by mail |
| [0070](0070-oidc.md) | Generic OpenID Connect sign-in (x/oauth2, go-oidc): PKCE, a state bound to the browser, a nonce; links by address only when the provider and the server both vouch for it |
| [0071](0071-capabilities-modes-and-tokens.md) | The capability matrix seeded from YAML and owned by the database, hybrid and closed, scoped API tokens that never carry admin powers, per-class limits, invitations |
| [0072](0072-identity-in-the-interface.md) | Handles reserved for their accounts and held after a rename, seat and chat markers, "My games" on the server, and the guest key copied to another device |
| [0073](0073-account-only-features.md) | Direct messages, who is here, the global chat and notifications are accounts'; confirmed contacts grandfathered and offered the way to an account; ADR 0006 retired |
| [0074](0074-a-qr-code-of-our-own.md) | The second factor's QR code is drawn by `internal/qr`, a byte-mode, level-M, version 1–10 encoder of our own, rather than a dependency |
| [0075](0075-yaml-definitions-and-includes.md) | Definitions written in YAML and loaded as JSON; `!include` within a game's own files, anchors, unknown fields refused with file and line, a generated JSON Schema, `bingle dsl convert` |
| [0076](0076-retiring-games.md) | Retiring a game: unfinished games end at startup with no result, every game stays readable as a record (410 for anything more), the page shows it; Tic-Tac-Toe and Connect Four retired, kept as test fixtures |
| [0077](0077-tutorials-are-data-played-by-the-engine.md) | Tutorials are YAML in the game's files, positions given as moves, rules/strategy/variant kinds, a seed for games with chance, every tutorial replayed against the engine in the tests |
| [0078](0078-hex-board-styles.md) | A hex board is drawn as hexagons or as a grid of points: the player's choice per game family, kept in the browser, with `board.style` as the default |
| [0079](0079-clock-increments-and-restarts.md) | The clock gains an increment that taking a move back removes, and a restart costs nobody time, on a clock or a per-move limit |
| [0080](0080-rated-games-and-results.md) | Rated games: Glicko-2 per family, chosen at setup, written after the game ends from a mark made in the transaction that ends it, and kept apart from the games retention deletes |
| [0081](0081-clock-minutes-and-game-defaults.md) | The setup takes 1 to 60 minutes and keeps two long times, and a game's definition can suggest a time, which never turns a clock on |
| [0082](0082-correspondence-tournaments.md) | Correspondence tournaments: round robin and Swiss, a sweep instead of a scheduler, results copied to the tournament's own tables, and turn mail by default |
| [0083](0083-a-move-stands.md) | A move stands once the turn has passed: take-backs are gone, Back takes back steps of the turn still being played, and a move that cannot come back waits for Submit in every game (partly replaced by 0114) |
| [0084](0084-tutorial-pictures-need-not-be-reachable.md) | A tutorial's `pieces` draws any board the game can draw, reachable or not, and `illegal` marks what must not be done |
| [0085](0085-card-sets-belong-to-the-table.md) | A themed set of cards is chosen when a game is set up and is the same for everyone at the table; its art and its lettering are drawn in code |
| [0087](0087-the-setup-shows-the-variant-chosen.md) | Variants are chosen from a list; a table of cards shows a game in progress from its own rules; a tutorial can belong to one variant |
| [0086](0086-games-for-one-player.md) | A game can have one seat: it starts at once, moves never wait for Submit, every move can be taken back, and only clearing the table is a win (narrowed by 0114) |
| [0088](0088-connection-replaces-presence.md) | A seat is connected while a socket that joined for it is open: server pings, a grace period, and the opt-in presence mark gone |
| [0089](0089-real-time-games.md) | A real-time game is chosen at setup; without a timer the slow player and then the table are warned, and another connected player may have a bot play the turn |
| [0091](0091-taking-over-a-seat.md) | A bot can take a seat for good: by a vote in a real-time game, or when time runs out at a table of three or more, and the clock now works at any number of seats |
| [0090](0090-real-time-tournaments.md) | A real-time tournament starts by itself at its start time: check-in, one game of a pairing at a time, a break between rounds, and a first-move deadline where a no-show forfeits |
| [0092](0092-a-game-page-that-fits-the-screen.md) | A game page is as tall as the window and never scrolls: the board fills one side, the cards scroll beside it, and on your turn the actions are pinned in view; Blooms first |
| [0116](0116-effects-at-the-end-of-a-turn.md) | Effects at the end of a turn: `end_of_turn` removes the groups with no liberties of the mover's opponents, credits a pool, and can tint them (proposed) |
| [0117](0117-groups-by-colour-enemies-by-seat.md) | Groups by colour, enemies by seat: a seat may play several `colors`; groups stay one colour, ownership goes by seat |
| [0115](0115-a-declared-turn-of-several-selections.md) | A declared turn may have several selections: parts with a `min`, a `max` and `distinct`, moved through by choosing, and `done` to stop |
| [0114](0114-impulses-selections-moves-and-gestures.md) | An impulse is what one seat chooses before the game needs another seat or chance, submitted at once; selections are its parts, a move its data and gestures the interface's, and a selections query lets bots and clients walk it |
| [0113](0113-the-api-describes-itself.md) | The API says where to start at `GET /api`, and serves its own docs |
| [0112](0112-legal-moves-go-to-whoever-holds-the-seat.md) | In a game with secrets, legal moves go to whoever holds the seat |
| [0111](0111-a-tutorial-points-at-the-page-by-name.md) | A tutorial points at the page by name, not by selector |
| [0110](0110-branding.md) | An instance can have a name and an icon of its own, set at start |
| [0109](0109-scheduled-games.md) | Scheduled games: a lobby that stays open, invitations with calendar files, and agreeing the earliest common time |
| [0108](0108-a-great-old-one-emerges.md) | A Great Old One's first awakening is a show on the map, its art procedural SVG, with a badge in the history to replay it |
| [0107](0107-who-can-find-a-game.md) | Seats kept for friends, friends-only and invite-only games: lists and seats are restricted, not the game |
| [0106](0106-presence-can-be-hidden.md) | An account can choose not to be seen online: hidden from friends and from *Here now* alike |
| [0105](0105-friends.md) | Friends: requests, messages only between friends, stored invitations to games |
| [0104](0104-the-setup-asks-for-no-name.md) | The setup asks for no name, and Cthulhu Wars' setup has titled sections, with a card for each sand-timer spellbook |
| [0103](0103-one-history-card.md) | The history is one card with one set of step buttons, written out in words or as notation, and not two tabs |
| [0102](0102-joining-and-reading-back.md) | Join Game joins or watches by itself, under the profile's name; unread chat is counted by the browser; the rule of the History tab goes to your last move in the Moves tab |
| [0101](0101-the-x-resigns-and-games-are-found-where-they-are-wanted.md) | The ✕ of an active game resigns from it, after asking; a game's games are in its setup, and tables looking for players in Join Game |
| [0100](0100-a-move-gathers-its-units.md) | Cthulhu Wars: the page gathers the units of a Move from one area and sends them to one destination, and a Move always waits for Done (partly replaced by 0114) |
| [0099](0099-impulses.md) | The seat to act is the impulse seat, everywhere, and the impulse card names it over what it is asked to choose, for everyone (partly replaced by 0114) |
| [0098](0098-prototype-games-and-rules-versions.md) | Prototype games: each version of the rules is a folder holding the whole game and a game of its own, `family-version`; the family's name means the newest, and a game keeps the version it started with |
| [0097](0097-every-map-is-traced.md) | Cthulhu Wars: every Earth and Primeval board up to five players is traced, the two four-player Primeval boards become layouts, and each board's own print decides its borders and Glaciers |
| [0096](0096-earth-glyphs-and-starts-follow-the-board.md) | Cthulhu Wars: Earth's glyphs and the Black Goat's and Crawling Chaos' starts follow the printed boards |
| [0095](0095-a-map-traced-from-a-scan.md) | A map may be traced from a scan and have a generated picture behind it, with no pixel of the scan in the repository |
| [0094](0094-the-cthulhu-wars-setup-picker.md) | Cthulhu Wars' setup is a picker of single items, each still an option of the create call; saved setups are an account's, shared by choice |
| [0093](0093-cthulhu-wars-later-cards-and-the-primeval-map.md) | Cthulhu Wars: the later Loyalty Cards share one action, and the Primeval Map is drawn by hand |
