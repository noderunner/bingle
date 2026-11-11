// Where a learner is in a tutorial (ADR 0077), worked out from the moves the
// room has played, so a reload, a second tab or a phone all agree.
//
// A step's moves are its `play` (the learner's) then its `reply` (played by
// the page for the other seats). A step without `play` waits for the learner
// to read it and press Next before its replies are played; `read` is how many
// steps they have pressed Next on. Moves that do not follow the script (an
// a step taken back, a move made elsewhere) leave the tutorial "off script".

const same = (a, b) => String(a).trim().toLowerCase() === String(b).trim().toLowerCase();

/** The moves of step i, in order: [{who: "learner"|"reply", notation}]. */
export function stepMoves(step) {
	return [...(step.play || []).map((n) => ({ who: "learner", notation: n })), ...(step.reply || []).map((n) => ({ who: "reply", notation: n }))];
}

/**
 * progress(tutorial, played, read) says where the learner is:
 *   {step, next, needsNext, finished, offScript}
 * step:      the index of the step to show
 * next:      the move expected next ({who, notation, index}), or null; index
 *            is its place among the step's moves (the learner's come first)
 * needsNext: the step waits for the learner to press Next
 * finished:  every step is done
 * offScript: the moves played are not the script's
 */
export function progress(tut, played, read) {
	const steps = tut.steps || [];
	const moves = played || [];
	let k = 0;
	for (let i = 0; i < steps.length; i++) {
		const seq = stepMoves(steps[i]);
		const reading = !(steps[i].play || []).length && i >= read;
		for (let j = 0; j < seq.length; j++) {
			if (k + j >= moves.length) {
				if (reading) return { step: i, next: null, needsNext: true, finished: false, offScript: false };
				return { step: i, next: { ...seq[j], index: j }, needsNext: false, finished: false, offScript: false };
			}
			if (!same(moves[k + j], seq[j].notation)) return { step: i, next: null, needsNext: false, finished: false, offScript: true };
		}
		k += seq.length;
		// A move played beyond the script while a step waits to be read.
		if (reading && k < moves.length) return { step: i, next: null, needsNext: false, finished: false, offScript: true };
		if (reading) return { step: i, next: null, needsNext: true, finished: false, offScript: false };
	}
	if (k < moves.length) return { step: Math.max(steps.length - 1, 0), next: null, needsNext: false, finished: false, offScript: true };
	return { step: Math.max(steps.length - 1, 0), next: null, needsNext: false, finished: true, offScript: false };
}

/** Whether a move's notation is the one expected. */
export function isExpected(next, notation) {
	return !!next && next.who === "learner" && same(next.notation, notation);
}

const DONE_KEY = "bingle.tutorials.done";

/** The tutorials this browser has finished, as "family/id" (kept only to mark them in the list). */
export function finishedTutorials(store) {
	const list = store.get(DONE_KEY, []);
	return new Set(Array.isArray(list) ? list : []);
}

/** Marks a tutorial finished in this browser. */
export function markFinished(store, ref) {
	const done = finishedTutorials(store);
	if (done.has(ref)) return;
	done.add(ref);
	store.set(DONE_KEY, [...done]);
}

/**
 * allowed(expected, chosen, kind, value): whether a selection may be made
 * now. expected are the selections of the move the step asks for, chosen
 * those made so far; only the next expected one is allowed.
 */
export function allowed(expected, chosen, kind, value) {
	if (!expected) return false;
	const next = expected[chosen.length];
	return !!next && next.kind === kind && String(next.value) === String(value);
}

/**
 * Where the page has each part a step can pick out (Step.spotlight, ADR 0111).
 * `seat` makes a selector for a seat's part. `pane` is the tab of the column it
 * lies in; `open` says it is in the seat's details, which are folded away.
 * internal/tutorial lists the same names, and a test keeps the two the same.
 */
const seatPart = (inner, open = false) => ({ seat: (n) => `.seat-strip[data-seat="${n}"] ${inner}`, pane: "seats", open });
const PAGE_PARTS = {
	actions: { selector: ".move-bar" },
	battle: { selector: ".exchange.battle" },
	"elder-bag": { selector: ".round-item.bag" },
	"first-player": { selector: ".first-mark", pane: "seats" },
	history: { selector: ".side-moves", pane: "history" },
	"history-since": { selector: ".log-since", pane: "history" },
	impulse: { selector: ".impulse-card" },
	options: { selector: "#open-options" },
	phases: { selector: ".phase-steps" },
	"ritual-track": { selector: ".ritual-track" },
	"round-bar": { selector: ".round-bar" },
	rules: { selector: "#show-rules" },
	books: seatPart('[data-counter="books"]'),
	doom: seatPart('[data-counter="doom"]'),
	"elder-signs": seatPart('.faction-card[data-card="signs"]', true),
	gates: seatPart('[data-counter="gates"]'),
	power: seatPart('[data-counter="power"]'),
	signs: seatPart('[data-counter="signs"]'),
	spellbooks: seatPart('.faction-card[data-card="books"]', true),
	units: seatPart('.faction-card[data-card="units"]', true),
};

/**
 * pagePart(part, learner) is where a step's part is: {selector, pane, openSeat}.
 * A part of a seat's panel is the learner's unless it names a seat ("doom:2").
 */
export function pagePart(part, learner) {
	const [name, seat] = String(part).split(":");
	const where = PAGE_PARTS[name];
	if (!where) return null;
	const n = Number(seat) || learner;
	return { selector: where.seat ? where.seat(n) : where.selector, pane: where.pane || null, openSeat: where.open ? n : null };
}

/** The unit a move of the map sends where: {piece, from, to, leaving}, or null for any other move. */
function unitMove(notation) {
	const found = /^(go|leave):([^@]+)@([^>]+)>(.+)$/.exec(notation || "");
	return found ? { piece: found[2], from: found[3], to: found[4], leaving: found[1] === "leave" } : null;
}

/**
 * The units the script moves together from here: the moves of the step from
 * `index` on that send a unit into the same area, from wherever it stands, up to
 * the first that does something else or goes elsewhere. The learner gathers them
 * on the map and clicks their destination, as in any game, so they count as one
 * thing to do.
 */
export function scriptedMoveGroup(step, index) {
	const group = [];
	for (const notation of (step.play || []).slice(index)) {
		const mv = unitMove(notation);
		if (!mv || (group.length && mv.to !== group[0].to)) break;
		group.push({ ...mv, notation });
	}
	return group;
}
