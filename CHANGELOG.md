# Changelog

What changed in Bingle, newest first. Bingle Core (the server, the engine and the web client) and
every game have a version of their own, and each entry says which of them moved. The newest entry is
what the help window shows under *Recent changes*, so it is written for the people who play.

## 0.20.0 - 2026-10-29

### Bingle Core 0.20.0
- **The pie rule now moves the players, not the pieces.** When the second player swaps sides,
  nothing on the board moves or changes colour: the two of you change seats, the opening move is
  the second player's now, and the first player answers it from the other seat. Taking the swap
  back puts everyone in their first seats again.
- **Games whose rules are written in Go can offer the pie rule now.** The tick box in the setup,
  and the game that remembers the choice, used to be only for the games described in plain data.

### Blooms 0.2.0
- **The pie rule, for tables that want it.** Tick *Pie rule* when you make the game and the second
  player may swap seats instead of answering the opening stone. Unticked, the game is exactly the
  one it was.

### Amazons 0.3.0, Ataxx 0.3.0, Connect Four 0.3.0, Go 0.3.0, Gomoku 0.3.0, Havannah 0.3.0, Hex 0.3.0, Pente 0.3.0, Reversi 0.3.0, Yavalath 0.3.0
- The pie-rule swap changes seats, not colours; the rules pages say so.

## 0.19.0 - 2026-10-29

### Bingle Core 0.19.0
- **Bingle now listens only to the machine it is running on, unless you tell it otherwise.** This
  is the first step towards real accounts, and it is the one that may need something from you:
  passwords and sign-ins are coming, and they must not travel over an unprotected connection.
- **If you run Bingle for friends on your network or over the internet, read this.** On this
  version Bingle will refuse to start on a public address until the connection is protected.
  There are four ways, and the error message when it refuses names all of them:
  a certificate of your own, automatic certificates (`BINGLE_TLS_DOMAIN`), one Bingle makes for
  itself (`BINGLE_TLS_SELFSIGNED=1`), or `BINGLE_TRUST_PROXY=1` if something in front of Bingle
  already handles it. **Nothing changes if you only play on your own machine.**
- The certificate Bingle makes for itself is kept, so your browser only complains the first time
  rather than after every restart. It prints its fingerprint so you can check what you are
  agreeing to trust.
- Nothing about how games work has changed.

## 0.18.0 - 2026-10-29

### Bingle Core 0.18.0
- **A game that goes wrong can now be put back exactly as it was.** If the rules of a game ever
  contradict themselves — a move the board offers you that the server then refuses, or a position
  the rules cannot make sense of — Bingle writes down the whole game at that moment, and whoever
  fixes it can replay your exact position on their own machine. No more "it happened once and I
  can't make it happen again", which is how this sort of bug usually survives.
- Practice batches do the same for themselves: they play hundreds of games unattended, and if one
  of them hits a hole in the rules it files the evidence on its own.
- Once a fault is fixed, the record of it is kept as a test, so the same fault cannot come back
  unnoticed.
- For whoever runs the server: `BINGLE_DEBUG_DIR` turns this on, and `scripts/replay` replays what
  it finds there. These files hold a whole game including the parts normally hidden from players,
  so they are written only where you ask and never into the ordinary log. See
  [docs/observability.md](docs/observability.md).

## 0.17.0 - 2026-10-28

### Bingle Core 0.17.0
- **If something breaks in your browser, Bingle now finds out.** Until now, a fault in the page —
  a board that would not draw, a button that did nothing — left no trace anywhere unless you
  happened to mention it. The page now reports what went wrong, so it can be fixed without anyone
  having to describe it.
- **What it reports is deliberately thin.** The error itself, which page it happened on, and which
  release you were on. Not the code of your game, not your name, not anything you typed, and not
  the address of the page you were looking at. Anything that looks like a game code or a private
  token is stripped out twice over before it is stored.
- Repeats of the same fault are counted rather than piled up, so a bad release shows as one
  problem that happened four hundred times instead of four hundred problems. Faults nobody has hit
  for a fortnight are forgotten.
- For whoever runs the server: `BINGLE_CLIENT_ERRORS=off` switches it off entirely, and the faults
  are listed on the Health page. See [docs/observability.md](docs/observability.md).

## 0.16.0 - 2026-10-28

### Bingle Core 0.16.0
- **The server now remembers how it has been doing.** Until now it could only say how things stood
  at the moment you asked. It now keeps a record — how busy it has been, how quickly it answered,
  how fast the database is growing, how games have been ending — and there is a **Health page**
  that draws it: a picture of the last day, or the last year, instead of a single number.
- The record keeps itself small without anyone minding it. Recent history is kept in full detail,
  older history is summarised by the hour and then by the day, and the summaries keep the highest
  and lowest points rather than smoothing them away. It is kept in its own file beside the games,
  so it travels with a backup and can be deleted without touching a single game.
- For whoever runs the server: `BINGLE_METRICS_SAMPLE` and `BINGLE_METRICS_RETAIN_DAYS`, with the
  page at `/metrics/health` and the figures downloadable as a spreadsheet. As before, none of it is
  on the public site and none of it is about you. See
  [docs/observability.md](docs/observability.md).

## 0.15.0 - 2026-10-28

### Bingle Core 0.15.0
- **When something is slow, the server now records why.** If a page or a move takes longer than it
  should, Bingle writes down where the time actually went — waiting for the game to be free,
  reading the database, working out the rules — instead of leaving it a mystery. The same happens
  whenever the server fails a request, including the kind of failure that used to leave no trace
  at all. If you report that a game felt sluggish at a particular moment, there is now something
  to look at.
- **Every move is accounted for.** How long the rules took to work out, how many moves a position
  offered, how often a move was refused and why games end are all counted now, for every game.
  Two of those are early warnings: a sudden rise in refused moves usually means a player's page is
  out of date, and a sudden rise in games ending without a winner means a rule has stopped working.
- For whoever runs the server: `BINGLE_SLOW_REQUEST` and `BINGLE_METRICS_DB` for the journal
  (which is kept in its own file, so it can be deleted without touching a single game), and
  `BINGLE_DEBUG_GAMES` / `BINGLE_DEBUG_ROOM` to follow one game or one room. What gets written
  down still never includes the code of your game, your name, or anything you typed.
  See [docs/observability.md](docs/observability.md).

## 0.14.0 - 2026-10-28

### Bingle Core 0.14.0
- **The database is now watched as closely as the rest of the server.** Every kind of question
  Bingle asks its database is timed and counted by name, so a slow page can be traced to the thing
  that is actually slow instead of guessed at. The size of the database, how fast it is growing and
  how many games, moves and messages it holds are all visible too — which is what turns "the
  server felt sluggish last night" into something anyone can look up.
- It also notices the one failure that used to be silent: a save that could not go through because
  something else was writing at the same time. If that ever starts happening to you, it now leaves
  a trace.
- As before, none of this is on the public site and none of it is about you — it counts
  Bingle's own work. See [docs/observability.md](docs/observability.md).

## 0.13.0 - 2026-10-28

### Bingle Core 0.13.0
- **The server can now be watched while it runs.** Whoever runs Bingle can see how many games are
  being played, how quickly pages and moves are answered, which requests are being refused and
  how much work the machine is doing — in the standard format every monitoring tool reads, with
  nothing extra to install. If something feels slow, there is now a way to find out where the time
  goes instead of guessing.
- This is **off the public site on purpose.** The figures live on a separate address that only the
  machine Bingle runs on can reach, and putting them anywhere a network can get to them requires
  a password. Nothing about *you* is in them: not the code of the game you are in, not your name,
  not what you typed.
- For whoever runs the server: `BINGLE_METRICS_ADDR` (default `127.0.0.1:9095`, empty to turn it
  off) and `BINGLE_METRICS_TOKEN`. See [docs/observability.md](docs/observability.md).

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
