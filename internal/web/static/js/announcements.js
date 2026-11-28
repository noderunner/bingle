// Major announcements (ADR 0155), in the welcome tour's card (js/tour-card.js):
// their words the Markdown their administrator wrote, untranslated, in one
// slide or several. The main page offers those waiting for this guest or
// account (home.js asks offerAnnouncements, only when /auth/me names some), and
// closing the card marks every one it showed as seen. The help window's Recent
// Changes opens any one alone (openAnnouncement), which marks nothing.

import { api } from "./api.js";
import { t } from "./i18n.js";
import { openCard } from "./tour-card.js";
import { renderMarkdown } from "./markdown.js";

const RE_FENCE = /^\s{0,3}(```|~~~)/;
const RE_BREAK = /^---\s*$/;
const RE_HEADING = /^\s{0,3}#{1,6}\s+(.*?)\s*#*\s*$/;

/**
 * The parts of a body that are its slides: split at a line of --- after a
 * blank line (or at the start), outside fenced code. Empty parts are dropped.
 */
function splitSlides(body) {
	const parts = [[]];
	let fenced = false;
	let blank = true;
	for (const line of String(body || "").replace(/\r\n?/g, "\n").split("\n")) {
		if (RE_FENCE.test(line)) fenced = !fenced;
		if (!fenced && blank && RE_BREAK.test(line)) parts.push([]);
		else parts[parts.length - 1].push(line);
		blank = /^\s*$/.test(line);
	}
	return parts.map((p) => p.join("\n").trim()).filter(Boolean);
}

// The slides of one announcement. A slide that starts with a heading has it as
// its title; any other has the announcement's. Its pictures are its own, by name.
function slidesOf(a) {
	const assetBase = `/api/announcements/${encodeURIComponent(a.id)}/pictures/`;
	return splitSlides(a.body).map((part, k) => {
		let title = a.title;
		const lines = part.split("\n");
		const m = RE_HEADING.exec(lines[0]);
		if (m) {
			title = m[1];
			part = lines.slice(1).join("\n");
		}
		const id = k ? `announcement-${a.id}-${k + 1}` : `announcement-${a.id}`;
		return { id, title, body: renderMarkdown(part, { idPrefix: `${id}-`, assetBase }).node };
	});
}

function show(list, onClose) {
	return openCard({ id: "announcement-card", slides: list.flatMap(slidesOf), skip: t("common.close"), onClose });
}

/** Shows the announcements waiting for me, the answer of /auth/me, oldest first. */
export async function offerAnnouncements(me) {
	const waiting = new Set(me.announcements || []);
	if (!waiting.size) return null;
	const { announcements } = await api.announcements();
	const list = announcements.filter((a) => waiting.has(a.id));
	if (!list.length) return null;
	return show(list, () => api.announcementsSeen(list.map((a) => a.id)).catch(() => {}));
}

/** Shows one announcement alone, for looking back: nothing is marked seen. */
export function openAnnouncement(a) {
	return show([a], null);
}
