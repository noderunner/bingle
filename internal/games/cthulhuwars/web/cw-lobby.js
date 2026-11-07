// What a Cthulhu Wars table was set up with, as pictures, for the lobby: its
// map, then the factions and all that was added (ADR 0124: the lobby asks
// page.js's lobbyPool for it).

import { h } from "/static/js/util.js";
import { api } from "/static/js/api.js";
import { setupIcon } from "./cw-setup.js";

const catalogs = new Map(); // by family: the setup catalog, once read

/** What this table was set up with, as pictures: its map, then the factions and all that was added. */
export function lobbyPool(info) {
	const family = info.family || info.id;
	const box = h("div", { class: "card lobby-pool", id: "lobby-pool", "aria-label": "What this table plays with" });
	const paint = (catalog) => {
		const params = info.params || {};
		const has = (key, value) => (params[key] || []).includes(value);
		const pool = params.faction_pool || [];
		const shown = [];
		const m = /\(([^)]*)\)/.exec(info.name || "");
		shown.push({ kind: "map", id: String(info.id).split("~")[0], name: m ? m[1] : info.name, art: params.art || "" });
		for (const tab of catalog.tabs) {
			for (const item of tab.items) {
				const on =
					tab.id === "factions"
						? pool.length
							? pool.includes(item.value)
							: info.seats.some((sd) => sd.label === item.name)
						: item.option === "high-priests"
							? !!params.high_priests
							: item.option === "neutral-books"
								? has("neutral_books", item.value)
								: tab.id === "neutrals"
									? has("neutral_units", item.value)
									: tab.id === "independents" && has("independents", item.id);
				if (on) shown.push(item);
			}
		}
		for (const item of shown) {
			box.append(h("span", { class: "lobby-pool-item", title: item.name, "data-kind": item.kind, "data-item": item.id, role: "img", "aria-label": item.name }, setupIcon(item, 34, family, info.pieces)));
		}
	};
	if (catalogs.has(family)) {
		paint(catalogs.get(family));
	} else {
		api.setupCatalog(info.id)
			.then((catalog) => {
				catalogs.set(family, catalog);
				paint(catalog);
			})
			.catch(() => {});
	}
	return box;
}
