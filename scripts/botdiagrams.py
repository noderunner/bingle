#!/usr/bin/env python3
"""Draws the diagrams of the bots' guide (internal/web/botguide/assets).

    python3 scripts/botdiagrams.py

Every picture is a self-contained SVG with its own opaque colours, so it reads
the same on the light and the dark page — the same rule the rules guides follow
(see guide_lib.py, which this borrows).
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from guide_lib import Svg, INK, PAPER, GREEN, RED, AMBER, BLUE, FRAME, f  # noqa: E402

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "internal", "web", "botguide", "assets")
MUTED = "#6b6155"


def node(s, x, y, label, fill=PAPER, stroke=INK, r=17, size=13, text=INK):
    s.circle(x, y, r, fill=fill, stroke=stroke, sw=2)
    s.text(x, y, label, size=size, fill=text)


def tree():
    """A game tree searched with alpha-beta: two branches never looked at."""
    s = Svg(720, 330, "A game tree with two branches cut off by alpha-beta")
    # Three levels: the root (we choose), the replies (they choose), the leaves.
    root = (360, 42)
    mid = [(170, 140), (360, 140), (550, 140)]
    leaves = [[(100, 250), (240, 250)], [(300, 250), (420, 250)], [(480, 250), (620, 250)]]
    values = [["3", "5"], ["6", "?"], ["2", "?"]]
    s.text(40, 42, "you", size=13, fill=MUTED, anchor="start")
    s.text(40, 140, "them", size=13, fill=MUTED, anchor="start")
    s.text(40, 250, "score", size=13, fill=MUTED, anchor="start")
    for i, m in enumerate(mid):
        s.line(root[0], root[1] + 17, m[0], m[1] - 17, INK, 1.6)
        for j, lf in enumerate(leaves[i]):
            cut = values[i][j] == "?"
            s.line(m[0], m[1] + 17, lf[0], lf[1] - 17, MUTED if cut else INK, 1.6, dash="5 4" if cut else None)
    for i, m in enumerate(mid):
        best = ["3", "6", "2"][i]
        node(s, m[0], m[1], best, fill=AMBER if i == 1 else PAPER)
    for i, row in enumerate(leaves):
        for j, lf in enumerate(row):
            cut = values[i][j] == "?"
            node(s, lf[0], lf[1], values[i][j], fill="#efe9dc" if cut else PAPER,
                 stroke=MUTED if cut else INK, r=15, size=12, text=MUTED if cut else INK)
    node(s, root[0], root[1], "6", fill=GREEN, r=19, size=14)
    s.text(360, 300, "A branch that cannot beat what you already have is never looked at.",
           size=13, fill=MUTED)
    s.text(620, 215, "cut", size=12, fill=RED)
    s.text(455, 215, "cut", size=12, fill=RED)
    return s


def mcts():
    """The four steps of Monte Carlo tree search, round a loop."""
    s = Svg(720, 300, "The four steps of Monte Carlo tree search")
    steps = [
        ("Select", "walk down the tree,\nfavouring what looks good\nand what is untried"),
        ("Expand", "add one new move\nto the tree"),
        ("Play out", "finish the game\nby random moves"),
        ("Back up", "add the result to every\nnode on the way home"),
    ]
    w, gap = 150, 40
    x0 = 30
    for i, (title, body) in enumerate(steps):
        x = x0 + i * (w + gap)
        s.rect(x, 70, w, 120, fill=PAPER, stroke=INK, sw=2, rx=10)
        s.circle(x + 22, 92, 13, fill=BLUE, stroke=INK, sw=1.5)
        s.text(x + 22, 92, str(i + 1), size=12, fill="#ffffff")
        s.text(x + w / 2 + 10, 92, title, size=15, fill=INK)
        for k, line in enumerate(body.split("\n")):
            s.text(x + w / 2, 122 + k * 18, line, size=12, fill=MUTED, weight="400")
        if i < 3:
            s.arrow(x + w + 6, 130, x + w + gap - 6, 130, stroke=INK, sw=2.5, head=8)
    # The loop back to the start.
    s.path("M %s 196 C %s 250, %s 250, %s 196" % (f(x0 + w / 2), f(x0 + w / 2), f(x0 + 3 * (w + gap) + w / 2), f(x0 + 3 * (w + gap) + w / 2)),
           stroke=INK, sw=2)
    s.arrow(x0 + w / 2 + 30, 228, x0 + w / 2, 198, stroke=INK, sw=2.5, head=8)
    s.text(360, 258, "Thousands of times a move; the move tried most often is the one played.",
           size=13, fill=MUTED)
    s.text(360, 36, "One iteration", size=15, fill=INK)
    return s


def strength():
    """What each bot spends its effort on, as a bar of how far it looks ahead."""
    s = Svg(720, 300, "How far each bot looks, and what it knows")
    rows = [
        ("random", 0.06, "#b9b1a4", "no idea at all: the baseline every other bot is measured against"),
        ("tactical", 0.18, AMBER, "one move deep, with the game's own hints: fast, and no plan"),
        ("minimax", 0.62, GREEN, "every line a few moves deep, alpha-beta cutting the hopeless ones"),
        ("mcts", 0.9, BLUE, "thousands of random finishes, spent on the moves that look best"),
    ]
    x0, w = 120, 470
    for i, (name, frac, colour, note) in enumerate(rows):
        y = 60 + i * 58
        s.text(x0 - 12, y + 11, name, size=14, fill=INK, anchor="end")
        s.rect(x0, y, w, 22, fill="#efe9dc", stroke=None, rx=11)
        s.rect(x0, y, max(22, w * frac), 22, fill=colour, stroke=INK, sw=1.2, rx=11)
        s.text(x0 + 4, y + 40, note, size=12, fill=MUTED, anchor="start", weight="400")
    s.text(x0 + w / 2, 40, "how much of the game it looks at before moving", size=13, fill=MUTED)
    return s


def main():
    os.makedirs(OUT, exist_ok=True)
    for name, fn in (("alphabeta.svg", tree), ("mcts.svg", mcts), ("bots.svg", strength)):
        path = os.path.join(OUT, name)
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(fn().render())
        print("wrote", os.path.relpath(path))


if __name__ == "__main__":
    main()
