#!/usr/bin/env python3
"""Traces the five-player Primeval Map from a picture of the printed board.

    python3 scripts/cwprimevalart.py --board PICTURE [--check OUT.jpg]

PICTURE is a scan of the board, 2:1, kept outside the repository: no pixel of it is
copied. A trace starts from the marks in SEEDS and floods out to the board's dark
borders, which also gives the adjacency. The map's background is then generated: the
colours are those of each area, blurred until only their flow is left, with lettering
and drawings left out, and the mottling, cracks and ink are noise. It writes the
layout's areas and edges, the background and the Glacier glyph.

Needs numpy, scipy, scikit-image, shapely and Pillow.
"""
import argparse
import os
import re

import numpy as np
from PIL import Image
from scipy import ndimage as ndi
from shapely.geometry import Polygon
from skimage import color, draw, filters, measure, segmentation

Image.MAX_IMAGE_PIXELS = None

# Everything here is measured in a 1500 x 750 picture of the board.
W, H = 1500, 750
# The columns of the black line between the two halves of the board.
SEAM = (746, 755)

# Strokes inside each area. A thin line on the board only divides a sea from its own
# shallows, so a sea's strokes run on both sides of it.
SEEDS = {
    "hyperborea": [[(240, 55), (330, 90), (230, 130), (130, 150), (120, 200)]],
    "thule": [[(420, 120), (330, 190), (240, 260)]],
    "paratethys-ocean": [[(300, 15), (480, 40), (570, 70), (560, 150), (470, 190)], [(560, 150), (700, 210)]],
    "lomar": [[(650, 30), (780, 60), (860, 20)], [(650, 30), (700, 110)]],
    "poseidonis": [[(960, 20), (900, 100), (800, 170), (950, 230)]],
    "mezcalera-ocean": [[(100, 30), (20, 200), (60, 300), (300, 280), (450, 320), (560, 250)], [(20, 200), (90, 100)], [(300, 280), (170, 335)], [(450, 320), (530, 300)]],
    "mezcalera-east": [[(1470, 15), (1490, 150), (1495, 330)]],
    "iapetus-ocean": [[(30, 430), (10, 600), (40, 700), (200, 700), (400, 690), (510, 715)], [(60, 500), (30, 430)]],
    "iapetus-east": [[(1480, 385), (1465, 450), (1450, 600), (1485, 735)], [(1480, 385), (1496, 420), (1496, 730)]],
    "tethys-ocean": [[(500, 500), (620, 450), (700, 550), (560, 640)], [(700, 550), (740, 600)]],
    "west-atlantis": [[(600, 330), (680, 320), (640, 400)]],
    "atlantis": [[(720, 420), (830, 330), (800, 240), (930, 260)]],
    "ocean-trench": [[(1000, 240), (960, 380), (1000, 450), (1040, 430)]],
    "mirovia-ocean": [[(1100, 60), (1250, 100), (1150, 160), (1360, 150), (1420, 120), (1450, 250), (1440, 330)], [(1360, 150), (1400, 300)], [(1150, 160), (1040, 200), (1060, 300)]],
    "upper-mu": [[(1150, 280), (1300, 270)]],
    "central-mu": [[(1090, 440), (1100, 380), (1200, 430), (1260, 420)]],
    "lower-mu": [[(1180, 500), (1300, 480), (1370, 500)]],
    "rheic-ocean": [[(1075, 550), (1050, 650), (1250, 650), (1375, 550), (1375, 425), (1340, 400)], [(1250, 650), (1400, 730)]],
    "terra-australis": [[(960, 560), (900, 640), (1010, 620)]],
    "rutas": [[(830, 480), (800, 580), (860, 620)]],
    "antarctica": [[(540, 730), (650, 680), (800, 700), (950, 735)], [(650, 680), (652, 590)]],
    "south-lemuria": [[(70, 645), (110, 610), (200, 590), (290, 640)]],
    "shasta": [[(110, 490), (180, 440), (240, 500)]],
    "east-lemuria": [[(330, 480), (400, 560), (490, 540), (310, 640)]],
}

# Land the flood would get wrong, as a printed glyph opens a way in from the sea.
SOLID = {"thule": [(212, 262), (240, 296), (272, 306), (306, 268)]}

HERE = os.path.dirname(os.path.abspath(__file__))
GAME = os.path.join(HERE, "..", "internal", "games", "cthulhuwars")
LAYOUT = os.path.join(GAME, "definition", "variants", "cthulhu-wars-primeval-5p.yaml")
ART_NAME = "guide/maps/primeval-5p.webp"
ICE_JS = os.path.join(HERE, "..", "internal", "web", "static", "js", "cw-ice.js")
CANVAS = 1000 / W

# What is printed on the board and is not the board's colour: (x, y, radius).
PRINTED = [
    (232, 170, 30), (1238, 310, 30), (907, 118, 30), (657, 685, 30), (1283, 487, 30), (950, 595, 30), (360, 697, 30),
    (257, 50, 24), (263, 285, 24), (110, 490, 24), (278, 630, 24), (475, 545, 24),
    (655, 420, 24), (900, 325, 24), (950, 530, 24), (770, 473, 24), (955, 215, 22),
    (1268, 382, 22), (1097, 447, 22), (1188, 513, 22),
    (70, 30, 40), (120, 705, 95), (1375, 35, 100), (1430, 710, 45),
]

# Pieces of one area that the map's edge cuts apart.
SAME = {"mezcalera-east": "mezcalera-ocean", "iapetus-east": "iapetus-ocean"}


def load(path):
    im = Image.open(path).convert("RGB")
    rgb = np.asarray(im.resize((W, H), Image.LANCZOS), dtype=np.float32) / 255
    return close_seam(rgb)


def close_seam(rgb, x0=SEAM[0], x1=SEAM[1]):
    """Paints out the black line where the two halves of the board meet: a border would
    run along it, and it is no part of the map."""
    out = rgb.copy()
    t = (np.arange(x0, x1) - (x0 - 1)) / (x1 - x0 + 1)
    out[:, x0:x1] = rgb[:, x0 - 1, None] * (1 - t[None, :, None]) + rgb[:, x1, None] * t[None, :, None]
    return out


def ice_blobs(rgb):
    """The Glacier glyphs: large pale, unsaturated blobs. Lettering and arrows are far smaller."""
    hsv = color.rgb2hsv(rgb)
    pale = (hsv[..., 2] > 0.72) & (hsv[..., 1] < 0.32)
    pale = ndi.binary_closing(pale, iterations=2)
    blobs, n = ndi.label(pale)
    out = []
    for i, box in enumerate(ndi.find_objects(blobs), 1):
        h, w = box[0].stop - box[0].start, box[1].stop - box[1].start
        area = (blobs[box] == i).sum()
        if area > 700 and 33 < w < 70 and 35 < h < 85:
            out.append((box, ndi.binary_fill_holes(blobs[box] == i)))
    return out


def flood(relief, marks):
    return segmentation.watershed(relief, marks)


def trace(rgb):
    """Floods from the seeds over the board's darkness and edges: labels from 1."""
    grey = color.rgb2gray(rgb)
    fine = ndi.gaussian_filter(grey, 1.0)
    # A border is dark and has edges; a black line anywhere outweighs mottling.
    relief = (1 - fine) ** 2 + 2.5 * filters.sobel(fine)
    ids = list(SEEDS)
    marks = np.zeros((H, W), dtype=np.int32)
    for n, aid in enumerate(ids, 1):
        for stroke in SEEDS[aid]:
            for (x0, y0), (x1, y1) in zip(stroke, stroke[1:]):
                rr, cc = draw.line(y0, x0, y1, x1)
                ok = (rr >= 0) & (rr < H) & (cc >= 0) & (cc < W)
                marks[rr[ok], cc[ok]] = n
    for aid, poly in SOLID.items():
        rr, cc = draw.polygon([y for _, y in poly], [x for x, _ in poly], (H, W))
        marks[rr, cc] = ids.index(aid) + 1
    # A glyph printed over a coast would bite into it: it belongs whole to the area
    # most of it stands in.
    first = flood(relief, marks)
    glyphs = []
    blobs = ice_blobs(rgb)
    for box, mask in blobs:
        owner = np.bincount(first[box][mask]).argmax()
        grown = ndi.binary_dilation(mask, iterations=3)
        marks[box][grown[:]] = owner
        cy = (box[0].start + box[0].stop) / 2
        cx = (box[1].start + box[1].stop) / 2
        glyphs.append((SAME.get(ids[owner - 1], ids[owner - 1]), cx, cy, box[1].stop - box[1].start, box[0].stop - box[0].start))
    return flood(relief, marks), ids, glyphs, blobs


def merged(labels, ids):
    """Labels with the pieces the map's edge cuts apart made one area."""
    names = sorted({SAME.get(i, i) for i in ids})
    table = np.zeros(len(ids) + 1, dtype=np.int32)
    for n, aid in enumerate(ids, 1):
        table[n] = names.index(SAME.get(aid, aid)) + 1
    return table[labels], names


def adjacency(labels, names, min_edge=3):
    """Areas that share a stretch of border."""
    count = {}
    for a, b in ((labels[:, :-1], labels[:, 1:]), (labels[:-1, :], labels[1:, :])):
        diff = a != b
        for x, y in zip(a[diff], b[diff]):
            key = (min(x, y), max(x, y))
            count[key] = count.get(key, 0) + 1
    return {(names[x - 1], names[y - 1]): n for (x, y), n in count.items() if n >= min_edge}


def check(rgb, labels, out):
    edge = segmentation.find_boundaries(labels, mode="inner")
    pic = (rgb * 255).astype(np.uint8)
    pic[edge] = (255, 0, 255)
    Image.fromarray(pic).save(out, quality=88)


# ---- the areas -----------------------------------------------------------------------


def outlines(area, k, min_size=120):
    """The polygons of area k, in the canvas's units: the contour, a little smoothed."""
    parts, n = ndi.label(area == k)
    out = []
    for i in range(1, n + 1):
        part = parts == i
        if part.sum() < min_size:
            continue
        pad = np.pad(part, 1).astype(float)
        c = max(measure.find_contours(pad, 0.5), key=len)[:-1] - 1
        c = ndi.gaussian_filter1d(c, 1.2, axis=0, mode="wrap")
        c = measure.approximate_polygon(np.vstack([c, c[:1]]), 1.1)[:-1]
        pts = [(round(float(np.clip(x * CANVAS, 0, 1000)), 1), round(float(np.clip(y * CANVAS, 0, 500)), 1)) for y, x in c]
        out.append(pts)
    return sorted(out, key=lambda p: -Polygon(p).buffer(0).area)


def anchor(area, k):
    """The roomiest point of the area's largest piece."""
    parts, n = ndi.label(area == k)
    big = np.argmax(ndi.sum_labels(np.ones_like(parts), parts, range(1, n + 1))) + 1
    room = ndi.distance_transform_edt(parts == big)
    y, x = np.unravel_index(room.argmax(), room.shape)
    return round(x * CANVAS, 1), round(y * CANVAS, 1)


def write_layout(area, names, glyphs, fills):
    with open(LAYOUT) as f:
        old = f.read()
    tags = dict(re.findall(r"- id: (\S+)\n\s+name: .*\n\s+tags: \[(.*)\]", old))
    label = dict(re.findall(r"- id: (\S+)\n\s+name: (.*)\n", old))
    head = old[: old.index("    positions:")]
    tail = old[old.index("  seats:"):]
    params = old[old.index("  params:") : old.index("  seats:")]
    lines = ["    positions:"]
    for n, aid in enumerate(names, 1):
        x, y = anchor(area, n)
        lines += [f"      - id: {aid}", f"        name: {label[aid]}", f"        tags: [{tags[aid]}]", f"        anchor: [{x}, {y}]", "        polygons:"]
        for poly in outlines(area, n):
            for i, (px, py) in enumerate(poly):
                lines.append(("          - - " if i == 0 else "            - ") + f"[{px}, {py}]")
    lines.append("    edges:")
    # The seas that meet round the map's edge are the same two on either side of it.
    edges = set(adjacency(area, names))
    for a, b in sorted(edges):
        lines.append(f"      - {{from: {a}, to: {b}}}")
    keep = [l for l in params.splitlines() if not re.match(r"    (art|ice_glyphs|area_fill):", l)]
    keep.append(f"    art: {ART_NAME}")
    marks = ", ".join(f"[{round(x * CANVAS, 1)}, {round(y * CANVAS, 1)}]" for _, x, y, _, _ in sorted(glyphs))
    keep.append(f"    ice_glyphs: [{marks}]")
    fill = ", ".join(f"{a}: '{fills[a]}'" for a in names)
    keep.append(f"    area_fill: {{{fill}}}")
    with open(LAYOUT, "w") as f:
        f.write(head + "\n".join(lines) + "\n" + "\n".join(keep) + "\n" + tail)
    return edges


# ---- the background ------------------------------------------------------------------


def surface(rgb, glyph_boxes):
    """The pixels that show the board's own colour."""
    grey = color.rgb2gray(rgb)
    keep = np.abs(grey - ndi.gaussian_filter(grey, 7)) < 0.055
    yy, xx = np.mgrid[:H, :W]
    for x, y, r in PRINTED:
        keep[(xx - x) ** 2 + (yy - y) ** 2 < r * r] = False
    for box, _ in glyph_boxes:
        y0, y1 = max(box[0].start - 8, 0), box[0].stop + 8
        x0, x1 = max(box[1].start - 8, 0), box[1].stop + 8
        keep[y0:y1, x0:x1] = False
    return keep


def noise(rng, sigmas):
    out = np.zeros((H, W), dtype=np.float32)
    for sg, weight in sigmas:
        n = ndi.gaussian_filter(rng.standard_normal((H, W)).astype(np.float32), sg, mode="wrap")
        out += weight * n / n.std()
    return out / out.std()


def cell_walls(rng, spacing=46):
    """How near each pixel is to the wall between two warped cells, in pixels."""
    n = int(W * H / spacing**2)
    pts = np.column_stack([rng.random(n) * H, rng.random(n) * W])
    yy, xx = np.mgrid[:H, :W].astype(np.float32)
    yy = yy + 9 * noise(rng, [(18, 1.0), (6, 0.35)])
    xx = xx + 9 * noise(rng, [(18, 1.0), (6, 0.35)])
    from scipy.spatial import cKDTree

    d, _ = cKDTree(pts).query(np.column_stack([yy.ravel(), xx.ravel()]), k=2)
    return (d[:, 1] - d[:, 0]).reshape(H, W)


def shelf_width(rgb, ocean, to_land):
    """How far from land the board's thin coastal line runs: read where it is seen, and
    carried smoothly along the coast."""
    grey = ndi.gaussian_filter(color.rgb2gray(rgb), 0.7)
    ridge = np.clip(ndi.gaussian_filter(grey, 4) - grey, 0, None)
    ridge = ridge * ocean * (to_land > 5) * (to_land < 48)
    total = ndi.gaussian_filter(ridge, 26)
    return np.where(total > 1e-4, ndi.gaussian_filter(ridge * to_land, 26) / np.maximum(total, 1e-4), 17)


def bake(rgb, area, names, glyph_boxes):
    """The map's background, and each area's middle colour."""
    keep = surface(rgb, glyph_boxes)
    grey = color.rgb2gray(rgb)
    rng = np.random.default_rng(1928)
    border = segmentation.find_boundaries(area, mode="inner")
    field = np.zeros_like(rgb)
    fills = {}
    amp = np.zeros((H, W), dtype=np.float32)
    lines = np.zeros((H, W), dtype=np.float32)
    ocean = np.zeros((H, W), dtype=bool)
    for k, aid in enumerate(names, 1):
        region = area == k
        ocean[region] = "ocean" in aid or aid == "ocean-trench"
        m = (region & keep).astype(np.float32)
        median = np.median(rgb[region & keep], axis=0)
        # Fine where the board shows, broad across what was left out: lettering leaves no smudge.
        flow = median
        for sg, need in ((40, 0.02), (16, 0.2), (6, 0.45)):
            weight = ndi.gaussian_filter(m, sg)[..., None]
            spread = np.stack([ndi.gaussian_filter(rgb[..., c] * m, sg) for c in range(3)], -1)
            sure = np.clip(weight / need - 0.5, 0, 1)
            flow = flow * (1 - sure) + sure * spread / np.maximum(weight, 1e-3)
        field[region] = flow[region]
        fills[aid] = "#%02x%02x%02x" % tuple(int(v * 255) for v in median)
        rest = grey - color.rgb2gray(flow)
        amp[region] = np.clip(rest[region & keep].std() / max(float(np.median(grey[region & keep])), 0.05), 0.04, 0.2)
        # How much of the area is drawn over with thin dark lines.
        lines[region] = np.mean((grey[region] - ndi.gaussian_filter(grey, 7)[region]) < -0.05)
    mottle = noise(rng, [(24, 0.8), (10, 0.8), (4, 0.7), (1.6, 0.7), (0.7, 0.7)])
    grain = noise(rng, [(0.5, 1.0)])
    shade = 1 + amp * (0.8 * mottle + 0.5 * grain)
    # Cracks: the walls of warped cells, broken up, stronger on the areas with most lines.
    gate = np.clip(noise(rng, [(28, 1.0)]) + 0.35, 0, 1)
    wall = np.clip(1 - cell_walls(rng) / 1.1, 0, 1)
    shade *= 1 - np.clip(lines * 3.6, 0, 0.6) * wall * gate
    # Pale specks on the darker seas.
    specks = ndi.gaussian_filter((rng.random((H, W)) > 0.9987).astype(np.float32), 0.7) * 3.4
    out = field * shade[..., None] + (ocean * specks * 0.32 * (1 - color.rgb2gray(field)))[..., None]
    dist = ndi.distance_transform_edt(~border)
    land = ~ocean
    to_land = ndi.distance_transform_edt(~land)
    # A land darkens toward its shore, with etched lines inside it.
    wander = noise(rng, [(14, 1.0), (4, 0.5)])
    etch = np.exp(-(((dist - (8 + 1.6 * wander)) / 0.8) ** 2)) + 0.7 * np.exp(-(((dist - (15 + 2.5 * wander)) / 0.7) ** 2))
    breaks = np.clip(noise(rng, [(11, 1.0)]) + 0.45, 0, 1)
    out = out * (1 - (0.3 * np.exp(-dist / 5) + 0.3 * etch * breaks) * land)[..., None]
    # The shallows end in a thin line out at sea, as far out as the board's own does.
    out_at = shelf_width(rgb, ocean, to_land)
    jag = noise(rng, [(5, 1.0), (1.8, 0.5)])
    shelf = np.exp(-(((to_land - (out_at + 1.1 * jag)) / 0.85) ** 2)) * ocean * (to_land < 60)
    out = out * (1 - 0.55 * shelf)[..., None]
    # The ink: heavier where two seas meet, and never quite even.
    sea_sea = ocean & border & ~ndi.binary_dilation(land, iterations=2)
    width = np.where(ndi.binary_dilation(sea_sea, iterations=2), 2.5, 1.45) * (1 + 0.28 * noise(rng, [(5, 1.0)]))
    ink = np.exp(-((dist / np.maximum(width, 0.6)) ** 2))
    out = out * (1 - 0.9 * ink)[..., None] + np.array([0.07, 0.055, 0.04]) * (0.9 * ink)[..., None]
    pic = Image.fromarray((np.clip(out, 0, 1) * 255).astype(np.uint8))
    path = os.path.join(GAME, ART_NAME)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    pic.save(path, "WEBP", quality=80, method=6)
    return fills, os.path.getsize(path), pic


# ---- the Glacier glyph ---------------------------------------------------------------


def trace_ice(path, glyphs):
    """All twelve glyphs averaged, then drawn as stacked shapes of its tones."""
    im = np.asarray(Image.open(path).convert("L"), dtype=np.float32) / 255
    scale = im.shape[1] / W
    bw, bh = int(46 * scale), int(62 * scale)
    crops = []
    for _, x, y, _, _ in glyphs:
        x0, y0 = int(x * scale - bw / 2), int(y * scale - bh / 2)
        crops.append(im[y0 : y0 + bh, x0 : x0 + bw])
    mean = np.median(np.stack(crops), axis=0)
    # The sticker's outline is the outermost pale shape.
    pale, _ = ndi.label(ndi.binary_closing(mean > 0.62, iterations=2))
    big = np.argmax(np.bincount(pale.ravel())[1:]) + 1
    outline = ndi.binary_fill_holes(pale == big)
    smooth = ndi.gaussian_filter(mean, 0.8)
    layers = [("#ffffff", outline)]
    for fill, level in (("#c9e3f1", 0.86), ("#8fbcdb", 0.70), ("#3f7ba8", 0.50), ("#1f4e75", 0.32)):
        layers.append((fill, (smooth < level) & ndi.binary_erosion(outline, iterations=2)))
    k = 1 / scale * CANVAS
    out = []
    for fill, mask in layers:
        d = []
        for c in measure.find_contours(np.pad(mask, 1).astype(float), 0.5):
            if len(c) < 10:
                continue
            c = measure.approximate_polygon(ndi.gaussian_filter1d(c[:-1] - 1, 0.8, axis=0, mode="wrap"), 0.55)
            pts = " ".join(f"{(px - bw / 2) * k:.1f} {(py - bh / 2) * k:.1f}" for py, px in c)
            d.append("M" + pts + "Z")
        out.append((fill, "".join(d)))
    with open(ICE_JS, "w") as f:
        f.write("// Written by scripts/cwprimevalart.py: the Glacier glyph of the Primeval Map, centred on\n")
        f.write("// the origin in the units of the map, as stacked shapes from its palest tone to its darkest.\n")
        f.write("export const ICE_GLYPH = [\n")
        for fill, d in out:
            f.write(f'\t{{ fill: "{fill}", d: "{d}" }},\n')
        f.write("];\n")
    return outline.sum() / outline.size


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--board", required=True)
    ap.add_argument("--check", help="draw the borders over the board, to this file")
    args = ap.parse_args()
    rgb = load(args.board)
    labels, ids, glyphs, blobs = trace(rgb)
    area, names = merged(labels, ids)
    if args.check:
        check(rgb, area, args.check)
    fills, size, _ = bake(rgb, area, names, blobs)
    edges = write_layout(area, names, glyphs, fills)
    trace_ice(args.board, glyphs)
    print(f"{len(names)} areas, {len(edges)} edges, {len(glyphs)} Glacier glyphs, background {size // 1024} KB")


if __name__ == "__main__":
    main()
