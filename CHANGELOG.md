# Changelog

What changed in Bingle, newest first. Bingle Core (the server, the engine and the web client) and
every game have a version of their own, and each entry says which of them moved. The newest entry is
what the help window shows under *Recent changes*, so it is written for the people who play.

## 0.90.0 - 2026-11-06

### Bingle Core 0.90.0
- **For game authors: games of patience.** A game definition can now describe a game of patience in a few lines: its columns and how cards build on them, the foundations, free cells, the stock, and runs that leave the table when complete. Klondike, Spider and FreeCell can be written this way, and so can Yukon, Scorpion, Baker's Dozen and Forty Thieves. Solitaire itself plays exactly as before. See *patience* in the definition guide.

## 0.89.0 - 2026-11-06

### Bingle Core 0.89.0
- **For game authors: a card turned up stays turned up.** In a game definition with cards, a move that shows a player a card they could not see before can no longer be taken back, so nobody can peek and choose again. It is the rule Solitaire already plays by. Games already in the catalog play as before.

## 0.88.1 - 2026-11-06

### Bingle Core 0.88.1
- Nothing changes in play. Behind the scenes, Onitama now has a game definition of its own, which is checked against today's Onitama over many random games before it takes over.

### Onitama 0.1.2
- **The deal is shuffled the way every card game's is.** Which five cards come up is still random, and games in progress keep the cards they were dealt.

## 0.88.0 - 2026-11-06

### Bingle Core 0.88.0
- **For game authors: move cards.** A card in a game definition can now show the squares a piece may reach, read from the point of view of the player holding it, and playing it moves a piece and passes the card on, as Onitama's Move cards do. A win can also count one side's pieces of a type both sides have, such as a Master. Games already in the catalog play as before. See *card-move* in the definition guide.

## 0.87.0 - 2026-11-06

### Bingle Core 0.87.0
- **For game authors: cards dealt at the start.** A game definition can now shuffle its cards and deal them into hands and onto the table before the first move, as Onitama deals its Move cards. The same game seed always deals the same cards, and each player sees only the cards their hands and the table show them. Games already in the catalog play as before. See *cards* in the definition guide.

## 0.86.0 - 2026-11-06

### Bingle Core 0.86.0
- **For game authors: a move limit decided on captures.** A game definition's end at a move limit can now be won by whoever has more in a pool, such as stones captured, rather than more pieces on the board. With it, Blooms can be written as a definition. The new definition is checked against today's Blooms over many random games, and Blooms itself plays exactly as before.

## 0.85.0 - 2026-11-06

### Bingle Core 0.85.0
- **For game authors: captures at the end of a turn.** A game definition can now remove groups with no empty cell beside them when a turn ends rather than as each stone lands, take only the opponent's, count the stones taken towards a win, and tint such groups on the board so players see what the end of the turn will take, as Blooms does. Games already in the catalog play as before. See *Effects at the end of a turn* in the definition guide.

## 0.84.0 - 2026-11-06

### Bingle Core 0.84.0
- **For game authors: a seat with several colours.** A game definition can now give a seat more than one colour, as Blooms gives Warm red and yellow. A stone is placed in its own colour, counts as its seat's for moving, capturing and winning, and a group is still one colour, so a player never captures their own stones of the other colour. Games already in the catalog play as before. See *seats, pieces, setup* in the definition guide.

## 0.83.1 - 2026-11-06

### Bingle Core 0.83.1
- **Fixes for moves of several parts.** Since moves of several parts were grouped into one, the History tab's *Emerges* button sometimes played nothing: in Cthulhu Wars, a Great Old One whose awakening earned a spellbook in the same move, as the King in Yellow does, now emerges again, both when he wakes and from the badge. The last move is marked whole as well, so a Blooms turn of two stones marks both stones, and a stone followed by *Done* marks the stone.

## 0.83.0 - 2026-11-06

### Bingle Core 0.83.0
- **For game authors: turns of several choices.** A game definition can now say that a turn is made of several parts, such as two stones, or a move and then a wall, with how many choices each part takes and a *Done* button to stop early. An opening can have a turn of its own, as in Connect6's single first stone. Played on the page, the choices of a turn are built up and sent together, like a Blooms turn. Games already in the catalog play as before. See *A turn of several selections* in the definition guide.

## 0.82.1 - 2026-11-06

### Bingle Core 0.82.1
- **A choice is not lost while the board catches up.** A piece or a cell chosen just after another move arrived, as in a tutorial right after its scripted replies, sometimes did nothing until it was clicked again. It now counts the first time.

### Blooms 0.5.0
- **The move limit counts turns.** A ply is now one player's whole turn, as the rules always said, rather than each stone and *Done* on its own. The limit is halved to match: 250 turns (500 on base 8, 750 on base 10), so a game lasts as long as before, games in progress included.

## 0.82.0 - 2026-11-06

### Bingle Core 0.82.0
- **For programs:** the API and MCP take the names of ADR 0114, without aliases. `GET /games/{code}/selections` replaces `GET /games/{code}/moves/legal` and `GET /games/{code}/legal-moves?from=` (a piece's destinations are the options from its square), and MCP's `list_selections`, whose options now carry their structured fields, replaces `get_legal_moves` and `list_legal_moves`. The clicks that enter a selection are `gestures` rather than `steps`, in the selections, a hint and a tutorial's guide (`play_gestures`). `auto_play_turn` is `auto_play_impulse`, and `POST …/auto-play-turn` is `POST …/auto-play-impulse`. The page's Back button is `#back`.

## 0.81.0 - 2026-11-06

### Bingle Core 0.81.0
- **The history goes a move at a time.** A turn of several parts, such as two Blooms stones or a Cthulhu Wars Move with its units, is one entry in the move list and one step back or forward through the game, and the story of a move in the History tab tells all of it together. A link to a position (`#move=N`) counts moves the same way.
- **Cthulhu Wars fixes since the last two updates.** A battle shows on the map again while the attacker is still choosing, the History tab again says when nothing new happened since your last move, and a reaction passed for you no longer hides the rest of the history until your next move.
- **For programs:** move numbers in the API and MCP count moves, not their parts: `move_seq`, `seq` in the history (where a move of several parts lists them in `selections`), and the new names `at_ply` (state, simulate, hint) and `since_ply` (the history), which replace `at_seq` and `since_seq`.

## 0.80.0 - 2026-11-06

### Bingle Core 0.80.0
- **Moves are counted by turn.** A move made of several parts, such as a Blooms turn of two stones, is now stored and counted as one, and games already played are counted again the same way when the server starts.
- **Undo is for games of patience.** Elsewhere a move is sent whole once it is finished, so there is nothing to take back once it is played; the Back button takes back what you have chosen before you submit. In Solitaire and FreeCell, Back still takes back a move that showed no card.

## 0.79.0 - 2026-11-06

### Bingle Core 0.79.0
- **A turn goes in one piece.** In a game of several parts to a turn, such as a Blooms turn of two stones or a Move of several units in Cthulhu Wars, the board now shows each part as you choose it, and nothing reaches your opponents until the turn is over. Back takes the last part off at once, a reload keeps what you have chosen so far, and when you submit, the whole turn is played together or not at all. Games of patience and the tutorials still send each move as it is made.

## 0.78.0 - 2026-11-06

### Bingle Core 0.78.0
- **Ask what comes next, then send it all at once.** A program playing through the API or MCP can ask what it may choose next, step by step, without sending anything: a Blooms turn is a stone and then another stone or *done*, and the server lists each part in turn (`GET /api/games/{code}/selections`, or the `list_selections` tool). When the choices add up to a whole move, it sends them together as one, joined by `;`, and either all of it is played or none of it.
- **No more moves at a stale position.** A move can say which position it was made for, and is turned down if the game has moved on in the meantime, for example because the other player resigned.
- **The setup's picture follows the game you chose.** Switching quickly between variants, such as the games of patience, could leave the picture of one chosen earlier.

## 0.77.0 - 2026-11-05

### Bingle Core 0.77.0
- **Play from the command line.** The help window has a new **API** button that opens the guide to bingle's API, which now starts with a game of Hex played step by step with curl. A program that asks the server where to begin is pointed at the guide and the handful of calls it needs, and a wrong turn gets a clear answer rather than a bare "page not found".

### Solitaire 0.2.0
- **A card you have seen stays seen.** Back still takes back any move, but no longer one that drew from the stock or turned a card face up: those stand once played, so Back can't be used to peek at what is coming.

### Blooms 0.4.0
- **Two big boards for a long game.** Choose **base 8** (169 cells, first to 40 captures) or **base 10** (271 cells, first to 50) when you make a game. They suggest 25 and 35 minutes on the clock, and a game on them runs to 1000 and 1500 plies before it is settled on captures.

## 0.76.1 - 2026-11-05

### Bingle Core 0.76.1
- **Your hand is your own.** Anybody with a game's room code could ask the server for the moves of the player whose turn it was, and in a card game like Nyet! that listed their hand. Now only the player in that seat can see them.

## 0.76.0 - 2026-11-05

### Bingle Core 0.76.0
- **More to watch in the Animation Tester.** Every new Great Old One below has its own button.

### Cthulhu Wars 0.16.0
- **Eight more Great Old Ones have their moment,** the first time each awakens. **Tsathoggua** stirs from his slumber in a black cavern, **Ithaqua** strides out of a blizzard, **Rhan-Tegoth** breaks free of a glacier, and **Yog-Sothoth** presses through a rift among the dials of time. **Ubbo-Sathla** throws the lid off his hellish banquet, **Bastet** rises from a golden pyramid on the moon, **Cthugha** gathers in a fire that swallows the map, and **Abhoth** heaves up out of the muck, shedding his brood.
- **Independent Great Old Ones now get a badge in the history** when they first emerge, and so does Ubbo-Sathla when he is awakened in the Doom phase. These awakenings were missing the badge before.

## 0.75.0 - 2026-11-05

### Bingle Core 0.75.0
- **Tutorials can point at the page.** A step can now light up the part of the screen it is talking about, like your Power, the round's phases or the History, and opens that tab or card for you.
- **Move several units in a tutorial,** just as you do in a game.
- **Back stays out of the way in a tutorial,** so you can't knock yourself off the script by taking back a step of your turn.

### Cthulhu Wars 0.15.0
- **Eight new tutorials for the core game.** Find them under *Learn the rules*, each short enough to come back to as a refresher: **How to win**, **Power**, **Units**, **Movement**, **Spellbooks**, **Combat**, **Sequence of play** and **UI features**.
- **The High Priests tutorial now fills in the History** as you play. It was staying empty.

## 0.74.0 - 2026-11-05

### Bingle Core 0.74.0
- **Animation Tester.** Cthulhu Wars' setup has a new tab for watching its animations without starting a game.

### Cthulhu Wars 0.14.0
- **Every core Great Old One has its moment.** The first time each awakens: **Nyarlathotep** condenses out of the shadows as pale eyes open around him, the **King in Yellow** rises through the trap of a rotting theatre with the Yellow Sign burning behind him, **Hastur** comes from a point in the abyss as the stars are dragged into a spiral, and **Shub-Niggurath** heaves up out of a swamp, with mouths opening all over her.
- **Watch them again** from their badges in the history.
- **Easier to read battle dice.** Dice are now in the colour of the faction rolling them, and Kills and Pains stand out clearly on every colour.

## 0.73.0 - 2026-11-05

### Bingle Core 0.73.0
- **Run the platform under your own name.** A server can now set its own name and icon, and players see them everywhere: pages, mail, calendar invitations and the rules.

## 0.72.0 - 2026-11-05

### Bingle Core 0.72.0
- **Schedule a game.** When you set up a real-time game while signed in, you can choose *Schedule a time*, in your own time zone, and invite your friends or anyone by their exact handle. The room opens straight away, so everyone can chat and choose seats. When the time comes, any player at the table can press *Start game*.
- **Calendar invitations.** You get one when you schedule a game, and each player gets one when they accept. The lobby also has *Add to calendar*.
- **Find a time together.** Instead of one time, offer several in the next 30 days. Each player ticks the ones they can make, and can change their answer whenever they like. Once everyone has answered, the earliest time they can all make becomes the game's time.
- **When no time suits everyone,** the player who scheduled the game has 48 hours to offer new times or cancel it. After that it closes, and everyone invited is told.

## 0.71.0 - 2026-11-05

### Bingle Core 0.71.0
- **Accept or decline game invitations** in your friends list. Once you decline, you are not asked again for that game.
- **Choose whether to be invited.** In *All Settings*, turn off *Let other players invite me to games* and nobody can send you invitations.

## 0.70.0 - 2026-11-05

### Bingle Core 0.70.0
- **Add a friend from any chat.** A player's @handle in a chat is now a badge. Click it to send them a friend request, or to accept one. For a friend, it does nothing for now.
- **A tidier Active Games list.** The window is now titled *Active Games*, and its badges sit in two columns. Under each game: *vs* the other player at a table of two, or the number of players at a bigger table, and how long ago the last move was. The move number is gone.

### Cthulhu Wars 0.13.0
- **A shorter lobby title.** The lobby now just says *Cthulhu Wars*, with a card of icons below it for the map, factions, neutral units and everything else at the table.
- **Clearer headings in the picker:** *High Priests* and *Azathoth - Neutral Spellbooks*.

## 0.69.0 - 2026-11-05

### Bingle Core 0.69.0
- The option for battle animations now covers Great Old Ones too.

### Cthulhu Wars 0.12.0
- **Cthulhu Emerges.** The first time Cthulhu is awakened, the map zooms in, the sea rises, the drowned city breaks the surface, and he comes up out of the water, wings unfolding, until his eyes open.
- **Watch it again.** The history marks each Great Old One's first awakening with a badge in its cult's colour. Press *Cthulhu Emerges* to play it again. The other Great Old Ones get their shows later.

## 0.68.0 - 2026-11-04

### Bingle Core 0.68.0
- **Reserve seats for friends.** When you set up a game, you can keep a seat for a friend. They get an invitation, nobody else can sit there, and you can release the seat if they do not come.
- **Friends-only games.** When every seat is held or kept for friends, the game leaves the public lists. Your friends still see it, and can join or watch.
- **Invite-only games.** A new option in the setup keeps a game off every list, even your friends'. Players come by its link, or an invitation from your friends list. Anyone with the link can still watch.

## 0.67.0 - 2026-11-04

### Bingle Core 0.67.0
- **Stay private if you like.** In *All Settings*, turn off *Show when I am online* to look offline to your friends and leave *Here now*. You can still write to friends, and they can reply.

## 0.66.0 - 2026-11-04

### Bingle Core 0.66.0
- **Friends.** The speech-bubble icon now opens your friends list, for signed-in players. Add somebody by their handle, or from *Here now* in your profile window. They can accept or decline.
- **Talk with friends.** Direct messages and voice calls are now between friends, and a dot shows who is online. Messages only reach friends who are online.
- **Invite a friend to a game.** Choose *Invite to game* beside a friend's name. The invitation waits in their friends list even if they are away.
- **Remove a friend** from the same place.
- **Mail about friend requests,** if you want it. It is off unless you turn it on in *All Settings*.

## 0.65.0 - 2026-11-04

### Bingle Core 0.65.0
- **No room code in the top bar of a game.** It is in the address, and at the bottom of the game options.

### Cthulhu Wars 0.11.0
- **Shorter names in the lists.** Public Lobbies and Active Games say *Cthulhu Wars* and the room code. Badges say the rest, where they apply: *Core only*, the map if it is not the usual one, *Neutral units* and *High Priests*.
- **The real map in the rules.** The rules show the painted map, with the name of each area on it.
- **Faction glyphs in the lobby**, in place of the coloured squares.
- **A tidier Round card.** The Ritual track and the Elder Signs stand at its right.
- **Fewer options.** *Board Theme* and *Flip board* are gone: neither did anything for a map.

## 0.64.0 - 2026-11-04

### Bingle Core 0.64.0
- **The history can replay a moment on the map.** Going to a move of the history can now play what happened there, and leaving it takes the show away. Cthulhu Wars' battles are the first to use it.

### Cthulhu Wars 0.10.0
- **Battles are a show.** When a battle starts the map zooms in on the area and everything around it goes dark. The two sides stand across from each other, large, and each side's dice pop in one by one.
- **Pre-Battle and Post-Battle Phases.** If either side has an ability that needs a choice, the phase is named at the top and its abilities pop in one by one, with the glyph of the cult each belongs to. The one the game is waiting on lights up, and the show waits for it.
- **The dice and the dead.** The dice are thrown between the sides, die by die, and a red X lands on each unit as it dies. You can choose your casualties right there on the stage. Then the map zooms back out for the retreats.
- **Phones too.** On a phone the show fills the screen, on its side if you hold the phone upright, and turning the phone straightens it. If a choice needs a button, the show steps aside until you have made it.
- **Watch it again.** Choosing a battle's move in the history plays it again, and so does *Replay* on the battle's card.
- **Yours to turn off.** *Skip* or Esc ends a show, and *Battle animation* in the options turns them off. They stay off if your device asks for reduced motion.


## 0.63.0 - 2026-11-04

### Bingle Core 0.63.0
- **No more name box when setting up a game.** You play under the name in your profile. The same goes for puzzles and tutorials.
- **Tabs in the setup.** *New game* is now *Create Game*, and tutorials and practice batches have tabs of their own.

### Cthulhu Wars 0.9.2
- **A clearer setup.** *Expansions*, *Seats* and *Other Options* each have a title and a line, and the other options are small cards in
  two columns. The maps tab shows the real pictures of the maps.
- **Sand-timer spellbooks.** A table with Ghroth or The Thousand Forms gets a card for each, to replace it for a correspondence game, with
  a recommended choice.
- **Fewer words.** *Set every bot to…* and the note on bot names are gone. The Factions tab explains how open seats choose.

## 0.62.0 - 2026-11-04

### Bingle Core 0.62.0
- **Game Options** is shorter and tidier, with fewer words all round.
- **All Settings** no longer mentions email to guests.

### Cthulhu Wars 0.9.1
- **Cult cards say more with less.** The cult's glyph replaces the coloured square, the player's name and the card's details are
  in the cult's colour, the summary counts spellbooks, and a line replaces each section's title.
- **The impulse card** shows the player's name in their cult's colour, with its glyph.
- **The turn order** left the bar above the map, as the cult cards are already in that order. The first player's card shows the
  direction of play.
- **Pass just says Pass.** If you still have Power, the card asks you to confirm that you will lose it.

## 0.61.0 - 2026-11-04

### Bingle Core 0.61.0
- **One History card.** The Moves and History tabs are now a single card with a *Friendly* / *Notation* switch, and the same
  buttons step through the game either way. Click a line in either view to go to that move.
- **The whole story while you step.** The friendly view keeps every move on screen and highlights the one you are on, instead of
  cutting off at it. *Since your last move* is in both views.

## 0.60.0 - 2026-11-04

### Bingle Core 0.60.0
- **History in full colour.** Stepping through a game's moves, the card is titled *History: 12* and tells the move just as the
  History tab does, with the players' colours and icons.
- **Better guest names.** A guest who has not chosen a name gets one like *Mossy Otter*.
- **A tidier profile window.** Your name comes right under the sign-in buttons as *My Name*, sections have clear titles and lines
  between them, and guests no longer see *Here Now*. *Use this key* is now *Submit Key*.
- **All Settings.** The profile page has a new name and clearer sections.

## 0.59.0 - 2026-11-04

### Bingle Core 0.59.0
- **Carry on as a guest on another device.** The person icon's window now has *Move to another device*: copy your guest key
  there, as a link or as text, and use it on your phone or another browser to carry on your games. It used to be on the profile
  page, where it was easy to miss.

## 0.58.0 - 2026-11-04

### Bingle Core 0.58.0
- **Submit just says Submit.** The move you are about to play is no longer spelled out on the button.
- **No Reset in the stone games.** *Back* is all they need.
- **Going back to your last move.** *Since your last move* in the History tab is now a button. It takes you to your last move in
  the Moves tab, with *Next* ready to click.
- **History in words.** Stepping through a game's moves, the impulse card says what happened in the same words as the History tab.
- **Unread chat.** In the active games list, a red number on *Open* says how many chat messages have come in for that game.
- **Join Game is simpler.** Paste a room code or a link and press *Join Game*. You take an open seat under your usual name, or
  watch if the table is full. The tables looking for players are now under *Public Lobbies*.
- **Cthulhu Wars:** the action buttons show their cost with the Power icon, as the summoning buttons do.

## 0.57.0 - 2026-11-04

### Bingle Core 0.57.0
- **Simpler words in the stone games.** Games where all you do is place a piece now just say *Place a stone.*, or whatever the
  game calls it.
- **The ✕ on an active game resigns.** After you confirm, it resigns from the game and takes it off your list. A game that has
  not started is only taken off your list, and you keep your seat.
- **Each game's own Active Games.** The setup of a game has a new *Active Games* tab listing the games of it going on, to watch.
  The *All games* tab of the active games list is gone.
- **Tables looking for players.** The Join Game window lists the games that have not started and still have a seat free, each
  with a *Join* button. Its button in the top bar is now a +.
- **Step back through the history.** A small back arrow under *Since your last move* goes back one move, and again with each
  click, to go through what happened move by move.
- **Cthulhu Wars:** the neutral items card no longer counts its items.

### Havannah 0.3.1
- Its pieces are called stones.

### Meridians 0.1.2
- The card says *Place a stone.*

## 0.56.0 - 2026-11-04

### Bingle Core 0.56.0
- No changes of its own: this release is the Cthulhu Wars moves below.

### Cthulhu Wars 0.9.0
- **Move several units with one destination.** Click units on the map to gather them in the impulse card, then click where
  they go. Each type shows its icon with a count, next to the Power the move will cost, and gathered units leave the map. Click
  an icon to put a unit back, or *Back* to put them all back.
- **You can only gather what you can pay for**, and only units in the same area. Move another area's units afterwards.
- **A move now waits for *Done*,** even when you have spent your last Power, with a summary of what moved. *Back* there
  takes back everything sent to that destination.
- **No more *Reset* button** on the map: *Back* does the job.

## 0.55.0 - 2026-11-04

### Bingle Core 0.55.0
- **The card of actions names who is up.** Its title is now the name of the player being waited for, instead of *Your move*,
  and under it is what they are being asked to choose. Everyone at the table sees the same two lines, on your impulse and on
  anybody else's, and so does anyone watching.
- **Each step asks its own question.** Choose *Build Gate* and the card changes from *Choose an action.* to *Choose an area to
  place a gate.* These steps show only for the player taking them, until the move is sent.
- **For anyone with a bot or a program playing through the API:** the seat to act is now called the impulse seat. `turn_seat`,
  `to_move`, `your_turn` and `turn_deadline` are now `impulse_seat`, `impulse_seat`, `your_impulse` and `impulse_deadline`, and
  the old names are gone. The `to-move` plane of a training environment is now `on-impulse`.

### Cthulhu Wars 0.8.1
- The card says *Choose a turn order.*, *Choose an action.* and *Choose a pre-battle ability or pass.*, and the turn order
  choices no longer say "down the list".

### Blooms 0.3.1, Fendo 0.1.1, Fist 0.1.2, Hive 0.1.1, Meridians 0.1.1, Nyet! 0.2.1, Onitama 0.1.1, Solitaire 0.1.2, Yinsh 0.1.1
- Each says in its own words what the player on impulse is asked to choose.

## 0.54.0 - 2026-11-04

### Bingle Core 0.54.0
- **Prototype games.** A game whose rules are still changing now carries a *Prototype* mark in the list of games.
- **Choose which rules to play.** The setup of a prototype has a *Rules version* list, with the newest chosen first, so
  playtesters can try one version of the rules against another.
- **A prototype game keeps its version.** A game stays on the version it started with, even when a newer one arrives, and
  its name says which it is (*Courtfall 1.0.0*) in the lists of games and in the rules window.
- **Prototypes are not rated.** A rated game or a rated tournament can no longer be set up for one, and they leave the
  leaderboards and profiles. A rated game of one that is under way can finish, but does not change anybody's rating.

### Courtfall 1.0.0
- **Now a prototype,** and its rules become version 1.0.0. The rules themselves do not change, and games under way carry on.

### Mycelium 1.0.0
- **Now a prototype,** and its rules become version 1.0.0. The rules themselves do not change, and games under way carry on.

## 0.53.1 - 2026-11-03

### Bingle Core 0.53.1
- No changes of its own: this release is the Cthulhu Wars maps below.

### Cthulhu Wars 0.8.0
- **Every map now looks like its board.** The 3-player, 5-3 and 5-player Earth maps and the 3-player Primeval map get the colour
  and texture of the printed boards, with the borders traced from them.
- **Two new maps for four players:** Primeval 3-5 and Primeval 5-3, the two four-player boards of the Primeval Map, each with
  18 areas.
- **Glyphs stand where they are printed** on every Earth map, and all the Primeval maps show their Glacier glyphs.
- **A few borders change, because the board says so.** These change legal moves, and a game under way gets them on its next move:
  - On the 5-3 and 5-player Earth maps, Central America also borders South America East.
  - On the 5-player Earth map, South America West borders the South Atlantic rather than the North Atlantic.
  - On the 3-player Primeval map, Upper Mu borders the Rheic Ocean, and Poseidonis can take a Glacier.

## 0.53.0 - 2026-11-03

### Bingle Core 0.53.0
- **New-message badge.** A chat tab now shows how many messages came in while you were on another tab or view. It never
  counted them before.
- **Easier chat to read.** Each message shows the name and @username or guest above the text, on alternating backgrounds.
- **The rules on a phone.** The rules no longer pop up the keyboard when opened, the close button is bigger and stays in reach,
  Back closes the rules instead of leaving the game, and pages fit the screen.
- **History stays put.** On a map it is always a tab in the right-hand column, instead of sometimes moving below the map.
- **History opens on what's newest.** Switching to History or Moves, or turning your phone back upright, scrolls to the latest entry.

### Cthulhu Wars 0.7.0
- **Cult cards follow the order of play.** The first player's card is on top and the rest follow, and they re-order whenever
  the order changes. The note about play running down the list is gone.
- **Glyphs on the cult cards.** Each card shows its faction's glyph beside the colour square, in place of the faction's name on
  human seats.
- **Tcho-Tcho are pink.**
- **Ubbo-Sathla's Growth** is now an icon. Hover over it for what it means.
- **Choosing where to begin** now says so plainly in the *Your move* card.

## 0.52.1 - 2026-11-03

### Bingle Core 0.52.1
- No changes of its own: this release is the Cthulhu Wars rules correction below.

### Cthulhu Wars 0.6.0
- **Every Earth map now has the board's glyphs.** The Americas are Dragon; Africa and Australasia are Chevron; Europe, Arabia
  and Asia are Thorns. This changes where the Yellow Sign and The Ancients earn their glyph spellbooks.
- **Two cults begin where the board says.** The Black Goat starts in Africa, or West Africa where the map splits it, instead of
  South America. The Crawling Chaos starts in South Asia on the maps that split Asia, instead of North Asia. This is a real
  change to how those cults open, and games already under way keep the positions they have.

## 0.52.0 - 2026-11-03

### Bingle Core 0.52.0
- **Glyphs where they are printed.** A map with a painted background can place its glyphs exactly where the board does.

### Cthulhu Wars 0.5.0
- **The 3-5 Earth map, redrawn.** The four-player 3-5 map now looks like the printed board, with its coasts, colours and islands.
- **New glyphs on every map.** The chevron, thorns and dragon are traced from the board. They replace the old line drawings
  everywhere, and on the 3-5 map they stand where the board prints them.
- **The 3-5 map's glyphs now match the board.** Eight areas change glyph: North and South America are Dragon; Europe,
  Scandinavia, North and South Asia and Arabia are Thorns; Australia is Chevron. This changes which areas meet the Yellow
  Sign's and The Ancients' glyph requirements on this map.

## 0.51.0 - 2026-11-03

### Bingle Core 0.51.0
- **Maps can have a painted background.** An area map may now show a picture behind its areas, which stay just as clickable.

### Cthulhu Wars 0.4.0
- **The five-player Primeval Map, redrawn.** Traced from the printed board, its coasts follow the real ones, and its colours,
  shallows and cracks now look like the board's.
- **Glacier glyphs on the map.** The Primeval Map shows its ice glyphs where the board prints them. Turn them off in the game
  options under *Glacier glyphs*.
- **Two borders were missing from that map.** Upper Mu now borders the Rheic Ocean, and the Paratethys Ocean borders
  Poseidonis, as printed. This changes where units can move.

## 0.50.0 - 2026-11-03

### Bingle Core 0.50.0
- **Saved setups.** A game whose setup has many parts can keep it: with an account, save the table you have chosen under a
  name and share it if you like. Anyone, guests too, can start from a shared setup. Cthulhu Wars is the first such game.

### Cthulhu Wars 0.3.0
- **A new setup page.** The long list of check boxes is gone. Maps, Factions, Monsters & Terrors, IGOOs and Other are tabs
  of pictures, each with its name; hover over one for a summary of its rules, click to add it.
- **Pick single units.** You no longer take a whole expansion: choose just the Gnorri, one Terror, or a single neutral
  spellbook.
- **Current selections.** A card shows the table so far, and a click there takes something out again. A small notice appears
  when you have more neutral units and Independents than one fewer than the players, which is only advice.
- **Let the table choose.** Add more factions than there are seats and players pick theirs from those in the lobby, which
  switches on by itself. Anyone who doesn't choose plays the faction their seat already has.
- **Shuffle.** One button fills any open seats and deals random neutral units and Independents up to the recommended number.
  Press it again for another deal; what you chose yourself stays.
- **Start from a setup.** Four ready-made tables to begin with, and your own or other players' saved ones.

## 0.49.0 - 2026-11-02

### Bingle Core 0.49.0
- **Every game fits your screen.** The page Blooms has been trying out is now every game's: the whole play area stays in
  view with the cards beside it, and nothing scrolls but the cards. The status bar is gone too: the result of a game and
  its *Rematch* button are in the card with your moves, as are the controls for history and analysis.
- **Your pieces are with your moves.** Your hand of pieces, and Onitama's move cards, are in the card that stays in view on
  your turn.
- **Card games get the whole table.** Nyet and the games of patience take all the width they can, with bigger cards on a
  bigger window.
- **Cthulhu Wars.** The map sits right under the round, with the history and any battle below it. The cults and the chat share the column beside the map, behind tabs, under your actions, and a badge tells
  you when a message comes in.
- **Tidier cult cards.** Each cult is one line of numbers until you open it with its triangle, and only one is open at a
  time, as with the neutral items. Open, a cult's spellbooks and requirements stand in two columns of tiles. A spellbook you
  earn replaces a requirement, so the list gets shorter as you go. Hover over a tile for the full rule.
- **Smaller summon buttons.** Summoning a monster is its picture and its price; hover over it for the rest.
- **A calmer map.** Area names sit in the middle of their areas, and they and the glyphs are faded, so pieces stand clearly
  on top. Glyphs stay inside their areas' borders. The gold stars on upgraded units are gone, and so is the card under the
  map when you click a unit (it stays on phones), along with the zoom buttons: the mouse wheel and dragging still work.
- **Turn your phone to switch views.** In Cthulhu Wars on a phone, hold it upright for your actions, the cults, the history
  and the chat, and turn it on its side for the whole map, to tap areas, move units, pinch and drag.
- **Hide the units.** The game options can now turn the units off to study the map, next to the names, glyphs and sigils.
  Any unit you can move right then stays visible.
### Cthulhu Wars 0.2.0
- **A new cult: The Ancients.** A cult with no Great Old One, who build Cathedrals, give their enemies gifts and move their
  army across the world in the Doom phase. They play in teal.
- **A new cult: Bubastis.** The cats on the Moon, a new area of the map that only they can reach. They have no Cultists, and
  every cat gives them Power. They play in sand.
- **Many more neutral units.** New options at the setup page: the Dreamlands Underworld Monsters (Ghast, Leng Spider, Gug),
  the Cosmic Terrors (Great Race of Yith, Dhole, Quachil Uttaus), Something About Cats and the Satyr.
- **The Primeval Map.** A prehistoric Earth for three or five players, where Glaciers creep over the map and abandon Gates
  for good, which makes everyone rich in Power. Choose it as the layout when you create a game.
- **Twelve more Independent Great Old Ones.** Atlach-Nacha, Bokrug, Father Dagon, Ghatanothoa, Gobogeg, Byatis, Nyogtha,
  Tulzscha, Gla'aki, Daoloth, Y'Golonac and Hagarg Ryonis, each with a check box. The guide has their rules and strategy.
- The guide no longer mentions the gold stars or clicking a unit for its card.
- **Neutral spellbooks are spellbooks.** One you hold covers a requirement like any other, and one swapped in for Ghroth or
  The Thousand Forms at setup reads as your cult's own. Those still to take are under *Neutral items*.
- **No more lists of Loyalty Cards on the cults.** A cult's cards show under its *Units*. An Independent Great Old One's
  spellbook and requirement are on its hover card.

## 0.48.0 - 2026-11-02

### Bingle Core 0.48.0
- **Blooms fits your screen.** A game of Blooms no longer scrolls: the whole board stays in view, with the players,
  chat, history and tutorial beside it (below it on an upright phone). The board and the cards sit side by side and grow with
  the window, with any spare width on the outside. The cards scroll by themselves and leave the board where it is. Blooms is the first game to get this page, so tell us how it plays before the others follow.
- **Your move stands out.** On your turn, the card with your stones and buttons lights up and stays in view however far
  you have scrolled the column, without moving anything else.
- **Keep typing.** A move arriving while you are typing a chat message no longer takes the cursor out of the message box,
  in any game.
- **Tutorials can go back.** A Back button returns to any earlier step of a tutorial, so you can read it again with
  the board as it stood. Next brings you forward to where you were.

### Blooms 0.3.0
- **Warm and Cool.** The two sides are now called Warm (red and yellow) and Cool (black and blue), everywhere including
  the history, and their names and colour swatches blend their two colours.
- **The captured stones are on show.** Each player's card shows the stones they have captured, in their colours, and how
  many are needed to win, such as *Captured: 3/15*.
- **Shorter words on the controls.** Each stone's button says *Place*, *End turn* is now *End*, and the line above them
  says what you can do next. Once you pick a stone, the card says *Place* with a picture of it, and *Back* puts it down
  again; there is no separate *Reset* button.
- **More room for the board.** The bar above the board is gone. The result of a game and its *Rematch* button are in the
  card with your stones, as are the controls for looking through the history and for analysis.
- **A longer tutorial.** It uses the new names and buttons, and now explains blooms and liberties with two boards to count
  on yourself. *Need a hint?* gives the answer.

## 0.47.0 - 2026-11-02

### Bingle Core 0.47.0
- **Vote someone out of a real-time game.** When a player has stopped playing, the others can vote to have a bot take
  their seat for good, using *Vote to replace* beside their name. It takes more than half of the connected players,
  counting the player in question, and the vote lapses after 2 minutes. It needs at least two others connected, and
  rated games don't have it. Whoever is voted out can go on watching, but can't play in that game again.
- **Game clocks at bigger tables.** Any game for two or more can now have a game clock. At a table of three or more,
  running out of time, on a clock or a time per move, no longer ends the game for everyone: a bot takes your seat and
  the others play on. If no other person is left at the table, the game ends with no winner.
- **Who played what stays on the record.** A seat a bot has taken says who it replaced, and the history still shows that
  person beside the moves they made.

## 0.46.0 - 2026-11-02

### Bingle Core 0.46.0
- **Real-time tournaments.** An organiser can now set a tournament to start at a set time, with everybody playing at
  once. Check in on the tournament's page before the start, or you are dropped from it.
- **One game at a time.** When a pairing plays two games, the second starts as soon as the first ends, and you are told
  so with a link to it. The next round starts after a short break that the organiser chooses.
- **Make your first move in time.** In these tournaments, a player who has not made their first move by the deadline
  forfeits that game. If neither player came, nobody scores. Ratings don't change either way.

## 0.45.0 - 2026-11-02

### Bingle Core 0.45.0
- **Real-time games.** Tick *Real-time game* when you set up any game for two or more, and everyone is expected to stay
  until it ends. If the game has a clock or a time per move, running out of time is the only consequence, and it now
  happens on time even when nobody is looking.
- **No more waiting forever.** With no timer, a slow player gets a reminder after 2 minutes. After 5 more, the table is
  told, and anyone else who is connected can **let a bot play that one turn**, or you can all keep waiting. You choose
  both times at setup. A player who has left has their turns played until they come back.

## 0.44.0 - 2026-11-02

### Bingle Core 0.44.0
- **See who is connected.** A player whose connection has gone now shows as *disconnected* beside their name, in every
  game, until they are back. A reload or a brief drop won't flag anyone. In a game played over days, your opponent will
  usually show this, which only means they don't have the game open.
- **The "watching" eye is gone**, along with clicking your colour box to switch it on. Bingle now works this out from
  the connection, so there is nothing to set.
- **Turn mail is more reliable.** A page whose connection has quietly died no longer counts as you being there, so the
  mail for your turn still goes out.

## 0.43.0 - 2026-11-02

### Bingle Core 0.43.0
- **Variants are a list now.** Setting up a game with variants, you choose from a *Variant* list with their full names,
  in place of the row of little badges. This goes for every game that has them.
- **A game's own title.** A family of variants is always shown under the name of its main game.
- **A tutorial can belong to one variant.** It is then offered only while that variant is chosen.

### Solitaire 0.1.1
- The game is called **Solitaire** on the main page. It was showing up as Bowling Solitaire.
- Choosing a variant now shows a game of it in progress, in the set of cards you have chosen.
- The Bowling Solitaire tutorial is offered only when Bowling Solitaire is chosen.
- **Hover over a card on the table to see it twice as large**, as with a hand of cards, even one mostly covered
  by others. It goes away while you drag.

## 0.42.0 - 2026-11-02

### Bingle Core 0.42.0
- **Games for one player.** A game can now have a single seat. It starts as soon as you create it, with no
  opponent to choose. **Give up** ends it whenever you like, and **← Back** takes moves back as far as you want,
  since there is nobody to wrong. Moves go straight in, without waiting for Submit.

### Solitaire 0.1.0
- **Solitaire is here**, all of it for one player, and every game can be played with any of the themed sets of cards:
  - **Solitaire** (Klondike), turning over one card at a time or, in the harder version, three.
  - **Spider**, with four suits, two suits, or one for learning it.
  - **FreeCell**, with every card face up and almost every deal winnable.
  - **Bowling Solitaire**, Sid Sackson's game of ten frames played with twenty cards, with a score sheet and
    a tutorial under *Learn to play*.
- Click a card and then where it should go, or drag it. A card with only one place to go goes straight
  there, and clicking a selected card again sends it to its foundation. **Finish** appears when everything can go
  home by itself.
- With a themed set of cards, the suits are not red and black any more. So in Solitaire and FreeCell, where
  colours matter, each card also shows a small mark with the classic suit it stands for.

## 0.41.0 - 2026-11-02

### Bingle Core 0.41.0
- **Themed sets of cards.** When you set up a game played with ordinary suits and ranks (Nyet! today), the
  new *Cards* choice lets you play with one of five sets: **Ancient Civilizations**, **Amphibians**, **Fantasy
  Creatures**, **Lovecraftian** or the **Atomic Age**. Each has its own card back, colours and lettering for the
  ranks, eight suits and four jokers.
- **Everyone at the table gets the same cards.** The set is chosen once, by whoever creates the game, and
  stays for the whole game, so "the frogs are trump" means the same to all of you. A rematch keeps it. Only
  the look changes: hover over a card or a trump box to see what the rules call that suit.

## 0.40.0 - 2026-11-02

### Bingle Core 0.40.0
- **Tutorials can show examples that aren't part of the game.** A step can now draw any board, whether or
  not a real game could reach it, and can mark a move in red with a cross to show what you mustn't do.
- **Example boards are labelled.** While a tutorial shows an example, the top of the page says so, instead of
  telling you it's your turn.

### Blooms 0.2.2
- The tutorial is rewritten, with worked examples of a fenced bloom and how it is captured.

## 0.39.1 - 2026-11-02

### Bingle Core 0.39.1
- **You can turn off turn mail for a tournament game.** The switch in the game's options was greyed out
  and asked you to set up an address you already had. It now works, and says where the mail goes.
- **A forced pass can be played.** When passing was your only legal move, in Fendo for example, the page
  showed nothing to press and the game stopped until your clock ran out. It now offers the move.
- **Turn mail says why it came.** It no longer says you asked for it, as a tournament game turns it on for
  you, and it tells you where to turn it off.

## 0.39.0 - 2026-11-02

### Bingle Core 0.39.0
- **Tournaments.** The new **Tournaments** tab lists round robin and Swiss tournaments for signed-in
  players. Enter one, and your games appear round by round. They are played by correspondence, so you
  move when you have time, and everybody's clock keeps running until they do.
- **How it works.** Each pairing plays two games with sides swapped, unless the organiser chooses one.
  A win scores 1, a draw ½, and a bye scores as winning the pairing's games. Ties are broken by
  Sonneborn-Berger, with Buchholz first in Swiss, and a tie that remains is a shared place.
- **Tournament games are straight.** There is no Auto-Play in a tournament game, and it is rated unless
  the organiser says otherwise. Using a bot or another program to choose your moves is against the rules.
- **Turn mail comes by itself.** If your account's email address is confirmed, you are mailed when it is
  your turn in a tournament game. You can turn that off in the game.
- **Leaving.** You can withdraw at any time. You forfeit your games in progress and any still to come,
  and what you have played stands.
- **Who can run one.** Administrators, for now. They can open it up to every account on the admin page.
- **A move now stands once your turn is over, in every game.** Undo and asking to take a move back are
  gone, including against bots. There is just one **Back** button: it clears a move you haven't played yet,
  or takes back a step of your turn while it's still your turn and nothing has been rolled or drawn, like
  the first of two stones in Blooms.
- **Submit in every game.** A move you can't take back now waits for you to press **Submit**, in the games
  that used to play it straight away too. Buttons like **End turn** still play at once. Don't want the extra
  click? Turn it off in the game's options, under display.
- **The move list shows finished moves.** The steps of a turn appear there once the turn is over.

### Cthulhu Wars 0.1.1
- The rules window describes the new **Back** button.

## 0.38.0 - 2026-11-02

### Bingle Core 0.38.0
- **Set the game clock to the minute.** Instead of a short list of times, type any whole number of
  minutes from 1 to 60, for both players or for each one. 2 hours and 1 day are still on the list, for
  slower games.
- **Each game suggests its own time.** Choosing a game clock now starts from a time that suits the game
  and the size of its board. The clock is still off until you choose it.

### Blooms 0.2.1
- A game clock now suggests 5 minutes on the small board, 10 on the standard board and 15 on the big
  one.

## 0.37.1 - 2026-11-02

### Bingle Core 0.37.1
- **Reaction switches stay where you put them.** In Cthulhu Wars, a box in the reaction options could
  flip back for a moment after you clicked it if other players were moving, so your next click could
  do the opposite of what you meant.

## 0.37.0 - 2026-11-02

### Bingle Core 0.37.0
- **Ratings.** Every game of two now has a rating of its own, across all its sizes and variants. Tick
  **Rated game** when you set up a game with a game clock, and the result changes both players'
  ratings. You need to be signed in, and so does your opponent.
- **A rated game is played straight.** Both players get the same clock, there are no bots, and undo,
  take-backs and Auto-Play are off. Using a bot or another program to choose your moves is against the
  rules.
- **See how you stand.** Your rating shows beside your name during a rated game, with how much it
  changed when the game ends, and all your ratings are on your profile. The new **Ratings** tab on the
  home page has a leaderboard for each game, once you have played 5 rated games against at least 3
  different people. A question mark means a rating is provisional and could still move a long way.
- **What doesn't count:** a game where a player never made a move, and more than 5 rated games of one
  game between the same two people in a day.

## 0.36.0 - 2026-11-01

### Bingle Core 0.36.0
- **Clocks can add time for each move.** When you set up a game with a game clock, *Added per move*
  gives each player a few extra seconds, up to a day in a long game, whenever they finish a turn. A
  player short of time then still has a chance to finish, rather than losing to a slow connection.
- **A server restart no longer costs you time.** If the server goes down during your turn, on a game
  clock or a time per move, you can no longer lose on time because of it. You get the whole of that
  turn's time back, too.
- **Taking back moves now puts the clocks right.** Undoing a move or agreeing to a take-back removes
  any time those moves added. Taking back a pie-rule swap now returns each player's own time to them;
  before, the players kept each other's clocks.

## 0.35.0 - 2026-11-01

### Bingle Core 0.35.0
- **A tutorial for Mycelium.** *How to play Mycelium* teaches the game by playing it: you place your
  first nodes, take the centre, and make the spore and hyphae spreads of a whole confluence. Find it
  under **Learn to play** when you choose the game, or in the *Tutorials* tab of its rules.

## 0.34.1 - 2026-11-01

### Bingle Core 0.34.1
- **The stone under your mouse is now the colour you chose.** In Blooms, picking yellow still showed
  a red stone on the board until you placed it; the preview now matches your choice. The same goes
  for the black and green nodes of Mycelium.

## 0.34.0 - 2026-11-01

### Bingle Core 0.34.0
- **A new way to see hex boards.** In any game on a hex board, *Options* now has a *Board style*:
  *Hexagons*, or *Hex grid*, where the spaces are points joined by lines and the stones sit on the
  points, as on a Go board. Your choice is remembered for each game.
- **Meridians now starts on the hex grid**, the way the game is printed. You can switch it back to
  hexagons in *Options*.

## 0.33.0 - 2026-11-01

### Bingle Core 0.33.0
- **A tutorial for Cthulhu Wars' High Priests.** It opens in the second round of a four-player game
  of the core cults and shows how a High Priest is recruited, holds a Gate, is captured, and is
  sacrificed for Power with the Unspeakable Oath — on your own turn, and on another player's when you
  have no Power left, to get back into the round.
- Tutorials can now begin in the middle of a game, from a position written out rather than played to.
- In a tutorial you are asked about every reaction, as the tutorial expects, and the buttons to stop
  being asked are gone: pressing one could leave the tutorial stuck.

## 0.32.0 - 2026-11-01

### Bingle Core 0.32.0
- **A tutorial for Meridians**, and a new one for Blooms. Both start with what the game is about and
  a finished game, then the board, the pieces and a turn, and then each part of the game as you play
  it.
- **Tutorials use the game's own buttons.** In Blooms you press the red or yellow stone and then the
  cell, as in any game: the button to press is picked out, and the others wait.
- A tutorial has no chat or voice call — there is nobody to talk to.

## 0.31.0 - 2026-11-01

### Bingle Core 0.31.0
- **Tutorials, to learn a game by playing it.** Games that have one show a *Tutorial* badge in the
  list of games. Choose the game and start it under **Learn to play**, or open the game's rules and
  pick it from the new **Tutorials** tab, where a tutorial you have finished is marked done. The
  first is for Blooms: placing stones and capturing a bloom. Tutorials for Meridians and Courtfall
  come next.

## 0.30.3 - 2026-11-01

### Bingle Core 0.30.3
- **The tutorial page.** A tutorial's room now guides you: each step's explanation above the board,
  the cells it talks about ringed and an arrow where it points, only the move it asks for offered,
  a hint when you want one, your opponent's scripted replies played for you, and at the end a button
  to finish the game against a bot. The list to open tutorials from comes next.

## 0.30.2 - 2026-11-01

### Bingle Core 0.30.2
- **A tutorial can now be opened, through the API, in a room of its own.** It plays on a real board
  under the real rules, with you in every seat so its scripted replies can be played for you, and it
  stays out of "My games", the list of games going and turn emails. The page that guides you
  through one comes next.

## 0.30.1 - 2026-11-01

### Bingle Core 0.30.1
- **Groundwork for guided tutorials.** Games can now carry tutorials that teach by playing, and the
  first one, for Blooms, is written. There is no way to open one yet: the tutorial page comes next.

## 0.30.0 - 2026-11-01

### Bingle Core 0.30.0
- **Tic-Tac-Toe and Connect Four are no longer offered.** The bots and the other games now show
  everything they were there to show. If you have played them, your games are still there as a
  record: open one to see who played, how it ended and every move. A game of either that was still
  going ends now, with no result.
- **Reversi is just "Reversi"**, without "(Othello)", which is a trademark.
- Twelve Connect Four puzzles are gone with the game.

### Reversi 0.3.1
- Called Reversi everywhere.

## 0.29.0 - 2026-10-31

### Bingle Core 0.29.0
- **Games are now written in YAML instead of JSON.** Nothing changes at the table: every game plays
  exactly as before, and the API still speaks JSON. For anyone writing or changing a game, a
  definition can now carry comments, be split across several files (Cthulhu Wars' map variants each
  have their own), and reuse a block instead of copying it.
- **A misspelled field in a game's definition is now an error** that names the file and line,
  instead of being quietly ignored. Editors that read the new schema complete and check the fields
  as you type.
- `bingle dsl convert` turns a JSON game definition into YAML, and shows the JSON a YAML one becomes.

## 0.28.3 - 2026-10-31

### Bingle Core 0.28.3
- **Setting up two-step sign-in shows a QR code.** Point Google Authenticator (or any authenticator
  app) at it instead of typing the key; the key and the link are still there beside it.
- **An administrator without two-step sign-in is told what is missing.** Away from the server's own
  machine, the administration page now says to set up two-step sign-in and links straight to it,
  instead of saying it is only for administrators.
- **The administration page is easier to read:** the permissions line up in even rows under their
  headings, the limits are a table of users and guests, and the history has headings, local times
  and the kind of each event picked out.

## 0.28.2 - 2026-10-31

### Bingle Core 0.28.2
- **The server's log now says what happened to every email.** Whether mail is switched on, and for
  each confirmation, password reset or turn email whether it was sent, held back by the limits, or
  refused by the mail server — without ever writing the address down. The metrics count the same.
- **Signing up with an address that already has an account now tells its owner.** The page still
  answers as it always does, so it never says whose address it is, but the address gets an email
  saying it has an account already, with links to sign in and to reset the password (at most one an
  hour). The log and the administrator's audit log say so too.

## 0.28.1 - 2026-10-31

### Bingle Core 0.28.1
- **Signing in is where you would look for it.** The profile button in the top bar now says whether
  you are signed in, and its window has *Sign in* and *Sign up* — or, once you are signed in, your
  handle, your account page and *Sign out*. Before, they were only on the profile page.
- **Guests are no longer offered what needs an account.** The direct messages button, running bot
  batches and setting up turn emails only appear once you are signed in (or if the server's
  administrator opens them to guests); a guest is pointed to the sign-in instead.
- The sign-in and account pages ask for a **username** (it was "handle"), and the password boxes
  look like the rest.

## 0.28.0 - 2026-10-31

### Bingle Core 0.28.0
- **Direct messages, the list of who is here and writing in the global chat are now for
  signed-in players.** Everybody can still read the global chat, and guests still play, chat in
  their games and talk in their games' voice chat. A message, and a name in the list or the global
  chat, is now always the account it says: @handle.
- **Email notifications are for signed-in players too.** If you set them up as a guest, nothing
  stops: the games you switched them on for keep mailing you. And because your address is
  confirmed already, your profile page can make it your account at once — choose a handle and a
  password, and your games come with you.

## 0.27.0 - 2026-10-31

### Bingle Core 0.27.0
- **Take your games to your phone.** As a guest, your profile page can copy your guest key: open
  the link it gives you on your phone (or another browser) and that device becomes you, with every
  game you have going, ready to carry on. Whatever the phone was playing comes along too. If the
  key ends up somewhere it should not, *New key, same me* makes the old one stop working.
- **Your games follow you.** Active games now come from the server, so a game you started on one
  device shows up on the others — your account's, or your guest's.
- **Who is who.** Next to every player's name the game says whether they are a signed-in account
  (@handle) or a guest, in the seats and in the chat. Nobody can take an account's handle as their
  name in a game, and a handle you change is kept for you for a month.
- Your account page shows your address, lets you change your handle, lists where you are signed in
  and signs you out everywhere else. Administrators see the latest sign-ins and changes.

## 0.26.0 - 2026-10-31

### Bingle Core 0.26.0
- **Practice batches are for accounts now.** Running bot batches on the Analysis tab needs you to be
  signed in; reading a batch's report does not.
- **API tokens.** Your account page makes tokens for scripts and agents: each acts as you, within
  what you allowed it (playing, making games), and can be revoked there too.
- Guests can make three games a minute, and say ten things a minute in a game's chat; signed-in
  players more.
- For whoever runs the server: an **administration page** (`/admin`) says what guests and members
  may do, and changes it. `BINGLE_AUTH_MODE=closed` makes a server for its members only, and
  `BINGLE_SIGNUP=invite` lets people sign up only with an invitation link from that page. The
  defaults come from `bingle-capabilities.yaml` the first time a server starts; `bingle caps reset`
  applies the file again.

## 0.25.0 - 2026-10-31

### Bingle Core 0.25.0
- **Sign in with Google**, or any other provider the server is set up with: a button on the sign-in
  page. The first time, you choose your handle; after that the button signs you straight in. If you
  already have an account with the same email address, it is used — as long as the provider has
  checked the address. You can link and unlink providers on your account page, and an account made
  this way can set a password there too.
- For whoever runs the server: providers are set up with `BINGLE_OIDC_PROVIDERS` and a block of
  settings each; see the build guide.

## 0.24.0 - 2026-10-31

### Bingle Core 0.24.0
- **Two-step sign-in.** On your account page you can make signing in ask for a code from an
  authenticator app on your phone as well as your password. You get ten recovery codes for the day
  the phone is not to hand: each works once.
- **Forgot your password?** The sign-in page mails you a link to choose a new one. If your account
  uses two-step sign-in, the link asks for a code as well.
- Too many wrong passwords or codes in a row now make you wait a while before trying again.
- For whoever runs the server: administrators must set up two-step sign-in before they can do
  anything else, unless they are on the server's own machine. `bingle account reset` now also
  removes two-step sign-in, for somebody who lost their phone and their recovery codes.

## 0.23.0 - 2026-10-31

### Bingle Core 0.23.0
- **Accounts, if you want one.** You can still just play, but you can now sign up with a handle
  (shown as @handle), your email address and a password, and sign in from the profile page. The
  games you played as a guest in that browser become your account's when you sign up, and signing
  in on another browser brings that browser's games along too. Signing up needs the server to be
  able to send email; the confirmation link works for two days.
- Changing your password signs you out everywhere else.
- For whoever runs the server: a new server makes an administrator, `@admin`, whose password is
  printed when it first starts (or is `bingle123` on a server only this machine can reach), and
  asks for a new one at the first sign-in. `bingle account list`, `create` and `reset` manage
  accounts from the command line, and `BINGLE_SIGNUP=closed` turns signing up off.

## 0.22.0 - 2026-10-31

### Bingle Core 0.22.0
- **Your seat is yours.** Until now anybody who knew a game's code and your name could make moves
  for you, read your hand or speak as you, just by typing your name. Now a seat belongs to the
  browser that took it: other people in the game, and anybody else, are turned away. Nothing
  changes in how you play — your browser remembers you by itself — but typing somebody else's
  name no longer takes their seat, so finish a game in the browser you started it in. (Moving to
  another device comes in a later version.)
- Seats in games started before this version are claimed by the first browser to play them.
- For people who write scripts against Bingle: `POST /api/guest` gives a script a guest key to send
  as `Authorization: Bearer`, so its seats are protected too. A script that sends no key plays as
  before, on seats it took the same way.

## 0.21.0 - 2026-10-31

### Mycelium 0.1.0 — a new game
- **Mycelium**, by Miles Stevenson, joins the list, so it can be played and tested here. Two fungal
  colonies place nodes on a hexagonal board; whoever takes the centre starts a **confluence**, in
  which every cluster spreads in turn, smallest first. A spore that can see an enemy node down a
  line of empty cells turns it to its own colour; a cluster touching the enemy grows into it along
  a winding chain, as many nodes as its longest strand. A colony left with nothing when the
  confluence ends has lost. You make each spread yourself, one node at a time, the board shows which
  clusters have already spread, and the rules page walks through a confluence.

### Bingle Core 0.21.0
- The catalog lists Mycelium.

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
