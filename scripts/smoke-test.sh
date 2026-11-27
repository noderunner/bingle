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
# Loopback, not ":${PORT}": a bind a network can reach now needs a secure
# connection (ADR 0065), and this script only ever talks to 127.0.0.1 anyway.
export BINGLE_ADDR="127.0.0.1:${PORT}"
export BINGLE_DB_PATH="${WORKDIR}/smoke.db"
# A port of its own for the metrics listener, rather than the fixed default:
# a developer with bingle already running would otherwise collide with it.
export BINGLE_METRICS_ADDR="127.0.0.1:$((PORT + 1))"

go build -o "${WORKDIR}/bingle" .

# An install pack (ADR 0147, docs/packs.md): the Draughts fixture, with its
# rules built as a module, the way a pack's maker builds one.
export BINGLE_PACKS="${WORKDIR}/packs"
mkdir -p "${BINGLE_PACKS}"
cp -R tests/testdata/draughts/pack "${BINGLE_PACKS}/draughts"
GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -buildmode=c-shared -trimpath \
  -o "${BINGLE_PACKS}/draughts/rules.wasm" ./tests/testdata/draughts/wasm

# The bingle-3d client for download (ADR 0151, docs/client-builds.md): a
# directory with one version's folder and one archive in it.
export BINGLE_CLIENT_BUILDS="${WORKDIR}/builds"
mkdir -p "${BINGLE_CLIENT_BUILDS}/0.0.1"
echo "not really an archive" >"${BINGLE_CLIENT_BUILDS}/0.0.1/bingle-3d-linux-x86_64.tar.gz"

"${WORKDIR}/bingle" &
PID=$!

# Up to twenty seconds: the pack's module is compiled at start-up.
for i in $(seq 1 80); do
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
for id in tak-5x5 hex-11 othello; do
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

# The pack's game is offered, and played against a bot like any other.
curl -fsS "http://127.0.0.1:${PORT}/api/game-defs" -o "${WORKDIR}/defs.json"
if ! grep -q '"pack-draughts-0.1.0"' "${WORKDIR}/defs.json"; then
  echo "smoke test FAILED: the pack's game is not offered"
  exit 1
fi
PACKCODE=$(curl -fsS -X POST "http://127.0.0.1:${PORT}/api/games" \
  -H 'Content-Type: application/json' \
  -d '{"game_def_id":"pack-draughts-0.1.0","player_name":"Smoke","opponent":"random"}' |
  sed -n 's/.*"room_code":"\([A-Z0-9]*\)".*/\1/p')
if [ -z "${PACKCODE}" ]; then
  echo "smoke test FAILED: could not create a game of the pack against a bot"
  exit 1
fi
FIRST=$(curl -fsS "http://127.0.0.1:${PORT}/api/games/${PACKCODE}/selections" |
  grep -o '"notation":"[^"]*"' | head -n 1 | sed 's/"notation":"\(.*\)"/\1/')
curl -fsS -X POST "http://127.0.0.1:${PORT}/api/games/${PACKCODE}/moves" \
  -H 'Content-Type: application/json' \
  -d "{\"player_name\":\"Smoke\",\"notation\":\"${FIRST}\"}" -o "${WORKDIR}/packmove.json"
if ! grep -q '"bot_moves"' "${WORKDIR}/packmove.json"; then
  echo "smoke test FAILED: the bot did not reply in the pack's game"
  cat "${WORKDIR}/packmove.json"
  exit 1
fi

# The pack's files for a client, fetched as a client with nothing but the API
# does: list them, download each, check it against its hash and keep it by
# that hash; the second time, download nothing.
CLIENT="http://127.0.0.1:${PORT}/api/game-defs/pack-draughts-0.1.0/clients/made-up"
curl -fsS "${CLIENT}" -o "${WORKDIR}/client.json"
grep -o '"path":"[^"]*","size":[0-9]*,"sha256":"[0-9a-f]*"' "${WORKDIR}/client.json" |
  sed 's/"path":"\([^"]*\)".*"sha256":"\([0-9a-f]*\)"/\1 \2/' >"${WORKDIR}/client.txt"
if [ ! -s "${WORKDIR}/client.txt" ]; then
  echo "smoke test FAILED: the pack lists no files for its client"
  cat "${WORKDIR}/client.json"
  exit 1
fi
sha256() { if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -d' ' -f1; }
mkdir -p "${WORKDIR}/kept"
while read -r FILE HASH; do
  curl -fsS "${CLIENT}/files/${FILE}" -o "${WORKDIR}/kept/${HASH}"
  if [ "$(sha256 "${WORKDIR}/kept/${HASH}")" != "${HASH}" ]; then
    echo "smoke test FAILED: the client's ${FILE} does not match its hash"
    exit 1
  fi
done <"${WORKDIR}/client.txt"
while read -r FILE HASH; do
  STATUS=$(curl -sS -o /dev/null -w '%{http_code}' -H "If-None-Match: \"${HASH}\"" "${CLIENT}/files/${FILE}")
  if [ "${STATUS}" != 304 ]; then
    echo "smoke test FAILED: the client's ${FILE} was sent again (${STATUS})"
    exit 1
  fi
done <"${WORKDIR}/client.txt"

# The client's build is listed and sent, the logo has its menu, and a setup
# file says which server it came from.
curl -fsS "http://127.0.0.1:${PORT}/api/client" -o "${WORKDIR}/builds.json"
if ! grep -q '"platform":"linux-x86_64","version":"0.0.1","file":"bingle-3d-linux-x86_64.tar.gz"' "${WORKDIR}/builds.json"; then
  echo "smoke test FAILED: the client's build is not on offer"
  cat "${WORKDIR}/builds.json"
  exit 1
fi
curl -fsS "http://127.0.0.1:${PORT}/api/client/files/bingle-3d-linux-x86_64.tar.gz" -o "${WORKDIR}/build.tar.gz"
if ! cmp -s "${WORKDIR}/build.tar.gz" "${BINGLE_CLIENT_BUILDS}/0.0.1/bingle-3d-linux-x86_64.tar.gz"; then
  echo "smoke test FAILED: the client's build is not the one on the disk"
  exit 1
fi
curl -fsS "http://127.0.0.1:${PORT}/" -o "${WORKDIR}/index.html"
if ! grep -q 'id="brand-menu-setup"' "${WORKDIR}/index.html"; then
  echo "smoke test FAILED: the logo has no menu"
  exit 1
fi
curl -fsS -X POST "http://127.0.0.1:${PORT}/api/client/setup" -o "${WORKDIR}/setup.toml"
if ! grep -qx "server = \"http://127.0.0.1:${PORT}\"" "${WORKDIR}/setup.toml"; then
  echo "smoke test FAILED: the setup file does not name this server"
  cat "${WORKDIR}/setup.toml"
  exit 1
fi

echo "smoke test PASSED: catalog, game creation vs a bot, a move with a bot reply, static assets, puzzles, environments, hints, an install pack with a client's files, and the client's own build and setup file"
