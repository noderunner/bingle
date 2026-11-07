// The theatre of a battle in Cthulhu Wars: the map zooms in on the area and darkens around
// it, the two sides face each other, the dice are thrown, and a red X falls on each unit that
// dies. Abilities that come before and after the dice are listed, and the show waits on each
// one that needs a choice. Then it zooms out again, for the Pains to retreat on the map.
//
// It is drawn on a layer of its own over a copy of the map, from nothing but snapshots of the
// position, so the page and the game never wait for it, and a battle in the history is played
// again from the same snapshots. Everything the game needs of it goes through `ctx`.

import { h, s, clear } from "/static/js/util.js";
import { cssColor } from "/static/js/render.js";
import { aimAtArea, copyMap, createZoom, darkenAround, placeStage } from "./cw-map-stage.js";

const KILL = 6;
const PAIN = 4;
const face = (v) => (v === KILL ? "kill" : v >= PAIN ? "pain" : "miss");
const STRIKE_MS = 330;
const ABILITY_MS = 380;
const UNIT_REM = 5.6;
const MIN_UNIT_REM = 1.6;
const POOL_MS = 1200; // the longest the dice of a pool take to arrive
const wait = (ms) => new Promise((done) => setTimeout(done, ms));

// The rotation of a cube that brings each of its six faces to the front.
const TO_FRONT = { 1: [0, 0], 2: [0, -90], 3: [0, 180], 4: [0, 90], 5: [-90, 0], 6: [90, 0] };

/** The points of a burst of eight rays, around the middle of a mark. */
const BURST = Array.from({ length: 16 }, (_, i) => {
	const r = i % 2 ? 3.7 : 8.6;
	const a = (i * Math.PI) / 8;
	return `${(12 + r * Math.sin(a)).toFixed(2)},${(12 - r * Math.cos(a)).toFixed(2)}`;
}).join(" ");

/**
 * What a face did. Each sits on a dark disc with a pale ring, whatever colour the die is: a yellow,
 * a white and a red cult's dice all have to show it at a glance, even at the size of a phone's.
 */
function faceMark(f) {
	if (f === "miss") return null;
	const parts =
		f === "kill"
			? [
					s("path", { class: "bt-skull", d: "M12 4.6c-3.9 0-6.5 2.7-6.5 6 0 2.2 1 3.9 2.6 4.9v2.3c0 .6.4 1 1 1h5.8c.6 0 1-.4 1-1v-2.3c1.6-1 2.6-2.7 2.6-4.9 0-3.3-2.6-6-6.5-6z" }),
					s("circle", { class: "bt-socket", cx: 9.3, cy: 11, r: 1.9 }),
					s("circle", { class: "bt-socket", cx: 14.7, cy: 11, r: 1.9 }),
					s("path", { class: "bt-socket", d: "M12 13.2l1.1 2h-2.2z" }),
				]
			: [s("polygon", { class: "bt-burst", points: BURST })];
	return s("svg", { class: `bt-mark ${f}`, viewBox: "0 0 24 24", "aria-hidden": "true" }, s("circle", { class: "bt-disc", cx: 12, cy: 12, r: 11 }), ...parts);
}

/** A die: a cube with the faces of the game's dice, which tumbles and lands on `value`. */
function cube(value) {
	const f = face(value);
	const box = h("div", { class: `bt-die ${f}`, "data-value": value, title: f === "kill" ? "Kill" : f === "pain" ? "Pain" : "Nothing" });
	const inner = h("div", { class: "bt-cube" });
	for (let n = 1; n <= 6; n++) inner.append(h("span", { class: `bt-face n${n} ${face(n)}` }, faceMark(face(n))));
	const [rx, ry] = TO_FRONT[value] || [0, 0];
	inner.style.setProperty("--rx", `${rx}deg`);
	inner.style.setProperty("--ry", `${ry}deg`);
	box.append(inner);
	return box;
}

/** A node's entrance, played once: its other animations must not start it over when they end. */
function enter(node, delay) {
	node.classList.add("enter");
	node.style.animationDelay = `${delay}ms`;
	node.addEventListener(
		"animationend",
		() => {
			node.classList.remove("enter");
			node.style.animationDelay = "";
		},
		{ once: true }
	);
}

/** A die of a side's pool, not yet thrown. */
const poolDie = () => h("div", { class: "bt-die pooled" }, h("span", { class: "bt-face" }));

/** The mark of a battle's Pre- or Post-Battle abilities: a sword, pointing up before and down after. */
function phaseIcon(step) {
	const svg = s("svg", { class: `bt-phase-icon ${step}`, viewBox: "0 0 24 24", "aria-hidden": "true" });
	svg.append(s("circle", { cx: 12, cy: 12, r: 11 }), s("path", { d: "M12 4 L14 7 V15 H10 V7 Z M7.5 15 H16.5 M12 15 V20" }));
	return svg;
}

const PHASE_TITLE = { pre: "Pre-Battle Phase", post: "Post-Battle Phase" };

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
	let phase = null; // the step whose abilities are listed: "pre", "post" or none
	let replay = false;
	let seen = true; // the player saw the move that brought this battle, so its dice are thrown for them
	let before = null; // the units of the two sides just before the battle was declared
	const zoomer = createZoom();
	let baseBox = null;
	let cloneSvg = null;
	let last = null;
	let area = "";
	let token = 0; // a new show ends the old one's pauses

	function place() {
		if (stage) placeStage(stage, ctx);
	}

	const zoom = (to, ms) => zoomer.to(cloneSvg, to, ms);

	/** The copy of the map: the area's own units gone (they stand in the arena). */
	function cloneMap(board, sides) {
		const { svg, base } = copyMap(board);
		const fighters = new Set(sides.map((x) => ctx.colorOf(x.seat)));
		for (const el of svg.querySelectorAll(`.token[data-position="${area}"]`)) if (fighters.has(el.getAttribute("data-color"))) el.remove();
		for (const el of svg.querySelectorAll(`.gate-chip[data-position="${area}"] .gate-holder`)) el.remove();
		baseBox = base;
		return svg;
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
		phase = null;
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
					h("div", { class: "bt-phase", id: "battle-phase", "aria-live": "polite" }),
					h("div", { class: "bt-tray" }, ...snap.bt.sides.map((side, i) => h("div", { class: `bt-dice ${i ? "right" : "left"}`, "data-seat": side.seat, style: `--bt-die: var(--piece-${cssColor(ctx.colorOf(side.seat))}-fill)` })))
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
		const cell = darkenAround(cloneSvg, baseBox, area, ctx.ink(snap.bt.sides[0].seat));
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
					if (animate) enter(node, n++ * 70);
					host.append(node);
				}
			}
		}
	}

	/** A big army shrinks until each side fits on the stage. */
	function fitUnits() {
		for (const side of stage.querySelectorAll(".bt-side")) {
			let size = UNIT_REM;
			side.style.setProperty("--bt-unit", `${size}rem`);
			while (side.scrollHeight > side.clientHeight && size > MIN_UNIT_REM) {
				size = Math.max(MIN_UNIT_REM, size * 0.88);
				side.style.setProperty("--bt-unit", `${size}rem`);
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
			if (instant) clear(host);
			for (const extra of [...host.children].slice((side.dice || []).length)) extra.remove();
			(side.dice || []).forEach((v, i) => {
				const die = cube(v);
				if (instant) {
					die.classList.add("landed");
					host.append(die);
					return;
				}
				// Die by die, on both sides at once: each leaves its place in the pool as it is thrown.
				const delay = i * 90;
				longest = Math.max(longest, delay);
				setTimeout(() => {
					if (mine !== token) return;
					const waiting = host.children[i];
					if (waiting) waiting.replaceWith(die);
					else host.append(die);
				}, delay);
				setTimeout(() => die.classList.add("landed"), delay + 900);
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

	/** The dice each side brings pop in one by one, on both sides at once; the pool follows its Combat. */
	async function fillPools(snap, mine) {
		const wanted = snap.bt.sides.map((side) => ({
			host: stage.querySelector(`.bt-dice[data-seat="${side.seat}"]`),
			n: side.dice && side.dice.length ? side.dice.length : side.combat || 0,
		}));
		for (const { host, n } of wanted) for (const extra of [...host.children].slice(n)) extra.remove();
		const most = Math.max(...wanted.map((w) => w.n - w.host.children.length));
		if (most <= 0 || !seen) return;
		const gap = Math.min(130, POOL_MS / most);
		for (let i = 0; i < most; i++) {
			for (const { host, n } of wanted) if (host.children.length < n) host.append(poolDie());
			await wait(gap);
			if (mine !== token || !stage) return;
		}
		await wait(200);
	}

	/** The list of the abilities goes away. */
	function endPhase() {
		if (!phase || !stage) return;
		phase = null;
		clear(stage.querySelector("#battle-phase"));
		stage.querySelector(".bt-field").classList.remove("in-phase");
	}

	/** The abilities of the step pop in one by one; the one the game waits on is lit, and those past are dimmed. */
	async function listAbilities(snap, mine) {
		const { step, abilities } = snap.bt;
		const list = abilities || [];
		if (phase !== step) {
			endPhase();
			if (!list.length) return;
			phase = step;
			const box = stage.querySelector("#battle-phase");
			box.append(h("div", { class: "bt-phase-title" }, PHASE_TITLE[step]), h("div", { class: "bt-abilities" }));
			stage.querySelector(".bt-field").classList.add("in-phase");
		}
		const host = stage.querySelector(".bt-abilities");
		const nodeOf = (a) => [...host.children].find((n) => n.dataset.ability === a.name && n.dataset.seat === String(a.seat));
		for (const a of list) {
			if (nodeOf(a)) continue;
			const node = h("div", { class: "bt-ability", "data-ability": a.name, "data-seat": a.seat, title: a.name, style: `--bt-ink: ${ctx.ink(a.seat)}` }, phaseIcon(step), ctx.glyph(a.seat), h("span", {}, a.name));
			enter(node, 0);
			host.append(node);
			if (seen) await wait(ABILITY_MS);
			if (mine !== token || !stage) return;
		}
		for (const node of host.children) {
			const a = list.find((x) => node.dataset.ability === x.name && node.dataset.seat === String(x.seat));
			node.classList.toggle("current", !!a && !!a.current);
			node.classList.toggle("done", !a);
		}
		// Live, the game itself pauses here until the choice is made. A replay holds a moment.
		if (replay && list.some((a) => a.current)) await wait(1000);
	}

	function stepWords(snap) {
		const bt = snap.bt;
		if (bt.step === "assign") return "Choosing the casualties";
		return "";
	}

	const isOver = (snap) => snap.bt.resolved || snap.bt.step === "retreat";

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
				await zoom(aimAtArea(cell, baseBox), 800);
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
		fitUnits();
		stage.querySelector("#battle-step").textContent = stepWords(snap);
		const rolled = snap.bt.sides.some((x) => x.dice && x.dice.length);
		if (!played) await fillPools(snap, mine);
		if (mine !== token || !stage) return;
		// Before the dice, the abilities are listed first: they may take units away.
		if (snap.bt.step === "pre") await listAbilities(snap, mine);
		else if (snap.bt.step !== "post") endPhase();
		if (mine !== token || !stage) return;
		if (rolled && !played) await throwDice(snap, mine, !seen);
		if (mine !== token || !stage) return;
		await strike(reckon(snap), mine);
		if (mine !== token || !stage) return;
		// After the dice, the casualties fall first: the abilities answer them.
		if (snap.bt.step === "post") await listAbilities(snap, mine);
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
		zoomer.stop();
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
		else ctx.onIdle?.();
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
			if (stage) stage.hidden = on;
		},
		/** What can be chosen has changed. */
		refresh() {
			if (stage && last && !replay) reckon(last);
			place();
			if (stage) fitUnits();
		},
		close,
		dismiss,
		/** The show has played all it has been given, and waits for the game. */
		isIdle: () => !!stage && !working && !queue.length,
		/** A unit on the stage can be chosen. */
		hasChoices: () => !!stage && !!stage.querySelector(".bt-unit.can-choose"),
		isOpen: () => !!stage,
		isReplay: () => replay,
	};
}
