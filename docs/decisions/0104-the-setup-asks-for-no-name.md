# 0104. The setup asks for no name, and Cthulhu Wars' sand-timer spellbooks are options of their own

Status: accepted (2026-11-04)

## Decision

**A game is played under the name in the profile.** The boxes for a name are gone from the setup, the puzzles and the tutorials. Every
browser has a name from its first page (a guest's is made up, ADR 0101's Join Game uses the same one), and it is changed in the
profile window. The API still takes `player_name`: this is the page's choice, not the server's.

**The setup of Cthulhu Wars has three titled sections:** Expansions, Seats and Other Options, the last as cards in two columns. Other games
keep their form as it was. Tutorials and practice batches are tabs of the setup rather than parts of its page.

**A sand-timer spellbook is an option of the table,** with a card among the Other Options for each one at the table, rather than a list in the
Other tab. A slot of a `book-swap` may name the choice it recommends (`recommended`).

## Consequences

- A person who wants another name for one game changes their profile first, and a game in progress keeps the name it was joined under.
