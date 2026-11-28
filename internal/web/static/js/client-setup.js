// The bingle-3d client's setup file (ADR 0151), for the logo menu and the quick setup page,
// and the link that opens a game in the client (ADR 0163). Both are made for whoever asks,
// and so are asked for with a POST.

import { api } from "./api.js";
import { t } from "./i18n.js";
import { isMobileLayout } from "./mobile.js";
import { h, lastName, toast } from "./util.js";

const SETUP_FILE = "bingle-3d-setup.toml";

// What the setup file's button says: the signed-in player's, or the guest's, name in it.
export function setupLabel() {
	return t("bar.setup_file", { name: lastName() }).trim();
}

export async function downloadSetup() {
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
}

/** The platform the client is built for that this computer is, "windows" or "linux", or "" for any other. */
export function platform() {
	const hint = navigator.userAgentData?.platform || navigator.userAgent || "";
	if (/android/i.test(navigator.userAgent || "")) return "";
	if (/win/i.test(hint)) return "windows";
	if (/linux|x11|cros/i.test(hint)) return "linux";
	return "";
}

// How long the page waits after trying a link before it offers the download instead.
const NOT_FOUND_AFTER = 1500;

/**
 * "Join with Bingle3D" or "Watch with Bingle3D" for a room (action "join" or "watch"), on a
 * computer the server has a build of the client for and not in the phone layout; null
 * anywhere else. A click runs before (an invitation accepted, say), then opens the link.
 */
export function clientLink(room, action, before) {
	const platforms = document.getElementById("brand-menu")?.dataset.platforms || "";
	if (!platform() || !platforms.split(" ").includes(platform()) || isMobileLayout()) return null;
	const button = h(
		"button",
		{ type: "button", class: "ghost small client-link", "data-client-link": action, onclick: () => openInClient(button, room, action, before) },
		t(action === "watch" ? "bar.watch_3d" : "bar.join_3d")
	);
	return button;
}

/**
 * Opens the link: bingle-3d://join or watch, with this server, the room and a code good once
 * for a minute, which the client trades for its own sign-in as this player. Without a code
 * (an administrator gets none) the client joins as whoever it is there. A page cannot ask
 * whether the link has a program behind it: if the page still has the focus a moment later,
 * the download is offered beside the button.
 */
async function openInClient(button, room, action, before) {
	if (before) await before();
	const params = [
		["server", location.origin],
		["room", room],
	];
	try {
		params.push(["code", (await api.clientCode()).code]);
	} catch (_) {
		/* the link without one */
	}
	let away = false;
	const leave = () => (away = true);
	window.addEventListener("blur", leave);
	document.addEventListener("visibilitychange", leave);
	const link = document.createElement("a");
	link.href = `bingle-3d://${action}?` + params.map(([k, v]) => `${k}=${encodeURIComponent(v)}`).join("&");
	document.body.append(link);
	link.click();
	link.remove();
	setTimeout(() => {
		window.removeEventListener("blur", leave);
		document.removeEventListener("visibilitychange", leave);
		if (away || !button.isConnected || button.nextElementSibling?.classList.contains("client-get")) return;
		button.after(h("a", { class: "client-get small", href: "/client" }, t("bar.client")));
	}, NOT_FOUND_AFTER);
}
