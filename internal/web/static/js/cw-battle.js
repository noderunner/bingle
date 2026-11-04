// The theatre of a battle in Cthulhu Wars: the map zooms in on the area and darkens around
// it, the two sides face each other, the dice are thrown, and a red X falls on each unit that
// dies. Then it zooms out again, for the Pains to retreat on the map.
//
// It is drawn on a layer of its own over a copy of the map, from nothing but snapshots of the
// position, so the page and the game never wait for it, and a battle in the history is played
// again from the same snapshots. Everything the game needs of it goes through `ctx`.

import { h, s, clear } from "./util.js";

const KILL = 6;
const PAIN = 4;
const face = (v) => (v === KILL ? "kill" : v >= PAIN ? "pain" : "miss");
const FACE_MARK = { kill: "☠", pain: "✹", miss: "" };
const STRIKE_MS = 330;
const wait = (ms) => new Promise((done) => setTimeout(done, ms));
const ease = (t) => (t < 0.5 ? 4 * t * t * t : 1 - (-2 * t + 2) ** 3 / 2);

// The rotation of a cube that brings each of its six faces to the front.
const TO_FRONT = { 1: [0, 0], 2: [0, -90], 3: [0, 180], 4: [0, 90], 5: [-90, 0], 6: [90, 0] };

/** A die: a cube with the faces of the game's dice, which tumbles and lands on `value`. */
function cube(value, delay) {
	const f = face(value);
	const box = h("div", { class: `bt-die ${f}`, "data-value": value, title: f === "kill" ? "Kill" : f === "pain" ? "Pain" : "Nothing" });
	const inner = h("div", { class: "bt-cube" });
	for (let n = 1; n <= 6; n++) inner.append(h("span", { class: `bt-face n${n} ${face(n)}` }, FACE_MARK[face(n)]));
	const [rx, ry] = TO_FRONT[value] || [0, 0];
	inner.style.setProperty("--rx", `${rx}deg`);
	inner.style.setProperty("--ry", `${ry}deg`);
	inner.style.animationDelay = `${delay}ms`;
	box.append(inner);
	return box;
}

/** The stylized X of a unit that dies: two strokes drawn on, one after the other. */
function redX() {
	const svg = s("svg", { class: "bt-x", viewBox: "0 0 40 40", "aria-hidden": "true" });
	svg.append(s("path", { d: "M7 8 Q20 19 33 33" }), s("path", { d: "M33 7 Q20 20 8 33" }));
	return svg;
}

export function createBattleTheatre(ctx) {
	let stage = null;
	let key = null; // the battle on the stage
	let queue = [];
	let working = false;
	let closed = new Set(); // battles the player sent away, which stay away
	let roster = new Map(); // "seat|type" -> how many stood here when last counted, the most seen
	let played = false; // the dice have been thrown
	let replay = false;
	let seen = true; // the player saw the move that brought this battle, so its dice are thrown for them
	let before = null; // the units of the two sides just before the battle was declared
	let frame = 0;
	let endZoom = null; // ends the zoom under way, so that a show taken down does not wait for it for ever
	let baseBox = null;
	let cloneSvg = null;
	let last = null;
	let area = "";
	let token = 0; // a new show ends the old one's pauses

	function place() {
		const board = ctx.board();
		if (!stage || !board) return;
		const r = board.getBoundingClientRect();
		Object.assign(stage.style, { left: `${r.left}px`, top: `${r.top}px`, width: `${r.width}px`, height: `${r.height}px` });
	}

	/** Moves the map's viewBox to box over ms; resolves when it is there. */
	function zoom(to, ms) {
		const svg = cloneSvg;
		if (!svg) return Promise.resolve();
		const from = svg.getAttribute("viewBox").split(" ").map(Number);
		const t0 = performance.now();
		cancelAnimationFrame(frame);
		endZoom?.();
		// A page out of sight gets no frames: the show would stop until the player came back.
		if (document.hidden) {
			svg.setAttribute("viewBox", to.join(" "));
			return Promise.resolve();
		}
		return new Promise((done) => {
			endZoom = done;
			const step = (now) => {
				const t = Math.min(1, (now - t0) / ms);
				const k = ease(t);
				svg.setAttribute("viewBox", from.map((v, i) => v + (to[i] - v) * k).join(" "));
				if (t < 1) {
					frame = requestAnimationFrame(step);
				} else {
					endZoom = null;
					done();
				}
			};
			frame = requestAnimationFrame(step);
		});
	}

	/** The copy of the map: the area's own units gone (they stand in the arena), the rest dark. */
	function cloneMap(board, sides) {
		const svg = board.cloneNode(true);
		svg.removeAttribute("id");
		svg.classList.add("bt-map");
		const fighters = new Set(sides.map((x) => ctx.colorOf(x.seat)));
		for (const el of svg.querySelectorAll(`.token[data-position="${area}"]`)) if (fighters.has(el.getAttribute("data-color"))) el.remove();
		for (const el of svg.querySelectorAll(`.gate-chip[data-position="${area}"] .gate-holder`)) el.remove();
		baseBox = (svg.getAttribute("viewBox") || "0 0 1000 490").split(" ").map(Number);
		return svg;
	}

	/** The dark around the area, as a mask with the shape of the area cut out of it, and its glow. */
	function darken(svg, seatColor) {
		const cell = svg.querySelector(`.cell.area[data-position="${area}"]`);
		const shapes = cell ? [...cell.querySelectorAll("path, polygon")] : [];
		if (!shapes.length) return null;
		const id = "bt-hole";
		const mask = s("mask", { id });
		mask.append(s("rect", { x: baseBox[0], y: baseBox[1], width: baseBox[2], height: baseBox[3], fill: "#fff" }));
		const glow = s("g", { class: "bt-glow", style: `--bt-glow: ${seatColor}` });
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
		svg.append(
			s("defs", {}, mask),
			s("rect", { class: "bt-dark", x: baseBox[0], y: baseBox[1], width: baseBox[2], height: baseBox[3], mask: `url(#${id})` }),
			glow
		);
		return cell;
	}

	/** Where to look: the area, with room around it for the two sides to stand. */
	function aimAt(svg, cell) {
		// An ocean that wraps the map has a piece at each edge: look at the bigger one.
		const pieces = [...cell.querySelectorAll("polygon.area-bg")].map((p) => p.getBBox());
		const box = pieces.sort((a, b) => b.width * b.height - a.width * a.height)[0] || cell.getBBox();
		const aspect = baseBox[2] / baseBox[3];
		// The area stands in the middle, between the two sides, even at the edge of the map.
		let w = Math.max(box.width * 2.6, baseBox[2] * 0.34);
		let h2 = w / aspect;
		if (h2 < box.height * 1.7) {
			h2 = box.height * 1.7;
			w = h2 * aspect;
		}
		const x = box.x + box.width / 2 - w / 2;
		const y = box.y + box.height / 2 - h2 / 2;
		return [x, y, w, h2];
	}

	function sideNode(side, i) {
		const node = h(
			"div",
			{ class: `bt-side ${i ? "right" : "left"}`, "data-seat": side.seat, style: `--bt-ink: ${ctx.ink(side.seat)}` },
			h("div", { class: "bt-who" }, ctx.glyph(side.seat), h("span", {}, side.name)),
			h("div", { class: "bt-units" }),
			h("div", { class: "bt-tally", "aria-live": "polite" })
		);
		return node;
	}

	function build(snap) {
		const board = ctx.board();
		if (!board) return false;
		area = snap.bt.area;
		roster = new Map();
		played = false;
		last = null;
		stage = h(
			"div",
			{ class: "battle-stage" + (replay ? " replay" : ""), id: "battle-stage", role: "group", "aria-label": `Battle in ${snap.bt.area_name}` },
			h("div", { class: "bt-map-host" }),
			h("div", { class: "bt-embers", "aria-hidden": "true" }),
			h("div", { class: "bt-banner" }, h("span", { class: "bt-crest", "aria-hidden": "true" }, "⚔"), h("span", { class: "bt-title" }, `Battle in ${snap.bt.area_name}`), h("span", { class: "bt-step", id: "battle-step" })),
			h(
				"div",
				{ class: "bt-arena" },
				sideNode(snap.bt.sides[0], 0),
				h(
					"div",
					{ class: "bt-field" },
					h("div", { class: "bt-vs", "aria-hidden": "true" }, "VS"),
					h("div", { class: "bt-tray" }, ...snap.bt.sides.map((side, i) => h("div", { class: `bt-dice ${i ? "right" : "left"}`, "data-seat": side.seat, style: `--bt-ink: ${ctx.ink(side.seat)}` })))
				),
				sideNode(snap.bt.sides[1], 1)
			),
			h("button", { type: "button", class: "bt-skip", id: "battle-skip", title: "Show the map (Esc)", onclick: dismiss }, "Skip ✕")
		);
		for (let i = 0; i < 16; i++) {
			const ember = h("i", {});
			ember.style.left = `${Math.random() * 100}%`;
			ember.style.animationDelay = `${Math.random() * 4}s`;
			ember.style.animationDuration = `${3 + Math.random() * 3}s`;
			stage.querySelector(".bt-embers").append(ember);
		}
		document.body.append(stage);
		place();
		cloneSvg = cloneMap(board, snap.bt.sides);
		stage.querySelector(".bt-map-host").append(cloneSvg);
		const cell = darken(cloneSvg, ctx.ink(snap.bt.sides[0].seat));
		return cell;
	}

	/** Puts on the stage any unit that is not there yet: all of them, the first time. */
	function placeUnits(snap, animate) {
		for (const side of snap.bt.sides) {
			const host = stage.querySelector(`.bt-side[data-seat="${side.seat}"] .bt-units`);
			if (!host) continue;
			let n = host.children.length;
			for (const [k, many] of roster) {
				const [seat, type] = k.split("|");
				if (Number(seat) !== side.seat) continue;
				for (let i = 0; i < many; i++) {
					if (host.querySelector(`[data-unit="${type}#${i}"]`)) continue;
					const node = h(
						"button",
						{ type: "button", class: "bt-unit", "data-unit": `${type}#${i}`, "data-piece": type, title: ctx.label(type), "aria-label": ctx.label(type), onclick: (ev) => { if (!ev.currentTarget.classList.contains("dead")) ctx.onUnit(area, type, ctx.colorOf(side.seat)); } },
						ctx.icon(type, ctx.colorOf(side.seat), 40)
					);
					if (animate) node.style.animationDelay = `${n++ * 70}ms`;
					host.append(node);
				}
			}
		}
	}

	/** Marks what each unit on the stage has become; the units that have just died, in `fallen`. */
	function reckon(snap) {
		const fallen = [];
		for (const side of snap.bt.sides) {
			// When the battle is over a unit may be gone because it retreated: only so many died.
			let kills = snap.bt.resolved ? side.kills : Infinity;
			for (const [k, many] of roster) {
				const [seat, type] = k.split("|");
				if (Number(seat) !== side.seat) continue;
				const stands = snap.units.filter((u) => u.seat === side.seat && u.type === type).length;
				const pained = snap.pained.filter((u) => u.seat === side.seat && u.type === type).length;
				for (let i = 0; i < many; i++) {
					const node = stage.querySelector(`.bt-side[data-seat="${side.seat}"] [data-unit="${type}#${i}"]`);
					if (!node) continue;
					// The first of a type stand; those after them are the ones gone.
					const gone = i >= stands;
					const dead = gone && kills > 0;
					if (gone) kills--;
					if (dead && !node.classList.contains("dead")) fallen.push(node);
					node.classList.toggle("pained", !dead && (gone || i < pained));
					node.classList.toggle("can-choose", !replay && !gone && ctx.unitOk(area, type, ctx.colorOf(side.seat)));
				}
			}
		}
		return fallen;
	}

	/** The X falls on each unit that has just died, one at a time. */
	async function strike(fallen, mine) {
		for (const node of fallen) {
			if (mine !== token || !stage) return;
			node.classList.add("dead");
			node.classList.remove("can-choose", "pained");
			node.append(redX());
			// The enemy's die that did it has done its work.
			const seat = node.closest(".bt-side").dataset.seat;
			stage.querySelector(`.bt-dice:not([data-seat="${seat}"]) .bt-die.kill:not(.spent)`)?.classList.add("spent");
			stage.classList.remove("bt-shake");
			void stage.offsetWidth;
			stage.classList.add("bt-shake");
			await wait(STRIKE_MS);
		}
	}

	/** Counts in what now stands in the area: a unit that arrives joins its side. */
	function count(snap) {
		const here = new Map();
		for (const u of before && !roster.size ? before : []) here.set(`${u.seat}|${u.type}`, (here.get(`${u.seat}|${u.type}`) || 0) + 1);
		for (const [k, n] of here) roster.set(k, n);
		here.clear();
		for (const u of snap.units) here.set(`${u.seat}|${u.type}`, (here.get(`${u.seat}|${u.type}`) || 0) + 1);
		for (const [k, n] of here) roster.set(k, Math.max(roster.get(k) || 0, n));
	}

	function tally(side) {
		const parts = [];
		if (side.kills) parts.push(`${side.kills} Kill${side.kills === 1 ? "" : "s"}`);
		if (side.pains) parts.push(`${side.pains} Pain${side.pains === 1 ? "" : "s"}`);
		return parts.length ? `Takes ${parts.join(" · ")}` : "Unharmed";
	}

	/** The throw: each side's dice tumble in, and what they did lands on the other side. */
	async function throwDice(snap, mine, instant) {
		played = true;
		let longest = 0;
		for (const side of snap.bt.sides) {
			const host = stage.querySelector(`.bt-dice[data-seat="${side.seat}"]`);
			clear(host);
			(side.dice || []).forEach((v, i) => {
				const delay = instant ? 0 : i * 90 + (side === snap.bt.sides[1] ? 250 : 0);
				longest = Math.max(longest, delay);
				const die = cube(v, delay);
				if (instant) die.classList.add("landed");
				else setTimeout(() => die.classList.add("landed"), delay + 900);
				host.append(die);
			});
		}
		if (instant) {
			for (const side of snap.bt.sides) stage.querySelector(`.bt-side[data-seat="${side.seat}"] .bt-tally`).textContent = tally(side);
			return;
		}
		stage.classList.add("bt-rolling");
		await wait(longest + 1000);
		if (mine !== token || !stage) return;
		stage.classList.remove("bt-rolling");
		stage.classList.add("bt-flash");
		for (const side of snap.bt.sides) {
			const t = stage.querySelector(`.bt-side[data-seat="${side.seat}"] .bt-tally`);
			t.textContent = tally(side);
			t.classList.add("in");
			t.classList.toggle("hurt", side.kills > 0);
		}
		await wait(replay ? 800 : 500);
		if (stage) stage.classList.remove("bt-flash");
	}

	function stepWords(snap) {
		const bt = snap.bt;
		if (bt.step === "assign") return "Choosing the casualties";
		if (bt.step === "pre") return bt.sides.some((x) => x.dice && x.dice.length) ? "" : "Pre-battle";
		return "";
	}

	const isOver = (snap) => snap.bt.resolved || snap.bt.step === "retreat" || snap.bt.step === "post";

	async function show(snap, mine) {
		const first = !stage;
		if (first) {
			const cell = build(snap);
			if (cell === false) return;
			count(snap);
			placeUnits(snap, true);
			reckon(before ? { ...snap, units: before, pained: [] } : snap);
			if (cell) {
				await wait(30);
				stage?.classList.add("in");
				await zoom(aimAt(cloneSvg, cell), 800);
			} else {
				stage.classList.add("in");
			}
			if (mine !== token || !stage) return;
			stage.classList.add("bt-met");
			await wait(replay ? 450 : 350);
			if (mine !== token || !stage) return;
		}
		count(snap);
		placeUnits(snap, false);
		stage.querySelector("#battle-step").textContent = stepWords(snap);
		const rolled = snap.bt.sides.some((x) => x.dice && x.dice.length);
		if (rolled && !played) await throwDice(snap, mine, !seen);
		if (mine !== token || !stage) return;
		await strike(reckon(snap), mine);
		last = snap;
		if (replay && !isOver(snap)) await wait(650);
	}

	async function outro(mine) {
		if (!stage) return;
		await wait(1100);
		if (mine !== token || !stage) return;
		stage.classList.add("out");
		await zoom(baseBox, 650);
		if (mine !== token) return;
		remove();
	}

	function remove() {
		cancelAnimationFrame(frame);
		endZoom?.();
		endZoom = null;
		stage?.remove();
		stage = null;
		cloneSvg = null;
		last = null;
		before = null;
		replay = false;
	}

	async function pump(mine) {
		if (working) return;
		working = true;
		try {
			while (queue.length && mine === token) {
				const snap = queue.shift();
				await show(snap, mine);
				if (isOver(snap) && mine === token) {
					closed.add(key);
					queue = [];
					await outro(mine);
					break;
				}
			}
		} finally {
			working = false;
		}
		if (mine !== token) pump(token);
	}

	/** The player has had enough of this battle's show. */
	function dismiss() {
		if (key) closed.add(key);
		close();
	}

	/** Takes the show down at once. */
	function close() {
		token++;
		queue = [];
		key = null;
		remove();
	}

	return {
		/**
		 * A position of a battle being played: the show starts, or goes on. `unitsBefore` is who stood
		 * there before it was declared, and `wasSeen` whether the player has just watched it happen.
		 */
		feed(battleKey, snap, unitsBefore, wasSeen) {
			if (closed.has(battleKey)) return;
			if (key !== battleKey || replay) {
				if (stage) close();
				key = battleKey;
				before = unitsBefore;
				seen = wasSeen;
			}
			queue.push(snap);
			pump(token);
		},
		/** Plays a battle again from its positions. */
		play(battleKey, snaps, unitsBefore) {
			close();
			closed.delete(battleKey);
			key = battleKey;
			replay = true;
			seen = true;
			before = unitsBefore;
			queue = [...snaps];
			pump(token);
		},
		/** Keeps the layer over the map, or lets the player get at the map underneath. */
		hide(on) {
			if (stage) stage.hidden = on || !ctx.board();
		},
		/** What can be chosen has changed. */
		refresh() {
			if (stage && last && !replay) reckon(last);
			place();
		},
		close,
		dismiss,
		isOpen: () => !!stage,
		isReplay: () => replay,
	};
}
