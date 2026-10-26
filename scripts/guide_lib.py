"""Small drawing helpers for the diagrams of the rules guides (see guide_diagrams.py).

Every diagram is a self-contained SVG with its own opaque colours, so it reads the same in
the light and the dark page. Squares are addressed (col, row) with row 0 at the BOTTOM, the
way the games name them (a1 is the bottom-left square).
"""
import math

FONT = "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif"
INK = "#2b2118"
PAPER = "#f7f0df"
LIGHT = "#ecdcb2"
DARK = "#cdb78a"
FRAME = "#8a6a43"
GREEN = "#7fcf7f"
RED = "#e0584a"
AMBER = "#f2c14e"
BLUE = "#4a7fd6"

PIECE = {
    "white": ("#f8f4ea", "#3a3631"),
    "black": ("#2e2d33", "#0d0d10"),
    "red": ("#d6453a", "#6e1a14"),
    "blue": ("#3f6fd0", "#14306e"),
    "yellow": ("#ecc13a", "#7a5a05"),
    "orange": ("#ee8a2a", "#7a3a05"),
    "green": ("#3ea86a", "#14502c"),
    "purple": ("#8a5ad0", "#3a1a72"),
}


def esc(t):
    return str(t).replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def f(v):
    return ("%.1f" % v).rstrip("0").rstrip(".")


class Svg:
    def __init__(self, w, h, title, bg=PAPER, pad=0):
        self.w, self.h = w, h
        self.title = title
        self.parts = []
        self.bg = bg
        self.bottom = 0  # the lowest thing drawn: the picture grows to hold it

    def add(self, s):
        self.parts.append(s)

    def rect(self, x, y, w, h, fill="none", stroke=None, sw=1, rx=0, op=None, extra=""):
        self.bottom = max(self.bottom, y + h)
        s = '<rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="%s"' % (f(x), f(y), f(w), f(h), f(rx), fill)
        if stroke:
            s += ' stroke="%s" stroke-width="%s"' % (stroke, f(sw))
        if op is not None:
            s += ' fill-opacity="%s"' % op
        self.add(s + extra + "/>")

    def circle(self, cx, cy, r, fill="none", stroke=None, sw=1, op=None, extra=""):
        s = '<circle cx="%s" cy="%s" r="%s" fill="%s"' % (f(cx), f(cy), f(r), fill)
        if stroke:
            s += ' stroke="%s" stroke-width="%s"' % (stroke, f(sw))
        if op is not None:
            s += ' fill-opacity="%s"' % op
        self.add(s + extra + "/>")

    def line(self, x1, y1, x2, y2, stroke=INK, sw=1, cap="round", dash=None, op=None):
        s = '<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s" stroke-linecap="%s"' % (f(x1), f(y1), f(x2), f(y2), stroke, f(sw), cap)
        if dash:
            s += ' stroke-dasharray="%s"' % dash
        if op is not None:
            s += ' stroke-opacity="%s"' % op
        self.add(s + "/>")

    def poly(self, pts, fill="none", stroke=None, sw=1, op=None, join="round"):
        s = '<polygon points="%s" fill="%s"' % (" ".join("%s,%s" % (f(x), f(y)) for x, y in pts), fill)
        if stroke:
            s += ' stroke="%s" stroke-width="%s" stroke-linejoin="%s"' % (stroke, f(sw), join)
        if op is not None:
            s += ' fill-opacity="%s"' % op
        self.add(s + "/>")

    def path(self, d, fill="none", stroke=None, sw=1, cap="round", op=None):
        s = '<path d="%s" fill="%s"' % (d, fill)
        if stroke:
            s += ' stroke="%s" stroke-width="%s" stroke-linecap="%s" stroke-linejoin="round"' % (stroke, f(sw), cap)
        if op is not None:
            s += ' fill-opacity="%s"' % op
        self.add(s + "/>")

    def text(self, x, y, t, size=14, fill=INK, anchor="middle", weight="600", italic=False):
        self.bottom = max(self.bottom, y + size)
        self.add('<text x="%s" y="%s" font-size="%s" fill="%s" text-anchor="%s" font-weight="%s"%s dominant-baseline="central">%s</text>' % (
            f(x), f(y), f(size), fill, anchor, weight, ' font-style="italic"' if italic else "", esc(t)))

    def arrow(self, x1, y1, x2, y2, stroke=RED, sw=3, head=9, dash=None):
        ang = math.atan2(y2 - y1, x2 - x1)
        bx, by = x2 - head * 0.8 * math.cos(ang), y2 - head * 0.8 * math.sin(ang)
        self.line(x1, y1, bx, by, stroke, sw, dash=dash)
        a = math.radians(26)
        pts = [(x2, y2), (x2 - head * math.cos(ang - a), y2 - head * math.sin(ang - a)), (x2 - head * math.cos(ang + a), y2 - head * math.sin(ang + a))]
        self.poly(pts, stroke, stroke, 1)

    def render(self):
        self.h = max(self.h, math.ceil(self.bottom + 10))
        return ('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s" role="img" aria-label="%s" font-family="%s">'
                '<rect width="%s" height="%s" rx="10" fill="%s"/>%s</svg>\n') % (
            f(self.w), f(self.h), f(self.w), f(self.h), esc(self.title), FONT, f(self.w), f(self.h), self.bg, "".join(self.parts))


# ---- pieces ---------------------------------------------------------------------------------

def piece(svg, cx, cy, kind, color, size, label=None):
    """kind: disc, flat, wall, cap, king, queen, arrow, x, o, man, crown, student, master."""
    fill, edge = PIECE.get(color, PIECE["white"])
    r = size / 2
    txt = "#222" if color in ("white", "yellow", "orange") else "#fff"
    if kind in ("disc", "man", "blob", "stone"):
        svg.circle(cx, cy, r * 0.86, fill, edge, 2)
    elif kind == "flat":
        svg.rect(cx - r * 0.82, cy - r * 0.82, r * 1.64, r * 1.64, fill, edge, 2, rx=r * 0.2)
    elif kind == "wall":
        svg.rect(cx - r * 0.34, cy - r * 0.92, r * 0.68, r * 1.84, fill, edge, 2, rx=r * 0.14)
    elif kind == "cap":
        svg.circle(cx, cy, r * 0.86, fill, edge, 2)
        svg.circle(cx, cy, r * 0.42, "none", edge, 2)
    elif kind == "king":
        svg.circle(cx, cy, r * 0.86, fill, edge, 2)
        svg.circle(cx, cy, r * 0.46, "none", edge, 2.2)
        svg.text(cx, cy + 1, "K", size * 0.34, txt)
    elif kind == "queen":
        svg.circle(cx, cy, r * 0.86, fill, edge, 2)
        svg.text(cx, cy + 1, "Q", size * 0.5, txt, weight="800")
    elif kind == "arrow":
        d = r * 0.5
        svg.path("M%s %sL%s %sM%s %sL%s %s" % (f(cx - d), f(cy - d), f(cx + d), f(cy + d), f(cx - d), f(cy + d), f(cx + d), f(cy - d)), stroke="#444", sw=4)
    elif kind == "x":
        d = r * 0.5
        svg.path("M%s %sL%s %sM%s %sL%s %s" % (f(cx - d), f(cy - d), f(cx + d), f(cy + d), f(cx - d), f(cy + d), f(cx + d), f(cy - d)), stroke=fill, sw=size * 0.14)
    elif kind == "o":
        svg.circle(cx, cy, r * 0.5, "none", fill, size * 0.14)
    elif kind == "student":
        svg.circle(cx, cy, r * 0.62, fill, edge, 2)
    elif kind == "master":
        svg.circle(cx, cy, r * 0.82, fill, edge, 2)
        svg.circle(cx, cy, r * 0.42, "none", edge, 2.2)
    elif kind == "ring":
        svg.circle(cx, cy, r * 0.8, "none", edge, size * 0.2)
        svg.circle(cx, cy, r * 0.8, "none", fill, size * 0.12)
    elif kind == "marker":
        svg.circle(cx, cy, r * 0.5, fill, edge, 2)
    if label is not None:
        svg.text(cx, cy + 1, label, size * 0.36, txt, weight="800")


# ---- square boards ---------------------------------------------------------------------------

class Board:
    """A rectangle of cols x rows squares drawn at (x0, y0); row 0 is the bottom."""

    def __init__(self, svg, x0, y0, cols, rows, cell=44, checker=None, points=False, labels=False, frame=True, light=None, dark=None, edge=None):
        self.svg, self.x0, self.y0, self.cols, self.rows, self.cell = svg, x0, y0, cols, rows, cell
        self.points = points
        w, h = cols * cell, rows * cell
        if frame:
            svg.rect(x0 - 6, y0 - 6, w + 12, h + 12, edge or FRAME, rx=6)
        if points:
            svg.rect(x0, y0, w, h, "#e6c27a")
            for c in range(cols):
                cx = x0 + c * cell + cell / 2
                svg.line(cx, y0 + cell / 2, cx, y0 + h - cell / 2, "#5a4326", 1.2, cap="butt")
            for r in range(rows):
                cy = y0 + r * cell + cell / 2
                svg.line(x0 + cell / 2, cy, x0 + w - cell / 2, cy, "#5a4326", 1.2, cap="butt")
        else:
            for c in range(cols):
                for r in range(rows):
                    dark_sq = checker is not None and (c + r) % 2 == checker
                    svg.rect(x0 + c * cell, y0 + (rows - 1 - r) * cell, cell, cell, (dark or DARK) if dark_sq else (light or LIGHT), "#b9a57a" if checker is None else None, 1)
        if labels:
            for c in range(cols):
                svg.text(x0 + c * cell + cell / 2, y0 + h + 17, chr(97 + c), 11, "#6a5a40", weight="700")
            for r in range(rows):
                svg.text(x0 - 17, y0 + (rows - 1 - r) * cell + cell / 2, str(r + 1), 11, "#6a5a40", weight="700")

    def at(self, c, r):
        return self.x0 + c * self.cell + self.cell / 2, self.y0 + (self.rows - 1 - r) * self.cell + self.cell / 2

    def tint(self, cells, color=GREEN, op=0.7):
        for c, r in cells:
            x, y = self.at(c, r)
            if self.points:
                self.svg.circle(x, y, self.cell * 0.2, color, op=op)
            else:
                self.svg.rect(x - self.cell / 2 + 1, y - self.cell / 2 + 1, self.cell - 2, self.cell - 2, color, op=op)

    def dots(self, cells, color=GREEN):
        for c, r in cells:
            x, y = self.at(c, r)
            self.svg.circle(x, y, self.cell * 0.15, color, "#2b6a2b", 1.5)

    def put(self, c, r, kind, color, label=None, scale=0.82):
        x, y = self.at(c, r)
        piece(self.svg, x, y, kind, color, self.cell * scale, label)

    def ring_mark(self, c, r, color=RED, sw=3):
        x, y = self.at(c, r)
        self.svg.circle(x, y, self.cell * 0.46, "none", color, sw)

    def cross_mark(self, c, r, color=RED):
        x, y = self.at(c, r)
        d = self.cell * 0.22
        self.svg.path("M%s %sL%s %sM%s %sL%s %s" % (f(x - d), f(y - d), f(x + d), f(y + d), f(x - d), f(y + d), f(x + d), f(y - d)), stroke=color, sw=4)

    def arrow(self, a, b, color=RED, sw=3):
        (x1, y1), (x2, y2) = self.at(*a), self.at(*b)
        ang = math.atan2(y2 - y1, x2 - x1)
        self.svg.arrow(x1 + math.cos(ang) * self.cell * 0.3, y1 + math.sin(ang) * self.cell * 0.3, x2 - math.cos(ang) * self.cell * 0.28, y2 - math.sin(ang) * self.cell * 0.28, color, sw)

    def fence(self, c, r, side, color="#6a3a14", sw=5):
        """A fence on the n/e/s/w side of a square."""
        x, y = self.at(c, r)
        h = self.cell / 2
        a, b = {"n": ((-h, -h), (h, -h)), "s": ((-h, h), (h, h)), "w": ((-h, -h), (-h, h)), "e": ((h, -h), (h, h))}[side]
        self.svg.line(x + a[0], y + a[1], x + b[0], y + b[1], color, sw, cap="square")

    def width(self):
        return self.cols * self.cell

    def height(self):
        return self.rows * self.cell


# ---- hexagonal boards ---------------------------------------------------------------------------

class HexBoard:
    """Pointy-topped hexagons placed by axial (q, r); r grows downwards. `cells` is a list of (q, r)."""

    def __init__(self, svg, ox, oy, cells, size=20, outline=True):
        self.svg, self.ox, self.oy, self.size = svg, ox, oy, size
        self.cells = list(cells)
        if outline:
            for q, r in self.cells:
                self.hexagon(q, r, "#e9d6a4", "#a8905a", 1.4)

    def at(self, q, r):
        return self.ox + self.size * math.sqrt(3) * (q + r / 2), self.oy + self.size * 1.5 * r

    def corners(self, q, r, k=1.0):
        x, y = self.at(q, r)
        return [(x + self.size * k * math.cos(math.radians(60 * i - 30)), y + self.size * k * math.sin(math.radians(60 * i - 30))) for i in range(6)]

    def hexagon(self, q, r, fill, stroke=None, sw=1, op=None, k=1.0):
        self.svg.poly(self.corners(q, r, k), fill, stroke, sw, op)

    def put(self, q, r, color, label=None, kind="disc", scale=1.0):
        x, y = self.at(q, r)
        piece(self.svg, x, y, kind, color, self.size * 1.5 * scale, label)

    def tint(self, cells, color=GREEN, op=0.75):
        for q, r in cells:
            self.hexagon(q, r, color, None, 1, op)

    def mark(self, q, r, color=RED):
        x, y = self.at(q, r)
        self.svg.circle(x, y, self.size * 0.82, "none", color, 3)

    def bounds(self):
        xs, ys = [], []
        for q, r in self.cells:
            for x, y in self.corners(q, r):
                xs.append(x)
                ys.append(y)
        return min(xs), min(ys), max(xs), max(ys)


def hexhex(n):
    """Cells of a hexagon-shaped board with n cells to a side."""
    N = n - 1
    return [(q, r) for q in range(-N, N + 1) for r in range(-N, N + 1) if abs(q + r) <= N]


def caption(svg, x, y, t, size=13, fill=INK, anchor="middle"):
    svg.text(x, y, t, size, fill, anchor, weight="700")


def note(svg, cx, y, text, chars=36, size=11.5, line=16, fill=INK, anchor="middle", weight="500"):
    """Centred text wrapped to about `chars` characters a line."""
    import textwrap
    for i, t in enumerate(textwrap.wrap(text, chars)):
        svg.text(cx, y + i * line, t, size, fill, anchor, weight=weight)
