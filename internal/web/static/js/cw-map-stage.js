// What the shows on the map of Cthulhu Wars have in common: a layer over a copy of the map (js/cw-battle.js,
// js/cw-emerge.js), which zooms in on an area and darkens around it.

import { s } from "./util.js";

export const ease = (t) => (t < 0.5 ? 4 * t * t * t : 1 - (-2 * t + 2) ** 3 / 2);

/**
 * Lays a stage over the map. ctx says whether this is a phone (phone), held upright (upright), and
 * where the map on the page is (onPage).
 */
export function placeStage(stage, ctx) {
	const box = stage.style;
	stage.classList.toggle("phone", ctx.phone());
	if (ctx.phone()) {
		// A phone gives the whole screen to the show. Held upright, the show lies on its side,
		// and turning the phone straightens it: a page can only ask for a real turn of the screen
		// in full screen and after a tap, and a battle often comes from another player's move.
		const [w, h2] = [window.innerWidth, window.innerHeight];
		const up = ctx.upright();
		Object.assign(box, { left: `${up ? w : 0}px`, top: "0", width: `${up ? h2 : w}px`, height: `${up ? w : h2}px`, transform: up ? "rotate(90deg)" : "" });
		return;
	}
	const board = ctx.onPage();
	if (!board) return;
	const r = board.getBoundingClientRect();
	Object.assign(box, { left: `${r.left}px`, top: `${r.top}px`, width: `${r.width}px`, height: `${r.height}px`, transform: "" });
}

/** A copy of the map to play on, and its whole view. */
export function copyMap(board) {
	const svg = board.cloneNode(true);
	svg.removeAttribute("id");
	svg.classList.add("bt-map");
	return { svg, base: (svg.getAttribute("viewBox") || "0 0 1000 490").split(" ").map(Number) };
}

/** The dark around an area, as a mask with its shape cut out, and a glow on its edge. Returns its cell. */
export function darkenAround(svg, base, area, color) {
	const cell = svg.querySelector(`.cell.area[data-position="${area}"]`);
	const shapes = cell ? [...cell.querySelectorAll("path, polygon")] : [];
	if (!shapes.length) return null;
	const id = "bt-hole";
	const mask = s("mask", { id });
	mask.append(s("rect", { x: base[0], y: base[1], width: base[2], height: base[3], fill: "#fff" }));
	const glow = s("g", { class: "bt-glow", style: `--bt-glow: ${color}` });
	for (const shape of shapes) {
		const hole = shape.cloneNode(true);
		hole.removeAttribute("class");
		hole.setAttribute("fill", "#000");
		hole.setAttribute("stroke", "#000");
		mask.append(hole);
		const edge = shape.cloneNode(true);
		edge.removeAttribute("class");
		glow.append(edge);
	}
	svg.append(s("defs", {}, mask), s("rect", { class: "bt-dark", x: base[0], y: base[1], width: base[2], height: base[3], mask: `url(#${id})` }), glow);
	return cell;
}

/** Where to look: an area, in the middle, with room around it, even at the edge of the map. */
export function aimAtArea(cell, base) {
	// An ocean that wraps the map has a piece at each edge: look at the bigger one.
	const pieces = [...cell.querySelectorAll("polygon.area-bg")].map((p) => p.getBBox());
	const box = pieces.sort((a, b) => b.width * b.height - a.width * a.height)[0] || cell.getBBox();
	const aspect = base[2] / base[3];
	let w = Math.max(box.width * 2.6, base[2] * 0.34);
	let h2 = w / aspect;
	if (h2 < box.height * 1.7) {
		h2 = box.height * 1.7;
		w = h2 * aspect;
	}
	return [box.x + box.width / 2 - w / 2, box.y + box.height / 2 - h2 / 2, w, h2];
}

/** Moves a map's view. stop() ends a move under way, so that nothing waits for it for ever. */
export function createZoom() {
	let frame = 0;
	let finish = null;
	const stop = () => {
		cancelAnimationFrame(frame);
		finish?.();
		finish = null;
	};
	return {
		stop,
		to(svg, box, ms) {
			if (!svg) return Promise.resolve();
			const from = svg.getAttribute("viewBox").split(" ").map(Number);
			const t0 = performance.now();
			stop();
			// A page out of sight gets no frames: the show would stop until the player came back.
			if (document.hidden) {
				svg.setAttribute("viewBox", box.join(" "));
				return Promise.resolve();
			}
			return new Promise((done) => {
				finish = done;
				const step = (now) => {
					const t = Math.min(1, (now - t0) / ms);
					const k = ease(t);
					svg.setAttribute("viewBox", from.map((v, i) => v + (box[i] - v) * k).join(" "));
					if (t < 1) {
						frame = requestAnimationFrame(step);
					} else {
						finish = null;
						done();
					}
				};
				frame = requestAnimationFrame(step);
			});
		},
	};
}
