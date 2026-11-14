// A player's page: /players/{handle}. Their badges, which every account may
// see (ADR 0130), and the ratings they show on a leaderboard. Your own page
// shows you what others see: the badges you hold, without those still
// waiting for their game to end.

import { installTopbar } from "./topbar.js";
import { installNav } from "./nav.js";
import { api, whoAmI } from "./api.js";
import { paintShelf } from "./badges.js";
import { ratingText } from "./ratings.js";

installNav();
installTopbar();

const handle = decodeURIComponent(window.location.pathname.split("/").filter(Boolean).pop() || "");
const title = document.getElementById("player-title");
const note = document.getElementById("player-note");
title.textContent = "@" + handle;
document.title = `${document.title.replace(/ — Player$/, "")} — @${handle}`;

function say(text) {
	note.textContent = text;
	note.hidden = false;
}

function paintRatings() {
	api.playerRatings(handle)
		.then(({ ratings }) => {
			const listed = ratings.filter((r) => r.listed);
			if (!listed.length) return;
			const rows = document.getElementById("player-ratings");
			for (const r of listed) {
				const row = document.createElement("tr");
				row.dataset.family = r.family;
				for (const text of [r.name, ratingText(r.rating, r.provisional), r.won, r.drawn, r.lost]) {
					const cell = document.createElement("td");
					cell.textContent = String(text);
					row.append(cell);
				}
				rows.append(row);
			}
			document.getElementById("player-ratings-box").hidden = false;
		})
		.catch(() => {});
}

async function paint() {
	const me = await whoAmI().catch(() => null);
	if (!me || me.kind !== "account") {
		say("Sign in to see a player's badges.");
		paintRatings();
		return;
	}
	let held;
	try {
		held = (await api.playerBadges(handle)).badges.filter((b) => !b.hidden);
	} catch (err) {
		say(err.status === 404 ? `There is no player @${handle}.` : "Their badges could not be loaded: " + err.message);
		return;
	}
	const self = me.handle && me.handle.toLowerCase() === handle.toLowerCase();
	const mineHeld = self ? null : await api.playerBadges(me.handle).then((r) => r.badges).catch(() => []);
	if (self) say("This is your page as other players see it. Your settings page also shows the badges you have yet to earn.");
	document.getElementById("player-badges-box").hidden = false;
	await paintShelf(document.getElementById("player-badges"), { held, mineHeld });
	paintRatings();
}

paint();
