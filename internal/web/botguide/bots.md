# The bots, and what to do with them

Bingle ships four built-in opponents. None of them knows anything about any particular game: they
ask the engine for the legal moves of a position, try some of them, and answer with one. That is
why they work for every game here, and why they are useful for studying a game — a bot that knew
Hex would tell you about Hex, while these tell you about *your* game.

![What each bot spends its effort on|720](bots.svg)

## The four bots

| Bot | What it does | Costs | Good for |
|---|---|---|---|
| **random** | Picks a legal move with no thought at all. | Nothing. | The baseline. A bot that cannot beat random does not work. |
| **tactical** | Plays a move that wins at once if there is one, avoids moves that let the opponent win at once, and otherwise picks randomly among what is left. | One move deep. | A quick opponent, and the pace-setter for long batches. |
| **minimax** | Alpha-beta search with iterative deepening, up to 6 plies and 30 000 positions a move, judging a position by material and (in connection games) by the size of your largest connected group. | Tens of thousands of positions a move. | Games decided by pieces and lines: Tak, Reversi, draughts, Connect Four. |
| **mcts** | Monte Carlo tree search (UCT): thousands of random finishes of the game, spending its effort on the moves that look best so far. | A second a move by default, or a fixed number of iterations. | Games where no simple count tells you who is winning: Hex, Havannah, Blooms, Cthulhu Wars. |

Both thinking bots are **reproducible**: given the same seed and the same budget they play the same
game every time, which is what makes a batch of a few hundred games worth comparing with another.

## Minimax: looking ahead

Minimax is the old idea, and the right one when a position can be scored: *I will play the move
whose worst reply is least bad.* Expand the moves, then their replies, then the replies to those,
score the positions at the bottom, and carry the scores back up — taking the best for you at your
levels and the worst for you at the opponent's.

![A game tree with two branches cut off by alpha-beta|720](alphabeta.svg)

A full tree is hopeless: a game with 30 moves a turn has 810 000 positions three plies down.
**Alpha-beta** is what makes the search affordable. While walking the tree you remember the best
you have been promised so far (*alpha*) and the best the opponent has been promised (*beta*); the
moment a branch cannot beat that promise, the rest of the branch cannot matter and is never looked
at. In the picture, the middle reply already gives you 6, so once the right-hand branch shows a 2
the opponent will never let you have more, and the dashed positions are never visited. With a good
move order alpha-beta searches roughly the square root of the tree — twice the depth for the same
effort.

Bingle's minimax has three bounds worth knowing:

* **Iterative deepening.** It searches 1 ply, then 2, then 3, keeping the best move it has found,
  and stops when the budget runs out. So an interrupted search always has an answer, and the move
  order from the shallow search makes the deeper one faster.
* **A node budget, not a clock** (30 000 positions a move by default). A budget is reproducible
  where a clock is not: the same batch on a slower machine plays the same games, only later.
* **A material heuristic.** It counts what is on the board — your pieces on top of stacks, your
  discs, your men — and, in games with a connection goal, adds the size of your largest connected
  group. That is honest and shallow. It says nothing about shape, tempo or territory, which is why
  minimax is weak at Go and at Hex and strong at Tak and Reversi.

## Monte Carlo tree search: playing it out

MCTS answers the question minimax cannot: *what if no formula can score this position?* Instead of
judging a position, it finishes the game from there at random, many times, and counts who won. A
move that wins 70 of 100 random finishes is probably better than one that wins 40 — not because
random play is good, but because the difference between the two means something.

![The four steps of Monte Carlo tree search|720](mcts.svg)

One iteration walks down the tree it has built so far, adds a node, plays a random game out from
it, and adds that result to every node on the way home. The walk down is the clever part: at each
node it takes the child with the largest

```
wins / visits  +  c * sqrt( ln(parent visits) / visits )
```

The first term is what the child is worth so far; the second grows for children nobody has tried,
so the search keeps glancing at the moves it has neglected. That is **UCT** — upper confidence
bounds applied to trees — and `c` (1.4 here) sets how curious it is. Low `c` digs into the moves
that already look good; high `c` spreads its effort. After thousands of iterations the move visited
most often is the one played.

Bingle's MCTS: a second a move by default, or `mcts_iterations` for a reproducible batch; playouts
are cut off after 120 plies and scored as a draw; and it searches only the moves the tactical filter
considers safe, so it never has to discover that giving the game away is bad.

The honest weakness of random playouts is that random play is nothing like good play. In games
where a position's value depends on a long accurate sequence, the playouts are noise, and MCTS
needs either many more of them or a better playout policy than random (which is what a learned
network gives AlphaZero-style programs).

## Tactical, and why random matters

The tactical bot is one move deep and costs nothing: it takes a win it can see and refuses a loss
it can see. It is the bot behind *Auto-Play*, it is what makes long batches finish, and it is a
surprisingly stiff test — a great many "clever" players lose to it because they overlook a mate in
one.

Keep **random** in your batches anyway. It is the only absolute measure here: a win rate against
random tells you whether a bot (or a learned policy) has learnt anything at all, and a game in
which a thinking bot cannot beat random by a wide margin is usually a game with a bug in its rules
or a game that is mostly chance.

## Which bot for which game

The rule bingle itself follows when it offers a *Hint* is: **minimax where piece counts matter,
MCTS elsewhere.** Put differently:

* Does a simple count (pieces, discs, material on top, length of a chain) usually say who is
  winning? Use **minimax**; it will look further than MCTS can.
* Is the position's value shape, territory or timing — something no count catches? Use **MCTS**.
* Is there chance or hidden information (Cthulhu Wars, Nyet!, Courtfall)? Use **MCTS**: it
  samples, which is the right instinct where the future is not knowable.
* Many seats? Minimax is written for two; with three or more, prefer MCTS.

## Running a batch

A batch plays the games in memory — no rooms, no move rows, no browser — and keeps the outcome of
each: who won, how it ended, how long it ran. A few hundred games take seconds with tactical bots
and minutes with MCTS. One batch runs at a time, on three quarters of the machine's cores.

What the report tells you:

* **Win rate by seat** is where the **advantage of moving first** shows. The whiskers are a 95%
  Wilson interval: where they still straddle an even share, the batch has not shown a difference —
  play more games before believing one. As a rule of thumb, 100 games pins a win rate to about
  ±10 points, 400 games to ±5, 1600 games to ±2.5. Four times the games for half the uncertainty.
* **Move the bots round the seats** (on by default). Each game shifts the line-up by one seat, so
  the same bot plays every seat equally often and the seat advantage is separated from the bot's
  strength.
* **Win rate by bot** answers the other question: is minimax actually better than tactical *in this
  game*? A bot that cannot beat the one below it is being wasted.
* **Drawn or unfinished** matters as much as who won. A high draw rate says the game is hard to
  decide (or that the bots are too weak to break a symmetry); *unfinished* means the games hit the
  ply ceiling, which usually means a rule is missing an ending.
* **Moves per game** sets the pace of a real game and the cost of every future batch.

## For game designers

A batch is the cheapest playtest you will ever run. What to look at, in order:

1. **Does it finish?** Unfinished games, or games ending in `no-legal-moves` where you did not
   intend a stalemate, are rule bugs. Fix those before reading anything else.
2. **First-player advantage.** Take tactical against tactical over 400+ games with the seats
   rotated. Much above 55% for the first player and the game wants a balancing rule — bingle has
   the **pie rule** as a setup option for exactly this, and it costs you nothing to offer.
3. **Decisiveness.** Draws near 100% between weak bots sometimes vanish between strong ones (the
   game needs skill to decide) and sometimes do not (the game is drawish). Run tactical *and*
   minimax or MCTS and compare.
4. **Depth.** Does a better bot win more? Plot random → tactical → minimax/MCTS. If stronger bots
   do not win more, the game is mostly chance — fine if you meant it, a warning if you did not.
5. **Length and branching.** Moves per game, together with how many legal moves a turn offers,
   tells you both how long a game takes to play and how expensive it will be to analyse.

Change one rule at a time and re-run the same batch with the same seed: the games are reproducible,
so a difference in the numbers is a difference in the rules and not in the dice.

## Training models on bingle

Everything above uses the same generic engine API that the training surface does, so a learned
policy slots in where a bot sits:

* `/api/envs` (and the matching MCP tools) are single-agent episodes in memory — reset, step,
  observe — with model-ready planes and fixed action ids, so a position is a tensor and a move is
  an integer.
* `/api/matches` plays a policy against the built-in bots for an honest win rate.
* A batch's reproducible seeds make a fixed evaluation set: the same 400 games, before and after
  training.

See `docs/mcp.md` for the planes and the action ids, and `docs/dsl.md` if you are adding the game
you want to train on.

## Further reading

* C. E. Shannon, *Programming a Computer for Playing Chess*, Philosophical Magazine 41 (1950) —
  where position evaluation and depth-limited search begin.
* D. E. Knuth and R. W. Moore, *An Analysis of Alpha-Beta Pruning*, Artificial Intelligence 6
  (1975) — why the cut-offs are worth a square root.
* R. Coulom, *Efficient Selectivity and Backup Operators in Monte-Carlo Tree Search*, Computers and
  Games (2006) — the first MCTS as we use it.
* L. Kocsis and C. Szepesvári, *Bandit based Monte-Carlo Planning*, ECML (2006) — UCT, and the
  formula in this page.
* C. B. Browne et al., *A Survey of Monte Carlo Tree Search Methods*, IEEE Transactions on
  Computational Intelligence and AI in Games 4 (2012) — the standard survey.
* D. Silver et al., *Mastering the game of Go without human knowledge*, Nature 550 (2017) — what
  replacing random playouts with a learned network buys you.
* C. Browne, *Evolutionary Game Design* (2011) — measuring a game by playing it, which is what the
  Analysis tab is for. Yavalath, shipped here, came out of that work.
