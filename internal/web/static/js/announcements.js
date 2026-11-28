// Major announcements (ADR 0155), in the welcome tour's card (js/tour-card.js):
// a slide each, its words the Markdown its administrator wrote, untranslated.
// The main page offers those waiting for this guest or account (home.js asks
// offerAnnouncements, only when /auth/me names some), and closing the card marks
// every one it showed as seen. The help window's Recent Changes opens any one
// alone (openAnnouncement), which marks nothing.

import { api } from "./api.js";
import { t } from "./i18n.js";
import { openCard } from "./tour-card.js";
import { renderMarkdown } from "./markdown.js";

const slideOf = (a) => ({
	id: `announcement-${a.id}`,
	title: a.title,
	body: renderMarkdown(a.body, { idPrefix: `announcement-${a.id}-` }).node,
});

function show(list, onClose) {
	return openCard({ id: "announcement-card", slides: list.map(slideOf), skip: t("common.close"), onClose });
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
