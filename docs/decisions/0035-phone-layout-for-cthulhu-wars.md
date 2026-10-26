# 0035. A phone layout for area-map games (Cthulhu Wars)

Status: accepted (2026-10-17)

## Context

The game page is a desktop layout: the map, a column of cult cards, the chat and the log side by side. On a phone the map is tiny, the controls sit far down a long scroll and every tap target is small.

## Decision

Area-map games get a second layout, built from the same pieces by `renderMobile` in `js/game.js` and `js/mobile.js`:

- One full-screen view at a time under a slim top bar (Rules, Options), swiped between with CSS scroll-snap: **Map**, **Actions**, **Round** (phases, turn order, Ritual track), one page per **cult** (own cult first, in table order), **History**, **Chat**.
- A bottom bar of six large buttons (thumb reach, safe-area padding), a strip of cult chips on the cult pages, a pulsing mark on Actions when it is the player's turn, an unread count on Chat. A new turn buzzes the phone once where the browser allows it.
- The map page keeps the map filling the screen with the move bar (the choices) docked beneath it, so a move can be made without leaving the map. The map keeps its own pinch/drag zoom (`touch-action: none`), so swipes that start at the left or right screen edge turn the page; the bar works too. In the phone layout the zoomable view takes the shape of the map's box, so zooming in uses the whole height.
- The page is rebuilt on every event: the current view and each view's scroll position survive it, and while the chat field has focus the redraw waits (the keyboard stays up).
- Automatic choice: `(pointer: coarse) and (max-width: 900px)`. A mouse in a narrow window keeps the desktop page. The options window has a "Page layout" setting (Automatic, Phone / tablet, Desktop), stored in `localStorage` as `bingle.layout`.
- The lobby and games with other board styles keep the desktop layout.

## Consequences

- The move bar appears twice (map dock and Actions view); the copy has its ids removed.
- Hover cards do not exist on touch; unit information comes from tapping, as before.
- Tests run with a fine pointer, so they get the desktop layout unless the phone is emulated (`tests/app/mobile_ui_test.go`).
