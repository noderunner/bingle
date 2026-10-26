# Project: bingle
## Problem & users
Human and AI's are the users that will be playing custom abstract turn based games (similar to go, chess, etc.) via web UI or a remote client connected via API. 
## Core capabilities (v1)
- Separated API layer from the UI layer so that either the default web UI can be used by players or custom clients remotely conecting to the server can be used.
- TLS support for the API connections
- separated game logic so that multiple custom games with their own rules can be defined and played as separate modules
- include a demo game with no rules which simply allows users to move pieces of 2 colors around a Go board to test
- ability for AI models to easily implement MCP for playtesting games and serving as an opponent
## Explicitly out of scope
- no need for rbac/auth, just ask users to provide a name during play. this will be a casual application with low security requirements
- no need to implement a full set of game rules
- no need for audio or animations support initially
## Constraints
- Prefer a golang oriented stack with well supported libraries
- decide on a simple, easy to use database backend appropriate for the task
- all individually running processes should easily run inside the same container for trivial deployment and testing
## Decisions already made
- ability to support custom games
- support for both human and AI model play
## Open questions
- Graphics representation is open. could be vector graphics, ascii, png images, or configurable depending on the client being used.
- No strong opinions on UI layout
- A few basic obvious features implemented however they make sense like turn tracking, player turn notifications, game timers, etc.
- Custom game implementation approach. Custom games that describe rules, components, etc. perhaps implemented in json or yaml for ease of use, with a library of backend game related functions? Whatever makes sense so things like how pieces move, how game objectives are defined, how turns are taken, etc. can be defined in a streamlined way.
