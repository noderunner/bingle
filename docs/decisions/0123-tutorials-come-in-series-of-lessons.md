# 0123. Tutorials come in series of named lessons, and an account keeps the ones it has finished

Status: accepted (2026-11-06). It changes [ADR 0077](0077-tutorials-are-data-played-by-the-engine.md)'s page and its
record of finished tutorials; the format, the replay and the rooms are as there.

## Context

Cthulhu Wars teaches its rules in eight tutorials by topic, meant to be taken in order, but nothing said so: each was
a loose item of the list, and its end led back to the game picker. Every tutorial ended with the same three buttons,
*Finish this game against a bot*, *Start again* and *Back to the games*, though playing on against a bot from a
contrived Cthulhu Wars position teaches nothing, and replaying a tutorial is one click away in its list. A step's card
said *Step 3 of 8* and nothing of what the step was about. Which tutorials had been finished was kept only by the
browser, so a player who signs in elsewhere lost them.

## Decision

- **A step is a lesson, and has a name** (`name`, required). The card's head is the tutorial's title and the lesson's
  name, `Combat · Assigning Pains`. At its foot is a dot for each lesson, this one lit, between back `‹` and forward
  `›`; *Step N of M* is gone from sight and kept for screen readers.
- **A tutorial may name a `series`**, taken in `order`. Without one it is a series of its own (keyed by its id, so two
  tutorials of one title are not merged). A series is of one kind. The lists show a series' tutorials together under
  its name, numbered, and mark the series *✓ Completed* once all are done.
- **The end of a tutorial with another after it in its series offers *Start Next Tutorial***, and its last lesson
  says what that covers (`The next tutorial covers **Power**.`), which a test checks. The served tutorial carries
  `next`, so a client need not work out series order itself.
- **Playing on against a bot is the tutorial's to offer**, from a lesson marked `keep_playing`, and is called
  **Keep Playing**. *Start again* is gone from the end; it stays on the off-script card, the one way on from there.
  *Back to the games* becomes **Return to Tutorials**, the game's own list (`/?game=<id>&tab=tutorials`).
- **A signed-in account keeps the tutorials it has finished** (`tutorials_done`). The page reports the end with
  `POST /api/tutorials/done`, and the server records it only when the room's moves are the whole script and the
  account holds the learner's seat, so the mark cannot be had by asking. A guest's browser keeps its own list, as
  before, and a list shows a tutorial done if either says so. It is still not an achievement.

## Consequences

- Every shipped tutorial's lessons were named, and the eight Cthulhu Wars rules tutorials are the series *Core Game*;
  the High Priests stand alone.
- A tutorial file without lesson names no longer loads; private tutorials need them added.
- A finished reading lesson at the end of a script is reported once the last move is played, so the server's check
  does not need to know what the learner has read.
