// Cthulhu Wars' battles and awakenings on the game page (ADR 0124): the battle
// panel, the battle theatre that plays each battle on the map, its replay from
// the history, and a Great Old One's emergence. game.js asks page.js's battles
// once, handing it the parts of the page this needs (game.js viewContext).

import { h, s, ago } from "/static/js/util.js";
import { api } from "/static/js/api.js";
import { isLandscape, isMobileLayout } from "/static/js/mobile.js";
import { ONGATE, pieceIcon } from "/static/js/render.js";
import { createBattleTheatre } from "./cw-battle.js";
import { createEmergence, hasEmergence } from "./cw-emerge.js";

/** The battles and awakenings of one game page. */
export function battles(ctx) {
	const { S, actingSeat, activeBuilder, areaName, code, colorOf, drawBoard, inAnalysis, isLive, isMyImpulse, latestLog, onToken, pendingSelections, pieceLook, rulesetView, seatGlyph, seatInk, tokenOk } = ctx;

	/** The battle being fought, or the last one fought. */
	function battlePanel() {
		const v = rulesetView();
		if (!v || !v.battle) return null;
		const bt = v.battle;
		const card = h("div", { class: "exchange battle", id: "battle-panel", "data-resolved": bt.resolved ? "1" : "0" });
		card.append(
			h(
				"h3",
				{},
				`${bt.resolved ? "Last battle" : "Battle"} in ${bt.area_name}`,
				bt.step && !bt.resolved ? h("span", { class: "muted small" }, ` (${bt.step})`) : null
			)
		);
		if (battlesAnimated() && (bt.resolved || !isLive())) {
			card.append(
				h(
					"button",
					{
						type: "button",
						class: "ghost small battle-replay",
						id: "battle-replay",
						title: "Watch this battle again",
						onclick: async () => {
							const found = await lastBattleBefore(isLive() ? S.moves.length : S.viewSeq);
							if (found && found.snaps.length) battleTheatre().play(found.key, found.snaps, found.before);
						},
					},
					"▶ Replay"
				)
			);
		}
		for (const side of bt.sides) {
			const row = h("div", { class: "side", "data-seat": side.seat });
			row.append(h("div", { class: "who" }, side.name));
			const pool = h("div", { class: "pool" });
			pool.append(h("span", { class: "pool-part" }, h("span", { class: "pool-label" }, "Combat"), h("strong", {}, String(side.combat))));
			if (side.dice && side.dice.length) {
				const dice = h("span", { class: "pool-part" });
				for (const d of side.dice) dice.append(h("span", { class: "die " + (d === 6 ? "ok" : d >= 4 ? "mid" : "fail"), title: d === 6 ? "Kill" : d >= 4 ? "Pain" : "Miss" }, String(d)));
				pool.append(dice);
			}
			pool.append(
				h("span", { class: "pool-part muted small" }, `suffers ${side.kills} Kill${side.kills === 1 ? "" : "s"}, ${side.pains} Pain${side.pains === 1 ? "" : "s"}`)
			);
			row.append(pool);
			card.append(row);
		}
		return card;
	}

	// ---- the theatre of a battle --------------------------------------------------------------

	let theatre = null;

	const BATTLE_CATCH_UP = 12; // the most moves missed that are still played in order

	/**
	 * The map to copy for the stage. An upright phone draws none of its own, so one is drawn here,
	 * off the page, for the copy to be taken from.
	 */
	function battleMap() {
		const shown = document.getElementById("board-svg");
		if (shown) return shown;
		const host = h("div", {});
		drawBoard(host);
		return host.querySelector("svg");
	}

	function battleTheatre() {
		if (theatre) return theatre;
		theatre = createBattleTheatre({
			board: battleMap,
			onPage: () => document.getElementById("board-svg"),
			phone: isMobileLayout,
			upright: () => isMobileLayout() && !isLandscape(),
			onIdle: () => {
				if (theatre.isOpen()) showChoicesOnTheatre();
			},
			ink: seatInk,
			glyph: seatGlyph,
			colorOf,
			label: (type) => pieceLook(type).label,
			icon: (type, color, px) => pieceIcon(pieceLook(type).shape, color, px),
			unitOk: (area, type, color) => !!actingSeat() && (tokenOk(area, type, color) || tokenOk(area, ONGATE + type, color)),
			onUnit: (area, type, color) => onToken(area, tokenOk(area, type, color) ? type : ONGATE + type, color),
		});
		window.addEventListener("resize", () => theatre.refresh());
		window.addEventListener("keydown", (ev) => {
			if (ev.key === "Escape" && theatre.isOpen()) theatre.dismiss();
		});
		return theatre;
	}

	/** Whether this page plays the battles: Cthulhu Wars does, unless the player asks it not to. */
	function battlesAnimated() {
		return S.settings.battleAnim && !(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches);
	}

	/** The units of the two sides of a battle standing in its area, in a position. */
	function battleUnitsIn(st, bt) {
		const sides = new Map(bt.sides.map((x) => [colorOf(x.seat), x.seat]));
		const units = [];
		for (const pc of (st.cells || {})[bt.area] || []) if (sides.has(pc.color)) units.push({ seat: sides.get(pc.color), type: pc.piece });
		return units;
	}

	/** What the theatre needs of a position with a battle in it. */
	function battleSnapshot(st) {
		const bt = st && st.ext && st.ext.view && st.ext.view.battle;
		if (!bt) return null;
		return { bt, units: battleUnitsIn(st, bt), pained: (st.ext.battle && st.ext.battle.pained) || [] };
	}

	/** Tells one finished battle from another: a position goes on showing the last one fought. */
	function battleResult(snap) {
		return snap && snap.bt.resolved ? JSON.stringify([snap.bt.area, snap.bt.att, snap.bt.def, snap.bt.sides]) : "";
	}

	/** A battle that was declared and finished within this position's move. */
	function battleFoughtInOneMove(snap, before) {
		return !!snap && snap.bt.resolved && !(before && !before.bt.resolved) && battleResult(snap) !== battleResult(before);
	}

	/** Shows a live battle as it goes: called after every redraw. */
	function syncBattleTheatre() {
		if (!S.state || !S.info) return;
		if (!battlesAnimated() || inAnalysis() || S.tut) {
			if (theatre && theatre.isOpen()) theatre.close();
			S.battleWatch = { seq: S.moves.length, pending: pendingSelections().length, st: S.state };
			S.battleKey = null;
			return;
		}
		if (!isLive()) return;
		const was = S.battleWatch;
		const seq = S.moves.length;
		// A selection added to the pending move, or taken off it, is a step of the
		// battle too: the board shows the position after it (ADR 0114).
		const pending = pendingSelections().length;
		if (was && was.seq === seq && (was.pending || 0) === pending) {
			if (theatre && theatre.isOpen()) showChoicesOnTheatre();
			return;
		}
		S.battleWatch = { seq, pending, st: S.state };
		if (was && was.seq === seq) {
			battleStep(was.st, S.state, seq, true);
			return;
		}
		// A move taken back: whatever was playing did not happen.
		if (was && seq < was.seq) {
			S.battleKey = null;
			if (theatre && theatre.isOpen()) theatre.close();
			emergence?.close();
		}
		const seen = !!was && seq > was.seq; // otherwise the page has just opened on this position
		const gap = seen ? seq - was.seq : 0;
		// After this redraw, and apart from the battle: nothing here may keep a battle from playing.
		if (seen) queueMicrotask(() => emergedSince(was.seq, seq));
		// Moves can come faster than the page draws them. A short run is played in order, or a step
		// of the battle would be lost; a long one (back from the history) only shows where it ended.
		if (gap < 2 || gap > BATTLE_CATCH_UP) {
			battleStep(gap === 1 ? was.st : null, S.state, seq, seen);
			return;
		}
		const final = S.state;
		S.battleCatchUp = (S.battleCatchUp || Promise.resolve()).then(async () => {
			let prev = was.st;
			for (let n = was.seq + 1; n <= seq && isLive(); n++) {
				const st = n === seq ? final : await stateAt(n);
				battleStep(prev, st, n, true);
				prev = st;
			}
		}).catch(() => {});
	}

	/** A position's part in the battle under way, given the position before it if there was one. */
	function battleStep(prevState, st, seq, seen) {
		const snap = battleSnapshot(st);
		const before = prevState ? battleSnapshot(prevState) : null;
		if (!snap) {
			S.battleKey = null;
			return;
		}
		const bt = snap.bt;
		if (!bt.resolved || S.battleKey || (seen && battleFoughtInOneMove(snap, before))) {
			const opening = !S.battleKey;
			// The map is the battle's now.
			emergence?.close();
			if (opening) S.battleKey = `${bt.area}|${bt.att}|${bt.def}|${seq}`;
			battleTheatre().feed(S.battleKey, snap, opening && prevState ? battleUnitsIn(prevState, bt) : null, seen);
		}
		if (bt.resolved) S.battleKey = null;
		if (theatre && theatre.isOpen()) showChoicesOnTheatre();
	}

	/** A choice elsewhere on the map needs the map; one in the battle is made on the stage. */
	function showChoicesOnTheatre() {
		const bt = (battleSnapshot(S.state) || {}).bt;
		// The retreat comes after the zoom out, which frees the map by itself.
		const onStage = !!bt && !bt.resolved && bt.step !== "retreat";
		const mine = onStage && !theatre.isReplay() && isMyImpulse();
		const elsewhere = mine && [...activeBuilder().nextPositions()].some((pos) => pos !== bt.area);
		theatre.refresh();
		// A phone's buttons are on a page of their own, under the stage: once the show has caught up
		// with a choice that cannot be made by touching a unit, the stage steps aside for it.
		theatre.hide(elsewhere || (mine && isMobileLayout() && theatre.isIdle() && !theatre.hasChoices()));
	}

	// ---- a Great Old One's first awakening ------------------------------------------------------

	let emergence = null;

	function theEmergence() {
		if (emergence) return emergence;
		emergence = createEmergence({
			board: battleMap,
			onPage: () => document.getElementById("board-svg"),
			phone: isMobileLayout,
			upright: () => isMobileLayout() && !isLandscape(),
			ink: seatInk,
			glyph: seatGlyph,
		});
		window.addEventListener("resize", () => emergence.refresh());
		window.addEventListener("keydown", (ev) => {
			if (ev.key === "Escape" && emergence.isOpen()) emergence.close();
		});
		return emergence;
	}

	const EMERGES_RE = /^(.+) Emerges$/;

	/** The unit a line of the history says has emerged, and whether it has a show. */
	function emergedUnit(line) {
		const m = EMERGES_RE.exec(line);
		if (!m || !S.info) return null;
		const def = S.info.pieces.find((p) => p.label === m[1]);
		return def ? { name: m[1], type: def.type, show: hasEmergence(def.type) } : { name: m[1], type: "", show: false };
	}

	/** Plays the show of a line of the history, from the move that told it. */
	function playEmergence(unit, e) {
		const move = S.moves.find((m) => m.seq === e.ply);
		// A move of several selections says the last one's area: the unit's own is
		// where it awoke (an awakening may be followed by a spellbook it earned).
		const sels = move ? move.selections || [move] : [];
		const sel = sels.find((x) => x.piece === unit.type && x.to) || [...sels].reverse().find((x) => x.to);
		const area = sel && sel.to;
		if (!area) return;
		const def = (S.info.board.areas || []).find((a) => a.id === area);
		const still = !!(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches);
		theEmergence().play({ type: unit.type, name: unit.name, seat: e.seat, area, areaName: def ? def.name : "", still });
	}

	/** A first awakening among the moves that have just come, which the player sees as it happens. */
	function emergedSince(from, to) {
		if (document.hidden) return;
		for (const e of latestLog(S.state)) {
			if (e.ply <= from || e.ply > to) continue;
			for (const line of e.lines || []) {
				const unit = emergedUnit(line);
				if (unit && unit.show) playEmergence(unit, e);
			}
		}
	}

	/** The badge of a first awakening in the history, which plays it again if it has a show. */
	function emergesBadge(unit, e) {
		const style = `--goo-ink: ${seatInk(e.seat)}`;
		if (!unit.show) return h("span", { class: "log-emerges", style }, `${unit.name} Emerges`);
		return h(
			"button",
			{
				type: "button",
				class: "log-emerges playable",
				style,
				"data-emerges": unit.type,
				title: "Watch it again",
				onclick: (ev) => {
					ev.stopPropagation();
					playEmergence(unit, e);
				},
			},
			`▶ ${unit.name} Emerges`
		);
	}

	/** The position after a move, from the cache of the history. */
	async function stateAt(seq) {
		if (seq >= S.moves.length) return S.state;
		if (!S.viewCache.has(seq)) S.viewCache.set(seq, (await api.state(code, seq, S.me.name)).state);
		return S.viewCache.get(seq);
	}

	/** The positions of the battle a move belongs to, from the declaration to the zoom out; null if it belongs to none. */
	async function battleAround(seq) {
		const snapAt = async (n) => (n > 0 ? battleSnapshot(await stateAt(n)) : null);
		const here = await snapAt(seq);
		if (!here) return null;
		if (here.bt.resolved) {
			const before = await snapAt(seq - 1);
			// A position long after a battle still shows it: only the battle's own moves start the show.
			if (!(before && !before.bt.resolved) && !battleFoughtInOneMove(here, before)) return null;
		}
		const fighting = (snap) => !!snap && !snap.bt.resolved && snap.bt.area === here.bt.area;
		let start = seq;
		while (start > 1 && fighting(await snapAt(start - 1))) start--;
		const snaps = [];
		for (let n = start; n <= S.moves.length && snaps.length < 120; n++) {
			const snap = await snapAt(n);
			if (!snap || snap.bt.area !== here.bt.area) break;
			snaps.push(snap);
			if (snap.bt.resolved) break;
		}
		const earlier = start > 1 ? await stateAt(start - 1) : null;
		return { key: `replay|${here.bt.area}|${start}`, snaps, before: earlier ? battleUnitsIn(earlier, here.bt) : null };
	}

	/** The battle whose result a position shows: the latest fought up to it, however long ago. */
	async function lastBattleBefore(seq) {
		for (let n = seq; n > 0; n--) {
			const m = S.moves[n - 1];
			if (!(m.selections || [m]).some((x) => x.action === "battle")) continue;
			const found = await battleAround(n);
			if (found) return found;
		}
		return null;
	}

	/** Plays again the battle a move of the history belongs to, unless it is already showing. */
	async function replayBattleAt(seq) {
		if (!battlesAnimated() || S.autoplay) return;
		const found = seq ? await battleAround(seq) : null;
		if (S.viewSeq !== seq) return; // the player has moved on
		if (!found || !found.snaps.length) {
			S.replayed = null;
			if (theatre && theatre.isOpen()) theatre.close();
			return;
		}
		if (S.replayed === found.key) return;
		S.replayed = found.key;
		battleTheatre().play(found.key, found.snaps, found.before);
	}


	return {
		panel: battlePanel,
		sync: syncBattleTheatre,
		replayAt: replayBattleAt,
		close: () => {
			if (theatre && theatre.isOpen()) theatre.close();
		},
		emergedUnit,
		emergesBadge,
	};
}
