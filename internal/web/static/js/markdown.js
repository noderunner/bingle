// A small, safe Markdown renderer for the rules window. It builds DOM nodes
// directly (never innerHTML), so a page cannot inject markup or scripts, and
// it understands exactly what long-form game rules need:
//
//   headings, paragraphs, **bold**, *italic*, ~~strike~~, `code`, fenced code
//   blocks, ordered/unordered/nested lists, task items, > quotes and
//   [!NOTE]-style callouts, [[Key]] keycaps, ::: containers (note/tip/warning/important/
//   details), tables with column alignment, horizontal rules, links
//   (http(s), #section, tab:<id>) and images — relative to the game's asset
//   folder, or ![](piece:cthulhu) / ![](seat:1) for the game's own icons.
//
// renderMarkdown(src, opts) returns {node, headings}. Options:
//   assetBase  URL prefix for relative image paths
//   piece(type, color)  → Node for ![](piece:type[:color])
//   marker(kind, arg)   → Node for ![](marker:kind[:arg]) (a mark of the map)
//   seat(n)             → Node for ![](seat:n)
//   onTab(id, anchor)   called for tab:<id>[#anchor] links
//   onAnchor(id)        called for #anchor links (default: scroll to it)
//   idPrefix            prefix of generated heading ids (unique per page)

import { h } from "./util.js";

const CALLOUTS = {
	note: "Note",
	tip: "Tip",
	important: "Important",
	warning: "Warning",
	caution: "Caution",
	danger: "Danger",
	example: "Example",
};

export function renderMarkdown(src, opts = {}) {
	const ctx = { opts, headings: [], used: new Map() };
	const root = h("div", { class: "md" });
	const lines = String(src || "")
		.replace(/\r\n?/g, "\n")
		.replace(/\t/g, "    ")
		.split("\n");
	blocks(lines, root, ctx);
	return { node: root, headings: ctx.headings };
}

/** The headings of a document (for a table of contents) without rendering it. */
export function headingsOf(src) {
	return renderMarkdown(src).headings;
}

// ---------------------------------------------------------------------------
// Blocks
// ---------------------------------------------------------------------------

const RE_FENCE = /^\s{0,3}(```+|~~~+)\s*([\w-]*)\s*$/;
const RE_HEADING = /^\s{0,3}(#{1,6})\s+(.*?)\s*#*\s*$/;
const RE_HR = /^\s{0,3}([-*_])(?:\s*\1){2,}\s*$/;
const RE_QUOTE = /^\s{0,3}>\s?(.*)$/;
const RE_LIST = /^(\s*)([-*+]|\d{1,9}[.)])\s+(.*)$/;
const RE_CONTAINER_OPEN = /^\s{0,3}:::+\s*([A-Za-z][\w-]*)\s*(.*?)\s*$/;
const RE_CONTAINER_CLOSE = /^\s{0,3}:::+\s*$/;
const RE_TABLE_DELIM = /^\s*\|?\s*:?-{1,}:?\s*(\|\s*:?-{1,}:?\s*)*\|?\s*$/;

function blocks(lines, parent, ctx) {
	let i = 0;
	while (i < lines.length) {
		const line = lines[i];
		if (/^\s*$/.test(line)) {
			i++;
			continue;
		}

		let m = RE_FENCE.exec(line);
		if (m) {
			const fence = m[1];
			const body = [];
			i++;
			while (i < lines.length && !new RegExp("^\\s{0,3}" + fence[0] + "{" + fence.length + ",}\\s*$").test(lines[i])) {
				body.push(lines[i]);
				i++;
			}
			i++; // the closing fence
			const code = h("code", {}, body.join("\n"));
			parent.append(h("pre", { class: "md-code", "data-lang": m[2] || null }, code));
			continue;
		}

		m = RE_CONTAINER_OPEN.exec(line);
		if (m && !RE_CONTAINER_CLOSE.test(line)) {
			// Find the matching close, counting nested containers.
			let depth = 1;
			let j = i + 1;
			const inner = [];
			while (j < lines.length) {
				if (RE_CONTAINER_CLOSE.test(lines[j])) {
					depth--;
					if (depth === 0) break;
				} else if (RE_CONTAINER_OPEN.test(lines[j])) {
					depth++;
				}
				inner.push(lines[j]);
				j++;
			}
			container(m[1].toLowerCase(), m[2], inner, parent, ctx);
			i = j + 1;
			continue;
		}

		m = RE_HEADING.exec(line);
		if (m) {
			const level = m[1].length;
			const nodes = inline(m[2], ctx);
			const text = plainText(nodes);
			const id = uniqueId(text, ctx);
			if (level >= 2 && level <= 3) ctx.headings.push({ level, text, id });
			if (level === 1 && !ctx.headings.some((x) => x.level === 1)) ctx.headings.push({ level, text, id });
			parent.append(h("h" + level, { id: (ctx.opts.idPrefix || "") + "md-" + id, class: "md-h" }, nodes));
			i++;
			continue;
		}

		if (RE_HR.test(line)) {
			parent.append(h("hr", { class: "md-hr" }));
			i++;
			continue;
		}

		if (RE_QUOTE.test(line)) {
			const inner = [];
			while (i < lines.length && RE_QUOTE.test(lines[i])) {
				inner.push(RE_QUOTE.exec(lines[i])[1]);
				i++;
			}
			quote(inner, parent, ctx);
			continue;
		}

		// Table: a header row followed by a delimiter row.
		if (line.includes("|") && i + 1 < lines.length && RE_TABLE_DELIM.test(lines[i + 1]) && lines[i + 1].includes("-")) {
			i = table(lines, i, parent, ctx);
			continue;
		}

		if (RE_LIST.test(line)) {
			i = list(lines, i, parent, ctx);
			continue;
		}

		// Paragraph: runs until a blank line or the start of another block.
		const para = [];
		while (i < lines.length && !/^\s*$/.test(lines[i]) && !startsBlock(lines, i, para.length > 0)) {
			para.push(lines[i]);
			i++;
		}
		if (para.length === 0) {
			// A line that looks like a block start but did not parse as one.
			para.push(lines[i]);
			i++;
		}
		paragraph(para, parent, ctx);
	}
}

function startsBlock(lines, i, inParagraph) {
	const line = lines[i];
	if (!inParagraph) return false;
	if (RE_FENCE.test(line) || RE_HEADING.test(line) || RE_HR.test(line) || RE_QUOTE.test(line)) return true;
	if (RE_CONTAINER_OPEN.test(line) || RE_CONTAINER_CLOSE.test(line)) return true;
	if (RE_LIST.test(line)) return true;
	if (line.includes("|") && i + 1 < lines.length && RE_TABLE_DELIM.test(lines[i + 1]) && lines[i + 1].includes("-")) return true;
	return false;
}

function paragraph(para, parent, ctx) {
	const text = para.map((l) => l.replace(/^\s+/, "")).join("\n");
	const nodes = inline(text, ctx);
	// A paragraph that is just one image with a title becomes a figure.
	if (nodes.length === 1 && nodes[0].nodeType === 1 && nodes[0].dataset && nodes[0].dataset.mdImage === "1") {
		const img = nodes[0];
		const caption = img.getAttribute("data-caption");
		const fig = h("figure", { class: "md-figure" }, img);
		if (caption) fig.append(h("figcaption", {}, caption));
		parent.append(fig);
		return;
	}
	parent.append(h("p", {}, nodes));
}

function quote(inner, parent, ctx) {
	// GitHub alert: the first line is [!NOTE], [!TIP] ...
	const m = /^\s*\[!(\w+)\]\s*(.*)$/.exec(inner[0] || "");
	if (m && CALLOUTS[m[1].toLowerCase()]) {
		const kind = m[1].toLowerCase();
		const rest = m[2] ? [m[2], ...inner.slice(1)] : inner.slice(1);
		callout(kind, CALLOUTS[kind], rest, parent, ctx);
		return;
	}
	const bq = h("blockquote", { class: "md-quote" });
	blocks(inner, bq, ctx);
	parent.append(bq);
}

function callout(kind, title, inner, parent, ctx) {
	const box = h("aside", { class: `md-callout ${kind}`, role: "note" }, h("p", { class: "md-callout-title" }, title));
	blocks(inner, box, ctx);
	parent.append(box);
}

function container(kind, title, inner, parent, ctx) {
	if (kind === "details" || kind === "collapse") {
		const d = h("details", { class: "md-details" });
		d.append(h("summary", {}, inline(title || "Details", ctx)));
		blocks(inner, d, ctx);
		parent.append(d);
		return;
	}
	if (kind === "columns") {
		// Columns are separated by a line holding only "|||".
		const cols = [[]];
		for (const l of inner) {
			if (/^\s*\|\|\|\s*$/.test(l)) cols.push([]);
			else cols[cols.length - 1].push(l);
		}
		const row = h("div", { class: "md-columns", style: `--cols: ${cols.length}` });
		for (const c of cols) {
			const col = h("div", { class: "md-column" });
			blocks(c, col, ctx);
			row.append(col);
		}
		parent.append(row);
		return;
	}
	const known = CALLOUTS[kind] ? kind : "note";
	callout(known, title || CALLOUTS[known], inner, parent, ctx);
}

// ---- tables ---------------------------------------------------------------

function splitRow(line) {
	let s = line.trim();
	if (s.startsWith("|")) s = s.slice(1);
	if (s.endsWith("|") && !s.endsWith("\\|")) s = s.slice(0, -1);
	const cells = [];
	let cur = "";
	for (let k = 0; k < s.length; k++) {
		const c = s[k];
		if (c === "\\" && s[k + 1] === "|") {
			cur += "|";
			k++;
		} else if (c === "`") {
			// A pipe inside a code span is part of the code.
			const end = s.indexOf("`", k + 1);
			if (end > k) {
				cur += s.slice(k, end + 1);
				k = end;
			} else cur += c;
		} else if (c === "|") {
			cells.push(cur.trim());
			cur = "";
		} else cur += c;
	}
	cells.push(cur.trim());
	return cells;
}

function table(lines, start, parent, ctx) {
	const head = splitRow(lines[start]);
	const aligns = splitRow(lines[start + 1]).map((c) => {
		const l = c.startsWith(":");
		const r = c.endsWith(":");
		return l && r ? "center" : r ? "right" : l ? "left" : "";
	});
	let i = start + 2;
	const rows = [];
	while (i < lines.length && lines[i].includes("|") && !/^\s*$/.test(lines[i])) {
		rows.push(splitRow(lines[i]));
		i++;
	}
	const cell = (tag, text, k) => {
		const td = h(tag, { style: aligns[k] ? `text-align: ${aligns[k]}` : null });
		td.append(...inline(text, ctx));
		return td;
	};
	const thead = h("thead", {}, h("tr", {}, head.map((t, k) => cell("th", t, k))));
	const tbody = h(
		"tbody",
		{},
		rows.map((r) => h("tr", {}, head.map((_, k) => cell("td", r[k] || "", k))))
	);
	parent.append(h("div", { class: "md-table-wrap" }, h("table", { class: "md-table" }, thead, tbody)));
	return i;
}

// ---- lists ----------------------------------------------------------------

function indentOf(s) {
	return /^\s*/.exec(s)[0].length;
}

function list(lines, start, parent, ctx) {
	const first = RE_LIST.exec(lines[start]);
	const base = first[1].length;
	const ordered = /\d/.test(first[2]);
	const listEl = h(ordered ? "ol" : "ul", { class: "md-list" });
	if (ordered) {
		const n = parseInt(first[2], 10);
		if (n !== 1) listEl.setAttribute("start", String(n));
	}
	let i = start;
	while (i < lines.length) {
		const m = RE_LIST.exec(lines[i]);
		if (!m || m[1].length < base || m[1].length > base + 1) {
			// Not a sibling item: a deeper one is handled below, anything
			// else ends the list.
			break;
		}
		if (/\d/.test(m[2]) !== ordered) break;
		const markerWidth = m[1].length + m[2].length + 1;
		const body = [m[3]];
		i++;
		// Continuation lines and nested blocks are indented past the marker.
		while (i < lines.length) {
			const l = lines[i];
			if (/^\s*$/.test(l)) {
				// A blank line continues the item only if more indented
				// content follows.
				let k = i + 1;
				while (k < lines.length && /^\s*$/.test(lines[k])) k++;
				if (k < lines.length && indentOf(lines[k]) > base && !(RE_LIST.test(lines[k]) && indentOf(lines[k]) <= base + 1)) {
					body.push("");
					i++;
					continue;
				}
				break;
			}
			const sib = RE_LIST.exec(l);
			if (sib && sib[1].length <= base + 1) break;
			if (indentOf(l) <= base && !sib) {
				// Lazy continuation of the paragraph, unless it opens a block.
				if (startsBlock([l, ""], 0, true)) break;
			}
			body.push(l.slice(Math.min(indentOf(l), markerWidth)));
			i++;
		}
		const li = h("li", {});
		let text = body;
		const task = /^\[([ xX])\]\s+(.*)$/.exec(body[0]);
		if (task) {
			li.classList.add("task");
			li.append(h("input", { type: "checkbox", disabled: true, checked: task[1] !== " " ? true : null, "aria-hidden": "true" }), " ");
			text = [task[2], ...body.slice(1)];
		}
		const holder = h("div", {});
		blocks(text, holder, ctx);
		// A tight item: unwrap its single paragraph.
		const kids = [...holder.childNodes];
		if (kids.length && kids[0].tagName === "P") {
			li.append(...kids[0].childNodes);
			kids.slice(1).forEach((k) => li.append(k));
		} else {
			kids.forEach((k) => li.append(k));
		}
		listEl.append(li);
	}
	parent.append(listEl);
	return i;
}

// ---------------------------------------------------------------------------
// Inline
// ---------------------------------------------------------------------------

function plainText(nodes) {
	return nodes.map((n) => n.textContent || "").join("");
}

function uniqueId(text, ctx) {
	let base = text
		.toLowerCase()
		.replace(/[^\p{L}\p{N}\s-]/gu, "")
		.trim()
		.replace(/\s+/g, "-");
	if (!base) base = "section";
	const n = ctx.used.get(base) || 0;
	ctx.used.set(base, n + 1);
	return n ? `${base}-${n}` : base;
}

const SAFE_LINK = /^(https?:|mailto:|#|tab:)/i;

/** Finds the index of the bracket closing the one at `open`, or -1. */
function matching(text, open, left, right) {
	let depth = 0;
	for (let k = open; k < text.length; k++) {
		const c = text[k];
		if (c === "\\") {
			k++;
			continue;
		}
		if (c === "`") {
			const end = text.indexOf("`", k + 1);
			if (end > k) k = end;
			continue;
		}
		if (c === left) depth++;
		else if (c === right) {
			depth--;
			if (depth === 0) return k;
		}
	}
	return -1;
}

/** Parses "(url "title")" starting at `at`; returns {url, title, end} or null. */
function target(text, at) {
	if (text[at] !== "(") return null;
	const end = matching(text, at, "(", ")");
	if (end < 0) return null;
	let inner = text.slice(at + 1, end).trim();
	let title = "";
	const tm = /^(.*?)\s+"([^"]*)"$/.exec(inner) || /^(.*?)\s+'([^']*)'$/.exec(inner);
	if (tm) {
		inner = tm[1];
		title = tm[2];
	}
	if (inner.startsWith("<") && inner.endsWith(">")) inner = inner.slice(1, -1);
	return { url: inner, title, end };
}

// Character references the guides may use (&nbsp; and the like). They become
// text, never markup.
const ENTITY = /^&(nbsp|amp|lt|gt|quot|apos|mdash|ndash|hellip|middot|times|#\d{1,6}|#x[0-9a-f]{1,5});/i;
const NAMED_ENTITIES = { nbsp: "\u00a0", amp: "&", lt: "<", gt: ">", quot: '"', apos: "'", mdash: "\u2014", ndash: "\u2013", hellip: "\u2026", middot: "\u00b7", times: "\u00d7" };

function decodeEntity(name) {
	const lower = name.toLowerCase();
	if (NAMED_ENTITIES[lower]) return NAMED_ENTITIES[lower];
	const code = lower[1] === "x" ? parseInt(lower.slice(2), 16) : parseInt(lower.slice(1), 10);
	return code > 0 && code < 0x110000 && !(code >= 0xd800 && code < 0xe000) ? String.fromCodePoint(code) : "";
}

function inline(text, ctx) {
	const out = [];
	let buf = "";
	const flush = () => {
		if (buf) {
			out.push(document.createTextNode(buf));
			buf = "";
		}
	};
	let i = 0;
	while (i < text.length) {
		const c = text[i];

		if (c === "\\" && i + 1 < text.length && /[\\`*_{}[\]()#+\-.!|~<>:]/.test(text[i + 1])) {
			buf += text[i + 1];
			i += 2;
			continue;
		}
		if (c === "&") {
			const ent = ENTITY.exec(text.slice(i, i + 12));
			if (ent) {
				buf += decodeEntity(ent[1]);
				i += ent[0].length;
				continue;
			}
		}
		if (c === "\n") {
			// A hard break is two trailing spaces or a backslash.
			if (/( {2,}|\\)$/.test(buf)) {
				buf = buf.replace(/( {2,}|\\)$/, "");
				flush();
				out.push(h("br"));
			} else {
				buf = buf.replace(/ +$/, "") + " ";
			}
			i++;
			continue;
		}
		if (c === "`") {
			let n = 1;
			while (text[i + n] === "`") n++;
			const fence = "`".repeat(n);
			const end = text.indexOf(fence, i + n);
			if (end > i) {
				flush();
				out.push(h("code", { class: "md-inline-code" }, text.slice(i + n, end).replace(/^ (.*) $/, "$1")));
				i = end + n;
				continue;
			}
			buf += fence;
			i += n;
			continue;
		}
		if (c === "!" && text[i + 1] === "[") {
			const close = matching(text, i + 1, "[", "]");
			const t = close > 0 ? target(text, close + 1) : null;
			if (t) {
				flush();
				out.push(image(text.slice(i + 2, close), t.url, t.title, ctx));
				i = t.end + 1;
				continue;
			}
		}
		if (c === "[" && text[i + 1] === "[") {
			// [[Enter]] is a keyboard key.
			const end = text.indexOf("]]", i + 2);
			if (end > i + 2) {
				flush();
				out.push(h("kbd", {}, text.slice(i + 2, end)));
				i = end + 2;
				continue;
			}
		}
		if (c === "[") {
			const close = matching(text, i, "[", "]");
			const t = close > 0 ? target(text, close + 1) : null;
			if (t) {
				flush();
				out.push(link(inline(text.slice(i + 1, close), ctx), t.url, t.title, ctx));
				i = t.end + 1;
				continue;
			}
		}
		if (c === "<") {
			const m = /^<(https?:\/\/[^\s<>]+)>/.exec(text.slice(i));
			if (m) {
				flush();
				out.push(link([document.createTextNode(m[1])], m[1], "", ctx));
				i += m[0].length;
				continue;
			}
		}
		if (c === "~" && text[i + 1] === "~") {
			const end = text.indexOf("~~", i + 2);
			if (end > i + 2) {
				flush();
				out.push(h("del", {}, inline(text.slice(i + 2, end), ctx)));
				i = end + 2;
				continue;
			}
		}
		if (c === "*" || c === "_") {
			const double = text[i + 1] === c;
			const mark = double ? c + c : c;
			const prev = i > 0 ? text[i - 1] : " ";
			const canOpen = c === "*" || !/[\p{L}\p{N}]/u.test(prev);
			if (canOpen) {
				let end = -1;
				let from = i + mark.length;
				while (from < text.length) {
					const k = text.indexOf(mark, from);
					if (k < 0) break;
					const before = text[k - 1];
					const after = text[k + mark.length];
					const okClose = k > i + mark.length && !/\s/.test(before) && (c === "*" || !/[\p{L}\p{N}]/u.test(after || " "));
					// A single mark must not be half of a double one.
					const lone = double || (text[k + 1] !== c && before !== c);
					if (okClose && lone) {
						end = k;
						break;
					}
					from = k + mark.length;
				}
				if (end > 0 && !/\s/.test(text[i + mark.length])) {
					flush();
					out.push(h(double ? "strong" : "em", {}, inline(text.slice(i + mark.length, end), ctx)));
					i = end + mark.length;
					continue;
				}
			}
		}
		buf += c;
		i++;
	}
	flush();
	return out;
}

function link(children, url, title, ctx) {
	const href = url.trim();
	if (!SAFE_LINK.test(href)) {
		// Not a link we will follow: show the text only.
		return h("span", { class: "md-badlink", title: title || null }, children);
	}
	const a = h("a", { href, title: title || null, class: "md-link" }, children);
	if (href.startsWith("#")) {
		a.addEventListener("click", (ev) => {
			ev.preventDefault();
			const id = href.slice(1);
			if (ctx.opts.onAnchor) ctx.opts.onAnchor(id);
			else {
				const el = document.getElementById((ctx.opts.idPrefix || "") + "md-" + id);
				if (el) el.scrollIntoView({ block: "start" });
			}
		});
	} else if (href.toLowerCase().startsWith("tab:")) {
		const [tab, anchor] = href.slice(4).split("#");
		a.setAttribute("href", "#" + tab);
		a.addEventListener("click", (ev) => {
			ev.preventDefault();
			if (ctx.opts.onTab) ctx.opts.onTab(tab, anchor || "");
		});
	} else {
		a.setAttribute("target", "_blank");
		a.setAttribute("rel", "noopener noreferrer");
	}
	return a;
}

function image(alt, src, title, ctx) {
	let altText = alt;
	let width = 0;
	const wm = /^(.*)\|(\d{1,4})$/.exec(alt);
	if (wm) {
		altText = wm[1];
		width = parseInt(wm[2], 10);
	}
	const u = src.trim();
	const pm = /^piece:([\w-]+)(?::([\w-]+))?$/.exec(u);
	if (pm && ctx.opts.piece) {
		const node = ctx.opts.piece(pm[1], pm[2] || "");
		if (node) {
			node.classList.add("md-icon");
			node.setAttribute("role", "img");
			node.setAttribute("aria-label", altText || pm[1]);
			if (title) node.setAttribute("title", title);
			return node;
		}
	}
	const mm = /^marker:([\w-]+)(?::(.+))?$/.exec(u);
	if (mm && ctx.opts.marker) {
		const node = ctx.opts.marker(mm[1], mm[2] || "");
		if (node) {
			node.classList.add("md-icon");
			node.setAttribute("role", "img");
			node.setAttribute("aria-label", altText || mm[1]);
			if (title) node.setAttribute("title", title);
			return node;
		}
	}
	const sm = /^seat:(\d+)$/.exec(u);
	if (sm && ctx.opts.seat) {
		const node = ctx.opts.seat(parseInt(sm[1], 10));
		if (node) return node;
	}
	// Only the game's own files are loaded: relative paths under assetBase.
	if (/^[a-z][a-z0-9+.-]*:/i.test(u) || u.startsWith("//") || u.startsWith("/") || u.includes("..")) {
		return h("span", { class: "md-badimage" }, altText || "image");
	}
	const url = (ctx.opts.assetBase || "") + u.replace(/^\.\//, "");
	const img = h("img", {
		class: "md-img",
		src: url,
		alt: altText,
		title: title || null,
		loading: "lazy",
		decoding: "async",
		width: width || null,
		"data-md-image": "1",
		"data-caption": title || null,
	});
	return img;
}
