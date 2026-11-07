#!/usr/bin/env python3
"""Traces a map of Cthulhu Wars from a picture of the printed board.

    python3 scripts/cwmapart.py --map NAME --board PICTURE [--check OUT.jpg]

NAME is earth-3p, earth-3-5, earth-5-3, earth-5p, or the same four of primeval: left half
then right, as the corners of each board print them. PICTURE is a scan of that board,
about 2:1, kept outside the repository: no pixel of it is copied. A trace starts from a map's seed strokes and floods out to the board's dark
borders, which also gives the adjacency. The map's background is then generated: the
colours are those of each area, blurred until only their flow is left, with lettering
and drawings left out, and the mottling, cracks and ink are noise. It writes the
layout's areas and edges, the background, and the glyphs printed on the board.

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

# Everything here is measured in a picture of the board 1500 wide.
W = 1500
HERE = os.path.dirname(os.path.abspath(__file__))
GAME = os.path.join(HERE, "..", "internal", "games", "cthulhuwars")
JS = os.path.join(HERE, "..", "internal", "games", "cthulhuwars", "web")
CANVAS = 1000 / W

# The boards are built from halves, a three-player or a five-player one on each side: the
# coasts are the same on all, and only how the land is cut up differs. So each map is its
# two halves, plus the seas every board shares.
#
# A half or a map has: strokes inside each area, where a thin line on the board only
# divides a sea from its own shallows, so a sea's strokes run on both sides of it (the ids
# in "same" are pieces of one area that the map's edge cuts apart); land the flood would
# get wrong ("solid"); what is printed and is not the board's colour, as (x, y, radius);
# and where its printed Desecrate glyphs stand. A map has its height, the columns of the
# black line between the board's two halves ("seam"), and "shapes": this map's glyphs are
# the ones traced into the game's drawings of them.

EARTH = {
    "height": 748,
    "islands": True,
    "same": {"north-pacific-east": "north-pacific", "indian-west": "indian-ocean"},
    "printed": [(780, 65, 28), (450, 688, 30), (1365, 40, 105), (130, 712, 110), (60, 30, 36), (1432, 712, 40)],
    "seeds": {
        "arctic-ocean": [[(30, 40), (300, 22), (600, 30), (700, 100), (800, 40), (950, 15), (1200, 50), (1450, 50)], [(700, 100), (810, 125)]],
        "north-pacific": [[(60, 220), (100, 330), (240, 420), (325, 483), (360, 470)], [(325, 483), (325, 443)]],
        "north-pacific-east": [[(1490, 110), (1480, 200), (1470, 300), (1450, 418)]],
        "north-atlantic": [[(380, 360), (480, 280), (600, 280), (750, 440), (900, 300), (660, 285), (670, 170)], [(900, 300), (1060, 340)], [(480, 280), (440, 395)]],
        "south-atlantic": [[(700, 520), (800, 500), (800, 560), (620, 610), (700, 640), (1000, 650), (1120, 690)]],
        "south-pacific": [[(180, 430), (210, 493), (325, 518), (300, 640), (330, 720), (150, 720), (20, 735)], [(330, 720), (500, 715), (600, 720)]],
        "indian-west": [[(100, 433), (50, 468), (15, 568), (50, 658), (120, 650)]],
        "indian-ocean": [[(1250, 480), (1400, 500), (1300, 650), (1400, 700), (1340, 730)], [(1300, 650), (1230, 600)]],
        "antarctica": [[(700, 715), (720, 680), (850, 640), (900, 680), (1000, 710), (1100, 720), (1300, 740)]],
    },
    "solid": {"antarctica": [(685, 665), (725, 632), (815, 640), (835, 622), (875, 623), (880, 652), (930, 682), (990, 684), (1010, 700), (700, 730)]},
}
EARTH_LEFT = {
    3: {
        "printed": [(315, 240, 30), (437, 272, 24), (510, 633, 24), (203, 632, 24)],
        "glyphs": {"chevron": [(203, 632)], "dragon": [(437, 272), (510, 633)]},
        "seeds": {
            "north-america": [[(150, 100), (300, 100), (400, 180), (260, 280), (340, 390)], [(400, 180), (560, 130)]],
            "south-america": [[(390, 450), (450, 500), (600, 480), (500, 600), (540, 650)]],
            "australia": [[(80, 550), (200, 540), (230, 620)]],
        },
    },
    5: {
        "printed": [(300, 232, 30), (522, 175, 24), (425, 272, 24), (295, 375, 24), (403, 495, 24), (617, 482, 24), (107, 512, 24), (182, 487, 24)],
        "glyphs": {"chevron": [(107, 512), (182, 487)], "dragon": [(522, 175), (425, 272), (295, 375), (403, 495), (617, 482)]},
        "seeds": {
            # Round Hudson Bay, which is the Arctic Ocean's.
            "north-america-east": [[(330, 85), (400, 130), (470, 190), (560, 130)], [(470, 190), (480, 230)]],
            "north-america-west": [[(150, 105), (250, 130), (320, 240), (400, 250), (440, 300)], [(320, 240), (220, 230)]],
            # It runs on past the isthmus, along the north of South America, with no line across.
            "central-america": [[(210, 300), (280, 345), (340, 395), (357, 417), (370, 428), (393, 447), (420, 442), (453, 437)], [(393, 447), (378, 480)]],
            "south-america-west": [[(390, 490), (450, 530), (500, 620), (570, 660)]],
            "south-america-east": [[(480, 445), (540, 470), (640, 480), (560, 570)]],
            "australia": [[(40, 570), (110, 510), (170, 540), (100, 620)]],
            "new-zealand": [[(140, 470), (200, 500), (270, 590), (200, 650), (140, 640)]],
        },
    },
}
EARTH_RIGHT = {
    3: {
        "printed": [(955, 190, 30), (1045, 490, 30), (1260, 295, 30), (815, 680, 30), (780, 225, 24), (1415, 242, 24), (855, 445, 24)],
        "glyphs": {"chevron": [(855, 445)], "thorns": [(780, 225), (1415, 242)]},
        "solid": {"asia": [(1440, 270), (1462, 290), (1470, 335), (1462, 375), (1430, 400), (1410, 380), (1430, 320)]},
        "seeds": {
            "europe": [[(750, 220), (850, 200), (1000, 220), (1080, 250)], [(900, 130), (1000, 100), (1120, 110)], [(1000, 100), (1040, 50)]],
            "asia": [[(1180, 180), (1250, 130), (1400, 100), (1430, 130)], [(1300, 260), (1400, 320), (1420, 380)], [(1200, 300), (1150, 400), (1280, 400), (1230, 460)]],
            "africa": [[(880, 420), (1000, 400), (850, 490)], [(1050, 480), (1000, 580), (1100, 600), (1180, 500)]],
        },
    },
    5: {
        "printed": [(933, 210, 30), (933, 435, 30), (1352, 298, 30), (795, 685, 30), (780, 225, 24), (900, 140, 24), (1410, 112, 24), (1435, 362, 24), (1300, 392, 24), (845, 435, 24), (1075, 625, 24)],
        "glyphs": {"chevron": [(845, 435), (1075, 625)], "thorns": [(780, 225), (900, 140), (1410, 112), (1435, 362), (1300, 392)]},
        "solid": {"south-asia": [(1440, 270), (1462, 290), (1470, 335), (1462, 375), (1430, 400), (1410, 380), (1430, 320)]},
        "seeds": {
            "europe": [[(750, 220), (850, 200), (1000, 220), (1080, 250)]],
            "scandinavia": [[(900, 130), (1000, 100), (1120, 110)], [(1000, 100), (1040, 50)]],
            "north-asia": [[(1180, 180), (1250, 130), (1400, 100), (1430, 130)]],
            "south-asia": [[(1300, 260), (1400, 320), (1420, 380)]],
            "arabia": [[(1200, 300), (1150, 400), (1280, 400), (1230, 460)]],
            "west-africa": [[(880, 420), (1000, 400), (850, 490)]],
            "east-africa": [[(1050, 480), (1000, 580), (1100, 600), (1180, 500)]],
        },
    },
}

PRIMEVAL = {
    "height": 750,
    "seam": (746, 755),
    "ice": True,
    "same": {"mezcalera-east": "mezcalera-ocean", "iapetus-east": "iapetus-ocean"},
    "printed": [(657, 685, 30), (360, 697, 30), (70, 30, 40), (120, 705, 95), (1375, 35, 100), (1430, 710, 45)],
    "seeds": {
        "paratethys-ocean": [[(300, 15), (480, 40), (570, 70), (560, 150), (470, 190)], [(560, 150), (700, 210)]],
        "mezcalera-ocean": [[(100, 30), (20, 200), (60, 300), (300, 280), (450, 320), (560, 250)], [(20, 200), (90, 100)], [(300, 280), (170, 335)], [(450, 320), (530, 300)]],
        "mezcalera-east": [[(1470, 15), (1490, 150), (1495, 330)]],
        "iapetus-ocean": [[(30, 430), (10, 600), (40, 700), (200, 700), (400, 690), (510, 715)], [(60, 500), (30, 430)]],
        "iapetus-east": [[(1480, 385), (1465, 450), (1450, 600), (1485, 735)], [(1480, 385), (1496, 420), (1496, 730)]],
        "tethys-ocean": [[(500, 500), (620, 450), (700, 550), (560, 640)], [(700, 550), (740, 600)]],
        "mirovia-ocean": [[(1100, 60), (1250, 100), (1150, 160), (1360, 150), (1420, 120), (1450, 250), (1440, 330)], [(1360, 150), (1400, 300)], [(1150, 160), (1040, 200), (1060, 300)]],
        "upper-mu": [[(1150, 280), (1300, 270)]],
        "rheic-ocean": [[(1075, 550), (1050, 650), (1250, 650), (1375, 550), (1375, 425), (1340, 400)], [(1250, 650), (1400, 730)]],
        "antarctica": [[(540, 730), (650, 680), (800, 700), (950, 735)], [(650, 680), (652, 590)]],
    },
}
PRIMEVAL_LEFT = {
    3: {
        "printed": [(318, 172, 30), (267, 268, 24), (112, 597, 24), (690, 435, 24)],
        "solid": {"hyperborea": [(212, 262), (240, 296), (272, 306), (306, 268)]},
        "seeds": {
            "hyperborea": [[(240, 55), (330, 90), (230, 130), (130, 150), (120, 200)], [(420, 120), (330, 190), (240, 260)]],
            "lemuria": [[(70, 645), (110, 610), (200, 590), (290, 640)], [(110, 490), (180, 440), (240, 500)], [(330, 480), (400, 560), (490, 540), (310, 640)]],
        },
    },
    5: {
        "printed": [(232, 170, 30), (257, 50, 24), (263, 285, 24), (110, 490, 24), (278, 630, 24), (475, 545, 24), (655, 420, 24)],
        "solid": {"thule": [(212, 262), (240, 296), (272, 306), (306, 268)]},
        "seeds": {
            "hyperborea": [[(240, 55), (330, 90), (230, 130), (130, 150), (120, 200)]],
            "thule": [[(420, 120), (330, 190), (240, 260)]],
            "south-lemuria": [[(70, 645), (110, 610), (200, 590), (290, 640)]],
            "shasta": [[(110, 490), (180, 440), (240, 500)]],
            "east-lemuria": [[(330, 480), (400, 560), (490, 540), (310, 640)]],
            "west-atlantis": [[(600, 330), (680, 320), (640, 400)]],
        },
    },
}
PRIMEVAL_RIGHT = {
    3: {
        "printed": [(865, 78, 30), (1232, 305, 30), (882, 540, 30), (1205, 415, 30), (955, 215, 22), (765, 475, 24), (1270, 382, 22), (1225, 530, 22)],
        "seeds": {
            "poseidonis": [[(960, 20), (900, 100), (800, 170), (950, 230)], [(650, 30), (780, 60), (860, 20)], [(650, 30), (700, 110)]],
            "mirovia-ocean": [[(1000, 240), (960, 380), (1000, 450), (1040, 430)]],
            "terra-australis": [[(960, 560), (900, 640), (1010, 620)], [(830, 480), (800, 580), (860, 620)]],
            "lower-mu": [[(1180, 500), (1300, 480), (1370, 500)], [(1090, 440), (1100, 380), (1200, 430), (1260, 420)]],
        },
    },
    5: {
        "printed": [(907, 118, 30), (1238, 310, 30), (950, 595, 30), (1283, 487, 30), (900, 325, 24), (770, 473, 24), (950, 530, 24), (955, 215, 22), (1268, 382, 22), (1097, 447, 22), (1188, 513, 22)],
        "seeds": {
            "lomar": [[(650, 30), (780, 60), (860, 20)], [(650, 30), (700, 110)]],
            "poseidonis": [[(960, 20), (900, 100), (800, 170), (950, 230)]],
            "ocean-trench": [[(1000, 240), (960, 380), (1000, 450), (1040, 430)]],
            "rutas": [[(830, 480), (800, 580), (860, 620)]],
            "terra-australis": [[(960, 560), (900, 640), (1010, 620)]],
            "central-mu": [[(1090, 440), (1100, 380), (1200, 430), (1260, 420)]],
            "lower-mu": [[(1180, 500), (1300, 480), (1370, 500)]],
        },
    },
}
# Atlantis crosses the seam: it is one area unless the left half is a five-player one,
# which cuts West Atlantis from it, wherever its own coast runs on the board.
ATLANTIS = {
    3: {"atlantis": [[(600, 330), (680, 320), (640, 400)], [(720, 420), (830, 330), (800, 240), (930, 260)]]},
    5: {"atlantis": [[(720, 420), (830, 330), (800, 240), (930, 260)]]},
}


def board(common, left, right, **more):
    """A map from the seas every board shares and its two halves."""
    out = {**common, **more}
    out["printed"] = common["printed"] + left["printed"] + right["printed"]
    out["solid"] = {**common.get("solid", {}), **left.get("solid", {}), **right.get("solid", {})}
    seeds = {k: list(v) for k, v in common["seeds"].items()}
    for half in (left, right):
        for aid, strokes in half["seeds"].items():
            seeds[aid] = seeds.get(aid, []) + strokes
    out["seeds"] = seeds
    glyphs = {}
    for half in (left, right):
        for kind, spots in half.get("glyphs", {}).items():
            glyphs[kind] = glyphs.get(kind, []) + spots
    if glyphs:
        out["glyphs"] = glyphs
    return out


def earth(left, right, layout, art, **more):
    return board(EARTH, EARTH_LEFT[left], EARTH_RIGHT[right], layout=layout, art=f"guide/maps/{art}.webp", **more)


def primeval(left, right, layout, art, **more):
    m = board(PRIMEVAL, PRIMEVAL_LEFT[left], PRIMEVAL_RIGHT[right], layout=layout, art=f"guide/maps/{art}.webp", **more)
    m["seeds"].update(ATLANTIS[left])
    return m


MAPS = {
    "earth-3p": earth(3, 3, "cthulhu-wars-3p", "earth-3p"),
    "earth-3-5": earth(
        3, 5, "cthulhu-wars-3-5", "earth-3-5",
        # The glyphs that stand on the plainest ground, which trace cleanest.
        shapes={"chevron": [2], "thorns": [2, 3], "dragon": [1]},
    ),
    "earth-5-3": earth(5, 3, "cthulhu-wars", "earth-5-3", base=True),
    "earth-5p": earth(5, 5, "cthulhu-wars-5p", "earth-5p"),
    "primeval-3p": primeval(3, 3, "cthulhu-wars-primeval-3p", "primeval-3p", seam=None),
    "primeval-3-5": primeval(3, 5, "cthulhu-wars-primeval-3-5", "primeval-3-5", seam=(749, 755)),
    "primeval-5-3": primeval(5, 3, "cthulhu-wars-primeval-5-3", "primeval-5-3", seam=(745, 751)),
    "primeval-5p": primeval(5, 5, "cthulhu-wars-primeval-5p", "primeval-5p", ice_shape=True),
}

# The map being drawn, set by choose().
H = 0
M = {}


def choose(name):
    global H, M
    M = MAPS[name]
    H = M["height"]


def layout_path():
    """The base layout's board has a file of its own, and its params are in the main one."""
    if M.get("base"):
        return os.path.join(GAME, "definition", "board.yaml")
    return os.path.join(GAME, "definition", "variants", M["layout"] + ".yaml")


def load(path):
    im = Image.open(path).convert("RGB")
    rgb = np.asarray(im.resize((W, H), Image.LANCZOS), dtype=np.float32) / 255
    return close_seam(rgb, *M["seam"]) if M.get("seam") else rgb


def close_seam(rgb, x0, x1):
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
        if area > 700 and 33 < w < 41 and 47 < h < 56:
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
    ids = list(M["seeds"])
    marks = np.zeros((H, W), dtype=np.int32)
    for n, aid in enumerate(ids, 1):
        for stroke in M["seeds"][aid]:
            for (x0, y0), (x1, y1) in zip(stroke, stroke[1:]):
                rr, cc = draw.line(y0, x0, y1, x1)
                ok = (rr >= 0) & (rr < H) & (cc >= 0) & (cc < W)
                marks[rr[ok], cc[ok]] = n
    for aid, poly in M["solid"].items():
        rr, cc = draw.polygon([y for _, y in poly], [x for x, _ in poly], (H, W))
        marks[rr, cc] = ids.index(aid) + 1
    # A glyph printed over a coast would bite into it: it belongs whole to the area
    # most of it stands in.
    first = flood(relief, marks)
    glyphs = []
    blobs = ice_blobs(rgb) if M.get("ice") else []
    for box, mask in blobs:
        owner = np.bincount(first[box][mask]).argmax()
        grown = ndi.binary_dilation(mask, iterations=3)
        marks[box][grown[:]] = owner
        cy = (box[0].start + box[0].stop) / 2
        cx = (box[1].start + box[1].stop) / 2
        glyphs.append((M["same"].get(ids[owner - 1], ids[owner - 1]), cx, cy, box[1].stop - box[1].start, box[0].stop - box[0].start))
    return flood(relief, marks), ids, glyphs, blobs


def merged(labels, ids):
    """Labels with the pieces the map's edge cuts apart made one area."""
    names = sorted({M["same"].get(i, i) for i in ids})
    table = np.zeros(len(ids) + 1, dtype=np.int32)
    for n, aid in enumerate(ids, 1):
        table[n] = names.index(M["same"].get(aid, aid)) + 1
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
        pts = [(round(float(np.clip(x * CANVAS, 0, 1000)), 1), round(float(np.clip(y * CANVAS, 0, round(H * CANVAS))), 1)) for y, x in c]
        out.append(pts)
    return sorted(out, key=lambda p: -Polygon(p).buffer(0).area)


def anchor(area, k):
    """The roomiest point of the area's largest piece."""
    parts, n = ndi.label(area == k)
    big = np.argmax(ndi.sum_labels(np.ones_like(parts), parts, range(1, n + 1))) + 1
    room = ndi.distance_transform_edt(parts == big)
    y, x = np.unravel_index(room.argmax(), room.shape)
    return round(x * CANVAS, 1), round(y * CANVAS, 1)


def read_tags():
    with open(layout_path()) as f:
        return dict(re.findall(r"- id: (\S+)\n\s+name: .*\n\s+tags: \[(.*)\]", f.read()))


def write_layout(area, names, glyphs, fills, printed):
    """The layout's areas, borders and what is printed; printed gives each area's Desecrate
    glyph, and the Glacier glyphs say which areas are tagged glacier."""
    path = layout_path()
    with open(path) as f:
        old = f.read()
    tags = read_tags()
    label = dict(re.findall(r"- id: (\S+)\n\s+name: (.*)\n", old))
    pad = "" if M.get("base") else "    "
    head = re.sub(r"canvas: \{width: 1000, height: \d+\}", f"canvas: {{width: 1000, height: {round(H * CANVAS)}}}", old[: old.index(pad + "positions:")])
    frozen = {owner for owner, _, _, _, _ in glyphs}
    lines = [pad + "positions:"]
    for n, aid in enumerate(names, 1):
        x, y = anchor(area, n)
        tag = tags[aid]
        if printed:
            tag = ", ".join(t for t in tag.split(", ") if not t.startswith("glyph-"))
            if aid in printed:
                tag += f", glyph-{printed[aid][0]}"
        if M.get("ice"):
            tag = ", ".join(t for t in tag.split(", ") if t != "glacier")
            if aid in frozen:
                tag += ", glacier"
        lines += [f"{pad}  - id: {aid}", f"{pad}    name: {label[aid]}", f"{pad}    tags: [{tag}]", f"{pad}    anchor: [{x}, {y}]", f"{pad}    polygons:"]
        for poly in outlines(area, n):
            for i, (px, py) in enumerate(poly):
                lines.append(f"{pad}      " + ("- - " if i == 0 else "  - ") + f"[{px}, {py}]")
    lines.append(pad + "edges:")
    edges = set(adjacency(area, names))
    for a, b in sorted(edges):
        lines.append(f"{pad}  - {{from: {a}, to: {b}}}")
    art = [f"art: {M['art']}"]
    if glyphs:
        marks = ", ".join(f"[{round(x * CANVAS, 1)}, {round(y * CANVAS, 1)}]" for _, x, y, _, _ in sorted(glyphs))
        art.append(f"ice_glyphs: [{marks}]")
    if printed:
        marks = ", ".join(f"{a}: [{x}, {y}]" for a, (_, x, y) in sorted(printed.items()))
        art.append(f"glyph_marks: {{{marks}}}")
    elif M.get("ice"):
        # A variant is a merge patch: without this it would stand glyphs where the base layout prints them.
        art.append("glyph_marks: null")
    fill = ", ".join(f"{a}: '{fills[a]}'" for a in names)
    art.append(f"area_fill: {{{fill}}}")
    drawn = re.compile(r"\s+(art|ice_glyphs|area_fill|glyph_marks):")
    if M.get("base"):
        with open(path, "w") as f:
            f.write(head + "\n".join(lines) + "\n")
        main = os.path.join(GAME, "definition", "cthulhuwars.yaml")
        with open(main) as f:
            doc = f.read()
        at = doc.index("params:\n")
        end = doc.index("board: !include")
        kept = [l for l in doc[at:end].splitlines() if not drawn.match(l)]
        with open(main, "w") as f:
            f.write(doc[:at] + "\n".join(kept + ["  " + l for l in art]) + "\n" + doc[end:])
        return edges
    at = old.index("  params:")
    after = re.search(r"^  \S+:", old[at + 1:], re.M)
    params = old[at : at + 1 + after.start()] if after else old[at:]
    tail = old[at + 1 + after.start() :] if after else ""
    keep = [l for l in params.splitlines() if not drawn.match(l)] + ["    " + l for l in art]
    with open(path, "w") as f:
        f.write(head + "\n".join(lines) + "\n" + "\n".join(keep) + "\n" + tail)
    return edges


# ---- the background ------------------------------------------------------------------


def surface(rgb, glyph_boxes):
    """The pixels that show the board's own colour."""
    grey = color.rgb2gray(rgb)
    keep = np.abs(grey - ndi.gaussian_filter(grey, 7)) < 0.055
    yy, xx = np.mgrid[:H, :W]
    for x, y, r in M["printed"]:
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


def find_islands(rgb, ocean):
    """Small lands at sea that are no area: warm where the sea is cool."""
    warm = ndi.gaussian_filter(rgb[..., 0] - rgb[..., 2], 0.8) > 0.07
    yy, xx = np.mgrid[:H, :W]
    for x, y, r in M["printed"]:
        warm[(xx - x) ** 2 + (yy - y) ** 2 < r * r] = False
    parts, n = ndi.label(ndi.binary_opening(warm & ocean, iterations=1))
    sizes = ndi.sum_labels(np.ones_like(parts), parts, range(1, n + 1))
    return np.isin(parts, np.flatnonzero((sizes > 25) & (sizes < 2500)) + 1)


def bake(rgb, area, names, glyph_boxes):
    """The map's background, and each area's middle colour."""
    keep = surface(rgb, glyph_boxes)
    grey = color.rgb2gray(rgb)
    tags = read_tags()
    sea = np.isin(area, [k for k, aid in enumerate(names, 1) if "ocean" in tags[aid].split(", ")])
    islands = find_islands(rgb, sea) if M.get("islands") else None
    if islands is not None:
        keep &= ~ndi.binary_dilation(islands, iterations=3)
    rng = np.random.default_rng(1928)
    border = segmentation.find_boundaries(area, mode="inner")
    field = np.zeros_like(rgb)
    fills = {}
    amp = np.zeros((H, W), dtype=np.float32)
    lines = np.zeros((H, W), dtype=np.float32)
    ocean = np.zeros((H, W), dtype=bool)
    for k, aid in enumerate(names, 1):
        region = area == k
        ocean[region] = "ocean" in tags[aid].split(", ")
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
    if islands is not None:
        rim = ndi.binary_dilation(islands, iterations=1) & ~islands
        tone = np.array([0.42, 0.2, 0.11]) * (0.85 + 0.25 * mottle[..., None])
        out = np.where(islands[..., None], tone, out)
        out = np.where(rim[..., None], out * 0.3, out)
    # The ink: heavier where two seas meet, and never quite even.
    sea_sea = ocean & border & ~ndi.binary_dilation(land, iterations=2)
    width = np.where(ndi.binary_dilation(sea_sea, iterations=2), 2.5, 1.45) * (1 + 0.28 * noise(rng, [(5, 1.0)]))
    ink = np.exp(-((dist / np.maximum(width, 0.6)) ** 2))
    out = out * (1 - 0.9 * ink)[..., None] + np.array([0.07, 0.055, 0.04]) * (0.9 * ink)[..., None]
    pic = Image.fromarray((np.clip(out, 0, 1) * 255).astype(np.uint8))
    path = os.path.join(GAME, M["art"])
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
    with open(os.path.join(JS, "cw-ice.js"), "w") as f:
        f.write("// Written by scripts/cwmapart.py: the Glacier glyph of the Primeval Map, centred on\n")
        f.write("// the origin in the units of the map, as stacked shapes from its palest tone to its darkest.\n")
        f.write("export const ICE_GLYPH = [\n")
        for fill, d in out:
            f.write(f'\t{{ fill: "{fill}", d: "{d}" }},\n')
        f.write("];\n")
    return outline.sum() / outline.size


# ---- the Desecrate glyphs ------------------------------------------------------------

GOLD = "#ffcf45"
AMBER = "#f08f1e"
INK = "#1d1208"


def gold(crop):
    """The glyph in a crop: gold, and clear of the crop's edge, where lettering and coasts run."""
    mask = (crop[..., 0] > 0.72) & (crop[..., 1] > 0.5) & (crop[..., 2] < 0.42) & (crop[..., 0] - crop[..., 2] > 0.42)
    mask = segmentation.clear_border(mask)
    parts, n = ndi.label(mask)
    if not n:
        return mask
    sizes = ndi.sum_labels(mask, parts, range(1, n + 1))
    return np.isin(parts, np.flatnonzero(sizes > 12) + 1)


def trace_glyphs(path, area, names):
    """Where each printed glyph stands and whose it is; writes the glyphs' shapes, from an
    outline of ink, a body of amber and the brighter part in gold."""
    im = np.asarray(Image.open(path).convert("RGB"), dtype=np.float32) / 255
    scale = im.shape[1] / W
    box = int(30 * scale)
    k = CANVAS / scale
    printed = {}
    shapes = {}
    for kind, spots in M["glyphs"].items():
        crops = []
        for i, (x, y) in enumerate(spots):
            cx, cy = int(x * scale), int(y * scale)
            crop = im[cy - box : cy + box, cx - box : cx + box]
            mask = gold(crop)
            # On ground much the same colour the glyph cannot be picked out: there the
            # strokes above say where it is.
            py, px = ndi.center_of_mass(mask) if 150 < mask.sum() < 3000 else (box, box)
            ax, ay = cx - box + px, cy - box + py
            owner = names[area[int(ay / scale), int(ax / scale)] - 1]
            printed[owner] = (kind, round(ax * k, 1), round(ay * k, 1))
            if i in M.get("shapes", {}).get(kind, []):
                shift = (box - py, box - px, 0)
                crops.append(ndi.shift(crop, shift, order=1, mode="nearest"))
        if "shapes" not in M:
            continue
        # Traced four times larger, where a thin blade is still a shape. Green tells gold
        # from dark ground, and the level is halfway from that ground to the glyph's top.
        up = 4
        green = ndi.zoom(np.median(np.stack(crops), axis=0)[..., 1], up, order=3)
        ground, top = float(np.median(green)), float(np.percentile(green, 99.5))
        body = segmentation.clear_border(green > ground + 0.45 * (top - ground))
        parts, n = ndi.label(body)
        sizes = ndi.sum_labels(body, parts, range(1, n + 1))
        body = np.isin(parts, np.flatnonzero(sizes > 120) + 1)
        outline = ndi.binary_dilation(body, iterations=6)
        bright = (green > ground + 0.8 * (top - ground)) & body
        unit = k / up
        centre = box * up
        layers = []
        for fill, mask in ((INK, outline), (AMBER, body), (GOLD, bright)):
            d = []
            for c in measure.find_contours(np.pad(mask, 1).astype(float), 0.5):
                if len(c) < 24:
                    continue
                c = measure.approximate_polygon(ndi.gaussian_filter1d(c[:-1] - 1, 2.2, axis=0, mode="wrap"), 0.9)
                d.append("M" + " ".join(f"{(px - centre) * unit:.2f} {(py - centre) * unit:.2f}" for py, px in c) + "Z")
            layers.append((fill, "".join(d)))
        shapes[kind] = layers
    if "shapes" not in M:
        return printed
    shapes = {kind: shapes[kind] for kind in M["shapes"]}
    with open(os.path.join(JS, "cw-map-glyphs.js"), "w") as f:
        f.write("// Written by scripts/cwmapart.py: the Desecrate glyphs as the board prints them, centred on\n")
        f.write("// the origin in the units of the map, as stacked shapes.\n")
        f.write("export const MAP_GLYPHS = {\n")
        for kind, layers in shapes.items():
            f.write(f"\t{kind}: [\n")
            for fill, d in layers:
                f.write(f'\t\t{{ fill: "{fill}", d: "{d}" }},\n')
            f.write("\t],\n")
        f.write("};\n")
    with open(os.path.join(HERE, "..", "internal", "session", "map_glyphs.go"), "w") as f:
        f.write("// Code generated by scripts/cwmapart.py. DO NOT EDIT.\n\npackage session\n\n")
        f.write("// glyphShapes are the Desecrate glyphs as the board prints them: stacked {fill, path}.\n")
        f.write("var glyphShapes = map[string][][2]string{\n")
        for kind, layers in shapes.items():
            f.write(f'\t"{kind}": {{\n')
            for fill, d in layers:
                f.write(f'\t\t{{"{fill}", "{d}"}},\n')
            f.write("\t},\n")
        f.write("}\n")
    return printed


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--map", required=True, choices=sorted(MAPS))
    ap.add_argument("--board", required=True)
    ap.add_argument("--check", help="draw the borders over the board, to this file")
    args = ap.parse_args()
    choose(args.map)
    rgb = load(args.board)
    labels, ids, glyphs, blobs = trace(rgb)
    area, names = merged(labels, ids)
    if args.check:
        check(rgb, area, args.check)
    fills, size, _ = bake(rgb, area, names, blobs)
    printed = trace_glyphs(args.board, area, names) if "glyphs" in M else None
    edges = write_layout(area, names, glyphs, fills, printed)
    if M.get("ice_shape"):
        trace_ice(args.board, glyphs)
    print(f"{args.map}: {len(names)} areas, {len(edges)} edges, {len(glyphs)} Glacier glyphs, {len(printed or {})} other glyphs, background {size // 1024} KB")


if __name__ == "__main__":
    main()
