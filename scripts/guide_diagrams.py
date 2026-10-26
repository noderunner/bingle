#!/usr/bin/env python3
"""Draws the diagrams of the rules guides (SVG files next to the guide pages).

    python3 scripts/guide_diagrams.py            # rewrite every diagram
    python3 scripts/guide_diagrams.py tak go     # only the games named

The pictures are checked in; this script is how they are made, so a change to a diagram is
a change here. The shared drawing helpers are in guide_lib.py. (Onitama's cards are drawn by
the game's own Go code from its card table, not here.)
"""
import math
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from guide_lib import *  # noqa: E402,F401,F403

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "internal", "games")
GAMES = {}


def game(name, folder):
    def deco(fn):
        GAMES[name] = (folder, fn)
        return fn
    return deco


def save(folder, name, svg):
    path = os.path.join(ROOT, folder, name + ".svg")
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as fh:
        fh.write(svg.render())


# =================================================================================================
# Tak
# =================================================================================================

def pile(bd, c, r, items, numbered=False, first=1):
    """A stack seen from the side: items are (kind, color) from the bottom to the top."""
    x, y = bd.at(c, r)
    base = y + bd.cell / 2 - 9
    w, hh, gap = bd.cell * 0.66, 11, 12.5
    for i, (kind, color) in enumerate(items):
        fill, edge = PIECE[color]
        txt = "#222" if color == "white" else "#fff"
        yy = base - i * gap
        if kind == "wall":
            bd.svg.rect(x - 6, yy - 24, 12, 30, fill, edge, 2, rx=3)
        elif kind == "cap":
            bd.svg.circle(x, yy - 3, 11, fill, edge, 2)
            bd.svg.circle(x, yy - 3, 4.5, "none", edge, 1.8)
        else:
            bd.svg.rect(x - w / 2, yy - hh / 2, w, hh, fill, edge, 2, rx=3)
            if numbered:
                bd.svg.text(x, yy + 0.5, str(first + i), 9.5, txt, weight="800")


def strip(svg, x, y, n, cell=64):
    return Board(svg, x, y, n, 1, cell, checker=None)


@game("tak", "tak/guide")
def tak():
    # -- the three kinds of stone
    s = Svg(560, 190, "The three kinds of Tak stone")
    for i, (kind, name, note) in enumerate([("flat", "Flat stone", "Lies flat. Counts for roads."), ("wall", "Standing stone (wall)", "Blocks. Counts for no road."), ("cap", "Capstone", "Counts for roads. Flattens walls.")]):
        cx = 100 + i * 180
        s.rect(cx - 82, 12, 164, 166, "#ecdcb2", "#b9a57a", 1, rx=8)
        piece(s, cx - 30, 62, kind, "white", 60)
        piece(s, cx + 30, 62, kind, "black", 60)
        caption(s, cx, 112, name, 13)
        s.text(cx, 134, note.split(". ")[0].rstrip(".") + ".", 11.5, INK, weight="500")
        s.text(cx, 152, note.split(". ")[1], 11.5, INK, weight="500")
    save("tak/guide", "tak-pieces", s)

    # -- moving a stack
    s = Svg(560, 470, "Moving a stack")
    W, B = ("flat", "white"), ("flat", "black")

    def panel(y0, title, notation, before, after):
        caption(s, 280, y0, title, 14)
        s.text(280, y0 + 19, notation, 12.5, "#6a5a40", weight="600")
        s.text(38, y0 + 66, "Before", 12, INK, "middle", "700")
        s.text(38, y0 + 160, "After", 12, INK, "middle", "700")
        b1 = strip(s, 78, y0 + 34, 4)
        b2 = strip(s, 78, y0 + 128, 4)
        for c, items, num in before:
            pile(b1, c, 0, items, True, num)
        for c, items, num in after:
            pile(b2, c, 0, items, True, num)

    panel(18, "Pick up all three and drop one on each square", "3a1>111", [(0, [B, W, W], 1)], [(1, [B], 1), (2, [W], 2), (3, [W], 3)])
    panel(244, "Pick up the top three of four: drop two, then one", "3a1>21", [(0, [("flat", "white"), B, W, B], 0)],
          [(0, [("flat", "white")], 0), (1, [B, W], 1), (2, [B], 3)])
    s.text(410, 105, "Numbers show the order", 11, "#6a5a40", weight="500", anchor="start")
    s.text(410, 121, "the stones lay in,", 11, "#6a5a40", weight="500", anchor="start")
    s.text(410, 137, "bottom first.", 11, "#6a5a40", weight="500", anchor="start")
    save("tak/guide", "tak-stack-move", s)

    # -- a road
    s = Svg(330, 330, "A winning road")
    bd = Board(s, 40, 18, 5, 5, 50, checker=None, labels=True)
    road = [(0, 1), (1, 1), (2, 1), (2, 2), (3, 2), (4, 2)]
    bd.tint(road, AMBER, 0.55)
    for c, r in road:
        if (c, r) != (3, 2):
            bd.put(c, r, "flat", "white")
    bd.put(3, 2, "cap", "white")
    for c, r in [(0, 3), (1, 3), (3, 0), (4, 4), (2, 4), (1, 0)]:
        bd.put(c, r, "flat", "black")
    bd.put(3, 3, "wall", "black")
    caption(s, 165, 312, "White's road joins the left and right edges", 12)
    save("tak/guide", "tak-road", s)

    # -- not a road
    s = Svg(660, 232, "Chains that are not roads")
    def mini(x0, title, white, extra=None, marks=(), stacks=()):
        bd = Board(s, x0, 36, 5, 5, 28, checker=None)
        for c, r in white:
            bd.put(c, r, "flat", "white")
        for c, r, kind, col in (extra or []):
            bd.put(c, r, kind, col)
        for c, r, items in stacks:
            pile(bd, c, r, items)
        for c, r in marks:
            bd.cross_mark(c, r)
        s.text(x0 + 70, 16, title, 12, INK, weight="700")
        return bd
    mini(18, "Corners don't join", [(0, 2), (1, 2), (2, 3), (3, 3), (4, 3)], marks=[(2, 2)])
    mini(184, "A gap", [(0, 2), (1, 2), (3, 2), (4, 2)], marks=[(2, 2)])
    mini(350, "A wall is not a road", [(0, 2), (1, 2), (3, 2), (4, 2)], extra=[(2, 2, "wall", "white")])
    mini(516, "Only the top counts", [(0, 2), (1, 2), (3, 2), (4, 2)], stacks=[(2, 2, [("flat", "white"), ("flat", "black")])])
    s.text(330, 206, "Roads are made of flat stones and capstones that are on top of their squares,", 12, INK, weight="500")
    s.text(330, 222, "joined edge to edge.", 12, INK, weight="500")
    save("tak/guide", "tak-not-a-road", s)

    # -- walls and the capstone
    s = Svg(560, 380, "Walls and the capstone")
    caption(s, 280, 22, "A flat stone cannot be moved onto a wall", 14)
    b = strip(s, 144, 40, 4)
    pile(b, 0, 0, [("flat", "white")])
    pile(b, 1, 0, [("wall", "black")])
    b.arrow((0, 0), (1, 0))
    b.cross_mark(1, 0, RED)
    caption(s, 280, 138, "but the capstone can flatten it, moving alone as the last step of its move", 12.5)
    s.text(38, 196, "Before", 12, INK, weight="700")
    s.text(38, 290, "After", 12, INK, weight="700")
    b = strip(s, 78, 164, 4)
    pile(b, 0, 0, [("cap", "white")])
    pile(b, 1, 0, [("wall", "black")])
    pile(b, 2, 0, [("flat", "black")])
    b.arrow((0, 0), (1, 0), GREEN)
    b = strip(s, 78, 258, 4)
    pile(b, 1, 0, [("flat", "black"), ("cap", "white")])
    pile(b, 2, 0, [("flat", "black")])
    s.text(350, 188, "The wall lies down and", 11.5, "#6a5a40", weight="500", anchor="start")
    s.text(350, 204, "becomes a flat stone,", 11.5, "#6a5a40", weight="500", anchor="start")
    s.text(350, 220, "with the capstone on top.", 11.5, "#6a5a40", weight="500", anchor="start")
    s.text(280, 346, "Nothing can be put or moved on top of a capstone.", 12, INK, weight="500")
    s.text(280, 364, "A stack topped by a wall or a capstone can still be picked up and moved.", 12, INK, weight="500")
    save("tak/guide", "tak-wall-and-cap", s)


# =================================================================================================
# The classics
# =================================================================================================

CL = "classics/guide/"
HIGH = "#e8a317"


def panels(width, height, title, items, save_as, gap=18, lead=34, note=None):
    """A row of panels: items are (caption, draw(svg, x, y)->(w, h)); returns nothing, saves the SVG."""
    s = Svg(width, height, title)
    x = gap
    for cap, draw in items:
        w, h = draw(s, x, lead)
        caption(s, x + w / 2, 16, cap, 12.5)
        x += w + gap
    if note:
        s.text(width / 2, height - 14, note, 12, INK, weight="500")
    save(*save_as, s)


@game("tictactoe", "classics/guide/tictactoe")
def tictactoe():
    s = Svg(300, 250, "Three in a row wins")
    bd = Board(s, 54, 20, 3, 3, 64, checker=None)
    marks = {(0, 2): "x", (1, 2): "o", (2, 2): "x", (1, 1): "x", (0, 1): "o", (2, 0): "x", (0, 0): "o"}
    for (c, r), m in marks.items():
        bd.put(c, r, m, "red" if m == "x" else "blue", scale=1.0)
    bd.tint([(0, 0), (1, 1), (2, 2)] if False else [(2, 0), (1, 1), (0, 2)], AMBER, 0.55)
    # redraw the marks over the tint
    for (c, r), m in marks.items():
        bd.put(c, r, m, "red" if m == "x" else "blue", scale=1.0)
    caption(s, 150, 232, "Red completes the diagonal from c1 to a3", 12.5)
    save("classics/guide/tictactoe", "ttt-win", s)


@game("connect4", "classics/guide/connect4")
def connect4():
    s = Svg(430, 350, "Connect Four")
    bd = Board(s, 38, 16, 7, 6, 46, checker=None, labels=True, edge="#2d5fc4", light="#3f74d6")
    cols = {
        0: ["red", "yellow"],
        1: ["yellow", "red", "yellow"],
        2: ["yellow", "yellow", "red"],
        3: ["red", "yellow", "red", "red"],
        4: ["yellow", "red"],
        5: ["red"],
        6: [],
    }
    for c, stack in cols.items():
        for r, col in enumerate(stack):
            bd.put(c, r, "disc", col, scale=0.88)
    for c in range(7):
        for r in range(len(cols[c]), 6):
            x, y = bd.at(c, r)
            s.circle(x, y, 17, "#1f4aa6", op=0.55)
    for c, r in [(0, 0), (1, 1), (2, 2), (3, 3)]:
        bd.ring_mark(c, r, HIGH, 4)
    note(s, 215, 334, "Discs fall to the lowest free square. Red has four in a diagonal.", 52, 12.5)
    save("classics/guide/connect4", "connect4-win", s)


@game("gomoku", "classics/guide/gomoku")
def gomoku():
    s = Svg(340, 350, "Five in a row")
    bd = Board(s, 38, 16, 9, 9, 32, points=True, labels=True)
    for c in range(2, 7):
        bd.put(c, 4, "disc", "black", scale=0.9)
    for c, r in [(3, 5), (4, 3), (5, 5), (2, 3), (6, 3), (1, 4), (7, 2)]:
        bd.put(c, r, "disc", "white", scale=0.9)
    bd.put(3, 6, "disc", "black", scale=0.9)
    bd.put(5, 2, "disc", "black", scale=0.9)
    s.line(*bd.at(2, 4), *bd.at(6, 4), HIGH, 5, op=0.7)
    note(s, 170, 340, "Black has five in an unbroken row: Black wins", 44, 12.5)
    save("classics/guide/gomoku", "gomoku-five", s)


def hex_edges(hb, n):
    """Colour the sides of an n x n rhombus: red top and bottom, blue left and right."""
    cs = hb.corners
    for q in range(n):
        for (a, b) in [(4, 5), (5, 0)]:
            p, r_ = cs(q, 0)[a], cs(q, 0)[b]
            hb.svg.line(p[0], p[1], r_[0], r_[1], "#d6453a", 6, cap="butt")
        for (a, b) in [(1, 2), (2, 3)]:
            p, r_ = cs(q, n - 1)[a], cs(q, n - 1)[b]
            hb.svg.line(p[0], p[1], r_[0], r_[1], "#d6453a", 6, cap="butt")
    for r in range(n):
        for (a, b) in [(3, 4), (2, 3)]:
            p, r_ = cs(0, r)[a], cs(0, r)[b]
            hb.svg.line(p[0], p[1], r_[0], r_[1], "#3f6fd0", 6, cap="butt")
        for (a, b) in [(5, 0), (0, 1)]:
            p, r_ = cs(n - 1, r)[a], cs(n - 1, r)[b]
            hb.svg.line(p[0], p[1], r_[0], r_[1], "#3f6fd0", 6, cap="butt")


@game("hex", "classics/guide/hex")
def hexgame():
    n = 5
    s = Svg(380, 270, "A winning chain in Hex")
    cells = [(q, r) for r in range(n) for q in range(n)]
    hb = HexBoard(s, 70, 40, cells, 22)
    hex_edges(hb, n)
    red = [(2, 0), (2, 1), (1, 2), (1, 3), (1, 4)]
    blue = [(0, 1), (1, 1), (3, 1), (3, 2), (2, 2), (2, 3), (3, 3), (0, 3), (3, 0), (0, 0)]
    for q, r in red:
        hb.put(q, r, "red")
    for q, r in blue:
        hb.put(q, r, "blue")
    hb.tint(red, AMBER, 0.45)
    for q, r in red:
        hb.put(q, r, "red")
    caption(s, 190, 252, "Red's chain joins the top and bottom edges", 12.5)
    save("classics/guide/hex", "hex-win", s)


def checker_board(s, x, y, n, cell, label=True):
    return Board(s, x, y, n, n, cell, checker=0, labels=False)


@game("draughts", "classics/guide/draughts")
def draughts():
    def p_move(s, x, y):
        bd = checker_board(s, x + 8, y + 8, 6, 36)
        bd.put(2, 2, "man", "black")
        bd.put(4, 4, "man", "red")
        bd.put(1, 4, "man", "red")
        bd.dots([(1, 3), (3, 3)])
        s.text(x + 8 + 108, y + 8 + 6 * 36 + 20, "A man steps one square forward,", 11.5, INK, weight="500")
        s.text(x + 8 + 108, y + 8 + 6 * 36 + 36, "diagonally, onto an empty square", 11.5, INK, weight="500")
        return 6 * 36 + 16, 6 * 36 + 16
    def p_jump(s, x, y):
        bd = checker_board(s, x + 8, y + 8, 6, 36)
        bd.put(2, 2, "man", "black")
        bd.put(3, 3, "man", "red")
        bd.put(0, 4, "man", "red")
        bd.arrow((2, 2), (4, 4), GREEN)
        bd.cross_mark(3, 3, RED)
        bd.dots([(4, 4)])
        s.text(x + 8 + 108, y + 8 + 6 * 36 + 20, "A jump takes the piece it leaps.", 11.5, INK, weight="500")
        s.text(x + 8 + 108, y + 8 + 6 * 36 + 36, "Jumping is compulsory.", 11.5, INK, weight="500")
        return 6 * 36 + 16, 6 * 36 + 16
    def p_multi(s, x, y):
        bd = checker_board(s, x + 8, y + 8, 6, 36)
        bd.put(0, 0, "man", "black")
        bd.put(1, 1, "man", "red")
        bd.put(3, 3, "man", "red")
        bd.arrow((0, 0), (2, 2), GREEN)
        bd.arrow((2, 2), (4, 4), GREEN)
        bd.cross_mark(1, 1, RED)
        bd.cross_mark(3, 3, RED)
        s.text(x + 8 + 108, y + 8 + 6 * 36 + 20, "If another jump is possible from", 11.5, INK, weight="500")
        s.text(x + 8 + 108, y + 8 + 6 * 36 + 36, "where you land, you must take it", 11.5, INK, weight="500")
        return 6 * 36 + 16, 6 * 36 + 16
    panels(3 * (232 + 18) + 18, 330, "Draughts: moves and jumps", [("Moving", p_move), ("Jumping", p_jump), ("Jumping on", p_multi)], ("classics/guide/draughts", "draughts-moves"))


@game("breakthrough", "classics/guide/breakthrough")
def breakthrough():
    def p_move(s, x, y):
        bd = Board(s, x + 8, y + 8, 5, 5, 40, checker=None)
        bd.put(2, 1, "disc", "white")
        bd.put(1, 2, "disc", "black")
        bd.put(2, 2, "disc", "black")
        bd.dots([(3, 2)])
        bd.cross_mark(2, 2, RED)
        bd.arrow((2, 1), (1, 2), GREEN)
        note(s, x + 108, y + 232, "Forward or diagonally forward onto an empty square; straight ahead is never a capture", 32)
        return 216, 214
    def p_cap(s, x, y):
        bd = Board(s, x + 8, y + 8, 5, 5, 40, checker=None)
        bd.put(2, 1, "disc", "white")
        bd.put(1, 2, "disc", "black")
        bd.put(3, 2, "disc", "black")
        bd.arrow((2, 1), (1, 2), GREEN)
        bd.arrow((2, 1), (3, 2), GREEN)
        note(s, x + 108, y + 232, "Capture only on the two diagonals ahead; the captured pawn is removed", 32)
        return 216, 214
    def p_win(s, x, y):
        bd = Board(s, x + 8, y + 8, 5, 5, 40, checker=None)
        bd.tint([(c, 4) for c in range(5)], AMBER, 0.45)
        bd.put(2, 3, "disc", "white")
        bd.put(0, 4, "disc", "black")
        bd.put(4, 4, "disc", "black")
        bd.arrow((2, 3), (3, 4), GREEN)
        note(s, x + 108, y + 232, "Reach the opponent's home row with any pawn to win", 32)
        return 216, 214
    panels(3 * 234 + 18, 300, "Breakthrough", [("Moving", p_move), ("Capturing", p_cap), ("Winning", p_win)], ("classics/guide/breakthrough", "breakthrough-moves"))


@game("reversi", "classics/guide/reversi")
def reversi():
    green = dict(light="#3b8f5c", edge="#245a3a")
    def board(s, x, y, discs):
        bd = Board(s, x + 8, y + 8, 5, 5, 44, checker=None, **green)
        for (c, r), col in discs.items():
            bd.put(c, r, "disc", col)
        return bd
    before = {(0, 2): "white", (1, 2): "black", (2, 2): "black", (3, 2): "black", (3, 3): "black", (2, 4): "white"}
    def p_before(s, x, y):
        bd = board(s, x, y, before)
        bd.dots([(4, 2)], "#ffffff")
        bd.arrow((4, 2), (1, 2), AMBER)
        bd.arrow((4, 2), (2, 4), AMBER)
        note(s, x + 118, y + 248, "White places on e3: from there two lines of black discs end at a white disc", 38)
        return 236, 236
    def p_after(s, x, y):
        after = {k: "white" for k in [(0, 2), (1, 2), (2, 2), (3, 2), (3, 3), (2, 4), (4, 2)]}
        bd = board(s, x, y, after)
        bd.ring_mark(4, 2, AMBER, 3)
        note(s, x + 118, y + 248, "All the bracketed discs turn white", 38)
        return 236, 236
    panels(2 * 254 + 18, 310, "Reversi: flipping discs", [("Before", p_before), ("After", p_after)], ("classics/guide/reversi", "reversi-flip"))


@game("go", "classics/guide/go")
def go():
    cell = 30
    def p_lib(s, x, y):
        bd = Board(s, x + 8, y + 8, 6, 6, cell, points=True)
        bd.put(3, 3, "disc", "black", scale=0.92)
        bd.dots([(2, 3), (4, 3), (3, 2), (3, 4)], AMBER)
        bd.put(0, 4, "disc", "black", scale=0.92)
        bd.dots([(0, 5), (1, 4), (0, 3)], AMBER)
        bd.put(5, 0, "disc", "black", scale=0.92)
        bd.dots([(4, 0), (5, 1)], AMBER)
        note(s, x + 98, y + 218, "Liberties are the empty points next to a stone: 4 in the open, 3 on an edge, 2 in a corner", 32)
        return 196, 196
    def p_before(s, x, y):
        bd = Board(s, x + 8, y + 8, 6, 6, cell, points=True)
        for c, r in [(1, 2), (3, 2), (2, 1)]:
            bd.put(c, r, "disc", "black", scale=0.92)
        bd.put(2, 2, "disc", "white", scale=0.92)
        bd.ring_mark(2, 3, RED, 3)
        note(s, x + 98, y + 218, "White has one liberty left, at c4. Black plays there...", 32)
        return 196, 196
    def p_after(s, x, y):
        bd = Board(s, x + 8, y + 8, 6, 6, cell, points=True)
        for c, r in [(1, 2), (3, 2), (2, 1), (2, 3)]:
            bd.put(c, r, "disc", "black", scale=0.92)
        bd.dots([(2, 2)], RED)
        note(s, x + 98, y + 218, "...and the white stone has no liberties left: it is captured and removed", 32)
        return 196, 196
    panels(3 * 214 + 18, 300, "Go: liberties and capture", [("Liberties", p_lib), ("Atari", p_before), ("Capture", p_after)], ("classics/guide/go", "go-capture"))
    # territory
    s = Svg(340, 350, "Go: area scoring")
    bd = Board(s, 40, 20, 7, 7, 38, points=True, labels=True)
    blk = [(3, 0), (3, 1), (3, 2), (2, 3), (1, 3), (0, 3)]
    wht = [(4, 0), (4, 1), (4, 2), (4, 3), (4, 4), (5, 4), (6, 4), (3, 4), (3, 5), (3, 6)]
    for c, r in blk:
        bd.put(c, r, "disc", "black", scale=0.9)
    for c, r in wht:
        bd.put(c, r, "disc", "white", scale=0.9)
    for c in range(0, 3):
        for r in range(0, 3):
            bd.dots([(c, r)], "#1d1d1d")
    for c, r in [(5, 0), (5, 1), (5, 2), (5, 3), (6, 0), (6, 1), (6, 2), (6, 3)]:
        bd.dots([(c, r)], "#f4f4f4")
    note(s, 170, 322, "Black surrounds 9 empty points and White 8; the rest are shared", 46, 12)
    save("classics/guide/go", "go-territory", s)


def hexpanel(s, x, y, n, size, pieces=None, tints=None, tint_color=AMBER, extra=None, corners=False, marks=()):
    """A hexagon-shaped board with n cells to a side at (x, y); pieces maps (q, r) to a colour."""
    w = math.sqrt(3) * size * (2 * n - 1) + 2 * size * 0.9
    h = 1.5 * size * (2 * n - 2) + 2 * size + 8
    hb = HexBoard(s, x + w / 2, y + h / 2 - 2, hexhex(n), size)
    N = n - 1
    if corners:
        hb.tint([(N, 0), (N, -N), (0, -N), (-N, 0), (-N, N), (0, N)], "#d8c9a0", 1.0)
    if tints:
        hb.tint(tints, tint_color, 0.6)
    for (q, r), col in (pieces or {}).items():
        hb.put(q, r, col)
    for q, r in marks:
        hb.mark(q, r)
    return hb, w, h


@game("yavalath", "classics/guide/yavalath")
def yavalath():
    n, sz = 4, 15
    def p_four(s, x, y):
        pcs = {(-2, 0): "white", (-1, 0): "white", (0, 0): "white", (1, 0): "white", (0, -1): "black", (1, -1): "black", (-1, 1): "black", (0, 1): "black", (2, -2): "black"}
        hb, w, h = hexpanel(s, x, y, n, sz, pcs, [(-2, 0), (-1, 0), (0, 0), (1, 0)])
        note(s, x + w / 2, y + h + 8, "Four in a row wins", 30, 12)
        return w, h + 20
    def p_three(s, x, y):
        pcs = {(-1, 1): "white", (0, 0): "white", (1, -1): "white", (0, -1): "black", (-1, 0): "black", (1, 0): "black", (0, 1): "black", (2, -2): "black"}
        hb, w, h = hexpanel(s, x, y, n, sz, pcs, [(-1, 1), (0, 0), (1, -1)], RED)
        note(s, x + w / 2, y + h + 8, "Three in a row loses", 30, 12)
        return w, h + 20
    def p_both(s, x, y):
        pcs = {(-2, 0): "white", (-1, 0): "white", (1, 0): "white", (1, -1): "white", (-1, 1): "white", (0, -1): "black", (0, 1): "black", (2, -2): "black", (-2, 2): "black"}
        hb, w, h = hexpanel(s, x, y, n, sz, pcs, [(0, 0)], GREEN, marks=[(0, 0)])
        note(s, x + w / 2, y + h + 8, "A move that makes four and three at once wins", 34, 12)
        return w, h + 20
    panels(3 * 229 + 20, 250, "Yavalath: four wins, three loses", [("Win", p_four), ("Lose", p_three), ("Four beats three", p_both)], ("classics/guide/yavalath", "yavalath-lines"), gap=20)


@game("havannah", "classics/guide/havannah")
def havannah():
    n, sz = 4, 14
    def p_ring(s, x, y):
        ring = [(2, -1), (2, -2), (1, -2), (0, -1), (0, 0), (1, 0)]
        # a ring of six cells round (1, -1)
        ring = [(2, -1), (1, -2), (0, -1), (0, 0), (1, 0), (2, -2)]
        ring = [(1 + dq, -1 + dr) for dq, dr in [(1, 0), (1, -1), (0, -1), (-1, 0), (-1, 1), (0, 1)]]
        pcs = {c: "white" for c in ring}
        pcs.update({(-1, 1): "black", (-2, 1): "black", (0, 2): "black", (-1, -1): "black"})
        hb, w, h = hexpanel(s, x, y, n, sz, pcs, ring, corners=True)
        note(s, x + w / 2, y + h + 8, "Ring: a closed loop round at least one cell", 28, 12)
        return w, h + 36
    def p_bridge(s, x, y):
        line = [(q, 0) for q in range(-3, 4)]
        pcs = {c: "white" for c in line}
        pcs.update({(0, -1): "black", (1, -1): "black", (-1, 1): "black", (2, -3): "black", (0, 2): "black"})
        hb, w, h = hexpanel(s, x, y, n, sz, pcs, line, corners=True)
        note(s, x + w / 2, y + h + 8, "Bridge: a chain that touches two of the six corners (shaded)", 28, 12)
        return w, h + 36
    def p_fork(s, x, y):
        arms = [(0, 0), (1, 0), (2, -1), (3, -1), (0, -1), (0, -2), (1, -3), (-1, 0), (-2, 1), (-3, 1)]
        pcs = {c: "white" for c in arms}
        pcs.update({(1, 1): "black", (-1, 1): "black", (2, 0): "black", (-1, -1): "black"})
        hb, w, h = hexpanel(s, x, y, n, sz, pcs, arms, corners=True)
        note(s, x + w / 2, y + h + 8, "Fork: a chain that touches three different sides (corners do not count)", 28, 12)
        return w, h + 36
    panels(3 * 215 + 20, 290, "Havannah: ring, bridge and fork", [("Ring", p_ring), ("Bridge", p_bridge), ("Fork", p_fork)], ("classics/guide/havannah", "havannah-wins"), gap=20)


def rays(cols, rows, start, blocked):
    """The squares a queen on `start` reaches before something in `blocked` stops it."""
    out = []
    for dc in (-1, 0, 1):
        for dr in (-1, 0, 1):
            if dc == dr == 0:
                continue
            c, r = start[0] + dc, start[1] + dr
            while 0 <= c < cols and 0 <= r < rows and (c, r) not in blocked:
                out.append((c, r))
                c, r = c + dc, r + dr
    return out


@game("amazons", "classics/guide/amazons")
def amazons():
    cell = 38
    black_q = (4, 4)
    arrows = [(2, 4), (0, 1)]
    def p_move(s, x, y):
        bd = Board(s, x + 8, y + 8, 6, 6, cell, checker=None)
        blocked = set(arrows) | {black_q}
        bd.tint(rays(6, 6, (2, 2), blocked), GREEN, 0.5)
        bd.put(2, 2, "queen", "white")
        bd.put(*black_q, "queen", "black")
        for a in arrows:
            bd.put(*a, "arrow", "black")
        note(s, x + 120, y + 252, "1. Slide a queen like a chess queen: any distance, no jumping", 34)
        return 244, 244
    def p_shoot(s, x, y):
        bd = Board(s, x + 8, y + 8, 6, 6, cell, checker=None)
        blocked = set(arrows) | {black_q} | {(3, 1)}
        bd.tint(rays(6, 6, (3, 1), set(arrows) | {black_q}), AMBER, 0.5)
        bd.ring_mark(2, 2, "#8a7a5a", 2)
        bd.put(3, 1, "queen", "white")
        bd.put(*black_q, "queen", "black")
        for a in arrows:
            bd.put(*a, "arrow", "black")
        bd.put(3, 4, "arrow", "red")
        bd.arrow((3, 1), (3, 4), RED)
        note(s, x + 120, y + 252, "2. From where it landed, shoot an arrow along any line; that square is blocked for the rest of the game", 34)
        return 244, 244
    panels(2 * 262 + 18, 330, "Amazons: slide, then shoot", [("Move", p_move), ("Shoot", p_shoot)], ("classics/guide/amazons", "amazons-turn"))


@game("ataxx", "classics/guide/ataxx")
def ataxx():
    cell = 44
    def near(c0, r0, d, cols=5, rows=5):
        return [(c, r) for c in range(cols) for r in range(rows) if max(abs(c - c0), abs(r - r0)) == d]
    base = {(1, 2): "red", (3, 2): "blue", (3, 3): "blue", (0, 0): "red"}
    def p_before(s, x, y):
        bd = Board(s, x + 8, y + 8, 5, 5, cell, checker=None)
        occupied = set(base)
        bd.dots([c for c in near(1, 2, 1) if c not in occupied], GREEN)
        bd.dots([c for c in near(1, 2, 2) if c not in occupied], AMBER)
        for c, col in base.items():
            bd.put(*c, "disc", col)
        bd.ring_mark(1, 2, "#333", 2.5)
        note(s, x + 118, y + 240, "Green: clone (the piece stays). Amber: jump (the piece moves)", 34)
        return 236, 236
    def p_after(s, x, y):
        bd = Board(s, x + 8, y + 8, 5, 5, cell, checker=None)
        after = {(1, 2): "red", (2, 2): "red", (3, 2): "red", (3, 3): "red", (0, 0): "red"}
        for c, col in after.items():
            bd.put(*c, "disc", col)
        bd.ring_mark(2, 2, AMBER, 3)
        note(s, x + 118, y + 240, "Cloning to c3 converts the enemy pieces next to it", 34)
        return 236, 236
    panels(2 * 254 + 18, 310, "Ataxx: clone and jump", [("Choose a move", p_before), ("After the clone", p_after)], ("classics/guide/ataxx", "ataxx-move"))


@game("pente", "classics/guide/pente")
def pente():
    cell = 30
    def b7(s, x, y):
        return Board(s, x + 8, y + 8, 7, 7, cell, points=True)
    def p_before(s, x, y):
        bd = b7(s, x, y)
        bd.put(1, 3, "disc", "black", scale=0.9)
        bd.put(2, 3, "disc", "white", scale=0.9)
        bd.put(3, 3, "disc", "white", scale=0.9)
        bd.ring_mark(4, 3, AMBER, 3)
        note(s, x + 110, y + 232, "Black plays at the end of a pair of white stones", 34)
        return 220, 220
    def p_after(s, x, y):
        bd = b7(s, x, y)
        bd.put(1, 3, "disc", "black", scale=0.9)
        bd.put(4, 3, "disc", "black", scale=0.9)
        bd.dots([(2, 3), (3, 3)], RED)
        note(s, x + 110, y + 232, "The flanked pair is captured. Black has captured one pair", 34)
        return 220, 220
    def p_five(s, x, y):
        bd = b7(s, x, y)
        for i in range(5):
            bd.put(1 + i, 1 + i, "disc", "black", scale=0.9)
        for c, r in [(2, 1), (4, 3), (5, 2), (3, 5), (1, 3)]:
            bd.put(c, r, "disc", "white", scale=0.9)
        s.line(*bd.at(1, 1), *bd.at(5, 5), HIGH, 5, op=0.7)
        note(s, x + 110, y + 232, "Five in a row in any direction wins", 34)
        return 220, 220
    panels(3 * 238 + 18, 300, "Pente: capture and five in a row", [("Before", p_before), ("Capture", p_after), ("Five", p_five)], ("classics/guide/pente", "pente-capture"))


@game("brandubh", "classics/guide/brandubh")
def brandubh():
    cell = 34
    def board(s, x, y):
        bd = Board(s, x + 8, y + 8, 7, 7, cell, checker=None)
        bd.tint([(0, 0), (6, 0), (0, 6), (6, 6)], "#e0584a", 0.55)
        bd.tint([(3, 3)], AMBER, 0.65)
        return bd
    def p_setup(s, x, y):
        bd = board(s, x, y)
        for c, r in [(3, 0), (3, 1), (3, 5), (3, 6), (0, 3), (1, 3), (5, 3), (6, 3)]:
            bd.put(c, r, "disc", "black")
        for c, r in [(3, 2), (3, 4), (2, 3), (4, 3)]:
            bd.put(c, r, "flat", "white", scale=0.72)
        bd.put(3, 3, "king", "white")
        note(s, x + 124, y + 262, "Red: the corners. Gold: the throne. The king starts on the throne", 38)
        return 250, 250
    def p_sand(s, x, y):
        bd = board(s, x, y)
        bd.put(0, 5, "disc", "black")
        bd.put(4, 5, "disc", "black")
        bd.put(3, 5, "flat", "white", scale=0.72)
        bd.arrow((0, 5), (2, 5), GREEN)
        bd.cross_mark(3, 5, RED)
        note(s, x + 124, y + 262, "An attacker slides next to a defender with another attacker on its far side: captured", 38)
        return 250, 250
    def p_corner(s, x, y):
        bd = board(s, x, y)
        bd.put(0, 1, "flat", "white", scale=0.72)
        bd.put(4, 2, "disc", "black")
        bd.arrow((4, 2), (0, 2), GREEN)
        bd.cross_mark(0, 1, RED)
        note(s, x + 124, y + 262, "An empty corner or throne counts as an enemy: this defender is captured against the corner", 38)
        return 250, 250
    panels(3 * 268 + 18, 350, "Brandubh: setup and captures", [("Setup", p_setup), ("Capture", p_sand), ("Against a corner", p_corner)], ("classics/guide/brandubh", "brandubh-board"))


# =================================================================================================
# Blooms, Fendo, Onitama, Yinsh
# =================================================================================================

HEX_DIRS = [(1, 0), (1, -1), (0, -1), (-1, 0), (-1, 1), (0, 1)]


@game("blooms", "blooms/guide")
def blooms():
    n, sz = 3, 22
    group = [(0, 0), (1, 0)]
    around = []
    for q, r in group:
        for dq, dr in HEX_DIRS:
            c = (q + dq, r + dr)
            if c not in group and c not in around:
                around.append(c)
    ring = {c: ("blue" if i % 2 else "black") for i, c in enumerate(around)}
    def p_free(s, x, y):
        pcs = {c: "red" for c in group}
        pcs.update({c: v for c, v in ring.items() if c != (1, 1)})
        hb, w, h = hexpanel(s, x, y, n, sz, pcs)
        hb.mark(1, 1, GREEN)
        note(s, x + w / 2, y + h + 6, "This red bloom still touches an empty cell, so it is safe", 30, 12)
        return w, h + 40
    def p_fenced(s, x, y):
        pcs = {c: "red" for c in group}
        pcs.update(ring)
        hb, w, h = hexpanel(s, x, y, n, sz, pcs)
        for c in group:
            hb.mark(*c, RED)
        note(s, x + w / 2, y + h + 6, "Now no empty cell touches it: the bloom is fenced and is captured at the end of the turn", 30, 12)
        return w, h + 56
    panels(2 * 236 + 20, 300, "Blooms: a free and a fenced bloom", [("Not fenced", p_free), ("Fenced", p_fenced)], ("blooms/guide", "blooms-fence"), gap=20)


@game("fendo", "fendo/guide")
def fendo():
    cell = 34
    def p_move(s, x, y):
        bd = Board(s, x + 8, y + 8, 7, 7, cell, checker=None)
        bd.put(1, 1, "disc", "white")
        bd.put(5, 4, "disc", "orange")
        bd.put(3, 1, "disc", "orange")
        a, b, c = bd.at(1, 1), bd.at(1, 4), bd.at(4, 4)
        s.line(a[0], a[1], b[0], b[1], GREEN, 4)
        s.arrow(b[0], b[1], c[0] - 6, c[1], GREEN, 4)
        gx, gy = bd.at(4, 4)
        s.circle(gx, gy, cell * 0.38, "none", "#3a3631", 2, extra=' stroke-dasharray="4 3"')
        bd.fence(4, 4, "e")
        note(s, x + 124, y + 262, "A piece moves like a rook, with at most one right-angle turn, and builds a fence on a free side of the square it ends on (here its east side)", 40)
        return 250, 250
    def p_closed(s, x, y):
        bd = Board(s, x + 8, y + 8, 7, 7, cell, checker=None)
        area = [(0, 0), (1, 0), (0, 1), (1, 1)]
        bd.tint(area, AMBER, 0.5)
        for c in (0, 1):
            bd.fence(c, 1, "n")
        for r in (0, 1):
            bd.fence(1, r, "e")
        bd.put(1, 0, "disc", "white")
        bd.put(4, 3, "disc", "orange")
        bd.put(5, 5, "disc", "white")
        note(s, x + 124, y + 262, "A fence that closes off an area with exactly one piece in it gives the area to that piece's owner: 4 squares for White", 40)
        return 250, 250
    panels(2 * 268 + 18, 350, "Fendo: moving and fencing", [("A move", p_move), ("A closed area", p_closed)], ("fendo/guide", "fendo-turn"))


CARDS = {
    "crane": [(0, 1), (-1, -1), (1, -1)],
}


@game("onitama", "onitama/guide")
def onitama():
    cell = 44
    s = Svg(560, 316, "Onitama: the setup and a move")
    caption(s, 140, 18, "The setup", 13)
    bd = Board(s, 22, 34, 5, 5, cell, checker=None, labels=True)
    bd.tint([(2, 0), (2, 4)], AMBER, 0.6)
    for c in (0, 1, 3, 4):
        bd.put(c, 0, "student", "red")
        bd.put(c, 4, "student", "blue")
    bd.put(2, 0, "master", "red")
    bd.put(2, 4, "master", "blue")
    note(s, 140, 296, "Gold: the Temple Arches", 30, 11.5)
    caption(s, 420, 18, "Using a card (the Crane)", 13)
    bd2 = Board(s, 300, 34, 5, 5, 28, checker=None)
    bd2.put(2, 2, "student", "red")
    for dc, dr in CARDS["crane"]:
        bd2.tint([(2 + dc, 2 + dr)], GREEN, 0.7)
    bd2.arrow((2, 2), (2, 3), GREEN, 2.5)
    note(s, 372, 205, "The coloured squares on the card are where the piece may go", 26, 11.5)
    save("onitama/guide", "onitama-setup", s)


def points_row(s, x, y, n, step=44):
    s.line(x, y, x + step * (n - 1), y, "#5a4326", 3)
    for i in range(n):
        s.circle(x + i * step, y, 4, "#5a4326")
    return [x + i * step for i in range(n)]


@game("yinsh", "yinsh/guide")
def yinsh():
    s = Svg(470, 330, "Yinsh: moving a ring")
    s.rect(14, 14, 442, 302, "#ecdcb2", "#b9a57a", 1, rx=8)
    n, step, x0 = 9, 44, 54
    caption(s, 235, 34, "Moving a ring along a line", 14)
    def row(y, label, items, ringat=None, ringcol="white", mk=None):
        s.text(30, y + 28, label, 11.5, INK, "start", "700")
        xs = points_row(s, x0, y, n, step)
        for i, x in enumerate(xs):
            if i in items:
                piece(s, x, y, "marker", items[i], 30)
        if ringat is not None:
            piece(s, xs[ringat], y, "ring", ringcol, 38)
        return xs
    xs = row(100, "Before", {2: "black", 3: "white", 4: "black"}, 0)
    s.arrow(xs[0], 74, xs[5], 74, GREEN, 3)
    s.text((xs[0] + xs[5]) / 2, 62, "the ring jumps the whole group", 11, "#6a5a40", weight="500")
    xs = row(190, "After", {0: "white", 2: "white", 3: "black", 4: "white"}, 5)
    note(s, 235, 232, "A marker of your colour is dropped in the ring first. The ring then moves in a straight line and lands on the first empty point after the markers it jumped; every marker it jumped changes colour.", 62, 12)
    save("yinsh/guide", "yinsh-move", s)

    s = Svg(470, 240, "Yinsh: a row of five")
    s.rect(14, 14, 442, 212, "#ecdcb2", "#b9a57a", 1, rx=8)
    caption(s, 235, 34, "Making a row of five", 14)
    xs = points_row(s, 54, 92, 9, 44)
    for i in range(1, 6):
        piece(s, xs[i], 92, "marker", "white", 30)
    piece(s, xs[0], 92, "marker", "black", 30)
    piece(s, xs[7], 92, "ring", "white", 38)
    s.rect(xs[1] - 20, 72, xs[5] - xs[1] + 40, 40, "none", HIGH, 3, rx=10)
    note(s, 235, 146, "Five markers of one colour in a row are removed, and you also take one of your rings off the board: that ring is your score.", 62, 12)
    save("yinsh/guide", "yinsh-row", s)


if __name__ == "__main__":
    wanted = sys.argv[1:] or list(GAMES)
    for name in wanted:
        if name not in GAMES:
            sys.exit("unknown game %r (have: %s)" % (name, ", ".join(GAMES)))
        GAMES[name][1]()
        print("drew", name)
