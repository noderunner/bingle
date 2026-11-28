// The bingle-3d client's setup file (ADR 0151), for the logo menu and the quick setup page.
// It is made for whoever asks and so is asked for with a POST, and saved under its name.

import { api } from "./api.js";
import { t } from "./i18n.js";
import { lastName, toast } from "./util.js";

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
