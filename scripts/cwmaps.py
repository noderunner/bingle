#!/usr/bin/env python3
"""Builds the Cthulhu Wars map layouts of the game's definition from reference images.

    python3 scripts/cwmaps.py temp            # rewrites the board parts of the definition
    python3 scripts/cwmaps.py temp --preview /tmp/maps   # also writes PNG previews

The images (one per layout, "closeness centrality" diagrams of the printed
boards, from the boardgameanalysis.com article on the maps) are only an
input: they are coloured, hand-outlined pictures of the areas. The script

  1. finds the areas as regions bounded by the outlines or by a change of
     colour; "seeds" (pixel positions) below say which area each region is,
     and which pieces are the same area (the Pacific continues round both edges
     of the board, so one area can be several regions);
  2. smooths the coastlines, assigns the outline pixels to the nearest area
     and vectorises the areas with *shared* borders, simplified once, so
     neighbours always fit together;
  3. reads adjacency off the shared borders (which reproduces the degrees the
     article lists, e.g. Australia 2, Europe 3, Antarctica 3, Asia 6);
  4. writes the polygons (canvas 1000 x 490) and edges of the four layouts:
     the base game is the 4-player "5-3" map, the others are variants.

The expansion cults' sigil areas (where the Windwalker and the Sleeper begin)
are part of each layout (``sigils``), and the five-player layout seats the
Windwalker fifth; the script writes both, so re-running it keeps them.

    python3 scripts/cwmaps.py --guide        # only rewrite the Map page's tables

The definition is YAML split across files (definition/cthulhuwars.yaml and the
files it includes); the script reads it, and writes the base board, the map
variants and the main file back, through `go run . dsl convert`.

Needs Go, numpy, scipy, shapely and Pillow.
"""
import json
import os
import re
import subprocess
import sys

import numpy as np
from PIL import Image
from scipy import ndimage as ndi
from shapely.algorithms.polylabel import polylabel
from shapely.geometry import LineString, Polygon
from shapely.ops import polygonize, unary_union

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.join(HERE, "..")
GAME_DIR = os.path.join(REPO, "internal", "games", "cthulhuwars")
DEF_DIR = os.path.join(GAME_DIR, "definition")
MAIN = os.path.join(DEF_DIR, "cthulhuwars.yaml")
SCHEMA = "# yaml-language-server: $schema=../../../../docs/game-definition.schema.json\n"
PART = "# Part of cthulhuwars.yaml, included by name.\n"
# The parts of the main file that live in files of their own.
PARTS = {"board": "board.yaml", "pieces": "pieces.yaml", "options": "options.yaml"}
CANVAS = (1000, 490)
MIN_EDGE = 15          # shared border (image px) needed for two areas to be adjacent
MIN_FRAGMENT = 900     # smaller stray pieces of an area are merged into a neighbour

OCEANS = {
    "arctic-ocean": "arctic", "north-pacific": "pacific", "south-pacific": "pacific",
    "north-atlantic": "atlantic", "south-atlantic": "atlantic", "indian-ocean": "indian",
}
LAND = {  # area -> (continent tag, glyph)
    "north-america": ("c-na", "thorns"), "north-america-west": ("c-na", "thorns"), "north-america-east": ("c-na", "thorns"),
    "central-america": ("c-na", "thorns"),
    "south-america": ("c-sa", "thorns"), "south-america-west": ("c-sa", "thorns"), "south-america-east": ("c-sa", "thorns"),
    "europe": ("c-eu", "chevron"), "scandinavia": ("c-eu", "chevron"),
    "africa": ("c-af", "chevron"), "west-africa": ("c-af", "chevron"), "east-africa": ("c-af", "chevron"),
    "arabia": ("c-me", "chevron"),
    "asia": ("c-as", "dragon"), "north-asia": ("c-as", "dragon"), "south-asia": ("c-as", "dragon"),
    "australia": ("c-au", "dragon"), "new-zealand": ("c-au", "dragon"),
    "antarctica": ("c-ice", None),
}
NAMES = {
    "arctic-ocean": "Arctic Ocean", "north-pacific": "North Pacific", "south-pacific": "South Pacific",
    "north-atlantic": "North Atlantic", "south-atlantic": "South Atlantic", "indian-ocean": "Indian Ocean",
}

# Per layout: image, the rows holding the map (the title and legend are
# outside), seeds -> where on the picture each region of an area is, and
# bridges -> thin land strips (the isthmus) the outlines draw too thinly.
LAYOUTS = {
    "cthulhu-wars": dict(  # 4 players, "5-3": the base game
        file="CthulhuWars-Map-5-3-Closeness-Centrality.webp", band=(105, 855),
        name="Cthulhu Wars (4 players, 5-3 map)",
        starts={"gc": "south-pacific", "cc": "asia", "bg": "south-america-west", "ys": "europe"},
        sigils={"ww": ["arctic-ocean", "antarctica"], "sl": ["north-america-west"]},
        seeds={"africa": [[1135, 644]], "antarctica": [[937, 803], [1238, 824]], "arctic-ocean": [[117, 161], [1027, 180]],
               "asia": [[1274, 439]], "australia": [[124, 692]], "central-america": [[266, 394], [347, 484], [435, 559]],
               "europe": [[1136, 244]], "indian-ocean": [[105, 564], [1472, 767], [96, 781]], "new-zealand": [[278, 649]],
               "north-america-east": [[454, 216]], "north-america-west": [[415, 345]], "north-atlantic": [[821, 446]],
               "north-pacific": [[1508, 278], [147, 433], [377, 542]], "south-america-east": [[651, 651]],
               "south-america-west": [[537, 711]], "south-atlantic": [[825, 604]], "south-pacific": [[398, 712]]}),
    "cthulhu-wars-3-5": dict(  # 4 players, "3-5"
        file="CthulhuWars-Map-3-5-Closeness-Centrality.webp", band=(95, 820),
        name="Cthulhu Wars (4 players, 3-5 map)",
        starts={"cc": "north-asia", "bg": "south-america"},
        sigils={"sl": ["north-america"]},
        bridges=[("north-america", (426, 497), (446, 521), 8)],
        seeds={"antarctica": [[757, 762]], "arabia": [[1267, 371]], "arctic-ocean": [[124, 143], [555, 165], [1020, 167]],
               "australia": [[260, 683]], "east-africa": [[1179, 585], [1242, 629]], "europe": [[1112, 322]],
               "indian-ocean": [[1444, 729], [117, 539]], "north-america": [[491, 304], [457, 522]],
               "north-asia": [[1409, 210]], "north-atlantic": [[778, 439]],
               "north-pacific": [[1492, 259], [180, 432], [379, 516]], "scandinavia": [[1140, 208]],
               "south-america": [[537, 664]], "south-asia": [[1454, 432]], "south-atlantic": [[709, 671]],
               "south-pacific": [[403, 663]], "west-africa": [[908, 518]]}),
    "cthulhu-wars-3p": dict(  # 3 players
        file="CthulhuWars-Map-3-3-Closeness-Centrality.jpg", band=(95, 845),
        name="Cthulhu Wars (3 players)",
        starts={"cc": "asia", "bg": "south-america"},
        sigils={"sl": ["north-america"]},
        bridges=[("north-america", (392, 508), (418, 534), 8)],
        seats=3,
        seeds={"africa": [[1128, 644]], "antarctica": [[946, 791]], "arctic-ocean": [[946, 147], [430, 140]],
               "asia": [[1279, 429]], "australia": [[266, 682]], "europe": [[1140, 244], [905, 194]],
               "indian-ocean": [[83, 551], [1485, 768], [71, 769]], "north-america": [[480, 149], [309, 351], [439, 543]],
               "north-atlantic": [[805, 450]], "north-pacific": [[1531, 280], [135, 420], [363, 528]],
               "south-america": [[523, 672]], "south-atlantic": [[702, 695]], "south-pacific": [[387, 699]]}),
    "cthulhu-wars-5p": dict(  # the 5-player board (21 areas): the four core cults and the Windwalker
        file="Closeness-Centrality-5-players-map-Result.jpg", band=(120, 845),
        name="Cthulhu Wars (5 players, 21-area map)",
        starts={"cc": "north-asia", "bg": "south-america-west"},
        sigils={"sl": ["north-america-west"]},
        seats=5,
        bridges=[("central-america", (388, 511), (408, 540), 8)],
        seeds={"antarctica": [[751, 794], [1187, 818]], "arabia": [[1248, 500]], "arctic-ocean": [[800, 177], [1020, 198]],
               "australia": [[113, 682]], "central-america": [[317, 465], [434, 556]], "east-africa": [[1115, 695]],
               "europe": [[1110, 346]], "indian-ocean": [[86, 568], [86, 776], [1443, 753]], "new-zealand": [[271, 642]],
               "north-america-east": [[440, 226]], "north-america-west": [[412, 359]], "north-asia": [[1255, 270]],
               "north-atlantic": [[744, 479]], "north-pacific": [[1500, 284], [129, 434], [367, 543]],
               "scandinavia": [[1134, 233]], "south-america-east": [[599, 669]], "south-america-west": [[516, 692]],
               "south-asia": [[1451, 454]], "south-atlantic": [[694, 703]], "south-pacific": [[388, 707]],
               "west-africa": [[1035, 514]]}),
}


def display_name(area_id):
    return NAMES.get(area_id) or " ".join(w.capitalize() for w in area_id.split("-"))


# ---- segmentation ------------------------------------------------------------

def segment(path, band, edge_thr=25, dark_thr=100, min_size=120):
    im = np.array(Image.open(path).convert("RGB")).astype(int)
    h, w, _ = im.shape
    dark = im.max(axis=2) < dark_thr
    gx = np.abs(im[:, 1:, :] - im[:, :-1, :]).max(axis=2)
    gy = np.abs(im[1:, :, :] - im[:-1, :, :]).max(axis=2)
    edge = np.zeros((h, w), bool)
    edge[:, 1:] |= gx > edge_thr
    edge[:, :-1] |= gx > edge_thr
    edge[1:, :] |= gy > edge_thr
    edge[:-1, :] |= gy > edge_thr
    inband = np.zeros((h, w), bool)
    inband[band[0]:band[1], :] = True
    core = ndi.binary_erosion((~(dark | edge)) & inband, iterations=1)
    lab, n = ndi.label(core)
    sizes = ndi.sum(core, lab, index=range(1, n + 1))
    keep = np.zeros(n + 1, bool)
    keep[1:] = sizes >= min_size
    return np.where(keep[lab], lab, 0)


def area_labels(lab, seeds, bridges=()):
    """Image of area numbers from the regions the seeds point at."""
    names = sorted(seeds)
    idx = {n: i + 1 for i, n in enumerate(names)}
    out = np.zeros(lab.shape, int)
    near, (iy, ix) = ndi.distance_transform_edt(lab == 0, return_indices=True)
    for name, pts in seeds.items():
        for x, y in pts:
            comp = lab[y, x]
            if comp == 0:
                if near[y, x] > 25:
                    raise SystemExit(f"seed {name} {x},{y} is not on a region")
                comp = lab[iy[y, x], ix[y, x]]
            out[lab == comp] = idx[name]
    if bridges:
        from PIL import ImageDraw
        for name, p0, p1, width in bridges:
            canvas = Image.new("L", (lab.shape[1], lab.shape[0]), 0)
            ImageDraw.Draw(canvas).line([p0, p1], fill=255, width=width)
            out[np.array(canvas) > 0] = idx[name]
    return names, out


# ---- vectorising ---------------------------------------------------------------

def douglas_peucker(points, eps):
    if len(points) <= 2:
        return points
    pts = np.array(points, float)
    keep = np.zeros(len(pts), bool)
    keep[0] = keep[-1] = True
    stack = [(0, len(pts) - 1)]
    while stack:
        a, b = stack.pop()
        if b <= a + 1:
            continue
        p, q = pts[a], pts[b]
        d = q - p
        length = np.hypot(*d)
        seg = pts[a + 1:b]
        if length == 0:
            dist = np.hypot(*(seg - p).T)
        else:
            dist = np.abs(d[0] * (seg[:, 1] - p[1]) - d[1] * (seg[:, 0] - p[0])) / length
        i = int(np.argmax(dist))
        if dist[i] > eps:
            k = a + 1 + i
            keep[k] = True
            stack.append((a, k))
            stack.append((k, b))
    return [tuple(p) for p in pts[keep]]


def trace_chains(lab):
    """Boundary chains between different labels, as (label pair, vertices)."""
    h, w = lab.shape
    edges = []
    vy, vx = np.nonzero(lab[:, 1:] != lab[:, :-1])
    for y, x in zip(vy, vx):
        x += 1
        edges.append(((x, y), (x, y + 1), int(lab[y, x - 1]), int(lab[y, x])))
    hy, hx = np.nonzero(lab[1:, :] != lab[:-1, :])
    for y, x in zip(hy, hx):
        y += 1
        edges.append(((x, y), (x + 1, y), int(lab[y - 1, x]), int(lab[y, x])))
    adj = {}
    for i, (a, b, _, _) in enumerate(edges):
        adj.setdefault(a, []).append(i)
        adj.setdefault(b, []).append(i)
    junction = {v for v, es in adj.items() if len(es) != 2}
    used = [False] * len(edges)
    chains = []

    def follow(start, e):
        verts, cur = [start], start
        first = e
        while True:
            used[e] = True
            a, b, _, _ = edges[e]
            cur = b if a == cur else a
            verts.append(cur)
            if cur in junction or cur == start:
                break
            es = adj[cur]
            e = es[0] if es[1] == e else es[1]
            if used[e]:
                break
        return tuple(sorted((edges[first][2], edges[first][3]))), verts

    for v in junction:
        for e in adj[v]:
            if not used[e]:
                chains.append(follow(v, e))
    for e in range(len(edges)):
        if not used[e]:
            chains.append(follow(edges[e][0], e))
    return chains


def vectorise(lab, eps):
    chains = trace_chains(np.pad(lab, 1))
    lines, pair_len = [], {}
    for pair, verts in chains:
        pts = [(x - 1, y - 1) for x, y in verts]
        pair_len[pair] = pair_len.get(pair, 0) + len(pts) - 1
        simple = douglas_peucker(pts, eps)
        if len(simple) >= 2:
            lines.append(LineString(simple))
    faces = {}
    for f in polygonize(unary_union(lines)):
        p = f.representative_point()
        x, y = int(p.x), int(p.y)
        if 0 <= y < lab.shape[0] and 0 <= x < lab.shape[1] and lab[y, x]:
            faces.setdefault(int(lab[y, x]), []).append(f)
    return faces, pair_len


def merge_fragments(lab):
    """Stray small pieces of an area go to the neighbour they touch most."""
    changed = True
    while changed:
        changed = False
        for k in np.unique(lab):
            if k == 0:
                continue
            comps, n = ndi.label(lab == k)
            if n < 2:
                continue
            sizes = ndi.sum(lab == k, comps, index=range(1, n + 1))
            main = int(np.argmax(sizes)) + 1
            for c in range(1, n + 1):
                if c == main or sizes[c - 1] >= MIN_FRAGMENT:
                    continue
                m = comps == c
                ring = ndi.binary_dilation(m, iterations=2) & ~m
                around = lab[ring]
                around = around[(around != k) & (around != 0)]
                if len(around):
                    lab[m] = np.bincount(around).argmax()
                    changed = True
    return lab


def build_layout(path, spec, sigma=4.0, eps=6.0):
    lab = segment(path, spec["band"])
    names, a = area_labels(lab, spec["seeds"], spec.get("bridges", ()))
    seeds = a > 0
    rows = np.where(seeds.any(axis=1))[0]
    x0 = int(np.median([np.nonzero(seeds[y])[0].min() for y in rows]))
    x1 = int(np.median([np.nonzero(seeds[y])[0].max() for y in rows]))
    cols = [x for x in range(x0, x1) if seeds[:, x].any()]
    y0 = int(np.median([np.nonzero(seeds[:, x])[0].min() for x in cols]))
    y1 = int(np.median([np.nonzero(seeds[:, x])[0].max() for x in cols]))
    _, (iy, ix) = ndi.distance_transform_edt(~seeds, return_indices=True)
    full = a[iy, ix][y0:y1 + 1, x0:x1 + 1]
    best = np.full(full.shape, -1.0, np.float32)
    out = np.zeros(full.shape, int)
    for k in range(1, len(names) + 1):
        g = ndi.gaussian_filter((full == k).astype(np.float32), sigma, mode="nearest")
        m = g > best
        best[m], out[m] = g[m], k
    out = merge_fragments(out)
    faces, pair_len = vectorise(out, eps)
    height, width = out.shape
    sx, sy = CANVAS[0] / width, CANVAS[1] / height

    positions = {}
    for k, polys in faces.items():
        area_id = names[k - 1]
        rings = []
        for f in sorted(polys, key=lambda p: -p.area):
            ring = [[round(x * sx, 1), round(y * sy, 1)] for x, y in list(f.exterior.coords)[:-1]]
            rings.append(ring)
        big = Polygon(rings[0])
        c = polylabel(big, tolerance=1.0)
        tags = []
        if area_id in OCEANS:
            tags = ["ocean", OCEANS[area_id]]
        else:
            cont, glyph = LAND[area_id]
            tags = ["land", cont] + ([f"glyph-{glyph}"] if glyph else [])
        positions[area_id] = {"id": area_id, "name": display_name(area_id), "tags": tags,
                              "anchor": [round(c.x, 1), round(c.y, 1)], "polygons": rings}
    edges = []
    for (la, lb), n in pair_len.items():
        if la and lb and n >= MIN_EDGE:
            a_id, b_id = sorted((names[la - 1], names[lb - 1]))
            edges.append({"from": a_id, "to": b_id})
    edges = sorted({(e["from"], e["to"]) for e in edges})
    board = {"style": "areas", "canvas": {"width": CANVAS[0], "height": CANVAS[1]},
             "positions": [positions[n] for n in sorted(positions)],
             "edges": [{"from": a, "to": b} for a, b in edges]}
    return board


def preview(board, path):
    from PIL import ImageDraw, ImageFont
    scale = 1.4
    im = Image.new("RGB", (int(CANVAS[0] * scale) + 20, int(CANVAS[1] * scale) + 20), (20, 20, 30))
    d = ImageDraw.Draw(im)
    font = ImageFont.load_default(size=13)
    rng = np.random.RandomState(4)
    for p in board["positions"]:
        col = tuple(int(v) for v in rng.randint(120, 240, 3))
        for ring in p["polygons"]:
            d.polygon([(x * scale + 10, y * scale + 10) for x, y in ring], fill=col, outline=(0, 0, 0))
        ax, ay = p["anchor"]
        d.text((ax * scale + 10 - 30, ay * scale + 10 - 7), p["name"], fill=(0, 0, 0), font=font)
    im.save(path)


def guide_tables(doc, layouts):
    """Markdown for the Map guide page: one table of areas per layout."""
    def table(board, starts, sigils):
        names = {p["id"]: p["name"] for p in board["positions"]}
        tags = {p["id"]: p["tags"] for p in board["positions"]}
        adj = {k: [] for k in names}
        for e in board["edges"]:
            adj[e["from"]].append(e["to"])
            adj[e["to"]].append(e["from"])
        start_of = {v: k for k, v in starts.items()}
        label = {"gc": "Great Cthulhu", "cc": "Crawling Chaos", "bg": "Black Goat", "ys": "Yellow Sign",
                 "ww": "Windwalker", "sl": "Sleeper"}
        sigil_of = {}
        for cult, areas in sigils.items():
            for a in areas:
                sigil_of.setdefault(a, []).append(label[cult])
        rows = ["| Area | Type | Glyph | Start | Sigil | Borders |", "|---|---|---|---|---|---|"]
        for p in sorted(board["positions"], key=lambda p: p["name"]):
            i = p["id"]
            kind = "Ocean" if "ocean" in tags[i] else "Land"
            glyph = next((t[6:].capitalize() for t in tags[i] if t.startswith("glyph-")), "—")
            start = label[start_of[i]] if i in start_of else ""
            sigil = ", ".join(sigil_of.get(i, []))
            rows.append(f"| {p['name']} | {kind} | {glyph} | {start} | {sigil} | {', '.join(sorted(names[x] for x in adj[i]))} |")
        return "\n".join(rows)

    out = []
    for gid in ("cthulhu-wars", "cthulhu-wars-3-5", "cthulhu-wars-3p", "cthulhu-wars-5p"):
        spec = layouts[gid]
        board = doc["board"] if gid == "cthulhu-wars" else next(v for v in doc["variants"] if v["id"] == gid)["patch"]["board"]
        starts = dict(doc["params"]["starts"])
        starts.update(spec["starts"])
        sigils = dict(doc["params"].get("sigils", {}))
        sigils.update(spec.get("sigils", {}))
        if spec.get("seats") == 3:
            starts.pop("ys", None)
        out.append(f"### {spec['name'].replace('Cthulhu Wars (', '').rstrip(')').capitalize()}\n\n"
                   f"{len(board['positions'])} areas, {len(board['edges'])} borders. Variant id `{gid}`.\n\n{table(board, starts, sigils)}\n")
    return "\n".join(out)


def write_guide(doc):
    """The Map page of the rules guide lists every layout's areas and borders."""
    page = os.path.join(GAME_DIR, "guide", "map.md")
    text = open(page).read()
    marker = "## Areas and borders"
    head = text.split(marker)[0] if marker in text else text.rstrip() + "\n\n"
    with open(page, "w") as f:
        f.write(head + marker + "\n\n" + guide_tables(doc, LAYOUTS))


def convert(arg, stdin=None):
    """Runs `bingle dsl convert` on a file, or on JSON given as stdin ("-")."""
    return subprocess.run(["go", "run", ".", "dsl", "convert", arg], cwd=REPO, input=stdin,
                          capture_output=True, text=True, check=True).stdout


def load_definition():
    """The whole definition as JSON, its includes resolved."""
    return json.loads(convert(MAIN))


def to_yaml(value):
    return convert("-", json.dumps(value, ensure_ascii=False))


def write_definition(doc, parts):
    """Writes the main file (the parts as includes), the named parts and
    every variant, each in its own file."""
    main = dict(doc)
    for key, name in PARTS.items():
        main[key] = "__include__" + name
    main["variants"] = ["__include__variants/%s.yaml" % v["id"] for v in doc["variants"]]
    text = re.sub(r"__include__(\S+)", r"!include \1", to_yaml(main))
    with open(MAIN, "w") as f:
        f.write(SCHEMA + text)
    for key in parts:
        with open(os.path.join(DEF_DIR, PARTS[key]), "w") as f:
            f.write(PART + to_yaml(doc[key]))
    os.makedirs(os.path.join(DEF_DIR, "variants"), exist_ok=True)
    for v in doc["variants"]:
        with open(os.path.join(DEF_DIR, "variants", v["id"] + ".yaml"), "w") as f:
            f.write(PART + to_yaml(v))


# The fifth seat of the five-player layout.
FIFTH_SEAT = {"seat": 5, "color": "white", "label": "Windwalker"}


def main():
    if len(sys.argv) < 2:
        raise SystemExit(__doc__)
    if sys.argv[1] == "--guide":
        write_guide(load_definition())
        return
    src = sys.argv[1]
    prev = sys.argv[sys.argv.index("--preview") + 1] if "--preview" in sys.argv else None
    doc = load_definition()
    doc.pop("_base_board", None)
    boards = {}
    for gid, spec in LAYOUTS.items():
        boards[gid] = build_layout(os.path.join(src, spec["file"]), spec)
        degs = {}
        for e in boards[gid]["edges"]:
            degs[e["from"]] = degs.get(e["from"], 0) + 1
            degs[e["to"]] = degs.get(e["to"], 0) + 1
        print(f"{gid}: {len(boards[gid]['positions'])} areas, {len(boards[gid]['edges'])} borders, "
              f"degrees {sorted(degs.values())}")
        if prev:
            os.makedirs(prev, exist_ok=True)
            preview(boards[gid], os.path.join(prev, gid + ".png"))

    base = LAYOUTS["cthulhu-wars"]
    doc["name"] = base["name"]
    doc["board"] = boards["cthulhu-wars"]
    params = doc.setdefault("params", {})
    params["starts"] = base["starts"]
    params["sigils"] = base["sigils"]
    full_seats = doc["seats"]
    doc["variants"] = []
    for gid in ("cthulhu-wars-3-5", "cthulhu-wars-3p", "cthulhu-wars-5p"):
        spec = LAYOUTS[gid]
        patch = {"board": boards[gid], "params": {"starts": spec["starts"]}}
        if spec.get("sigils"):
            patch["params"]["sigils"] = spec["sigils"]
        if spec.get("seats") == 3:
            patch["seats"] = full_seats[:3]
            patch["turn_order"] = {"type": "round-robin", "seats": [1, 2, 3]}
        if spec.get("seats") == 5:
            # The Windwalker sets up first (it chooses where it begins), so its
            # seat leads the turn order.
            patch["seats"] = full_seats + [FIFTH_SEAT]
            patch["turn_order"] = {"type": "round-robin", "seats": [5, 1, 2, 3, 4]}
        doc["variants"].append({"id": gid, "name": spec["name"], "patch": patch})
    write_definition(doc, parts=("board",))
    write_guide(doc)


if __name__ == "__main__":
    main()
