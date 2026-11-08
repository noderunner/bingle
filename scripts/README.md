# Scripts

Helper programs. None is needed to build, run or test bingle.

| Script | What it does |
|---|---|
| `ghapp/` | `go run ./scripts/ghapp token\|api\|push`: acts on GitHub as the repository's GitHub App, for agent work; `push` recreates a branch's commits through the API so that GitHub signs them (CLAUDE.md). |
| `smoke-test.sh` | Builds the binary, runs it on a real port and checks `/healthz` (CI runs it). |
| `run.example.sh` | An example of starting bingle with HTTPS, email and a TURN relay. Copy it to `run_bingle.sh` (git ignores that name) and edit. |
| `gendocs/` | `go run scripts/gendocs/main.go > docs/games.md` regenerates the game catalog. |
| `genpuzzles/` | `go run scripts/genpuzzles/main.go` regenerates and re-proves `internal/puzzles/puzzles.json`. |
| `tune/` | Plays bots against each other to tune the guide weights of a game whose practice study has knobs (ADR 0126). |
| `cardsheet/` | `go run scripts/cardsheet/main.go <url> <game> internal/games/<package>/guide/cards.svg` draws every card of a card game as one picture for its rules, with the page's own card drawing in headless Chrome. Run it again when the cards or the suits change. |
| `shot/` | Takes screenshots of a running server with headless Chrome, for reviewing UI changes (the images in `docs/screenshots`). |
| `bingle_env.py` | A Gym-style Python client for the training environments (docs/mcp.md); standard library only. |
| `guide_diagrams.py`, `guide_lib.py` | Draw the SVG diagrams of the rules guides (`python3 scripts/guide_diagrams.py [game...]`). |
