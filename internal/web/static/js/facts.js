// Fun facts for the top of the home page: one is drawn at random each time the
// page loads. They are about game theory in game design, the algorithms that
// play games (and the bots of this site), and the games themselves.
//
// Keep them true and checkable. The Tak figure is computed (see
// docs/ui.md, "Fun facts"), not a published statistic, and says so.

export const FACTS = [
	// ---- game theory and game design ----
	"Zermelo proved in 1913 that in every finite two-player game of perfect information without chance, either one player can force a win or both can force a draw. Nobody knows which for chess.",
	"Hex can never end in a draw, and John Nash showed that the first player must win with perfect play — by a strategy-stealing argument that proves a winning strategy exists without saying what it is.",
	"Piet Hein invented Hex in 1942, and John Nash reinvented it independently in 1948.",
	"The pie rule — one player cuts, the other chooses which side to take — fixes a first-move advantage by making the opening move as fair as the first player can bear.",
	"Charles Bouton solved the game of Nim in 1901. The Sprague–Grundy theorem later showed that every impartial game is secretly a Nim heap.",
	"In rock–paper–scissors the only equilibrium strategy is to play each option a third of the time. Game designers use the same idea of a cycle to keep any one unit or faction from being best.",
	"A dominant strategy — one that is best whatever the opponent does — is usually a design bug: it removes the decision. Good strategy games make the best move depend on the position.",
	"Nash's equilibrium (1950) and the prisoner's dilemma (devised the same year at RAND) are the two ideas of game theory that designers meet most often, usually without calling them that.",
	"Yavalath was invented by a computer: Cameron Browne's LUDI program generated and play-tested thousands of rule sets in 2007. Four in a row wins — but make three first and you lose.",
	"Games with hidden information need different mathematics: in 2015 the researchers behind Cepheus announced that heads-up limit hold'em was essentially solved by counterfactual regret minimization.",
	"Elo's rating system, devised by Arpad Elo for chess and adopted by FIDE in 1970, predicts the chance of winning from the rating difference alone — which is why it also sorts players of nearly every game.",
	"A game's branching factor — how many moves a player can choose from — is the design lever that decides whether computers can search it. Chess averages about 35; Go about 250.",
	"Intransitive dice and rock–paper–scissors show that “better than” need not be transitive: A can beat B, B beat C, and C beat A. Many card and faction games rely on it.",
	"Chess and Go have no hidden information and no dice. Add either — poker's hidden cards, the combat dice of a Cthulhu Wars battle — and a search must also weigh chances, not just replies.",

	// ---- algorithms ----
	"The Monte Carlo method was invented by Stanisław Ulam in the 1940s while he was wondering how likely a game of solitaire was to come out; John von Neumann helped turn it into a program for the ENIAC computer, and Nicholas Metropolis named it after the casino.",
	"Monte Carlo tree search plays thousands of random games from a position and keeps statistics, steering toward moves that win more often. UCT (2006) added the rule for how often to try each move again.",
	"Alpha-beta pruning gives the same answer as minimax while skipping moves that cannot matter. With perfect move ordering it searches about b^(d/2) positions instead of b^d: twice as deep for the same effort.",
	"Minimax comes from John von Neumann's 1928 minimax theorem: in a two-player zero-sum game, each side picks the move whose worst case is best.",
	"Claude Shannon wrote down how a computer could play chess in 1950, and estimated about 10^120 possible games — the Shannon number — far more than the roughly 10^80 atoms in the observable universe.",
	"Deep Blue beat Garry Kasparov in 1997 while searching around 200 million chess positions per second.",
	"AlphaGo beat Lee Sedol 4–1 in March 2016. It combined Monte Carlo tree search with neural networks that suggested moves and judged positions, instead of relying on random playouts alone.",
	"Bingle's bots span the usual ladder: random, a tactical bot that looks for immediate wins and threats, minimax with alpha-beta, and Monte Carlo tree search.",
	"A transposition table remembers positions a search has already analysed, because the same position can be reached by different move orders. Without one, searches repeat enormous amounts of work.",
	"Iterative deepening searches one move deep, then two, then three, and so on until the clock runs out. The repeated work is a small price, because each level costs several times more than the one before.",
	"Checkers was solved in 2007 — a draw with perfect play — by combining a search of the opening with databases of every endgame position, built by working backwards from the finished games.",
	"Monte Carlo playouts need no evaluation function, only the rules of the game. That is why the same search can play Hex, Yinsh or a Cthulhu Wars table with no game-specific knowledge.",

	// ---- how big are games ----
	"There are about 4.8 × 10^44 legal positions in chess (John Tromp's 2021 estimate) — and exactly 2.08 × 10^170 legal positions on a 19×19 Go board, more than the atoms in the universe squared.",
	"A 6×6 Tak board can be filled in at most about 3.7 × 10^53 ways within the pieces each player owns (30 stones and a capstone), ignoring which of them can ever occur. Chess has about 4.8 × 10^44 legal positions.",
	"Tak on 5×5 can be filled in at most about 2.6 × 10^37 ways within the 21 stones and capstone each player owns, again ignoring reachability. Adding one row and one column (6×6) multiplies that by about 14 quadrillion.",
	"Connect Four has 4,531,985,219,092 reachable positions and was solved in 1988: the first player wins with perfect play, starting in the middle column.",
	"Tic-tac-toe has 255,168 possible games but only 5,478 different legal positions. Almost all of the games are the same few positions reached in different orders.",
	"Checkers has about 5 × 10^20 positions. Jonathan Schaeffer's Chinook team finished solving it in 2007: with perfect play it is a draw.",
	"A Rubik's Cube has 43 quintillion arrangements, yet any of them can be solved in at most 20 moves — “God's number”, proven in 2010 with about 35 CPU-years of idle computer time donated by Google.",

	// ---- the games ----
	"Tak is a game from Patrick Rothfuss's novel The Wise Man's Fear (2011). Rothfuss and James Ernest turned the fictional game into a real one, with stones, walls and a capstone that flattens walls.",
	"In Tak a stack can be moved by the player whose stone is on top, carrying up to as many stones as the board is wide. Stacking is why Tak positions are so much more numerous than the squares suggest.",
	"The Royal Game of Ur was played in Mesopotamia more than 4,000 years ago — a race game with safe rosette squares, whose rules were pieced together from a cuneiform tablet.",
	"Go is more than 2,500 years old. Its rules fit on a page; its strategy has filled libraries.",
	"Havannah, from Christian Freeling (1979), was designed to be hard for computers: the board is a hexagon of hexagons and a win may be a bridge, a fork or a ring. Freeling once offered a prize to any program that beat strong human players.",
	"Hive, by John Yianni, has no board at all: the pieces are the board. Surround the opposing queen bee on all six sides to win.",
	"Yinsh, by Kris Burm (2003), is played with rings and markers: moving a ring flips the markers it jumps, and a row of five of your markers lets you remove a ring — the first to remove three wins.",
	"Onitama (Shimpei Sato, 2014) is chess boiled down to a 5×5 board and five move cards: the card you play is handed on to your opponent after their next move, so every move also gives something away.",
	"Cthulhu Wars was designed by Sandy Petersen, who also wrote the Call of Cthulhu role-playing game for Chaosium in 1981. Each faction plays by different rules, which is why balancing it is a game-design problem of its own.",
	"Bingle's Cthulhu Wars supports three to five cults at one table; every cult has its own spellbooks, units and way to win doom, so there are very many different tables to play.",
];

/** One fact, chosen at random. */
export function randomFact(rand = Math.random) {
	return FACTS[Math.floor(rand() * FACTS.length)];
}
