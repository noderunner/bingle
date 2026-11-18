// What the page shows when a game of patience is solved (ADR 0137). First the cards
// go home: each card that was not yet on a foundation flies there, the low ranks
// first. Then a few seconds of a picture over the page, in the spirit of the cards:
// fireworks for the game's own cards, and for a themed set something of its world.
// It is drawn on a canvas, with the set's own suit pictures as sprites, and a click or
// a key ends it early. Nothing of it is shown to a player who asks for less motion.

import { h } from "./util.js";
import { setSuitChip } from "./card-sets.js";

/** How long the picture lasts, fading out over its last half second. */
export const SHOW_MS = 4000;
const FADE_MS = 500;

export function reducedMotion() {
	return !!(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches);
}

// ---- the cards go home ----------------------------------------------------------

/**
 * Flies cards to their places. Each flight is {node, from, to}: a card's face, and
 * the rectangles it leaves and lands on; they leave in the order given. It resolves
 * when the last has landed.
 */
export function flyHome(flights) {
	if (!flights.length) return Promise.resolve();
	// The more cards there are, the closer together they leave: never more than 1.6 s in all.
	const gap = Math.min(70, 1600 / flights.length);
	const flight = 480;
	const runs = flights.map((f, i) => {
		const el = h("div", { class: "sol-flyer", "aria-hidden": "true", style: `left:${f.from.left}px;top:${f.from.top}px;--cw:${f.from.width}px;--ch:${f.from.height}px` }, f.node);
		document.body.append(el);
		const dx = f.to.left + (f.to.width - f.from.width) / 2 - f.from.left;
		const dy = f.to.top + (f.to.height - f.from.height) / 2 - f.from.top;
		const scale = f.to.width && f.from.width ? f.to.width / f.from.width : 1;
		// A little lift on the way, as a card picked up and put down.
		const a = el.animate(
			[
				{ transform: "translate(0, 0) rotate(0deg) scale(1)" },
				{ transform: `translate(${dx * 0.5}px, ${dy * 0.5 - 40}px) rotate(${i % 2 ? 8 : -8}deg) scale(${(1 + scale) / 2 + 0.08})`, offset: 0.5 },
				{ transform: `translate(${dx}px, ${dy}px) rotate(0deg) scale(${scale})` },
			],
			{ duration: flight, delay: i * gap, easing: "cubic-bezier(.3,.1,.3,1)", fill: "both" }
		);
		return a.finished.catch(() => {}).then(() => el.remove());
	});
	return Promise.all(runs);
}

// ---- the picture ----------------------------------------------------------------

let showing = null;

/**
 * Shows the picture for set (null for the game's own cards) with words over it,
 * and resolves when it has gone. One at a time: a second call ends the first.
 */
export function celebrate(set, words) {
	if (showing) showing.end();
	const theme = THEMES[set ? set.id : "classic"] || THEMES.classic;
	const canvas = h("canvas", { class: "victory-canvas" });
	const sprites = h("div", { class: "victory-sprites" });
	const banner = h("div", { class: "victory-banner" }, words);
	const root = h("div", { class: `victory victory-${set ? set.id : "classic"}`, id: "victory", role: "status", "aria-live": "polite" }, canvas, sprites, banner);
	document.body.append(root);

	const g = canvas.getContext("2d");
	const dpr = Math.min(window.devicePixelRatio || 1, 2);
	let W = 0;
	let H = 0;
	const size = () => {
		W = window.innerWidth;
		H = window.innerHeight;
		canvas.width = Math.round(W * dpr);
		canvas.height = Math.round(H * dpr);
		g.setTransform(dpr, 0, 0, dpr, 0, 0);
	};
	size();
	const scene = theme({ g, set, sprites, get W() { return W; }, get H() { return H; } });

	return new Promise((resolve) => {
		let start = 0;
		let last = 0;
		let frame = 0;
		let ended = false;
		const end = () => {
			if (ended) return;
			ended = true;
			cancelAnimationFrame(frame);
			window.removeEventListener("resize", size);
			window.removeEventListener("pointerdown", end, true);
			window.removeEventListener("keydown", end, true);
			root.remove();
			showing = null;
			resolve();
		};
		showing = { end };
		window.addEventListener("resize", size);
		window.addEventListener("pointerdown", end, true);
		window.addEventListener("keydown", end, true);
		const tick = (now) => {
			if (!start) start = last = now;
			const t = now - start;
			const dt = Math.min(now - last, 50) / 1000;
			last = now;
			if (t >= SHOW_MS || !root.isConnected) return end();
			root.style.opacity = String(Math.min(1, t / 250, (SHOW_MS - t) / FADE_MS));
			if (g) scene(t, dt);
			frame = requestAnimationFrame(tick);
		};
		frame = requestAnimationFrame(tick);
	});
}

// ---- helpers --------------------------------------------------------------------

const rand = (a, b) => a + Math.random() * (b - a);
const pick = (list) => list[Math.floor(Math.random() * list.length)];
const ease = (x) => 1 - Math.pow(1 - Math.min(Math.max(x, 0), 1), 3);

/** Clears what was drawn, leaving a fraction of it: a trail. */
function fade(g, W, H, keep) {
	g.globalCompositeOperation = "destination-out";
	g.fillStyle = `rgba(0,0,0,${1 - keep})`;
	g.fillRect(0, 0, W, H);
	g.globalCompositeOperation = "source-over";
}

/** A star of n points, for sparkles and starbursts. */
function starPath(g, x, y, n, outer, inner, turn = 0) {
	g.beginPath();
	for (let i = 0; i < n * 2; i++) {
		const r = i % 2 ? inner : outer;
		const a = turn + (i * Math.PI) / n - Math.PI / 2;
		g.lineTo(x + Math.cos(a) * r, y + Math.sin(a) * r);
	}
	g.closePath();
}

/**
 * A sprite: one of the set's suit pictures, moved by the scene each frame. The
 * pictures come in turn from the suits the scene names (all eight by default).
 */
function sprite(c, i, px, suits) {
	const list = suits || [0, 1, 2, 3, 4, 5, 6, 7];
	const el = h("div", { class: "victory-sprite" }, setSuitChip(c.set, list[i % list.length], px));
	c.sprites.append(el);
	return {
		el,
		place(x, y, rot = 0, scale = 1, opacity = 1) {
			el.style.transform = `translate(${x - px / 2}px, ${y - px / 2}px) rotate(${rot}rad) scale(${scale})`;
			el.style.opacity = String(opacity);
		},
	};
}

// ---- the scenes -----------------------------------------------------------------
//
// A scene is made once with the canvas and the sprite layer, and returns the
// function that draws it at t ms (dt s since the last frame).

/** The game's own cards: fireworks. */
function fireworks(c) {
	const { g } = c;
	const unit = () => Math.min(c.W, c.H) / 700;
	const launches = [0, 280, 620, 900, 1250, 1500, 1850, 2150, 2450, 2700, 3000].map((at) => ({ at, done: false }));
	const rockets = [];
	const sparks = [];
	const burst = (x, y) => {
		const hue = rand(0, 360);
		const n = 70;
		for (let i = 0; i < n; i++) {
			const a = (i / n) * Math.PI * 2 + rand(-0.05, 0.05);
			const v = rand(110, 300) * unit();
			sparks.push({ x, y, vx: Math.cos(a) * v, vy: Math.sin(a) * v, life: rand(0.9, 1.5), age: 0, hue: hue + rand(-20, 20) });
		}
	};
	return (t, dt) => {
		fade(g, c.W, c.H, 0.8);
		for (const l of launches) {
			if (l.done || t < l.at) continue;
			l.done = true;
			const x = rand(0.15, 0.85) * c.W;
			rockets.push({ x, y: c.H + 10, vx: rand(-30, 30), top: rand(0.15, 0.45) * c.H, speed: rand(0.9, 1.2) * c.H });
		}
		g.globalCompositeOperation = "lighter";
		for (let i = rockets.length - 1; i >= 0; i--) {
			const r = rockets[i];
			r.y -= r.speed * dt;
			r.x += r.vx * dt;
			g.fillStyle = "rgba(255,230,180,0.9)";
			g.fillRect(r.x - 1.5, r.y - 1.5, 3, 7);
			if (r.y <= r.top) {
				burst(r.x, r.y);
				rockets.splice(i, 1);
			}
		}
		for (let i = sparks.length - 1; i >= 0; i--) {
			const p = sparks[i];
			p.age += dt;
			if (p.age > p.life) {
				sparks.splice(i, 1);
				continue;
			}
			p.vx *= 1 - 1.2 * dt;
			p.vy = p.vy * (1 - 1.2 * dt) + 140 * unit() * dt;
			p.x += p.vx * dt;
			p.y += p.vy * dt;
			const left = 1 - p.age / p.life;
			g.fillStyle = `hsla(${p.hue}, 95%, ${55 + 25 * left}%, ${left})`;
			g.beginPath();
			g.arc(p.x, p.y, 2.2 * Math.max(unit(), 0.6), 0, Math.PI * 2);
			g.fill();
		}
		g.globalCompositeOperation = "source-over";
	};
}

/** Ancient Civilizations: a golden sun rises in a burst of rays, and the suits rise with it. */
function sunrise(c) {
	const { g } = c;
	const chips = [];
	for (let i = 0; i < 18; i++) {
		chips.push({ s: sprite(c, i, 46), x: rand(0.06, 0.94), at: i * 150 + rand(0, 120), dur: rand(2400, 3000), spin: rand(-3, 3), sway: rand(10, 30) });
	}
	return (t) => {
		g.clearRect(0, 0, c.W, c.H);
		const cx = c.W / 2;
		const cy = c.H * (0.75 - 0.3 * ease(t / 1400));
		const R = Math.max(c.W, c.H);
		g.save();
		g.translate(cx, cy);
		g.rotate(t / 4000);
		for (let i = 0; i < 18; i++) {
			const a = (i / 18) * Math.PI * 2;
			const grad = g.createLinearGradient(0, 0, Math.cos(a) * R, Math.sin(a) * R);
			grad.addColorStop(0, "rgba(255, 214, 102, 0.55)");
			grad.addColorStop(1, "rgba(255, 214, 102, 0)");
			g.fillStyle = grad;
			g.beginPath();
			g.moveTo(0, 0);
			g.arc(0, 0, R, a - 0.07, a + 0.07);
			g.closePath();
			g.fill();
		}
		g.restore();
		const disc = Math.min(c.W, c.H) * 0.09;
		const sun = g.createRadialGradient(cx, cy, disc * 0.2, cx, cy, disc * 1.8);
		sun.addColorStop(0, "rgba(255, 244, 200, 1)");
		sun.addColorStop(0.45, "rgba(244, 184, 66, 0.95)");
		sun.addColorStop(1, "rgba(244, 184, 66, 0)");
		g.fillStyle = sun;
		g.beginPath();
		g.arc(cx, cy, disc * 1.8, 0, Math.PI * 2);
		g.fill();
		for (const ch of chips) {
			const k = (t - ch.at) / ch.dur;
			if (k <= 0 || k >= 1) {
				ch.s.place(-100, -100, 0, 1, 0);
				continue;
			}
			const y = c.H + 40 - (c.H + 100) * ease(k);
			ch.s.place(ch.x * c.W + Math.sin(k * 6) * ch.sway, y, ch.spin * k, 1, Math.min(1, (1 - k) * 4));
		}
	};
}

/** Amphibians: frogs hop across the pond, rippling the water where they land. */
function pond(c) {
	const { g } = c;
	const ripples = [];
	const bubbles = [];
	const frogs = [];
	const hops = 4;
	for (let i = 0; i < 7; i++) {
		const ltr = i % 2 === 0;
		frogs.push({ s: sprite(c, i, 54, [0, 2, 4, 3, 1, 5, 7]), ltr, at: i * 260, hop: rand(520, 640), row: rand(0.55, 0.9), height: rand(0.12, 0.2), landed: -1 });
	}
	return (t, dt) => {
		g.clearRect(0, 0, c.W, c.H);
		for (const f of frogs) {
			const k = (t - f.at) / (f.hop * hops);
			if (k <= 0 || k >= 1) {
				f.s.place(-100, -100, 0, 1, 0);
				continue;
			}
			const span = (c.W + 120) / hops;
			const n = Math.floor(k * hops);
			const in_ = k * hops - n;
			const x0 = -60 + n * span + in_ * span;
			const x = f.ltr ? x0 : c.W - x0;
			const ground = f.row * c.H;
			const y = ground - 4 * in_ * (1 - in_) * f.height * c.H;
			f.s.place(x, y, (f.ltr ? 1 : -1) * (in_ - 0.5) * 0.6, 1, 1);
			if (n > f.landed) {
				f.landed = n;
				if (n > 0) ripples.push({ x, y: ground + 24, age: 0 });
			}
		}
		if (Math.random() < 0.5) bubbles.push({ x: rand(0, c.W), y: c.H + 10, r: rand(3, 9), v: rand(40, 110), wob: rand(0, 6) });
		g.lineWidth = 2;
		for (let i = ripples.length - 1; i >= 0; i--) {
			const r = ripples[i];
			r.age += dt;
			if (r.age > 1.4) {
				ripples.splice(i, 1);
				continue;
			}
			for (const lag of [0, 0.25, 0.5]) {
				const a = r.age - lag;
				if (a <= 0) continue;
				g.strokeStyle = `rgba(220, 255, 235, ${0.7 * (1 - a / 1.4)})`;
				g.beginPath();
				g.ellipse(r.x, r.y, 10 + a * 90, 4 + a * 26, 0, 0, Math.PI * 2);
				g.stroke();
			}
		}
		g.lineWidth = 1.5;
		for (let i = bubbles.length - 1; i >= 0; i--) {
			const b = bubbles[i];
			b.y -= b.v * dt;
			if (b.y < -20) {
				bubbles.splice(i, 1);
				continue;
			}
			g.strokeStyle = "rgba(230, 255, 240, 0.55)";
			g.beginPath();
			g.arc(b.x + Math.sin(b.y / 30 + b.wob) * 6, b.y, b.r, 0, Math.PI * 2);
			g.stroke();
		}
	};
}

/** Fantasy Creatures: sparkles spiral out of the middle, and the creatures fly out with them. */
function enchantment(c) {
	const { g } = c;
	const sparkles = [];
	const flyers = [];
	for (let i = 0; i < 16; i++) flyers.push({ s: sprite(c, i, 50, [0, 2, 5, 1, 4, 3, 6, 7]), at: i * 170, dur: rand(1800, 2400), turn: (i / 16) * Math.PI * 2 + rand(-0.2, 0.2), dir: i % 2 ? 1 : -1 });
	const colours = ["#ffe066", "#e6b3ff", "#9ff0ff", "#ffffff", "#ffb3d9"];
	return (t, dt) => {
		fade(g, c.W, c.H, 0.82);
		const cx = c.W / 2;
		const cy = c.H / 2;
		const R = Math.hypot(c.W, c.H) / 2;
		for (let arm = 0; arm < 5; arm++) {
			if (t > SHOW_MS - 900) break;
			const a = (arm / 5) * Math.PI * 2 + t / 600;
			const r = ((t / 900) % 1) * R * 0.8;
			sparkles.push({ x: cx + Math.cos(a + r / 140) * r, y: cy + Math.sin(a + r / 140) * r, size: rand(3, 8), age: 0, life: rand(0.6, 1.2), col: pick(colours), turn: rand(0, 1) });
		}
		g.globalCompositeOperation = "lighter";
		for (let i = sparkles.length - 1; i >= 0; i--) {
			const p = sparkles[i];
			p.age += dt;
			if (p.age > p.life) {
				sparkles.splice(i, 1);
				continue;
			}
			const left = 1 - p.age / p.life;
			g.globalAlpha = left;
			g.fillStyle = p.col;
			starPath(g, p.x, p.y, 4, p.size * (0.6 + 0.4 * Math.sin(p.age * 18)), p.size * 0.25, p.turn);
			g.fill();
		}
		g.globalAlpha = 1;
		g.globalCompositeOperation = "source-over";
		for (const f of flyers) {
			const k = (t - f.at) / f.dur;
			if (k <= 0 || k >= 1) {
				f.s.place(-100, -100, 0, 1, 0);
				continue;
			}
			const r = ease(k) * R;
			const a = f.turn + f.dir * k * 2.2;
			f.s.place(cx + Math.cos(a) * r, cy + Math.sin(a) * r, f.dir * k * 2, 0.4 + k * 1.1, Math.min(1, k * 5, (1 - k) * 3));
		}
	};
}

/** Lovecraftian: tentacles rise from the deep, and something opens an eye. */
function deep(c) {
	const { g } = c;
	const arms = [];
	for (let i = 0; i < 9; i++) arms.push({ x: (i + 0.5) / 9 + rand(-0.03, 0.03), reach: rand(0.4, 0.68), width: rand(22, 34), phase: rand(0, 6), at: rand(0, 500) });
	// Drawn as a ribbon that narrows to its tip, with suckers down one side.
	const arm = (a, t) => {
		const grow = ease((t - a.at) / 1100) * (1 - ease((t - (SHOW_MS - 1100)) / 900));
		if (grow <= 0) return;
		const len = a.reach * c.H * grow;
		const steps = 28;
		const pts = [];
		for (let i = 0; i <= steps; i++) {
			const s = i / steps;
			const sway = Math.sin(t / 420 + a.phase + s * 4.2) * 46 * s * s;
			pts.push([a.x * c.W + sway + Math.sin(a.phase) * 30 * s, c.H + 10 - s * len]);
		}
		const side = (sign) =>
			pts.map(([x, y], i) => {
				const [nx, ny] = pts[Math.min(i + 1, steps)];
				const [px, py] = pts[Math.max(i - 1, 0)];
				const dx = nx - px;
				const dy = ny - py;
				const d = Math.hypot(dx, dy) || 1;
				const w = (a.width / 2) * (1 - i / steps) + 0.8;
				return [x + (-dy / d) * w * sign, y + (dx / d) * w * sign];
			});
		const left = side(1);
		const right = side(-1).reverse();
		const grad = g.createLinearGradient(0, c.H, 0, c.H - len);
		grad.addColorStop(0, "#0d2b26");
		grad.addColorStop(1, "#3f8a73");
		g.fillStyle = grad;
		g.beginPath();
		for (const [x, y] of [...left, ...right]) g.lineTo(x, y);
		g.closePath();
		g.fill();
		g.fillStyle = "rgba(190, 230, 205, 0.55)";
		for (let i = 2; i < steps - 3; i += 2) {
			const [x, y] = left[i];
			g.beginPath();
			g.arc(x, y, (a.width / 9) * (1 - i / steps) + 1, 0, Math.PI * 2);
			g.fill();
		}
	};
	const eye = (t) => {
		const open = ease((t - 900) / 700) * (1 - ease((t - (SHOW_MS - 900)) / 500));
		if (open <= 0) return;
		// A blink, once.
		const blink = t > 2500 && t < 2750 ? 1 - Math.sin(((t - 2500) / 250) * Math.PI) : 1;
		const cx = c.W / 2;
		const cy = c.H * 0.42;
		const rx = Math.min(c.W, c.H) * 0.16;
		const ry = rx * 0.55 * open * blink;
		const glow = g.createRadialGradient(cx, cy, 0, cx, cy, rx * 2.4);
		glow.addColorStop(0, `rgba(120, 255, 170, ${0.35 * open})`);
		glow.addColorStop(1, "rgba(120, 255, 170, 0)");
		g.fillStyle = glow;
		g.fillRect(cx - rx * 2.4, cy - rx * 2.4, rx * 4.8, rx * 4.8);
		if (ry < 1) return;
		g.save();
		g.beginPath();
		g.ellipse(cx, cy, rx, ry, 0, 0, Math.PI * 2);
		g.fillStyle = "#e8f0c8";
		g.fill();
		g.clip();
		const look = Math.sin(t / 700) * rx * 0.25;
		const iris = g.createRadialGradient(cx + look, cy, 0, cx + look, cy, rx * 0.5);
		iris.addColorStop(0, "#f2ff7a");
		iris.addColorStop(1, "#3c8f2c");
		g.fillStyle = iris;
		g.beginPath();
		g.arc(cx + look, cy, rx * 0.5, 0, Math.PI * 2);
		g.fill();
		g.fillStyle = "#05100b";
		g.beginPath();
		g.ellipse(cx + look, cy, rx * 0.09, rx * 0.42, 0, 0, Math.PI * 2);
		g.fill();
		g.restore();
		g.strokeStyle = "#0d2b26";
		g.lineWidth = 3;
		g.beginPath();
		g.ellipse(cx, cy, rx, ry, 0, 0, Math.PI * 2);
		g.stroke();
	};
	return (t) => {
		g.clearRect(0, 0, c.W, c.H);
		eye(t);
		for (const a of arms) arm(a, t);
	};
}

/** Atomic Age: an atom spins up, starbursts twinkle, and rockets and saucers streak past. */
function atomic(c) {
	const { g } = c;
	const puffs = [];
	const ships = [];
	for (let i = 0; i < 6; i++) {
		const ltr = i % 2 === 0;
		// Rockets (the first suit), saucers (the third) and sputniks (the seventh), in turn.
		const suit = [0, 2, 6][i % 3];
		ships.push({ s: sprite(c, i, 52, [suit]), rocket: suit === 0, ltr, at: 200 + i * 520, dur: rand(1500, 1900), y0: rand(0.55, 0.95), y1: rand(0.05, 0.4), last: 0 });
	}
	const bursts = [];
	for (let i = 0; i < 14; i++) bursts.push({ x: rand(0.04, 0.96), y: rand(0.05, 0.95), at: rand(0, 3200), size: rand(10, 22), col: pick(["#ff8a3d", "#2ec4b6", "#ffd23f", "#f4f1de"]) });
	return (t, dt) => {
		g.clearRect(0, 0, c.W, c.H);
		const cx = c.W / 2;
		const cy = c.H * 0.45;
		const grow = ease(t / 700);
		const R = Math.min(c.W, c.H) * 0.24 * grow;
		g.lineWidth = 3;
		for (let k = 0; k < 3; k++) {
			const tilt = (k * Math.PI) / 3;
			g.strokeStyle = "rgba(46, 196, 182, 0.85)";
			g.beginPath();
			g.ellipse(cx, cy, R, R * 0.34, tilt, 0, Math.PI * 2);
			g.stroke();
			const a = t / (260 + k * 50) + k * 2;
			const ex = Math.cos(a) * R;
			const ey = Math.sin(a) * R * 0.34;
			g.fillStyle = "#ffd23f";
			g.beginPath();
			g.arc(cx + ex * Math.cos(tilt) - ey * Math.sin(tilt), cy + ex * Math.sin(tilt) + ey * Math.cos(tilt), 7 * grow, 0, Math.PI * 2);
			g.fill();
		}
		for (const [dx, dy, col] of [[-6, -4, "#ff8a3d"], [6, -3, "#f4f1de"], [0, 6, "#ff8a3d"], [-4, 5, "#f4f1de"], [5, 5, "#ff8a3d"]]) {
			g.fillStyle = col;
			g.beginPath();
			g.arc(cx + dx * grow * 1.6, cy + dy * grow * 1.6, 9 * grow, 0, Math.PI * 2);
			g.fill();
		}
		for (const b of bursts) {
			const k = (t - b.at) / 700;
			if (k <= 0 || k >= 1) continue;
			g.fillStyle = b.col;
			g.globalAlpha = Math.sin(k * Math.PI);
			starPath(g, b.x * c.W, b.y * c.H, 8, b.size * Math.sin(k * Math.PI), b.size * 0.2, k);
			g.fill();
			g.globalAlpha = 1;
		}
		for (const s of ships) {
			const k = (t - s.at) / s.dur;
			if (k <= 0 || k >= 1) {
				s.s.place(-100, -100, 0, 1, 0);
				continue;
			}
			const x = s.ltr ? -60 + (c.W + 120) * k : c.W + 60 - (c.W + 120) * k;
			const y = (s.y0 + (s.y1 - s.y0) * k) * c.H;
			const heading = Math.atan2((s.y1 - s.y0) * c.H, s.ltr ? c.W : -c.W);
			// A rocket's picture points up, so it turns to fly nose first; a saucer stays level.
			s.s.place(x, y, s.rocket ? heading + Math.PI / 2 : 0, 1, 1);
			if (t - s.last > 40) {
				s.last = t;
				puffs.push({ x: x - Math.cos(heading) * 26, y: y - Math.sin(heading) * 26, age: 0 });
			}
		}
		for (let i = puffs.length - 1; i >= 0; i--) {
			const p = puffs[i];
			p.age += dt;
			if (p.age > 0.9) {
				puffs.splice(i, 1);
				continue;
			}
			g.fillStyle = `rgba(244, 241, 222, ${0.5 * (1 - p.age / 0.9)})`;
			g.beginPath();
			g.arc(p.x, p.y, 5 + p.age * 16, 0, Math.PI * 2);
			g.fill();
		}
	};
}

const THEMES = { classic: fireworks, ancient: sunrise, amphibians: pond, fantasy: enchantment, lovecraftian: deep, atomic };
