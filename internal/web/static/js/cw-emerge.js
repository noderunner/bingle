// A Great Old One's first awakening, played on the map: the map zooms in on the area and dims, and it
// comes out of the dark, or, for Cthulhu, out of the sea. A first awakening is what the game's history
// says "<name> Emerges" of, and that badge plays it again. Like the battle show it is a layer over a
// copy of the map, and the game never waits for it.

import { h } from "./util.js";
import { aimAtArea, copyMap, createZoom, darkenAround, placeStage } from "./cw-map-stage.js";
import { cthulhuScene, CTHULHU_PEAK, CTHULHU_SECONDS } from "./cw-cthulhu-art.js";
import { kingScene, KING_PEAK, KING_SECONDS } from "./cw-king-art.js";
import { nyarlathotepScene, NYARLATHOTEP_PEAK, NYARLATHOTEP_SECONDS } from "./cw-nyarlathotep-art.js";
import { hasturScene, HASTUR_PEAK, HASTUR_SECONDS } from "./cw-hastur-art.js";
import { shubScene, SHUB_PEAK, SHUB_SECONDS } from "./cw-shub-art.js";

// The Great Old Ones with art of their own, by unit type.
const SCENES = {
	cthulhu: { make: cthulhuScene, seconds: CTHULHU_SECONDS, peak: CTHULHU_PEAK },
	nyarlathotep: { make: nyarlathotepScene, seconds: NYARLATHOTEP_SECONDS, peak: NYARLATHOTEP_PEAK },
	"shub-niggurath": { make: shubScene, seconds: SHUB_SECONDS, peak: SHUB_PEAK },
	"king-in-yellow": { make: kingScene, seconds: KING_SECONDS, peak: KING_PEAK },
	hastur: { make: hasturScene, seconds: HASTUR_SECONDS, peak: HASTUR_PEAK },
};

/** The Great Old Ones with a show, for the Animation Tester. */
export const emergenceTypes = () => Object.keys(SCENES);

/** Whether a Great Old One has a show of its own. */
export const hasEmergence = (type) => !!SCENES[type];

// How long the map takes to zoom out again, and how long a still picture stays up.
const ZOOM_OUT_MS = 500;
const STILL_MS = 3000;

export function createEmergence(ctx) {
	let stage = null;
	let frame = 0;
	let timer = 0;
	let token = 0;
	const zoom = createZoom();

	function close() {
		token++;
		cancelAnimationFrame(frame);
		clearTimeout(timer);
		zoom.stop();
		stage?.remove();
		stage = null;
	}

	/** Plays a show: type is the unit, seat its cult, area and areaName where. still: one picture, no motion. */
	async function play({ type, name, seat, area, areaName, still }) {
		const scene = SCENES[type];
		const board = ctx.board();
		if (!scene || !board) return;
		close();
		const mine = token;
		const art = scene.make();
		const title = h(
			"div",
			{ class: "em-title", id: "emerge-title", style: `--bt-ink: ${ctx.ink(seat)}` },
			ctx.glyph(seat),
			h("span", { class: "em-name" }, `${name} Emerges`),
			areaName ? h("span", { class: "em-where" }, `in ${areaName}`) : null
		);
		const host = h("div", { class: "bt-map-host" });
		stage = h(
			"div",
			{ class: "battle-stage emerge-stage", id: "emerge-stage", role: "img", "aria-label": `${name} emerges${areaName ? ` in ${areaName}` : ""}`, "data-goo": type },
			host,
			art.el,
			title,
			h("button", { type: "button", class: "bt-skip", id: "emerge-skip", title: "Show the map (Esc)", onclick: close }, "Skip ✕")
		);
		document.body.append(stage);
		placeStage(stage, ctx);
		const { svg, base } = copyMap(board);
		// The art takes the place of its token.
		for (const el of svg.querySelectorAll(`.token[data-position="${area}"][data-piece="${type}"]`)) el.remove();
		host.append(svg);
		const cell = darkenAround(svg, base, area, ctx.ink(seat));
		art.seek(still ? scene.peak : 0);
		await new Promise((done) => setTimeout(done, 30));
		if (mine !== token || !stage) return;
		stage.classList.add("in");
		if (still) {
			if (cell) svg.setAttribute("viewBox", aimAtArea(cell, base).join(" "));
			title.classList.add("on");
			timer = setTimeout(close, STILL_MS);
			return;
		}
		if (cell) zoom.to(svg, aimAtArea(cell, base), 900);
		const t0 = performance.now();
		const step = (now) => {
			if (mine !== token || !stage) return;
			// A page out of sight gets no frames: there is nothing to come back to.
			const t = (now - t0) / 1000;
			art.seek(Math.min(t, scene.seconds));
			if (t >= 4.5) title.classList.add("on");
			if (t >= scene.seconds - 0.8) title.classList.remove("on");
			if (t >= scene.seconds - 0.6) stage.classList.add("em-leaving");
			if (t < scene.seconds) {
				frame = requestAnimationFrame(step);
				return;
			}
			stage.classList.add("out");
			zoom.to(svg, base, ZOOM_OUT_MS).then(() => {
				if (mine === token) close();
			});
		};
		frame = requestAnimationFrame(step);
	}

	return {
		play,
		close,
		isOpen: () => !!stage,
		refresh() {
			if (stage) placeStage(stage, ctx);
		},
	};
}
