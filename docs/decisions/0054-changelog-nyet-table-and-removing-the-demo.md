# 0054. A changelog in the help window, Nyet!'s table, and the Demo game removed

Status: accepted (2026-10-26)

## Context

`bingle_tasks.md` (the owner's list of coding tasks, now the way work is handed to the agent that
writes much of this code; the file itself is git-ignored) asked for: a changelog the players can
read in the app, with a mark on the help icon while it holds something they have not seen; a
designer and publisher for Fist; the Demo game gone; and a long list of changes to how Nyet! looks
and plays on the page, with a bug (a game in which the first player was never offered the team).

## Decision

**The changelog.** `CHANGELOG.md` at the root is built into the binary (`main.go` embeds it and
hands it to the app in `config.Config.Changelog`; it is not read from the environment).
`GET /api/changelog` returns its newest release, `{version, date, markdown}`: the first heading of
the form `## 0.4.0 - 2026-10-26` and the Markdown up to the next one. The help window shows it under
*Recent changes*, rendered with the rules renderer, which is loaded only when the window opens.
A dot on the help icon says there is an entry this browser has not shown: opening the window stores
the entry's version in `localStorage` (`bingle.changelog.seen`), so a new release brings the dot
back. There are no accounts, so "the current user" is the browser; a new browser sees the dot once.
`.containerignore` and `.dockerignore` leave out `*.md`, which would have kept the file out of the
image's build context and broken the embed, so `!CHANGELOG.md` is added. Each entry says which
versions moved: the engine's (`internal/version`) and the games'.

**The Demo game is gone** from the catalog, the game list, the docs and the scripts. It was the one
small game whose every move and starting position the platform tests know, so those tests (REST,
MCP, store, DSL) keep its definition as `tests/testdata/freeplay.json`, id `test-freeplay`, which
the test servers register and the real server never does. Nothing in a database is affected: no game
of it had been created on the live server.

**Nyet!'s table.**
- *Classic suits.* The deck's four colours are drawn as spades, hearts, diamonds and clubs (red
  hearts, blue spades, yellow diamonds, green clubs), with the rank in two corners and the pips of
  the value (`js/card-art.js`; 11–13 are framed, the three 1s of a suit stand in a ring). Only what
  a player reads and sees changed: the card ids (`red-9`), the saved positions, the notation of the
  moves and the engine keep the colours' names, so a stored game, a move history or an agent that
  plays over MCP keeps working. The mapping is fixed (`suits.go`), the notation help says so, and
  every sentence the game says (the history, the prompts, the rules, the guide) names suits. The
  alternative, renaming the ids, would have needed a migration of every saved position and a second
  name for every card in notation; it can still be done later behind the same view.
- *Players are animals.* Wolf, Crane, Tiger, Bear and Eagle are drawn in the colour of their seat
  (`js/animals.js`, the seat colours of the theme, so they follow light and dark): beside the name in
  the list of players, under the card each played into the trick, and in the box of the board that
  each crossed off.
- *The other players' hands are not drawn.* The list of players on the right shows, with the points,
  tricks and loot, the number of cards in each hand (a card icon and a number; the count is public)
  and the dealer, bonus and team marks that were on the opponents' windows.
- *Your hand, twice as large.* A card of a hand that is private to its owner (a zone with the
  visibility `owner`, so every card game, not Nyet! alone) is shown at double size while the pointer
  or the focus is on it (`cards.js`).
- *The board of terms is pictures.* Every box and line head is an icon (`js/choice-icons.js`,
  named by the server in `icon`, such as `suit:red` or `discard:passleft`), in cells of one size with
  each line centred. A box that has been crossed off shows the picture of the player who crossed it
  *instead of its own*, as asked; its words stay as the tooltip (`Hearts: crossed off by Wolf`), the
  box left open keeps its own picture, and a box of a character who is not at the table is faded. On
  a wide screen the board stands beside the trick; on a phone the game is five views (play, terms,
  players, moves, chat) on the same swipeable shell as Cthulhu Wars, the terms view with a legend
  because a phone has no hover text.
- *The cards in the rules.* The Overview's *The cards* shows all sixty cards, a row for each suit,
  splayed in the order of their values. The picture (`guide/cards.svg`) is made by
  `scripts/cardsheet` with the page's own card drawing, so it cannot drift from what the game shows.
- *The bug.* The team buttons were never drawn for three or more players: `moveBar` read
  `S.info.actions.find(...)`, and a game with no actions of its own (Nyet!) sends `null`. The three
  direct reads use the null-safe `actionDef()`; a test plays a four-player game to the team choice
  and picks a team through the page.

**Fist** names James Ernest as designer and Crab Fragment Games as publisher (0.1.1).

## Interpretations

- "The Helm icon" is the help icon (the `?` in the top bar), the only icon the task could mean.
- "James Earnest" is spelled **James Ernest**, as Tak's designer already is (and as the Cheapass
  Games founder spells it).
- The classic suits are coloured the classic way, two red and two black. Hearts and diamonds differ
  by shape only, as in any deck; the board's pictures and the card faces use the same two colours.
- A crossed box no longer shows what it was; only the box left open does, which is the rule of the
  round. What was crossed is in the tooltip and in the history.
- "Cards in hand" counts only the hand, not the discards or the tricks won.
