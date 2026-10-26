#!/usr/bin/env bash
# End-to-end smoke test: builds the real binary, runs it as a real process
# bound to a real port, and curls /healthz against it. Unlike the unit and
# integration tests, this never touches App directly, so it's the only check
# that exercises main.go + httpserve's actual net.Listen path.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

WORKDIR="$(mktemp -d)"
trap 'kill "${PID:-}" 2>/dev/null || true; rm -rf "$WORKDIR"' EXIT

PORT=18080
export BINGLE_ADDR=":${PORT}"
export BINGLE_DB_PATH="${WORKDIR}/smoke.db"

go build -o "${WORKDIR}/bingle" .

"${WORKDIR}/bingle" &
PID=$!

for i in $(seq 1 20); do
  if curl -fsS "http://127.0.0.1:${PORT}/healthz" -o "${WORKDIR}/response.json" 2>/dev/null; then
    break
  fi
  sleep 0.25
done

if ! kill -0 "$PID" 2>/dev/null; then
  echo "smoke test FAILED: server process exited early"
  exit 1
fi

if ! grep -q '"status":"ok"' "${WORKDIR}/response.json"; then
  echo "smoke test FAILED: unexpected /healthz response:"
  cat "${WORKDIR}/response.json"
  exit 1
fi

echo "smoke test PASSED: /healthz returned $(cat "${WORKDIR}/response.json")"

# The game catalog is registered, a Tak game can be created and a move made
# through the real HTTP stack, and the static UI assets are served.
curl -fsS "http://127.0.0.1:${PORT}/api/game-defs" -o "${WORKDIR}/defs.json"
for id in tak-5x5 demo-freeplay reversi; do
  if ! grep -q "\"${id}\"" "${WORKDIR}/defs.json"; then
    echo "smoke test FAILED: game definition ${id} is not registered"
    cat "${WORKDIR}/defs.json"
    exit 1
  fi
done

CODE=$(curl -fsS -X POST "http://127.0.0.1:${PORT}/api/games" \
  -H 'Content-Type: application/json' \
  -d '{"game_def_id":"tak-5x5","player_name":"Smoke","opponent":"random"}' |
  sed -n 's/.*"room_code":"\([A-Z0-9]*\)".*/\1/p')
if [ -z "${CODE}" ]; then
  echo "smoke test FAILED: could not create a game against a bot"
  exit 1
fi
curl -fsS -X POST "http://127.0.0.1:${PORT}/api/games/${CODE}/moves" \
  -H 'Content-Type: application/json' \
  -d '{"player_name":"Smoke","notation":"c3"}' -o "${WORKDIR}/move.json"
if ! grep -q '"bot_moves"' "${WORKDIR}/move.json"; then
  echo "smoke test FAILED: the bot did not reply"
  cat "${WORKDIR}/move.json"
  exit 1
fi
curl -fsS "http://127.0.0.1:${PORT}/static/js/game.js" -o /dev/null
curl -fsS "http://127.0.0.1:${PORT}/game/${CODE}" -o /dev/null

# Training and study features: puzzles, an in-memory environment with
# planes and action IDs, and a hint.
curl -fsS "http://127.0.0.1:${PORT}/api/puzzles?max_win_in=1" -o "${WORKDIR}/puzzles.json"
if ! grep -q '"win_in"' "${WORKDIR}/puzzles.json"; then
  echo "smoke test FAILED: no puzzles are served"
  exit 1
fi
curl -fsS -X POST "http://127.0.0.1:${PORT}/api/envs" \
  -H 'Content-Type: application/json' \
  -d '{"game_def_id":"tak-5x5","opponent":"random","include":["planes","action_ids"]}' -o "${WORKDIR}/env.json"
if ! grep -q '"legal_action_ids"' "${WORKDIR}/env.json" || ! grep -q '"planes"' "${WORKDIR}/env.json"; then
  echo "smoke test FAILED: environment observation lacks planes / action ids"
  exit 1
fi
curl -fsS -X POST "http://127.0.0.1:${PORT}/api/games/${CODE}/hint" \
  -H 'Content-Type: application/json' \
  -d '{"player_name":"Smoke","bot":"tactical"}' -o "${WORKDIR}/hint.json"
if ! grep -q '"notation"' "${WORKDIR}/hint.json"; then
  echo "smoke test FAILED: no hint"
  cat "${WORKDIR}/hint.json"
  exit 1
fi

echo "smoke test PASSED: catalog, game creation vs a bot, a move with a bot reply, static assets, puzzles, environments and hints"
