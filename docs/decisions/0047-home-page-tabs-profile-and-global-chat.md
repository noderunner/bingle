# 0047. The home page: tabs, top-bar icons, a profile page and a global chat

Status: accepted (2026-10-25)

## Context

The home page had grown into a column of cards: a title and tagline, *Your games*, the gallery, the create form beside the join form, puzzles, practice batches, *Watch bots play* and *Import a game*, plus zoom buttons floating over the Cthulhu Wars map preview. The player asked for it to be cleaned up.

## Decision

- **One tabbed card** (*Games*, *Puzzles*, *Global Chat*). Picking a game in the gallery swaps the gallery for that game's **setup** (the former create form, with a preview and the variants as chips); `?game=ID` opens it directly. The form's `<select id="game_def_id">` stays as a hidden data holder so the slot/option code is unchanged.
- **Top-bar icons** replace cards: *Active games* (a dialog in the layout of the old *Your games*, unfinished games only, with a badge counting the games waiting for you), *Join a game* (a dialog) and the profile. Finished games moved to the **profile page** (`/profile`).
- **Watch bots and Import became options of the setup**: *Two bots* is an opponent of every two-seat game (it uses `POST /api/bot-matches`), and *Start from: a move log* (paste or upload) uses `POST /api/games/import`. Tables of three or more seats cannot be all bots (the creator keeps a seat) and Cthulhu Wars cannot be imported, so neither option is offered there.
- **Profile settings** (name, email, notification switch) are kept in `localStorage` (`bingle.name`, `bingle.profile`). There are no accounts, and the email notifications are still only a plan: the address and the switch are stored and never sent anywhere, and the page says so.
- **Global chat**: its own table (`global_chat`, no game), `GET|POST /api/chat`, and a hub room (`session.GlobalChatRoom`, `~global`, which no room code can equal) fed to the page by `GET /ws-chat`. It shares the per-name rate limiter and message limits of the game chat and keeps the latest 500 lines. Anyone can write under any name — like a spectator in a game chat.
- **Fun facts** replace the title (`js/facts.js`); the Tak position bounds in them were computed (see `docs/ui.md`), not quoted.
- The map **zoom buttons** are not drawn on previews (`view.preview`), only on a game's own map.

## Consequences

Tests that used the old cards were rewritten (`tests/app/home_page_test.go`, `import_test.go`, `puzzles_test.go`, `web_*`). Names in the global chat are not authenticated, so it should not be trusted for identity.
