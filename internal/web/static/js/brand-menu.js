// The menu on the logo, on a server that offers the bingle-3d client (ADR 0151): the way
// home, a download for each build the server has, and the client's setup file. The page
// comes with the menu written out (templates/brand.html.tmpl); this opens and closes it,
// moves through it with the keys a menu takes, and fetches the setup file, which is made
// for whoever asks and so is asked for with a POST.

import { api } from "./api.js";
import { t } from "./i18n.js";
import { lastName, toast } from "./util.js";

const SETUP_FILE = "bingle-3d-setup.toml";

const button = document.getElementById("brand-menu-button");
const menu = document.getElementById("brand-menu");

if (button && menu) {
	const items = () => [...menu.querySelectorAll('[role="menuitem"]')];
	const open = () => !menu.hidden;
	const close = (refocus) => {
		menu.hidden = true;
		button.setAttribute("aria-expanded", "false");
		if (refocus) button.focus();
	};
	// The setup file is the signed-in player's, or the guest's: the page cannot know the name.
	const setup = document.getElementById("brand-menu-setup");
	const nameSetup = () => {
		if (setup) setup.textContent = t("bar.setup_file", { name: lastName() }).trim();
	};
	const show = () => {
		nameSetup();
		menu.hidden = false;
		button.setAttribute("aria-expanded", "true");
		items()[0]?.focus();
	};

	button.addEventListener("click", (ev) => {
		ev.stopPropagation();
		if (open()) close(false);
		else show();
	});
	// The page may be replaced without the document being loaded again (js/nav.js): what
	// listens on the document for a menu that is gone lets go by itself.
	const here = (fn) => {
		const on = (ev) => {
			if (menu.isConnected) return fn(ev);
			document.removeEventListener(ev.type, on);
		};
		return on;
	};
	document.addEventListener(
		"click",
		here((ev) => {
			if (open() && !ev.composedPath().includes(menu)) close(false);
		})
	);
	document.addEventListener(
		"keydown",
		here((ev) => {
			if (ev.key === "Escape" && open()) close(true);
		})
	);
	menu.addEventListener("keydown", (ev) => {
		const list = items();
		const at = list.indexOf(document.activeElement);
		const to = { ArrowDown: at + 1, ArrowUp: at - 1, Home: 0, End: list.length - 1 }[ev.key];
		if (to === undefined) return;
		ev.preventDefault();
		list[(to + list.length) % list.length]?.focus();
	});
	// A download leaves the page where it is, so the menu closes behind it.
	for (const link of menu.querySelectorAll("a[download]")) link.addEventListener("click", () => close(false));

	setup?.addEventListener("click", async () => {
		close(false);
		try {
			const url = URL.createObjectURL(await api.clientSetup());
			const link = document.createElement("a");
			link.href = url;
			link.download = SETUP_FILE;
			document.body.append(link);
			link.click();
			link.remove();
			setTimeout(() => URL.revokeObjectURL(url), 60000);
		} catch (err) {
			toast(err?.status ? err.message : t("bar.setup_failed"), "error");
		}
	});
}
