# Glossary

The platform's own words, and what each means, for whoever translates or reviews a language (ADR 0141). A game's
own words (its pieces, seats and actions) are its own, and are not here. Each term names the catalog key whose
words show it; each language's file gives its word for every term, as that key shows it, and a test
(`TestGlossary_EachLanguageHasEveryTermAsItsCatalogShowsIt`) checks that the catalog says it, and that two terms
share a word only where the file says so. Change a term here and in the catalogs together.

Languages: [Español](es.md), [Français](fr.md), [Deutsch](de.md), [Português (Brasil)](pt-BR.md), [中文（简体）](zh-Hans.md), [日本語](ja.md), [한국어](ko.md), [Русский](ru.md), [العربية](ar.md).

| Term | Key | Meaning |
|---|---|---|
| game (a kind) | `rating.game` | A kind of game: Tak, Hex, a variant of one. |
| game (one played) | `batch.col_game` | One game played at a table, from its lobby to its end. |
| room | `lobby.room` | A game's address: its room code, which a link or a friend's invitation carries. |
| lobby | `games.in_lobby` | Where a game waits for its players to take their seats before it starts. |
| seat | `game.seat_n` | A place at a game's table, which a player, a bot or nobody holds. |
| bot | `game.bot` | A program that plays a seat. |
| guest | `game.guest` | Somebody playing without an account, known to this browser alone. |
| spectator | `chat.spectator` | Somebody watching a game without a seat. |
| turn | `game.let_bot_play` | The time a seat has to act, until play passes to another. |
| impulse | `game.impulse` | The choices one seat makes before the game needs something from elsewhere (ADR 0114). |
| move | `game.move_n` | The choices of one impulse, as the move list numbers them (ADR 0114). |
| ply | `game.ply_n` | One impulse, counted from the start of the game (ADR 0114). |
| round | `game.round_n` | A round of a game that has rounds: each seat's turns until play comes back to the first. |
| reaction | `options.reactions` | An ability used on another player's turn, in answer to what they did. |
| Auto-Play Timeout | `autoplay.title` | The setting that has a bot play a person's turn when they take too long. |
| draw | `options.offer_draw` | A game's end with no winner, as players agree to it or the rules give it. |
| resign | `options.resign` | To end a two-player game by giving it to the opponent. |
| hint | `options.hint` | A bot's suggestion of a move, asked for in a game. |
| history | `game.history` | The story of a game so far, its moves in words or in notation. |
| notation | `hist.notation` | How a game writes its moves down (PTN, coordinates). |
| analysis | `home.analysis` | Trying moves out without playing them, and the tab of bot batches and reports. |
| rules | `setup.rules` | A game's rules, in its rules window. |
| variant | `setup.variant` | A game with some of its rules or its board changed. |
| seed | `setup.seed` | The number a deal of cards comes from, which deals it again. |
| tutorial | `home.tutorial` | A game's guided lessons, played on its own board. |
| lesson | `tutorial.lesson_of` | One step of a tutorial. |
| puzzle | `home.puzzles` | A position with one best line to find. |
| muster | `tour.muster_title` | Finding a time for a game: the times offered, and who can make which. |
| practice batch | `setup.practice` | Many games of bots against each other, run to compare them. |
| badge | `settings.badges` | An award for something done in a game. |
| rating | `settings.rating` | A player's strength at one game, a number that moves with each rated game. |
| rated | `game.rated` | A game whose result changes its players' ratings. |
| real-time | `game.real_time` | A game whose players stay until it ends, each on a clock. |
| tournament | `home.tournaments` | A set of games between entrants, with standings. |
| standings | `tourney.standings` | The order of a tournament's entrants, by their scores. |
| friends | `settings.friends` | The people an account has added, whom it can message, call and invite. |
| global chat | `home.global_chat` | The chat every signed-in player shares. |
| invitation | `admin.invitations` | A link that lets somebody sign up, or an invitation to a game. |
| account | `common.your_account` | What a person signs in with; a guest has none. |
| username | `settings.username` | An account's handle: @ana. |
| two-step sign-in | `account.two_step` | Signing in with a code from an app as well as the password. |
| recovery code | `account.new_codes` | A one-time code that stands in for the app's. |
| API token | `account.tokens` | A key that lets a program act as the account. |
| cards | `setup.cards` | The set of cards a table plays with: the game's own or a themed set. |
| board theme | `options.board_theme` | How a board looks: wood, slate, an illustrated theme. |
| options | `options.button` | A game's window of settings, tools and its link. |
