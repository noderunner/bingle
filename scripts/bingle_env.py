#!/usr/bin/env python3
"""A Gym-style client for Bingle's training environments, standard library only.

    python3 scripts/bingle_env.py http://localhost:8080 tak-5x5 --episodes 5

``BingleEnv`` follows the Gymnasium shape (``reset`` / ``step`` returning
``obs, reward, terminated, truncated, info``) without importing it, so it works
anywhere; wrap it or subclass ``gymnasium.Env`` if you want the registry:

    env = BingleEnv("http://localhost:8080", "tak-5x5", opponent="tactical")
    obs, info = env.reset(seed=0)
    while True:
        action = random.choice(info["legal_action_ids"])
        obs, reward, terminated, truncated, info = env.step(action)
        if terminated or truncated:
            break

Observations are the server's planes: ``obs["shape"] = [C, H, W]`` and
``obs["data"]`` a flat row-major list (feed it to numpy with ``np.array(...)
.reshape(obs["shape"])``). Actions are the game's fixed integer action IDs;
games with chained jumps (draughts) have no fixed space and are stepped with
``step(move="c3xe5")`` using ``info["legal_moves"]``.
"""

import argparse
import json
import random
import urllib.error
import urllib.request


class BingleError(RuntimeError):
    pass


class BingleEnv:
    def __init__(self, base_url, game_def_id, opponent="random", seat=1, max_plies=0,
                 bot_iterations=0, plane_depth=0, timeout=60):
        self.base = base_url.rstrip("/")
        self.game = game_def_id
        self.opponent = opponent
        self.seat = seat
        self.max_plies = max_plies
        self.bot_iterations = bot_iterations
        self.plane_depth = plane_depth
        self.timeout = timeout
        self.env_id = None
        self.spec = self._call("GET", f"/api/env-specs/{game_def_id}")
        self.num_actions = self.spec["actions"].get("count", 0)
        planes = self.spec.get("planes")
        self.observation_shape = planes["shape"] if planes else None

    # -- gym-style API -------------------------------------------------

    def reset(self, seed=None, puzzle_id=None, start_state=None, start_seat=None):
        if self.env_id:
            self.close()
        body = {
            "opponent": self.opponent, "seat": self.seat, "seed": seed or 0,
            "max_plies": self.max_plies, "bot_iterations": self.bot_iterations,
            "include": ["planes", "action_ids"], "plane_depth": self.plane_depth,
        }
        if puzzle_id:
            body["puzzle_id"] = puzzle_id
        else:
            body["game_def_id"] = self.game
            if start_state is not None:
                body["start_state"] = start_state
                body["start_seat"] = start_seat or 0
        obs = self._call("POST", "/api/envs", body)
        self.env_id = obs["env_id"]
        return self._split(obs)

    def step(self, action=None, move=None):
        body = {"move": move} if move is not None else {"action_id": int(action)}
        obs = self._call("POST", f"/api/envs/{self.env_id}/step", body)
        o, info = self._split(obs)
        terminated = bool(obs["done"]) and not obs.get("truncated")
        return o, float(obs.get("reward", 0.0)), terminated, bool(obs.get("truncated")), info

    def fork(self):
        """Clone the current position into a new environment object."""
        obs = self._call("POST", f"/api/envs/{self.env_id}/fork")
        twin = object.__new__(BingleEnv)
        twin.__dict__.update(self.__dict__)
        twin.env_id = obs["env_id"]
        return twin

    def close(self):
        if self.env_id:
            try:
                self._call("DELETE", f"/api/envs/{self.env_id}")
            except BingleError:
                pass
            self.env_id = None

    # -- helpers --------------------------------------------------------

    @staticmethod
    def _split(obs):
        planes = obs.get("planes") or {}
        info = {
            "legal_moves": obs.get("legal_moves", []),
            "legal_action_ids": obs.get("legal_action_ids", []),
            "board_text": obs.get("board_text", ""),
            "to_move": obs.get("to_move"),
            "ply": obs.get("ply"),
            "winner_seat": obs.get("winner_seat"),
            "result": obs.get("result"),
            "last_moves": obs.get("last_moves", []),
        }
        return {"shape": planes.get("shape"), "channels": planes.get("channels"), "data": planes.get("data")}, info

    def _call(self, method, path, body=None):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(self.base + path, data=data, method=method,
                                     headers={"Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                return json.loads(resp.read().decode() or "{}")
        except urllib.error.HTTPError as err:
            try:
                msg = json.loads(err.read().decode()).get("error", err.reason)
            except ValueError:
                msg = err.reason
            raise BingleError(f"{method} {path}: {err.code} {msg}") from None


def main():
    ap = argparse.ArgumentParser(description="Play random episodes against a Bingle bot.")
    ap.add_argument("url")
    ap.add_argument("game")
    ap.add_argument("--opponent", default="tactical")
    ap.add_argument("--episodes", type=int, default=5)
    args = ap.parse_args()

    env = BingleEnv(args.url, args.game, opponent=args.opponent, max_plies=400)
    print(f"{args.game}: planes {env.observation_shape}, "
          f"{env.num_actions or 'dynamic'} actions")
    tally = {1.0: 0, 0.0: 0, -1.0: 0}
    for ep in range(args.episodes):
        obs, info = env.reset(seed=ep)
        steps = 0
        while True:
            if info["legal_action_ids"]:
                obs, reward, terminated, truncated, info = env.step(random.choice(info["legal_action_ids"]))
            else:
                obs, reward, terminated, truncated, info = env.step(move=random.choice(info["legal_moves"]))
            steps += 1
            if terminated or truncated:
                break
        tally[reward] += 1
        print(f"episode {ep}: reward {reward:+.0f} in {steps} steps ({info['result'] or 'truncated'})")
    env.close()
    print(f"wins {tally[1.0]:.0f}  draws {tally[0.0]:.0f}  losses {tally[-1.0]:.0f}")


if __name__ == "__main__":
    main()
