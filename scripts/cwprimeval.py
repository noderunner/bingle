#!/usr/bin/env python3
"""Writes the three-player Primeval map layout of Cthulhu Wars into the game's definition.
The five-player layout is traced from a scan by scripts/cwmapart.py.

    python3 scripts/cwprimeval.py [--preview FILE]

The areas below are outlines drawn by eye over pictures of the printed boards
(the Primeval Map page of the Cthulhu Wars wiki), in the pixels of a 1468 x 734
picture of the left and right boards side by side. They are painted in order,
each over the last, so that land sits on top of the seas and no gaps are left;
the borders are then read back from the painting, which is also where the
adjacency comes from. An area can have several outlines: Mezcalera and Iapetus
continue round the edges of the board.

Needs numpy, scikit-image, shapely and Pillow.
"""
import os
import sys

import numpy as np
from PIL import Image, ImageDraw
from shapely.geometry import Polygon
from skimage import measure

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, "..", "internal", "games", "cthulhuwars", "definition", "variants")
W, H = 1468, 734
CANVAS = (1000, 500)

SEAS = {
    "mezcalera-ocean": ("Mezcalera Ocean", [
        [(0, 0), (240, 0), (240, 110), (400, 215), (560, 205), (600, 245), (540, 355), (430, 435), (330, 380), (250, 375), (170, 320), (130, 390), (0, 340)],
        [(1420, 0), (1468, 0), (1468, 345), (1440, 360), (1420, 300), (1440, 150)],
    ]),
    "paratethys-ocean": ("Paratethys Ocean", [[(210, 0), (630, 0), (660, 50), (735, 130), (735, 195), (600, 250), (560, 205), (400, 215), (230, 100)]]),
    "mirovia-ocean": ("Mirovia Ocean", [[(1000, 0), (1425, 0), (1445, 150), (1425, 300), (1445, 362), (1300, 345), (1200, 420), (1040, 400), (1030, 340), (1000, 230)]]),
    "rheic-ocean": ("Rheic Ocean", [[(1040, 400), (1100, 490), (1180, 530), (1290, 545), (1355, 490), (1265, 385), (1300, 345), (1390, 420), (1445, 362), (1400, 500), (1450, 640), (1468, 734), (1020, 734), (880, 690), (960, 680), (1020, 640), (965, 520), (980, 470)]]),
    "iapetus-ocean": ("Iapetus Ocean", [
        [(0, 340), (130, 390), (80, 490), (65, 625), (220, 625), (305, 650), (390, 595), (415, 590), (470, 640), (555, 675), (500, 734), (0, 734)],
        [(1468, 345), (1445, 362), (1400, 500), (1450, 640), (1468, 734)],
    ]),
    "tethys-ocean": ("Tethys Ocean", [[(530, 355), (590, 400), (650, 435), (735, 470), (755, 530), (730, 590), (640, 570), (555, 675), (470, 640), (415, 590), (430, 435)]]),
}

# The legend's sea colours, chosen to tell neighbouring oceans apart.
SEA_TINT = {"mezcalera-ocean": "atlantic", "paratethys-ocean": "indian", "mirovia-ocean": "pacific", "rheic-ocean": "atlantic", "iapetus-ocean": "pacific", "tethys-ocean": "indian", "ocean-trench": "indian"}

# The lands of the five-player boards, and the oceans' split.
LANDS5 = {
    "hyperborea": ("Hyperborea", [(103, 100), (170, 88), (220, 125), (190, 70), (210, 30), (290, 30), (372, 68), (305, 118), (255, 180), (185, 232), (125, 195), (105, 150)]),
    "thule": ("Thule", [(372, 68), (440, 100), (445, 130), (420, 190), (350, 235), (280, 285), (255, 298), (215, 260), (185, 232), (255, 180), (305, 118)]),
    "lomar": ("Lomar", [(618, 0), (880, 0), (800, 70), (755, 150), (735, 130), (690, 85), (640, 30)]),
    "poseidonis": ("Poseidonis", [(880, 0), (1010, 0), (990, 60), (975, 150), (945, 255), (880, 210), (770, 180), (752, 155), (800, 70)]),
    "west-atlantis": ("West Atlantis", [(700, 210), (720, 330), (690, 400), (650, 435), (590, 400), (535, 345), (545, 290), (600, 250), (680, 220)]),
    "atlantis": ("Atlantis", [(700, 210), (735, 190), (752, 155), (770, 180), (880, 210), (945, 255), (880, 345), (800, 400), (730, 440), (650, 435), (690, 400), (720, 330)]),
    "shasta": ("Shasta", [(185, 335), (215, 390), (280, 440), (285, 530), (220, 535), (130, 510), (80, 490), (115, 430)]),
    "east-lemuria": ("East Lemuria", [(280, 440), (350, 375), (400, 400), (440, 440), (500, 500), (510, 550), (440, 590), (390, 595), (305, 650), (285, 530)]),
    "south-lemuria": ("South Lemuria", [(130, 510), (220, 535), (285, 530), (305, 650), (220, 625), (100, 635), (65, 625), (75, 570)]),
    "antarctica": ("Antarctica", [(500, 734), (555, 675), (640, 570), (735, 600), (810, 680), (880, 690), (1020, 734)]),
    "rutas": ("Rutas", [(735, 470), (800, 400), (880, 390), (920, 440), (915, 520), (860, 590), (830, 650), (770, 620), (735, 590), (755, 530)]),
    "terra-australis": ("Terra Australis", [(920, 470), (960, 530), (1020, 640), (960, 680), (860, 680), (830, 650), (860, 590), (915, 520)]),
    "ocean-trench": ("Ocean Trench", [(975, 150), (1000, 230), (1030, 340), (1040, 400), (980, 470), (965, 520), (920, 470), (920, 440), (870, 370), (945, 255)]),
    "upper-mu": ("Upper Mu", [(1060, 295), (1130, 225), (1240, 215), (1340, 215), (1350, 280), (1320, 320), (1265, 365), (1230, 392), (1155, 345), (1060, 320)]),
    "central-mu": ("Central Mu", [(1030, 340), (1060, 320), (1155, 345), (1230, 392), (1265, 405), (1200, 450), (1105, 490), (1060, 440), (1040, 400)]),
    "lower-mu": ("Lower Mu", [(1105, 490), (1200, 450), (1265, 405), (1355, 490), (1290, 545), (1180, 530)]),
}

# The three-player boards join what the five-player ones split.
LANDS3 = {
    "hyperborea": ("Hyperborea", [(103, 100), (170, 88), (220, 125), (190, 70), (210, 30), (290, 30), (372, 68), (440, 100), (445, 130), (420, 190), (350, 235), (280, 285), (255, 298), (215, 260), (185, 232), (125, 195), (105, 150)]),
    "poseidonis": ("Poseidonis", [(618, 0), (1010, 0), (990, 60), (975, 150), (945, 255), (880, 210), (770, 180), (752, 155), (735, 130), (690, 85), (640, 30)]),
    "atlantis": ("Atlantis", [(700, 210), (735, 190), (752, 155), (770, 180), (880, 210), (945, 255), (880, 345), (800, 400), (730, 440), (650, 435), (590, 400), (535, 345), (545, 290), (600, 250), (680, 220)]),
    "lemuria": ("Lemuria", [(185, 335), (215, 390), (280, 440), (350, 375), (400, 400), (440, 440), (500, 500), (510, 550), (440, 590), (390, 595), (305, 650), (220, 625), (100, 635), (65, 625), (75, 570), (80, 490), (115, 430)]),
    "antarctica": LANDS5["antarctica"],
    "terra-australis": ("Terra Australis", [(735, 470), (800, 400), (880, 390), (920, 440), (960, 530), (1020, 640), (960, 680), (860, 680), (830, 650), (770, 620), (735, 590), (755, 530)]),
    "upper-mu": LANDS5["upper-mu"],
    "lower-mu": ("Lower Mu", [(1030, 340), (1060, 320), (1155, 345), (1230, 392), (1265, 405), (1355, 490), (1290, 545), (1180, 530), (1105, 490), (1060, 440), (1040, 400)]),
}
TRENCH3 = [(975, 150), (1000, 230), (1030, 340), (1040, 400), (980, 470), (965, 520), (920, 440), (870, 370), (945, 255)]

LAYOUTS = {
    "cthulhu-wars-primeval-3p": {
        "name": "Cthulhu Wars (3 players, Primeval map)",
        "lands": LANDS3,
        "seas": SEAS,
        "extra_sea": ("mirovia-ocean", TRENCH3),
        "tags": {
            "hyperborea": ["c-eu", "glyph-chevron", "glacier"], "poseidonis": ["c-af", "glyph-thorns"],
            "atlantis": ["c-me", "glyph-thorns"], "lemuria": ["c-sa", "glyph-chevron"], "antarctica": ["c-ice", "glacier"],
            "terra-australis": ["c-au", "glyph-thorns", "glacier"], "upper-mu": ["c-na", "glyph-dragon", "glacier"], "lower-mu": ["c-as", "glyph-dragon", "glacier"],
            "paratethys-ocean": ["glacier"], "mirovia-ocean": ["glacier"], "iapetus-ocean": ["glacier"],
        },
        "starts": {"gc": "iapetus-ocean", "cc": "upper-mu", "bg": "hyperborea", "ys": "terra-australis"},
        "sigils": {"ww": ["antarctica", "poseidonis"], "sl": ["lower-mu"]},
        "seats": [(1, "green", "Great Cthulhu"), (2, "blue", "Crawling Chaos"), (3, "red", "Black Goat")],
        "order": [1, 2, 3],
    },
}


def paint(layout):
    """Paints the areas in order and returns the label image and the area ids."""
    img = Image.new("I", (W, H), 0)
    draw = ImageDraw.Draw(img)
    ids = []

    def fill(aid, poly):
        if aid not in ids:
            ids.append(aid)
        draw.polygon(poly, fill=ids.index(aid) + 1)

    for aid, (_, polys) in layout["seas"].items():
        for poly in polys:
            fill(aid, poly)
    if "extra_sea" in layout:
        fill(*layout["extra_sea"])
    for aid, (_, poly) in layout["lands"].items():
        fill(aid, poly)
    return np.array(img), ids


def fill_gaps(labels):
    """Gives every unpainted pixel the label of the nearest painted one."""
    from scipy import ndimage as ndi

    idx = ndi.distance_transform_edt(labels == 0, return_distances=False, return_indices=True)
    return labels[tuple(idx)]


def outlines(labels, n):
    """The polygons of area n, in canvas units."""
    mask = np.pad(labels == n, 1)
    sx, sy = CANVAS[0] / W, CANVAS[1] / H
    out = []
    for c in measure.find_contours(mask.astype(float), 0.5):
        p = Polygon([(x - 1, y - 1) for y, x in c])
        if p.area < 300:
            continue
        p = p.simplify(5)
        out.append([(round(x * sx, 1), round(y * sy, 1)) for x, y in p.exterior.coords[:-1]])
    return out


def adjacency(labels, ids, min_edge=6):
    pairs = {}
    for a, b in [(labels[:-1, :], labels[1:, :]), (labels[:, :-1], labels[:, 1:])]:
        diff = a != b
        for x, y in zip(a[diff], b[diff]):
            k = tuple(sorted((int(x), int(y))))
            pairs[k] = pairs.get(k, 0) + 1
    return sorted(tuple(sorted((ids[a - 1], ids[b - 1]))) for (a, b), n in pairs.items() if n >= min_edge)


def write(lid, layout):
    labels, ids = paint(layout)
    labels = fill_gaps(labels)
    names = {k: v[0] for k, v in layout["seas"].items()}
    names.update({k: v[0] for k, v in layout["lands"].items()})
    lines = ["# Part of cthulhuwars.yaml, included by name. Written by scripts/cwprimeval.py.", f"id: {lid}", f"name: {layout['name']}", "patch:", "  board:", "    style: areas", f"    canvas: {{width: {CANVAS[0]}, height: {CANVAS[1]}}}", "    positions:"]
    for aid in sorted(ids):
        polys = outlines(labels, ids.index(aid) + 1)
        big = max(polys, key=lambda p: Polygon(p).area)
        pt = Polygon(big).representative_point()
        ocean = "ocean" in aid
        tags = (["ocean", SEA_TINT.get(aid, "pacific")] if ocean else ["land"]) + layout["tags"].get(aid, [])
        lines += [f"      - id: {aid}", f"        name: {names[aid]}", f"        tags: [{', '.join(tags)}]", f"        anchor: [{round(pt.x, 1)}, {round(pt.y, 1)}]", "        polygons:"]
        for poly in polys:
            for i, (x, y) in enumerate(poly):
                lines.append(("          - - " if i == 0 else "            - ") + f"[{x}, {y}]")
    lines.append("    edges:")
    for a, b in adjacency(labels, ids):
        lines.append(f"      - {{from: {a}, to: {b}}}")
    starts = ", ".join(f"{k}: {v}" for k, v in layout["starts"].items())
    lines += ["  params:", "    glaciers: true", f"    starts: {{{starts}}}", "    sigils:"]
    for k, v in layout["sigils"].items():
        lines.append(f"      {k}: [{', '.join(v)}]")
    lines.append("  seats:")
    for s, c, l in layout["seats"]:
        lines.append(f"    - {{seat: {s}, color: {c}, label: {l}}}")
    lines += ["  turn_order:", "    type: round-robin", f"    seats: [{', '.join(map(str, layout['order']))}]"]
    with open(os.path.join(OUT, lid + ".yaml"), "w") as f:
        f.write("\n".join(lines) + "\n")
    print(lid, len(ids), "areas")
    return labels, ids


def main():
    out = {}
    for lid, layout in LAYOUTS.items():
        out[lid] = write(lid, layout)
    if "--preview" in sys.argv:
        base = sys.argv[sys.argv.index("--preview") + 1]
        for lid, (labels, ids) in out.items():
            rng = np.random.default_rng(3)
            pal = rng.integers(60, 255, (len(ids) + 1, 3)).astype(np.uint8)
            Image.fromarray(pal[labels]).save(base + "-" + lid + ".png")


if __name__ == "__main__":
    main()
