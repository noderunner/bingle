// Board themes for the games played with stones: a palette for the board and
// the stones, SVG gradients, patterns and filters, a decorated frame and a
// small mark on every stone — all drawn here, none of it a picture file.
//
// Every theme keeps the same idea for the two sides: the first player's stones
// are warm (reds, golds, oranges, cream) and the second player's cold (blues,
// violets, near-black), whatever the colour names of the game say, so the
// players can be told apart at a glance in any theme. (A game whose sides are
// "white" and "black" gets a warm light and a cold dark stone.)
//
// A theme is selected with <html data-board="id">; the stylesheet that makes it
// work is built from the tables below (installThemes) and put in the page once,
// together with the hidden <svg> that holds the gradients (url(#th-…)).

import { s } from "./util.js";

const SVG_COLORS = ["red", "yellow", "orange", "white", "blue", "black", "green", "purple", "brown"];

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

function rgb(hex) {
	const n = parseInt(hex.slice(1), 16);
	return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

function hex(c) {
	return "#" + c.map((v) => Math.max(0, Math.min(255, Math.round(v))).toString(16).padStart(2, "0")).join("");
}

/** Mixes two #rrggbb colours: t = 0 is a, t = 1 is b. */
function mix(a, b, t) {
	const x = rgb(a);
	const y = rgb(b);
	return hex(x.map((v, i) => v + (y[i] - v) * t));
}

/** A small deterministic random number generator, so a board is always drawn the same. */
function rng(seed) {
	let x = seed >>> 0 || 1;
	return () => {
		x = (Math.imul(x, 1664525) + 1013904223) >>> 0;
		return x / 4294967296;
	};
}

const f1 = (v) => (Math.round(v * 10) / 10).toString();

/** A closed polygon path from [x, y] pairs. */
function polyPath(pts) {
	return "M" + pts.map((p) => `${f1(p[0])} ${f1(p[1])}`).join("L") + "Z";
}

/** A regular star with `n` points between radius `ro` and `ri`, as a path around (cx, cy). */
function starPath(cx, cy, n, ro, ri, rot = -Math.PI / 2) {
	const pts = [];
	for (let k = 0; k < 2 * n; k++) {
		const r = k % 2 === 0 ? ro : ri;
		const a = rot + (Math.PI * k) / n;
		pts.push([cx + r * Math.cos(a), cy + r * Math.sin(a)]);
	}
	return polyPath(pts);
}

/** The Elder Futhark, as strokes in a box 1 wide and 2 high. */
const RUNES = {
	fehu: "M.5 0V2M.5 .6L1 .15M.5 1.1L1 .65",
	uruz: "M.2 0V2M.2 .3L.8 .8V2",
	thurisaz: "M.3 0V2M.3 .55L.85 1L.3 1.45",
	ansuz: "M.3 0V2M.3 .3L.85 .75M.3 .85L.85 1.3",
	raidho: "M.3 0V2M.3 .2L.85 .6L.3 1L.85 1.9",
	kenaz: "M.8 .3L.25 1L.8 1.7",
	gebo: "M.15 .3L.85 1.7M.85 .3L.15 1.7",
	wunjo: "M.3 0V2M.3 .2L.85 .7L.3 1.2",
	hagalaz: "M.15 0V2M.85 0V2M.15 .7L.85 1.3",
	nauthiz: "M.5 0V2M.15 .55L.85 1.15",
	isa: "M.5 0V2",
	jera: "M.35 .3L.65 .7L.35 1.1M.65 .9L.35 1.3L.65 1.7",
	eihwaz: "M.5 0V2M.5 .4L.15 .1M.5 1.6L.85 1.9",
	perthro: "M.25 0V2M.25 .2L.8 .45V1.4L.25 1.7",
	algiz: "M.5 0V2M.5 .7L.15 .2M.5 .7L.85 .2",
	sowilo: "M.8 .1L.2 .7L.8 1.3L.2 1.9",
	tiwaz: "M.5 0V2M.5 0L.15 .6M.5 0L.85 .6",
	berkano: "M.25 0V2M.25 .1L.8 .55L.25 1L.8 1.45L.25 1.9",
	ehwaz: "M.2 0V2M.8 0V2M.2 .2L.5 .7L.8 .2",
	mannaz: "M.2 0V2M.8 0V2M.2 .1L.8 1.1M.8 .1L.2 1.1",
	laguz: "M.35 0V2M.35 0L.8 .5",
	ingwaz: "M.5 .2L.85 1L.5 1.8L.15 1Z",
	dagaz: "M.15 .1V1.9L.85 .1V1.9Z",
	othala: "M.5 .1L.85 .9L.5 1.4L.15 .9ZM.15 1.9L.5 1.4L.85 1.9",
};
const RUNE_NAMES = Object.keys(RUNES);

/** A rune as a path scaled to `h` high with its top-left at (x, y). */
function runePath(name, x, y, h) {
	const k = h / 2;
	const nums = RUNES[name].match(/[MLVHZ]|-?[\d.]+/g);
	let out = "";
	let i = 0;
	let cx = 0;
	let cy = 0;
	while (i < nums.length) {
		const cmd = nums[i++];
		if (cmd === "Z") {
			out += "Z";
		} else if (cmd === "V") {
			cy = parseFloat(nums[i++]);
			out += `V${f1(y + cy * k)}`;
		} else if (cmd === "H") {
			cx = parseFloat(nums[i++]);
			out += `H${f1(x + cx * k)}`;
		} else {
			cx = parseFloat(nums[i++]);
			cy = parseFloat(nums[i++]);
			out += `${cmd}${f1(x + cx * k)} ${f1(y + cy * k)}`;
		}
	}
	return out;
}

let clipCounter = 0;

/** A group of the decorations of a frame (they never take clicks). */
function decorGroup(id, children) {
	return s("g", { class: `theme-decor th-${id}`, "pointer-events": "none", "aria-hidden": "true" }, children);
}

/** The frame of the board as a rounded rectangle path, for clipping. */
function frameClip(w, h) {
	const id = `th-clip-${clipCounter++}`;
	return { id, node: s("clipPath", { id }, s("rect", { x: 0, y: 0, width: w, height: h, rx: 14 })) };
}

// ---------------------------------------------------------------------------
// The palette of a stone: mid colour, highlight, shadow, rim and glow
// ---------------------------------------------------------------------------

/**
 * A stone palette from its main colour; every part can be set by hand.
 * `hi` is the highlight of the gradient, `lo` its shade, `rim` the outline.
 */
function stone(mid, o = {}) {
	return {
		mid,
		hi: o.hi || mix(mid, "#ffffff", 0.55),
		lo: o.lo || mix(mid, "#000000", 0.55),
		rim: o.rim || mix(mid, "#000000", 0.7),
		glow: o.glow || null,
		glowSize: o.glowSize,
		mark: o.mark || null,
	};
}

// ---------------------------------------------------------------------------
// The themes
// ---------------------------------------------------------------------------

const SYNTH = {
	id: "synthwave",
	label: "Hot Synthwave Vibes",
	board: { light: "#3a1466", mid: "#2c0f55", dark: "#1f0a42", line: "#ff4fd8", frame: "#12032b", label: "#7df9ff", last: "rgba(255,238,90,0.5)", focus: "#7df9ff" },
	stones: {
		// Warm: neon pinks, suns and oranges. Cold: electric cyan and violet-black.
		red: stone("#ff2a84", { hi: "#ffb0d6", lo: "#8a0048", rim: "#ffd6ec", glow: "#ff2a84", glowSize: 4 }),
		yellow: stone("#ffd23a", { hi: "#fffbc0", lo: "#ff8a00", rim: "#fff2a0", glow: "#ffb000", glowSize: 4 }),
		orange: stone("#ff6a1a", { hi: "#ffc79a", lo: "#a52a00", rim: "#ffd2ae", glow: "#ff6a1a", glowSize: 4 }),
		white: stone("#ffc4de", { hi: "#ffffff", lo: "#ff7fb8", rim: "#ffffff", glow: "#ff9ad0", glowSize: 4 }),
		blue: stone("#19c6ff", { hi: "#c8fcff", lo: "#0a3fd1", rim: "#bff8ff", glow: "#19c6ff", glowSize: 4 }),
		black: stone("#2a1458", { hi: "#7a4dc2", lo: "#07021a", rim: "#3df5ff", glow: "#7d3cff", glowSize: 4 }),
		green: stone("#3dffa2", { glow: "#3dffa2", glowSize: 4 }),
		purple: stone("#b86bff", { glow: "#b86bff", glowSize: 4 }),
		brown: stone("#c4642a", { glow: "#ff8a3a", glowSize: 3 }),
	},
};

const CELTIC = {
	id: "celtic",
	label: "Celtic Rune Fantasy",
	board: { light: "#4a6a52", mid: "#3a5a44", dark: "#2c4636", line: "#d9b45a", frame: "#12281c", label: "#e9cf86", last: "rgba(240,205,100,0.5)", focus: "#f0cd64" },
	stones: {
		// Warm: garnet, amber, copper and bone. Cold: woad blue and slate-black.
		red: stone("#c4331f", { hi: "#ff9a7a", lo: "#4e0c06", rim: "#f0c060" }),
		yellow: stone("#e2aa2a", { hi: "#fff1a8", lo: "#7a4e06", rim: "#fff0b0" }),
		orange: stone("#d9742a", { hi: "#ffc08a", lo: "#5e2a08", rim: "#f6c27a" }),
		white: stone("#e8e0c4", { hi: "#fffdf0", lo: "#a89c78", rim: "#6b5d36" }),
		blue: stone("#3b6fc4", { hi: "#a8d0ff", lo: "#10224f", rim: "#d5dfec" }),
		black: stone("#262b33", { hi: "#707a88", lo: "#07090c", rim: "#b8c2d0" }),
		green: stone("#4f9a5b"),
		purple: stone("#7a56b8"),
		brown: stone("#8a5a32"),
	},
};

const SOLSTICE = {
	id: "solstice",
	label: "Southwestern Solstice",
	board: { light: "#e8c99a", mid: "#d9a96f", dark: "#c78c52", line: "#7a3b1d", frame: "#5b2a16", label: "#f6dca8", last: "rgba(255,226,120,0.6)", focus: "#1f8f95" },
	stones: {
		// Warm: terracotta, sunstone, saffron and bone. Cold: turquoise and obsidian.
		red: stone("#c4532b", { hi: "#f2a37c", lo: "#5c1c0a", rim: "#f6d2a0" }),
		yellow: stone("#f2b92b", { hi: "#fff0a0", lo: "#9a5a05", rim: "#fff0b8" }),
		orange: stone("#ee7a22", { hi: "#ffc58a", lo: "#7c3305", rim: "#ffd9a6" }),
		white: stone("#efe2c8", { hi: "#fffaf0", lo: "#b09a74", rim: "#7a3b1d" }),
		blue: stone("#2fc4b2", { hi: "#c0fff3", lo: "#075a5e", rim: "#d8fff6" }),
		black: stone("#26223a", { hi: "#6a6488", lo: "#08070f", rim: "#46d6c4" }),
		green: stone("#5a9a4a"),
		purple: stone("#7a5aa0"),
		brown: stone("#8a5a32"),
	},
};

const BAZAAR = {
	id: "bazaar",
	label: "Moroccan Bazaar",
	board: { light: "#f1e3c4", mid: "#e3cda0", dark: "#cfb27c", line: "#8a5a22", frame: "#0f2c4a", label: "#f0cf7a", last: "rgba(255,200,60,0.55)", focus: "#1f56c8" },
	stones: {
		// Warm: paprika, brass, saffron and pearl. Cold: majolica cobalt and ebony.
		red: stone("#d4361f", { hi: "#ff9b6b", lo: "#6a1006", rim: "#f6d27a" }),
		yellow: stone("#e0a82a", { hi: "#fff3b0", lo: "#7a4e06", rim: "#fff0b0" }),
		orange: stone("#ef8a1a", { hi: "#ffd08a", lo: "#7c3d04", rim: "#ffe1a6" }),
		white: stone("#f0e6d0", { hi: "#ffffff", lo: "#b3a283", rim: "#8a5a22" }),
		blue: stone("#1f56c8", { hi: "#8fb6ff", lo: "#081d55", rim: "#f0d58a" }),
		black: stone("#1e1d22", { hi: "#5a5a66", lo: "#050507", rim: "#d6ad4a" }),
		green: stone("#1f8f6a"),
		purple: stone("#6a3a9a"),
		brown: stone("#8a5a32"),
	},
};

const ABYSS = {
	id: "abyss",
	label: "Coral Abyss",
	board: { light: "#2f8aa6", mid: "#256f90", dark: "#1b567a", line: "#a4f2ff", frame: "#031c2e", label: "#a4f2ff", last: "rgba(255,230,120,0.5)", focus: "#7df9ff" },
	stones: {
		// Warm: coral, amber, clownfish and pearl. Cold: bioluminescent blue and the black pearl.
		red: stone("#ff5a4a", { hi: "#ffb8a8", lo: "#8a1620", rim: "#ffd8c8", glow: "#ff6a5a", glowSize: 3 }),
		yellow: stone("#ffc236", { hi: "#fff2b0", lo: "#c47a06", rim: "#fff0c0", glow: "#ffc236", glowSize: 3 }),
		orange: stone("#ff8a2a", { hi: "#ffd0a0", lo: "#a8420a", rim: "#ffe0c0", glow: "#ff8a2a", glowSize: 3 }),
		white: stone("#eaf4ff", { hi: "#ffffff", lo: "#9cc0dc", rim: "#ffffff", glow: "#d8f0ff", glowSize: 3 }),
		blue: stone("#2ab7ff", { hi: "#c6f7ff", lo: "#0a3d9c", rim: "#b8f4ff", glow: "#2ab7ff", glowSize: 4 }),
		black: stone("#1a2540", { hi: "#7088b0", lo: "#02040a", rim: "#7a9cff", glow: "#4f6bff", glowSize: 4 }),
		green: stone("#3adf9a", { glow: "#3adf9a", glowSize: 3 }),
		purple: stone("#a07aff", { glow: "#a07aff", glowSize: 3 }),
		brown: stone("#c08a5a"),
	},
};

const GEMS = {
	id: "gems",
	label: "Precious Gemstones",
	board: { light: "#3b3566", mid: "#312b58", dark: "#272246", line: "#e6d595", frame: "#120e26", label: "#e6d595", last: "rgba(255,226,130,0.5)", focus: "#7df9ff" },
	stones: {
		// Warm: ruby, citrine, fire opal, diamond and smoky quartz. Cold: sapphire and onyx, with emerald and amethyst besides.
		red: stone("#d8173d", { hi: "#ff9aae", lo: "#6a0620", rim: "#ffc4d0", glow: "#ff3d63", glowSize: 2.5 }),
		yellow: stone("#f6c622", { hi: "#fff6b0", lo: "#a8700a", rim: "#fff2b0", glow: "#ffd23a", glowSize: 2.5 }),
		orange: stone("#f08a1c", { hi: "#ffd29a", lo: "#8c3e04", rim: "#ffe0b0", glow: "#ff9a2a", glowSize: 2.5 }),
		white: stone("#eef6ff", { hi: "#ffffff", lo: "#9cc0e0", rim: "#ffffff", glow: "#cfe8ff", glowSize: 2.5 }),
		blue: stone("#2455dc", { hi: "#9cbcff", lo: "#0a1e6a", rim: "#bcd0ff", glow: "#3d78ff", glowSize: 2.5 }),
		black: stone("#1e1a30", { hi: "#7a6aa8", lo: "#05040c", rim: "#c8a6ff", glow: "#8a5cff", glowSize: 2.5 }),
		green: stone("#14a864", { hi: "#9cf0c4", lo: "#04502c", rim: "#c0f6da", glow: "#2ce08a", glowSize: 2.5 }),
		purple: stone("#9a58e0", { hi: "#e0c0ff", lo: "#3e1672", rim: "#e8d0ff", glow: "#b47aff", glowSize: 2.5 }),
		brown: stone("#8a6a4a", { hi: "#d8b896", lo: "#3a2814", rim: "#e8d0b0" }),
	},
};

export const THEMES = [SYNTH, CELTIC, SOLSTICE, BAZAAR, ABYSS, GEMS];

// ---------------------------------------------------------------------------
// Gradients, patterns and filters (the hidden <svg> of the page)
// ---------------------------------------------------------------------------

function stoneDefs(t) {
	const out = [];
	for (const c of SVG_COLORS) {
		const st = t.stones[c];
		if (!st) continue;
		out.push(
			s(
				"radialGradient",
				{ id: `th-${t.id}-${c}`, cx: "0.38", cy: "0.32", r: "0.78" },
				s("stop", { offset: "0", "stop-color": st.hi }),
				s("stop", { offset: "0.55", "stop-color": st.mid }),
				s("stop", { offset: "1", "stop-color": st.lo })
			)
		);
		// A soft glow in the colour of the stone, or a plain shadow.
		out.push(
			s(
				"filter",
				{ id: `th-${t.id}-${c}-fx`, x: "-40%", y: "-40%", width: "180%", height: "180%" },
				st.glow
					? s("feDropShadow", { dx: 0, dy: 0, stdDeviation: st.glowSize || 3, "flood-color": st.glow, "flood-opacity": 0.95 })
					: s("feDropShadow", { dx: 0, dy: 2, stdDeviation: 1.8, "flood-color": "#000000", "flood-opacity": 0.5 })
			)
		);
	}
	return out;
}

/** The three fills of the cells of a theme (light, mid, dark) and any extras. */
function cellDefs(t) {
	const id = t.id;
	const out = [];
	const tile = (suffix, size, fill, ...children) =>
		s("pattern", { id: `th-${id}-${suffix}`, width: size, height: size, patternUnits: "userSpaceOnUse" }, s("rect", { width: size, height: size, fill }), ...children);
	const b = t.board;
	switch (id) {
		case "synthwave": {
			// Violet glass with scan lines.
			const scan = (suffix, fill) =>
				tile(suffix, 8, fill, s("rect", { y: 6, width: 8, height: 2, fill: "#000000", "fill-opacity": 0.22 }), s("rect", { y: 0, width: 8, height: 1, fill: "#ff4fd8", "fill-opacity": 0.1 }));
			out.push(scan("cl", "rgba(76,24,130,0.6)"), scan("cm", "rgba(54,16,104,0.64)"), scan("cd", "rgba(36,9,76,0.68)"));
			break;
		}
		case "celtic": {
			// Moss and weathered stone: speckled with lichen.
			const r = rng(7);
			const speck = (suffix, fill, light) => {
				const dots = [];
				for (let i = 0; i < 26; i++) {
					dots.push(s("circle", { cx: f1(r() * 40), cy: f1(r() * 40), r: f1(0.5 + r() * 1.3), fill: r() > 0.5 ? light : "#0a1a10", "fill-opacity": 0.22 + r() * 0.2 }));
				}
				dots.push(s("path", { d: "M0 14C10 10 14 20 26 15S36 12 40 17", fill: "none", stroke: "#0a1a10", "stroke-opacity": 0.18, "stroke-width": 1 }));
				return tile(suffix, 40, fill, ...dots);
			};
			out.push(speck("cl", b.light, "#9fc08a"), speck("cm", b.mid, "#8fb07a"), speck("cd", b.dark, "#7aa070"));
			break;
		}
		case "solstice": {
			// Sun-baked adobe: grain and hairline cracks.
			const r = rng(11);
			const grain = (suffix, fill) => {
				const dots = [];
				for (let i = 0; i < 34; i++) dots.push(s("circle", { cx: f1(r() * 44), cy: f1(r() * 44), r: f1(0.4 + r() * 0.9), fill: r() > 0.5 ? "#ffffff" : "#5a2a10", "fill-opacity": 0.18 }));
				dots.push(s("path", { d: "M4 30L12 26L17 31L26 24", fill: "none", stroke: "#5a2a10", "stroke-opacity": 0.16, "stroke-width": 0.8 }));
				return tile(suffix, 44, fill, ...dots);
			};
			out.push(grain("cl", "rgba(236,208,162,0.94)"), grain("cm", "rgba(222,182,128,0.94)"), grain("cd", "rgba(204,152,98,0.94)"));
			break;
		}
		case "bazaar": {
			// Smooth plaster: a flat wash with a hairline edge.
			const plaster = (suffix, fill) => tile(suffix, 48, fill, s("rect", { x: 0.5, y: 0.5, width: 47, height: 47, fill: "none", stroke: "#8a5a22", "stroke-opacity": 0.14, "stroke-width": 1 }));
			out.push(plaster("cl", "rgba(246,234,206,0.97)"), plaster("cm", "rgba(232,212,170,0.97)"), plaster("cd", "rgba(214,184,132,0.97)"));
			break;
		}
		case "gems": {
			// Polished stone cut in facets: a few faint planes in every tile.
			const facets = (suffix, fill, a) =>
				tile(suffix, 44, fill, s("path", { d: "M0 0L44 44M44 0L0 44", stroke: "#e6d595", "stroke-opacity": a, "stroke-width": 0.8 }), s("path", { d: "M22 0L44 22L22 44L0 22Z", fill: "none", stroke: "#e6d595", "stroke-opacity": a, "stroke-width": 0.8 }));
			out.push(facets("cl", "rgba(70,60,122,0.94)", 0.12), facets("cm", "rgba(56,48,104,0.94)", 0.11), facets("cd", "rgba(42,35,82,0.94)", 0.1));
			break;
		}
		case "abyss": {
			// Glass water: translucent, with a caustic shimmer.
			const caustic = (suffix, a) =>
				s(
					"pattern",
					{ id: `th-${id}-${suffix}`, width: 70, height: 70, patternUnits: "userSpaceOnUse" },
					s("rect", { width: 70, height: 70, fill: `rgba(60,170,200,${a})` }),
					s("path", { d: "M0 20C12 8 24 30 36 18S58 10 70 22", fill: "none", stroke: "#d8fbff", "stroke-opacity": 0.22, "stroke-width": 1.4 }),
					s("path", { d: "M0 52C14 40 26 62 40 50S60 44 70 54", fill: "none", stroke: "#d8fbff", "stroke-opacity": 0.16, "stroke-width": 1.2 })
				);
			out.push(caustic("cl", 0.34), caustic("cm", 0.4), caustic("cd", 0.46));
			break;
		}
		default:
			break;
	}
	return out;
}

/** Everything that the decorated frames refer to by id. */
function extraDefs(t) {
	const id = t.id;
	const out = [];
	switch (id) {
		case "synthwave":
			out.push(
				s("linearGradient", { id: "th-sw-sky", x1: 0, y1: 0, x2: 0, y2: 1 }, s("stop", { offset: 0, "stop-color": "#1a0a3a" }), s("stop", { offset: 0.6, "stop-color": "#3a1466" }), s("stop", { offset: 1, "stop-color": "#6a1a7e" })),
				s("filter", { id: "th-sw-neon", x: "-10%", y: "-10%", width: "120%", height: "120%" }, s("feGaussianBlur", { stdDeviation: 2.2, result: "b" }), s("feMerge", {}, s("feMergeNode", { in: "b" }), s("feMergeNode", { in: "SourceGraphic" })))
			);
			break;
		case "celtic":
			out.push(
				s("radialGradient", { id: "th-ce-bg", cx: 0.5, cy: 0.5, r: 0.75 }, s("stop", { offset: 0, "stop-color": "#1f4a32" }), s("stop", { offset: 1, "stop-color": "#0b1d13" })),
				s("filter", { id: "th-ce-moss", x: 0, y: 0, width: "100%", height: "100%" }, s("feTurbulence", { type: "fractalNoise", baseFrequency: 0.035, numOctaves: 3, seed: 4 }), s("feColorMatrix", { values: "0 0 0 0 0.35  0 0 0 0 0.6  0 0 0 0 0.35  0 0 0 0.55 -0.12" }))
			);
			break;
		case "solstice":
			out.push(
				s("linearGradient", { id: "th-so-sky", x1: 0, y1: 0, x2: 0, y2: 1 }, s("stop", { offset: 0, "stop-color": "#1b4d63" }), s("stop", { offset: 0.45, "stop-color": "#d77a4a" }), s("stop", { offset: 1, "stop-color": "#f3c15a" })),
				s("radialGradient", { id: "th-so-sun", cx: 0.5, cy: 0.5, r: 0.5 }, s("stop", { offset: 0, "stop-color": "#fff6c0" }), s("stop", { offset: 0.6, "stop-color": "#ffd25a" }), s("stop", { offset: 1, "stop-color": "#ff9a2a", "stop-opacity": 0 }))
			);
			break;
		case "bazaar":
			out.push(
				s("linearGradient", { id: "th-bz-bg", x1: 0, y1: 0, x2: 1, y2: 1 }, s("stop", { offset: 0, "stop-color": "#0f2c4a" }), s("stop", { offset: 0.5, "stop-color": "#14506a" }), s("stop", { offset: 1, "stop-color": "#0b2240" })),
				s("pattern", { id: "th-bz-tile", width: 40, height: 40, patternUnits: "userSpaceOnUse" },
					s("rect", { width: 40, height: 40, fill: "none" }),
					s("path", { d: "M20 0L40 20L20 40L0 20Z", fill: "none", stroke: "#f0cf7a", "stroke-opacity": 0.16, "stroke-width": 1 })
				)
			);
			break;
		case "gems":
			out.push(
				s("radialGradient", { id: "th-gm-bg", cx: 0.5, cy: 0.5, r: 0.8 }, s("stop", { offset: 0, "stop-color": "#2a1f52" }), s("stop", { offset: 1, "stop-color": "#0c0820" })),
				s("linearGradient", { id: "th-gm-gold", x1: 0, y1: 0, x2: 0, y2: 1 }, s("stop", { offset: 0, "stop-color": "#f6e6a8" }), s("stop", { offset: 0.5, "stop-color": "#c9a24a" }), s("stop", { offset: 1, "stop-color": "#7a5a1e" }))
			);
			break;
		case "abyss":
			out.push(
				s("linearGradient", { id: "th-ab-sea", x1: 0, y1: 0, x2: 0, y2: 1 }, s("stop", { offset: 0, "stop-color": "#0d7da0" }), s("stop", { offset: 0.45, "stop-color": "#04466b" }), s("stop", { offset: 1, "stop-color": "#010d1a" })),
				s("linearGradient", { id: "th-ab-ray", x1: 0, y1: 0, x2: 0, y2: 1 }, s("stop", { offset: 0, "stop-color": "#d8fbff", "stop-opacity": 0.5 }), s("stop", { offset: 1, "stop-color": "#d8fbff", "stop-opacity": 0 })),
				s("radialGradient", { id: "th-ab-bubble", cx: 0.35, cy: 0.3, r: 0.8 }, s("stop", { offset: 0, "stop-color": "#ffffff", "stop-opacity": 0.8 }), s("stop", { offset: 0.6, "stop-color": "#bff4ff", "stop-opacity": 0.12 }), s("stop", { offset: 1, "stop-color": "#7ae0ff", "stop-opacity": 0.4 }))
			);
			break;
		default:
			break;
	}
	return out;
}

// ---------------------------------------------------------------------------
// The stylesheet of the themes
// ---------------------------------------------------------------------------

function themeCss(t) {
	const sel = `:root[data-board="${t.id}"]`;
	const b = t.board;
	let css = `${sel} { --board-light: ${b.light}; --board-dark: ${b.dark}; --board-line: ${b.line}; --board-frame: ${b.frame}; --board-label: ${b.label}; --last-move: ${b.last}; `;
	for (const c of SVG_COLORS) {
		const st = t.stones[c];
		if (!st) continue;
		css += `--piece-${c}-fill: ${st.mid}; --piece-${c}-stroke: ${st.rim}; `;
	}
	css += "}\n";
	// Cells: patterns, but not where a game tints a cell or marks an edge.
	const plain = ':not([class*="edge-"]):not(.tint)';
	css += `${sel} .cell-bg.light${plain} { fill: url(#th-${t.id}-cl); }\n`;
	css += `${sel} .cell-bg.mid${plain} { fill: url(#th-${t.id}-cm); }\n`;
	css += `${sel} .cell-bg.dark${plain} { fill: url(#th-${t.id}-cd); }\n`;
	css += `${sel} .cell-bg.mid { fill-opacity: 1; }\n`;
	css += `${sel} .ring-hole { fill: ${b.mid}; }\n`;
	for (const c of SVG_COLORS) {
		const st = t.stones[c];
		if (!st) continue;
		css += `${sel} .piece.color-${c} .piece-body, ${sel} .ghost-piece.color-${c} .piece-body { fill: url(#th-${t.id}-${c}); stroke: ${st.rim}; stroke-width: 2.2; filter: url(#th-${t.id}-${c}-fx); }\n`;
		css += `${sel} .piece.color-${c} .piece-detail, ${sel} .ghost-piece.color-${c} .piece-detail { stroke: ${st.rim}; }\n`;
		css += `${sel} .swatch.color-${c} { background: ${st.mid}; border-color: ${st.rim}; }\n`;
	}
	// A ring (Yinsh) is drawn as the ring body and a hole: keep the body flat so the hole shows the board.
	css += `${sel} .piece.shape-donut .piece-body { filter: none; }\n`;
	return css;
}

let installed = false;

/** Puts the gradients and the stylesheet of the themes in the page (once). */
export function installThemes() {
	if (installed || typeof document === "undefined") return;
	installed = true;
	const svg = s("svg", { id: "theme-defs", "data-persist": "1", width: 0, height: 0, "aria-hidden": "true", style: "position:absolute;width:0;height:0;overflow:hidden" });
	const defs = s("defs", {});
	for (const t of THEMES) {
		for (const d of [...stoneDefs(t), ...cellDefs(t), ...extraDefs(t)]) defs.append(d);
	}
	svg.append(defs);
	document.body.prepend(svg);
	const style = document.createElement("style");
	style.id = "theme-style";
	style.textContent = THEMES.map(themeCss).join("\n");
	document.head.append(style);
}

// ---------------------------------------------------------------------------
// Which games may wear a theme
// ---------------------------------------------------------------------------

const STONE_SHAPES = new Set(["disc", "donut", "chip", "cross", "cap", "square", "wall", "flat"]);

/** A game played with stones on a board of cells: its pieces are all stone-like and its board is plain. */
export function stoneGame(info) {
	if (!info || !info.board || !info.pieces || !info.pieces.length) return false;
	const style = info.board.style;
	if (style === "areas" || style === "free" || style === "none") return false;
	return info.pieces.every((p) => STONE_SHAPES.has(p.shape));
}

/** The themes a game offers, in order. */
export function themesFor(info) {
	return stoneGame(info) ? THEMES : [];
}

export function themeById(id) {
	return THEMES.find((t) => t.id === id) || null;
}

// ---------------------------------------------------------------------------
// The frame: a backdrop and a border drawn behind the cells
// ---------------------------------------------------------------------------

/** The current theme of the page, or null for the plain ones. */
export function currentTheme() {
	if (typeof document === "undefined") return null;
	return themeById(document.documentElement.dataset.board);
}

/**
 * The decorations of a board frame `w` × `h` with a band `m` wide round the
 * cells, or null when the page wears a plain theme. They go right after the
 * frame rectangle.
 */
export function frameDecor(w, h, m) {
	const t = currentTheme();
	if (!t) return null;
	switch (t.id) {
		case "synthwave":
			return synthwaveFrame(w, h, m);
		case "celtic":
			return celticFrame(w, h, m);
		case "solstice":
			return solsticeFrame(w, h, m);
		case "bazaar":
			return bazaarFrame(w, h, m);
		case "abyss":
			return abyssFrame(w, h, m);
		case "gems":
			return gemsFrame(w, h, m);
	}
	return null;
}

/** A band along the four sides: `draw(L, T)` draws one horizontal band L long and T thick at the origin. */
function bands(w, h, m, draw) {
	const inset = Math.min(5, m / 4);
	const T = Math.max(6, m - 2 * inset);
	const g = s("g", {});
	g.append(s("g", { transform: `translate(${inset} ${inset})` }, draw(w - 2 * inset, T)));
	g.append(s("g", { transform: `translate(${w - inset} ${h - inset}) rotate(180)` }, draw(w - 2 * inset, T)));
	g.append(s("g", { transform: `translate(${w - inset} ${inset}) rotate(90)` }, draw(h - 2 * inset, T)));
	g.append(s("g", { transform: `translate(${inset} ${h - inset}) rotate(-90)` }, draw(h - 2 * inset, T)));
	return g;
}

// ---- Hot Synthwave Vibes ----------------------------------------------------

function synthwaveFrame(w, h, m) {
	const clip = frameClip(w, h);
	const g = decorGroup("synthwave", [clip.node]);
	const art = s("g", { "clip-path": `url(#${clip.id})` });
	// A plain dusk: just the sky.
	art.append(s("rect", { width: w, height: h, fill: "url(#th-sw-sky)" }));
	g.append(art);
	// A double neon border.
	const inset = Math.min(5, m / 4);
	g.append(
		s("rect", { class: "th-neon", x: inset, y: inset, width: w - 2 * inset, height: h - 2 * inset, rx: 12, fill: "none", stroke: "#ff2a84", "stroke-width": 3, filter: "url(#th-sw-neon)" }),
		s("rect", { class: "th-neon th-neon2", x: inset + 5, y: inset + 5, width: w - 2 * inset - 10, height: h - 2 * inset - 10, rx: 9, fill: "none", stroke: "#7df9ff", "stroke-width": 1.8, filter: "url(#th-sw-neon)" })
	);
	return g;
}

// ---- Celtic Rune Fantasy --------------------------------------------------

/** One side of interlaced gold knotwork: two strands that cross over and under. */
function knotBand(L, T) {
	const g = s("g", {});
	const mid = T / 2;
	const A = T * 0.3;
	const wave = Math.max(24, T * 2.2);
	const k = (Math.PI * 2) / wave;
	const strand = (phase, x0, x1) => {
		let d = "";
		for (let x = x0; x <= x1; x += 2) d += `${d ? "L" : "M"}${f1(x)} ${f1(mid + A * Math.sin(k * x + phase))}`;
		return d;
	};
	const draw = (d, w1, w2) => {
		g.append(s("path", { d, fill: "none", stroke: "#2a1a05", "stroke-width": w1, "stroke-linecap": "round", "stroke-linejoin": "round" }));
		g.append(s("path", { d, fill: "none", stroke: "#d9b45a", "stroke-width": w2, "stroke-linecap": "round", "stroke-linejoin": "round" }));
	};
	g.append(s("rect", { width: L, height: T, rx: 4, fill: "#0b1d13", "fill-opacity": 0.85 }));
	const w2 = Math.max(3, T * 0.2);
	draw(strand(0, 0, L), w2 + 2.4, w2);
	draw(strand(Math.PI, 0, L), w2 + 2.4, w2);
	// At every crossing one strand passes over the other, in turn.
	let n = 0;
	for (let x = wave / 2; x < L; x += wave / 2) {
		const ph = n % 2 === 0 ? 0 : Math.PI;
		draw(strand(ph, Math.max(0, x - 5), Math.min(L, x + 5)), w2 + 2.4, w2);
		n++;
	}
	return g;
}

function celticFrame(w, h, m) {
	const clip = frameClip(w, h);
	const g = decorGroup("celtic", [clip.node]);
	const art = s("g", { "clip-path": `url(#${clip.id})` });
	art.append(s("rect", { width: w, height: h, fill: "url(#th-ce-bg)" }));
	art.append(s("rect", { width: w, height: h, filter: "url(#th-ce-moss)", opacity: 0.55 }));
	// Runes carved in the stone behind the cells.
	const r = rng(5);
	const carved = s("g", { class: "th-runes", fill: "none", stroke: "#e9cf86", "stroke-opacity": 0.2, "stroke-width": 3, "stroke-linecap": "round", "stroke-linejoin": "round" });
	const cols = Math.max(4, Math.round(w / 120));
	const rows = Math.max(3, Math.round(h / 120));
	for (let i = 0; i < cols; i++) {
		for (let j = 0; j < rows; j++) {
			const name = RUNE_NAMES[Math.floor(r() * RUNE_NAMES.length)];
			carved.append(s("path", { d: runePath(name, (i + 0.4 + r() * 0.2) * (w / cols), (j + 0.2 + r() * 0.3) * (h / rows), 54) }));
		}
	}
	art.append(carved);
	g.append(art);
	g.append(bands(w, h, m, knotBand));
	// A triple-circle knot in every corner.
	const inset = Math.min(5, m / 4);
	const c = Math.max(8, m / 2 - 1);
	for (const [x, y] of [[inset + c + 1, inset + c + 1], [w - inset - c - 1, inset + c + 1], [w - inset - c - 1, h - inset - c - 1], [inset + c + 1, h - inset - c - 1]]) {
		const knot = s("g", { transform: `translate(${x} ${y})` });
		knot.append(s("circle", { r: c, fill: "#0b1d13", stroke: "#d9b45a", "stroke-width": 2 }));
		for (let k = 0; k < 3; k++) {
			const a = (Math.PI * 2 * k) / 3 - Math.PI / 2;
			knot.append(s("circle", { cx: f1(Math.cos(a) * c * 0.38), cy: f1(Math.sin(a) * c * 0.38), r: f1(c * 0.5), fill: "none", stroke: "#d9b45a", "stroke-width": 1.6 }));
		}
		g.append(knot);
	}
	return g;
}

// ---- Southwestern Solstice ----------------------------------------------

/** A band of diamonds and zigzags in the colours of a Navajo blanket. */
function diamondBand(L, T) {
	const g = s("g", {});
	g.append(s("rect", { width: L, height: T, rx: 3, fill: "#2a120a" }));
	const cell = T * 1.25;
	const n = Math.max(1, Math.floor(L / cell));
	const step = L / n;
	const cols = ["#c4532b", "#2fc4b2", "#f2b92b", "#efe2c8"];
	for (let i = 0; i < n; i++) {
		const cx = step * (i + 0.5);
		const cy = T / 2;
		g.append(s("path", { d: polyPath([[cx, cy - T * 0.46], [cx + step * 0.46, cy], [cx, cy + T * 0.46], [cx - step * 0.46, cy]]), fill: cols[i % 4] }));
		g.append(s("path", { d: polyPath([[cx, cy - T * 0.25], [cx + step * 0.24, cy], [cx, cy + T * 0.25], [cx - step * 0.24, cy]]), fill: cols[(i + 2) % 4] }));
		g.append(s("rect", { x: f1(cx - 1.6), y: f1(cy - 1.6), width: 3.2, height: 3.2, fill: "#2a120a" }));
	}
	g.append(s("path", { d: `M0 1.5H${L}M0 ${T - 1.5}H${L}`, stroke: "#efe2c8", "stroke-width": 1.2, "stroke-opacity": 0.8 }));
	return g;
}

function solsticeFrame(w, h, m) {
	const clip = frameClip(w, h);
	const g = decorGroup("solstice", [clip.node]);
	const art = s("g", { "clip-path": `url(#${clip.id})` });
	art.append(s("rect", { width: w, height: h, fill: "url(#th-so-sky)" }));
	// A sun with rays, turning very slowly behind the board.
	const cx = w / 2;
	const cy = h * 0.5;
	const R = Math.max(w, h) * 0.75;
	const raysAt = s("g", { transform: `translate(${cx} ${cy})` });
	const rays = s("g", { class: "th-rays" });
	raysAt.append(rays);
	for (let i = 0; i < 24; i++) {
		const a0 = (Math.PI * 2 * i) / 24;
		const a1 = a0 + Math.PI / 24;
		rays.append(s("path", { d: `M0 0L${f1(Math.cos(a0) * R)} ${f1(Math.sin(a0) * R)}L${f1(Math.cos(a1) * R)} ${f1(Math.sin(a1) * R)}Z`, fill: "#fff1a0", "fill-opacity": 0.2 }));
	}
	art.append(raysAt);
	art.append(s("circle", { cx, cy, r: Math.min(w, h) * 0.34, fill: "url(#th-so-sun)" }));
	// Mesas along the bottom.
	const mesa = (x0, x1, top, base, fill) => polyPath([[x0, base], [x0 + (x1 - x0) * 0.06, top + 12], [x0 + (x1 - x0) * 0.12, top + 12], [x0 + (x1 - x0) * 0.16, top], [x0 + (x1 - x0) * 0.84, top], [x0 + (x1 - x0) * 0.88, top + 12], [x0 + (x1 - x0) * 0.94, top + 12], [x1, base]]);
	art.append(s("path", { d: mesa(-w * 0.1, w * 0.5, h - m * 1.9, h, 0), fill: "#7a2f18", "fill-opacity": 0.85 }));
	art.append(s("path", { d: mesa(w * 0.35, w * 1.1, h - m * 1.5, h, 0), fill: "#5b2210", "fill-opacity": 0.9 }));
	// Saguaro silhouettes.
	const cactus = (x, y, k) =>
		s("path", { d: "M0 0V-34M0 -16H-9V-26M0 -22H9V-30", transform: `translate(${f1(x)} ${f1(y)}) scale(${k})`, fill: "none", stroke: "#2a120a", "stroke-width": 4.5, "stroke-linecap": "round", "stroke-linejoin": "round" });
	art.append(cactus(m * 0.9, h - m * 0.35, 0.9), cactus(w - m * 1.1, h - m * 0.35, 1.15));
	g.append(art);
	g.append(bands(w, h, m, diamondBand));
	return g;
}

// ---- Moroccan Bazaar -----------------------------------------------------

/** A row of horseshoe arches with a star between them, in brass on deep blue. */
function archBand(L, T) {
	const g = s("g", {});
	g.append(s("rect", { width: L, height: T, rx: 3, fill: "#0b2240" }));
	g.append(s("rect", { width: L, height: T, rx: 3, fill: "none", stroke: "#f0cf7a", "stroke-width": 1.2 }));
	const cell = T * 1.5;
	const n = Math.max(1, Math.floor(L / cell));
	const step = L / n;
	for (let i = 0; i < n; i++) {
		const x = step * i;
		const a = T * 0.28;
		const archD = `M${f1(x + step * 0.22)} ${T - 2}V${f1(T * 0.5)}A${f1(a)} ${f1(a)} 0 1 1 ${f1(x + step * 0.78)} ${f1(T * 0.5)}V${T - 2}`;
		g.append(s("path", { d: archD, fill: i % 2 ? "#14506a" : "#1f56c8", stroke: "#f0cf7a", "stroke-width": 1.3, "stroke-linejoin": "round" }));
		g.append(s("path", { d: starPath(x + step * 0.5, T * 0.44, 8, T * 0.16, T * 0.08), fill: "#f0cf7a" }));
	}
	return g;
}

function bazaarFrame(w, h, m) {
	const clip = frameClip(w, h);
	const g = decorGroup("bazaar", [clip.node]);
	const art = s("g", { "clip-path": `url(#${clip.id})` });
	art.append(s("rect", { width: w, height: h, fill: "url(#th-bz-bg)" }));
	art.append(s("rect", { width: w, height: h, fill: "url(#th-bz-tile)" }));
	g.append(art);
	g.append(bands(w, h, m, archBand));
	return g;
}

// ---- Coral Abyss ---------------------------------------------------------

function abyssFrame(w, h, m) {
	const clip = frameClip(w, h);
	const g = decorGroup("abyss", [clip.node]);
	const art = s("g", { "clip-path": `url(#${clip.id})` });
	art.append(s("rect", { width: w, height: h, fill: "url(#th-ab-sea)" }));
	// Rays of light from the surface.
	const rays = s("g", { class: "th-godrays" });
	const r = rng(31);
	for (let i = 0; i < 7; i++) {
		const x = r() * w;
		const sp = 30 + r() * 60;
		rays.append(s("path", { d: polyPath([[x - 6, 0], [x + 10, 0], [x + sp + 70, h], [x + sp - 40, h]]), fill: "url(#th-ab-ray)", "fill-opacity": f1(0.5 + r() * 0.5) }));
	}
	art.append(rays);
	// Kelp swaying at both edges.
	const kelp = (x, k, delay) => {
		const at = s("g", { transform: `translate(${f1(x)} ${h})` });
		const blades = s("g", { class: "th-kelp", style: `animation-delay:${delay}s` });
		at.append(blades);
		for (let i = 0; i < 3; i++) {
			const hh = h * (0.28 + r() * 0.3);
			const dx = (i - 1) * 9 * k;
			blades.append(s("path", { d: `M${f1(dx)} 0C${f1(dx - 14)} ${f1(-hh * 0.3)} ${f1(dx + 14)} ${f1(-hh * 0.6)} ${f1(dx)} ${f1(-hh)}`, fill: "none", stroke: i % 2 ? "#1f9a6a" : "#157a55", "stroke-width": 7 - i, "stroke-linecap": "round", "stroke-opacity": 0.75 }));
		}
		return at;
	};
	art.append(kelp(m * 0.5, 1, 0), kelp(w - m * 0.5, -1, -2));
	// Bubbles that rise.
	for (let i = 0; i < 16; i++) {
		const rad = 2 + r() * 6;
		art.append(s("circle", { class: "th-bubble", style: `animation-duration:${f1(9 + r() * 9)}s;animation-delay:-${f1(r() * 14)}s`, cx: f1(r() * w), cy: f1(h * (0.2 + r() * 0.8)), r: f1(rad), fill: "url(#th-ab-bubble)", stroke: "#bff4ff", "stroke-opacity": 0.5, "stroke-width": 0.8 }));
	}
	g.append(art);
	// A rope round the board, and coral in the corners.
	const inset = Math.min(5, m / 4);
	g.append(
		s("rect", { x: inset + 1, y: inset + 1, width: w - 2 * inset - 2, height: h - 2 * inset - 2, rx: 12, fill: "none", stroke: "#8a6a3a", "stroke-width": Math.max(5, m / 3) }),
		s("rect", { x: inset + 1, y: inset + 1, width: w - 2 * inset - 2, height: h - 2 * inset - 2, rx: 12, fill: "none", stroke: "#e6cf96", "stroke-width": Math.max(5, m / 3) - 1.5, "stroke-dasharray": "5 6" })
	);
	const coral = (x, y, k) => {
		const c = s("g", { transform: `translate(${x} ${y}) scale(${k})`, fill: "none", "stroke-linecap": "round" });
		c.append(s("path", { d: "M0 0V-14M0 -8L-7 -16M0 -10L7 -20M-7 -16L-9 -22M7 -20L9 -27M0 -14L-1 -24", stroke: "#ff5a4a", "stroke-width": 3.4 }));
		c.append(s("circle", { cx: -9, cy: -23, r: 2, fill: "#ffb8a8" }), s("circle", { cx: 9, cy: -28, r: 2, fill: "#ffb8a8" }), s("circle", { cx: -1, cy: -25, r: 2, fill: "#ffb8a8" }));
		return c;
	};
	g.append(coral(inset + 14, h - inset - 6, 0.8), coral(w - inset - 14, h - inset - 6, 0.8), coral(inset + 14, inset + 30, 0.7), coral(w - inset - 14, inset + 30, 0.7));
	return g;
}

// ---- Precious Gemstones --------------------------------------------------

const GEM_COLORS = [
	["#d8173d", "#ff9aae", "#6a0620"], // ruby
	["#2455dc", "#9cbcff", "#0a1e6a"], // sapphire
	["#14a864", "#9cf0c4", "#04502c"], // emerald
	["#eef6ff", "#ffffff", "#9cc0e0"], // diamond
	["#9a58e0", "#e0c0ff", "#3e1672"], // amethyst
	["#f6c622", "#fff6b0", "#a8700a"], // citrine
];

/** A small cut gem centred on the origin, `r` across from the middle to a corner. */
function gemShape(r, [mid, hi, lo]) {
	const k = r * 0.62;
	const g = s("g", {});
	g.append(s("path", { d: polyPath([[0, -r], [k, -k], [r, 0], [k, k], [0, r], [-k, k], [-r, 0], [-k, -k]]), fill: mid, stroke: lo, "stroke-width": 0.9, "stroke-linejoin": "round" }));
	g.append(s("path", { d: polyPath([[0, -r * 0.5], [r * 0.36, -r * 0.36], [r * 0.5, 0], [r * 0.36, r * 0.36], [0, r * 0.5], [-r * 0.36, r * 0.36], [-r * 0.5, 0], [-r * 0.36, -r * 0.36]]), fill: hi, "fill-opacity": 0.6 }));
	g.append(s("path", { d: `M0 ${f1(-r)}V${f1(-r * 0.5)}M${f1(k)} ${f1(-k)}L${f1(r * 0.36)} ${f1(-r * 0.36)}M${f1(r)} 0H${f1(r * 0.5)}M${f1(k)} ${f1(k)}L${f1(r * 0.36)} ${f1(r * 0.36)}M0 ${f1(r)}V${f1(r * 0.5)}M${f1(-k)} ${f1(k)}L${f1(-r * 0.36)} ${f1(r * 0.36)}M${f1(-r)} 0H${f1(-r * 0.5)}M${f1(-k)} ${f1(-k)}L${f1(-r * 0.36)} ${f1(-r * 0.36)}`, stroke: lo, "stroke-opacity": 0.5, "stroke-width": 0.7, fill: "none" }));
	return g;
}

/** One side of the setting: a gold rail with gems set in it. */
function gemBand(L, T) {
	const g = s("g", {});
	g.append(s("rect", { width: L, height: T, rx: 3, fill: "url(#th-gm-gold)" }));
	g.append(s("rect", { x: 1, y: 1, width: L - 2, height: T - 2, rx: 2.5, fill: "none", stroke: "#fff0b8", "stroke-opacity": 0.55, "stroke-width": 0.9 }));
	const r = Math.max(3, T * 0.34);
	const n = Math.max(2, Math.floor(L / (r * 4.2)));
	const step = L / n;
	for (let i = 0; i < n; i++) {
		const at = s("g", { transform: `translate(${f1(step * (i + 0.5))} ${f1(T / 2)})` });
		at.append(gemShape(r, GEM_COLORS[i % GEM_COLORS.length]));
		g.append(at);
	}
	return g;
}

function gemsFrame(w, h, m) {
	const clip = frameClip(w, h);
	const g = decorGroup("gems", [clip.node]);
	const art = s("g", { "clip-path": `url(#${clip.id})` });
	art.append(s("rect", { width: w, height: h, fill: "url(#th-gm-bg)" }));
	g.append(art);
	g.append(bands(w, h, m, gemBand));
	return g;
}

// ---------------------------------------------------------------------------
// Marks on the stones
// ---------------------------------------------------------------------------

const WARM = new Set(["red", "yellow", "orange", "white", "brown"]);

/** A chord of a circle of radius r at height y: its half length. */
function chord(r, y) {
	return Math.sqrt(Math.max(0, r * r - y * y));
}

/**
 * The little drawings on a stone of the current theme: retro slits, a rune, a
 * sun or a stepped diamond, an eight-point star, a highlight. `size` is the
 * stone's size in board units; only round stones carry one.
 */
export function stoneMarks(shape, color, size) {
	const t = currentTheme();
	if (!t || shape !== "disc") return [];
	const r = size * 0.44;
	const warm = WARM.has(color);
	const st = t.stones[color];
	if (!st) return [];
	const light = color === "white" || color === "yellow" || color === "orange";
	const ink = light ? "rgba(40,20,5,0.62)" : "rgba(255,240,200,0.88)";
	const out = [];
	const mk = (children) => s("g", { class: "theme-mark", "pointer-events": "none" }, children);
	switch (t.id) {
		case "synthwave": {
			// A chrome shine, and the slits of the retro sun across the lower half.
			const slits = [];
			for (let i = 0; i < 4; i++) {
				const y = r * (0.18 + i * 0.2);
				const half = chord(r * 0.92, y);
				slits.push(s("line", { x1: f1(-half), y1: f1(y), x2: f1(half), y2: f1(y), stroke: "#12032b", "stroke-opacity": 0.5, "stroke-width": f1(1 + i * 0.9) }));
			}
			out.push(mk([s("ellipse", { cx: f1(-r * 0.3), cy: f1(-r * 0.4), rx: f1(r * 0.4), ry: f1(r * 0.17), fill: "#ffffff", "fill-opacity": 0.55, transform: `rotate(-28 ${f1(-r * 0.3)} ${f1(-r * 0.4)})` }), ...slits]));
			break;
		}
		case "celtic": {
			const name = { red: "fehu", yellow: "sowilo", orange: "kenaz", white: "othala", blue: "isa", black: "algiz", green: "berkano", purple: "mannaz", brown: "ansuz" }[color] || "fehu";
			const hh = r * 1.1;
			const d = runePath(name, -hh / 4, -hh / 2, hh);
			const sw = f1(Math.max(1.4, size * 0.055));
			out.push(
				mk([
					s("path", { d, transform: "translate(0.8 0.9)", fill: "none", stroke: light ? "rgba(255,255,255,0.4)" : "rgba(0,0,0,0.55)", "stroke-width": sw, "stroke-linecap": "round", "stroke-linejoin": "round" }),
					s("path", { d, fill: "none", stroke: ink, "stroke-width": sw, "stroke-linecap": "round", "stroke-linejoin": "round" }),
				])
			);
			break;
		}
		case "solstice": {
			if (warm) {
				// A sun: a ring with eight rays.
				const ray = [];
				for (let i = 0; i < 8; i++) {
					const a = (Math.PI * i) / 4;
					ray.push(s("line", { x1: f1(Math.cos(a) * r * 0.36), y1: f1(Math.sin(a) * r * 0.36), x2: f1(Math.cos(a) * r * 0.6), y2: f1(Math.sin(a) * r * 0.6) }));
				}
				out.push(mk(s("g", { fill: "none", stroke: ink, "stroke-width": f1(Math.max(1.3, size * 0.05)), "stroke-linecap": "round" }, [s("circle", { r: f1(r * 0.22) }), ...ray])));
			} else {
				// A stepped diamond, the way blankets weave it.
				const k = r * 0.58;
				const d = polyPath([[0, -k], [k * 0.5, -k * 0.5], [k, 0], [k * 0.5, k * 0.5], [0, k], [-k * 0.5, k * 0.5], [-k, 0], [-k * 0.5, -k * 0.5]]);
				out.push(mk([s("path", { d, fill: "none", stroke: ink, "stroke-width": f1(Math.max(1.3, size * 0.05)), "stroke-linejoin": "miter" }), s("rect", { x: f1(-k * 0.16), y: f1(-k * 0.16), width: f1(k * 0.32), height: f1(k * 0.32), fill: ink })]));
			}
			break;
		}
		case "bazaar": {
			const d = starPath(0, 0, 8, r * 0.62, r * 0.4);
			out.push(mk([s("path", { d, fill: light ? "rgba(40,20,5,0.12)" : "rgba(255,240,200,0.14)", stroke: ink, "stroke-width": f1(Math.max(1.1, size * 0.04)), "stroke-linejoin": "round" }), s("circle", { r: f1(r * 0.12), fill: ink })]));
			break;
		}
		case "gems": {
			// A brilliant cut seen from above: the table, the star facets and a glint.
			const k = r * 0.55;
			const table = polyPath([0, 1, 2, 3, 4, 5, 6, 7].map((i) => [Math.cos((Math.PI * i) / 4 + Math.PI / 8) * k, Math.sin((Math.PI * i) / 4 + Math.PI / 8) * k]));
			const spokes = [0, 1, 2, 3, 4, 5, 6, 7]
				.map((i) => {
					const a = (Math.PI * i) / 4 + Math.PI / 8;
					return `M${f1(Math.cos(a) * k)} ${f1(Math.sin(a) * k)}L${f1(Math.cos(a) * r * 0.9)} ${f1(Math.sin(a) * r * 0.9)}`;
				})
				.join("");
			out.push(
				mk([
					s("path", { d: spokes, fill: "none", stroke: light ? "rgba(60,40,10,0.32)" : "rgba(255,255,255,0.3)", "stroke-width": 1 }),
					s("path", { d: table, fill: light ? "rgba(255,255,255,0.28)" : "rgba(255,255,255,0.14)", stroke: light ? "rgba(60,40,10,0.45)" : "rgba(255,255,255,0.5)", "stroke-width": 1 }),
					s("path", { d: starPath(-r * 0.36, -r * 0.4, 4, r * 0.2, r * 0.045), fill: "#ffffff", "fill-opacity": 0.9 }),
				])
			);
			break;
		}
		case "abyss": {
			// A specular shine, and scales on the warm stones or ripples on the cold ones.
			const detail = warm
				? [-0.25, 0.05, 0.35].map((y) => s("path", { d: `M${f1(-r * 0.5)} ${f1(r * y + r * 0.25)}Q${f1(-r * 0.25)} ${f1(r * y - r * 0.05)} 0 ${f1(r * y + r * 0.25)}Q${f1(r * 0.25)} ${f1(r * y - r * 0.05)} ${f1(r * 0.5)} ${f1(r * y + r * 0.25)}`, fill: "none", stroke: "#ffffff", "stroke-opacity": 0.28, "stroke-width": 1.2 }))
				: [0.3, 0.55].map((k) => s("circle", { r: f1(r * k), fill: "none", stroke: "#bff4ff", "stroke-opacity": 0.35 - k * 0.2, "stroke-width": 1.2 }));
			out.push(mk([...detail, s("ellipse", { cx: f1(-r * 0.32), cy: f1(-r * 0.4), rx: f1(r * 0.3), ry: f1(r * 0.16), fill: "#ffffff", "fill-opacity": 0.6, transform: `rotate(-30 ${f1(-r * 0.32)} ${f1(-r * 0.4)})` })]));
			break;
		}
	}
	return out;
}
