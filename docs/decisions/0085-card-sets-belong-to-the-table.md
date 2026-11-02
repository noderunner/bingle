# 0085. A set of cards belongs to the table

Status: accepted (2026-11-02)

## Context

People asked for themed sets of cards that any game with ordinary suits and ranks could use: each with
its own back, eight suits, four jokers, palette and lettering. Board themes (ADR 0053) are the obvious
model, and they are each viewer's own choice, kept in the browser. Cards are different. A board theme
changes how a stone looks, and a stone is still on its square. A suit is something players talk about: if
one player's hearts are another's frogs, "the frogs are trump" means nothing to half the table.

## Decision

**The set is chosen when the game is set up and is the same for everyone.** `card_set` is a field of
`POST /games` and of MCP's `create_game`, stored with the game (`games.card_set`) and reported with it. It
cannot be changed later, and there is no per-player setting. A rematch and *Play from here* keep it.
Tournament games use the game's own cards, since a tournament has no such setting yet.

**It is a look, not a rule.** The engine, the notation and the rules do not know about it: `blue-9` stays
`blue-9`, and a bot plays the same. It lives on the game rather than in the definition or the setup
options, so that choosing one does not make a derived game (ADR 0028) with its own id, ratings and leaderboard.
A tooltip gives both names, *9 of Tree Frogs (hearts)*, because the rules and the move list use the game's.

**What can wear a set is decided by the server.** A deck is standard
(`catalog.CompiledCards.Standard`) when it has at most eight suits and nothing but plain cards of a
suit and a rank, and jokers (the tag `joker`). A card with rule text or symbols says something the art would
lose. No game opts in: a new game with a standard deck has the sets the moment it is registered.

**How a game maps onto a set.** Suits go by place: a game's suits, in `suit_order`, wear the first suits of
the set, so a poker deck uses suits one to four. Ranks are the game's own: any integers and letters, with
`rank_list` as the short way to write them. An integer from 2 to 10 lays out that many pips, 1 or A one large
one, and anything else is a framed court card. Each joker has the colours of two suits (one and two, three
and four, and so on), and a game's first joker is the set's first; a second copy of one joker type is the next.

**Everything is drawn in code.** As with the board themes, a set is vector art in `js/card-sets.js`, with no
picture files. The ranks are lettered too: `js/card-lettering.js` has one skeleton of strokes for the digits
and capitals, and each set has a pen that makes it a typeface of its own. A web font would need a file
for each set in the binary, with a licence to carry for each, and a list of system fonts would come out the
same on most machines, which defeats the point of giving each set its own lettering.

## Consequences

The server and the page each hold the list of sets, one for checking and one for drawing; a test holds them
together. A label outside digits and basic letters falls back to the page's own type. A set can be added in
one file plus one id, and one with fewer than eight suits or four jokers fails the tests.
