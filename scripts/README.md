# Scripts

Helper programs. None is needed to build, run or test bingle.

| Script | What it does |
|---|---|
| `ghapp/` | `go run ./scripts/ghapp token\|api\|push`: acts on GitHub as the repository's GitHub App, for agent work; `push` recreates a branch's commits through the API so that GitHub signs them (CLAUDE.md). |
| `smoke-test.sh` | Builds the binary, runs it on a real port and checks `/healthz` (CI runs it). |
| `run.example.sh` | An example of starting bingle with HTTPS, email and a TURN relay. Copy it to `run_bingle.sh` (git ignores that name) and edit. |
| `gendocs/` | `go run scripts/gendocs/main.go > docs/games.md` regenerates the game catalog. |
| `genpuzzles/` | `go run scripts/genpuzzles/main.go` regenerates and re-proves `internal/puzzles/puzzles.json`. |
| `tune/` | Plays bots against each other to tune the Cthulhu Wars bot weights (`internal/games/cthulhuwars/tuned.json`). |
| `cardsheet/` | `go run scripts/cardsheet/main.go <url> nyet internal/games/nyet/guide/cards.svg` draws every card of a card game as one picture for its rules, with the page's own card drawing in headless Chrome. Run it again when the cards or the suits change. |
| `shot/` | Takes screenshots of a running server with headless Chrome, for reviewing UI changes (the images in `docs/screenshots`). |
| `bingle_env.py` | A Gym-style Python client for the training environments (docs/mcp.md); standard library only. |
| `cwmapart.py` | `python3 scripts/cwmapart.py --map NAME --board SCAN` traces a Cthulhu Wars map (`earth-3p`, `earth-3-5`, `earth-5-3`, `earth-5p`, `primeval-3p`, `primeval-3-5`, `primeval-5-3`, `primeval-5p`) from a scan kept outside the repository, and writes its layout, its generated picture and the glyphs printed on it (ADR 0095). |
| `guide_diagrams.py`, `guide_lib.py` | Draw the SVG diagrams of the rules guides (`python3 scripts/guide_diagrams.py [game...]`). |
| `cwglyphs.py`, `cwmaps.py` | Built the Cthulhu Wars sigils and map layouts from reference pictures. The pictures are inputs only and are not in the repository; the results are checked in (`internal/games/cthulhuwars`, `internal/games/cthulhuwars/web/cw-glyphs.js`), so you never need to run these. |
