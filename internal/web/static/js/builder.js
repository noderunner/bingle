// MoveBuilder turns the server's full list of legal moves into an
// interactive decision tree. Each legal selection comes with `gestures` — the
// ordered selections that build it (pick a piece type, pick a square, pick a
// pick-up count, ...). The builder tracks the selections made so far and
// reports which selections may come next, so any game's moves can be
// entered click by click with no game-specific UI code.

export class MoveBuilder {
	/** @param moves [{notation, gestures: [{kind, value}]}] */
	constructor(moves) {
		this.moves = moves || [];
		this.gestures = [];
		// How many leading steps were taken automatically because the
		// choice was forced (a single piece type, a single pick-up count).
		this.forced = 0;
	}

	/** Steps the player actually chose. */
	manualGestures() {
		return this.gestures.slice(this.forced);
	}

	/** Legal moves consistent with the selections so far. */
	candidates() {
		const n = this.gestures.length;
		return this.moves.filter((m) => {
			if (!m.gestures || m.gestures.length < n) return false;
			for (let i = 0; i < n; i++) {
				if (m.gestures[i].kind !== this.gestures[i].kind || m.gestures[i].value !== this.gestures[i].value) return false;
			}
			return true;
		});
	}

	/** Distinct selections that may come next: [{kind, value, moves}]. */
	options() {
		const n = this.gestures.length;
		const seen = new Map();
		for (const m of this.candidates()) {
			if (m.gestures.length <= n) continue;
			const st = m.gestures[n];
			const key = st.kind + "\u0000" + st.value;
			const cur = seen.get(key);
			if (cur) cur.moves++;
			else seen.set(key, { kind: st.kind, value: st.value, moves: 1 });
		}
		return [...seen.values()];
	}

	/** The legal move exactly matching the selections so far, if any. */
	complete() {
		const n = this.gestures.length;
		if (n === 0) return null;
		return this.candidates().find((m) => m.gestures.length === n) || null;
	}

	/** True when the selections form a complete move that nothing extends. */
	finished() {
		return this.complete() !== null && this.options().length === 0;
	}

	push(kind, value) {
		this.gestures.push({ kind, value: String(value) });
	}

	pop() {
		const st = this.gestures.pop();
		if (this.forced > this.gestures.length) this.forced = this.gestures.length;
		return st;
	}

	reset() {
		this.gestures = [];
		this.forced = 0;
	}

	/**
	 * Quick placement: while nothing has been selected, an empty square can be
	 * clicked directly if some move is "choose piece P, then square". Returns
	 * Map<position, [piece types that can be placed there]> (in the order the
	 * game lists them).
	 */
	quickPlacements() {
		const out = new Map();
		if (this.gestures.length > 0) return out;
		for (const m of this.moves) {
			const st = m.gestures || [];
			if (st.length === 2 && st[0].kind === "piece" && st[1].kind === "pos") {
				const list = out.get(st[1].value) || [];
				if (!list.includes(st[0].value)) list.push(st[0].value);
				out.set(st[1].value, list);
			}
		}
		return out;
	}

	/** Position values among the next options (squares to highlight). */
	nextPositions() {
		return new Set(this.options().filter((o) => o.kind === "pos").map((o) => o.value));
	}

	nextPieces() {
		return new Set(this.options().filter((o) => o.kind === "piece").map((o) => o.value));
	}

	/** Single-click actions among the next options (e.g. "pass"). */
	nextActions() {
		return this.options().filter((o) => o.kind === "action").map((o) => o.value);
	}

	nextCounts() {
		return this.options()
			.filter((o) => o.kind === "count")
			.map((o) => Number(o.value))
			.sort((a, b) => a - b);
	}

	/**
	 * Automatically takes forced non-square choices (a single piece type, a
	 * single pick-up count) so players never click through no-op prompts.
	 */
	autoAdvance() {
		for (let guard = 0; guard < 8; guard++) {
			const opts = this.options();
			// A card or a box the player chooses by clicking it where it is drawn
			// stays a click even when it is the only one left.
			const manual = opts.length === 1 && opts[0].kind === "action" && MoveBuilder.manual.some((p) => String(opts[0].value).startsWith(p));
			if (opts.length === 1 && opts[0].kind !== "pos" && !manual) {
				const onlyForced = this.forced === this.gestures.length;
				this.push(opts[0].kind, opts[0].value);
				if (onlyForced) this.forced = this.gestures.length;
			} else {
				break;
			}
		}
	}

	/** The squares already selected as drop targets, with piece counts. */
	dropCounts() {
		// After [pos(from), count(n)] every further pos is one drop.
		const out = new Map();
		const ci = this.gestures.findIndex((s) => s.kind === "count");
		if (ci < 0) return out;
		for (const st of this.gestures.slice(ci + 1)) {
			if (st.kind === "pos") out.set(st.value, (out.get(st.value) || 0) + 1);
		}
		return out;
	}

	/** The origin square of a stack move in progress. */
	origin() {
		const first = this.gestures[0];
		const hasCount = this.gestures.some((s) => s.kind === "count");
		return first && first.kind === "pos" && hasCount ? first.value : null;
	}

	/** The pick-up count chosen so far, or 0. */
	pickedCount() {
		const st = this.gestures.find((s) => s.kind === "count");
		return st ? Number(st.value) : 0;
	}
}

/** Action id prefixes that are never taken automatically (set by the game page). */
MoveBuilder.manual = [];
