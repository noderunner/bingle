#!/usr/bin/env python3
"""Builds the faction glyphs (sigils) of Cthulhu Wars as SVG.

    python3 scripts/cwglyphs.py DIR      # DIR holds the wiki's sigil images

The Cthulhu Wars wiki shows every faction's sigil as a small picture
("Sigil - Great Cthulhu.png"; they are really WebP files of 90 x 90 pixels).
Those pictures are only an input: this script traces their outline (the
transparent background says what is the sigil), smooths it, and writes

  * internal/games/cthulhuwars/guide/glyphs/<key>.svg   one SVG per faction,
    which the rules window shows next to each faction; and
  * internal/web/static/js/cw-glyphs.js                  the same paths as data,
    which the map draws as the sigil of the areas where a cult begins.

Every glyph is scaled to a 100 x 100 box and filled with the colour the
original shows (the average of its opaque pixels) and a darker outline.

The images: DIR/<Name>.png with the names of GLYPHS below (the wiki file names
without "Sigil - ", spaces turned into underscores). They can be fetched with
the MediaWiki API:

    https://cthulhuwars.fandom.com/api.php?action=query&list=allimages
        &aiprefix=Sigil&ailimit=100&format=json&aiprop=url

The Ancients, Bubastis, Daemon Sultan and The Invasion are not factions of this
game (yet): their glyphs are placeholders so that the rules can list them.
(The Bad Movie Dimension, a joke faction, is left out on purpose.)

Needs numpy, scipy, scikit-image and Pillow.
"""
import json
import os
import sys

import numpy as np
from PIL import Image
from scipy import ndimage as ndi
from skimage import measure

# key → (image name, display name, playable)
GLYPHS = [
    ("gc", "Great_Cthulhu", "Great Cthulhu", True),
    ("cc", "Crawling_Chaos", "Crawling Chaos", True),
    ("bg", "Black_Goat", "Black Goat", True),
    ("ys", "Yellow_Sign", "Yellow Sign", True),
    ("ww", "Windwalker", "Windwalker", True),
    ("sl", "Sleeper", "Sleeper", True),
    ("ow", "Opener_of_the_Way", "Opener of the Way", True),
    ("tt", "Tcho-Tcho", "Tcho-Tcho", True),
    ("az", "Azathoth", "Azathoth", True),
    ("an", "The_Ancients", "The Ancients", False),
    ("bu", "Bubastis", "Bubastis", False),
    ("ds", "Daemon_Sultan", "Daemon Sultan", False),
    ("in", "The_Invasion", "The Invasion", False),
]

BOX = 100.0


def load_mask(path):
    """Alpha of the image as a float array in [0, 1], upscaled and smoothed."""
    im = Image.open(path).convert("RGBA")
    w, h = im.size
    # Upscale so that the contours have sub-pixel detail: about 720 px a side.
    k = max(1, round(720 / max(w, h)))
    big = im.resize((w * k, h * k), Image.LANCZOS)
    arr = np.asarray(big).astype(np.float32)
    # "Ink" is what is opaque and not dark: the sigils are drawn with a dark
    # shadow that is only half transparent, and the Invasion's is red lines on a
    # black disc.
    ink = (arr[..., 3] / 255.0) * (arr[..., :3].max(axis=2) / 255.0)
    ink = ndi.gaussian_filter(ink, sigma=k * 0.35)
    # Dark sigils (the Sleeper's brown, the Opener's purple) are normalised to
    # the brightest part of their own drawing.
    solid = ink[arr[..., 3] > 250]
    if solid.size:
        ink = ink / max(float(np.percentile(solid, 92)), 0.2)
    return im, np.clip(ink, 0, 1.5), k


def average_colour(im):
    """The colour of the sigil: the average of its brightest opaque pixels."""
    arr = np.asarray(im).astype(np.float32)
    bright = arr[..., :3].max(axis=2)
    solid = (arr[..., 3] > 200) & (bright > 0.75 * bright[arr[..., 3] > 200].max())
    if not solid.any():
        return (128, 128, 128)
    rgb = arr[..., :3][solid]
    return tuple(int(round(v)) for v in rgb.mean(axis=0))


def hexcol(rgb, mul=1.0):
    return "#%02x%02x%02x" % tuple(max(0, min(255, int(round(c * mul)))) for c in rgb)


def simplify(points, tol):
    """Douglas-Peucker on a closed ring."""
    pts = measure.approximate_polygon(np.asarray(points), tolerance=tol)
    if len(pts) > 1 and np.allclose(pts[0], pts[-1]):
        pts = pts[:-1]
    return pts


def ring_path(pts, scale, ox, oy):
    """A smooth closed path through pts: Catmull-Rom curves, with corners where
    the outline turns sharply (the tips of the sigils stay sharp)."""
    n = len(pts)
    if n < 3:
        return ""
    P = [(ox + p[1] * scale, oy + p[0] * scale) for p in pts]

    def corner(i):
        a, b, c = P[i - 1], P[i], P[(i + 1) % n]
        v1 = (b[0] - a[0], b[1] - a[1])
        v2 = (c[0] - b[0], c[1] - b[1])
        l1 = (v1[0] ** 2 + v1[1] ** 2) ** 0.5
        l2 = (v2[0] ** 2 + v2[1] ** 2) ** 0.5
        if l1 < 1e-6 or l2 < 1e-6:
            return False
        cos = (v1[0] * v2[0] + v1[1] * v2[1]) / (l1 * l2)
        return cos < 0.35  # turns by more than about 70 degrees

    corners = [corner(i) for i in range(n)]

    def tangent(i):
        if corners[i]:
            return (0.0, 0.0)
        a, c = P[i - 1], P[(i + 1) % n]
        return ((c[0] - a[0]) / 6.0, (c[1] - a[1]) / 6.0)

    f = lambda v: ("%.2f" % v).rstrip("0").rstrip(".")
    out = ["M%s %s" % (f(P[0][0]), f(P[0][1]))]
    for i in range(n):
        j = (i + 1) % n
        t0, t1 = tangent(i), tangent(j)
        c1 = (P[i][0] + t0[0], P[i][1] + t0[1])
        c2 = (P[j][0] - t1[0], P[j][1] - t1[1])
        if corners[i] and corners[j]:
            out.append("L%s %s" % (f(P[j][0]), f(P[j][1])))
        else:
            out.append("C%s %s %s %s %s %s" % (f(c1[0]), f(c1[1]), f(c2[0]), f(c2[1]), f(P[j][0]), f(P[j][1])))
    out.append("Z")
    return "".join(out)


def trace(path, coarse=False):
    im, alpha, k = load_mask(path)
    # The outline is where the alpha crosses one half.
    padded = np.pad(alpha, 2)
    contours = measure.find_contours(padded, 0.42)
    h, w = alpha.shape
    rings = []
    for c in contours:
        c = c - 2  # undo the padding
        if len(c) < 12:
            continue
        pts = simplify(c, tol=(1.1 * k * 0.18 + 0.6) * (2.2 if coarse else 1.0))
        if len(pts) < 3:
            continue
        # Tiny specks are noise.
        area = 0.5 * abs(np.dot(pts[:, 0], np.roll(pts[:, 1], 1)) - np.dot(pts[:, 1], np.roll(pts[:, 0], 1)))
        if area < (w * h) * 0.0007:
            continue
        rings.append(pts)
    # Fit the whole drawing into the box, centred.
    allp = np.vstack(rings)
    y0, x0 = allp.min(axis=0)
    y1, x1 = allp.max(axis=0)
    side = max(y1 - y0, x1 - x0)
    margin = 3.0
    scale = (BOX - 2 * margin) / side
    ox = margin + ((BOX - 2 * margin) - (x1 - x0) * scale) / 2 - x0 * scale
    oy = margin + ((BOX - 2 * margin) - (y1 - y0) * scale) / 2 - y0 * scale
    d = "".join(ring_path(r, scale, ox, oy) for r in rings)
    return d, average_colour(im)


def svg(d, rgb, title):
    fill = hexcol(rgb, 1.0)
    edge = hexcol(rgb, 0.5)
    return (
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" width="100" height="100" role="img">'
        "<title>%s</title>"
        '<path fill="%s" fill-rule="evenodd" stroke="%s" stroke-width="0.8" stroke-linejoin="round" d="%s"/>'
        "</svg>\n" % (title, fill, edge, d)
    )


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(2)
    src = sys.argv[1]
    root = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
    out_dir = os.path.join(root, "internal", "games", "cthulhuwars", "guide", "glyphs")
    os.makedirs(out_dir, exist_ok=True)
    data = {}
    for key, name, title, playable in GLYPHS:
        d, rgb = trace(os.path.join(src, name + ".png"), coarse=not playable)
        with open(os.path.join(out_dir, key + ".svg"), "w") as f:
            f.write(svg(d, rgb, title + " glyph"))
        if playable:
            data[key] = {"name": title, "fill": hexcol(rgb), "edge": hexcol(rgb, 0.5), "d": d, "playable": playable}
        print("%-3s %-18s %6d bytes  %s" % (key, title, len(d), hexcol(rgb)))
    js = os.path.join(root, "internal", "web", "static", "js", "cw-glyphs.js")
    with open(js, "w") as f:
        f.write("// Generated by scripts/cwglyphs.py from the sigils of the Cthulhu Wars wiki: do not edit.\n")
        f.write("// Each glyph is one path in a 100 x 100 box (even-odd fill), with the colours the sigil shows.\n")
        f.write("export const CW_GLYPHS = ")
        json.dump(data, f, indent=1, sort_keys=False)
        f.write(";\n")


if __name__ == "__main__":
    main()
