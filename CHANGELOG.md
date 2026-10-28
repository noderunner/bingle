# Changelog

What changed in Bingle, newest first. Bingle Core (the server, the engine and the web client) and
every game have a version of their own, and each entry says which of them moved. The newest entry is
what the help window shows under *Recent changes*, so it is written for the people who play.

## 0.12.0 - 2026-10-28

### Bingle Core 0.12.0
- **A crash in the server is now written down and answered.** If something went badly wrong while
  the server was dealing with one of your clicks, the page used to simply lose its connection with
  no explanation anywhere. Now the problem is recorded — with enough detail to find it — and your
  browser is told the request failed, so a fault that used to be invisible can be fixed.
- Every answer the server sends carries a reference (`X-Request-Id`). If you ever report something
  going wrong, that reference is what ties your report to what the server was doing at the time.
- For whoever runs the server: two new settings, `BINGLE_LOG_LEVEL` and `BINGLE_LOG_FORMAT`. The
  default is quiet — the server says what it is listening on and then only speaks up when
  something is wrong — and `BINGLE_LOG_LEVEL=debug` shows the traffic. What the server writes down
  is deliberately short of detail about *you*: never the address of the game you are in, what you
  typed in a chat or a message, your email address, or your name. See
  [docs/observability.md](docs/observability.md).

## 0.11.0 - 2026-10-27

### Meridians 0.1.0 — a new game
- **Meridians**, by Kanare Kato, joins the list. Stones never move and are never taken one at a
  time: a group of yours that can see no other group of yours — along a straight line of empty
  points — comes off the board when your turn ends. You win when your opponent has nothing left.
  Lay your first stone anywhere, your second away from it, and after that only where your own
  stones can see. It is played on a hexagon with **no middle point** (six intersections along two
  sides, seven along the other four), with smaller and larger boards as well, and the rules page
  explains sight and paths with pictures.

### Bingle Core 0.11.0
- Hexagonal boards can now have two side lengths, which is what gives Meridians its centreless
  board; every existing hexagonal game is unchanged.

## 0.10.0 - 2026-10-27

### Bingle Core 0.10.0
- **An Analysis tab.** The home page has a fourth tab where you set bots against each other over
  hundreds of games — in **any** game now, not only Cthulhu Wars — and read what it says: how often
  each seat won (with an interval, so you know when to believe it), how long the games ran, how they
  ended, and which bot came out ahead. Batches used to be a Cthulhu Wars wizard; the wizard stays
  where it is, and every game can be studied here.
- **A guide to the bots** under the tab: what random, tactical, minimax and MCTS each do, how
  alpha-beta and Monte Carlo tree search work (with drawings), which bot suits which game, how to
  read a batch, what a game designer should look at first, and where to go for training models.

### Courtfall 0.2.0
- **The odds of a pool, in the rules.** The Combat page now has a table of what dice are worth: the
  success number down the side, the number of dice across, and each cell the chance of at least one
  success, then two, then three — so you can see at a glance that three dice at 6+ is nearly
  nothing and three at 4+ is a real threat.
- **A standard arena.** Tick *Standard arena* when you make the game and the ten-tile draft is
  skipped for a fixed terrain that obeys all the draft's rules and is the same for both players
  (it maps onto itself under a half turn of the board). Ten decisions fewer before the first move,
  and the steady ground a batch of bot games needs.

## 0.9.0 - 2026-10-27

### Bingle Core 0.9.0
- **Direct messages.** The speech bubble in the top bar opens a window with everybody who is here:
  pick one of them and write. A badge counts what you have not read.
- **Talk to them.** A direct message has a *Call* button: a voice call between the two of you,
  straight from one browser to the other, with mute and hang up. Somebody calling you shows as
  *Answer*.
- **One call at a time.** Joining a call — a game's or somebody's — leaves whatever call you were
  in, and that call is told. No more being in two places and heard in neither.
- Nothing you write here is stored: the server keeps a conversation in memory for a couple of
  hours, and names are chosen by the people themselves, so a name is not proof of who somebody is.

## 0.8.0 - 2026-10-27

### Bingle Core 0.8.0
- **Every game on the server, in one place.** The *Active games* window now has two tabs: *My
  games*, as before, and *All games* — every game that has not finished, whoever made it. Open one
  to watch it, or take a seat where one is open. Games waiting in a lobby are listed too, and both
  tabs say how many seats are still free.
- **Who is here.** The person icon in the top bar opens *Your profile* as a window, with the name
  you play under and everybody who is here right now. Names are chosen by the people themselves —
  there are no accounts — so a name is not proof of who somebody is.
- **The Global Chat tab says how much you have missed,** including what was said while you were
  away, not only while you had another tab open.
- **Two new icons.** *Join a game* is two plugs coming together, not a door with an arrow through
  it, and every *Rules* button is a rule book.

## 0.7.0 - 2026-10-27

### Bingle Core 0.7.0
- **A game clock.** Two-player games can be played with a chess clock: each player has a store of
  time that runs only on their own turn, and the first to run out forfeits. Choose it when you make
  the game — and give the two players different times if one of you wants odds. Every player's
  clock is shown beside their name, the one that is running counting down. A game clock takes the
  place of a time per move, and games are played without one unless you ask.

## 0.6.0 - 2026-10-27

### Bingle Core 0.6.0
- **The pie rule is yours to choose.** Ten two-player games can be played with the pie rule —
  instead of answering the first move, the second player may swap sides, and every piece on the
  board changes hands — and the box for it is in the setup when you make the game. Hex and Gomoku
  start with it on, as they are normally played; Yavalath, Havannah, Pente, Connect Four, Amazons,
  Ataxx, Reversi and Go offer it. Each game's rules page says how it works.

### Amazons 0.2.0, Ataxx 0.2.0, Connect Four 0.2.0, Go 0.2.0, Havannah 0.2.0, Pente 0.2.0, Reversi 0.2.0, Yavalath 0.2.0
- The pie rule can be switched on for a game, and the rules page explains it.

### Gomoku 0.2.0, Hex 0.2.0
- The pie rule is still how these games are played here, and a table may now leave it out.

## 0.5.0 - 2026-10-27

### Bingle Core 0.5.0
- **Your hand is in order.** In every card game the cards you are dealt come sorted: by suit, in
  the game's own order of suits, and by value inside each suit.
- **Arrange your hand as you like.** Press a card and drag it along your hand; the cards make room
  for it, and your hand keeps that order until the next deal. Dragging a card never plays it, and
  Alt+← / Alt+→ move the card you have selected with the keyboard.

## 0.4.0 - 2026-10-26

### Bingle Core 0.4.0
- **Recent changes.** The help window (the **?** in the top bar) now shows what changed in the latest
  update, and a dot on the **?** tells you when there is something you have not read yet.
- **Card games.** Point at a card in your hand to see it twice as large. The other players' hands are
  no longer drawn: the list of players shows how many cards each one holds.
- **Phones.** Card games get their own phone layout, one view each for the play, the terms, the
  players, the moves and the chat.
- The Demo game is gone; Hex, Tic-Tac-Toe and the others are the examples to start from.

### Nyet! 0.2.0
- **Fixed:** with three or more players the first player was never offered the team to pick, and the
  game could not go on.
- **The four classic suits.** The cards are redrawn as spades, hearts, diamonds and clubs, with the
  value in two corners and the pips of its value.
- **Every player is an animal** — Wolf, Crane, Tiger, Bear or Eagle — in the colour of their seat.
  You see it beside their name, under the card they played into the trick, and on each box of the
  board they crossed off.
- **The board of terms is pictures,** not words, in cells of one size; point at one to read it. On a
  wide screen it stands beside the trick.
- **Points, tricks, loot and now cards in hand** are shown for each player, with the dealer, the
  bonus card and the team.
- The rules show all sixty cards, by suit and value.

### Fist 0.1.1
- James Ernest is listed as the designer and Crab Fragment Games as the publisher.
