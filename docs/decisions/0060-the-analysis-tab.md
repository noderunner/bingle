# 0060. The Analysis tab: batches for every game, and a guide to the bots

Status: accepted (2026-10-27)

## Context

`bingle_tasks.md` asked for "an *Analysis* tab in the main game page" to be "the main UI for
kicking off automated bot batches for games and viewing reports for all games that support this
feature", and for a documentation section on it: "a crash course in the differences between the
tactical, minimax, and mcts bots, a lightweight explanation of the central algorithms they use, and
what advanced users and game designers should know about training them and how to use them for game
analysis", with diagrams, references, and a ceiling of "20 pages of an equivalent textbook".

Batches existed, but only for Cthulhu Wars: the wizard lived on that game's lobby page
(`js/practice.js`), the spec named a cult per seat, the runner was watched by
`cthulhuwars.Observer`, and the report was all Doom, Gates and spellbooks. "All games that support
this feature" therefore meant one game.

When asked where the tab belonged, the owner chose the **home page**, beside *Games*, *Puzzles* and
*Global Chat* — which is right: a batch is not about a room, and you want to start one before any
game exists.

## Decision

**Every game can be batched.** The runner was already generic — it asks the engine for the legal
moves and applies them, as the bots do — so three things were separated from Cthulhu Wars:

* a `Player` names a cult only where the seats *are* a choice; a spec either names one for every
  seat or for none, and anything in between is refused (as is `high_priests` on another game);
* the derived-table id (`base~gc-cc-…`) is built only for such a table; every other game is played
  as itself, with the bots moved round its seats;
* `cthulhuwars.Observer` watches Cthulhu Wars alone, and the outcome every game has — who won, why
  it ended, how long it ran — is now carried by the `Record` itself and stored from there.

The report gained two sections every game has: **by seat** (where the advantage of moving first
shows) and **by bot** (whichever seat it played, which is the question "is minimax better *here*").
The cult report is unchanged and simply empty for other games, so the deep Cthulhu Wars page at
`/batches/{id}` keeps working exactly as it did.

**The tab** (`js/analysis.js`, built the first time it is shown) is the form, the list of batches,
and the report of the one picked, with a link to the full report for a game that has one. Win rates
are drawn with a **95% Wilson interval** and a marker at an even share, because the first thing
anybody does with 20 games is believe them: an interval that straddles the line says *play more
games*. `js/practice.js` keeps its place on the Cthulhu Wars lobby, where a wizard that knows about
cults and spellbook tuning belongs.

**The guide** is one Markdown document embedded in the binary
(`internal/web/botguide/bots.md`, `GET /api/bot-guide`), rendered by the same renderer as the rules
guides, with three drawings made by `scripts/botdiagrams.py` (an alpha-beta cut-off, the four steps
of MCTS, and what each bot spends its effort on) in the style the game guides use — self-contained
SVG with opaque colours, so they read the same light or dark. About 2 000 words, a tenth of the
ceiling asked for: it teaches the four bots, alpha-beta and UCT, which bot suits which game, how to
read a batch (including what sample size buys), what a game designer should look at in order, where
the training surface is, and seven references. Writing it as a document rather than as JavaScript
means it can be read, edited and reviewed as prose.

## Consequences

* A game designer can now measure any game here: first-player advantage, draw rate, decisiveness
  and length, with one seed so a rule change is the only thing that moved.
* The batch manager still runs one batch at a time on three quarters of the cores; a batch of MCTS
  games is minutes, of tactical games seconds, which the tab says.
* The guide is served, not built into the page, so it can grow without the home page growing.
