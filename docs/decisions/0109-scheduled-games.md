# 0109. Scheduled games, and agreeing on a time

Status: accepted (2026-11-05)

## Context

The maintainer asked for real-time games to be set up for later, by accounts, with invitations, a
calendar invitation for those going, and the option of offering several times and letting the players
find the earliest they can all make.

## Decision

- **A scheduled game is a room whose lobby stays open.** It is made at once, as a lobby, and never
  starts by itself. People chat and choose seats in it up to the start, as in any lobby.
- **From its time on, any seated player can press Start** once the seats are full, so that an absent
  host cannot keep the others waiting. The host can still start early (the maintainer's choice).
- **Only real-time games, and only accounts, can schedule.** The setup offers it with *Real-Time Game*.
- **Anybody can be invited by their exact handle, or from the friends list** (the maintainer's choice).
  There is no search of accounts, as that would be a way to list them, and no more invitations than
  open seats. The opt-out of ADR 0105's invitations covers these, and is told to the one asking.
- **The *Invite only* box starts ticked** for a scheduled game (the maintainer's choice), and can be
  unticked. A game weeks away has no business filling the public lists.
- **A time is agreed by everybody counted.** Those counted are the creator, and everybody invited who has
  not declined. The creator starts with all their times ticked. Once all have answered, the earliest
  time still to come that they all can make is the game's. A silent invitation holds it up until
  the last time has passed, or until the creator presses *Schedule with those who've answered*, which
  leaves them out (the maintainer's choice). Saying which times you can make accepts an invitation.
- **With no common time, its creator has 48 hours** to offer new times or cancel. Everybody can still
  change their answers in that time, and if a common time appears, it is taken. After that the game
  closes, with the end reason `unscheduled`, and all invited are told.
- **Calendar invitations are iCalendar files** in UTC, so the calendar shows them in its own time, under
  one UID and a rising `SEQUENCE`. Each names only its recipient: nobody's address reaches another
  player. A cancellation withdraws them. They go to confirmed addresses, within the mail limits, so the
  lobby also has *Add to calendar*, a file with no people in it, and the friends list carries the news.
  A game's length is unknowable, so the entry lasts an hour.
- **Times are stored in UTC.** The setup and lobby show them in the browser's time zone.

## Consequences

- A scheduled game nobody ever starts stays in its lobby, like any other lobby.
- `RunSchedules` looks at the open schedules once a minute, so a deadline can be a minute late.
