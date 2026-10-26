# Scripts

Helper programs. None is needed to build, run or test bingle.

| Script | What it does |
|---|---|
| `smoke-test.sh` | Builds the binary, runs it on a real port and checks `/healthz` (CI runs it). |
| `run.example.sh` | An example of starting bingle with HTTPS, email and a TURN relay. Copy it to `run_bingle.sh` (git ignores that name) and edit. |
| `gendocs/` | `go run scripts/gendocs/main.go > docs/games.md` regenerates the game catalog. |
| `genpuzzles/` | `go run scripts/genpuzzles/main.go` regenerates and re-proves `internal/puzzles/puzzles.json`. |
| `tune/` | Plays bots against each other to tune the Cthulhu Wars bot weights (`internal/games/cthulhuwars/tuned.json`). |
| `cardsheet/` | `go run scripts/cardsheet/main.go <url> nyet internal/games/nyet/guide/cards.svg` draws every card of a card game as one picture for its rules, with the page's own card drawing in headless Chrome. Run it again when the cards or the suits change. |
| `shot/` | Takes screenshots of a running server with headless Chrome, for reviewing UI changes (the images in `docs/screenshots`). |
| `bingle_env.py` | A Gym-style Python client for the training environments (docs/mcp.md); standard library only. |
| `guide_diagrams.py`, `guide_lib.py` | Draw the SVG diagrams of the rules guides (`python3 scripts/guide_diagrams.py [game...]`). |
| `cwglyphs.py`, `cwmaps.py` | Built the Cthulhu Wars sigils and map layouts from reference pictures. The pictures are inputs only and are not in the repository; the results are checked in (`internal/games/cthulhuwars`, `internal/web/static/js/cw-glyphs.js`), so you never need to run these. |
