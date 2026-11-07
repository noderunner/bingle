// Cthulhu Wars' cards on the game page (ADR 0124): each cult's faction cards
// (Elder Signs, its pool, captives, spellbooks and their requirements), the
// round bar with its phase and Ritual track, the turn order, the unit card, the
// units' and neutral items' hover cards. game.js asks page.js's cultCards once,
// handing it the parts of the page they need (game.js viewContext).

import { isMobileLayout } from "/static/js/mobile.js";
import { cssColor, pieceIcon } from "/static/js/render.js";
import { hoverCard } from "/static/js/tooltip.js";
import { h, s } from "/static/js/util.js";

/** The cults' cards of one game page. */
export function cultCards(ctx) {
	const { S, abilitiesBlock, areaName, bookRequirements, closeUnitInfo, colorOf, currentCells, displayName, enterBlock, expandToggle, isRuleset, neutralTip, rulesetView, seatGlyph, seatLabel, seatName, spellMark, statIcon, unitCardFor, unitText } = ctx;

	/**
	 * A cult's Elder Signs: face-down ones (their values are private: the owner
	 * sees them, everybody else sees a count) and the revealed ones, which are
	 * public and already in the Doom total. Drawn as tokens.
	 */
	function elderSignsBody(seat, sv) {
		const hidden = (sv && sv.hidden_signs) || 0;
		const shown = (sv && sv.shown_signs) || [];
		const mine = seat === S.me.seat;
		const row = h("div", { class: "sign-tokens", id: `signs-${seat}`, "data-seat": seat });
		if (!hidden && !shown.length) {
			row.append(h("span", { class: "muted small" }, "None yet."));
			return row;
		}
		if (hidden) {
			const values = mine ? sv.signs || [] : [];
			const group = h("span", {
				class: "sign-group",
				title: mine ? "Your Elder Signs: only you can see these values until you reveal them." : "Face down: only their owner can see them.",
			});
			for (let i = 0; i < hidden; i++) {
				const v = values[i];
				group.append(h("span", { class: "sign-chip facedown" + (v ? " own" : ""), "data-value": v || null }, v ? String(v) : "?"));
			}
			row.append(group, h("span", { class: "small muted" }, mine ? "yours alone" : "face down"));
		}
		if (shown.length) {
			const group = h("span", { class: "sign-group", title: "Revealed: everybody sees them and they count in the Doom total." });
			for (const v of shown) group.append(h("span", { class: "sign-chip shown" }, String(v)));
			row.append(h("span", { class: "small muted" }, "revealed"), group);
		}
		return row;
	}

	/**
	 * Every unit of the cult, one icon each: those in the pool in full colour, those
	 * standing on the map or held elsewhere (captured, asleep, submerged…) greyed
	 * out. Hover for the card, which says where that unit is.
	 */
	function poolBody(seat, sv) {
		const color = colorOf(seat);
		const wrap = h("div", { class: "pool-icons", id: `pool-${seat}`, "data-seat": seat });
		let n = 0;
		for (const u of (sv && sv.units) || []) {
			const def = S.info.pieces.find((p) => p.type === u.id);
			const pool = u.pool || 0;
			const total = Math.max(u.total || 0, pool);
			for (let i = 0; i < total; i++) {
				const inPool = i < pool;
				n++;
				// The units away are told apart by the places the server lists.
				const where = inPool ? "in the pool" : (u.places || [])[i - pool] || "not in the pool";
				const icon = h(
					"span",
					{
						class: "pool-unit" + (inPool ? "" : " away"),
						tabindex: "0",
						"data-piece": u.id,
						"data-zone": inPool ? "pool" : "away",
						role: "img",
						"aria-label": `${u.name}: ${where}`,
					},
					pieceIcon(def ? def.shape : "disc", color, 26)
				);
				hoverCard(icon, () => unitTip(u, color, null, where));
				wrap.append(icon);
			}
		}
		if (!n) wrap.append(h("span", { class: "muted small" }, "No units."));
		const zone = capturedZone(seat, sv);
		if (!zone) return wrap;
		const both = h("div", { class: "pool-wrap" });
		both.append(wrap, zone);
		return both;
	}

	/**
	 * The units this cult holds captive: one icon each, in the colour of the cult
	 * they belong to. They go home — to their owners' pools — in Gather Power.
	 */
	function capturedZone(seat, sv) {
		const held = (sv && sv.captured) || [];
		if (!held.length) return null;
		const zone = h("div", { class: "captured-zone", id: `captured-${seat}`, "data-seat": seat });
		zone.append(h("div", { class: "zone-title small muted" }, `Captured (${held.length})`));
		const icons = h("div", { class: "pool-icons captured-icons" });
		for (const c of held) {
			const def = S.info.pieces.find((p) => p.type === c.unit);
			const ownerColor = colorOf(c.owner);
			const unit = unitCardFor(c.owner, c.unit);
			const name = unit ? unit.name : def ? def.label : c.unit;
			const fate =
				c.fate === "sacrificed"
					? `Held by ${seatLabel(seat)}: sacrificed in Gather Power for 1 Power, then back to ${seatLabel(c.owner)}'s pool`
					: `Held by ${seatLabel(seat)}: goes back to ${seatLabel(c.owner)}'s pool in Gather Power`;
			const icon = h(
				"span",
				{
					class: "pool-unit captured",
					tabindex: "0",
					"data-piece": c.unit,
					"data-owner": c.owner,
					role: "img",
					"aria-label": `${seatLabel(c.owner)} ${name}: ${fate}`,
				},
				pieceIcon(def ? def.shape : "disc", ownerColor, 26)
			);
			hoverCard(icon, () => {
				if (unit) return unitTip(unit, ownerColor, null, fate);
				const box = h("div", { class: "unit-tip" });
				box.append(h("div", { class: "unit-tip-head" }, pieceIcon(def ? def.shape : "disc", ownerColor, 34), h("div", { class: "unit-tip-title" }, h("strong", {}, name))));
				box.append(h("p", { class: "unit-where small" }, fate));
				return box;
			});
			icons.append(icon);
		}
		zone.append(icons);
		return zone;
	}

	function factionCards(seat, sv) {
		const wrap = h("div", { class: "faction-cards", "data-seat": seat });
		// A faction's sections are told apart by a rule between them: the numbers are in the summary.
		const plain = !!seatGlyph(seat);
		const section = (id, title, count, body) => {
			if (plain) {
				if (body) wrap.append(h("section", { class: "faction-card plain", "data-card": id, "aria-label": title }, body));
				return;
			}
			wrap.append(h("section", { class: "faction-card", "data-card": id }, h("h4", { class: "faction-heading" }, title, count ? h("span", { class: "muted small" }, ` ${count}`) : null), body));
		};
		const rowsBody = (card) => {
			const ul = h("ul", { class: "card-rows" });
			for (const r of card.rows) {
				ul.append(
					h(
						"li",
						{ class: r.state || (r.detail ? "info-row" : null) },
						r.state ? h("span", { class: "row-mark" }, spellMark(r.state === "done", r.state === "done" ? "Done" : "Not yet")) : null,
						h("span", { class: "row-text" }, r.state || !r.detail ? r.text : h("strong", {}, r.text)),
						r.detail && !r.state ? h("span", { class: "muted small row-detail" }, " — " + r.detail) : null
					)
				);
			}
			return ul;
		};
		const counted = (card) => {
			const done = card.rows.filter((r) => r.state === "done").length;
			const todo = card.rows.filter((r) => r.state).length;
			return todo ? `${done}/${todo}` : "";
		};
		const bySlot = new Map((sv.cards || []).map((c) => [c.id, c]));
		// The faction ability first, then what the cult may earn, its Elder Signs
		// and the units still to be placed. Games with other cards list them as sent.
		const ability = bySlot.get("ability");
		if (ability) section(ability.id, ability.title, "", rowsBody(ability));
		const books = bySlot.get("books");
		if (books) section(books.id, books.title, spellbookCount(sv), spellbookGrid(sv, books, bySlot.get("reqs")));
		for (const card of sv.cards || []) {
			if (["ability", "books", "reqs", "units"].includes(card.id)) continue;
			section(card.id, card.title, counted(card), rowsBody(card));
		}
		if (sv.hidden_signs !== undefined) {
			const total = (sv.hidden_signs || 0) + (sv.shown_signs || []).length;
			section("signs", "Elder Signs", String(total), total ? elderSignsBody(seat, sv) : null);
		}
		if (bySlot.has("units")) {
			const inPool = (sv.units || []).reduce((n, u) => n + (u.pool || 0), 0);
			section("units", "Units", `${inPool} in the pool`, poolBody(seat, sv));
		}
		return wrap;
	}

	/** How many spellbooks a cult holds of its six, the neutral ones among them. */
	function spellbookCount(sv) {
		const counter = (sv.counters || []).find((c) => c.id === "books");
		return `${counter.value}/${counter.max}`;
	}

	/**
	 * A cult's requirements, then its spellbooks still to be earned, in two columns. A spellbook
	 * earned takes the place of a requirement met, so the list shortens as the cult grows. The
	 * server keeps what is met and what is held, not which paid for which: they pair up in order.
	 */
	function spellbookGrid(sv, books, reqs) {
		const grid = h("div", { class: "spellbook-grid" });
		const held = books.rows.filter((r) => r.state === "done");
		const earned = [...held];
		for (const r of reqs ? reqs.rows : []) grid.append(r.state === "done" && earned.length ? bookTile(earned.shift()) : requirementTile(r));
		// A spellbook held with no requirement to its name: a position set up that way.
		for (const r of earned) grid.append(bookTile(r));
		// With all six a cult can earn no more, even if a neutral one took the place of its own.
		const counter = (sv.counters || []).find((c) => c.id === "books");
		if (counter.value < counter.max) for (const r of books.rows) if (r.state !== "done") grid.append(bookTile(r));
		return grid;
	}

	/** A spellbook, by its name; the hover card has its rule. */
	function bookTile(row) {
		const on = row.state === "done";
		const tile = h("span", { class: "spell-tile book " + (on ? "held" : "locked"), tabindex: "0", "aria-label": `${on ? "Spellbook" : "Spellbook to earn"}: ${row.text}` }, spellMark(on, on ? "Unlocked" : "Not yet", 14), h("span", { class: "spell-text" }, row.text));
		hoverCard(tile, () => {
			const i = (row.detail || "").indexOf(": ");
			return h(
				"div",
				{ class: "unit-tip action-tip" },
				h("div", { class: "unit-tip-head" }, h("div", { class: "unit-tip-title" }, h("strong", {}, row.text), i > 0 ? h("span", { class: "muted small" }, row.detail.slice(0, i)) : null)),
				h("p", { class: "unit-text small" }, i > 0 ? row.detail.slice(i + 2) : row.detail || "")
			);
		});
		return tile;
	}

	/** A requirement, cut to fit; the hover card has all of it. */
	function requirementTile(row) {
		const met = row.state === "done";
		const tile = h("span", { class: "spell-tile req" + (met ? " met" : ""), tabindex: "0", "aria-label": `${met ? "Requirement met" : "Requirement"}: ${row.text}` }, h("span", { class: "spell-text" }, row.text));
		hoverCard(tile, () =>
			h(
				"div",
				{ class: "unit-tip action-tip" },
				h("div", { class: "unit-tip-head" }, h("div", { class: "unit-tip-title" }, h("strong", {}, met ? "Requirement met" : "Spellbook requirement"), met ? h("span", { class: "muted small" }, "A spellbook is earned for it") : null)),
				h("p", { class: "unit-text small" }, row.text)
			)
		);
		return tile;
	}

	/** A cult's colour swatch and name, for turn orders. */
	function seatChip(seat, first) {
		return h(
			"span",
			{ class: "order-seat" + (first ? " first" : ""), "data-seat": seat, title: `${seatLabel(seat)} — ${displayName(seat)}${first ? " (first player)" : ""}` },
			h("span", { class: `swatch color-${cssColor(colorOf(seat))}` }),
			seatLabel(seat)
		);
	}

	/** The cults in play order, each way round, as arrowed chips. */
	function orderLine(seats) {
		const line = h("span", { class: "order-line" });
		seats.forEach((seat, i) => {
			if (i) line.append(h("span", { class: "order-sep", "aria-hidden": "true" }, "→"));
			line.append(seatChip(seat, i === 0));
		});
		return line;
	}

	const PHASE_HINTS = {
		action: "Action phase: in turn order everyone spends Power on one action at a time until nobody has any left.",
		gather: "Gather Power: Cultists, Gates and captives pay out Power; nobody ends below half the leader.",
		first: "Determine first player: the most Power goes first and chooses the direction of play.",
		doom: "Doom phase: Gates score Doom; Rituals of Annihilation and Elder Signs are played in turn.",
	};

	/** Round, the phases in order, who plays when, the Ritual track and the bag. */
	function roundBar() {
		const v = rulesetView();
		if (!v || !v.round || v.table) return null;
		const bar = h("div", { class: "round-bar", id: "round-bar" });
		bar.append(h("span", { class: "round-item round-no" }, h("strong", {}, `Round ${v.round}`)));

		// The phases of a round, left to right in the order they run; the current
		// one is lit and the ones already played this round are ticked.
		const steps = h("ol", { class: "phase-steps", id: "phase-steps", "aria-label": "Phases of the round" });
		for (const ph of v.phases || []) {
			steps.append(
				h(
					"li",
					{
						class: `phase-step ${ph.state}`,
						"data-phase": ph.id,
						"data-state": ph.state,
						"aria-current": ph.state === "current" ? "step" : null,
						title: PHASE_HINTS[ph.id] || null,
					},
					ph.state === "done" ? h("span", { class: "phase-mark", "aria-hidden": "true" }, "✓") : null,
					ph.label
				)
			);
		}
		bar.append(steps);
		// All that follows the phases stands at the right of the card.
		const tail = h("div", { class: "round-tail", id: "round-tail" });
		if (v.phase === "over") tail.append(h("span", { class: "round-item alarm" }, "Game over"));

		if (v.ritual && v.ritual.track && v.ritual.track.length) tail.append(ritualTrack(v.ritual));
		if (v.bag) {
			tail.append(
				h(
					"span",
					{
						class: "round-item bag",
						id: "elder-bag",
						"data-left": v.bag.left,
						"data-total": v.bag.total,
						title: `Elder Signs still in the bag: ${v.bag.left} of ${v.bag.total}. Their values are secret; the count is not.`,
						"aria-label": `Elder Signs in the bag: ${v.bag.left} of ${v.bag.total}`,
					},
					statIcon("signs", "Elder Signs in the bag", 16),
					h("strong", {}, `${v.bag.left}/${v.bag.total}`)
				)
			);
		}
		if (tail.children.length) bar.append(tail);
		return bar;
	}

	/**
	 * The Ritual of Annihilation track: a space for each Ritual that may still be
	 * performed, with its cost in Power, and the marker on the one the next Ritual
	 * pays. After the last space comes Instant Death: the game ends when that
	 * Doom phase has been scored.
	 */
	function ritualTrack(r) {
		const wrap = h("span", { class: "round-item ritual" + (r.instant ? " instant" : ""), id: "ritual" });
		wrap.append(h("span", { class: "ritual-label" }, "Ritual of Annihilation"));
		const steps = h("ol", {
			class: "ritual-track",
			id: "ritual-track",
			"aria-label": `Ritual of Annihilation track: ${r.track.length} spaces, the marker is on ${r.instant ? "Instant Death" : "space " + (r.marker + 1)}`,
		});
		r.track.forEach((cost, i) => {
			const state = i < r.marker ? "past" : i === r.marker && !r.instant ? "next" : "ahead";
			steps.append(
				h(
					"li",
					{
						class: `ritual-step ${state}`,
						"data-step": i,
						"data-cost": cost,
						"data-state": state,
						title:
							state === "past"
								? `Space ${i + 1}: a Ritual was performed here (${cost} Power)`
								: state === "next"
								? `Space ${i + 1}: the next Ritual costs ${cost} Power`
								: `Space ${i + 1}: a later Ritual will cost ${cost} Power`,
					},
					String(cost)
				)
			);
		});
		steps.append(
			h(
				"li",
				{
					class: "ritual-end" + (r.instant ? " reached" : ""),
					"data-state": r.instant ? "reached" : "ahead",
					title: "Instant Death: when the marker gets here the game ends once the Doom phase has been scored.",
				},
				"☠"
			)
		);
		wrap.append(steps);
		// The marker on the track already says what the next Ritual costs; only
		// Instant Death needs a word.
		if (r.instant) {
			wrap.append(h("span", { class: "small ritual-note alarm" }, "Instant Death — the game ends when this Doom phase is scored"));
		}
		return wrap;
	}

	const CLASS_LABEL = { cultist: "Cultist", monster: "Monster", terror: "Terror", goo: "Great Old One" };

	/** "color|unit" of every unit one of whose spellbooks its cult has earned. */
	function enhancedUnits() {
		const out = new Set();
		const rv = rulesetView();
		if (!rv || !rv.seats) return out;
		for (const sd of S.info.seats) {
			const sv = rv.seats[String(sd.seat)];
			if (!sv) continue;
			for (const u of sv.units || []) if (u.enhanced) out.add(`${sd.color}|${u.id}`);
		}
		return out;
	}

	/** The information card of the unit last clicked on the map. */
	function unitInfoCard() {
		const ui = S.unitInfo;
		if (!ui || !isRuleset()) return null;
		const rv = rulesetView();
		const sd = S.info.seats.find((x) => x.color === ui.color);
		const sv = rv && rv.seats && sd ? rv.seats[String(sd.seat)] : null;
		const unit = sv ? (sv.units || []).find((x) => x.id === ui.piece) : null;
		const here = ((currentCells()[ui.pos] || []).filter((pc) => pc.color === ui.color && pc.piece === ui.piece) || []).length;
		if (!unit || !here) {
			// The unit has gone from there: nothing to look at any more.
			S.unitInfo = null;
			return null;
		}
		const def = S.info.pieces.find((p) => p.type === ui.piece);
		const card = h("div", { class: "card unit-info", id: "unit-info", "data-piece": ui.piece, "data-seat": sd.seat });
		card.append(
			h(
				"div",
				{ class: "unit-info-head" },
				pieceIcon(def ? def.shape : "disc", ui.color, 30),
				h(
					"div",
					{ class: "unit-info-title" },
					h("strong", {}, unit.name),
					h("span", { class: "muted small" }, `${seatLabel(sd.seat)} · ${CLASS_LABEL[unit.class] || unit.class}${seatName(sd.seat) ? " · " + seatName(sd.seat) : ""}`)
				),
				h("button", { class: "ghost icon", id: "unit-info-close", title: "Close (Esc)", "aria-label": "Close the unit details", onclick: closeUnitInfo }, "×")
			)
		);
		const stats = h("dl", { class: "unit-stats" });
		const stat = (label, value, title) => stats.append(h("div", { title: title || null }, h("dt", {}, label), h("dd", {}, value)));
		const combat = /^\d+$/.test(unit.combat) ? (unit.combat === "0" ? "none" : `${unit.combat} ${unit.combat === "1" ? "die" : "dice"}`) : unit.combat;
		stat(unit.class === "cultist" ? "Recruit" : unit.class === "goo" ? "Awaken" : "Summon", `${unit.cost} Power`);
		stat("Combat", combat);
		stat("Here", `${here} in ${areaName(ui.pos)}`, "units of this kind in the area you clicked");
		stat("On the map", `${unit.on_map} of ${unit.total}`);
		stat("Pool", String(unit.pool), "units still to be placed");
		if (unit.away) stat("Submerged", String(unit.away));
		if (unit.captured) stat("Captured", String(unit.captured), "Cultists held on other cults' cards");
		card.append(stats);
		const abilities = abilitiesBlock(unit);
		if (abilities) card.append(abilities);
		const enter = enterBlock(unit);
		if (enter) card.append(enter);
		const text = unitText(unit);
		if (text) card.append(h("p", { class: "unit-text small" }, text));
		if (unit.books && unit.books.length) {
			card.append(h("p", { class: "small muted unit-books-title" }, "Spellbooks that enhance this unit"));
			const ul = h("ul", { class: "card-rows unit-books" });
			for (const b of unit.books) {
				ul.append(
					h(
						"li",
						{ class: b.unlocked ? "done" : "todo", "data-book": b.id, title: b.text },
						h("span", { class: "row-mark" }, spellMark(b.unlocked, b.unlocked ? `${b.name}: unlocked` : `${b.name}: not unlocked`)),
						h("span", { class: "row-text" }, h("strong", {}, b.name), h("span", { class: "muted small" }, ` — ${b.timing}: ${b.text}`))
					)
				);
			}
			card.append(ul, ...bookRequirements(unit));
		} else {
			card.append(h("p", { class: "small muted" }, "No spellbook enhances this unit."));
		}
		return card;
	}

	const NEUTRAL_GROUPS = [
		["book", "Spellbooks"],
		["monster", "Monsters"],
		["terror", "Terrors"],
		["goo", "Great Old Ones"],
	];

	/**
	 * What is still there to take of the neutral spellbooks and Loyalty Cards: a
	 * unit is just its icon, a spellbook the book pictogram and its name; hover for
	 * the rules and the price. Null at a table that plays without neutral content.
	 */
	function neutralCard() {
		const rv = rulesetView();
		const nv = rv && rv.neutral;
		if (!nv) return null;
		const items = nv.items || [];
		const card = h("div", { class: "card neutral-card", id: "neutral-window" }, h("h3", {}, "Neutral items"));
		if (!items.length) {
			card.append(h("p", { class: "muted small", id: "neutral-empty" }, "Everything has been taken."));
			return card;
		}
		const whole = isMobileLayout();
		const body = h("div", { class: "seat-more", id: "neutral-items", hidden: !whole && S.expanded !== "neutral" });
		for (const [kind, title] of NEUTRAL_GROUPS) {
			const group = items.filter((it) => it.kind === kind);
			if (!group.length) continue;
			const row = h("div", { class: "neutral-items neutral-" + kind, "data-kind": kind });
			for (const item of group) {
				const label = `${item.name}: available for ${item.obtain}`;
				let node;
				if (kind === "book") {
					node = h("span", { class: "neutral-item neutral-book", tabindex: "0", "data-item": item.id, "aria-label": label }, spellMark(true, item.name, 16), h("span", {}, item.name));
				} else {
					const def = S.info.pieces.find((p) => p.type === item.id);
					node = h("span", { class: "neutral-item neutral-unit", tabindex: "0", role: "img", "data-item": item.id, "aria-label": label }, pieceIcon(def ? def.shape : "disc", "black", 32));
				}
				hoverCard(node, () => neutralTip(item, S.info.pieces));
				row.append(node);
			}
			body.append(h("p", { class: "neutral-title small muted" }, title), row);
		}
		card.append(body);
		if (!whole) card.append(expandToggle("neutral", "the neutral items"));
		return card;
	}

	/** The hover card of a unit: its icon, numbers, what it can do and the spellbooks that bear on it. */
	function unitTip(unit, color, cost, where) {
		const def = S.info.pieces.find((p) => p.type === unit.id);
		const box = h("div", { class: "unit-tip" });
		box.append(
			h(
				"div",
				{ class: "unit-tip-head" },
				pieceIcon(def ? def.shape : "disc", color, 34),
				h("div", { class: "unit-tip-title" }, h("strong", {}, unit.name), h("span", { class: "muted small" }, CLASS_LABEL[unit.class] || unit.class))
			)
		);
		// Which of the cult's units this is: in the pool, or where it stands.
		if (where) box.append(h("p", { class: "unit-where small" }, h("strong", {}, "This one: "), where));
		const combat = /^\d+$/.test(unit.combat) ? (unit.combat === "0" ? "none" : `${unit.combat} ${unit.combat === "1" ? "die" : "dice"}`) : unit.combat;
		const stats = h("dl", { class: "unit-stats" });
		const stat = (label, value) => stats.append(h("div", {}, h("dt", {}, label), h("dd", {}, value)));
		stat(unit.class === "cultist" ? "Recruit" : unit.class === "goo" ? "Awaken" : "Summon", `${cost === undefined || cost === null ? unit.cost : cost} Power`);
		stat("Combat", combat);
		stat("In the pool", `${unit.pool} of ${unit.total}`);
		if (unit.on_map) stat("On the map", String(unit.on_map));
		box.append(stats);
		const abilities = abilitiesBlock(unit);
		if (abilities) box.append(abilities);
		const enter = enterBlock(unit);
		if (enter) box.append(enter);
		// A Great Old One's text restates where it awakens: the steps above say it.
		const text = unitText(unit);
		if (text) box.append(h("p", { class: "unit-text small" }, text));
		if (unit.books && unit.books.length) {
			const ul = h("ul", { class: "card-rows unit-books" });
			for (const b of unit.books) {
				ul.append(
					h(
						"li",
						{ class: b.unlocked ? "done" : "todo" },
						h("span", { class: "row-mark" }, spellMark(b.unlocked, b.unlocked ? `${b.name}: unlocked` : `${b.name}: not unlocked`)),
						h("span", { class: "row-text" }, h("strong", {}, b.name), h("span", { class: "muted small" }, ` (${b.timing}): ${b.text}`))
					)
				);
			}
			box.append(h("p", { class: "small muted unit-books-title" }, "Spellbooks"), ul, ...bookRequirements(unit));
		}
		return box;
	}

	return { factionCards, spellbookCount, spellbookGrid, bookTile, requirementTile, elderSignsBody, poolBody, capturedZone, PHASE_HINTS, roundBar, ritualTrack, CLASS_LABEL, enhancedUnits, unitInfoCard, neutralCard, NEUTRAL_GROUPS, unitTip, seatChip, orderLine };
}
